package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/clientip"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
	"github.com/weirdstreamer/server/internal/subtitles"
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

// clientIP is the client's address, from a trusted proxy's headers behind one.
func clientIP(r *http.Request) string {
	return clientip.Of(r)
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
	Path     string  `json:"path"` // base64 client path
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
	if req.Position < 0 { // a player that lost track of time; it would become the resume point
		respondError(w, http.StatusBadRequest, "Invalid position")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
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
	// What's playing, by the path clients know it by: not where it is on disk, nor in ffmpeg's arguments.
	list := h.streams.List()
	for i := range list {
		s := &list[i]
		if t := s.Transcode; t != nil {
			shown := *t
			shown.Args = make([]string, len(t.Args))
			for j, arg := range t.Args {
				shown.Args[j] = withoutDiskPath(arg, s.Path)
			}
			s.Transcode = &shown
		}
		s.Path = clientPath(s.Path)
	}
	respondJSON(w, http.StatusOK, list)
}

// LocalStreamSummary says how many streams are active, without what is playing, to tools on the
// server itself (checking before a restart). It answers only loopback connections.
func (h *MediaHandler) LocalStreamSummary(w http.ResponseWriter, r *http.Request) {
	// The connection's own address: a forwarded one could claim to be local.
	if ip := net.ParseIP(clientip.Peer(r)); ip == nil || !ip.IsLoopback() {
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
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	if !media.IsDiscImage(path) {
		respondError(w, http.StatusBadRequest, "Not a disc image")
		return
	}
	info, err := media.LoadDisc(path)
	if err != nil {
		log.Printf("[DISC] Could not read %s: %v", path, err)
		respondError(w, http.StatusUnprocessableEntity, "Could not read the disc: "+withoutDiskPath(err.Error(), path))
		return
	}
	respondJSON(w, http.StatusOK, info)
}

// GetSubtitles streams a text subtitle track of a file as WebVTT (GET /api/media/subtitles?
// path=<base64 path>&track=<index among its subtitles>&start=<s>&duration=<s>). Cue times count
// from start; without a duration the cues go to the end of the file. format=sup gives a PGS track's
// raw pictures, format=ass an ASS/SSA track (or file) as an ASS script, header and all.
func (h *MediaHandler) GetSubtitles(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	track, err := strconv.Atoi(r.URL.Query().Get("track"))
	if err != nil || track < 0 {
		respondError(w, http.StatusBadRequest, "Invalid track")
		return
	}
	start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
	duration, _ := strconv.ParseFloat(r.URL.Query().Get("duration"), 64)
	start, duration = max(start, 0), max(duration, 0)
	// Tracks past the video's own are its subtitle files, in the codecs report's order.
	own := 0
	codecs, err := media.ProbeCodecs(path)
	if err == nil {
		own = len(codecs.Subtitles)
	} else {
		codecs = nil
	}
	assFormat := r.URL.Query().Get("format") == "ass"
	if track >= own {
		external := externalSubtitles(path)
		if track-own >= len(external) {
			respondError(w, http.StatusNotFound, "No such subtitle track")
			return
		}
		file := external[track-own]
		if assFormat {
			if !media.IsAssSubtitle(file.Codec) {
				respondError(w, http.StatusBadRequest, "Not an ASS/SSA track")
				return
			}
			// A file is small: read whole, every window is cut from all of it.
			script, err := media.AssFileScript(r.Context(), file.Path, subtitles.Charset(file.Path))
			if err != nil {
				respondError(w, http.StatusUnprocessableEntity, "Could not read the subtitle file")
				return
			}
			media.ServeAssWindow(w, script, start, duration)
			return
		}
		media.ServeSubtitleFile(w, r, file.Path, subtitles.Charset(file.Path), start, duration)
		return
	}
	if r.URL.Query().Get("format") == "sup" { // a PGS track, for the players to draw
		media.ServePictureSubtitles(w, r, path, track, start, duration)
		return
	}
	if assFormat { // an ASS/SSA track, for the players' ASS renderers
		if codecs == nil || !media.IsAssSubtitle(codecs.Subtitles[track].Codec) {
			respondError(w, http.StatusBadRequest, "Not an ASS/SSA track")
			return
		}
		// From the track copied out by the ASS job when there is one: every line on screen in the
		// window, those started before it too. Else read from the video.
		if cached := media.CachedAss(path, track); cached != "" {
			if script, err := os.ReadFile(cached); err == nil {
				media.ServeAssWindow(w, script, start, duration)
				return
			}
		}
		media.ServeAssSubtitles(w, r, path, track, start, duration)
		return
	}
	media.ServeSubtitles(w, r, path, track, start, duration)
}

// subsetLimit is a per-user budget of subset requests: 20 a minute.
var subsetLimit = &requestBudget{perMinute: 20, used: map[string][]time.Time{}}

type requestBudget struct {
	mu        sync.Mutex
	perMinute int
	used      map[string][]time.Time
}

func (b *requestBudget) allow(user string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	recent := b.used[user][:0]
	for _, t := range b.used[user] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= b.perMinute {
		b.used[user] = recent
		return false
	}
	b.used[user] = append(recent, now)
	return true
}

// fontType names a font's type from its first bytes: OpenType with CFF outlines (OTTO), or TrueType.
func fontType(b []byte) string {
	if bytes.HasPrefix(b, []byte("OTTO")) {
		return "font/otf"
	}
	return "font/ttf"
}

// GetSubtitleFont makes a subset of the CJK font for the Chinese, Japanese and Korean characters
// in the text given, with the basic Latin (POST /api/media/subtitle-font with {"text": "…"}), for
// the players' ASS renderers: the default font of their lines.
func (h *MediaHandler) GetSubtitleFont(w http.ResponseWriter, r *http.Request) {
	// Subsets take CPU and time: a signed-in user gets a few a minute (the cache answers the rest).
	if claims := auth.GetClaims(r.Context()); claims != nil && !subsetLimit.allow(claims.Username) {
		respondError(w, http.StatusTooManyRequests, "Too many font requests; try again in a minute")
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	font, err := media.SubsetCJK(r.Context(), req.Text)
	switch {
	case errors.Is(err, media.ErrTooManyCharacters):
		respondError(w, http.StatusBadRequest, "Too many characters for one font")
		return
	case err != nil:
		// Clients get no paths or commands: the reason is in the server's log.
		log.Printf("[SUBTITLES] CJK font: %v", err)
		respondError(w, http.StatusServiceUnavailable, "No CJK font on this server")
		return
	}
	w.Header().Set("Content-Type", fontType(font))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(font)
}

// GetSeekPoint reports where a stream that copies the video (transcode=audio) starts when asked to
// start at start: {"start": seconds} (GET /api/media/seekpoint?path=<base64 path>&start=<s>, with
// start as in the stream request). With next=1, the first keyframe at or after start instead, and
// "ask": the start to request for it.
func (h *MediaHandler) GetSeekPoint(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	start := r.URL.Query().Get("start")
	if t, err := strconv.ParseFloat(start, 64); err != nil || t < 0 {
		respondError(w, http.StatusBadRequest, "Invalid start")
		return
	}
	if r.URL.Query().Get("next") == "1" {
		// The keyframe at or after start instead, for a jump forward that would land where it began.
		t, _ := strconv.ParseFloat(start, 64)
		ask, begins, err := media.NextStart(path, t)
		if err != nil {
			respondError(w, http.StatusUnprocessableEntity, "No keyframe after that time")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"start": begins, "ask": ask})
		return
	}
	point, err := media.SeekPoint(path, start)
	if err == nil && point < 0 {
		err = fmt.Errorf("seek point %v", point) // a time ffmpeg couldn't give
	}
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Could not read the file")
		return
	}
	respondJSON(w, http.StatusOK, map[string]float64{"start": point})
}

// GetCodecs reports a file's container and its video and audio codecs, for players that check
// what they can play (GET /api/media/codecs?path=<base64 path>).
func (h *MediaHandler) GetCodecs(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	if respondUnreadable(w, path) {
		return
	}
	codecs, err := media.ProbeCodecs(path)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Could not read the file's codecs")
		return
	}
	addExternalSubtitles(codecs, externalSubtitles(path))
	h.addSubtitleOffsets(path, codecs)
	respondJSON(w, http.StatusOK, codecs)
}

// GetFont serves a font embedded in a video, for the players' ASS renderers (GET /api/media/font?
// path=<base64 path>&font=<index among its attachments, from the codecs report's fonts>).
func (h *MediaHandler) GetFont(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	index, err := strconv.Atoi(r.URL.Query().Get("font"))
	if err != nil || index < 0 {
		respondError(w, http.StatusBadRequest, "Invalid font")
		return
	}
	codecs, err := media.ProbeCodecs(path)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "Could not read the file")
		return
	}
	name := ""
	for _, f := range codecs.Fonts {
		if f.Index == index {
			name = f.Name
		}
	}
	if name == "" {
		respondError(w, http.StatusNotFound, "No such font")
		return
	}
	data, err := media.ReadFont(path, index)
	if err != nil {
		respondError(w, http.StatusNotFound, "No such font")
		return
	}
	w.Header().Set("Content-Type", media.FontMimeType(name))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Write(data)
}
