package main

// Per-user appearance preferences (theme, accent colour, button style), stored as JSON
// in users.prefs. Empty values mean "as the app is by default".

import "encoding/json"

type Prefs struct {
	Theme   string `json:"theme"`   // "" (system) | light | dark
	Accent  string `json:"accent"`  // "" (black/white) | one of accentNames
	Buttons string `json:"buttons"` // "" (icon + text) | icons | text
	Zoom    int    `json:"zoom"`    // 0 = 100 %; otherwise one of zoomLevels
	Vision  string `json:"vision"`  // "" | "high": theme for people with low vision
}

var zoomLevels = map[int]bool{80: true, 90: true, 110: true, 125: true, 150: true, 175: true}

var accentNames = map[string]bool{"blue": true, "indigo": true, "violet": true, "pink": true, "red": true,
	"green": true, "teal": true, "cyan": true, "olive": true, "slate": true}

func (p Prefs) clean() Prefs {
	if p.Theme != "light" && p.Theme != "dark" {
		p.Theme = ""
	}
	if !accentNames[p.Accent] {
		p.Accent = ""
	}
	if p.Buttons != "icons" && p.Buttons != "text" {
		p.Buttons = ""
	}
	if !zoomLevels[p.Zoom] {
		p.Zoom = 0
	}
	if p.Vision != "high" {
		p.Vision = ""
	}
	return p
}

func (a *App) userPrefs(uid int64) Prefs {
	var raw string
	var p Prefs
	a.db.QueryRow(`SELECT prefs FROM users WHERE id=?`, uid).Scan(&raw)
	json.Unmarshal([]byte(raw), &p)
	return p.clean()
}

func (a *App) setUserPrefs(uid int64, p Prefs) {
	b, _ := json.Marshal(p.clean())
	a.db.Exec(`UPDATE users SET prefs=? WHERE id=?`, string(b), uid)
}
