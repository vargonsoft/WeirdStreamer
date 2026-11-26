package media

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/fileio"
	"github.com/weirdstreamer/server/internal/models"
)

// livePhotoStills are the photos an iPhone saves a Live Photo's still as.
var livePhotoStills = map[string]bool{".heic": true, ".heif": true, ".jpg": true, ".jpeg": true}

// PairLivePhotos finds Apple Live Photos in a folder's files: an iPhone saves each as a photo and
// a short .MOV of the same name (IMG_1234.HEIC and IMG_1234.MOV), and marks the video as a Live
// Photo's (IsLivePhotoVideo). The photo gets the video as its LiveVideo, and the video leaves the
// list, so the pair shows once. Any other .MOV stays, and everything is kept in order.
func PairLivePhotos(items []models.MediaItem) []models.MediaItem {
	return pairLivePhotos(items, IsLivePhotoVideo)
}

func pairLivePhotos(items []models.MediaItem, isLive func(path string) bool) []models.MediaItem {
	key := func(it models.MediaItem) string {
		return strings.ToLower(strings.TrimSuffix(it.Name, filepath.Ext(it.Name)))
	}
	movs := map[string]int{}
	for i, it := range items {
		if it.Type == "video" && strings.EqualFold(filepath.Ext(it.Name), ".mov") {
			movs[key(it)] = i
		}
	}
	if len(movs) == 0 {
		return items
	}
	paired := map[int]bool{}
	for i := range items {
		it := &items[i]
		if it.Type != "image" || !livePhotoStills[strings.ToLower(filepath.Ext(it.Name))] {
			continue
		}
		if j, ok := movs[key(*it)]; ok && !paired[j] && isLive(items[j].Path) {
			it.LiveVideo = items[j].Path
			paired[j] = true
		}
	}
	if len(paired) == 0 {
		return items
	}
	kept := make([]models.MediaItem, 0, len(items)-len(paired))
	for i, it := range items {
		if !paired[i] {
			kept = append(kept, it)
		}
	}
	return kept
}

// IsLivePhotoPartner reports whether a video is the .MOV half of an Apple Live Photo: a photo of its
// name is beside it and it's marked as a Live Photo's. Folder listings show it with its photo
// (PairLivePhotos), and lists of videos leave it out.
func IsLivePhotoPartner(path string) bool {
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".mov") {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(path), ext)
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if livePhotoStills[strings.ToLower(filepath.Ext(name))] && strings.EqualFold(strings.TrimSuffix(name, filepath.Ext(name)), base) {
			return IsLivePhotoVideo(path)
		}
	}
	return false
}

// The key an iPhone writes in a Live Photo's video, holding the identifier its photo shares.
var livePhotoKey = []byte("com.apple.quicktime.content.identifier")

type liveCheck struct {
	size    int64
	modTime time.Time
	live    bool
}

var (
	liveChecksMu sync.Mutex
	liveChecks   = map[string]liveCheck{}
)

// IsLivePhotoVideo reports whether a QuickTime file is a Live Photo's video: its index (the moov
// box) has Apple's content identifier. Results are kept until the file changes.
func IsLivePhotoVideo(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	liveChecksMu.Lock()
	c, ok := liveChecks[path]
	liveChecksMu.Unlock()
	if ok && c.size == st.Size() && c.modTime.Equal(st.ModTime()) {
		return c.live
	}
	live := false
	if f, err := fileio.Open(path); err == nil {
		live = bytes.Contains(readMoov(f, st.Size()), livePhotoKey)
		f.Close()
	}
	liveChecksMu.Lock()
	liveChecks[path] = liveCheck{st.Size(), st.ModTime(), live}
	liveChecksMu.Unlock()
	return live
}

// readMoov walks a QuickTime file's top-level boxes to its moov box (at the start or the end) and
// returns it; nil if there's none or it's implausibly big for a short clip.
func readMoov(r io.ReaderAt, size int64) []byte {
	const maxMoov = 8 << 20
	head := make([]byte, 16)
	for pos := int64(0); pos+8 <= size; {
		if _, err := r.ReadAt(head[:8], pos); err != nil {
			return nil
		}
		boxSize, headerSize := int64(binary.BigEndian.Uint32(head)), int64(8)
		switch boxSize {
		case 0: // to the end of the file
			boxSize = size - pos
		case 1: // a 64-bit size follows
			if _, err := r.ReadAt(head[8:16], pos+8); err != nil {
				return nil
			}
			boxSize, headerSize = int64(binary.BigEndian.Uint64(head[8:])), 16
		}
		if boxSize < headerSize || pos+boxSize > size {
			return nil
		}
		if string(head[4:8]) == "moov" {
			if boxSize > maxMoov {
				return nil
			}
			moov := make([]byte, boxSize-headerSize)
			if _, err := r.ReadAt(moov, pos+headerSize); err != nil {
				return nil
			}
			return moov
		}
		pos += boxSize
	}
	return nil
}
