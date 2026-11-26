package guide

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// Settings are the schedule's: when prime time starts and the time zone it's in.
type Settings struct {
	PrimeTime int // the hour: 19 or 20
	Location  *time.Location
}

func (s Settings) loc() *time.Location {
	if s.Location == nil {
		return time.Local
	}
	return s.Location
}

// PrimeHours is how long prime time is; the news hour (a late-night block for now) follows.
const PrimeHours = 3

// Slot kinds
const (
	SlotProgram = "program" // a title's episode or movie
	SlotFiller  = "filler"  // a clip from a filler library
	SlotBumper  = "bumper"  // a card the server draws ("Up next…", "We'll be right back")
)

// Slot is one thing on a channel's schedule: from Start, for Length.
type Slot struct {
	Channel string
	Start   time.Time
	Length  time.Duration
	Kind    string
	Title   string // programs: the title's key
	Item    int    // programs: the index of its item (episode) in the title's Items
	Path    string // programs and filler: the file's client path
	Track   string // programs: what placed it ("weekly:<weekday>:<minutes>", "strip", "movie")
}

// End is when the slot ends.
func (s Slot) End() time.Time { return s.Start.Add(s.Length) }

// Day parts: the broadcast day starts at 6 am.
const (
	partMorning = "morning" // weekdays 6 to 9
	partDay     = "day"     // weekdays 9 to 4
	partFringe  = "fringe"  // weekdays 4 to prime time
	partKids    = "kids"    // Saturday 6 to noon
	partWeekend = "weekend" // Saturday afternoon, Sunday until prime time
	partPrime   = "prime"
	partLate    = "late" // from the news hour until 6 am
)

// dayStart is 6 am of the broadcast day t is in.
func (s Settings) dayStart(t time.Time) time.Time {
	l := t.In(s.loc())
	d := l.Day()
	if l.Hour() < 6 {
		d--
	}
	return time.Date(l.Year(), l.Month(), d, 6, 0, 0, 0, s.loc())
}

// part is the day part t is in, and its broadcast day's start.
func (s Settings) part(t time.Time) (string, time.Time) {
	day := s.dayStart(t)
	l := t.In(s.loc())
	m := l.Hour()*60 + l.Minute()
	prime := s.PrimeTime * 60
	switch {
	case m < 6*60 || m >= prime+PrimeHours*60:
		return partLate, day
	case m >= prime:
		return partPrime, day
	}
	switch day.Weekday() {
	case time.Saturday:
		if m < 12*60 {
			return partKids, day
		}
		return partWeekend, day
	case time.Sunday:
		return partWeekend, day
	}
	switch {
	case m < 9*60:
		return partMorning, day
	case m < 16*60:
		return partDay, day
	}
	return partFringe, day
}

// AlignHalfHour is t rounded down to the half hour, local time.
func (s Settings) AlignHalfHour(t time.Time) time.Time {
	l := t.In(s.loc())
	return time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute()/30*30, 0, 0, s.loc())
}

// seasonYear is the TV season a day is in: seasons start in September.
func seasonYear(day time.Time) int {
	if day.Month() >= time.September {
		return day.Year()
	}
	return day.Year() - 1
}

// State is what a channel's schedule so far means for what comes next: each title's last episode
// on each track, the weekly slots' shows, and when titles and clips last aired. Built by Record
// from the stored slots, in order, then kept up by the planner.
type State struct {
	set       Settings
	cursor    map[string]int       // "track|title" → the item last aired
	slotTitle map[string]string    // weekly track → the title last aired in it
	lastAired map[string]time.Time // title key, clip path or weekly track → when it last started
	partCount map[string]int       // "<day>|<part>" → programs started in it
}

// NewState is an empty schedule's state.
func NewState(set Settings) *State {
	return &State{set: set, cursor: map[string]int{}, slotTitle: map[string]string{},
		lastAired: map[string]time.Time{}, partCount: map[string]int{}}
}

// Record takes a slot already on the schedule into account. Slots must come in order.
func (st *State) Record(s Slot) {
	switch s.Kind {
	case SlotProgram:
		st.cursor[s.Track+"|"+s.Title] = s.Item
		if strings.HasPrefix(s.Track, "weekly:") {
			st.slotTitle[s.Track] = s.Title
			st.lastAired[s.Track] = s.Start
		}
		st.lastAired[s.Title] = s.Start
		part, day := st.set.part(s.Start)
		st.partCount[partKey(day, part)]++
	case SlotFiller:
		st.lastAired[s.Path] = s.Start
	}
}

func partKey(day time.Time, part string) string {
	return day.Format("2006-01-02") + "|" + part
}

type position struct {
	start, length time.Duration // from prime time's start
	queue         []*Title      // the shows that take turns in it, a season each
}

type planner struct {
	ch     Channel
	set    Settings
	st     *State
	clips  []Clip
	primes map[int]*[7][]position // by season year
	pools  map[string][]*Title    // strip rotations, by week and part
}

