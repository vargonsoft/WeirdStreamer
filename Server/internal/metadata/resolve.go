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
	return tvRef(lib, title, year, country)
}

func tvRef(lib config.MediaPath, title string, year int, country string) (Ref, bool) {
	if title == "" {
		return Ref{}, false
	}
	key := LookupKey(title, year)
	if country != "" {
		key += "|" + strings.ToLower(country)
	}
	return Ref{Kind: config.LibraryTypeTV, LibraryID: lib.ID, Key: key, Title: title, Year: year, Country: country}, true
}

// episodeShow decides which show an episode file belongs to, from the show named in the file and
// the show its top-level folder names, and returns the other one as a fallback:
//   - the same show: one ref, with year and country from whichever name has them (the file's win)
//   - one name's words contain the other's: the more specific name, keeping the other's year and
//     country ("Big Brother Sweden/big brother - S07E01" stays Big Brother Sweden)
//   - unrelated names: the file's, since the file name is the source of truth
func episodeShow(lib config.MediaPath, root, path string) (primary, fallback *Ref, ok bool) {
	file, fileOK, folder, folderOK := episodeShowRefs(lib, root, path)
	switch {
	case !fileOK && !folderOK:
		return nil, nil, false
	case !fileOK:
		return &folder, nil, true
	case !folderOK:
		return &file, nil, true
	}
	fileWords, folderWords := NormalizeTitle(file.Title), NormalizeTitle(folder.Title)
	switch {
	case fileWords == folderWords:
		merged := mergeShowRefs(lib, file, folder)
		return &merged, nil, true
	case wordsWithin(fileWords, folderWords):
		primary, fallback := mergeShowRefs(lib, folder, file), mergeShowRefs(lib, file, folder)
		return &primary, &fallback, true
	case wordsWithin(folderWords, fileWords):
		primary := mergeShowRefs(lib, file, folder)
		return &primary, &folder, true
	}
	return &file, &folder, true
}

// mergeShowRefs returns base's title with year and country from base, else from other.
func mergeShowRefs(lib config.MediaPath, base, other Ref) Ref {
	year, country := base.Year, base.Country
	if year == 0 {
		year = other.Year
	}
	if country == "" {
		country = other.Country
	}
	ref, _ := tvRef(lib, base.Title, year, country)
	return ref
}

// wordsWithin reports whether every word of a also appears in b.
func wordsWithin(a, b string) bool {
	words := map[string]bool{}
	for _, w := range strings.Fields(b) {
		words[w] = true
	}
	for _, w := range strings.Fields(a) {
		if !words[w] {
			return false
		}
	}
	return true
}

// episodeShowRefs returns the show an episode file belongs to according to its own name (fileOK)
// and according to its top-level folder (folderOK).
func episodeShowRefs(lib config.MediaPath, root, path string) (file Ref, fileOK bool, folder Ref, folderOK bool) {
	if title, year, country, ok := ParseEpisodeShow(filepath.Base(path)); ok {
		file, fileOK = tvRef(lib, title, year, country)
	}
	if filepath.Dir(path) != root {
		folder, folderOK = showRef(lib, root, path)
	}
	return
}

// ResolveFile maps a video file to its title. For movies the file name is the source of truth;
// its folder only supplies a missing year when the folder names the same title
// ("Inception (2010)/Inception.mkv").
func ResolveFile(lib config.MediaPath, path string) (Ref, bool) {
	root, _ := filepath.Abs(lib.Path)
	switch lib.Type {
	case config.LibraryTypeMovie:
		title, year := ParseTitleYear(filepath.Base(path))
		if title == "" {
			return Ref{}, false
		}
		if parent := filepath.Dir(path); year == 0 && parent != root {
			if folderTitle, folderYear := ParseTitleYear(filepath.Base(parent)); folderYear > 0 &&
				NormalizeTitle(folderTitle) == NormalizeTitle(title) {
				year = folderYear
			}
		}
		return Ref{Kind: config.LibraryTypeMovie, LibraryID: lib.ID, Key: LookupKey(title, year), Title: title, Year: year}, true
	case config.LibraryTypeTV:
		// The show named in the episode file wins; its folder only adds a year or country tag when
		// it names the same show ("Bluey (2018)/Bluey - S01E01.mkv"), or stands in when the file
		// doesn't name one. Loose files in the library root and mixed folders work this way.
		show, _, ok := episodeShow(lib, root, path)
		if !ok {
			return Ref{}, false
		}
		ref := *show
		ref.Season, ref.Episode, ref.IsEpisode = ParseEpisode(filepath.Base(path))
		return ref, true
	}
	return Ref{}, false
}

// ResolveFolder maps a folder to the title it stands for: a movie folder whose videos are all one
// title (folderKey, recorded by a sync or discovery), a TV show folder, or a season folder inside
// one (Season is set; IsEpisode stays false). Movie folder refs carry only the key: the title
// comes from the files.
func ResolveFolder(lib config.MediaPath, path, folderKey string) (Ref, bool) {
	root, _ := filepath.Abs(lib.Path)
	switch lib.Type {
	case config.LibraryTypeMovie:
		if folderKey == "" {
			return Ref{}, false
		}
		return Ref{Kind: config.LibraryTypeMovie, LibraryID: lib.ID, Key: folderKey}, true
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
