package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"

	"github.com/gorilla/mux"
)

func setupConfig(t *testing.T) (movies, private string) {
	t.Helper()
	root := t.TempDir()
	movies = filepath.Join(root, "movies")
	private = filepath.Join(root, "movies-private")
	os.MkdirAll(movies, 0755)
	os.MkdirAll(private, 0755)
	config.GlobalConfig = &config.Config{
		Media: config.MediaConfig{Paths: []config.MediaPath{
			{Name: "Movies", Path: movies, ID: "movies", Groups: []string{"adults", "normal"}},
			{Name: "Private", Path: private, ID: "private", Groups: []string{"adults"}},
		}},
		Auth: config.AuthConfig{JWTSecret: "test-secret", TokenExpiryHours: 1},
	}
	return movies, private
}

func TestLibraryForPathUsesDirectoryBoundaries(t *testing.T) {
	movies, private := setupConfig(t)

	cases := map[string]string{
		filepath.Join(movies, "a.mp4"):  "movies",
		movies:                          "movies",
		filepath.Join(private, "b.mp4"): "private",
		filepath.Join(movies, "..", "movies-private", "b"): "private",
		movies + "x/c.mp4": "",
		filepath.Join(movies, "..", "elsewhere", "d.mp4"): "",
	}
	for path, want := range cases {
		lib, ok := libraryForPath(path)
		got := ""
		if ok {
			got = lib.ID
		}
		if got != want {
			t.Errorf("libraryForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func newTestAuth(t *testing.T) (*AuthHandler, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.CreateUser("viewer", "pw", false, "normal"); err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(db)
	login, err := service.Authenticate("viewer", "pw")
	if err != nil {
		t.Fatal(err)
	}
	return NewAuthHandler(service), login.Token
}

func TestAuthMiddlewareTokenSources(t *testing.T) {
	setupConfig(t)
	handler, token := newTestAuth(t)
	var gotGroup string
	protected := handler.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotGroup = userGroup(r)
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name   string
		build  func() *http.Request
		status int
	}{
		{"header", func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/api/x", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			return r
		}, http.StatusOK},
		{"cookie on GET", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/thumbnails/x.jpg", nil)
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
			return r
		}, http.StatusOK},
		{"cookie on POST is ignored", func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/api/x", nil)
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
			return r
		}, http.StatusUnauthorized},
		{"query token is ignored", func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/api/x?token="+token, nil)
		}, http.StatusUnauthorized},
		{"bad token", func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
			r.Header.Set("Authorization", "Bearer nope")
			return r
		}, http.StatusUnauthorized},
	}
	for _, c := range cases {
		gotGroup = ""
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, c.build())
		if rec.Code != c.status {
			t.Errorf("%s: status %d, want %d", c.name, rec.Code, c.status)
		}
		if c.status == http.StatusOK && gotGroup != "normal" {
			t.Errorf("%s: user group %q, want %q", c.name, gotGroup, "normal")
		}
	}
}

