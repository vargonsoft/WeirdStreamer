package media

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTrackOptions(t *testing.T) {
	cases := map[string]TrackOptions{
		"":                      {Audio: -1, Subtitle: -1},
		"audio=2":               {Audio: 2, Subtitle: -1},
		"audio=1&subtitle=0":    {Audio: 1, Subtitle: 0},
		"audio=x&subtitle=-3":   {Audio: -1, Subtitle: -1},
		"subtitle=4&title=1&a=": {Audio: -1, Subtitle: 4},
		"channels=2":            {Audio: -1, Subtitle: -1, Stereo: true},
		"channels=6":            {Audio: -1, Subtitle: -1},
		"audiocodec=copy":       {Audio: -1, Subtitle: -1, AudioCodec: "copy"},
		"audiocodec=ac3":        {Audio: -1, Subtitle: -1, AudioCodec: "ac3"},
		"audiocodec=dts":        {Audio: -1, Subtitle: -1},
	}
	for query, want := range cases {
		r := httptest.NewRequest("GET", "/api/media/stream?"+query, nil)
		if got := ParseTrackOptions(r); got != want {
			t.Errorf("%q: got %+v, want %+v", query, got, want)
		}
	}
}

func TestAudioArgs(t *testing.T) {
	cases := []struct {
		tracks TrackOptions
		stereo bool
		args   string
		delay  bool
	}{
		{TrackOptions{}, false, "-c:a aac -b:a 192k", false},
		{TrackOptions{}, true, "-c:a aac -ac 2 -b:a 192k", false},
		{TrackOptions{Stereo: true}, false, "-c:a aac -ac 2 -b:a 192k", false},
		{TrackOptions{AudioCodec: "copy"}, true, "-c:a copy", true},
		{TrackOptions{AudioCodec: "ac3"}, true, "-c:a ac3 -ac 6 -b:a 640k", true},
		{TrackOptions{AudioCodec: "eac3"}, false, "-c:a eac3 -ac 6 -b:a 640k", true},
		{TrackOptions{AudioCodec: "aac"}, true, "-c:a aac -ac 6 -b:a 384k", false},
		{TrackOptions{AudioCodec: "aac", Stereo: true}, false, "-c:a aac -ac 2 -b:a 192k", false},
	}
	for _, c := range cases {
		args, _ := c.tracks.audioArgs(c.stereo)
		if got := strings.Join(args, " "); got != c.args {
			t.Errorf("%+v: got %q, want %q", c.tracks, got, c.args)
		}
		if got := strings.Contains(c.tracks.movFlags(), "delay_moov"); got != c.delay {
			t.Errorf("%+v: delay_moov %v, want %v", c.tracks, got, c.delay)
		}
	}
}

func TestCodecsChoices(t *testing.T) {
	c := &Codecs{
		Video: &VideoCodec{Codec: "h264", PixFmt: "yuv420p"},
		Audio: []AudioCodec{{Index: 0, Codec: "aac"}, {Index: 1, Codec: "eac3", Default: true}},
	}
	if c.DefaultAudio() != 1 {
		t.Errorf("default audio: got %d, want the track marked default", c.DefaultAudio())
	}
	if (TrackOptions{Audio: 0}).audioIndex(c) != 0 || (TrackOptions{Audio: 5}).audioIndex(c) != 1 {
		t.Error("audioIndex: a chosen track, or the default one when it doesn't exist")
	}
	if !c.BrowserVideo() {
		t.Error("8-bit H.264 is browser video")
	}
	c.Video.PixFmt = "yuv420p10le"
	if c.BrowserVideo() {
		t.Error("10-bit H.264 isn't")
	}
	if (&Codecs{Audio: []AudioCodec{{Codec: "dts"}}}).DefaultAudio() != 0 {
		t.Error("without a default track, the first one")
	}
	if !IsTextSubtitle("subrip") || !IsTextSubtitle("ass") || IsTextSubtitle("hdmv_pgs_subtitle") || IsTextSubtitle("dvd_subtitle") {
		t.Error("text subtitles are text codecs, not pictures")
	}
}

