package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// DVD-Video disc images (.iso) are played with ffmpeg's dvdvideo demuxer. It plays one program
// chain (PGC) at a time, while a title can be several chained PGCs, so the disc's own tables
// (its IFO files) are read to know each title's chain; ffprobe supplies chapters and tracks.

// IsDiscImage reports whether a file is a disc played through the DVD reader: an image (.iso),
// or an unpacked DVD's VIDEO_TS folder (DVDFolder), which ffmpeg's dvdvideo opens the same way.
func IsDiscImage(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".iso") || config.IsDVDFolder(path)
}

// DVDFolder says whether path is a VIDEO_TS folder holding a DVD (its VIDEO_TS.IFO): listed and
// played as one video, named after the folder it's in, its VOB, IFO and BUP files never on their own.
func DVDFolder(path string) bool {
	if !config.IsDVDFolder(path) {
		return false
	}
	_, err := findInFolder(path, "VIDEO_TS.IFO")
	return err == nil
}

// DiscName is the name a disc is listed by: an image's own, and a VIDEO_TS folder's parent's
// (the folder it was unpacked into, as "Frivolous_Lola").
func DiscName(path string) string {
	if config.IsDVDFolder(path) {
		return filepath.Base(filepath.Dir(path))
	}
	return filepath.Base(path)
}

// DiscSize is a disc's size: an image's, or the files in a VIDEO_TS folder together.
func DiscSize(path string, info os.FileInfo) int64 {
	if info == nil || !info.IsDir() {
		if info != nil {
			return info.Size()
		}
		return 0
	}
	var total int64
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && !e.IsDir() {
			total += fi.Size()
		}
	}
	return total
}

// findInFolder is the path of a file in dir by its name, in any case (discs' files are VIDEO_TS.IFO
// or video_ts.ifo, as copied).
func findInFolder(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) && !e.IsDir() {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, errNotOnDisc)
}

// dvdFiles reads a disc's files by their path on the disc ("VIDEO_TS", "VTS_01_0.IFO"): from an ISO
// image, or from an unpacked VIDEO_TS folder.
type dvdFiles interface {
	readFile(parts ...string) ([]byte, error)
}

// folderDisc is a VIDEO_TS folder on disk, as dvdFiles.
type folderDisc string

func (d folderDisc) readFile(parts ...string) ([]byte, error) {
	if len(parts) == 0 {
		return nil, errNotOnDisc
	}
	file, err := findInFolder(string(d), parts[len(parts)-1])
	if err != nil {
		return nil, err
	}
	return os.ReadFile(file)
}

// discInfoVersion changes when probing changes, so discs stored by older builds are probed again.
const discInfoVersion = 6

// DiscInfo describes what a DVD disc image holds.
type DiscInfo struct {
	Titles    []DiscTitle `json:"titles"`
	MainTitle int         `json:"main_title"` // number of the main feature: the longest title
	Version   int         `json:"version"`
	// Unreadable lists the titles ffprobe got stuck on (killed after discProbeTimeout). They're
	// saved with the rest and not probed again while the disc's size and time stay the same.
	Unreadable []UnreadableTitle `json:"unreadable,omitempty"`
	// NextTitle: probing stopped at this title when the disc's time (discProbeBudget) ran out; it
	// and the titles after it weren't probed. The next start goes on from there. 0 when done.
	NextTitle int `json:"next_title,omitempty"`
	// MainTitleUncertain: a title that couldn't be read (or wasn't probed yet) may be longer than
	// MainTitle, so the main title may be a bonus feature. Trickplay, thumbnails and downloads say so.
	MainTitleUncertain bool `json:"main_title_uncertain,omitempty"`
}

// UnreadableTitle is a title ffprobe couldn't read: its length from the disc's tables (0 when
// they can't be read) and why.
type UnreadableTitle struct {
	Number   int     `json:"number"`
	Duration float64 `json:"duration,omitempty"`
	Reason   string  `json:"reason"`
}

// discProbeTimeout bounds each ffprobe call on a disc. A title's probe takes seconds; one that
// runs longer is stuck (ffmpeg's DVD demuxer loops forever on some titles of some images). The
// title is then saved as unreadable, so this is paid once per disc. A variable so tests can
// shorten it.
var discProbeTimeout = 60 * time.Second

// discThumbnailTimeout bounds making a disc's thumbnail: one frame, so ffmpeg shows no progress
// before it's done (an accurate seek into a chained title decodes up to it first).
var discThumbnailTimeout = 3 * time.Minute

