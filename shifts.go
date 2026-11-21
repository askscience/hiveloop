package main

// Volunteer shifts: an event can say how many volunteers it needs and when
// (e.g. "Christmas stall, 10:00–13:00, 2 volunteers"). People sign up for one or
// more shifts; when a shift is still short, the group is reminded 3 days, 1 day
// and 3 hours before it starts (chat message + notification).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const shiftsSchema = `CREATE TABLE IF NOT EXISTS event_shifts (
  id INTEGER PRIMARY KEY,
  event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  start_at INTEGER NOT NULL,
  end_at INTEGER NOT NULL,
  needed INTEGER NOT NULL,
  label TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS event_shifts_event ON event_shifts(event_id);
CREATE INDEX IF NOT EXISTS event_shifts_start ON event_shifts(start_at);
CREATE TABLE IF NOT EXISTS shift_signups (
  shift_id INTEGER NOT NULL REFERENCES event_shifts(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (shift_id, user_id)
);
CREATE TABLE IF NOT EXISTS event_reminders (
  event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  stage INTEGER NOT NULL,
  sent_at INTEGER NOT NULL,
  PRIMARY KEY (event_id, stage)
)`

const (
	maxShifts       = 20
	maxShiftNeeded  = 50
	maxShiftLabel   = 80
	reminderEvery   = 5 * time.Minute
	reminderHorizon = 72 * time.Hour
)

// reminder stages, in hours before the first shift that is still short
var reminderStages = []int{3, 24, 72}

type ShiftPerson struct {
	UserID   int64  `json:"user_id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

type Shift struct {
	ID     int64         `json:"id"`
	Start  string        `json:"start"`
	End    string        `json:"end"`
	Needed int           `json:"needed"`
	Label  string        `json:"label"`
	People []ShiftPerson `json:"people"`
	Mine   bool          `json:"mine"`

	startAt, endAt int64
}

type ShiftInput struct {
	ID     *int64 `json:"id"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Needed int    `json:"needed"`
	Label  string `json:"label"`

	start, end int64
}

// checkShifts validates shifts against the event span (times are clamped into it).
func checkShifts(a *App, in []ShiftInput, evStart, evEnd time.Time) ([]ShiftInput, error) {
	if len(in) > maxShifts {
		return nil, fmt.Errorf("%w: too many shifts", errBadInput)
	}
	out := make([]ShiftInput, 0, len(in))
	for _, s := range in {
		st, err := a.parseTime(s.Start)
		if err != nil {
			return nil, err
		}
		en, err := a.parseTime(s.End)
		if err != nil {
			return nil, err
		}
		if st.Before(evStart) {
			st = evStart
		}
		if en.After(evEnd) {
			en = evEnd
		}
		if !en.After(st) || s.Needed < 1 || s.Needed > maxShiftNeeded {
			return nil, fmt.Errorf("%w: shift outside the event or bad number of volunteers", errBadInput)
		}
		s.start, s.end = st.Unix(), en.Unix()
		s.Label = clip(strings.TrimSpace(s.Label), maxShiftLabel)
		out = append(out, s)
	}
	return out, nil
}

// saveShifts makes the event's shifts match `in`: listed ids are updated (keeping their
// volunteers), new ones inserted, the rest removed.
func (a *App) saveShifts(eventID int64, in []ShiftInput) error {
	keep := map[int64]bool{}
	for _, s := range in {
		if s.ID != nil {
			res, err := a.db.Exec(`UPDATE event_shifts SET start_at=?,end_at=?,needed=?,label=? WHERE id=? AND event_id=?`,
				s.start, s.end, s.Needed, s.Label, *s.ID, eventID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				keep[*s.ID] = true
				continue
			}
		}
		res, err := a.db.Exec(`INSERT INTO event_shifts(event_id,start_at,end_at,needed,label) VALUES(?,?,?,?,?)`,
			eventID, s.start, s.end, s.Needed, s.Label)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		keep[id] = true
	}
	ids := []string{"0"}
	for id := range keep {
		ids = append(ids, strconv.FormatInt(id, 10))
	}
	_, err := a.db.Exec(`DELETE FROM event_shifts WHERE event_id=? AND id NOT IN (`+strings.Join(ids, ",")+`)`, eventID)
	return err
}

// moveShifts follows the event when it is moved, and keeps shifts inside its span.
func (a *App) moveShifts(eventID, delta, start, end int64) {
	if delta != 0 {
		a.db.Exec(`UPDATE event_shifts SET start_at=start_at+?, end_at=end_at+? WHERE event_id=?`, delta, delta, eventID)
	}
	a.db.Exec(`UPDATE event_shifts SET start_at=MAX(start_at,?), end_at=MIN(end_at,?) WHERE event_id=?`, start, end, eventID)
	a.db.Exec(`UPDATE event_shifts SET start_at=?, end_at=? WHERE event_id=? AND end_at<=start_at`, start, end, eventID)
}

// attachShifts fills Shifts and Missing for a list of events (rows closed before returning).
func (a *App) attachShifts(u *User, evs []Event) {
	if len(evs) == 0 {
		return
	}
	idx := map[int64]int{}
	ids := make([]string, len(evs))
	for i, e := range evs {
		idx[e.ID] = i
		ids[i] = strconv.FormatInt(e.ID, 10)
		evs[i].Shifts = []Shift{}
		evs[i].Missing = 0
	}
	in := strings.Join(ids, ",")
	rows, err := a.db.Query(`SELECT id,event_id,start_at,end_at,needed,label FROM event_shifts WHERE event_id IN (` + in + `) ORDER BY start_at, id`)
	if err != nil {
		return
	}
	shiftAt := map[int64][2]int{} // shift id -> (event index, shift index)
	for rows.Next() {
		var s Shift
		var eid int64
		if rows.Scan(&s.ID, &eid, &s.startAt, &s.endAt, &s.Needed, &s.Label) != nil {
			continue
		}
		s.Start, s.End = a.fmtTime(s.startAt), a.fmtTime(s.endAt)
		s.People = []ShiftPerson{}
		e := &evs[idx[eid]]
		shiftAt[s.ID] = [2]int{idx[eid], len(e.Shifts)}
		e.Shifts = append(e.Shifts, s)
	}
	rows.Close()
	if len(shiftAt) == 0 {
		return
	}
	rows, err = a.db.Query(`SELECT x.shift_id,x.user_id,COALESCE(NULLIF(u.name,''),u.username),u.username FROM shift_signups x
		JOIN users u ON u.id=x.user_id JOIN event_shifts s ON s.id=x.shift_id WHERE s.event_id IN (` + in + `) ORDER BY x.created_at`)
	if err != nil {
		return
	}
	for rows.Next() {
		var sid int64
		var p ShiftPerson
		if rows.Scan(&sid, &p.UserID, &p.Name, &p.Username) != nil {
			continue
		}
		at, ok := shiftAt[sid]
		if !ok {
			continue
		}
		s := &evs[at[0]].Shifts[at[1]]
		s.People = append(s.People, p)
		if u != nil && p.UserID == u.ID {
			s.Mine = true
		}
	}
	rows.Close()
	now := time.Now().Unix()
	for i := range evs {
		for _, s := range evs[i].Shifts {
			if s.startAt > now && len(s.People) < s.Needed {
				evs[i].Missing += s.Needed - len(s.People)
			}
		}
	}
}

// shiftOf checks that the shift belongs to an event u can see.
func (a *App) shiftOf(u *User, eventID, shiftID int64) (Event, error) {
	e, err := a.getEvent(u, eventID)
	if err != nil {
		return e, err
	}
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM event_shifts WHERE id=? AND event_id=?`, shiftID, eventID).Scan(&n)
	if n == 0 {
		return e, errNotFound
	}
	return e, nil
}

