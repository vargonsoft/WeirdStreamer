package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// A transcode's output goes through chunks on disk instead of straight to the client, so a stream
// can be resumed. Browsers drop and reopen a media connection whenever they like, asking to go on
// from a byte: ffmpeg can't, and restarting it from the top sends other bytes, so the player
// stalled. Here a reconnect is served from the chunks, and ffmpeg outlives the connection.
const (
	spoolChunk  = int64(64 << 20)  // the size of a chunk
	spoolBehind = int64(512 << 20) // what's kept behind the furthest a client has read
	spoolAhead  = int64(96 << 20)  // how far ffmpeg runs ahead of it, then it waits
	spoolLinger = 60 * time.Second // how long a transcode waits for a client to come back
)

// transcodeCommand is what runs the transcode (tests replace it).
var transcodeCommand = "ffmpeg"

// errCannotResume: the client asked for bytes that are gone, or were never made here.
var errCannotResume = errors.New("this part of the stream isn't available")

type transcodeCache struct {
	mu     sync.Mutex
	dir    string
	spools map[*transcodeSpool]bool
	byID   map[string]*transcodeSpool
	made   int
	reaper bool
}

var transcodes = &transcodeCache{spools: map[*transcodeSpool]bool{}, byID: map[string]*transcodeSpool{}}

// SetTranscodeCacheDir sets where transcodes are kept while they play, clearing what a previous
// run left there.
func SetTranscodeCacheDir(dir string) error {
	transcodes.mu.Lock()
	defer transcodes.mu.Unlock()
	transcodes.dir = dir
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

// spoolName is a transcode's name in the cache, which says nothing of what's playing. Each viewer
// has their own: what's kept and how far ffmpeg runs follow one player's position, and two viewers
// of a file can be anywhere in it.
func spoolName(viewer, filePath string, args []string) string {
	return config.TextHash(viewer + "\x00" + config.CacheKey(filePath) + "\x00" + strings.Join(args, "\x00"))
}

// attach finds the transcode to serve offset from, starting one for offset 0 when there's none to
// join, and counts the caller as its reader: it must call detach.
func (c *transcodeCache) attach(filePath string, args []string, offset int64, sess *StreamSession) (*transcodeSpool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := spoolName(sess.viewer(), filePath, args)
	if s := c.byID[id]; s != nil && s.join(offset, sess) {
		return s, nil
	}
	if offset > 0 {
		return nil, errCannotResume
	}
	if c.dir == "" {
		c.dir = filepath.Join(TempDir(), "weirdstreamer-transcodes")
		if err := os.MkdirAll(c.dir, 0755); err != nil {
			return nil, err
		}
	}
	c.made++ // the same stream can be replayed while an older copy of it is still leaving
	s := newTranscodeSpool(filepath.Join(c.dir, id+"-"+strconv.Itoa(c.made)), filePath, sess)
	// The video encoded on the GPU Settings chose, while it has room, with the CPU's arguments to
	// fall back on. The name stays the CPU arguments', so reconnects find it either way.
	run, fallback := args, []string(nil)
	if hw, device, ok := hardwareChoice(); ok && encodesVideo(args) && c.hardwareRoom() {
		run, fallback = hardwareArgs(args, hw, device), args
		s.gpu = true
		sess.SetVideoEncoder(encoderLabel(run), run)
	}
	if err := s.start(run, fallback); err != nil {
		return nil, err
	}
	c.byID[id] = s
	c.spools[s] = true
	if !c.reaper {
		c.reaper = true
		go c.reap()
	}
	return s, nil
}

// hardwareRoom reports whether another transcode may use the GPU: under
// streaming.hardwareMaxTranscodes, and none failed to start on it in the last few minutes.
func (c *transcodeCache) hardwareRoom() bool {
	if since := time.Since(time.Unix(0, hardwareFailed.Load())); since < hardwareBackoff {
		return false
	}
	limit := 0
	if config.GlobalConfig != nil {
		limit = config.GlobalConfig.Streaming.HardwareMaxTranscodes
	}
	if limit <= 0 {
		return true
	}
	n := 0
	for s := range c.spools {
		s.mu.Lock()
		if s.gpu && !s.done && !s.closed {
			n++
		}
		s.mu.Unlock()
	}
	return n < limit
}

// hardwareFailed is when a transcode last failed to start on the GPU (UnixNano); for
// hardwareBackoff after it, transcodes go straight to the CPU.
var hardwareFailed atomic.Int64

const hardwareBackoff = 5 * time.Minute

// encodesVideo reports whether ffmpeg's arguments encode the video with x264 (not a copy).
func encodesVideo(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c:v" && args[i+1] == "libx264" {
			return true
		}
	}
	return false
}

