package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// A disc image's subtitles go to the players beside the stream, as a file's DVD subtitles do
// (dvdsub.go): each track's subpictures as PGS, in the title's timeline (what the player shows:
// seconds from the title's start, the stream's offset added), drawn by the players over the
// transcode. The palette is the disc's (ffmpeg's dvdvideo demuxer gives it as .idx text), the
// canvas the title's picture and its pixels' shape the title's.
//
// The times must be the stream's. ffmpeg's dvdvideo demuxer lands a seek near its target and
// labels what follows as if it were the target: the stream started at 3000 s may show the picture
// of 3002 s as 3000 s (up to 15 s off on a feature). The labels depend only on the seek target, so
// the subtitles are read the same way: a reader per stream start (the player's from=) seeks where
// the stream did and reads on, every subtitle track at once, about 160 times faster than playback,
// keeping what it read for the windows the player asks for next. A title of chained program chains,
// or read from a later chapter, isn't sought (InputArgs trims it to the time), so its reader reads
// from the start in the title's own time, for every stream start.

// ServeDiscSubtitles writes subtitle track (among the title's) of a disc's title from start to
// start+duration (0: to the end) as a PGS .sup stream, for a stream that started at from;
// forcedOnly keeps only the forced lines.
func ServeDiscSubtitles(w http.ResponseWriter, r *http.Request, path string, title, angle, track int, from, start, duration float64, forcedOnly bool) error {
	sup, err := DiscSubtitlesAsPGS(r.Context(), path, title, angle, track, from, start, duration, forcedOnly)
	if err != nil {
		http.Error(w, "Could not read the subtitles", http.StatusUnprocessableEntity)
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, err = w.Write(sup)
	return err
}

// DiscSubtitlesAsPGS reads a disc title's subtitle track in the window and writes it as PGS.
func DiscSubtitlesAsPGS(ctx context.Context, path string, title, angle, track int, from, start, duration float64, forcedOnly bool) ([]byte, error) {
	info, err := LoadDisc(path)
	if err != nil {
		return nil, err
	}
	t := info.Title(title)
	if t == nil || track < 0 || track >= len(t.Subtitles) {
		return nil, errors.New("no such subtitle track")
	}
	angle = min(max(angle, 1), max(t.Angles, 1))
	look, err := discSubtitleLook(ctx, path, t, track)
	if err != nil {
		return nil, err
	}
	until := math.Inf(1)
	if duration > 0 {
		until = start + duration
	}
	packets, err := discReaderFor(path, t, angle, streamStart(t, from)).packetsUntil(ctx, track, until)
	if err != nil {
		return nil, err
	}
	return pgsFromPackets(packets, look, start, duration, forcedOnly), nil
}

// streamStart is where a stream asked to start at from was sought to (as DiscTranscodeArgs
// clamps it), or 0 for a title that isn't sought: the readers' key.
func streamStart(t *DiscTitle, from float64) float64 {
	if !seekable(t) {
		return 0
	}
	return min(max(from, 0), max(t.Duration-1, 0))
}

// seekable reports whether ffmpeg reads the title in one part from where it's sought to
// (InputArgs' -ss): its timeline is then the demuxer's labels.
func seekable(t *DiscTitle) bool {
	return len(t.Segments) == 1 && t.Segments[0].PGC == 0 && t.FirstChapter <= 1 && !t.Padding
}

// discSubtitleLook is a title's subtitle track's palette (from the demuxer's .idx text), with the
// title's picture as its canvas and the shape of its pixels (the display aspect over the stored one).
func discSubtitleLook(ctx context.Context, path string, t *DiscTitle, track int) (*dvdLook, error) {
	key := fmt.Sprintf("%s\x00%d\x00%d", path, t.Number, track)
	discLooks.Lock()
	look := discLooks.m[key]
	discLooks.Unlock()
	if look != nil {
		return look, nil
	}
	input, _ := t.InputArgs(path, 0, 1, 0)
	args := append([]string{"-v", "error"}, input...)
	args = append(args, "-select_streams", "s:"+strconv.Itoa(track), "-show_streams", "-show_data", "-of", "json")
	out, err := ffprobeOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	var probe struct {
		Streams []struct {
			Extradata string `json:"extradata"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(withoutDVDLibLines(out), &probe); err != nil {
		return nil, err
	}
	if len(probe.Streams) == 0 {
		return nil, errors.New("no such subtitle track")
	}
	look = &dvdLook{palette: defaultDVDPalette, width: 720, height: 480, aspect: 1}
	if t.Width > 0 && t.Height > 0 {
		look.width, look.height = t.Width, t.Height
		if t.Aspect > 0 {
			look.aspect = t.Aspect * float64(t.Height) / float64(t.Width)
		}
	}
	parseIdx(string(hexDump(probe.Streams[0].Extradata)), look)
	discLooks.Lock()
	discLooks.m[key] = look
	discLooks.Unlock()
	return look, nil
}

var discLooks = struct {
	sync.Mutex
	m map[string]*dvdLook
}{m: map[string]*dvdLook{}}

// discReader reads a title's subtitle tracks from a stream start on, in the stream's timeline.
type discReader struct {
	mu      sync.Mutex
	tracks  [][]dvdPacket
	reached float64 // read up to this time (the last packet's)
	done    bool    // to the title's end, or failed
	err     error
	changed chan struct{} // closed (and replaced) when more was read
	used    time.Time
	cancel  context.CancelFunc
}

// discReadersKept is how many readers stay (one per title, angle and stream start): the newest.
// A feature's tracks take a few MB each.
const discReadersKept = 4

// discReadTimeout bounds one reader (a feature reads in about 40 seconds over a network share).
const discReadTimeout = 10 * time.Minute

var discReaders = struct {
	sync.Mutex
	m map[string]*discReader
}{m: map[string]*discReader{}}

// discReaderFor is the reader for a title, angle and stream start, started on first use.
func discReaderFor(path string, t *DiscTitle, angle int, from float64) *discReader {
	key := fmt.Sprintf("%s\x00%d\x00%d\x00%.3f", path, t.Number, angle, from)
	discReaders.Lock()
	defer discReaders.Unlock()
	if r := discReaders.m[key]; r != nil {
		r.mu.Lock()
		failed := r.done && r.err != nil
		r.used = time.Now()
		r.mu.Unlock()
		if !failed { // a failed read is tried again
			return r
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), discReadTimeout)
	r := &discReader{tracks: make([][]dvdPacket, len(t.Subtitles)), changed: make(chan struct{}), used: time.Now(), cancel: cancel}
	discReaders.m[key] = r
	go r.read(ctx, path, t, angle, from)
	for len(discReaders.m) > discReadersKept { // drop the least recently used
		oldest := ""
		for k, o := range discReaders.m {
			if k != key && (oldest == "" || o.used.Before(discReaders.m[oldest].used)) {
				oldest = k
			}
		}
		discReaders.m[oldest].cancel()
		delete(discReaders.m, oldest)
	}
	return r
}

// packetsUntil waits until the reader has read past until (or to the end), then returns the
// track's packets read so far.
func (r *discReader) packetsUntil(ctx context.Context, track int, until float64) ([]dvdPacket, error) {
	for {
		r.mu.Lock()
		if r.done || r.reached >= until {
			packets, err := r.tracks[track], r.err
			r.mu.Unlock()
			if err != nil && len(packets) == 0 {
				return nil, err
			}
			return packets, nil
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (r *discReader) read(ctx context.Context, path string, t *DiscTitle, angle int, from float64) {
	defer r.cancel()
	err := func() error {
		if seekable(t) {
			input, _ := t.InputArgs(path, 0, angle, 0)
			return r.readInput(ctx, input, from, 0, false)
		}
		for i, seg := range t.Segments {
			input, _ := t.InputArgs(path, i, angle, 0)
			// Where this input's first picture is in the title: a chained part where it starts, a
			// title read from a later chapter where that chapter starts.
			base := seg.Start
			if seg.PGC == 0 && t.FirstChapter > 1 && t.FirstChapter <= len(t.Chapters) {
				base = t.Chapters[t.FirstChapter-1]
			}
			if err := r.readInput(ctx, input, 0, base, true); err != nil {
				return err
			}
		}
		return nil
	}()
	r.mu.Lock()
	r.done, r.err = true, err
	close(r.changed)
	r.changed = make(chan struct{})
	r.mu.Unlock()
}

// readInput reads one input's subtitle packets as ffprobe goes, from seek (0: its start). Times
// are the labels, or with relative base plus the time since the input's first picture.
func (r *discReader) readInput(ctx context.Context, input []string, seek, base float64, relative bool) error {
	// The input's subtitle streams in order, and where its first picture is.
	args := append([]string{"-v", "error"}, input...)
	out, err := ffprobeOutput(ctx, append(args, "-show_entries", "stream=index,codec_type,start_time", "-of", "json")...)
	if err != nil {
		return err
	}
	var streams struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecType string `json:"codec_type"`
			StartTime string `json:"start_time"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(withoutDVDLibLines(out), &streams); err != nil {
		return err
	}
	order, videoStart, videoSeen := map[int]int{}, 0.0, false
	for _, s := range streams.Streams {
		switch s.CodecType {
		case "video":
			if !videoSeen {
				videoSeen = true
				videoStart, _ = strconv.ParseFloat(s.StartTime, 64)
			}
		case "subtitle":
			order[s.Index] = len(order)
		}
	}
	if !relative {
		base, videoStart = 0, 0
	}

	args = append(args, "-select_streams", "s", "-show_entries", "packet=stream_index,pts_time,duration_time,data", "-show_data", "-of", "json")
	if seek > 0 {
		// With an end: ffprobe reads nothing for an open interval ("3000%") with -show_data.
		args = append(args, "-read_intervals", strconv.FormatFloat(seek, 'f', 3, 64)+"%+86400")
	}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	cmd.WaitDelay = 5 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The JSON as it comes: {"packets": [ {...}, ... ] }, each packet kept as soon as it's read.
	dec := json.NewDecoder(&libLineFilter{r: bufio.NewReader(stdout)})
	readErr := func() error {
		for {
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			if tok == "packets" {
				break
			}
		}
		if _, err := dec.Token(); err != nil { // [
			return err
		}
		for dec.More() {
			var p struct {
				Stream   int    `json:"stream_index"`
				PTS      string `json:"pts_time"`
				Duration string `json:"duration_time"`
				Data     string `json:"data"`
			}
			if err := dec.Decode(&p); err != nil {
				return err
			}
			k, ok := order[p.Stream]
			at, perr := strconv.ParseFloat(p.PTS, 64)
			if !ok || perr != nil || k >= len(r.tracks) {
				continue
			}
			d, _ := strconv.ParseFloat(p.Duration, 64)
			at = base + at - videoStart
			r.mu.Lock()
			r.tracks[k] = append(r.tracks[k], dvdPacket{time: at, duration: d, data: hexDump(p.Data)})
			r.reached = max(r.reached, at)
			close(r.changed)
			r.changed = make(chan struct{})
			r.mu.Unlock()
		}
		return nil
	}()
	waitErr := cmd.Wait()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	return waitErr
}

// libLineFilter drops the lines libdvdread and libdvdnav print into ffprobe's output (as
// withoutDVDLibLines does for a whole output), for reading it as it comes.
type libLineFilter struct {
	r    *bufio.Reader
	line []byte
}

func (f *libLineFilter) Read(p []byte) (int, error) {
	for len(f.line) == 0 {
		line, err := f.r.ReadBytes('\n')
		trimmed := bytes.TrimSpace(line)
		if !bytes.HasPrefix(trimmed, []byte("libdvdread:")) && !bytes.HasPrefix(trimmed, []byte("libdvdnav:")) {
			f.line = line
		}
		if err != nil {
			if len(f.line) == 0 {
				return 0, err
			}
			break
		}
	}
	n := copy(p, f.line)
	f.line = f.line[n:]
	return n, nil
}
