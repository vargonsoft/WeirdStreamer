package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

// A file whose thumbnail fails isn't tried again until it changes or is forgotten: browsing its
// folder retried (and logged) every one of them each time.
func TestThumbnailFailureRemembered(t *testing.T) {
	dir := t.TempDir()
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{ThumbnailPath: filepath.Join(dir, "thumbs")}}
	video := filepath.Join(dir, "broken.mkv")
	if err := os.WriteFile(video, []byte("not a video"), 0644); err != nil {
		t.Fatal(err)
	}
	tg := NewThumbnailGenerator()
	if _, err := tg.GenerateThumbnail(video); err == nil || errors.Is(err, ErrThumbnailFailedBefore) {
		t.Fatalf("first try: %v, want a real failure", err)
	}
	if _, err := tg.GenerateThumbnail(video); !errors.Is(err, ErrThumbnailFailedBefore) {
		t.Fatalf("second try: %v, want ErrThumbnailFailedBefore", err)
	}
	tg.Forget(video)
	if _, err := tg.GenerateThumbnail(video); errors.Is(err, ErrThumbnailFailedBefore) {
		t.Fatal("tried again after Forget: still skipped")
	}
	if err := os.WriteFile(video, []byte("not a video, longer"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := tg.GenerateThumbnail(video); errors.Is(err, ErrThumbnailFailedBefore) {
		t.Fatal("tried again after the file changed: still skipped")
	}
}
