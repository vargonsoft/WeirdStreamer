package metadata

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/weirdstreamer/server/internal/config"
)

// Ref identifies the title a file or folder belongs to, as derived from its path.
type Ref struct {
	Kind      string // config.LibraryTypeMovie or config.LibraryTypeTV
	LibraryID string
	Key       string
	Title     string
	Year      int
	Country   string // tv only: origin country hint
	Season    int    // tv only
	Episode   int    // tv only
	IsEpisode bool   // tv file with a parsed season/episode
}

// showRef derives the show a TV library path belongs to from its top-level folder.
func showRef(lib config.MediaPath, root, path string) (Ref, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return Ref{}, false
	}
	showFolder := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
	title, year, country := ParseShowFolder(showFolder)
	if title == "" {
		return Ref{}, false
	}
	key := LookupKey(title, year)
	if country != "" {
		key += "|" + strings.ToLower(country)
	}
	return Ref{Kind: config.LibraryTypeTV, LibraryID: lib.ID, Key: key, Title: title, Year: year, Country: country}, true
}

// ResolveFile maps a video file to its title. folderKey is the key recorded for the file's
// parent folder by a library sync (movie libraries only), or "".
func ResolveFile(lib config.MediaPath, path, folderKey string) (Ref, bool) {
	root, _ := filepath.Abs(lib.Path)
	switch lib.Type {
	case config.LibraryTypeMovie:
		parent := filepath.Dir(path)
		name := filepath.Base(path)
		if folderKey != "" && parent != root {
			name = filepath.Base(parent)
		}
		title, year := ParseTitleYear(name)
		if title == "" {
			return Ref{}, false
		}
		return Ref{Kind: config.LibraryTypeMovie, LibraryID: lib.ID, Key: LookupKey(title, year), Title: title, Year: year}, true
	case config.LibraryTypeTV:
		ref, ok := showRef(lib, root, path)
		if !ok || filepath.Dir(path) == root {
			return Ref{}, false
		}
		ref.Season, ref.Episode, ref.IsEpisode = ParseEpisode(filepath.Base(path))
		return ref, true
	}
	return Ref{}, false
}

// ResolveFolder maps a folder to the title it stands for: a movie folder with a recorded key, a
// TV show folder, or a season folder inside one (Season is set; IsEpisode stays false).
func ResolveFolder(lib config.MediaPath, path, folderKey string) (Ref, bool) {
	root, _ := filepath.Abs(lib.Path)
	switch lib.Type {
	case config.LibraryTypeMovie:
		if folderKey == "" {
			return Ref{}, false
		}
		title, year := ParseTitleYear(filepath.Base(path))
		return Ref{Kind: config.LibraryTypeMovie, LibraryID: lib.ID, Key: folderKey, Title: title, Year: year}, true
	case config.LibraryTypeTV:
		ref, ok := showRef(lib, root, path)
		if !ok {
			return Ref{}, false
		}
		if filepath.Dir(path) != root {
			season, isSeason := ParseSeasonFolder(filepath.Base(path))
			if !isSeason {
				return Ref{}, false
			}
			ref.Season = season
		}
		return ref, true
	}
	return Ref{}, false
}

// isVideo reports whether name has one of the configured video extensions.
func isVideo(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, v := range config.GlobalConfig.Media.SupportedVideoFormats {
		if ext == v {
			return true
		}
	}
	return false
}

func skipEntry(d os.DirEntry) bool {
	name := d.Name()
	if strings.HasPrefix(name, ".") || strings.EqualFold(name, "@eaDir") {
		return true
	}
	lower := strings.ToLower(name)
	return !d.IsDir() && (strings.Contains(lower, "sample") || strings.Contains(lower, "trailer"))
}