// Plan fills a channel's schedule from from (on a half hour) until to, going on from st, which it
// updates. Every program starts on a half hour and takes whole half hours; the rest of its time is
// filler from clips, then a bumper.
func Plan(ch Channel, from, to time.Time, st *State, clips []Clip) []Slot {
	p := &planner{ch: ch, set: st.set, st: st, clips: clips, primes: map[int]*[7][]position{}, pools: map[string][]*Title{}}
	var out []Slot
	t := from
	for t.Before(to) {
		title, item, track := p.pick(t)
		if title == nil {
			out = append(out, Slot{Channel: ch.Key, Start: t, Length: halfHour, Kind: SlotBumper})
			t = t.Add(halfHour)
			continue
		}
		it := title.Items[item]
		prog := Slot{Channel: ch.Key, Start: t, Length: it.Duration, Kind: SlotProgram, Title: title.Key, Item: item,
			Path: it.Path, Track: track}
		st.Record(prog)
		out = append(out, prog)
		end := t.Add(slotLength(it.Duration))
		out = append(out, p.fill(prog.End(), end, title.Year)...)
		t = end
	}
	return out
}

func (p *planner) pick(t time.Time) (*Title, int, string) {
	part, day := p.set.part(t)
	if p.ch.Kind == ChannelMovies {
		return p.pickMovie(part, day)
	}
	if part == partPrime {
		if title, item, track := p.pickWeekly(t, day); title != nil {
			return title, item, track
		}
	}
	return p.pickStrip(part, day)
}

// eligible are the titles the part may air: kids' ratings on Saturday morning, nothing adult
// before the news hour. With none, the next wider set.
func (p *planner) eligible(titles []*Title, part string) []*Title {
	allowed := []int{ratingKids, ratingGeneral}
	switch part {
	case partKids:
		allowed = []int{ratingKids}
	case partLate:
		allowed = []int{ratingKids, ratingGeneral, ratingAdult}
	}
	for {
		var out []*Title
		for _, t := range titles {
			if slices.Contains(allowed, ratingClass(t.Rating)) {
				out = append(out, t)
			}
		}
		if len(out) > 0 || len(allowed) == 3 {
			return out
		}
		allowed = append(allowed, len(allowed))
	}
}

// nextItem is the item after the title's last on the track (its first when it hasn't aired there,
// and from the start again after its last), skipping items that can't air.
func (p *planner) nextItem(track string, t *Title) int {
	i := 0
	if last, ok := p.st.cursor[track+"|"+t.Key]; ok {
		i = last + 1
	}
	for range len(t.Items) {
		if i >= len(t.Items) {
			i = 0
		}
		if t.Items[i].Duration > 0 {
			return i
		}
		i++
	}
	return 0
}

// pickWeekly is prime time: the show whose turn it is in the weekly position t is in.
func (p *planner) pickWeekly(t, day time.Time) (*Title, int, string) {
	week := p.primeTemplate(seasonYear(day))
	primeStart := time.Date(day.Year(), day.Month(), day.Day(), p.set.PrimeTime, 0, 0, 0, p.set.loc())
	off := t.Sub(primeStart)
	wd := day.Weekday()
	for _, pos := range week[wd] {
		if off < pos.start || off >= pos.start+pos.length {
			continue
		}
		track := fmt.Sprintf("weekly:%d:%d", wd, int(pos.start/time.Minute))
		if last, ok := p.st.lastAired[track]; ok && p.set.dayStart(last).Equal(day) {
			return nil, 0, "" // aired tonight already (a shorter episode left room in its position)
		}
		title := p.turn(track, pos.queue)
		return title, p.nextItem(track, title), track
	}
	return nil, 0, ""
}

// turn is the show of a weekly position this week: the one that aired last week goes on with its
// season; after its season's last episode the next in the queue has its turn (a season), and so on
// round.
func (p *planner) turn(track string, queue []*Title) *Title {
	cur := slices.IndexFunc(queue, func(t *Title) bool { return t.Key == p.st.slotTitle[track] })
	if cur < 0 {
		return queue[0]
	}
	t := queue[cur]
	last, aired := p.st.cursor[track+"|"+t.Key]
	next := p.nextItem(track, t)
	if len(queue) == 1 || aired && next > last && t.Items[next].Season == t.Items[last].Season {
		return t
	}
	return queue[(cur+1)%len(queue)]
}

