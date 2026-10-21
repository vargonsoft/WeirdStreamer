package media

import (
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
	}
	for query, want := range cases {
		r := httptest.NewRequest("GET", "/api/media/stream?"+query, nil)
		if got := ParseTrackOptions(r); got != want {
			t.Errorf("%q: got %+v, want %+v", query, got, want)
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

func TestSubtitleArgs(t *testing.T) {
	got := strings.Join(SubtitleArgs("/m/a.mkv", 2, 90.5, 120), " ")
	want := "-v error -ss 90.500 -i /m/a.mkv -t 120.000 -map 0:s:2 -c:s webvtt -f webvtt pipe:1"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	got = strings.Join(SubtitleArgs("/m/a.mkv", 0, 0, 0), " ")
	if want := "-v error -i /m/a.mkv -map 0:s:0 -c:s webvtt -f webvtt pipe:1"; got != want {
		t.Errorf("from the start, to the end: got %s", got)
	}
}
