package metadata

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// ErrNotInLibrary is returned for a title page of a title the library has no files for.
var ErrNotInLibrary = errors.New("title not found in this library")

// libraryIndex is a library's video files grouped by title lookup key, as found by a sync.
type libraryIndex struct {
	files     map[string][]string // lookup key -> files (TV: episode files)
	other     map[string][]string // tv: show key -> files in its folder that aren't episodes
	unmatched []string            // files that resolve to no title
}

func newLibraryIndex() *libraryIndex {
	return &libraryIndex{files: map[string][]string{}, other: map[string][]string{}}
}

// FileInfo fills in what's known about a file (size, duration, thumbnail); set by the server.
type FileInfo func(path string) models.LibraryFile

// libraryIndexFor returns a library's index, walking the library first if there is none yet or
// browsing found new files since.
func (s *Service) libraryIndexFor(ctx context.Context, lib config.MediaPath) (*libraryIndex, error) {
	s.mu.Lock()
	idx, stale := s.indexes[lib.ID], s.staleIndex[lib.ID]
	s.mu.Unlock()
	if idx != nil && !stale {
		return idx, nil
	}
	if err := s.syncLibrary(ctx, lib); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.indexes[lib.ID], nil
}

// titleKey identifies a matched title: "movie:603", "show:1399".
type titleKey struct {
	kind string
	id   int
}

// matchedTitles groups a library's lookup keys by the title they're matched to; keys without a
// match are returned separately.
func (s *Service) matchedTitles(lib config.MediaPath, idx *libraryIndex) (map[titleKey][]string, []string) {
	byTitle := map[titleKey][]string{}
	var unmatched []string
	for key := range idx.files {
		l, err := s.db.GetLookup(lib.Type, lib.ID, key)
		if err != nil || l == nil || l.Status != "matched" || l.TMDBID == 0 {
			unmatched = append(unmatched, key)
			continue
		}
		kind := "movie"
		if l.MatchKind() == config.LibraryTypeTV {
			kind = "show"
		}
		tk := titleKey{kind, l.TMDBID}
		byTitle[tk] = append(byTitle[tk], key)
	}
	return byTitle, unmatched
}

// Library lists a movie or TV library's titles, sorted by title, and its unmatched files.
func (s *Service) Library(ctx context.Context, lib config.MediaPath, info FileInfo) (*models.LibraryListing, error) {
	idx, err := s.libraryIndexFor(ctx, lib)
	if err != nil {
		return nil, err
	}
	listing := &models.LibraryListing{LibraryID: lib.ID, LibraryName: lib.Name, LibraryType: lib.Type,
		Titles: []models.LibraryTitle{}, Unmatched: []models.LibraryFile{}}

	byTitle, unmatchedKeys := s.matchedTitles(lib, idx)
	for tk, keys := range byTitle {
		count := 0
		for _, k := range keys {
			count += len(idx.files[k])
		}
		t := models.LibraryTitle{Kind: tk.kind, TMDBID: tk.id, FileCount: count}
		for _, k := range keys {
			t.Paths = append(t.Paths, idx.files[k]...)
		}
		if tk.kind == "movie" {
			m, err := s.db.GetMovie(tk.id)
			if err != nil || m == nil {
				unmatchedKeys = append(unmatchedKeys, keys...) // details still downloading
				continue
			}
			t.Title, t.Year, t.Poster = m.Title, m.Year, ArtworkURL(m.Poster, m.UpdatedAt)
			if m.CollectionID > 0 {
				t.CollectionID = m.CollectionID
			}
		} else {
			sh, err := s.db.GetShow(tk.id)
			if err != nil || sh == nil {
				unmatchedKeys = append(unmatchedKeys, keys...)
				continue
			}
			t.Title, t.Year, t.Poster = sh.Name, sh.Year, ArtworkURL(sh.Poster, sh.UpdatedAt)
		}
		listing.Titles = append(listing.Titles, t)
	}
	sort.Slice(listing.Titles, func(i, j int) bool {
		a, b := listing.Titles[i], listing.Titles[j]
		if sa, sb := SortTitle(a.Title), SortTitle(b.Title); sa != sb {
			return sa < sb
		}
		return a.Year < b.Year
	})

	listing.Collections = s.libraryCollections(listing.Titles)

	for _, key := range unmatchedKeys {
		files := idx.files[key]
		if lib.Type == config.LibraryTypeTV {
			// One entry per unmatched show, named by the title searched for.
			f := info(files[0])
			if l, _ := s.db.GetLookup(lib.Type, lib.ID, key); l != nil {
				f.Name = l.QueryTitle
			} else if ref, ok := ResolveFile(lib, files[0]); ok {
				f.Name = ref.Title
			}
			f.Count = len(files)
			listing.Unmatched = append(listing.Unmatched, f)
			continue
		}
		for _, path := range files {
			listing.Unmatched = append(listing.Unmatched, info(path))
		}
	}
	d := s.newDescriber()
	for _, path := range idx.unmatched {
		f := info(path)
		f.Match = d.extraSummary(lib, path)
		listing.Unmatched = append(listing.Unmatched, f)
	}
	sort.Slice(listing.Unmatched, func(i, j int) bool {
		return SortTitle(listing.Unmatched[i].Name) < SortTitle(listing.Unmatched[j].Name)
	})
	return listing, nil
}

