package handlers

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/config"
)

// GetMediaDetails returns full TMDB metadata (credits included) for a file or folder.
// Files are addressed like streams (?path=<base64 absolute path>); folders by their browse
// path (?dir=<library id>/<relative path>).
func (h *MediaHandler) GetMediaDetails(w http.ResponseWriter, r *http.Request) {
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return
	}

	var path string
	isDir := false
	if dir := r.URL.Query().Get("dir"); dir != "" {
		parts := strings.SplitN(strings.Trim(dir, "/"), "/", 2)
		for _, lib := range config.GlobalConfig.Media.Paths {
			if lib.ID == parts[0] {
				path = lib.Path
				if len(parts) == 2 {
					path = filepath.Join(lib.Path, parts[1])
				}
			}
		}
		isDir = true
	} else {
		decoded, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
		if err != nil || len(decoded) == 0 {
			respondError(w, http.StatusBadRequest, "Missing or invalid path")
			return
		}
		path = string(decoded)
	}
	absPath, err := filepath.Abs(path)
	if path == "" || err != nil {
		respondError(w, http.StatusNotFound, "Not found")
		return
	}

	lib, ok := libraryForPath(absPath)
	if !ok || !h.canAccessLibrary(r, lib) {
		respondError(w, http.StatusNotFound, "Not found")
		return
	}
	details, err := h.meta.Details(lib, absPath, isDir)
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
