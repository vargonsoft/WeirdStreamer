package config

import (
	"path/filepath"
	"strings"
)

// SamePath reports whether two clean, absolute paths name the same place, as the platform's file
// systems compare names (exactly, or on Windows ignoring case).
func SamePath(a, b string) bool { return samePath(a, b) }

// InDir reports whether path is the folder dir or inside it, both absolute and clean, by whole
// folder names: "/media/movies-private/x" isn't in "/media/movies". Names compare as the
// platform's file systems do (samePath): exactly, or on Windows ignoring case.
func InDir(path, dir string) bool {
	if dir == "" || path == "" {
		return false
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(dir, sep) { // a drive's or the file system's root ends in one already
		if samePath(path, dir) {
			return true
		}
		dir += sep
	}
	return len(path) >= len(dir) && samePath(path[:len(dir)], dir)
}

// InGuideFiller reports whether a file (a disk path) is in a library lent to the Guide as filler
// (Use for Guide Filler): short clips aired whole between programs, which get no trickplay frames.
func InGuideFiller(path string) bool {
	if GlobalConfig == nil {
		return false
	}
	for _, lib := range GlobalConfig.Media.Paths {
		if lib.GuideFiller && InDir(path, lib.Path) {
			return true
		}
	}
	return false
}
