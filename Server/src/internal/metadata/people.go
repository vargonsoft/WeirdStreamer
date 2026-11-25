package metadata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// ErrUnknownPerson is returned for a person no stored title credits.
var ErrUnknownPerson = errors.New("no title credits that person")

// personDetails returns a person's details, fetched from TMDB (credits and a larger photo) unless
// stored ones are fresh; stored ones serve when TMDB can't be reached.
func (s *Service) personDetails(ctx context.Context, id int) (*database.PersonDetails, error) {
	stored, err := s.db.GetPersonDetails(id)
	if err != nil {
		return nil, err
	}
	if stored != nil && time.Since(stored.UpdatedAt) < refreshDetails && (stored.Photo == "" || artworkPresent(stored.Photo)) {
		return stored, nil
	}
	client := s.tmdb()
	if client == nil {
		if stored != nil {
			return stored, nil
		}
		return nil, ErrNoToken
	}
	d, err := client.Person(ctx, id)
	if err != nil {
		if stored != nil {
			return stored, nil
		}
		return nil, err
	}
	p := &database.PersonDetails{TMDBID: id, Name: d.Name, Biography: d.Biography, Birthday: d.Birthday,
		Deathday: d.Deathday, PlaceOfBirth: d.PlaceOfBirth, KnownFor: d.KnownForDepartment,
		Photo:     s.download(ctx, client, "h632", d.ProfilePath, fmt.Sprintf("person/%dlarge", id)),
		Credits:   personCredits(d),
		UpdatedAt: time.Now()}
	if err := s.db.SavePersonDetails(p); err != nil {
		return nil, err
	}
	return p, nil
}

// personCredits merges a person's cast and crew credits into one per title (their parts joined),
// newest first, undated (announced) ones at the top.
func personCredits(d *PersonDetails) []database.PersonCredit {
	type key struct {
		kind string
		id   int
	}
	byTitle := map[key]*database.PersonCredit{}
	var order []key
	add := func(e PersonCreditEntry, role string) {
		kind, title, date := "movie", e.Title, e.ReleaseDate
		if e.MediaType == "tv" {
			kind, title, date = "show", e.Name, e.FirstAirDate
		} else if e.MediaType != "movie" {
			return
		}
		k := key{kind, e.ID}
		c, ok := byTitle[k]
		if !ok {
			c = &database.PersonCredit{Kind: kind, TMDBID: e.ID, Title: title, Date: date, Poster: e.PosterPath}
			byTitle[k] = c
			order = append(order, k)
		}
		if role != "" && !strings.Contains(c.Role, role) {
			if c.Role != "" {
				c.Role += ", "
			}
			c.Role += role
		}
	}
	for _, e := range d.CombinedCredits.Cast {
		add(e, e.Character)
	}
	for _, e := range d.CombinedCredits.Crew {
		add(e, e.Job)
	}
	out := make([]database.PersonCredit, 0, len(order))
	for _, k := range order {
		out = append(out, *byTitle[k])
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if (a == "") != (b == "") {
			return a == ""
		}
		return a > b
	})
	return out
}

// Person builds a person's page for a library (its listing annotated with the user's progress):
// their details, the library's titles they're credited in, and their filmography.
func (s *Service) Person(ctx context.Context, listing *models.LibraryListing, id int) (*models.PersonDetail, error) {
	if ok, err := s.db.PersonCredited(id); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrUnknownPerson
	}
	d, err := s.personDetails(ctx, id)
	if err != nil {
		return nil, err
	}
	owned := map[titleKey]models.LibraryTitle{}
	for _, t := range listing.Titles {
		owned[titleKey{t.Kind, t.TMDBID}] = t
	}
	imageBase := tmdbImageBase
	if c := s.tmdb(); c != nil {
		imageBase = c.imageBase
	}
	out := &models.PersonDetail{ID: id, Name: d.Name, Photo: ArtworkURL(d.Photo, d.UpdatedAt), Biography: d.Biography,
		Birthday: d.Birthday, Deathday: d.Deathday, PlaceOfBirth: d.PlaceOfBirth, KnownFor: d.KnownFor,
		InLibrary: []models.LibraryTitle{}, Filmography: []models.PersonCredit{}}
	for _, c := range d.Credits {
		credit := models.PersonCredit{Kind: c.Kind, TMDBID: c.TMDBID, Title: c.Title, Year: yearOf(c.Date), Role: c.Role}
		if c.Poster != "" {
			credit.Poster = imageBase + "w185" + c.Poster
		}
		if t, ok := owned[titleKey{c.Kind, c.TMDBID}]; ok {
			credit.InLibrary = true
			t.Role = c.Role
			out.InLibrary = append(out.InLibrary, t)
		}
		out.Filmography = append(out.Filmography, credit)
	}
	return out, nil
}
