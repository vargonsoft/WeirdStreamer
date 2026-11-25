package media

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShareProbes(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.mkv")
	if err := os.WriteFile(big, make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	small := filepath.Join(dir, "small.mkv")
	os.WriteFile(small, make([]byte, 1000), 0o644)

	if ms, err := timeReads(big); err != nil || ms < 0 || ms > 1000 { // 0 on a fast disk with a coarse clock
		t.Errorf("timeReads = %v ms, %v", ms, err)
	}
	if _, err := timeReads(small); err == nil {
		t.Error("a file too small to time was timed")
	}
	if _, err := timeReads(filepath.Join(dir, "gone.mkv")); err == nil {
		t.Error("a missing file was timed")
	}
	if IsTimedMount(dir) {
		t.Errorf("%s was taken for a network share or FUSE mount", dir)
	}

	if runtime.GOOS != "linux" {
		return // the load monitor reads Linux's /proc
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewLoadMonitor(ctx, func() []ShareProbe {
		return []ShareProbe{{Library: "Movies", Path: big}, {Library: "Gone", Path: filepath.Join(dir, "gone.mkv")}}
	})
	deadline := time.Now().Add(5 * time.Second)
	for len(m.Shares()) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	sh := m.Shares()
	if len(sh) != 1 || sh[0].Library != "Movies" || sh[0].Ms < 0 {
		t.Errorf("shares = %+v, want one reading for Movies", sh)
	}
	var none *LoadMonitor
	if none.Shares() != nil {
		t.Error("no monitor has share readings")
	}
}

// A FUSE mount (a mergerfs pool of disks) is timed like a share: its device is no block device, so
// /proc/diskstats can't say which disk it's on.
func TestFUSEIsTimed(t *testing.T) {
	b, err := os.ReadFile("/proc/mounts")
	if err != nil {
		t.Skip("no /proc/mounts")
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 || !strings.HasPrefix(f[2], "fuse.") {
			continue
		}
		if _, err := os.Stat(f[1]); err != nil {
			continue
		}
		if !IsTimedMount(f[1]) {
			t.Errorf("%s (%s) wasn't taken for a timed mount", f[1], f[2])
		}
		return
	}
	t.Skip("no FUSE mount here")
}
