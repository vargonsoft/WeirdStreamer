package handlers

import (
	"os"
	"path/filepath"
	"reflect"
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

	got, err := listFolder(dir + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != dir || got.Parent != filepath.Dir(dir) {
		t.Errorf("path %q parent %q", got.Path, got.Parent)
	}
	if want := []string{"anime", "linked", "movies", "TV Shows"}; !reflect.DeepEqual(got.Folders, want) {
		t.Errorf("folders %v, want %v", got.Folders, want)
	}
	if got.Videos != 2 {
		t.Errorf("videos %d, want 2", got.Videos)
	}

	if top, err := listFolder(""); err != nil || top.Path != "/" || top.Parent != "" {
		t.Errorf("top: %+v, %v", top, err)
	}
	for _, bad := range []string{"relative/path", filepath.Join(dir, "a.mkv"), filepath.Join(dir, "missing")} {
		if _, err := listFolder(bad); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}
