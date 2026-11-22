package media

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Streamer struct{}

func NewStreamer() *Streamer {
	return &Streamer{}
}

// ServeContent handles HTTP range requests for efficient streaming. sess, if not nil, is the
// tracked stream the request belongs to. audioTranscode asks for the video as it is with the audio
// encoded to AAC (see transcodeAudioAndServe).
func (s *Streamer) ServeContent(w http.ResponseWriter, r *http.Request, filePath string, contentType string, forceTranscode bool, audioTranscode bool, autoTranscode bool, startTime string, autoConvertImages bool, bitrate string, resolution string, sess *StreamSession) error {
	// Check if file is HEIC/HEIF and auto-conversion is enabled
	ext := strings.ToLower(filepath.Ext(filePath))
	if autoConvertImages && (ext == ".heic" || ext == ".heif") {
		return s.convertAndServeImage(w, r, filePath)
	}
	if IsDiscImage(filePath) {
		// Disc images can't be played as files: always transcoded, from the disc's titles.
		return s.serveDisc(w, r, filePath, startTime, bitrate, resolution, sess)
	}
	tracks := ParseTrackOptions(r)
	if audioTranscode && !forceTranscode && bitrate == "" && resolution == "" && tracks.Subtitle < 0 {
		return s.transcodeAudioAndServe(w, r, filePath, startTime, tracks, sess)
	}

	// Check if transcoding is needed
	// 1. Explicitly requested via ?transcode=1 or bitrate/resolution provided
	// 2. Auto-transcoding enabled (default) and file needs transcoding
	shouldTranscode := forceTranscode || bitrate != "" || resolution != ""
	reason := ""
	switch {
	case bitrate != "" || resolution != "":
		reason = "The player asked for a lower quality"
	case forceTranscode:
		reason = "The player asked for a transcode (it couldn't play the original)"
	case tracks.Subtitle >= 0:
		shouldTranscode = true
		reason = "The player asked for subtitles burned in"
	}
	if !shouldTranscode && autoTranscode && s.needsTranscoding(filePath) {
		shouldTranscode = true
		reason = fmt.Sprintf("Browsers can't play %s files", ext)
		fmt.Printf("Auto-transcoding %s for browser compatibility\n", filePath)
	}

	if shouldTranscode {
		if forceTranscode || bitrate != "" || resolution != "" {
			fmt.Printf("Transcoding %s (requested: bitrate=%s, res=%s)\n", filePath, bitrate, resolution)
		}
		return s.transcodeAndServe(w, r, filePath, startTime, bitrate, resolution, reason, tracks, sess)
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
		src := &watchedFile{f: file}
		io.Copy(&trackedWriter{w: w, sess: sess}, src)
		logReadFailure(filePath, src)
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
	src := &watchedFile{f: file}
	io.CopyN(&trackedWriter{w: w, sess: sess, offset: ra.start}, src, ra.end-ra.start+1)
	logReadFailure(filePath, src)
	return nil
}

// watchedFile notes a read error, so a copy that stopped because the disk or share failed partway
// through can be told from one the client left. The response has begun by then, so the player only
// sees it end; the log says why.
type watchedFile struct {
	f   *os.File
	err error
}

func (w *watchedFile) Read(p []byte) (int, error) {
	n, err := w.f.Read(p)
	if err != nil && err != io.EOF {
		w.err = err
	}
	return n, err
}

func logReadFailure(filePath string, file *watchedFile) {
	if file.err != nil {
		fmt.Printf("Stopped serving %s: %v\n", filePath, file.err)
	}
}

// trackedWriter counts what is sent for a tracked stream, and how far into the file it reaches.
type trackedWriter struct {
	w      io.Writer
	sess   *StreamSession
	offset int64 // file offset of the next byte; -1 when the output isn't the file itself
}

// Flush sends what's written so far, so a transcode's bytes go out as they're made.
func (t *trackedWriter) Flush() {
	if f, ok := t.w.(http.Flusher); ok {
		f.Flush()
	}
}

func (t *trackedWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	if t.offset >= 0 {
		t.offset += int64(n)
	}
	t.sess.Sent(n, t.offset)
	return n, err
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
	tempFile, err := tempFileName("heic-*.jpg")
	if err != nil {
		return err
	}
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
	w.Header().Set("Cache-Control", "private, max-age=3600")
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

// UnreadableFile is a video the server can't open or read: gone from the disk, or refused by the
// disk or share. Message is for the viewer; Err is what the system said.
type UnreadableFile struct {
	Status  int // 404 when it's gone, 503 otherwise (often passing: a share briefly refusing)
	Message string
	Err     error
}

func (e *UnreadableFile) Error() string { return e.Message + ": " + e.Err.Error() }

// CheckReadable opens filePath and reads its first byte, so a stream or a transcode fails with
// the real reason before it starts, not with an empty response or a player error.
func CheckReadable(filePath string) *UnreadableFile {
	f, err := os.Open(filePath)
	if err != nil && Wake(filePath, err) {
		f, err = os.Open(filePath)
	}
	if err == nil {
		defer f.Close()
		_, err = f.Read(make([]byte, 1))
		if err == io.EOF {
			err = nil // empty: readable, just nothing to play
		}
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrNotExist):
		return &UnreadableFile{http.StatusNotFound, "The video file is missing from the server's disk or share", err}
	case errors.Is(err, os.ErrPermission):
		return &UnreadableFile{http.StatusServiceUnavailable, "The server isn't allowed to read this file (the disk or share refused it)", err}
	default:
		return &UnreadableFile{http.StatusServiceUnavailable, "The server can't read this file from its disk or share", err}
	}
}

// needsTranscoding checks if a file format needs transcoding for browser compatibility
func (s *Streamer) needsTranscoding(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	// Formats that need transcoding for browser compatibility
	incompatibleFormats := []string{".3gp", ".3gpp", ".avi", ".divx", ".xvid", ".iso", ".wmv", ".asf", ".flv", ".mkv"}
	for _, fmt := range incompatibleFormats {
		if ext == fmt {
			return true
		}
	}
	return false
}

// transcodeAndServe transcodes video on-the-fly and streams to client
func (s *Streamer) transcodeAndServe(w http.ResponseWriter, r *http.Request, filePath string, startTime string, bitrate string, resolution string, reason string, tracks TrackOptions, sess *StreamSession) error {
	if startTime != "" {
		fmt.Printf("Starting transcoding from %s seconds for: %s\n", startTime, filePath)
	} else {
		fmt.Printf("Starting transcoding from beginning for: %s\n", filePath)
	}

	// FFmpeg command for fast transcoding
	args := []string{}
	if startTime != "" {
		// Add -ss before -i for faster seeking
		args = append(args, "-ss", startTime)
	}
	args = append(args, "-i", filePath)

	// Down to that height, never up: a smaller source keeps its size.
	scale := ""
	if resolution != "" {
		scale = fmt.Sprintf("scale=-2:'min(%s,ih)'", resolution)
	}
	// A chosen audio track or burned-in subtitles need the streams mapped; otherwise ffmpeg picks.
	if tracks.Audio >= 0 || tracks.Subtitle >= 0 {
		args = append(args, tracks.mapArgs(filePath, scale)...)
		scale = "" // in the filter graph
	}

	// Video transcoding options
	args = append(args, "-c:v", "libx264", "-preset", "ultrafast")

	// Handle bitrate reduction
	if strings.HasSuffix(bitrate, "%") {
		percentageStr := strings.TrimSuffix(bitrate, "%")
		percentage, err := strconv.ParseFloat(percentageStr, 64)
		if err == nil {
			// Get original bitrate
			origBitrate := s.getBitrate(filePath)
			if origBitrate > 0 {
				targetBitrate := int64(float64(origBitrate) * (percentage / 100.0))
				if targetBitrate > 0 {
					args = append(args, "-b:v", strconv.FormatInt(targetBitrate, 10))
					args = append(args, "-maxrate", strconv.FormatInt(targetBitrate, 10))
					args = append(args, "-bufsize", strconv.FormatInt(targetBitrate*2, 10))
				}
			}
		}
	} else if bitrate != "" {
		args = append(args, "-b:v", bitrate)
	} else {
		// Default to CRF if no specific bitrate requested
		args = append(args, "-crf", "23")
	}

	if scale != "" {
		args = append(args, "-vf", scale)
	}

	audio, audioInfo := tracks.audioArgs(false)
	args = append(args, "-pix_fmt", "yuv420p") // 8-bit 4:2:0, which every hardware H.264 decoder plays
	args = append(args, audio...)
	args = append(args,
		"-movflags", tracks.movFlags(),
		"-f", "mp4",
		"pipe:1",
	)
	info := describeTranscode(args, startTime, bitrate, resolution, reason)
	info.Audio = audioInfo
	sess.StartTranscode(info)
	return s.runTranscode(w, r, filePath, args, sess)
}

// runTranscode streams a transcode (ffmpeg with args, writing fragmented MP4 to stdout) to the
// client, from the byte it asks for: see transcache.go.
func (s *Streamer) runTranscode(w http.ResponseWriter, r *http.Request, filePath string, args []string, sess *StreamSession) error {
	offset := transcodeOffset(r)
	spool, err := transcodes.attach(filePath, args, offset, sess)
	if errors.Is(err, errCannotResume) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}
	if err != nil {
		return err
	}
	defer spool.detach()
	if err := spool.ready(r.Context(), offset); err != nil {
		return err
	}

	// The length isn't known, so a resumed part can't name its real end.
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	if offset > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", offset, offset+transcodeUnknownLength-1))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	err = spool.streamFrom(r.Context(), &trackedWriter{w: w, sess: sess, offset: -1}, offset)
	if r.Context().Err() != nil {
		fmt.Printf("Client disconnected from transcode for: %s\n", filePath)
		return nil
	}
	return err
}

