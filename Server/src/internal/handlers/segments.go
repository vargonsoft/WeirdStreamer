package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// Intros and credits, for the players' Skip buttons, and each user's shows to skip them in by
// themselves.

type segmentJSON struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Source string  `json:"source"` // "chapters", "fingerprint", "black" or "manual"
}

type segmentsResponse struct {
	Intro    *segmentJSON `json:"intro,omitempty"`
	Credits  *segmentJSON `json:"credits,omitempty"`
	AutoSkip bool         `json:"auto_skip"`         // the user skips this show's intros and credits by themselves
	ShowID   int          `json:"show_id,omitempty"` // the show an episode belongs to
	// The episode after this one (on into the next season), for Skip Credits
	Next *models.MediaItem `json:"next,omitempty"`
	// What this video is, for the players' title
	Metadata *models.ItemMetadata `json:"metadata,omitempty"`
}

func toSegmentJSON(s *database.Span) *segmentJSON {
	if s == nil {
		return nil
	}
	return &segmentJSON{Start: s.Start, End: s.End, Source: s.Source}
}

// decodeMediaPath reads a base64 path parameter, checking it's a media file the user may see.
func (h *MediaHandler) decodeMediaPath(w http.ResponseWriter, r *http.Request, encoded string) (string, bool) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(decoded) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return "", false
	}
	path := string(decoded)
	if !h.isValidMediaPath(path) || !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return "", false
	}
	return path, true
}

// GetSegments returns a video's intro and credits, and whether the user skips them by
// themselves: GET /api/media/segments?path=<base64>
func (h *MediaHandler) GetSegments(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	segs, err := h.db.GetSegments(path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load segments")
		return
	}
	resp := segmentsResponse{Intro: toSegmentJSON(segs.Intro), Credits: toSegmentJSON(segs.Credits)}
	if lib, found := libraryForPath(path); found {
		if item := h.continueEntry(lib, path); item != nil {
			resp.ShowID = episodeShow(item)
			resp.Metadata = item.Metadata
		}
		if resp.ShowID != 0 {
			if after := h.episodesAfter(r.Context(), lib, resp.ShowID, path); len(after) > 0 {
				resp.Next = h.continueEntry(lib, after[0])
			}
		}
	}
	if uid, ok := userID(r); ok && resp.ShowID != 0 {
		resp.AutoSkip = h.db.AutoSkip(uid, resp.ShowID)
	}
	respondJSON(w, http.StatusOK, resp)
}

// SetSegment sets (or, with clear, forgets) a video's intro or credits by hand, for admins:
// PUT /api/media/segments {path, kind: "intro"|"credits", start, end, clear}
func (h *MediaHandler) SetSegment(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	var req struct {
		Path  string  `json:"path"`
		Kind  string  `json:"kind"`
		Start float64 `json:"start"`
		End   float64 `json:"end"`
		Clear bool    `json:"clear"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	if req.Kind != "intro" && req.Kind != "credits" {
		respondError(w, http.StatusBadRequest, "kind must be intro or credits")
		return
	}
	var span *database.Span
	if !req.Clear {
		if req.Start < 0 || req.End <= req.Start {
			respondError(w, http.StatusBadRequest, "end must come after start")
			return
		}
		span = &database.Span{Start: req.Start, End: req.End, Source: "manual"}
	}
	if err := h.db.SetManualSegment(path, req.Kind, span); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save")
		return
	}
	if req.Clear && h.segments != nil {
		h.segments.Wake() // look at it again
	}
	h.GetSegments(w, withQuery(r, "path", req.Path))
}

// SetAutoSkip turns skipping a show's intros and credits by themselves on or off for the user:
// PUT /api/library/autoskip {tmdb_id, on}
func (h *MediaHandler) SetAutoSkip(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	var req struct {
		TMDBID int  `json:"tmdb_id"`
		On     bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TMDBID <= 0 {
		respondError(w, http.StatusBadRequest, "tmdb_id is required")
		return
	}
	if err := h.db.SetAutoSkip(uid, req.TMDBID, req.On); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// withQuery is r with one query parameter set (to answer a PUT with what a GET would return).
func withQuery(r *http.Request, key, value string) *http.Request {
	q := r.URL.Query()
	q.Set(key, value)
	r2 := r.Clone(r.Context())
	r2.URL.RawQuery = q.Encode()
	return r2
}
