package main

// Live notifications pushed to every open tab over the /api/stream SSE connection:
// calendar, groups, documents and sign-up requests refresh without reloading.

import (
	"encoding/json"
	"net/http"
)

// broadcastWhere sends v to every connected user for which keep returns true.
func (a *App) broadcastWhere(v any, keep func(*User) bool) {
	b, _ := json.Marshal(v)
	liveHub.mu.Lock()
	defer liveHub.mu.Unlock()
	for ch, u := range liveHub.subs {
		if keep != nil && !keep(u) {
			continue
		}
		select {
		case ch <- b:
		default:
		}
	}
}

func (a *App) liveAll(kind string) { a.broadcastWhere(map[string]any{"type": kind}, nil) }

func (a *App) liveAdmins(kind string, extra map[string]any) {
	m := map[string]any{"type": kind}
	for k, v := range extra {
		m[k] = v
	}
	a.broadcastWhere(m, func(u *User) bool { return u.IsAdmin() })
}

// liveEvent announces a calendar change. A private event's details go only to the people
// who can see it; everyone else just refreshes (so an event that turned private disappears).
func (a *App) liveEvent(by *User, action string, e Event) {
	name := by.Name
	if name == "" {
		name = by.Username
	}
	full := map[string]any{
		"type": "event", "action": action, "id": e.ID, "title": e.Title, "start": e.Start,
		"group_id": e.GroupID, "group_name": e.GroupName, "by": name, "by_id": by.ID, "via_ai": e.ViaAI,
	}
	if !e.Private {
		a.broadcastWhere(full, nil)
		return
	}
	members := map[int64]bool{}
	if e.GroupID != nil {
		for _, id := range a.groupMemberIDs(*e.GroupID) {
			members[id] = true
		}
	}
	sees := func(u *User) bool { return u.IsAdmin() || members[u.ID] }
	a.broadcastWhere(full, sees)
	a.broadcastWhere(map[string]any{"type": "event", "action": "refresh"}, func(u *User) bool { return !sees(u) })
}

// groupMemberIDs: active members of a group.
func (a *App) groupMemberIDs(gid int64) []int64 {
	rows, err := a.db.Query(`SELECT m.user_id FROM group_members m JOIN users u ON u.id=m.user_id WHERE m.group_id=? AND u.status='active'`, gid)
	out := []int64{}
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		out = append(out, id)
	}
	return out
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

// changes wraps a handler and, if it succeeded, notifies clients that `kind` changed.
func (a *App) changes(kind string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: 200}
		h.ServeHTTP(sw, r)
		if sw.code < 400 {
			switch kind {
			case "pending":
				a.liveAdmins("pending", map[string]any{"count": a.pendingCount()})
			default:
				a.liveAll(kind)
			}
		}
	})
}
