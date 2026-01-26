package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	_ "golang.org/x/image/webp"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

type Scanner struct {
	db *database.DB
}

func NewScanner(db *database.DB) *Scanner {
	return &Scanner{db: db}
}

// generateThumbnailHash creates a unique hash for a file path
func generateThumbnailHash(filePath string) string {
	hash := sha256.Sum256([]byte(filePath))
	return hex.EncodeToString(hash[:])
}

// getThumbnailPath returns the full path and URL path for a thumbnail
// hash is stored in subdirectories: hash "ab123..." -> "/a/b/ab123..."
func getThumbnailPath(baseDir, hash, ext string) (fullPath, urlPath string) {
	if len(hash) < 2 {
		// Fallback for short hashes (shouldn't happen with SHA256)
		fullPath = filepath.Join(baseDir, hash+ext)
		urlPath = "/thumbnails/" + hash + ext
		return
	}
	
	subDir1 := string(hash[0])
	subDir2 := string(hash[1])
	fileName := hash + ext
	
	// Create full filesystem path
	fullPath = filepath.Join(baseDir, subDir1, subDir2, fileName)
	// Create URL path
	urlPath = "/thumbnails/" + subDir1 + "/" + subDir2 + "/" + fileName
	return
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
				item.ThumbnailURL = s.generateVideoThumbnail(path)
			}
		case "image":
			if err := s.getImageMetadata(item); err == nil {
				item.ThumbnailURL = s.generateImageThumbnail(path)
			}
		case "document":
			item.ThumbnailURL = s.generateDocThumbnail(path)
		}

		return s.db.SaveMediaItem(item)
	})
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

func (s *Scanner) generateVideoThumbnail(videoPath string) string {
	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath

	hash := generateThumbnailHash(videoPath)
	thumbnailPath, thumbnailURL := getThumbnailPath(thumbnailDir, hash, ".jpg")

	// Skip if thumbnail already exists
	if _, err := os.Stat(thumbnailPath); err == nil {
		return thumbnailURL
	}

	// Create subdirectories
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
		return ""
	}

	// Use ffmpeg to extract thumbnail
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-ss", "00:00:01.000",
		"-vframes", "1",
		"-vf", "scale=320:-1",
		"-y",
		thumbnailPath,
	)

	if err := cmd.Run(); err != nil {
		return ""
	}

	return thumbnailURL
}

func (s *Scanner) generateImageThumbnail(imagePath string) string {
	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath

	hash := generateThumbnailHash(imagePath)
	ext := filepath.Ext(imagePath)
	var thumbnailExt string
	
	// Preserve original format for PNG, use JPEG for others
	if strings.ToLower(ext) == ".png" {
		thumbnailExt = ".png"
	} else {
		thumbnailExt = ".jpg"
	}
	
	thumbnailPath, thumbnailURL := getThumbnailPath(thumbnailDir, hash, thumbnailExt)

	// Skip if thumbnail already exists
	if _, err := os.Stat(thumbnailPath); err == nil {
		return thumbnailURL
	}

	file, err := os.Open(imagePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	// Try to read EXIF data for orientation
	orientation := 1
	file.Seek(0, 0)
	if exifData, err := exif.Decode(file); err == nil {
		if orientTag, err := exifData.Get(exif.Orientation); err == nil {
			if orientVal, err := orientTag.Int(0); err == nil {
				orientation = orientVal
			}
		}
	}

	// Reset file pointer for image decoding
	file.Seek(0, 0)
	img, _, err := image.Decode(file)
	if err != nil {
		return ""
	}

	// Apply orientation correction
	img = applyOrientation(img, orientation)

	// Generate thumbnail
	thumbnail := resizeImage(img, 320, 320)

	// Create subdirectories
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
		return ""
	}

	out, err := os.Create(thumbnailPath)
	if err != nil {
		return ""
	}
	defer out.Close()

	if strings.ToLower(ext) == ".png" {
		png.Encode(out, thumbnail)
	} else {
		jpeg.Encode(out, thumbnail, &jpeg.Options{Quality: 85})
	}

	return thumbnailURL
}

// applyOrientation rotates/flips an image according to EXIF orientation tag
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 1:
		// Normal - no transformation needed
		return img
	case 2:
		// Flip horizontal
		return flipHorizontal(img)
	case 3:
		// Rotate 180
		return rotate180(img)
	case 4:
		// Flip vertical
		return flipVertical(img)
	case 5:
		// Rotate 90 CW and flip horizontal
		return flipHorizontal(rotate90(img))
	case 6:
		// Rotate 90 CW
		return rotate90(img)
	case 7:
		// Rotate 270 CW and flip horizontal
		return flipHorizontal(rotate270(img))
	case 8:
		// Rotate 270 CW
		return rotate270(img)
	default:
		return img
	}
}

func rotate90(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rotated := image.NewRGBA(image.Rect(0, 0, h, w))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rotated.Set(h-y-1, x, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return rotated
}

func rotate180(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rotated := image.NewRGBA(image.Rect(0, 0, w, h))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rotated.Set(w-x-1, h-y-1, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return rotated
}

func rotate270(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rotated := image.NewRGBA(image.Rect(0, 0, h, w))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rotated.Set(y, w-x-1, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return rotated
}

func flipHorizontal(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	flipped := image.NewRGBA(image.Rect(0, 0, w, h))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			flipped.Set(w-x-1, y, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return flipped
}

func flipVertical(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	flipped := image.NewRGBA(image.Rect(0, 0, w, h))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			flipped.Set(x, h-y-1, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return flipped
}

func (s *Scanner) generateDocThumbnail(docPath string) string {
	// For PDFs, we could use a library like pdfcpu or poppler
	// For now, return a placeholder
	return "/static/pdf-icon.png"
}

// resizeImage creates a thumbnail maintaining aspect ratio
func resizeImage(src image.Image, maxWidth, maxHeight int) image.Image {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Calculate new dimensions maintaining aspect ratio
	ratio := float64(srcW) / float64(srcH)
	var dstW, dstH int

	if srcW > srcH {
		dstW = maxWidth
		dstH = int(float64(maxWidth) / ratio)
	} else {
		dstH = maxHeight
		dstW = int(float64(maxHeight) * ratio)
	}

	// Ensure dimensions are at least 1
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	// Simple bilinear scaling
	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
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
