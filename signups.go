package main

// Event sign-ups: people book themselves for an event, for the whole event (default)
// or for a time slot inside it (e.g. a shift at a stand).

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Signup struct {
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Start    string `json:"start"`
	End      string `json:"end"`
	AllDay   bool   `json:"all_day"`
	Note     string `json:"note"`
}

type SignupInput struct {
	AllDay *bool   `json:"all_day"`
	Start  *string `json:"start"`
	End    *string `json:"end"`
	Note   *string `json:"note"`
}

// attachSignups fills Participants/MySignup for a list of events.
func (a *App) attachSignups(u *User, evs []Event) {
	if len(evs) == 0 {
		return
	}
	a.attachShifts(u, evs)
	idx := map[int64]int{}
	ids := make([]string, len(evs))
	for i, e := range evs {
		idx[e.ID] = i
		ids[i] = strconv.FormatInt(e.ID, 10)
		evs[i].Participants = []Signup{}
	}
	rows, err := a.db.Query(`SELECT s.event_id,s.user_id,COALESCE(NULLIF(u.name,''),u.username),u.username,s.start_at,s.end_at,s.all_day,s.note
		FROM event_signups s JOIN users u ON u.id=s.user_id WHERE s.event_id IN (` + strings.Join(ids, ",") + `) ORDER BY s.start_at, u.name COLLATE NOCASE`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var eid, st, en int64
		var s Signup
		rows.Scan(&eid, &s.UserID, &s.Name, &s.Username, &st, &en, &s.AllDay, &s.Note)
		s.Start, s.End = a.fmtTime(st), a.fmtTime(en)
		e := &evs[idx[eid]]
		e.Participants = append(e.Participants, s)
		if s.UserID == u.ID {
			c := s
			e.MySignup = &c
		}
	}
}

// signup books u for the event. Without times (or all_day) it covers the whole event.
func (a *App) signup(u *User, eventID int64, in SignupInput) (Event, error) {
	e, err := a.getEvent(u, eventID)
	if err != nil {
		return e, err
	}
	es, _ := time.Parse(time.RFC3339, e.Start)
	ee, _ := time.Parse(time.RFC3339, e.End)
	start, end := es, ee
	allDay := in.AllDay == nil || *in.AllDay
	if in.Start != nil && *in.Start != "" {
		allDay = in.AllDay != nil && *in.AllDay
	}
	if !allDay {
		if in.Start == nil || *in.Start == "" {
			return e, errBadInput
		}
		if start, err = a.parseTime(*in.Start); err != nil {
			return e, err
		}
		if in.End != nil && *in.End != "" {
			if end, err = a.parseTime(*in.End); err != nil {
				return e, err
			}
		} else {
			end = ee
		}
		// clamp to the event span
		if start.Before(es) {
			start = es
		}
		if end.After(ee) {
			end = ee
		}
		if !end.After(start) {
			return e, fmt.Errorf("%w: time slot outside the event", errBadInput)
		}
	}
	note := strings.TrimSpace(deref(in.Note))
	if len([]rune(note)) > 300 {
		note = string([]rune(note)[:300])
	}
	_, err = a.db.Exec(`INSERT INTO event_signups(event_id,user_id,start_at,end_at,all_day,note,created_at) VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(event_id,user_id) DO UPDATE SET start_at=excluded.start_at,end_at=excluded.end_at,all_day=excluded.all_day,note=excluded.note`,
		eventID, u.ID, start.Unix(), end.Unix(), allDay, note, time.Now().Unix())
	if err != nil {
		return e, err
	}
	e, err = a.getEventFull(u, eventID)
	if err == nil {
		a.liveEvent(u, "signup", e)
	}
	return e, err
}

func (a *App) cancelSignup(u *User, eventID, userID int64) (Event, error) {
	e, err := a.getEvent(u, eventID)
	if err != nil {
		return e, err
	}
	if userID != u.ID && !e.CanEdit {
		return e, errForbidden
	}
	a.db.Exec(`DELETE FROM event_signups WHERE event_id=? AND user_id=?`, eventID, userID)
	e, err = a.getEventFull(u, eventID)
	if err == nil {
		a.liveEvent(u, "signup", e)
	}
	return e, err
}

func (a *App) getEventFull(u *User, id int64) (Event, error) {
	e, err := a.getEvent(u, id)
	if err != nil {
		return e, err
	}
	evs := []Event{e}
	a.attachSignups(u, evs)
	return evs[0], nil
}

// ---------- HTTP ----------

func (a *App) handleSignup(w http.ResponseWriter, r *http.Request) {
	var in SignupInput
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	e, err := a.signup(currentUser(r), pathID(r, "id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}

func (a *App) handleCancelSignup(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	uid := u.ID
	if r.PathValue("uid") != "" {
		uid = pathID(r, "uid")
	}
	e, err := a.cancelSignup(u, pathID(r, "id"), uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}
