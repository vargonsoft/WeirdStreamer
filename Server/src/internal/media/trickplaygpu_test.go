package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// A batch too big for the GPU's memory halves the pixels batches may hold, so smaller videos still
// get bigger batches; videos it can't decode, a few in a row, rest it.
func TestGPUDecodeLearning(t *testing.T) {
	old := gpuDecode
	defer func() { gpuDecode = old }()
	gpuDecode = &gpuDecodeState{pixels: map[string]int64{}}
	g := &gpuDecoder{hwaccel: "cuda"}

	if got := g.batchLimit(3840, 2160); got != 0 {
		t.Fatalf("no limit yet: %d", got)
	}
	g.tooBig(20, 3840, 2160)
	if got := g.batchLimit(3840, 2160); got != 10 {
		t.Errorf("4K after 20 failed: %d, want 10", got)
	}
	if got := g.batchLimit(1920, 1080); got != 40 {
		t.Errorf("1080p after 4K's 20 failed: %d, want 40", got)
	}
	g.tooBig(40, 1920, 1080) // a smaller limit replaces it
	if got := g.batchLimit(3840, 2160); got != 5 {
		t.Errorf("4K after 1080p's 40 failed: %d, want 5", got)
	}
	g.tooBig(40, 3840, 2160) // a bigger one doesn't
	if got := g.batchLimit(3840, 2160); got != 5 {
		t.Errorf("a bigger limit replaced a smaller one: %d", got)
	}
	if (&gpuDecoder{hwaccel: "vaapi", device: "/dev/dri/renderD128"}).batchLimit(3840, 2160) != 0 {
		t.Error("another GPU took this one's limit")
	}

	for i := 0; i < gpuFailuresToRest-1; i++ {
		g.videoDone(false)
	}
	g.videoDone(true) // one it decoded starts the count again
	g.videoDone(false)
	if gpuDecode.off() {
		t.Fatal("rested before enough failures in a row")
	}
	for i := 0; i < gpuFailuresToRest; i++ {
		g.videoDone(false)
	}
	if !gpuDecode.off() || time.Until(gpuDecode.offUntil) > gpuRest {
		t.Error("not resting after failures in a row")
	}
}

// cudaDecoder is the NVIDIA GPU when this machine's ffmpeg can decode on it, else the test skips.
func cudaDecoder(t *testing.T) *gpuDecoder {
	t.Helper()
	if exec.Command("ffmpeg", "-v", "error", "-init_hw_device", "cuda=tp", "-f", "lavfi", "-i", "nullsrc=s=64x64",
		"-frames:v", "1", "-f", "null", "-").Run() != nil {
		t.Skip("no CUDA here")
	}
	return &gpuDecoder{hwaccel: "cuda"}
}

// Frames made on the GPU: all of them, and the same pictures as the CPU's (only the scaler differs).
func TestTrickplayOnGPU(t *testing.T) {
	g := cudaDecoder(t)
	clip := makeClip(t)
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.TrickplayPath = t.TempDir()
	defer func() { chooseTrickplayDecoder = trickplayDecoder }()
	chooseTrickplayDecoder = func() *gpuDecoder { return g }

	tg := &TrickplayGenerator{}
	if err := tg.generate(context.Background(), clip, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(clip))
	for i := 0; i < 300; i += 10 {
		gpuFrame := filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", i))
		if _, err := os.Stat(gpuFrame); err != nil {
			t.Fatalf("frame %d missing", i)
		}
		if i%100 != 0 {
			continue
		}
		cpuFrame := filepath.Join(t.TempDir(), "cpu.jpg")
		if !tg.extractFrame(context.Background(), clip, cpuFrame, float64(i), "h264") {
			t.Fatalf("CPU frame %d", i)
		}
		out, _ := exec.Command("ffmpeg", "-hide_banner", "-i", cpuFrame, "-i", gpuFrame, "-lavfi",
			"scale=320:180,psnr", "-f", "null", "-").CombinedOutput()
		var db float64
		if _, err := fmt.Sscanf(lastField(string(out), "average:"), "%f", &db); err == nil && db < 25 {
			t.Errorf("frame %d: the GPU's picture is %.1f dB from the CPU's", i, db)
		}
	}
}

// A codec no GPU decodes (FFV1) still gets every frame: from the CPU.
func TestTrickplayGPUFallsBackToCPU(t *testing.T) {
	g := cudaDecoder(t)
	clip := filepath.Join(t.TempDir(), "clip.mkv")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=60:size=320x180:rate=24",
		"-c:v", "ffv1", "-g", "24", "-y", clip).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	old, oldState := config.GlobalConfig, gpuDecode
	defer func() { config.GlobalConfig, gpuDecode = old, oldState }()
	gpuDecode = &gpuDecodeState{pixels: map[string]int64{}}
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.TrickplayPath = t.TempDir()
	defer func() { chooseTrickplayDecoder = trickplayDecoder }()
	chooseTrickplayDecoder = func() *gpuDecoder { return g }

	tg := &TrickplayGenerator{}
	if err := tg.generate(context.Background(), clip, nil); err != nil {
		t.Fatal(err)
	}
	frames, _ := filepath.Glob(filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(clip), "frame-*.jpg"))
	if len(frames) != 6 {
		t.Errorf("%d frames, want 6", len(frames))
	}
	if gpuDecode.pixels[g.key()] != 0 {
		t.Error("a video the GPU can't decode was taken for a memory limit")
	}
	if gpuDecode.failed != 1 {
		t.Errorf("the GPU's failures: %d, want 1 (this video)", gpuDecode.failed)
	}
}

// lastField is what follows the last marker in text ("" when there's none).
func lastField(text, marker string) string {
	for i := len(text) - len(marker); i >= 0; i-- {
		if text[i:i+len(marker)] == marker {
			return text[i+len(marker):]
		}
	}
	return ""
}
