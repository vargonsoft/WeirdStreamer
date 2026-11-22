package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/jobs"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/metadata"
	"github.com/weirdstreamer/server/internal/models"
)

// SSE client for thumbnail notifications
type MediaHandler struct {
	db                 *database.DB
	streamer           *media.Streamer
	scanner            *media.Scanner
	thumbnailGenerator *media.ThumbnailGenerator
	thumbnailHub       *thumbnailHub
	authService        *auth.Service
	meta               *metadata.Service // nil until SetMetadata
	streams            *media.StreamTracker
	downloads          *media.DownloadManager // nil until SetDownloads
	segments           *media.SegmentAnalyzer // nil until StartSegmentAnalysis
	pruner             *media.Pruner          // nil until StartCleanup
	trickplay          *media.TrickplayJob    // nil until StartTrickplay
	assCache           *media.AssJob          // nil until StartAssCache
	thumbnails         *thumbnailQueue        // missing thumbnails pages asked for
}

// StartTrickplay makes missing trickplay frames in the background until ctx ends.
func (h *MediaHandler) StartTrickplay(ctx context.Context) {
	h.trickplay = media.NewTrickplayJob(h.db, h.streams, func(path string) bool {
		_, found := libraryForPath(path)
		return found
	})
	go h.trickplay.Run(ctx)
}

// StartAssCache copies videos' ASS/SSA tracks out of them in the background, until ctx ends.
func (h *MediaHandler) StartAssCache(ctx context.Context) {
	h.assCache = media.NewAssJob(h.db, h.streams, func(path string) bool {
		_, found := libraryForPath(path)
		return found
	})
	go h.assCache.Run(ctx)
}

// StartCleanup forgets files that are gone, in the background until ctx ends.
func (h *MediaHandler) StartCleanup(ctx context.Context) {
	h.pruner = media.NewPruner(h.db)
	go h.pruner.Run(ctx)
}

// shareProbes names the biggest video of each library whose files are on a network share (by where
// that video really is, so symlinks count; a library inside another is that library's), for the
// load monitor to time reads from.
func (h *MediaHandler) shareProbes() []media.ShareProbe {
	libs := config.GlobalConfig.Media.Paths
	var out []media.ShareProbe
	for _, lib := range libs {
		root, err := filepath.Abs(lib.Path)
		if err != nil {
			continue
		}
		nested := false
		for _, other := range libs {
			if o, err := filepath.Abs(other.Path); err == nil && o != root && strings.HasPrefix(root, o+string(filepath.Separator)) {
				nested = true
			}
		}
		if nested {
			continue
		}
		p, err := h.db.LargestVideo(root)
		if err != nil || p == "" {
			continue
		}
		if real, err := filepath.EvalSymlinks(p); err == nil && media.IsNetworkShare(filepath.Dir(real)) {
			out = append(out, media.ShareProbe{Library: lib.Name, Path: real})
		}
	}
	return out
}

// StartSegmentAnalysis finds videos' intros and credits in the background until ctx ends. It and
// the other background jobs run while the machine has room to spare (CPU and disk use, and how
// long reads from network shares take).
func (h *MediaHandler) StartSegmentAnalysis(ctx context.Context) {
	media.SetLoadMonitor(media.NewLoadMonitor(ctx, h.shareProbes))
	h.segments = media.NewSegmentAnalyzer(h.db, h.streams, func(path string) (string, bool) {
		lib, found := libraryForPath(path)
		return lib.Type, found
	})
	go h.segments.Run(ctx)
}

// SetMetadata enables TMDB metadata for movie and tv libraries.
func (h *MediaHandler) SetMetadata(meta *metadata.Service) {
	h.meta = meta
}

// NotifyMetadataReady tells SSE clients that may access path that its metadata changed. Sent as
// a named "metadata" event so clients that only handle thumbnail messages ignore it.
func (h *MediaHandler) NotifyMetadataReady(path string) {
	data, err := json.Marshal(map[string]string{"path": clientPath(path)})
	if err == nil {
		h.thumbnailHub.broadcast(path, sseEvent{name: "metadata", data: string(data)})
	}
}

