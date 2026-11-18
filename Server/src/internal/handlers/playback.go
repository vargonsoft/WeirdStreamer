package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
)

// Each user's playback progress: recorded from player reports, attached to browse and library
// results, and listed as Continue Watching.

const (
	// Videos at least this long count as watched within the last creditsAllowance; shorter ones
	// only when played to the end.
	longVideo        = 20 * 60
	creditsAllowance = 2 * 60
	// Positions before this aren't worth resuming from.
	minResumePosition = 10
	continueLimit     = 30
)

// playbackFinished reports whether a position counts as having watched the video. creditsStart is
// where its detected credits begin (0 when none was found): reaching them counts as finished.
func playbackFinished(position, duration, creditsStart float64, ended bool) bool {
	if ended {
		return true
	}
	if duration <= 0 {
		return false
	}
	if creditsStart > 0 && position >= creditsStart {
		return true
	}
	if duration >= longVideo {
		return position >= duration-creditsAllowance
	}
	return position >= duration-1 // allow for the player's last report landing just short of the end
}

func userID(r *http.Request) (int64, bool) {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		return 0, false
	}
	return claims.UserID, true
}

// videoDuration is a video's length for progress: a disc's main title, else the file's.
func (h *MediaHandler) videoDuration(item *models.MediaItem) float64 {
	if media.IsDiscImage(item.Path) {
		if info, err := media.LoadDiscWithin(item.Path, media.DiscRequestWait); err == nil {
			if t := info.Title(0); t != nil {
				return t.Duration
			}
		}
		return 0
	}
	if item.Duration <= 0 {
		if duration, width, height, err := media.ProbeVideo(item.Path); err == nil && duration > 0 {
			item.Duration, item.Width, item.Height = duration, width, height
			h.db.SaveMediaItem(item)
		}
	}
	return item.Duration
}

// creditsStart is where a video's detected credits begin (0 when none was found).
func (h *MediaHandler) creditsStart(path string) float64 {
	segs, err := h.db.GetSegments(path)
	if err != nil || segs.Credits == nil {
		return 0
	}
	return segs.Credits.Start
}

// recordPlayback saves a player's report as the user's progress in the video.
func (h *MediaHandler) recordPlayback(r *http.Request, item *models.MediaItem, position float64, ended bool, at time.Time, onlyIfNewer bool) {
	uid, ok := userID(r)
	if !ok {
		return
	}
	duration := h.videoDuration(item)
	update := database.PlaybackUpdate{Position: position, Duration: duration,
		Finished: playbackFinished(position, duration, h.creditsStart(item.Path), ended), UpdatedAt: at}
	if err := h.db.SavePlayback(uid, item.Path, update, onlyIfNewer); err != nil {
		log.Printf("[PLAYBACK] Failed to save progress: %v", err)
	}
}

// requestPlayback loads the signed-in user's progress for annotating results (nil if none).
func (h *MediaHandler) requestPlayback(r *http.Request) map[string]*models.Playback {
	uid, ok := userID(r)
	if !ok {
		return nil
	}
	pb, err := h.db.UserPlayback(uid)
	if err != nil {
		log.Printf("[PLAYBACK] Failed to load progress: %v", err)
		return nil
	}
	return pb
}

func annotateItems(items []models.MediaItem, pb map[string]*models.Playback) {
	for i := range items {
		if items[i].Type == "video" {
			items[i].Playback = pb[items[i].Path]
		}
	}
}

func annotateFiles(files []models.LibraryFile, pb map[string]*models.Playback) {
	for i := range files {
		if files[i].Count == 0 { // not an unmatched show's stand-in
			files[i].Playback = pb[files[i].Path]
		}
	}
}

func annotateTitle(t *models.TitleDetail, pb map[string]*models.Playback) {
	annotateFiles(t.Files, pb)
	annotateFiles(t.OtherFiles, pb)
	for s := range t.Seasons {
		for e := range t.Seasons[s].Episodes {
			annotateFiles(t.Seasons[s].Episodes[e].Files, pb)
		}
	}
}

// annotateListing adds each title's progress: files watched and, for a movie in progress, how far.
func annotateListing(l *models.LibraryListing, pb map[string]*models.Playback) {
	annotateFiles(l.Unmatched, pb)
	for i := range l.Titles {
		t := &l.Titles[i]
		var latest *models.Playback
		for _, path := range t.Paths {
			p := pb[path]
			if p == nil {
				continue
			}
			if p.Watched {
				t.Watched++
			}
			if p.Position >= minResumePosition && p.Duration > 0 && (latest == nil || p.UpdatedAt > latest.UpdatedAt) {
				latest = p
			}
		}
		if t.Kind == "movie" && latest != nil {
			t.Progress = min(latest.Position/latest.Duration, 1)
		}
	}
}

type watchedRequest struct {
	Paths   []string `json:"paths"` // base64 client paths
	Watched bool     `json:"watched"`
	// Or a whole title: every file of a movie or show in a library
	Library string `json:"library,omitempty"`
	Kind    string `json:"kind,omitempty"`
	TMDBID  int    `json:"tmdb_id,omitempty"`
}

