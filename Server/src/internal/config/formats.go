package config

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// MediaType is what a file is by its extension and the configured formats (media's
// supportedVideoFormats, supportedImageFormats and supportedDocFormats): "video", "image",
// "document", or "" for a file the server doesn't serve.
func MediaType(path string) string {
	if GlobalConfig == nil {
		return ""
	}
	m := GlobalConfig.Media
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == "":
		return ""
	case slices.Contains(m.SupportedVideoFormats, ext):
		if transportStreamExts[ext] && !looksLikeTransportStream(path) {
			return "" // a TypeScript .ts file, say
		}
		return "video"
	case slices.Contains(m.SupportedImageFormats, ext):
		return "image"
	case slices.Contains(m.SupportedDocFormats, ext):
		return "document"
	}
	return ""
}

// IsVideo reports whether a file has one of the configured video extensions.
func IsVideo(path string) bool { return MediaType(path) == "video" }

// MPEG-TS extensions, which other files share (TypeScript's .ts). A file of one counts as a video
// only if it starts like a transport stream.
var transportStreamExts = map[string]bool{".ts": true, ".m2ts": true, ".mts": true}

type tsCheck struct {
	size    int64
	modTime time.Time
	ok      bool
}

var tsChecks sync.Map // path → tsCheck

// looksLikeTransportStream reports whether the file at path starts with MPEG-TS packets: the sync
// byte 0x47 every 188 bytes (TS), or every 192 bytes after a 4-byte timestamp (M2TS, Blu-ray). A
// bare name (callers that only have the name) or a file that can't be read counts as one, as its
// extension says; a file that reads and doesn't is something else. Remembered per path until the
// file changes, so listings read each file once.
func looksLikeTransportStream(path string) bool {
	if !strings.ContainsRune(path, filepath.Separator) {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	if c, ok := tsChecks.Load(path); ok {
		if c := c.(tsCheck); c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
			return c.ok
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()
	head := make([]byte, 2*192+4)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return true
	}
	ok := syncBytes(head[:n], 0, 188) || syncBytes(head[:n], 4, 192)
	tsChecks.Store(path, tsCheck{info.Size(), info.ModTime(), ok})
	return ok
}

// syncBytes reports whether head has 0x47 at offset and, if it's long enough, a packet later.
func syncBytes(head []byte, offset, packet int) bool {
	if len(head) <= offset || head[offset] != 0x47 {
		return false
	}
	return len(head) <= offset+packet || head[offset+packet] == 0x47
}
