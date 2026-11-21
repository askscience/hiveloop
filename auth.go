package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "hiveloop_session"
const sessionTTL = 30 * 24 * time.Hour

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("hiveloop-dummy-password"), bcrypt.DefaultCost)

type User struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	Lang      string     `json:"lang"`
	Status    string     `json:"status,omitempty"`
	CreatedAt int64      `json:"created_at"`
	Usage     *userUsage `json:"usage,omitempty"` // super admin's users list only
}

func (u *User) IsAdmin() bool { return u.Role == "admin" || u.Role == "superadmin" }
func (u *User) IsSuper() bool { return u.Role == "superadmin" }

type ctxKey struct{}

func currentUser(r *http.Request) *User { return r.Context().Value(ctxKey{}).(*User) }

// ---------- JSON helpers ----------

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

func pathID(r *http.Request, name string) int64 {
	id, _ := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id
}

// ---------- middleware ----------

func (a *App) userFromRequest(r *http.Request) *User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u := &User{}
	err = a.db.QueryRow(`SELECT u.id,u.username,u.name,u.role,u.lang,u.created_at FROM sessions s JOIN users u ON u.id=s.user_id
		WHERE s.token=? AND s.expires_at>? AND u.status='active'`, c.Value, time.Now().Unix()).
		Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang, &u.CreatedAt)
	if err != nil {
		return nil
	}
	return u
}

func (a *App) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := a.userFromRequest(r)
		if u == nil {
			jsonErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (a *App) admin(h http.HandlerFunc) http.Handler {
	return a.auth(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsAdmin() {
			jsonErr(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r)
	})
}

func (a *App) superadmin(h http.HandlerFunc) http.Handler {
	return a.auth(func(w http.ResponseWriter, r *http.Request) {
		if !currentUser(r).IsSuper() {
			jsonErr(w, http.StatusForbidden, "forbidden")
			return
		}
		h(w, r)
	})
}

// ---------- rate limiter (login) ----------

type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, max: max, window: window}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	var recent []time.Time
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func (l *rateLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, v := range l.hits {
		if len(v) == 0 || time.Since(v[len(v)-1]) > l.window {
			delete(l.hits, k)
		}
	}
}

// clientIP returns the caller's address. X-Forwarded-For is trusted only when the
// request comes from the local reverse proxy (Tailscale Funnel), and only its last hop:
// anything a client puts in that header itself is ignored.
func clientIP(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return host
}

// clip trims s and cuts it to at most n characters.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// ---------- handlers ----------

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.rl.allow(clientIP(r)) {
		jsonErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var in struct{ Username, Password string }
	if err := decode(r, &in); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	uname := normalizeUsername(in.Username)
	if len(in.Password) > 200 || !a.rlUser.allow("u:"+uname) { // slows down guessing one account from many addresses
		jsonErr(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	var id int64
	var hash, status string
	err := a.db.QueryRow(`SELECT id,password_hash,status FROM users WHERE username=?`, uname).Scan(&id, &hash, &status)
	if err != nil {
		// constant-ish time to avoid username probing
		bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
		log.Printf("login failed: user=%q ip=%s", uname, clientIP(r))
		jsonErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		log.Printf("login failed: user=%q ip=%s", uname, clientIP(r))
		jsonErr(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if status != "active" {
		jsonErr(w, http.StatusForbidden, "pending_approval")
		return
	}
	if err := a.startSession(w, id); err != nil {
		jsonErr(w, http.StatusInternalServerError, "server_error")
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) startSession(w http.ResponseWriter, uid int64) error {
	token := randomToken(32)
	exp := time.Now().Add(sessionTTL)
	if _, err := a.db.Exec(`INSERT INTO sessions(token,user_id,expires_at) VALUES(?,?,?)`, token, uid, exp.Unix()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.Exec(`DELETE FROM sessions WHERE token=?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.cfg.SecureCookie, SameSite: http.SameSiteLaxMode})
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	out := map[string]any{
		"user":         u,
		"groups":       a.userGroupIDs(u.ID),
		"registration": a.registrationMode(),
		"prefs":        a.userPrefs(u.ID),
		"avatar":       a.userAvatar(u.ID),
		"general":      a.generalLook(),
		"ai":           a.aiEnabled(),
		"ai_switch":    a.aiSwitchOn(),
		"ai_schedule":  a.aiSchedule(),
		"ai_quota":     a.aiQuota(u.ID, 0),
		"features":     map[string]bool{"doc_create": a.docCreateOn(), "doc_upload": a.docUploadOn()},
	}
	if u.IsAdmin() {
		out["pending"] = a.pendingCount()
	}
	jsonOK(w, out)
}

func (a *App) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Name        *string          `json:"name"`
		Lang        *string          `json:"lang"`
		Password    *string          `json:"password"`
		OldPassword string           `json:"old_password"`
		Prefs       *Prefs           `json:"prefs"`
		Avatar      *json.RawMessage `json:"avatar"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, http.StatusBadRequest, "bad_request")
		return
	}
	if in.Name != nil {
		a.db.Exec(`UPDATE users SET name=? WHERE id=?`, clip(*in.Name, 80), u.ID)
	}
	if in.Prefs != nil {
		a.setUserPrefs(u.ID, *in.Prefs)
	}
	if in.Avatar != nil { // null or {} removes it
		var av map[string]string
		json.Unmarshal(*in.Avatar, &av)
		a.setAvatar(u.ID, av)
	}
	if in.Lang != nil && (*in.Lang == "it" || *in.Lang == "en") {
		a.db.Exec(`UPDATE users SET lang=? WHERE id=?`, *in.Lang, u.ID)
	}
	if in.Password != nil {
		var hash string
		a.db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, u.ID).Scan(&hash)
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.OldPassword)) != nil {
			jsonErr(w, http.StatusForbidden, "wrong_password")
			return
		}
		if err := a.setPassword(u.ID, *in.Password); err != nil {
			jsonErr(w, http.StatusBadRequest, err.Error())
			return
		}
		// a new password signs out every other device
		if c, err := r.Cookie(sessionCookie); err == nil {
			a.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token<>?`, u.ID, c.Value)
		}
		log.Printf("password changed: user=%q ip=%s", u.Username, clientIP(r))
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) setPassword(uid int64, pw string) error {
	if len(pw) < 8 {
		return errors.New("password_too_short")
	}
	if len(pw) > 200 {
		return errors.New("bad_request")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`UPDATE users SET password_hash=? WHERE id=?`, string(hash), uid)
	return err
}

func (a *App) getUser(id int64) (*User, error) {
	u := &User{}
	err := a.db.QueryRow(`SELECT id,username,name,role,lang,created_at FROM users WHERE id=?`, id).
		Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Lang, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return u, err
}
