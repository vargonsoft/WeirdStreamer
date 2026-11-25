package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
	clientFiles(listing.Unmatched)
	if lib.Type == config.LibraryTypeMovie {
		cols, err := h.db.UserCollections(lib.ID)
		if err != nil {
			log.Printf("[LIBRARY] %s: collections: %v", lib.Name, err)
		}
		listing.UserCollections = h.meta.UserCollections(listing, cols)
	}
	respondJSON(w, http.StatusOK, listing)
}

// GetLibraryPerson returns a person's page, opened from a title page's cast: GET
// /api/library/person?lib=<id>&id=<tmdb person id>. Their details, the library's titles they're
// credited in, and their TMDB filmography (fetched once, then kept).
func (h *MediaHandler) GetLibraryPerson(w http.ResponseWriter, r *http.Request) {
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}
	listing, err := h.meta.Library(r.Context(), lib, h.libraryFileInfo)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list library")
		return
	}
	annotateListing(listing, h.requestPlayback(r))
	person, err := h.meta.Person(r.Context(), listing, id)
	switch {
	case errors.Is(err, metadata.ErrUnknownPerson):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, metadata.ErrNoToken):
		respondError(w, http.StatusServiceUnavailable, err.Error())
	case err != nil:
		log.Printf("[LIBRARY] person %d: %v", id, err)
		respondError(w, http.StatusBadGateway, "Couldn't get their details from TMDB")
	default:
		respondJSON(w, http.StatusOK, person)
	}
}

// Collections admins make (database/collections.go): a name, a description and movies of the
// library, everyone who sees the library sees them.

// movieLibraryListing is a movie library's listing with the user's progress, for collections.
func (h *MediaHandler) movieLibraryListing(w http.ResponseWriter, r *http.Request, lib config.MediaPath) (*models.LibraryListing, bool) {
	if lib.Type != config.LibraryTypeMovie {
		respondError(w, http.StatusBadRequest, "Collections are for movie libraries")
		return nil, false
	}
	listing, err := h.meta.Library(r.Context(), lib, h.libraryFileInfo)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list library")
		return nil, false
	}
	annotateListing(listing, h.requestPlayback(r))
	return listing, true
}

// respondUserCollection answers with a made collection's page.
func (h *MediaHandler) respondUserCollection(w http.ResponseWriter, lib config.MediaPath, listing *models.LibraryListing, id int64, status int) {
	c, err := h.db.UserCollection(lib.ID, id)
	if errors.Is(err, database.ErrNoUserCollection) {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load collection")
		return
	}
	respondJSON(w, status, h.meta.UserCollection(listing, *c))
}

// GetUserCollection returns a made collection's page: GET /api/library/usercollection?lib=<id>&id=<collection id>
func (h *MediaHandler) GetUserCollection(w http.ResponseWriter, r *http.Request) {
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}
	listing, ok := h.movieLibraryListing(w, r, lib)
	if !ok {
		return
	}
	h.respondUserCollection(w, lib, listing, id, http.StatusOK)
}

type userCollectionRequest struct {
	Library     string                   `json:"lib"`
	ID          int64                    `json:"id"`
	Name        *string                  `json:"name"`
	Description *string                  `json:"description"`
	Titles      []models.CollectionTitle `json:"titles"` // creating: its titles
	Add         []models.CollectionTitle `json:"add"`
	Remove      []models.CollectionTitle `json:"remove"`
}

const (
	maxCollectionName        = 200
	maxCollectionDescription = 4000
)

