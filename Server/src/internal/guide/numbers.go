package guide

import (
	"sort"

	"github.com/weirdstreamer/server/internal/database"
)

// Channel numbers come in blocks, with gaps to grow into (up to 3 digits): the Guide channel 1,
// network stations 2–19, Weather (and channels like it) 20–29, TV genre channels 30–49, movie
// channels 50–69, each typed definition its block of ten from 100 (its Number); a full block goes on
// from 200. A channel keeps its number while it's in its block; one outside it (numbered before the
// blocks, or a typed channel's definition given another block) moves into it once.

type numberBlock struct{ lo, hi int }

const overflowFrom = 200

// Typed is each typed definition's first number, by its ID.
type Typed map[string]int

// blockFor is the block a channel's number belongs in.
func blockFor(c database.GuideChannel, typed Typed) numberBlock {
	if def := TypedDefinition(c.LibraryID); def != "" {
		if first, ok := typed[def]; ok {
			return numberBlock{first, first + 9}
		}
		return numberBlock{overflowFrom, 999}
	}
	switch {
	case c.Key == GuideChannelKey:
		return numberBlock{GuideChannelNum, GuideChannelNum}
	case c.Kind == KindWeather:
		return numberBlock{20, 29}
	case c.Kind == ChannelShows && IsStation(c.Name):
		return numberBlock{2, 19}
	case c.Kind == ChannelShows:
		return numberBlock{30, 49}
	case c.Kind == ChannelMovies:
		return numberBlock{50, 69}
	}
	return numberBlock{overflowFrom, 999}
}

func (b numberBlock) holds(n int) bool { return n >= b.lo && n <= b.hi }

// nextFree is the first number of the block nobody has, else the first free from 200.
func (b numberBlock) nextFree(taken map[int]bool) int {
	for n := b.lo; n <= b.hi; n++ {
		if !taken[n] {
			return n
		}
	}
	n := overflowFrom
	for taken[n] {
		n++
	}
	return n
}

// Renumber is the channels that move into their blocks, by key, with their new numbers. Those in
// their block stay; the rest move in by block from the highest (so a low number one of them held is
// free for the block it belongs to), each to the first free number there.
func Renumber(chans []database.GuideChannel, typed Typed) map[string]int {
	taken := map[int]bool{}
	var movers []database.GuideChannel
	for _, c := range chans {
		if b := blockFor(c, typed); b.holds(c.Number) || c.Number >= overflowFrom && b.lo < overflowFrom && TypedDefinition(c.LibraryID) == "" {
			taken[c.Number] = true
		} else {
			movers = append(movers, c)
		}
	}
	sort.SliceStable(movers, func(i, j int) bool {
		bi, bj := blockFor(movers[i], typed), blockFor(movers[j], typed)
		if bi.lo != bj.lo {
			return bi.lo > bj.lo
		}
		return movers[i].Number < movers[j].Number
	})
	out := map[string]int{}
	for _, c := range movers {
		n := blockFor(c, typed).nextFree(taken)
		taken[n] = true
		out[c.Key] = n
	}
	return out
}

// nextNumber is the number a new channel takes: the first free one of its block.
func nextNumber(c database.GuideChannel, existing map[string]database.GuideChannel, typed Typed) int {
	taken := map[int]bool{}
	for _, e := range existing {
		taken[e.Number] = true
	}
	return blockFor(c, typed).nextFree(taken)
}
