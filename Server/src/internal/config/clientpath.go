package config

import (
	"path/filepath"
	"strings"
)

// A media file or folder is known, to clients and in the databases, by its client path: its
// library's ID and its path below the library, with forward slashes ("01c6653f/EAC3 Test/EAC3
// Test.mkv"; a library's own folder is just its ID). Moving a library's folder on disk changes
// no client path, so nothing stored by one needs changing. ClientPath and DiskPath are the one
// place disk paths and client paths are turned into each other.

// ClientPath is the client path of a disk path, or false for one outside every library (which
// has none: nothing is stored or shown for it).
func ClientPath(path string) (string, bool) {
	if GlobalConfig == nil {
		return "", false
	}
	return ClientPathIn(GlobalConfig.Media.Paths, path)
}

// DiskPath is the disk path a client path names, and its library. A path that isn't a library's
// ID and a path within it (a disk path, or one that climbs out with "..") names nothing.
func DiskPath(client string) (string, MediaPath, bool) {
	if GlobalConfig == nil {
		return "", MediaPath{}, false
	}
	return DiskPathIn(GlobalConfig.Media.Paths, client)
}

// LibraryOf returns the library of libs that holds a disk path (the first that does), compared by
// directory boundary, so "/media/movies-private/x" isn't in "/media/movies".
func LibraryOf(libs []MediaPath, path string) (MediaPath, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return MediaPath{}, false
	}
	for _, lib := range libs {
		root, err := filepath.Abs(lib.Path)
		if err != nil || lib.Path == "" {
			continue
		}
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return lib, true
		}
	}
	return MediaPath{}, false
}

// ClientPathIn is ClientPath with the given libraries (the conversion tool's, from a config file).
func ClientPathIn(libs []MediaPath, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	lib, ok := LibraryOf(libs, path)
	if !ok || lib.ID == "" {
		return "", false
	}
	abs, _ := filepath.Abs(path)
	root, _ := filepath.Abs(lib.Path)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return lib.ID, true
	}
	return lib.ID + "/" + filepath.ToSlash(rel), true
}

// DiskPathIn is DiskPath with the given libraries.
func DiskPathIn(libs []MediaPath, client string) (string, MediaPath, bool) {
	if strings.HasPrefix(client, "/") {
		return "", MediaPath{}, false
	}
	parts := strings.SplitN(strings.Trim(client, "/"), "/", 2)
	if parts[0] == "" {
		return "", MediaPath{}, false
	}
	var lib MediaPath
	found := false
	for _, l := range libs {
		if l.ID == parts[0] {
			lib, found = l, true
			break
		}
	}
	if !found || lib.Path == "" {
		return "", MediaPath{}, false
	}
	path, err := filepath.Abs(lib.Path)
	if err != nil {
		return "", MediaPath{}, false
	}
	if len(parts) == 2 {
		path = filepath.Join(path, filepath.FromSlash(parts[1]))
	}
	// It must stay inside the library it was addressed through.
	if in, ok := LibraryOf(libs, path); !ok || in.ID != lib.ID {
		return "", MediaPath{}, false
	}
	return path, lib, true
}
