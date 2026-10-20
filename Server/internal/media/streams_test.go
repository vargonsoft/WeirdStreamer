package media

import (
	"testing"
	"time"
)

func TestStreamTracker(t *testing.T) {
	clock := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tr := NewStreamTracker()
	tr.now = func() time.Time { return clock }
	init := func(info *StreamInfo) {
		info.User, info.Path, info.Size, info.Duration = "ash", "/m/a.mkv", 1000, 100
	}

	// Range requests for one file from one client are one stream; bytes give the served position.
	s := tr.Begin("1|ip|/m/a.mkv", init)
	s.Sent(100, 500)
	if again := tr.Begin("1|ip|/m/a.mkv", init); again != s {
		t.Fatal("second request started a new stream")
	}
	list := tr.List()
	if len(list) != 1 || list[0].Requests != 2 || list[0].Active != 2 || list[0].State != "streaming" ||
		list[0].BytesSent != 100 || list[0].ServedTo != 50 || list[0].Mode != "direct" {
		t.Fatalf("direct stream = %+v", list)
	}
	s.End()
	s.End()

	// Player reports say playing or paused while fresh.
	tr.Report("1|ip|/m/a.mkv", init, 42, false)
	if st := tr.List()[0]; st.State != "playing" || *st.Position != 42 {
		t.Errorf("reported = %+v", st)
	}
	tr.Report("1|ip|/m/a.mkv", init, 43, true)
	if st := tr.List()[0]; st.State != "paused" {
		t.Errorf("paused report state = %q", st.State)
	}
	clock = clock.Add(reportFresh + time.Second)
	if st := tr.List()[0]; st.State != "idle" {
		t.Errorf("stale report state = %q", st.State)
	}

	// Transcodes: settings from the args, progress from ffmpeg's output, stalls when it waits.
	s = tr.Begin("1|ip|/m/a.mkv", init)
	s.StartTranscode(describeTranscode([]string{"-ss", "60", "-i", "/m/a.mkv", "-c:v", "libx264", "-preset", "ultrafast",
		"-crf", "23", "-c:a", "aac"}, "60", "", "", "Browsers can't play .mkv files"))
	s.FFmpegOutput("Input #0, avi, from 'a.avi':\n  Stream #0:0: Video: mpeg4 (Simple Profile) (XVID / 0x44495658), yuv420p, 640x360, 25 fps\n" +
		"  Stream #0:1: Audio: mp3, 48000 Hz, stereo, fltp, 128 kb/s\n")
	s.FFmpegOutput("frame=  100 fps= 50 q=27.0 size=  1024KiB time=00:00:04.00 bitrate=2097.2kbits/s speed=2.0x\r" +
		"frame=  250 fps= 62 q=27.0 size=  2048KiB time=00:00:10.00 bitrate=1677.7kbits/s speed=2.5x\r")
	st := tr.List()[0]
	tc := st.Transcode
	if st.Mode != "transcode" || tc == nil || tc.Start != 60 || tc.OutTime != 10 || tc.Speed != 2.5 || tc.FPS != 62 ||
		tc.Frame != 250 || tc.OutBitrate != 1677.7 || tc.Stalled || st.ServedTo != 70 {
		t.Fatalf("transcode = %+v / %+v", st, tc)
	}
	if tc.Video != "H.264 (libx264, preset ultrafast, CRF 23)" || tc.SourceVideo == "" || tc.SourceAudio == "" {
		t.Errorf("transcode description = %+v", tc)
	}
	clock = clock.Add(transcodeStall + time.Second)
	s.FFmpegOutput("frame=  250 fps= 40 q=27.0 size=  2048KiB time=00:00:10.00 bitrate=1677.7kbits/s speed=1.6x\r")
	if tc := tr.List()[0].Transcode; !tc.Stalled || tc.Speed != 0 {
		t.Errorf("waiting transcode not stalled: %+v", tc)
	}
	s.End()
	if st := tr.List()[0]; st.Transcode != nil || st.Mode != "direct" {
		t.Errorf("finished transcode still listed: %+v", st)
	}

	// Streams go away once quiet.
	clock = clock.Add(streamIdleTimeout + time.Second)
	if list := tr.List(); len(list) != 0 {
		t.Errorf("quiet stream still listed: %+v", list)
	}
}

func TestClientName(t *testing.T) {
	for ua, want := range map[string]string{
		"WeirdStreamer/1.0": "WeirdStreamer app",
		"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:157.0) Gecko/20100101 Firefox/157.0":                          "Firefox",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36":         "Chrome",
		"Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140": "Edge",
		"": "Unknown",
	} {
		if got := ClientName(ua); got != want {
			t.Errorf("ClientName(%q) = %q, want %q", ua, got, want)
		}
	}
}
