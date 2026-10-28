package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestTrickplayTruncatedFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 400 seconds with the index at the front, then cut to 60% of its size: the index still lists
	// 400 seconds, but the data stops at about 240 (more than 10 frames short).
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10",
		"-t", "400", "-c:v", "libx264", "-g", "10", "-movflags", "+faststart", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(src)
	if err := os.Truncate(src, st.Size()*6/10); err != nil {
		t.Fatal(err)
	}

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	if tg.truncatedAt(src, 50) {
		t.Error("the data at 50s was taken for past the end")
	}
	if !tg.truncatedAt(src, 390) {
		t.Error("390s wasn't seen as past the end")
	}
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("the truncated file wasn't marked done")
	}
	if _, err := os.Stat(filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(src), "frame-200.jpg")); err != nil {
		t.Error("the frames before the cut weren't kept")
	}
}

func TestTrickplayStopsAtTheVideosEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 30 seconds of video with 300 of audio: the file's length is the audio's (MP4 and AVI list
	// each stream's length too; MKV doesn't, so it keeps the file's).
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=30",
		"-f", "lavfi", "-i", "sine=duration=300", "-c:v", "libx264", "-c:a", "aac", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	if d, codec, err := tg.getVideoInfo(src); err != nil || d > 31 || codec != "h264" {
		t.Errorf("video info: %.1fs %q %v, want about 30s of h264", d, codec, err)
	}
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("not marked done")
	}
}
