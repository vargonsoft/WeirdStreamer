package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/database"
)

// Wrong passwords get a lock on the username and the address, and a right one can't get past it.
func TestLoginLocksAfterWrongPasswords(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateUser("viewer", "old-pw", false, "normal"); err != nil {
		t.Fatal(err)
	}
	login := NewAuthHandler(auth.NewService(db)).Login

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		login(rec, req)
		return rec
	}
	for i := 0; i < 5; i++ {
		if rec := post(`{"username":"viewer","password":"wrong"}`); rec.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i+1, rec.Code)
		}
	}
	rec := post(`{"username":"viewer","password":"old-pw"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("right password while locked: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"retry_after":30`) {
		t.Errorf("locked response lacks the wait: Retry-After %q, body %s", rec.Header().Get("Retry-After"), rec.Body)
	}
}

// A success before the limit clears the username's failures, so the next wrong passwords start a
// new count. The address's failures stay: signing in doesn't reset guesses made from there.
func TestLoginSuccessResetsUserFailures(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateUser("viewer", "old-pw", false, "normal"); err != nil {
		t.Fatal(err)
	}
	login := NewAuthHandler(auth.NewService(db)).Login

	post := func(addr, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = addr + ":40000"
		rec := httptest.NewRecorder()
		login(rec, req)
		return rec.Code
	}
	for i := 0; i < 4; i++ {
		post("10.0.0.5", `{"username":"viewer","password":"wrong"}`)
	}
	if got := post("10.0.0.5", `{"username":"viewer","password":"old-pw"}`); got != http.StatusOK {
		t.Fatalf("right password after 4 failures: status %d", got)
	}
	for i := 0; i < 4; i++ {
		if got := post("10.0.0.6", `{"username":"viewer","password":"wrong"}`); got != http.StatusUnauthorized {
			t.Fatalf("wrong password %d after success: status %d, want 401", i+1, got)
		}
	}
	// The first address's fifth failure locks it.
	post("10.0.0.5", `{"username":"viewer","password":"wrong"}`)
	if got := post("10.0.0.5", `{"username":"viewer","password":"old-pw"}`); got != http.StatusTooManyRequests {
		t.Fatalf("address with 5 failures: status %d, want 429", got)
	}
}

// Guessing the current password in change-password is limited too.
func TestChangePasswordLocksAfterWrongCurrentPassword(t *testing.T) {
	setupConfig(t)
	dir := t.TempDir()
	db, err := database.New(filepath.Join(dir, "users.db"), filepath.Join(dir, "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateUser("viewer", "old-pw", false, "normal"); err != nil {
		t.Fatal(err)
	}
	service := auth.NewService(db)
	login, err := service.Authenticate("viewer", "old-pw", "test")
	if err != nil {
		t.Fatal(err)
	}
	change := NewAuthHandler(service).AuthMiddleware(http.HandlerFunc(NewUserHandler(db, service).ChangePassword))

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/users/change-password", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		rec := httptest.NewRecorder()
		change.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 5; i++ {
		if got := post(`{"old_password":"wrong","new_password":"new-pw"}`); got != http.StatusForbidden {
			t.Fatalf("wrong current password %d: status %d, want 403", i+1, got)
		}
	}
	if got := post(`{"old_password":"old-pw","new_password":"new-pw"}`); got != http.StatusTooManyRequests {
		t.Fatalf("right current password while locked: status %d, want 429", got)
	}
	if _, err := service.Authenticate("viewer", "old-pw", "test"); err != nil {
		t.Fatal("password changed while locked")
	}
}
