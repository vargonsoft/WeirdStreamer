package media

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/models"
)

func TestPairLivePhotos(t *testing.T) {
	item := func(name, typ string) models.MediaItem {
		return models.MediaItem{Name: name, Path: "/photos/" + name, Type: typ}
	}
	notLive := map[string]bool{"/photos/IMG_0005.MOV": true}
	got := pairLivePhotos([]models.MediaItem{
		item("IMG_0001.HEIC", "image"),
		item("IMG_0001.MOV", "video"),
		item("img_0002.jpg", "image"),
		item("IMG_0002.mov", "video"),
		item("IMG_0003.PNG", "image"), // not a Live Photo's still: its video stays
		item("IMG_0003.MOV", "video"),
		item("Holiday.mov", "video"), // no photo of that name
		item("IMG_0004.HEIC", "image"),
		item("IMG_0005.JPG", "image"), // a video beside a photo of its name, but not a Live Photo's
		item("IMG_0005.MOV", "video"),
	}, func(path string) bool { return !notLive[path] })
	want := []struct{ name, live string }{
		{"IMG_0001.HEIC", "/photos/IMG_0001.MOV"},
		{"img_0002.jpg", "/photos/IMG_0002.mov"},
		{"IMG_0003.PNG", ""},
		{"IMG_0003.MOV", ""},
		{"Holiday.mov", ""},
		{"IMG_0004.HEIC", ""},
		{"IMG_0005.JPG", ""},
		{"IMG_0005.MOV", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w.name || got[i].LiveVideo != w.live {
			t.Errorf("item %d = %s (live %q), want %s (live %q)", i, got[i].Name, got[i].LiveVideo, w.name, w.live)
		}
	}
}

// TestIsLivePhotoVideo: a clip with Apple's content identifier in its moov box is a Live Photo's,
// with the moov before or after the media; one without it isn't.
func TestIsLivePhotoVideo(t *testing.T) {
	box := func(typ string, body []byte) []byte {
		b := make([]byte, 8, 8+len(body))
		binary.BigEndian.PutUint32(b, uint32(8+len(body)))
		copy(b[4:], typ)
		return append(b, body...)
	}
	ftyp := box("ftyp", []byte("qt  \x00\x00\x00\x00qt  "))
	mdat := box("mdat", bytes.Repeat([]byte{0xAB}, 5000))
	liveMoov := box("moov", box("meta", append(box("keys", []byte("mdta"+string(livePhotoKey))), box("ilst", []byte("ABCD-1234"))...)))
	plainMoov := box("moov", box("trak", []byte("nothing here")))
	dir := t.TempDir()
	for name, parts := range map[string][][]byte{
		"start.mov": {ftyp, liveMoov, mdat},
		"end.mov":   {ftyp, mdat, liveMoov},
		"plain.mov": {ftyp, mdat, plainMoov},
		"junk.mov":  {[]byte("not a QuickTime file at all")},
	} {
		if err := os.WriteFile(filepath.Join(dir, name), bytes.Join(parts, nil), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]bool{"start.mov": true, "end.mov": true, "plain.mov": false, "junk.mov": false, "missing.mov": false} {
		if got := IsLivePhotoVideo(filepath.Join(dir, name)); got != want {
			t.Errorf("IsLivePhotoVideo(%s) = %v, want %v", name, got, want)
		}
	}
	// The .MOV half of a Live Photo needs its photo beside it, too.
	for _, photo := range []string{"start.HEIC", "plain.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, photo), []byte("photo"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]bool{"start.mov": true, "end.mov": false, "plain.mov": false} {
		if got := IsLivePhotoPartner(filepath.Join(dir, name)); got != want {
			t.Errorf("IsLivePhotoPartner(%s) = %v, want %v", name, got, want)
		}
	}
}