func (a *App) joinShift(u *User, eventID, shiftID int64) (Event, error) {
	if _, err := a.shiftOf(u, eventID, shiftID); err != nil {
		return Event{}, err
	}
	if _, err := a.db.Exec(`INSERT OR IGNORE INTO shift_signups(shift_id,user_id,created_at) VALUES(?,?,?)`, shiftID, u.ID, time.Now().Unix()); err != nil {
		return Event{}, err
	}
	e, err := a.getEventFull(u, eventID)
	if err == nil {
		a.liveEvent(u, "signup", e)
	}
	return e, err
}

// leaveShift: people take themselves off; whoever can edit the event can take anyone off.
func (a *App) leaveShift(u *User, eventID, shiftID, userID int64) (Event, error) {
	e, err := a.shiftOf(u, eventID, shiftID)
	if err != nil {
		return e, err
	}
	if userID != u.ID && !e.CanEdit {
		return e, errForbidden
	}
	a.db.Exec(`DELETE FROM shift_signups WHERE shift_id=? AND user_id=?`, shiftID, userID)
	e, err = a.getEventFull(u, eventID)
	if err == nil {
		a.liveEvent(u, "signup", e)
	}
	return e, err
}

func (a *App) handleJoinShift(w http.ResponseWriter, r *http.Request) {
	e, err := a.joinShift(currentUser(r), pathID(r, "id"), pathID(r, "sid"))
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}

