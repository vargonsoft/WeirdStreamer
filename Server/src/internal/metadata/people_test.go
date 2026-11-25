package metadata

import "testing"

// A person's cast and crew credits become one per title (parts joined), newest first, undated first.
func TestPersonCredits(t *testing.T) {
	d := &PersonDetails{}
	d.CombinedCredits.Cast = []PersonCreditEntry{
		{ID: 603, MediaType: "movie", Title: "The Matrix", ReleaseDate: "1999-03-30", Character: "Neo"},
		{ID: 1399, MediaType: "tv", Name: "A Show", FirstAirDate: "2011-04-17", Character: "Himself"},
		{ID: 9, MediaType: "movie", Title: "Announced"},
		{ID: 5, MediaType: "person", Title: "Not a title"},
	}
	d.CombinedCredits.Crew = []PersonCreditEntry{
		{ID: 603, MediaType: "movie", Title: "The Matrix", ReleaseDate: "1999-03-30", Job: "Stunts"},
		{ID: 603, MediaType: "movie", Title: "The Matrix", ReleaseDate: "1999-03-30", Job: "Stunts"},
	}
	got := personCredits(d)
	if len(got) != 3 || got[0].TMDBID != 9 || got[1].Kind != "show" || got[2].TMDBID != 603 {
		t.Fatalf("order: %+v", got)
	}
	if got[2].Role != "Neo, Stunts" {
		t.Errorf("parts joined: %q", got[2].Role)
	}
}
