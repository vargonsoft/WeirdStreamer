package auth

import (
	"testing"
	"time"
)

// A limiter whose clock the test moves by hand.
func newTestLimiter() (*Limiter, *time.Time) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewLimiter()
	l.now = func() time.Time { return now }
	return l, &now
}

// fail makes n wrong-password checks of username from addr.
func fail(t *testing.T, l *Limiter, username, addr string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		a, wait := l.Begin(username, addr)
		if a == nil {
			t.Fatalf("check %d of %s from %s refused for %v", i+1, username, addr, wait)
		}
		a.Failed()
	}
}

// wait is how long a check of username from addr would have to wait (and ends it if let through).
func wait(l *Limiter, username, addr string) time.Duration {
	a, wait := l.Begin(username, addr)
	if a != nil {
		a.Abandon()
	}
	return wait
}

func TestDelayDoublesToCap(t *testing.T) {
	want := map[int]time.Duration{
		4:  0,
		5:  30 * time.Second,
		6:  60 * time.Second,
		7:  2 * time.Minute,
		8:  4 * time.Minute,
		9:  8 * time.Minute,
		10: 16 * time.Minute,
		11: 32 * time.Minute,
		12: 60 * time.Minute,
		13: 60 * time.Minute,
		50: 60 * time.Minute,
	}
	for failures, delay := range want {
		if got := delayFor(failures); got != delay {
			t.Errorf("delayFor(%d) = %v, want %v", failures, got, delay)
		}
	}
}

func TestLockLiftsAfterDelay(t *testing.T) {
	l, now := newTestLimiter()
	fail(t, l, "viewer", "10.0.0.5", freeAttempts)
	if got := wait(l, "viewer", "10.0.0.6"); got != baseDelay {
		t.Fatalf("wait after %d failures = %v, want %v", freeAttempts, got, baseDelay)
	}
	*now = now.Add(baseDelay - time.Second)
	if got := wait(l, "viewer", "10.0.0.6"); got != time.Second {
		t.Errorf("wait with 1s left = %v", got)
	}
	*now = now.Add(time.Second)
	if got := wait(l, "viewer", "10.0.0.6"); got != 0 {
		t.Errorf("wait after the lock ran out = %v, want 0", got)
	}
}

// Checks still running count, so parallel guesses can't all start before the first one fails.
func TestRunningChecksCount(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < freeAttempts; i++ {
		if a, _ := l.Begin("viewer", "10.0.0.5"); a == nil {
			t.Fatalf("check %d refused", i+1)
		}
	}
	if got := wait(l, "viewer", "10.0.0.6"); got != busyWait {
		t.Errorf("wait with %d checks running = %v, want %v", freeAttempts, got, busyWait)
	}
}

// Once the free attempts are used, checks go one at a time.
func TestOneCheckAtATimeAfterLock(t *testing.T) {
	l, now := newTestLimiter()
	fail(t, l, "viewer", "10.0.0.5", freeAttempts)
	*now = now.Add(baseDelay)
	if a, _ := l.Begin("viewer", "10.0.0.5"); a == nil {
		t.Fatal("check after the lock ran out refused")
	}
	if got := wait(l, "viewer", "10.0.0.6"); got != busyWait {
		t.Errorf("second check while one runs: wait %v, want %v", got, busyWait)
	}
}

// A right password clears the username's failures but not the address's.
func TestPassClearsUserNotAddress(t *testing.T) {
	l, _ := newTestLimiter()
	fail(t, l, "viewer", "10.0.0.5", freeAttempts-1)
	a, _ := l.Begin("viewer", "10.0.0.5")
	a.Passed()
	if l.failures["user:viewer"] != nil {
		t.Error("user's failures kept after a right password")
	}
	if rec := l.failures["ip:10.0.0.5"]; rec == nil || rec.count != freeAttempts-1 {
		t.Errorf("address's failures = %+v, want count %d", rec, freeAttempts-1)
	}
}

// An address the user signed in from isn't held by the username's lock; other addresses are.
func TestKnownAddressSkipsUserLock(t *testing.T) {
	l, _ := newTestLimiter()
	a, _ := l.Begin("viewer", "10.0.0.5")
	a.Passed()
	fail(t, l, "viewer", "203.0.113.9", freeAttempts)
	if got := wait(l, "viewer", "10.0.0.5"); got != 0 {
		t.Errorf("known address: wait %v, want 0", got)
	}
	if got := wait(l, "viewer", "198.51.100.7"); got != baseDelay {
		t.Errorf("other address: wait %v, want %v", got, baseDelay)
	}
}

// An abandoned check counts neither way.
func TestAbandonDoesNotCount(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < 2*freeAttempts; i++ {
		a, wait := l.Begin("viewer", "10.0.0.5")
		if a == nil {
			t.Fatalf("check %d refused for %v", i+1, wait)
		}
		a.Abandon()
	}
	if len(l.failures) != 0 {
		t.Errorf("records left after abandoned checks: %v", l.failures)
	}
}

// Either key being locked is enough to refuse.
func TestAnyLockedKeyRefuses(t *testing.T) {
	l, _ := newTestLimiter()
	for i := 0; i < freeAttempts; i++ {
		fail(t, l, "user"+string(rune('a'+i)), "10.0.0.5", 1)
	}
	if got := wait(l, "viewer", "10.0.0.5"); got != baseDelay {
		t.Errorf("wait with the address locked = %v, want %v", got, baseDelay)
	}
}

func TestQuietKeyStartsOver(t *testing.T) {
	l, now := newTestLimiter()
	fail(t, l, "viewer", "10.0.0.5", freeAttempts)
	*now = now.Add(baseDelay)
	fail(t, l, "viewer", "10.0.0.5", 1)
	*now = now.Add(maxDelay + time.Second)
	if got := wait(l, "viewer", "10.0.0.5"); got != 0 {
		t.Errorf("wait after a quiet hour = %v, want 0", got)
	}
	fail(t, l, "viewer", "10.0.0.5", 1)
	if l.failures["user:viewer"].count != 1 {
		t.Errorf("count after quiet hour = %d, want 1", l.failures["user:viewer"].count)
	}
}
