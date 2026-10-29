package media

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/jobs"
)

// within runs f and says whether it returned (and what) within d.
func within(d time.Duration, f func(ctx context.Context) bool) (returned, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- f(ctx) }()
	select {
	case ok := <-done:
		return ctx.Err() == nil, ok
	case <-time.After(d + time.Second):
		return false, false
	}
}

func setRunners(n int) {
	config.SetTrickplaySettings(n, 25)
}

func TestRunnersFollowTheSetting(t *testing.T) {
	config.GlobalConfig = &config.Config{}
	defer config.SetTrickplaySettings(0, 0)
	old := load
	defer func() { load = old }()
	// A busy CPU doesn't hold the runners back (the jobs run at low priority instead).
	load = &LoadMonitor{cpu: 0.95, ok: true, shares: map[string]*shareProbe{}}
	j := jobs.Register("test-runners", "Runners", "")
	j.Begin(0)
	setRunners(2)
	r := newRunners()

	if returned, ok := within(200*time.Millisecond, func(ctx context.Context) bool { return r.wait(ctx, j, nil, 0) }); !returned || !ok {
		t.Fatal("runner 0 waited with the CPU busy")
	}
	if returned, _ := within(200*time.Millisecond, func(ctx context.Context) bool { return r.claim(ctx, j, 2) }); returned {
		t.Fatal("runner 2 took a video with 2 runners set")
	}

	// Raised on the Jobs page: a waiting runner starts at once.
	done := make(chan bool, 1)
	go func() { done <- r.claim(context.Background(), j, 2) }()
	time.Sleep(50 * time.Millisecond)
	setRunners(3)
	r.mu.Lock()
	r.update(j) // what any runner's next step does
	r.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("raising the setting didn't let runner 2 start")
	}

	// Lowered part way through runner 2's video: it waits, and the Jobs page says so.
	setRunners(1)
	if returned, _ := within(200*time.Millisecond, func(ctx context.Context) bool { return r.wait(ctx, j, nil, 2) }); returned {
		t.Fatal("runner 2 went on after the setting went down to 1")
	}
	go func() { done <- r.wait(context.Background(), j, nil, 2) }()
	time.Sleep(50 * time.Millisecond)
	var shown string
	j.SetRunner(2, "clip.mkv") // as the worker does when it takes a video
	j.RunnerWaiting(2, true)
	for _, s := range jobs.All() {
		if s.ID == "test-runners" {
			shown = s.Runners[2]
		}
	}
	if !strings.Contains(shown, "waiting") {
		t.Errorf("a stopped runner's file shows as %q", shown)
	}

	// Every video handed out: the stopped runner goes on with the one it holds.
	r.drain()
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("drain released runner 2 with false")
		}
	case <-time.After(time.Second):
		t.Fatal("drain didn't release the stopped runner")
	}
}
