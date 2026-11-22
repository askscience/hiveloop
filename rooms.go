package main

// Organisational chats: one room per group plus a general room (id 0), with @mentions,
// unread tracking and live updates over Server-Sent Events.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Room struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	IsMember bool    `json:"is_member"`
	Unread   int     `json:"unread"`
	Mentions int     `json:"mentions"`
	LastText string  `json:"last_text"`
	LastAt   int64   `json:"last_at"`
	LastBy   string  `json:"last_by"`
	DM       *DMPeer `json:"dm,omitempty"`
}

type RoomMsg struct {
	ID        int64           `json:"id"`
	RoomID    int64           `json:"room_id"`
	UserID    *int64          `json:"user_id"`
	Author    string          `json:"author"`
	Username  string          `json:"username"`
	Content   string          `json:"content"`
	Doc       *MsgDoc         `json:"doc"`
	Mentions  []int64         `json:"mentions"`
	IsAI      bool            `json:"is_ai"`
	Reply     *MsgReply       `json:"reply,omitempty"`
	Meta      json.RawMessage `json:"meta,omitempty"`
	CreatedAt int64           `json:"created_at"`
}

// MsgReply is a short preview of the message being answered.
type MsgReply struct {
	ID      int64  `json:"id"`
	Author  string `json:"author"`
	Text    string `json:"text"`
	IsAI    bool   `json:"is_ai"`
	Deleted bool   `json:"deleted,omitempty"`
}

type MsgDoc struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
}

// canRoom: general room is open to all; group rooms to members and admins; a direct
// conversation only to its two people.
func (a *App) canRoom(u *User, room int64) bool {
	if isDM(room) {
		return a.inDM(u.ID, room)
	}
	if room == 0 || u.IsAdmin() {
		return room == 0 || a.groupExists(room)
	}
	return a.isMember(u.ID, room)
}

// ---------- live hub ----------

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]*User
}

var liveHub = &hub{subs: map[chan []byte]*User{}}

func (a *App) broadcast(room int64, v any) {
	b, _ := json.Marshal(v)
	liveHub.mu.Lock()
	defer liveHub.mu.Unlock()
	for ch, u := range liveHub.subs {
		if !a.canRoom(u, room) {
			continue
		}
		select {
		case ch <- b:
		default: // slow client: drop, it will resync on reconnect
		}
	}
}