// SetWatched marks videos (or every file of a title) watched or unwatched for the user:
// POST /api/playback/watched
func (h *MediaHandler) SetWatched(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	var req watchedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	var paths []string
	for _, encoded := range req.Paths {
		path, ok := h.decodeMediaPath(w, r, encoded)
		if !ok {
			return
		}
		paths = append(paths, path)
	}
	if req.Library != "" && h.meta != nil {
		lib, found := libraryByID(req.Library)
		if !found || !h.canAccessLibrary(r, lib) {
			respondError(w, http.StatusForbidden, "Access denied")
			return
		}
		detail, err := h.meta.Title(r.Context(), lib, req.Kind, req.TMDBID, func(path string) models.LibraryFile {
			return models.LibraryFile{Path: path}
		})
		if err != nil {
			respondError(w, http.StatusNotFound, "Title not found")
			return
		}
		for _, files := range titleFiles(detail) {
			for _, f := range files {
				paths = append(paths, f.Path)
			}
		}
	}
	if err := h.db.SetWatched(uid, paths, req.Watched); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func titleFiles(t *models.TitleDetail) [][]models.LibraryFile {
	out := [][]models.LibraryFile{t.Files, t.OtherFiles}
	for _, s := range t.Seasons {
		for _, e := range s.Episodes {
			out = append(out, e.Files)
		}
	}
	return out
}

func libraryByID(id string) (config.MediaPath, bool) {
	for _, lib := range config.GlobalConfig.Media.Paths {
		if lib.ID == id {
			return lib, true
		}
	}
	return config.MediaPath{}, false
}

type syncedPlayback struct {
	Path      string  `json:"path"` // base64 client path
	Position  float64 `json:"position"`
	Ended     bool    `json:"ended"`
	UpdatedAt int64   `json:"updated_at"` // Unix milliseconds, when it was played
}

// SyncPlayback takes progress recorded while offline (downloaded videos), applying each report
// unless the server has a newer one: POST /api/playback/sync
func (h *MediaHandler) SyncPlayback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []syncedPlayback `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	now := time.Now()
	for _, it := range req.Items {
		path, status := h.filePath(r, it.Path)
		if status != http.StatusOK {
			continue
		}
		item, err := h.db.GetMediaItem(path)
		if err != nil || item.Type != "video" {
			continue // gone from the server since it was downloaded
		}
		at := time.UnixMilli(it.UpdatedAt)
		if at.After(now) {
			at = now
		}
		h.recordPlayback(r, item, it.Position, it.Ended, at, true)
	}
	w.WriteHeader(http.StatusNoContent)
}

// continueItem is a video in progress, described like a browse result.
type continueItem struct {
	models.MediaItem
	LibraryID   string `json:"library_id"`
	LibraryType string `json:"library_type,omitempty"`
	UpNext      bool   `json:"up_next,omitempty"`  // the episode after one just finished, not started yet
	AddedAt     int64  `json:"added_at,omitempty"` // on Recently Added: when it arrived (Unix seconds)
}

// ContinueWatching lists the user's videos in progress, most recent first, optionally in one
// library: GET /api/playback/continue?lib=<id>
func (h *MediaHandler) ContinueWatching(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	only := r.URL.Query().Get("lib")
	limit := continueLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n < continueLimit {
		limit = n
	}
	// Some rows may be filtered out (other libraries, removed files): read a few extra. Finished
	// videos are read apart (so a run of them can't crowd out those in progress) and merged in by
	// time: a finished episode hands over to the next one.
	paths, pb, err := h.db.RecentPlayback(uid, minResumePosition, limit*4)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load progress")
		return
	}
	if done, donePB, err := h.db.RecentFinished(uid, minResumePosition, limit*4); err == nil {
		for path, p := range donePB {
			pb[path] = p
		}
		paths = append(paths, done...)
		sort.SliceStable(paths, func(i, j int) bool { return pb[paths[i]].UpdatedAt > pb[paths[j]].UpdatedAt })
	}
	items := []continueItem{}
	listed := map[string]bool{}         // paths on the list
	shows := map[int]bool{}             // shows with an entry already (a newer one than the rows that follow)
	var all map[string]*models.Playback // every record, read when an up-next episode needs checking
	for _, path := range paths {
		if len(items) >= limit {
			break
		}
		p := pb[path]
		finished := (p.Watched && p.Position < minResumePosition) || (p.Duration > 0 && playbackFinished(p.Position, p.Duration, h.creditsStart(path), false))
		lib, found := libraryForPath(path)
		// A library's own page asks for its list (only); the home screen blends all libraries.
		if !found || !lib.OnContinueWatching(only != "") || (only != "" && lib.ID != only) || !h.canAccessLibrary(r, lib) {
			continue
		}
		item := h.continueEntry(lib, path)
		if item == nil {
			continue
		}
		item.Playback = p
		show := episodeShow(item)
		if finished {
			// A finished episode hands over to the next, so the show stays on the list; unless the
			// show already has a newer entry, or the next has been watched or is listed already.
			if show == 0 || shows[show] {
				continue
			}
			shows[show] = true
			if all == nil {
				if all, err = h.db.UserPlayback(uid); err != nil {
					all = map[string]*models.Playback{}
				}
			}
			// The first episode after this one not watched yet (marking a season watched leaves
			// its episodes in any order). One taken off the list since is left off.
			next := ""
			for _, candidate := range h.episodesAfter(r.Context(), lib, show, path) {
				if np := all[candidate]; np == nil || !np.Watched {
					if np == nil || np.UpdatedAt < p.UpdatedAt || np.Position >= minResumePosition {
						next = candidate
					}
					break
				}
			}
			if next == "" || listed[next] {
				continue
			}
			nextItem := h.continueEntry(lib, next)
			if nextItem == nil {
				continue
			}
			nextItem.Playback = all[next]
			listed[next] = true
			items = append(items, continueItem{MediaItem: *nextItem, LibraryID: lib.ID, LibraryType: lib.Type, UpNext: true})
			continue
		}
		if listed[path] {
			continue
		}
		if show != 0 {
			shows[show] = true
		}
		listed[path] = true
		items = append(items, continueItem{MediaItem: *item, LibraryID: lib.ID, LibraryType: lib.Type})
	}
	clientContinue(items)
	respondJSON(w, http.StatusOK, items)
}

// clientContinue is clientItems for a Continue Watching or Recently Added list.
func clientContinue(items []continueItem) {
	for i := range items {
		items[i].MediaItem = *clientItem(&items[i].MediaItem)
	}
}

// continueEntry is a video for Continue Watching, with its TMDB summary, or nil when it isn't one.
func (h *MediaHandler) continueEntry(lib config.MediaPath, path string) *models.MediaItem {
	item, err := h.db.GetMediaItem(path)
	if err != nil || item.Type != "video" {
		return nil
	}
	if h.meta != nil {
		one := []models.MediaItem{*item}
		h.meta.Annotate(lib, filepath.Dir(path), nil, one)
		item = &one[0]
	}
	return item
}

// episodeShow is the TMDB ID of the show an item is an episode of, or 0.
func episodeShow(item *models.MediaItem) int {
	if item.Metadata != nil && item.Metadata.Kind == "episode" {
		return item.Metadata.TMDBID
	}
	return 0
}

// showEpisodes lists a show's episodes in order, each as its season number and its files' paths.
func (h *MediaHandler) showEpisodes(ctx context.Context, lib config.MediaPath, show int) []showEpisode {
	if h.meta == nil {
		return nil
	}
	detail, err := h.meta.Title(ctx, lib, "show", show, func(path string) models.LibraryFile {
		return models.LibraryFile{Path: path}
	})
	if err != nil {
		return nil
	}
	var out []showEpisode
	for _, s := range detail.Seasons {
		for _, e := range s.Episodes {
			ep := showEpisode{season: s.Season}
			for _, f := range e.Files {
				ep.paths = append(ep.paths, f.Path)
			}
			if len(ep.paths) > 0 {
				out = append(out, ep)
			}
		}
	}
	return out
}

type showEpisode struct {
	season int
	paths  []string
}

func (e showEpisode) has(path string) bool {
	for _, p := range e.paths {
		if p == path {
			return true
		}
	}
	return false
}

// episodesAfter lists the first file of each episode after path's, in order (on into the next
// season, skipping specials unless path is one).
func (h *MediaHandler) episodesAfter(ctx context.Context, lib config.MediaPath, show int, path string) []string {
	eps := h.showEpisodes(ctx, lib, show)
	for i, e := range eps {
		if !e.has(path) {
			continue
		}
		var out []string
		for _, n := range eps[i+1:] {
			if n.season != 0 || e.season == 0 {
				out = append(out, n.paths[0])
			}
		}
		return out
	}
	return nil
}

// Upcoming lists the episodes after a video, for the phone's Download ahead:
// GET /api/media/upcoming?path=<base64>&count=N (1 to 10, default 5). A video that isn't an
// episode, or the last one, has none.
func (h *MediaHandler) Upcoming(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	count := 5
	if n, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && n > 0 {
		count = min(n, 10)
	}
	items := []continueItem{}
	if lib, found := libraryForPath(path); found {
		if item := h.continueEntry(lib, path); item != nil {
			if show := episodeShow(item); show != 0 {
				for _, next := range h.episodesAfter(r.Context(), lib, show, path) {
					if len(items) == count {
						break
					}
					if e := h.continueEntry(lib, next); e != nil {
						items = append(items, continueItem{MediaItem: *e, LibraryID: lib.ID, LibraryType: lib.Type})
					}
				}
			}
		}
	}
	clientContinue(items)
	respondJSON(w, http.StatusOK, map[string]any{"episodes": items})
}

// RemoveFromContinue forgets where the user stopped in a video, taking it off Continue
// Watching: DELETE /api/playback/continue?path=<base64>
func (h *MediaHandler) RemoveFromContinue(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	if err := h.db.ClearPosition(uid, path); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
