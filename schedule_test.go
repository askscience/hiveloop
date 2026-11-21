package main

import (
	"testing"
	"time"
)

func TestAIScheduleAllows(t *testing.T) {
	at := func(day, hm string) time.Time { // day "2026-11-23" is a Monday
		tt, _ := time.Parse("2006-01-02 15:04", day+" "+hm)
		return tt
	}
	work := aiSchedule{On: true, From: "08:00", To: "20:00", Days: []int{1, 2, 3, 4, 5}}
	night := aiSchedule{On: true, From: "20:00", To: "08:00", Days: []int{5}} // Friday night
	cases := []struct {
		s    aiSchedule
		t    time.Time
		want bool
	}{
		{aiSchedule{}, at("2026-11-23", "03:00"), true},                                     // no schedule: always
		{work, at("2026-11-23", "07:59"), false},                                            // Monday before 8
		{work, at("2026-11-23", "08:00"), true},                                             // Monday 8:00
		{work, at("2026-11-23", "19:59"), true},                                             // Monday 19:59
		{work, at("2026-11-23", "20:00"), false},                                            // Monday 20:00 (end excluded)
		{work, at("2026-11-28", "10:00"), false},                                            // Saturday
		{night, at("2026-11-27", "21:00"), true},                                            // Friday evening
		{night, at("2026-11-28", "07:30"), true},                                            // Saturday early morning = Friday's window
		{night, at("2026-11-28", "21:00"), false},                                           // Saturday evening
		{night, at("2026-11-27", "07:30"), false},                                           // Friday early morning = Thursday's window
		{aiSchedule{On: true, From: "22:00", To: "06:00"}, at("2026-11-25", "23:30"), true}, // every day
	}
	for i, c := range cases {
		if got := c.s.allows(c.t); got != c.want {
			t.Errorf("case %d (%s %s-%s %v at %s): got %v", i, c.t.Weekday(), c.s.From, c.s.To, c.s.Days, c.t.Format("Mon 15:04"), got)
		}
	}
	if (aiSchedule{On: true, From: "25:00", To: "08:00"}).valid() || (aiSchedule{On: true, From: "08:00", To: "08:00"}).valid() {
		t.Error("invalid schedules accepted")
	}
}
