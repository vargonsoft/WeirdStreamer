package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
	"github.com/weirdstreamer/server/internal/subtitles"
)

// Subtitles outside a video: files beside it and ones downloaded from OpenSubtitles. They follow
// the video's own tracks in GET /api/media/codecs (marked "external"), and GET
// /api/media/subtitles serves them by that index like the video's own text tracks.

// externalSubtitles lists the subtitle files for a video: beside it, then downloaded.
func externalSubtitles(path string) []subtitles.Track {
	tracks := subtitles.Sidecars(path, config.IsVideo)
	return append(tracks, subtitles.Downloaded(config.GlobalConfig.Media.SubtitlePath, path)...)
}

// addExternalSubtitles appends a video's subtitle files to its codecs report, after its own tracks.
func addExternalSubtitles(c *media.Codecs, tracks []subtitles.Track) {
	for _, t := range tracks {
		c.Subtitles = append(c.Subtitles, media.SubtitleCodec{Index: len(c.Subtitles), Codec: t.Codec,
			Language: t.Language, Title: t.Title, Forced: t.Forced, Text: true, External: t.Source,
			HearingImpaired: t.HearingImpaired, Key: "file:" + database.PathHash(t.Path)[:16]})
	}
}

// addSubtitleOffsets keys the video's own tracks and adds each track's timing correction.
func (h *MediaHandler) addSubtitleOffsets(path string, c *media.Codecs) {
	offsets, _ := h.db.SubtitleOffsets(path)
	for i := range c.Subtitles {
		s := &c.Subtitles[i]
		if s.Key == "" {
			s.Key = fmt.Sprintf("stream:%d", s.Index)
		}
		s.OffsetMs = offsets[s.Key]
	}
}

