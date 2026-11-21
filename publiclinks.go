package main

// Public links: a read-only page, reachable without an account, for one event or for a
// whole day of the calendar (optionally limited to one group). Links use a random token,
// can be revoked at any time and never show who signed up.

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const publicLinksSchema = `CREATE TABLE IF NOT EXISTS public_links (
  token TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('event','day')),
  event_id INTEGER REFERENCES events(id) ON DELETE CASCADE,
  day TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
)`

type publicLink struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

func (a *App) linkURL(r *http.Request, kind, token string) string {
	p := "e/"
	if kind == "day" {
		p = "d/"
	}
	return a.publicURL(r) + p + token
}

// ---------- link management (used by the API and by the assistant) ----------

var dayRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// eventLinkToken returns the event's public token ("" if none); create makes one if missing.
func (a *App) eventLinkToken(u *User, id int64, create bool) (string, error) {
	e, err := a.getEvent(u, id)
	if err != nil {
		return "", err
	}
	if !e.CanEdit {
		return "", errForbidden
	}
	var tok string
	err = a.db.QueryRow(`SELECT token FROM public_links WHERE kind='event' AND event_id=?`, id).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		if !create {
			return "", nil
		}
		tok = randomToken(16)
		_, err = a.db.Exec(`INSERT INTO public_links(token,kind,event_id,created_by,created_at) VALUES(?,?,?,?,?)`,
			tok, "event", id, u.ID, time.Now().Unix())
	}
	return tok, err
}

func (a *App) revokeEventLink(u *User, id int64) error {
	e, err := a.getEvent(u, id)
	if err != nil {
		return err
	}
	if !e.CanEdit {
		return errForbidden
	}
	_, err = a.db.Exec(`DELETE FROM public_links WHERE kind='event' AND event_id=?`, id)
	return err
}

// canShareDay: a group's day can be shared by its members (and admins); the whole
// calendar ("all") only by admins.
func (a *App) canShareDay(u *User, scope string) bool {
	if scope == "all" {
		return u.IsAdmin()
	}
	gid, err := strconv.ParseInt(scope, 10, 64)
	return err == nil && a.groupExists(gid) && a.canEditGroup(u, &gid)
}

// dayLinkToken: like eventLinkToken for a whole day; scope is "all" or a group id.
func (a *App) dayLinkToken(u *User, day, scope string, create bool) (string, error) {
	if scope == "" {
		scope = "all"
	}
	if !dayRe.MatchString(day) {
		return "", errBadInput
	}
	if _, err := time.ParseInLocation("2006-01-02", day, a.cfg.TZ); err != nil {
		return "", errBadInput
	}
	if !a.canShareDay(u, scope) {
		return "", errForbidden
	}
	var tok string
	err := a.db.QueryRow(`SELECT token FROM public_links WHERE kind='day' AND day=? AND scope=?`, day, scope).Scan(&tok)
	if errors.Is(err, sql.ErrNoRows) {
		if !create {
			return "", nil
		}
		tok = randomToken(16)
		_, err = a.db.Exec(`INSERT INTO public_links(token,kind,day,scope,created_by,created_at) VALUES(?,?,?,?,?,?)`,
			tok, "day", day, scope, u.ID, time.Now().Unix())
	}
	return tok, err
}

func (a *App) revokeDayLink(u *User, day, scope string) error {
	tok, err := a.dayLinkToken(u, day, scope, false)
	if err != nil || tok == "" {
		return err
	}
	_, err = a.db.Exec(`DELETE FROM public_links WHERE token=?`, tok)
	return err
}

// ---------- API (members) ----------

func (a *App) replyLink(w http.ResponseWriter, r *http.Request, kind, tok string, err error) {
	switch {
	case err != nil:
		writeErr(w, err)
	case tok == "":
		jsonOK(w, map[string]any{"url": nil})
	default:
		jsonOK(w, publicLink{tok, a.linkURL(r, kind, tok)})
	}
}

