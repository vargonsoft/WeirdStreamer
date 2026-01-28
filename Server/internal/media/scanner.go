package media

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

type Scanner struct {
	db                 *database.DB
	thumbnailGenerator *ThumbnailGenerator
}

func NewScanner(db *database.DB) *Scanner {
	return &Scanner{
		db:                 db,
		thumbnailGenerator: NewThumbnailGenerator(),
	}
}

func (s *Scanner) ScanMediaPaths() error {
	// First pass: count total files
	totalFiles := 0
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		filepath.Walk(mediaPath.Path, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && s.getMediaType(p) != "" {
				totalFiles++
			}
			return nil
		})
	}

	fmt.Printf("Found %d media files to scan\n", totalFiles)

	// Second pass: scan with progress
	processedFiles := 0
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		if err := s.scanDirectory(mediaPath.Path, &processedFiles, totalFiles); err != nil {
			return fmt.Errorf("failed to scan %s: %w", mediaPath.Path, err)
		}
	}

	// Clear progress line and print completion
	fmt.Printf("\r%s\n", strings.Repeat(" ", 100))
	fmt.Printf("Scan complete: %d files processed\n", processedFiles)

	// Clean up orphaned thumbnails after scanning
	if err := s.cleanupOrphanedThumbnails(); err != nil {
		fmt.Printf("Warning: failed to cleanup thumbnails: %v\n", err)
	}

	return nil
}

func (s *Scanner) scanDirectory(root string, processed *int, total int) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		mediaType := s.getMediaType(path)
		if mediaType == "" {
			return nil // Skip unsupported files
		}

		// Update progress
		*processed++
		percentage := float64(*processed) / float64(total) * 100
		fileName := filepath.Base(path)
		if len(fileName) > 50 {
			fileName = fileName[:47] + "..."
		}
		fmt.Printf("\r[%3.0f%%] Scanning: %-50s ", percentage, fileName)

		item := &models.MediaItem{
			Path:     path,
			Name:     info.Name(),
			Type:     mediaType,
			Size:     info.Size(),
			MimeType: s.getMimeType(path),
		}

		// Get metadata based on type
		switch mediaType {
		case "video":
			if err := s.getVideoMetadata(item); err == nil {
				item.ThumbnailURL = s.GenerateVideoThumbnail(path)
			}
		case "image":
			if err := s.getImageMetadata(item); err == nil {
				item.ThumbnailURL = s.GenerateImageThumbnail(path)
			}
		case "document":
			item.ThumbnailURL = s.generateDocThumbnail(path)
		}

		return s.db.SaveMediaItem(item)
	})
}

// GetFileMetadata gets metadata for a file in real-time without database
func (s *Scanner) GetFileMetadata(path string) (*models.MediaItem, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	item := &models.MediaItem{
		Path:     path,
		Name:     filepath.Base(path),
		Type:     s.getMediaType(path),
		Size:     info.Size(),
		MimeType: s.getMimeType(path),
	}

	// Get metadata based on type
	if item.Type == "video" {
		// Get duration using ffprobe
		cmd := exec.Command("ffprobe",
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "default=noprint_wrappers=1:nokey=1",
			path,
		)

		output, err := cmd.Output()
		if err == nil {
			var duration float64
			fmt.Sscanf(string(output), "%f", &duration)
			item.Duration = duration
		}
	} else if item.Type == "image" {
		s.getImageMetadata(item)
	}

	return item, nil
}

func (s *Scanner) getMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))

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

func (s *Scanner) getMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	mimeTypes := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".asf":  "video/x-ms-asf",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".3gp":  "video/3gpp",
		".3gpp": "video/3gpp",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".webp": "image/webp",
		".pdf":  "application/pdf",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

func (s *Scanner) getVideoMetadata(item *models.MediaItem) error {
	// Use ffprobe to get video metadata
	cmd := exec.Command("ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		item.Path,
	)

	output, err := cmd.Output()
	if err != nil {
		return err
	}

	// Parse basic info - in production you'd parse the JSON properly
	// For now, we'll just mark it as having metadata
	_ = output
	return nil
}

func (s *Scanner) getImageMetadata(item *models.MediaItem) error {
	file, err := os.Open(item.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return err
	}

	item.Width = config.Width
	item.Height = config.Height
	return nil
}

func (s *Scanner) GenerateVideoThumbnail(videoPath string) string {
	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath
	hash := GenerateThumbnailHash(videoPath)
	_, thumbnailURL := GetThumbnailPath(thumbnailDir, hash, ".jpg")

	// Use the shared thumbnail generator
	if _, err := s.thumbnailGenerator.GenerateThumbnail(videoPath); err != nil {
		log.Printf("[SCANNER] WARNING: Failed to generate video thumbnail for %s: %v", videoPath, err)
		return ""
	}

	return thumbnailURL
}

func (s *Scanner) GenerateImageThumbnail(imagePath string) string {
	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath
	hash := GenerateThumbnailHash(imagePath)
	ext := filepath.Ext(imagePath)

	var thumbnailExt string
	// Preserve original format for PNG, use JPEG for others
	if strings.ToLower(ext) == ".png" {
		thumbnailExt = ".png"
	} else {
		thumbnailExt = ".jpg"
	}

	_, thumbnailURL := GetThumbnailPath(thumbnailDir, hash, thumbnailExt)

	// Use the shared thumbnail generator
	if _, err := s.thumbnailGenerator.GenerateThumbnail(imagePath); err != nil {
		log.Printf("[SCANNER] WARNING: Failed to generate image thumbnail for %s: %v", imagePath, err)
		return ""
	}

	return thumbnailURL
}

func (s *Scanner) generateDocThumbnail(docPath string) string {
	// For PDFs, we could use a library like pdfcpu or poppler
	// For now, return a placeholder
	return "/static/pdf-icon.png"
}

// cleanupOrphanedThumbnails removes thumbnails for files that no longer exist
func (s *Scanner) cleanupOrphanedThumbnails() error {
	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath

	// Get all media items from database
	allMedia, err := s.db.ListMediaItems("")
	if err != nil {
		return fmt.Errorf("failed to list media items: %w", err)
	}

	// Build set of valid thumbnail paths (relative to thumbnailDir)
	validThumbnails := make(map[string]bool)
	for _, item := range allMedia {
		if item.ThumbnailURL != "" {
			// Extract path from URL (e.g., "/thumbnails/a/b/abc123.jpg" -> "a/b/abc123.jpg")
			relativePath := strings.TrimPrefix(item.ThumbnailURL, "/thumbnails/")
			validThumbnails[relativePath] = true
		}
	}

	// Recursively walk thumbnail directory
	deletedCount := 0
	err = filepath.Walk(thumbnailDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory
		if path == thumbnailDir {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(thumbnailDir, path)
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Delete if not in valid set
		if !validThumbnails[relPath] {
			if err := os.Remove(path); err != nil {
				fmt.Printf("Warning: failed to delete orphaned thumbnail %s: %v\n", relPath, err)
			} else {
				deletedCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk thumbnail directory: %w", err)
	}

	// Clean up empty directories
	filepath.Walk(thumbnailDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || path == thumbnailDir {
			return err
		}

		if info.IsDir() {
			// Try to remove directory (will only succeed if empty)
			os.Remove(path)
		}
		return nil
	})

	if deletedCount > 0 {
		fmt.Printf("Cleaned up %d orphaned thumbnails\n", deletedCount)
	}

	return nil
}