// Title builds a title page: full metadata, credits, and the library's files for it (TV: by
// season and episode, as manual episode fixes map them).
func (s *Service) Title(ctx context.Context, lib config.MediaPath, kind string, tmdbID int, info FileInfo) (*models.TitleDetail, error) {
	idx, err := s.libraryIndexFor(ctx, lib)
	if err != nil {
		return nil, err
	}
	byTitle, _ := s.matchedTitles(lib, idx)
	keys := byTitle[titleKey{kind, tmdbID}]
	if len(keys) == 0 {
		return nil, ErrNotInLibrary
	}
	details, err := s.titleDetails(kind, tmdbID)
	if err != nil {
		return nil, err
	}
	detail := &models.TitleDetail{MediaDetails: *details}

	var files []string
	for _, k := range keys {
		files = append(files, idx.files[k]...)
	}
	sort.Strings(files)

	if lib.Type == config.LibraryTypeMovie {
		// Movie libraries list files, whether the title is a movie or a TV show (a TV movie).
		for _, path := range files {
			detail.Files = append(detail.Files, info(path))
		}
		return detail, nil
	}

	mappings := s.loadEpisodeMappings(lib.ID)
	type epKey struct{ season, episode int }
	episodes := map[epKey]*models.LibraryEpisode{}
	for _, path := range files {
		ref, ok := ResolveFile(lib, path)
		if !ok {
			continue
		}
		season, episode, _ := mappings.apply(ref, path)
		k := epKey{season, episode}
		ep := episodes[k]
		if ep == nil {
			ep = &models.LibraryEpisode{Season: season, Episode: episode, Title: filepath.Base(path)}
			if e, err := s.db.GetEpisode(tmdbID, season, episode); err == nil && e != nil {
				ep.Title, ep.Overview, ep.AirDate, ep.Runtime = e.Name, e.Overview, e.AirDate, e.Runtime
				ep.Still = ArtworkURL(e.Still, e.UpdatedAt)
			}
			episodes[k] = ep
		}
		ep.Files = append(ep.Files, info(path))
	}
	seasons := map[int]*models.LibrarySeason{}
	for _, ep := range episodes {
		se := seasons[ep.Season]
		if se == nil {
			se = &models.LibrarySeason{Season: ep.Season, Name: seasonLabel(ep.Season)}
			if dbSeason, err := s.db.GetSeason(tmdbID, ep.Season); err == nil && dbSeason != nil && dbSeason.Status == "fetched" {
				if dbSeason.Name != "" {
					se.Name = dbSeason.Name
				}
				se.Poster = ArtworkURL(dbSeason.Poster, dbSeason.FetchedAt)
			}
			seasons[ep.Season] = se
		}
		se.Episodes = append(se.Episodes, *ep)
	}
	for _, se := range seasons {
		sort.Slice(se.Episodes, func(i, j int) bool { return se.Episodes[i].Episode < se.Episodes[j].Episode })
		detail.Seasons = append(detail.Seasons, *se)
	}
	// Regular seasons first, specials (season 0) last.
	sort.Slice(detail.Seasons, func(i, j int) bool {
		a, b := detail.Seasons[i].Season, detail.Seasons[j].Season
		if (a == 0) != (b == 0) {
			return b == 0
		}
		return a < b
	})

	var other []string
	for _, k := range keys {
		other = append(other, idx.other[k]...)
	}
	sort.Strings(other)
	d := s.newDescriber()
	for _, path := range other {
		f := info(path)
		f.Match = d.extraSummary(lib, path) // a TV movie or special fixed by hand
		detail.OtherFiles = append(detail.OtherFiles, f)
	}
	return detail, nil
}

