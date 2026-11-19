package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTransportStreamsMustLookLikeOne(t *testing.T) {
	GlobalConfig = &Config{Media: MediaConfig{SupportedVideoFormats: []string{".ts", ".m2ts", ".mkv"}}}
	defer func() { GlobalConfig = nil }()
	dir := t.TempDir()
	packets := func(offset, size, n int) []byte {
		b := make([]byte, size*n)
		for i := 0; i < n; i++ {
			b[i*size+offset] = 0x47
		}
		return b
	}
	files := map[string][]byte{
		"video.ts":        packets(0, 188, 4),
		"bluray.m2ts":     packets(4, 192, 4),
		"tiny.ts":         {0x47},
		"web-tree.d.ts":   []byte("export declare class Parser {\n}\n"),
		"empty.ts":        {},
		"near-miss.ts":    append([]byte{0x47}, bytes.Repeat([]byte{'a'}, 400)...),
		"not-checked.mkv": []byte("anything"),
	}
	for name, data := range files {
		os.WriteFile(filepath.Join(dir, name), data, 0o644)
	}
	want := map[string]string{"video.ts": "video", "bluray.m2ts": "video", "tiny.ts": "video",
		"web-tree.d.ts": "", "empty.ts": "", "near-miss.ts": "", "not-checked.mkv": "video",
		"missing.ts": "video"}
	for name, w := range want {
		if got := MediaType(filepath.Join(dir, name)); got != w {
			t.Errorf("%s: %q, want %q", name, got, w)
		}
	}
	if got := MediaType("web-tree.d.ts"); got != "video" {
		t.Errorf("a bare name: %q, want video (by its extension)", got)
	}
	// A file that changes is looked at again.
	p := filepath.Join(dir, "web-tree.d.ts")
	os.WriteFile(p, packets(0, 188, 4), 0o644)
	os.Chtimes(p, timeAfter(p), timeAfter(p))
	if got := MediaType(p); got != "video" {
		t.Errorf("after becoming a transport stream: %q", got)
	}
}

func timeAfter(path string) time.Time {
	info, _ := os.Stat(path)
	return info.ModTime().Add(2 * time.Second)
}
