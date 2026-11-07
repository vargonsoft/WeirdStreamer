package handlers

import (
	"fmt"
	"sync"
	"testing"

	"github.com/weirdstreamer/server/internal/models"
)

// A page opened again while its thumbnails are being made queues nothing twice, and the files are
// made a few at a time, all of them.
func TestThumbnailQueue(t *testing.T) {
	var mu sync.Mutex
	made := map[string]int{}
	running, most := 0, 0
	release := make(chan struct{})
	var done sync.WaitGroup
	q := newThumbnailQueue(func(job thumbnailJob) {
		mu.Lock()
		running++
		most = max(most, running)
		mu.Unlock()
		<-release
		mu.Lock()
		running--
		made[job.item.Path]++
		mu.Unlock()
		done.Done()
	})
	const files = 50
	done.Add(files)
	for range 3 { // the folder opened three times before anything is made
		for i := range files {
			q.add(models.MediaItem{Path: fmt.Sprintf("/lib/%02d.jpg", i)}, false)
		}
	}
	close(release)
	done.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(made) != files {
		t.Fatalf("made %d files, want %d", len(made), files)
	}
	for path, n := range made {
		if n != 1 {
			t.Errorf("%s made %d times", path, n)
		}
	}
	if most > thumbnailWorkers {
		t.Errorf("%d made at once, want at most %d", most, thumbnailWorkers)
	}
}
