package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/metadata"
)

// Manual metadata matching (admin only): fixing a file or folder fixes its title for every file
// in the library that resolves to it.

type matchRequest struct {
	Path   string `json:"path"` // base64 absolute file path
	Dir    string `json:"dir"`  // browse path of a folder
	TMDBID int    `json:"tmdb_id"`
	Poster string `json:"poster"` // TMDB image path for SetMatchPoster; "" for the default poster
}

// matchTarget resolves a match request's file or folder to the title lookup it fixes.
func (h *MediaHandler) matchTarget(w http.ResponseWriter, r *http.Request, encodedPath, dir string) (metadata.Ref, *mediaTarget, bool) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return metadata.Ref{}, nil, false
	}
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return metadata.Ref{}, nil, false
	}
	t, ok := h.resolveTarget(w, r, encodedPath, dir)
	if !ok {
		return metadata.Ref{}, nil, false
	}
	if t.lib.Type == "" {
		respondError(w, http.StatusBadRequest, "This library has no Movies or TV Shows type")
		return metadata.Ref{}, nil, false
	}
	ref, err := h.meta.MatchTarget(t.lib, t.path, t.isDir)
	if errors.Is(err, metadata.ErrNotFixable) {
		respondError(w, http.StatusBadRequest, err.Error())
		return metadata.Ref{}, nil, false
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to resolve item")
		return metadata.Ref{}, nil, false
	}
	return ref, t, true
}

func (h *MediaHandler) respondMatchInfo(w http.ResponseWriter, status int, ref metadata.Ref, t *mediaTarget) {
	info, err := h.meta.MatchInfo(ref)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load match")
		return
	}
	if !t.isDir && t.lib.Type == "tv" {
		info.Episode = h.meta.EpisodeInfo(t.lib, t.path)
	}
	respondJSON(w, status, info)
}

// GetMatch returns what a file or folder is matched to and the title it was searched by.
func (h *MediaHandler) GetMatch(w http.ResponseWriter, r *http.Request) {
	ref, t, ok := h.matchTarget(w, r, r.URL.Query().Get("path"), r.URL.Query().Get("dir"))
	if ok {
		h.respondMatchInfo(w, http.StatusOK, ref, t)
	}
}

// SearchMatch searches TMDB for the fix-match dialog: ?kind=movie|tv&query=...&year=...
func (h *MediaHandler) SearchMatch(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return
	}
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("query"))
	kind := q.Get("kind")
	if query == "" || (kind != "movie" && kind != "tv") {
		respondError(w, http.StatusBadRequest, "kind (movie or tv) and query are required")
		return
	}
	year, _ := strconv.Atoi(q.Get("year"))
	hits, err := h.meta.Search(r.Context(), kind, query, year)
	if errors.Is(err, metadata.ErrNoToken) {
		respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		respondError(w, http.StatusBadGateway, "TMDB search failed")
		return
	}
	respondJSON(w, http.StatusOK, hits)
}

// SetMatch matches a file or folder's title to a TMDB ID by hand (tmdb_id 0 means "no match").
func (h *MediaHandler) SetMatch(w http.ResponseWriter, r *http.Request) {
	var req matchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.TMDBID < 0 {
		respondError(w, http.StatusBadRequest, "Invalid tmdb_id")
		return
	}
	ref, t, ok := h.matchTarget(w, r, req.Path, req.Dir)
	if !ok {
		return
	}
	if err := h.meta.SetMatch(ref, req.TMDBID, t.path); err != nil {
		if errors.Is(err, metadata.ErrNoToken) {
			respondError(w, http.StatusServiceUnavailable, err.Error())
		} else {
			respondError(w, http.StatusInternalServerError, "Failed to save match")
		}
		return
	}
	h.respondMatchInfo(w, http.StatusAccepted, ref, t)
}

// ResetMatch forgets a title's match so it is matched automatically again.
func (h *MediaHandler) ResetMatch(w http.ResponseWriter, r *http.Request) {
	ref, t, ok := h.matchTarget(w, r, r.URL.Query().Get("path"), r.URL.Query().Get("dir"))
	if !ok {
		return
	}
	if err := h.meta.ResetMatch(ref, t.path); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to reset match")
		return
	}
	h.respondMatchInfo(w, http.StatusAccepted, ref, t)
}

// matchedTitle returns the TMDB title a fix-match target is matched to.
func (h *MediaHandler) matchedTitle(w http.ResponseWriter, ref metadata.Ref) (int, bool) {
	info, err := h.meta.MatchInfo(ref)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load match")
		return 0, false
	}
	if info.Current == nil {
		respondError(w, http.StatusConflict, "Match this item to a title first")
		return 0, false
	}
	return info.Current.TMDBID, true
}