// discProbeBudget bounds probing one disc in all, so a disc with several stuck titles doesn't
// hold up a scan for long: four stuck titles (a minute each) and the rest still fit. Titles not
// reached are probed at the next start (DiscInfo.NextTitle).
var discProbeBudget = 5 * time.Minute

// errProbeTimeout: an ffprobe call on a disc ran past discProbeTimeout and was killed.
var errProbeTimeout = errors.New("ffprobe timed out reading the title")

// errProbeBudget: an ffprobe call was killed because the disc's probe budget ran out.
var errProbeBudget = errors.New("the disc's probe time ran out")

// ErrDiscReading: a request didn't wait for a disc still being probed (LoadDiscWithin).
var ErrDiscReading = errors.New("still reading this disc; try again in a minute")

// DiscRequestWait is how long a request waits for a disc being probed before giving up with
// ErrDiscReading (the probe goes on, and its result is kept). Below the time players give up
// on a request.
var DiscRequestWait = 20 * time.Second

// ffprobeOutput runs ffprobe for at most discProbeTimeout, and no longer than ctx allows.
func ffprobeOutput(ctx context.Context, args ...string) ([]byte, error) {
	return ffprobeOutputWithin(ctx, discProbeTimeout, args...)
}

// ffprobeOutputWithin runs ffprobe for at most timeout, and no longer than ctx allows.
func ffprobeOutputWithin(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(call, "ffprobe", args...)
	// Once killed, don't wait on its output for longer than this (a child it started could keep
	// the pipe open).
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.Output()
	if err != nil && killedAtDeadline(cmd, call) {
		if ctx.Err() != nil {
			return nil, errProbeBudget
		}
		return nil, errProbeTimeout
	}
	return out, err
}

// killedAtDeadline reports whether cmd was killed by exec for its context: it ended by a signal,
// not by exiting, and its context is done. One that exited just as the deadline passed isn't.
func killedAtDeadline(cmd *exec.Cmd, ctx context.Context) bool {
	ps := cmd.ProcessState
	return ps != nil && !ps.Exited() && ctx.Err() != nil
}

// DiscTitle is one title (the feature, a bonus feature, an episode...).
type DiscTitle struct {
	Number    int           `json:"number"`
	Duration  float64       `json:"duration"`
	Angles    int           `json:"angles"`
	Chapters  []float64     `json:"chapters"` // start times, seconds
	Audio     []DiscTrack   `json:"audio"`
	Subtitles []DiscTrack   `json:"subtitles"`
	Width     int           `json:"width,omitempty"`
	Height    int           `json:"height,omitempty"`
	Aspect    float64       `json:"aspect,omitempty"` // display aspect ratio (4:3, 16:9)
	Segments  []DiscSegment `json:"segments"`         // the program chains played one after another
	// FirstChapter (from 1) is where ffmpeg reads a title from when it can't read past the title's
	// opening (a short padding cell that ends the title for it); 0 for the start.
	FirstChapter int `json:"first_chapter,omitempty"`
	// Padding marks a title that's nothing but padding cells (a moment of black between menus):
	// ffmpeg reads it only with its trimming off (-trim 0), and it plays as it is.
	Padding bool `json:"padding,omitempty"`
}

// DiscTrack is an audio or subtitle track; Index counts tracks of its kind from 0.
type DiscTrack struct {
	Index    int    `json:"index"`
	Language string `json:"language,omitempty"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels,omitempty"`
}

