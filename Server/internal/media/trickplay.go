package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

type TrickplayGenerator struct {
	db *database.DB
}

func NewTrickplayGenerator(db *database.DB) *TrickplayGenerator {
	return &TrickplayGenerator{
		db: db,
	}
}

func (tg *TrickplayGenerator) Generate(videoPath string) error {
	return tg.generate(context.Background(), videoPath, nil)
}

// maxFrameFailures is how many frames in a row may fail before a video is given up on.
const maxFrameFailures = 10

// trickplayDone marks a video's frames as all made (a folder without it was stopped part way).
const trickplayDone = ".done"

// HasTrickplay says whether all of a video's frames have been made.
func HasTrickplay(videoPath string) bool {
	dir := config.GlobalConfig.Media.TrickplayPath
	if dir == "" {
		return true // nowhere to put them
	}
	_, err := os.Stat(filepath.Join(dir, GenerateThumbnailHash(videoPath), trickplayDone))
	return err == nil
}

// generate makes a video's frames, calling wait (when given) before each one so a background
// job can pause; it stops when ctx ends or wait says so, and marks the video done when finished.
func (tg *TrickplayGenerator) generate(ctx context.Context, videoPath string, wait func() bool) error {
	fmt.Printf("Generating trickplay frames for %s\n", videoPath)

	duration, codec, err := tg.getVideoInfo(videoPath)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}

	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		return fmt.Errorf("trickplay path is not configured")
	}

	hash := GenerateThumbnailHash(videoPath)
	trickplayVideoDir := filepath.Join(trickplayDir, hash)

	if err := os.MkdirAll(trickplayVideoDir, 0755); err != nil {
		return fmt.Errorf("failed to create trickplay directory: %w", err)
	}
	removeParts(trickplayVideoDir)

	start, err := os.Stat(videoPath)
	if err != nil {
		return err
	}

	// Generate frames every 10 seconds.
	// Stop 1 second before the end to avoid empty output issues with input seeking.
	failures, lastGood := 0, -1
	for i := 0; i <= int(duration)-1; i += 10 {
		if ctx.Err() != nil || (wait != nil && !wait()) {
			return ctx.Err()
		}
		if err := tg.generateFrame(ctx, videoPath, trickplayVideoDir, float64(i), i, codec); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Printf("Warning: failed to generate frame at %ds: %v\n", i, err)
			if failures++; failures >= maxFrameFailures {
				// Frames worked up to a point and none after it: a damaged file (cut short, empty or
				// garbled past there, or a header claiming more than it holds). If it's still the
				// same file and its last good frame still comes out, it's the file and not a share
				// that went away: mark it done with the frames it has instead of trying it again
				// every start. Otherwise give up for now, keeping the frames made, and leave it
				// unmarked.
				if lastGood >= 0 && tg.stillReadable(ctx, videoPath, start, float64(lastGood), codec) {
					fmt.Printf("Damaged file: no frames after %ds of %.0fs; keeping the frames up to there: %s\n", lastGood, duration, videoPath)
					break
				}
				return fmt.Errorf("%d frames in a row failed (at %ds of %.0fs)", failures, i, duration)
			}
		} else {
			failures, lastGood = 0, i
		}
	}
	if err := os.WriteFile(filepath.Join(trickplayVideoDir, trickplayDone), nil, 0644); err != nil {
		return err
	}

	fmt.Printf("Finished generating trickplay frames for %s\n", videoPath)
	return nil
}

func (tg *TrickplayGenerator) GenerateForDirectory(dirPath string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip Mac-specific files if configured to ignore them
		if config.GlobalConfig.Media.IgnoreMacFiles && isMacFile(path) {
			return nil
		}

		if !info.IsDir() && isSupportedVideo(path) {
			if err := tg.Generate(path); err != nil {
				fmt.Printf("Error generating trickplay frames for %s: %v\n", path, err)
			}
		}
		return nil
	})
}

