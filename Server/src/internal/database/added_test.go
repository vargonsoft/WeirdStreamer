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
	write := func(name string, at time.Time) string {
		t.Helper()
		path := filepath.Join(dir, "lib", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
		return path
	}
	onDisk := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	old := write("old.mp4", onDisk)
	// Found long after it arrived: dated by its time on disk, not by when the server saw it.
	found := write("found.mp4", onDisk.Add(time.Hour))
	saveVideo(t, db, old)
	saveVideo(t, db, found)
	saveVideo(t, db, filepath.Join(dir, "library2", "other.mp4")) // another library, its name a prefix

	got, err := db.NewestMedia(filepath.Join(dir, "lib")+"/", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != found || got[1].Path != old {
		t.Fatalf("got %+v", got)
	}
	if got[0].AddedAt != onDisk.Add(time.Hour).Unix() || got[1].AddedAt != onDisk.Unix() {
		t.Errorf("dated %d and %d, want their times on disk", got[0].AddedAt, got[1].AddedAt)
	}

	// A date that differs from the disk's (one dated by when the server first saw it) is
	// corrected, and a time on disk ahead of the clock counts as now.
	db.mediaConn.Exec(`UPDATE media_added SET added_at = ? WHERE path = ?`, time.Now().Unix(), old)
	future := write("future.mp4", time.Now().Add(48*time.Hour))
	saveVideo(t, db, future)
	db.mediaConn.Exec(`DELETE FROM media_added WHERE path = ?`, found)
	if err := db.DateMediaAdded(); err != nil {
		t.Fatal(err)
	}
	got, _ = db.NewestMedia(filepath.Join(dir, "lib"), 10)
	if len(got) != 3 || got[0].Path != future || got[1].AddedAt != onDisk.Add(time.Hour).Unix() || got[2].AddedAt != onDisk.Unix() {
		t.Fatalf("after DateMediaAdded: %+v", got)
	}
	if time.Since(time.Unix(got[0].AddedAt, 0)) > time.Minute {
		t.Errorf("a file from the future dated %d, want now", got[0].AddedAt)
	}

	if err := db.ForgetMedia([]GoneFile{{Path: found}, {Path: future}}); err != nil {
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
