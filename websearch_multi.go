package main

// Web search with fallbacks. DuckDuckGo blocks automated queries from an address after a
// while ("anomaly" page), so we try DuckDuckGo → Bing → Wikipedia and cache results briefly.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

type cachedSearch struct {
	at      time.Time
	engine  string
	results []webResult
}

var (
	searchCache   = map[string]cachedSearch{}
	searchCacheMu sync.Mutex
)

// searchWeb returns results and the engine that produced them.
func searchWeb(ctx context.Context, query, region string, max int) ([]webResult, string, error) {
	key := strings.ToLower(strings.TrimSpace(query)) + "|" + region
	searchCacheMu.Lock()
	if c, ok := searchCache[key]; ok && time.Since(c.at) < 10*time.Minute {
		searchCacheMu.Unlock()
		return c.results, c.engine, nil
	}
	searchCacheMu.Unlock()

	lang := "en"
	if strings.HasPrefix(region, "it") || strings.HasSuffix(region, "-it") {
		lang = "it"
	}
	type engine struct {
		name string
		fn   func() ([]webResult, error)
	}
	engines := []engine{}
	if k := braveKeyFn(); k != "" {
		engines = append(engines, engine{"brave", func() ([]webResult, error) { return braveSearch(ctx, k, query, lang, max) }})
	}
	engines = append(engines, []engine{
		{"duckduckgo", func() ([]webResult, error) { return webSearch(ctx, query, region, max) }},
		{"bing", func() ([]webResult, error) { return bingSearch(ctx, query, lang, max) }},
		{"wikipedia", func() ([]webResult, error) { return wikiSearch(ctx, query, lang, max) }},
	}...)
	var errs []string
	for _, e := range engines {
		res, err := e.fn()
		if err == nil && len(res) > 0 {
			searchCacheMu.Lock()
			if len(searchCache) > 500 {
				searchCache = map[string]cachedSearch{}
			}
			searchCache[key] = cachedSearch{time.Now(), e.name, res}
			searchCacheMu.Unlock()
			return res, e.name, nil
		}
		if err == nil {
			err = errors.New("no results")
		}
		errs = append(errs, e.name+": "+err.Error())
	}
	return nil, "", fmt.Errorf("search unavailable (%s)", strings.Join(errs, "; "))
}

func getPage(ctx context.Context, u, lang string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", webUA)
	req.Header.Set("Accept-Language", lang+";q=0.9,en;q=0.6")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.5")
	resp, err := webClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 3<<20))
}

// bingTarget unwraps Bing's click-tracking links (…/ck/a?…&u=a1<base64url>).
func bingTarget(h string) string {
	u, err := url.Parse(h)
	if err != nil || !strings.HasSuffix(u.Host, "bing.com") {
		return h
	}
	enc := u.Query().Get("u")
	if len(enc) < 3 {
		return h
	}
	enc = enc[2:] // "a1" prefix
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(enc, "=")); err == nil && strings.HasPrefix(string(b), "http") {
		return string(b)
	}
	return h
}

func bingSearch(ctx context.Context, query, lang string, max int) ([]webResult, error) {
	cc := "IT"
	if lang == "en" {
		cc = "US"
	}
	body, err := getPage(ctx, "https://www.bing.com/search?"+url.Values{"q": {query}, "setlang": {lang}, "cc": {cc}}.Encode(), lang)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(string(body[:min(len(body), 4000)])), "captcha") {
		return nil, errors.New("captcha")
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	var out []webResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(out) >= max {
			return
		}
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "b_algo") {
			var r webResult
			var find func(*html.Node)
			find = func(c *html.Node) {
				if c.Type == html.ElementNode {
					if c.Data == "h2" && r.URL == "" {
						for a := c.FirstChild; a != nil; a = a.NextSibling {
							if a.Type == html.ElementNode && a.Data == "a" {
								r.Title, r.URL = textOf(a), bingTarget(attr(a, "href"))
							}
						}
					} else if c.Data == "p" && r.Snippet == "" {
						r.Snippet = textOf(c)
					}
				}
				for k := c.FirstChild; k != nil; k = k.NextSibling {
					find(k)
				}
			}
			find(n)
			if r.URL != "" && r.Title != "" && strings.HasPrefix(r.URL, "http") {
				r.Snippet = strings.TrimPrefix(r.Snippet, "Web")
				out = append(out, r)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

func wikiSearch(ctx context.Context, query, lang string, max int) ([]webResult, error) {
	api := fmt.Sprintf("https://%s.wikipedia.org/w/api.php?", lang) + url.Values{
		"action": {"query"}, "list": {"search"}, "srsearch": {query}, "format": {"json"}, "srlimit": {fmt.Sprint(max)},
	}.Encode()
	body, err := getPage(ctx, api, lang)
	if err != nil {
		return nil, err
	}
	var r struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	var out []webResult
	for _, s := range r.Query.Search {
		snip := s.Snippet
		if doc, err := html.Parse(strings.NewReader(snip)); err == nil {
			snip = textOf(doc)
		}
		out = append(out, webResult{
			Title:   s.Title + " — Wikipedia",
			URL:     fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(strings.ReplaceAll(s.Title, " ", "_"))),
			Snippet: snip,
		})
	}
	return out, nil
}
