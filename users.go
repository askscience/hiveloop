package main

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,32}$`)

var (
	spacesRe      = regexp.MustCompile(`\s+`)
	usernameBadRe = regexp.MustCompile(`[^a-z0-9._-]`)
)

// normalizeUsername turns "Jane Dœ" into "jane.doe".
func normalizeUsername(s string) string {
	// a Chain is stateful: build one per call
	if r, _, err := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s); err == nil {
		s = r
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = spacesRe.ReplaceAllString(s, ".")
	return usernameBadRe.ReplaceAllString(s, "")
}

// canManage reports whether actor may modify/delete target.
func canManage(actor, target *User) bool {
	if target.Role == "superadmin" {
		return false
	}
	if actor.IsSuper() {
		return true
	}
	return actor.Role == "admin" && target.Role == "user"
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := `SELECT id,username,name,role,lang,status,created_at FROM users`
	if !currentUser(r).IsAdmin() {
		q += ` WHERE status='active'`
	}
	rows, err := a.db.Query(q + ` ORDER BY name COLLATE NOCASE, username`)
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang, &u.Status, &u.CreatedAt)
		out = append(out, u)
	}
	rows.Close()
	if currentUser(r).IsSuper() {
		a.userUsage(out)
	}
	jsonOK(w, out)
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	var in struct {
		Username, Name, Password, Role string
		Groups                         []int64
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	in.Username = normalizeUsername(in.Username)
	if !usernameRe.MatchString(in.Username) {
		jsonErr(w, 400, "invalid_username")
		return
	}
	if in.Role == "" {
		in.Role = "user"
	}
	if in.Role != "user" && in.Role != "admin" {
		jsonErr(w, 400, "invalid_role")
		return
	}
	if in.Role == "admin" && !actor.IsSuper() {
		jsonErr(w, 403, "forbidden")
		return
	}
	if len(in.Password) < 8 {
		jsonErr(w, 400, "password_too_short")
		return
	}
	now := time.Now().Unix()
	res, err := a.db.Exec(`INSERT INTO users(username,name,password_hash,role,created_at) VALUES(?,?,'!',?,?)`,
		in.Username, clip(in.Name, 80), in.Role, now)
	if err != nil {
		jsonErr(w, 409, "username_taken")
		return
	}
	id, _ := res.LastInsertId()
	if err := a.setPassword(id, in.Password); err != nil {
		a.db.Exec(`DELETE FROM users WHERE id=?`, id)
		jsonErr(w, 400, err.Error())
		return
	}
	for _, g := range in.Groups {
		a.db.Exec(`INSERT OR IGNORE INTO group_members(group_id,user_id,joined_at) VALUES(?,?,?)`, g, id, now)
	}
	u, _ := a.getUser(id)
	jsonOK(w, u)
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	target, err := a.getUser(pathID(r, "id"))
	if err != nil {
		jsonErr(w, 404, "not_found")
		return
	}
	if !canManage(actor, target) {
		jsonErr(w, 403, "forbidden")
		return
	}
	var in struct {
		Name     *string `json:"name"`
		Password *string `json:"password"`
		Role     *string `json:"role"`
		QuotaMB  *int    `json:"quota_mb"` // super admin: 0 = default
		AIDaily  *int    `json:"ai_daily"` // super admin: -1 = default, 0 = no assistant
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.Role != nil {
		if !actor.IsSuper() || (*in.Role != "user" && *in.Role != "admin") {
			jsonErr(w, 403, "forbidden")
			return
		}
		a.db.Exec(`UPDATE users SET role=? WHERE id=?`, *in.Role, target.ID)
	}
	if in.Name != nil {
		a.db.Exec(`UPDATE users SET name=? WHERE id=?`, clip(*in.Name, 80), target.ID)
	}
	if in.QuotaMB != nil || in.AIDaily != nil {
		if !actor.IsSuper() {
			jsonErr(w, 403, "forbidden")
			return
		}
		if in.QuotaMB != nil && *in.QuotaMB >= 0 && *in.QuotaMB <= 100000 {
			a.db.Exec(`UPDATE users SET quota_mb=? WHERE id=?`, *in.QuotaMB, target.ID)
		}
		if in.AIDaily != nil && *in.AIDaily >= -1 && *in.AIDaily <= 10000 {
			a.db.Exec(`UPDATE users SET ai_daily=? WHERE id=?`, *in.AIDaily, target.ID)
		}
	}
	if in.Password != nil {
		if err := a.setPassword(target.ID, *in.Password); err != nil {
			jsonErr(w, 400, err.Error())
			return
		}
		a.db.Exec(`DELETE FROM sessions WHERE user_id=?`, target.ID)
	}
	u, _ := a.getUser(target.ID)
	jsonOK(w, u)
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	actor := currentUser(r)
	target, err := a.getUser(pathID(r, "id"))
	if err != nil {
		jsonErr(w, 404, "not_found")
		return
	}
	if target.ID == actor.ID || !canManage(actor, target) {
		jsonErr(w, 403, "forbidden")
		return
	}
	if _, err := a.db.Exec(`DELETE FROM users WHERE id=?`, target.ID); err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	os.RemoveAll(a.userDir(target.ID))
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) userDir(uid int64) string {
	return filepath.Join(a.cfg.DataDir, "files", strconv.FormatInt(uid, 10))
}
