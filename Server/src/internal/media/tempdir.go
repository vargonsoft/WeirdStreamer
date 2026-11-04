package media

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Short-lived work files (HEIC photos converted to JPEG, a frame checked while making trickplay)
// go in the folder media.tempPath names: the system's temp folder is often small or in RAM.
var tempDir struct {
	mu  sync.Mutex
	dir string
}

// tempPrefix starts the name of every work file, so clearing the folder at startup removes only
// the server's own files, wherever it is (even /tmp).
const tempPrefix = "weirdstreamer-"

// SetTempDir sets the folder for work files, creating it and removing any the server left there.
func SetTempDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tempDir.mu.Lock()
	tempDir.dir = dir
	tempDir.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), tempPrefix) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// TempDir is the folder for work files: media.tempPath, else the system's.
func TempDir() string {
	tempDir.mu.Lock()
	defer tempDir.mu.Unlock()
	if tempDir.dir != "" {
		return tempDir.dir
	}
	return os.TempDir()
}

// tempFileName makes an empty work file and returns its name, for a tool that writes to a path
// (heif-convert, ffmpeg). pattern is as os.CreateTemp's; the caller removes the file.
func tempFileName(pattern string) (string, error) {
	f, err := os.CreateTemp(TempDir(), tempPrefix+pattern)
	if err != nil {
		return "", err
	}
	f.Close()
	return f.Name(), nil
}
