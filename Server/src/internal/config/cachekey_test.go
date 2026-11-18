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

func TestClientPathBothWays(t *testing.T) {
	withLibraries(t, MediaPath{ID: "01c6653f", Path: "/media/lib"}, MediaPath{ID: "8838533a", Path: "/media/shows"})
	cases := map[string]string{
		"/media/lib/EAC3 Test/EAC3 Test.mkv": "01c6653f/EAC3 Test/EAC3 Test.mkv",
		"/media/shows":                       "8838533a",
		"/media/shows/Show/S01E01.mkv":       "8838533a/Show/S01E01.mkv",
		"/media/lib/a/../b.mkv":              "01c6653f/b.mkv",
	}
	for disk, client := range cases {
		got, ok := ClientPath(disk)
		if !ok || got != client {
			t.Errorf("ClientPath(%q) = %q %v, want %q", disk, got, ok, client)
		}
		back, lib, ok := DiskPath(client)
		if !ok || back != filepath.Clean(disk) || lib.ID != client[:8] {
			t.Errorf("DiskPath(%q) = %q %q %v", client, back, lib.ID, ok)
		}
	}
	for _, outside := range []string{"/media/libsibling/x.mkv", "/elsewhere/x.mkv", ""} {
		if got, ok := ClientPath(outside); ok || got != "" {
			t.Errorf("ClientPath(%q) = %q, want none", outside, got)
		}
	}
	for _, bad := range []string{"/media/lib/x.mkv", "01c6653f/../shows/x.mkv", "01c6653f/../../etc/passwd", "ffffffff/x.mkv", ""} {
		if _, _, ok := DiskPath(bad); ok {
			t.Errorf("DiskPath(%q) named a file", bad)
		}
	}
}

func TestClientPathIgnoresLibraryRoot(t *testing.T) {
	withLibraries(t, MediaPath{ID: "8838533a", Path: "/old/shows"})
	before, _ := ClientPath("/old/shows/Show/S01E01.mkv")

	GlobalConfig.Media.Paths[0].Path = "/new/place/shows"
	after, _ := ClientPath("/new/place/shows/Show/S01E01.mkv")
	if before != after || after != "8838533a/Show/S01E01.mkv" {
		t.Fatalf("client path changed when the library moved: %q, %q", before, after)
	}
	if disk, _, ok := DiskPath(before); !ok || disk != "/new/place/shows/Show/S01E01.mkv" {
		t.Fatalf("DiskPath after the move = %q %v", disk, ok)
	}
	if _, ok := ClientPath("/old/shows/Show/S01E01.mkv"); ok {
		t.Fatal("the old folder still has a client path")
	}
}