func (a *App) handleStream(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	fl, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, 500, "streaming_unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := make(chan []byte, 32)
	liveHub.mu.Lock()
	liveHub.subs[ch] = u
	liveHub.mu.Unlock()
	defer func() {
		liveHub.mu.Lock()
		delete(liveHub.subs, ch)
		liveHub.mu.Unlock()
	}()
	fmt.Fprint(w, "retry: 3000\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-ping.C:
			// re-check the session: removed, demoted or signed-out users stop receiving updates
			fresh := a.userFromRequest(r)
			if fresh == nil {
				return
			}
			liveHub.mu.Lock()
			liveHub.subs[ch] = fresh
			liveHub.mu.Unlock()
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

// ---------- queries ----------

const roomMsgSelect = `SELECT m.id,m.room_id,m.user_id,COALESCE(NULLIF(u.name,''),u.username,'—'),COALESCE(u.username,''),m.content,
	m.doc_id,COALESCE(d.name,''),COALESCE(d.mime,''),m.created_at,m.is_ai,m.meta,
	m.reply_to,rm.id,COALESCE(rm.content,''),COALESCE(rm.is_ai,0),COALESCE(NULLIF(ru.name,''),ru.username,''),COALESCE(rd.name,'')
	FROM room_messages m LEFT JOIN users u ON u.id=m.user_id LEFT JOIN documents d ON d.id=m.doc_id
	LEFT JOIN room_messages rm ON rm.id=m.reply_to LEFT JOIN users ru ON ru.id=rm.user_id LEFT JOIN documents rd ON rd.id=rm.doc_id`

func (a *App) scanRoomMsg(s interface{ Scan(...any) error }) (RoomMsg, error) {
	var m RoomMsg
	var uid, did sql.NullInt64
	var dname, dmime, meta string
	var replyTo, rid sql.NullInt64
	var rtext, rauthor, rdoc string
	var rai bool
	err := s.Scan(&m.ID, &m.RoomID, &uid, &m.Author, &m.Username, &m.Content, &did, &dname, &dmime, &m.CreatedAt, &m.IsAI, &meta,
		&replyTo, &rid, &rtext, &rai, &rauthor, &rdoc)
	if replyTo.Valid && rid.Valid { // original deleted: no quote, no trace
		m.Reply = &MsgReply{ID: replyTo.Int64, Deleted: !rid.Valid, IsAI: rai, Author: rauthor, Text: clip(oneLine(rtext), 160)}
		if rai {
			m.Reply.Author = "Hiveloop AI"
		}
		if m.Reply.Text == "" {
			m.Reply.Text = rdoc
		}
	}
	if m.IsAI {
		m.Author, m.Username = "Hiveloop AI", "ai"
	}
	if !m.IsAI && !uid.Valid && strings.Contains(meta, `"system"`) {
		m.Author, m.Username = "", ""
	}
	if meta != "" {
		m.Meta = json.RawMessage(meta)
	}
	if uid.Valid {
		m.UserID = &uid.Int64
	}
	if did.Valid && dname != "" {
		m.Doc = &MsgDoc{did.Int64, dname, dmime}
	}
	m.Mentions = []int64{}
	return m, err
}

func (a *App) fillMentions(msgs []RoomMsg) {
	if len(msgs) == 0 {
		return
	}
	idx := map[int64]int{}
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		idx[m.ID] = i
		ids[i] = strconv.FormatInt(m.ID, 10)
	}
	rows, err := a.db.Query(`SELECT message_id,user_id FROM mentions WHERE message_id IN (` + strings.Join(ids, ",") + `)`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var mid, uid int64
		rows.Scan(&mid, &uid)
		msgs[idx[mid]].Mentions = append(msgs[idx[mid]].Mentions, uid)
	}
}

func (a *App) roomName(room int64) (string, string) {
	if room == 0 {
		return "", ""
	}
	var n, c string
	a.db.QueryRow(`SELECT name,color FROM groups WHERE id=?`, room).Scan(&n, &c)
	return n, c
}

func (a *App) handleListRooms(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	gs, err := a.listGroups(u)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	rooms := []Room{{ID: 0, IsMember: true}}
	for _, g := range gs {
		if g.IsMember || u.IsAdmin() {
			rooms = append(rooms, Room{ID: g.ID, Name: g.Name, Color: g.Color, IsMember: g.IsMember})
		}
	}
	nGroups := len(rooms)
	rooms = append(rooms, a.dmRooms(u)...)
	for i := range rooms {
		rm := &rooms[i]
		var last int64
		a.db.QueryRow(`SELECT last_id FROM room_reads WHERE user_id=? AND room_id=?`, u.ID, rm.ID).Scan(&last)
		a.db.QueryRow(`SELECT COUNT(*) FROM room_messages WHERE room_id=? AND id>? AND COALESCE(user_id,0)<>?`, rm.ID, last, u.ID).Scan(&rm.Unread)
		a.db.QueryRow(`SELECT COUNT(*) FROM mentions x JOIN room_messages m ON m.id=x.message_id WHERE x.user_id=? AND m.room_id=? AND m.id>?`, u.ID, rm.ID, last).Scan(&rm.Mentions)
		a.db.QueryRow(`SELECT m.content,m.created_at,CASE WHEN m.is_ai THEN 'Hiveloop AI' ELSE COALESCE(NULLIF(u.name,''),u.username,'') END FROM room_messages m LEFT JOIN users u ON u.id=m.user_id
			WHERE m.room_id=? ORDER BY m.id DESC LIMIT 1`, rm.ID).Scan(&rm.LastText, &rm.LastAt, &rm.LastBy)
		if len([]rune(rm.LastText)) > 80 {
			rm.LastText = string([]rune(rm.LastText)[:80]) + "…"
		}
	}
	sortDMs(rooms[nGroups:])
	jsonOK(w, rooms)
}

func (a *App) handleRoomMessages(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	room := pathID(r, "id")
	if !a.canRoom(u, room) {
		jsonErr(w, 403, "forbidden")
		return
	}
	q := roomMsgSelect + ` WHERE m.room_id=?`
	args := []any{room}
	if b, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64); err == nil && b > 0 {
		q += ` AND m.id<?`
		args = append(args, b)
	}
	if af, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64); err == nil && af > 0 {
		q += ` AND m.id>?`
		args = append(args, af)
	}
	rows, err := a.db.Query(`SELECT * FROM (`+q+` ORDER BY m.id DESC LIMIT 60) ORDER BY 1`, args...)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	out := []RoomMsg{}
	for rows.Next() {
		if m, err := a.scanRoomMsg(rows); err == nil {
			out = append(out, m)
		}
	}
	rows.Close()
	a.fillMentions(out)
	jsonOK(w, out)
}

