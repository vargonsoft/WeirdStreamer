package media

import (
	"os"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestAutoTrickplayFrames(t *testing.T) {
	gb := int64(1 << 30)
	cpu := TrickplayMemory{TotalBytes: 4 * gb, ProcessBytes4K: trickplayCPU.process, FrameBytes4K: trickplayCPU.frame}
	vaapi := TrickplayMemory{TotalBytes: 4 * gb, ProcessBytes4K: trickplayVAAPI.process, FrameBytes4K: trickplayVAAPI.frame}
	cuda := TrickplayMemory{TotalBytes: 64 * gb, ProcessBytes4K: trickplayCUDA.process, FrameBytes4K: trickplayCUDA.frame,
		CardProcessBytes4K: trickplayCUDACard.process, CardFrameBytes4K: trickplayCUDACard.frame, CardBytes: 6 * gb}
	for _, c := range []struct {
		name string
		r    int
		m    TrickplayMemory
		want int
	}{
		{"4 GB, the CPU, 1 runner", 1, cpu, 8},  // (2048-60)/228
		{"4 GB, the CPU, 2 runners", 2, cpu, 4}, // (1024-60)/228
		{"4 GB, VA-API, 1 runner", 1, vaapi, 5}, // (2048-40)/370
		{"4 GB, the CPU, 8 runners", 8, cpu, 1}, // less than one fits: still one
		{"64 GB, the CPU", 1, TrickplayMemory{TotalBytes: 64 * gb, ProcessBytes4K: trickplayCPU.process, FrameBytes4K: trickplayCPU.frame}, 50},
		{"64 GB, the CPU, 4 runners", 4, TrickplayMemory{TotalBytes: 64 * gb, ProcessBytes4K: trickplayCPU.process, FrameBytes4K: trickplayCPU.frame}, 25}, // 100 at once
		{"a 6 GB card", 1, cuda, 10},           // (5530-60)/540
		{"a 6 GB card, 2 runners", 2, cuda, 5}, // (2765-60)/540
		{"memory unknown", 1, TrickplayMemory{FrameBytes4K: trickplayCPU.frame}, 10},
	} {
		if got := AutoTrickplayFrames(c.r, c.m); got != c.want {
			t.Errorf("%s: %d frames, want %d", c.name, got, c.want)
		}
	}
}

func TestCountDisks(t *testing.T) {
	a, b := t.TempDir(), t.TempDir() // the same disk
	if n := countDisks([]string{a, b}); n != 1 {
		t.Errorf("two folders on one disk: %d disks", n)
	}
	if n := countDisks(nil); n != 1 {
		t.Errorf("no libraries: %d disks, want 1", n)
	}
	// /proc is a device of its own.
	if _, err := os.Stat("/proc/self"); err == nil {
		if n := countDisks([]string{a, "/proc"}); n != 2 {
			t.Errorf("a disk and /proc: %d disks, want 2", n)
		}
	}
}

func TestTrickplaySettingsAuto(t *testing.T) {
	defer config.SetTrickplaySettings(0, 0)
	config.SetTrickplaySettings(3, 7)
	if r, f := TrickplayRunners(), TrickplayFramesPerRunner(); r != 3 || f != 7 {
		t.Errorf("set: %d × %d, want 3 × 7", r, f)
	}
	config.SetTrickplaySettings(2, 0)
	if want := AutoTrickplayFrames(2, CurrentTrickplayMemory()); TrickplayFramesPerRunner() != want {
		t.Errorf("automatic frames: %d, want %d", TrickplayFramesPerRunner(), want)
	}
}
