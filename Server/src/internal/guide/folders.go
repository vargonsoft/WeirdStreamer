package guide

import (
	pathpkg "path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A folder library (no TMDB) read for typed channels (docs/GUIDE.md, "Typed channels"): a folder whose
// videos mostly share a name apart from their numbers is a series; any other is a collection, aired in a
// shuffled order, its short clips packed into blocks. Small collections join their parent's.

// FolderFile is a folder library's video as the Guide reads it.
type FolderFile struct {
	Path      string // client path: "<library>/<path below it>"
	Duration  time.Duration
	Thumbnail string // its thumbnail's URL, "" none
}

// Folder titles' limits
const (
	minSeries     = 3  // videos a series needs
	minCollection = 5  // a collection with fewer joins its parent folder's
	seriesShare   = 60 // percent of a folder's videos that must share the series' name
)

var (
	seasonFolder = regexp.MustCompile(`(?i)^(?:season|series|staffel|saison|s)[ ._-]*(\d{1,3})$`)
	specials     = regexp.MustCompile(`(?i)^specials?$`)
	seasonEp     = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,3})[ ._-]?e(\d{1,4})(?:[^0-9]|$)`)
	// Tags that differ between copies of one series' files and say nothing of its order: brackets,
	// resolutions, codecs, sources.
	nameTags = regexp.MustCompile(`(?i)\[[^\]]*\]|\([^)]*\)|\b(?:\d{3,4}p|4k|uhd|hd|sd|[hx]\.?26[45]|hevc|avc|xvid|divx|web-?dl|webrip|bluray|bdrip|dvdrip|hdtv|aac|ac3|corrected-aspect)\b|[ ._-](?:480|576|720|1080|2160)$`)
	digits   = regexp.MustCompile(`\d+`)
)

// FolderTitles reads a folder library's videos (libraryID, named libraryName) as titles: series, collections
// of long videos, and blocks of short clips (with the clips themselves, for filler).
func FolderTitles(libraryID, libraryName string, files []FolderFile) (series, long, blocks []*Title, clips []Clip) {
	type file struct {
		FolderFile
		rel    string // below the library
		season int    // from its Season folder, -1 none
	}
	byDir := map[string][]file{}
	for _, f := range files {
		if f.Duration <= 0 {
			continue
		}
		_, rel, ok := strings.Cut(f.Path, "/")
		if !ok {
			continue
		}
		dir, season := pathpkg.Dir(rel), -1
		if base := pathpkg.Base(dir); dir != "." {
			if m := seasonFolder.FindStringSubmatch(base); m != nil {
				season, _ = strconv.Atoi(m[1])
				dir = pathpkg.Dir(dir)
			} else if specials.MatchString(base) {
				season, dir = 0, pathpkg.Dir(dir)
			}
		}
		byDir[dir] = append(byDir[dir], file{f, rel, season})
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	name := func(dir string) string {
		if dir == "." {
			return libraryName
		}
		return pathpkg.Base(dir)
	}
	collections := map[string][]file{}
	for _, dir := range dirs {
		fs := byDir[dir]
		sf := make([]seriesFile, len(fs))
		for i, f := range fs {
			sf[i] = seriesFile{rel: f.rel, season: f.season, item: Item{Path: f.Path, Duration: f.Duration, Poster: f.Thumbnail,
				Name: fileTitle(f.rel)}}
		}
		items, ok := seriesItems(sf)
		if !ok {
			collections[dir] = append(collections[dir], fs...)
			continue
		}
		for i := range items {
			items[i].Name = episodeSummary(items[i].Name, name(dir))
		}
		series = append(series, &Title{Key: "folder:" + hashKey(libraryID+"/"+dir), Kind: KindShow, Name: name(dir), Folder: true, Items: items})
	}
	// Small collections join their parent folder's, deepest first.
	cdirs := make([]string, 0, len(collections))
	for d := range collections {
		cdirs = append(cdirs, d)
	}
	sort.Slice(cdirs, func(i, j int) bool {
		di, dj := strings.Count(cdirs[i], "/"), strings.Count(cdirs[j], "/")
		if di != dj {
			return di > dj
		}
		return cdirs[i] < cdirs[j]
	})
	for _, d := range cdirs {
		if d != "." && len(collections[d]) < minCollection {
			parent := pathpkg.Dir(d)
			collections[parent] = append(collections[parent], collections[d]...)
			delete(collections, d)
			if !slices.Contains(cdirs, parent) {
				cdirs = append(cdirs, parent) // reached below, being shallower
			}
		}
	}
	cdirs = cdirs[:0]
	for d := range collections {
		cdirs = append(cdirs, d)
	}
	sort.Strings(cdirs)
	for _, dir := range cdirs {
		fs := collections[dir]
		sort.Slice(fs, func(i, j int) bool { return fs[i].rel < fs[j].rel })
		var longItems []Item
		var short []Clip
		for _, f := range fs {
			it := Item{Path: f.Path, Duration: f.Duration, Poster: f.Thumbnail, Name: fileTitle(f.rel)}
			if f.Duration > MaxClip {
				longItems = append(longItems, it)
			} else {
				short = append(short, Clip{Path: f.Path, Duration: f.Duration, Poster: f.Thumbnail, Name: it.Name})
			}
		}
		key := hashKey(libraryID + "/" + dir)
		if len(longItems) > 0 {
			shuffle(longItems, "long|"+dir) // each once before any again: the planner goes through them in turn
			long = append(long, &Title{Key: "folder:" + key + ":long", Kind: KindShow, Name: name(dir), Folder: true, Items: longItems})
		}
		if len(short) > 0 {
			clips = append(clips, short...)
			if b := packBlocks(short, "clips|"+dir); len(b) > 0 {
				blocks = append(blocks, &Title{Key: "folder:" + key + ":clips", Kind: KindShow, Name: name(dir), Folder: true, Block: true, Items: b})
			}
		}
	}
	return series, long, blocks, clips
}

type seriesFile struct {
	rel    string
	season int // from its Season folder, -1 none
	item   Item
}

// seriesItems orders a folder's videos as a series' episodes if they are one: in Season folders, or
// most of them named SnnEnn, or most sharing a name apart from their numbers ("Episode 5", "1987").
// Episodes go by season and episode (SnnEnn, else the Season folder's and the name's numbers), the
// specials (season 0) last; the videos that don't fit the name come after, by name.
func seriesItems(fs []seriesFile) ([]Item, bool) {
	if len(fs) < minSeries {
		return nil, false
	}
	type ep struct {
		f               seriesFile
		fits            bool
		season, episode int
		nums            []int
	}
	eps := make([]ep, len(fs))
	inSeasons, withSE := 0, 0
	skel := map[string]int{}
	for i, f := range fs {
		eps[i] = ep{f: f, season: f.season}
		if f.season >= 0 {
			inSeasons++
		}
		if m := seasonEp.FindStringSubmatch(pathpkg.Base(f.rel)); m != nil {
			eps[i].season, _ = strconv.Atoi(m[1])
			eps[i].episode, _ = strconv.Atoi(m[2])
			eps[i].fits = true
			withSE++
		}
		skel[skeleton(f.rel)]++
	}
	common, most := "", 0
	for k, n := range skel {
		if n > most || n == most && k < common {
			common, most = k, n
		}
	}
	switch {
	case withSE*100 >= len(fs)*seriesShare:
	case strings.Contains(common, "#") && most >= minSeries && most*100 >= len(fs)*seriesShare, inSeasons*2 > len(fs):
		withSE = 0
		for i := range eps {
			e := &eps[i]
			e.fits = skeleton(e.f.rel) == common || e.f.season >= 0
			e.nums = numbers(e.f.rel)
			e.episode = 0
			if e.season < 0 {
				e.season = 1
			}
		}
	default:
		return nil, false
	}
	for i := range eps {
		if eps[i].season < 0 {
			eps[i].season = 1
		}
	}
	sort.SliceStable(eps, func(i, j int) bool {
		a, b := eps[i], eps[j]
		if a.fits != b.fits {
			return a.fits
		}
		if !a.fits {
			return a.f.rel < b.f.rel
		}
		if (a.season == 0) != (b.season == 0) {
			return b.season == 0
		}
		if a.season != b.season {
			return a.season < b.season
		}
		if a.episode != b.episode {
			return a.episode < b.episode
		}
		if c := slices.Compare(a.nums, b.nums); c != 0 {
			return c < 0
		}
		return a.f.rel < b.f.rel
	})
	items := make([]Item, len(eps))
	for i, e := range eps {
		items[i] = e.f.item
		items[i].Season, items[i].Episode = e.season, e.episode
		if e.episode == 0 && e.fits {
			items[i].Episode = i + 1 // its place in the series
		}
	}
	return items, true
}

// fileTitle is a video's name for the guide: its file name without the extension and tags.
func fileTitle(rel string) string {
	base := pathpkg.Base(rel)
	base = strings.TrimSuffix(base, pathpkg.Ext(base))
	t := strings.ReplaceAll(base, "_", " ")
	if !strings.Contains(t, " ") {
		t = strings.ReplaceAll(t, ".", " ") // "Some.Name.Here"
	}
	t = nameTags.ReplaceAllString(t, " ")
	t = strings.Trim(strings.Join(strings.Fields(t), " "), " -_.:")
	if t == "" {
		return base
	}
	return t
}

// episodeSummary is an episode's part of its program's title ("Velvet Hour - The Pier"): its own title
// where the file name has one, else the file name without the series' name; "" when that's only its
// SnnEnn or nothing (the program is then titled the series alone: the guide shows the season and episode).
func episodeSummary(title, series string) string {
	if t := episodeTitle(title, series); t != "" {
		return t
	}
	t := title
	if len(t) >= len(series) && strings.EqualFold(t[:len(series)], series) {
		t = strings.Trim(t[len(series):], " -_.:")
	}
	if onlySeasonEp.MatchString(t) {
		return ""
	}
	return t
}

var onlySeasonEp = regexp.MustCompile(`(?i)^\s*(?:s\d{1,3}[ ._-]?e\d{1,4})?\s*$`)

// episodeTitle is what's left of an episode's file title for the guide: without the series' name and
// its SnnEnn ("Velvet Hour - S01E02 - The Pier" is "The Pier"), "" when nothing is.
func episodeTitle(title, series string) string {
	t := seasonEp.ReplaceAllString(title, " ")
	if len(t) >= len(series) && strings.EqualFold(t[:len(series)], series) {
		t = t[len(series):]
	}
	t = strings.Trim(t, " -_.:")
	if !strings.ContainsFunc(t, func(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }) {
		return ""
	}
	return t
}

// skeleton is a file name with its tags gone and every number a #: the files of one series share it.
func skeleton(rel string) string {
	base := strings.ToLower(pathpkg.Base(rel))
	base = strings.TrimSuffix(base, pathpkg.Ext(base))
	base = nameTags.ReplaceAllString(base, " ")
	base = digits.ReplaceAllString(base, "#")
	return strings.Join(strings.FieldsFunc(base, func(r rune) bool { return strings.ContainsRune(" ._-", r) }), " ")
}

func numbers(rel string) []int {
	base := pathpkg.Base(rel)
	base = strings.TrimSuffix(base, pathpkg.Ext(base))
	base = nameTags.ReplaceAllString(base, " ")
	var out []int
	for _, m := range digits.FindAllString(base, -1) {
		n, _ := strconv.Atoi(m)
		out = append(out, n)
	}
	return out
}

// packBlocks packs clips into blocks of a half hour (an hour when they're long), in an order shuffled
// once: each block an item whose parts air one after another.
func packBlocks(clips []Clip, seed string) []Item {
	order := slices.Clone(clips)
	shuffle(order, seed)
	lengths := make([]time.Duration, len(order))
	for i, c := range order {
		lengths[i] = c.Duration
	}
	slices.Sort(lengths)
	target := halfHour
	if lengths[len(lengths)/2] > 8*time.Minute {
		target = 2 * halfHour
	}
	var out []Item
	var cur Item
	for _, c := range order {
		if len(cur.Parts) > 0 && cur.Duration+c.Duration > target {
			out = append(out, cur)
			cur = Item{}
		}
		if len(cur.Parts) == 0 {
			cur.Path, cur.Poster = c.Path, c.Poster
		}
		cur.Parts = append(cur.Parts, Part{Path: c.Path, Duration: c.Duration})
		cur.Duration += c.Duration
	}
	if len(cur.Parts) > 0 {
		out = append(out, cur)
	}
	// a block's part of its title: its first clip's name, and how many follow
	names := map[string]string{}
	for _, c := range order {
		names[c.Path] = c.Name
	}
	for i := range out {
		out[i].Name = names[out[i].Parts[0].Path]
		if n := len(out[i].Parts) - 1; n > 0 {
			out[i].Name += " and " + strconv.Itoa(n) + " more"
		}
	}
	return out
}

func hashKey(s string) string { return strconv.FormatUint(hash(s), 36) }
