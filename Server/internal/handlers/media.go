package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
}

// SetMetadata enables TMDB metadata for movie and tv libraries.
func (h *MediaHandler) SetMetadata(meta *metadata.Service) {
	h.meta = meta
}

// NotifyMetadataReady tells SSE clients that may access path that its metadata changed. Sent as
// a named "metadata" event so clients that only handle thumbnail messages ignore it.
func (h *MediaHandler) NotifyMetadataReady(path string) {
	data, err := json.Marshal(map[string]string{"path": path})
	if err == nil {
		h.thumbnailHub.broadcast(path, sseEvent{name: "metadata", data: string(data)})
	}
}

func NewMediaHandler(db *database.DB, authService *auth.Service) *MediaHandler {
	return &MediaHandler{
		db:                 db,
		streamer:           media.NewStreamer(),
		scanner:            media.NewScanner(db),
		thumbnailGenerator: media.NewThumbnailGenerator(),
		thumbnailHub:       newThumbnailHub(),
		authService:        authService,
		streams:            media.NewStreamTracker(),
	}
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
	vars := mux.Vars(r)
	path := vars["path"]

	item, err := h.db.GetMediaItem(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}

	// Check access
	if !h.canAccessPath(r, item.Path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	respondJSON(w, http.StatusOK, item)
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

	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		log.Printf("[TRICKPLAY] Error: Trickplay path not configured in config.json")
		respondError(w, http.StatusInternalServerError, "Trickplay path not configured")
		return
	}

	hash := media.GenerateThumbnailHash(item.Path)
	trickplayVideoDir := filepath.Join(trickplayDir, hash)

	files, err := os.ReadDir(trickplayVideoDir)
	if err != nil {
		log.Printf("[TRICKPLAY] No frames found for %s (hash: %s) in directory: %s", item.Name, hash, trickplayVideoDir)
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

	log.Printf("[TRICKPLAY] Returning %d frames for %s", len(frames), item.Name)

	info := models.TrickplayInfo{
		Frames: frames,
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

	// Base64 decode the path
	pathBytes, err := base64.StdEncoding.DecodeString(encodedPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(pathBytes)

	// Log the request for debugging
	log.Printf("Stream request - Path: %s, User-Agent: %s", path, r.Header.Get("User-Agent"))

	// Check if user wants to force transcoding
	forceTranscode := r.URL.Query().Get("transcode") == "1"

	// Check if client wants to disable auto-transcoding (enabled by default)
	// WebUI can set auto-transcode=0 to let browser attempt native playback first
	autoTranscode := r.URL.Query().Get("auto-transcode") != "0"

	// Get start time for seeking in transcoded streams
	startTime := r.URL.Query().Get("start")

	// Get bitrate and resolution for custom transcoding
	bitrate := r.URL.Query().Get("bitrate")
	resolution := r.URL.Query().Get("resolution")

	// Validate path is within allowed media paths
	if !h.isValidMediaPath(path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Check access
	if !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	item, err := h.db.GetMediaItem(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}

	sess := h.beginStream(r, item)
	defer sess.End()

	if err := h.streamer.ServeContent(w, r, path, item.MimeType, forceTranscode, autoTranscode, startTime, config.GlobalConfig.Streaming.AutoConvertImages, bitrate, resolution, sess); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to stream media")
		return
	}
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

	// Base64 decode the path
	pathBytes, err := base64.StdEncoding.DecodeString(encodedPath)
	if err != nil {
		log.Printf("[METADATA] Invalid path encoding: %s", encodedPath)
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(pathBytes)

	log.Printf("[METADATA] Request for path: %s", path)

	// Validate path is within allowed media paths
	if !h.isValidMediaPath(path) {
		log.Printf("[METADATA] Access denied for path: %s", path)
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Check access
	if !h.canAccessPath(r, path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

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
		respondJSON(w, http.StatusOK, metadata)
		return
	}

	log.Printf("[METADATA] Returning cached metadata for: %s (Duration: %.2fs)", item.Name, item.Duration)
	respondJSON(w, http.StatusOK, item)
}

// BrowseDirectory godoc
// @Summary Browse directory
// @Description Browse media directory structure
// @Tags media
// @Produce json
// @Param path query string false "Directory path (relative to media root)"
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
			if stat, err := os.Stat(mediaPath.Path); err == nil && stat.IsDir() {
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
	if err != nil {
		respondError(w, http.StatusNotFound, "Directory not found")
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetPath, entry.Name())
		relPath := filepath.Join(relativePath, entry.Name())

		// Skip Mac-specific files if configured to ignore them
		if config.GlobalConfig.Media.IgnoreMacFiles && h.isMacFile(entry.Name()) {
			continue
		}

		if entry.IsDir() {
			dirs = append(dirs, models.Directory{
				Path: relPath,
				Name: entry.Name(),
			})
		} else {
			// Get file info
			fileInfo, err := entry.Info()
			if err != nil {
				log.Printf("Failed to get file info for %s: %v", entryPath, err)
				continue
			}

			// Determine media type based on extension
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			mediaType := h.getMediaTypeFromExtension(ext)

			// Only include supported media files
			if mediaType == "" {
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
					MimeType: h.getMimeTypeFromExtension(ext),
				}

				// Generate thumbnail in background for images and videos
				// Don't block the browse request
				if mediaType == "image" || mediaType == "video" {
					go func(itemCopy *models.MediaItem) {
						var thumbnailURL string
						if itemCopy.Type == "image" {
							thumbnailURL = h.scanner.GenerateImageThumbnail(itemCopy.Path)
						} else {
							thumbnailURL = h.scanner.GenerateVideoThumbnail(itemCopy.Path)
						}
						itemCopy.ThumbnailURL = thumbnailURL

						// Re-fetch metadata to get duration if missing
						h.scanner.GetFileMetadata(itemCopy.Path)

						h.db.SaveMediaItem(itemCopy)

						// Notify thumbnail ready via SSE
						h.notifyThumbnailReady(itemCopy.Path, thumbnailURL)
					}(item)
				} else {
					// Save non-media items immediately
					go h.db.SaveMediaItem(item)
				}
			} else {
				// Item found in database
				// If thumbnail URL is empty, regenerate it in background
				if item.ThumbnailURL == "" && (mediaType == "image" || mediaType == "video") {
					go func(itemCopy *models.MediaItem) {
						var thumbnailURL string
						if itemCopy.Type == "image" {
							thumbnailURL = h.scanner.GenerateImageThumbnail(itemCopy.Path)
						} else {
							thumbnailURL = h.scanner.GenerateVideoThumbnail(itemCopy.Path)
						}
						itemCopy.ThumbnailURL = thumbnailURL
						h.db.SaveMediaItem(itemCopy)

						// Notify thumbnail ready via SSE
						h.notifyThumbnailReady(itemCopy.Path, thumbnailURL)
					}(item)
				}
			}

			items = append(items, *item)
		}
	}

	if h.meta != nil {
		if absTarget, err := filepath.Abs(targetPath); err == nil {
			h.meta.Annotate(targetLib, absTarget, dirs, items)
		}
	}

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

	go h.scanner.ScanMediaPaths()
	if h.meta != nil {
		go h.meta.SyncAll(context.Background())
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "scan started"})
}

// RegenerateThumbnails godoc
// @Summary Regenerate thumbnails for a directory
// @Description Force regeneration of all thumbnails in the specified directory (admin only)
// @Tags media
// @Produce json
// @Param path query string false "Directory path (relative to media root)"
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

	relativePath := r.URL.Query().Get("path")

	// Determine target directory
	var targetPath string
	if relativePath == "" {
		// Regenerate all thumbnails
		respondError(w, http.StatusBadRequest, "Path parameter required")
		return
	}

	pathParts := strings.Split(strings.Trim(relativePath, "/"), "/")

	// Find which root this belongs to. Match only by the stable ID.
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		if mediaPath.ID != "" && mediaPath.ID == pathParts[0] {
			if len(pathParts) > 1 {
				targetPath = filepath.Join(mediaPath.Path, filepath.Join(pathParts[1:]...))
			} else {
				targetPath = mediaPath.Path
			}
			break
		}
	}

	if targetPath == "" {
		respondError(w, http.StatusNotFound, "Path not found")
		return
	}

	// Validate path is within allowed media paths
	if !h.isValidMediaPath(targetPath) {
		respondError(w, http.StatusForbidden, "Access denied")
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
			mediaType := h.getMediaTypeFromExtension(ext)

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

		log.Printf("Regenerated %d thumbnails for path: %s", count, relativePath)
	}()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "regeneration started",
		"path":   relativePath,
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

// getMediaTypeFromExtension determines the media type based on file extension
func (h *MediaHandler) getMediaTypeFromExtension(ext string) string {
	// Check against configured formats
	for _, videoExt := range config.GlobalConfig.Media.SupportedVideoFormats {
		if ext == videoExt {
			return "video"
		}
	}
	for _, imageExt := range config.GlobalConfig.Media.SupportedImageFormats {
		if ext == imageExt {
			return "image"
		}
	}
	for _, docExt := range config.GlobalConfig.Media.SupportedDocFormats {
		if ext == docExt {
			return "document"
		}
	}
	return ""
}

// getMimeTypeFromExtension returns the MIME type for a file extension
func (h *MediaHandler) getMimeTypeFromExtension(ext string) string {
	mimeTypes := map[string]string{
		// Video
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".divx": "video/x-msvideo", // DivX and Xvid files are AVI containers
		".xvid": "video/x-msvideo",
		".iso":  "application/x-iso9660-image", // DVD images, played transcoded
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".3gp":  "video/3gpp",
		".3gpp": "video/3gpp",
		".m4v":  "video/x-m4v",
		".mpg":  "video/mpeg",
		".mpeg": "video/mpeg",
		".ts":   "video/mp2t",
		".mp2":  "video/mpeg",
		// Image
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".webp": "image/webp",
		// Document
		".pdf": "application/pdf",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}
	return "application/octet-stream"
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

// notifyThumbnailReady tells SSE clients that may access filePath about its new thumbnail
func (h *MediaHandler) notifyThumbnailReady(filePath, thumbnailURL string) {
	if thumbnailURL == "" {
		return
	}
	jsonData, err := json.Marshal(map[string]string{
		"path":         filePath,
		"thumbnailURL": thumbnailURL,
	})
	if err != nil {
		return
	}
	h.thumbnailHub.broadcast(filePath, sseEvent{data: string(jsonData)})
}

// isMacFile checks if a filename is a Mac-specific file that should be ignored
func (h *MediaHandler) isMacFile(filename string) bool {
	return filename == ".DS_Store" || strings.HasPrefix(filename, "._")
}
