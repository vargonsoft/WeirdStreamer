package metadata

import "testing"

func TestParseTitleYear(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"10 Things I Hate About You (1999) - 1080p", "10 Things I Hate About You", 1999},
		{"8MM 2 (2005)", "8MM 2", 2005},
		{"A Knight's Tale (2001)", "A Knight's Tale", 2001},
		{"Harry Potter and the Sorcerers Stone (2001) [1080p]", "Harry Potter and the Sorcerers Stone", 2001},
		{"Harry Potter and the Deathly Hallows Part 2 2011 1080p Bluray x264 (DTS) - HarryLala", "Harry Potter and the Deathly Hallows Part 2", 2011},
		{"I.Tonya.2017.1080p.BluRay.H264.AAC-RARBG", "I Tonya", 2017},
		{"Example Film (1992) 720p HDTV DD5.1 MPEG2-TrollHD.ts", "Example Film", 1992},
		{"1917 (2019)", "1917", 2019},
		{"1917.2019.1080p.WEB-DL.mkv", "1917", 2019},
		{"2001 - A Space Odyssey (1968)", "2001 - A Space Odyssey", 1968},
		{"Inception.mkv", "Inception", 0},
		{"Hellboy II - The Golden Army", "Hellboy II - The Golden Army", 0},
		{"Toy Story 3 1080p BluRay.mkv", "Toy Story 3", 0},
		{"Shōgun (2024)", "Shōgun", 2024},
		{"Harry Potter and the Deathly Hallows - Part 1", "Harry Potter and the Deathly Hallows - Part 1", 0},
	}
	for _, c := range cases {
		title, year := ParseTitleYear(c.in)
		if title != c.title || year != c.year {
			t.Errorf("ParseTitleYear(%q) = (%q, %d), want (%q, %d)", c.in, title, year, c.title, c.year)
		}
	}
}

func TestParseEpisode(t *testing.T) {
	cases := []struct {
		in      string
		season  int
		episode int
		ok      bool
	}{
		{"1923 - S01E02 - Nature's Empty Throne Bluray-1080p.mkv", 1, 2, true},
		{"The Walking Dead - 4x13 - Alone.mkv", 4, 13, true},
		{"Star Trek - Deep Space Nine - S01E01-E02 - Emissary Bluray-720p.mkv", 1, 1, true},
		{"Show.S10E105.720p.mkv", 10, 105, true},
		{"Bluey - S00E03 - Special.mp4", 0, 3, true},
		{"Big Brother - Week 3 - Day 4.mkv", 0, 0, false},
		{"1923 Bluray-1080p.mkv", 0, 0, false},
		{"Movie 1920x1080.mkv", 0, 0, false},
	}
	for _, c := range cases {
		s, e, ok := ParseEpisode(c.in)
		if s != c.season || e != c.episode || ok != c.ok {
			t.Errorf("ParseEpisode(%q) = (%d, %d, %v), want (%d, %d, %v)", c.in, s, e, ok, c.season, c.episode, c.ok)
		}
	}
}

func TestParseSeasonFolder(t *testing.T) {
	cases := map[string]int{"Season 1": 1, "Season 01": 1, "S02": 2, "Specials": 0, "season 12": 12}
	for in, want := range cases {
		if got, ok := ParseSeasonFolder(in); !ok || got != want {
			t.Errorf("ParseSeasonFolder(%q) = (%d, %v), want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"Week 3", "Movies", "Vakna Nu"} {
		if _, ok := ParseSeasonFolder(in); ok {
			t.Errorf("ParseSeasonFolder(%q) matched, want no match", in)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	pairs := [][2]string{
		{"A Knights Tale", "A Knight's Tale"},
		{"Cats & Dogs - The Revenge of Kitty Galore", "Cats & Dogs: The Revenge of Kitty Galore"},
		{"Cats and Dogs", "Cats & Dogs"},
		{"Hellboy II - The Golden Army", "Hellboy II: The Golden Army"},
		{"The Lion King 1.5", "The Lion King 1½"},
	}
	for _, p := range pairs {
		if NormalizeTitle(p[0]) != NormalizeTitle(p[1]) {
			t.Errorf("NormalizeTitle(%q)=%q != NormalizeTitle(%q)=%q", p[0], NormalizeTitle(p[0]), p[1], NormalizeTitle(p[1]))
		}
	}
}

func TestParseShowFolder(t *testing.T) {
	cases := []struct {
		in, title string
		year      int
		country   string
	}{
		{"Bluey (2018)", "Bluey", 2018, ""},
		{"Big Brother (UK)", "Big Brother", 0, "GB"},
		{"The Traitors (US)", "The Traitors", 0, "US"},
		{"1923", "1923", 0, ""},
	}
	for _, c := range cases {
		title, year, country := ParseShowFolder(c.in)
		if title != c.title || year != c.year || country != c.country {
			t.Errorf("ParseShowFolder(%q) = (%q, %d, %q), want (%q, %d, %q)", c.in, title, year, country, c.title, c.year, c.country)
		}
	}
}
