// Load test for Hiveloop: N simulated users that log in, load the app, keep the live stream
// open and use calendar, chats, groups and documents with realistic pauses.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	base    = flag.String("url", "http://127.0.0.1:18091", "Hiveloop base URL")
	users   = flag.Int("users", 50, "simulated users")
	ramp    = flag.Duration("ramp", 60*time.Second, "time to bring all users online")
	dur     = flag.Duration("dur", 5*time.Minute, "duration after ramp")
	adminPw = flag.String("adminpw", "", "test admin password")
	probe   = flag.String("probe", "", "optional URL probed every 5 s (production, via Funnel)")
	setup   = flag.Bool("setup", true, "create users, groups, events and documents first")
	userPw  = "loadtest-password-1"
	assets  = []string{"/", "/app.css", "/fonts/fonts.css", "/fonts/Roboto-latin.woff2", "/vendor/marked.min.js", "/vendor/purify.min.js", "/app.js", "/i18n.js", "/icons.js", "/help.js", "/editor.js", "/preview.js", "/avatar.js", "/vendor/avataaars.mjs", "/manifest.webmanifest", "/favicon.svg"}
)

// ---------- stats ----------

type stat struct {
	mu    sync.Mutex
	lat   map[string][]time.Duration
	codes map[string]map[int]int
	bytes int64
}

var st = stat{lat: map[string][]time.Duration{}, codes: map[string]map[int]int{}}
var sseEvents, sseOpen int64

func (s *stat) add(kind string, d time.Duration, code int, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lat[kind] = append(s.lat[kind], d)
	if s.codes[kind] == nil {
		s.codes[kind] = map[int]int{}
	}
	s.codes[kind][code]++
	s.bytes += n
}

func pct(v []time.Duration, p float64) time.Duration {
	if len(v) == 0 {
		return 0
	}
	i := int(float64(len(v)-1) * p)
	return v[i]
}

// ---------- client ----------

type client struct {
	h  *http.Client
	ip string
}

func newClient(ip string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{h: &http.Client{Jar: jar, Timeout: 30 * time.Second}, ip: ip}
}

func (c *client) do(kind, method, path string, body any, out any) int {
	var rd io.Reader
	ct := ""
	if body != nil {
		b, _ := json.Marshal(body)
		rd, ct = bytes.NewReader(b), "application/json"
	}
	req, _ := http.NewRequest(method, *base+path, rd)
	req.Header.Set("X-Hiveloop", "1")
	req.Header.Set("X-Forwarded-For", c.ip) // as Tailscale Funnel would send it: one address per user
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	t0 := time.Now()
	resp, err := c.h.Do(req)
	if err != nil {
		st.add(kind, time.Since(t0), -1, 0)
		return -1
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	st.add(kind, time.Since(t0), resp.StatusCode, int64(len(b)))
	if out != nil && resp.StatusCode < 300 {
		json.Unmarshal(b, out)
	}
	return resp.StatusCode
}

func (c *client) stream(stop <-chan struct{}) {
	req, _ := http.NewRequest("GET", *base+"/api/stream", nil)
	req.Header.Set("X-Forwarded-For", c.ip)
	h := &http.Client{Jar: c.h.Jar} // no timeout: long-lived
	resp, err := h.Do(req)
	if err != nil {
		return
	}
	atomic.AddInt64(&sseOpen, 1)
	defer atomic.AddInt64(&sseOpen, -1)
	go func() { <-stop; resp.Body.Close() }()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "data:") {
			atomic.AddInt64(&sseEvents, 1)
		}
	}
}

// ---------- world ----------

type group struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
type event struct {
	ID int64 `json:"id"`
}
type doc struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
	OwnerID int64  `json:"owner_id"`
}

var sentences = []string{"See you on Saturday at the centre", "I will bring the projector", "Does anyone have the guide's number?", "I updated the programme",
	"Great job everyone!", "It rains tomorrow, shall we postpone?", "We need two people at the stand from 10", "I uploaded the photos of the day"}

func month() (string, string) {
	n := time.Now()
	f := time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.Local)
	return f.Format("2006-01-02"), f.AddDate(0, 1, 0).Format("2006-01-02")
}

