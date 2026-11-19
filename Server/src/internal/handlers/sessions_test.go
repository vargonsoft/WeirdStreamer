package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

type sessionHarness struct {
	t       *testing.T
	db      *database.DB
	service *auth.Service
	auth    *AuthHandler
	users   *UserHandler
}

func newSessionHarness(t *testing.T) *sessionHarness {
	t.Helper()
	setupConfig(t)
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	service := auth.NewService(db)
	return &sessionHarness{t: t, db: db, service: service, auth: NewAuthHandler(service), users: NewUserHandler(db, service)}
}

func (h *sessionHarness) signIn(user, password string) string {
	h.t.Helper()
	login, err := h.service.Authenticate(user, password, "test")
	if err != nil {
		h.t.Fatal(err)
	}
	return login.Token
}

// call runs handler behind the auth middleware with token, returning the status.
func (h *sessionHarness) call(handler http.HandlerFunc, method, url, token, body string) int {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.auth.AuthMiddleware(handler).ServeHTTP(rec, req)
	return rec.Code
}

func (h *sessionHarness) works(token string) bool {
	return h.call(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) },
		http.MethodGet, "/api/media", token, "") == http.StatusOK
}

func TestSignOutEndsOnlyThatSession(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("viewer", "pw", false, "normal")
	phone, tv := h.signIn("viewer", "pw"), h.signIn("viewer", "pw")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+phone)
	h.auth.Logout(httptest.NewRecorder(), req)

	if h.works(phone) {
		t.Error("the signed-out token still works")
	}
	if !h.works(tv) {
		t.Error("signing out one device signed out the other")
	}
}

func TestNewPasswordEndsOtherSessions(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("viewer", "old-pw", false, "normal")
	here, elsewhere := h.signIn("viewer", "old-pw"), h.signIn("viewer", "old-pw")

	if got := h.call(h.users.ChangePassword, http.MethodPost, "/api/users/change-password", here,
		`{"old_password":"old-pw","new_password":"new-pw"}`); got != http.StatusOK {
		t.Fatalf("change: %d", got)
	}
	if !h.works(here) {
		t.Error("the session that changed the password was signed out")
	}
	if h.works(elsewhere) {
		t.Error("another session survived the new password")
	}

	// The command line's reset ends them all.
	if err := h.db.UpdateUserPassword("viewer", "third-pw", ""); err != nil {
		t.Fatal(err)
	}
	if h.works(here) {
		t.Error("a session survived a password reset")
	}
}

func TestDemotedAdminLosesAdminAtOnce(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("boss", "pw", true, "normal")
	h.db.CreateUser("other", "pw", true, "normal")
	token := h.signIn("boss", "pw")
	list := func() int { return h.call(h.users.ListUsers, http.MethodGet, "/api/users", token, "") }
	if got := list(); got != http.StatusOK {
		t.Fatalf("as admin: %d", got)
	}
	no := false
	h.db.UpdateUser("boss", models.UserUpdateRequest{IsAdmin: &no})
	if got := list(); got != http.StatusForbidden {
		t.Errorf("after demotion: %d, want 403", got)
	}
}

func TestDeletedUsersTokenDoesntWorkForANewUserOfTheName(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("viewer", "pw", false, "normal")
	old := h.signIn("viewer", "pw")
	h.db.DeleteUser("viewer")
	if h.works(old) {
		t.Error("a deleted user's token works")
	}
	h.db.CreateUser("viewer", "pw2", false, "normal")
	if h.works(old) {
		t.Error("the old token works for the new user of the same name")
	}
}

func TestUsernamesTakeEmailCharactersOnly(t *testing.T) {
	h := newSessionHarness(t)
	config.GlobalConfig.Groups = []string{"normal"}
	h.db.CreateUser("boss", "pw", true, "normal")
	token := h.signIn("boss", "pw")
	create := func(name string) int {
		body, _ := json.Marshal(map[string]any{"username": name, "password": "pw", "group": "normal"})
		return h.call(h.users.CreateUser, http.MethodPost, "/api/users", token, string(body))
	}
	for _, name := range []string{"ann", "ann.lee@example.com", "o'brien+tv", "a_b-c~d"} {
		if got := create(name); got != http.StatusCreated {
			t.Errorf("%q: %d, want 201", name, got)
		}
	}
	for _, name := range []string{"<img src=x onerror=alert(1)>", "two words", `quo"te`, "semi;colon", "tab\there", "émile", strings.Repeat("a", 255)} {
		if got := create(name); got != http.StatusBadRequest {
			t.Errorf("%q: %d, want 400", name, got)
		}
	}
	if _, err := h.db.CreateUser("<b>", "pw", false, "normal"); err == nil {
		t.Error("the database took a username with < and >")
	}
}

