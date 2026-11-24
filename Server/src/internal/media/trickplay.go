package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
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
	if IsDiscImage(videoPath) {
		return tg.generateDisc(context.Background(), videoPath, nil, nil)
	}
	return tg.generate(context.Background(), videoPath, nil)
}

// trickplayBytes is the size of every video's frames: counted with the other data folders (see
// storage.go), with each frame made since added (frames removed with their video are taken off at
// the next count).
var trickplayBytes atomic.Int64

// addFrame counts a frame just made.
func addFrame(path string) {
	if info, err := os.Stat(path); err == nil {
		trickplayBytes.Add(info.Size())
	}
}

// maxFrameFailures is how many frames in a row may fail before a video is given up on.
const maxFrameFailures = 10

// trickplayDone marks a video's frames as all made (a folder without it was stopped part way).
const trickplayDone = ".done"

// noFramesMade says whether a video was marked done without frames (not a video: see
// markNoFrames), as opposed to done with frames. Both leave an empty marker, so this looks for a
// frame.
func noFramesMade(videoPath string) bool {
	dir := config.GlobalConfig.Media.TrickplayPath
	if dir == "" || !HasTrickplay(videoPath) {
		return false
	}
	frames, _ := filepath.Glob(filepath.Join(dir, GenerateThumbnailHash(videoPath), "frame-*.jpg"))
	return len(frames) == 0
}

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
	// A damaged video's frames come from its fixed copy (src); what's kept is keyed by videoPath.
	src := Playable(videoPath)

	duration, codec, err := tg.getVideoInfo(src)
	if errors.Is(err, errUnreadableMedia) && ctx.Err() == nil && readable(videoPath) {
		// Every way of reading it failed, but the file is there and reads (so it's not the share):
		// not a video (code named .ts, say, or a corrupt file). Marked done, it isn't tried again.
		fmt.Printf("Not a video, no frames to make: %s (%v)\n", videoPath, err)
		return markNoFrames(videoPath)
	}
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}
	tg.saveDuration(videoPath, duration)

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

	start, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Generate frames every 10 seconds, in batches of config.TrickplayFramesPerRunner (one ffmpeg
	// process each, read for every batch so a change applies at once). A frame a batch doesn't
	// make is tried again on its own, so a failure is judged as it always was.
	var pending []int
	failures, lastGood := 0, -1
	for i := 0; i <= int(duration)-1; i += 10 {
		if info, err := os.Stat(filepath.Join(trickplayVideoDir, fmt.Sprintf("frame-%d.jpg", i))); err == nil && info.Size() > 0 {
			failures, lastGood = 0, i // made already
			continue
		}
		pending = append(pending, i)
	}
loop:
	for len(pending) > 0 {
		if ctx.Err() != nil || (wait != nil && !wait()) {
			return ctx.Err()
		}
		n := min(config.TrickplayFramesPerRunner(), len(pending))
		batch := pending[:n]
		pending = pending[n:]
		made := tg.extractBatch(ctx, src, trickplayVideoDir, codec, batch)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for k, i := range batch {
			if made[k] {
				framePath := filepath.Join(trickplayVideoDir, fmt.Sprintf("frame-%d.jpg", i))
				if err := finishPart(framePath); err != nil {
					return err
				}
				addFrame(framePath)
				failures, lastGood = 0, i
				continue
			}
			if err := tg.generateFrame(ctx, src, trickplayVideoDir, float64(i), i, codec); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				fmt.Printf("Warning: failed to generate frame at %ds of %s: %v\n", i, videoPath, err)
				if failures++; failures >= maxFrameFailures {
					// Frames worked up to a point and none after it: a damaged file (cut short, empty or
					// garbled past there, or a header claiming more than it holds). If it's still the
					// same file and its last good frame still comes out, it's the file and not a share
					// that went away: mark it done with the frames it has instead of trying it again
					// every start. Otherwise give up for now, keeping the frames made, and leave it
					// unmarked.
					if lastGood >= 0 && tg.stillReadable(ctx, src, start, float64(lastGood), codec) {
						fmt.Printf("Damaged file: no frames after %ds of %.0fs; keeping the frames up to there: %s\n", lastGood, duration, videoPath)
						break loop
					}
					return fmt.Errorf("%d frames in a row failed (at %ds of %.0fs)", failures, i, duration)
				}
			} else {
				failures, lastGood = 0, i
			}
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
		if IgnoredMacFile(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
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
	return config.IsVideo(path)
}

func (tg *TrickplayGenerator) getVideoInfo(videoPath string) (float64, string, error) {
	cmd := backgroundCommand(context.Background(), "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name,duration:format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// ffprobe ran and turned the file down (rather than not running, or being stopped).
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.Exited() {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			return 0, "", fmt.Errorf("%w: ffprobe failed: %s", errUnreadableMedia, lines[len(lines)-1]) // its summary
		}
		return 0, "", fmt.Errorf("ffprobe failed: %v %s", err, string(output))
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
		duration = PacketDuration(videoPath) // a header without a length
	}
	if duration <= 0 {
		return 0, "", fmt.Errorf("%w: ffprobe gave no duration, nor its packets", errUnreadableMedia)
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
	check, err := tempFileName("trickplay-check-*.jpg")
	if err != nil {
		return false
	}
	defer os.Remove(check)
	return tg.extractFrame(ctx, videoPath, check, seconds, codec)
}

func (tg *TrickplayGenerator) generateFrame(ctx context.Context, videoPath, outputDir string, seekTime float64, frameIndex int, codec string) error {
	framePath := filepath.Join(outputDir, fmt.Sprintf("frame-%d.jpg", frameIndex))

	if info, err := os.Stat(framePath); err == nil && info.Size() > 0 {
		return nil
	}
	part := partPath(framePath)

	if tg.extractFrame(ctx, videoPath, part, seekTime, codec) {
		if err := finishPart(framePath); err != nil {
			return err
		}
		addFrame(framePath)
		return nil
	}
	os.Remove(part)

	return fmt.Errorf("failed to extract frame %d", frameIndex)
}

// saveDuration stores a video's length when the library has none (older scans didn't save
// lengths, and a header without one gave 0 before the packets were read for it).
func (tg *TrickplayGenerator) saveDuration(videoPath string, duration float64) {
	tg.saveLength(videoPath, duration, 0, 0)
}

// saveLength stores a video's length, and its picture size if it has none, when the library has
// no length for it.
func (tg *TrickplayGenerator) saveLength(videoPath string, duration float64, width, height int) {
	if tg.db == nil || duration <= 0 {
		return
	}
	if item, err := tg.db.GetMediaItem(videoPath); err == nil && item.Duration <= 0 {
		item.Duration = duration
		if item.Width <= 0 && width > 0 {
			item.Width, item.Height = width, height
		}
		if err := tg.db.SaveMediaItem(item); err != nil {
			log.Printf("trickplay: %s: saving its length: %v", videoPath, err)
			return
		}
		log.Printf("trickplay: %s: length filled in (%s)", videoPath,
			time.Duration(duration*float64(time.Second)).Round(time.Second))
	}
}

// fillLength reads and stores the length of a video the library has none for (one whose frames
// are already made): a disc's main title's, else the file's (its packets', when its header has none).
func (tg *TrickplayGenerator) fillLength(path string) error {
	if IsDiscImage(path) {
		info, err := LoadDisc(path)
		if IsNotVideoDisc(err) {
			return nil // an installer or data disc: no length to have
		}
		if err != nil {
			return err
		}
		if t := info.Title(0); t != nil && t.Duration > 0 {
			tg.saveLength(path, t.Duration, t.Width, t.Height)
			return nil
		}
		return errors.New("no length")
	}
	d, w, h, err := ProbeVideo(path)
	if err != nil {
		return err
	}
	if d <= 0 {
		return errors.New("no length")
	}
	tg.saveLength(path, d, w, h)
	return nil
}

// extractBatch makes the frames at the given times (seconds) in one ffmpeg process, each input
// seeking like extractFrame does, and says which came out. ffmpeg exits with success even when
// some inputs fail, so a frame counts only when its file was written.
func (tg *TrickplayGenerator) extractBatch(ctx context.Context, videoPath, dir, codec string, times []int) []bool {
	made := make([]bool, len(times))
	outs := make([]string, len(times))
	args := []string{"-v", "error"}
	for k, i := range times {
		outs[k] = partPath(filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", i)))
		os.Remove(outs[k])
		if codec == "h264" || codec == "hevc" {
			args = append(args, "-threads", "1", "-bsf:v", codec+"_metadata=matrix_coefficients=1")
		} else {
			args = append(args, "-threads", "1")
		}
		args = append(args, "-ss", fmt.Sprintf("%.3f", float64(i)), "-noaccurate_seek", "-i", videoPath)
	}
	for k := range times {
		args = append(args, "-map", fmt.Sprintf("%d:v:0", k), "-vf", "scale=320:-1,format=yuvj420p",
			"-frames:v", "1", "-strict", "unofficial", "-y", outs[k])
	}
	backgroundCommand(ctx, "ffmpeg", args...).Run()
	for k := range times {
		if info, err := os.Stat(outs[k]); err == nil && info.Size() > 0 && ctx.Err() == nil {
			made[k] = true
		} else {
			os.Remove(outs[k])
		}
	}
	return made
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

	cmd := backgroundCommand(ctx, "ffmpeg", args...)
	if err := cmd.Run(); err == nil {
		if info, statErr := os.Stat(out); statErr == nil && info.Size() > 0 {
			return true
		}
	}

	// Simple fallback without BSF or format flags.
	cmdFallback := backgroundCommand(ctx, "ffmpeg", "-ss", timestamp, "-i", videoPath, "-vf", "scale=320:-1", "-frames:v", "1", "-strict", "unofficial", "-y", out)
	if errFallback := cmdFallback.Run(); errFallback == nil {
		if info, statErr := os.Stat(out); statErr == nil && info.Size() > 0 {
			return true
		}
	}
	return false
}

// IsMacFile says whether a path is a Mac's leftover rather than media: .DS_Store, an AppleDouble
// "._" file (a file's resource fork on a share), or anything in an .AppleDouble folder (Netatalk's).
func IsMacFile(path string) bool {
	base := filepath.Base(path)
	if base == ".DS_Store" || strings.HasPrefix(base, "._") {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".AppleDouble" {
			return true
		}
	}
	return false
}

// errUnreadableMedia: ffprobe read the file and found no video it can use (no length either).
var errUnreadableMedia = errors.New("not readable as a video")

// readable says whether a file's first bytes can be read now.
func readable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	n, err := f.Read(make([]byte, 64<<10))
	return n > 0 && (err == nil || err == io.EOF)
}

// hasData says whether a file is there (following symlinks) and isn't empty (a copy that failed).
func hasData(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// IgnoredMacFile says whether a path is a Mac's leftover and the settings say to ignore those.
func IgnoredMacFile(path string) bool {
	return config.GlobalConfig.Media.IgnoreMacFiles && IsMacFile(path)
}

// errInterrupted: a disc's frames were stopped part way (someone started streaming); try later.
var errInterrupted = errors.New("interrupted")

// generateDisc makes frames for a disc image's main title. Seeking into a title means decoding from
// the start of its part (program chain), so each part is decoded once instead, keyframes only, with
// a frame kept every 10 seconds; frames are named by their time in the title. busy (when given) is
// checked while decoding, and stops it. The .done marker holds the title's number.
func (tg *TrickplayGenerator) generateDisc(ctx context.Context, path string, wait, busy func() bool) error {
	info, err := LoadDisc(path)
	if IsNotVideoDisc(err) {
		// An installer or data disc: it never will have frames, so mark it done (with no title).
		fmt.Printf("Not a video DVD, no frames to make: %s (%v)\n", path, err)
		return markNoFrames(path)
	}
	if err != nil {
		return err
	}
	title := info.Title(0)
	if title == nil || len(title.Segments) == 0 {
		return errors.New("no main title")
	}
	if info.MainTitleUncertain {
		log.Printf("trickplay: %s: a title that can't be read may be longer than title %d, used as the main title", path, title.Number)
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
	// As for files: a line when a disc starts and when it's done (or why it stopped, from the job).
	fmt.Printf("Generating trickplay frames for %s (title %d, %s)\n", path, title.Number, formatLength(title.Duration))
	for i, seg := range title.Segments {
		if ctx.Err() != nil || (wait != nil && !wait()) {
			return ctx.Err()
		}
		if err := tg.discSegmentFrames(ctx, path, title, i, seg, dir, busy); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, trickplayDone), []byte(strconv.Itoa(title.Number)), 0644); err != nil {
		return err
	}
	frames, _ := filepath.Glob(filepath.Join(dir, "frame-*.jpg"))
	fmt.Printf("Finished generating trickplay frames for %s (%d frames)\n", path, len(frames))
	return nil
}

// formatLength is a length for the log: "1h09m33s", "4m05s".
func formatLength(seconds float64) string {
	d := time.Duration(seconds) * time.Second
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm%02ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
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
		"-progress", "pipe:1", "-nostats", filepath.Join(tmp, "%d.jpg"))
	var stderr bytes.Buffer
	cmd := backgroundCommand(run, "ffmpeg", args...)
	cmd.Stderr = &stderr
	// A stuck DVD demuxer makes no frames: stop it when its position stops moving (watchdog.go).
	watch := watchStalls(cancel)
	defer watch.done()
	err = runWatched(cmd, watch)
	if watch.stalled() {
		return fmt.Errorf("disc frames: %w", ErrFFmpegStalled)
	}
	if err != nil {
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
		frame := filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", at))
		if os.Rename(filepath.Join(tmp, e.Name()), frame) == nil {
			addFrame(frame)
		}
	}
	return nil
}

// markNoFrames marks a video done without frames.
func markNoFrames(path string) error {
	if config.GlobalConfig.Media.TrickplayPath == "" {
		return fmt.Errorf("trickplay path is not configured")
	}
	dir := filepath.Join(config.GlobalConfig.Media.TrickplayPath, GenerateThumbnailHash(path))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, trickplayDone), nil, 0644)
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