func (a *App) handleLeaveShift(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	uid := u.ID
	if r.PathValue("uid") != "" {
		uid = pathID(r, "uid")
	}
	e, err := a.leaveShift(u, pathID(r, "id"), pathID(r, "sid"), uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, e)
}

// ---------- reminders ----------

var systemViewer = &User{Role: "superadmin"} // sees every event; never a person

func (a *App) reminderLoop() {
	time.Sleep(time.Minute)
	for {
		a.checkReminders(time.Now())
		time.Sleep(reminderEvery)
	}
}

// reminderStage: which reminder is due for a shift starting in `left` (0 = none yet).
func reminderStage(left time.Duration) int {
	for _, h := range reminderStages {
		if left <= time.Duration(h)*time.Hour {
			return h
		}
	}
	return 0
}

func (a *App) checkReminders(now time.Time) {
	rows, err := a.db.Query(`SELECT DISTINCT event_id FROM event_shifts WHERE start_at > ? AND start_at <= ?`, now.Unix(), now.Add(reminderHorizon).Unix())
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		e, err := a.getEventFull(systemViewer, id)
		if err != nil {
			continue
		}
		var open []Shift
		for _, s := range e.Shifts {
			if s.startAt > now.Unix() && len(s.People) < s.Needed {
				open = append(open, s)
			}
		}
		if len(open) == 0 {
			continue
		}
		sort.Slice(open, func(i, j int) bool { return open[i].startAt < open[j].startAt })
		stage := reminderStage(time.Unix(open[0].startAt, 0).Sub(now))
		if stage == 0 {
			continue
		}
		// one message per stage; a later (closer) stage also covers the earlier ones
		var sent sql.NullInt64
		a.db.QueryRow(`SELECT MIN(stage) FROM event_reminders WHERE event_id=?`, id).Scan(&sent)
		if sent.Valid && int(sent.Int64) <= stage {
			continue
		}
		res, err := a.db.Exec(`INSERT OR IGNORE INTO event_reminders(event_id,stage,sent_at) VALUES(?,?,?)`, id, stage, now.Unix())
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}
		a.sendVolunteerReminder(e, open, stage)
	}
}

// volunteerRecipients: the group's members (everyone for general events), minus anyone
// already signed up for one of the event's shifts.
func (a *App) volunteerRecipients(e Event) []User {
	signed := map[int64]bool{}
	for _, s := range e.Shifts {
		for _, p := range s.People {
			signed[p.UserID] = true
		}
	}
	q := `SELECT id,username,name,role,lang FROM users WHERE status='active'`
	args := []any{}
	if e.GroupID != nil {
		q += ` AND id IN (SELECT user_id FROM group_members WHERE group_id=?)`
		args = append(args, *e.GroupID)
	}
	rows, err := a.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang) == nil && !signed[u.ID] {
			out = append(out, u)
		}
	}
	return out
}

type reminderShift struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Needed int    `json:"needed"`
	Have   int    `json:"have"`
	Label  string `json:"label,omitempty"`
}