func TestParseFrameTime(t *testing.T) {
	out := "#tb 0: 1/1000\n#media_type 0: video\n#codec_id 0: h264\n0,    1202081,    1202164,       41,    81374, 0xf88fd388\n"
	if got, err := parseFrameTime(out); err != nil || got != 1202.164 {
		t.Errorf("got %v, %v; want the pts, 1202.164", got, err)
	}
	if _, err := parseFrameTime("#tb 0: 1/1000\n"); err == nil {
		t.Error("no frame: want an error")
	}
	// An AVI's packets have no pts (framecrc prints AV_NOPTS_VALUE), only a dts.
	avi := "#tb 0: 1/25\n0,        289, -9223372036854775808,        1,    16087, 0x699eec5f\n"
	if got, err := parseFrameTime(avi); err != nil || got != 11.56 {
		t.Errorf("got %v, %v; want the dts, 11.56", got, err)
	}
	if _, err := parseFrameTime("#tb 0: 1/25\n0, -9223372036854775808, -9223372036854775808, 1, 1, 0x0\n"); err == nil {
		t.Error("no time at all: want an error")
	}
}

func TestParseNextKeyframe(t *testing.T) {
	// From the file's start (1.4 s here), the first keyframe at or after 15 s.
	out := `{"packets": [
		{"pts_time": "11.405", "dts_time": "11.325", "flags": "K__"},
		{"pts_time": "15.405", "dts_time": "15.325", "flags": "___"},
		{"pts_time": "21.405", "dts_time": "21.325", "flags": "K__"}],
		"format": {"start_time": "1.400000"}}`
	if got, err := parseNextKeyframe([]byte(out), 15); err != nil || math.Abs(got-20.005) > 1e-9 {
		t.Errorf("got %v, %v; want 20.005", got, err)
	}
	// An AVI's packets: no pts, only a dts.
	avi := `{"packets": [{"dts_time": "14.000000", "flags": "K__"}, {"dts_time": "16.000000", "flags": "K__"}],
		"format": {"start_time": "0.000000"}}`
	if got, err := parseNextKeyframe([]byte(avi), 15); err != nil || got != 16 {
		t.Errorf("AVI: got %v, %v; want 16", got, err)
	}
	if _, err := parseNextKeyframe([]byte(avi), 30); err == nil {
		t.Error("no keyframe after: want an error")
	}
}

func TestPictureSubtitleArgs(t *testing.T) {
	got := strings.Join(PictureSubtitleArgs("/m/a.mkv", 2, 90.5, 120), " ")
	want := "-v error -ss 90.500 -i /m/a.mkv -copyts -start_at_zero -to 210.500 -map 0:s:2 -c:s copy -f sup pipe:1"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestSubtitleArgs(t *testing.T) {
	got := strings.Join(SubtitleArgs("/m/a.mkv", 2, 90.5, 120), " ")
	want := "-v error -ss 90.500 -i /m/a.mkv -copyts -start_at_zero -to 210.500 -map 0:s:2 -c:s webvtt -f webvtt pipe:1"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	got = strings.Join(SubtitleArgs("/m/a.mkv", 0, 0, 0), " ")
	if want := "-v error -i /m/a.mkv -copyts -start_at_zero -map 0:s:0 -c:s webvtt -f webvtt pipe:1"; got != want {
		t.Errorf("from the start, to the end: got %s", got)
	}
}

func TestAssSubtitleArgs(t *testing.T) {
	got := strings.Join(AssSubtitleArgs("/m/a.mkv", 2, 90.5, 120), " ")
	want := "-v error -ss 90.500 -i /m/a.mkv -copyts -start_at_zero -to 210.500 -map 0:s:2 -c:s ass -f ass pipe:1"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	// A file is seeked as it's read: an output -ss would shift ASS lines.
	got = strings.Join(AssSubtitleFileArgs("/m/a.ass", "UTF-8", 90.5, 120), " ")
	want = "-v error -sub_charenc UTF-8 -ss 90.500 -i /m/a.ass -copyts -to 210.500 -map 0:s:0 -c:s ass -f ass pipe:1"
	if got != want {
		t.Errorf("file: got  %s\nwant %s", got, want)
	}
}

func TestIsFont(t *testing.T) {
	for _, c := range []struct {
		mime, name string
		want       bool
	}{
		{"font/otf", "a.otf", true},
		{"application/x-truetype-font", "a.ttf", true},
		{"application/vnd.ms-opentype", "a", true},
		{"application/octet-stream", "A.TTF", true},
		{"image/jpeg", "cover.jpg", false},
		{"text/plain", "notes.txt", false},
	} {
		if got := isFont(c.mime, c.name); got != c.want {
			t.Errorf("isFont(%q, %q) = %v", c.mime, c.name, got)
		}
	}
}