// roomMembers: users who can see the room.
func (a *App) roomMembers(room int64) []User {
	q := `SELECT id,username,name,role,lang,created_at FROM users WHERE status='active'`
	args := []any{}
	if isDM(room) {
		x, y, _ := a.dmPair(room)
		q += ` AND id IN (?,?)`
		args = append(args, x, y)
	} else if room != 0 {
		q += ` AND (id IN (SELECT user_id FROM group_members WHERE group_id=?) OR role IN ('admin','superadmin'))`
		args = append(args, room)
	}
	rows, err := a.db.Query(q+` ORDER BY name COLLATE NOCASE, username`, args...)
	out := []User{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang, &u.CreatedAt)
		out = append(out, u)
	}
	return out
}

func (a *App) handleRoomMembers(w http.ResponseWriter, r *http.Request) {
	room := pathID(r, "id")
	if !a.canRoom(currentUser(r), room) {
		jsonErr(w, 403, "forbidden")
		return
	}
	jsonOK(w, a.roomMembers(room))
}

var mentionRe = regexp.MustCompile(`(^|[^\w@.])@([a-zA-Z0-9._-]{2,32})`)

func (a *App) handlePostRoomMessage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	room := pathID(r, "id")
	if !a.canRoom(u, room) {
		jsonErr(w, 403, "forbidden")
		return
	}
	var in struct {
		Content string `json:"content"`
		DocID   *int64 `json:"doc_id"`
		ReplyTo *int64 `json:"reply_to"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if !a.rlMsg.allow(fmt.Sprint(u.ID)) {
		jsonErr(w, http.StatusTooManyRequests, "too_many_messages")
		return
	}
	in.Content = strings.TrimSpace(in.Content)
	if (in.Content == "" && in.DocID == nil) || len(in.Content) > 8000 {
		jsonErr(w, 400, "bad_request")
		return
	}
	if isDM(room) && in.DocID != nil { // documents are shared with groups, not in direct messages
		jsonErr(w, 400, "bad_request")
		return
	}
	// Attaching a document shares it with the room (owner only), or it must already be visible there.
	if in.DocID != nil {
		if d, err := a.getDoc(u.ID, *in.DocID); err == nil {
			a.db.Exec(`INSERT OR IGNORE INTO doc_shares(doc_id,group_id,created_at) VALUES(?,?,?)`, d.ID, room, time.Now().Unix())
		} else {
			var n int
			a.db.QueryRow(`SELECT COUNT(*) FROM doc_shares WHERE doc_id=? AND (group_id=0 OR group_id=?)`, *in.DocID, room).Scan(&n)
			if n == 0 {
				jsonErr(w, 403, "forbidden")
				return
			}
		}
	}
	// replying: the original must be in the same room. Replying to the assistant asks it
	// again (no @ai needed); replying to someone notifies them like a mention.
	var replyAI bool
	var replyUser sql.NullInt64
	if in.ReplyTo != nil {
		var rroom int64
		if err := a.db.QueryRow(`SELECT room_id,is_ai,user_id FROM room_messages WHERE id=?`, *in.ReplyTo).Scan(&rroom, &replyAI, &replyUser); err != nil || rroom != room {
			in.ReplyTo, replyAI = nil, false
		}
	}
	res, err := a.db.Exec(`INSERT INTO room_messages(room_id,user_id,content,doc_id,reply_to,created_at) VALUES(?,?,?,?,?,?)`,
		room, u.ID, in.Content, in.DocID, in.ReplyTo, time.Now().Unix())
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	id, _ := res.LastInsertId()

	// mentions: @username, or @tutti / @all for everyone in the room
	members := a.roomMembers(room)
	byName := map[string]int64{}
	for _, m := range members {
		byName[strings.ToLower(m.Username)] = m.ID
	}
	targets := map[int64]bool{}
	askAI := replyAI && in.Content != ""
	if replyUser.Valid {
		targets[replyUser.Int64] = true
	}
	for _, mm := range mentionRe.FindAllStringSubmatch(in.Content, -1) {
		name := strings.ToLower(strings.TrimRight(mm[2], "."))
		if name == "ai" {
			askAI = true
		} else if name == "tutti" || name == "all" {
			for _, m := range members {
				targets[m.ID] = true
			}
		} else if uid, ok := byName[name]; ok {
			targets[uid] = true
		}
	}
	if isDM(room) { // the other person is always notified; no assistant here
		askAI = false
		for _, m := range members {
			targets[m.ID] = true
		}
	}
	delete(targets, u.ID)
	for uid := range targets {
		a.db.Exec(`INSERT OR IGNORE INTO mentions(message_id,user_id) VALUES(?,?)`, id, uid)
	}
	a.db.Exec(`INSERT INTO room_reads(user_id,room_id,last_id) VALUES(?,?,?) ON CONFLICT(user_id,room_id) DO UPDATE SET last_id=MAX(last_id,excluded.last_id)`, u.ID, room, id)

	m, _ := a.scanRoomMsg(a.db.QueryRow(roomMsgSelect+` WHERE m.id=?`, id))
	ms := []RoomMsg{m}
	a.fillMentions(ms)
	name, _ := a.roomName(room)
	a.broadcast(room, map[string]any{"type": "message", "msg": ms[0], "room_name": name})
	a.notifyMessage(u, room, name, ms[0], targets)
	if askAI && a.aiEnabled() && a.rlAI.allow(fmt.Sprint(u.ID)) {
		if why := a.useAI(u.ID, room); why == "" {
			go a.roomAI(u, room, name, id)
		} else {
			go a.postAI(u, room, name, "⚠️ "+why, "", id)
		}
	}
	jsonOK(w, ms[0])
}

// roomAI answers an @ai mention inside a chat, acting with the asker's permissions.
func (a *App) roomAI(u *User, room int64, roomName string, askID int64) {
	a.broadcast(room, map[string]any{"type": "ai_typing", "room_id": room, "on": true})
	defer a.broadcast(room, map[string]any{"type": "ai_typing", "room_id": room, "on": false})
	select {
	case aiSlots <- struct{}{}:
		defer func() { <-aiSlots }()
	case <-time.After(2 * time.Minute):
		a.postAI(u, room, roomName, "⚠️ ai_busy", "", askID)
		return
	}
	// recent conversation as context
	rows, err := a.db.Query(`SELECT * FROM (`+roomMsgSelect+` WHERE m.room_id=? ORDER BY m.id DESC LIMIT 30) ORDER BY 1`, room)
	if err != nil {
		return
	}
	var tr strings.Builder
	for rows.Next() {
		m, err := a.scanRoomMsg(rows)
		if err != nil {
			continue
		}
		t := time.Unix(m.CreatedAt, 0).In(a.cfg.TZ).Format("02/01 15:04")
		fmt.Fprintf(&tr, "[%s] #%d %s (@%s)", t, m.ID, m.Author, m.Username)
		if m.Reply != nil {
			fmt.Fprintf(&tr, " (replying to #%d %s: %q)", m.Reply.ID, m.Reply.Author, m.Reply.Text)
		}
		fmt.Fprintf(&tr, ": %s", m.Content)
		if m.Doc != nil {
			fmt.Fprintf(&tr, " [attached document id %d: %s]", m.Doc.ID, m.Doc.Name)
		}
		tr.WriteString("\n")
	}
	rows.Close()
	where := "the GENERAL chat (everyone in the organisation)"
	if room != 0 {
		where = fmt.Sprintf("the chat of group %q (group id %d)", roomName, room)
	}
	name := u.Name
	if name == "" {
		name = u.Username
	}
	sys := a.systemPrompt(u) + fmt.Sprintf(`

CONTEXT: you are now answering inside %s, because %s (@%s) mentioned you with @ai or replied to one of your messages. Everyone in this chat reads your answer, so keep it short and useful for the group.
Act with %s's permissions. Documents: you can ONLY list/read documents shared in this chat (list_documents shows them). Never reveal private documents. Markdown files you create are shared in this chat automatically.
When the request is about "this group", use group id %d for calendar events. Attendance (signup_event, "presenza") is marked only for the person who asked, never for others.`, where, name, u.Username, name, room)
	msgs := []oMsg{{Role: "system", Content: sys}, {Role: "user", Content: "Chat transcript (latest last):\n" + tr.String() + fmt.Sprintf("\nAnswer message #%d (it mentions @ai or replies to you; if it replies to you, it continues that conversation).", askID)}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	text, tools, errMsg := a.runAgent(ctx, u, &room, msgs, func(string) {}, func(string, *toolResult) {})
	meta := ""
	if len(tools) > 0 || errMsg != "" {
		b, _ := json.Marshal(map[string]any{"tools": tools, "error": errMsg})
		meta = string(b)
	}
	if text == "" && errMsg != "" {
		text = "⚠️ " + errMsg
	}
	a.postAI(u, room, roomName, text, meta, askID)
}

func (a *App) postAI(asker *User, room int64, roomName, text, meta string, replyTo int64) {
	res, err := a.db.Exec(`INSERT INTO room_messages(room_id,user_id,content,is_ai,meta,reply_to,created_at) VALUES(?,NULL,?,1,?,?,?)`, room, text, meta, replyTo, time.Now().Unix())
	if err != nil {
		return
	}
	id, _ := res.LastInsertId()
	a.db.Exec(`INSERT OR IGNORE INTO mentions(message_id,user_id) VALUES(?,?)`, id, asker.ID)
	m, _ := a.scanRoomMsg(a.db.QueryRow(roomMsgSelect+` WHERE m.id=?`, id))
	ms := []RoomMsg{m}
	a.fillMentions(ms)
	a.broadcast(room, map[string]any{"type": "message", "msg": ms[0], "room_name": roomName})
}

func (a *App) handleDeleteRoomMessage(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	id := pathID(r, "id")
	var room int64
	var uid sql.NullInt64
	if err := a.db.QueryRow(`SELECT room_id,user_id FROM room_messages WHERE id=?`, id).Scan(&room, &uid); err != nil {
		jsonErr(w, 404, "not_found")
		return
	}
	mine := uid.Valid && uid.Int64 == u.ID
	if !(mine || (u.IsAdmin() && !isDM(room))) { // nobody else touches direct messages
		jsonErr(w, 403, "forbidden")
		return
	}
	a.db.Exec(`DELETE FROM room_messages WHERE id=?`, id)
	a.broadcast(room, map[string]any{"type": "delete", "id": id, "room_id": room})
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleRoomRead(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	room := pathID(r, "id")
	if !a.canRoom(u, room) {
		jsonErr(w, 403, "forbidden")
		return
	}
	var max int64
	a.db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM room_messages WHERE room_id=?`, room).Scan(&max)
	a.db.Exec(`INSERT INTO room_reads(user_id,room_id,last_id) VALUES(?,?,?) ON CONFLICT(user_id,room_id) DO UPDATE SET last_id=excluded.last_id`, u.ID, room, max)
	jsonOK(w, map[string]bool{"ok": true})
}

