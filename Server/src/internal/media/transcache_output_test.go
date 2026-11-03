package media

import (
	"io"
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
