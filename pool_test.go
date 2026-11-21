package main

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Regression: listing events used to run a membership query for every row while the rows
// were still open. With the pool capped at 4 connections, a handful of ordinary users
// listing the calendar at once could take all connections and wait forever for a fifth.
func TestEventsListNoPoolDeadlock(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := &App{db: db, cfg: &Config{TZ: time.UTC}}
	now := time.Now().Unix()
	db.Exec(`INSERT INTO groups(id,name,created_at) VALUES(1,'g',?)`, now)
	for i := 1; i <= 20; i++ {
		db.Exec(`INSERT INTO users(id,username,password_hash,role,created_at) VALUES(?,?,'x','user',?)`, i, "u"+string(rune('a'+i)), now)
		db.Exec(`INSERT INTO group_members(group_id,user_id,joined_at) VALUES(1,?,?)`, i, now)
	}
	for i := 0; i < 200; i++ {
		db.Exec(`INSERT INTO events(title,start_at,end_at,group_id,created_at,updated_at) VALUES('e',?,?,1,?,?)`, now+int64(i*60), now+int64(i*60)+3600, now, now)
	}
	done := make(chan struct{})
	go func() {
		var wg sync.WaitGroup
		for i := 1; i <= 20; i++ {
			wg.Add(1)
			go func(uid int64) {
				defer wg.Done()
				u := &User{ID: uid, Role: "user"}
				for k := 0; k < 5; k++ {
					evs, err := a.listEvents(u, time.Unix(now-3600, 0), time.Unix(now+86400, 0), "all")
					if err != nil || len(evs) != 200 || !evs[0].CanEdit {
						t.Errorf("user %d: %d events, err %v", uid, len(evs), err)
						return
					}
				}
			}(int64(i))
		}
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("listEvents deadlocked: connection pool exhausted")
	}
}
