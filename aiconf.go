package main

// AI provider settings, editable by the super admin in Manage. Values saved there override
// the environment (HIVELOOP_AI_PROVIDER, HIVELOOP_AI_URL, HIVELOOP_MODEL, HIVELOOP_AI_KEY), which remain the
// starting point. The API key is never sent back to the browser, only a masked hint.

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

type aiConf struct {
	Provider string // "ollama" or "openai" (any OpenAI-compatible API)
	URL      string
	Model    string
	Key      string
}

func (a *App) aiConf() aiConf {
	c := aiConf{a.cfg.AIProvider, a.cfg.AIURL, a.cfg.AIModel, a.cfg.AIKey}
	if v := a.db.Setting("ai_provider"); v == "ollama" || v == "openai" {
		c.Provider = v
	}
	if v := a.db.Setting("ai_url"); v != "" {
		c.URL = v
	}
	if v := a.db.Setting("ai_model"); v != "" {
		c.Model = v
	}
	c.URL = strings.TrimRight(strings.TrimSpace(c.URL), "/")
	c.Key, _ = a.keyFor(c.URL)
	return c
}

// keyFor returns the API key for an address and whether it comes from the environment. A key
// saved in Manage belongs to the address it was saved with, and the environment key to the
// environment address: switching provider never sends a key to another service.
func (a *App) keyFor(url string) (string, bool) {
	if k := a.db.Setting("ai_key"); k != "" {
		ku := a.db.Setting("ai_key_url")
		if ku == "" {
			ku = strings.TrimRight(a.db.Setting("ai_url"), "/")
		}
		if ku == url {
			return k, false
		}
	}
	if a.cfg.AIKey != "" && url == strings.TrimRight(strings.TrimSpace(a.cfg.AIURL), "/") {
		return a.cfg.AIKey, true
	}
	return "", false
}

func (a *App) aiConfView() map[string]any {
	c := a.aiConf()
	_, env := a.keyFor(c.URL)
	return map[string]any{"provider": c.Provider, "url": c.URL, "model": c.Model,
		"key_set": c.Key != "", "key_hint": maskKey(c.Key), "key_from_env": env}
}

type aiConfInput struct {
	Provider *string `json:"provider"`
	URL      *string `json:"url"`
	Model    *string `json:"model"`
	Key      *string `json:"key"`       // empty = keep the current key
	ClearKey bool    `json:"clear_key"` // remove the saved key
}

// merged: the current configuration with the (possibly unsaved) values from the form.
func (a *App) merged(in aiConfInput) (aiConf, bool) {
	c := a.aiConf()
	if in.Provider != nil {
		if *in.Provider != "ollama" && *in.Provider != "openai" {
			return c, false
		}
		c.Provider = *in.Provider
	}
	if in.URL != nil {
		u := strings.TrimRight(strings.TrimSpace(*in.URL), "/")
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") || len(u) > 300 {
			return c, false
		}
		c.URL = u
		c.Key, _ = a.keyFor(u)
	}
	if in.Model != nil {
		c.Model = clip(*in.Model, 200)
	}
	if in.ClearKey {
		c.Key = ""
	}
	if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
		c.Key = strings.TrimSpace(*in.Key)
	}
	return c, c.URL != "" && c.Model != ""
}

func (a *App) handleGetAIConf(w http.ResponseWriter, r *http.Request) { jsonOK(w, a.aiConfView()) }

func (a *App) handleSetAIConf(w http.ResponseWriter, r *http.Request) {
	var in aiConfInput
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	c, ok := a.merged(in)
	if !ok {
		jsonErr(w, 400, "ai_conf_invalid")
		return
	}
	a.db.SetSetting("ai_provider", c.Provider)
	a.db.SetSetting("ai_url", c.URL)
	a.db.SetSetting("ai_model", c.Model)
	if in.Key != nil && strings.TrimSpace(*in.Key) != "" {
		a.db.SetSetting("ai_key", c.Key)
		a.db.SetSetting("ai_key_url", c.URL)
	} else if in.ClearKey {
		a.db.SetSetting("ai_key", "")
		a.db.SetSetting("ai_key_url", "")
	}
	jsonOK(w, a.aiConfView())
}

// handleTestAIConf sends a tiny request with the form's values (saved or not) and lists the
// models the endpoint offers, so the super admin can check before saving.
func (a *App) handleTestAIConf(w http.ResponseWriter, r *http.Request) {
	var in aiConfInput
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	c, ok := a.merged(in)
	if !ok {
		jsonErr(w, 400, "ai_conf_invalid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	t0 := time.Now()
	msgs := []oMsg{{Role: "user", Content: "Reply with just the word OK."}}
	m, err := a.aiStreamWith(ctx, c, msgs, nil, func(string) {})
	out := map[string]any{"models": aiModels(ctx, c), "ms": time.Since(t0).Milliseconds()}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["reply"] = clip(m.Content, 200)
	}
	jsonOK(w, out)
}

// handleAIModels lists the models of the endpoint in the form (saved or not), with the key
// state for that address.
func (a *App) handleAIModels(w http.ResponseWriter, r *http.Request) {
	var in aiConfInput
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.Model == nil {
		x := "-"
		in.Model = &x // not needed to list models
	}
	c, ok := a.merged(in)
	if !ok {
		jsonErr(w, 400, "ai_conf_invalid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	_, env := a.keyFor(c.URL)
	jsonOK(w, map[string]any{"models": aiModels(ctx, c), "key_set": c.Key != "", "key_hint": maskKey(c.Key), "key_from_env": env && (in.Key == nil || *in.Key == "")})
}

// aiModels asks the endpoint which models it has (best effort; empty on failure).
func aiModels(ctx context.Context, c aiConf) []string {
	url := c.URL + "/api/tags"
	if c.Provider == "openai" {
		url = c.URL + "/models"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var r struct {
		Models []struct{ Name string } `json:"models"` // Ollama
		Data   []struct{ ID string }   `json:"data"`   // OpenAI
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&r) != nil {
		return nil
	}
	var out []string
	for _, m := range r.Models {
		out = append(out, m.Name)
	}
	for _, m := range r.Data {
		out = append(out, m.ID)
	}
	sort.Strings(out)
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
