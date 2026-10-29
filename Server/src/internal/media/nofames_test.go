package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestNoFramesMade(t *testing.T) {
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.TrickplayPath = t.TempDir()
	video := filepath.Join(t.TempDir(), "not-a-video.ts")
	if noFramesMade(video) {
		t.Fatal("a video with no marker counted as not a video")
	}
	if err := markNoFrames(video); err != nil {
		t.Fatal(err)
	}
	if !noFramesMade(video) {
		t.Fatal("a file marked not a video wasn't recognised")
	}
	dir := filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(video))
	if err := os.WriteFile(filepath.Join(dir, "frame-0.jpg"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if noFramesMade(video) {
		t.Fatal("a video with frames counted as not a video")
	}
}
