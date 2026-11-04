package media

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	_ "golang.org/x/image/webp"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

// ThumbnailGenerator handles on-demand thumbnail generation with caching
type ThumbnailGenerator struct {
	mu sync.Mutex // Protects concurrent generation of the same thumbnail
	// Files whose thumbnail couldn't be made, by their size and time when it failed: browsing
	// their folder again doesn't retry (and log) them until the file changes, the server
	// restarts, or the folder's thumbnails are refreshed.
	failed map[string]fileStamp
}

type fileStamp struct {
	size    int64
	modTime int64
}

// ErrThumbnailFailedBefore is a file whose thumbnail already failed, unchanged since.
var ErrThumbnailFailedBefore = errors.New("its thumbnail failed before")

func NewThumbnailGenerator() *ThumbnailGenerator {
	return &ThumbnailGenerator{failed: map[string]fileStamp{}}
}

// Forget lets filePath's thumbnail be tried again after a failure.
func (tg *ThumbnailGenerator) Forget(filePath string) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	delete(tg.failed, filePath)
}

// GenerateThumbnailHash creates a unique hash for a file path
func GenerateThumbnailHash(filePath string) string {
	return database.PathHash(filePath)
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
	var stamp fileStamp
	if st, err := os.Stat(filePath); err == nil {
		stamp = fileStamp{st.Size(), st.ModTime().UnixNano()}
		if tg.failed[filePath] == stamp {
			return "", ErrThumbnailFailedBefore
		}
	}
	path, err := tg.generate(filePath)
	if err != nil && stamp != (fileStamp{}) {
		tg.failed[filePath] = stamp
	}
	return path, err
}

func (tg *ThumbnailGenerator) generate(filePath string) (string, error) {

	ext := strings.ToLower(filepath.Ext(filePath))
	mediaType := config.MediaType(filePath)
	if mediaType == "" {
		return "", fmt.Errorf("unsupported media type: %s", ext)
	}

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
			os.Remove(thumbnailPath)
		}

		// Verify source file exists and log details
		if stat, err := os.Stat(filePath); os.IsNotExist(err) {
			return "", fmt.Errorf("source video file not found: %s", filePath)
		} else if err != nil {
			return "", fmt.Errorf("cannot access source file: %w", err)
		} else if stat.Size() == 0 {
			return "", fmt.Errorf("source video file is empty: %s", filePath)
		}

		// Create subdirectories
		if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create thumbnail directory: %w", err)
		}

		log.Printf("[THUMBNAIL] Generating video thumbnail for %s -> %s", filePath, thumbnailPath)
		part := partPath(thumbnailPath)

		// Try different seek positions: 1 minute, 30 seconds, 1 second
		seekTimes := []string{"00:01:00.000", "00:00:30.000", "00:00:01.000"}
		var success bool
		codec := ""
		if IsDiscImage(filePath) {
			// Disc images: a frame a tenth of the way into the main title
			seekTimes = nil
			if args, err := DiscFrameArgs(filePath, 0.1); err == nil {
				args = append(args, "-vf", "scale=320:-1,format=yuvj420p", "-frames:v", "1", "-y", part)
				if err := exec.Command("ffmpeg", args...).Run(); err == nil {
					if info, statErr := os.Stat(part); statErr == nil && info.Size() > 1024 {
						success = true
					}
				}
			}
		} else {
			// Get codec info for conditional BSF
			codec = tg.getVideoCodec(filePath)
		}

		for _, seekTime := range seekTimes {
			// format=yuvj420p is the critical fix for MJPEG compliance in FFmpeg 7.1.1.
			var args []string
			if codec == "h264" || codec == "hevc" {
				args = append(args, "-bsf:v", codec+"_metadata=matrix_coefficients=1")
			}
			args = append(args, "-ss", seekTime, "-noaccurate_seek", "-i", filePath, "-vf", "scale=320:-1,format=yuvj420p", "-frames:v", "1", "-y", part)

			cmd := exec.Command("ffmpeg", args...)
			if err := cmd.Run(); err == nil {
				if info, statErr := os.Stat(part); statErr == nil && info.Size() > 1024 {
					success = true
					break
				} else if statErr == nil {
					os.Remove(part)
				}
			}

			// Fallback without BSF if needed
			cmdFallback := exec.Command("ffmpeg", "-ss", seekTime, "-i", filePath, "-vf", "scale=320:-1,format=yuvj420p", "-frames:v", "1", "-y", part)
			if errFallback := cmdFallback.Run(); errFallback == nil {
				if info, statErr := os.Stat(part); statErr == nil && info.Size() > 1024 {
					success = true
					break
				} else if statErr == nil {
					os.Remove(part)
				}
			}
		}

		// If all attempts failed, return error
		if !success {
			os.Remove(part)
			return "", fmt.Errorf("thumbnail generation failed for %s", filePath)
		}
		if err := finishPart(thumbnailPath); err != nil {
			return "", err
		}

		log.Printf("[THUMBNAIL] Successfully generated video thumbnail: %s", thumbnailPath)

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
			os.Remove(thumbnailPath)
		}

		log.Printf("[THUMBNAIL] Generating image thumbnail for %s -> %s", filePath, thumbnailPath)

		// Handle HEIC/HEIF files (Go's image decoder doesn't support them)
		// Convert to JPEG first using heif-convert (properly handles tiled/grid HEIC)
		var imageFile string
		if strings.ToLower(ext) == ".heic" || strings.ToLower(ext) == ".heif" {
			// Create a temporary JPEG from HEIC using libheif
			tempFile, err := tempFileName("heic-thumb-*.jpg")
			var output []byte
			if err == nil {
				output, err = exec.Command("heif-convert", filePath, tempFile).CombinedOutput()
			}
			if err != nil {
				os.Remove(tempFile)
				log.Printf("[THUMBNAIL] heif-convert failed for %s (possibly a mislabeled JPEG): %v\nOutput: %s", filePath, err, string(output))
				// Fallback: try using the original file directly, maybe it's actually a JPEG
				imageFile = filePath
			} else {
				imageFile = tempFile
				defer os.Remove(tempFile) // Clean up temp file
			}
		} else {
			imageFile = filePath
		}

		file, err := os.Open(imageFile)
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

		part := partPath(thumbnailPath)
		out, err := os.Create(part)
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
		if info, err := os.Stat(part); err != nil {
			return "", fmt.Errorf("thumbnail not created")
		} else if info.Size() < 1024 {
			os.Remove(part)
			return "", fmt.Errorf("thumbnail too small: %d bytes", info.Size())
		} else if err := finishPart(thumbnailPath); err != nil {
			return "", err
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

func (tg *ThumbnailGenerator) getVideoCodec(videoPath string) string {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
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
