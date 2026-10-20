package metadata

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// ErrNotFixable is returned for files and folders that don't stand for one title (collection
// folders, non-episode TV files) and so can't be matched by hand.
var ErrNotFixable = errors.New("this item doesn't stand for a single title; fix the files inside it instead")

// ErrNoToken is returned when no TMDB token is configured.
var ErrNoToken = errors.New("no TMDB access token is configured")

// MatchInfo describes what a file or folder was matched against, for the fix-match dialog.
type MatchInfo struct {
	Kind    string               `json:"kind"`  // "movie" or "tv"
	Query   string               `json:"query"` // the title that was (or will be) searched
	Year    int                  `json:"year,omitempty"`
	Status  string               `json:"status"` // "matched", "not_found", "error" or "pending"
	Manual  bool                 `json:"manual"`
	Current *models.ItemMetadata `json:"current,omitempty"`
	Episode *EpisodeInfo         `json:"episode,omitempty"` // TV episode files: which TMDB episode it shows as
}

// SearchHit is one TMDB search result offered in the fix-match dialog.
type SearchHit struct {
	TMDBID   int    `json:"tmdb_id"`
	Title    string `json:"title"`
	Year     int    `json:"year,omitempty"`
	Overview string `json:"overview,omitempty"`
	Poster   string `json:"poster,omitempty"` // TMDB image URL
	// TMDB image path of the poster, which SetPoster takes to use it for another title.
	PosterPath string `json:"poster_path,omitempty"`
}

// MatchTarget resolves a file or folder to the title lookup a manual fix applies to. Fixing a
// title fixes every file in the library that resolves to it. A TV season folder fixes its show.
func (s *Service) MatchTarget(lib config.MediaPath, path string, isDir bool) (Ref, error) {
	if !isDir {
		ref, ok := ResolveFile(lib, path)
		if !ok || (ref.Kind == config.LibraryTypeTV && !ref.IsEpisode) {
			return Ref{}, ErrNotFixable
		}
		return ref, nil
	}
	if lib.Type == config.LibraryTypeTV {
		path = ShowFolderOf(lib, path)
	}
	keys, err := s.db.FolderKeys(lib.ID, []string{path})
	if err != nil {
		return Ref{}, err
	}
	ref, ok := ResolveFolder(lib, path, keys[path])
	if !ok {
		return Ref{}, ErrNotFixable
	}
	ref.Season = 0
	if ref.Title == "" {
		// Key-only folder refs get their search title from the lookup the files made.
		if l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key); err == nil && l != nil {
			ref.Title, ref.Year, ref.Country = l.QueryTitle, l.QueryYear, l.QueryCountry
		}
	}
	return ref, nil
}

// MatchInfo reports the current match for a target ref.
func (s *Service) MatchInfo(ref Ref) (*MatchInfo, error) {
	info := &MatchInfo{Kind: ref.Kind, Query: ref.Title, Year: ref.Year, Status: "pending"}
	l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key)
	if err != nil || l == nil {
		return info, err
	}
	info.Query, info.Year, info.Status, info.Manual = l.QueryTitle, l.QueryYear, l.Status, l.Manual
	if l.Status != "matched" {
		return info, nil
	}
	if ref.Kind == config.LibraryTypeMovie {
		if m, err := s.db.GetMovie(l.TMDBID); err == nil && m != nil {
			info.Current = &models.ItemMetadata{Kind: "movie", TMDBID: m.TMDBID, Title: m.Title, Year: m.Year,
				Overview: m.Overview, Poster: ArtworkURL(m.Poster, m.UpdatedAt)}
		}
	} else if sh, err := s.db.GetShow(l.TMDBID); err == nil && sh != nil {
		info.Current = &models.ItemMetadata{Kind: "show", TMDBID: sh.TMDBID, Title: sh.Name, Year: sh.Year,
			Overview: sh.Overview, Poster: ArtworkURL(sh.Poster, sh.UpdatedAt)}
	}
	if info.Current == nil {
		info.Status = "pending" // matched, details still downloading
	}
	return info, nil
}

// Search runs a TMDB search for the fix-match dialog.
func (s *Service) Search(ctx context.Context, kind, query string, year int) ([]SearchHit, error) {
	client := s.tmdb()
	if client == nil {
		return nil, ErrNoToken
	}
	var results []SearchResult
	var err error
	if kind == config.LibraryTypeTV {
		results, err = client.SearchTV(ctx, query, year)
	} else {
		results, err = client.SearchMovie(ctx, query, year)
	}
	if err != nil {
		return nil, err
	}
	hits := []SearchHit{}
	for _, r := range results {
		title, date := r.Title, r.ReleaseDate
		if kind == config.LibraryTypeTV {
			title, date = r.Name, r.FirstAirDate
		}
		hit := SearchHit{TMDBID: r.ID, Title: title, Year: yearOf(date), Overview: truncate(r.Overview, 300)}
		if r.PosterPath != "" {
			hit.Poster, hit.PosterPath = client.imageBase+"w154"+r.PosterPath, r.PosterPath
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

// SetMatch records a manual match (tmdbID > 0) or a manual "no match" (tmdbID == 0) for ref's
// title, then fetches its details and artwork in the background.
func (s *Service) SetMatch(ref Ref, tmdbID int, notifyPath string) error {
	if s.tmdb() == nil {
		return ErrNoToken
	}
	l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key)
	if err != nil {
		return err
	}
	if l == nil {
		l = &database.Lookup{Kind: ref.Kind, LibraryID: ref.LibraryID, QueryKey: ref.Key,
			QueryTitle: ref.Title, QueryYear: ref.Year, QueryCountry: ref.Country}
	}
	l.TMDBID, l.Status, l.Manual, l.AttemptedAt = tmdbID, "matched", true, time.Now()
	if tmdbID == 0 {
		l.Status = "not_found"
	}
	if err := s.db.SaveLookup(l); err != nil {
		return err
	}
	if tmdbID > 0 {
		var seasons []int
		if ref.IsEpisode {
			seasons = []int{ref.Season}
		}
		s.enqueue(job{lookup: l, seasons: seasons, notifyPath: notifyPath})
		// A sync queues the seasons of every episode on disk for the new show.
		go s.SyncAll(context.Background())
	}
	return nil
}

// ResetMatch drops a title's lookup (manual or not) and matches it automatically again.
func (s *Service) ResetMatch(ref Ref, notifyPath string) error {
	l, err := s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key)
	if err != nil || l == nil {
		return err
	}
	if err := s.db.DeleteLookup(ref.Kind, ref.LibraryID, ref.Key); err != nil {
		return err
	}
	ref.Title, ref.Year, ref.Country = l.QueryTitle, l.QueryYear, l.QueryCountry
	s.EnqueueRef(ref, notifyPath)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], " ")
	if cut < n/2 {
		cut = n
	}
	return s[:cut] + "…"
}
