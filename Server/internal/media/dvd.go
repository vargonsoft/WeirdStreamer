package media

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// DVD-Video disc images (.iso) are played with ffmpeg's dvdvideo demuxer. It plays one program
// chain (PGC) at a time, while a title can be several chained PGCs, so the disc's own tables
// (its IFO files) are read to know each title's chain; ffprobe supplies chapters and tracks.

// IsDiscImage reports whether a file is a disc image played through the DVD reader.
func IsDiscImage(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".iso")
}

// DiscInfo describes what a DVD disc image holds.
type DiscInfo struct {
	Titles    []DiscTitle `json:"titles"`
	MainTitle int         `json:"main_title"` // number of the main feature: the longest title
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
	Segments  []DiscSegment `json:"segments"` // the program chains played one after another
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
	seg := t.Segments[i]
	if seg.PGC > 0 {
		args = append(args, "-pgc", strconv.Itoa(seg.PGC), "-pg", strconv.Itoa(max(seg.PG, 1)))
		trim = seek
	} else if seek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(seek, 'f', 3, 64))
	}
	if angle > 1 && angle <= t.Angles {
		args = append(args, "-angle", strconv.Itoa(angle))
	}
	return append(args, "-i", path), trim
}

// DiscStore persists probed disc info between restarts (the database); optional.
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
	info          *DiscInfo
	once          sync.Once
	err           error
}

// SetDiscStore sets where probed disc info is kept.
func SetDiscStore(s DiscStore) { discStore = s }

// LoadDisc returns a disc image's info, probing it the first time (a few seconds) and caching
// it in memory and in the DiscStore.
func LoadDisc(path string) (*DiscInfo, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size, modTime := st.Size(), st.ModTime().Unix()
	discMu.Lock()
	e := discCache[path]
	if e == nil || e.size != size || e.modTime != modTime {
		e = &discEntry{size: size, modTime: modTime}
		discCache[path] = e
	}
	discMu.Unlock()

	e.once.Do(func() {
		if discStore != nil {
			if data, err := discStore.GetDiscInfo(path, size, modTime); err == nil && data != nil {
				var info DiscInfo
				if json.Unmarshal(data, &info) == nil && len(info.Titles) > 0 {
					e.info = &info
					return
				}
			}
		}
		e.info, e.err = probeDisc(path)
		if e.err == nil && discStore != nil {
			if data, err := json.Marshal(e.info); err == nil {
				discStore.SaveDiscInfo(path, size, modTime, data)
			}
		}
	})
	return e.info, e.err
}

// probeDisc reads a disc's title chains from its IFO tables and each title's chapters and tracks
// with ffprobe.
func probeDisc(path string) (*DiscInfo, error) {
	tables, tablesErr := readDVDTables(path)
	count := 99
	if tablesErr == nil {
		count = len(tables)
	}
	info := &DiscInfo{}
	for n := 1; n <= count; n++ {
		t, err := probeTitle(path, n)
		if err != nil {
			if tablesErr != nil {
				break // without the tables, titles are probed until one is missing
			}
			continue
		}
		t.Angles = 1
		t.Segments = []DiscSegment{{Start: 0, Duration: t.Duration}}
		if tablesErr == nil {
			tt := tables[n-1]
			t.Angles = max(tt.angles, 1)
			if segs := tt.segments(); len(segs) > 1 && chainMatches(segs, t.Duration) {
				t.Segments = segs
			}
		}
		info.Titles = append(info.Titles, *t)
	}
	if len(info.Titles) == 0 {
		if tablesErr != nil {
			return nil, fmt.Errorf("not a readable DVD image: %w", tablesErr)
		}
		return nil, errors.New("no playable titles on the disc")
	}
	info.MainTitle = mainTitle(info.Titles)
	return info, nil
}

// chainMatches reports whether a title's PGC chain adds up to the title's length, so it can be
// played piece by piece; otherwise the title is played as ffmpeg plays it.
func chainMatches(segs []DiscSegment, duration float64) bool {
	total := 0.0
	for _, s := range segs {
		if s.Duration <= 0 {
			return false
		}
		total += s.Duration
	}
	diff := total - duration
	return diff < 0.02*duration+2 && -diff < 0.02*duration+2
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

func probeTitle(path string, n int) (*DiscTitle, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-f", "dvdvideo", "-title", strconv.Itoa(n), "-i", path,
		"-show_format", "-show_chapters", "-show_streams", "-of", "json").Output()
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
			Tags      struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
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
	ptts        []ptt           // chapters: the PGC and program each starts at
	pgcDuration map[int]float64 // by PGC number, in the title's title set
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
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fs, err := openISO9660(f)
	if err != nil {
		return nil, err
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
		tt := dvdTitleTable{angles: angles, pgcDuration: map[int]float64{}}
		if vts != nil {
			tt.ptts = readPTTs(vts, ttn)
			tt.pgcDuration = readPGCDurations(vts)
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

// readPGCDurations returns the playback time of each PGC in a title set IFO.
func readPGCDurations(vts []byte) map[int]float64 {
	durations := map[int]float64{}
	if len(vts) < 0xD0 {
		return durations
	}
	off := int(be32(vts, 0xCC)) * sectorSize
	if off+8 > len(vts) {
		return durations
	}
	n := int(be16(vts, off))
	for i := 0; i < n; i++ {
		e := off + 8 + 8*i
		if e+8 > len(vts) {
			break
		}
		pgc := off + int(be32(vts, e+4))
		if pgc+8 > len(vts) {
			continue
		}
		durations[i+1] = dvdTime(vts[pgc+4 : pgc+8])
	}
	return durations
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
			return nil, fmt.Errorf("%s not found on the disc", strings.Join(parts, "/"))
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
	i, offset := t.SegmentAt(t.Duration * fraction)
	args, trim := t.InputArgs(path, i, 1, offset)
	if trim > 0 {
		args = append(args, "-ss", strconv.FormatFloat(trim, 'f', 3, 64))
	}
	return args, nil
}
