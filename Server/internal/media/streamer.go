package media

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Streamer struct{}

func NewStreamer() *Streamer {
	return &Streamer{}
}

// ServeContent handles HTTP range requests for efficient streaming
func (s *Streamer) ServeContent(w http.ResponseWriter, r *http.Request, filePath string, contentType string, forceTranscode bool, autoTranscode bool, startTime string, autoConvertImages bool) error {
	// Check if file is HEIC/HEIF and auto-conversion is enabled
	ext := strings.ToLower(filepath.Ext(filePath))
	if autoConvertImages && (ext == ".heic" || ext == ".heif") {
		return s.convertAndServeImage(w, r, filePath)
	}

	// Check if transcoding is needed
	// 1. Explicitly requested via ?transcode=1
	// 2. Auto-transcoding enabled (default) and file needs transcoding
	shouldTranscode := forceTranscode
	if !shouldTranscode && autoTranscode && s.needsTranscoding(filePath) {
		shouldTranscode = true
		fmt.Printf("Auto-transcoding %s for browser compatibility\n", filePath)
	}

	if shouldTranscode {
		if forceTranscode {
			fmt.Printf("Transcoding %s (explicitly requested)\n", filePath)
		}
		return s.transcodeAndServe(w, r, filePath, startTime)
	}

	// Serve original file
	fmt.Printf("Serving original file: %s\n", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	size := stat.Size()
	rangeHeader := r.Header.Get("Range")

	// Set headers
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", contentType)

	if rangeHeader == "" {
		// No range request, serve entire file
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
		return nil
	}

	// Parse range header
	ranges, err := parseRange(rangeHeader, size)
	if err != nil || len(ranges) == 0 {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return fmt.Errorf("invalid range")
	}

	// Support single range for now
	ra := ranges[0]

	// Seek to start position
	if _, err := file.Seek(ra.start, 0); err != nil {
		return err
	}

	// Set headers for partial content
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", ra.start, ra.end, size))
	w.Header().Set("Content-Length", strconv.FormatInt(ra.end-ra.start+1, 10))
	w.WriteHeader(http.StatusPartialContent)

	// Stream the requested range
	io.CopyN(w, file, ra.end-ra.start+1)
	return nil
}

type httpRange struct {
	start, end int64
}

func parseRange(s string, size int64) ([]httpRange, error) {
	if !strings.HasPrefix(s, "bytes=") {
		return nil, fmt.Errorf("invalid range")
	}

	var ranges []httpRange
	for _, ra := range strings.Split(s[6:], ",") {
		ra = strings.TrimSpace(ra)
		if ra == "" {
			continue
		}

		i := strings.Index(ra, "-")
		if i < 0 {
			return nil, fmt.Errorf("invalid range")
		}

		start, end := strings.TrimSpace(ra[:i]), strings.TrimSpace(ra[i+1:])
		var r httpRange

		if start == "" {
			// "-500" means last 500 bytes
			n, err := strconv.ParseInt(end, 10, 64)
			if err != nil {
				return nil, err
			}
			if n > size {
				n = size
			}
			r.start = size - n
			r.end = size - 1
		} else {
			n, err := strconv.ParseInt(start, 10, 64)
			if err != nil || n < 0 {
				return nil, err
			}
			r.start = n

			if end == "" {
				// "500-" means from byte 500 to end
				r.end = size - 1
			} else {
				n, err := strconv.ParseInt(end, 10, 64)
				if err != nil || r.start > n {
					return nil, err
				}
				if n >= size {
					n = size - 1
				}
				r.end = n
			}
		}

		if r.start > size {
			return nil, fmt.Errorf("invalid range")
		}

		ranges = append(ranges, r)
	}

	return ranges, nil
}

// convertAndServeImage converts HEIC/HEIF images to JPEG on-the-fly and streams them
func (s *Streamer) convertAndServeImage(w http.ResponseWriter, r *http.Request, filePath string) error {
	fmt.Printf("Converting HEIC/HEIF image to JPEG on-the-fly: %s\n", filePath)

	// Use heif-convert to properly handle tiled/grid HEIC images
	// Create a temporary file since heif-convert doesn't support stdout
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("heic_stream_%d.jpg", time.Now().UnixNano()))
	defer os.Remove(tempFile)

	cmd := exec.CommandContext(r.Context(), "heif-convert", filePath, tempFile)
	
	// Capture stderr for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start conversion
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start heif-convert: %w", err)
	}

	// Monitor for client disconnect and kill process
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
			fmt.Printf("Client disconnected, killing conversion for: %s\n", filePath)
			cmd.Process.Kill()
		case <-done:
			// Normal completion
		}
	}()

	// Log stderr in background
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				fmt.Printf("heif-convert: %s", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	// Wait for conversion to complete
	cmdErr := cmd.Wait()
	close(done)

	if cmdErr != nil {
		// If context was cancelled, this is expected
		if r.Context().Err() != nil {
			return nil
		}

		// Fallback: if heif-convert fails, it might actually be a JPEG file mislabeled
		fmt.Printf("heif-convert failed for %s, trying to serve as original JPEG: %v\n", filePath, cmdErr)
		return s.serveOriginalFile(w, r, filePath, "image/jpeg")
	}

	// Now stream the converted JPEG file
	file, err := os.Open(tempFile)
	if err != nil {
		return fmt.Errorf("failed to open converted file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat converted file: %w", err)
	}

	// Set headers for JPEG content
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)

	// Stream the file
	_, copyErr := io.Copy(w, file)

	fmt.Printf("Image conversion finished for: %s\n", filePath)

	return copyErr
}

