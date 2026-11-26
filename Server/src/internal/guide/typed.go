package guide

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Typed channels (docs/GUIDE.md): channels an admin defines, a type and the libraries they're made
// from. Their channels' library is TypedLibraryID(definition), their keys TypedLibraryID/<group>:<part>.

// TypedLateNight is the type that airs everything in its libraries, all day.
const TypedLateNight = "latenight"

// TypedLibraryID is what a typed channel stores as its library: its definition's.
func TypedLibraryID(id string) string { return "typed:" + id }

// TypedDefinition is the definition a typed channel's library names, "" for a library's channel.
func TypedDefinition(libraryID string) string {
	id, ok := strings.CutPrefix(libraryID, "typed:")
	if !ok {
		return ""
	}
	return id
}

var typedNames = map[string]string{TypedLateNight: "Late Night", "kids": "Kids", "cartoons": "Cartoons",
	"comedy": "Comedy", "scifi": "Sci-Fi", "drama": "Drama", "reallife": "Real Life", "action": "Action",
	"horror": "Horror"}

// TypedChannelNames are each type's channels' names, in the order they're given (each has its logo,
// logodesign.go's typedDesigns). A type's fifth channel and on is its first name and a number.
var TypedChannelNames = map[string][]string{
	TypedLateNight: {"After Dark", "Nightcap", "Velvet Room", "Moonlight"},
	"kids":         {"Kid Zone", "Playground", "Toy Box", "Jamboree"},
	"cartoons":     {"Toon Time", "Doodle", "Funnybones", "Inkwell"},
	"comedy":       {"Laugh Track", "Chuckles", "Punchline", "Giggles"},
	"scifi":        {"Star Port", "Nebula", "Warp Drive", "Orbit"},
	"drama":        {"Spotlight", "Stage Door", "Curtain Call", "Marquee"},
	"reallife":     {"True Story", "Field Trip", "Lookout", "Almanac"},
	"action":       {"Adrenaline", "Blast Zone", "Rapid Fire", "Stunt Show"},
	"horror":       {"Graveyard Shift", "Creature Feature", "Spook House", "Haunt"},
}

// TypedName is the name a typed channel of type typ takes: the first of its type's names no other
// channel has (taken), else the type's first name and the first number free.
func TypedName(typ string, taken map[string]bool) string {
	pool := TypedChannelNames[typ]
	if len(pool) == 0 {
		pool = []string{TypedTypeName(typ)}
	}
	for _, n := range pool {
		if !taken[n] {
			return n
		}
	}
	for i := 2; ; i++ {
		if n := pool[0] + " " + strconv.Itoa(i); !taken[n] {
			return n
		}
	}
}

// IsTypedName says whether name is one a typed channel of type typ takes.
func IsTypedName(typ, name string) bool {
	pool := TypedChannelNames[typ]
	if slices.Contains(pool, name) {
		return true
	}
	if len(pool) == 0 {
		return name == TypedTypeName(typ)
	}
	rest, ok := strings.CutPrefix(name, pool[0]+" ")
	_, err := strconv.Atoi(rest)
	return ok && err == nil
}

// TypedTypeName is a type's name.
func TypedTypeName(t string) string {
	if n, ok := typedNames[t]; ok {
		return n
	}
	return t
}

// TypedMatch says whether a library's TMDB title is of a genre type (Late Night takes all).
func TypedMatch(typ string, t *Title) bool {
	switch typ {
	case TypedLateNight:
		return true
	case "kids":
		return isKids(t) || t.Kind == KindMovie && ratingClass(t.Rating) == ratingKids
	case "cartoons":
		return t.hasGenre("Animation")
	case "comedy":
		return t.hasGenre("Comedy")
	case "scifi":
		return t.hasGenre("Sci-Fi & Fantasy", "Science Fiction", "Fantasy")
	case "drama":
		return t.hasGenre("Drama", "Crime", "Mystery")
	case "reallife":
		return t.hasGenre("Documentary", "Reality", "News", "Talk")
	case "action":
		return t.hasGenre("Action", "Adventure", "Action & Adventure", "War", "Western")
	case "horror":
		return t.hasGenre("Horror", "Thriller")
	}
	return false
}

// A typed definition's content, by kind: the groups its channels are made of, in their channels' order.
const (
	TypedSeries = "series" // series (a folder's, or TMDB shows)
	TypedLong   = "long"   // collections' long videos
	TypedClips  = "clips"  // collections' short clips, in blocks
	TypedMovies = "movies" // TMDB movies
)