// reap closes the transcodes no client came back to.
func (c *transcodeCache) reap() {
	for range time.Tick(spoolLinger / 6) {
		c.reapIdle(time.Now())
	}
}

func (c *transcodeCache) reapIdle(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for s := range c.spools {
		if s.idleSince(now) < spoolLinger {
			continue
		}
		s.close()
		delete(c.spools, s)
		for id, current := range c.byID {
			if current == s {
				delete(c.byID, id)
			}
		}
	}
}

// transcodeSpool is one ffmpeg's output, in chunks named prefix.0, prefix.1…
type transcodeSpool struct {
	prefix   string
	filePath string
	cancel   context.CancelFunc

	mu      sync.Mutex
	changed *sync.Cond
	size    int64 // bytes made so far
	first   int64 // the first byte still kept
	wanted  int64 // the furthest any client has read
	readers int
	idle    time.Time // when the last reader left
	done    bool      // ffmpeg finished
	err     error
	closed  bool
	sess    *StreamSession // the latest reader's, for ffmpeg's progress
	watch   *stallWatch    // a disc's transcode: stops ffmpeg when it makes nothing (nil for files)
	gpu     bool           // its video is encoded on the GPU
}

func newTranscodeSpool(prefix, filePath string, sess *StreamSession) *transcodeSpool {
	s := &transcodeSpool{prefix: prefix, filePath: filePath, readers: 1, sess: sess}
	s.changed = sync.NewCond(&s.mu)
	return s
}

// join counts in a reader starting at offset, if this transcode has that byte or will.
func (s *transcodeSpool) join(offset int64, sess *StreamSession) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || offset < s.first || (s.done && offset >= s.size && offset > 0) || (s.done && s.err != nil) {
		return false
	}
	s.readers++
	s.sess = sess
	s.changed.Broadcast()
	return true
}

func (s *transcodeSpool) detach() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readers--
	s.idle = time.Now()
	s.changed.Broadcast()
}

func (s *transcodeSpool) idleSince(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readers > 0 {
		return 0
	}
	return now.Sub(s.idle)
}

func (s *transcodeSpool) close() {
	s.mu.Lock()
	s.closed = true
	first, size, cancel := s.first, s.size, s.cancel
	s.changed.Broadcast()
	s.mu.Unlock()
	cancel()
	for n := first / spoolChunk; n <= size/spoolChunk; n++ {
		os.Remove(s.chunk(n))
	}
}

func (s *transcodeSpool) chunk(n int64) string {
	return s.prefix + "." + strconv.FormatInt(n, 10)
}

// start runs ffmpeg with args. If it fails before making anything and there's a fallback (the
// CPU's arguments for a GPU transcode), it runs again with that.
func (s *transcodeSpool) start(args, fallback []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	cmd := exec.CommandContext(ctx, transcodeCommand, args...)
	stdout, stderr, err := transcodePipes(cmd)
	if err != nil {
		cancel()
		return err
	}
	if isDiscArgs(args) {
		// Progress is output: the watch runs while store waits on ffmpeg, not while it holds
		// ffmpeg back (far enough ahead of the player, or no player).
		s.watch = watchStalls(cancel)
	}
	started := time.Now()
	s.mu.Lock()
	sess := s.sess
	s.mu.Unlock()
	log.Printf("[TRANSCODE] Started for %s: %s (ffmpeg %s)", sess.Who(), s.filePath, strings.Join(args, " "))
	tail := make(chan []string, 1)
	go func() { tail <- s.readOutput(stderr) }()
	go func() {
		err := s.store(stdout)
		waited := cmd.Wait()
		if err == nil {
			err = waited
		}
		s.watch.done()
		if s.watch.stalled() {
			err = ErrFFmpegStalled
		}
		s.mu.Lock()
		retry := fallback != nil && err != nil && s.size == 0 && !s.closed && !shuttingDown.Load()
		if retry {
			s.gpu = false
			sess := s.sess
			s.mu.Unlock()
			lines := <-tail
			stdout.Close()
			stderr.Close()
			hardwareFailed.Store(time.Now().UnixNano())
			log.Printf("[TRANSCODE] The GPU encoder failed (%v), so the CPU encodes instead for %s: %s", err, hardwareBackoff, s.filePath)
			for _, line := range lines {
				log.Printf("[TRANSCODE]   ffmpeg: %s", line)
			}
			sess.SetVideoEncoder("libx264 on the CPU: the GPU failed", fallback)
			if err := s.start(fallback, nil); err != nil {
				s.mu.Lock()
				s.done, s.err = true, err
				s.changed.Broadcast()
				s.mu.Unlock()
			}
			return
		}
		s.done, s.err = true, err
		closed, size := s.closed, s.size
		s.changed.Broadcast()
		s.mu.Unlock()
		lines := <-tail
		stdout.Close()
		stderr.Close()
		took := time.Since(started).Round(time.Second)
		if err == nil && !closed && emptyOutput(lines) {
			// ffmpeg exits 0 when its input gave it nothing to encode.
			err = errors.New("ffmpeg read no video or audio")
		}
		switch {
		case shuttingDown.Load():
			log.Printf("[TRANSCODE] Stopped by the server's shutdown after %s (%s made): %s", took, sizeText(size), s.filePath)
		case closed:
			log.Printf("[TRANSCODE] Stopped after %s (%s made; no player came back for it): %s", took, sizeText(size), s.filePath)
		case err == nil:
			log.Printf("[TRANSCODE] Finished in %s (%s): %s", took, sizeText(size), s.filePath)
		default:
			log.Printf("[TRANSCODE] Failed after %s (%s made): %s: %v", took, sizeText(size), s.filePath, err)
			for _, line := range lines {
				log.Printf("[TRANSCODE]   ffmpeg: %s", line)
			}
		}
	}()
	return nil
}

