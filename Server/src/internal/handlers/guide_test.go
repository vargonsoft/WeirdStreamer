package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/guide"
	"github.com/weirdstreamer/server/internal/models"
)

// The Guide's grid shows each program until the next, what's on now names the file and how far
// in, and a user sees only the channels of libraries their group may see, where they're in the Guide.
func TestGuideListingsAndNow(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = saved })
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{Paths: []config.MediaPath{
		{Name: "TV", ID: "tv1", Path: "/tv", Groups: []string{"family"}, Type: "tv", ShowInGuide: config.ContinueOn},
		{Name: "Private", ID: "tv2", Path: "/p", Groups: []string{"adults"}, Type: "tv", ShowInGuide: config.ContinueLibraryOnly},
	}}}
	for _, c := range []database.GuideChannel{
		{Key: "tv1/shows:comedy", LibraryID: "tv1", Number: 14, Name: "Comedy", Kind: "shows"},
		{Key: "tv2/shows:drama", LibraryID: "tv2", Number: 15, Name: "Drama", Kind: "shows"},
	} {
		if err := db.AddGuideChannel(c); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().Truncate(time.Minute).Add(-10 * time.Minute)
	info := `{"kind":"show","tmdb_id":1,"title":"Starlight Detectives","season":1,"episode":2}`
	if err := db.AddGuideSlots([]database.GuideSlot{
		{Channel: "tv1/shows:comedy", Start: start, Length: 22 * time.Minute, Kind: guide.SlotProgram, Title: "show:1",
			Item: 1, Path: "tv1/Starlight/S01E02.mkv", Track: "strip", Info: info},
		{Channel: "tv1/shows:comedy", Start: start.Add(22 * time.Minute), Length: 30 * time.Second, Kind: guide.SlotFiller,
			Path: "ads/1987/Soup.mp4"},
		{Channel: "tv1/shows:comedy", Start: start.Add(22*time.Minute + 30*time.Second), Length: 7*time.Minute + 30*time.Second,
			Kind: guide.SlotBumper},
		{Channel: "tv1/shows:comedy", Start: start.Add(30 * time.Minute), Length: 22 * time.Minute, Kind: guide.SlotProgram,
			Title: "show:2", Path: "tv1/Harbor/S01E01.mkv", Info: `{"kind":"show","tmdb_id":2,"title":"Harbor Street"}`},
	}); err != nil {
		t.Fatal(err)
	}
	h := NewGuideHandler(db, nil)
	get := func(group, url string, handler http.HandlerFunc) (int, map[string]any) {
		r := httptest.NewRequest(http.MethodGet, url, nil)
		r = r.WithContext(auth.WithUser(r.Context(), &models.User{ID: 1, Username: "u", Group: group}))
		w := httptest.NewRecorder()
		handler(w, r)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}

	_, body := get("family", "/api/guide?from="+strconv.FormatInt(start.UnixMilli(), 10)+"&hours=1", h.Listings)
	chans := body["channels"].([]any)
	if len(chans) != 1 {
		t.Fatalf("channels = %v", chans)
	}
	listings := chans[0].(map[string]any)["listings"].([]any)
	first := listings[0].(map[string]any)
	if len(listings) != 2 || int64(first["end"].(float64)) != start.Add(30*time.Minute).UnixMilli() ||
		first["info"].(map[string]any)["title"] != "Starlight Detectives" {
		t.Errorf("listings = %v", listings)
	}

	code, now := get("family", "/api/guide/now?channel=14", h.Now)
	if code != 200 || now["kind"] != guide.SlotProgram || now["path"] != "tv1/Starlight/S01E02.mkv" ||
		now["offset"].(float64) < 590 || now["next"].(map[string]any)["title"] != "Harbor Street" {
		t.Errorf("now = %d %v", code, now)
	}

	// Another group sees no channel; the private library's is only on its own page.
	if code, _ := get("family", "/api/guide/now?channel=15", h.Now); code != 404 {
		t.Errorf("another library's channel: %d", code)
	}
	if _, body := get("adults", "/api/guide/available", h.Available); body["available"] != false {
		t.Errorf("home Guide for adults: %v", body)
	}
	if _, body := get("adults", "/api/guide/available?lib=tv2", h.Available); body["available"] != true {
		t.Errorf("tv2's Guide: %v", body)
	}
	if code, _ := get("family", "/api/guide?lib=tv2", h.Listings); code != 404 {
		t.Errorf("a library the group can't see: %d", code)
	}

	// Each channel has a logo, as PNG (at the width asked) or SVG.
	logo := func(url string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, url, nil)
		r = r.WithContext(auth.WithUser(r.Context(), &models.User{ID: 1, Username: "u", Group: "family"}))
		w := httptest.NewRecorder()
		h.Logo(w, r)
		return w
	}
	if w := logo("/api/guide/logo?channel=14&w=128"); w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Body.Len() < 1000 {
		t.Errorf("logo: %d %s, %d bytes", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	if w := logo("/api/guide/logo?channel=14&format=svg"); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<svg") {
		t.Errorf("SVG logo: %d", w.Code)
	}
	if !strings.Contains(chans[0].(map[string]any)["logo"].(string), "/api/guide/logo?channel=14") {
		t.Errorf("channel without its logo: %v", chans[0])
	}
}
