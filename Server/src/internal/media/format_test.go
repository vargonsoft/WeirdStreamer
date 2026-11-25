package media

import (
	"encoding/json"
	"testing"
)

func TestFormatLine(t *testing.T) {
	cases := []struct{ probe, want string }{
		{`{"streams":[{"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,"color_transfer":"smpte2084",
			"side_data_list":[{"side_data_type":"DOVI configuration record"}]},
			{"codec_type":"audio","codec_name":"truehd","profile":"Dolby TrueHD + Dolby Atmos","channels":8,"disposition":{"default":1}},
			{"codec_type":"audio","codec_name":"ac3","channels":6}]}`, "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"},
		{`{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":800},
			{"codec_type":"audio","codec_name":"aac","channels":2}]}`, "1080p · H.264 · AAC 2.0"},
		{`{"streams":[{"codec_type":"video","codec_name":"hevc","width":3840,"height":1600,"color_transfer":"smpte2084"},
			{"codec_type":"audio","codec_name":"dts","profile":"DTS-HD MA","channels":6}]}`, "4K · HEVC · HDR10 · DTS-HD MA 5.1"},
		// A cover picture isn't the video; the default audio track wins
		{`{"streams":[{"codec_type":"video","codec_name":"mjpeg","width":600,"height":600,"disposition":{"attached_pic":1}},
			{"codec_type":"video","codec_name":"mpeg4","width":720,"height":400},
			{"codec_type":"audio","codec_name":"mp3","channels":2},
			{"codec_type":"audio","codec_name":"eac3","channels":6,"disposition":{"default":1}}]}`, "480p · MPEG-4 · DD+ 5.1"},
		{`{"streams":[{"codec_type":"audio","codec_name":"flac","channels":2}]}`, ""},
	}
	for _, c := range cases {
		var p formatProbe
		if err := json.Unmarshal([]byte(c.probe), &p); err != nil {
			t.Fatal(err)
		}
		if got := formatLine(p); got != c.want {
			t.Errorf("formatLine = %q, want %q", got, c.want)
		}
	}
}