// GetMatchPosters lists every TMDB poster for the title a file or folder is matched to.
func (h *MediaHandler) GetMatchPosters(w http.ResponseWriter, r *http.Request) {
	ref, _, ok := h.matchTarget(w, r, r.URL.Query().Get("path"), r.URL.Query().Get("dir"))
	if !ok {
		return
	}
	id, ok := h.matchedTitle(w, ref)
	if !ok {
		return
	}
	list, err := h.meta.Posters(r.Context(), ref.Kind, id)
	if errors.Is(err, metadata.ErrNotFetched) {
		respondError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		respondTMDBError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, list)
}

// SetMatchPoster sets the poster of the title a file or folder is matched to: any TMDB image
// path (one of its own posters, or another title's), or "" for its default poster.
func (h *MediaHandler) SetMatchPoster(w http.ResponseWriter, r *http.Request) {
	var req matchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	ref, t, ok := h.matchTarget(w, r, req.Path, req.Dir)
	if !ok {
		return
	}
	id, ok := h.matchedTitle(w, ref)
	if !ok {
		return
	}
	err := h.meta.SetPoster(r.Context(), ref.Kind, id, req.Poster, t.path)
	switch {
	case errors.Is(err, metadata.ErrBadPoster):
		respondError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, metadata.ErrNotFetched):
		respondError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		respondTMDBError(w, err)
		return
	}
	h.respondMatchInfo(w, http.StatusOK, ref, t)
}

// requireAdminMeta checks the caller is an admin and metadata is enabled.
func (h *MediaHandler) requireAdminMeta(w http.ResponseWriter, r *http.Request) bool {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return false
	}
	if h.meta == nil {
		respondError(w, http.StatusNotFound, "Metadata not enabled")
		return false
	}
	return true
}

func respondTMDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrNoToken) {
		respondError(w, http.StatusServiceUnavailable, err.Error())
	} else {
		respondError(w, http.StatusBadGateway, "TMDB request failed")
	}
}

// MatchSeasons lists a TMDB show's seasons for the episode picker: ?show=<tmdb id>
func (h *MediaHandler) MatchSeasons(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminMeta(w, r) {
		return
	}
	show, err := strconv.Atoi(r.URL.Query().Get("show"))
	if err != nil || show <= 0 {
		respondError(w, http.StatusBadRequest, "show is required")
		return
	}
	seasons, err := h.meta.ShowSeasons(r.Context(), show)
	if err != nil {
		respondTMDBError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, seasons)
}

// MatchEpisodes lists one TMDB season's episodes: ?show=<tmdb id>&season=<n>
func (h *MediaHandler) MatchEpisodes(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdminMeta(w, r) {
		return
	}
	show, err1 := strconv.Atoi(r.URL.Query().Get("show"))
	season, err2 := strconv.Atoi(r.URL.Query().Get("season"))
	if err1 != nil || err2 != nil || show <= 0 || season < 0 {
		respondError(w, http.StatusBadRequest, "show and season are required")
		return
	}
	episodes, err := h.meta.SeasonEpisodes(r.Context(), show, season)
	if err != nil {
		respondTMDBError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, episodes)
}

type episodeMappingRequest struct {
	Path        string `json:"path"` // base64 absolute file path
	Season      int    `json:"season"`
	Episode     int    `json:"episode"`
	WholeSeason bool   `json:"whole_season"`
}

// SetEpisodeMapping maps a TV file (or its whole season) to a TMDB season and episode.
func (h *MediaHandler) SetEpisodeMapping(w http.ResponseWriter, r *http.Request) {
	var req episodeMappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Season < 0 || req.Episode < 1 {
		respondError(w, http.StatusBadRequest, "path, season and episode are required")
		return
	}
	ref, t, ok := h.matchTarget(w, r, req.Path, "")
	if !ok {
		return
	}
	if t.isDir || t.lib.Type != "tv" {
		respondError(w, http.StatusBadRequest, "Episode mapping applies to TV episode files")
		return
	}
	if err := h.meta.SetEpisodeMapping(t.lib, t.path, req.Season, req.Episode, req.WholeSeason); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.NotifyMetadataReady(t.path)
	h.respondMatchInfo(w, http.StatusAccepted, ref, t)
}

// ClearEpisodeMapping removes a file's episode override, or (whole_season=1) its season's map.
func (h *MediaHandler) ClearEpisodeMapping(w http.ResponseWriter, r *http.Request) {
	ref, t, ok := h.matchTarget(w, r, r.URL.Query().Get("path"), "")
	if !ok {
		return
	}
	if t.isDir || t.lib.Type != "tv" {
		respondError(w, http.StatusBadRequest, "Episode mapping applies to TV episode files")
		return
	}
	if err := h.meta.ClearEpisodeMapping(t.lib, t.path, r.URL.Query().Get("whole_season") == "1"); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.NotifyMetadataReady(t.path)
	h.respondMatchInfo(w, http.StatusAccepted, ref, t)
}
