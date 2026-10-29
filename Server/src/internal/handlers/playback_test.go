package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/database"
)

func TestPlaybackFinished(t *testing.T) {
	cases := []struct {
		position, duration float64
		ended, want        bool
	}{
		{3500, 3600, false, true},  // long video, in the last 2 minutes (credits)
		{3470, 3600, false, false}, // long video, before the last 2 minutes
		{595, 600, false, false},   // short video, 5 seconds left
		{599.5, 600, false, true},  // short video, at the end
		{100, 600, true, true},     // the player says it ended
		{100, 0, false, false},     // unknown length
	}
	for _, c := range cases {
		if got := playbackFinished(c.position, c.duration, c.ended); got != c.want {
			t.Errorf("playbackFinished(%v, %v, %v) = %v, want %v", c.position, c.duration, c.ended, got, c.want)
		}
	}
}

func TestPlaybackStore(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const user, path = 1, "/media/a.mp4"
	t0 := time.Now()
	get := func() (float64, bool, bool) {
		pb, err := db.UserPlayback(user)
		if err != nil {
			t.Fatal(err)
		}
		p := pb[path]
		if p == nil {
			return 0, false, false
		}
		return p.Position, p.Watched, true
	}

	db.SavePlayback(user, path, database.PlaybackUpdate{Position: 120, Duration: 3600, UpdatedAt: t0}, false)
	if pos, watched, _ := get(); pos != 120 || watched {
		t.Fatalf("in progress: got %v watched=%v", pos, watched)
	}
	// Finishing starts it over and marks it watched; playing it again keeps it watched.
	db.SavePlayback(user, path, database.PlaybackUpdate{Position: 3550, Duration: 3600, Finished: true, UpdatedAt: t0.Add(time.Minute)}, false)
	db.SavePlayback(user, path, database.PlaybackUpdate{Position: 60, Duration: 3600, UpdatedAt: t0.Add(2 * time.Minute)}, false)
	if pos, watched, _ := get(); pos != 60 || !watched {
		t.Fatalf("rewatching: got %v watched=%v", pos, watched)
	}
	// A late offline report older than what's stored is ignored.
	db.SavePlayback(user, path, database.PlaybackUpdate{Position: 900, UpdatedAt: t0.Add(30 * time.Second)}, true)
	if pos, _, _ := get(); pos != 60 {
		t.Fatalf("stale sync applied: got %v", pos)
	}
	db.SavePlayback(user, path, database.PlaybackUpdate{Position: 900, UpdatedAt: t0.Add(3 * time.Minute)}, true)
	if pos, _, _ := get(); pos != 900 {
		t.Fatalf("newer sync not applied: got %v", pos)
	}
	// Unwatched forgets the video entirely.
	db.SetWatched(user, []string{path}, false)
	if _, _, found := get(); found {
		t.Fatal("unwatched video still has a record")
	}
	db.SetWatched(user, []string{path}, true)
	if pos, watched, _ := get(); pos != 0 || !watched {
		t.Fatalf("marked watched: got %v watched=%v", pos, watched)
	}
}