func doSetup() {
	a := newClient("10.200.0.1")
	if a.do("setup", "POST", "/api/login", map[string]string{"username": "superadmin", "password": *adminPw}, nil) != 200 {
		fmt.Println("admin login failed")
		os.Exit(1)
	}
	names := []string{"Hiking", "Logistics", "Stands", "Education", "Photography"}
	for i, n := range names {
		a.do("setup", "POST", "/api/groups", map[string]string{"name": n, "color": []string{"#16a34a", "#0284c7", "#e11d48", "#7c3aed", "#ca8a04"}[i], "description": "Test group " + n}, nil)
	}
	var gs []group
	a.do("setup", "GET", "/api/groups", nil, &gs)
	for i := 1; i <= *users; i++ {
		g1, g2 := gs[i%len(gs)].ID, gs[(i*3)%len(gs)].ID
		a.do("setup", "POST", "/api/users", map[string]any{"username": fmt.Sprintf("user%02d", i), "name": fmt.Sprintf("User %d", i), "password": userPw, "role": "user", "groups": []int64{g1, g2}}, nil)
	}
	f, _ := month()
	d0, _ := time.Parse("2006-01-02", f)
	for i := 0; i < 40; i++ {
		day := d0.AddDate(0, 0, rand.Intn(28))
		h := 8 + rand.Intn(10)
		a.do("setup", "POST", "/api/events", map[string]any{"title": fmt.Sprintf("Activity %d", i+1), "start": day.Format("2006-01-02") + fmt.Sprintf("T%02d:00", h),
			"end": day.Format("2006-01-02") + fmt.Sprintf("T%02d:00", h+2), "location": "Community Centre", "group_id": gs[i%len(gs)].ID,
			"description": "## Programme\n- Meet\n- Activity\n- Goodbye"}, nil)
	}
	fmt.Printf("setup: %d groups, %d users, 40 events\n", len(gs), *users)
}

func user(i int, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	r := rand.New(rand.NewSource(int64(i)))
	c := newClient(fmt.Sprintf("10.200.%d.%d", 1+i/200, 1+i%200))
	for _, p := range assets { // first visit: the whole app is downloaded
		c.do("static", "GET", p, nil, nil)
	}
	if c.do("login", "POST", "/api/login", map[string]string{"username": fmt.Sprintf("user%02d", i), "password": userPw}, nil) != 200 {
		return
	}
	var me struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
		Groups []int64 `json:"groups"`
	}
	c.do("api-read", "GET", "/api/me", nil, &me)
	if i%2 == 0 { // half of them make an avatar
		c.do("api-write", "PATCH", "/api/me", map[string]any{"avatar": map[string]string{"top": "shortFlat", "eyes": "happy", "mouth": "smile", "skinColor": "edb98a", "clothing": "hoodie"}}, nil)
	}
	c.do("api-read", "GET", "/api/groups", nil, nil)
	c.do("api-read", "GET", "/api/avatars", nil, nil)
	c.do("api-read", "GET", "/api/rooms", nil, nil)
	go c.stream(stop)
	var myDoc *doc
	if i%3 == 0 { // a third of them write a shared Markdown document
		var d doc
		if c.do("api-write", "POST", "/api/docs/new", map[string]string{"name": fmt.Sprintf("Minutes %d", i), "content": "# Minutes\n\n- point one\n"}, &d) == 200 {
			myDoc = &d
			if len(me.Groups) > 0 {
				c.do("api-write", "PUT", fmt.Sprintf("/api/docs/%d/shares", d.ID), map[string]any{"groups": []int64{me.Groups[0]}}, nil)
			}
		}
	}
	f, t := month()
	for {
		select {
		case <-stop:
			return
		case <-time.After(time.Duration(3000+r.Intn(7000)) * time.Millisecond): // think time 3–10 s
		}
		room := int64(0)
		if len(me.Groups) > 0 && r.Intn(3) > 0 {
			room = me.Groups[r.Intn(len(me.Groups))]
		}
		switch x := r.Intn(100); {
		case x < 35: // chat
			c.do("api-read", "GET", fmt.Sprintf("/api/rooms/%d/messages", room), nil, nil)
			c.do("api-read", "GET", fmt.Sprintf("/api/rooms/%d/members", room), nil, nil)
			if r.Intn(2) == 0 {
				msg := sentences[r.Intn(len(sentences))]
				if r.Intn(4) == 0 {
					msg = fmt.Sprintf("@user%02d %s", 1+r.Intn(*users), msg)
				}
				c.do("chat-send", "POST", fmt.Sprintf("/api/rooms/%d/messages", room), map[string]string{"content": msg}, nil)
			}
			c.do("api-write", "POST", fmt.Sprintf("/api/rooms/%d/read", room), nil, nil)
		case x < 55: // calendar
			var evs []event
			c.do("api-read", "GET", "/api/events?from="+f+"&to="+t+"&scope=all", nil, &evs)
			if len(evs) > 0 && r.Intn(3) == 0 {
				c.do("api-write", "PUT", fmt.Sprintf("/api/events/%d/signup", evs[r.Intn(len(evs))].ID), map[string]any{"all_day": true, "note": "I'll be there"}, nil)
			}
		case x < 70: // groups
			c.do("api-read", "GET", "/api/groups", nil, nil)
			if len(me.Groups) > 0 {
				g := me.Groups[r.Intn(len(me.Groups))]
				c.do("api-read", "GET", fmt.Sprintf("/api/groups/%d/members", g), nil, nil)
				c.do("api-read", "GET", fmt.Sprintf("/api/docs?group=%d", g), nil, nil)
				c.do("api-read", "GET", fmt.Sprintf("/api/events?from=%s&to=%s&scope=%d", f, t, g), nil, nil)
			}
		case x < 88: // documents
			var list struct {
				Docs   []doc `json:"docs"`
				Shared []doc `json:"shared"`
			}
			c.do("api-read", "GET", "/api/docs", nil, &list)
			all := append(list.Docs, list.Shared...)
			if len(all) > 0 {
				c.do("api-read", "GET", fmt.Sprintf("/api/docs/%d/text", all[r.Intn(len(all))].ID), nil, nil)
			}
			if myDoc != nil && r.Intn(2) == 0 {
				var cur struct {
					Text    string `json:"text"`
					Version int64  `json:"version"`
				}
				c.do("api-read", "GET", fmt.Sprintf("/api/docs/%d/text", myDoc.ID), nil, &cur)
				c.do("doc-save", "PUT", fmt.Sprintf("/api/docs/%d/content", myDoc.ID), map[string]any{"content": cur.Text + fmt.Sprintf("- aggiunta %d\n", r.Intn(1000)), "version": cur.Version}, nil)
				if r.Intn(3) == 0 {
					c.do("api-read", "GET", fmt.Sprintf("/api/docs/%d/versions", myDoc.ID), nil, nil)
				}
			}
		default: // the app polls its version and refreshes the profile
			c.do("api-read", "GET", "/api/version", nil, nil)
			c.do("api-read", "GET", "/api/me", nil, nil)
		}
	}
}

