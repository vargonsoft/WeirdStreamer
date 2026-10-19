// Package metadata looks up movie and TV information from TMDB for files in typed libraries.
package metadata

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	// "(1999)" or "[1999]": the most reliable year marker, so it wins over bare years.
	bracketedYear = regexp.MustCompile(`[(\[]((?:19|20)\d\d)[)\]]`)
	// A bare year, as in scene names ("I.Tonya.2017.1080p"); separators are checked by isBareYear.
	bareYear = regexp.MustCompile(`(?:19|20)\d\d`)
	// Release tags that end the title when no year is present.
	qualityTag   = regexp.MustCompile(`(?i)(?:^|[ ._-])(2160p|1080p|720p|480p|4k|uhd|bluray|blu-ray|bdrip|brrip|web-?dl|webrip|hdtv|dvdrip|dvd|x264|x265|h\.?264|h\.?265|hevc|remux)(?:$|[ ._-])`)
	bracketGroup = regexp.MustCompile(`[(\[][^)\]]*[)\]]`)

	episodeSxE  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,2})[ ._-]?e(\d{1,3})(?:-?e(\d{1,3}))?`)
	episodeNxNN = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:$|[^a-z0-9])`)
	seasonDir   = regexp.MustCompile(`(?i)^(?:season|series|s)[ ._-]*(\d{1,3})$`)

	nonAlnum = regexp.MustCompile(`[^\p{L}\p{N}]+`)

	countryTag = regexp.MustCompile(`(?i)[(\[](uk|us|usa|au|ca|nz|ie)[)\]]`)

	// " - Special Edition", " - Restored International Cut": edition labels after a dash. The dash
	// is required so titles like "The Final Cut" are left alone.
	editionTag = regexp.MustCompile(`(?i)\s+-\s+(?:special|collector'?s|anniversary|ultimate|extended|director'?s|theatrical|unrated|uncut|remastered|restored)\b.*$`)
	// Trailing part or disc markers: "Ice Age Disc1", "Title - pt1", "Title - a".
	partMarker = regexp.MustCompile(`(?i)(?:[ ._-]+(?:disc|disk|cd|pt)[ ._-]*\d{1,2}|\s+-\s+[a-z])$`)
	// A season in a show folder name: "Big Brother Sweden - Season 7".
	showSeasonSuffix = regexp.MustCompile(`(?i)\s+-\s+season\s+\d+$`)
)

// countryCodes maps folder country tags to TMDB origin_country codes.
var countryCodes = map[string]string{"uk": "GB", "us": "US", "usa": "US", "au": "AU", "ca": "CA", "nz": "NZ", "ie": "IE"}

// ParseShowFolder parses a TV show folder name such as "Bluey (2018)" or "Big Brother (UK)".
// country is an ISO code hint from a "(UK)"-style tag, or "".
func ParseShowFolder(name string) (title string, year int, country string) {
	if m := countryTag.FindStringSubmatch(name); m != nil {
		country = countryCodes[strings.ToLower(m[1])]
		name = countryTag.ReplaceAllString(name, " ")
	}
	name = showSeasonSuffix.ReplaceAllString(strings.TrimSpace(name), "")
	title, year = ParseTitleYear(name)
	return title, year, country
}

// ParseTitleYear extracts a title and release year from a movie or show folder or file name, such
// as "10 Things I Hate About You (1999) - 1080p" or "I.Tonya.2017.1080p.BluRay". Year is 0 when
// the name has none.
func ParseTitleYear(name string) (title string, year int) {
	name = stripVideoExt(name)

	cut := -1
	if m := lastMatch(bracketedYear, name); m != nil && m[0] > 0 {
		cut, year = m[0], atoi(name[m[2]:m[3]])
	} else {
		// Skip a year at the very start, which is part of the title ("1917.2019.1080p").
		for _, m := range bareYear.FindAllStringIndex(name, -1) {
			if m[0] > 0 && isBareYear(name, m[0], m[1]) {
				cut, year = m[0], atoi(name[m[0]:m[1]])
				break
			}
		}
	}
	if cut < 0 {
		if m := qualityTag.FindStringIndex(name); m != nil && m[0] > 0 {
			cut = m[0]
		}
	}
	if cut >= 0 {
		name = name[:cut]
	}
	title = cleanTitle(name)
	if stripped := cleanTitle(partMarker.ReplaceAllString(editionTag.ReplaceAllString(title, ""), "")); stripped != "" {
		title = stripped
	}
	return title, year
}