func TestSettingsNeverGetTheJWTSecret(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("boss", "pw", true, "normal")
	token := h.signIn("boss", "pw")
	secret := config.GlobalConfig.Auth.JWTSecret
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.auth.AuthMiddleware(http.HandlerFunc(NewConfigHandler(h.db).GetConfig)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "jwtSecret") {
		t.Error("the config sent to Settings has the JWT secret")
	}
	if config.GlobalConfig.Auth.JWTSecret != secret {
		t.Error("sending the config changed the server's secret")
	}
}

func TestStartupRefusesWeakJWTSecrets(t *testing.T) {
	for secret, ok := range map[string]bool{"": false, "  ": false, config.ExampleJWTSecret: false, "3f9c0d1e2a7b": true} {
		c := &config.Config{Auth: config.AuthConfig{JWTSecret: secret}}
		if err := c.CheckJWTSecret(); (err == nil) != ok {
			t.Errorf("%q: %v", secret, err)
		}
	}
}

func TestSettingsChangeOnlyTheirOwnFields(t *testing.T) {
	movies, _ := setupConfig(t)
	current := config.GlobalConfig
	current.Media.SubsetCommand = "pyftsubset"
	current.Media.ThumbnailPath = "/data/thumbs"
	current.Groups = []string{"adults", "normal"}
	body := `{"server":{"name":"Den","port":9000,"host":"0.0.0.0","certFile":"/etc/shadow"},
		"auth":{"jwtSecret":"mine"},"groups":["everyone"],
		"metadata":{"tmdbAccessToken":"tok","language":"de-DE"},
		"media":{"subsetCommand":"/bin/sh","thumbnailPath":"/","paths":[{"name":"Films","path":"` + movies + `","id":"movies","groups":["normal"],"showNewAdditions":"on"}]}}`
	var u settingsUpdate
	if err := json.Unmarshal([]byte(body), &u); err != nil {
		t.Fatal(err)
	}
	c := u.applyTo(current)
	if c.Server.Name != "Den" || c.Metadata.TMDBAccessToken != "tok" || c.Metadata.Language != "de-DE" ||
		len(c.Media.Paths) != 1 || c.Media.Paths[0].Name != "Films" || c.Media.Paths[0].ShowNewAdditions != "on" {
		t.Errorf("Settings' own fields weren't applied: %+v", c)
	}
	if c.Media.SubsetCommand != "pyftsubset" || c.Media.ThumbnailPath != "/data/thumbs" || c.Server.CertFile != "" ||
		c.Auth.JWTSecret != "test-secret" || len(c.Groups) != 2 {
		t.Errorf("a field Settings doesn't show was changed: %+v", c)
	}
	if len(current.Media.Paths) != 2 {
		t.Error("the current config was changed")
	}
}

func TestWebSignInKeepsTheTokenInTheCookie(t *testing.T) {
	h := newSessionHarness(t)
	h.db.CreateUser("viewer", "pw", false, "normal")
	rec := httptest.NewRecorder()
	h.auth.Login(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"username":"viewer","password":"pw","web":true}`)))
	var reply map[string]any
	json.Unmarshal(rec.Body.Bytes(), &reply)
	if rec.Code != http.StatusOK || reply["token"] != "" {
		t.Fatalf("status %d, token in the reply: %v", rec.Code, reply["token"])
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Value == "" {
		t.Fatalf("cookie: %+v", cookies)
	}
	call := func(method string, header bool) int {
		req := httptest.NewRequest(method, "/api/x", nil)
		req.AddCookie(cookies[0])
		if header {
			req.Header.Set(webClientHeader, "1")
		}
		rec := httptest.NewRecorder()
		h.auth.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call(http.MethodGet, false); got != http.StatusOK {
		t.Errorf("GET with the cookie: %d", got)
	}
	if got := call(http.MethodPost, true); got != http.StatusOK {
		t.Errorf("POST with the cookie and the web header: %d", got)
	}
	if got := call(http.MethodPost, false); got != http.StatusUnauthorized {
		t.Errorf("POST with only the cookie (another site's form): %d, want 401", got)
	}
}
