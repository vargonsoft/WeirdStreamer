package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadConfig loads body (JSON is YAML too, so the cases can stay on one line).
func loadConfig(t *testing.T, body string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
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
		err := loadConfig(t, `{"groups": ["a"], "media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["a"]`+c.typeVal+`}]}}`)
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

func TestShowNewAdditions(t *testing.T) {
	cfg := &Config{Media: MediaConfig{Paths: []MediaPath{
		{Name: "a"}, {Name: "b", ShowNewAdditions: "ON"}, {Name: "c", ShowNewAdditions: "LibraryOnly"}, {Name: "d", ShowNewAdditions: "off"},
	}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		mode          string
		home, library bool
	}{{"", false, false}, {ContinueOn, true, true}, {ContinueLibraryOnly, false, true}, {ContinueOff, false, false}}
	for i, p := range cfg.Media.Paths {
		if p.ShowNewAdditions != want[i].mode || p.OnNewAdditions(false) != want[i].home || p.OnNewAdditions(true) != want[i].library {
			t.Errorf("%s: %q, home %v, library %v; want %+v", p.Name, p.ShowNewAdditions, p.OnNewAdditions(false), p.OnNewAdditions(true), want[i])
		}
	}
	bad := &Config{Media: MediaConfig{Paths: []MediaPath{{Name: "x", ShowNewAdditions: "sometimes"}}}}
	if err := bad.Validate(); err == nil {
		t.Error("an invalid showNewAdditions passed")
	}
}

func TestContinueWatchingMode(t *testing.T) {
	c := &Config{Media: MediaConfig{Paths: []MediaPath{
		{Name: "a"}, {Name: "b", ContinueWatchingMode: "LibraryOnly"}, {Name: "c", ContinueWatchingMode: "OFF"},
	}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []string{"", ContinueLibraryOnly, ContinueOff} // empty stays empty, meaning on
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

func TestGroups(t *testing.T) {
	ok := `{"groups": ["adults", "kids"], "media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["adults", "kids"]}]}}`
	if err := loadConfig(t, ok); err != nil {
		t.Fatalf("defined groups: %v", err)
	}
	if !GlobalConfig.HasGroup("kids") || GlobalConfig.HasGroup("Kids") || GlobalConfig.HasGroup("") {
		t.Error("HasGroup: the exact names only")
	}
	for name, body := range map[string]string{
		"undefined": `{"groups": ["adults"], "media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["adluts"]}]}}`,
		"twice":     `{"groups": ["adults", "Adults"]}`,
		"empty":     `{"groups": [""]}`,
		"comma":     `{"groups": ["a,b"]}`,
		"spaces":    `{"groups": [" kids"]}`,
		"no groups": `{"media": {"paths": [{"name": "Lib", "path": "/x", "groups": ["adults"]}]}}`,
	} {
		if err := loadConfig(t, body); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	c := &Config{Groups: []string{"adults"}}
	if c.CheckGroupName("teens") != nil || c.CheckGroupName("ADULTS") == nil {
		t.Error("CheckGroupName: a new name is fine, an existing one in any case isn't")
	}
}

func TestTLSFilesDefaultAndExplicit(t *testing.T) {
	cert, key := ServerConfig{}.TLSFiles()
	if cert != "cert.pem" || key != "key.pem" {
		t.Errorf("defaults = %q, %q", cert, key)
	}
	cert, key = ServerConfig{CertFile: "/etc/ws/fullchain.pem", KeyFile: "/etc/ws/privkey.pem"}.TLSFiles()
	if cert != "/etc/ws/fullchain.pem" || key != "/etc/ws/privkey.pem" {
		t.Errorf("explicit = %q, %q", cert, key)
	}
}

// Saving rewrites every field, so a field the config struct doesn't name would be dropped.
func TestSaveKeepsLogsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := Save(path, &Config{Server: ServerConfig{LogsPath: "./var/logs"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `logsPath: ./var/logs`) {
		t.Errorf("saved config lost logsPath: %s", data)
	}
}
