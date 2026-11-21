// Hiveloop — shared calendar, groups, private documents, AI assistant.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata" // timezone data built in (Alpine has no tzdata)
)

//go:embed web
var webFS embed.FS

type Config struct {
	Addr         string
	DataDir      string
	AIProvider   string // "ollama" or "openai" (any OpenAI-compatible API, e.g. DeepSeek)
	AIURL        string
	AIModel      string
	AIKey        string
	TZ           *time.Location
	MaxUpload    int64 // bytes per file
	UserQuota    int64 // bytes per user
	SecureCookie bool
}

type App struct {
	cfg *Config
	db  *DB
	rl  *rateLimiter // login attempts per IP
	// further abuse limits
	rlUser *rateLimiter // login attempts per username
	rlReg  *rateLimiter // sign-ups per IP
	rlAI   *rateLimiter // assistant requests per user
	rlMsg  *rateLimiter // chat messages per user
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func init() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

func main() {
	if runSubcommand() {
		return
	}
	tz, err := time.LoadLocation(env("HIVELOOP_TZ", "UTC"))
	if err != nil {
		log.Fatalf("timezone: %v", err)
	}
	cfg := &Config{
		Addr:         env("HIVELOOP_ADDR", "127.0.0.1:8090"),
		DataDir:      env("HIVELOOP_DATA", "./data"),
		AIProvider:   env("HIVELOOP_AI_PROVIDER", "ollama"),
		AIURL:        env("HIVELOOP_AI_URL", "http://127.0.0.1:11434"),
		AIModel:      env("HIVELOOP_MODEL", "llama3.1"),
		AIKey:        os.Getenv("HIVELOOP_AI_KEY"),
		TZ:           tz,
		MaxUpload:    25 << 20,
		UserQuota:    500 << 20,
		SecureCookie: env("HIVELOOP_SECURE_COOKIE", "1") == "1",
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "files"), 0o750); err != nil {
		log.Fatal(err)
	}

	db, err := OpenDB(filepath.Join(cfg.DataDir, "hiveloop.db"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	app := &App{cfg: cfg, db: db,
		rl:     newRateLimiter(10, time.Minute),
		rlUser: newRateLimiter(20, 15*time.Minute),
		rlReg:  newRateLimiter(5, time.Hour),
		rlAI:   newRateLimiter(40, 10*time.Minute),
		rlMsg:  newRateLimiter(40, time.Minute),
	}
	braveKeyFn = app.braveKey
	appTZ = cfg.TZ
	go app.optimizeGroupImages() // older covers: shrink once, in the background
	if err := app.bootstrap(); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	go app.janitor()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		log.Printf("hiveloop listening on http://%s (ai %s %s)", cfg.Addr, cfg.AIProvider, cfg.AIModel)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	db.Close()
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	// auth
	mux.HandleFunc("POST /api/login", a.handleLogin)
	mux.HandleFunc("POST /api/logout", a.handleLogout)
	mux.HandleFunc("GET /api/config", a.handleConfig)
	mux.HandleFunc("GET /api/version", a.handleVersion)
	mux.HandleFunc("GET /api/downloads", a.handleDownloadsInfo)
	mux.HandleFunc("GET /download/hiveloop.apk", a.handleAPK)
	mux.HandleFunc("GET /download/hiveloop.mobileconfig", a.handleMobileConfig)
	mux.Handle("POST /api/register", a.changes("pending", http.HandlerFunc(a.handleRegister)))
	mux.Handle("POST /api/users/{id}/approve", a.changes("pending", a.admin(a.handleApproveUser)))
	mux.Handle("PUT /api/settings/registration", a.superadmin(a.handleSetRegistration))
	mux.Handle("GET /api/settings/search", a.superadmin(a.handleGetSearchSettings))
	mux.Handle("PUT /api/settings/search", a.superadmin(a.handleSetSearchSettings))
	mux.Handle("POST /api/settings/search/test", a.superadmin(a.handleTestSearch))
	mux.Handle("GET /api/me", a.auth(a.handleMe))
	mux.Handle("PATCH /api/me", a.auth(a.handleUpdateMe))
	mux.Handle("GET /api/avatars", a.auth(a.handleAvatars))
	mux.Handle("PUT /api/settings/general", a.changes("groups", a.admin(a.handleSetGeneral)))
	mux.Handle("PUT /api/settings/general/image", a.changes("groups", a.admin(a.handleSetGeneralImage)))
	mux.Handle("DELETE /api/settings/general/image", a.changes("groups", a.admin(a.handleDeleteGeneralImage)))
	mux.Handle("PUT /api/settings/ai-avatar", a.admin(a.handleSetAIAvatar))

	// users
	mux.Handle("GET /api/users", a.auth(a.handleListUsers))
	mux.Handle("POST /api/users", a.changes("users", a.admin(a.handleCreateUser)))
	mux.Handle("PATCH /api/users/{id}", a.changes("users", a.admin(a.handleUpdateUser)))
	mux.Handle("DELETE /api/users/{id}", a.changes("pending", a.admin(a.handleDeleteUser)))

	// groups
	mux.Handle("GET /api/groups", a.auth(a.handleListGroups))
	mux.Handle("POST /api/groups", a.changes("groups", a.admin(a.handleCreateGroup)))
	mux.Handle("PATCH /api/groups/{id}", a.changes("groups", a.groupManager(a.handleUpdateGroup)))
	mux.Handle("GET /api/groups/{id}/image", a.auth(a.handleGroupImage))
	mux.Handle("PUT /api/groups/{id}/image", a.changes("groups", a.groupManager(a.handleSetGroupImage)))
	mux.Handle("DELETE /api/groups/{id}/image", a.changes("groups", a.groupManager(a.handleDeleteGroupImage)))
	mux.Handle("DELETE /api/groups/{id}", a.changes("groups", a.admin(a.handleDeleteGroup)))
	mux.Handle("POST /api/groups/{id}/join", a.changes("groups", a.auth(a.handleJoinGroup)))
	mux.Handle("POST /api/groups/{id}/leave", a.changes("groups", a.auth(a.handleLeaveGroup)))
	mux.Handle("GET /api/groups/{id}/members", a.auth(a.handleGroupMembers))
	mux.Handle("POST /api/groups/{id}/members", a.changes("groups", a.groupManager(a.handleAddMember)))
	mux.Handle("DELETE /api/groups/{id}/members/{uid}", a.changes("groups", a.groupManager(a.handleRemoveMember)))
	mux.Handle("GET /api/groups/{id}/votes", a.auth(a.handleVotes))
	mux.Handle("POST /api/groups/{id}/vote", a.auth(a.handleVote))
	mux.Handle("PUT /api/groups/{id}/leader", a.admin(a.handleSetLeader))
	mux.Handle("DELETE /api/groups/{id}/leader", a.auth(a.handleResignLeader))

	// events
	mux.Handle("GET /api/events", a.auth(a.handleListEvents))
	mux.Handle("POST /api/events", a.auth(a.handleCreateEvent))
	mux.Handle("PATCH /api/events/{id}", a.auth(a.handleUpdateEvent))
	mux.Handle("DELETE /api/events/{id}", a.auth(a.handleDeleteEvent))
	mux.Handle("PUT /api/events/{id}/signup", a.auth(a.handleSignup))
	mux.Handle("DELETE /api/events/{id}/signup", a.auth(a.handleCancelSignup))
	mux.Handle("DELETE /api/events/{id}/signups/{uid}", a.auth(a.handleCancelSignup))
	mux.Handle("GET /api/events/{id}/link", a.auth(a.handleGetEventLink))
	mux.Handle("POST /api/events/{id}/link", a.auth(a.handleGetEventLink))
	mux.Handle("DELETE /api/events/{id}/link", a.auth(a.handleDeleteEventLink))
	mux.Handle("GET /api/day-link", a.auth(a.handleDayLink))
	mux.Handle("POST /api/day-link", a.auth(a.handleDayLink))
	mux.Handle("DELETE /api/day-link", a.auth(a.handleDayLink))

	// public, read-only pages for shared events and days (no account needed)
	mux.HandleFunc("GET /e/{token}", a.handlePublicEvent)
	mux.HandleFunc("GET /e/{token}/ics", a.handlePublicEventICS)
	mux.HandleFunc("GET /d/{token}", a.handlePublicDay)
	mux.HandleFunc("GET /d/{token}/ics", a.handlePublicDayICS)

	// documents
	mux.Handle("GET /api/docs", a.auth(a.handleListDocs))
	mux.Handle("POST /api/docs", a.auth(a.handleUploadDoc))
	mux.Handle("GET /api/docs/{id}", a.auth(a.handleGetDoc))
	mux.Handle("GET /api/docs/{id}/text", a.auth(a.handleDocText))
	mux.Handle("DELETE /api/docs/{id}", a.changes("docs", a.auth(a.handleDeleteDoc)))
	mux.Handle("PUT /api/docs/{id}/shares", a.auth(a.handleShareDoc))
	mux.Handle("DELETE /api/docs/{id}/shares/{gid}", a.auth(a.handleUnshareDoc))
	mux.Handle("PUT /api/docs/{id}/content", a.changes("docs", a.auth(a.handleSaveDocContent)))
	mux.Handle("POST /api/docs/new", a.auth(a.handleNewDoc))
	mux.Handle("GET /api/docs/{id}/versions", a.auth(a.handleDocVersions))
	mux.Handle("GET /api/docs/{id}/versions/{v}", a.auth(a.handleDocVersion))
	mux.Handle("POST /api/docs/{id}/versions/{v}/restore", a.auth(a.handleRestoreDocVersion))

	// organisational chats (room 0 = general, otherwise group id)
	mux.Handle("GET /api/rooms", a.auth(a.handleListRooms))
	mux.Handle("GET /api/rooms/{id}/messages", a.auth(a.handleRoomMessages))
	mux.Handle("POST /api/rooms/{id}/messages", a.auth(a.handlePostRoomMessage))
	mux.Handle("GET /api/rooms/{id}/members", a.auth(a.handleRoomMembers))
	mux.Handle("POST /api/rooms/{id}/read", a.auth(a.handleRoomRead))
	mux.Handle("DELETE /api/room-messages/{id}", a.auth(a.handleDeleteRoomMessage))
	mux.Handle("GET /api/stream", a.auth(a.handleStream))

	// AI chat
	mux.Handle("GET /api/chats", a.aiOn(a.auth(a.handleListChats)))
	mux.Handle("POST /api/chats", a.aiOn(a.auth(a.handleCreateChat)))
	mux.Handle("GET /api/chats/{id}", a.aiOn(a.auth(a.handleGetChat)))
	mux.Handle("DELETE /api/chats/{id}", a.aiOn(a.auth(a.handleDeleteChat)))
	mux.Handle("POST /api/chats/{id}/messages", a.aiOn(a.auth(a.handleChatMessage)))
	mux.Handle("PUT /api/settings/ai", a.superadmin(a.handleSetAI))
	mux.Handle("PUT /api/settings/ai/schedule", a.superadmin(a.handleSetAISchedule))
	mux.Handle("GET /api/settings/ai-config", a.superadmin(a.handleGetAIConf))
	mux.Handle("PUT /api/settings/ai-config", a.superadmin(a.handleSetAIConf))
	mux.Handle("POST /api/settings/ai-config/test", a.superadmin(a.handleTestAIConf))
	mux.Handle("POST /api/settings/ai-config/models", a.superadmin(a.handleAIModels))
	mux.Handle("GET /api/ai/quota", a.auth(a.handleAIQuota))
	mux.Handle("GET /api/settings/ai-groups", a.superadmin(a.handleGetGroupLimits))
	mux.Handle("PUT /api/settings/ai-groups", a.superadmin(a.handleSetGroupLimits))
	mux.Handle("GET /api/settings/limits", a.superadmin(a.handleGetLimits))
	mux.Handle("PUT /api/settings/limits", a.superadmin(a.handleSetLimits))

	// static SPA
	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	// Testing phase: no caching at all, every load gets the latest files.
	serve := func(w http.ResponseWriter, r *http.Request, path string) {
		if path == "index.html" {
			http.ServeFileFS(w, r, sub, path)
			return
		}
		files.ServeHTTP(w, r)
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p != "/" && p != "/index.html" {
			if _, err := fs.Stat(sub, p[1:]); err == nil {
				serve(w, r, p[1:])
				return
			}
		}
		serve(w, r, "index.html")
	})

	return a.secure(mux)
}

// secure adds security headers and CSRF protection for mutating API requests.
func (a *App) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store, max-age=0")
		if a.cfg.SecureCookie { // served over HTTPS: tell browsers never to fall back to HTTP
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self'; script-src 'self'; connect-src 'self'; frame-src 'self' blob:; object-src 'self' blob:")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-Hiveloop") != "1" {
			jsonErr(w, http.StatusForbidden, "csrf")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) janitor() {
	t := time.NewTicker(time.Hour)
	for range t.C {
		a.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
		// sign-up requests nobody approved in 14 days are dropped
		a.db.Exec(`DELETE FROM users WHERE status='pending' AND created_at < ?`, time.Now().Add(-14*24*time.Hour).Unix())
		for _, l := range []*rateLimiter{a.rl, a.rlUser, a.rlReg, a.rlAI, a.rlMsg} {
			l.cleanup()
		}
	}
}
