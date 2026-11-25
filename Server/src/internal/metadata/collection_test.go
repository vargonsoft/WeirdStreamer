package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// A movie in a TMDB collection: the collection and its films are stored, the listing names it, and
// its page has every film, the library's first and the rest missing.
func TestCollections(t *testing.T) {
	var movieDetails atomic.Int32
	responses := map[string]string{
		"/3/search/movie": `{"results":[{"id":603,"title":"The Matrix","release_date":"1999-03-30"}]}`,
		"/3/movie/603": `{"id":603,"title":"The Matrix","release_date":"1999-03-30","poster_path":"/m.jpg","backdrop_path":"/mb.jpg",
			"belongs_to_collection":{"id":2344,"name":"The Matrix Collection","poster_path":"/c.jpg"}}`,
		"/3/collection/2344": `{"id":2344,"name":"The Matrix Collection","overview":"Neo, three times.","poster_path":"/c.jpg",
			"backdrop_path":"/cb.jpg","parts":[
			{"id":605,"title":"The Matrix Revolutions","release_date":"2003-11-05","poster_path":"/r2.jpg"},
			{"id":603,"title":"The Matrix","release_date":"1999-03-30","poster_path":"/m.jpg"},
			{"id":604,"title":"The Matrix Reloaded","release_date":"2003-05-15","poster_path":"/r1.jpg"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/t/p/") {
			w.Write([]byte("image:" + r.URL.Path))
			return
		}
		if r.URL.Path == "/3/movie/603" {
			movieDetails.Add(1)
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	root := t.TempDir()
	movies := filepath.Join(root, "Movies")
	artwork := filepath.Join(root, "artwork")
	os.MkdirAll(filepath.Join(movies, "The Matrix (1999)"), 0755)
	os.WriteFile(filepath.Join(movies, "The Matrix (1999)", "The Matrix.m4v"), nil, 0644)
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
	waitFor(t, func() bool { m, _ := db.GetMovie(603); return m != nil }, "movie lookup did not finish")

	m, _ := db.GetMovie(603)
	if m.CollectionID != 2344 {
		t.Fatalf("collection of the movie: %d", m.CollectionID)
	}
	c, _ := db.GetCollection(2344)
	if c == nil || c.Name != "The Matrix Collection" || len(c.Parts) != 3 || c.Parts[0].TMDBID != 603 || c.Parts[2].TMDBID != 605 {
		t.Fatalf("stored collection: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(artwork, "collection", "2344", "604.jpg")); err != nil {
		t.Errorf("a missing film's poster wasn't saved: %v", err)
	}

	info := func(path string) models.LibraryFile { return models.LibraryFile{Path: path} }
	listing, err := s.Library(ctx, lib, info)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Titles) != 1 || listing.Titles[0].CollectionID != 2344 {
		t.Fatalf("titles: %+v", listing.Titles)
	}
	if len(listing.Collections) != 1 || listing.Collections[0].Count != 1 || listing.Collections[0].Total != 3 ||
		!strings.HasPrefix(listing.Collections[0].Poster, "/artwork/collection/2344/poster.jpg") {
		t.Fatalf("collections: %+v", listing.Collections)
	}
	page, err := s.Collection(listing, 2344)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Parts) != 3 || page.Parts[0].TMDBID != 603 || page.Parts[0].Missing ||
		!page.Parts[1].Missing || page.Parts[1].Title != "The Matrix Reloaded" || page.Parts[1].Year != 2003 {
		t.Fatalf("collection page: %+v", page.Parts)
	}
	if _, err := s.Collection(listing, 9999); err != ErrNoCollection {
		t.Errorf("a collection the library has nothing of: %v", err)
	}

	// A made collection: the library's titles by release date (undated last, gone ones left out),
	// the collage the first four posters
	listing.Titles = append(listing.Titles,
		models.LibraryTitle{Kind: "movie", TMDBID: 777, Title: "Undated", Poster: "/artwork/movie/777/poster.jpg"})
	made := database.UserCollection{ID: 1, Name: "Mine", Titles: []database.CollectionTitle{
		{Kind: "movie", TMDBID: 777}, {Kind: "movie", TMDBID: 603}, {Kind: "movie", TMDBID: 404}}}
	summaries := s.UserCollections(listing, []database.UserCollection{made})
	if len(summaries) != 1 || summaries[0].Count != 2 || len(summaries[0].Posters) != 2 ||
		summaries[0].Titles[0].TMDBID != 603 || summaries[0].Titles[1].TMDBID != 777 {
		t.Fatalf("made collection: %+v", summaries)
	}
	if page := s.UserCollection(listing, made); len(page.Parts) != 2 || page.Parts[0].TMDBID != 603 {
		t.Fatalf("made collection's page: %+v", page.Parts)
	}

	// A movie stored before collections were is looked up again by its light details request only
	before := movieDetails.Load()
	db.SetMovieCollection(603, -1)
	if !s.needsWork(&database.Lookup{Kind: lib.Type, LibraryID: lib.ID, QueryKey: listingKey(t, s, lib)}, nil) {
		t.Error("a movie with an unknown collection isn't due")
	}
	if _, err := s.ensureMovie(ctx, s.tmdb(), 603); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.GetMovie(603); m.CollectionID != 2344 || movieDetails.Load() != before+1 {
		t.Errorf("after catching up: collection %d, %d details requests", m.CollectionID, movieDetails.Load()-before)
	}
}

// listingKey is the lookup key of the library's one title.
func listingKey(t *testing.T, s *Service, lib config.MediaPath) string {
	idx, err := s.libraryIndexFor(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}
	for key := range idx.files {
		return key
	}
	t.Fatal("no title in the library")
	return ""
}
