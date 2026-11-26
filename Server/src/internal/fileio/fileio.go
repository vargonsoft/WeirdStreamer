// Package fileio opens, reads, renames and serves files so that a file the server has open can
// still be renamed, replaced or deleted, on Windows as on Unix: a video someone is watching that
// another program upgrades, a thumbnail remade while it's being sent, a transcode's chunk removed
// while a reader has it. On Unix these are the os package's own calls. Windows refuses to rename or
// delete a file that's open unless every handle to it allows that (os.Open's don't), and to rename
// a file over one that's open, so there the files are opened sharing delete access and renamed with
// POSIX semantics.
package fileio

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Open opens a file for reading.
func Open(name string) (*os.File, error) {
	return OpenFile(name, os.O_RDONLY, 0)
}

// Create creates or truncates a file, as os.Create.
func Create(name string) (*os.File, error) {
	return OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// ReadFile reads a whole file, as os.ReadFile.
func ReadFile(name string) ([]byte, error) {
	f, err := Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	size := 0
	if info, err := f.Stat(); err == nil && info.Size() < 1<<30 {
		size = int(info.Size())
	}
	buf := bytes.NewBuffer(make([]byte, 0, size+512))
	_, err = buf.ReadFrom(f)
	return buf.Bytes(), err
}

// CrossDevice tells whether a rename failed because the two paths are on different disks (a copy
// would do it), and not for another reason, such as the file being in use.
func CrossDevice(err error) bool {
	return crossDevice(err)
}

// InUse tells whether a rename or delete failed because another program has the file open without
// allowing that (Windows only: ffmpeg, a player, an editor). Trying again once it's closed works.
func InUse(err error) bool {
	return inUse(err)
}

// ServeFile answers a request with a file, as http.ServeFile does for a file (not a folder).
func ServeFile(w http.ResponseWriter, r *http.Request, name string) {
	f, err := Open(name)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			http.Error(w, "404 page not found", http.StatusNotFound)
		case errors.Is(err, fs.ErrPermission):
			http.Error(w, "403 Forbidden", http.StatusForbidden)
		default:
			http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		}
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, "404 page not found", http.StatusNotFound)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

// Dir is http.Dir with files opened by Open.
type Dir string

func (d Dir) Open(name string) (http.File, error) {
	if filepath.Separator != '/' && strings.ContainsRune(name, filepath.Separator) {
		return nil, errors.New("http: invalid character in file path")
	}
	dir := string(d)
	if dir == "" {
		dir = "."
	}
	f, err := Open(filepath.Join(dir, filepath.FromSlash(path.Clean("/"+name))))
	if err != nil {
		return nil, err
	}
	return f, nil
}
