package guide

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func ff(path string, minutes float64) FolderFile {
	return FolderFile{Path: "lib/" + path, Duration: time.Duration(minutes * float64(time.Minute))}
}

func TestFolderSeries(t *testing.T) {
	files := []FolderFile{
		ff("Shows/Star Run/Season 2/Star Run - S02E01.mp4", 30),
		ff("Shows/Star Run/Season 1/Star Run [720p] - S01E02.mp4", 30),
		ff("Shows/Star Run/Season 1/Star Run [1080p] - S01E01.mp4", 30),
		ff("Shows/Star Run/Specials/Star Run - S00E01.mp4", 30),
		ff("Calendars/1989.avi", 50),
		ff("Calendars/1987.avi", 50),
		ff("Calendars/1988.avi", 50),
		ff("Calendars/Review (1992).avi", 50),
		ff("Talks/Episode10_s10__1080p_h264.mp4", 25),
		ff("Talks/Episode2_s02__1080p_h264.mp4", 25),
		ff("Talks/Episode1_s01__1080p_h264.mp4", 25),
	}
	series, _, _, _ := FolderTitles("lib", "Library", files)
	got := map[string][]string{}
	for _, s := range series {
		for _, it := range s.Items {
			got[s.Name] = append(got[s.Name], it.Path[len("lib/"):]+"|"+strconv.Itoa(it.Season)+"x"+strconv.Itoa(it.Episode))
		}
	}
	want := map[string][]string{
		"Star Run": {"Shows/Star Run/Season 1/Star Run [1080p] - S01E01.mp4|1x1", "Shows/Star Run/Season 1/Star Run [720p] - S01E02.mp4|1x2",
			"Shows/Star Run/Season 2/Star Run - S02E01.mp4|2x1", "Shows/Star Run/Specials/Star Run - S00E01.mp4|0x1"},
		"Calendars": {"Calendars/1987.avi|1x1", "Calendars/1988.avi|1x2", "Calendars/1989.avi|1x3", "Calendars/Review (1992).avi|1x0"},
		"Talks":     {"Talks/Episode1_s01__1080p_h264.mp4|1x1", "Talks/Episode2_s02__1080p_h264.mp4|1x2", "Talks/Episode10_s10__1080p_h264.mp4|1x3"},
	}
	if len(got) != len(want) {
		t.Fatalf("series %v", got)
	}
	for name, w := range want {
		g := got[name]
		if len(g) != len(w) {
			t.Errorf("%s: %v", name, g)
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s[%d] = %s, want %s", name, i, g[i], w[i])
			}
		}
	}
}

func TestEpisodeTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Velvet Hour - S01E02 - The Pier": "The Pier",
		"Velvet Hour - S01E02":            "",
		"Episode5 s05":                    "Episode5 s05",
		"The Pier S01E02":                 "The Pier",
	} {
		if got := episodeTitle(in, "Velvet Hour"); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestFileNames(t *testing.T) {
	for in, want := range map[string]string{
		"a/Harbor.Lights.Vol.01.avi":           "Harbor Lights Vol 01",
		"a/Sunset_Drive_-_Part_Two_1080.mp4":   "Sunset Drive - Part Two",
		"a/Episode5_s05__1080p_h264.mp4":       "Episode5 s05",
		"a/Weekly-Pick-2017-01-18-Pier-4k.mp4": "Weekly-Pick-2017-01-18-Pier",
		"a/1987.avi":                           "1987",
	} {
		if got := fileTitle(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"Velvet Hour - S01E02 - The Pier": "The Pier",
		"Velvet Hour - S01E02":            "",
		"Velvet Hour":                     "",
		"S01E02":                          "",
		"Episode5 s05":                    "Episode5 s05",
	} {
		if got := episodeSummary(in, "Velvet Hour"); got != want {
			t.Errorf("summary %q: %q, want %q", in, got, want)
		}
	}
}

func TestFolderCollections(t *testing.T) {
	var files []FolderFile
	// a folder of differently named long videos, and one with short clips in small subfolders
	for i, n := range []string{"Sunset Drive", "City Lights", "The Long Road", "Night Swim", "Harbor"} {
		files = append(files, ff("Features/"+n+".mp4", 40+float64(i)))
	}
	for i, who := range []string{"Ann", "Bea", "Cy", "Dee", "Eve", "Flo", "Gus"} {
		files = append(files, ff("Shorts/"+who+"/Morning walk "+who+".mp4", 5+float64(i%3)))
		files = append(files, ff("Shorts/"+who+"/Beach day with "+who+".mp4", 6))
	}
	files = append(files, ff("loose clip.mp4", 3), ff("another loose one.mp4", 4))
	series, long, blocks, clips := FolderTitles("lib", "Library", files)
	if len(series) != 0 {
		t.Errorf("series %v", series[0].Name)
	}
	if len(long) != 1 || long[0].Name != "Features" || len(long[0].Items) != 5 {
		t.Fatalf("long %+v", long)
	}
	names := map[string]int{}
	for _, b := range blocks {
		names[b.Name] += len(b.Items)
		if !b.Block || b.BlockLength != halfHour || b.typicalSlot() != halfHour {
			t.Errorf("%s: block %v of %v", b.Name, b.Block, b.BlockLength)
		}
	}
	// the people's folders (two clips each) join Shorts; the loose clips are the library's own
	if names["Shorts"] != 14 || names["Library"] != 2 || len(names) != 2 {
		t.Errorf("blocks %v", names)
	}
	if len(clips) != 16 {
		t.Errorf("%d clips", len(clips))
	}
}

func hoursTitle(key string, hours float64) *Title {
	return &Title{Key: key, Kind: KindShow, Items: []Item{{Path: key, Duration: time.Duration(hours * float64(time.Hour))}}}
}

func TestTypedChannelSplit(t *testing.T) {
	groups := map[string][]*Title{}
	for i := range 100 {
		groups[TypedSeries] = append(groups[TypedSeries], hoursTitle("s"+strconv.Itoa(i), 9.8)) // 980 hours
	}
	groups[TypedLong] = []*Title{hoursTitle("l1", 150), hoursTitle("l2", 150)}
	groups[TypedClips] = []*Title{hoursTitle("c1", 100)}
	keys := func(chans []Channel) []string {
		var out []string
		for _, c := range chans {
			out = append(out, c.Key[len(TypedLibraryID("d"))+1:])
		}
		return out
	}
	check := func(max int, want ...string) {
		t.Helper()
		got := keys(TypedChannels("d", TypedLateNight, max, groups))
		if len(got) != len(want) {
			t.Fatalf("max %d: %v, want %v", max, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("max %d: %v, want %v", max, got, want)
			}
		}
	}
	check(4, "series:1", "series:2", "long:1", "clips:1")
	check(3, "series:1", "long:1", "clips:1")
	check(2, "series:1", "long:1")
	check(1, "series:1")
	small := map[string][]*Title{TypedSeries: {hoursTitle("a", 10)}, TypedClips: {hoursTitle("b", 3)}}
	if got := keys(TypedChannels("d", TypedLateNight, 4, small)); len(got) != 1 || got[0] != "series:1" {
		t.Errorf("small: %v", got)
	}
	chans := TypedChannels("d", TypedLateNight, 4, groups)
	if !chans[0].AnyTime || chans[0].Type != TypedLateNight || chans[0].LibraryID != TypedLibraryID("d") {
		t.Errorf("channel %+v", chans[0])
	}
}

func TestPlanBlock(t *testing.T) {
	clip := func(name string, minutes int) Item {
		return Item{Path: "lib/" + name + ".mp4", Duration: time.Duration(minutes) * time.Minute, Name: name}
	}
	title := &Title{Key: "folder:x:clips", Kind: KindShow, Name: "Shorts", Folder: true, Block: true, BlockLength: halfHour,
		Items: []Item{clip("a", 8), clip("b", 7), clip("c", 12), clip("d", 5), clip("e", 9), clip("f", 6)}}
	ch := Channel{Key: "typed:d/clips:1", Kind: ChannelShows, AnyTime: true, Titles: []*Title{title}}
	set := Settings{PrimeTime: 20, Location: time.UTC}
	from := time.Date(2026, 11, 26, 13, 0, 0, 0, time.UTC)
	st := NewState(set)
	// the channel's own clips fill what's left: none aired in the last 2 hours
	clips := []Clip{{Path: "lib/a.mp4", Duration: 8 * time.Minute}, {Path: "lib/g.mp4", Duration: 2 * time.Minute}}
	slots := Plan(ch, from, from.Add(2*halfHour), st, clips)
	var got []string
	for _, s := range slots {
		got = append(got, s.Kind+":"+s.Path)
	}
	// a (8) b (7) c (12) fill 27 minutes, d no longer fits, g (2) tops it up, a bumper the last minute; the
	// next block starts with the clips never aired: d e f
	want := []string{"program:lib/a.mp4", "filler:lib/b.mp4", "filler:lib/c.mp4", "filler:lib/g.mp4", "bumper:",
		"program:lib/d.mp4", "filler:lib/e.mp4", "filler:lib/f.mp4"}
	if len(got) < len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v…", got, want)
		}
	}
	if slots[0].Parts != 3 || slots[5].Parts < 3 {
		t.Errorf("parts %d, %d", slots[0].Parts, slots[5].Parts)
	}
	if slots[4].End() != from.Add(halfHour) || slots[5].Start != from.Add(halfHour) {
		t.Errorf("times %v %v", slots[4].End(), slots[5].Start)
	}
	if info := programInfo("lib", title, slots[0].Item, slots[0].Parts); !strings.Contains(info, `"title":"Shorts - a and 2 more"`) {
		t.Errorf("info %s", info)
	}
}

func TestTypedNames(t *testing.T) {
	taken := map[string]bool{}
	var got []string
	for range 5 {
		n := TypedName("kids", taken)
		taken[n] = true
		got = append(got, n)
	}
	want := []string{"Kid Zone", "Playground", "Toy Box", "Jamboree", "Kid Zone 2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%v, want %v", got, want)
		}
	}
	if !IsTypedName("kids", "Kid Zone 2") || IsTypedName("kids", "WKBT") || IsTypedName("comedy", "Kid Zone") {
		t.Error("IsTypedName")
	}
	// every type's names have their logos
	for typ, names := range TypedChannelNames {
		for _, n := range names {
			if a := LogoFor(n, TypedTypeName(typ), false); a == nil || a.Name != n {
				t.Errorf("%s: no logo of its own", n)
			}
		}
	}
}
