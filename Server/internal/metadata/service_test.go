package metadata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// fakeTMDB answers the handful of endpoints the service uses.
func fakeTMDB(t *testing.T) *httptest.Server {
	responses := map[string]string{
		"/3/search/movie": `{"results":[
			{"id":99,"title":"A Knight's Tale","release_date":"1950-01-01"},
			{"id":9476,"title":"A Knight's Tale","release_date":"2001-05-11"}]}`,
		"/3/movie/9476": `{"id":9476,"title":"A Knight's Tale","release_date":"2001-05-11","overview":"Jousting.",
			"runtime":133,"genres":[{"name":"Adventure"},{"name":"Drama"}],"imdb_id":"tt0183790",
			"poster_path":"/p.jpg","backdrop_path":"/b.jpg",
			"credits":{"cast":[{"id":1810,"name":"Heath Ledger","character":"William Thatcher","order":0,"profile_path":"/h.jpg"}],
				"crew":[{"id":7,"name":"Brian Helgeland","job":"Director"},{"id":7,"name":"Brian Helgeland","job":"Screenplay"}]},
			"images":{"logos":[{"file_path":"/fr.png","iso_639_1":"fr","vote_average":9},{"file_path":"/en.png","iso_639_1":"en","vote_average":1}]}}`,
		"/3/movie/9476/images": `{"posters":[{"file_path":"/en2.png","iso_639_1":"en","vote_average":5},
			{"file_path":"/fr.jpg","iso_639_1":"fr","vote_average":9},{"file_path":"/plain.jpg","iso_639_1":null,"vote_average":3},
			{"file_path":"/en1.jpg","iso_639_1":"en","vote_average":8}]}`,
		"/3/movie/603": `{"id":603,"title":"The Matrix","release_date":"1999-03-30","overview":"Neo.","poster_path":"/m.jpg"}`,
		"/3/search/tv": `{"results":[{"id":157744,"name":"1923","first_air_date":"2022-12-18","origin_country":["US"]}]}`,
		"/3/tv/157744": `{"id":157744,"name":"1923","first_air_date":"2022-12-18","overview":"Duttons.","episode_run_time":[50],
			"poster_path":"/tp.jpg","created_by":[{"id":5,"name":"Taylor Sheridan"}],"external_ids":{"imdb_id":"tt18335752"},
			"aggregate_credits":{"cast":[{"id":15735,"name":"Helen Mirren","order":0,"roles":[{"character":"Cara Dutton"}]}]}}`,
		"/3/tv/157744/season/1": `{"season_number":1,"name":"Season 1","episodes":[
			{"id":1,"season_number":1,"episode_number":2,"name":"Nature's Empty Throne","overview":"Ep two.","air_date":"2022-12-25",
			 "runtime":48,"still_path":"/s.jpg","crew":[{"id":8,"name":"Ben Richardson","job":"Director"}],
			 "guest_stars":[{"id":9,"name":"Guest","character":"Someone","order":0}]}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/t/p/") {
			w.Write([]byte("image:" + r.URL.Path))
			return
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestServiceSyncMatchesAndAnnotates(t *testing.T) {
	root := t.TempDir()
	movies := filepath.Join(root, "Movies")
	tv := filepath.Join(root, "TV")
	artwork := filepath.Join(root, "artwork")
	knight := filepath.Join(movies, "A Knight's Tale (2001)")
	season := filepath.Join(tv, "1923", "Season 1")
	for _, d := range []string{knight, season} {
		os.MkdirAll(d, 0755)
	}
	// A show folder whose name carries a season number, with recordings that aren't episodes.
	bb := filepath.Join(tv, "Big Brother 7 UK")
	os.MkdirAll(filepath.Join(bb, "Week 1"), 0755)
	os.WriteFile(filepath.Join(bb, "Week 1", "Day 1.mkv"), nil, 0644)
	os.WriteFile(filepath.Join(bb, "Big Brother - 7x57.mkv"), nil, 0644)
	os.WriteFile(filepath.Join(bb, "Big Brother - 7x58b - Eviction Part 2.mkv"), nil, 0644)
	moviePath := filepath.Join(knight, "A Knights Tale.m4v")
	episodePath := filepath.Join(season, "1923 - S01E02 - Nature's Empty Throne Bluray-1080p.mkv")
	os.WriteFile(moviePath, nil, 0644)
	os.WriteFile(episodePath, nil, 0644)

	movieLib := config.MediaPath{Name: "Movies", Path: movies, ID: "m", Type: config.LibraryTypeMovie}
	tvLib := config.MediaPath{Name: "TV", Path: tv, ID: "t", Type: config.LibraryTypeTV}
	config.GlobalConfig = &config.Config{
		Media: config.MediaConfig{
			Paths:                 []config.MediaPath{movieLib, tvLib},
			ArtworkPath:           artwork,
			SupportedVideoFormats: []string{".m4v", ".mkv"},
		},
		Metadata: config.MetadataConfig{TMDBAccessToken: "token"},
	}

	db, err := database.New(filepath.Join(root, "users.db"), filepath.Join(root, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	srv := fakeTMDB(t)
	notified := make(chan string, 16)
	s := NewService(db, func(path string) { notified <- path })
	s.newClient = func(token, language string) *Client {
		c := NewClient(token, language)
		c.apiBase, c.imageBase = srv.URL+"/3", srv.URL+"/t/p/"
		return c
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.SyncAll(ctx)

	// Wait for the movie lookup, show lookup and season fetch to land.
	deadline := time.Now().Add(10 * time.Second)
	for {
		e, _ := db.GetEpisode(157744, 1, 2)
		m, _ := db.GetMovie(9476)
		if e != nil && m != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lookups did not finish")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The show folder follows its episodes; non-episode files create no lookup.
	if l, _ := db.GetLookup("tv", "t", "big brother 7|gb"); l != nil {
		t.Errorf("non-episode files created a lookup for the folder name: %+v", l)
	}
	if keys, _ := db.FolderKeys("t", []string{bb}); keys[bb] != "big brother|gb" {
		t.Errorf("show folder key = %q, want the episodes' show", keys[bb])
	}
	os.RemoveAll(bb)

	// Movie: right year chosen over the earlier same-titled result, English logo over French.
	m, _ := db.GetMovie(9476)
	if m.Year != 2001 || m.Genres != "Adventure, Drama" || m.Logo != "movie/9476/logo.png" || m.Poster != "movie/9476/poster.jpg" {
		t.Errorf("movie = %+v", m)
	}
	for _, rel := range []string{"movie/9476/poster.jpg", "movie/9476/backdrop.jpg", "movie/9476/logo.png", "person/1810.jpg", "tv/157744/s01e02.jpg"} {
		if _, err := os.Stat(filepath.Join(artwork, rel)); err != nil {
			t.Errorf("artwork %s missing: %v", rel, err)
		}
	}

	// Browse annotations: the movie folder, the movie file, the show folder and the episode.
	dirs := []models.Directory{{Name: "A Knight's Tale (2001)"}}
	s.Annotate(movieLib, movies, dirs, nil)
	if dirs[0].Metadata == nil || dirs[0].Metadata.Title != "A Knight's Tale" || !strings.HasPrefix(dirs[0].Metadata.Poster, "/artwork/movie/9476/poster.jpg?v=") {
		t.Errorf("movie folder metadata = %+v", dirs[0].Metadata)
	}
	items := []models.MediaItem{{Path: episodePath, Type: "video"}}
	s.Annotate(tvLib, season, nil, items)
	if md := items[0].Metadata; md == nil || md.Kind != "episode" || md.Title != "Nature's Empty Throne" || md.ShowTitle != "1923" || *md.Episode != 2 {
		t.Errorf("episode metadata = %+v", items[0].Metadata)
	}
	showDirs := []models.Directory{{Name: "1923"}}
	s.Annotate(tvLib, tv, showDirs, nil)
	if md := showDirs[0].Metadata; md == nil || md.Kind != "show" || md.Runtime != 50 {
		t.Errorf("show folder metadata = %+v", showDirs[0].Metadata)
	}

	// Details include directors, writers (deduplicated), cast with headshots; episodes add the show's cast.
	d, err := s.Details(movieLib, moviePath, false)
	if err != nil || d == nil || len(d.Directors) != 1 || len(d.Writers) != 1 || d.Cast[0].Character != "William Thatcher" || d.Cast[0].Profile != "/artwork/person/1810.jpg" {
		t.Errorf("movie details = %+v, %v", d, err)
	}
	ed, err := s.Details(tvLib, episodePath, false)
	if err != nil || ed == nil || ed.Directors[0].Name != "Ben Richardson" || ed.GuestStars[0].Name != "Guest" ||
		ed.Cast[0].Name != "Helen Mirren" || ed.Creators[0].Name != "Taylor Sheridan" {
		t.Errorf("episode details = %+v, %v", ed, err)
	}
	if len(notified) == 0 {
		t.Error("no metadata-ready notifications")
	}

	// Library view: titles sorted by title, unmatched files listed separately.
	fileInfo := func(path string) models.LibraryFile { return models.LibraryFile{Path: path, Name: filepath.Base(path)} }
	listing, err := s.Library(ctx, movieLib, fileInfo)
	if err != nil || len(listing.Titles) != 1 || listing.Titles[0].TMDBID != 9476 || listing.Titles[0].Poster == "" || len(listing.Unmatched) != 0 {
		t.Errorf("movie listing = %+v, %v", listing, err)
	}
	page, err := s.Title(ctx, movieLib, "movie", 9476, fileInfo)
	if err != nil || len(page.Files) != 1 || page.Files[0].Path != moviePath || page.Directors[0].Name != "Brian Helgeland" {
		t.Errorf("movie page = %+v, %v", page, err)
	}
	if _, err := s.Title(ctx, movieLib, "movie", 603, fileInfo); err != ErrNotInLibrary {
		t.Errorf("title not in library: err = %v", err)
	}
	tvListing, err := s.Library(ctx, tvLib, fileInfo)
	if err != nil || len(tvListing.Titles) != 1 || tvListing.Titles[0].Kind != "show" || tvListing.Titles[0].Title != "1923" {
		t.Errorf("tv listing = %+v, %v", tvListing, err)
	}
	showPage, err := s.Title(ctx, tvLib, "show", 157744, fileInfo)
	if err != nil || len(showPage.Seasons) != 1 || showPage.Seasons[0].Episodes[0].Title != "Nature's Empty Throne" ||
		showPage.Seasons[0].Episodes[0].Files[0].Path != episodePath || showPage.Cast[0].Name != "Helen Mirren" {
		t.Errorf("show page = %+v, %v", showPage, err)
	}
	// A file added later shows up as unmatched once its lookup finds nothing.
	stray := filepath.Join(movies, "Unknown Film.mkv")
	os.WriteFile(stray, nil, 0644)
	s.SyncAll(ctx)
	waitFor(t, func() bool {
		l, _ := s.Library(ctx, movieLib, fileInfo)
		return len(l.Unmatched) == 1 && l.Unmatched[0].Path == stray
	}, "unmatched file not listed")
	os.Remove(stray)
	s.SyncAll(ctx)

	// A folder added after the sync is discovered by browsing: one video means one movie; a
	// collection of different titles or a folder without video stays a plain folder.
	added := filepath.Join(movies, "Knight Copy")
	collection := filepath.Join(movies, "Collection")
	empty := filepath.Join(movies, "Extras")
	for _, d := range []string{added, collection, empty} {
		os.MkdirAll(d, 0755)
	}
	os.WriteFile(filepath.Join(added, "A Knight's Tale (2001).mkv"), nil, 0644)
	os.WriteFile(filepath.Join(collection, "A Knight's Tale.mkv"), nil, 0644)
	os.WriteFile(filepath.Join(collection, "Inception.mkv"), nil, 0644)
	os.WriteFile(filepath.Join(empty, "notes.txt"), nil, 0644)
	browse := func() []models.Directory {
		listing := []models.Directory{{Name: "Knight Copy"}, {Name: "Collection"}, {Name: "Extras"}}
		s.Annotate(movieLib, movies, listing, nil)
		return listing
	}
	if first := browse(); first[0].Metadata != nil {
		t.Errorf("new folder had metadata before discovery: %+v", first[0].Metadata)
	}
	deadline = time.Now().Add(10 * time.Second)
	for {
		listing := browse()
		if listing[0].Metadata != nil {
			if listing[0].Metadata.TMDBID != 9476 || listing[1].Metadata != nil || listing[2].Metadata != nil {
				t.Errorf("discovered folders = %+v / %+v / %+v", listing[0].Metadata, listing[1].Metadata, listing[2].Metadata)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("new movie folder was never discovered")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Every movie in a collection folder is looked up by its own file name.
	waitFor(t, func() bool {
		l, _ := db.GetLookup("movie", "m", LookupKey("Inception", 0))
		return l != nil
	}, "collection file was not looked up")
	for _, d := range []string{added, collection, empty} {
		os.RemoveAll(d)
	}

	// A TV show folder added after the sync is looked up once it has episodes; an unhelpful folder
	// name falls back to the show name in the episode files.
	renamed := filepath.Join(tv, "Renamed Show", "Season 1")
	os.MkdirAll(renamed, 0755)
	os.WriteFile(filepath.Join(renamed, "1923 - S01E02 - Nature's Empty Throne.mkv"), nil, 0644)
	os.MkdirAll(filepath.Join(tv, "Empty Show", "Season 1"), 0755)
	listShows := func() []models.Directory {
		listing := []models.Directory{{Name: "Renamed Show"}, {Name: "Empty Show"}}
		s.Annotate(tvLib, tv, listing, nil)
		return listing
	}
	listShows()
	waitFor(t, func() bool { return listShows()[0].Metadata != nil }, "renamed show folder was never discovered")
	if got := listShows(); got[0].Metadata.TMDBID != 157744 || got[1].Metadata != nil {
		t.Errorf("discovered shows = %+v / %+v", got[0].Metadata, got[1].Metadata)
	}
	if l, _ := db.GetLookup("tv", "t", LookupKey("Empty Show", 0)); l != nil {
		t.Errorf("show folder without episodes was looked up: %+v", l)
	}
	os.RemoveAll(filepath.Join(tv, "Renamed Show"))
	os.RemoveAll(filepath.Join(tv, "Empty Show"))

	// A manual match replaces the automatic one, survives syncs, and can be reset.
	ref, err := s.MatchTarget(movieLib, moviePath, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMatch(ref, 603, moviePath); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { m, _ := db.GetMovie(603); return m != nil }, "manual match details were not fetched")
	s.SyncAll(ctx)
	info, _ := s.MatchInfo(ref)
	if !info.Manual || info.Current == nil || info.Current.TMDBID != 603 || info.Query != "A Knights Tale" || info.Year != 2001 {
		t.Errorf("after manual match + sync: %+v / %+v", info, info.Current)
	}
	// The movie folder resolves to the same title, so it can be fixed (and shows the fix) too.
	folderRef, err := s.MatchTarget(movieLib, knight, true)
	if err != nil || folderRef.Key != ref.Key {
		t.Errorf("folder target = %+v, %v; want key %q", folderRef, err, ref.Key)
	}
	if err := s.SetMatch(ref, 0, moviePath); err != nil {
		t.Fatal(err)
	}
	s.SyncAll(ctx)
	if info, _ := s.MatchInfo(ref); !info.Manual || info.Status != "not_found" {
		t.Errorf("manual no-match was not kept: %+v", info)
	}
	if err := s.ResetMatch(ref, moviePath); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		info, _ := s.MatchInfo(ref)
		return info.Current != nil && info.Current.TMDBID == 9476 && !info.Manual
	},
		"reset did not restore the automatic match")
	// Collection folders and non-episode TV files can't be fixed as a whole.
	os.MkdirAll(filepath.Join(movies, "Mixed"), 0755)
	os.WriteFile(filepath.Join(movies, "Mixed", "One.mkv"), nil, 0644)
	os.WriteFile(filepath.Join(movies, "Mixed", "Two.mkv"), nil, 0644)
	s.SyncAll(ctx)
	if _, err := s.MatchTarget(movieLib, filepath.Join(movies, "Mixed"), true); err != ErrNotFixable {
		t.Errorf("collection folder target err = %v, want ErrNotFixable", err)
	}
	os.RemoveAll(filepath.Join(movies, "Mixed"))

	// Artwork deleted from disk (while the record says it's there) is downloaded again.
	os.RemoveAll(filepath.Join(artwork, "movie", "9476"))
	s.SyncAll(ctx)
	waitFor(t, func() bool {
		_, err := os.Stat(filepath.Join(artwork, "movie", "9476", "poster.jpg"))
		return err == nil
	}, "missing artwork was not downloaded again")
	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.pending) == 0
	}, "lookup queue did not drain")

	// A stored record nothing is matched to (and with no artwork folder) is removed too.
	db.SaveMovie(&database.Movie{TMDBID: 4242, Title: "Orphan", UpdatedAt: time.Now()}, nil)
	s.SyncAll(ctx)
	if m, _ := db.GetMovie(4242); m != nil {
		t.Error("orphaned movie record was not removed")
	}

	// Episode mapping: a whole file season can be sent to another TMDB season (with an episode
	// shift), or one file pinned to an exact episode. Only TMDB's S01E02 exists in the fake.
	s5 := filepath.Join(tv, "1923", "Season 5")
	os.MkdirAll(s5, 0755)
	seasonFile := filepath.Join(s5, "1923 - S05E04.mkv")
	pinnedFile := filepath.Join(season, "1923 - S01E07.mkv")
	os.WriteFile(seasonFile, nil, 0644)
	os.WriteFile(pinnedFile, nil, 0644)
	episodeTitle := func(path string) string {
		items := []models.MediaItem{{Path: path, Type: "video"}}
		s.Annotate(tvLib, filepath.Dir(path), nil, items)
		if items[0].Metadata == nil {
			return ""
		}
		return fmt.Sprintf("S%02dE%02d %s", *items[0].Metadata.Season, *items[0].Metadata.Episode, items[0].Metadata.Title)
	}
	if got := episodeTitle(seasonFile); got != "" {
		t.Fatalf("unmapped S05E04 had metadata: %q", got)
	}
	if err := s.SetEpisodeMapping(tvLib, seasonFile, 1, 2, true); err != nil { // season 5 -> 1, episodes -2
		t.Fatal(err)
	}
	if got := episodeTitle(seasonFile); got != "S01E02 Nature's Empty Throne" {
		t.Errorf("season-mapped file = %q", got)
	}
	if info := s.EpisodeInfo(tvLib, seasonFile); info == nil || info.Source != MappedBySeason || info.FileSeason != 5 || info.Title != "Nature's Empty Throne" {
		t.Errorf("episode info = %+v", info)
	}
	if err := s.SetEpisodeMapping(tvLib, pinnedFile, 1, 2, false); err != nil {
		t.Fatal(err)
	}
	if got := episodeTitle(pinnedFile); got != "S01E02 Nature's Empty Throne" {
		t.Errorf("pinned file = %q", got)
	}
	// The original S01E02 file is unaffected by the pin on another file.
	if got := episodeTitle(episodePath); got != "S01E02 Nature's Empty Throne" {
		t.Errorf("unrelated episode = %q", got)
	}
	s.ClearEpisodeMapping(tvLib, pinnedFile, false)
	s.ClearEpisodeMapping(tvLib, seasonFile, true)
	if got := episodeTitle(pinnedFile) + episodeTitle(seasonFile); got != "" {
		t.Errorf("cleared mappings still apply: %q", got)
	}
	if _, err := s.MatchTarget(tvLib, seasonFile, false); err != nil {
		t.Errorf("episode file not fixable: %v", err)
	}
	os.RemoveAll(s5)
	os.Remove(pinnedFile)

	// A title removed from disk loses its lookup and artwork on the next sync.
	os.RemoveAll(knight)
	s.SyncAll(ctx)
	if libs, _ := db.LibrariesForTMDB("movie", 9476); len(libs) != 0 {
		t.Errorf("stale movie lookup still present: %v", libs)
	}
	if _, err := os.Stat(filepath.Join(artwork, "movie", "9476")); !os.IsNotExist(err) {
		t.Errorf("artwork for removed movie not cleaned up: %v", err)
	}
	if m, _ := db.GetMovie(9476); m != nil {
		t.Error("removed movie's record was kept without its artwork")
	}
}

func TestPickResultRequiresTitleMatch(t *testing.T) {
	results := []SearchResult{{ID: 1, Title: "Something Else"}, {ID: 2, Title: "Inception", ReleaseDate: "2010-07-15"}}
	if got := pickResult(results, "Inception", 0, "", false); got != 2 {
		t.Errorf("pickResult = %d, want 2", got)
	}
	if got := pickResult(results[:1], "Inception", 0, "", false); got != 0 {
		t.Errorf("pickResult accepted an unrelated title: %d", got)
	}
	// Top result with the exact year is trusted even when the localized title differs.
	hp := []SearchResult{{ID: 671, Title: "Harry Potter and the Philosopher's Stone", ReleaseDate: "2001-11-16"}}
	if got := pickResult(hp, "Harry Potter and the Sorcerers Stone", 2001, "", false); got != 671 {
		t.Errorf("alternate-title top result rejected: got %d", got)
	}
	if got := pickResult(hp, "Harry Potter and the Sorcerers Stone", 2005, "", false); got != 0 {
		t.Errorf("alternate-title result accepted with the wrong year: got %d", got)
	}
	for _, c := range []struct {
		query, tmdb string
	}{
		{"Tinkerbell and the Great Fairy Rescue", "Tinker Bell and the Great Fairy Rescue"},
		{"Transformers 3 - Dark of the Moon", "Transformers: Dark of the Moon"},
		{"90 Day Fiance", "90 Day Fiancé"},
		{"Big Brother Sweden", "Big Brother (Sweden and Norway)"},
	} {
		if got := pickResult([]SearchResult{{ID: 7, Title: c.tmdb, Name: c.tmdb}}, c.query, 0, "", false); got != 7 {
			t.Errorf("%q did not match %q", c.query, c.tmdb)
		}
	}
	// An exact title (number words = digits) beats one that merely contains the query.
	ff := []SearchResult{
		{ID: 1, Title: "Marvel Studios' The Fantastic Four: First Steps - World Premiere", ReleaseDate: "2025-07-21"},
		{ID: 617126, Title: "The Fantastic 4: First Steps", ReleaseDate: "2025-07-23"},
	}
	if got := pickResult(ff, "The Fantastic Four First Steps", 2025, "", false); got != 617126 {
		t.Errorf("Fantastic Four picked %d, want 617126", got)
	}
	// Word overlap needs every word of the shorter title and at most two extras.
	if titleSimilarity("star wars the empire strikes back", "the empire") != 1 {
		t.Errorf("contained title should be a partial match")
	}
	if titleSimilarity("a b c d e f", "a b x") != 0 {
		t.Errorf("titles with different words should not match")
	}
	shows := []SearchResult{{ID: 10, Name: "Big Brother", OriginCountry: []string{"US"}}, {ID: 11, Name: "Big Brother", OriginCountry: []string{"GB"}}}
	if got := pickResult(shows, "Big Brother", 0, "GB", true); got != 11 {
		t.Errorf("country hint ignored: got %d", got)
	}
}

func waitFor(t *testing.T, cond func() bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal(failure)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