// oneLine collapses whitespace and newlines (and strips Markdown markers) for previews.
func oneLine(s string) string {
	s = strings.NewReplacer("**", "", "__", "", "`", "", "#", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// notifyMessage pushes a direct message, a mention or a reply to the people concerned.
func (a *App) notifyMessage(from *User, room int64, roomName string, m RoomMsg, targets map[int64]bool) {
	if len(targets) == 0 {
		return
	}
	ids := make([]int64, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	author := from.Name
	if author == "" {
		author = from.Username
	}
	text := m.Content
	if text == "" && m.Doc != nil {
		text = "📎 " + m.Doc.Name
	}
	langs := a.userLangs(ids)
	a.notify(ids, func(uid int64) Notice {
		lang := langs[uid]
		title := author
		switch {
		case isDM(room):
		case room == 0:
			title += " · " + tr(lang, "Generale", "General")
		default:
			title += " · " + roomName
		}
		return Notice{Title: title, Body: oneLine(text), URL: fmt.Sprintf("/#/chat/%d", room), Tag: fmt.Sprintf("chat%d", room)}
	})
}

func (a *App) userLangs(ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	list := make([]string, len(ids))
	for i, id := range ids {
		list[i] = strconv.FormatInt(id, 10)
	}
	rows, err := a.db.Query(`SELECT id,lang FROM users WHERE id IN (` + strings.Join(list, ",") + `)`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var l string
		if rows.Scan(&id, &l) == nil {
			out[id] = l
		}
	}
	return out
}
