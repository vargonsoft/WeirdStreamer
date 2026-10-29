package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

func TestStorage(t *testing.T) {
	dir := t.TempDir()
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{
		Database: config.DatabaseConfig{UserPath: filepath.Join(dir, "users.db"), MediaPath: filepath.Join(dir, "media.db")},
		Media: config.MediaConfig{ThumbnailPath: filepath.Join(dir, "thumbs"), TrickplayPath: filepath.Join(dir, "trickplay"),
			ArtworkPath: filepath.Join(dir, "artwork")},
	}
	write := func(path string, n int) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "media.db"), 1000)
	write(filepath.Join(dir, "media.db-wal"), 200)
	write(filepath.Join(dir, "thumbs", "a", "b", "x.jpg"), 300)
	video := filepath.Join(dir, "v.mkv")
	frames := filepath.Join(dir, "trickplay", GenerateThumbnailHash(video))
	write(filepath.Join(frames, "frame-0.jpg"), 50)
	write(filepath.Join(frames, "frame-10.jpg"), 50)
	CountStorage()

	sizes := func() map[string]int64 {
		uses, counted := Storage()
		if counted.IsZero() {
			t.Fatal("not counted")
		}
		m := map[string]int64{}
		for _, u := range uses {
			m[u.Name] = u.Bytes
		}
		if uses[2].Name != trickplayStorage {
			t.Errorf("order: %+v, want the frames after the thumbnails", uses)
		}
		return m
	}
	got := sizes()
	if got["Databases"] != 1200 || got["Thumbnails"] != 300 || got[trickplayStorage] != 100 || got["Artwork"] != 0 {
		t.Errorf("sizes = %v", got)
	}
	if _, ok := got["Subtitles"]; ok {
		t.Error("a folder that isn't configured is listed")
	}

	// A frame made counts at once; a forgotten video's frames come off at once.
	write(filepath.Join(frames, "frame-20.jpg"), 70)
	addFrame(filepath.Join(frames, "frame-20.jpg"))
	if got := sizes()[trickplayStorage]; got != 170 {
		t.Errorf("after a frame: %d, want 170", got)
	}
	removeGenerated(database.MediaRecord{Path: video})
	if got := sizes()[trickplayStorage]; got != 0 {
		t.Errorf("after removing the video's frames: %d, want 0", got)
	}

	// The other folders are counted again once storageEvery has passed (when asked).
	write(filepath.Join(dir, "artwork", "p.jpg"), 400)
	storage.mu.Lock()
	storage.counted = time.Now().Add(-storageEvery - time.Second)
	storage.mu.Unlock()
	Storage()
	deadline := time.Now().Add(5 * time.Second)
	for sizes()["Artwork"] != 400 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := sizes()["Artwork"]; got != 400 {
		t.Errorf("artwork after a recount: %d, want 400", got)
	}
}
