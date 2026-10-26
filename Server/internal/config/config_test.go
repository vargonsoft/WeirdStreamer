package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadJSON(t *testing.T, body string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLibraryType(t *testing.T) {
	cases := []struct {
		name    string
		typeVal string
		wantErr string
	}{
		{"none", ``, ""},
		{"movie", `, "type": "movie"`, ""},
		{"tv", `, "type": "tv"`, ""},
		{"unknown", `, "type": "music"`, `invalid type "music"`},
		{"case-sensitive", `, "type": "TV"`, `invalid type "TV"`},
		{"only one", `, "type": ["movie", "tv"]`, "cannot unmarshal"},
	}
	for _, c := range cases {
		err := loadJSON(t, `{"media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["a"]`+c.typeVal+`}]}}`)
		if c.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: error %v, want containing %q", c.name, err, c.wantErr)
		}
	}
}

func TestContinueWatchingMode(t *testing.T) {
	c := &Config{Media: MediaConfig{Paths: []MediaPath{
		{Name: "a"}, {Name: "b", ContinueWatchingMode: "LibraryOnly"}, {Name: "c", ContinueWatchingMode: "OFF"},
	}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{ContinueOn, ContinueLibraryOnly, ContinueOff}
	for i, p := range c.Media.Paths {
		if p.ContinueWatchingMode != want[i] {
			t.Errorf("%s: mode %q, want %q (any case, written back canonically; empty is on)", p.Name, p.ContinueWatchingMode, want[i])
		}
	}
	// Where each shows: the home screen's list, then the library's own page's
	for i, shows := range [][2]bool{{true, true}, {false, true}, {false, false}} {
		p := c.Media.Paths[i]
		if p.OnContinueWatching(false) != shows[0] || p.OnContinueWatching(true) != shows[1] {
			t.Errorf("%s (%s): home %v, library page %v; want %v, %v", p.Name, p.ContinueWatchingMode,
				p.OnContinueWatching(false), p.OnContinueWatching(true), shows[0], shows[1])
		}
	}
	bad := &Config{Media: MediaConfig{Paths: []MediaPath{{Name: "x", ContinueWatchingMode: "sometimes"}}}}
	if err := bad.Validate(); err == nil {
		t.Error("an unknown mode: want an error")
	}
}
