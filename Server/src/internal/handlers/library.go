package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/metadata"
	"github.com/weirdstreamer/server/internal/models"
)

// Library view: movie and tv libraries as lists of titles instead of folders.

// libraryFromRequest finds the library named by ?lib= and checks the user may access it.
func (h *MediaHandler) libraryFromRequest(w http.ResponseWriter, r *http.Request) (config.MediaPath, bool) {
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return config.MediaPath{}, false
	}
	id := r.URL.Query().Get("lib")
	for _, lib := range config.GlobalConfig.Media.Paths {
		if lib.ID == id && h.canAccessLibrary(r, lib) {
			if lib.Type == "" {
				respondError(w, http.StatusBadRequest, "This library has no Movies or TV Shows type")
				return config.MediaPath{}, false
			}
			return lib, true
		}
	}
	respondError(w, http.StatusNotFound, "Library not found")
	return config.MediaPath{}, false
}

// GetLibrary lists a library's titles: GET /api/library?lib=<id>
func (h *MediaHandler) GetLibrary(w http.ResponseWriter, r *http.Request) {
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	listing, err := h.meta.Library(r.Context(), lib, h.libraryFileInfo)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list library")
		return
	}
	annotateListing(listing, h.requestPlayback(r))
	respondJSON(w, http.StatusOK, listing)
}

// GetLibraryTitle returns a title page: GET /api/library/title?lib=<id>&kind=movie|show&id=<tmdb id>
func (h *MediaHandler) GetLibraryTitle(w http.ResponseWriter, r *http.Request) {
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	kind := r.URL.Query().Get("kind")
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if (kind != "movie" && kind != "show") || err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "kind (movie or show) and id are required")
		return
	}
	detail, err := h.meta.Title(r.Context(), lib, kind, id, h.libraryFileInfo)
	if errors.Is(err, metadata.ErrNotInLibrary) {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load title")
		return
	}
	annotateTitle(detail, h.requestPlayback(r))
	if uid, ok := userID(r); ok && kind == "show" {
		detail.AutoSkip = h.db.AutoSkip(uid, id)
	}
	respondJSON(w, http.StatusOK, detail)
}

// thumbnailsInFlight limits how many missing thumbnails a library page starts generating at once.
var thumbnailsInFlight sync.Map

const maxThumbnailsInFlight = 64

// libraryFileInfo describes a file for Library view from the media database, starting thumbnail
// generation (as browsing does) for files that have none yet.
func (h *MediaHandler) libraryFileInfo(path string) models.LibraryFile {
	file := models.LibraryFile{Path: path, Name: filepath.Base(path)}
	item, err := h.db.GetMediaItem(path)
	if err == nil {
		file.ID, file.Size, file.Duration, file.ThumbnailURL = item.ID, item.Size, item.Duration, item.ThumbnailURL
		if item.ThumbnailURL != "" {
			return file
		}
	} else {
		if st, err := os.Stat(path); err == nil {
			file.Size = st.Size()
		}
		item = &models.MediaItem{Path: path, Name: file.Name, Type: "video", Size: file.Size,
			MimeType: h.getMimeTypeFromExtension(filepath.Ext(path))}
	}

	n := 0
	thumbnailsInFlight.Range(func(_, _ any) bool { n++; return n < maxThumbnailsInFlight })
	if n >= maxThumbnailsInFlight {
		return file // the next visit to the page starts the rest
	}
	if _, busy := thumbnailsInFlight.LoadOrStore(path, true); busy {
		return file
	}
	go func(item *models.MediaItem) {
		defer thumbnailsInFlight.Delete(item.Path)
		item.ThumbnailURL = h.scanner.GenerateVideoThumbnail(item.Path)
		h.db.SaveMediaItem(item)
		h.notifyThumbnailReady(item.Path, item.ThumbnailURL)
	}(item)
	return file
}
