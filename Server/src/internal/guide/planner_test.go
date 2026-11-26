package guide

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var chicago, _ = time.LoadLocation("America/Chicago")

func testSettings() Settings { return Settings{PrimeTime: 20, Location: chicago} }

// show makes a show of seasons × episodes, each of length d.
func show(id int, seasons, episodes int, d time.Duration, rating string, genres ...string) *Title {
	t := &Title{Key: TitleKey(KindShow, id), Kind: KindShow, TMDBID: id, Name: fmt.Sprintf("Show %d", id),
		Year: 1985, Rating: rating, Genres: genres}
	for s := 1; s <= seasons; s++ {
		for e := 1; e <= episodes; e++ {
			t.Items = append(t.Items, Item{Path: fmt.Sprintf("lib/show%d/S%02dE%02d.mkv", id, s, e), Duration: d,
				Season: s, Episode: e})
		}
	}
	return t
}

func movie(id int, d time.Duration, rating string, genres ...string) *Title {
	return &Title{Key: TitleKey(KindMovie, id), Kind: KindMovie, TMDBID: id, Name: fmt.Sprintf("Movie %d", id),
		Year: 1988, Rating: rating, Genres: genres, Items: []Item{{Path: fmt.Sprintf("lib/movie%d.mkv", id), Duration: d}}}
}

func clips(n int) []Clip {
	var out []Clip
	for i := range n {
		out = append(out, Clip{Path: fmt.Sprintf("ads/%d.mp4", i), Duration: time.Duration(15+15*(i%4)) * time.Second,
			Year: 1980 + i%20})
	}
	return out
}

// Programs start on half hours, slots follow one another without gaps or overlaps, and each
// program's half hours are filled to the second.
func checkContiguous(t *testing.T, slots []Slot, from, to time.Time) {
	t.Helper()
	at := from
	for _, s := range slots {
		if !s.Start.Equal(at) {
			t.Fatalf("slot at %v, want %v", s.Start, at)
		}
		if s.Kind == SlotProgram && s.Start.Minute()%30 != 0 || s.Start.Second() != 0 && s.Kind == SlotProgram {
			t.Fatalf("program at %v", s.Start)
		}
		at = s.End()
	}
	if at.Before(to) {
		t.Fatalf("planned until %v, want %v", at, to)
	}
	if at.Minute()%30 != 0 || at.Second() != 0 {
		t.Fatalf("ends at %v, not on a half hour", at)
	}
}

func TestChannelsGroupTitles(t *testing.T) {
	var titles []*Title
	for i := 1; i <= 4; i++ {
		s := show(i, 1, 5, 22*time.Minute, "TV-PG", "Drama")
		s.NetworkID = 6
		titles = append(titles, s)
	}
	titles = append(titles, show(10, 1, 5, 22*time.Minute, "TV-PG", "Comedy"),
		show(11, 1, 5, 22*time.Minute, "TV-PG", "Comedy"),
		show(12, 1, 5, 44*time.Minute, "TV-14", "Crime")) // alone: Variety
	for i := 20; i < 23; i++ {
		titles = append(titles, movie(i, 100*time.Minute, "R", "Action"))
	}
	titles = append(titles, movie(30, 90*time.Minute, "R", "Horror"), movie(31, 90*time.Minute, "PG", "Comedy"))
	titles = append(titles, &Title{Key: "show:99", Kind: KindShow}) // nothing airable

	var got []string
	for _, c := range Channels("lib", titles) {
		got = append(got, fmt.Sprintf("%s=%d", c.Key, len(c.Titles)))
	}
	want := "lib/network:6=4 lib/shows:comedy=2 lib/shows:variety=1 lib/movies:action=3 lib/movies:all=2"
	if strings.Join(got, " ") != want {
		t.Errorf("channels = %v, want %s", got, want)
	}
	if CallSign("lib/network:6") != CallSign("lib/network:6") || len(CallSign("x")) != 4 {
		t.Error("call sign isn't stable")
	}
}

func showChannel() Channel {
	var titles []*Title
	for i := 1; i <= 12; i++ {
		d := 23 * time.Minute
		if i%3 == 0 {
			d = 46 * time.Minute
		}
		titles = append(titles, show(i, 2, 10, d, "TV-PG", "Comedy"))
	}
	titles = append(titles, show(50, 1, 10, 25*time.Minute, "TV-MA", "Comedy"))
	return Channel{Key: "lib/shows:comedy", Kind: ChannelShows, Titles: titles}
}

