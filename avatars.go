package main

// Cartoon avatars (Avataaars style, drawn in the browser by a bundled library). The server
// only stores each user's choices: a small JSON object of option → value.

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
)

var (
	avatarKeys = map[string]bool{"top": true, "hairColor": true, "hatColor": true, "accessories": true, "accessoriesColor": true,
		"facialHair": true, "facialHairColor": true, "clothing": true, "clothesColor": true, "clothingGraphic": true,
		"eyes": true, "eyebrows": true, "mouth": true, "skinColor": true, "backgroundColor": true}
	avatarVal = regexp.MustCompile(`^[a-zA-Z0-9]{1,32}$`)
)

// cleanAvatar keeps only known options with simple values ("none" = without, e.g. no beard).
func cleanAvatar(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		if avatarKeys[k] && avatarVal.MatchString(v) {
			out[k] = v
		}
	}
	return out
}

func (a *App) setAvatar(uid int64, av map[string]string) {
	if av == nil || len(av) == 0 {
		a.db.Exec(`UPDATE users SET avatar='' WHERE id=?`, uid)
	} else {
		b, _ := json.Marshal(cleanAvatar(av))
		a.db.Exec(`UPDATE users SET avatar=? WHERE id=?`, string(b), uid)
	}
	a.liveAll("avatars")
}

func (a *App) userAvatar(uid int64) map[string]string {
	var raw string
	a.db.QueryRow(`SELECT avatar FROM users WHERE id=?`, uid).Scan(&raw)
	var av map[string]string
	if json.Unmarshal([]byte(raw), &av) != nil || len(av) == 0 {
		return nil
	}
	return cleanAvatar(av)
}

// handleAvatars: every active user's avatar choices, keyed by user id.
func (a *App) handleAvatars(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT id,avatar FROM users WHERE avatar<>'' AND status='active'`)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := map[string]map[string]string{} // user id -> choices; "ai" = the assistant
	for rows.Next() {
		var id int64
		var raw string
		var av map[string]string
		if rows.Scan(&id, &raw) == nil && json.Unmarshal([]byte(raw), &av) == nil && len(av) > 0 {
			out[strconv.FormatInt(id, 10)] = cleanAvatar(av)
		}
	}
	rows.Close() // release the connection before the next query
	if ai := a.aiAvatar(); ai != nil {
		out["ai"] = ai
	}
	jsonOK(w, out)
}
