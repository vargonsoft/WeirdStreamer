package media

import (
	"encoding/binary"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestDVDTime(t *testing.T) {
	// 1:02:03 and 15 frames at 30 fps (rate bits 11), and 25 frames at 25 fps (01).
	if got := dvdTime([]byte{0x01, 0x02, 0x03, 0xC0 | 0x15}); got < 3723.49 || got > 3723.51 {
		t.Errorf("30 fps time = %v", got)
	}
	if got := dvdTime([]byte{0x00, 0x00, 0x10, 0x40 | 0x20}); got != 10.8 {
		t.Errorf("25 fps time = %v", got)
	}
}

// vtsWith builds a title set IFO holding one title's chapters and its PGCs' lengths.
func vtsWith(ptts []ptt, pgcTimes [][4]byte) []byte {
	b := make([]byte, 4*sectorSize)
	copy(b, "DVDVIDEO-VTS")
	binary.BigEndian.PutUint32(b[0xC8:], 1) // PTT table at sector 1
	binary.BigEndian.PutUint32(b[0xCC:], 2) // PGC table at sector 2
	p := sectorSize
	binary.BigEndian.PutUint16(b[p:], 1)
	binary.BigEndian.PutUint32(b[p+4:], uint32(8+4+4*len(ptts)-1))
	binary.BigEndian.PutUint32(b[p+8:], 12)
	for i, e := range ptts {
		binary.BigEndian.PutUint16(b[p+12+4*i:], uint16(e.pgc))
		binary.BigEndian.PutUint16(b[p+14+4*i:], uint16(e.pg))
	}
	g := 2 * sectorSize
	binary.BigEndian.PutUint16(b[g:], uint16(len(pgcTimes)))
	for i, tm := range pgcTimes {
		start := 8 + 8*len(pgcTimes) + 16*i
		binary.BigEndian.PutUint32(b[g+8+8*i+4:], uint32(start))
		copy(b[g+start+4:], tm[:])
	}
	return b
}

func TestTitleTables(t *testing.T) {
	// A title of three chained PGCs; the second holds two chapters.
	vts := vtsWith([]ptt{{1, 1}, {2, 1}, {2, 2}, {3, 1}},
		[][4]byte{{0, 0, 0x07, 0xC0}, {0, 0x01, 0x13, 0xC0}, {0, 0x04, 0x34, 0xC0}})
	durations, programs := readPGCs(vts)
	tt := dvdTitleTable{angles: 2, ptts: readPTTs(vts, 1), pgcDuration: durations, programs: programs}
	want := []DiscSegment{{PGC: 1, PG: 1, Start: 0, Duration: 7}, {PGC: 2, PG: 1, Start: 7, Duration: 73},
		{PGC: 3, PG: 1, Start: 80, Duration: 274}}
	if got := tt.segments(); !reflect.DeepEqual(got, want) {
		t.Fatalf("segments = %+v", got)
	}
	if !validChain(want) || validChain(nil) || validChain([]DiscSegment{{PGC: 1}}) {
		t.Error("validChain should accept PGCs with lengths only")
	}

	title := DiscTitle{Number: 1, Angles: 2, Duration: 354, Segments: want}
	if i, off := title.SegmentAt(100); i != 2 || off != 20 {
		t.Errorf("SegmentAt(100) = %d, %v", i, off)
	}
	args, trim := title.InputArgs("/d.iso", 2, 2, 20)
	if strings.Join(args, " ") != "-f dvdvideo -title 1 -pgc 3 -pg 1 -angle 2 -i /d.iso" || trim != 20 {
		t.Errorf("chained input = %v, trim %v", args, trim)
	}
	whole := DiscTitle{Number: 4, Angles: 1, Duration: 600, Segments: []DiscSegment{{Duration: 600}}}
	args, trim = whole.InputArgs("/d.iso", 0, 3, 90)
	if strings.Join(args, " ") != "-f dvdvideo -title 4 -ss 90.000 -i /d.iso" || trim != 0 {
		t.Errorf("whole-title input = %v, trim %v (angle beyond the title's must be dropped)", args, trim)
	}

	// A title ffmpeg reads only from chapter 2 (a padding cell first): from chapter 2 at the
	// start, else from the chapter the seek lands in, with the rest cut after decoding.
	padded := DiscTitle{Number: 14, Angles: 1, Duration: 193.2, FirstChapter: 2,
		Chapters: []float64{0, 0.5, 41.3, 192.7}, Segments: []DiscSegment{{Duration: 193.2}}}
	for _, c := range []struct {
		seek, trim float64
		args       string
	}{
		{0, 0, "-f dvdvideo -title 14 -chapter_start 2 -i /d.iso"},
		{10.5, 10, "-f dvdvideo -title 14 -chapter_start 2 -i /d.iso"},
		{51.3, 10, "-f dvdvideo -title 14 -chapter_start 3 -i /d.iso"},
		{193, 0.3, "-f dvdvideo -title 14 -chapter_start 4 -i /d.iso"},
	} {
		args, trim = padded.InputArgs("/d.iso", 0, 1, c.seek)
		if strings.Join(args, " ") != c.args || math.Abs(trim-c.trim) > 1e-9 {
			t.Errorf("padded title at %v = %v, trim %v; want %s, trim %v", c.seek, args, trim, c.args, c.trim)
		}
	}

	// A title of nothing but padding plays as it is, untrimmed.
	blank := DiscTitle{Number: 15, Angles: 1, Duration: 0.5, Padding: true, Segments: []DiscSegment{{Duration: 0.5}}}
	if args, _ = blank.InputArgs("/d.iso", 0, 1, 0); strings.Join(args, " ") != "-f dvdvideo -title 15 -trim 0 -i /d.iso" {
		t.Errorf("padding title input = %v", args)
	}
}

func TestEmptyOutput(t *testing.T) {
	if !emptyOutput([]string{"[out#0/mp4 @ 0x1] video:0KiB audio:0KiB", "[out#0/mp4 @ 0x1] Output file is empty, nothing was encoded"}) {
		t.Error("an output with nothing encoded should count as empty")
	}
	if emptyOutput([]string{"[out#0/mp4 @ 0x1] video:2048KiB audio:300KiB"}) {
		t.Error("an output with frames should not count as empty")
	}
}

func TestMainTitle(t *testing.T) {
	titles := []DiscTitle{
		{Number: 1, Duration: 5524.93, Audio: make([]DiscTrack, 3), Chapters: make([]float64, 33)},
		{Number: 3, Duration: 37},
		{Number: 10, Duration: 5524.97, Audio: make([]DiscTrack, 1), Chapters: make([]float64, 33)}, // commentary cut
	}
	if got := mainTitle(titles); got != 1 {
		t.Errorf("main title = %d, want the full-audio feature 1", got)
	}
	titles[0].Duration = 3000
	if got := mainTitle(titles); got != 10 {
		t.Errorf("main title = %d, want the clearly longest 10", got)
	}
}

// A PGC whose programs start at its 1st, 2nd and 4th cells, with cells 2 and 3 the two angles of
// one block: the block counts once, so program 3 starts after cell 1 and one angle cell.
func TestProgramStarts(t *testing.T) {
	b := make([]byte, 4*sectorSize)
	copy(b, "DVDVIDEO-VTS")
	binary.BigEndian.PutUint32(b[0xCC:], 1) // PGC table at sector 1
	g := sectorSize
	binary.BigEndian.PutUint16(b[g:], 1)
	binary.BigEndian.PutUint32(b[g+12:], 16) // the PGC, 16 bytes into the table
	pgc := g + 16
	b[pgc+2], b[pgc+3] = 3, 4                                                               // programs, cells
	copy(b[pgc+4:], []byte{0, 0, 0x50, 0xC0})                                               // PGC length 50s
	binary.BigEndian.PutUint16(b[pgc+0xE6:], 0xEC)                                          // program map
	binary.BigEndian.PutUint16(b[pgc+0xE8:], 0xF0)                                          // cell table
	copy(b[pgc+0xEC:], []byte{1, 2, 4})                                                     // programs' first cells
	cells := []struct{ flags, secs byte }{{0, 0x10}, {0x50, 0x15}, {0xD0, 0x15}, {0, 0x20}} // angle block: first (01) and last (11)
	for i, c := range cells {
		e := pgc + 0xF0 + 24*i
		b[e] = c.flags
		copy(b[e+4:], []byte{0, 0, c.secs, 0xC0})
	}
	durations, programs := readPGCs(b)
	if durations[1] != 50 || !reflect.DeepEqual(programs[1], []float64{0, 10, 25}) {
		t.Errorf("PGC = %v, programs %v; want 50s and starts [0 10 25]", durations[1], programs[1])
	}
	tt := dvdTitleTable{ptts: []ptt{{1, 1}, {1, 2}, {1, 3}}, pgcDuration: durations, programs: programs}
	if got := tt.chapters(tt.segments()); !reflect.DeepEqual(got, []float64{0, 10, 25}) {
		t.Errorf("chapters = %v", got)
	}
}