// transcodePipes starts cmd with a pipe for its output and one for its messages. ffmpeg's output
// to stdout ("pipe:1") goes to its own pipe (fd 3) instead: libraries ffmpeg uses print to stdout
// too (libdvdread, on a damaged navigation packet), which landed in the middle of a stream. What
// they print joins ffmpeg's messages. The caller closes both readers when ffmpeg is done.
func transcodePipes(cmd *exec.Cmd) (output, messages io.ReadCloser, err error) {
	if n := len(cmd.Args); n < 2 || cmd.Args[n-1] != "pipe:1" {
		if output, err = cmd.StdoutPipe(); err != nil {
			return nil, nil, fmt.Errorf("failed to create stdout pipe: %w", err)
		}
		if messages, err = cmd.StderrPipe(); err != nil {
			return nil, nil, fmt.Errorf("failed to create stderr pipe: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
		}
		return output, messages, nil
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create the output pipe: %w", err)
	}
	msgR, msgW, err := os.Pipe()
	if err != nil {
		outR.Close()
		outW.Close()
		return nil, nil, fmt.Errorf("failed to create the message pipe: %w", err)
	}
	cmd.Args[len(cmd.Args)-1] = "pipe:3"
	cmd.ExtraFiles = []*os.File{outW}
	cmd.Stdout, cmd.Stderr = msgW, msgW
	err = cmd.Start()
	// ffmpeg has its own copies of the write ends: closing these lets the readers see its end.
	outW.Close()
	msgW.Close()
	if err != nil {
		outR.Close()
		msgR.Close()
		return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	return outR, msgR, nil
}

// outputTail is how many of ffmpeg's last lines a failed transcode logs.
const outputTail = 15

// readOutput passes ffmpeg's messages to the stream's session (its progress, for the Streams page)
// and returns its last lines, without the progress ones, for the log if it fails.
// emptyOutput reports whether ffmpeg's last lines say it encoded nothing (its input ended before
// a frame: a DVD title it can't read past the start of), which it doesn't count as an error.
func emptyOutput(lines []string) bool {
	for _, line := range lines {
		if strings.Contains(line, "Output file is empty, nothing was encoded") {
			return true
		}
	}
	return false
}

func (s *transcodeSpool) readOutput(stderr io.Reader) []string {
	var lines []string
	var partial string
	buf := make([]byte, 4096)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			s.mu.Lock()
			sess := s.sess
			s.mu.Unlock()
			sess.FFmpegOutput(chunk)
			// Progress lines end in \r, the rest in \n.
			parts := strings.FieldsFunc(partial+chunk, func(r rune) bool { return r == '\r' || r == '\n' })
			partial = ""
			if last := chunk[len(chunk)-1]; last != '\r' && last != '\n' && len(parts) > 0 {
				partial, parts = parts[len(parts)-1], parts[:len(parts)-1]
			}
			for _, line := range parts {
				if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "frame=") || strings.HasPrefix(line, "size=") {
					continue
				}
				if lines = append(lines, line); len(lines) > outputTail {
					lines = lines[1:]
				}
			}
		}
		if err != nil {
			if partial = strings.TrimSpace(partial); partial != "" && !strings.HasPrefix(partial, "frame=") && !strings.HasPrefix(partial, "size=") {
				lines = append(lines, partial)
			}
			return lines
		}
	}
}

