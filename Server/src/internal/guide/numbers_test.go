package guide

import (
	"testing"

	"github.com/weirdstreamer/server/internal/database"
)

func TestRenumberIntoBlocks(t *testing.T) {
	// the live server's lineup before the blocks: networks 2-9, genres from 14, Weather after them
	var chans []database.GuideChannel
	add := func(key string, n int, name, kind string) {
		chans = append(chans, database.GuideChannel{Key: key, Number: n, Name: name, Kind: kind, Active: true})
	}
	add(GuideChannelKey, 1, "Guide", KindGuideChannel)
	for i, station := range []string{"WGHB", "WLVR", "KFBZ", "WKBT", "WZGT", "KZXM", "WNSF", "KRGL"} {
		add("net"+station, 2+i, station, ChannelShows)
	}
	for i, genre := range []string{"Kids", "Cartoons", "Comedy", "Sci-Fi", "Drama", "Real Life", "Variety"} {
		add("tv"+genre, 14+i, genre, ChannelShows)
	}
	for i, genre := range []string{"Family Movies", "Fright Night", "Sci-Fi Movies", "Action Movies", "Comedy Movies", "Drama Movies"} {
		add("mov"+genre, 21+i, genre, ChannelMovies)
	}
	add(WeatherChannelKey, 27, "Weather", KindWeather)

	moves := Renumber(chans, nil)
	// networks and Weather (27, already between them and the genre channels) stay
	if _, ok := moves["netWGHB"]; ok || len(moves) != 13 {
		t.Fatalf("moves %v", moves)
	}
	if _, ok := moves[WeatherChannelKey]; ok || moves["tvKids"] != 30 || moves["tvVariety"] != 36 ||
		moves["movFamily Movies"] != 50 || moves["movDrama Movies"] != 55 {
		t.Fatalf("moves %v", moves)
	}
	// applied, the lineup is where it belongs, and a second pass moves nothing
	for i := range chans {
		if n, ok := moves[chans[i].Key]; ok {
			chans[i].Number = n
		}
	}
	if again := Renumber(chans, nil); len(again) != 0 {
		t.Fatalf("moved again: %v", again)
	}
	// a new genre channel takes the next number of its block
	existing := map[string]database.GuideChannel{}
	for _, c := range chans {
		existing[c.Key] = c
	}
	if n := nextNumber(database.GuideChannel{Name: "Westerns", Kind: ChannelShows}, existing, nil); n != 37 {
		t.Fatalf("new genre channel %d", n)
	}
	if n := nextNumber(database.GuideChannel{Name: "WTJM", Kind: ChannelShows}, existing, nil); n != 10 {
		t.Fatalf("new network channel %d", n)
	}
}

func TestFullBlockOverflows(t *testing.T) {
	var chans []database.GuideChannel
	existing := map[string]database.GuideChannel{}
	for n := 50; n <= 69; n++ {
		c := database.GuideChannel{Key: string(rune('a' + n - 50)), Number: n, Kind: ChannelMovies}
		chans = append(chans, c)
		existing[c.Key] = c
	}
	if n := nextNumber(database.GuideChannel{Kind: ChannelMovies}, existing, nil); n != 200 {
		t.Fatalf("overflow %d", n)
	}
	if moves := Renumber(chans, nil); len(moves) != 0 {
		t.Fatalf("moved %v", moves)
	}
}

func TestTypedBlocks(t *testing.T) {
	typed := Typed{"d1": 100, "d2": 110}
	existing := map[string]database.GuideChannel{
		"a": {Key: "a", Number: 100, LibraryID: TypedLibraryID("d1"), Kind: ChannelShows},
	}
	if n := nextNumber(database.GuideChannel{LibraryID: TypedLibraryID("d1"), Kind: ChannelShows}, existing, typed); n != 101 {
		t.Errorf("d1's next %d, want 101", n)
	}
	if n := nextNumber(database.GuideChannel{LibraryID: TypedLibraryID("d2"), Kind: ChannelMovies}, existing, typed); n != 110 {
		t.Errorf("d2's first %d, want 110", n)
	}
	chans := []database.GuideChannel{
		{Key: "a", Number: 100, LibraryID: TypedLibraryID("d1"), Kind: ChannelShows},
		{Key: "b", Number: 103, LibraryID: TypedLibraryID("d2"), Kind: ChannelShows}, // d2's block moved: 110 on
		{Key: "c", Number: 105, LibraryID: "lib", Kind: ChannelMovies},               // an old overflow: back to its block
		{Key: "d", Number: 230, LibraryID: "lib", Kind: ChannelShows},                // overflow stays
	}
	moves := Renumber(chans, typed)
	if moves["b"] != 110 || moves["c"] != 50 || len(moves) != 2 {
		t.Errorf("moves %v", moves)
	}
}
