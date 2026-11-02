package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/models"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func saveVideo(t *testing.T, db *DB, path string) {
	t.Helper()
	item := &models.MediaItem{Path: path, Name: filepath.Base(path), Type: "video", MimeType: "video/mp4"}
	if err := db.SaveMediaItem(item); err != nil {
		t.Fatal(err)
	}
}

func TestMediaAdded(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	old := filepath.Join(dir, "lib", "old.mp4")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	onDisk := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, onDisk, onDisk); err != nil {
		t.Fatal(err)
	}
	saveVideo(t, db, old) // already known when arrival times start being kept
	if err := db.FillMediaAdded(); err != nil {
		t.Fatal(err)
	}

	// A file found now is dated now, whatever its time on disk says.
	found := filepath.Join(dir, "lib", "found.mp4")
	if err := os.WriteFile(found, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(found, onDisk.Add(-time.Hour), onDisk.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	saveVideo(t, db, found)
	saveVideo(t, db, filepath.Join(dir, "library2", "other.mp4")) // another library, its name a prefix

	got, err := db.NewestMedia(filepath.Join(dir, "lib")+"/", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != found || got[1].Path != old {
		t.Fatalf("got %+v", got)
	}
	if got[1].AddedAt != onDisk.Unix() {
		t.Errorf("existing file dated %d, want its time on disk %d", got[1].AddedAt, onDisk.Unix())
	}
	if time.Since(time.Unix(got[0].AddedAt, 0)) > time.Minute {
		t.Errorf("new file dated %d, want now", got[0].AddedAt)
	}

	// Saving again (a scan, a thumbnail) doesn't move a file's date.
	first := got[0].AddedAt
	db.mediaConn.Exec(`UPDATE media_added SET added_at = ? WHERE path = ?`, first-500, found)
	saveVideo(t, db, found)
	got, _ = db.NewestMedia(filepath.Join(dir, "lib"), 1)
	if len(got) != 1 || got[0].AddedAt != first-500 {
		t.Errorf("a save moved the date: %+v", got)
	}

	if err := db.ForgetMedia([]string{found}); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.NewestMedia(filepath.Join(dir, "lib"), 10); len(got) != 1 || got[0].Path != old {
		t.Errorf("forgotten file still listed: %+v", got)
	}
}

func TestNewestMediaByDay(t *testing.T) {
	db := testDB(t)
	day := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)
	dates := map[string]time.Time{
		"/lib/S01E09.mkv": day.Add(time.Hour), // a day's files in order, whenever they arrived
		"/lib/S01E10.mkv": day.Add(-time.Hour),
		"/lib/S01E11.mkv": day.Add(-2 * time.Hour),
		"/lib/S01E01.mkv": day.Add(-7 * 24 * time.Hour),
	}
	for path, at := range dates {
		saveVideo(t, db, path)
		if err := recordAdded(db.mediaConn, path, at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.NewestMedia("/lib", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/lib/S01E09.mkv", "/lib/S01E10.mkv", "/lib/S01E11.mkv", "/lib/S01E01.mkv"}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Errorf("%d: %s, want %s", i, got[i].Path, want[i])
		}
	}
}
