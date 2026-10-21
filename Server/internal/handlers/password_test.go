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

// Changing a password needs the current one, so a signed-in session alone can't take over the
// account.
func TestChangePasswordRequiresCurrentPassword(t *testing.T) {
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
	login, err := service.Authenticate("viewer", "old-pw")
	if err != nil {
		t.Fatal(err)
	}
	authHandler := NewAuthHandler(service)
	change := authHandler.AuthMiddleware(http.HandlerFunc(NewUserHandler(db).ChangePassword))

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/users/change-password", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+login.Token)
		rec := httptest.NewRecorder()
		change.ServeHTTP(rec, req)
		return rec.Code
	}
	for body, want := range map[string]int{
		`{"old_password":"","new_password":"new-pw"}`:      http.StatusBadRequest,
		`{"old_password":"wrong","new_password":"new-pw"}`: http.StatusForbidden,
	} {
		if got := post(body); got != want {
			t.Errorf("%s: status %d, want %d", body, got, want)
		}
	}
	if _, err := service.Authenticate("viewer", "old-pw"); err != nil {
		t.Fatal("password changed without the right current password")
	}
	if got := post(`{"old_password":"old-pw","new_password":"new-pw"}`); got != http.StatusOK {
		t.Fatalf("change with the right current password: status %d", got)
	}
	if _, err := service.Authenticate("viewer", "new-pw"); err != nil {
		t.Errorf("new password doesn't work: %v", err)
	}
}
