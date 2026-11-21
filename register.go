package main

// Self-registration. Mode (set by the super admin):
//   closed   – only admins create accounts
//   approval – anyone can sign up, an admin approves before first login (default)
//   open     – anyone can sign up and use the app immediately

import (
	"log"
	"net/http"
	"strings"
	"time"
)

func (a *App) registrationMode() string {
	switch m := a.db.Setting("registration"); m {
	case "closed", "open", "approval":
		return m
	}
	return "approval"
}

// handleConfig is public: the login page uses it to show the sign-up link.
func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"registration": a.registrationMode()})
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !a.rlReg.allow(clientIP(r)) {
		jsonErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	mode := a.registrationMode()
	if mode == "closed" {
		jsonErr(w, http.StatusForbidden, "registration_closed")
		return
	}
	var in struct {
		Username, Name, Password string
		Website                  string // honeypot: humans leave it empty
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.Website != "" {
		jsonOK(w, map[string]string{"status": "pending"})
		return
	}
	in.Username = normalizeUsername(in.Username)
	in.Name = strings.TrimSpace(in.Name)
	if !usernameRe.MatchString(in.Username) {
		jsonErr(w, 400, "invalid_username")
		return
	}
	if in.Name == "" || len([]rune(in.Name)) > 80 {
		jsonErr(w, 400, "name_required")
		return
	}
	if len(in.Password) < 8 {
		jsonErr(w, 400, "password_too_short")
		return
	}
	var pending int
	a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE status='pending'`).Scan(&pending)
	if mode == "approval" && pending >= 50 {
		jsonErr(w, http.StatusTooManyRequests, "too_many_pending")
		return
	}
	status := "pending"
	if mode == "open" {
		status = "active"
	}
	res, err := a.db.Exec(`INSERT INTO users(username,name,password_hash,role,status,created_at) VALUES(?,?,'!','user',?,?)`,
		in.Username, in.Name, status, time.Now().Unix())
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
	log.Printf("sign-up: user=%q status=%s ip=%s", in.Username, status, clientIP(r))
	if status == "active" {
		if err := a.startSession(w, id); err != nil {
			jsonErr(w, 500, "server_error")
			return
		}
	}
	jsonOK(w, map[string]string{"status": status, "username": in.Username})
}

func (a *App) handleApproveUser(w http.ResponseWriter, r *http.Request) {
	res, err := a.db.Exec(`UPDATE users SET status='active' WHERE id=? AND status='pending'`, pathID(r, "id"))
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonErr(w, 404, "not_found")
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleSetRegistration(w http.ResponseWriter, r *http.Request) {
	var in struct{ Mode string }
	if err := decode(r, &in); err != nil || (in.Mode != "closed" && in.Mode != "approval" && in.Mode != "open") {
		jsonErr(w, 400, "bad_request")
		return
	}
	a.db.SetSetting("registration", in.Mode)
	jsonOK(w, map[string]string{"registration": in.Mode})
}

func (a *App) pendingCount() int {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE status='pending'`).Scan(&n)
	return n
}
