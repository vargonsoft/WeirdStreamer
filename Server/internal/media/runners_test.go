package media

import (
	"context"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/jobs"
)

// waitsWithin says whether runner w's wait returns (true) within d.
func waitsWithin(r *runners, j *jobs.Job, w int, d time.Duration) (returned, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- r.wait(ctx, j, nil, w) }()
	select {
	case ok := <-done:
		return ctx.Err() == nil, ok
	case <-time.After(d + time.Second):
		return false, false
	}
}

func TestRunnersDropOneInsteadOfPausing(t *testing.T) {
	config.GlobalConfig = &config.Config{}
	old := load
	defer func() { load = old }()
	load = &LoadMonitor{cpu: 0.9, ok: true, shares: map[string]*shareProbe{}}
	j := jobs.Register("test-runners", "Runners", "")
	j.Begin(0)
	r := newRunners()
	r.changed = time.Now().Add(-time.Hour)

	// CPU busy with 3 runners: the last one stops, and the others carry on.
	if returned, _ := waitsWithin(r, j, 2, 200*time.Millisecond); returned {
		t.Fatal("runner 2 went on after the CPU got busy; it should have stopped")
	}
	if r.active != 2 {
		t.Fatalf("active = %d, want 2", r.active)
	}
	if returned, ok := waitsWithin(r, j, 0, 200*time.Millisecond); !returned || !ok {
		t.Fatal("runner 0 paused while a drop was settling")
	}
	// Settled and still busy: down to one runner, which still carries on.
	r.changed = time.Now().Add(-time.Hour)
	if returned, _ := waitsWithin(r, j, 0, 200*time.Millisecond); !returned || r.active != 1 {
		t.Fatalf("after a second drop: returned %v, active %d", returned, r.active)
	}
	// One runner left and still busy: now the job pauses.
	if returned, _ := waitsWithin(r, j, 0, 200*time.Millisecond); returned {
		t.Fatal("the last runner went on with the CPU busy")
	}

	// The CPU quiet for a while: a runner is added back.
	load.cpu = 0.2
	r.changed = time.Now().Add(-time.Hour)
	if returned, _ := waitsWithin(r, j, 0, 200*time.Millisecond); !returned || r.active != 2 {
		t.Fatalf("after quiet: returned %v, active %d", returned, r.active)
	}

	// Every video handed out: a parked runner goes on with the one it holds.
	r.changed = time.Now()
	r.mu.Lock()
	r.set(1)
	r.mu.Unlock()
	go func() { time.Sleep(50 * time.Millisecond); r.drain() }()
	if returned, ok := waitsWithin(r, j, 2, time.Second); !returned || !ok {
		t.Fatal("drain didn't release a parked runner")
	}
}