var typedGroupOrder = []string{TypedSeries, TypedLong, TypedClips, TypedMovies}

// Typed channels' sizes
const (
	typedMinHours   = 24      // a group with less joins the largest
	typedSplitHours = 14 * 24 // a group is split while each part would have more
)

// TypedChannels are a definition's channels: up to max, one per group with a day of content (the
// smaller ones joining the largest), then the group with the most hours per channel split while each
// part would have more than two weeks. Shows-like groups (series, long videos, clips) share channels;
// movies have their own. Late Night channels air at any time. The service names them (TypedName).
func TypedChannels(def, typ string, max int, groups map[string][]*Title) []Channel {
	type group struct {
		key    string
		titles []*Title
		hours  float64
		movies bool
		parts  int
	}
	var gs []*group
	for _, k := range typedGroupOrder {
		if len(groups[k]) == 0 {
			continue
		}
		g := &group{key: k, titles: slices.Clone(groups[k]), movies: k == TypedMovies, parts: 1}
		for _, t := range g.titles {
			g.hours += titleHours(t)
		}
		gs = append(gs, g)
	}
	if len(gs) == 0 || max <= 0 {
		return nil
	}
	// merge g into the largest other group of its kind (of any, with none)
	merge := func(g *group) bool {
		var into *group
		for _, o := range gs {
			if o == g {
				continue
			}
			same, intoSame := o.movies == g.movies, into != nil && into.movies == g.movies
			if into == nil || same && !intoSame || same == intoSame && o.hours > into.hours {
				into = o
			}
		}
		if into == nil {
			return false
		}
		into.titles = append(into.titles, g.titles...)
		into.hours += g.hours
		gs = slices.DeleteFunc(gs, func(o *group) bool { return o == g })
		return true
	}
	// Shows-like groups with less than a day join the largest (even when none has a day: one channel)
	for _, g := range slices.Clone(gs) {
		if g.movies || g.hours >= typedMinHours {
			continue
		}
		var largest *group
		for _, o := range gs {
			if !o.movies && (largest == nil || o.hours > largest.hours) {
				largest = o
			}
		}
		if largest != g {
			merge(g)
		}
	}
	for len(gs) > max {
		smallest := gs[0]
		for _, g := range gs[1:] {
			if g.hours < smallest.hours {
				smallest = g
			}
		}
		merge(smallest)
	}
	for n := len(gs); n < max; n++ {
		var best *group
		for _, g := range gs {
			if g.hours/float64(g.parts+1) > typedSplitHours && (best == nil || g.hours/float64(g.parts) > best.hours/float64(best.parts)) {
				best = g
			}
		}
		if best == nil {
			break
		}
		best.parts++
	}
	var out []Channel
	for _, g := range gs {
		kind := ChannelShows
		if g.movies {
			kind = ChannelMovies
		}
		for i, part := range splitTitles(g.titles, g.parts) {
			out = append(out, Channel{Key: TypedLibraryID(def) + "/" + g.key + ":" + strconv.Itoa(i+1),
				LibraryID: TypedLibraryID(def), Kind: kind, Genre: TypedTypeName(typ), Type: typ,
				AnyTime: typ == TypedLateNight, Titles: part})
		}
	}
	return out
}

// splitTitles deals titles into n parts of about the same hours, the longest first.
func splitTitles(titles []*Title, n int) [][]*Title {
	if n <= 1 {
		return [][]*Title{titles}
	}
	order := slices.Clone(titles)
	sort.SliceStable(order, func(i, j int) bool {
		hi, hj := titleHours(order[i]), titleHours(order[j])
		if hi != hj {
			return hi > hj
		}
		return order[i].Key < order[j].Key
	})
	parts := make([][]*Title, n)
	hours := make([]float64, n)
	for _, t := range order {
		least := 0
		for i := range hours {
			if hours[i] < hours[least] {
				least = i
			}
		}
		parts[least] = append(parts[least], t)
		hours[least] += titleHours(t)
	}
	for _, p := range parts {
		sort.SliceStable(p, func(i, j int) bool { return p[i].Key < p[j].Key })
	}
	return parts
}

func titleHours(t *Title) float64 {
	var d time.Duration
	for _, it := range t.Items {
		d += it.Duration
	}
	return d.Hours()
}
