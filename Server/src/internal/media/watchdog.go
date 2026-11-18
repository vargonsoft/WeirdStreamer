package media

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ffmpeg's DVD demuxer can loop forever on some titles of some disc images, reading the same
// navigation data and making nothing. The disc's ffmpeg runs (transcodes, download encodes,
// trickplay) are watched, and one that makes no progress for ffmpegStallLimit is killed.

// ffmpegStallLimit is how long a disc's ffmpeg may go without progress. Reading a DVD makes
// output within seconds; a minute without any is a stuck demuxer. A variable for tests.
var ffmpegStallLimit = 60 * time.Second

// ErrFFmpegStalled: a disc's ffmpeg made no progress for ffmpegStallLimit and was stopped.
var ErrFFmpegStalled = errors.New("ffmpeg stopped making progress reading the disc (this title may be unreadable)")

// stallWatch calls stop once progress hasn't been reported for ffmpegStallLimit, except while
// paused (the caller has stopped taking output on purpose).
type stallWatch struct {
	mu     sync.Mutex
	last   time.Time
	paused bool
	fired  bool
	quit   chan struct{}
	once   sync.Once
}

// watchStalls starts watching; end with done.
func watchStalls(stop func()) *stallWatch {
	w := &stallWatch{last: time.Now(), quit: make(chan struct{})}
	limit := ffmpegStallLimit
	go func() {
		tick := time.NewTicker(max(limit/10, 10*time.Millisecond))
		defer tick.Stop()
		for {
			select {
			case <-w.quit:
				return
			case now := <-tick.C:
				w.mu.Lock()
				stalled := !w.paused && now.Sub(w.last) >= limit
				if stalled {
					w.fired = true
				}
				w.mu.Unlock()
				if stalled {
					stop()
					return
				}
			}
		}
	}()
	return w
}

// progress reports that ffmpeg moved on.
func (w *stallWatch) progress() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.last = time.Now()
	w.mu.Unlock()
}

// pause stops the clock (on) or starts it again from now (off).
func (w *stallWatch) pause(on bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.paused = on
	w.last = time.Now()
	w.mu.Unlock()
}

// stalled reports whether the watch stopped ffmpeg.
func (w *stallWatch) stalled() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fired
}

func (w *stallWatch) done() {
	if w != nil {
		w.once.Do(func() { close(w.quit) })
	}
}

// watchProgressLines reads ffmpeg's -progress output and reports progress to w each time its
// position (out_time_us) or frame count moves; each line also goes to line, if given. A stuck
// demuxer keeps printing progress blocks, with the same position.
func watchProgressLines(r io.Reader, w *stallWatch, line func(string)) {
	seen := map[string]string{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		if line != nil {
			line(text)
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok || (key != "out_time_us" && key != "frame") {
			continue
		}
		if seen[key] != value {
			seen[key] = value
			w.progress()
		}
	}
}

// runWatched runs cmd (with -progress pipe:1 in its arguments), reporting its progress to w.
func runWatched(cmd *exec.Cmd, w *stallWatch) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	watchProgressLines(stdout, w, nil)
	return cmd.Wait()
}

// isDiscArgs reports whether ffmpeg arguments read a DVD through the dvdvideo demuxer.
func isDiscArgs(args []string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" && args[i+1] == "dvdvideo" {
			return true
		}
	}
	return false
}
