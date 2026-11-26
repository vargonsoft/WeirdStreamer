// Package guide plans the Guide's live channels: pseudo-channels made from the libraries with Show
// in Guide on, programmed the way 80s and 90s TV was (docs/GUIDE.md). This file and channels.go,
// planner.go and filler.go are pure: they plan from titles and clips they're given. service.go
// feeds them from the libraries and stores what they plan.
package guide

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// Title kinds
const (
	KindShow  = "show"
	KindMovie = "movie"
)

// Title is a show or movie the Guide can air.
type Title struct {
	Key       string // "show:1399", "movie:603" (TitleKey)
	Kind      string
	TMDBID    int
	Name      string
	Year      int
	Genres    []string
	NetworkID int // shows: the TMDB network it first aired on, 0 none
	Rating    string
	Overview  string
	Poster    string
	Items     []Item // a show's episodes in order, or a movie's file
	Library   string // the library it's from, where its channel isn't one library's (typed channels)
	Block     bool   // its items are blocks of short clips (a folder collection's: no voice-overs)
}

// TitleKey is a title's key, from its kind and TMDB ID.
func TitleKey(kind string, tmdbID int) string {
	return kind + ":" + strconv.Itoa(tmdbID)
}

// Item is one airable file of a title: an episode, or a movie.
type Item struct {
	Path            string // client path
	Duration        time.Duration
	Season, Episode int // episodes
	Name, Overview  string
	Poster          string // its own picture (a folder library's video's thumbnail), where the title has none
	Parts           []Part // a block of clips: they air one after another (Path is the first's)
}

// Part is a clip of a block.
type Part struct {
	Path     string // client path
	Duration time.Duration
}

// Clip is a short video that can fill a gap between programs (a Use for Guide Filler library's).
type Clip struct {
	Path     string // client path
	Duration time.Duration
	Year     int    // from its folder or name (1987/…, "… (1987) …"), 0 unknown: the planner prefers clips near the program's year
	Poster   string // its thumbnail (a folder library's), "" none
}

// MaxClip is the longest video used as filler.
const MaxClip = 15 * time.Minute

// Rating classes: what a time of day may air.
const (
	ratingKids    = iota // TV-Y, TV-Y7, TV-G, G
	ratingGeneral        // the rest, unrated included
	ratingAdult          // TV-MA, R, NC-17: only late at night
)

func ratingClass(r string) int {
	switch strings.ToUpper(strings.TrimSpace(r)) {
	case "TV-Y", "TV-Y7", "TV-Y7-FV", "TV-G", "G":
		return ratingKids
	case "TV-MA", "R", "NC-17", "X":
		return ratingAdult
	}
	return ratingGeneral
}

func (t *Title) hasGenre(names ...string) bool {
	for _, g := range t.Genres {
		if slices.Contains(names, g) {
			return true
		}
	}
	return false
}

// typicalSlot is how long the title's items usually take on the schedule (the median of their
// half-hour slots), 0 with none airable.
func (t *Title) typicalSlot() time.Duration {
	var slots []time.Duration
	for _, it := range t.Items {
		if it.Duration > 0 {
			slots = append(slots, slotLength(it.Duration))
		}
	}
	if len(slots) == 0 {
		return 0
	}
	slices.Sort(slots)
	return slots[len(slots)/2]
}

// slotLength is the schedule time a program of length d takes: whole half hours.
func slotLength(d time.Duration) time.Duration {
	n := (d + halfHour - 1) / halfHour
	return max(n, 1) * halfHour
}

const halfHour = 30 * time.Minute