func (a *App) handleGetEventLink(w http.ResponseWriter, r *http.Request) {
	tok, err := a.eventLinkToken(currentUser(r), pathID(r, "id"), r.Method == http.MethodPost)
	a.replyLink(w, r, "event", tok, err)
}

func (a *App) handleDeleteEventLink(w http.ResponseWriter, r *http.Request) {
	if err := a.revokeEventLink(currentUser(r), pathID(r, "id")); err != nil {
		writeErr(w, err)
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleDayLink(w http.ResponseWriter, r *http.Request) {
	u, q := currentUser(r), r.URL.Query()
	if r.Method == http.MethodDelete {
		if err := a.revokeDayLink(u, q.Get("day"), q.Get("scope")); err != nil {
			writeErr(w, err)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	tok, err := a.dayLinkToken(u, q.Get("day"), q.Get("scope"), r.Method == http.MethodPost)
	a.replyLink(w, r, "day", tok, err)
}

// ---------- public pages ----------

type pubText struct {
	Lang, AllDay, General, Where, Map, AddCal, NoEvents, Gone, GoneHint, Event, Day, Member, Footer string
}

var pubIT = pubText{Lang: "it", AllDay: "Tutto il giorno", General: "Generale", Where: "Luogo", Map: "Apri la mappa",
	AddCal: "Aggiungi al calendario", NoEvents: "Nessun evento in programma per questa giornata.", Gone: "Link non disponibile",
	GoneHint: "Questo link è stato disattivato oppure l'evento è stato rimosso.", Event: "Evento", Day: "Programma della giornata",
	Member: "Fai parte del gruppo? Apri Hiveloop", Footer: "Condiviso con Hiveloop"}
var pubEN = pubText{Lang: "en", AllDay: "All day", General: "General", Where: "Place", Map: "Open map",
	AddCal: "Add to calendar", NoEvents: "Nothing planned for this day.", Gone: "Link not available",
	GoneHint: "This link was turned off or the event was removed.", Event: "Event", Day: "Programme of the day",
	Member: "Are you a member? Open Hiveloop", Footer: "Shared with Hiveloop"}

var (
	monthsIT = []string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
	daysIT   = []string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"}
)

func pubLang(r *http.Request) pubText {
	if al := strings.ToLower(r.Header.Get("Accept-Language")); strings.HasPrefix(al, "it") {
		return pubIT
	}
	return pubEN
}

func (p pubText) date(t time.Time) string {
	if p.Lang == "en" {
		return t.Format("Monday 2 January 2006")
	}
	return fmt.Sprintf("%s %d %s %d", daysIT[t.Weekday()], t.Day(), monthsIT[t.Month()-1], t.Year())
}

func (p pubText) shortDate(t time.Time) string {
	if p.Lang == "en" {
		return t.Format("2 Jan")
	}
	return fmt.Sprintf("%d %s", t.Day(), monthsIT[t.Month()-1][:3])
}

type pubEvent struct {
	ID                            int64
	LocIsURL                      bool
	Title, When, Location, MapURL string
	Group, Color                  string
	Desc                          template.HTML
	ICS                           string
	dateLine                      string
}

func (a *App) pubEventView(p pubText, e Event, icsURL string) pubEvent {
	s, _ := time.Parse(time.RFC3339, e.Start)
	en, _ := time.Parse(time.RFC3339, e.End)
	s, en = s.In(a.cfg.TZ), en.In(a.cfg.TZ)
	same := s.Format("20060102") == en.Format("20060102")
	var when string
	switch {
	case e.AllDay && same:
		when = p.AllDay
	case e.AllDay:
		when = p.shortDate(s) + " → " + p.shortDate(en)
	case same:
		when = s.Format("15:04") + " – " + en.Format("15:04")
	default:
		when = p.shortDate(s) + " " + s.Format("15:04") + " → " + p.shortDate(en) + " " + en.Format("15:04")
	}
	v := pubEvent{ID: e.ID, Title: e.Title, When: when, Location: e.Location, Group: e.GroupName, Color: e.GroupColor,
		Desc: miniMarkdown(e.Description), ICS: icsURL, dateLine: p.date(s)}
	if v.Group == "" {
		v.Group = p.General
	}
	if !hexColor.MatchString(v.Color) {
		v.Color = "#3b3a36"
	}
	if e.Location != "" {
		v.MapURL = mapsURL(e.Location)
		v.LocIsURL = v.MapURL == strings.TrimSpace(e.Location)
	}
	return v
}

// mapsURL opens the place in Google Maps (app on phones, website elsewhere). A pasted
// maps link is used as is.
func mapsURL(place string) string {
	place = strings.TrimSpace(place)
	if strings.HasPrefix(place, "https://") && !strings.ContainsAny(place, " \t") {
		return place
	}
	return "https://www.google.com/maps/search/?api=1&query=" + strings.ReplaceAll(url.QueryEscape(place), "+", "%20")
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

type pubPage struct {
	T                      pubText
	Title, Kicker, Date    string
	OGDesc, OGURL, OGImage string
	AppURL                 string
	Events                 []pubEvent
	Gone, ShowH2           bool
	DayICS                 string
}

var pubTpl = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="{{.T.Lang}}"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{.Title}}</title>
<meta name="robots" content="noindex">
{{if not .Gone}}<meta property="og:type" content="website"><meta property="og:title" content="{{.Title}}">
<meta property="og:description" content="{{.OGDesc}}"><meta property="og:url" content="{{.OGURL}}">
<meta property="og:image" content="{{.OGImage}}"><meta name="description" content="{{.OGDesc}}">{{end}}
<link rel="icon" href="/favicon.svg" type="image/svg+xml">
<link rel="stylesheet" href="/fonts/fonts.css"><link rel="stylesheet" href="/public.css">
</head><body><main class="pub">
<header><a class="logo" href="{{.AppURL}}"><img src="/icon-192.png" alt="" width="28" height="28">Hiveloop</a></header>
{{if .Gone}}<h1>{{.T.Gone}}</h1><p class="muted">{{.T.GoneHint}}</p>{{else}}
<p class="kicker">{{.Kicker}}</p>
<h1>{{.Title}}</h1>
{{if .Date}}<p class="date">{{.Date}}</p>{{end}}
{{range .Events}}<article style="--c:{{.Color}}">
  {{if $.ShowH2}}<h2>{{.Title}}</h2>{{end}}
  <ul class="meta">
    <li><svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/></svg><span>{{.When}}</span></li>
    {{if .Location}}<li><svg viewBox="0 0 24 24"><path d="M12 21s-6.5-5.6-6.5-11a6.5 6.5 0 0 1 13 0c0 5.4-6.5 11-6.5 11z"/><circle cx="12" cy="10" r="2.3"/></svg><span>{{if not .LocIsURL}}{{.Location}} · {{end}}<a href="{{.MapURL}}" target="_blank" rel="noopener">{{$.T.Map}}</a></span></li>{{end}}
    <li><i class="sw"></i><span>{{.Group}}</span></li>
  </ul>
  {{if .Desc}}<div class="desc">{{.Desc}}</div>{{end}}
  <a class="btn" href="{{.ICS}}">{{$.T.AddCal}}</a>
</article>{{else}}<p class="muted">{{.T.NoEvents}}</p>{{end}}
{{end}}
<footer><a href="{{.AppURL}}">{{.T.Member}}</a></footer>
</main></body></html>`))

func (a *App) renderPub(w http.ResponseWriter, code int, pg pubPage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Robots-Tag", "noindex")
	w.WriteHeader(code)
	pubTpl.Execute(w, pg)
}

func (a *App) lookupLink(token string) (kind string, eventID int64, day, scope string, err error) {
	var eid sql.NullInt64
	if len(token) < 16 || len(token) > 64 {
		return "", 0, "", "", errNotFound
	}
	err = a.db.QueryRow(`SELECT kind,event_id,day,scope FROM public_links WHERE token=?`, token).Scan(&kind, &eid, &day, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return kind, eid.Int64, day, scope, err
}

var publicViewer = &User{Role: "user"} // no memberships: sees nothing editable

func (a *App) dayEvents(day, scope string) (time.Time, []Event, error) {
	d, err := time.ParseInLocation("2006-01-02", day, a.cfg.TZ)
	if err != nil {
		return d, nil, err
	}
	q := eventSelect + ` WHERE e.start_at < ? AND e.end_at >= ?`
	args := []any{d.AddDate(0, 0, 1).Unix(), d.Unix()}
	if scope != "all" {
		q += ` AND e.group_id=?`
		args = append(args, scope)
	}
	rows, err := a.db.Query(q+` ORDER BY e.all_day DESC, e.start_at LIMIT 200`, args...)
	if err != nil {
		return d, nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := a.scanEvent(rows, nil) // public page: nothing is editable
		if err != nil {
			return d, nil, err
		}
		out = append(out, e)
	}
	return d, out, rows.Err()
}

func (a *App) handlePublicEvent(w http.ResponseWriter, r *http.Request) {
	p, base := pubLang(r), a.publicURL(r)
	tok := r.PathValue("token")
	kind, eid, _, _, err := a.lookupLink(tok)
	if err != nil || kind != "event" {
		a.renderPub(w, 404, pubPage{T: p, Title: p.Gone, Gone: true, AppURL: base})
		return
	}
	e, err := a.getEvent(publicViewer, eid)
	if err != nil {
		a.renderPub(w, 404, pubPage{T: p, Title: p.Gone, Gone: true, AppURL: base})
		return
	}
	v := a.pubEventView(p, e, base+"e/"+tok+"/ics")
	desc := v.dateLine + " · " + v.When
	if e.Location != "" {
		desc += " · " + e.Location
	}
	a.renderPub(w, 200, pubPage{T: p, Title: e.Title, Kicker: p.Event + " · " + v.Group, Date: v.dateLine,
		OGDesc: desc, OGURL: base + "e/" + tok, OGImage: base + "icon-512.png", AppURL: base, Events: []pubEvent{v}})
}

func (a *App) handlePublicDay(w http.ResponseWriter, r *http.Request) {
	p, base := pubLang(r), a.publicURL(r)
	tok := r.PathValue("token")
	kind, _, day, scope, err := a.lookupLink(tok)
	if err != nil || kind != "day" {
		a.renderPub(w, 404, pubPage{T: p, Title: p.Gone, Gone: true, AppURL: base})
		return
	}
	d, evs, err := a.dayEvents(day, scope)
	if err != nil {
		writeErr(w, err)
		return
	}
	title := p.date(d)
	title = strings.ToUpper(title[:1]) + title[1:]
	kicker := p.Day
	if scope != "all" {
		var name string
		a.db.QueryRow(`SELECT name FROM groups WHERE id=?`, scope).Scan(&name)
		if name != "" {
			kicker += " · " + name
		}
	}
	var views []pubEvent
	var titles []string
	for _, e := range evs {
		views = append(views, a.pubEventView(p, e, fmt.Sprintf("%sd/%s/ics?e=%d", base, tok, e.ID)))
		titles = append(titles, e.Title)
	}
	og := p.NoEvents
	if len(titles) > 0 {
		og = strings.Join(titles, " · ")
	}
	a.renderPub(w, 200, pubPage{T: p, Title: title, Kicker: kicker, OGDesc: og, OGURL: base + "d/" + tok,
		OGImage: base + "icon-512.png", AppURL: base, Events: views, ShowH2: true})
}

// ---------- iCalendar ----------

func icsEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`).Replace(s)
}

func (a *App) icsEvent(b *strings.Builder, e Event, url string) {
	s, _ := time.Parse(time.RFC3339, e.Start)
	en, _ := time.Parse(time.RFC3339, e.End)
	fmt.Fprintf(b, "BEGIN:VEVENT\r\nUID:hiveloop-%d@hiveloop\r\nDTSTAMP:%s\r\n", e.ID, time.Now().UTC().Format("20060102T150405Z"))
	if e.AllDay {
		ls, le := s.In(a.cfg.TZ), en.In(a.cfg.TZ)
		fmt.Fprintf(b, "DTSTART;VALUE=DATE:%s\r\nDTEND;VALUE=DATE:%s\r\n", ls.Format("20060102"), le.AddDate(0, 0, 1).Format("20060102"))
	} else {
		fmt.Fprintf(b, "DTSTART:%s\r\nDTEND:%s\r\n", s.UTC().Format("20060102T150405Z"), en.UTC().Format("20060102T150405Z"))
	}
	fmt.Fprintf(b, "SUMMARY:%s\r\n", icsEscape(e.Title))
	if e.Location != "" {
		fmt.Fprintf(b, "LOCATION:%s\r\n", icsEscape(e.Location))
	}
	desc := e.Description
	if url != "" {
		desc = strings.TrimSpace(desc + "\n\n" + url)
	}
	if desc != "" {
		fmt.Fprintf(b, "DESCRIPTION:%s\r\n", icsEscape(desc))
	}
	b.WriteString("END:VEVENT\r\n")
}

func (a *App) writeICS(w http.ResponseWriter, name string, evs []Event, url string) {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Hiveloop//IT\r\nCALSCALE:GREGORIAN\r\n")
	for _, e := range evs {
		a.icsEvent(&b, e, url)
	}
	b.WriteString("END:VCALENDAR\r\n")
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.ics"`)
	w.Write([]byte(b.String()))
}

func (a *App) handlePublicEventICS(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	kind, eid, _, _, err := a.lookupLink(tok)
	if err != nil || kind != "event" {
		http.NotFound(w, r)
		return
	}
	e, err := a.getEvent(publicViewer, eid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.writeICS(w, "event", []Event{e}, a.publicURL(r)+"e/"+tok)
}

func (a *App) handlePublicDayICS(w http.ResponseWriter, r *http.Request) {
	tok := r.PathValue("token")
	kind, _, day, scope, err := a.lookupLink(tok)
	if err != nil || kind != "day" {
		http.NotFound(w, r)
		return
	}
	_, evs, err := a.dayEvents(day, scope)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if id, _ := strconv.ParseInt(r.URL.Query().Get("e"), 10, 64); id > 0 {
		var one []Event
		for _, e := range evs {
			if e.ID == id {
				one = append(one, e)
			}
		}
		evs = one
	}
	if len(evs) == 0 {
		http.NotFound(w, r)
		return
	}
	a.writeICS(w, day, evs, a.publicURL(r)+"d/"+tok)
}

// ---------- tiny, safe Markdown subset for descriptions ----------

var (
	mdBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdLink = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	mdURL  = regexp.MustCompile(`(^|[\s(])(https?://[^\s<)]+)`)
)

func mdInline(s string) string {
	s = template.HTMLEscapeString(s)
	s = mdLink.ReplaceAllString(s, `<a href="$2" target="_blank" rel="noopener nofollow">$1</a>`)
	if !strings.Contains(s, "<a ") {
		s = mdURL.ReplaceAllString(s, `$1<a href="$2" target="_blank" rel="noopener nofollow">$2</a>`)
	}
	return mdBold.ReplaceAllString(s, "<b>$1</b>")
}

func miniMarkdown(src string) template.HTML {
	var b strings.Builder
	inList := false
	var para []string
	flush := func() {
		if len(para) > 0 {
			b.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
			para = nil
		}
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	for _, ln := range strings.Split(strings.ReplaceAll(src, "\r", ""), "\n") {
		t := strings.TrimSpace(ln)
		switch {
		case t == "":
			flush()
		case strings.HasPrefix(t, "#"):
			flush()
			b.WriteString("<h3>" + mdInline(strings.TrimLeft(t, "# ")) + "</h3>")
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			if len(para) > 0 {
				b.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
				para = nil
			}
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			b.WriteString("<li>" + mdInline(t[2:]) + "</li>")
		default:
			if inList {
				b.WriteString("</ul>")
				inList = false
			}
			para = append(para, mdInline(t))
		}
	}
	flush()
	return template.HTML(b.String())
}
