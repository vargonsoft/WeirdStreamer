package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/guide"
	"github.com/weirdstreamer/server/internal/models"
)

// A typed channel is seen only by the groups that can open every one of its libraries, on the home
// screen's Guide; the Guide tab adds one with an ID and a block of numbers, and keeps both.
func TestTypedChannels(t *testing.T) {
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = saved })
	config.GlobalConfig = &config.Config{Groups: []string{"family", "night"}, Media: config.MediaConfig{Paths: []config.MediaPath{
		{Name: "Shows", ID: "a", Path: "/a", Groups: []string{"family", "night"}, Type: "tv", ShowInGuide: config.ContinueOn},
		{Name: "Folders", ID: "b", Path: "/b", Groups: []string{"night"}},
	}}, Guide: config.GuideConfig{TypedChannels: []config.TypedChannel{
		{ID: "d1", Type: guide.TypedLateNight, Libraries: []string{"a", "b"}, Max: 4, Number: 100},
	}}}
	for _, c := range []database.GuideChannel{
		{Key: "a/shows:comedy", LibraryID: "a", Number: 30, Name: "Comedy", Kind: "shows", Active: true},
		{Key: "typed:d1/series:1", LibraryID: guide.TypedLibraryID("d1"), Number: 100, Name: "WKBT", Kind: "shows", Active: true},
	} {
		if err := db.AddGuideChannel(c); err != nil {
			t.Fatal(err)
		}
	}
	h := NewGuideHandler(db, nil)
	numbers := func(group, url string) []int {
		r := httptest.NewRequest(http.MethodGet, url, nil)
		r = r.WithContext(auth.WithUser(r.Context(), &models.User{ID: 1, Username: "u", Group: group}))
		chans, _ := h.channels(r)
		var out []int
		for _, c := range chans {
			out = append(out, c.Number)
		}
		return out
	}
	if got := numbers("night", "/api/guide"); len(got) != 2 || got[1] != 100 {
		t.Errorf("night's Guide %v", got)
	}
	if got := numbers("family", "/api/guide"); len(got) != 1 || got[0] != 30 {
		t.Errorf("family's Guide %v (can't open library b)", got)
	}
	if got := numbers("night", "/api/guide?lib=a"); len(got) != 1 || got[0] != 30 {
		t.Errorf("library a's own Guide %v", got)
	}

	// The Guide tab: a new typed channel gets an ID and the next block; the old one keeps its own.
	var savedCfg *config.Config
	h.SetConfigSaver(func(c *config.Config) (int, error) {
		if err := c.Validate(); err != nil {
			return http.StatusBadRequest, err
		}
		savedCfg = c
		config.GlobalConfig = c
		return http.StatusOK, nil
	})
	body, _ := json.Marshal(map[string]any{"prime_time": 19, "weather_zip": "02139",
		"libraries": []map[string]any{{"id": "b", "show_in_guide": "", "guide_filler": true}},
		"typed_channels": []map[string]any{
			{"id": "d1", "type": "latenight", "libraries": []string{"b"}, "max": 2},
			{"type": "kids", "libraries": []string{"a"}, "max": 9},
		}})
	r := httptest.NewRequest(http.MethodPut, "/api/admin/guide", bytes.NewReader(body))
	r = r.WithContext(auth.WithClaims(r.Context(), &auth.Claims{IsAdmin: true}))
	w := httptest.NewRecorder()
	h.SaveGuideSettings(w, r)
	if w.Code != 200 || savedCfg == nil {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	tc := savedCfg.Guide.TypedChannels
	if len(tc) != 2 || tc[0].ID != "d1" || tc[0].Number != 100 || tc[0].Max != 2 || tc[1].ID == "" || tc[1].Number != 110 ||
		tc[1].Max != config.TypedChannelMax {
		t.Errorf("typed channels %+v", tc)
	}
	if savedCfg.Guide.PrimeTime != 19 || savedCfg.Guide.WeatherZip != "02139" || !savedCfg.Media.Paths[1].GuideFiller ||
		savedCfg.Media.Paths[0].ShowInGuide != config.ContinueOn {
		t.Errorf("guide %+v, libraries %+v", savedCfg.Guide, savedCfg.Media.Paths)
	}
	var tab guideAdmin
	json.Unmarshal(w.Body.Bytes(), &tab)
	if len(tab.TypedChannels) != 2 || len(tab.TypedChannels[0].Groups) != 1 || tab.TypedChannels[0].Groups[0] != "night" ||
		!tab.Libraries[1].Typed || len(tab.Types) != len(config.TypedChannelTypes) {
		t.Errorf("tab %+v", tab)
	}

	// Not for anyone else
	r = httptest.NewRequest(http.MethodPut, "/api/admin/guide", bytes.NewReader(body))
	w = httptest.NewRecorder()
	h.SaveGuideSettings(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("not an admin: %d", w.Code)
	}
}
