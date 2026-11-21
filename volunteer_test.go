package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testApp: groups 1 and 2; users 1 (admin), 2 and 3 in group 1, 4 in group 2, 5 in none.
func testApp(t *testing.T) *App {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &App{db: db, cfg: &Config{TZ: time.UTC}, rlMsg: newRateLimiter(1000, time.Minute), rlAI: newRateLimiter(1000, time.Minute)}
	now := time.Now().Unix()
	db.Exec(`INSERT INTO groups(id,name,created_at) VALUES(1,'stars',?),(2,'theatre',?)`, now, now)
	for i, role := range []string{"admin", "user", "user", "user", "user"} {
		id := i + 1
		db.Exec(`INSERT INTO users(id,username,name,password_hash,role,created_at) VALUES(?,?,?,'x',?,?)`, id, "u"+string(rune('0'+id)), "User "+string(rune('0'+id)), role, now)
	}
	db.Exec(`INSERT INTO group_members(group_id,user_id,joined_at) VALUES(1,2,?),(1,3,?),(2,4,?)`, now, now, now)
	return a
}

func user(id int64) *User {
	if id == 1 {
		return &User{ID: 1, Username: "u1", Role: "admin"}
	}
	return &User{ID: id, Username: "u" + string(rune('0'+id)), Role: "user"}
}

func ptr[T any](v T) *T { return &v }

func ids(evs []Event) map[int64]bool {
	m := map[int64]bool{}
	for _, e := range evs {
		m[e.ID] = true
	}
	return m
}

