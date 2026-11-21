package main

// Direct messages between two people. A conversation is a chat room with a negative id
// (-dm_rooms.id) and reuses messages, replies, unread counts and live updates. Only the
// two people can read it — admins included in neither.

import (
	"net/http"
	"sort"
)

const dmSchema = `CREATE TABLE IF NOT EXISTS dm_rooms (
  id INTEGER PRIMARY KEY,
  user_a INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  user_b INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  UNIQUE (user_a, user_b)
);
CREATE TRIGGER IF NOT EXISTS dm_rooms_gone AFTER DELETE ON dm_rooms BEGIN
  DELETE FROM room_messages WHERE room_id = -OLD.id;
  DELETE FROM room_reads WHERE room_id = -OLD.id;
END`

// DMPeer is the other person of a direct conversation.
type DMPeer struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
}

func isDM(room int64) bool { return room < 0 }

// dmPair returns the two people of a direct conversation.
func (a *App) dmPair(room int64) (int64, int64, bool) {
	if !isDM(room) {
		return 0, 0, false
	}
	var x, y int64
	if err := a.db.QueryRow(`SELECT user_a,user_b FROM dm_rooms WHERE id=?`, -room).Scan(&x, &y); err != nil {
		return 0, 0, false
	}
	return x, y, true
}

func (a *App) inDM(uid, room int64) bool {
	x, y, ok := a.dmPair(room)
	return ok && uid != 0 && (uid == x || uid == y)
}

// dmPeer: the other person, seen from uid.
func (a *App) dmPeer(uid, room int64) (DMPeer, bool) {
	x, y, ok := a.dmPair(room)
	if !ok || (uid != x && uid != y) {
		return DMPeer{}, false
	}
	other := x
	if uid == x {
		other = y
	}
	var p DMPeer
	err := a.db.QueryRow(`SELECT id,name,username FROM users WHERE id=?`, other).Scan(&p.ID, &p.Name, &p.Username)
	return p, err == nil
}

// dmRooms lists u's conversations that have messages (or that u opened).
func (a *App) dmRooms(u *User) []Room {
	rows, err := a.db.Query(`SELECT d.id, CASE WHEN d.user_a=? THEN d.user_b ELSE d.user_a END, d.created_by FROM dm_rooms d
		WHERE (d.user_a=? OR d.user_b=?)`, u.ID, u.ID, u.ID)
	if err != nil {
		return nil
	}
	type rec struct{ id, peer, by int64 }
	var recs []rec
	for rows.Next() {
		var r rec
		if rows.Scan(&r.id, &r.peer, &r.by) == nil {
			recs = append(recs, r)
		}
	}
	rows.Close()
	out := []Room{}
	for _, r := range recs {
		room := -r.id
		var n int
		a.db.QueryRow(`SELECT COUNT(*) FROM room_messages WHERE room_id=?`, room).Scan(&n)
		if n == 0 && r.by != u.ID {
			continue
		}
		var p DMPeer
		var status string
		if a.db.QueryRow(`SELECT id,name,username,status FROM users WHERE id=?`, r.peer).Scan(&p.ID, &p.Name, &p.Username, &status) != nil {
			continue
		}
		name := p.Name
		if name == "" {
			name = p.Username
		}
		out = append(out, Room{ID: room, Name: name, IsMember: true, DM: &p})
	}
	return out
}

// sortDMs: most recent conversation first (after the general chat and the groups).
func sortDMs(rooms []Room) {
	sort.SliceStable(rooms, func(i, j int) bool { return rooms[i].LastAt > rooms[j].LastAt })
}

// handleOpenDM finds or creates the conversation with another person.
func (a *App) handleOpenDM(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		UserID int64 `json:"user_id"`
	}
	if err := decode(r, &in); err != nil || in.UserID == 0 || in.UserID == u.ID {
		jsonErr(w, 400, "bad_request")
		return
	}
	var status string
	if a.db.QueryRow(`SELECT status FROM users WHERE id=?`, in.UserID).Scan(&status) != nil || status != "active" {
		jsonErr(w, 404, "not_found")
		return
	}
	x, y := u.ID, in.UserID
	if x > y {
		x, y = y, x
	}
	a.db.Exec(`INSERT OR IGNORE INTO dm_rooms(user_a,user_b,created_by,created_at) VALUES(?,?,?,strftime('%s','now'))`, x, y, u.ID)
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM dm_rooms WHERE user_a=? AND user_b=?`, x, y).Scan(&id); err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	// still empty: it shows up in the list of whoever opened it last
	a.db.Exec(`UPDATE dm_rooms SET created_by=? WHERE id=? AND NOT EXISTS (SELECT 1 FROM room_messages WHERE room_id=?)`, u.ID, id, -id)
	jsonOK(w, map[string]int64{"room_id": -id})
}
