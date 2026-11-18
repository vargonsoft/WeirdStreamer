package config

import (
	"path/filepath"
	"testing"
)

func withLibraries(t *testing.T, paths ...MediaPath) {
	t.Helper()
	saved := GlobalConfig
	GlobalConfig = &Config{}
	GlobalConfig.Media.Paths = paths
	t.Cleanup(func() { GlobalConfig = saved })
}

func TestCacheKeyIgnoresLibraryRoot(t *testing.T) {
	withLibraries(t, MediaPath{ID: "8838533a", Path: "/old/shows"})
	before := CacheKey("/old/shows/Show/S01E01.mkv")

	GlobalConfig.Media.Paths[0].Path = "/new/place/shows"
	after := CacheKey("/new/place/shows/Show/S01E01.mkv")

	if before != after {
		t.Fatalf("key changed when the library moved: %s != %s", before, after)
	}
}

func TestCacheKeySeparatesLibraries(t *testing.T) {
	withLibraries(t,
		MediaPath{ID: "aaaaaaaa", Path: "/one"},
		MediaPath{ID: "bbbbbbbb", Path: "/two"})
	if CacheKey("/one/Movie.mkv") == CacheKey("/two/Movie.mkv") {
		t.Fatal("same relative path in two libraries gave one key")
	}
}

func TestLibraryRelative(t *testing.T) {
	withLibraries(t, MediaPath{ID: "01c6653f", Path: "/media/lib"})
	id, rel, ok := LibraryRelative(filepath.Join("/media/lib", "EAC3 Test", "EAC3 Test.mkv"))
	if !ok || id != "01c6653f" || rel != "EAC3 Test/EAC3 Test.mkv" {
		t.Fatalf("got %q %q %v", id, rel, ok)
	}
	if _, _, ok := LibraryRelative("/media/libsibling/x.mkv"); ok {
		t.Fatal("a sibling folder with the library's name prefix matched")
	}
}

func TestCacheKeyOutsideLibraryUsesPath(t *testing.T) {
	withLibraries(t, MediaPath{ID: "aaaaaaaa", Path: "/one"})
	if CacheKey("/elsewhere/x.mkv") == CacheKey("/elsewhere/y.mkv") {
		t.Fatal("different paths outside libraries gave one key")
	}
}
