package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestKeepDataPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no modes: TestKeepDataPrivateWindows")
	}
	dir := t.TempDir()
	mk := func(p string) string {
		p = filepath.Join(dir, p)
		os.MkdirAll(p, 0o755)
		os.Chmod(p, 0o755)
		return p
	}
	file := func(p string) string {
		p = filepath.Join(dir, p)
		os.WriteFile(p, []byte("x"), 0o644)
		os.Chmod(p, 0o644)
		return p
	}
	thumbs, logs, movies := mk("thumbnails"), mk("logs"), mk("Movies")
	trickInLibrary := mk("Movies/.trickplay")
	mk("data")
	users, wal := file("data/users.db"), file("data/users.db-wal")
	c := &config.Config{}
	c.Database.UserPath = users
	c.Server.LogsPath = logs
	c.Media.ThumbnailPath = thumbs
	c.Media.TrickplayPath = trickInLibrary
	c.Media.SubtitlePath = filepath.Join(dir, "missing")
	c.Media.Paths = []config.MediaPath{{Path: movies}}

	keepDataPrivate(c, "")

	for path, want := range map[string]os.FileMode{thumbs: 0o700, logs: 0o700, users: 0o600, wal: 0o600,
		trickInLibrary: 0o755, movies: 0o755} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s: %o, want %o", filepath.Base(path), got, want)
		}
	}
	if _, err := os.Stat(c.Media.SubtitlePath); err == nil {
		t.Error("a missing folder was created")
	}
}
