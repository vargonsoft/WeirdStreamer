// Package logs keeps the server's own log files beside the one live.sh writes (server-*.log,
// everything the server prints): auth.log for sign-ins and refused tokens, and access.log with a
// line per HTTP request. Each grows to maxSize, then moves to .1 (.1 to .2, and so on) and starts
// over, and the last five are kept.
package logs

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	maxSize = 10 << 20
	keep    = 5
)

var (
	authLog   = log.New(os.Stdout, "[AUTH] ", log.LstdFlags)
	accessLog io.Writer // nil: requests aren't logged
)

// Open sends the auth and access logs to files in dir. Without a dir (or if it can't be made),
// auth lines go to the server's output and requests aren't logged.
func Open(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	auth, err := openRotating(filepath.Join(dir, "auth.log"))
	if err != nil {
		return err
	}
	access, err := openRotating(filepath.Join(dir, "access.log"))
	if err != nil {
		return err
	}
	authLog = log.New(auth, "", log.LstdFlags)
	accessLog = access
	opened = []*rotating{auth, access}
	return nil
}

// opened are the files Open opened, for Close.
var opened []*rotating

// Close closes the log files: auth lines go to the server's output again and requests aren't
// logged. (Windows can't remove a folder holding an open file.)
func Close() {
	authLog = log.New(os.Stdout, "[AUTH] ", log.LstdFlags)
	accessLog = nil
	for _, r := range opened {
		r.Close()
	}
	opened = nil
}

// Auth writes a line to the auth log.
func Auth(format string, args ...any) {
	authLog.Printf(format, args...)
}

// AuthOnce writes a line to the auth log unless the same key was written in the last hour: an
// expired token is refused on every request (a page's thumbnails, a player's reports) until the
// client signs in again.
func AuthOnce(key, format string, args ...any) {
	onceMu.Lock()
	now := time.Now()
	if last, ok := onceSeen[key]; ok && now.Sub(last) < time.Hour {
		onceMu.Unlock()
		return
	}
	if len(onceSeen) > 1000 {
		for k, t := range onceSeen {
			if now.Sub(t) >= time.Hour {
				delete(onceSeen, k)
			}
		}
	}
	onceSeen[key] = now
	onceMu.Unlock()
	Auth(format, args...)
}

var (
	onceMu   sync.Mutex
	onceSeen = map[string]time.Time{}
)

// rotating is a log file that moves aside when it reaches maxSize.
type rotating struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func openRotating(path string) (*rotating, error) {
	r := &rotating{path: path}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotating) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.file, r.size = f, info.Size()
	return nil
}

func (r *rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(p)) > maxSize && r.size > 0 {
		r.rotate()
	}
	if r.file == nil {
		return 0, fmt.Errorf("log %s isn't open", r.path)
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotating) rotate() {
	r.file.Close()
	r.file = nil
	os.Remove(r.path + "." + strconv.Itoa(keep))
	for n := keep - 1; n >= 1; n-- {
		os.Rename(r.path+"."+strconv.Itoa(n), r.path+"."+strconv.Itoa(n+1))
	}
	os.Rename(r.path, r.path+".1")
	if err := r.open(); err != nil {
		log.Printf("Can't reopen %s: %v", r.path, err)
	}
}