// transcodeUnknownLength stands in for the rest of a stream of unknown length.
const transcodeUnknownLength = int64(1) << 42

// describeTranscode summarizes ffmpeg arguments for the Streams page.
func describeTranscode(args []string, startTime, bitrate, resolution, reason string) TranscodeInfo {
	info := TranscodeInfo{Reason: reason, Bitrate: bitrate, Resolution: resolution, Args: args,
		Container: "Fragmented MP4"}
	info.Start, _ = strconv.ParseFloat(startTime, 64)
	opt := map[string]string{}
	for i := 0; i+1 < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			opt[args[i]] = args[i+1]
		}
	}
	info.encoder = "libx264, preset " + opt["-preset"]
	video := "H.264 (" + info.encoder
	if crf, ok := opt["-crf"]; ok {
		video += ", CRF " + crf
	}
	if b, ok := opt["-b:v"]; ok {
		if n, err := strconv.ParseInt(b, 10, 64); err == nil {
			video += fmt.Sprintf(", %d kb/s", n/1000)
		} else {
			video += ", " + b
		}
	}
	if vf, ok := opt["-vf"]; ok {
		video += ", " + vf
	}
	info.Video = video + ")"
	return info
}

func (s *Streamer) getBitrate(filePath string) int64 {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=bit_rate",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	bitrate, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	return bitrate
}
