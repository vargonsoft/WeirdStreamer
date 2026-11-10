package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/models"
)

// Clients know a file by its library's ID and its path below the library, never by where the
// library is on disk.
func TestClientPaths(t *testing.T) {
	movies, private := setupConfig(t)

	out := map[string]string{
		filepath.Join(movies, "EAC3 Test", "EAC3 Test.mkv"): "movies/EAC3 Test/EAC3 Test.mkv",
		movies:                           "movies",
		filepath.Join(private, "b.mp4"):  "private/b.mp4",
		filepath.Join(movies, "..", "x"): "", // outside every library
		"":                               "",
	}
	for disk, want := range out {
		if got := clientPath(disk); got != want {
			t.Errorf("clientPath(%q) = %q, want %q", disk, got, want)
		}
	}

	in := map[string]string{
		"movies/EAC3 Test/EAC3 Test.mkv": filepath.Join(movies, "EAC3 Test", "EAC3 Test.mkv"),
		"movies":                         movies,
		"private/b.mp4":                  filepath.Join(private, "b.mp4"),
		// A disk path names nothing, even a real file's; nor does a path that climbs out of its
		// library, into another one or anywhere else.
		filepath.Join(movies, "a.mp4"):  "",
		"movies/../movies-private/b":    "",
		"movies/../../etc/passwd":       "",
		"nosuch/a.mp4":                  "",
		"":                              "",
		"/":                             "",
		"/movies/a.mp4":                 "",
		strings.TrimPrefix(movies, "/"): "",
	}
	for client, want := range in {
		got, _, ok := diskPath(client)
		if got != want || ok != (want != "") {
			t.Errorf("diskPath(%q) = %q, %v; want %q", client, got, ok, want)
		}
	}
}

// What a response carries of a file has no disk path in it.
func TestResponsesHaveNoDiskPaths(t *testing.T) {
	movies, _ := setupConfig(t)
	photo := filepath.Join(movies, "Trip", "IMG_0001.HEIC")
	items := []models.MediaItem{{Path: photo, Name: "IMG_0001.HEIC", LiveVideo: filepath.Join(movies, "Trip", "IMG_0001.MOV")}}
	clientItems(items)
	files := []models.LibraryFile{{Path: photo, Name: "IMG_0001.HEIC"}}
	clientFiles(files)
	continued := []continueItem{{MediaItem: models.MediaItem{Path: photo}, LibraryID: "movies"}}
	clientContinue(continued)
	text := withoutDiskPath("ffmpeg: "+photo+": Invalid data found", photo)
	body, _ := json.Marshal([]any{items, files, continued, clientItem(&models.MediaItem{Path: photo}), text})
	if strings.Contains(string(body), movies) {
		t.Errorf("a response has the library's disk path: %s", body)
	}
	if !strings.Contains(string(body), `"path":"movies/Trip/IMG_0001.HEIC"`) || !strings.Contains(string(body), `"live_video":"movies/Trip/IMG_0001.MOV"`) {
		t.Errorf("a response lost its client paths: %s", body)
	}
}

// A request's path is a client path in base64; a disk path in base64 (what clients sent before)
// is refused, and so is a library the user's group can't see.
func TestFilePathRefusesDiskPaths(t *testing.T) {
	movies, private := setupConfig(t)
	h := &MediaHandler{}
	encode := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	normal := requestAs(t, "normal")

	if got, status := h.filePath(normal, encode("movies/a.mp4")); got != filepath.Join(movies, "a.mp4") || status != 200 {
		t.Errorf("a client path gave %q, %d", got, status)
	}
	for name, sent := range map[string]string{
		"a disk path":       encode(filepath.Join(movies, "a.mp4")),
		"another library's": encode("private/b.mp4"),
		"its disk path":     encode(filepath.Join(private, "b.mp4")),
	} {
		if got, status := h.filePath(normal, sent); got != "" || status != 403 {
			t.Errorf("%s gave %q, %d; want 403", name, got, status)
		}
	}
	if _, status := h.filePath(normal, "not base64!"); status != 400 {
		t.Errorf("bad base64 gave %d, want 400", status)
	}
}

// requestAs is a request from a signed-in member of group.
func requestAs(t *testing.T, group string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	return r.WithContext(auth.WithUser(r.Context(), &models.User{Username: "u", Group: group}))
}
