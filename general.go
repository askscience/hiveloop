package main

// Appearance of the general chat (colour, photo) and the assistant's avatar, set by admins.
// The photo uses the group image storage with id 0 (files/_groups/0.img).

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type generalLook struct {
	Color  string `json:"color"`
	ImageV int64  `json:"image_v"`
}

func (a *App) generalLook() generalLook {
	v, _ := strconv.ParseInt(a.db.Setting("general_image_v"), 10, 64)
	return generalLook{Color: a.db.Setting("general_color"), ImageV: v}
}

func (a *App) aiAvatar() map[string]string {
	var av map[string]string
	if json.Unmarshal([]byte(a.db.Setting("ai_avatar")), &av) != nil || len(av) == 0 {
		return nil
	}
	return cleanAvatar(av)
}

func (a *App) handleSetGeneral(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Color string `json:"color"`
	}
	if decode(r, &in) != nil || (in.Color != "" && !colorRe.MatchString(in.Color)) {
		jsonErr(w, 400, "bad_request")
		return
	}
	a.db.SetSetting("general_color", in.Color)
	jsonOK(w, a.generalLook())
}

func (a *App) handleSetGeneralImage(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(io.LimitReader(r.Body, groupImageMax+1))
	if err != nil || len(b) == 0 || len(b) > groupImageMax {
		jsonErr(w, 400, "bad_request")
		return
	}
	out, err := optimizeImage(b)
	if err != nil {
		jsonErr(w, 415, "bad_image")
		return
	}
	p := a.groupImagePath(0)
	if os.MkdirAll(filepath.Dir(p), 0o750) != nil || writeFileAtomic(p, out) != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	a.db.SetSetting("general_image_v", strconv.FormatInt(time.Now().UnixNano()/1e6, 10))
	jsonOK(w, a.generalLook())
}

func (a *App) handleDeleteGeneralImage(w http.ResponseWriter, r *http.Request) {
	os.Remove(a.groupImagePath(0))
	a.db.SetSetting("general_image_v", "")
	jsonOK(w, a.generalLook())
}

func (a *App) handleSetAIAvatar(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Avatar map[string]string `json:"avatar"`
	}
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if len(in.Avatar) == 0 {
		a.db.SetSetting("ai_avatar", "")
	} else {
		b, _ := json.Marshal(cleanAvatar(in.Avatar))
		a.db.SetSetting("ai_avatar", string(b))
	}
	a.liveAll("avatars")
	jsonOK(w, map[string]any{"ok": true})
}

// ---------- assistant on/off (super admin) ----------

// aiSwitchOn: the master switch (super admin).
func (a *App) aiSwitchOn() bool { return a.db.Setting("ai_disabled") != "1" }

// aiEnabled: switch on and, if a schedule is set, inside its hours and days.
func (a *App) aiEnabled() bool {
	return a.aiSwitchOn() && a.aiSchedule().allows(time.Now().In(a.cfg.TZ))
}

// aiSchedule: optional hours/days when the assistant can be used.
type aiSchedule struct {
	On   bool   `json:"on"`
	From string `json:"from"` // "HH:MM"
	To   string `json:"to"`   // "HH:MM"; earlier than From = across midnight
	Days []int  `json:"days"` // 1 = Monday … 7 = Sunday (day the window starts)
}

func hhmm(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

func isoDay(t time.Time) int { return (int(t.Weekday())+6)%7 + 1 }

func (s aiSchedule) valid() bool {
	_, ok1 := hhmm(s.From)
	_, ok2 := hhmm(s.To)
	if !ok1 || !ok2 || s.From == s.To {
		return false
	}
	for _, d := range s.Days {
		if d < 1 || d > 7 {
			return false
		}
	}
	return true
}

func (s aiSchedule) allows(now time.Time) bool {
	if !s.On {
		return true
	}
	f, _ := hhmm(s.From)
	to, _ := hhmm(s.To)
	m := now.Hour()*60 + now.Minute()
	day := func(d int) bool {
		if len(s.Days) == 0 {
			return true
		}
		for _, x := range s.Days {
			if x == d {
				return true
			}
		}
		return false
	}
	if f < to {
		return day(isoDay(now)) && m >= f && m < to
	}
	// across midnight: the evening belongs to today, the early morning to yesterday's window
	if m >= f {
		return day(isoDay(now))
	}
	return m < to && day(isoDay(now.AddDate(0, 0, -1)))
}

func (a *App) aiSchedule() aiSchedule {
	var s aiSchedule
	if json.Unmarshal([]byte(a.db.Setting("ai_schedule")), &s) != nil || !s.valid() {
		return aiSchedule{}
	}
	return s
}

func (a *App) handleSetAISchedule(w http.ResponseWriter, r *http.Request) {
	var in aiSchedule
	if decode(r, &in) != nil || (in.On && !in.valid()) {
		jsonErr(w, 400, "bad_schedule")
		return
	}
	if !in.valid() { // turned off with incomplete hours: keep it simple
		in = aiSchedule{From: "08:00", To: "20:00"}
	}
	b, _ := json.Marshal(in)
	a.db.SetSetting("ai_schedule", string(b))
	log.Printf("assistant schedule %s by %s", b, currentUser(r).Username)
	a.liveAll("settings")
	jsonOK(w, map[string]any{"schedule": in, "active": a.aiEnabled()})
}

// aiOn guards every assistant endpoint: with the assistant off they answer 403.
func (a *App) aiOn(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.aiEnabled() {
			jsonErr(w, 403, "ai_disabled")
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (a *App) handleSetAI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	v := "1"
	if in.Enabled {
		v = ""
	}
	a.db.SetSetting("ai_disabled", v)
	log.Printf("assistant %s by %s", map[bool]string{true: "enabled", false: "disabled"}[in.Enabled], currentUser(r).Username)
	a.liveAll("settings")
	jsonOK(w, map[string]bool{"enabled": a.aiSwitchOn(), "active": a.aiEnabled()})
}