func TestPlanShows(t *testing.T) {
	set := testSettings()
	ch := showChannel()
	from := time.Date(2026, 11, 30, 6, 0, 0, 0, chicago) // a Monday
	to := from.Add(14 * 24 * time.Hour)
	slots := Plan(ch, from, to, NewState(set), clips(40))
	checkContiguous(t, slots, from, to)

	var progs []Slot
	for _, s := range slots {
		if s.Kind == SlotProgram {
			progs = append(progs, s)
			if s.Title == "show:50" {
				if part, _ := set.part(s.Start); part != partLate {
					t.Errorf("TV-MA show at %v (%s)", s.Start, part)
				}
			}
		}
	}
	for i := 1; i < len(progs); i++ {
		a, b := progs[i-1], progs[i]
		pa, _ := set.part(a.Start)
		pb, _ := set.part(b.Start)
		if a.Title == b.Title && pa == pb && pa != partLate && pa != partPrime {
			t.Errorf("%s twice in a row at %v and %v", a.Title, a.Start, b.Start)
		}
	}

	// Prime time is weekly: Monday 8 pm's show the next Monday is the same, an episode on.
	at := func(day time.Time) Slot {
		for _, s := range progs {
			if s.Start.Equal(day) {
				return s
			}
		}
		t.Fatalf("nothing starts at %v", day)
		return Slot{}
	}
	mon := time.Date(2026, 11, 30, 20, 0, 0, 0, chicago)
	a, b := at(mon), at(mon.AddDate(0, 0, 7))
	if a.Title != b.Title || b.Item != a.Item+1 || !strings.HasPrefix(a.Track, "weekly:") {
		t.Errorf("Monday 8 pm: %+v then %+v", a, b)
	}
	// and that's its only weekly slot (prime time this channel can't fill with weekly shows is reruns)
	for _, s := range progs {
		if strings.HasPrefix(s.Track, "weekly:") && s.Title == a.Title && s.Start.Weekday() != time.Monday {
			t.Errorf("%s also weekly at %v", a.Title, s.Start)
		}
	}
	// Strips: Tuesday's 9 am show is Monday's, an episode on.
	m9, t9 := at(time.Date(2026, 11, 30, 9, 0, 0, 0, chicago)), at(time.Date(2026, 12, 1, 9, 0, 0, 0, chicago))
	if m9.Title != t9.Title || m9.Track != "strip" {
		t.Errorf("9 am: Monday %+v, Tuesday %+v", m9, t9)
	}
}

// Planning in two steps, the second from the stored first's state, plans the same as in one.
func TestPlanContinuesFromState(t *testing.T) {
	set := testSettings()
	ch := showChannel()
	from := time.Date(2026, 12, 3, 6, 0, 0, 0, chicago)
	mid, to := from.Add(30*time.Hour), from.Add(72*time.Hour)
	whole := Plan(ch, from, to, NewState(set), clips(40))

	first := Plan(ch, from, mid, NewState(set), clips(40))
	st := NewState(set)
	for _, s := range first {
		st.Record(s)
	}
	second := Plan(ch, first[len(first)-1].End(), to, st, clips(40))
	both := append(first, second...)
	if len(both) != len(whole) {
		t.Fatalf("%d slots in two steps, %d in one", len(both), len(whole))
	}
	for i := range whole {
		if both[i] != whole[i] {
			t.Fatalf("slot %d: %+v, in one step %+v", i, both[i], whole[i])
		}
	}
}