func main() {
	flag.Parse()
	if *setup {
		doSetup()
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	start := time.Now()
	// production probe (network path through Funnel)
	var probeLat []time.Duration
	var probeErr int
	var pmu sync.Mutex
	if *probe != "" {
		go func() {
			h := &http.Client{Timeout: 15 * time.Second}
			for {
				select {
				case <-stop:
					return
				case <-time.After(5 * time.Second):
				}
				t0 := time.Now()
				resp, err := h.Get(*probe)
				pmu.Lock()
				if err != nil || resp.StatusCode != 200 {
					probeErr++
				} else {
					probeLat = append(probeLat, time.Since(t0))
				}
				pmu.Unlock()
				if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}
	for i := 1; i <= *users; i++ {
		wg.Add(1)
		go user(i, stop, &wg)
		time.Sleep(*ramp / time.Duration(*users))
	}
	fmt.Printf("all %d users started after %s\n", *users, time.Since(start).Round(time.Second))
	tick := time.NewTicker(30 * time.Second)
	end := time.After(*dur)
loop:
	for {
		select {
		case <-tick.C:
			st.mu.Lock()
			n := 0
			for _, v := range st.lat {
				n += len(v)
			}
			st.mu.Unlock()
			fmt.Printf("  t=%s requests=%d live-streams=%d live-events=%d\n", time.Since(start).Round(time.Second), n, atomic.LoadInt64(&sseOpen), atomic.LoadInt64(&sseEvents))
		case <-end:
			break loop
		}
	}
	close(stop)
	wg.Wait()
	total := time.Since(start)

	st.mu.Lock()
	kinds := []string{}
	for k := range st.lat {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	fmt.Printf("\n%-10s %7s %8s %8s %8s %8s  %s\n", "type", "count", "p50", "p95", "p99", "max", "status")
	all := 0
	for _, k := range kinds {
		v := st.lat[k]
		sort.Slice(v, func(a, b int) bool { return v[a] < v[b] })
		all += len(v)
		codes := []string{}
		for c, n := range st.codes[k] {
			codes = append(codes, fmt.Sprintf("%d×%d", c, n))
		}
		sort.Strings(codes)
		r := func(d time.Duration) string { return d.Round(time.Millisecond).String() }
		fmt.Printf("%-10s %7d %8s %8s %8s %8s  %s\n", k, len(v), r(pct(v, .5)), r(pct(v, .95)), r(pct(v, .99)), r(v[len(v)-1]), strings.Join(codes, " "))
	}
	fmt.Printf("\ntotal %d requests in %s (%.1f req/s), %.1f MB downloaded, live events delivered: %d\n", all, total.Round(time.Second), float64(all)/total.Seconds(), float64(st.bytes)/1e6, sseEvents)
	st.mu.Unlock()
	if *probe != "" {
		pmu.Lock()
		sort.Slice(probeLat, func(a, b int) bool { return probeLat[a] < probeLat[b] })
		fmt.Printf("production via Funnel: %d probes, p50 %s, p95 %s, max %s, errors %d\n", len(probeLat), pct(probeLat, .5).Round(time.Millisecond), pct(probeLat, .95).Round(time.Millisecond), pct(probeLat, 1).Round(time.Millisecond), probeErr)
		pmu.Unlock()
	}
}
