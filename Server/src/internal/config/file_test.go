package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const commented = `# Weird Streamer's settings
groups: ["parents", "kids"] # who may see what

server:
  # What the apps call it
  name: "Home"
  port: 9000 # HTTPS
  host: 0.0.0.0
  myNote: kept as it is # not a setting

media:
  paths:
    # The films
    - name: Movies
      path: /m
      id: aaaa1111
      groups: [parents]
    # The cartoons
    - name: Cartoons
      path: /c
      id: bbbb2222
      groups: [kids, parents]
    # The shows
    - name: Shows
      path: /s
      id: cccc3333
      groups: [parents]
  thumbnailPath: ./var/thumbnails

auth:
  jwtSecret: "0123" # quoted: it's a string
`

func TestSaveKeepsComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(commented), 0640); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	c := *GlobalConfig
	c.Server.Port = 9443
	c.Server.TrustedProxies = []string{"192.168.94.54"}
	c.Media.Paths = []MediaPath{c.Media.Paths[0], c.Media.Paths[2]} // Cartoons removed
	c.Media.Paths[1].Name = "TV"
	if err := Save(path, &c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"# Weird Streamer's settings", "# who may see what", "# What the apps call it",
		"port: 9443 # HTTPS", "myNote: kept as it is # not a setting", "# The films",
		"# The shows\n    - name: TV", `jwtSecret: "0123" # quoted`, "trustedProxies:",
		`name: "Home"`, "\n\nserver:\n", "\n\nmedia:\n", "\n\nauth:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("saved config lacks %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"Cartoons", "# The cartoons"} {
		if strings.Contains(got, gone) {
			t.Errorf("saved config still has %q:\n%s", gone, got)
		}
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0640 { // Windows has no Unix modes
		t.Errorf("mode %v, want 0640", st.Mode().Perm())
	}
	saved := *GlobalConfig
	if err := Load(path); err != nil {
		t.Fatalf("saved config doesn't load: %v", err)
	}
	if !reflect.DeepEqual(GlobalConfig.Media.Paths, c.Media.Paths) || GlobalConfig.Server.Port != 9443 ||
		!reflect.DeepEqual(GlobalConfig.Server.TrustedProxies, []string{"192.168.94.54"}) {
		t.Errorf("reloaded %+v, want %+v", GlobalConfig, saved)
	}
	// A library setting emptied is removed, with its line.
	c = *GlobalConfig
	c.Media.Paths = append([]MediaPath(nil), c.Media.Paths...)
	c.Media.Paths[0].Type = "movie"
	if err := Save(path, &c); err != nil {
		t.Fatal(err)
	}
	c.Media.Paths[0].Type = ""
	if err := Save(path, &c); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "type:") {
		t.Errorf("emptied setting kept:\n%s", data)
	}
}

func TestSaveFromTemplate(t *testing.T) {
	dir := t.TempDir()
	example := filepath.Join(dir, "config.example.yml")
	if err := os.WriteFile(example, []byte(commented), 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yml")
	c := &Config{Groups: []string{"admins"}, Server: ServerConfig{Name: "New", Port: 8080},
		Media: MediaConfig{Paths: []MediaPath{{Name: "Films", Path: "/f", Groups: []string{"admins"}}}}}
	if err := SaveFrom(example, path, c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	for _, want := range []string{"# Weird Streamer's settings", "# The films", "name: Films", "port: 8080 # HTTPS"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Shows") || !strings.Contains(got, "myNote") || !strings.Contains(got, `jwtSecret: "" # quoted`) {
		t.Errorf("kept the example's values:\n%s", got)
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Errorf("new config mode %v, want 0600", st.Mode().Perm())
	}
}

// Saving what was loaded changes nothing, however often: Settings saves the whole config.
func TestSaveUnchangedIsIdentical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	// As Load leaves it: ids filled in, defaults written.
	if err := os.WriteFile(path, []byte(commented), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, GlobalConfig); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	for range 3 {
		if err := Load(path); err != nil {
			t.Fatal(err)
		}
		if err := Save(path, GlobalConfig); err != nil {
			t.Fatal(err)
		}
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(first) {
		t.Errorf("saves drift:\n--- first\n%s\n--- after three more\n%s", first, again)
	}
	t.Logf("saved:\n%s", first)
}
