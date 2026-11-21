package main

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Members     int    `json:"members"`
	IsMember    bool   `json:"is_member"`
	CreatedAt   int64  `json:"created_at"`
	LeaderID    int64  `json:"leader_id"`
	LeaderName  string `json:"leader_name"`
	IsLeader    bool   `json:"is_leader"`
	ImageV      int64  `json:"image_v"` // 0 = no cover image
}

func (a *App) userGroupIDs(uid int64) []int64 {
	rows, err := a.db.Query(`SELECT group_id FROM group_members WHERE user_id=?`, uid)
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

func (a *App) isMember(uid, gid int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE user_id=? AND group_id=?`, uid, gid).Scan(&n)
	return n > 0
}

func (a *App) listGroups(uid int64) ([]Group, error) {
	rows, err := a.db.Query(`SELECT g.id,g.name,g.description,g.color,g.created_at,
		(SELECT COUNT(*) FROM group_members m WHERE m.group_id=g.id),
		EXISTS(SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=?),
		COALESCE(g.leader_id,0), COALESCE(NULLIF(lu.name,''),lu.username,''), g.image_v
		FROM groups g LEFT JOIN users lu ON lu.id=g.leader_id ORDER BY g.name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		rows.Scan(&g.ID, &g.Name, &g.Description, &g.Color, &g.CreatedAt, &g.Members, &g.IsMember, &g.LeaderID, &g.LeaderName, &g.ImageV)
		g.IsLeader = g.LeaderID != 0 && g.LeaderID == uid
		out = append(out, g)
	}
	return out, nil
}

func (a *App) handleListGroups(w http.ResponseWriter, r *http.Request) {
	gs, err := a.listGroups(currentUser(r).ID)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	jsonOK(w, gs)
}

type groupInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Color       *string `json:"color"`
}

func (a *App) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var in groupInput
	if err := decode(r, &in); err != nil || in.Name == nil || strings.TrimSpace(*in.Name) == "" {
		jsonErr(w, 400, "bad_request")
		return
	}
	color := a.freeColor()
	if in.Color != nil && colorRe.MatchString(*in.Color) {
		if a.colorTaken(*in.Color, 0) {
			jsonErr(w, 409, "color_taken")
			return
		}
		color = *in.Color
	}
	desc := ""
	if in.Description != nil {
		desc = *in.Description
	}
	_, err := a.db.Exec(`INSERT INTO groups(name,description,color,created_by,created_at) VALUES(?,?,?,?,?)`,
		clip(*in.Name, 80), clip(desc, 1000), color, currentUser(r).ID, time.Now().Unix())
	if err != nil {
		jsonErr(w, 409, "group_exists")
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	var in groupInput
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		if _, err := a.db.Exec(`UPDATE groups SET name=? WHERE id=?`, clip(*in.Name, 80), id); err != nil {
			jsonErr(w, 409, "group_exists")
			return
		}
	}
	if in.Description != nil {
		a.db.Exec(`UPDATE groups SET description=? WHERE id=?`, clip(*in.Description, 1000), id)
	}
	if in.Color != nil && colorRe.MatchString(*in.Color) && currentUser(r).IsAdmin() { // colour: admins only
		if a.colorTaken(*in.Color, id) {
			jsonErr(w, 409, "color_taken")
			return
		}
		a.db.Exec(`UPDATE groups SET color=? WHERE id=?`, *in.Color, id)
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	a.db.Exec(`DELETE FROM groups WHERE id=?`, id)
	a.db.Exec(`DELETE FROM doc_shares WHERE group_id=?`, id)
	a.db.Exec(`DELETE FROM room_messages WHERE room_id=?`, id)
	a.db.Exec(`DELETE FROM room_reads WHERE room_id=?`, id)
	os.Remove(a.groupImagePath(id))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) groupExists(id int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM groups WHERE id=?`, id).Scan(&n)
	return n > 0
}

func (a *App) handleJoinGroup(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if !a.groupExists(id) {
		jsonErr(w, 404, "not_found")
		return
	}
	a.db.Exec(`INSERT OR IGNORE INTO group_members(group_id,user_id,joined_at) VALUES(?,?,?)`, id, currentUser(r).ID, time.Now().Unix())
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleLeaveGroup(w http.ResponseWriter, r *http.Request) {
	gid, uid := pathID(r, "id"), currentUser(r).ID
	a.db.Exec(`DELETE FROM group_members WHERE group_id=? AND user_id=?`, gid, uid)
	a.memberLeft(gid, uid)
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleGroupMembers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(`SELECT u.id,u.username,u.name,u.role,u.lang,u.created_at FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? ORDER BY u.name COLLATE NOCASE`, pathID(r, "id"))
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang, &u.CreatedAt)
		out = append(out, u)
	}
	jsonOK(w, out)
}

func (a *App) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID int64 `json:"user_id"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	var active int
	a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=? AND status='active'`, in.UserID).Scan(&active)
	if active == 0 {
		jsonErr(w, 400, "bad_request")
		return
	}
	if _, err := a.db.Exec(`INSERT OR IGNORE INTO group_members(group_id,user_id,joined_at) VALUES(?,?,?)`,
		pathID(r, "id"), in.UserID, time.Now().Unix()); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	gid, uid := pathID(r, "id"), pathID(r, "uid")
	a.db.Exec(`DELETE FROM group_members WHERE group_id=? AND user_id=?`, gid, uid)
	a.memberLeft(gid, uid)
	jsonOK(w, map[string]bool{"ok": true})
}

// groupColors is the palette offered to admins; each colour can be used by one group only.
var groupColors = []string{"#334155", "#e11d48", "#d946ef", "#7c3aed", "#4f46e5", "#0284c7", "#0d9488", "#16a34a", "#ca8a04", "#78716c",
	"#dc2626", "#b45309", "#65a30d", "#059669", "#0891b2", "#2563eb", "#9333ea", "#c026d3", "#db2777", "#be123c",
	"#a16207", "#57534e", "#0f766e", "#1d4ed8", "#6d28d9", "#4d7c0f"}

// colorTaken: another group (not except) already has this colour.
func (a *App) colorTaken(color string, except int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM groups WHERE lower(color)=lower(?) AND id<>?`, color, except).Scan(&n)
	return n > 0
}

// freeColor: the first palette colour no group uses (the first one if all are taken).
func (a *App) freeColor() string {
	for _, c := range groupColors {
		if !a.colorTaken(c, 0) {
			return c
		}
	}
	return groupColors[0]
}