// SetSubtitleOffset saves a subtitle track's timing correction for everyone:
// PUT /api/media/subtitle-offset {path, key, offset_ms} (positive: lines shown later)
func (h *MediaHandler) SetSubtitleOffset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path     string `json:"path"`
		Key      string `json:"key"`
		OffsetMs int    `json:"offset_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	path, ok := h.decodeMediaPath(w, r, req.Path)
	if !ok {
		return
	}
	if !strings.HasPrefix(req.Key, "stream:") && !strings.HasPrefix(req.Key, "file:") || len(req.Key) > 40 {
		respondError(w, http.StatusBadRequest, "Unknown subtitle track")
		return
	}
	if req.OffsetMs < -600_000 || req.OffsetMs > 600_000 {
		respondError(w, http.StatusBadRequest, "Offset out of range")
		return
	}
	if err := h.db.SetSubtitleOffset(path, req.Key, req.OffsetMs); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// subtitleSearch is GET /api/media/subtitles/search's answer.
type subtitleSearch struct {
	Query   subtitleQueryInfo   `json:"query"`
	Results []subtitleSearchHit `json:"results"`
}

// subtitleQueryInfo says what was searched for, for the player to show.
type subtitleQueryInfo struct {
	Title     string   `json:"title,omitempty"`
	Year      int      `json:"year,omitempty"`
	Season    int      `json:"season,omitempty"`
	Episode   int      `json:"episode,omitempty"`
	TMDB      bool     `json:"tmdb"` // by the TMDB match, not the title
	Languages []string `json:"languages,omitempty"`
}

type subtitleSearchHit struct {
	subtitles.Result
	LanguageCode string `json:"language_code"` // ISO 639-2, like the codecs report
	Downloaded   bool   `json:"downloaded"`    // already kept: choosing it costs no download
}

// SearchSubtitles finds subtitles for a video on OpenSubtitles (GET /api/media/subtitles/search?
// path=<base64 path>&languages=en,es). Without languages, it searches in English.
func (h *MediaHandler) SearchSubtitles(w http.ResponseWriter, r *http.Request) {
	t, ok := h.resolveTarget(w, r, r.URL.Query().Get("path"), "")
	if !ok || !h.subtitleTarget(w, t) {
		return
	}
	q := metadata.FileSubtitleQuery(t.path)
	if h.meta != nil {
		q = h.meta.SubtitleQuery(t.lib, t.path)
	}
	q.MovieHash = subtitles.MovieHash(t.path)
	for _, l := range strings.Split(r.URL.Query().Get("languages"), ",") {
		if short := subtitles.Short(l); short != "" {
			q.Languages = append(q.Languages, short)
		}
	}
	if len(q.Languages) == 0 {
		q.Languages = []string{"en"}
	}
	results, err := h.subtitleClient().Search(r.Context(), q)
	if err != nil {
		respondSubtitleError(w, err)
		return
	}
	kept := map[int]bool{}
	for _, d := range subtitles.Downloaded(config.GlobalConfig.Media.SubtitlePath, t.path) {
		kept[d.FileID] = true
	}
	answer := subtitleSearch{
		Query: subtitleQueryInfo{Title: q.Title, Year: q.Year, Season: q.Season, Episode: q.Episode,
			TMDB: q.TMDBID > 0 || q.ParentTMDBID > 0, Languages: q.Languages},
		Results: []subtitleSearchHit{},
	}
	for _, res := range results {
		answer.Results = append(answer.Results, subtitleSearchHit{Result: res,
			LanguageCode: subtitles.Language(res.Language), Downloaded: kept[res.FileID]})
	}
	respondJSON(w, http.StatusOK, answer)
}

// downloadLocks keeps two requests for one video from storing at once.
var downloadLocks sync.Map

// DownloadSubtitle downloads a search result for a video and keeps it in subtitlePath (POST
// /api/media/subtitles/download, {"path": <base64 path>, "result": <a search result>}). Answers
// {"subtitles": the video's subtitle tracks, "index": the new one's, "quota": downloads left
// today}. One already kept isn't downloaded again.
func (h *MediaHandler) DownloadSubtitle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string           `json:"path"`
		Result subtitles.Result `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Result.FileID <= 0 {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	t, ok := h.resolveTarget(w, r, req.Path, "")
	if !ok || !h.subtitleTarget(w, t) {
		return
	}
	root := config.GlobalConfig.Media.SubtitlePath
	if root == "" {
		respondError(w, http.StatusServiceUnavailable, "No subtitlePath is configured for downloaded subtitles")
		return
	}
	lock, _ := downloadLocks.LoadOrStore(t.path, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	var quota *subtitles.Quota
	if !hasDownload(root, t.path, req.Result.FileID) {
		data, q, err := h.subtitleClient().Download(r.Context(), req.Result.FileID)
		quota = &q
		if err != nil {
			respondSubtitleError(w, err)
			return
		}
		if _, err := subtitles.Store(root, t.path, req.Result, data); err != nil {
			log.Printf("[SUBTITLES] Failed to keep a download for %s: %v", t.path, err)
			respondError(w, http.StatusInternalServerError, "Could not keep the subtitle")
			return
		}
		log.Printf("[SUBTITLES] Downloaded %q (%s) for %s; %d downloads left today", req.Result.Release,
			req.Result.Language, t.path, q.Remaining)
	}
	codecs, err := media.ProbeCodecs(t.path)
	if err != nil {
		codecs = &media.Codecs{}
	}
	own := len(codecs.Subtitles)
	external := externalSubtitles(t.path)
	addExternalSubtitles(codecs, external)
	h.addSubtitleOffsets(t.path, codecs)
	index := -1
	for i, tr := range external {
		if tr.Source == subtitles.SourceDownload && tr.FileID == req.Result.FileID {
			index = own + i
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"subtitles": codecs.Subtitles, "index": index, "quota": quota})
}

func hasDownload(root, path string, fileID int) bool {
	for _, d := range subtitles.Downloaded(root, path) {
		if d.FileID == fileID {
			return true
		}
	}
	return false
}

// subtitleTarget checks the request is for a video file, writing the error if not.
func (h *MediaHandler) subtitleTarget(w http.ResponseWriter, t *mediaTarget) bool {
	if t.isDir || !h.isValidMediaPath(t.path) {
		respondError(w, http.StatusBadRequest, "Not a video")
		return false
	}
	return true
}

func (h *MediaHandler) subtitleClient() *subtitles.Client {
	return subtitles.NewClient(config.GlobalConfig.Metadata.OpenSubtitlesAPIKey)
}

func respondSubtitleError(w http.ResponseWriter, err error) {
	if errors.Is(err, subtitles.ErrNoKey) {
		respondError(w, http.StatusServiceUnavailable, "No OpenSubtitles API key is configured (Settings → Metadata)")
		return
	}
	log.Printf("[SUBTITLES] %v", err)
	respondError(w, http.StatusBadGateway, err.Error())
}
