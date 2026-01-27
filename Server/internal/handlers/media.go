package handlers

import (
	"encoding/base64"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/media"
	"github.com/weirdstreamer/server/internal/models"
)

type MediaHandler struct {
	db                 *database.DB
	streamer           *media.Streamer
	scanner            *media.Scanner
	thumbnailGenerator *media.ThumbnailGenerator
}

func NewMediaHandler(db *database.DB) *MediaHandler {
	return &MediaHandler{
		db:                 db,
		streamer:           media.NewStreamer(),
		scanner:            media.NewScanner(db),
		thumbnailGenerator: media.NewThumbnailGenerator(),
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

	// Check if client wants to disable auto-transcoding (e.g., native apps)
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

	if err := h.streamer.ServeContent(w, r, path, item.MimeType, forceTranscode, autoTranscode, startTime); err != nil {
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
				dirs = append(dirs, models.Directory{
					Path: filepath.Base(mediaPath.Path),
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

	// Find which root this belongs to
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		if filepath.Base(mediaPath.Path) == pathParts[0] {
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
			// Try to get from database
			if item, err := h.db.GetMediaItem(entryPath); err == nil {
				items = append(items, *item)
			}
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
