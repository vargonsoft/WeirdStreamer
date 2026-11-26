package guide

import (
	"hash/fnv"
	"sort"
	"strconv"
)

// Channel kinds: a channel airs shows or movies, never both.
const (
	ChannelShows  = "shows"
	ChannelMovies = "movies"
)

// Channel is a group of a library's titles that air together. Its Key stays the same while the
// group exists, so its number and name (given by the service) do too.
type Channel struct {
	Key       string // "<library>/network:6", "<library>/shows:comedy", "<library>/movies:action"
	LibraryID string
	Kind      string
	Genre     string // what it mostly airs: "Comedy", "Action Movies"…
	Network   bool   // grouped by a TV network (named by a call sign rather than its genre)
	AnyTime   bool   // any title at any time (a Late Night typed channel: no ratings by day part)
	Type      string // a typed channel's type (named from its type's names, TypedName), "" for a library's
	Titles    []*Title
}

// Grouping limits
const (
	minNetworkShows = 4 // a network with fewer shows goes into the genre channels
	minShowGroup    = 2 // a genre channel with fewer shows goes into Variety
	minMovieGroup   = 3 // likewise for movies
)

type bucket struct {
	key, label string
	match      func(*Title) bool
}

func isKids(t *Title) bool {
	return t.hasGenre("Kids") || ratingClass(t.Rating) == ratingKids && t.hasGenre("Family", "Animation")
}

// The genre channels, in the order a title is tried against them.
var showBuckets = []bucket{
	{"kids", "Kids", isKids},
	{"animation", "Cartoons", func(t *Title) bool { return t.hasGenre("Animation") }},
	{"comedy", "Comedy", func(t *Title) bool { return t.hasGenre("Comedy") }},
	{"scifi", "Sci-Fi", func(t *Title) bool { return t.hasGenre("Sci-Fi & Fantasy") }},
	{"drama", "Drama", func(t *Title) bool {
		return t.hasGenre("Drama", "Crime", "Mystery", "Action & Adventure", "War & Politics", "Western", "Soap")
	}},
	{"reality", "Real Life", func(t *Title) bool { return t.hasGenre("Documentary", "Reality", "News", "Talk") }},
}

var movieBuckets = []bucket{
	{"family", "Family Movies", func(t *Title) bool {
		return t.hasGenre("Family", "Animation") || ratingClass(t.Rating) == ratingKids
	}},
	{"horror", "Fright Night", func(t *Title) bool { return t.hasGenre("Horror", "Thriller") }},
	{"scifi", "Sci-Fi Movies", func(t *Title) bool { return t.hasGenre("Science Fiction", "Fantasy") }},
	{"action", "Action Movies", func(t *Title) bool { return t.hasGenre("Action", "Adventure", "War", "Western") }},
	{"comedy", "Comedy Movies", func(t *Title) bool { return t.hasGenre("Comedy", "Romance") }},
	{"drama", "Drama Movies", func(t *Title) bool {
		return t.hasGenre("Drama", "Crime", "Mystery", "History", "Music", "Documentary")
	}},
}

// Channels groups one library's titles into channels: shows by the network they aired on (those
// with enough shows), the rest by genre; movies by genre. Small groups join a general channel.
// Titles with nothing airable are left out. The order is stable: networks, then genres as listed.
func Channels(libraryID string, titles []*Title) []Channel {
	var shows, movies []*Title
	for _, t := range titles {
		if t.typicalSlot() == 0 {
			continue
		}
		if t.Kind == KindMovie {
			movies = append(movies, t)
		} else {
			shows = append(shows, t)
		}
	}
	var out []Channel

	byNetwork := map[int][]*Title{}
	for _, t := range shows {
		if t.NetworkID > 0 {
			byNetwork[t.NetworkID] = append(byNetwork[t.NetworkID], t)
		}
	}
	var networks []int
	for id, ts := range byNetwork {
		if len(ts) >= minNetworkShows {
			networks = append(networks, id)
		}
	}
	sort.Ints(networks)
	onNetwork := map[*Title]bool{}
	for _, id := range networks {
		ts := byNetwork[id]
		for _, t := range ts {
			onNetwork[t] = true
		}
		out = append(out, Channel{Key: libraryID + "/network:" + strconv.Itoa(id), LibraryID: libraryID,
			Kind: ChannelShows, Genre: mainGenre(ts, showBuckets, "Variety"), Network: true, Titles: ts})
	}
	var rest []*Title
	for _, t := range shows {
		if !onNetwork[t] {
			rest = append(rest, t)
		}
	}
	out = append(out, byGenre(libraryID, ChannelShows, rest, showBuckets, minShowGroup, "variety", "Variety")...)
	out = append(out, byGenre(libraryID, ChannelMovies, movies, movieBuckets, minMovieGroup, "all", "Movies")...)
	return out
}

// byGenre puts each title in the first bucket it matches; buckets with fewer than min titles, and
// titles matching none, go into the general one.
func byGenre(libraryID, kind string, titles []*Title, buckets []bucket, min int, generalKey, generalLabel string) []Channel {
	groups := make([][]*Title, len(buckets))
	var general []*Title
	for _, t := range titles {
		placed := false
		for i, b := range buckets {
			if b.match(t) {
				groups[i] = append(groups[i], t)
				placed = true
				break
			}
		}
		if !placed {
			general = append(general, t)
		}
	}
	var out []Channel
	for i, b := range buckets {
		if len(groups[i]) < min {
			general = append(general, groups[i]...)
			continue
		}
		out = append(out, Channel{Key: libraryID + "/" + kind + ":" + b.key, LibraryID: libraryID, Kind: kind,
			Genre: b.label, Titles: groups[i]})
	}
	if len(general) > 0 {
		sort.Slice(general, func(i, j int) bool { return general[i].Key < general[j].Key })
		out = append(out, Channel{Key: libraryID + "/" + kind + ":" + generalKey, LibraryID: libraryID, Kind: kind,
			Genre: generalLabel, Titles: general})
	}
	return out
}

// mainGenre is the label of the bucket most of the titles fall in.
func mainGenre(titles []*Title, buckets []bucket, fallback string) string {
	counts := make([]int, len(buckets))
	for _, t := range titles {
		for i, b := range buckets {
			if b.match(t) {
				counts[i]++
				break
			}
		}
	}
	best, label := 0, fallback
	for i, n := range counts {
		if n > best {
			best, label = n, buckets[i].label
		}
	}
	return label
}

// Channel colors (badges), an 80s palette
var Colors = []string{"#E4572E", "#F3A712", "#3D5A98", "#4CB944", "#A23B72", "#2E86AB", "#F18F01", "#C73E1D",
	"#8E5572", "#6A4C93", "#1B998B", "#ED217C"}

func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
