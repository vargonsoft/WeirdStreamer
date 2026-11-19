package tlscert

import (
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	saved := time.Local
	time.Local = time.UTC
	defer func() { time.Local = saved }()
	now := time.Date(2027, 10, 1, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		served, disk time.Time
		state        string
		days         int
		newer        bool
	}{
		{now.Add(90 * day), time.Time{}, "ok", 90, false},
		{now.Add(29*day + time.Hour), time.Time{}, "expiring", 29, false},
		{now.Add(30*day + time.Hour), time.Time{}, "ok", 30, false},
		{now.Add(-time.Hour), time.Time{}, "expired", 0, false},
		{now.Add(-3*day - time.Hour), time.Time{}, "expired", -3, false},
		{now.Add(11 * time.Hour), time.Time{}, "expiring", 0, false},
		{now.Add(13 * time.Hour), time.Time{}, "expiring", 1, false},
		{now.Add(5 * day), now.Add(400 * day), "expiring", 5, true},
		{now.Add(90 * day), now.Add(90 * day), "ok", 90, false},
	}
	for i, c := range cases {
		s := Check(c.served, c.disk, now)
		if s.State != c.state || s.DaysLeft != c.days || s.NewerOnDisk != c.newer {
			t.Errorf("case %d: got %s %d newer=%v, want %s %d newer=%v", i, s.State, s.DaysLeft, s.NewerOnDisk, c.state, c.days, c.newer)
		}
		if (s.State != "ok" || s.NewerOnDisk) != (s.Message != "") {
			t.Errorf("case %d: message %q for state %s", i, s.Message, s.State)
		}
		if s.NewerOnDisk && !strings.Contains(s.Message, "restart") {
			t.Errorf("case %d: %q doesn't say to restart", i, s.Message)
		}
	}
}

func TestCheckSaysWhen(t *testing.T) {
	saved := time.Local
	time.Local = time.UTC
	defer func() { time.Local = saved }()
	now := time.Date(2027, 10, 1, 12, 0, 0, 0, time.UTC)
	for served, want := range map[time.Time]string{
		now.Add(6 * time.Hour):      "(today)",
		now.Add(20 * time.Hour):     "(tomorrow)",
		now.Add(2*24*time.Hour - 1): "(in 2 days)",
	} {
		if m := Check(served, time.Time{}, now).Message; !strings.Contains(m, want) {
			t.Errorf("%v: %q lacks %q", served, m, want)
		}
	}
}
