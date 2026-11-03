package media

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckReadable(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	empty := filepath.Join(dir, "empty.mp4")
	os.WriteFile(video, []byte("data"), 0o644)
	os.WriteFile(empty, nil, 0o644)
	locked := filepath.Join(dir, "locked")
	os.Mkdir(locked, 0o755)
	inLocked := filepath.Join(locked, "video.mp4")
	os.WriteFile(inLocked, []byte("data"), 0o644)
	os.Chmod(locked, 0)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	if p := CheckReadable(video); p != nil {
		t.Errorf("readable file: %v", p)
	}
	if p := CheckReadable(empty); p != nil {
		t.Errorf("empty file: %v", p)
	}
	if p := CheckReadable(filepath.Join(dir, "gone.mp4")); p == nil || p.Status != http.StatusNotFound {
		t.Errorf("missing file: %v", p)
	}
	if os.Geteuid() != 0 { // root reads it anyway
		if p := CheckReadable(inLocked); p == nil || p.Status != http.StatusServiceUnavailable {
			t.Errorf("refused folder: %v", p)
		}
	}
}
