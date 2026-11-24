package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

// An unpacked DVD: its VIDEO_TS folder is a disc named after the folder it's in, sized as its files
// together, read whatever the case of its files' names; a VIDEO_TS folder without VIDEO_TS.IFO isn't.
func TestDVDFolder(t *testing.T) {
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{SupportedVideoFormats: []string{".mkv"}}}
	t.Cleanup(func() { config.GlobalConfig = saved })

	root := t.TempDir()
	disc := filepath.Join(root, "Frivolous Lola", "VIDEO_TS")
	os.MkdirAll(disc, 0o755)
	os.WriteFile(filepath.Join(disc, "video_ts.ifo"), []byte("not a real manager"), 0o644) // lower case, as some copies are
	os.WriteFile(filepath.Join(disc, "VTS_01_1.VOB"), make([]byte, 1000), 0o644)
	empty := filepath.Join(root, "Other", "VIDEO_TS")
	os.MkdirAll(empty, 0o755)

	if !DVDFolder(disc) || DVDFolder(empty) || DVDFolder(filepath.Dir(disc)) {
		t.Fatalf("DVDFolder: disc %v, empty %v, parent %v", DVDFolder(disc), DVDFolder(empty), DVDFolder(filepath.Dir(disc)))
	}
	if !IsDiscImage(disc) || config.MediaType(disc) != "video" {
		t.Fatalf("a disc %v, a video %q", IsDiscImage(disc), config.MediaType(disc))
	}
	if DiscName(disc) != "Frivolous Lola" || DiscName("/m/Yes Man.iso") != "Yes Man.iso" {
		t.Fatalf("names %q", DiscName(disc))
	}
	info, _ := os.Stat(disc)
	if DiscSize(disc, info) != 1000+int64(len("not a real manager")) {
		t.Fatalf("size %d", DiscSize(disc, info))
	}
	if CheckReadable(disc) != nil {
		t.Fatal("the folder's disc reads as unreadable")
	}
	if b, err := folderDisc(disc).readFile("VIDEO_TS", "VIDEO_TS.IFO"); err != nil || string(b) != "not a real manager" {
		t.Fatalf("reading its IFO: %q %v", b, err)
	}
	// The tables are read from the folder (this one's manager is fake, so it says so)
	if _, err := readDVDTables(disc); err == nil || !strings.Contains(err.Error(), "not a DVD video manager") {
		t.Fatalf("tables: %v", err)
	}
}
