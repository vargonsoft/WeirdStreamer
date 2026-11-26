package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// rerunFixture is a movie library with one file, a service whose TMDB fake blocks the first
// lookup's search until release is called, and that lookup's title with a due lookup row.
type rerunFixture struct {
	s         *Service
	db        *database.DB
	ref       Ref
	moviePath string
	searches  atomic.Int32  // searches made with a year: one per lookup run
	started   chan struct{} // receives when the first run's search arrives
	release   func()        // lets the blocked search answer; safe to call more than once
}

func newRerunFixture(t *testing.T) *rerunFixture {
	t.Helper()
	root := t.TempDir()
	movies := filepath.Join(root, "Movies")
	knight := filepath.Join(movies, "A Knight's Tale (2001)")
	os.MkdirAll(knight, 0755)
	moviePath := filepath.Join(knight, "A Knights Tale.m4v")
	os.WriteFile(moviePath, nil, 0644)

	movieLib := config.MediaPath{Name: "Movies", Path: movies, ID: "m", Type: config.LibraryTypeMovie}
	config.GlobalConfig = &config.Config{
		Media: config.MediaConfig{
			Paths:                 []config.MediaPath{movieLib},
			ArtworkPath:           filepath.Join(root, "artwork"),
			SupportedVideoFormats: []string{".m4v", ".mkv"},
		},
		Metadata: config.MetadataConfig{TMDBAccessToken: "token"},
	}

	db, err := database.New(filepath.Join(root, "users.db"), filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	f := &rerunFixture{db: db, moviePath: moviePath, started: make(chan struct{}, 1)}
	release := make(chan struct{})
	var releaseOnce sync.Once
	f.release = func() { releaseOnce.Do(func() { close(release) }) }
	// Every search is answered with no results. A lookup makes one search with the year and, finding
	// nothing, one without it: only the search with the year counts as a run. The first run's
	// search blocks until release.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/search/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("year") != "" && f.searches.Add(1) == 1 {
			f.started <- struct{}{}
			<-release
		}
		json.NewEncoder(w).Encode(map[string]any{"page": 1, "results": []any{}, "total_pages": 0, "total_results": 0})
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(f.release)

	f.s = NewService(db, func(string) {})
	f.s.newClient = func(token, language string) *Client {
		c := NewClient(token, language)
		c.apiBase, c.imageBase = srv.URL+"/3", srv.URL+"/t/p/"
		return c
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f.s.Start(ctx)

	f.ref, err = f.s.MatchTarget(movieLib, moviePath, false)
	if err != nil {
		t.Fatal(err)
	}
	// ResetMatch only re-runs a lookup that exists, so the title starts with a lookup that is due.
	if err := db.SaveLookup(&database.Lookup{Kind: f.ref.Kind, LibraryID: f.ref.LibraryID, QueryKey: f.ref.Key,
		QueryTitle: f.ref.Title, QueryYear: f.ref.Year, QueryCountry: f.ref.Country,
		Status: "error", AttemptedAt: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return f
}

// waitStarted waits until the first run's search is blocked.
func (f *rerunFixture) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the lookup did not start")
	}
}

// A request made while the same lookup is running must not be lost: it runs once more after.
func TestRequestDuringARunRunsAgain(t *testing.T) {
	f := newRerunFixture(t)
	f.s.EnqueueRef(f.ref, f.moviePath)
	f.waitStarted(t)
	// The lookup is running now: this request arrives while it is blocked.
	if err := f.s.ResetMatch(f.ref, f.moviePath); err != nil {
		t.Fatal(err)
	}
	f.release()

	deadline := time.Now().Add(10 * time.Second)
	for f.searches.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("a request made while the lookup ran was lost: %d searches", f.searches.Load())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A match set by hand while the automatic lookup runs is kept: the run's result (not found, from
// the row before) saves nothing over it.
func TestHandMatchDuringARunIsKept(t *testing.T) {
	f := newRerunFixture(t)
	f.s.EnqueueRef(f.ref, f.moviePath)
	f.waitStarted(t)
	if err := f.s.SetMatch(f.ref, 9476, config.LibraryTypeMovie, f.moviePath); err != nil {
		t.Fatal(err)
	}
	f.release()

	kept := func() (bool, string) {
		l, err := f.db.GetLookup(f.ref.Kind, f.ref.LibraryID, f.ref.Key)
		if err != nil || l == nil {
			return false, "no lookup"
		}
		return l.Manual && l.TMDBID == 9476, l.Status
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if ok, _ := kept(); ok {
			break
		}
		if time.Now().After(deadline) {
			_, status := kept()
			t.Fatalf("the hand match was lost during the run: status %q", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// It must stay: the run could still save over it after the first look.
	stay := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(stay) {
		if ok, status := kept(); !ok || status == "not_found" {
			t.Fatalf("the hand match was overwritten during the run: status %q", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
