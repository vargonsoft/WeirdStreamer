package guide

import (
	"testing"
	"time"
)

func TestZipPoint(t *testing.T) {
	lat, lon, ok := ZipPoint("10001")
	if !ok || lat < 40.7 || lat > 40.8 || lon > -73.9 || lon < -74.1 {
		t.Fatalf("10001: %v %v %v", lat, lon, ok)
	}
	if _, _, ok := ZipPoint("00000"); ok {
		t.Fatal("00000 is no ZIP code")
	}
}

func TestConditionIcon(t *testing.T) {
	for _, c := range []struct {
		text string
		day  bool
		want string
	}{
		{"Heavy Rain", true, "rain"},
		{"Chance Light Rain", true, "rain"},
		{"Mostly Sunny", true, "cloudsun"},
		{"Partly Cloudy", false, "cloudmoon"},
		{"Sunny", true, "sun"},
		{"Clear", false, "moon"},
		{"Cloudy", true, "cloud"},
		{"Chance Showers And Thunderstorms", true, "storm"},
		{"Snow Likely", false, "snow"},
		{"Patchy Fog", true, "fog"},
		{"Breezy", true, "wind"},
	} {
		if got := conditionIcon(c.text, c.day); got != c.want {
			t.Errorf("%q: %s, want %s", c.text, got, c.want)
		}
	}
	for _, n := range WeatherIcons {
		if _, ok := WeatherIconPNG(n, 64, LogoColor("ffffff")); !ok {
			t.Errorf("no icon %s", n)
		}
	}
}

func TestMusicAtFollowsTheClock(t *testing.T) {
	tracks := []MusicTrack{{Name: "a.mp3", Duration: 3 * time.Minute}, {Name: "b.mp3", Duration: 4 * time.Minute},
		{Name: "c.mp3", Duration: 2 * time.Minute}}
	at := time.Date(2026, 11, 26, 9, 15, 30, 0, time.Local)
	tr, start, end, ok := musicAt(tracks, at)
	if !ok || at.Before(start) || !at.Before(end) || end.Sub(start) != tr.Duration {
		t.Fatalf("%v %v %v %v", tr.Name, start, end, ok)
	}
	// the same answer for anyone asking, and the next track starts where this one ends
	if tr2, start2, _, _ := musicAt(tracks, at); tr2.Name != tr.Name || !start2.Equal(start) {
		t.Fatal("not the same for everyone")
	}
	if tr3, start3, _, _ := musicAt(tracks, end); !start3.Equal(end) || tr3.Name == "" {
		t.Fatalf("the next track starts at %v, not %v", start3, end)
	}
}

func TestGuideChannelBlocks(t *testing.T) {
	now := time.Date(2026, 11, 26, 19, 0, 0, 0, time.Local)
	upcoming := []Airing{
		{Channel: 15, Start: now.Add(time.Hour), Length: 45 * time.Minute, Path: "lib/show.mkv", Info: &ProgramInfo{Title: "Show"}},
		{Channel: 14, Start: now.Add(time.Minute), Length: 2 * time.Hour, Path: "lib/soon.mkv", Info: &ProgramInfo{Title: "Too soon"}},
	}
	clips := []Clip{{Path: "ads/a.mp4", Duration: 30 * time.Second}, {Path: "ads/long.mp4", Duration: 2 * time.Minute}}
	seen := map[string]bool{}
	for i := range 8 {
		at := now.Add(time.Duration(i) * GuideBlock).Add(5 * time.Second)
		b := GuideChannelBlock(at, upcoming, clips)
		seen[b.Kind] = true
		if b.Start.After(at) || !b.End.After(at) || b.End.Sub(b.Start) != GuideBlock {
			t.Fatalf("block %v–%v doesn't hold %v", b.Start, b.End, at)
		}
		switch b.Kind {
		case BlockPreview:
			if b.Path != "lib/show.mkv" || b.Offset < 0 || b.Offset+GuideBlock > 45*time.Minute {
				t.Fatalf("preview %s at %v", b.Path, b.Offset)
			}
		case BlockPromo:
			if b.Airing == nil || b.Airing.Info.Title != "Show" {
				t.Fatal("a promo card without its program")
			}
		case BlockFiller:
			if b.Path != "ads/a.mp4" {
				t.Fatalf("filler %s: too long for a block", b.Path)
			}
		}
	}
	for _, k := range []string{BlockPreview, BlockPromo, BlockFiller} {
		if !seen[k] {
			t.Errorf("no %s block", k)
		}
	}
	// nothing ahead and no commercials: cards
	if b := GuideChannelBlock(now, nil, nil); b.Kind != BlockPromo || b.Airing != nil {
		t.Fatalf("empty: %+v", b)
	}
}
