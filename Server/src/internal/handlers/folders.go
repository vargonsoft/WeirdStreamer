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
	Path    string   `json:"path"`
	Parent  string   `json:"parent,omitempty"` // "" at the top
	Folders []string `json:"folders"`          // names, without hidden ones (".name")
	Videos  int      `json:"videos"`           // video files directly in it
}

// ListFolders lists a folder of the server's disk, for admins choosing a library's folder:
// GET /api/config/folders?path=<absolute path> (none: the top, /). Hidden folders are left out;
// typing one's path still lists it.
func (h *ConfigHandler) ListFolders(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || !claims.IsAdmin {
		respondError(w, http.StatusForbidden, "Admin access required")
		return
	}
	listing, err := listFolder(r.URL.Query().Get("path"))
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
	if path == "" {
		path = "/"
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("Give the folder's full path, starting with /")
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
	listing := &folderListing{Path: path, Folders: []string{}}
	if path != "/" {
		listing.Parent = filepath.Dir(path)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 { // a link to a folder counts as one
			if info, err := os.Stat(filepath.Join(path, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		if isDir {
			listing.Folders = append(listing.Folders, name)
		} else if config.IsVideo(name) {
			listing.Videos++
		}
	}
	sort.Slice(listing.Folders, func(i, j int) bool {
		return strings.ToLower(listing.Folders[i]) < strings.ToLower(listing.Folders[j])
	})
	return listing, nil
}
