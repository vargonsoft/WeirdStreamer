package config

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// CacheKey names a media file in the caches (thumbnails, trickplay, ASS tracks, subtitle keys,
// transcode chunks): the hex SHA-256 of its library's ID and its path inside the library. Moving a
// library's folder on disk changes no key. A file outside every library (no library to name it by)
// is keyed by its full path instead.
func CacheKey(path string) string {
	if id, rel, ok := LibraryRelative(path); ok {
		return hashOf(id + "\x00" + rel)
	}
	return hashOf(path)
}

// LibraryRelative splits a disk path into its library's ID and its path below the library, with
// forward slashes (the client's form). ok is false when the path isn't inside a library.
func LibraryRelative(path string) (id, rel string, ok bool) {
	if GlobalConfig == nil {
		return "", "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", false
	}
	for _, lib := range GlobalConfig.Media.Paths {
		root, err := filepath.Abs(lib.Path)
		if err != nil || lib.ID == "" {
			continue
		}
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			r, err := filepath.Rel(root, abs)
			if err != nil {
				return "", "", false
			}
			return lib.ID, filepath.ToSlash(r), true
		}
	}
	return "", "", false
}

// TextHash is the hex SHA-256 of s, for names that aren't a media path (a transcode's viewer and arguments).
func TextHash(s string) string {
	return hashOf(s)
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