func NewMediaHandler(db *database.DB, authService *auth.Service) *MediaHandler {
	h := &MediaHandler{
		db:                 db,
		streamer:           media.NewStreamer(),
		scanner:            media.NewScanner(db),
		thumbnailGenerator: media.NewThumbnailGenerator(),
		thumbnailHub:       newThumbnailHub(),
		authService:        authService,
		streams:            media.NewStreamTracker(),
	}
	h.thumbnails = newThumbnailQueue(h.makeThumbnail)
	if j := jobs.Get("scan"); j != nil {
		j.OnRunNow(h.startScan)
	}
	return h
}

// ListMedia godoc
// @Summary List media items
// @Description Get list of all media items, optionally filtered by type
// @Tags media
// @Produce json
// @Param type query string false "Media type filter (video, image, document)"
// @Success 200 {array} models.MediaItem
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media [get]
func (h *MediaHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	if userGroup(r) == "" {
		respondJSON(w, http.StatusOK, []models.MediaItem{})
		return
	}

	mediaType := r.URL.Query().Get("type")

	items, err := h.db.ListMediaItems(mediaType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list media")
		return
	}

	// Filter items based on library access
	filteredItems := []models.MediaItem{}
	for _, item := range items {
		if h.canAccessPath(r, item.Path) {
			filteredItems = append(filteredItems, item)
		}
	}
	clientItems(filteredItems)

	respondJSON(w, http.StatusOK, filteredItems)
}

// GetMediaInfo godoc
// @Summary Get media item info
// @Description Get detailed information about a specific media item
// @Tags media
// @Produce json
// @Param id path string true "Media ID"
// @Success 200 {object} models.MediaItem
// @Failure 404 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/{id} [get]
func (h *MediaHandler) GetMediaInfo(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, mux.Vars(r)["path"])
	if !ok {
		return
	}

	item, err := h.db.GetMediaItem(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}

	respondJSON(w, http.StatusOK, clientItem(item))
}

// GetTrickplayInfo godoc
// @Summary Get trickplay frames
// @Description Get URLs for trickplay frames for a given media ID
// @Tags media
// @Produce json
// @Param id path int true "Media ID"
// @Success 200 {object} models.TrickplayInfo
// @Failure 404 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/trickplay/{id} [get]
func (h *MediaHandler) GetTrickplayInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Printf("[TRICKPLAY] Invalid ID: %s", idStr)
		respondError(w, http.StatusBadRequest, "Invalid media ID")
		return
	}

	// Get media item from database
	item, err := h.db.GetMediaItemByID(id)
	if err != nil {
		log.Printf("[TRICKPLAY] Media ID %d not found in DB", id)
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}

	// Check access
	if !h.canAccessPath(r, item.Path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	log.Printf("[TRICKPLAY] Request for media ID %d: %s", id, item.Name)
	h.respondTrickplay(w, item.Path, item.Name)
}

// GetTrickplayInfoByPath is GetTrickplayInfo for a file by its path, for a download (which keeps
// the path, not the ID): GET /api/media/trickplay?path=<base64>
func (h *MediaHandler) GetTrickplayInfoByPath(w http.ResponseWriter, r *http.Request) {
	path, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	h.respondTrickplay(w, path, filepath.Base(path))
}

// respondTrickplay answers with a video's trickplay frames, by time.
func (h *MediaHandler) respondTrickplay(w http.ResponseWriter, path, name string) {

	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		log.Printf("[TRICKPLAY] Error: Trickplay path not configured in config.yml")
		respondError(w, http.StatusInternalServerError, "Trickplay path not configured")
		return
	}

	hash := media.GenerateThumbnailHash(path)
	trickplayVideoDir := filepath.Join(trickplayDir, hash)

	files, err := os.ReadDir(trickplayVideoDir)
	if err != nil {
		log.Printf("[TRICKPLAY] No frames found for %s (hash: %s) in directory: %s", name, hash, trickplayVideoDir)
		respondError(w, http.StatusNotFound, "Trickplay frames not found")
		return
	}

	var frames []models.TrickplayFrame
	for _, file := range files {
		if !file.IsDir() {
			name := file.Name()
			if strings.HasPrefix(name, "frame-") && strings.HasSuffix(name, ".jpg") {
				timeStr := strings.TrimPrefix(strings.TrimSuffix(name, ".jpg"), "frame-")
				timeSec, err := strconv.Atoi(timeStr)
				if err == nil {
					url := filepath.ToSlash(filepath.Join("/trickplay", hash, name))
					frames = append(frames, models.TrickplayFrame{
						Time: timeSec,
						URL:  url,
					})
				}
			}
		}
	}

	// Sort frames by time
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].Time < frames[j].Time
	})

	log.Printf("[TRICKPLAY] Returning %d frames for %s", len(frames), name)

	info := models.TrickplayInfo{
		Frames: frames,
		Title:  media.TrickplayTitle(path), // a disc image's: the title they're for
	}

	respondJSON(w, http.StatusOK, info)
}

