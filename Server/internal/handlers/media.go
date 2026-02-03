package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
)

// SSE client for thumbnail notifications
type thumbnailClient struct {
	channel chan string
}

type MediaHandler struct {
	db                 *database.DB
	streamer           *media.Streamer
	scanner            *media.Scanner
	thumbnailGenerator *media.ThumbnailGenerator
	thumbnailClients   map[string]*thumbnailClient
	clientsMux         sync.RWMutex
	authService        *auth.Service
}

func NewMediaHandler(db *database.DB, authService *auth.Service) *MediaHandler {
	return &MediaHandler{
		db:                 db,
		streamer:           media.NewStreamer(),
		scanner:            media.NewScanner(db),
		thumbnailGenerator: media.NewThumbnailGenerator(),
		thumbnailClients:   make(map[string]*thumbnailClient),
		authService:        authService,
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
	mediaType := r.URL.Query().Get("type")

	items, err := h.db.ListMediaItems(mediaType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to list media")
		return
	}

	respondJSON(w, http.StatusOK, items)
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

	respondJSON(w, http.StatusOK, item)
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

	// Validate path is within allowed media paths
	if !h.isValidMediaPath(path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	item, err := h.db.GetMediaItem(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "Media not found")
		return
	}

	if err := h.streamer.ServeContent(w, r, path, item.MimeType, forceTranscode, autoTranscode, startTime, config.GlobalConfig.Streaming.AutoConvertImages); err != nil {
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
		respondError(w, http.StatusBadRequest, "Invalid path encoding")
		return
	}
	path := string(pathBytes)

	// Validate path is within allowed media paths
	if !h.isValidMediaPath(path) {
		respondError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Try to get from database first
	item, err := h.db.GetMediaItem(path)
	if err != nil || item.Duration == 0 {
		// Not in database or no duration - get metadata in real-time
		log.Printf("Getting metadata in real-time for: %s", path)
		metadata, err := h.scanner.GetFileMetadata(path)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Failed to get metadata")
			return
		}
		respondJSON(w, http.StatusOK, metadata)
		return
	}

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
					Path: dirPath,
					Name: displayName,
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

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		respondError(w, http.StatusNotFound, "Directory not found")
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(targetPath, entry.Name())
		relPath := filepath.Join(relativePath, entry.Name())

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

	result := models.Directory{
		Path:  relativePath,
		Name:  filepath.Base(relativePath),
		Dirs:  dirs,
		Items: items,
	}

	respondJSON(w, http.StatusOK, result)
}

// ScanMedia godoc
// @Summary Scan media directories
// @Description Trigger a scan of configured media directories
// @Tags media
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 500 {object} models.ErrorResponse
// @Security BearerAuth
// @Router /api/media/scan [post]
func (h *MediaHandler) ScanMedia(w http.ResponseWriter, r *http.Request) {
	go h.scanner.ScanMediaPaths()
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
	if claims == nil {
		respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if !claims.IsAdmin {
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

// ServeThumbnail serves thumbnail files with on-the-fly generation
func (h *MediaHandler) ServeThumbnail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]

	// Security: prevent path traversal
	filename = filepath.Clean("/" + filename)[1:]
	if strings.Contains(filename, "..") {
		respondError(w, http.StatusBadRequest, "Invalid filename")
		return
	}

	thumbnailPath := filepath.Join(config.GlobalConfig.Media.ThumbnailPath, filename)

	// Check if thumbnail exists
	if _, err := os.Stat(thumbnailPath); err == nil {
		// Thumbnail exists, serve it
		http.ServeFile(w, r, thumbnailPath)
		return
	}

	// Thumbnail doesn't exist - try to find the source file and generate it
	// Extract hash from filename (e.g., "abc123.jpg" -> "abc123")
	hash := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	// Look up the media item by thumbnail hash
	// We need to find which file this thumbnail belongs to
	mediaItems, err := h.db.ListMediaItems("")
	if err != nil {
		log.Printf("Failed to list media items for thumbnail generation: %v", err)
		respondError(w, http.StatusNotFound, "Thumbnail not found")
		return
	}

	// Find the media item that matches this thumbnail hash
	var sourceFile string
	for _, item := range mediaItems {
		itemHash := media.GenerateThumbnailHash(item.Path)
		if itemHash == hash {
			sourceFile = item.Path
			break
		}
	}

	if sourceFile == "" {
		// No matching source file found
		respondError(w, http.StatusNotFound, "Thumbnail source not found")
		return
	}

	// Verify the source file still exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "Source file not found")
		return
	}

	// Generate thumbnail on-the-fly
	log.Printf("Generating thumbnail on-the-fly for: %s", sourceFile)
	generatedPath, err := h.thumbnailGenerator.GenerateThumbnail(sourceFile)
	if err != nil {
		log.Printf("Failed to generate thumbnail for %s: %v", sourceFile, err)
		respondError(w, http.StatusInternalServerError, "Failed to generate thumbnail")
		return
	}

	// Serve the newly generated thumbnail
	http.ServeFile(w, r, generatedPath)
}

func (h *MediaHandler) isValidMediaPath(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		absRoot, err := filepath.Abs(mediaPath.Path)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, absRoot) {
			return true
		}
	}
	return false
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

// ThumbnailUpdates handles Server-Sent Events for thumbnail generation progress
func (h *MediaHandler) ThumbnailUpdates(w http.ResponseWriter, r *http.Request) {
	// EventSource doesn't support custom headers, so we accept token via query param
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		http.Error(w, "Unauthorized: token required", http.StatusUnauthorized)
		return
	}

	// Validate the token using authService
	_, err := h.authService.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create unique client ID
	clientID := fmt.Sprintf("client_%d", len(h.thumbnailClients))

	// Create channel for this client
	client := &thumbnailClient{
		channel: make(chan string, 10),
	}

	// Register client
	h.clientsMux.Lock()
	h.thumbnailClients[clientID] = client
	h.clientsMux.Unlock()

	// Remove client on disconnect
	defer func() {
		h.clientsMux.Lock()
		delete(h.thumbnailClients, clientID)
		close(client.channel)
		h.clientsMux.Unlock()
	}()

	// Send keep-alive comments periodically
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send initial connection message to trigger onopen event
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Create ticker for keep-alive comments (every 30 seconds)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Listen for messages
	for {
		select {
		case msg := <-client.channel:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			// Send keep-alive comment to prevent connection timeout
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// notifyThumbnailReady sends notification to all SSE clients about a new thumbnail
func (h *MediaHandler) notifyThumbnailReady(filePath, thumbnailURL string) {
	if thumbnailURL == "" {
		return
	}

	notification := map[string]string{
		"path":         filePath,
		"thumbnailURL": thumbnailURL,
	}

	jsonData, err := json.Marshal(notification)
	if err != nil {
		return
	}

	h.clientsMux.RLock()
	defer h.clientsMux.RUnlock()

	for _, client := range h.thumbnailClients {
		select {
		case client.channel <- string(jsonData):
		default:
			// Channel full, skip
		}
	}
}
