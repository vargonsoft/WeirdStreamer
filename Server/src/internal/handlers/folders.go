package handlers

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/weirdstreamer/server/internal/auth"
	"github.com/weirdstreamer/server/internal/config"
)

// folderListing is a folder on the server's disk, for picking a library's folder in Settings.
type folderListing struct {
	Path    string        `json:"path"`              // "" for Windows' list of drives
	Parent  string        `json:"parent"`            // the folder Up opens ("" is the top)
	Top     bool          `json:"top"`               // nothing above it: "/", or Windows' drives
	Folders []folderEntry `json:"folders"`           // without hidden ones
	Videos  int           `json:"videos"`            // video files directly in it
	Library string        `json:"library,omitempty"` // the library that is this folder already
}

// folderEntry is a folder in a listing: its name and its full path (clients never join paths:
// Windows' are another shape).
type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListFolders lists a folder of the server's disk, for admins choosing a library's folder:
// GET /api/config/folders?path=<absolute path> (none: the top: / or, on Windows, the drives), with
// parent=1 its parent instead (the picker opens beside the last library). Hidden folders are left
// out; typing one's path still lists it.
func (h *ConfigHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	path := r.URL.Query().Get("path")
	if r.URL.Query().Get("parent") == "1" && filepath.IsAbs(path) {
		path = parentFolder(filepath.Clean(path))
	}
	listing, err := listFolder(path)
	switch {
	case err == nil:
		respondJSON(w, http.StatusOK, listing)
	case errors.Is(err, fs.ErrNotExist):
		respondError(w, http.StatusNotFound, "No such folder")
	case errors.Is(err, fs.ErrPermission):
		respondError(w, http.StatusForbidden, "The server isn't allowed to read this folder")
	default:
		respondError(w, http.StatusBadRequest, err.Error())
	}
}

func listFolder(path string) (*folderListing, error) {
	if path == "" && topIsDrives {
		return &folderListing{Top: true, Folders: drives()}, nil
	}
	if path == "" {
		path = string(filepath.Separator)
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("Give the folder's full path, " + absoluteExample)
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("That's a file, not a folder")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	listing := &folderListing{Path: path, Parent: parentFolder(path), Folders: []folderEntry{}}
	listing.Top = listing.Parent == "" && !topIsDrives
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(path, name)
		if strings.HasPrefix(name, ".") || hiddenFolder(full) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 { // a link to a folder counts as one
			if info, err := os.Stat(full); err == nil {
				isDir = info.IsDir()
			}
		}
		if isDir {
			listing.Folders = append(listing.Folders, folderEntry{Name: name, Path: full})
		} else if config.IsVideo(full) {
			listing.Videos++
		}
	}
	sort.Slice(listing.Folders, func(i, j int) bool {
		return strings.ToLower(listing.Folders[i].Name) < strings.ToLower(listing.Folders[j].Name)
	})
	if config.GlobalConfig != nil {
		for _, lib := range config.GlobalConfig.Media.Paths {
			if root, err := filepath.Abs(lib.Path); err == nil && config.SamePath(root, path) {
				listing.Library = lib.Name
				break
			}
		}
	}
	return listing, nil
}

// parentFolder is the folder above path (clean and absolute), or "" at a root (/, a drive's, a
// network share's): Windows' top then is its drives.
func parentFolder(path string) string {
	parent := filepath.Dir(path)
	if parent == path {
		return ""
	}
	return parent
}