// DiscSegment is one program chain of a title. PGC 0 means "the title as ffmpeg plays it" (used
// when the disc's tables can't be read).
type DiscSegment struct {
	PGC      int     `json:"pgc"`
	PG       int     `json:"pg"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// Title returns title n, or the main title for n <= 0 or an unknown n.
func (d *DiscInfo) Title(n int) *DiscTitle {
	if n <= 0 {
		n = d.MainTitle
	}
	for i := range d.Titles {
		if d.Titles[i].Number == n {
			return &d.Titles[i]
		}
	}
	for i := range d.Titles {
		if d.Titles[i].Number == d.MainTitle {
			return &d.Titles[i]
		}
	}
	if len(d.Titles) > 0 {
		return &d.Titles[0]
	}
	return nil
}

// SegmentAt returns the index of the segment playing at time t and how far into it t is.
func (t *DiscTitle) SegmentAt(at float64) (int, float64) {
	for i := len(t.Segments) - 1; i >= 0; i-- {
		if at >= t.Segments[i].Start {
			return i, at - t.Segments[i].Start
		}
	}
	return 0, 0
}

// InputArgs are the ffmpeg input options (with -i) that play segment i of the title at an angle,
// starting seek seconds in. Seeking works in ffmpeg itself for whole titles; within a chained
// PGC it doesn't, so the returned trim is how much the caller must cut after decoding.
func (t *DiscTitle) InputArgs(path string, i, angle int, seek float64) (args []string, trim float64) {
	args = []string{"-f", "dvdvideo", "-title", strconv.Itoa(t.Number)}
	if t.Padding {
		args = append(args, "-trim", "0")
	}
	seg := t.Segments[i]
	if seg.PGC > 0 {
		args = append(args, "-pgc", strconv.Itoa(seg.PGC), "-pg", strconv.Itoa(max(seg.PG, 1)))
		trim = seek
	} else if t.FirstChapter > 1 && t.FirstChapter <= len(t.Chapters) {
		// Read from the chapter the seek lands in (never before FirstChapter), and cut the rest.
		c := t.FirstChapter
		for c < len(t.Chapters) && t.Chapters[c] <= seek {
			c++
		}
		args = append(args, "-chapter_start", strconv.Itoa(c))
		trim = max(seek-t.Chapters[c-1], 0)
	} else if seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(seek, 'f', 3, 64))
	}
	if angle > 1 && angle <= t.Angles {
		args = append(args, "-angle", strconv.Itoa(angle))
	}
	return append(args, "-i", path), trim
}

// DiscStore persists probed disc info between restarts (the database); optional. Discs are kept
// by client path (config.ClientPath); one outside every library isn't kept.
type DiscStore interface {
	GetDiscInfo(path string, size, modTime int64) ([]byte, error)
	SaveDiscInfo(path string, size, modTime int64, info []byte) error
}

var (
	discStore DiscStore
	discMu    sync.Mutex
	discCache = map[string]*discEntry{}
)

type discEntry struct {
	size, modTime int64
	once          sync.Once
	done          chan struct{} // closed when info and err are set
	info          *DiscInfo
	err           error
}

// SetDiscStore sets where probed disc info is kept.
func SetDiscStore(s DiscStore) { discStore = s }

// LoadDisc returns a disc image's info, probing it the first time (seconds, up to discProbeBudget)
// and caching it in memory and in the DiscStore. For background work; requests use LoadDiscWithin.
func LoadDisc(path string) (*DiscInfo, error) {
	e, err := discEntryFor(path)
	if err != nil {
		return nil, err
	}
	<-e.done
	return e.info, e.err
}

// LoadDiscWithin is LoadDisc waiting at most wait for a probe (one already running, or started
// here), then answering ErrDiscReading while the probe goes on. wait 0 takes only what's known.
func LoadDiscWithin(path string, wait time.Duration) (*DiscInfo, error) {
	e, err := discEntryFor(path)
	if err != nil {
		return nil, err
	}
	select {
	case <-e.done:
		return e.info, e.err
	default:
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-e.done:
		return e.info, e.err
	case <-timer.C:
		return nil, ErrDiscReading
	}
}

// discEntryFor returns the disc's cache entry, its probe started.
func discEntryFor(path string) (*discEntry, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size, modTime := st.Size(), st.ModTime().Unix()
	discMu.Lock()
	e := discCache[path]
	if e == nil || e.size != size || e.modTime != modTime {
		e = &discEntry{size: size, modTime: modTime, done: make(chan struct{})}
		discCache[path] = e
	}
	discMu.Unlock()
	e.once.Do(func() {
		go func() {
			defer close(e.done)
			e.info, e.err = e.load(path)
		}()
	})
	return e, nil
}

// load reads the disc's stored info, or probes it (going on from where a stored probe stopped),
// and stores what was probed.
func (e *discEntry) load(path string) (*DiscInfo, error) {
	var prev *DiscInfo
	key, stored := config.ClientPath(path)
	stored = stored && discStore != nil
	if stored {
		if data, err := discStore.GetDiscInfo(key, e.size, e.modTime); err == nil && data != nil {
			var info DiscInfo
			if json.Unmarshal(data, &info) == nil && info.Version == discInfoVersion &&
				(len(info.Titles) > 0 || len(info.Unreadable) > 0) {
				if info.NextTitle == 0 {
					return &info, info.readableErr()
				}
				prev = &info
			}
		}
	}
	info, err := probeDisc(path, prev)
	if info != nil && stored {
		if data, jerr := json.Marshal(info); jerr == nil {
			discStore.SaveDiscInfo(key, e.size, e.modTime, data)
		}
	}
	return info, err
}

// readableErr is the error for a disc none of whose titles could be read (all stuck).
func (d *DiscInfo) readableErr() error {
	if len(d.Titles) > 0 {
		return nil
	}
	if d.NextTitle > 0 {
		return fmt.Errorf("no titles read yet (%w)", errProbeBudget)
	}
	return errors.New("no readable titles on the disc: ffprobe got stuck on every one")
}

// probeDisc reads a disc's title chains from its IFO tables and each title's chapters and tracks
// with ffprobe, within discProbeBudget. prev, when given, is a probe stopped by the budget: its
// titles are kept and probing goes on from its NextTitle. The info is returned (to be saved)
// even with an error when titles were found stuck, so they aren't probed again.
func probeDisc(path string, prev *DiscInfo) (*DiscInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discProbeBudget)
	defer cancel()
	tables, tablesErr := readDVDTables(path)
	count := 99
	if tablesErr == nil {
		count = len(tables)
	}
	info := &DiscInfo{}
	first := 1
	if prev != nil {
		info.Titles, info.Unreadable = prev.Titles, prev.Unreadable
		first = prev.NextTitle
	}
	stuck := func(n int, err error) {
		u := UnreadableTitle{Number: n, Reason: fmt.Sprintf("%v (after %s)", err, discProbeTimeout)}
		if tablesErr == nil {
			if segs := tables[n-1].segments(); validChain(segs) {
				u.Duration = segs[len(segs)-1].Start + segs[len(segs)-1].Duration
			}
		}
		info.Unreadable = append(info.Unreadable, u)
		log.Printf("[DISC] Title %d of %s can't be read, skipped: %v", n, path, err)
	}
	for n := first; n <= count; n++ {
		if ctx.Err() != nil {
			info.NextTitle = n
			break
		}
		t, err := probeTitle(ctx, path, n, false)
		padding := false
		if err != nil && !errors.Is(err, errProbeTimeout) && !errors.Is(err, errProbeBudget) {
			// ffmpeg refuses a title of nothing but padding cells unless told not to trim them.
			t, err = probeTitle(ctx, path, n, true)
			padding = err == nil
		}
		if err == nil && !padding {
			var readable bool
			readable, err = readsPacket(ctx, path, n, 1)
			if err == nil && !readable {
				t.FirstChapter, err = firstReadableChapter(ctx, path, n, len(t.Chapters))
				// Nothing read from any chapter: what ffmpeg trims is all there is.
				padding = t.FirstChapter == 0
			}
		}
		if errors.Is(err, errProbeBudget) {
			info.NextTitle = n
			break
		}
		if errors.Is(err, errProbeTimeout) {
			// Skip this title only: the titles after it may probe fine (a title can hang while
			// the ones around it work).
			stuck(n, err)
			continue
		}
		if err != nil {
			if tablesErr != nil {
				break // without the tables, titles are probed until one is missing
			}
			continue
		}
		t.Angles = 1
		t.Segments = []DiscSegment{{Start: 0, Duration: t.Duration}}
		t.Padding = padding
		if tablesErr == nil {
			tt := tables[n-1]
			t.Angles = max(tt.angles, 1)
			// The disc's own tables say what a title plays: ffmpeg's probe stops at the end of a
			// title's first PGC, so it can report a whole feature as a few seconds.
			// A chain whose chapters run backwards hops between PGCs out of order (short
			// montage titles): those are left to ffmpeg.
			if segs := tt.segments(); validChain(segs) && ascending(tt.chapters(segs)) {
				if chapters := tt.chapters(segs); len(chapters) > 0 {
					t.Chapters = chapters
				}
				if len(segs) > 1 {
					t.Segments = segs
					t.Duration = segs[len(segs)-1].Start + segs[len(segs)-1].Duration
				}
			}
		}
		info.Titles = append(info.Titles, *t)
	}
	if info.NextTitle > 0 {
		log.Printf("[DISC] Probing %s stopped at title %d after %s; the rest at the next start", path, info.NextTitle, discProbeBudget)
	}
	info.Version = discInfoVersion
	if len(info.Titles) == 0 {
		if len(info.Unreadable) > 0 || info.NextTitle > 0 {
			return info, info.readableErr()
		}
		if tablesErr != nil {
			return nil, fmt.Errorf("not a readable DVD image: %w", tablesErr)
		}
		return nil, errors.New("no playable titles on the disc")
	}
	info.MainTitle = mainTitle(info.Titles)
	info.MainTitleUncertain = mainTitleUncertain(info)
	return info, nil
}

// mainTitleUncertain reports whether a title that wasn't read could be longer than the main
// title: an unreadable one longer than it or of unknown length, or titles not probed yet.
func mainTitleUncertain(info *DiscInfo) bool {
	if info.NextTitle > 0 {
		return true
	}
	main := info.Title(info.MainTitle)
	for _, u := range info.Unreadable {
		if u.Duration == 0 || main == nil || u.Duration > main.Duration {
			return true
		}
	}
	return false
}

// firstReadableChapter finds the first of a title's next few chapters (from 2) that ffmpeg reads a
// packet of, or 0. Some discs open a title with a half-second padding cell: ffmpeg skips it, then
// finds the title over and reads nothing, but it reads the title from the chapter after.
func firstReadableChapter(ctx context.Context, path string, title, chapters int) (int, error) {
	for c := 2; c <= min(chapters, 4); c++ {
		readable, err := readsPacket(ctx, path, title, c)
		if err != nil {
			return 0, err
		}
		if readable {
			return c, nil
		}
	}
	return 0, nil
}

// readsPacket reports whether ffmpeg reads a packet of a title, from a chapter (from 1).
func readsPacket(ctx context.Context, path string, title, chapter int) (bool, error) {
	args := []string{"-v", "error", "-f", "dvdvideo", "-title", strconv.Itoa(title)}
	if chapter > 1 {
		args = append(args, "-chapter_start", strconv.Itoa(chapter))
	}
	args = append(args, "-i", path, "-read_intervals", "%+#1",
		"-show_entries", "packet=pts_time", "-of", "csv=p=0")
	out, err := ffprobeOutput(ctx, args...)
	if errors.Is(err, errProbeTimeout) || errors.Is(err, errProbeBudget) {
		return false, err
	}
	return err == nil && len(bytes.TrimSpace(withoutDVDLibLines(out))) > 0, nil
}

func ascending(times []float64) bool {
	for i := 1; i < len(times); i++ {
		if times[i] < times[i-1] {
			return false
		}
	}
	return true
}

// validChain reports whether a title's PGC chain from the disc's tables can be played: it has
// PGCs, all with a length.
func validChain(segs []DiscSegment) bool {
	if len(segs) == 0 {
		return false
	}
	for _, s := range segs {
		if s.Duration <= 0 {
			return false
		}
	}
	return true
}

// mainTitle picks the main feature: the longest title. Among titles about as long (an alternate
// cut, or the feature with a commentary or isolated score), the one with the most audio tracks,
// then chapters, then the lowest number.
func mainTitle(titles []DiscTitle) int {
	longest := 0.0
	for _, t := range titles {
		longest = max(longest, t.Duration)
	}
	best := -1
	for i, t := range titles {
		if t.Duration < longest*0.99 {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		b := titles[best]
		switch {
		case len(t.Audio) != len(b.Audio):
			if len(t.Audio) > len(b.Audio) {
				best = i
			}
		case len(t.Chapters) != len(b.Chapters):
			if len(t.Chapters) > len(b.Chapters) {
				best = i
			}
		case t.Number < b.Number:
			best = i
		}
	}
	return titles[best].Number
}

// withoutDVDLibLines drops the lines libdvdread and libdvdnav print to stdout (a damaged
// navigation packet: "libdvdread: CHECK_VALUE failed in nav_read.c..."), which land in the middle
// of ffprobe's JSON. A disc with one such packet read as having no titles.
func withoutDVDLibLines(out []byte) []byte {
	if !bytes.Contains(out, []byte("libdvd")) {
		return out
	}
	lines := bytes.SplitAfter(out, []byte("\n"))
	kept := lines[:0]
	for _, line := range lines {
		if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("libdvdread:")) && !bytes.HasPrefix(bytes.TrimSpace(line), []byte("libdvdnav:")) {
			kept = append(kept, line)
		}
	}
	return bytes.Join(kept, nil)
}

// probeTitle reads title n's length, chapters and tracks; untrimmed reads its padding cells too.
func probeTitle(ctx context.Context, path string, n int, untrimmed bool) (*DiscTitle, error) {
	args := []string{"-v", "error", "-f", "dvdvideo", "-title", strconv.Itoa(n)}
	if untrimmed {
		args = append(args, "-trim", "0")
	}
	args = append(args, "-i", path, "-show_format", "-show_chapters", "-show_streams", "-of", "json")
	out, err := ffprobeOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Chapters []struct {
			StartTime string `json:"start_time"`
		} `json:"chapters"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
			Channels  int    `json:"channels"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Aspect    string `json:"display_aspect_ratio"`
			Tags      struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(withoutDVDLibLines(out), &probe); err != nil {
		return nil, err
	}
	t := &DiscTitle{Number: n, Chapters: []float64{}, Audio: []DiscTrack{}, Subtitles: []DiscTrack{}}
	t.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if t.Duration <= 0 {
		return nil, errors.New("empty title")
	}
	for _, c := range probe.Chapters {
		if start, err := strconv.ParseFloat(c.StartTime, 64); err == nil {
			t.Chapters = append(t.Chapters, start)
		}
	}
	for _, s := range probe.Streams {
		track := DiscTrack{Language: s.Tags.Language, Codec: s.CodecName, Channels: s.Channels}
		switch s.CodecType {
		case "video":
			if t.Width == 0 {
				t.Width, t.Height = s.Width, s.Height
				if w, h, ok := strings.Cut(s.Aspect, ":"); ok {
					a, _ := strconv.ParseFloat(w, 64)
					b, _ := strconv.ParseFloat(h, 64)
					if a > 0 && b > 0 {
						t.Aspect = a / b
					}
				}
			}
		case "audio":
			track.Index = len(t.Audio)
			t.Audio = append(t.Audio, track)
		case "subtitle":
			track.Index = len(t.Subtitles)
			t.Subtitles = append(t.Subtitles, track)
		}
	}
	return t, nil
}

// ---------------------------------------------------------------------------------------------
// The disc's tables: the ISO 9660 filesystem to find the IFO files, and the IFOs themselves.

const sectorSize = 2048

type dvdTitleTable struct {
	angles      int
	ptts        []ptt             // chapters: the PGC and program each starts at
	pgcDuration map[int]float64   // by PGC number, in the title's title set
	programs    map[int][]float64 // by PGC number: where each program (from 1) starts in it
}

type ptt struct{ pgc, pg int }

// segments lists the title's PGCs in chapter order, with their place on the title's timeline.
func (t dvdTitleTable) segments() []DiscSegment {
	var segs []DiscSegment
	seen := map[int]bool{}
	start := 0.0
	for _, p := range t.ptts {
		if seen[p.pgc] {
			continue
		}
		seen[p.pgc] = true
		d := t.pgcDuration[p.pgc]
		segs = append(segs, DiscSegment{PGC: p.pgc, PG: p.pg, Start: start, Duration: d})
		start += d
	}
	return segs
}

// readDVDTables reads the title table from VIDEO_TS.IFO and each title's chapters and PGC
// lengths from its title set's IFO.
func readDVDTables(path string) ([]dvdTitleTable, error) {
	var fs dvdFiles
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		fs = folderDisc(path)
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		iso, err := openISO9660(f)
		if err != nil {
			return nil, err
		}
		fs = iso
	}
	vmg, err := fs.readFile("VIDEO_TS", "VIDEO_TS.IFO")
	if err != nil {
		return nil, err
	}
	if len(vmg) < 0xC8 || string(vmg[:12]) != "DVDVIDEO-VMG" {
		return nil, errors.New("VIDEO_TS.IFO is not a DVD video manager")
	}
	srpt := int(be32(vmg, 0xC4)) * sectorSize
	if srpt+8 > len(vmg) {
		return nil, errors.New("bad title table")
	}
	n := int(be16(vmg, srpt))
	vtsCache := map[int][]byte{}
	var titles []dvdTitleTable
	for i := 0; i < n; i++ {
		e := srpt + 8 + 12*i
		if e+12 > len(vmg) {
			break
		}
		angles, vtsn, ttn := int(vmg[e+1]), int(vmg[e+6]), int(vmg[e+7])
		vts, ok := vtsCache[vtsn]
		if !ok {
			vts, _ = fs.readFile("VIDEO_TS", fmt.Sprintf("VTS_%02d_0.IFO", vtsn))
			vtsCache[vtsn] = vts
		}
		tt := dvdTitleTable{angles: angles, pgcDuration: map[int]float64{}, programs: map[int][]float64{}}
		if vts != nil {
			tt.ptts = readPTTs(vts, ttn)
			tt.pgcDuration, tt.programs = readPGCs(vts)
		}
		titles = append(titles, tt)
	}
	if len(titles) == 0 {
		return nil, errors.New("the disc has no titles")
	}
	return titles, nil
}

// readPTTs returns title ttn's chapters (part-of-title entries) in a title set IFO.
func readPTTs(vts []byte, ttn int) []ptt {
	if len(vts) < 0xD0 || string(vts[:12]) != "DVDVIDEO-VTS" {
		return nil
	}
	off := int(be32(vts, 0xC8)) * sectorSize
	if off+8 > len(vts) {
		return nil
	}
	n, last := int(be16(vts, off)), int(be32(vts, off+4))
	if ttn < 1 || ttn > n || off+8+4*n > len(vts) {
		return nil
	}
	start := off + int(be32(vts, off+8+4*(ttn-1)))
	end := off + last + 1
	if ttn < n {
		end = off + int(be32(vts, off+8+4*ttn))
	}
	var ptts []ptt
	for p := start; p+4 <= end && p+4 <= len(vts); p += 4 {
		ptts = append(ptts, ptt{pgc: int(be16(vts, p)), pg: int(be16(vts, p+2))})
	}
	return ptts
}

// readPGCs returns, for each PGC in a title set IFO, its playback time and where each of its
// programs starts. Program times add up the PGC's cells, counting each angle block once.
func readPGCs(vts []byte) (map[int]float64, map[int][]float64) {
	durations, programs := map[int]float64{}, map[int][]float64{}
	if len(vts) < 0xD0 {
		return durations, programs
	}
	off := int(be32(vts, 0xCC)) * sectorSize
	if off+8 > len(vts) {
		return durations, programs
	}
	n := int(be16(vts, off))
	for i := 0; i < n; i++ {
		e := off + 8 + 8*i
		if e+8 > len(vts) {
			break
		}
		pgc := off + int(be32(vts, e+4))
		if pgc+0xEC > len(vts) {
			continue
		}
		durations[i+1] = dvdTime(vts[pgc+4 : pgc+8])
		nPrograms, nCells := int(vts[pgc+2]), int(vts[pgc+3])
		programMap, cellTable := pgc+int(be16(vts, pgc+0xE6)), pgc+int(be16(vts, pgc+0xE8))
		if programMap+nPrograms > len(vts) || cellTable+24*nCells > len(vts) {
			continue
		}
		// Start time of each cell on the PGC's timeline.
		cellStart := make([]float64, nCells+1)
		for c := 0; c < nCells; c++ {
			cell := vts[cellTable+24*c:]
			blockMode, blockType := cell[0]>>6, (cell[0]>>4)&3
			length := dvdTime(cell[4:8])
			if blockType == 1 && blockMode != 1 {
				length = 0 // another angle of the block: same stretch of time
			}
			cellStart[c+1] = cellStart[c] + length
		}
		starts := make([]float64, nPrograms)
		for p := 0; p < nPrograms; p++ {
			if entry := int(vts[programMap+p]); entry >= 1 && entry <= nCells {
				starts[p] = cellStart[entry-1]
			}
		}
		programs[i+1] = starts
	}
	return durations, programs
}

// chapters returns the title's chapter start times on its timeline (its PGCs one after another).
func (t dvdTitleTable) chapters(segs []DiscSegment) []float64 {
	segStart := map[int]float64{}
	for _, s := range segs {
		segStart[s.PGC] = s.Start
	}
	var out []float64
	for _, p := range t.ptts {
		start := segStart[p.pgc]
		if starts := t.programs[p.pgc]; p.pg >= 1 && p.pg <= len(starts) {
			start += starts[p.pg-1] - starts[segFirstPG(segs, p.pgc)-1]
		}
		out = append(out, start)
	}
	return out
}

// segFirstPG is the program a PGC's segment starts playing at.
func segFirstPG(segs []DiscSegment, pgc int) int {
	for _, s := range segs {
		if s.PGC == pgc {
			return max(s.PG, 1)
		}
	}
	return 1
}

// dvdTime decodes a DVD playback time: BCD hours, minutes, seconds, and frames with the frame
// rate in the top two bits.
func dvdTime(b []byte) float64 {
	bcd := func(v byte) float64 { return float64(v>>4)*10 + float64(v&0x0f) }
	fps := 30.0 / 1.001
	if b[3]>>6 == 1 {
		fps = 25
	}
	return bcd(b[0])*3600 + bcd(b[1])*60 + bcd(b[2]) + bcd(b[3]&0x3f)/fps
}

func be16(b []byte, off int) uint16 { return binary.BigEndian.Uint16(b[off:]) }
func be32(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off:]) }

// iso9660 reads files from an ISO 9660 filesystem (the one DVDs carry alongside UDF).
type iso9660 struct {
	r                 io.ReaderAt
	rootLBA, rootSize uint32
}

func openISO9660(r io.ReaderAt) (*iso9660, error) {
	pvd := make([]byte, sectorSize)
	if _, err := r.ReadAt(pvd, 16*sectorSize); err != nil {
		return nil, err
	}
	if pvd[0] != 1 || string(pvd[1:6]) != "CD001" {
		return nil, errors.New("no ISO 9660 filesystem")
	}
	root := pvd[156:190]
	return &iso9660{r: r, rootLBA: binary.LittleEndian.Uint32(root[2:]), rootSize: binary.LittleEndian.Uint32(root[10:])}, nil
}

type isoEntry struct {
	name      string
	lba, size uint32
	dir       bool
}

func (fs *iso9660) list(lba, size uint32) ([]isoEntry, error) {
	if size > 1<<20 {
		return nil, errors.New("directory too large")
	}
	data := make([]byte, size)
	if _, err := fs.r.ReadAt(data, int64(lba)*sectorSize); err != nil && err != io.EOF {
		return nil, err
	}
	var entries []isoEntry
	for i := 0; i < len(data); {
		recLen := int(data[i])
		if recLen == 0 {
			i = (i/sectorSize + 1) * sectorSize // records don't cross sectors
			continue
		}
		if i+recLen > len(data) || recLen < 34 {
			break
		}
		rec := data[i : i+recLen]
		nameLen := int(rec[32])
		if 33+nameLen <= len(rec) {
			name := string(rec[33 : 33+nameLen])
			if j := strings.IndexByte(name, ';'); j >= 0 {
				name = name[:j]
			}
			entries = append(entries, isoEntry{name: name, lba: binary.LittleEndian.Uint32(rec[2:]),
				size: binary.LittleEndian.Uint32(rec[10:]), dir: rec[25]&2 != 0})
		}
		i += recLen
	}
	return entries, nil
}

// errNotOnDisc: a file a video DVD has isn't on the disc (its directory was read).
var errNotOnDisc = errors.New("not found on the disc")

// IsNotVideoDisc says whether LoadDisc's error means the image was read and isn't a video DVD (an
// installer or data disc), as opposed to one that couldn't be read.
func IsNotVideoDisc(err error) bool { return errors.Is(err, errNotOnDisc) }

// IsNonDiscISO says whether a file is a disc image that was read and isn't a video DVD (an
// installer or data disc). Those aren't videos, so the library leaves them out.
func IsNonDiscISO(path string) bool {
	if !IsDiscImage(path) {
		return false
	}
	_, err := LoadDisc(path)
	return IsNotVideoDisc(err)
}

// IsNonDiscISOWithin is IsNonDiscISO for requests: waiting at most wait for the disc to be read,
// and saying no until it is (the probe goes on).
func IsNonDiscISOWithin(path string, wait time.Duration) bool {
	if !IsDiscImage(path) {
		return false
	}
	_, err := LoadDiscWithin(path, wait)
	return IsNotVideoDisc(err)
}

// readFile reads a file by its path components (case-insensitive), up to 16 MB.
func (fs *iso9660) readFile(parts ...string) ([]byte, error) {
	lba, size := fs.rootLBA, fs.rootSize
	for i, part := range parts {
		entries, err := fs.list(lba, size)
		if err != nil {
			return nil, err
		}
		found := false
		for _, e := range entries {
			if strings.EqualFold(e.name, part) && e.dir == (i < len(parts)-1) {
				lba, size, found = e.lba, e.size, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%s %w", strings.Join(parts, "/"), errNotOnDisc)
		}
	}
	if size > 16<<20 {
		return nil, errors.New("file too large")
	}
	data := make([]byte, size)
	if _, err := fs.r.ReadAt(data, int64(lba)*sectorSize); err != nil && err != io.EOF {
		return nil, err
	}
	return data, nil
}

// DiscFrameArgs are ffmpeg arguments (inputs, then an accurate seek) for a frame of the main
// title at the given fraction of its length, for thumbnails.
func DiscFrameArgs(path string, fraction float64) ([]string, error) {
	info, err := LoadDisc(path)
	if err != nil {
		return nil, err
	}
	t := info.Title(0)
	if t == nil {
		return nil, errors.New("no titles")
	}
	if info.MainTitleUncertain {
		log.Printf("[DISC] %s: a title that can't be read may be longer than title %d, used as the main title", path, t.Number)
	}
	i, offset := t.SegmentAt(t.Duration * fraction)
	args, trim := t.InputArgs(path, i, 1, offset)
	if trim > 0 {
		args = append(args, "-ss", strconv.FormatFloat(trim, 'f', 3, 64))
	}
	return args, nil
}
