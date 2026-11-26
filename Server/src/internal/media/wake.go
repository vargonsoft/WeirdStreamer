package media

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/fileio"
)

// A share's folder can answer "permission denied" while the NAS behind it has let it go to sleep
// (a disk spun down or unmounted there), and asking about the folder itself doesn't bring it back.
// Listing the folder it's in does: the NAS looks at each entry and wakes it. StatWaking and
// CheckReadable do that before giving up.

const wakeEvery = 30 * time.Second // at most one wake-up try per path in this time

var (
	wakeMu    sync.Mutex
	wakeTried = map[string]time.Time{}
	wakeState = map[string]string{} // path → the last error logged for it ("" once readable)
)

// StatWaking is os.Stat, waking the share first when the path is refused or fails to read.
func StatWaking(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil && Wake(path, err) {
		info, err = os.Stat(path)
	}
	noteReadable(path, err)
	return info, err
}

// Wake lists the folders above path, nearest first, until path can be looked at again, when err
// (from reading it) is one that a sleeping share gives. Returns whether that worked.
func Wake(path string, err error) bool {
	if !asleep(err) {
		return false
	}
	wakeMu.Lock()
	if time.Since(wakeTried[path]) < wakeEvery {
		wakeMu.Unlock()
		return false
	}
	wakeTried[path] = time.Now()
	wakeMu.Unlock()

	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if f, err := fileio.Open(dir); err == nil {
			f.Readdirnames(-1)
			f.Close()
			if _, err := os.Stat(path); err == nil {
				log.Printf("[LIBRARY] Woke %s by listing %s", path, dir)
				return true
			}
		}
		if next := filepath.Dir(dir); next == dir {
			return false
		}
	}
}

// asleep reports whether err is what a share gives for a folder that's asleep, rather than gone.
func asleep(err error) bool {
	return err != nil && !errors.Is(err, os.ErrNotExist)
}

// noteReadable logs a path that can't be read, once until it changes, and when it's back.
func noteReadable(path string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	wakeMu.Lock()
	last, seen := wakeState[path]
	wakeState[path] = msg
	wakeMu.Unlock()
	switch {
	case msg != "" && msg != last:
		log.Printf("[LIBRARY] Can't read %s: %v", path, err)
	case msg == "" && seen && last != "":
		log.Printf("[LIBRARY] %s can be read again", path)
	}
}
