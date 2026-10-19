package handlers

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
)

// libraryForPath returns the configured library that contains path. Paths are compared by
// directory boundary, so "/media/movies-private/x" does not belong to "/media/movies".
func libraryForPath(path string) (config.MediaPath, bool) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return config.MediaPath{}, false
	}
	for _, lib := range config.GlobalConfig.Media.Paths {
		root, err := filepath.Abs(lib.Path)
		if err != nil {
			continue
		}
		if absPath == root || strings.HasPrefix(absPath, root+string(filepath.Separator)) {
			return lib, true
		}
	}
	return config.MediaPath{}, false
}

// groupCanAccess reports whether members of group may access lib.
func groupCanAccess(group string, lib config.MediaPath) bool {
	if group == "" {
		return false
	}
	for _, g := range lib.Groups {
		if g == group {
			return true
		}
	}
	return false
}

// userGroup returns the group of the authenticated user, loaded once by AuthMiddleware.
func userGroup(r *http.Request) string {
	if user := auth.GetUser(r.Context()); user != nil {
		return user.Group
	}
	return ""
}

func (h *MediaHandler) canAccessLibrary(r *http.Request, lib config.MediaPath) bool {
	return groupCanAccess(userGroup(r), lib)
}

// canAccessPath reports whether the requesting user may access the media file at path.
func (h *MediaHandler) canAccessPath(r *http.Request, path string) bool {
	lib, ok := libraryForPath(path)
	return ok && h.canAccessLibrary(r, lib)
}