func TestLoginSetsSessionCookie(t *testing.T) {
	setupConfig(t)
	handler, _ := newTestAuth(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"viewer","password":"pw"}`))
	handler.Login(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status %d", rec.Code)
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			session = c
		}
	}
	if session == nil || session.Value == "" || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode {
		t.Fatalf("missing or insecure session cookie: %+v", session)
	}
}

func TestThumbnailHubFiltersByGroupAndKeepsIDsUnique(t *testing.T) {
	movies, private := setupConfig(t)
	hub := newThumbnailHub()

	normalID, normal := hub.register("normal")
	adultID, adult := hub.register("adults")
	if normalID == adultID {
		t.Fatal("client IDs collide")
	}

	hub.broadcast(filepath.Join(private, "b.mp4"), sseEvent{data: "private"})
	hub.broadcast(filepath.Join(movies, "a.mp4"), sseEvent{data: "movies"})

	if got := (<-normal).data; got != "movies" {
		t.Errorf("normal user got %q, want only the movies event", got)
	}
	if got := (<-adult).data; got != "private" {
		t.Errorf("adult user first event %q, want %q", got, "private")
	}
	if got := (<-adult).data; got != "movies" {
		t.Errorf("adult user second event %q, want %q", got, "movies")
	}

	// Unregistering one client must not affect another, and IDs are not reused.
	hub.unregister(normalID)
	thirdID, _ := hub.register("normal")
	if thirdID == normalID || thirdID == adultID {
		t.Errorf("reused client ID %d", thirdID)
	}
	hub.broadcast(filepath.Join(movies, "c.mp4"), sseEvent{data: "again"})
	if got := (<-adult).data; got != "again" {
		t.Errorf("adult user stopped receiving events: %q", got)
	}
}

func TestServeThumbnailChecksLibraryAccess(t *testing.T) {
	_, private := setupConfig(t)
	thumbDir := t.TempDir()
	config.GlobalConfig.Media.ThumbnailPath = thumbDir

	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	videoPath := filepath.Join(private, "b.mp4")
	hash := database.PathHash(videoPath)
	thumbPath, thumbURL := media.GetThumbnailPath(thumbDir, hash, ".jpg")
	os.MkdirAll(filepath.Dir(thumbPath), 0755)
	os.WriteFile(thumbPath, []byte("jpeg"), 0644)
	if err := db.SaveMediaItem(&models.MediaItem{Path: videoPath, Name: "b.mp4", Type: "video", MimeType: "video/mp4", ThumbnailURL: thumbURL}); err != nil {
		t.Fatal(err)
	}

	h := NewMediaHandler(db, auth.NewService(db))
	router := mux.NewRouter()
	router.HandleFunc("/thumbnails/{filename:.*}", h.ServeThumbnail)

	for group, want := range map[string]int{"adults": http.StatusOK, "normal": http.StatusNotFound} {
		req := httptest.NewRequest(http.MethodGet, thumbURL, nil)
		req = req.WithContext(auth.WithUser(req.Context(), &models.User{Username: "u", Group: group}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("group %s: status %d, want %d", group, rec.Code, want)
		}
	}
}

func TestServeArtworkChecksLibraryAccess(t *testing.T) {
	setupConfig(t)
	artworkDir := t.TempDir()
	config.GlobalConfig.Media.ArtworkPath = artworkDir
	os.MkdirAll(filepath.Join(artworkDir, "movie", "603"), 0755)
	os.WriteFile(filepath.Join(artworkDir, "movie", "603", "poster.jpg"), []byte("jpeg"), 0644)
	os.MkdirAll(filepath.Join(artworkDir, "person"), 0755)
	os.WriteFile(filepath.Join(artworkDir, "person", "1.jpg"), []byte("jpeg"), 0644)

	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// The movie is only in the adults-only "private" library.
	db.SaveLookup(&database.Lookup{Kind: "movie", LibraryID: "private", QueryKey: "the matrix|1999",
		QueryTitle: "The Matrix", TMDBID: 603, Status: "matched", AttemptedAt: time.Now()})

	h := NewMediaHandler(db, auth.NewService(db))
	router := mux.NewRouter()
	router.HandleFunc("/artwork/{kind:movie|tv}/{id:[0-9]+}/{file}", h.ServeArtwork)
	router.HandleFunc("/artwork/{kind:person}/{file}", h.ServeArtwork)

	cases := []struct {
		group, url string
		want       int
	}{
		{"adults", "/artwork/movie/603/poster.jpg", http.StatusOK},
		{"normal", "/artwork/movie/603/poster.jpg", http.StatusNotFound},
		{"adults", "/artwork/movie/604/poster.jpg", http.StatusNotFound},
		{"normal", "/artwork/person/1.jpg", http.StatusOK},
		{"adults", "/artwork/movie/603/..%2Fx.jpg", 0}, // any non-200: mux redirects to a cleaned path
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.url, nil)
		req = req.WithContext(auth.WithUser(req.Context(), &models.User{Username: "u", Group: c.group}))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if (c.want == 0 && rec.Code == http.StatusOK) || (c.want != 0 && rec.Code != c.want) {
			t.Errorf("%s as %s: status %d, want %d", c.url, c.group, rec.Code, c.want)
		}
	}
}