// ChangeUserCollection creates (POST, {"lib", "name", "description", "titles"}), edits (PUT, {"lib",
// "id", "name"?, "description"?, "add"?, "remove"?}) or deletes (DELETE ?lib=&id=) a made collection
// (admins): /api/library/usercollections. Answers with the collection's page (DELETE: 204).
func (h *MediaHandler) ChangeUserCollection(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(w, r) {
		return
	}
	var req userCollectionRequest
	if r.Method == http.MethodDelete {
		req.Library = r.URL.Query().Get("lib")
		req.ID, _ = strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	} else if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	q := r.URL.Query()
	q.Set("lib", req.Library)
	r.URL.RawQuery = q.Encode()
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost && req.ID <= 0 {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}
	if r.Method == http.MethodDelete {
		err := h.db.DeleteUserCollection(lib.ID, req.ID)
		if errors.Is(err, database.ErrNoUserCollection) {
			respondError(w, http.StatusNotFound, err.Error())
		} else if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to delete collection")
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
		return
	}
	if req.Name != nil {
		if n := strings.TrimSpace(*req.Name); n == "" || len(n) > maxCollectionName {
			respondError(w, http.StatusBadRequest, "A collection needs a name (up to 200 characters)")
			return
		}
	}
	if req.Description != nil && len(*req.Description) > maxCollectionDescription {
		respondError(w, http.StatusBadRequest, "The description is too long")
		return
	}
	listing, ok := h.movieLibraryListing(w, r, lib)
	if !ok {
		return
	}
	// Only the library's titles can go in
	inLibrary := map[models.CollectionTitle]bool{}
	for _, t := range listing.Titles {
		inLibrary[models.CollectionTitle{Kind: t.Kind, TMDBID: t.TMDBID}] = true
	}
	titles := func(list []models.CollectionTitle) ([]database.CollectionTitle, bool) {
		out := make([]database.CollectionTitle, 0, len(list))
		for _, t := range list {
			if !inLibrary[t] {
				return nil, false
			}
			out = append(out, database.CollectionTitle{Kind: t.Kind, TMDBID: t.TMDBID})
		}
		return out, true
	}
	if r.Method == http.MethodPost {
		if req.Name == nil {
			respondError(w, http.StatusBadRequest, "A collection needs a name (up to 200 characters)")
			return
		}
		add, ok := titles(req.Titles)
		if !ok {
			respondError(w, http.StatusBadRequest, "Only the library's titles can be in its collections")
			return
		}
		desc := ""
		if req.Description != nil {
			desc = *req.Description
		}
		id, err := h.db.CreateUserCollection(lib.ID, *req.Name, desc, add)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to create collection")
			return
		}
		h.respondUserCollection(w, lib, listing, id, http.StatusCreated)
		return
	}
	add, ok := titles(req.Add)
	if !ok {
		respondError(w, http.StatusBadRequest, "Only the library's titles can be in its collections")
		return
	}
	remove := make([]database.CollectionTitle, 0, len(req.Remove))
	for _, t := range req.Remove { // a title gone from the library can still be taken out
		remove = append(remove, database.CollectionTitle{Kind: t.Kind, TMDBID: t.TMDBID})
	}
	err := h.db.UpdateUserCollection(lib.ID, req.ID, req.Name, req.Description, add, remove)
	if errors.Is(err, database.ErrNoUserCollection) {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to change collection")
		return
	}
	h.respondUserCollection(w, lib, listing, req.ID, http.StatusOK)
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
	for _, files := range titleFiles(detail) {
		clientFiles(files)
	}
	respondJSON(w, http.StatusOK, detail)
}

// GetLibraryCollection returns a collection's page: GET /api/library/collection?lib=<id>&id=<tmdb id>
func (h *MediaHandler) GetLibraryCollection(w http.ResponseWriter, r *http.Request) {
	lib, ok := h.libraryFromRequest(w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(r.URL.Query().Get("id"))
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "id is required")
		return
	}
	listing, err := h.meta.Library(r.Context(), lib, h.libraryFileInfo)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list library")
		return
	}
	annotateListing(listing, h.requestPlayback(r))
	detail, err := h.meta.Collection(listing, id)
	if errors.Is(err, metadata.ErrNoCollection) {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load collection")
		return
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
				clientContinue(items)
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
	clientContinue(items)
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
	// Ones without a thumbnail yet get one in the background, as browsing their folder would make;
	// pages showing them are told when it's ready.
	for _, item := range items {
		if item.ThumbnailURL == "" {
			if stored, err := h.db.GetMediaItem(item.Path); err == nil { // as stored, without the titles added
				h.thumbnails.add(*stored, false)
			}
		}
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

// libraryFileInfo describes a file for Library view from the media database, starting thumbnail
// generation (as browsing does) for files that have none yet.
func (h *MediaHandler) libraryFileInfo(path string) models.LibraryFile {
	file := models.LibraryFile{Path: path, Name: filepath.Base(path)}
	item, err := h.db.GetMediaItem(path)
	if err == nil {
		file.ID, file.Size, file.Duration, file.ThumbnailURL = item.ID, item.Size, item.Duration, item.ThumbnailURL
		file.Format = shownFormat(item.Format)
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

	h.thumbnails.add(*item, false)
	return file
}
