package handlers

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestListFolder(t *testing.T) {
	old := config.GlobalConfig
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Media.SupportedVideoFormats = []string{".mkv", ".mp4"}
	defer func() { config.GlobalConfig = old }()

	dir := t.TempDir()
	for _, d := range []string{"movies", "TV Shows", ".hidden", "anime"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a.mkv", "b.MP4", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "movies"), filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}

	got, err := listFolder(dir + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir || got.Parent != filepath.Dir(dir) || got.Top {
		t.Errorf("path %q parent %q top %v", got.Path, got.Parent, got.Top)
	}
	var names []string
	for _, f := range got.Folders {
		names = append(names, f.Name)
		if f.Path != filepath.Join(dir, f.Name) {
			t.Errorf("%s's path %q", f.Name, f.Path)
		}
	}
	if want := []string{"anime", "linked", "movies", "TV Shows"}; !reflect.DeepEqual(names, want) {
		t.Errorf("folders %v, want %v", names, want)
	}
	if got.Videos != 2 {
		t.Errorf("videos %d, want 2", got.Videos)
	}

	// The top: / (nothing above it), or on Windows its drives, each a folder to open.
	top, err := listFolder("")
	if err != nil || !top.Top || top.Parent != "" {
		t.Fatalf("top: %+v, %v", top, err)
	}
	if runtime.GOOS == "windows" {
		if top.Path != "" || len(top.Folders) == 0 || top.Folders[0].Path != filepath.VolumeName(top.Folders[0].Path)+`\` {
			t.Errorf("drives: %+v", top)
		}
		if root, err := listFolder(`C:\`); err != nil || root.Top || root.Parent != "" {
			t.Errorf(`C:\: %+v, %v`, root, err)
		}
	} else if top.Path != "/" {
		t.Errorf("top: %+v", top)
	}

	// A library's own folder says so.
	config.GlobalConfig.Media.Paths = []config.MediaPath{{ID: "m", Name: "Movies", Path: filepath.Join(dir, "movies")}}
	if l, err := listFolder(filepath.Join(dir, "movies")); err != nil || l.Library != "Movies" {
		t.Errorf("library: %+v, %v", l, err)
	}
	for _, bad := range []string{"relative/path", filepath.Join(dir, "a.mkv"), filepath.Join(dir, "missing")} {
		if _, err := listFolder(bad); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}
