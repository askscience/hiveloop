package main

// Notifications that reach people when the app is closed:
//   - Web Push (browsers, installed web app, iPhone with the app on the Home Screen),
//     implemented with the standard library: RFC 8291 encryption + RFC 8292 VAPID;
//   - a short list per user that the Android app checks periodically.
// Sent for: volunteer reminders, direct messages, @mentions and replies.

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pushSchema = `CREATE TABLE IF NOT EXISTS push_subs (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint TEXT NOT NULL UNIQUE,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS push_subs_user ON push_subs(user_id);
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  body TEXT NOT NULL,
  url TEXT NOT NULL DEFAULT '',
  tag TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS notifications_user ON notifications(user_id, id)`

// Notice is one notification for one person.
type Notice struct {
	ID    int64  `json:"id,omitempty"`
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag,omitempty"`
}

var b64 = base64.RawURLEncoding

// Push services browsers use. Subscriptions elsewhere are refused, so the server
// never posts to an address a user made up.
var pushHosts = []string{"fcm.googleapis.com", "android.googleapis.com", "push.services.mozilla.com", "push.apple.com", "notify.windows.com"}

func pushHostOK(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, ok := range pushHosts {
		if h == ok || strings.HasSuffix(h, "."+ok) {
			return true
		}
	}
	return false
}

// ---------- VAPID keys (generated once, kept in the settings table) ----------

var vapidMu sync.Mutex

func (a *App) vapidKey() (*ecdsa.PrivateKey, string, error) {
	vapidMu.Lock()
	defer vapidMu.Unlock()
	if raw, err := b64.DecodeString(a.db.Setting("vapid_private")); err == nil && len(raw) == 32 {
		k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
		if err == nil {
			pub, err := k.PublicKey.Bytes()
			if err == nil {
				return k, b64.EncodeToString(pub), nil
			}
		}
	}
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}
	raw, err := k.Bytes()
	if err != nil {
		return nil, "", err
	}
	if err := a.db.SetSetting("vapid_private", b64.EncodeToString(raw)); err != nil {
		return nil, "", err
	}
	pub, err := k.PublicKey.Bytes()
	if err != nil {
		return nil, "", err
	}
	return k, b64.EncodeToString(pub), nil
}

// vapidJWT signs the ES256 token for one push service origin.
func vapidJWT(k *ecdsa.PrivateKey, aud, sub string, exp time.Time) (string, error) {
	head := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": exp.Unix(), "sub": sub})
	signing := head + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, k, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + b64.EncodeToString(sig), nil
}

// ---------- RFC 8291 message encryption (aes128gcm, one record) ----------

// encryptPush encrypts plaintext for a subscription. asKey and salt are random in
// production; tests pass fixed values to check against the RFC example.
func encryptPush(plaintext, uaPublic, authSecret []byte, asKey *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, fmt.Errorf("bad subscription key: %w", err)
	}
	if len(authSecret) != 16 || len(salt) != 16 {
		return nil, errors.New("bad auth secret or salt")
	}
	shared, err := asKey.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic := asKey.PublicKey().Bytes()
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.Write(salt)
	binary.Write(&out, binary.BigEndian, uint32(4096))
	out.WriteByte(byte(len(asPublic)))
	out.Write(asPublic)
	out.Write(gcm.Seal(nil, nonce, append(append([]byte{}, plaintext...), 2), nil)) // 0x02: last record, no padding
	return out.Bytes(), nil
}

// ---------- sending ----------

var pushClient = &http.Client{Timeout: 15 * time.Second}

type pushSub struct {
	ID       int64
	Endpoint string
	P256dh   string
	Auth     string
}

