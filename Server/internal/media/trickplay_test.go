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

func TestTrickplayEmptiedFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "clip.mp4")
	// 400 seconds, then zeros from 40% of the way in to the end (a download that never finished
	// filling the file it set aside): the size and index are whole, the data past 160s isn't.
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10",
		"-t", "400", "-c:v", "libx264", "-g", "10", "-movflags", "+faststart", "-y", src).CombinedOutput(); err != nil {
		t.Fatalf("make clip: %v %s", err, out)
	}
	st, _ := os.Stat(src)
	f, err := os.OpenFile(src, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteAt(make([]byte, st.Size()-st.Size()*4/10), st.Size()*4/10)
	f.Close()

	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{TrickplayPath: filepath.Join(dir, "trickplay")}}

	tg := NewTrickplayGenerator(nil)
	before, _ := os.Stat(src)
	if !tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a frame at 50s didn't come out")
	}
	if err := tg.generate(context.Background(), src, nil); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !HasTrickplay(src) {
		t.Error("the emptied file wasn't marked done")
	}

	// A file that changed since its frames were started (or that's gone) isn't taken for damaged.
	if err := os.Truncate(src, before.Size()/2); err != nil {
		t.Fatal(err)
	}
	if tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a changed file was taken for the same one")
	}
	os.Remove(src)
	if tg.stillReadable(context.Background(), src, before, 50, "h264") {
		t.Error("a missing file was taken for readable")
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
