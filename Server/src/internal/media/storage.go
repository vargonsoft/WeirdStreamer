package media

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// StorageUse is one of the server's data folders and its size, for the Jobs page.
type StorageUse struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// The data folders are counted when the Jobs page asks (nothing is counted while nobody looks):
// the small ones again after storageEvery, so what's added or removed shows within seconds. The
// trickplay frames (over a million files) are kept current as frames are made and removed, and
// counted again only after trickplayEvery, for what changed some other way (cli trickplay clean).
const (
	storageEvery   = 10 * time.Second
	trickplayEvery = 10 * time.Minute
)

const trickplayStorage = "Trickplay frames"

var storage struct {
	mu                       sync.Mutex
	uses                     []StorageUse // without the frames
	counted, framesCounted   time.Time
	counting, countingFrames bool
}

// CountStorage counts the data folders now (at startup, so the Jobs page has sizes at once).
func CountStorage() {
	countFolders()
	countFrames()
}

// Storage returns each data folder's size and when they were counted (zero before the first
// count), starting a count of what's due in the background.
func Storage() ([]StorageUse, time.Time) {
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if !storage.counting && time.Since(storage.counted) > storageEvery {
		storage.counting = true
		go countFolders()
	}
	if !storage.countingFrames && time.Since(storage.framesCounted) > trickplayEvery {
		storage.countingFrames = true
		go countFrames()
	}
	out := append([]StorageUse(nil), storage.uses...)
	if config.GlobalConfig.Media.TrickplayPath != "" && !storage.framesCounted.IsZero() {
		// After the thumbnails (or last, without them).
		at := len(out)
		for i, u := range out {
			if u.Name == "Thumbnails" {
				at = i + 1
			}
		}
		out = slices.Insert(out, at, StorageUse{Name: trickplayStorage, Bytes: trickplayBytes.Load()})
	}
	return out, storage.counted
}

// countFolders counts every data folder but the frames.
func countFolders() {
	m := config.GlobalConfig.Media
	db := config.GlobalConfig.Database
	// The databases, with SQLite's write-ahead log and shared memory files.
	var dbBytes int64
	for _, p := range []string{db.UserPath, db.MediaPath} {
		if p == "" {
			continue
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if info, err := os.Stat(p + suffix); err == nil {
				dbBytes += info.Size()
			}
		}
	}
	uses := []StorageUse{{Name: "Databases", Bytes: dbBytes}}
	for _, f := range []struct{ name, dir string }{
		{"Thumbnails", m.ThumbnailPath},
		{"Artwork", m.ArtworkPath},
		{"Download encodes", m.DownloadPath},
		{"Subtitles", m.SubtitlePath},
		{"Backups", db.BackupDir()},
		{"Fixed videos", m.RepairPath},
		{"Trash", m.TrashPath},
	} {
		if f.dir != "" {
			uses = append(uses, StorageUse{Name: f.name, Bytes: dirSize(f.dir)})
		}
	}
	storage.mu.Lock()
	storage.uses, storage.counted, storage.counting = uses, time.Now(), false
	storage.mu.Unlock()
}

// countFrames counts the trickplay frames folder.
func countFrames() {
	if dir := config.GlobalConfig.Media.TrickplayPath; dir != "" {
		trickplayBytes.Store(dirSize(dir))
	}
	storage.mu.Lock()
	storage.framesCounted, storage.countingFrames = time.Now(), false
	storage.mu.Unlock()
}

// dirSize adds up the sizes of the files under a folder.
func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