// StreamMedia godoc
// @Summary Stream media file
// @Description Stream video or image file with range request support
// @Tags media
// @Produce application/octet-stream
// @Param path query string true "File path"
// @Success 200
// @Success 206
// @Failure 404 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/stream [get]
func (h *MediaHandler) StreamMedia(w http.ResponseWriter, r *http.Request) {
	encodedPath := r.URL.Query().Get("path")
	if encodedPath == "" {
		respondError(w, http.StatusBadRequest, "Missing path parameter")
		return
	}

	path, ok := h.decodeMediaPath(w, r, encodedPath)
	if !ok {
		return
	}

	// Log the request for debugging
	log.Printf("Stream request - Path: %s, User-Agent: %s", path, r.Header.Get("User-Agent"))

	// Check if user wants to force transcoding
	forceTranscode := r.URL.Query().Get("transcode") == "1"
	// transcode=audio: copy the video, encode the audio to AAC (the browser can't play the audio)
	audioTranscode := r.URL.Query().Get("transcode") == "audio"

	// Check if client wants to disable auto-transcoding (enabled by default)
	// WebUI can set auto-transcode=0 to let browser attempt native playback first
	autoTranscode := r.URL.Query().Get("auto-transcode") != "0"

	// Get start time for seeking in transcoded streams
	startTime := r.URL.Query().Get("start")

	// Get bitrate and resolution for custom transcoding
	bitrate := r.URL.Query().Get("bitrate")
	resolution := r.URL.Query().Get("resolution")
	// A height in pixels: it goes into ffmpeg's filter text, where anything else could add filters.
	if resolution != "" {
		if n, err := strconv.Atoi(resolution); err != nil || n <= 0 {
			respondError(w, http.StatusBadRequest, "resolution must be a height in pixels")
			return
		}
	}

	item, err := h.db.GetMediaItem(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}
	if respondUnreadable(w, path) {
		return
	}

	sess := h.beginStream(r, item)
	defer sess.End()

	if err := h.streamer.ServeContent(w, r, path, item.MimeType, forceTranscode, audioTranscode, autoTranscode, startTime, config.GlobalConfig.Streaming.AutoConvertImages, bitrate, resolution, sess); err != nil {
		log.Printf("[STREAM] Failed to stream %s: %v", path, err)
		if respondDiscBusy(w, err) {
			return
		}
		if errors.Is(err, media.ErrFFmpegStalled) {
			w.Header().Set(unreadableHeader, "unreadable")
			respondError(w, http.StatusServiceUnavailable, "Can't play this title: "+err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "Failed to stream media")
		return
	}
}

// respondDiscBusy answers a request that didn't wait for a disc still being read (503 with
// Retry-After), if that's what err says.
func respondDiscBusy(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, media.ErrDiscReading) {
		return false
	}
	w.Header().Set("Retry-After", "30")
	respondError(w, http.StatusServiceUnavailable, "The server is still reading this disc. Try again in a minute.")
	return true
}

// unreadableHeader marks a response for a video the server can't read, so players stop trying
// other ways to play it and show the message instead.
const unreadableHeader = "X-Stream-Error"

