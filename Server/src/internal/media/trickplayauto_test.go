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
		if got := AutoTrickplayFrames(c.r, c.m, 0); got != c.want {
			t.Errorf("%s: %d frames, want %d", c.name, got, c.want)
		}
	}
	// Smaller videos take more frames: each tier's memory per frame, and its own caps.
	for _, c := range []struct {
		name    string
		r, tier int
		m       TrickplayMemory
		want    int
	}{
		{"4 GB, the CPU, 1080p", 1, 1, cpu, 23}, // (2048-60)/84
		{"4 GB, the CPU, 720p", 1, 2, cpu, 48},  // (2048-60)/41
		{"4 GB, the CPU, SD", 1, 3, cpu, 96},    // (2048-60)/20.5
		{"24 GB, VA-API, 2 runners, 1080p", 2, 1, TrickplayMemory{TotalBytes: 24 * gb, ProcessBytes4K: trickplayVAAPI.process, FrameBytes4K: trickplayVAAPI.frame}, 44}, // (6144-40)/136.9
		{"64 GB, the CPU, SD", 1, 3, TrickplayMemory{TotalBytes: 64 * gb, ProcessBytes4K: trickplayCPU.process, FrameBytes4K: trickplayCPU.frame}, 200},                 // SD's cap
		{"64 GB, the CPU, 4 runners, 720p", 4, 2, TrickplayMemory{TotalBytes: 64 * gb, ProcessBytes4K: trickplayCPU.process, FrameBytes4K: trickplayCPU.frame}, 75},     // 300 at once
	} {
		if got := AutoTrickplayFrames(c.r, c.m, c.tier); got != c.want {
			t.Errorf("%s: %d frames, want %d", c.name, got, c.want)
		}
	}
}

func TestTrickplayTier(t *testing.T) {
	for _, c := range []struct{ w, h, tier int }{
		{3840, 2160, 0}, {2560, 1440, 0}, {0, 0, 0}, {1920, 800, 1}, {1920, 1080, 1}, {1280, 720, 2}, {720, 576, 3}, {640, 480, 3},
	} {
		if got := TrickplayTier(c.w, c.h); got != c.tier {
			t.Errorf("%d×%d: tier %d, want %d", c.w, c.h, got, c.tier)
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
	config.SetTrickplaySettings(3, 7, 0, 0, 30)
	if r, f, sd := TrickplayRunners(), TrickplayFramesPerRunner(0), TrickplayFramesPerRunner(3); r != 3 || f != 7 || sd != 30 {
		t.Errorf("set: %d × %d (SD %d), want 3 × 7 (SD 30)", r, f, sd)
	}
	if want := AutoTrickplayFrames(3, CurrentTrickplayMemory(), 1); TrickplayFramesPerRunner(1) != want {
		t.Errorf("automatic 1080p frames: %d, want %d", TrickplayFramesPerRunner(1), want)
	}
	config.SetTrickplaySettings(2, 0)
	if want := AutoTrickplayFrames(2, CurrentTrickplayMemory(), 0); TrickplayFramesPerRunner(0) != want {
		t.Errorf("automatic frames: %d, want %d", TrickplayFramesPerRunner(0), want)
	}
}

// The Jobs page's CPU choice keeps trickplay off the GPU the hardware encoder chose, while the jobs
// reply still names that GPU.
func TestTrickplayCPUOnly(t *testing.T) {
	saved := config.GlobalConfig
	hardware.once.Do(func() {}) // no detection: the report below stands
	hardware.Lock()
	savedReport := hardware.report
	hardware.report = HardwareReport{Encoders: []HardwareEncoder{{ID: "nvenc", Name: "NVENC", GPU: "NVIDIA GeForce GTX 1060 6GB"}}}
	hardware.Unlock()
	defer func() {
		config.GlobalConfig = saved
		config.SetTrickplayCPUOnly(false)
		hardware.Lock()
		hardware.report = savedReport
		hardware.Unlock()
	}()
	config.GlobalConfig = &config.Config{Streaming: config.StreamingConfig{HardwareEncoder: "nvenc"}}

	if g := trickplayDecoder(); g == nil || g.hwaccel != "cuda" {
		t.Fatalf("with NVENC chosen: decoder %v, want cuda", g)
	}
	config.SetTrickplayCPUOnly(true)
	if g := trickplayDecoder(); g != nil {
		t.Errorf("CPU chosen on the Jobs page: decoder %v, want the CPU", g)
	}
	if name := TrickplayGPUName(); name != "NVIDIA GeForce GTX 1060 6GB" {
		t.Errorf("GPU name %q with the CPU chosen", name)
	}
	if m := CurrentTrickplayMemory(); m.Decoder != "cpu" {
		t.Errorf("memory estimate for %q, want cpu", m.Decoder)
	}
}
