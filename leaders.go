package main

// Group leaders. One per group, elected in the group chat: every member
// votes for a member, whoever gets a majority of the members becomes leader.
// The leader can add/remove members and manage documents shared with the group.
// Admins can appoint or remove a leader directly; the leader can step down.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func (a *App) leaderOf(gid int64) (int64, string) {
	var id sql.NullInt64
	var name string
	a.db.QueryRow(`SELECT g.leader_id, COALESCE(NULLIF(u.name,''),u.username,'') FROM groups g LEFT JOIN users u ON u.id=g.leader_id WHERE g.id=?`, gid).Scan(&id, &name)
	if !id.Valid {
		return 0, ""
	}
	return id.Int64, name
}

func (a *App) isLeader(uid, gid int64) bool {
	l, _ := a.leaderOf(gid)
	return l != 0 && l == uid
}

// canManageGroup: admins everywhere, leaders in their own group.
func (a *App) canManageGroup(u *User, gid int64) bool {
	return u.IsAdmin() || a.isLeader(u.ID, gid)
}

func (a *App) groupManager(h http.HandlerFunc) http.Handler {
	return a.auth(func(w http.ResponseWriter, r *http.Request) {
		if !a.canManageGroup(currentUser(r), pathID(r, "id")) {
			jsonErr(w, 403, "forbidden")
			return
		}
		h(w, r)
	})
}

// systemMessage posts a small notice in a group chat (no author).
// sysMsg is a chat notice: a translation key for the app plus its values, and an English
// fallback text.
type sysMsg struct {
	Key  string
	Args map[string]any
	Text string
}

func (a *App) systemMessage(room int64, m0 sysMsg) {
	meta, _ := json.Marshal(map[string]any{"system": true, "key": m0.Key, "args": m0.Args})
	res, err := a.db.Exec(`INSERT INTO room_messages(room_id,user_id,content,meta,created_at) VALUES(?,NULL,?,?,?)`, room, m0.Text, string(meta), time.Now().Unix())
	if err != nil {
		return
	}
	id, _ := res.LastInsertId()
	m, err := a.scanRoomMsg(a.db.QueryRow(roomMsgSelect+` WHERE m.id=?`, id))
	if err != nil {
		return
	}
	name, _ := a.roomName(room)
	a.broadcast(room, map[string]any{"type": "message", "msg": m, "room_name": name})
}

func (a *App) setLeader(gid, uid int64, why sysMsg) {
	if uid == 0 {
		a.db.Exec(`UPDATE groups SET leader_id=NULL WHERE id=?`, gid)
	} else {
		a.db.Exec(`UPDATE groups SET leader_id=? WHERE id=?`, uid, gid)
	}
	a.db.Exec(`DELETE FROM group_votes WHERE group_id=?`, gid)
	if why.Key != "" {
		a.systemMessage(gid, why)
	}
	a.liveAll("groups")
}

func (a *App) displayName(uid int64) string {
	var n string
	a.db.QueryRow(`SELECT COALESCE(NULLIF(name,''),username) FROM users WHERE id=?`, uid).Scan(&n)
	return n
}

// memberLeft clears the leader role and the votes of someone who is no longer in the group.
func (a *App) memberLeft(gid, uid int64) {
	a.db.Exec(`DELETE FROM group_votes WHERE group_id=? AND (voter_id=? OR candidate_id=?)`, gid, uid, uid)
	if a.isLeader(uid, gid) {
		n := a.displayName(uid)
		a.setLeader(gid, 0, sysMsg{"sys_leader_gone", map[string]any{"name": n}, fmt.Sprintf("%s is no longer the group leader: a new vote can start.", n)})
	}
}

type voteRow struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Votes    int    `json:"votes"`
}

func (a *App) voteState(gid, uid int64) map[string]any {
	rows, _ := a.db.Query(`SELECT u.id, COALESCE(NULLIF(u.name,''),u.username), u.username,
		(SELECT COUNT(*) FROM group_votes v JOIN group_members vm ON vm.group_id=v.group_id AND vm.user_id=v.voter_id
		 WHERE v.group_id=? AND v.candidate_id=u.id)
		FROM group_members m JOIN users u ON u.id=m.user_id WHERE m.group_id=? AND u.status='active'
		ORDER BY u.name COLLATE NOCASE`, gid, gid)
	list := []voteRow{}
	if rows != nil {
		for rows.Next() {
			var v voteRow
			rows.Scan(&v.ID, &v.Name, &v.Username, &v.Votes)
			list = append(list, v)
		}
		rows.Close()
	}
	var mine sql.NullInt64
	a.db.QueryRow(`SELECT candidate_id FROM group_votes WHERE group_id=? AND voter_id=?`, gid, uid).Scan(&mine)
	leader, lname := a.leaderOf(gid)
	return map[string]any{"candidates": list, "my_vote": mine.Int64, "needed": len(list)/2 + 1, "members": len(list), "leader_id": leader, "leader_name": lname}
}

