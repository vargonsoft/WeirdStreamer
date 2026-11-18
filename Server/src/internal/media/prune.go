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
	"github.com/weirdstreamer/server/internal/jobs"
)

// Pruner removes the records of media files that are gone: rows outside every library (one
// removed or moved), files no longer on disk and a Mac's leftovers (when those are ignored), with
// their thumbnails, trickplay frames, intros and credits, and disc info. A library whose folder
// can't be read (a share not mounted) is left alone, as is one that would lose more than half its
// files at once (a share half there).
type Pruner struct {
	db   *database.DB
	wake chan struct{}
	job  *jobs.Job
}

func NewPruner(db *database.DB) *Pruner {
	p := &Pruner{db: db, wake: make(chan struct{}, 1),
		job: jobs.Register("cleanup", "Cleanup", "Forgets files that are gone, with their thumbnails, frames, intros and credits.")}
	p.job.OnRunNow(p.Wake)
	return p
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
	p.job.NextIn(5 * time.Minute)
	select {
	case <-ctx.Done():
		return
	case <-p.wake:
	case <-time.After(5 * time.Minute):
	}
	for {
		p.job.Begin(len(config.GlobalConfig.Media.Paths))
		if n, err := p.Prune(ctx, false); err != nil {
			log.Printf("cleanup: %v", err)
			p.job.End("Failed: " + err.Error())
		} else {
			if n > 0 {
				log.Printf("cleanup: forgot %d file(s) that are gone", n)
			}
			p.job.End(plural(n, "gone file") + " forgotten")
		}
		p.job.NextIn(24 * time.Hour)
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-time.After(24 * time.Hour):
		}
	}
}

// Prune forgets the files that are gone, returning how many. force forgets them even from a
// library that would lose more than half its files (one reorganised or moved to another kind of
// share, so most of its old paths are gone); a library that can't be read is still left alone.
func (p *Pruner) Prune(ctx context.Context, force bool) (int, error) {
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
		if IgnoredMacFile(r.Path) {
			gone = append(gone, r) // a Mac's leftover, which scans now skip
			continue
		}
		if IsNonDiscISO(r.Path) {
			gone = append(gone, r) // an installer or data disc image, which scans now skip
			continue
		}
		if i := libraryIndex(libraries, r.Path); i >= 0 {
			byLibrary[i] = append(byLibrary[i], r)
		} else {
			gone = append(gone, r) // in no library
		}
	}
	for i, lib := range libraries {
		if !p.job.Wait(ctx, nil) { // held while paused
			return 0, ctx.Err()
		}
		p.job.Step(1)
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
			if force {
				log.Printf("cleanup: %s loses %d of %d files (forced)", lib.Name, len(missing), len(byLibrary[i]))
				gone = append(gone, missing...)
				continue
			}
			log.Printf("cleanup: %s would lose %d of %d files; leaving them (part of it not mounted?)",
				lib.Name, len(missing), len(byLibrary[i]))
			continue
		}
		gone = append(gone, missing...)
	}
	if len(gone) == 0 {
		return 0, nil
	}
	files := make([]database.GoneFile, len(gone))
	for i, r := range gone {
		key, _ := config.ClientPath(r.Path) // none in no library: its client-path rows are left
		files[i] = database.GoneFile{Path: r.Path, Key: key}
	}
	if err := p.db.ForgetMedia(files); err != nil {
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
		frames := filepath.Join(dir, GenerateThumbnailHash(r.Path))
		size := dirSize(frames)
		if os.RemoveAll(frames) == nil {
			trickplayBytes.Add(-size) // the Jobs page's count of frames
		}
	}
}
