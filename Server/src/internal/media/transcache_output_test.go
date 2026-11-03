package media

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
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

// What a library prints to stdout during a transcode stays out of the stream.
func TestTranscodeOutputHasItsOwnPipe(t *testing.T) {
	cmd := exec.Command("sh", "-c", `echo "libdvdread: CHECK_VALUE failed"; printf video >&3`, "sh", "pipe:1")
	output, messages, err := transcodePipes(cmd)
	if err != nil {
		t.Fatal(err)
	}
	video, _ := io.ReadAll(output)
	said, _ := io.ReadAll(messages)
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if string(video) != "video" {
		t.Errorf("output %q, want only the video", video)
	}
	if !strings.Contains(string(said), "CHECK_VALUE") {
		t.Errorf("messages %q lack the library's line", said)
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