// primeTemplate is a season's prime-time lineup: each night's positions, each a show (and a
// second to take turns with, a season each, where there are enough). Shows are spread over the
// nights first: every night's first position, then every night's second… A show has one position.
func (p *planner) primeTemplate(year int) *[7][]position {
	if w := p.primes[year]; w != nil {
		return w
	}
	pool := p.eligible(p.ch.Titles, partPrime)
	pool = slices.DeleteFunc(slices.Clone(pool), func(t *Title) bool { return t.typicalSlot() > PrimeHours*time.Hour })
	shuffle(pool, fmt.Sprintf("%s|prime|%d", p.ch.Key, year))
	used := map[*Title]bool{}
	take := func(fit func(time.Duration) bool) *Title {
		for _, t := range pool {
			if !used[t] && fit(t.typicalSlot()) {
				used[t] = true
				return t
			}
		}
		return nil
	}
	var w [7][]position
	var off [7]time.Duration
	for progress := true; progress; {
		progress = false
		for n := range 7 {
			wd := (time.Monday + time.Weekday(n)) % 7
			left := PrimeHours*time.Hour - off[wd]
			if left <= 0 {
				continue
			}
			if t := take(func(d time.Duration) bool { return d <= left }); t != nil {
				w[wd] = append(w[wd], position{start: off[wd], length: t.typicalSlot(), queue: []*Title{t}})
				off[wd] += t.typicalSlot()
				progress = true
			}
		}
	}
	for n := range 7 {
		wd := (time.Monday + time.Weekday(n)) % 7
		for i := range w[wd] {
			length := w[wd][i].length
			if t := take(func(d time.Duration) bool { return d == length }); t != nil {
				w[wd][i].queue = append(w[wd][i].queue, t)
			}
		}
	}
	p.primes[year] = &w
	return &w
}

// pickStrip is syndication: the part's rotation of shows, the same each weekday (a week's
// lineup), each show on to its next episode.
func (p *planner) pickStrip(part string, day time.Time) (*Title, int, string) {
	pool := p.stripPool(part, day)
	if len(pool) == 0 {
		return nil, 0, ""
	}
	t := pool[p.st.partCount[partKey(day, part)]%len(pool)]
	return t, p.nextItem("strip", t), "strip"
}

func (p *planner) stripPool(part string, day time.Time) []*Title {
	y, wk := day.ISOWeek()
	key := fmt.Sprintf("%d-%d|%s", y, wk, part)
	if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
		key += "|" + wd.String()
	}
	if pool, ok := p.pools[key]; ok {
		return pool
	}
	pool := slices.Clone(p.eligible(p.ch.Titles, part))
	if part != partPrime {
		// Prime time's shows are kept for their night, where there are others to air.
		inPrime := map[*Title]bool{}
		for _, night := range p.primeTemplate(seasonYear(day)) {
			for _, pos := range night {
				for _, t := range pos.queue {
					inPrime[t] = true
				}
			}
		}
		if rest := slices.DeleteFunc(slices.Clone(pool), func(t *Title) bool { return inPrime[t] }); len(rest) >= 3 {
			pool = rest
		}
	}
	shuffle(pool, p.ch.Key+"|"+key)
	p.pools[key] = pool
	return pool
}

// pickMovie is the eligible movie that aired longest ago (or never), ties broken by the day.
func (p *planner) pickMovie(part string, day time.Time) (*Title, int, string) {
	pool := p.eligible(p.ch.Titles, part)
	if len(pool) == 0 {
		return nil, 0, ""
	}
	seed := day.Format("2006-01-02")
	best := pool[0]
	for _, t := range pool[1:] {
		a, b := p.st.lastAired[t.Key], p.st.lastAired[best.Key]
		if a.Before(b) || a.Equal(b) && hash(t.Key+seed) < hash(best.Key+seed) {
			best = t
		}
	}
	return best, p.nextItem("movie", best), "movie"
}

// Filler choice
const (
	clipRepeatAfter = 2 * time.Hour // a clip isn't aired again on the channel sooner
	clipNearYears   = 3             // clips this close to the program's year are tried first
	minFillerGap    = 3 * time.Second
)

// fill fills from..to after a program of the given year: clips from near its year first, then any,
// none aired on the channel lately, then a bumper for what's left.
func (p *planner) fill(from, to time.Time, year int) []Slot {
	var out []Slot
	t := from
	if len(p.clips) > 0 {
		order := slices.Clone(p.clips)
		shuffle(order, p.ch.Key+"|"+from.String())
		used := map[string]bool{}
		for to.Sub(t) >= minFillerGap {
			c := p.clip(order, to.Sub(t), t, year, used)
			if c == nil {
				break
			}
			used[c.Path] = true
			s := Slot{Channel: p.ch.Key, Start: t, Length: c.Duration, Kind: SlotFiller, Path: c.Path}
			p.st.Record(s)
			out = append(out, s)
			t = s.End()
		}
	}
	if t.Before(to) {
		out = append(out, Slot{Channel: p.ch.Key, Start: t, Length: to.Sub(t), Kind: SlotBumper})
	}
	return out
}

func (p *planner) clip(order []Clip, room time.Duration, now time.Time, year int, used map[string]bool) *Clip {
	ok := func(c *Clip) bool {
		if c.Duration <= 0 || c.Duration > room || used[c.Path] {
			return false
		}
		last, aired := p.st.lastAired[c.Path]
		return !aired || now.Sub(last) >= clipRepeatAfter
	}
	if year > 0 {
		for i := range order {
			c := &order[i]
			if c.Year > 0 && abs(c.Year-year) <= clipNearYears && ok(c) {
				return c
			}
		}
	}
	for i := range order {
		if ok(&order[i]) {
			return &order[i]
		}
	}
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// shuffle orders s the same way for the same seed.
func shuffle[T any](s []T, seed string) {
	h := hash(seed)
	r := rand.New(rand.NewPCG(h, h>>32|1))
	r.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
}
