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

func (h *MediaHandler) respondMatchInfo(w http.ResponseWriter, status int, ref metadata.Ref) {
	info, err := h.meta.MatchInfo(ref)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load match")
		return
	}
	respondJSON(w, status, info)
}

// GetMatch returns what a file or folder is matched to and the title it was searched by.
func (h *MediaHandler) GetMatch(w http.ResponseWriter, r *http.Request) {
	ref, _, ok := h.matchTarget(w, r, r.URL.Query().Get("path"), r.URL.Query().Get("dir"))
	if ok {
		h.respondMatchInfo(w, http.StatusOK, ref)
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
	h.respondMatchInfo(w, http.StatusAccepted, ref)
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
	h.respondMatchInfo(w, http.StatusAccepted, ref)
}
