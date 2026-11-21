package main

// Daily limits of assistant messages per group chat (super admin). Modes:
//   - "" (off): no group limits;
//   - "auto": the global daily limit is shared among groups in proportion to their members,
//     so the groups together never exceed it;
//   - "manual": the super admin sets each group's number (the app suggests consistent values).

import (
	"net/http"
	"strconv"
)

const groupLimitsSchema = `CREATE TABLE IF NOT EXISTS ai_group_usage (
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  day TEXT NOT NULL,
  n INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (group_id, day)
)`

func (a *App) aiGroupMode() string {
	switch m := a.db.Setting("ai_group_mode"); m {
	case "auto", "manual":
		return m
	}
	return ""
}

type groupLimitRow struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	Members int    `json:"members"`
	Manual  int    `json:"manual"` // -1 = no limit (manual mode)
	Auto    int    `json:"auto"`   // proportional share of the global limit (-1 = no global limit)
	Used    int    `json:"used"`   // today
}

// groupLimitRows reads every group with members, manual value, automatic share and use today.
func (a *App) groupLimitRows() []groupLimitRow {
	rows, err := a.db.Query(`SELECT g.id, g.name, g.color, g.ai_daily, (SELECT COUNT(*) FROM group_members m WHERE m.group_id=g.id),
		COALESCE((SELECT n FROM ai_group_usage u WHERE u.group_id=g.id AND u.day=?),0) FROM groups g ORDER BY g.name COLLATE NOCASE`, today())
	if err != nil {
		return nil
	}
	var out []groupLimitRow
	sum := 0
	for rows.Next() {
		var g groupLimitRow
		rows.Scan(&g.ID, &g.Name, &g.Color, &g.Manual, &g.Members, &g.Used)
		out = append(out, g)
		sum += g.Members
	}
	rows.Close()
	tot := a.defaultAITotal()
	for i := range out {
		out[i].Auto = -1
		if tot > 0 && sum > 0 {
			out[i].Auto = max(1, tot*out[i].Members/sum)
		}
	}
	return out
}

// groupLimit: today's limit for a group chat; -1 = none.
func (a *App) groupLimit(gid int64) int {
	switch a.aiGroupMode() {
	case "manual":
		n := -1
		a.db.QueryRow(`SELECT ai_daily FROM groups WHERE id=?`, gid).Scan(&n)
		return n
	case "auto":
		for _, g := range a.groupLimitRows() {
			if g.ID == gid {
				return g.Auto
			}
		}
	}
	return -1
}

func (a *App) groupUsedToday(gid int64) int {
	var n int
	a.db.QueryRow(`SELECT n FROM ai_group_usage WHERE group_id=? AND day=?`, gid, today()).Scan(&n)
	return n
}

func (a *App) handleGetGroupLimits(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{"mode": a.aiGroupMode(), "total": a.defaultAITotal(), "per_user": a.defaultAIDaily(), "groups": a.groupLimitRows()})
}

func (a *App) handleSetGroupLimits(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode   *string        `json:"mode"`
		Limits map[string]int `json:"limits"` // group id -> daily messages, -1 = none
	}
	if decode(r, &in) != nil {
		jsonErr(w, 400, "bad_request")
		return
	}
	if in.Mode != nil {
		if *in.Mode != "" && *in.Mode != "auto" && *in.Mode != "manual" {
			jsonErr(w, 400, "bad_request")
			return
		}
		a.db.SetSetting("ai_group_mode", *in.Mode)
	}
	for k, v := range in.Limits {
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil || v < -1 || v > 1000000 {
			jsonErr(w, 400, "bad_request")
			return
		}
		a.db.Exec(`UPDATE groups SET ai_daily=? WHERE id=?`, v, id)
	}
	a.handleGetGroupLimits(w, r)
}
