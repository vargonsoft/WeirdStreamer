package fileio

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAll(t *testing.T, f *os.File) string {
	t.Helper()
	b := make([]byte, 64)
	n, err := f.ReadAt(b, 0)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	return string(b[:n])
}

// An open file can be renamed, replaced and deleted, and reads on.
func TestOpenFileCanMove(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	write(t, a, "first")
	f, err := Open(a)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Rename(a, b); err != nil {
		t.Fatalf("renaming an open file: %v", err)
	}
	if got := readAll(t, f); got != "first" {
		t.Errorf("read %q after the rename", got)
	}

	// A new file renamed over the open one (a thumbnail remade while it's sent).
	g, err := Open(b)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	write(t, a, "second")
	if err := Rename(a, b); err != nil {
		t.Fatalf("renaming over an open file: %v", err)
	}
	if got, _ := ReadFile(b); string(got) != "second" {
		t.Errorf("the new file reads %q", got)
	}
	if got := readAll(t, g); got != "first" {
		t.Errorf("the replaced file's open handle reads %q", got)
	}

	// Deleted while open: the name is free again at once.
	h, err := Open(b)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if err := os.Remove(b); err != nil {
		t.Fatalf("deleting an open file: %v", err)
	}
	if _, err := os.Stat(b); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the deleted file's name is still there: %v", err)
	}
	write(t, b, "third")
	if got := readAll(t, h); got != "second" {
		t.Errorf("the deleted file's open handle reads %q", got)
	}

	// A folder holding an open file can be removed.
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	write(t, filepath.Join(sub, "c"), "x")
	c, err := Open(filepath.Join(sub, "c"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := os.RemoveAll(sub); err != nil {
		t.Errorf("removing a folder with an open file: %v", err)
	}
}

func TestOpenFileFlags(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if _, err := Open(p); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("opening a missing file: %v", err)
	}
	f, err := Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello world")
	f.Close()
	f, _ = OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	f.WriteString("!")
	f.Close()
	if got, _ := ReadFile(p); string(got) != "hello world!" {
		t.Errorf("after appending: %q", got)
	}
	f, _ = Create(p) // truncates
	f.WriteString("hi")
	f.Close()
	if got, _ := ReadFile(p); string(got) != "hi" {
		t.Errorf("after Create over it: %q", got)
	}
	if _, err := OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); !errors.Is(err, fs.ErrExist) {
		t.Errorf("O_EXCL over a file: %v", err)
	}
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	names, err := d.Readdirnames(-1)
	d.Close()
	if err != nil || len(names) != 1 || names[0] != "f" {
		t.Errorf("folder read as %v, %v", names, err)
	}
	if CrossDevice(errors.New("in use")) {
		t.Error("any error counts as another disk")
	}
}

func TestServe(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "index.html"), "<p>home</p>")
	write(t, filepath.Join(dir, "a.txt"), "0123456789")
	get := func(h http.Handler, url, rng string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", url, nil)
		if rng != "" {
			r.Header.Set("Range", rng)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	files := http.FileServer(Dir(dir))
	if w := get(files, "/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "home") {
		t.Errorf("/: %d %q", w.Code, w.Body.String())
	}
	if w := get(files, "/missing.js", ""); w.Code != 404 {
		t.Errorf("a missing file: %d", w.Code)
	}
	one := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ServeFile(w, r, name) })
	}
	if w := get(one(filepath.Join(dir, "a.txt")), "/x", "bytes=2-4"); w.Code != 206 || w.Body.String() != "234" {
		t.Errorf("a range: %d %q", w.Code, w.Body.String())
	}
	if w := get(one(filepath.Join(dir, "nope")), "/x", ""); w.Code != 404 {
		t.Errorf("a missing file: %d", w.Code)
	}
	if w := get(one(dir), "/x", ""); w.Code != 404 {
		t.Errorf("a folder: %d", w.Code)
	}
}
