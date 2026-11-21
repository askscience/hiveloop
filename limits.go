package main

// Super admin controls: document creation and file uploads on/off, storage space per user
// (a default plus per-person overrides) and a daily limit of assistant messages per user.

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

const limitsSchema = `CREATE TABLE IF NOT EXISTS ai_usage (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  day TEXT NOT NULL,
  n INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, day)
)`

func (a *App) docCreateOn() bool { return a.db.Setting("docs_create_off") != "1" }
func (a *App) docUploadOn() bool { return a.db.Setting("docs_upload_off") != "1" }

// defaultQuotaMB: storage per user in MB (setting, else the built-in value).
func (a *App) defaultQuotaMB() int {
	if n, err := strconv.Atoi(a.db.Setting("quota_mb")); err == nil && n > 0 {
		return n
	}
	return int(a.cfg.UserQuota >> 20)
}

// defaultAIDaily: assistant messages per user per day; 0 = no daily limit.
func (a *App) defaultAIDaily() int {
	n, _ := strconv.Atoi(a.db.Setting("ai_daily"))
	return max(n, 0)
}

// userQuota in bytes: the person's override (MB) or the default.
func (a *App) userQuota(uid int64) int64 {
	var mb int
	a.db.QueryRow(`SELECT quota_mb FROM users WHERE id=?`, uid).Scan(&mb)
	if mb <= 0 {
		mb = a.defaultQuotaMB()
	}
	return int64(mb) << 20
}

// defaultAITotal: assistant messages per day for everyone together; 0 = no limit.
func (a *App) defaultAITotal() int {
	n, _ := strconv.Atoi(a.db.Setting("ai_daily_total"))
	return max(n, 0)
}

func (a *App) aiTotalToday() int {
	var n int
	a.db.QueryRow(`SELECT COALESCE(SUM(n),0) FROM ai_usage WHERE day=?`, today()).Scan(&n)
	return n
}

// aiLimit: the person's daily limit (-1 in the table = use the default; 0 = no assistant).
// Returns -1 for "unlimited".
func (a *App) aiLimit(uid int64) int {
	o := -1
	a.db.QueryRow(`SELECT ai_daily FROM users WHERE id=?`, uid).Scan(&o)
	if o >= 0 {
		return o
	}
	if d := a.defaultAIDaily(); d > 0 {
		return d
	}
	return -1
}

// appTZ is Hiveloop's time zone (HIVELOOP_TZ, UTC by default): days start at local midnight
// whatever the server clock's zone.
var appTZ = time.Local

func today() string { return time.Now().In(appTZ).Format("2006-01-02") }

func (a *App) aiUsedToday(uid int64) int {
	var n int
	a.db.QueryRow(`SELECT n FROM ai_usage WHERE user_id=? AND day=?`, uid, today()).Scan(&n)
	return n
}

// aiQuota: what is left today for a person, in a room (0 = general chat or the assistant
// page), taking the tightest of the person's, the group's and everyone's limits.
type aiQuota struct {
	Used  int    `json:"used"`  // the person's messages today
	Left  int    `json:"left"`  // -1 = no limit
	Limit int    `json:"limit"` // the limit that applies (-1 = none)
	By    string `json:"by"`    // which one: "user", "group", "total"
}

func (a *App) aiQuota(uid, room int64) aiQuota {
	q := aiQuota{Used: a.aiUsedToday(uid), Left: -1, Limit: -1}
	try := func(limit, used int, by string) {
		if limit < 0 {
			return
		}
		if left := max(limit-used, 0); q.Left < 0 || left < q.Left {
			q.Left, q.Limit, q.By = left, limit, by
		}
	}
	try(a.aiLimit(uid), q.Used, "user")
	if room > 0 {
		try(a.groupLimit(room), a.groupUsedToday(room), "group")
	}
	if tot := a.defaultAITotal(); tot > 0 {
		try(tot, a.aiTotalToday(), "total")
	}
	return q
}

// useAI counts one assistant message (person, and group if room > 0). It returns "" when
// allowed, otherwise the reason: the person's, the group's or everyone's daily limit.
func (a *App) useAI(uid, room int64) string {
	if q := a.aiQuota(uid, room); q.Left == 0 {
		return map[string]string{"user": "ai_daily_limit", "group": "ai_group_limit", "total": "ai_daily_total"}[q.By]
	}
	a.db.Exec(`INSERT INTO ai_usage(user_id,day,n) VALUES(?,?,1) ON CONFLICT(user_id,day) DO UPDATE SET n=n+1`, uid, today())
	if room > 0 {
		a.db.Exec(`INSERT INTO ai_group_usage(group_id,day,n) VALUES(?,?,1) ON CONFLICT(group_id,day) DO UPDATE SET n=n+1`, room, today())
	}
	old := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	a.db.Exec(`DELETE FROM ai_usage WHERE day < ?`, old)
	a.db.Exec(`DELETE FROM ai_group_usage WHERE day < ?`, old)
	return ""
}