// ParseEpisode extracts the season and episode numbers from an episode file name, such as
// "Show - S01E02 - Title.mkv" or "Show - 4x13 - Title.mkv". For multi-episode files
// ("S01E01-E02") it returns the first episode.
func ParseEpisode(name string) (season, episode int, ok bool) {
	if m := episodeSxE.FindStringSubmatch(name); m != nil {
		return atoi(m[1]), atoi(m[2]), true
	}
	if m := episodeNxNN.FindStringSubmatch(name); m != nil {
		return atoi(m[1]), atoi(m[2]), true
	}
	return 0, 0, false
}

// ParseSeasonFolder recognizes season folder names: "Season 1", "Season 01", "S01", "Specials".
func ParseSeasonFolder(name string) (season int, ok bool) {
	if strings.EqualFold(strings.TrimSpace(name), "specials") {
		return 0, true
	}
	if m := seasonDir.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
		return atoi(m[1]), true
	}
	return 0, false
}

// NormalizeTitle reduces a title to lowercase words for matching and lookup keys:
// "Cats & Dogs: The Revenge" and "Cats & Dogs - The Revenge" normalize the same.
func NormalizeTitle(title string) string {
	title = foldAccents(strings.ToLower(title))
	title = strings.NewReplacer("'", "", "’", "", "&", " and ", "½", ".5", "¼", ".25", "¾", ".75").Replace(title)
	return strings.Join(strings.Fields(nonAlnum.ReplaceAllString(title, " ")), " ")
}

// accentFolds maps accented Latin letters to their base letter ("Fiancé" matches "Fiance").
var accentFolds = func() map[rune]rune {
	m := map[rune]rune{}
	for base, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćĉċč", 'd': "ďđ", 'e': "èéêëēĕėęě", 'g': "ĝğġģ", 'h': "ĥħ",
		'i': "ìíîïĩīĭįı", 'j': "ĵ", 'k': "ķ", 'l': "ĺļľŀł", 'n': "ñńņňŉ", 'o': "òóôõöøōŏő",
		'r': "ŕŗř", 's': "śŝşšș", 't': "ţťŧț", 'u': "ùúûüũūŭůűų", 'w': "ŵ", 'y': "ýÿŷ", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = base
		}
	}
	return m
}()

func foldAccents(s string) string {
	return strings.Map(func(r rune) rune {
		if base, ok := accentFolds[r]; ok {
			return base
		}
		return r
	}, s)
}

// LookupKey identifies one title/year query; files that parse to the same key share a lookup.
func LookupKey(title string, year int) string {
	key := NormalizeTitle(title)
	if year > 0 {
		key += "|" + strconv.Itoa(year)
	}
	return key
}

func cleanTitle(name string) string {
	name = bracketGroup.ReplaceAllString(name, " ")
	// Scene names use dots or underscores for spaces; keep dots in titles that have spaces ("I.T.").
	if !strings.Contains(name, " ") {
		name = strings.NewReplacer(".", " ", "_", " ").Replace(name)
	}
	name = strings.Join(strings.Fields(name), " ")
	return strings.Trim(name, " -._")
}

var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".wmv": true, ".flv": true,
	".webm": true, ".ts": true, ".mpg": true, ".mpeg": true, ".3gp": true, ".3gpp": true, ".mp2": true,
}

func stripVideoExt(name string) string {
	if ext := filepath.Ext(name); videoExts[strings.ToLower(ext)] {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// isBareYear reports whether name[start:end] stands alone between separators (or the end).
func isBareYear(name string, start, end int) bool {
	isSep := func(c byte) bool { return c == ' ' || c == '.' || c == '_' || c == '-' }
	return (start == 0 || isSep(name[start-1])) && (end == len(name) || isSep(name[end]))
}

func lastMatch(re *regexp.Regexp, s string) []int {
	all := re.FindAllStringSubmatchIndex(s, -1)
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
