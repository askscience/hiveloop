package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Event struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      bool   `json:"all_day"`
	GroupID     *int64 `json:"group_id"`
	GroupName   string `json:"group_name,omitempty"`
	GroupColor  string `json:"group_color,omitempty"`
	CreatedBy   *int64 `json:"created_by"`
	Author      string `json:"author,omitempty"`
	ViaAI       bool   `json:"via_ai"`
	Private     bool   `json:"private"`
	CanEdit     bool   `json:"can_edit"`
	AuthorUser  string `json:"author_username,omitempty"`

	Participants []Signup `json:"participants"`
	MySignup     *Signup  `json:"my_signup"`
	Shifts       []Shift  `json:"shifts"`
	Missing      int      `json:"missing"` // volunteers still needed in shifts that have not started
}

type EventInput struct {
	Title       *string       `json:"title"`
	Description *string       `json:"description"`
	Location    *string       `json:"location"`
	Start       *string       `json:"start"`
	End         *string       `json:"end"`
	AllDay      *bool         `json:"all_day"`
	GroupID     *int64        `json:"group_id"`
	ClearGroup  bool          `json:"clear_group"`
	Private     *bool         `json:"private"`
	Shifts      *[]ShiftInput `json:"shifts"` // nil = unchanged, [] = none
}

var (
	errForbidden = errors.New("forbidden")
	errNotFound  = errors.New("not_found")
	errBadInput  = errors.New("bad_request")
)

// canEditGroup: admins manage every group (and general events); users only groups they belong to.
func (a *App) canEditGroup(u *User, gid *int64) bool {
	if u.IsAdmin() {
		return true
	}
	return gid != nil && a.isMember(u.ID, *gid)
}

// canSeeEvent: private events are internal to their group (plus admins and the super admin).
func (a *App) canSeeEvent(u *User, e Event) bool {
	return !e.Private || u.IsAdmin() || (e.GroupID != nil && u.ID != 0 && a.isMember(u.ID, *e.GroupID))
}

// visibleSQL restricts an event query to what u may see.
func visibleSQL(u *User) (string, []any) {
	if u.IsAdmin() {
		return "", nil
	}
	return ` AND (e.private=0 OR e.group_id IN (SELECT group_id FROM group_members WHERE user_id=?))`, []any{u.ID}
}

func (a *App) parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, a.cfg.TZ); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errBadInput
}

func (a *App) fmtTime(unix int64) string {
	return time.Unix(unix, 0).In(a.cfg.TZ).Format(time.RFC3339)
}

const eventSelect = `SELECT e.id,e.title,e.description,e.location,e.start_at,e.end_at,e.all_day,e.group_id,
	COALESCE(g.name,''),COALESCE(g.color,''),e.created_by,COALESCE(NULLIF(u.name,''),u.username,''),e.via_ai,e.private,COALESCE(u.username,'')
	FROM events e LEFT JOIN groups g ON g.id=e.group_id LEFT JOIN users u ON u.id=e.created_by`

func (a *App) scanEvent(s interface{ Scan(...any) error }, u *User) (Event, error) {
	var e Event
	var start, end int64
	var gid, cby sql.NullInt64
	err := s.Scan(&e.ID, &e.Title, &e.Description, &e.Location, &start, &end, &e.AllDay, &gid, &e.GroupName, &e.GroupColor, &cby, &e.Author, &e.ViaAI, &e.Private, &e.AuthorUser)
	if err != nil {
		return e, err
	}
	e.Start, e.End = a.fmtTime(start), a.fmtTime(end)
	if gid.Valid {
		e.GroupID = &gid.Int64
	}
	if cby.Valid {
		e.CreatedBy = &cby.Int64
	}
	// u == nil while iterating rows: no query may run there (it would need a second pooled
	// connection while this one is busy, and under load every connection ends up waiting).
	if u != nil {
		e.CanEdit = a.canEditGroup(u, e.GroupID)
	}
	return e, nil
}

// markEventsEditable sets CanEdit after the rows are closed, with one membership query.
func (a *App) markEventsEditable(u *User, evs []Event) {
	if u.IsAdmin() {
		for i := range evs {
			evs[i].CanEdit = true
		}
		return
	}
	mine := map[int64]bool{}
	for _, g := range a.userGroupIDs(u.ID) {
		mine[g] = true
	}
	for i := range evs {
		evs[i].CanEdit = evs[i].GroupID != nil && mine[*evs[i].GroupID]
	}
}