// pushSend delivers one notice to one subscription. gone = the subscription no longer exists.
func (a *App) pushSend(s pushSub, n Notice) (gone bool, err error) {
	if !pushHostOK(s.Endpoint) {
		return true, errors.New("push host not allowed")
	}
	k, pub, err := a.vapidKey()
	if err != nil {
		return false, err
	}
	ua, err1 := b64.DecodeString(strings.TrimRight(s.P256dh, "="))
	auth, err2 := b64.DecodeString(strings.TrimRight(s.Auth, "="))
	if err1 != nil || err2 != nil {
		return true, errors.New("bad subscription keys")
	}
	payload, _ := json.Marshal(n)
	asKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return false, err
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	body, err := encryptPush(payload, ua, auth, asKey, salt)
	if err != nil {
		return true, err
	}
	u, _ := url.Parse(s.Endpoint)
	sub := a.db.Setting("push_origin")
	if sub == "" {
		sub = "mailto:hiveloop@example.org"
	}
	jwt, err := vapidJWT(k, u.Scheme+"://"+u.Host, strings.TrimRight(sub, "/"), time.Now().Add(12*time.Hour))
	if err != nil {
		return false, err
	}
	req, _ := http.NewRequest("POST", s.Endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+pub)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "high")
	resp, err := pushClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	switch {
	case resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode == 404 || resp.StatusCode == 410:
		return true, nil
	default:
		return false, fmt.Errorf("push service: %d %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
}

// notify stores a notice for each user (for the Android app) and pushes it to their
// devices, in the background. build makes the text in each person's language.
func (a *App) notify(userIDs []int64, build func(uid int64) Notice) {
	if len(userIDs) == 0 {
		return
	}
	type item struct {
		uid int64
		n   Notice
	}
	items := make([]item, 0, len(userIDs))
	now := time.Now().Unix()
	tx, err := a.db.Begin() // one write for everyone (e.g. @tutti)
	if err != nil {
		return
	}
	for _, uid := range userIDs {
		n := build(uid)
		n.Title, n.Body = clip(n.Title, 120), clip(n.Body, 300)
		res, err := tx.Exec(`INSERT INTO notifications(user_id,title,body,url,tag,created_at) VALUES(?,?,?,?,?,?)`, uid, n.Title, n.Body, n.URL, n.Tag, now)
		if err == nil {
			n.ID, _ = res.LastInsertId()
		}
		items = append(items, item{uid, n})
	}
	if tx.Commit() != nil {
		return
	}
	go func() {
		for _, it := range items {
			for _, s := range a.pushSubs(it.uid) {
				gone, err := a.pushSend(s, it.n)
				if gone {
					a.db.Exec(`DELETE FROM push_subs WHERE id=?`, s.ID)
				} else if err != nil {
					log.Printf("push to user %d: %v", it.uid, err)
				}
			}
		}
	}()
}

func (a *App) pushSubs(uid int64) []pushSub {
	rows, err := a.db.Query(`SELECT id,endpoint,p256dh,auth FROM push_subs WHERE user_id=?`, uid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []pushSub
	for rows.Next() {
		var s pushSub
		if rows.Scan(&s.ID, &s.Endpoint, &s.P256dh, &s.Auth) == nil {
			out = append(out, s)
		}
	}
	return out
}

// pruneNotifications keeps two weeks of notices.
func (a *App) pruneNotifications() {
	a.db.Exec(`DELETE FROM notifications WHERE created_at < ?`, time.Now().Add(-14*24*time.Hour).Unix())
}

// ---------- HTTP ----------

func (a *App) handlePushKey(w http.ResponseWriter, r *http.Request) {
	_, pub, err := a.vapidKey()
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	jsonOK(w, map[string]string{"key": pub})
}

func (a *App) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var in struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := decode(r, &in); err != nil || len(in.Endpoint) > 1024 || !pushHostOK(in.Endpoint) {
		jsonErr(w, 400, "bad_request")
		return
	}
	ua, err1 := b64.DecodeString(strings.TrimRight(in.Keys.P256dh, "="))
	auth, err2 := b64.DecodeString(strings.TrimRight(in.Keys.Auth, "="))
	if err1 != nil || err2 != nil || len(ua) != 65 || len(auth) != 16 {
		jsonErr(w, 400, "bad_request")
		return
	}
	if a.db.Setting("push_origin") == "" {
		if base := a.publicURL(r); strings.HasPrefix(base, "https://") {
			a.db.SetSetting("push_origin", strings.TrimRight(base, "/"))
		}
	}
	// a device that changes account follows the new account
	_, err := a.db.Exec(`INSERT INTO push_subs(user_id,endpoint,p256dh,auth,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(endpoint) DO UPDATE SET user_id=excluded.user_id,p256dh=excluded.p256dh,auth=excluded.auth`,
		u.ID, in.Endpoint, in.Keys.P256dh, in.Keys.Auth, time.Now().Unix())
	if err != nil {
		jsonErr(w, 500, "server_error")
		return
	}
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM push_subs WHERE user_id=?`, u.ID).Scan(&n)
	if n > 10 { // keep the 10 most recent devices
		a.db.Exec(`DELETE FROM push_subs WHERE user_id=? AND id NOT IN (SELECT id FROM push_subs WHERE user_id=? ORDER BY id DESC LIMIT 10)`, u.ID, u.ID)
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (a *App) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decode(r, &in); err != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	a.db.Exec(`DELETE FROM push_subs WHERE endpoint=? AND user_id=?`, in.Endpoint, currentUser(r).ID)
	jsonOK(w, map[string]bool{"ok": true})
}

// handlePushTest sends a test notification to the caller's devices.
func (a *App) handlePushTest(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	a.notify([]int64{u.ID}, func(int64) Notice {
		return Notice{Title: "Hiveloop", Body: tr(u.Lang, "Le notifiche funzionano su questo dispositivo.", "Notifications work on this device."), URL: "/#/settings", Tag: "test"}
	})
	jsonOK(w, map[string]bool{"ok": true})
}

// handleNotifications: what the Android app checks every few minutes. Without
// `after` it only returns the latest id, so a new install does not replay old notices.
func (a *App) handleNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	var latest int64
	a.db.QueryRow(`SELECT COALESCE(MAX(id),0) FROM notifications WHERE user_id=?`, u.ID).Scan(&latest)
	items := []Notice{}
	if after > 0 {
		rows, err := a.db.Query(`SELECT id,title,body,url,tag FROM notifications WHERE user_id=? AND id>? AND created_at>? ORDER BY id LIMIT 20`,
			u.ID, after, time.Now().Add(-24*time.Hour).Unix())
		if err == nil {
			for rows.Next() {
				var n Notice
				if rows.Scan(&n.ID, &n.Title, &n.Body, &n.URL, &n.Tag) == nil {
					items = append(items, n)
				}
			}
			rows.Close()
		}
	}
	jsonOK(w, map[string]any{"latest": latest, "items": items})
}
