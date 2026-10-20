package metadata

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// A TV library file that isn't an episode (a TV movie in its show's folder) can be fixed by
// hand as a movie; automatic matching leaves such files alone.
func TestTVExtraMatchedToMovie(t *testing.T) {
	root := t.TempDir()
	tv := filepath.Join(root, "TV")
	showDir := filepath.Join(tv, "1923")
	season := filepath.Join(showDir, "Season 1")
	os.MkdirAll(season, 0755)
	episode := filepath.Join(season, "1923 - S01E02 - Nature's Empty Throne.mkv")
	extra := filepath.Join(showDir, "A Knights Tale (2001).mkv")
	os.WriteFile(episode, nil, 0644)
	os.WriteFile(extra, nil, 0644)

	lib := config.MediaPath{Name: "TV", Path: tv, ID: "t", Type: config.LibraryTypeTV}
	config.GlobalConfig = &config.Config{
		Media: config.MediaConfig{Paths: []config.MediaPath{lib}, ArtworkPath: filepath.Join(root, "artwork"),
			SupportedVideoFormats: []string{".mkv"}},
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
	waitFor(t, func() bool { e, _ := db.GetEpisode(157744, 1, 2); return e != nil }, "show lookup did not finish")

	ref, err := s.MatchTarget(lib, extra, false)
	if err != nil || !IsExtraKey(ref.Key) || ref.Title != "A Knights Tale" || ref.Year != 2001 {
		t.Fatalf("extra target = %+v, %v", ref, err)
	}
	if l, _ := db.GetLookup(ref.Kind, ref.LibraryID, ref.Key); l != nil {
		t.Errorf("automatic sync looked up a non-episode file: %+v", l)
	}
	info, _ := s.MatchInfo(ref)
	if info.MatchKind != "movie" || len(info.Kinds) != 1 || info.Kinds[0] != "movie" {
		t.Errorf("extra match info = %+v", info)
	}
	if err := s.SetMatch(ref, 157744, config.LibraryTypeTV, extra); err != ErrBadMatchKind {
		t.Errorf("extra matched to a show: %v, want ErrBadMatchKind", err)
	}

	if err := s.SetMatch(ref, 9476, "", extra); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { m, _ := db.GetMovie(9476); return m != nil }, "movie details were not fetched")
	s.SyncAll(ctx) // the manual lookup survives pruning
	info, _ = s.MatchInfo(ref)
	if info.MatchKind != "movie" || info.Current == nil || info.Current.Kind != "movie" || info.Current.TMDBID != 9476 {
		t.Errorf("after fix: %+v / %+v", info, info.Current)
	}
	if libs, _ := db.LibrariesForTMDB(config.LibraryTypeMovie, 9476); !contains(libs, "t") {
		t.Errorf("TV library can't see the movie's artwork: %v", libs)
	}

	items := []models.MediaItem{{Path: extra, Name: filepath.Base(extra), Type: "video"}}
	s.Annotate(lib, showDir, nil, items)
	if md := items[0].Metadata; md == nil || md.Kind != "movie" || md.Title != "A Knight's Tale" {
		t.Errorf("browse metadata = %+v", md)
	}
	if d, _ := s.Details(lib, extra, false); d == nil || d.Kind != "movie" || len(d.Directors) == 0 {
		t.Errorf("details = %+v", d)
	}
	fileInfo := func(path string) models.LibraryFile { return models.LibraryFile{Path: path, Name: filepath.Base(path)} }
	page, err := s.Title(ctx, lib, "show", 157744, fileInfo)
	if err != nil || len(page.OtherFiles) != 1 || page.OtherFiles[0].Match == nil || page.OtherFiles[0].Match.TMDBID != 9476 {
		t.Errorf("show page other files = %+v, %v", page, err)
	}

	// Reset forgets the fix and leaves the file alone again.
	if err := s.ResetMatch(ref, extra); err != nil {
		t.Fatal(err)
	}
	s.SyncAll(ctx)
	if l, _ := db.GetLookup(ref.Kind, ref.LibraryID, ref.Key); l != nil {
		t.Errorf("reset extra was looked up again: %+v", l)
	}
	if !strings.HasPrefix(ref.Key, "file|") {
		t.Errorf("extra key = %q", ref.Key)
	}
}
