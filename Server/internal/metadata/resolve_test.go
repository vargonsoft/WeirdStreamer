package metadata

import (
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestResolveFileTV(t *testing.T) {
	root := "/media/tv"
	lib := config.MediaPath{Path: root, ID: "t", Type: config.LibraryTypeTV}
	cases := []struct {
		path            string
		key             string
		season, episode int
	}{
		{"1923/Season 1/1923 - S01E02 - Nature's Empty Throne.mkv", "1923", 1, 2},
		// A whole season in one folder, at the library root or inside the show folder
		{"Breaking.Bad.S01.1080p.BluRay/Breaking.Bad.S01E02.720p.mkv", "breaking bad", 1, 2},
		{"The Wire/The Wire - S02E05.mkv", "the wire", 2, 5},
		// Loose episode files in the library root
		{"Babylon 5 - S01E01 - Midnight on the Firing Line.mkv", "babylon 5", 1, 1},
		// A folder mixing shows: each file names its own show
		{"Mixed/1923 - S01E03.mkv", "1923", 1, 3},
		{"Mixed/Babylon 5 - S02E01.mkv", "babylon 5", 2, 1},
		// The folder adds its year or country tag when it names the same show
		{"Bluey (2018)/Season 1/Bluey - S01E01.mkv", "bluey|2018", 1, 1},
		{"Big Brother (UK)/Big Brother - S01E01.mkv", "big brother|gb", 1, 1},
		// The more specific of the two names wins, keeping the other's year/country
		{"Big Brother Sweden/big brother - S07E01.mkv", "big brother sweden", 7, 1},
		{"Example Show/Example Show - Example Episode - S01E01.mkv", "example show example episode", 1, 1},
		{"Big Brother 7 UK/Big Brother - S07E01.mkv", "big brother 7|gb", 7, 1},
		// A country the file names beats the folder's
		{"Big Brother (UK)/Big Brother US - S01E01.mkv", "big brother|us", 1, 1},
		// Files that don't name a show use the folder
		{"The Walking Dead/Season 4/S04E13 - Alone.mkv", "the walking dead", 4, 13},
	}
	for _, c := range cases {
		ref, ok := ResolveFile(lib, filepath.Join(root, c.path))
		if !ok || ref.Key != c.key || ref.Season != c.season || ref.Episode != c.episode || !ref.IsEpisode {
			t.Errorf("ResolveFile(%q) = %+v, %v; want key %q S%02dE%02d", c.path, ref, ok, c.key, c.season, c.episode)
		}
	}
	if _, ok := ResolveFile(lib, filepath.Join(root, "notes.mkv")); ok {
		t.Error("a root file without a show or episode resolved")
	}

	// The fallback is the name the primary lookup didn't use.
	file := filepath.Join(root, "Mixed/Babylon 5 - S02E01.mkv")
	ref, _ := ResolveFile(lib, file)
	if alt, ok := fallbackFor(lib, file, ref); !ok || alt.Title != "Mixed" {
		t.Errorf("fallback for mixed-folder file = %+v, %v", alt, ok)
	}
	file = filepath.Join(root, "Big Brother 7 UK/Big Brother - S07E01.mkv")
	ref, _ = ResolveFile(lib, file)
	if alt, ok := fallbackFor(lib, file, ref); !ok || alt.Key != "big brother|gb" {
		t.Errorf("fallback for more specific folder = %+v, %v; want the file's show with the folder's country", alt, ok)
	}
	file = filepath.Join(root, "TWD/S04E13 - Alone.mkv")
	ref, _ = ResolveFile(lib, file)
	if _, ok := fallbackFor(lib, file, ref); ok {
		t.Error("file with no show name should have no fallback")
	}
}

func TestResolveFileMovie(t *testing.T) {
	root := "/media/movies"
	lib := config.MediaPath{Path: root, ID: "m", Type: config.LibraryTypeMovie}
	cases := map[string]string{
		// The file name is the source of truth; the folder only supplies a missing year
		"Inception (2010)/Inception.mkv":                                      "inception|2010",
		"Collection/Inception (2010).mkv":                                     "inception|2010",
		"Collection/A Knight's Tale (2001).mkv":                               "a knights tale|2001",
		"Cherry 2000 (1987)/Cherry.2000.1987.1080p.BluRay.x264-GUACAMOLE.mkv": "cherry 2000|1987",
		"The Martian (2015)/ETRG.mp4":                                         "etrg",
		"Loose File (1999).mkv":                                               "loose file|1999",
		"Some Folder (2005)/A Different Movie.mkv":                            "a different movie",
	}
	for path, want := range cases {
		if ref, ok := ResolveFile(lib, filepath.Join(root, path)); !ok || ref.Key != want {
			t.Errorf("ResolveFile(%q) key = %q, want %q", path, ref.Key, want)
		}
	}
	file := filepath.Join(root, "The Martian (2015)/ETRG.mp4")
	ref, _ := ResolveFile(lib, file)
	if alt, ok := fallbackFor(lib, file, ref); !ok || alt.Title != "The Martian" || alt.Year != 2015 {
		t.Errorf("folder fallback = %+v, %v", alt, ok)
	}
}
