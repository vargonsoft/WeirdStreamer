package handlers

import (
	"sync"

	"github.com/weirdstreamer/server/internal/models"
)

// thumbnailWorkers is how many queued thumbnails are worked on at once. The generator makes one
// at a time anyway; a second worker reads the next file while it does.
const thumbnailWorkers = 2

// thumbnailQueue makes the thumbnails that browsing and the Library view find missing, in the
// background: each file once however often its page is opened, and a few at a time, so a folder of
// thousands of new photos is a list of them rather than thousands of goroutines.
type thumbnailQueue struct {
	mu      sync.Mutex
	queued  map[string]bool // waiting or being made
	todo    []thumbnailJob
	running int
	run     func(thumbnailJob)
}

type thumbnailJob struct {
	item  models.MediaItem
	probe bool // a file new to the database: its details are read too
}

func newThumbnailQueue(run func(thumbnailJob)) *thumbnailQueue {
	return &thumbnailQueue{queued: map[string]bool{}, run: run}
}

// add queues item's thumbnail, unless it's already queued.
func (q *thumbnailQueue) add(item models.MediaItem, probe bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queued[item.Path] {
		return
	}
	q.queued[item.Path] = true
	q.todo = append(q.todo, thumbnailJob{item, probe})
	if q.running < thumbnailWorkers {
		q.running++
		go q.work()
	}
}

func (q *thumbnailQueue) work() {
	for {
		q.mu.Lock()
		if len(q.todo) == 0 {
			q.running--
			q.todo = nil // lets a long list's memory go
			q.mu.Unlock()
			return
		}
		job := q.todo[0]
		q.todo[0] = thumbnailJob{}
		q.todo = q.todo[1:]
		q.mu.Unlock()
		q.run(job)
		q.mu.Lock()
		delete(q.queued, job.item.Path)
		q.mu.Unlock()
	}
}