func TestPrivateEventsStayInTheGroup(t *testing.T) {
	a := testApp(t)
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)
	in := func(title string, private bool) EventInput {
		return EventInput{Title: ptr(title), Start: ptr(start.Format(time.RFC3339)), End: ptr(start.Add(2 * time.Hour).Format(time.RFC3339)), GroupID: ptr(int64(1)), Private: ptr(private)}
	}
	pub, err := a.createEvent(user(2), in("Star party", false), false)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := a.createEvent(user(2), in("Planning meeting", true), false)
	if err != nil || !priv.Private {
		t.Fatalf("private create: %v %+v", err, priv)
	}
	// a private event needs a group
	if _, err := a.createEvent(user(1), EventInput{Title: ptr("x"), Start: ptr(start.Format(time.RFC3339)), Private: ptr(true)}, false); err == nil {
		t.Fatal("private general event accepted")
	}
	from, to := start.Add(-time.Hour), start.Add(5*time.Hour)
	for _, c := range []struct {
		uid  int64
		sees bool
	}{{1, true}, {2, true}, {3, true}, {4, false}, {5, false}} {
		for _, scope := range []string{"all", "mine", "1", "booked"} {
			evs, _ := a.listEvents(user(c.uid), from, to, scope)
			if ids(evs)[priv.ID] && !c.sees {
				t.Errorf("user %d sees the private event in scope %s", c.uid, scope)
			}
		}
		evs, _ := a.listEvents(user(c.uid), from, to, "all")
		if ids(evs)[priv.ID] != c.sees || !ids(evs)[pub.ID] {
			t.Errorf("user %d: list %v", c.uid, ids(evs))
		}
		_, err := a.getEvent(user(c.uid), priv.ID)
		if (err == nil) != c.sees {
			t.Errorf("user %d: get err=%v", c.uid, err)
		}
		_, err = a.signup(user(c.uid), priv.ID, SignupInput{})
		if (err == nil) != c.sees {
			t.Errorf("user %d: signup err=%v", c.uid, err)
		}
	}
	// never public: no link, not on the public day page, not for the anonymous viewer
	if _, err := a.eventLinkToken(user(2), priv.ID, true); err == nil {
		t.Error("public link for a private event")
	}
	if _, err := a.getEvent(publicViewer, priv.ID); err == nil {
		t.Error("anonymous viewer sees the private event")
	}
	_, day, _ := a.dayEvents(start.Format("2006-01-02"), "all")
	if ids(day)[priv.ID] || !ids(day)[pub.ID] {
		t.Errorf("public day page: %v", ids(day))
	}
	// turning a shared event private removes its public link
	tok, err := a.eventLinkToken(user(2), pub.ID, true)
	if err != nil || tok == "" {
		t.Fatal(err)
	}
	if _, err := a.updateEvent(user(2), pub.ID, EventInput{Private: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if k, _, _, _, err := a.lookupLink(tok); err == nil {
		t.Errorf("link still there (%s)", k)
	}
	// moving it to "general" makes it public again (general events cannot be private)
	e, err := a.updateEvent(user(1), pub.ID, EventInput{ClearGroup: true})
	if err != nil || e.Private {
		t.Fatalf("general: %v private=%v", err, e.Private)
	}
}

func TestPrivateLiveUpdates(t *testing.T) {
	a := testApp(t)
	subs := map[int64]chan []byte{}
	liveHub.mu.Lock()
	for _, id := range []int64{1, 2, 4} {
		ch := make(chan []byte, 8)
		subs[id] = ch
		liveHub.subs[ch] = user(id)
	}
	liveHub.mu.Unlock()
	t.Cleanup(func() {
		liveHub.mu.Lock()
		for _, ch := range subs {
			delete(liveHub.subs, ch)
		}
		liveHub.mu.Unlock()
	})
	start := time.Now().Add(time.Hour)
	_, err := a.createEvent(user(2), EventInput{Title: ptr("Secret"), Start: ptr(start.Format(time.RFC3339)), GroupID: ptr(int64(1)), Private: ptr(true)}, false)
	if err != nil {
		t.Fatal(err)
	}
	for id, ch := range subs {
		got := string(<-ch)
		if leaked := strings.Contains(got, "Secret"); leaked == (id == 4) {
			t.Errorf("user %d got %s", id, got)
		}
	}
}

func TestShiftsAndMissing(t *testing.T) {
	a := testApp(t)
	start := time.Now().UTC().Add(30 * time.Hour).Truncate(time.Hour)
	f := func(d time.Duration) string { return start.Add(d).Format(time.RFC3339) }
	e, err := a.createEvent(user(2), EventInput{Title: ptr("Stall"), Start: ptr(f(0)), End: ptr(f(8 * time.Hour)), GroupID: ptr(int64(1)),
		Shifts: &[]ShiftInput{{Start: f(0), End: f(3 * time.Hour), Needed: 2}, {Start: f(5 * time.Hour), End: f(9 * time.Hour), Needed: 2, Label: "till"}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Shifts) != 2 || e.Missing != 4 || e.Shifts[1].End != f(8*time.Hour) {
		t.Fatalf("shifts %+v missing %d", e.Shifts, e.Missing)
	}
	// anyone who sees the event can volunteer, also from another group
	e, err = a.joinShift(user(4), e.ID, e.Shifts[0].ID)
	if err != nil || e.Missing != 3 || len(e.Shifts[0].People) != 1 {
		t.Fatalf("join: %v missing %d", err, e.Missing)
	}
	if evs, _ := a.listEvents(user(4), start.Add(-time.Hour), start.Add(10*time.Hour), "booked"); !ids(evs)[e.ID] {
		t.Error("shift volunteer does not find the event in 'my bookings'")
	}
	if _, err := a.leaveShift(user(5), e.ID, e.Shifts[0].ID, 4); err == nil {
		t.Error("someone else removed a volunteer")
	}
	if _, err := a.leaveShift(user(2), e.ID, e.Shifts[0].ID, 4); err != nil { // the group can
		t.Error(err)
	}
	// editing keeps volunteers of shifts that keep their id
	a.joinShift(user(3), e.ID, e.Shifts[1].ID)
	keep := e.Shifts[1].ID
	e, err = a.updateEvent(user(2), e.ID, EventInput{Shifts: &[]ShiftInput{{ID: &keep, Start: f(5 * time.Hour), End: f(8 * time.Hour), Needed: 3}}})
	if err != nil || len(e.Shifts) != 1 || len(e.Shifts[0].People) != 1 || e.Missing != 2 {
		t.Fatalf("update: %v %+v", err, e.Shifts)
	}
	// moving the event moves its shifts
	e, _ = a.updateEvent(user(2), e.ID, EventInput{Start: ptr(f(24 * time.Hour))})
	if e.Shifts[0].Start != f(29*time.Hour) {
		t.Errorf("moved shift starts %s", e.Shifts[0].Start)
	}
	if _, err := a.updateEvent(user(2), e.ID, EventInput{Shifts: &[]ShiftInput{{Start: f(0), End: f(1), Needed: 0}}}); err == nil {
		t.Error("shift without volunteers accepted")
	}
}

func TestReminderStages(t *testing.T) {
	for left, want := range map[time.Duration]int{80 * time.Hour: 0, 71 * time.Hour: 72, 23 * time.Hour: 24, 2 * time.Hour: 3} {
		if got := reminderStage(left); got != want {
			t.Errorf("%v: stage %d, want %d", left, got, want)
		}
	}
	a := testApp(t)
	now := time.Now()
	start := now.Add(20 * time.Hour).Truncate(time.Minute)
	e, _ := a.createEvent(user(2), EventInput{Title: ptr("Visitor desk"), Start: ptr(start.Format(time.RFC3339)), End: ptr(start.Add(3 * time.Hour).Format(time.RFC3339)), GroupID: ptr(int64(1)),
		Shifts: &[]ShiftInput{{Start: start.Format(time.RFC3339), End: start.Add(3 * time.Hour).Format(time.RFC3339), Needed: 2}}}, false)
	a.joinShift(user(3), e.ID, e.Shifts[0].ID)
	count := func() (msgs, notes int) {
		a.db.QueryRow(`SELECT COUNT(*) FROM room_messages WHERE room_id=1 AND meta LIKE '%volunteers%'`).Scan(&msgs)
		a.db.QueryRow(`SELECT COUNT(*) FROM notifications`).Scan(&notes)
		return
	}
	a.checkReminders(now)
	a.checkReminders(now.Add(time.Minute)) // same stage: once
	msgs, notes := count()
	if msgs != 1 || notes != 1 { // only user 2: user 3 already volunteered
		t.Fatalf("after the 24h stage: %d messages, %d notifications", msgs, notes)
	}
	var to int64
	a.db.QueryRow(`SELECT user_id FROM notifications`).Scan(&to)
	if to != 2 {
		t.Errorf("notified user %d", to)
	}
	a.checkReminders(start.Add(-2 * time.Hour)) // 3h stage
	if msgs, _ := count(); msgs != 2 {
		t.Errorf("3h stage: %d messages", msgs)
	}
	// filled: no more reminders
	a.joinShift(user(2), e.ID, e.Shifts[0].ID)
	a.db.Exec(`DELETE FROM event_reminders`)
	a.checkReminders(start.Add(-time.Hour))
	if msgs, _ := count(); msgs != 2 {
		t.Errorf("full shift reminded anyway: %d", msgs)
	}
}

// call runs a handler as user u.
func call(h http.HandlerFunc, u *User, method, path string, body any, pathVals map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	for k, v := range pathVals {
		r.SetPathValue(k, v)
	}
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestDirectMessagesArePrivate(t *testing.T) {
	a := testApp(t)
	w := call(a.handleOpenDM, user(2), "POST", "/api/dm", map[string]int64{"user_id": 4}, nil)
	var out struct {
		RoomID int64 `json:"room_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.RoomID >= 0 {
		t.Fatalf("open: %d %s", w.Code, w.Body)
	}
	room := out.RoomID
	// same pair, other direction: same conversation
	w = call(a.handleOpenDM, user(4), "POST", "/api/dm", map[string]int64{"user_id": 2}, nil)
	var again struct {
		RoomID int64 `json:"room_id"`
	}
	json.Unmarshal(w.Body.Bytes(), &again)
	if again.RoomID != room {
		t.Fatalf("two conversations for one pair: %d %d", room, again.RoomID)
	}
	rs := map[string]string{"id": itoa(room)}
	if w := call(a.handlePostRoomMessage, user(2), "POST", "/", map[string]string{"content": "hi, a question about the stall?"}, rs); w.Code != 200 {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	for _, c := range []struct {
		uid int64
		ok  bool
	}{{2, true}, {4, true}, {3, false}, {1, false}} { // admins do not read direct messages
		if w := call(a.handleRoomMessages, user(c.uid), "GET", "/", nil, rs); (w.Code == 200) != c.ok {
			t.Errorf("user %d reads: %d", c.uid, w.Code)
		}
		if w := call(a.handlePostRoomMessage, user(c.uid), "POST", "/", map[string]string{"content": "x"}, rs); (w.Code == 200) != c.ok {
			t.Errorf("user %d writes: %d", c.uid, w.Code)
		}
	}
	var mid int64
	a.db.QueryRow(`SELECT id FROM room_messages WHERE room_id=? ORDER BY id LIMIT 1`, room).Scan(&mid)
	if w := call(a.handleDeleteRoomMessage, user(1), "DELETE", "/", nil, map[string]string{"id": itoa(mid)}); w.Code != 403 {
		t.Errorf("admin deleted a direct message: %d", w.Code)
	}
	// the other person is notified; documents are not attached here
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id=4`).Scan(&n)
	if n == 0 {
		t.Error("no notification for the recipient")
	}
	if w := call(a.handlePostRoomMessage, user(2), "POST", "/", map[string]any{"content": "doc", "doc_id": 1}, rs); w.Code != 400 {
		t.Errorf("document in a direct message: %d", w.Code)
	}
	// the conversation shows in both lists, not in a third person's
	for _, c := range []struct {
		uid  int64
		sees bool
	}{{2, true}, {4, true}, {3, false}} {
		w := call(a.handleListRooms, user(c.uid), "GET", "/", nil, nil)
		if strings.Contains(w.Body.String(), `"id":`+itoa(room)+`,`) != c.sees {
			t.Errorf("user %d room list: %s", c.uid, w.Body)
		}
	}
	// a deleted account takes its conversations with it
	a.db.Exec(`DELETE FROM users WHERE id=4`)
	a.db.QueryRow(`SELECT COUNT(*) FROM room_messages WHERE room_id=?`, room).Scan(&n)
	if n != 0 {
		t.Errorf("%d messages left", n)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

// RFC 8291, Appendix A.
func TestPushEncryptionRFCExample(t *testing.T) {
	d := func(s string) []byte {
		b, err := b64.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	asPriv, err := ecdh.P256().NewPrivateKey(d("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	uaPub := d("BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	auth := d("BTBZMqHH6r4Tts7J_aSIgg")
	salt := d("DGv6ra1nlYgDCS1FRnbzlw")
	plain := []byte("When I grow up, I want to be a watermelon")
	got, err := encryptPush(plain, uaPub, auth, asPriv, salt)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(got) != want {
		t.Errorf("ciphertext differs from the RFC example\n got %s\nwant %s", b64.EncodeToString(got), want)
	}
	// and the receiver can decrypt it
	uaPriv, _ := ecdh.P256().NewPrivateKey(d("q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"))
	keyID := got[21:86]
	asPub, _ := ecdh.P256().NewPublicKey(keyID)
	shared, _ := uaPriv.ECDH(asPub)
	ikm, _ := hkdf.Key(sha256.New, shared, auth, "WebPush: info\x00"+string(uaPub)+string(keyID), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, got[:16], "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, got[:16], "Content-Encoding: nonce\x00", 12)
	blk, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(blk)
	pt, err := gcm.Open(nil, nonce, got[86:], nil)
	if err != nil || !bytes.Equal(pt, append(plain, 2)) {
		t.Errorf("decrypt: %v %q", err, pt)
	}
}

func TestVAPIDAndPushHosts(t *testing.T) {
	a := testApp(t)
	k, pub, err := a.vapidKey()
	if err != nil || len(pub) != 87 {
		t.Fatalf("key: %v %q", err, pub)
	}
	if _, pub2, _ := a.vapidKey(); pub2 != pub {
		t.Error("key not kept")
	}
	jwt, err := vapidJWT(k, "https://fcm.googleapis.com", "https://example.org", time.Now().Add(time.Hour))
	if err != nil || strings.Count(jwt, ".") != 2 {
		t.Fatalf("jwt: %v", err)
	}
	for ep, ok := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":            true,
		"https://updates.push.services.mozilla.com/wpush/v2": true,
		"https://web.push.apple.com/QGx":                     true,
		"https://wns2-par02p.notify.windows.com/w/?token=x":  true,
		"http://fcm.googleapis.com/fcm/send/abc":             false,
		"https://evil.example/fcm.googleapis.com":            false,
		"https://fcm.googleapis.com.evil.example/x":          false,
		"https://127.0.0.1/x":                                false,
		"https://fcm.googleapis.com:8443/x":                  false,
	} {
		if pushHostOK(ep) != ok {
			t.Errorf("%s: want %v", ep, ok)
		}
	}
}
