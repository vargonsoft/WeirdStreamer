package media

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

// makeClip writes a 300-second test pattern (30 frames, two batches) and returns its path.
func makeClip(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=300:size=320x180:rate=24",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-y", path).CombinedOutput()
	if err != nil {
		t.Fatalf("making the clip: %v: %s", err, out)
	}
	return path
}

// TestBatchedFramesMatchPerFrame checks that frames made in batches are the frames one process
// per frame makes, byte for byte.
func TestBatchedFramesMatchPerFrame(t *testing.T) {
	clip := makeClip(t)
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.TrickplayPath = t.TempDir()
	ctx := context.Background()
	tg := &TrickplayGenerator{}

	duration, codec, err := tg.getVideoInfo(clip)
	if err != nil {
		t.Fatal(err)
	}
	if err := tg.generate(ctx, clip, nil); err != nil {
		t.Fatalf("batched generate: %v", err)
	}
	dir := filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(clip))
	ref := t.TempDir()
	for i := 0; i <= int(duration)-1; i += 10 {
		if !tg.extractFrame(ctx, clip, filepath.Join(ref, fmt.Sprintf("frame-%d.jpg", i)), float64(i), codec) {
			t.Fatalf("per-frame reference missing at %ds", i)
		}
		want, _ := os.ReadFile(filepath.Join(ref, fmt.Sprintf("frame-%d.jpg", i)))
		got, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", i)))
		if err != nil {
			t.Fatalf("batched frame at %ds: %v", i, err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("frame at %ds differs from the per-frame one", i)
		}
	}
	if !HasTrickplay(clip) {
		t.Error("video not marked done after the batches")
	}
}

// TestBatchLeavesNoParts checks that a damaged copy (cut short, so later frames can't be made)
// leaves no partly written frames behind, and keeps the frames that did come out.
func TestBatchLeavesNoParts(t *testing.T) {
	clip := makeClip(t)
	data, err := os.ReadFile(clip)
	if err != nil {
		t.Fatal(err)
	}
	cut := filepath.Join(t.TempDir(), "cut.mp4")
	if err := os.WriteFile(cut, data[:len(data)/3], 0644); err != nil {
		t.Fatal(err)
	}
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.TrickplayPath = t.TempDir()
	tg := &TrickplayGenerator{}
	_ = tg.generate(context.Background(), cut, nil) // may give up on it; what matters is the files
	dir := filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(cut))
	entries, _ := os.ReadDir(dir)
	made := 0
	for _, e := range entries {
		if bytes.Contains([]byte(e.Name()), []byte(".part")) {
			t.Errorf("partly written frame left behind: %s", e.Name())
		}
		if e.Name() != trickplayDone && bytes.HasPrefix([]byte(e.Name()), []byte("frame-")) {
			made++
		}
	}
	if made == 0 {
		t.Error("no frames came out of the part of the copy that is there")
	}
}