func (a *App) handleAIQuota(w http.ResponseWriter, r *http.Request) {
	room, _ := strconv.ParseInt(r.URL.Query().Get("room"), 10, 64)
	jsonOK(w, a.aiQuota(currentUser(r).ID, room))
}

// ---------- super admin settings ----------

type limitsView struct {
	DocCreate    bool `json:"doc_create"`
	DocUpload    bool `json:"doc_upload"`
	QuotaMB      int  `json:"quota_mb"`
	AIDaily      int  `json:"ai_daily"`
	AITotal      int  `json:"ai_total"`
	AITotalToday int  `json:"ai_total_today"`
}

func (a *App) limits() limitsView {
	return limitsView{a.docCreateOn(), a.docUploadOn(), a.defaultQuotaMB(), a.defaultAIDaily(), a.defaultAITotal(), a.aiTotalToday()}
}

func (a *App) handleGetLimits(w http.ResponseWriter, r *http.Request) { jsonOK(w, a.limits()) }

func (a *App) handleSetLimits(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DocCreate *bool `json:"doc_create"`
		DocUpload *bool `json:"doc_upload"`
		QuotaMB   *int  `json:"quota_mb"`
		AIDaily   *int  `json:"ai_daily"`
		AITotal   *int  `json:"ai_total"`
	}
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	off := func(b bool) string {
		if b {
			return ""
		}
		return "1"
	}
	if in.DocCreate != nil {
		a.db.SetSetting("docs_create_off", off(*in.DocCreate))
	}
	if in.DocUpload != nil {
		a.db.SetSetting("docs_upload_off", off(*in.DocUpload))
	}
	if in.QuotaMB != nil {
		if *in.QuotaMB < 1 || *in.QuotaMB > 100000 {
			jsonErr(w, 400, "bad_request")
			return
		}
		a.db.SetSetting("quota_mb", strconv.Itoa(*in.QuotaMB))
	}
	if in.AIDaily != nil {
		if *in.AIDaily < 0 || *in.AIDaily > 10000 {
			jsonErr(w, 400, "bad_request")
			return
		}
		a.db.SetSetting("ai_daily", strconv.Itoa(*in.AIDaily))
	}
	if in.AITotal != nil {
		if *in.AITotal < 0 || *in.AITotal > 1000000 {
			jsonErr(w, 400, "bad_request")
			return
		}
		a.db.SetSetting("ai_daily_total", strconv.Itoa(*in.AITotal))
	}
	a.liveAll("settings")
	jsonOK(w, a.limits())
}

// userUsage adds space used, overrides and today's assistant messages to the users list
// (super admin only). Runs after the users rows are closed: one query per figure.
func (a *App) userUsage(us []User) {
	used := map[int64]int64{}
	if rows, err := a.db.Query(`SELECT user_id, COALESCE(SUM(size),0) FROM documents GROUP BY user_id`); err == nil {
		for rows.Next() {
			var id, n int64
			rows.Scan(&id, &n)
			used[id] = n
		}
		rows.Close()
	}
	ai := map[int64]int{}
	if rows, err := a.db.Query(`SELECT user_id, n FROM ai_usage WHERE day=?`, today()); err == nil {
		for rows.Next() {
			var id int64
			var n int
			rows.Scan(&id, &n)
			ai[id] = n
		}
		rows.Close()
	}
	over := map[int64][2]int{}
	if rows, err := a.db.Query(`SELECT id, quota_mb, ai_daily FROM users`); err == nil {
		for rows.Next() {
			var id int64
			var q, d sql.NullInt64
			rows.Scan(&id, &q, &d)
			over[id] = [2]int{int(q.Int64), int(d.Int64)}
		}
		rows.Close()
	}
	defQ, defA := a.defaultQuotaMB(), a.defaultAIDaily()
	for i := range us {
		o := over[us[i].ID]
		q, d := o[0], o[1]
		us[i].Usage = &userUsage{Used: used[us[i].ID], QuotaMB: q, AIDaily: d, AIToday: ai[us[i].ID], DefQuotaMB: defQ, DefAIDaily: defA}
	}
}

type userUsage struct {
	Used       int64 `json:"used"`
	QuotaMB    int   `json:"quota_mb"` // 0 = default
	AIDaily    int   `json:"ai_daily"` // -1 = default, 0 = no assistant
	AIToday    int   `json:"ai_today"`
	DefQuotaMB int   `json:"def_quota_mb"`
	DefAIDaily int   `json:"def_ai_daily"`
}