func (tg *TrickplayGenerator) Clean() error {
	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		return fmt.Errorf("trickplay path is not configured")
	}

	items, err := tg.db.ListMediaItems("video")
	if err != nil {
		return fmt.Errorf("failed to list media items: %w", err)
	}

	validHashes := make(map[string]bool)
	for _, item := range items {
		hash := GenerateThumbnailHash(item.Path)
		trickplayVideoDir := filepath.Join(trickplayDir, hash)

		// Check if source file exists
		if _, err := os.Stat(item.Path); os.IsNotExist(err) {
			if _, err := os.Stat(trickplayVideoDir); err == nil {
				fmt.Printf("Removing trickplay images for non-existent path: %s\n", item.Path)
				fmt.Printf("  -> Mapping to: %s\n", trickplayVideoDir)
				if err := os.RemoveAll(trickplayVideoDir); err != nil {
					fmt.Printf("Error removing %s: %v\n", trickplayVideoDir, err)
				}
			}
		} else {
			validHashes[hash] = true
		}
	}

	// Also remove folders that aren't in the database at all
	entries, err := os.ReadDir(trickplayDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		hash := entry.Name()
		if !validHashes[hash] {
			trickplayVideoDir := filepath.Join(trickplayDir, hash)
			fmt.Printf("Removing orphaned trickplay folder: %s (no active record in database)\n", hash)
			if err := os.RemoveAll(trickplayVideoDir); err != nil {
				fmt.Printf("Error removing %s: %v\n", trickplayVideoDir, err)
			}
		}
	}

	return nil
}

func isSupportedVideo(path string) bool {
	if IsDiscImage(path) {
		return false // disc images have no single video stream to sample frames from
	}
	ext := strings.ToLower(filepath.Ext(path))
	for _, videoExt := range config.GlobalConfig.Media.SupportedVideoFormats {
		if ext == videoExt {
			return true
		}
	}
	return false
}

func (tg *TrickplayGenerator) getVideoInfo(videoPath string) (float64, string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,duration:format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("ffprobe failed: %s", string(output))
	}

	// The codec and the duration, in whatever order and with whatever repeats ffprobe prints them
	// (a transport stream can list its video stream more than once). The duration is the shorter of
	// the video's and the file's: the file's is its longest stream's, which can be an audio stream
	// that runs on past the video, or whose header claims days.
	var codec string
	var duration float64
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if d, err := strconv.ParseFloat(line, 64); err == nil {
			if d > 0 && (duration == 0 || d < duration) {
				duration = d
			}
		} else if codec == "" && line != "" && line != "N/A" {
			codec = line
		}
	}
	if duration <= 0 {
		return 0, "", fmt.Errorf("ffprobe gave no duration")
	}

	return duration, codec, nil
}

// stillReadable says whether a video is the file it was when its frames were started (same size
// and time) and a frame still comes out at seconds.
func (tg *TrickplayGenerator) stillReadable(ctx context.Context, videoPath string, start os.FileInfo, seconds float64, codec string) bool {
	now, err := os.Stat(videoPath)
	if err != nil || now.Size() != start.Size() || !now.ModTime().Equal(start.ModTime()) {
		return false
	}
	check, err := os.CreateTemp("", "trickplay-check-*.jpg")
	if err != nil {
		return false
	}
	check.Close()
	defer os.Remove(check.Name())
	return tg.extractFrame(ctx, videoPath, check.Name(), seconds, codec)
}

func (tg *TrickplayGenerator) generateFrame(ctx context.Context, videoPath, outputDir string, seekTime float64, frameIndex int, codec string) error {
	framePath := filepath.Join(outputDir, fmt.Sprintf("frame-%d.jpg", frameIndex))

	if info, err := os.Stat(framePath); err == nil && info.Size() > 0 {
		return nil
	}
	part := partPath(framePath)

	if tg.extractFrame(ctx, videoPath, part, seekTime, codec) {
		return finishPart(framePath)
	}
	os.Remove(part)

	return fmt.Errorf("failed to extract frame %d", frameIndex)
}

