package database

import (
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/models"
)

func TestFolderPreview(t *testing.T) {
	db := testDB(t)
	save := func(path, kind, thumb string) {
		t.Helper()
		item := &models.MediaItem{Path: path, Name: filepath.Base(path), Type: kind, ThumbnailURL: thumb}
		if err := db.SaveMediaItem(item); err != nil {
			t.Fatal(err)
		}
	}
	save(filepath.FromSlash("/lib/Photos/2020/A Trip/1.jpg"), "image", "/t/trip.jpg")
	save(filepath.FromSlash("/lib/Photos/2020/z.jpg"), "image", "/t/z.jpg")
	save(filepath.FromSlash("/lib/Photos/2020/b.jpg"), "image", "") // no thumbnail yet
	save(filepath.FromSlash("/lib/Photos/2020/notes.pdf"), "document", "/t/pdf.jpg")
	save(filepath.FromSlash("/lib/Photos/2020-old/a.jpg"), "image", "/t/old.jpg") // a sibling sharing the prefix
	save(filepath.FromSlash("/lib/Photos/2021/Deep/x.mp4"), "video", "/t/x.jpg")

	for dir, want := range map[string]string{
		filepath.FromSlash("/lib/Photos/2020"):  "/t/z.jpg", // its own file before its subfolder's
		filepath.FromSlash("/lib/Photos/2020/"): "/t/z.jpg",
		filepath.FromSlash("/lib/Photos/2021"):  "/t/x.jpg",   // a subfolder's when it has none of its own
		filepath.FromSlash("/lib/Photos/2019"):  "",           // nothing under it
		filepath.FromSlash("/lib/Photos"):       "/t/old.jpg", // the nearest: 2020-old/a.jpg is as deep as 2020/z.jpg and sorts first
	} {
		if got := db.FolderPreview(dir); got != want {
			t.Errorf("FolderPreview(%q) = %q, want %q", dir, got, want)
		}
	}
}
