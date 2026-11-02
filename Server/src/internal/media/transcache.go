package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/database"
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
	return database.PathHash(viewer + "\x00" + filePath + "\x00" + strings.Join(args, "\x00"))
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
		c.dir = filepath.Join(os.TempDir(), "weirdstreamer-transcodes")
		if err := os.MkdirAll(c.dir, 0755); err != nil {
			return nil, err
		}
	}
	c.made++ // the same stream can be replayed while an older copy of it is still leaving
	s := newTranscodeSpool(filepath.Join(c.dir, id+"-"+strconv.Itoa(c.made)), filePath, sess)
	if err := s.start(args); err != nil {
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
	first, size := s.first, s.size
	s.changed.Broadcast()
	s.mu.Unlock()
	s.cancel()
	for n := first / spoolChunk; n <= size/spoolChunk; n++ {
		os.Remove(s.chunk(n))
	}
}

func (s *transcodeSpool) chunk(n int64) string {
	return s.prefix + "." + strconv.FormatInt(n, 10)
}

func (s *transcodeSpool) start(args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	cmd := exec.CommandContext(ctx, transcodeCommand, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}
	go s.logProgress(stderr)
	go func() {
		err := s.store(stdout)
		if waited := cmd.Wait(); err == nil {
			err = waited
		}
		fmt.Printf("Transcoding finished for: %s\n", s.filePath)
		s.mu.Lock()
		s.done, s.err = true, err
		s.changed.Broadcast()
		s.mu.Unlock()
	}()
	return nil
}

func (s *transcodeSpool) logProgress(stderr io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			fmt.Printf("FFmpeg: %s", string(buf[:n]))
			s.mu.Lock()
			sess := s.sess
			s.mu.Unlock()
			sess.FFmpegOutput(string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
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
	for s.room() {
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