// serveOriginalFile serves the original file with range request support
func (s *Streamer) serveOriginalFile(w http.ResponseWriter, r *http.Request, filePath string, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	http.ServeContent(w, r, filePath, stat.ModTime(), file)
	return nil
}

// needsTranscoding checks if a file format needs transcoding for browser compatibility
func (s *Streamer) needsTranscoding(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	// Formats that need transcoding for browser compatibility
	incompatibleFormats := []string{".3gp", ".3gpp", ".avi", ".wmv", ".asf", ".flv", ".mkv"}
	for _, fmt := range incompatibleFormats {
		if ext == fmt {
			return true
		}
	}
	return false
}

// transcodeAndServe transcodes video on-the-fly and streams to client
func (s *Streamer) transcodeAndServe(w http.ResponseWriter, r *http.Request, filePath string, startTime string) error {
	if startTime != "" {
		fmt.Printf("Starting transcoding from %s seconds for: %s\n", startTime, filePath)
	} else {
		fmt.Printf("Starting transcoding from beginning for: %s\n", filePath)
	}

	// FFmpeg command for fast transcoding
	// -ss: seek to start position (before -i for faster seeking)
	// -i: input file
	// -c:v libx264: encode video with h264
	// -preset ultrafast: fastest encoding (lower quality but real-time)
	// -crf 23: constant quality (lower is better, 23 is good default)
	// -c:a aac: encode audio with aac
	// -b:a 128k: audio bitrate
	// -movflags frag_keyframe+empty_moov: enable streaming for mp4
	// -f mp4: output format
	args := []string{}
	if startTime != "" {
		// Add -ss before -i for faster seeking
		args = append(args, "-ss", startTime)
	}
	args = append(args,
		"-i", filePath,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4",
		"pipe:1",
	)

	cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)

	// Get stdout and stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start ffmpeg
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Monitor for client disconnect and kill process
	done := make(chan struct{})
	go func() {
		select {
		case <-r.Context().Done():
			fmt.Printf("Client disconnected, killing transcode for: %s\n", filePath)
			cmd.Process.Kill()
		case <-done:
			// Normal completion
		}
	}()

	// Log stderr in background
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				fmt.Printf("FFmpeg: %s", string(buf[:n]))
			}
			if err != nil {
				break
			}
		}
	}()

	// Set headers for transcoded content BEFORE writing anything
	w.Header().Set("Content-Type", "video/mp4")
	w.WriteHeader(http.StatusOK)

	// Stream the transcoded output
	_, copyErr := io.Copy(w, stdout)

	// Wait for ffmpeg to finish
	cmdErr := cmd.Wait()
	close(done)

	fmt.Printf("Transcoding finished for: %s\n", filePath)

	if copyErr != nil {
		return copyErr
	}
	if cmdErr != nil {
		// If context was cancelled, this is expected
		if r.Context().Err() != nil {
			return nil
		}
		return fmt.Errorf("ffmpeg error: %w", cmdErr)
	}

	return nil
}
