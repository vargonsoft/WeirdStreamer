package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	_ "golang.org/x/image/webp"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/jobs"
	"github.com/weirdstreamer/server/internal/models"
)

type Scanner struct {
	db                 *database.DB
	thumbnailGenerator *ThumbnailGenerator
	job                *jobs.Job
	probed             int // files probed (new or changed) in the scan running
}

func NewScanner(db *database.DB) *Scanner {
	return &Scanner{
		db:                 db,
		thumbnailGenerator: NewThumbnailGenerator(),
		job:                jobs.Register("scan", "Media scan", "Adds new and changed files in every library, with their length, size and thumbnail."),
	}
}

// scanning: a scan is running (one at a time, whichever Scanner started it).
var scanning atomic.Bool

// ErrScanRunning: a scan was asked for while one is running.
var ErrScanRunning = errors.New("a scan is already running")

func (s *Scanner) ScanMediaPaths() error {
	if !scanning.CompareAndSwap(false, true) {
		return ErrScanRunning
	}
	defer scanning.Store(false)
	s.job.Begin(0)
	s.probed = 0
	// First pass: count total files
	totalFiles := 0
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		filepath.Walk(mediaPath.Path, func(p string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() && DVDFolder(p) {
				totalFiles++ // an unpacked DVD: one video
				return filepath.SkipDir
			}
			if err == nil && !info.IsDir() && config.MediaType(p) != "" {
				totalFiles++
			}
			return nil
		})
	}

	fmt.Printf("Found %d media files to scan\n", totalFiles)
	s.job.SetTotal(totalFiles)

	// Second pass: scan with progress
	processedFiles := 0
	for _, mediaPath := range config.GlobalConfig.Media.Paths {
		if err := s.scanDirectory(mediaPath.Path, &processedFiles, totalFiles); err != nil {
			s.job.End("Failed: couldn't read the " + mediaPath.Name + " library")
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
	s.job.End(fmt.Sprintf("%s checked, %d new or changed", plural(processedFiles, "file"), s.probed))

	return nil
}

func (s *Scanner) scanDirectory(root string, processed *int, total int) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			if os.IsNotExist(err) {
				log.Printf("[SCAN] Skipping missing file or directory: %s", path)
				return nil
			}
			log.Printf("[SCAN] Warning: unable to access %s: %v", path, err)
			return nil
		}

		// Skip Mac-specific files (and .AppleDouble folders) if configured to ignore them
		if IgnoredMacFile(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// A symlinked file counts as what it points to (its size, not the link's).
		if info.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Stat(path); err == nil {
				info = target
			}
		}

		// An unpacked DVD (a VIDEO_TS folder) is one video, its files never on their own: once it's
		// taken (or left out), the walk doesn't go into it.
		dvd := info.IsDir() && DVDFolder(path)
		if info.IsDir() && !dvd {
			return nil
		}
		next := func(err error) error {
			if dvd && err == nil {
				return filepath.SkipDir
			}
			return err
		}
		size := info.Size()
		if dvd {
			size = DiscSize(path, info)
		}

		mediaType := config.MediaType(path)
		if mediaType == "" {
			return nil // Skip unsupported files
		}
		if IsNonDiscISO(path) {
			return next(nil) // An installer or data disc image, not a video
		}

		// Already scanned and unchanged (same size): nothing to redo, so a scan only probes new
		// and changed files. A missing thumbnail is retried when its folder is browsed instead,
		// so files that can't be read aren't probed again on every scan.
		s.job.Wait(context.Background(), nil) // held while paused
		s.job.Step(1)
		if known, err := s.db.GetMediaItem(path); err == nil && known.Size == size {
			*processed++
			// A video scanned before format lines were (or since): its format read once, the rest kept
			if known.Type == "video" && known.Format == "" {
				return next(s.db.SetMediaFormat(path, videoFormat(path)))
			}
			return next(nil)
		}

		// Update progress
		*processed++
		s.probed++
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
			Size:     size,
			MimeType: MimeType(path),
		}
		if dvd {
			item.Name = DiscName(path)
		}

		// Get metadata based on type
		switch mediaType {
		case "video":
			if IsDiscImage(path) {
				// A disc's length and size are its main title's
				if disc, err := LoadDisc(path); err == nil {
					if t := disc.Title(0); t != nil {
						item.Duration, item.Width, item.Height = t.Duration, t.Width, t.Height
					}
					item.ThumbnailURL = s.GenerateVideoThumbnail(path)
				}
				item.Format = "DVD"
			} else if err := s.getVideoMetadata(item); err == nil {
				item.ThumbnailURL = s.GenerateVideoThumbnail(path)
			}
		case "image":
			// Try to get metadata, but don't skip thumbnail if it fails (common for HEIC)
			s.getImageMetadata(item)
			item.ThumbnailURL = s.GenerateImageThumbnail(path)
		case "document":
			item.ThumbnailURL = s.generateDocThumbnail(path)
		}

		return next(s.db.SaveMediaItem(item))
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
		Type:     config.MediaType(path),
		Size:     info.Size(),
		MimeType: MimeType(path),
	}
	if info.IsDir() && DVDFolder(path) {
		item.Name, item.Size = DiscName(path), DiscSize(path, info)
	}

	// Get metadata based on type
	if item.Type == "video" && IsDiscImage(path) {
		// A disc image's length is its main feature's
		if info, err := LoadDisc(path); err == nil {
			if t := info.Title(0); t != nil {
				item.Duration, item.Width, item.Height = t.Duration, t.Width, t.Height
			}
		}
	} else if item.Type == "video" {
		// Get duration using ffprobe (a damaged video's fixed copy has the real one)
		src := Playable(path)
		cmd := exec.Command("ffprobe",
			"-v", "error",
			"-show_entries", "format=duration",
			"-of", "default=noprint_wrappers=1:nokey=1",
			src,
		)

		output, err := cmd.Output()
		if err == nil {
			var duration float64
			fmt.Sscanf(string(output), "%f", &duration)
			item.Duration = orPackets(src, duration)
		}
	} else if item.Type == "image" {
		s.getImageMetadata(item)
	}

	return item, nil
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

	// The length, and the first video stream's size.
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &probe); err != nil {
		return err
	}
	var fp formatProbe
	if json.Unmarshal(output, &fp) == nil {
		item.Format = formatLine(fp)
	}
	if item.Format == "" {
		item.Format = "?"
	}
	d, _ := strconv.ParseFloat(probe.Format.Duration, 64)
	item.Duration = orPackets(item.Path, d)
	for _, st := range probe.Streams {
		if st.CodecType == "video" && st.Width > 0 {
			item.Width, item.Height = st.Width, st.Height
			break
		}
	}
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
	thumbnailPath, thumbnailURL := GetThumbnailPath(thumbnailDir, hash, ".jpg")

	// Use the shared thumbnail generator
	if _, err := s.thumbnailGenerator.GenerateThumbnail(videoPath); err != nil {
		if errors.Is(err, ErrThumbnailFailedBefore) {
			return ""
		}
		log.Printf("[SCANNER] WARNING: Failed to generate video thumbnail for %s: %v", videoPath, err)
		return ""
	}

	return versionedThumbnailURL(thumbnailURL, thumbnailPath)
}

