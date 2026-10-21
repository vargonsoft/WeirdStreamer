package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
)

// Active streams: tracked as they're served, listed for admins on the Streams page.

// streamKey identifies a stream: one user playing one file from one client address.
func streamKey(r *http.Request, path string) string {
	user := "anonymous"
	if claims := auth.GetClaims(r.Context()); claims != nil {
		user = strconv.FormatInt(claims.UserID, 10)
	}
	return user + "|" + clientIP(r) + "|" + path
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// beginStream tracks a video request; the returned session (nil for other media) must be ended.
func (h *MediaHandler) beginStream(r *http.Request, item *models.MediaItem) *media.StreamSession {
	if item.Type != "video" {
		return nil
	}
	sess := h.streams.Begin(streamKey(r, item.Path), h.streamDescriber(r, item))
	if media.IsDiscImage(item.Path) {
		// A disc's stream plays one of its titles: show that title's length.
		if info, err := media.LoadDisc(item.Path); err == nil {
			if t := info.Title(media.ParseDiscOptions(r).Title); t != nil {
				sess.FillIn(func(s *media.StreamInfo) { s.Duration, s.Width, s.Height = t.Duration, t.Width, t.Height })
			}
		}
		return sess
	}
	h.probeStream(sess, item.Path)
	return sess
}

// probeStream fills in the duration and resolution of a stream whose file hasn't been probed yet
// (no duration in the database): once per stream, in the background.
func (h *MediaHandler) probeStream(sess *media.StreamSession, path string) {
	if !sess.NeedsProbe() {
		return
	}
	go func() {
		if duration, width, height, err := media.ProbeVideo(path); err == nil {
			sess.FillIn(func(info *media.StreamInfo) {
				info.Duration, info.Width, info.Height = duration, width, height
			})
		}
	}()
}

// streamDescriber fills in a new stream's details: who, what, from where.
func (h *MediaHandler) streamDescriber(r *http.Request, item *models.MediaItem) func(*media.StreamInfo) {
	user := "unknown"
	if claims := auth.GetClaims(r.Context()); claims != nil {
		user = claims.Username
	}
	ua := r.Header.Get("User-Agent")
	return func(info *media.StreamInfo) {
		info.User, info.ClientIP, info.UserAgent, info.Client = user, clientIP(r), ua, media.ClientName(ua)
		info.Path, info.Name, info.Size = item.Path, item.Name, item.Size
		info.Duration, info.Width, info.Height = item.Duration, item.Width, item.Height
		info.Title = h.streamTitle(item.Path)
	}
}

// streamTitle names a file by its TMDB match: "Title (Year)", or "Show S01E02 · Episode".
func (h *MediaHandler) streamTitle(path string) string {
	if h.meta == nil {
		return ""
	}
	lib, ok := libraryForPath(path)
	if !ok || lib.Type == "" {
		return ""
	}
	d, err := h.meta.Details(lib, path, false)
	if err != nil || d == nil {
		return ""
	}
	switch {
	case d.Kind == "episode" && d.Season != nil && d.Episode != nil:
		return fmt.Sprintf("%s S%02dE%02d · %s", d.ShowTitle, *d.Season, *d.Episode, d.Title)
	case d.Year > 0:
		return fmt.Sprintf("%s (%d)", d.Title, d.Year)
	}
	return d.Title
}

type streamProgress struct {
	Path     string  `json:"path"` // base64 absolute path
	Position float64 `json:"position"`
	Paused   bool    `json:"paused"`
	Ended    bool    `json:"ended"` // played to the end
	Title    int     `json:"title"` // discs: the title playing (0 is the main title)
}

// ReportStreamProgress takes a player's position and paused state for the Streams page.
func (h *MediaHandler) ReportStreamProgress(w http.ResponseWriter, r *http.Request) {
	var req streamProgress
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(req.Path)
	if err != nil || len(decoded) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(decoded)
	if !h.isValidMediaPath(path) || !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}
	item, err := h.db.GetMediaItem(path)
	if err != nil || item.Type != "video" {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}
	sess := h.streams.Report(streamKey(r, path), h.streamDescriber(r, item), req.Position, req.Paused)
	h.probeStream(sess, path)
	// A disc's progress is in its main title; other titles (extras) aren't tracked.
	if !media.IsDiscImage(path) || req.Title == 0 {
		h.recordPlayback(r, item, req.Position, req.Ended, time.Now(), false)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListStreams lists the active streams (admin only).
func (h *MediaHandler) ListStreams(w http.ResponseWriter, r *http.Request) {
	if claims := auth.GetClaims(r.Context()); claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	respondJSON(w, http.StatusOK, h.streams.List())
}

// LocalStreamSummary says how many streams are active, without what is playing, to tools on the
// server itself (checking before a restart). It answers only loopback connections.
func (h *MediaHandler) LocalStreamSummary(w http.ResponseWriter, r *http.Request) {
	if ip := net.ParseIP(clientIP(r)); ip == nil || !ip.IsLoopback() {
		http.NotFound(w, r)
		return
	}
	type entry struct {
		User       string `json:"user"`
		Client     string `json:"client"`
		State      string `json:"state"`
		Mode       string `json:"mode"`
		SecondsAgo int    `json:"seconds_since_activity"`
	}
	summary := struct {
		Active  int     `json:"active"` // playing, paused or streaming
		Streams []entry `json:"streams"`
	}{Streams: []entry{}}
	for _, s := range h.streams.List() {
		if s.State != "idle" {
			summary.Active++
		}
		summary.Streams = append(summary.Streams, entry{User: s.User, Client: s.Client, State: s.State, Mode: s.Mode,
			SecondsAgo: int(time.Since(s.LastActivity).Seconds())})
	}
	respondJSON(w, http.StatusOK, summary)
}

// GetDisc describes a DVD disc image's titles (chapters, angles, audio and subtitle tracks), for
// the players' disc controls: ?path=<base64 path>
func (h *MediaHandler) GetDisc(w http.ResponseWriter, r *http.Request) {
	decoded, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
	if err != nil || len(decoded) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(decoded)
	if !h.isValidMediaPath(path) || !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}
	if !media.IsDiscImage(path) {
		respondError(w, http.StatusBadRequest, "Not a disc image")
		return
	}
	info, err := media.LoadDisc(path)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Could not read the disc: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, info)
}

// GetSubtitles streams a text subtitle track of a file as WebVTT (GET /api/media/subtitles?
// path=<base64 path>&track=<index among its subtitles>&start=<s>&duration=<s>). Cue times count
// from start; without a duration the cues go to the end of the file.
func (h *MediaHandler) GetSubtitles(w http.ResponseWriter, r *http.Request) {
	decoded, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
	if err != nil || len(decoded) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(decoded)
	if !h.isValidMediaPath(path) || !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}
	track, err := strconv.Atoi(r.URL.Query().Get("track"))
	if err != nil || track < 0 {
		respondError(w, http.StatusBadRequest, "Invalid track")
		return
	}
	start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
	duration, _ := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)
	media.ServeSubtitles(w, r, path, track, max(start, 0), max(duration, 0))
}

// GetCodecs reports a file's container and its video and audio codecs, for players that check
// what they can play (GET /api/media/codecs?path=<base64 path>).
func (h *MediaHandler) GetCodecs(w http.ResponseWriter, r *http.Request) {
	decoded, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
	if err != nil || len(decoded) == 0 {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(decoded)
	if !h.isValidMediaPath(path) || !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}
	codecs, err := media.ProbeCodecs(path)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Could not read the file's codecs")
		return
	}
	respondJSON(w, http.StatusOK, codecs)
}