func (a *App) sendVolunteerReminder(e Event, open []Shift, stage int) {
	missing := 0
	shifts := make([]reminderShift, 0, len(open))
	for _, s := range open {
		missing += s.Needed - len(s.People)
		shifts = append(shifts, reminderShift{s.Start, s.End, s.Needed, len(s.People), s.Label})
	}
	recipients := a.volunteerRecipients(e)
	skip := []int64{}
	for _, s := range e.Shifts {
		for _, p := range s.People {
			skip = append(skip, p.UserID)
		}
	}
	room := int64(0)
	if e.GroupID != nil {
		room = *e.GroupID
	}
	meta, _ := json.Marshal(map[string]any{
		"system": true, "kind": "volunteers", "event_id": e.ID, "title": e.Title, "start": open[0].Start,
		"missing": missing, "shifts": shifts, "stage": stage, "skip": skip, "private": e.Private,
	})
	text := volunteerText("en", e.Title, open, missing, a.cfg.TZ)
	if err := a.postSystem(room, text, string(meta)); err != nil {
		log.Printf("volunteer reminder %d: %v", e.ID, err)
	}
	url := fmt.Sprintf("/#/calendar/%s/e%d", time.Unix(open[0].startAt, 0).In(a.cfg.TZ).Format("2006-01-02"), e.ID)
	ids := make([]int64, 0, len(recipients))
	langs := map[int64]string{}
	for _, u := range recipients {
		ids = append(ids, u.ID)
		langs[u.ID] = u.Lang
	}
	a.notify(ids, func(uid int64) Notice {
		lang := langs[uid]
		return Notice{
			Title: tr(lang, "Servono volontariə · ", "Volunteers needed · ") + e.Title,
			Body:  volunteerText(lang, e.Title, open, missing, a.cfg.TZ),
			URL:   url,
			Tag:   fmt.Sprintf("vol-%d", e.ID),
		}
	})
}

func tr(lang, it, en string) string {
	if lang == "it" {
		return it
	}
	return en
}

// shortDay: "Sat 20 Dec" / "sab 20 dic".
func shortDay(lang string, t time.Time) string {
	if lang != "it" {
		return t.Format("Mon 2 Jan")
	}
	return fmt.Sprintf("%s %d %s", daysIT[t.Weekday()][:3], t.Day(), monthsIT[t.Month()-1][:3])
}

// volunteerText: "sab 20 dic: mancano 3 volontariə — 10:00–13:00 (1/2), 15:00–18:00 (0/2)".
func volunteerText(lang, title string, open []Shift, missing int, tz *time.Location) string {
	var parts []string
	lastDay := ""
	for _, s := range open {
		st, en := time.Unix(s.startAt, 0).In(tz), time.Unix(s.endAt, 0).In(tz)
		p := ""
		if d := shortDay(lang, st); d != lastDay {
			p, lastDay = d+" ", d
		}
		p += fmt.Sprintf("%s–%s (%d/%d)", st.Format("15:04"), en.Format("15:04"), len(s.People), s.Needed)
		if s.Label != "" {
			p += " " + s.Label
		}
		parts = append(parts, p)
	}
	n := tr(lang, fmt.Sprintf("mancano %d volontariə", missing), fmt.Sprintf("%d volunteers still needed", missing))
	if missing == 1 {
		n = tr(lang, "manca 1 volontariə", "1 volunteer still needed")
	}
	return fmt.Sprintf("«%s»: %s — %s", title, n, strings.Join(parts, ", "))
}

// postSystem writes an automatic message (no author) in a chat room.
func (a *App) postSystem(room int64, text, meta string) error {
	res, err := a.db.Exec(`INSERT INTO room_messages(room_id,user_id,content,meta,created_at) VALUES(?,NULL,?,?,?)`, room, text, meta, time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m, err := a.scanRoomMsg(a.db.QueryRow(roomMsgSelect+` WHERE m.id=?`, id))
	if err != nil {
		return err
	}
	ms := []RoomMsg{m}
	a.fillMentions(ms)
	name, _ := a.roomName(room)
	a.broadcast(room, map[string]any{"type": "message", "msg": ms[0], "room_name": name})
	return nil
}
