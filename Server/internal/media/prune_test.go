package media

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

func TestPruneForgetsWhatIsGone(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	movies, shows, unmounted := filepath.Join(dir, "movies"), filepath.Join(dir, "shows"), filepath.Join(dir, "share")
	for _, d := range []string{movies, shows} {
		os.MkdirAll(d, 0o755)
	}
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{Paths: []config.MediaPath{
		{Name: "Movies", Path: movies}, {Name: "Shows", Path: shows}, {Name: "Share", Path: unmounted},
	}}}
	add := func(path string, onDisk bool) {
		if onDisk {
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, []byte("x"), 0o644)
		}
		if err := db.SaveMediaItem(&models.MediaItem{Path: path, Name: filepath.Base(path), Type: "video", MimeType: "video/mp4"}); err != nil {
			t.Fatal(err)
		}
	}
	add(filepath.Join(movies, "Kept.mkv"), true)
	add(filepath.Join(movies, "Deleted.mkv"), false)        // gone from disk
	add(filepath.Join(dir, "old library", "Old.mkv"), true) // in no library any more
	add(filepath.Join(unmounted, "Offline.mkv"), false)     // its library can't be read: kept
	for i := 0; i < 30; i++ {                               // most of a library missing at once: kept
		add(filepath.Join(shows, "Show", "E"+string(rune('a'+i))+".mkv"), i < 5)
	}
	db.SaveAnalysis(filepath.Join(movies, "Deleted.mkv"), &database.Span{Start: 1, End: 2, Source: "chapters"}, nil, 1)

	n, err := NewPruner(db).Prune(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("forgot %d, want 2", n)
	}
	records, _ := db.MediaRecords()
	var left []string
	for _, r := range records {
		rel, _ := filepath.Rel(dir, r.Path)
		left = append(left, rel)
	}
	sort.Strings(left)
	for _, gone := range []string{"movies/Deleted.mkv", "old library/Old.mkv"} {
		for _, l := range left {
			if l == gone {
				t.Errorf("%s still recorded", gone)
			}
		}
	}
	if len(left) != 32 {
		t.Errorf("%d left, want 32 (Kept, Offline, the 30 episodes)", len(left))
	}
	if s, _ := db.GetSegments(filepath.Join(movies, "Deleted.mkv")); s.Intro != nil || s.Analyzed != 0 {
		t.Errorf("its segments are still there: %+v", s)
	}

	// Forced: the 25 missing episodes go too, but the library that can't be read is still kept.
	if n, err := NewPruner(db).Prune(context.Background(), true); err != nil || n != 25 {
		t.Errorf("forced: forgot %d (%v), want 25", n, err)
	}
	if records, _ := db.MediaRecords(); len(records) != 7 {
		t.Errorf("forced: %d left, want 7 (Kept, Offline, 5 episodes)", len(records))
	}
}

func TestPruneForgetsMacFiles(t *testing.T) {
	for _, p := range []string{"/m/.DS_Store", "/m/._Movie.mkv", "/m/Movie (2013)/.AppleDouble/Movie (2013).avi"} {
		if !IsMacFile(p) {
			t.Errorf("%s isn't taken for a Mac file", p)
		}
	}
	for _, p := range []string{"/m/Movie.mkv", "/m/.hidden/Movie.mkv", "/m/Movie_.mkv", "/m/AppleDouble/Movie.mkv"} {
		if IsMacFile(p) {
			t.Errorf("%s is taken for a Mac file", p)
		}
	}

	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	movies := filepath.Join(dir, "movies")
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{Paths: []config.MediaPath{{Name: "Movies", Path: movies}}}}
	for _, p := range []string{"Movie.mkv", "._Movie.mkv", ".AppleDouble/Movie.mkv"} {
		path := filepath.Join(movies, p)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte("x"), 0o644) // all still on disk
		if err := db.SaveMediaItem(&models.MediaItem{Path: path, Name: filepath.Base(path), Type: "video", MimeType: "video/mp4"}); err != nil {
			t.Fatal(err)
		}
	}

	if n, _ := NewPruner(db).Prune(context.Background(), false); n != 0 {
		t.Errorf("forgot %d with Mac files not ignored, want 0", n)
	}
	config.GlobalConfig.Media.IgnoreMacFiles = true
	if n, _ := NewPruner(db).Prune(context.Background(), false); n != 2 {
		t.Errorf("forgot %d, want the 2 Mac files", n)
	}
	if records, _ := db.MediaRecords(); len(records) != 1 || filepath.Base(records[0].Path) != "Movie.mkv" || filepath.Dir(records[0].Path) != movies {
		t.Errorf("left %+v, want just Movie.mkv", records)
	}
}
