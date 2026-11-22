package main

import (
	"testing"
	"time"
)

// Group 1 becomes private (members 2 and 3); group 2 stays open (member 4); user 5 is in none.
func TestPrivateGroups(t *testing.T) {
	a := testApp(t)
	g1 := map[string]string{"id": "1"}

	// only admins switch a group to invite only (the leader of group 1 can't)
	a.db.Exec(`UPDATE groups SET leader_id=2 WHERE id=1`)
	call(a.handleUpdateGroup, user(2), "PATCH", "/api/groups/1", map[string]bool{"private": true}, g1)
	if a.groupPrivate(1) {
		t.Fatal("a leader made the group private")
	}
	call(a.handleUpdateGroup, user(1), "PATCH", "/api/groups/1", map[string]bool{"private": true}, g1)
	if !a.groupPrivate(1) {
		t.Fatal("admin could not make the group private")
	}

	// hidden from non-members, visible to members and admins
	for _, c := range []struct {
		uid  int64
		sees bool
	}{{1, true}, {2, true}, {4, false}, {5, false}} {
		gs, _ := a.listGroups(user(c.uid))
		found := false
		for _, g := range gs {
			if g.ID == 1 {
				found = true
				if !g.Private {
					t.Errorf("user %d: private flag missing", c.uid)
				}
			}
		}
		if found != c.sees {
			t.Errorf("user %d sees private group: %v, want %v", c.uid, found, c.sees)
		}
		if a.canSeeGroup(user(c.uid), 1) != c.sees {
			t.Errorf("canSeeGroup(%d) wrong", c.uid)
		}
		if w := call(a.handleGroupMembers, user(c.uid), "GET", "/api/groups/1/members", nil, g1); (w.Code == 200) != c.sees {
			t.Errorf("user %d members: %d", c.uid, w.Code)
		}
	}

	// nobody joins by themselves; an admin (or the leader) invites
	if w := call(a.handleJoinGroup, user(5), "POST", "/api/groups/1/join", nil, g1); w.Code != 403 || a.isMember(5, 1) {
		t.Fatalf("join private group: %d", w.Code)
	}
	if w := call(a.handleJoinGroup, user(5), "POST", "/api/groups/2/join", nil, map[string]string{"id": "2"}); w.Code != 200 || !a.isMember(5, 2) {
		t.Fatalf("join open group: %d", w.Code)
	}
	a.db.Exec(`UPDATE users SET status='active'`)
	if w := call(a.handleAddMember, user(2), "POST", "/api/groups/1/members", map[string]int64{"user_id": 5}, g1); w.Code != 200 || !a.isMember(5, 1) {
		t.Fatalf("leader invite: %d", w.Code)
	}
	var notes int
	a.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id=5`).Scan(&notes)
	if notes != 1 {
		t.Errorf("invited person got %d notifications, want 1", notes)
	}

	// events: private by default in a private group, public only when asked
	start := time.Now().Add(48 * time.Hour).Truncate(time.Hour)
	in := func(gid int64, private *bool) EventInput {
		return EventInput{Title: ptr("x"), Start: ptr(start.Format(time.RFC3339)), GroupID: ptr(gid), Private: private}
	}
	def, err := a.createEvent(user(2), in(1, nil), false)
	if err != nil || !def.Private {
		t.Fatalf("private group event default: %v private=%v", err, def.Private)
	}
	pub, err := a.createEvent(user(2), in(1, ptr(false)), false)
	if err != nil || pub.Private {
		t.Fatalf("private group event shared on purpose: %v private=%v", err, pub.Private)
	}
	open, _ := a.createEvent(user(4), in(2, nil), false)
	if open.Private {
		t.Fatal("open group event private by default")
	}
	// moving an event into the private group makes it internal, unless said otherwise
	moved, err := a.updateEvent(user(1), open.ID, EventInput{GroupID: ptr(int64(1))})
	if err != nil || !moved.Private {
		t.Fatalf("moved into private group: %v private=%v", err, moved.Private)
	}
	evs, _ := a.listEvents(user(4), start.Add(-time.Hour), start.Add(3*time.Hour), "all")
	got := ids(evs)
	if got[def.ID] || got[moved.ID] || !got[pub.ID] {
		t.Errorf("outsider sees %v (default %d, moved %d, shared %d)", got, def.ID, moved.ID, pub.ID)
	}
}