// respondUnreadable answers with why path can't be read, if it can't, and logs it.
func respondUnreadable(w http.ResponseWriter, path string) bool {
	problem := media.CheckReadable(path)
	if problem == nil {
		return false
	}
	reason := problem.Err
	var pathErr *os.PathError
	if errors.As(reason, &pathErr) {
		reason = pathErr.Err // the path is in the line already
	}
	log.Printf("[STREAM] Can't read %s: %v", path, reason)
	w.Header().Set(unreadableHeader, "unreadable")
	respondError(w, problem.Status, problem.Message+".")
	return true
}

// GetMediaMetadata godoc
// @Summary Get media metadata
// @Description Get metadata (duration, dimensions, etc.) for a media file
// @Tags media
// @Produce json
// @Param path query string true "Base64-encoded file path"
// @Success 200 {object} models.MediaItem
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/metadata [get]
func (h *MediaHandler) GetMediaMetadata(w http.ResponseWriter, r *http.Request) {
	encodedPath := r.URL.Query().Get("path")
	if encodedPath == "" {
		respondError(w, http.StatusBadRequest, "Missing path parameter")
		return
	}

	path, ok := h.decodeMediaPath(w, r, encodedPath)
	if !ok {
		return
	}

	log.Printf("[METADATA] Request for path: %s", path)

	// Try to get from database first
	item, err := h.db.GetMediaItem(path)
	if err != nil || item.Duration == 0 {
		// Not in database or no duration - get metadata in real-time
		log.Printf("[METADATA] Fetching in real-time for: %s", path)
		metadata, err := h.scanner.GetFileMetadata(path)
		if err != nil {
			log.Printf("[METADATA] Failed to get real-time metadata for %s: %v", path, err)
			respondError(w, http.StatusInternalServerError, "Failed to get metadata")
			return
		}
		// If we got real-time metadata but the file is in the database, preserve the ID
		if err == nil && item != nil {
			metadata.ID = item.ID
		}
		respondJSON(w, http.StatusOK, clientItem(metadata))
		return
	}

	log.Printf("[METADATA] Returning cached metadata for: %s (Duration: %.2fs)", item.Name, item.Duration)
	respondJSON(w, http.StatusOK, clientItem(item))
}

