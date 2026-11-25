package handlers

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/config"
)

// mediaTarget is a file or folder addressed by a metadata request, by its client path
// (<library id>/<relative path>): files like streams (path=, in base64), folders as browsed (dir=).
type mediaTarget struct {
	lib   config.MediaPath
	path  string
	isDir bool
}

// resolveTarget finds the library item a request addresses and checks the user may access it,
// writing the error response if not.
func (h *MediaHandler) resolveTarget(w http.ResponseWriter, r *http.Request, encodedPath, dir string) (*mediaTarget, bool) {
	client, isDir := dir, true
	if dir == "" {
		decoded, err := base64.StdEncoding.DecodeString(encodedPath)
		if err != nil || len(decoded) == 0 {
			respondError(w, http.StatusBadRequest, "Missing or invalid path")
			return nil, false
		}
		client, isDir = string(decoded), false
	}
	path, lib, ok := diskPath(client)
	if !ok || !h.canAccessLibrary(r, lib) {
		respondError(w, http.StatusNotFound, "Not found")
		return nil, false
	}
	return &mediaTarget{lib: lib, path: path, isDir: isDir}, true
}

// GetMediaDetails returns full TMDB metadata (credits included) for a file or folder.
func (h *MediaHandler) GetMediaDetails(w http.ResponseWriter, r *http.Request) {
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return
	}
	t, ok := h.resolveTarget(w, r, r.URL.Query().Get("path"), r.URL.Query().Get("dir"))
	if !ok {
		return
	}
	details, err := h.meta.Details(t.lib, t.path, t.isDir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load metadata")
		return
	}
	if details == nil {
		respondError(w, http.StatusNotFound, "No metadata")
		return
	}
	respondJSON(w, http.StatusOK, details)
}

var artworkFilePattern = regexp.MustCompile(`^[a-z0-9]+\.(jpg|jpeg|png)$`)

// ServeArtwork serves downloaded TMDB images. Movie and show artwork follows library access:
// it's served only if the user can access a library holding a file matched to that title.
// Headshots (/artwork/person/...) only require login.
func (h *MediaHandler) ServeArtwork(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	kind, file := vars["kind"], vars["file"]
	if !artworkFilePattern.MatchString(file) {
		respondError(w, http.StatusBadRequest, "Invalid artwork name")
		return
	}
	artworkRoot := config.GlobalConfig.Media.ArtworkPath
	var path string

	switch kind {
	case "person":
		if vars["id"] != "" {
			respondError(w, http.StatusNotFound, "Not found")
			return
		}
		path = filepath.Join(artworkRoot, "person", file)
	case "collection":
		// Seen by those who can see one of its movies
		id, err := strconv.Atoi(vars["id"])
		if err != nil || id <= 0 || !h.canAccessCollection(r, id) {
			respondError(w, http.StatusNotFound, "Not found")
			return
		}
		path = filepath.Join(artworkRoot, kind, strconv.Itoa(id), file)
	case config.LibraryTypeMovie, config.LibraryTypeTV:
		id, err := strconv.Atoi(vars["id"])
		if err != nil || id <= 0 || !h.canAccessTitle(r, kind, id) {
			respondError(w, http.StatusNotFound, "Not found")
			return
		}
		path = filepath.Join(artworkRoot, kind, strconv.Itoa(id), file)
	default:
		respondError(w, http.StatusNotFound, "Not found")
		return
	}

	if _, err := os.Stat(path); err != nil {
		respondError(w, http.StatusNotFound, "Not found")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, path)
}

// canAccessTitle reports whether the user can access any library holding the movie or show.
func (h *MediaHandler) canAccessTitle(r *http.Request, kind string, tmdbID int) bool {
	libIDs, err := h.db.LibrariesForTMDB(kind, tmdbID)
	if err != nil {
		return false
	}
	for _, id := range libIDs {
		for _, lib := range config.GlobalConfig.Media.Paths {
			if lib.ID == id && h.canAccessLibrary(r, lib) {
				return true
			}
		}
	}
	return false
}

// canAccessCollection reports whether the user can access a library holding one of a collection's movies.
func (h *MediaHandler) canAccessCollection(r *http.Request, collectionID int) bool {
	ids, err := h.db.CollectionMovies(collectionID)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if h.canAccessTitle(r, config.LibraryTypeMovie, id) {
			return true
		}
	}
	return false
}
