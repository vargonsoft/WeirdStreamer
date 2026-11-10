package handlers

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/models"
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

// Clients never see where a library is on disk. A file or folder is, to them, its library's ID
// and its path below the library ("01c6653f/EAC3 Test/EAC3 Test.mkv"): clientPath makes that from
// a disk path for every response, and diskPath turns one a client sent back into the disk path.

// clientPath is the path clients know a media file or folder by, or "" for one outside every
// library.
func clientPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	lib, ok := libraryForPath(abs)
	if !ok {
		return ""
	}
	root, err := filepath.Abs(lib.Path)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." {
		return lib.ID
	}
	return lib.ID + "/" + filepath.ToSlash(rel)
}

// diskPath finds the file or folder a client path names, and its library. A path that isn't a
// library's ID and a path within it (a disk path, or one that climbs out with "..") names nothing.
func diskPath(client string) (string, config.MediaPath, bool) {
	parts := strings.SplitN(strings.Trim(client, "/"), "/", 2)
	if strings.HasPrefix(client, "/") || parts[0] == "" {
		return "", config.MediaPath{}, false
	}
	lib, ok := libraryByID(parts[0])
	if !ok {
		return "", config.MediaPath{}, false
	}
	path, err := filepath.Abs(lib.Path)
	if err != nil {
		return "", config.MediaPath{}, false
	}
	if len(parts) == 2 {
		path = filepath.Join(path, filepath.FromSlash(parts[1]))
	}
	// It must stay inside the library it was addressed through.
	if in, ok := libraryForPath(path); !ok || in.ID != lib.ID {
		return "", config.MediaPath{}, false
	}
	return path, lib, true
}

// filePath reads a base64 client path a request sent, checking the user may see the file: the
// disk path, or the status to answer with (400: not a path; 403: no such file for this user).
func (h *MediaHandler) filePath(r *http.Request, encoded string) (string, int) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		return "", http.StatusBadRequest
	}
	path, lib, ok := diskPath(string(decoded))
	if !ok || !h.canAccessLibrary(r, lib) {
		return "", http.StatusForbidden
	}
	return path, http.StatusOK
}

// decodeMediaPath is filePath for a handler: it writes the error response when there's no file.
func (h *MediaHandler) decodeMediaPath(w http.ResponseWriter, r *http.Request, encoded string) (string, bool) {
	path, status := h.filePath(r, encoded)
	switch status {
	case http.StatusOK:
		return path, true
	case http.StatusBadRequest:
		respondError(w, status, "Invalid path encoding")
	default:
		respondError(w, status, "Access denied")
	}
	return "", false
}

// clientItems gives items the paths clients know them by, as the last step before a response
// (everything before it works with disk paths).
func clientItems(items []models.MediaItem) {
	for i := range items {
		items[i].Path = clientPath(items[i].Path)
		items[i].LiveVideo = clientPath(items[i].LiveVideo)
	}
}

// clientItem is a copy of item for a response, with the path clients know it by.
func clientItem(item *models.MediaItem) *models.MediaItem {
	if item == nil {
		return nil
	}
	one := []models.MediaItem{*item}
	clientItems(one)
	return &one[0]
}

// clientFiles is clientItems for a library listing's or a title page's files.
func clientFiles(files []models.LibraryFile) {
	for i := range files {
		files[i].Path = clientPath(files[i].Path)
	}
}

// withoutDiskPath takes a file's disk path out of text made for a person (an ffmpeg error, its
// arguments), leaving the path clients know it by.
func withoutDiskPath(text, path string) string {
	if path == "" {
		return text
	}
	return strings.ReplaceAll(text, path, clientPath(path))
}
