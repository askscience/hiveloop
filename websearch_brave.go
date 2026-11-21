package main

// Brave Search API (https://api.search.brave.com). The key is stored in the settings
// table and managed by the super admin in Gestione (HIVELOOP_BRAVE_KEY works as a fallback).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// braveKeyFn is set at startup; it returns the configured Brave key ("" = not configured).
var braveKeyFn = func() string { return "" }

func (a *App) braveKey() string {
	if k := strings.TrimSpace(a.db.Setting("brave_api_key")); k != "" {
		return k
	}
	return strings.TrimSpace(env("HIVELOOP_BRAVE_KEY", ""))
}

var errBraveQuota = errors.New("brave: quota or rate limit reached")

func braveSearch(ctx context.Context, key, query, lang string, max int) ([]webResult, error) {
	country := "IT"
	if lang == "en" {
		country = "US"
	}
	q := url.Values{"q": {query}, "count": {fmt.Sprint(max)}, "country": {country}, "search_lang": {lang}, "safesearch": {"moderate"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.search.brave.com/res/v1/web/search?"+q.Encode(), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", key)
	resp, err := webClient.Do(req)
	if err != nil {
		return nil, errors.New("brave unreachable")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
	case 401, 403, 422:
		return nil, errors.New("brave: invalid key")
	case 429:
		return nil, errBraveQuota
	default:
		return nil, fmt.Errorf("brave: status %d", resp.StatusCode)
	}
	var r struct {
		Web struct {
			Results []struct {
				Title       string   `json:"title"`
				URL         string   `json:"url"`
				Description string   `json:"description"`
				Extra       []string `json:"extra_snippets"`
			} `json:"results"`
		} `json:"web"`
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	var out []webResult
	for _, x := range r.Web.Results {
		snip := stripTags(x.Description)
		if len(x.Extra) > 0 {
			snip += " … " + stripTags(x.Extra[0])
		}
		out = append(out, webResult{Title: stripTags(x.Title), URL: x.URL, Snippet: snip})
	}
	return out, nil
}

func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("&quot;", `"`, "&#x27;", "'", "&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(b.String())
}

// ---------- Gestione (super admin) ----------

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "••••"
	}
	return "••••" + k[len(k)-4:]
}

func (a *App) handleGetSearchSettings(w http.ResponseWriter, r *http.Request) {
	k := a.braveKey()
	jsonOK(w, map[string]any{"brave": k != "", "hint": maskKey(k), "from_env": a.db.Setting("brave_api_key") == "" && k != ""})
}

func (a *App) handleSetSearchSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		BraveKey string `json:"brave_key"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	k := strings.TrimSpace(in.BraveKey)
	if k != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if _, err := braveSearch(ctx, k, "test", "it", 1); err != nil && !errors.Is(err, errBraveQuota) {
			jsonErr(w, 400, "brave_key_invalid")
			return
		}
	}
	a.db.SetSetting("brave_api_key", k)
	searchCacheMu.Lock()
	searchCache = map[string]cachedSearch{}
	searchCacheMu.Unlock()
	log.Printf("search settings: brave key %s by %s", map[bool]string{true: "set", false: "removed"}[k != ""], currentUser(r).Username)
	a.handleGetSearchSettings(w, r)
}

func (a *App) handleTestSearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	const q = "open source community calendar"
	if k := a.braveKey(); k != "" { // test Brave itself, bypassing cache and fallbacks
		res, err := braveSearch(ctx, k, q, "it", 3)
		if err != nil {
			jsonErr(w, 502, err.Error())
			return
		}
		jsonOK(w, map[string]any{"engine": "brave", "results": res})
		return
	}
	res, engine, err := searchWeb(ctx, q, "it-it", 3)
	if err != nil {
		jsonErr(w, 502, err.Error())
		return
	}
	jsonOK(w, map[string]any{"engine": engine, "results": res})
}
