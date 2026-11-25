package handlers

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
)

// libraryForPath returns the configured library that contains path. Paths are compared by
// directory boundary, so "/media/movies-private/x" does not belong to "/media/movies".
func libraryForPath(path string) (config.MediaPath, bool) {
	return config.LibraryOf(config.GlobalConfig.Media.Paths, path)
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
// library (config.ClientPath).
func clientPath(path string) string {
	client, _ := config.ClientPath(path)
	return client
}

// diskPath finds the file or folder a client path names, and its library (config.DiskPath).
func diskPath(client string) (string, config.MediaPath, bool) {
	return config.DiskPath(client)
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
// playable is the file to read a library video from: its fixed copy when it's damaged and has one
// (media.Playable), unless the request asks for the original as it is (original=1), or its user
// chose to play the original in Settings' Damaged videos (PlayDamaged). path stays the video's own for everything else: its client path, its
// progress, the files beside it.
func (h *MediaHandler) playable(r *http.Request, path string) string {
	if r.URL.Query().Get("original") == "1" || playsOriginal(r, path) {
		return path
	}
	return media.Playable(path)
}

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
		items[i].Format = shownFormat(items[i].Format)
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

// shownFormat is a stored format line as clients get it: "" for a video with none ("?").
func shownFormat(format string) string {
	if format == "?" {
		return ""
	}
	return format
}