// titleDetails returns a movie's or show's metadata with credits.
func (s *Service) titleDetails(kind string, id int) (*models.MediaDetails, error) {
	details := &models.MediaDetails{}
	var owner string
	if kind == "movie" {
		m, err := s.db.GetMovie(id)
		if err != nil || m == nil {
			return nil, ErrNotInLibrary
		}
		details.ItemMetadata = models.ItemMetadata{Kind: "movie", TMDBID: m.TMDBID, Title: m.Title, Year: m.Year,
			Overview: m.Overview, Runtime: m.Runtime, Poster: ArtworkURL(m.Poster, m.UpdatedAt),
			Backdrop: ArtworkURL(m.Backdrop, m.UpdatedAt), Logo: ArtworkURL(m.Logo, m.UpdatedAt)}
		details.Tagline, details.Genres, details.ReleaseDate, details.IMDBID = m.Tagline, splitGenres(m.Genres), m.ReleaseDate, m.IMDBID
		owner = database.MovieOwner(id)
	} else {
		sh, err := s.db.GetShow(id)
		if err != nil || sh == nil {
			return nil, ErrNotInLibrary
		}
		details.ItemMetadata = models.ItemMetadata{Kind: "show", TMDBID: sh.TMDBID, Title: sh.Name, Year: sh.Year,
			Overview: sh.Overview, Runtime: sh.Runtime, Poster: ArtworkURL(sh.Poster, sh.UpdatedAt),
			Backdrop: ArtworkURL(sh.Backdrop, sh.UpdatedAt), Logo: ArtworkURL(sh.Logo, sh.UpdatedAt)}
		details.Tagline, details.Genres, details.ReleaseDate, details.IMDBID = sh.Tagline, splitGenres(sh.Genres), sh.FirstAirDate, sh.IMDBID
		owner = database.ShowOwner(id)
	}
	return details, s.addCredits(details, owner)
}

func seasonLabel(n int) string {
	if n == 0 {
		return "Specials"
	}
	return "Season " + strconv.Itoa(n)
}

// SortTitle is the key titles are listed by: lowercase, without a leading article.
func SortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(t, article) && len(t) > len(article) {
			return t[len(article):]
		}
	}
	return t
}

// libraryCollections lists the stored collections the titles' movies belong to, by name.
func (s *Service) libraryCollections(titles []models.LibraryTitle) []models.LibraryCollection {
	counts := map[int]int{}
	for _, t := range titles {
		if t.Kind == "movie" && t.CollectionID > 0 {
			counts[t.CollectionID]++
		}
	}
	var out []models.LibraryCollection
	for id, n := range counts {
		c, err := s.db.GetCollection(id)
		if err != nil || c == nil {
			continue // still being fetched
		}
		out = append(out, models.LibraryCollection{ID: id, Name: c.Name, Poster: ArtworkURL(c.Poster, c.UpdatedAt),
			Count: n, Total: len(c.Parts)})
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := SortTitle(out[i].Name), SortTitle(out[j].Name); a != b {
			return a < b
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ErrNoCollection is returned for a collection none of the library's movies belong to.
var ErrNoCollection = errors.New("no movie in this library belongs to that collection")

// Collection builds a collection's page from a library's listing (its titles already annotated
// with the user's progress): every film TMDB lists in it, by release date, the library's titles
// where it has them, the others marked missing; films of the library TMDB doesn't list come last.
func (s *Service) Collection(listing *models.LibraryListing, id int) (*models.CollectionDetail, error) {
	owned := map[int]models.LibraryTitle{}
	for _, t := range listing.Titles {
		if t.Kind == "movie" && t.CollectionID == id {
			owned[t.TMDBID] = t
		}
	}
	c, err := s.db.GetCollection(id)
	if err != nil {
		return nil, err
	}
	if c == nil || len(owned) == 0 {
		return nil, ErrNoCollection
	}
	detail := &models.CollectionDetail{ID: id, Name: c.Name, Overview: c.Overview,
		Poster: ArtworkURL(c.Poster, c.UpdatedAt), Backdrop: ArtworkURL(c.Backdrop, c.UpdatedAt), Parts: []models.LibraryTitle{}}
	for _, p := range c.Parts {
		if t, ok := owned[p.TMDBID]; ok {
			detail.Parts = append(detail.Parts, t)
			delete(owned, p.TMDBID)
			continue
		}
		detail.Parts = append(detail.Parts, models.LibraryTitle{Kind: "movie", TMDBID: p.TMDBID, Title: p.Title,
			Year: yearOf(p.ReleaseDate), Poster: ArtworkURL(p.Poster, c.UpdatedAt), CollectionID: id, Missing: true})
	}
	var rest []models.LibraryTitle
	for _, t := range owned {
		rest = append(rest, t)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Year < rest[j].Year })
	detail.Parts = append(detail.Parts, rest...)
	return detail, nil
}
