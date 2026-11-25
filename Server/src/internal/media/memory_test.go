package media

import (
	"runtime"
	"testing"
)

// The batch limit keeps every runner's batch inside the budget: ws (4 GB, 3 runners) makes a 4K
// HEVC video 2 frames at a time, a 1080p H.264 one about 15 (and a setting under that wins).
func TestTrickplayBatchLimit(t *testing.T) {
	budget := trickplayBudget(4 << 30)
	if got := trickplayBatchLimit(3840, 2160, true, 3, budget); got != 2 {
		t.Errorf("4K HEVC, 3 runners, 2 GB: %d frames, want 2", got)
	}
	if got := trickplayBatchLimit(1920, 1080, false, 3, budget); got < 12 || got > 18 {
		t.Errorf("1080p H.264, 3 runners, 2 GB: %d frames, want about 15", got)
	}
	if got := trickplayBatchLimit(3840, 2160, true, 8, trickplayBudget(1<<30)); got != 1 {
		t.Errorf("never under one frame: %d", got)
	}
	if got := trickplayBatchLimit(3840, 2160, true, 3, 0); got != 0 {
		t.Errorf("unknown memory: %d, want no limit", got)
	}
	if got := trickplayBatchLimit(0, 0, true, 3, budget); got != 0 {
		t.Errorf("unknown size: %d, want no limit", got)
	}
	// The 4K figure is what was measured: about 228 MB a frame.
	if mb := trickplayFrameBytes(3840, 2160, true) >> 20; mb < 220 || mb > 250 {
		t.Errorf("4K HEVC frame: %d MB, want about 228", mb)
	}
	if !heavyDecoder("h264", "yuv420p10le") || heavyDecoder("h264", "yuv420p") || !heavyDecoder("hevc", "yuv420p") {
		t.Error("heavyDecoder: HEVC or more than 8 bits")
	}
}

func TestSystemMemory(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("no memory reading here")
	}
	if got := SystemMemory(); got < 256<<20 {
		t.Errorf("SystemMemory() = %d", got)
	}
	if runtime.GOOS == "linux" && memTotal() <= 0 {
		t.Error("MemTotal not read")
	}
}