func sizeText(n int64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}

// room waits until ffmpeg's output is wanted: while no client is attached, or it's far enough
// ahead of them, it isn't read, and ffmpeg waits on its pipe. False once the transcode is closed.
func (s *transcodeSpool) room() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for !s.closed && (s.readers == 0 || s.size >= s.wanted+spoolAhead) {
		s.changed.Wait()
	}
	return !s.closed
}

// store writes ffmpeg's output to the chunks, dropping those far behind what's been read.
func (s *transcodeSpool) store(stdout io.Reader) error {
	buf := make([]byte, 256<<10)
	var file *os.File
	var at int64
	defer func() {
		if file != nil {
			file.Close()
		}
	}()
	for {
		s.watch.pause(true)
		more := s.room()
		s.watch.pause(false)
		if !more {
			break
		}
		// Up to the chunk's end: at most what's left of it goes to this file.
		room := spoolChunk - at%spoolChunk
		n, err := stdout.Read(buf[:min(int64(len(buf)), room)])
		if n > 0 {
			if file == nil {
				if file, err = os.Create(s.chunk(at / spoolChunk)); err != nil {
					return err
				}
			}
			if _, werr := file.Write(buf[:n]); werr != nil {
				return werr
			}
			at += int64(n)
			if at%spoolChunk == 0 {
				file.Close()
				file = nil
			}
			s.stored(at)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *transcodeSpool) stored(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = size
	keep := s.wanted - spoolBehind
	for s.first+spoolChunk <= keep {
		os.Remove(s.chunk(s.first / spoolChunk))
		s.first += spoolChunk
	}
	s.changed.Broadcast()
}

// ready waits until the byte at offset is made, or the transcode ends, and returns the
// transcode's error if it ended without making it: the response's status waits for that, so a
// transcode that fails before making anything is answered with an error.
func (s *transcodeSpool) ready(ctx context.Context, offset int64) error {
	stop := context.AfterFunc(ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.changed.Broadcast()
	})
	defer stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	for offset >= s.size && !s.done && !s.closed && ctx.Err() == nil {
		s.changed.Wait()
	}
	if offset >= s.size && s.done && s.err != nil {
		return s.err
	}
	return nil
}

// read reads from offset, waiting for bytes ffmpeg hasn't made yet. io.EOF at the stream's end.
func (s *transcodeSpool) read(ctx context.Context, offset int64, p []byte) (int, error) {
	s.mu.Lock()
	if offset > s.wanted {
		s.wanted = offset
		s.changed.Broadcast()
	}
	for offset >= s.size && !s.done && !s.closed && ctx.Err() == nil {
		s.changed.Wait()
	}
	size, first, done, err := s.size, s.first, s.done, s.err
	closed := s.closed
	s.mu.Unlock()
	switch {
	case ctx.Err() != nil:
		return 0, ctx.Err()
	case offset < first || (closed && offset < size):
		return 0, errCannotResume
	case offset >= size && done && err != nil:
		return 0, err
	case offset >= size:
		return 0, io.EOF
	}
	within := offset % spoolChunk
	length := min(int64(len(p)), size-offset, spoolChunk-within)
	file, oerr := os.Open(s.chunk(offset / spoolChunk))
	if oerr != nil {
		return 0, errCannotResume
	}
	defer file.Close()
	return file.ReadAt(p[:length], within)
}

// streamFrom sends the stream from offset until it ends or the client leaves.
func (s *transcodeSpool) streamFrom(ctx context.Context, w io.Writer, offset int64) error {
	// Wakes the wait in read when the client leaves.
	stop := context.AfterFunc(ctx, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.changed.Broadcast()
	})
	defer stop()
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	for {
		n, err := s.read(ctx, offset, buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			offset += int64(n)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// transcodeOffset is where a request asks to start ("bytes=N-"); a transcode's length isn't known,
// so nothing else can be asked of it.
func transcodeOffset(r *http.Request) int64 {
	spec, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes=")
	if !ok {
		return 0
	}
	first, _, _ := strings.Cut(spec, "-")
	offset, err := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}