// extractFrame writes the frame at seekTime to out, saying whether a picture came out.
func (tg *TrickplayGenerator) extractFrame(ctx context.Context, videoPath, out string, seekTime float64, codec string) bool {
	timestamp := fmt.Sprintf("%.3f", seekTime)

	// format=yuvj420p and -strict unofficial are MANDATORY for MJPEG compliance in FFmpeg 7.1.1.
	var args []string
	if codec == "h264" || codec == "hevc" {
		args = append(args, "-bsf:v", codec+"_metadata=matrix_coefficients=1")
	}
	args = append(args, "-ss", timestamp, "-noaccurate_seek", "-i", videoPath, "-vf", "scale=320:-1,format=yuvj420p", "-frames:v", "1", "-strict", "unofficial", "-y", out)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if err := cmd.Run(); err == nil {
		if info, statErr := os.Stat(out); statErr == nil && info.Size() > 0 {
			return true
		}
	}

	// Simple fallback without BSF or format flags.
	cmdFallback := exec.CommandContext(ctx, "ffmpeg", "-ss", timestamp, "-i", videoPath, "-vf", "scale=320:-1", "-frames:v", "1", "-strict", "unofficial", "-y", out)
	if errFallback := cmdFallback.Run(); errFallback == nil {
		if info, statErr := os.Stat(out); statErr == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

// isMacFile checks if a filename is a Mac-specific file that should be ignored
func isMacFile(path string) bool {
	baseName := filepath.Base(path)
	return baseName == ".DS_Store" || strings.HasPrefix(baseName, "._")
}

// errInterrupted: a disc's frames were stopped part way (someone started streaming); try later.
var errInterrupted = errors.New("interrupted")

// generateDisc makes frames for a disc image's main title. Seeking into a title means decoding from
// the start of its part (program chain), so each part is decoded once instead, keyframes only, with
// a frame kept every 10 seconds; frames are named by their time in the title. busy (when given) is
// checked while decoding, and stops it. The .done marker holds the title's number.
func (tg *TrickplayGenerator) generateDisc(ctx context.Context, path string, wait, busy func() bool) error {
	info, err := LoadDisc(path)
	if err != nil {
		return err
	}
	title := info.Title(0)
	if title == nil || len(title.Segments) == 0 {
		return errors.New("no main title")
	}
	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		return fmt.Errorf("trickplay path is not configured")
	}
	dir := filepath.Join(trickplayDir, GenerateThumbnailHash(path))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	removeParts(dir)
	for i, seg := range title.Segments {
		if ctx.Err() != nil || (wait != nil && !wait()) {
			return ctx.Err()
		}
		if err := tg.discSegmentFrames(ctx, path, title, i, seg, dir, busy); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, trickplayDone), []byte(strconv.Itoa(title.Number)), 0644)
}

func (tg *TrickplayGenerator) discSegmentFrames(ctx context.Context, path string, title *DiscTitle, i int, seg DiscSegment, dir string, busy func() bool) error {
	tmp, err := os.MkdirTemp(dir, "part")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	interrupted := false
	if busy != nil {
		go func() {
			for run.Err() == nil {
				if busy() {
					interrupted = true
					cancel()
					return
				}
				select {
				case <-run.Done():
				case <-time.After(5 * time.Second):
				}
			}
		}()
	}
	input, _ := title.InputArgs(path, i, 1, 0)
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-skip_frame", "nokey"}, input...)
	args = append(args, "-map", "0:v:0", "-vf", "fps=1/10,scale=320:-1,format=yuvj420p", "-strict", "unofficial",
		filepath.Join(tmp, "%d.jpg"))
	var stderr bytes.Buffer
	cmd := exec.CommandContext(run, "ffmpeg", args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if interrupted {
			return errInterrupted
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("disc frames: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	// n.jpg (from 1) is the frame 10*(n-1) seconds into the part.
	entries, _ := os.ReadDir(tmp)
	for _, e := range entries {
		n, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".jpg"))
		if err != nil {
			continue
		}
		at := int(seg.Start + float64(n-1)*10)
		os.Rename(filepath.Join(tmp, e.Name()), filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", at)))
	}
	return nil
}

// TrickplayTitle is the disc title a disc image's frames are for (0 for a video file, or none yet).
func TrickplayTitle(videoPath string) int {
	dir := config.GlobalConfig.Media.TrickplayPath
	if dir == "" {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(dir, GenerateThumbnailHash(videoPath), trickplayDone))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// removeParts clears what a stopped run left in a video's frames folder: pictures made part way
// (*.part.jpg) and a disc's half-decoded parts (part* folders).
func removeParts(dir string) {
	for _, pattern := range []string{"*.part.jpg", "part*"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, m := range matches {
			os.RemoveAll(m)
		}
	}
}
