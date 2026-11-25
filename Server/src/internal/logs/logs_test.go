package logs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingMovesFullFilesAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.log")
	r, err := openRotating(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	line := []byte(strings.Repeat("x", 1<<20-1) + "\n")
	for range 2*maxSize/len(line) + 1 {
		if _, err := r.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{path, path + ".1", path + ".2"} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if info.Size() > maxSize {
			t.Errorf("%s is %d bytes, over %d", name, info.Size(), maxSize)
		}
	}
}

func TestAccessLogsUserStatusAndSize(t *testing.T) {
	dir := t.TempDir()
	if err := Open(dir); err != nil {
		t.Fatal(err)
	}
	defer Close()
	h := Access(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		SetUser(r, "ann")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("hello"))
	}))
	req := httptest.NewRequest("GET", "/api/media/stream?path=abc", nil)
	req.Header.Set("User-Agent", "test")
	h.ServeHTTP(httptest.NewRecorder(), req)
	data, err := os.ReadFile(filepath.Join(dir, "access.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{" - ann [", `"GET /api/media/stream?path=abc HTTP/1.1" 206 5 "-" "test"`} {
		if !strings.Contains(got, want) {
			t.Errorf("access.log %q lacks %q", got, want)
		}
	}
}

func TestAuthOnceSkipsRepeats(t *testing.T) {
	dir := t.TempDir()
	if err := Open(dir); err != nil {
		t.Fatal(err)
	}
	defer Close()
	for range 3 {
		AuthOnce("token|1.2.3.4", "Token refused from %s", "1.2.3.4")
	}
	AuthOnce("token|5.6.7.8", "Token refused from %s", "5.6.7.8")
	data, _ := os.ReadFile(filepath.Join(dir, "auth.log"))
	if n := strings.Count(string(data), "Token refused"); n != 2 {
		t.Errorf("%d lines, want 2:\n%s", n, data)
	}
}