// listEvents returns events overlapping [from,to). scope: "all", "mine" or a group id.
func (a *App) listEvents(u *User, from, to time.Time, scope string) ([]Event, error) {
	q := eventSelect + ` WHERE e.start_at < ? AND e.end_at >= ?`
	args := []any{to.Unix(), from.Unix()}
	switch {
	case scope == "booked":
		q += ` AND e.id IN (SELECT event_id FROM event_signups WHERE user_id=? UNION SELECT s.event_id FROM event_shifts s JOIN shift_signups x ON x.shift_id=s.id WHERE x.user_id=?)`
		args = append(args, u.ID, u.ID)
	case scope == "mine":
		q += ` AND (e.group_id IS NULL OR e.group_id IN (SELECT group_id FROM group_members WHERE user_id=?))`
		args = append(args, u.ID)
	case scope != "" && scope != "all":
		gid, err := strconv.ParseInt(scope, 10, 64)
		if err != nil {
			return nil, errBadInput
		}
		q += ` AND (e.group_id=? OR e.group_id IS NULL)` // general events concern everyone
		args = append(args, gid)
	}
	vq, vargs := visibleSQL(u)
	q += vq + ` ORDER BY e.start_at LIMIT 2000`
	args = append(args, vargs...)
	rows, err := a.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		e, err := a.scanEvent(rows, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	rows.Close()
	a.markEventsEditable(u, out)
	a.attachSignups(u, out)
	return out, nil
}

func (a *App) getEvent(u *User, id int64) (Event, error) {
	e, err := a.scanEvent(a.db.QueryRow(eventSelect+` WHERE e.id=?`, id), u)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !a.canSeeEvent(u, e)) {
		return Event{}, errNotFound
	}
	return e, err
}