// Shows in one weekly position take turns a season each: A's first season, B's, then A's second.
func TestWeeklyTurns(t *testing.T) {
	set := testSettings()
	a, b := show(1, 2, 3, 22*time.Minute, "TV-PG"), show(2, 2, 3, 22*time.Minute, "TV-PG")
	p := &planner{ch: Channel{Key: "c", Kind: ChannelShows, Titles: []*Title{a, b}}, set: set, st: NewState(set),
		primes: map[int]*[7][]position{}, pools: map[string][]*Title{}}
	var w [7][]position
	w[time.Monday] = []position{{0, halfHour, []*Title{a, b}}}
	p.primes[2026] = &w
	var got []string
	for week := range 10 {
		day := time.Date(2026, 11, 30+7*week, 6, 0, 0, 0, chicago)
		at := time.Date(day.Year(), day.Month(), day.Day(), 20, 0, 0, 0, chicago)
		title, item, track := p.pickWeekly(at, day)
		p.st.Record(Slot{Start: at, Length: 22 * time.Minute, Kind: SlotProgram, Title: title.Key, Item: item, Track: track})
		it := title.Items[item]
		got = append(got, fmt.Sprintf("%d:S%dE%d", title.TMDBID, it.Season, it.Episode))
	}
	want := "1:S1E1 1:S1E2 1:S1E3 2:S1E1 2:S1E2 2:S1E3 1:S2E1 1:S2E2 1:S2E3 2:S2E1"
	if strings.Join(got, " ") != want {
		t.Errorf("turns = %v\nwant %s", got, want)
	}
}

func TestPlanMoviesAndFiller(t *testing.T) {
	set := testSettings()
	var titles []*Title
	for i := 1; i <= 6; i++ {
		titles = append(titles, movie(i, time.Duration(85+10*i)*time.Minute, "PG", "Action"))
	}
	ch := Channel{Key: "lib/movies:action", Kind: ChannelMovies, Titles: titles}
	from := time.Date(2026, 12, 5, 6, 0, 0, 0, chicago)
	to := from.Add(48 * time.Hour)
	ads := clips(30)
	slots := Plan(ch, from, to, NewState(set), ads)
	checkContiguous(t, slots, from, to)

	var order []string
	lastClip := map[string]time.Time{}
	for _, s := range slots {
		switch s.Kind {
		case SlotProgram:
			order = append(order, s.Title)
		case SlotFiller:
			if last, ok := lastClip[s.Path]; ok && s.Start.Sub(last) < clipRepeatAfter {
				t.Errorf("clip %s again after %v", s.Path, s.Start.Sub(last))
			}
			lastClip[s.Path] = s.Start
		}
	}
	// every movie airs once before any airs again
	seen := map[string]bool{}
	for _, k := range order[:6] {
		if seen[k] {
			t.Errorf("movie %s again before the others: %v", k, order)
		}
		seen[k] = true
	}
	// clips near the movie's year (1988) come first
	for _, s := range slots {
		if s.Kind == SlotFiller {
			for _, c := range ads {
				if c.Path == s.Path && abs(c.Year-1988) > clipNearYears {
					t.Errorf("first clip %s is from %d", c.Path, c.Year)
				}
			}
			break
		}
	}
}

// Without clips, the gap after a program is one bumper; without titles, half-hour bumpers.
func TestPlanWithoutFiller(t *testing.T) {
	set := testSettings()
	from := time.Date(2026, 12, 5, 6, 0, 0, 0, chicago)
	slots := Plan(Channel{Key: "c", Kind: ChannelShows, Titles: []*Title{show(1, 1, 3, 22*time.Minute, "TV-G")}},
		from, from.Add(time.Hour), NewState(set), nil)
	if len(slots) != 4 || slots[1].Kind != SlotBumper || slots[1].Length != 8*time.Minute {
		t.Errorf("slots = %+v", slots)
	}
	empty := Plan(Channel{Key: "c", Kind: ChannelShows}, from, from.Add(time.Hour), NewState(set), nil)
	if len(empty) != 2 || empty[0].Kind != SlotBumper || empty[0].Length != halfHour {
		t.Errorf("empty channel = %+v", empty)
	}
}

func TestDayParts(t *testing.T) {
	set := Settings{PrimeTime: 19, Location: chicago}
	for _, c := range []struct {
		at   string
		part string
	}{
		{"2026-12-05 08:00", partKids}, // Saturday
		{"2026-12-05 13:00", partWeekend},
		{"2026-12-07 07:00", partMorning}, // Monday
		{"2026-12-07 12:00", partDay},
		{"2026-12-07 17:30", partFringe},
		{"2026-12-07 19:00", partPrime},
		{"2026-12-07 21:59", partPrime},
		{"2026-12-07 22:00", partLate}, // the news hour, prime at 7
		{"2026-12-08 05:59", partLate},
	} {
		at, _ := time.ParseInLocation("2006-01-02 15:04", c.at, chicago)
		if got, _ := set.part(at); got != c.part {
			t.Errorf("%s: %s, want %s", c.at, got, c.part)
		}
	}
}