// BrowseDirectory godoc
// @Summary Browse directory
// @Description Browse media directory structure
// @Tags media
// @Produce json
// @Param path query string true "Folder (base64 client path)"
// @Success 200 {object} models.Directory
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/browse [get]
func (h *MediaHandler) BrowseDirectory(w http.ResponseWriter, r *http.Request) {
	relativePath := r.URL.Query().Get("path")

	var dirs []models.Directory
	var items []models.MediaItem

	// If no path specified, show root media directories
	if relativePath == "" {
		for _, mediaPath := range config.GlobalConfig.Media.Paths {
			if stat, err := media.StatWaking(mediaPath.Path); err == nil && stat.IsDir() {
				// Check access
				if !h.canAccessLibrary(r, mediaPath) {
					continue
				}

				// Use configured name if available, otherwise fall back to base path
				displayName := mediaPath.Name
				if displayName == "" {
					displayName = filepath.Base(mediaPath.Path)
				}
				// Expose stable ID as the directory path at root so clients can reference it
				dirPath := mediaPath.ID
				if dirPath == "" {
					dirPath = filepath.Base(mediaPath.Path)
				}
				dirs = append(dirs, models.Directory{
					Path:        dirPath,
					Name:        displayName,
					LibraryType: mediaPath.Type,
				})
			}
		}

		result := models.Directory{
			Path:  "",
			Name:  "Media",
			Dirs:  dirs,
			Items: []models.MediaItem{},
		}
		respondJSON(w, http.StatusOK, result)
		return
	}

	// Browse within a specific media path
	var targetPath string
	var targetLib config.MediaPath
	pathParts := strings.Split(strings.Trim(relativePath, "/"), "/")

	// Find which root this belongs to. Match only by the stable ID.
	found := false
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		if mediaPath.ID != "" && mediaPath.ID == pathParts[0] {
			if len(pathParts) > 1 {
				targetPath = filepath.Join(mediaPath.Path, filepath.Join(pathParts[1:]...))
			} else {
				targetPath = mediaPath.Path
			}
			targetLib = mediaPath
			found = true
			break
		}
	}

	if !found {
		respondError(w, http.StatusNotFound, "Path not found")
		return
	}

	// Check access
	if !h.canAccessLibrary(r, targetLib) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// The resolved path must stay inside the library it was addressed through ("../" can't
	// escape into another library or outside the media paths).
	if lib, ok := libraryForPath(targetPath); !ok || lib.ID != targetLib.ID {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil && media.Wake(targetPath, err) {
		entries, err = os.ReadDir(targetPath)
	}
	if err != nil {
		respondError(w, http.StatusNotFound, "Directory not found")
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetPath, entry.Name())
		relPath := filepath.Join(relativePath, entry.Name())

		// Skip Mac-specific files if configured to ignore them
		if media.IgnoredMacFile(entry.Name()) {
			continue
		}

		// A symlink is listed as what it points to: its size, and a folder as a folder.
		isDir := entry.IsDir()
		var linked os.FileInfo
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(entryPath); err == nil {
				linked = info
				isDir = info.IsDir()
			}
		}

		if isDir {
			dirs = append(dirs, models.Directory{
				Path: relPath,
				Name: entry.Name(),
			})
		} else {
			// Get file info
			fileInfo, err := entry.Info()
			if linked != nil {
				fileInfo, err = linked, nil
			}
			if err != nil {
				log.Printf("Failed to get file info for %s: %v", entryPath, err)
				continue
			}

			// Determine media type based on extension
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			mediaType := config.MediaType(entryPath)

			// Only include supported media files
			if mediaType == "" {
				continue
			}
			// An installer or data disc image isn't a video: scans leave it out, and so does this.
			if media.IsNonDiscISOWithin(entryPath, 0) {
				continue
			}

			// Try to get from database first (for cached metadata/thumbnails)
			item, err := h.db.GetMediaItem(entryPath)
			if err != nil {
				// Not in database - create item with basic info
				item = &models.MediaItem{
					Path:     entryPath,
					Name:     entry.Name(),
					Type:     mediaType,
					Size:     fileInfo.Size(),
					MimeType: media.MimeType(ext),
				}

				// Its thumbnail is made in the background (images and videos); browsing doesn't wait
				if mediaType == "image" || mediaType == "video" {
					h.thumbnails.add(*item, true)
				} else {
					// Save non-media items immediately
					go h.db.SaveMediaItem(item)
				}
			} else {
				// Item found in database. A size that's out of date (the file changed, or a
				// symlink's own size was stored) is corrected.
				if item.Size != fileInfo.Size() {
					item.Size = fileInfo.Size()
					fixed := *item // a copy: the thumbnail goroutine below may change item
					go h.db.SaveMediaItem(&fixed)
				}
				// If thumbnail URL is empty, regenerate it in background
				if item.ThumbnailURL == "" && (mediaType == "image" || mediaType == "video") {
					h.thumbnails.add(*item, false)
				}
			}

			items = append(items, *item)
		}
	}
	items = media.PairLivePhotos(items)

	if h.meta != nil {
		if absTarget, err := filepath.Abs(targetPath); err == nil {
			h.meta.Annotate(targetLib, absTarget, dirs, items)
		}
	}
	annotateItems(items, h.requestPlayback(r))
	clientItems(items)

	result := models.Directory{
		Path:        relativePath,
		Name:        filepath.Base(relativePath),
		Dirs:        dirs,
		Items:       items,
		LibraryType: targetLib.Type,
	}

	respondJSON(w, http.StatusOK, result)
}

// ScanMedia godoc
// @Summary Scan media directories
// @Description Trigger a scan of configured media directories (admin only)
// @Tags media
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/scan [post]
func (h *MediaHandler) ScanMedia(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	h.startScan()
	respondJSON(w, http.StatusOK, map[string]string{"status": "scan started"})
}

// scanInterval is how often the libraries are scanned for new files on their own, so they reach
// Recently Added, the Library view and the other jobs without anyone pressing Scan Media.
const scanInterval = time.Hour

