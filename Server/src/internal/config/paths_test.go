package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media", "movies")
	fsRoot := filepath.VolumeName(root) + string(filepath.Separator)
	cases := []struct {
		path, dir string
		want      bool
	}{
		{root, root, true},
		{filepath.Join(root, "a", "b.mkv"), root, true},
		{root + "-private", root, false},
		{filepath.Join(root+"-private", "x.mkv"), root, false},
		{filepath.Dir(root), root, false},
		{filepath.Join(root, "x.mkv"), fsRoot, true}, // a root ends in its separator already
		{root, "", false},
	}
	for _, c := range cases {
		if got := InDir(c.path, c.dir); got != c.want {
			t.Errorf("InDir(%q, %q) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
	// Windows' file systems ignore case; elsewhere another case is another folder.
	upper := filepath.Join(filepath.Dir(root), "MOVIES", "x.mkv")
	if got := InDir(upper, root); got != (runtime.GOOS == "windows") {
		t.Errorf("InDir(%q, %q) = %v on %s", upper, root, got, runtime.GOOS)
	}
}

// A library inside another (a private folder an admin made its own library, for another group)
// can't be reached through the outer one's ID, whatever spelling of its folder is used.
func TestDiskPathNestedLibrary(t *testing.T) {
	outer := t.TempDir()
	inner := filepath.Join(outer, "Private")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "x.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// In either order: the innermost library holds its files.
	for _, libs := range [][]MediaPath{
		{{ID: "inner", Path: inner}, {ID: "outer", Path: outer}},
		{{ID: "outer", Path: outer}, {ID: "inner", Path: inner}},
	} {
		if p, lib, ok := DiskPathIn(libs, "inner/x.mkv"); !ok || lib.ID != "inner" || p != filepath.Join(inner, "x.mkv") {
			t.Fatalf("inner/x.mkv: %q %q %v", p, lib.ID, ok)
		}
		for _, client := range []string{"outer/Private/x.mkv", "outer/private/x.mkv", "outer/PRIVATE/x.mkv", "outer/Private./x.mkv", "outer/x/../Private/x.mkv", "outer/Private"} {
			if p, lib, ok := DiskPathIn(libs, client); ok && InDir(p, inner) {
				t.Errorf("%s reached %q through library %q", client, p, lib.ID)
			}
		}
		if c, ok := ClientPathIn(libs, filepath.Join(inner, "x.mkv")); !ok || c != "inner/x.mkv" {
			t.Errorf("ClientPathIn = %q %v", c, ok)
		}
		if lib, ok := LibraryOf(libs, filepath.Join(outer, "y.mkv")); !ok || lib.ID != "outer" {
			t.Errorf("outer's own file: %q %v", lib.ID, ok)
		}
	}
}

// abs is a test's Unix-style disk path as this platform's absolute path ("/media/lib" is
// C:\media\lib on Windows).
func abs(p string) string {
	a, err := filepath.Abs(filepath.FromSlash(p))
	if err != nil {
		panic(err)
	}
	return a
}
