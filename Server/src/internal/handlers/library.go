package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
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

const newAdditionsLimit = 10

// NewAdditions lists the videos that arrived last, newest first, for the Recently Added row: in a
// library (GET /api/library/new?lib=<id>), or blended from the libraries that show on the home
// screen (without lib). Empty where the showNewAdditions option leaves it off.
func (h *MediaHandler) NewAdditions(w http.ResponseWriter, r *http.Request) {
	libs := []config.MediaPath{}
	if id := r.URL.Query().Get("lib"); id != "" {
		lib, found := libraryByID(id)
		if !found || !h.canAccessLibrary(r, lib) {
			respondError(w, http.StatusNotFound, "Library not found")
			return
		}
		if lib.OnNewAdditions(true) {
			libs = append(libs, lib)
		}
	} else {
		for _, lib := range config.GlobalConfig.Media.Paths {
			if lib.OnNewAdditions(false) && h.canAccessLibrary(r, lib) {
				libs = append(libs, lib)
			}
		}
	}
	type arrival struct {
		database.AddedMedia
		lib config.MediaPath
	}
	var arrivals []arrival
	for _, lib := range libs {
		// Some may be gone from the disk: read a few extra.
		added, err := h.db.NewestMedia(lib.Path, newAdditionsLimit*2)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to list new additions")
			return
		}
		for _, a := range added {
			arrivals = append(arrivals, arrival{a, lib})
		}
	}
	sort.SliceStable(arrivals, func(i, j int) bool { return arrivals[i].AddedAt > arrivals[j].AddedAt })
	items := []continueItem{}
	pb := h.requestPlayback(r)
	for _, a := range arrivals {
		if len(items) >= newAdditionsLimit {
			break
		}
		if _, err := os.Stat(a.Path); err != nil {
			continue
		}
		item := h.continueEntry(a.lib, a.Path)
		if item == nil {
			continue
		}
		item.Playback = pb[a.Path]
		items = append(items, continueItem{MediaItem: *item, LibraryID: a.lib.ID, LibraryType: a.lib.Type, AddedAt: a.AddedAt})
	}
	respondJSON(w, http.StatusOK, items)
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
