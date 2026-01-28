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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/weirdstreamer/server/internal/config"
)

// ThumbnailGenerator handles on-demand thumbnail generation with caching
type ThumbnailGenerator struct {
	mu sync.Mutex // Protects concurrent generation of the same thumbnail
}

func NewThumbnailGenerator() *ThumbnailGenerator {
	return &ThumbnailGenerator{}
}

// GenerateThumbnailHash creates a unique hash for a file path
func GenerateThumbnailHash(filePath string) string {
	hash := sha256.Sum256([]byte(filePath))
	return hex.EncodeToString(hash[:])
}

// GetThumbnailPath returns the full path and URL path for a thumbnail
// hash is stored in subdirectories: hash "ab123..." -> "/a/b/ab123..."
func GetThumbnailPath(baseDir, hash, ext string) (fullPath, urlPath string) {
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

// GenerateThumbnail generates a thumbnail for any media file on-demand
// Returns the thumbnail file path and any error encountered
func (tg *ThumbnailGenerator) GenerateThumbnail(filePath string) (string, error) {
	tg.mu.Lock()
	defer tg.mu.Unlock()

	log.Printf("[THUMBNAIL] GenerateThumbnail called for: %s", filePath)

	// Determine media type
	ext := strings.ToLower(filepath.Ext(filePath))
	var mediaType string

	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".flv", ".wmv", ".m4v", ".mpg", ".mpeg", ".3gp", ".ts", ".mp2":
		mediaType = "video"
	case ".jpg", ".jpeg", ".png", ".webp", ".bmp", ".gif", ".jfif", ".heif", ".heic":
		mediaType = "image"
	case ".pdf":
		mediaType = "document"
	default:
		return "", fmt.Errorf("unsupported media type: %s", ext)
	}

	log.Printf("[THUMBNAIL] Media type determined: %s (ext: %s)", mediaType, ext)

	thumbnailDir := config.GlobalConfig.Media.ThumbnailPath
	hash := GenerateThumbnailHash(filePath)

	var thumbnailExt string
	var thumbnailPath, thumbnailURL string

	// Generate based on media type
	switch mediaType {
	case "video":
		thumbnailExt = ".jpg"
		thumbnailPath, thumbnailURL = GetThumbnailPath(thumbnailDir, hash, thumbnailExt)
		
		// Check if already exists and is valid size (> 1KB)
		if info, err := os.Stat(thumbnailPath); err == nil {
			if info.Size() > 1024 {
				return thumbnailPath, nil
			}
			// File exists but is too small - delete and regenerate
			log.Printf("[THUMBNAIL] Removing corrupted thumbnail (size=%d): %s", info.Size(), thumbnailPath)
			os.Remove(thumbnailPath)
		}

		// Verify source file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return "", fmt.Errorf("source video file not found: %s", filePath)
		}

		// Create subdirectories
		if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create thumbnail directory: %w", err)
		}

		// Use ffmpeg to extract thumbnail
		log.Printf("[THUMBNAIL] Generating video thumbnail: source=%s, output=%s", filePath, thumbnailPath)
		cmd := exec.Command("ffmpeg",
			"-ss", "00:00:01.000",  // Seek to 1 second (before input for faster seeking)
			"-i", filePath,
			"-frames:v", "1",       // Extract one frame
			"-vf", "scale=320:-1",  // Scale to 320px width, maintain aspect ratio
			"-q:v", "2",            // High quality JPEG
			"-y",                   // Overwrite output file
			thumbnailPath,
		)

		// Capture output for debugging
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Try again without seeking (for videos shorter than 1 second)
			log.Printf("[THUMBNAIL] FFmpeg failed with seek, trying without seek for: %s", filePath)
			cmd = exec.Command("ffmpeg",
				"-i", filePath,
				"-frames:v", "1",       // Extract first frame
				"-vf", "scale=320:-1",  // Scale to 320px width
				"-q:v", "2",            // High quality JPEG
				"-y",
				thumbnailPath,
			)
			output, err = cmd.CombinedOutput()
			if err != nil {
				log.Printf("[THUMBNAIL] FFmpeg error for %s: %v\nOutput: %s", filePath, err, string(output))
				return "", fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(output))
			}
		}

		// Verify the file was actually created and has content
		if info, err := os.Stat(thumbnailPath); err != nil {
			log.Printf("[THUMBNAIL] FFmpeg completed but thumbnail not found: %s", thumbnailPath)
			return "", fmt.Errorf("thumbnail not created")
		} else if info.Size() < 1024 {
			log.Printf("[THUMBNAIL] FFmpeg created thumbnail but size too small: %d bytes for %s", info.Size(), thumbnailPath)
			return "", fmt.Errorf("thumbnail too small: %d bytes", info.Size())
		}

		log.Printf("[THUMBNAIL] Successfully generated video thumbnail: %s (%d bytes)", thumbnailPath, info.Size())

	case "image":
		// Preserve original format for PNG, use JPEG for others
		if strings.ToLower(ext) == ".png" {
			thumbnailExt = ".png"
		} else {
			thumbnailExt = ".jpg"
		}
		
		thumbnailPath, thumbnailURL = GetThumbnailPath(thumbnailDir, hash, thumbnailExt)

		// Check if already exists and is valid size (> 1KB)
		if info, err := os.Stat(thumbnailPath); err == nil {
			if info.Size() > 1024 {
				return thumbnailPath, nil
			}
			// File exists but is too small - delete and regenerate
			log.Printf("[THUMBNAIL] Removing corrupted thumbnail (size=%d): %s", info.Size(), thumbnailPath)
			os.Remove(thumbnailPath)
		}

		file, err := os.Open(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to open image: %w", err)
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
			return "", fmt.Errorf("failed to decode image: %w", err)
		}

		// Apply orientation correction
		img = applyOrientation(img, orientation)

		// Generate thumbnail
		thumbnail := resizeImage(img, 320, 320)

		// Create subdirectories
		if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create thumbnail directory: %w", err)
		}

		out, err := os.Create(thumbnailPath)
		if err != nil {
			return "", fmt.Errorf("failed to create thumbnail file: %w", err)
		}
		defer out.Close()

		if strings.ToLower(ext) == ".png" {
			if err := png.Encode(out, thumbnail); err != nil {
				return "", fmt.Errorf("failed to encode PNG thumbnail: %w", err)
			}
		} else {
			if err := jpeg.Encode(out, thumbnail, &jpeg.Options{Quality: 85}); err != nil {
				return "", fmt.Errorf("failed to encode JPEG thumbnail: %w", err)
			}
		}

		out.Close() // Ensure file is flushed

		// Verify the thumbnail was created with valid size
		if info, err := os.Stat(thumbnailPath); err != nil {
			log.Printf("[THUMBNAIL] Image thumbnail not found after encoding: %s", thumbnailPath)
			return "", fmt.Errorf("thumbnail not created")
		} else if info.Size() < 1024 {
			log.Printf("[THUMBNAIL] Image thumbnail too small: %d bytes for %s", info.Size(), thumbnailPath)
			return "", fmt.Errorf("thumbnail too small: %d bytes", info.Size())
		} else {
			log.Printf("[THUMBNAIL] Successfully generated image thumbnail: %s (%d bytes)", thumbnailPath, info.Size())
		}

	case "document":
		// For PDFs, return placeholder for now
		// Could integrate pdfcpu or similar in the future
		return "", fmt.Errorf("PDF thumbnails not yet implemented")

	default:
		return "", fmt.Errorf("unknown media type: %s", mediaType)
	}

	_ = thumbnailURL // URL not needed for this function
	return thumbnailPath, nil
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
