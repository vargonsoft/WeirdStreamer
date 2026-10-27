package media

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// Pruner removes the records of media files that are gone: rows outside every library (one
// removed or moved) and files no longer on disk, with their thumbnails, trickplay frames,
// intros and credits, and disc info. A library whose folder can't be read (a share not mounted)
// is left alone, as is one that would lose more than half its files at once (a share half there).
type Pruner struct {
	db   *database.DB
	wake chan struct{}
}

func NewPruner(db *database.DB) *Pruner {
	return &Pruner{db: db, wake: make(chan struct{}, 1)}
}

// Wake has the pruner look now (after a scan).
func (p *Pruner) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Run prunes a few minutes after starting, when woken, and daily, until ctx ends.
func (p *Pruner) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Minute):
	}
	for {
		if n, err := p.Prune(ctx); err != nil {
			log.Printf("cleanup: %v", err)
		} else if n > 0 {
			log.Printf("cleanup: forgot %d file(s) that are gone", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-time.After(24 * time.Hour):
		}
	}
}

// Prune forgets the files that are gone, returning how many.
func (p *Pruner) Prune(ctx context.Context) (int, error) {
	records, err := p.db.MediaRecords()
	if err != nil {
		return 0, err
	}
	libraries := config.GlobalConfig.Media.Paths
	if len(libraries) == 0 {
		return 0, nil // nothing to judge by
	}
	byLibrary := make([][]database.MediaRecord, len(libraries))
	var gone []database.MediaRecord
	for _, r := range records {
		if i := libraryIndex(libraries, r.Path); i >= 0 {
			byLibrary[i] = append(byLibrary[i], r)
		} else {
			gone = append(gone, r) // in no library
		}
	}
	for i, lib := range libraries {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if entries, err := os.ReadDir(lib.Path); err != nil || len(entries) == 0 {
			if len(byLibrary[i]) > 0 {
				log.Printf("cleanup: %s can't be read (not mounted?); leaving its %d files", lib.Name, len(byLibrary[i]))
			}
			continue
		}
		var missing []database.MediaRecord
		for _, r := range byLibrary[i] {
			if _, err := os.Stat(r.Path); errors.Is(err, fs.ErrNotExist) {
				missing = append(missing, r)
			}
		}
		if len(missing) > 20 && len(missing)*2 > len(byLibrary[i]) {
			log.Printf("cleanup: %s would lose %d of %d files; leaving them (part of it not mounted?)",
				lib.Name, len(missing), len(byLibrary[i]))
			continue
		}
		gone = append(gone, missing...)
	}
	if len(gone) == 0 {
		return 0, nil
	}
	paths := make([]string, len(gone))
	for i, r := range gone {
		paths[i] = r.Path
	}
	if err := p.db.ForgetMedia(paths); err != nil {
		return 0, err
	}
	for _, r := range gone {
		removeGenerated(r)
	}
	return len(gone), nil
}

func libraryIndex(libraries []config.MediaPath, path string) int {
	abs, err := filepath.Abs(path)
	if err != nil {
		return -1
	}
	for i, lib := range libraries {
		root, err := filepath.Abs(lib.Path)
		if err != nil {
			continue
		}
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return i
		}
	}
	return -1
}

// removeGenerated deletes a forgotten file's thumbnail and trickplay frames.
func removeGenerated(r database.MediaRecord) {
	if dir := config.GlobalConfig.Media.ThumbnailPath; dir != "" && strings.HasPrefix(r.ThumbnailURL, "/thumbnails/") {
		rel := filepath.FromSlash(strings.TrimPrefix(r.ThumbnailURL, "/thumbnails/"))
		if !strings.Contains(rel, "..") {
			os.Remove(filepath.Join(dir, rel))
		}
	}
	if dir := config.GlobalConfig.Media.TrickplayPath; dir != "" {
		os.RemoveAll(filepath.Join(dir, GenerateThumbnailHash(r.Path)))
	}
}