func (a *App) createEvent(u *User, in EventInput, viaAI bool) (Event, error) {
	if in.Title == nil || strings.TrimSpace(*in.Title) == "" || in.Start == nil {
		return Event{}, errBadInput
	}
	if in.GroupID != nil && !a.groupExists(*in.GroupID) {
		return Event{}, errNotFound
	}
	if !a.canEditGroup(u, in.GroupID) {
		return Event{}, errForbidden
	}
	start, err := a.parseTime(*in.Start)
	if err != nil {
		return Event{}, err
	}
	end := start.Add(time.Hour)
	allDay := in.AllDay != nil && *in.AllDay
	if in.End != nil && *in.End != "" {
		if end, err = a.parseTime(*in.End); err != nil {
			return Event{}, err
		}
	} else if allDay {
		end = start.Add(24*time.Hour - time.Second)
	}
	if end.Before(start) {
		return Event{}, errBadInput
	}
	private := in.Private != nil && *in.Private
	if private && in.GroupID == nil { // only a group's own events can be internal
		return Event{}, errBadInput
	}
	var shifts []ShiftInput
	if in.Shifts != nil {
		if shifts, err = checkShifts(a, *in.Shifts, start, end); err != nil {
			return Event{}, err
		}
	}
	now := time.Now().Unix()
	res, err := a.db.Exec(`INSERT INTO events(title,description,location,start_at,end_at,all_day,group_id,created_by,via_ai,private,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, clip(*in.Title, 200), clip(deref(in.Description), 20000), clip(deref(in.Location), 200),
		start.Unix(), end.Unix(), allDay, in.GroupID, u.ID, viaAI, private, now, now)
	if err != nil {
		return Event{}, err
	}
	id, _ := res.LastInsertId()
	if len(shifts) > 0 {
		if err := a.saveShifts(id, shifts); err != nil {
			return Event{}, err
		}
	}
	e, err := a.getEventFull(u, id)
	if err == nil {
		a.liveEvent(u, "create", e)
	}
	return e, err
}

func (a *App) updateEvent(u *User, id int64, in EventInput) (Event, error) {
	cur, err := a.getEvent(u, id)
	if err != nil {
		return cur, err
	}
	if !cur.CanEdit {
		return cur, errForbidden
	}
	newGroup := cur.GroupID
	if in.ClearGroup {
		newGroup = nil
	} else if in.GroupID != nil {
		if !a.groupExists(*in.GroupID) {
			return cur, errNotFound
		}
		newGroup = in.GroupID
	}
	if !a.canEditGroup(u, newGroup) {
		return cur, errForbidden
	}
	title, desc, loc := cur.Title, cur.Description, cur.Location
	if in.Title != nil && strings.TrimSpace(*in.Title) != "" {
		title = strings.TrimSpace(*in.Title)
	}
	if in.Description != nil {
		desc = *in.Description
	}
	if in.Location != nil {
		loc = *in.Location
	}
	start, _ := time.Parse(time.RFC3339, cur.Start)
	end, _ := time.Parse(time.RFC3339, cur.End)
	if in.Start != nil {
		ns, err := a.parseTime(*in.Start)
		if err != nil {
			return cur, err
		}
		if in.End == nil { // keep duration
			end = ns.Add(end.Sub(start))
		}
		start = ns
	}
	if in.End != nil {
		if end, err = a.parseTime(*in.End); err != nil {
			return cur, err
		}
	}
	if end.Before(start) {
		return cur, errBadInput
	}
	allDay := cur.AllDay
	if in.AllDay != nil {
		allDay = *in.AllDay
	}
	private := cur.Private
	if in.Private != nil {
		private = *in.Private
	}
	if newGroup == nil {
		if in.Private != nil && *in.Private {
			return cur, errBadInput
		}
		private = false
	}
	var shifts []ShiftInput
	if in.Shifts != nil {
		if shifts, err = checkShifts(a, *in.Shifts, start, end); err != nil {
			return cur, err
		}
	}
	oldStart, _ := time.Parse(time.RFC3339, cur.Start)
	_, err = a.db.Exec(`UPDATE events SET title=?,description=?,location=?,start_at=?,end_at=?,all_day=?,group_id=?,private=?,updated_at=? WHERE id=?`,
		clip(title, 200), clip(desc, 20000), clip(loc, 200), start.Unix(), end.Unix(), allDay, newGroup, private, time.Now().Unix(), id)
	if err != nil {
		return cur, err
	}
	if private && !cur.Private { // internal now: the public page goes away
		a.db.Exec(`DELETE FROM public_links WHERE kind='event' AND event_id=?`, id)
	}
	if in.Shifts != nil {
		if err := a.saveShifts(id, shifts); err != nil {
			return cur, err
		}
	} else {
		a.moveShifts(id, start.Unix()-oldStart.Unix(), start.Unix(), end.Unix())
	}
	// keep sign-ups inside the new span
	a.db.Exec(`UPDATE event_signups SET start_at=MAX(start_at,?), end_at=MIN(end_at,?) WHERE event_id=?`, start.Unix(), end.Unix(), id)
	a.db.Exec(`UPDATE event_signups SET start_at=?, end_at=?, all_day=1 WHERE event_id=? AND (all_day=1 OR end_at<=start_at)`, start.Unix(), end.Unix(), id)
	e, err := a.getEventFull(u, id)
	if err == nil {
		a.liveEvent(u, "update", e)
	}
	return e, err
}

func (a *App) deleteEvent(u *User, id int64) (Event, error) {
	cur, err := a.getEvent(u, id)
	if err != nil {
		return cur, err
	}
	if !cur.CanEdit {
		return cur, errForbidden
	}
	_, err = a.db.Exec(`DELETE FROM events WHERE id=?`, id)
	if err == nil {
		a.liveEvent(u, "delete", cur)
	}
	return cur, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errForbidden):
		jsonErr(w, 403, "forbidden")
	case errors.Is(err, errNotFound):
		jsonErr(w, 404, "not_found")
	case errors.Is(err, errBadInput):
		jsonErr(w, 400, "bad_request")
	default:
		jsonErr(w, 500, "server_error")
	}
}

// ---------- HTTP ----------

func (a *App) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err1 := a.parseTime(q.Get("from"))
	to, err2 := a.parseTime(q.Get("to"))
	if err1 != nil || err2 != nil {
		now := time.Now().In(a.cfg.TZ)
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, a.cfg.TZ)
		to = from.AddDate(0, 1, 0)
	}
	evs, err := a.listEvents(currentUser(r), from, to, q.Get("scope"))
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, evs)
}

func (a *App) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var in EventInput
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	e, err := a.createEvent(currentUser(r), in, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}

func (a *App) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	var in EventInput
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	e, err := a.updateEvent(currentUser(r), pathID(r, "id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}

func (a *App) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if _, err := a.deleteEvent(currentUser(r), pathID(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}