func (a *App) handleVotes(w http.ResponseWriter, r *http.Request) {
	u, gid := currentUser(r), pathID(r, "id")
	if !a.isMember(u.ID, gid) && !u.IsAdmin() {
		jsonErr(w, 403, "forbidden")
		return
	}
	jsonOK(w, a.voteState(gid, u.ID))
}

func (a *App) handleVote(w http.ResponseWriter, r *http.Request) {
	u, gid := currentUser(r), pathID(r, "id")
	if !a.isMember(u.ID, gid) {
		jsonErr(w, 403, "forbidden")
		return
	}
	if l, _ := a.leaderOf(gid); l != 0 {
		jsonErr(w, 409, "leader_exists")
		return
	}
	var in struct {
		CandidateID int64 `json:"candidate_id"`
	}
	if err := decode(r, &in); err != nil || !a.isMember(in.CandidateID, gid) {
		jsonErr(w, 400, "bad_request")
		return
	}
	a.db.Exec(`INSERT INTO group_votes(group_id,voter_id,candidate_id,created_at) VALUES(?,?,?,?)
		ON CONFLICT(group_id,voter_id) DO UPDATE SET candidate_id=excluded.candidate_id, created_at=excluded.created_at`,
		gid, u.ID, in.CandidateID, time.Now().Unix())
	st := a.voteState(gid, u.ID)
	for _, c := range st["candidates"].([]voteRow) {
		if c.Votes >= st["needed"].(int) {
			a.setLeader(gid, c.ID, sysMsg{"sys_leader_elected", map[string]any{"name": c.Name, "votes": c.Votes, "members": st["members"].(int)},
				fmt.Sprintf("%s is the new group leader (%d votes out of %d members).", c.Name, c.Votes, st["members"].(int))})
			st = a.voteState(gid, u.ID)
			break
		}
	}
	a.liveAll("groups")
	jsonOK(w, st)
}

// PUT /api/groups/{id}/leader {user_id}: admins appoint (0 = remove).
func (a *App) handleSetLeader(w http.ResponseWriter, r *http.Request) {
	gid := pathID(r, "id")
	var in struct {
		UserID int64 `json:"user_id"`
	}
	if err := decode(r, &in); err != nil || (in.UserID != 0 && !a.isMember(in.UserID, gid)) {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.UserID == 0 {
		a.setLeader(gid, 0, sysMsg{"sys_leader_removed", nil, "An administrator removed the group leader: a new vote can start."})
	} else {
		n := a.displayName(in.UserID)
		a.setLeader(gid, in.UserID, sysMsg{"sys_leader_appointed", map[string]any{"name": n}, fmt.Sprintf("An administrator appointed %s as group leader.", n)})
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// DELETE /api/groups/{id}/leader: the leader steps down (or an admin removes them).
func (a *App) handleResignLeader(w http.ResponseWriter, r *http.Request) {
	u, gid := currentUser(r), pathID(r, "id")
	if !a.isLeader(u.ID, gid) && !u.IsAdmin() {
		jsonErr(w, 403, "forbidden")
		return
	}
	who, _ := a.leaderOf(gid)
	n := a.displayName(who)
	a.setLeader(gid, 0, sysMsg{"sys_leader_resigned", map[string]any{"name": n}, fmt.Sprintf("%s stepped down as group leader: a new vote can start.", n)})
	jsonOK(w, map[string]bool{"ok": true})
}

// DELETE /api/docs/{id}/shares/{gid}: remove a document from a group (owner, admin, or that group's leader).
func (a *App) handleUnshareDoc(w http.ResponseWriter, r *http.Request) {
	u, id, gid := currentUser(r), pathID(r, "id"), pathID(r, "gid")
	d, err := a.accessibleDoc(u, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !(d.OwnerID == u.ID || u.IsAdmin() || (gid != 0 && a.isLeader(u.ID, gid))) {
		jsonErr(w, 403, "forbidden")
		return
	}
	a.db.Exec(`DELETE FROM doc_shares WHERE doc_id=? AND group_id=?`, id, gid)
	a.liveAll("docs")
	jsonOK(w, map[string]bool{"ok": true})
}

// leadsGroupOf: is u the leader of a group this document is shared with?
func (a *App) leadsGroupOf(u *User, docID int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM doc_shares s JOIN groups g ON g.id=s.group_id WHERE s.doc_id=? AND g.leader_id=?`, docID, u.ID).Scan(&n)
	return n > 0
}
