package media

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// A thumbnail's URL carries its version (?v=): pruning still finds its file, and keeps it while
// another record uses it.
func TestPruneVersionedThumbnail(t *testing.T) {
	dir := t.TempDir()
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.ThumbnailPath = dir
	t.Cleanup(func() { config.GlobalConfig = saved })

	file := filepath.Join(dir, "a", "b", "abc.jpg")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, []byte("jpg"), 0o644)
	if got := versionedThumbnailURL("/thumbnails/a/b/abc.jpg", file); ThumbnailFile(got) != "/thumbnails/a/b/abc.jpg" || got == "/thumbnails/a/b/abc.jpg" {
		t.Fatalf("versioned URL %q", got)
	}
	record := database.MediaRecord{Path: "/m/a.mkv", ThumbnailURL: "/thumbnails/a/b/abc.jpg?v=1700000000"}

	removeGenerated(record, map[string]bool{"/thumbnails/a/b/abc.jpg": true})
	if _, err := os.Stat(file); err != nil {
		t.Fatal("removed a thumbnail another record uses")
	}
	removeGenerated(record, nil)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("kept a forgotten file's thumbnail")
	}
}
