package metadata

import (
	"context"
	"sort"

	"github.com/weirdstreamer/server/internal/config"
)

// GuideTitle is a library title as the Guide plans with it: its TMDB details and its files (a
// movie's, or a show's episodes in order). Paths are disk paths.
type GuideTitle struct {
	Kind      string // "movie" or "show"
	TMDBID    int
	Title     string
	Year      int
	Genres    []string
	NetworkID int // shows: the network it first aired on (0 none)
	Network   string
	Rating    string // US certification or TV rating, "" none
	Overview  string
	Poster    string         // artwork URL
	Files     []string       // movies: its files, sorted
	Episodes  []GuideEpisode // shows: regular seasons in order, then specials
}

// GuideEpisode is an episode of a show on disk.
type GuideEpisode struct {
	Season, Episode int
	Title, Overview string
	Path            string // its first file
}

// GuideTitles lists a movie or TV library's matched titles with their files, for the Guide. Titles
// whose details aren't stored yet are left out (the next lookup brings them).
func (s *Service) GuideTitles(ctx context.Context, lib config.MediaPath) ([]GuideTitle, error) {
	idx, err := s.libraryIndexFor(ctx, lib)
	if err != nil {
		return nil, err
	}
	byTitle, _ := s.matchedTitles(lib, idx)
	var mappings episodeMappings
	if lib.Type == config.LibraryTypeTV {
		mappings = s.loadEpisodeMappings(lib.ID)
	}
	var titles []GuideTitle
	for tk, keys := range byTitle {
		var files []string
		for _, k := range keys {
			files = append(files, idx.files[k]...)
		}
		sort.Strings(files)
		t := GuideTitle{Kind: tk.kind, TMDBID: tk.id}
		if tk.kind == "movie" {
			m, err := s.db.GetMovie(tk.id)
			if err != nil || m == nil {
				continue
			}
			t.Title, t.Year, t.Genres, t.Rating, t.Overview = m.Title, m.Year, splitGenres(m.Genres), m.Rating, m.Overview
			t.Poster = ArtworkURL(m.Poster, m.UpdatedAt)
			t.Files = files
			titles = append(titles, t)
			continue
		}
		sh, err := s.db.GetShow(tk.id)
		if err != nil || sh == nil {
			continue
		}
		t.Title, t.Year, t.Genres, t.Rating, t.Overview = sh.Name, sh.Year, splitGenres(sh.Genres), sh.Rating, sh.Overview
		t.NetworkID, t.Network = sh.NetworkID, sh.Network
		t.Poster = ArtworkURL(sh.Poster, sh.UpdatedAt)
		if lib.Type != config.LibraryTypeTV {
			t.Files = files // a TV movie in a movie library
			titles = append(titles, t)
			continue
		}
		type epKey struct{ season, episode int }
		seen := map[epKey]bool{}
		for _, path := range files {
			ref, ok := ResolveFile(lib, path)
			if !ok {
				continue
			}
			season, episode, _ := mappings.apply(ref, path)
			k := epKey{season, episode}
			if seen[k] {
				continue
			}
			seen[k] = true
			ep := GuideEpisode{Season: season, Episode: episode, Path: path}
			if e, err := s.db.GetEpisode(tk.id, season, episode); err == nil && e != nil {
				ep.Title, ep.Overview = e.Name, e.Overview
			}
			t.Episodes = append(t.Episodes, ep)
		}
		sort.Slice(t.Episodes, func(i, j int) bool {
			a, b := t.Episodes[i], t.Episodes[j]
			if (a.Season == 0) != (b.Season == 0) {
				return b.Season == 0
			}
			if a.Season != b.Season {
				return a.Season < b.Season
			}
			return a.Episode < b.Episode
		})
		if len(t.Episodes) > 0 {
			titles = append(titles, t)
		}
	}
	sort.Slice(titles, func(i, j int) bool {
		if titles[i].Kind != titles[j].Kind {
			return titles[i].Kind < titles[j].Kind
		}
		return titles[i].TMDBID < titles[j].TMDBID
	})
	return titles, nil
}
