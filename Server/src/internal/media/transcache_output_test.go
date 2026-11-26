package media

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// slowReader hands out its text a few bytes at a time, as a pipe might.
type slowReader struct{ text string }

func (r *slowReader) Read(p []byte) (int, error) {
	if r.text == "" {
		return 0, errEOF
	}
	n := copy(p[:min(len(p), 7)], r.text)
	r.text = r.text[n:]
	return n, nil
}

var errEOF = io.EOF

func TestReadOutputKeepsMessagesNotProgress(t *testing.T) {
	s := &transcodeSpool{}
	out := "Input #0, matroska\n  Stream #0:0: Video: h264\n" +
		"frame=  10 fps=0.0 size=  1KiB time=00:00:01.00\rframe=  20 fps=20 size=  2KiB time=00:00:02.00\r" +
		"[aac @ 0x1] Too many bits\nConversion failed!"
	got := s.readOutput(&slowReader{out})
	want := []string{"Input #0, matroska", "Stream #0:0: Video: h264", "[aac @ 0x1] Too many bits", "Conversion failed!"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// What a library prints to stdout during a transcode stays out of the stream. The test binary
// stands in for ffmpeg (TestOutputHelper): a line on stdout, the video to its output argument.
func TestTranscodeOutputHasItsOwnPipe(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestOutputHelper$", "--", "pipe:1")
	cmd.Env = append(os.Environ(), "WEIRDSTREAMER_OUTPUT_HELPER=1")
	output, messages, err := transcodePipes(cmd)
	if err != nil {
		t.Fatal(err)
	}
	said := make(chan []byte)
	go func() { b, _ := io.ReadAll(messages); said <- b }()
	video, _ := io.ReadAll(output)
	output.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%v: %s", err, <-said)
	}
	if string(video) != "video" {
		t.Errorf("output %q, want only the video", video)
	}
	if text := string(<-said); !strings.Contains(text, "CHECK_VALUE") {
		t.Errorf("messages %q lack the library's line", text)
	}
}

// TestOutputHelper is ffmpeg for TestTranscodeOutputHasItsOwnPipe: it writes a line to stdout and
// "video" to its last argument (pipe:3 is fd 3; anything else a file or a named pipe).
func TestOutputHelper(t *testing.T) {
	if os.Getenv("WEIRDSTREAMER_OUTPUT_HELPER") != "1" {
		t.Skip("run by TestTranscodeOutputHasItsOwnPipe")
	}
	fmt.Println("libdvdread: CHECK_VALUE failed")
	target := os.Args[len(os.Args)-1]
	var out *os.File
	if target == "pipe:3" {
		out = os.NewFile(3, "fd 3")
	} else {
		var err error
		if out, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	out.WriteString("video")
	out.Close()
	os.Exit(0)
}

// ffmpeg's output reaches its pipe (on Windows a named pipe), and one that fails before opening its
// output ends both readers instead of leaving them waiting.
func TestTranscodeOutputPipeWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=s=160x120:r=10:d=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")
	output, messages, err := transcodePipes(cmd)
	if err != nil {
		t.Fatal(err)
	}
	said := make(chan []byte)
	go func() { b, _ := io.ReadAll(messages); said <- b }()
	video, err := io.ReadAll(output)
	if err != nil {
		t.Fatal(err)
	}
	output.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, <-said)
	}
	<-said
	if len(video) < 1000 || string(video[4:8]) != "ftyp" {
		t.Errorf("output: %d bytes starting %q, want an MP4", len(video), video[:min(len(video), 8)])
	}

	cmd = exec.Command("ffmpeg", "-v", "error", "-i", filepath.Join(t.TempDir(), "missing.mkv"), "-f", "mp4", "pipe:1")
	if output, messages, err = transcodePipes(cmd); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() {
		io.ReadAll(output)
		io.ReadAll(messages)
		close(ended)
	}()
	select {
	case <-ended:
	case <-time.After(20 * time.Second):
		t.Fatal("the readers of an ffmpeg that failed before its output didn't end")
	}
	output.Close()
	if err := cmd.Wait(); err == nil {
		t.Error("ffmpeg with a missing input succeeded")
	}
}

func TestDVDLibLinesLeaveProbeJSON(t *testing.T) {
	out := []byte("{\nlibdvdread: CHECK_VALUE failed in ../src/nav_read.c:271 for dsi->dsi_gi.zero1 == 0\n    \"format\": {\"duration\": \"4173.1\"}\n}\n")
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(withoutDVDLibLines(out), &probe); err != nil || probe.Format.Duration != "4173.1" {
		t.Fatalf("duration %q, err %v", probe.Format.Duration, err)
	}
}
