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

// ShowFolderOf returns the top-level folder of a TV library path (its show folder), or "" for
// the library root itself.
func ShowFolderOf(lib config.MediaPath, path string) string {
	root, _ := filepath.Abs(lib.Path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.Join(root, strings.SplitN(filepath.ToSlash(rel), "/", 2)[0])
}

// mostCommonKey returns the key seen most often (ties broken by key, for stable results).
func mostCommonKey(counts map[string]int) string {
	best, bestN := "", 0
	for k, n := range counts {
		if n > bestN || (n == bestN && k < best) {
			best, bestN = k, n
		}
	}
	return best
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
	case fileWords == folderWords || sameApartFromNumbers(fileWords, folderWords):
		// Extra numbers are season numbers, not part of the show name ("Big Brother 7 UK").
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

// sameApartFromNumbers reports whether one title is the other plus only numeric words.
func sameApartFromNumbers(a, b string) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	if a == "" || !wordsWithin(a, b) {
		return false
	}
	words := map[string]bool{}
	for _, w := range strings.Fields(a) {
		words[w] = true
	}
	for _, w := range strings.Fields(b) {
		if !words[w] && strings.Trim(w, "0123456789") != "" {
			return false
		}
	}
	return true
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

// extraKeyPrefix starts the lookup keys of single TV-library files that aren't episodes (a TV
// movie or special in a show folder). Only manual fixes create such lookups.
const extraKeyPrefix = "file|"

// IsExtraKey reports whether a lookup key stands for one non-episode TV file.
func IsExtraKey(key string) bool {
	return strings.HasPrefix(key, extraKeyPrefix)
}

// ExtraRef is the ref a TV-library video that isn't an episode can be fixed by: its own lookup,
// keyed by its path in the library, searched by its file name.
func ExtraRef(lib config.MediaPath, path string) (Ref, bool) {
	if lib.Type != config.LibraryTypeTV {
		return Ref{}, false
	}
	if ref, ok := ResolveFile(lib, path); ok && ref.IsEpisode {
		return Ref{}, false
	}
	root, _ := filepath.Abs(lib.Path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return Ref{}, false
	}
	title, year := ParseTitleYear(filepath.Base(path))
	return Ref{Kind: config.LibraryTypeTV, LibraryID: lib.ID, Key: extraKeyPrefix + filepath.ToSlash(rel),
		Title: title, Year: year}, true
}

// ResolveFolder maps a folder to the title it stands for: a movie folder whose videos are all one
// title, a TV show folder, or a season folder inside one (Season is set; IsEpisode stays false).
// folderKey is what a sync or discovery recorded from the files: for movies, the folder's own
// key; for TV, the key of the show folder the path is in. Such refs carry only the key, since
// the title comes from the files. A TV folder without one falls back to its folder name.
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
		if folderKey != "" {
			ref = Ref{Kind: config.LibraryTypeTV, LibraryID: lib.ID, Key: folderKey}
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

func skipEntry(d os.DirEntry) bool {
	name := d.Name()
	if strings.HasPrefix(name, ".") || strings.EqualFold(name, "@eaDir") {
		return true
	}
	lower := strings.ToLower(name)
	return !d.IsDir() && (strings.Contains(lower, "sample") || strings.Contains(lower, "trailer"))
}
