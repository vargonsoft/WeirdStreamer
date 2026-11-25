package config

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestClientPartsOKWindows(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Some Long Folder Name")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Movie.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ok := []string{
		"Some Long Folder Name/Movie.mkv",
		"Some Long Folder Name",
		"Some Long Folder Name/new.srt", // doesn't exist yet: its own name
		"Missing/Any Case.mkv",
		"Some Long Folder Name/Movie.mkv.part.jpg",
		"CONSOLE.mkv", "COM10.mkv", "a.b.c",
	}
	bad := []string{
		"some long folder name/Movie.mkv",
		"Some Long Folder Name/movie.MKV",
		"Some Long Folder Name./Movie.mkv",
		"Some Long Folder Name /Movie.mkv",
		"Some Long Folder Name/Movie.mkv:stream",
		`Some Long Folder Name\Movie.mkv`,
		"C:/Windows/win.ini",
		"Some Long Folder Name/../Some Long Folder Name/Movie.mkv",
		"Some Long Folder Name/Movie*.mkv",
		"CON", "nul.txt", "Some Long Folder Name/COM1.mkv", "lpt9", "AUX .mkv", "COM¹",
	}
	if short := shortName(t, dir); short != "" {
		bad = append(bad, short+"/Movie.mkv")
	}
	for _, rel := range ok {
		if !clientPartsOK(root, rel) {
			t.Errorf("refused %q", rel)
		}
	}
	for _, rel := range bad {
		if clientPartsOK(root, rel) {
			t.Errorf("allowed %q", rel)
		}
	}
	libs := []MediaPath{{ID: "lib", Path: root}}
	if _, _, ok := DiskPathIn(libs, "lib/some long folder name/Movie.mkv"); ok {
		t.Error("DiskPathIn took another case")
	}
	if p, _, ok := DiskPathIn(libs, "lib/Some Long Folder Name/Movie.mkv"); !ok || p != filepath.Join(dir, "Movie.mkv") {
		t.Errorf("DiskPathIn = %q %v", p, ok)
	}
}

// shortName is a folder's 8.3 name, or "" where the volume makes none.
func shortName(t *testing.T, path string) string {
	p, _ := windows.UTF16PtrFromString(path)
	buf := make([]uint16, windows.MAX_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Logf("no short names here: %v", err)
		return ""
	}
	short := filepath.Base(windows.UTF16ToString(buf[:n]))
	if short == filepath.Base(path) {
		t.Log("no short names here")
		return ""
	}
	t.Logf("short name %s", short)
	return short
}
