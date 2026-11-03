package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
)

// Making a group the default moves it to the front, keeps the others' order, and is saved.
func TestMakeDefaultGroup(t *testing.T) {
	old := config.GlobalConfig
	defer func() { config.GlobalConfig = old }()
	path := filepath.Join(t.TempDir(), "config.yml")
	body := "# the groups\ngroups: [\"parents\", \"admins\", \"kids\"] # first is the default\n" +
		"media:\n  thumbnailPath: ./thumbs\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	h := NewConfigHandler(nil)
	call := func(name string, admin bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/api/groups/"+name+"/default", nil)
		req = mux.SetURLVars(req, map[string]string{"name": name})
		req = req.WithContext(auth.WithClaims(req.Context(), &auth.Claims{Username: "u", IsAdmin: admin}))
		rec := httptest.NewRecorder()
		h.MakeDefaultGroup(rec, req)
		return rec
	}

	rec := call("kids", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var groups []string
	json.Unmarshal(rec.Body.Bytes(), &groups)
	want := []string{"kids", "parents", "admins"}
	if !reflect.DeepEqual(groups, want) || !reflect.DeepEqual(config.GlobalConfig.Groups, want) {
		t.Errorf("groups %v (config %v), want %v", groups, config.GlobalConfig.Groups, want)
	}
	if config.GlobalConfig.DefaultGroup() != "kids" {
		t.Errorf("default %q", config.GlobalConfig.DefaultGroup())
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), `groups: ["kids", "parents", "admins"] # first is the default`) {
		t.Errorf("saved config:\n%s", saved)
	}

	if rec := call("nobody", true); rec.Code != http.StatusNotFound {
		t.Errorf("unknown group: status %d", rec.Code)
	}
	if rec := call("parents", false); rec.Code != http.StatusForbidden {
		t.Errorf("not an admin: status %d", rec.Code)
	}
}