// StartScans scans the libraries every scanInterval until ctx ends (the first soon after start,
// once the server's own startup work is done).
func (h *MediaHandler) StartScans(ctx context.Context) {
	j := jobs.Get("scan")
	wait := 10 * time.Minute
	for {
		if j != nil {
			j.NextIn(wait)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		h.scan()
		if h.meta != nil {
			h.meta.SyncAll(ctx)
		}
		wait = scanInterval
	}
}

// startScan scans the libraries in the background; the TMDB sync runs alongside. (Scan Media,
// and the scan's Run now on the Jobs page.)
func (h *MediaHandler) startScan() {
	go h.scan()
	if h.meta != nil {
		go h.meta.SyncAll(context.Background())
	}
}

// scan adds new and changed files, then has the other jobs pick up what changed.
func (h *MediaHandler) scan() {
	if err := h.scanner.ScanMediaPaths(); errors.Is(err, media.ErrScanRunning) {
		return // the scan running wakes them when it's done
	} else if err != nil {
		log.Printf("[SCAN] %v", err)
	}
	if h.segments != nil {
		h.segments.Wake() // new videos' intros and credits
	}
	if h.pruner != nil {
		h.pruner.Wake() // and forget those gone
	}
	if h.trickplay != nil {
		h.trickplay.Wake() // and new videos' seek-bar pictures
	}
	if h.assCache != nil {
		h.assCache.Wake() // and their styled subtitle tracks
	}
}

// RegenerateThumbnails godoc
// @Summary Regenerate thumbnails for a directory
// @Description Force regeneration of all thumbnails in the specified directory (admin only)
// @Tags media
// @Produce json
// @Param path query string true "Folder (base64 client path)"
// @Success 200 {object} map[string]interface{}
// @Failure 403 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/regenerate-thumbnails [post]
func (h *MediaHandler) RegenerateThumbnails(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}

	// A folder as every path is given: its client path in base64, checked like any other.
	targetPath, ok := h.decodeMediaPath(w, r, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	if info, err := os.Stat(targetPath); err != nil || !info.IsDir() {
		respondError(w, http.StatusNotFound, "Folder not found")
		return
	}

	// Start regeneration in background
	go func() {
		count := 0
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			log.Printf("Failed to read directory for thumbnail regeneration: %v", err)
			return
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			entryPath := filepath.Join(targetPath, entry.Name())
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			mediaType := config.MediaType(entryPath)

			if mediaType == "image" || mediaType == "video" {
				// Delete existing thumbnail
				hash := media.GenerateThumbnailHash(entryPath)
				var thumbExt string
				if mediaType == "image" && strings.ToLower(ext) == ".png" {
					thumbExt = ".png"
				} else {
					thumbExt = ".jpg"
				}
				thumbPath, _ := media.GetThumbnailPath(config.GlobalConfig.Media.ThumbnailPath, hash, thumbExt)
				os.Remove(thumbPath)
				h.scanner.ForgetThumbnailFailure(entryPath)

				// Regenerate thumbnail
				var thumbnailURL string
				if mediaType == "image" {
					thumbnailURL = h.scanner.GenerateImageThumbnail(entryPath)
				} else {
					thumbnailURL = h.scanner.GenerateVideoThumbnail(entryPath)
				}

				// Notify SSE clients about the regenerated thumbnail
				if thumbnailURL != "" {
					h.notifyThumbnailReady(entryPath, thumbnailURL)
				}

				count++
			}
		}

		log.Printf("Regenerated %d thumbnails in %s", count, targetPath)
	}()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "regeneration started",
		"path":   clientPath(targetPath),
	})
}

// ServeThumbnail serves a thumbnail, generating it on the fly if missing. Thumbnails are named
// by the hash of their media file's path, which identifies the item and therefore its library,
// so access follows the same group rules as streaming the file.
func (h *MediaHandler) ServeThumbnail(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(mux.Vars(r)["filename"])
	ext := strings.ToLower(filepath.Ext(name))
	hash := strings.TrimSuffix(name, filepath.Ext(name))
	if (ext != ".jpg" && ext != ".png") || !isPathHash(hash) {
		respondError(w, http.StatusBadRequest, "Invalid thumbnail name")
		return
	}

	item, ok := h.itemForHash(w, r, hash)
	if !ok {
		return
	}

	thumbnailPath, _ := media.GetThumbnailPath(config.GlobalConfig.Media.ThumbnailPath, hash, ext)
	if _, err := os.Stat(thumbnailPath); err != nil {
		log.Printf("Generating thumbnail on-the-fly for: %s", item.Path)
		generatedPath, err := h.thumbnailGenerator.GenerateThumbnail(item.Path)
		if err != nil {
			log.Printf("Failed to generate thumbnail for %s: %v", item.Path, err)
			respondError(w, http.StatusNotFound, "Thumbnail not available")
			return
		}
		thumbnailPath = generatedPath
	}

	// URLs are per-file and change (?v= / ?t=) when a thumbnail is regenerated.
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, thumbnailPath)
}

