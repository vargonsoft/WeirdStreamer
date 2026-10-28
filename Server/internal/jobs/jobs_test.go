package jobs

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

func status(id string) Status {
	for _, s := range All() {
		if s.ID == id {
			return s
		}
	}
	return Status{}
}

func TestPauseHoldsUntilResumed(t *testing.T) {
	j := Register("test-pause", "Test", "")
	j.Begin(2)
	j.Pause()
	if s := status("test-pause"); s.State != Paused || !s.Paused {
		t.Fatalf("paused job shows %q", s.State)
	}
	passed := make(chan bool)
	go func() { passed <- j.Wait(context.Background(), nil) }()
	select {
	case <-passed:
		t.Fatal("a paused job went on")
	case <-time.After(50 * time.Millisecond):
	}
	j.Resume()
	select {
	case ok := <-passed:
		if !ok {
			t.Fatal("Wait returned false after Resume")
		}
	case <-time.After(time.Second):
		t.Fatal("Resume didn't release the job")
	}
	if s := status("test-pause"); s.State != Running {
		t.Fatalf("resumed job shows %q", s.State)
	}
}

func TestWaitShowsWhyAndEndsWithContext(t *testing.T) {
	j := Register("test-busy", "Test", "")
	j.Begin(0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool)
	go func() { done <- j.Wait(ctx, func() (bool, string) { return true, "CPU busy" }) }()
	time.Sleep(50 * time.Millisecond)
	if s := status("test-busy"); s.State != Waiting || s.Note != "CPU busy" {
		t.Fatalf("busy job shows %q (%q)", s.State, s.Note)
	}
	cancel()
	if <-done {
		t.Fatal("Wait returned true after its context ended")
	}
}

func TestEndRecordsTheRun(t *testing.T) {
	j := Register("test-end", "Test", "")
	j.Begin(3)
	j.Step(3)
	j.End("3 done")
	j.NextIn(time.Hour)
	s := status("test-end")
	if s.State != Idle || s.LastResult != "3 done" || s.LastEnd == nil || s.Next == nil || s.Started != nil {
		t.Fatalf("after a run: %+v", s)
	}
}

func TestRunsAreLogged(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	j := Register("test-log", "Log test", "")
	j.Begin(0)
	j.End("2 things done")
	out := buf.String()
	if !strings.Contains(out, "jobs: Log test started") || !strings.Contains(out, "jobs: Log test finished in 0s: 2 things done") {
		t.Errorf("log: %q", out)
	}
}
