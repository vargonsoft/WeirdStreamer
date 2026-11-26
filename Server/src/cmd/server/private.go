package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/fileio"
)

// keepDataPrivate makes the server's data readable by its own account only: the databases
// (password hashes, everyone's viewing), logs (who signed in from where), thumbnails, trickplay,
// subtitles, encodes and work files. New files are private through the umask (privateUmask);
// this closes up what an older server, or another tool, left open. The folders are closed
// (0700), which hides everything in them without visiting each file; the databases' own files
// are 0600. A folder that is, or holds, a library is left alone: libraries aren't the server's.
// On Windows modes don't do this: an ACL does (fileio.OwnerOnly), and as there's no umask, new
// files take their folder's: so there the databases' folders (their WAL files come later), the
// backups' and config.yml (its secrets) are closed too.
func keepDataPrivate(c *config.Config, configPath string) {
	var dbDirs []string
	for _, db := range []string{c.Database.UserPath, c.Database.MediaPath} {
		if db == "" {
			continue
		}
		for _, f := range []string{db, db + "-wal", db + "-shm"} {
			makePrivate(f, 0o600, false)
		}
		dbDirs = append(dbDirs, filepath.Dir(db))
	}
	if c.Server.KeyFile != "" {
		makePrivate(c.Server.KeyFile, 0o600, false) // the TLS private key
	}
	m := c.Media
	dirs := []string{c.Server.LogsPath, m.ThumbnailPath, m.TrickplayPath, m.ArtworkPath,
		m.DownloadPath, m.TranscodeCachePath, m.SubtitlePath, m.TempPath, m.RepairPath, m.TrashPath, m.ThemePath}
	if runtime.GOOS == "windows" {
		dirs = append(append(dirs, dbDirs...), c.Database.BackupDir())
		if configPath != "" {
			makePrivate(configPath, 0o600, false)
		}
	}
	for _, dir := range dirs {
		if dir == "" || holdsLibrary(c, dir) {
			continue
		}
		makePrivate(dir, 0o700, true)
	}
}

// makePrivate gives path mode, or on Windows its owner-only ACL, if it's there (a folder if dir).
func makePrivate(path string, mode os.FileMode, dir bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() != dir {
		return
	}
	if runtime.GOOS == "windows" {
		err = fileio.OwnerOnly(path)
	} else if info.Mode().Perm() != mode {
		err = os.Chmod(path, mode)
	}
	if err != nil {
		log.Printf("Warning: could not make %s private: %v", path, err)
	}
}

// holdsLibrary reports whether dir is a library's folder, or one inside or above it.
func holdsLibrary(c *config.Config, dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	for _, lib := range c.Media.Paths {
		libAbs, err := filepath.Abs(lib.Path)
		if err != nil {
			return true
		}
		if within(abs, libAbs) || within(libAbs, abs) {
			return true
		}
	}
	return false
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
