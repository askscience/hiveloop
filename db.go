package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  name TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin','superadmin')),
  lang TEXT NOT NULL DEFAULT 'en',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS groups (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE COLLATE NOCASE,
  description TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '#6366f1',
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (group_id, user_id)
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  start_at INTEGER NOT NULL,
  end_at INTEGER NOT NULL,
  all_day INTEGER NOT NULL DEFAULT 0,
  group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
  via_ai INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS events_range ON events(start_at, end_at);
CREATE TABLE IF NOT EXISTS event_signups (
  event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  start_at INTEGER NOT NULL,
  end_at INTEGER NOT NULL,
  all_day INTEGER NOT NULL DEFAULT 1,
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  PRIMARY KEY (event_id, user_id)
);
CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  mime TEXT NOT NULL DEFAULT 'application/octet-stream',
  size INTEGER NOT NULL,
  path TEXT NOT NULL,
  via_ai INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS documents_user ON documents(user_id);
CREATE TABLE IF NOT EXISTS doc_shares (
  doc_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  group_id INTEGER NOT NULL, -- 0 = everyone
  created_at INTEGER NOT NULL,
  PRIMARY KEY (doc_id, group_id)
);
CREATE TABLE IF NOT EXISTS room_messages (
  id INTEGER PRIMARY KEY,
  room_id INTEGER NOT NULL, -- group id, 0 = general
  user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
  content TEXT NOT NULL,
  doc_id INTEGER REFERENCES documents(id) ON DELETE SET NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS room_messages_room ON room_messages(room_id, id);
CREATE TABLE IF NOT EXISTS mentions (
  message_id INTEGER NOT NULL REFERENCES room_messages(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (message_id, user_id)
);
CREATE INDEX IF NOT EXISTS mentions_user ON mentions(user_id);
CREATE TABLE IF NOT EXISTS room_reads (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  room_id INTEGER NOT NULL,
  last_id INTEGER NOT NULL,
  PRIMARY KEY (user_id, room_id)
);
CREATE TABLE IF NOT EXISTS conversations (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY,
  conversation_id INTEGER NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  meta TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_conv ON messages(conversation_id);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

func OpenDB(path string) (*DB, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	// additive migrations (errors = column already exists)
	for _, m := range []string{
		`ALTER TABLE room_messages ADD COLUMN is_ai INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE room_messages ADD COLUMN meta TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`,
		`ALTER TABLE documents ADD COLUMN version INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE groups ADD COLUMN leader_id INTEGER REFERENCES users(id) ON DELETE SET NULL`,
		`CREATE TABLE IF NOT EXISTS group_votes (group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, voter_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, candidate_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, created_at INTEGER NOT NULL, PRIMARY KEY (group_id, voter_id))`,
		`ALTER TABLE documents ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE documents ADD COLUMN updated_by INTEGER`,
		publicLinksSchema,
		`ALTER TABLE documents ADD COLUMN updated_via_ai INTEGER NOT NULL DEFAULT 0`,
		docVersionsSchema,
		`ALTER TABLE room_messages ADD COLUMN reply_to INTEGER`,
		`ALTER TABLE users ADD COLUMN prefs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN avatar TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE groups ADD COLUMN image_v INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN quota_mb INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN ai_daily INTEGER NOT NULL DEFAULT -1`,
		limitsSchema,
		`ALTER TABLE groups ADD COLUMN ai_daily INTEGER NOT NULL DEFAULT -1`,
		groupLimitsSchema,
		`CREATE UNIQUE INDEX IF NOT EXISTS public_links_event ON public_links(event_id) WHERE kind='event'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS public_links_day ON public_links(day, scope) WHERE kind='day'`,
	} {
		db.Exec(m)
	}
	return &DB{db}, nil
}

func (d *DB) Setting(key string) string {
	var v string
	_ = d.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v
}

func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// bootstrap creates the superadmin on first run.
func (a *App) bootstrap() error {
	var n int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role='superadmin'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	user := env("HIVELOOP_ADMIN_USER", "superadmin")
	pass := os.Getenv("HIVELOOP_ADMIN_PASS")
	generated := pass == ""
	if generated {
		pass = randomToken(12)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`INSERT INTO users(username,name,password_hash,role,created_at) VALUES(?,?,?,?,?)`,
		user, "Super Admin", string(hash), "superadmin", time.Now().Unix())
	if err != nil {
		return err
	}
	if generated {
		f := filepath.Join(a.cfg.DataDir, "initial-credentials.txt")
		_ = os.WriteFile(f, []byte(fmt.Sprintf("username: %s\npassword: %s\n", user, pass)), 0o600)
		log.Printf("superadmin created: %s (password saved in %s — change it after first login)", user, f)
	}
	return nil
}
