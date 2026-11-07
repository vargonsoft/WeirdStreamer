package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
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

// NewAdditions lists the videos that arrived last, newest first, for the Recently Added rows: in a
// library (GET /api/library/new?lib=<id>), or from the libraries that show on the home screen
// (without lib): one list per library with by=library, else blended into one (older apps). Empty
// where the showNewAdditions option leaves it off.
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
	if r.URL.Query().Get("by") == "library" {
		type libraryAdditions struct {
			LibraryID   string         `json:"library_id"`
			LibraryName string         `json:"library_name"`
			Items       []continueItem `json:"items"`
		}
		out := []libraryAdditions{}
		for _, lib := range libs {
			items, err := h.newAdditions(r, []config.MediaPath{lib})
			if err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to list new additions")
				return
			}
			if len(items) > 0 {
				out = append(out, libraryAdditions{lib.ID, lib.Name, items})
			}
		}
		// The library with the newest arrival first; libraries' order in the config breaks ties.
		newest := func(l libraryAdditions) (at int64) {
			for _, item := range l.Items {
				at = max(at, item.AddedAt)
			}
			return at
		}
		sort.SliceStable(out, func(i, j int) bool { return newest(out[i]) > newest(out[j]) })
		respondJSON(w, http.StatusOK, out)
		return
	}
	items, err := h.newAdditions(r, libs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list new additions")
		return
	}
	respondJSON(w, http.StatusOK, items)
}

// newAdditions lists the last videos to arrive in libs, blended: the newest days first.
func (h *MediaHandler) newAdditions(r *http.Request, libs []config.MediaPath) ([]continueItem, error) {
	type arrival struct {
		database.AddedMedia
		lib config.MediaPath
	}
	var arrivals []arrival
	for _, lib := range libs {
		// Some may be gone from the disk, or be Live Photos' videos: read extra.
		added, err := h.db.NewestMedia(lib.Path, newAdditionsLimit*10)
		if err != nil {
			return nil, err
		}
		for _, a := range added {
			arrivals = append(arrivals, arrival{a, lib})
		}
	}
	items := []continueItem{}
	pb := h.requestPlayback(r)
	for _, a := range arrivals {
		if _, err := os.Stat(a.Path); err != nil || media.IsLivePhotoPartner(a.Path) {
			continue
		}
		item := h.continueEntry(a.lib, a.Path)
		if item == nil {
			continue
		}
		item.Playback = pb[a.Path]
		items = append(items, continueItem{MediaItem: *item, LibraryID: a.lib.ID, LibraryType: a.lib.Type, AddedAt: a.AddedAt})
	}
	sortNewAdditions(items)
	if len(items) > newAdditionsLimit {
		items = items[:newAdditionsLimit]
	}
	return items, nil
}

// sortNewAdditions puts the newest day first, and a day's videos in viewing order: by show, season
// and episode, whatever order they happened to arrive in.
func sortNewAdditions(items []continueItem) {
	day := func(item continueItem) string { return time.Unix(item.AddedAt, 0).Format("2006-01-02") }
	order := func(item continueItem) (string, int, int) {
		if md := item.Metadata; md != nil && md.Kind == "episode" && md.Season != nil && md.Episode != nil {
			return strings.ToLower(md.ShowTitle), *md.Season, *md.Episode
		}
		if item.Metadata != nil && item.Metadata.Title != "" {
			return strings.ToLower(item.Metadata.Title), 0, 0
		}
		return strings.ToLower(item.Name), 0, 0
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if day(a) != day(b) {
			return day(a) > day(b)
		}
		aTitle, aSeason, aEpisode := order(a)
		bTitle, bSeason, bEpisode := order(b)
		if aTitle != bTitle {
			return aTitle < bTitle
		}
		if aSeason != bSeason {
			return aSeason < bSeason
		}
		return aEpisode < bEpisode
	})
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
			MimeType: media.MimeType(path)}
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
