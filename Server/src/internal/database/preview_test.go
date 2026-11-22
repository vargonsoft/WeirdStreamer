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
	save("/lib/Photos/2020/A Trip/1.jpg", "image", "/t/trip.jpg")
	save("/lib/Photos/2020/z.jpg", "image", "/t/z.jpg")
	save("/lib/Photos/2020/b.jpg", "image", "") // no thumbnail yet
	save("/lib/Photos/2020/notes.pdf", "document", "/t/pdf.jpg")
	save("/lib/Photos/2020-old/a.jpg", "image", "/t/old.jpg") // a sibling sharing the prefix
	save("/lib/Photos/2021/Deep/x.mp4", "video", "/t/x.jpg")

	for dir, want := range map[string]string{
		"/lib/Photos/2020":  "/t/z.jpg", // its own file before its subfolder's
		"/lib/Photos/2020/": "/t/z.jpg",
		"/lib/Photos/2021":  "/t/x.jpg",   // a subfolder's when it has none of its own
		"/lib/Photos/2019":  "",           // nothing under it
		"/lib/Photos":       "/t/old.jpg", // the nearest: 2020-old/a.jpg is as deep as 2020/z.jpg and sorts first
	} {
		if got := db.FolderPreview(dir); got != want {
			t.Errorf("FolderPreview(%q) = %q, want %q", dir, got, want)
		}
	}
}