// ServeTrickplayFrame serves one trickplay frame, with the same access rules as thumbnails.
func (h *MediaHandler) ServeTrickplayFrame(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	hash, frame := vars["hash"], vars["frame"]
	if !isPathHash(hash) || !trickplayFramePattern.MatchString(frame) {
		respondError(w, http.StatusBadRequest, "Invalid trickplay frame")
		return
	}
	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		respondError(w, http.StatusNotFound, "Trickplay not configured")
		return
	}

	if _, ok := h.itemForHash(w, r, hash); !ok {
		return
	}

	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, filepath.Join(trickplayDir, hash, frame))
}

// itemForHash looks up the media item a thumbnail/trickplay hash belongs to and checks the
// requester may access it, writing the error response if not.
func (h *MediaHandler) itemForHash(w http.ResponseWriter, r *http.Request, hash string) (*models.MediaItem, bool) {
	item, err := h.db.GetMediaItemByPathHash(hash)
	if err != nil {
		respondError(w, http.StatusNotFound, "Not found")
		return nil, false
	}
	if !h.canAccessPath(r, item.Path) {
		// Same response as a missing item, so hashes can't be probed.
		respondError(w, http.StatusNotFound, "Not found")
		return nil, false
	}
	return item, true
}

var trickplayFramePattern = regexp.MustCompile(`^frame-\d+\.jpg$`)

func isPathHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (h *MediaHandler) isValidMediaPath(path string) bool {
	_, ok := libraryForPath(path)
	return ok
}

// ThumbnailUpdates streams thumbnail-ready events over SSE. It sits behind AuthMiddleware, and
// each client only receives events for files its group can access.
func (h *MediaHandler) ThumbnailUpdates(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	id, events := h.thumbnailHub.register(userGroup(r))
	defer h.thumbnailHub.unregister(id)

	// Send initial connection message to trigger onopen event
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case ev := <-events:
			if ev.name != "" {
				fmt.Fprintf(w, "event: %s\n", ev.name)
			}
			fmt.Fprintf(w, "data: %s\n\n", ev.data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// makeThumbnail makes a queued thumbnail, saves it with the file and tells the pages showing it.
func (h *MediaHandler) makeThumbnail(job thumbnailJob) {
	item := job.item
	if item.Type == "image" {
		item.ThumbnailURL = h.scanner.GenerateImageThumbnail(item.Path)
	} else {
		item.ThumbnailURL = h.scanner.GenerateVideoThumbnail(item.Path)
	}
	if job.probe || item.Type == "video" && item.Duration <= 0 {
		// Its length and size, for a file new to the database, or a video whose length wasn't read
		// (it couldn't be read when it was scanned, and the scan doesn't look at it again; the
		// trickplay job fills it in too, later). This also keeps a length trickplay saved meanwhile
		// from being written over with the 0 this item was queued with.
		if probed, err := h.scanner.GetFileMetadata(item.Path); err == nil {
			item.Duration, item.Width, item.Height = probed.Duration, probed.Width, probed.Height
		}
	}
	h.db.SaveMediaItem(&item)
	h.notifyThumbnailReady(item.Path, item.ThumbnailURL)
}

// notifyThumbnailReady tells SSE clients that may access filePath about its new thumbnail
func (h *MediaHandler) notifyThumbnailReady(filePath, thumbnailURL string) {
	if thumbnailURL == "" {
		return
	}
	jsonData, err := json.Marshal(map[string]string{
		"path":         clientPath(filePath),
		"thumbnailURL": thumbnailURL,
	})
	if err != nil {
		return
	}
	h.thumbnailHub.broadcast(filePath, sseEvent{data: string(jsonData)})
}
