package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

func TestPosterChoice(t *testing.T) {
	root := t.TempDir()
	movies := filepath.Join(root, "Movies")
	artwork := filepath.Join(root, "artwork")
	knight := filepath.Join(movies, "A Knight's Tale (2001)")
	os.MkdirAll(knight, 0755)
	os.WriteFile(filepath.Join(knight, "A Knights Tale.m4v"), nil, 0644)

	lib := config.MediaPath{Name: "Movies", Path: movies, ID: "m", Type: config.LibraryTypeMovie}
	config.GlobalConfig = &config.Config{
		Media:    config.MediaConfig{Paths: []config.MediaPath{lib}, ArtworkPath: artwork, SupportedVideoFormats: []string{".m4v"}},
		Metadata: config.MetadataConfig{TMDBAccessToken: "token"},
	}
	db, err := database.New(filepath.Join(root, "users.db"), filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	srv := fakeTMDB(t)
	s := NewService(db, nil)
	s.newClient = func(token, language string) *Client {
		c := NewClient(token, language)
		c.apiBase, c.imageBase = srv.URL+"/3", srv.URL+"/t/p/"
		return c
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.SyncAll(ctx)
	waitFor(t, func() bool { m, _ := db.GetMovie(9476); return m != nil }, "movie lookup did not finish")

	m, _ := db.GetMovie(9476)
	if m.PosterSource != "/p.jpg" || m.PosterManual {
		t.Fatalf("default poster source = %q manual=%v", m.PosterSource, m.PosterManual)
	}

	// Every poster is offered: English first, then text-free, then other languages, best rated first.
	list, err := s.Posters(ctx, config.LibraryTypeMovie, 9476)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, p := range list.Posters {
		order = append(order, p.Path)
	}
	if want := "[/en1.jpg /en2.png /plain.jpg /fr.jpg]"; list.Current != "/p.jpg" || list.Manual || fmtList(order) != want {
		t.Errorf("posters = current %q manual %v order %v, want current /p.jpg order %s", list.Current, list.Manual, order, want)
	}

	// Picking one replaces the poster file (old one removed when the type differs).
	if err := s.SetPoster(ctx, config.LibraryTypeMovie, 9476, "/en2.png", ""); err != nil {
		t.Fatal(err)
	}
	m, _ = db.GetMovie(9476)
	if m.Poster != "movie/9476/poster.png" || m.PosterSource != "/en2.png" || !m.PosterManual {
		t.Errorf("after pick: %+v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(artwork, "movie", "9476", "poster.png")); string(b) != "image:/t/p/w500/en2.png" {
		t.Errorf("poster file = %q", b)
	}
	if _, err := os.Stat(filepath.Join(artwork, "movie", "9476", "poster.jpg")); !os.IsNotExist(err) {
		t.Errorf("old poster.jpg still there: %v", err)
	}

	// A refresh of the details keeps the picked poster.
	db.SetTitlePoster("movies", 9476, m.Poster, m.PosterSource, true, time.Time{})
	if changed, err := s.ensureMovie(ctx, s.tmdb(), 9476); err != nil || !changed {
		t.Fatalf("refresh: changed=%v err=%v", changed, err)
	}
	m, _ = db.GetMovie(9476)
	if m.PosterSource != "/en2.png" || !m.PosterManual || m.Poster != "movie/9476/poster.png" {
		t.Errorf("after refresh: %+v", m)
	}

	// Another title's poster can be used too, and "" goes back to the default.
	if err := s.SetPoster(ctx, config.LibraryTypeMovie, 9476, "/m.jpg", ""); err != nil {
		t.Fatal(err)
	}
	if m, _ = db.GetMovie(9476); m.PosterSource != "/m.jpg" || m.Poster != "movie/9476/poster.jpg" {
		t.Errorf("other title's poster: %+v", m)
	}
	if err := s.SetPoster(ctx, config.LibraryTypeMovie, 9476, "", ""); err != nil {
		t.Fatal(err)
	}
	if m, _ = db.GetMovie(9476); m.PosterSource != "/p.jpg" || m.PosterManual {
		t.Errorf("after reset: %+v", m)
	}

	// Only TMDB image paths are accepted, and only for stored titles.
	for _, bad := range []string{"https://example.com/x.jpg", "/../x.jpg", "/x.svg", "x.jpg"} {
		if err := s.SetPoster(ctx, config.LibraryTypeMovie, 9476, bad, ""); !errors.Is(err, ErrBadPoster) {
			t.Errorf("SetPoster(%q) = %v, want ErrBadPoster", bad, err)
		}
	}
	if err := s.SetPoster(ctx, config.LibraryTypeMovie, 603, "/m.jpg", ""); !errors.Is(err, ErrNotFetched) {
		t.Errorf("unfetched title: %v, want ErrNotFetched", err)
	}
}

func fmtList(items []string) string {
	out := "["
	for i, s := range items {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out + "]"
}