// ForgetThumbnailFailure lets path's thumbnail be tried again (refreshing a folder's thumbnails).
func (s *Scanner) ForgetThumbnailFailure(path string) {
	s.thumbnailGenerator.Forget(path)
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

	thumbnailPath, thumbnailURL := GetThumbnailPath(thumbnailDir, hash, thumbnailExt)

	// Use the shared thumbnail generator
	if _, err := s.thumbnailGenerator.GenerateThumbnail(imagePath); err != nil {
		if errors.Is(err, ErrThumbnailFailedBefore) {
			return ""
		}
		log.Printf("[SCANNER] WARNING: Failed to generate image thumbnail for %s: %v", imagePath, err)
		return ""
	}

	return versionedThumbnailURL(thumbnailURL, thumbnailPath)
}

// ThumbnailFile is a thumbnail URL without its version: "/thumbnails/a/b/<hash>.jpg".
func ThumbnailFile(url string) string {
	file, _, _ := strings.Cut(url, "?")
	return file
}

// versionedThumbnailURL adds the thumbnail's file time to its URL (?v=): a thumbnail made again
// (Regenerate Thumbnails) gets a new URL, so no cache keeps showing the old picture, in any listing
// that uses it (folder icons too). The same picture keeps the same URL.
func versionedThumbnailURL(url, path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return url
	}
	return url + "?v=" + strconv.FormatInt(info.ModTime().Unix(), 10)
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
			relativePath := strings.TrimPrefix(ThumbnailFile(item.ThumbnailURL), "/thumbnails/")
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

// videoFormat reads a video's format line (a disc's is "DVD"); "?" when it has none or can't be read.
func videoFormat(path string) string {
	if IsDiscImage(path) {
		return "DVD"
	}
	out, err := backgroundCommand(context.Background(), "ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", Playable(path)).Output()
	if err != nil {
		if ffmpegMissing() != "" {
			return "" // read on a later scan, once ffprobe is there
		}
		return "?"
	}
	var fp formatProbe
	if json.Unmarshal(out, &fp) != nil {
		return "?"
	}
	if line := formatLine(fp); line != "" {
		return line
	}
	return "?"
}
