package media

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"
)

func TestForcedOnly(t *testing.T) {
	plain, forced := testPicture(), testPicture()
	forced.forced = true
	packets := []dvdPacket{{time: 10, data: encodeSPU(plain, 2)}, {time: 20, data: encodeSPU(forced, 2)}}
	look := &dvdLook{palette: defaultDVDPalette, width: 720, height: 480, aspect: 1}
	all := readPGS(t, pgsFromPackets(packets, look, 0, 0, false))
	only := readPGS(t, pgsFromPackets(packets, look, 0, 0, true))
	// Each picture is a display set, then one that clears it.
	if len(all) != 4 {
		t.Fatalf("all lines: %d sets, want 4", len(all))
	}
	if len(only) != 2 || only[0].time != 20 {
		t.Fatalf("forced only: %d sets (first at %v), want the one at 20 s and its clearing", len(only), only)
	}
}

func TestStreamStart(t *testing.T) {
	plain := &DiscTitle{Duration: 100, Segments: []DiscSegment{{}}}
	chained := &DiscTitle{Duration: 100, Segments: []DiscSegment{{PGC: 1}, {PGC: 2, Start: 50}}}
	late := &DiscTitle{Duration: 100, Segments: []DiscSegment{{}}, FirstChapter: 2, Chapters: []float64{0, 1}}
	for _, c := range []struct {
		name  string
		t     *DiscTitle
		from  float64
		start float64
	}{
		{"sought as the stream is", plain, 30, 30},
		{"clamped as the stream is", plain, 500, 99},
		{"not before the start", plain, -3, 0},
		{"chained parts: the title's own time", chained, 30, 0},
		{"from a later chapter: the title's own time", late, 30, 0},
	} {
		if got := streamStart(c.t, c.from); got != c.start {
			t.Errorf("%s: %v, want %v", c.name, got, c.start)
		}
	}
}

func TestLibLineFilter(t *testing.T) {
	out := "{\n    \"packets\": [\nlibdvdnav: Suspected RCE Region Protection!!!\n        {\n            \"pts_time\": \"1.5\"\n        }\n    ]\n}\n"
	var got struct {
		Packets []struct {
			PTS string `json:"pts_time"`
		} `json:"packets"`
	}
	if err := json.NewDecoder(&libLineFilter{r: bufio.NewReader(strings.NewReader(out))}).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Packets) != 1 || got.Packets[0].PTS != "1.5" {
		t.Fatalf("got %+v", got)
	}
}
