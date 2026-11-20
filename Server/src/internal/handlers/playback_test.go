package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
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
		if got := playbackFinished(c.position, c.duration, 0, c.ended); got != c.want {
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
	const user, path = 1, "8838533a/a.mp4" // a client path
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
	// A disk path is never stored.
	if err := db.SavePlayback(user, "/media/a.mp4", database.PlaybackUpdate{Position: 5, UpdatedAt: t0}, false); err == nil {
		t.Fatal("a disk path was stored")
	}
	if err := db.SetWatched(user, []string{"/media/a.mp4"}, true); err == nil {
		t.Fatal("a disk path was marked watched")
	}
}

// Progress is kept by client path: moving a library's folder keeps it, with no data changed.
func TestPlaybackSurvivesLibraryMove(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = saved })
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{Paths: []config.MediaPath{{ID: "8838533a", Path: "/old/shows"}}}}

	key := clientPath("/old/shows/Show/S01E01.mkv")
	if key != "8838533a/Show/S01E01.mkv" {
		t.Fatalf("key %q", key)
	}
	if err := db.SavePlayback(1, key, database.PlaybackUpdate{Position: 300, Duration: 1800, UpdatedAt: time.Now()}, false); err != nil {
		t.Fatal(err)
	}

	config.GlobalConfig.Media.Paths[0].Path = "/new/place/shows"
	if moved := clientPath("/new/place/shows/Show/S01E01.mkv"); moved != key {
		t.Fatalf("key changed with the library's folder: %q", moved)
	}
	stored, _ := db.UserPlayback(1)
	if _, ok := stored[key]; !ok || len(stored) != 1 {
		t.Fatalf("stored keys %v", stored)
	}
	pb := byDiskPath(stored)
	if p := pb["/new/place/shows/Show/S01E01.mkv"]; p == nil || p.Position != 300 {
		t.Fatalf("progress not found at the new folder: %v", pb)
	}
	// A library no longer configured: its records are left out, not mistaken for another's.
	config.GlobalConfig.Media.Paths[0].ID = "ffffffff"
	if pb := byDiskPath(stored); len(pb) != 0 {
		t.Fatalf("records of a removed library listed: %v", pb)
	}
}

// Reaching the detected credits counts as finished, even well before the last two minutes.
func TestPlaybackFinishedAtCredits(t *testing.T) {
	const duration, credits = 7200.0, 6960.0 // a two-hour film whose credits start 4 minutes before the end
	if playbackFinished(6900, duration, credits, false) {
		t.Error("before the credits: not finished")
	}
	if !playbackFinished(6960, duration, credits, false) {
		t.Error("at the credits: finished")
	}
	if !playbackFinished(7000, duration, credits, false) {
		t.Error("in the credits: finished")
	}
	if playbackFinished(6900, duration, 0, false) {
		t.Error("no credits detected: the two-minute rule applies")
	}
}

// Continue Watching keeps a video it lists whatever position is reported later; a video only
// opened (under minResumePosition) never goes on it, and one taken off it stays off.
func TestContinueListing(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const user = 1
	t0 := time.Now()
	save := func(path string, pos float64, at int, finished bool) {
		if err := db.SavePlayback(user, path, database.PlaybackUpdate{Position: pos, Duration: 3600, Finished: finished,
			UpdatedAt: t0.Add(time.Duration(at) * time.Minute)}, false); err != nil {
			t.Fatal(err)
		}
	}
	listed := func() map[string]bool {
		paths, _, err := db.RecentPlayback(user, minResumePosition, 100)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, p := range paths {
			out[p] = true
		}
		return out
	}
	state := func(path string) int {
		pb, err := db.UserPlayback(user)
		if err != nil || pb[path] == nil {
			t.Fatalf("no record for %s", path)
		}
		return pb[path].ContinueState
	}

	save("8838533a/opened.mp4", 4, 0, false) // only opened: not listed
	save("8838533a/resumed.mp4", 300, 0, false)
	save("8838533a/resumed.mp4", 3, 1, false) // listed, then left near the start
	if l := listed(); l["8838533a/opened.mp4"] || !l["8838533a/resumed.mp4"] {
		t.Fatalf("listed %v", l)
	}
	// Finishing takes it off (the next episode takes over); rewatching lists it again.
	save("8838533a/resumed.mp4", 3590, 2, true)
	if listed()["8838533a/resumed.mp4"] || state("8838533a/resumed.mp4") != 0 {
		t.Fatal("finished video still listed")
	}
	save("8838533a/resumed.mp4", 60, 3, false)
	save("8838533a/resumed.mp4", 2, 4, false)
	if !listed()["8838533a/resumed.mp4"] {
		t.Fatal("rewatched video not listed")
	}
	// Taken off the list: off until it's played past minResumePosition again.
	if err := db.ClearPosition(user, "8838533a/resumed.mp4"); err != nil {
		t.Fatal(err)
	}
	save("8838533a/resumed.mp4", 5, 5, false)
	if listed()["8838533a/resumed.mp4"] || state("8838533a/resumed.mp4") != database.ContinueRemoved {
		t.Fatal("removed video back on the list")
	}
	save("8838533a/resumed.mp4", 40, 6, false)
	if !listed()["8838533a/resumed.mp4"] {
		t.Fatal("video played again not listed")
	}
	// An up-next episode removed is marked so; one only opened isn't.
	if err := db.ClearPosition(user, "8838533a/next.mp4"); err != nil {
		t.Fatal(err)
	}
	if state("8838533a/next.mp4") != database.ContinueRemoved || state("8838533a/opened.mp4") != 0 {
		t.Fatal("removal not told apart from opening")
	}
}
