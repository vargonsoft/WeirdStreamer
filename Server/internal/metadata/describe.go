package metadata

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
	"github.com/weirdstreamer/server/internal/models"
)

// ArtworkURL turns an artwork-relative path into the URL clients load it from. The ?v= changes
// when the record is refreshed, so client image caches pick up new artwork.
func ArtworkURL(rel string, updated time.Time) string {
	if rel == "" {
		return ""
	}
	return "/artwork/" + rel + "?v=" + strconv.FormatInt(updated.Unix(), 36)
}

// describer resolves refs to metadata, caching records for the duration of one request.
type describer struct {
	s       *Service
	shows   map[int]*database.Show
	lookups map[string]*database.Lookup
}

func (s *Service) newDescriber() *describer {
	return &describer{s: s, shows: map[int]*database.Show{}, lookups: map[string]*database.Lookup{}}
}

// Annotate attaches metadata to a browse listing of folder (an absolute path in lib), and
// schedules lookups for anything not looked up yet.
func (s *Service) Annotate(lib config.MediaPath, folder string, dirs []models.Directory, items []models.MediaItem) {
	if lib.Type == "" {
		return
	}
	folders := []string{folder}
	for _, d := range dirs {
		folders = append(folders, filepath.Join(folder, d.Name))
	}
	folderKeys, err := s.db.FolderKeys(lib.ID, folders)
	if err != nil {
		return
	}

	d := s.newDescriber()
	for i := range dirs {
		path := filepath.Join(folder, dirs[i].Name)
		if ref, ok := ResolveFolder(lib, path, folderKeys[path]); ok {
			dirs[i].Metadata = d.summary(ref, path, true)
		}
	}
	for i := range items {
		if items[i].Type != "video" {
			continue
		}
		if ref, ok := ResolveFile(lib, items[i].Path, folderKeys[filepath.Dir(items[i].Path)]); ok {
			items[i].Metadata = d.summary(ref, items[i].Path, false)
		}
	}
}

// Details returns the full metadata for a file or folder in lib, or nil if none is known.
func (s *Service) Details(lib config.MediaPath, path string, isDir bool) (*models.MediaDetails, error) {
	keys, err := s.db.FolderKeys(lib.ID, []string{path, filepath.Dir(path)})
	if err != nil {
		return nil, err
	}
	var ref Ref
	var ok bool
	if isDir {
		ref, ok = ResolveFolder(lib, path, keys[path])
	} else {
		ref, ok = ResolveFile(lib, path, keys[filepath.Dir(path)])
	}
	if !ok {
		return nil, nil
	}
	d := s.newDescriber()
	summary := d.summary(ref, path, isDir)
	if summary == nil {
		return nil, nil
	}
	details := &models.MediaDetails{ItemMetadata: *summary}

	var owner string
	switch summary.Kind {
	case "movie":
		m, err := s.db.GetMovie(summary.TMDBID)
		if err != nil || m == nil {
			return details, err
		}
		details.Tagline, details.Genres, details.ReleaseDate, details.IMDBID = m.Tagline, splitGenres(m.Genres), m.ReleaseDate, m.IMDBID
		owner = database.MovieOwner(m.TMDBID)
	case "show", "season":
		sh := d.show(summary.TMDBID)
		if sh == nil {
			return details, nil
		}
		details.Tagline, details.Genres, details.ReleaseDate, details.IMDBID = sh.Tagline, splitGenres(sh.Genres), sh.FirstAirDate, sh.IMDBID
		owner = database.ShowOwner(sh.TMDBID)
	case "episode":
		owner = database.EpisodeOwner(summary.TMDBID, *summary.Season, *summary.Episode)
		if sh := d.show(summary.TMDBID); sh != nil {
			details.Genres, details.IMDBID = splitGenres(sh.Genres), sh.IMDBID
			// Episodes list guest stars; the regular cast and creators come from the show.
			if err := s.addCredits(details, database.ShowOwner(sh.TMDBID), "cast", "creator"); err != nil {
				return nil, err
			}
		}
	}
	return details, s.addCredits(details, owner)
}

// addCredits appends owner's credits to details, optionally limited to some roles.
func (s *Service) addCredits(details *models.MediaDetails, owner string, roles ...string) error {
	credits, err := s.db.Credits(owner)
	if err != nil {
		return err
	}
	for _, c := range credits {
		if len(roles) > 0 && !contains(roles, c.Role) {
			continue
		}
		info := models.CreditInfo{TMDBID: c.Person.TMDBID, Name: c.Person.Name, Character: c.Character}
		if c.Person.Profile != "" {
			info.Profile = "/artwork/" + c.Person.Profile
		}
		switch c.Role {
		case "cast":
			details.Cast = append(details.Cast, info)
		case "guest":
			details.GuestStars = append(details.GuestStars, info)
		case "director":
			details.Directors = append(details.Directors, info)
		case "writer":
			details.Writers = append(details.Writers, info)
		case "creator":
			details.Creators = append(details.Creators, info)
		}
	}
	return nil
}

// summary returns ref's metadata, scheduling a lookup (or season fetch) when it is missing.
func (d *describer) summary(ref Ref, path string, isDir bool) *models.ItemMetadata {
	l := d.lookup(ref)
	if l == nil || (l.Status != "matched" && due(l.Status, l.AttemptedAt)) {
		d.s.EnqueueRef(ref, path)
		return nil
	}
	if l.Status != "matched" {
		return nil
	}

	if ref.Kind == config.LibraryTypeMovie {
		m, err := d.s.db.GetMovie(l.TMDBID)
		if err != nil || m == nil {
			d.s.EnqueueRef(ref, path)
			return nil
		}
		return &models.ItemMetadata{Kind: "movie", TMDBID: m.TMDBID, Title: m.Title, Year: m.Year,
			Overview: m.Overview, Runtime: m.Runtime,
			Poster: ArtworkURL(m.Poster, m.UpdatedAt), Backdrop: ArtworkURL(m.Backdrop, m.UpdatedAt), Logo: ArtworkURL(m.Logo, m.UpdatedAt)}
	}

	sh := d.show(l.TMDBID)
	if sh == nil {
		d.s.EnqueueRef(ref, path)
		return nil
	}
	show := &models.ItemMetadata{Kind: "show", TMDBID: sh.TMDBID, Title: sh.Name, Year: sh.Year,
		Overview: sh.Overview, Runtime: sh.Runtime,
		Poster: ArtworkURL(sh.Poster, sh.UpdatedAt), Backdrop: ArtworkURL(sh.Backdrop, sh.UpdatedAt), Logo: ArtworkURL(sh.Logo, sh.UpdatedAt)}

	switch {
	case isDir && !isSeasonDir(path):
		return show
	case isDir:
		season := ref.Season
		show.Kind, show.ShowTitle, show.Season = "season", sh.Name, &season
		if s, err := d.s.db.GetSeason(sh.TMDBID, season); err == nil && s != nil && s.Status == "fetched" {
			if s.Name != "" {
				show.Title = s.Name
			}
			if s.Overview != "" {
				show.Overview = s.Overview
			}
			if s.Poster != "" {
				show.Poster = ArtworkURL(s.Poster, s.FetchedAt)
			}
		} else {
			d.s.enqueueSeason(sh.TMDBID, season, path)
		}
		return show
	case !ref.IsEpisode:
		return nil
	}

	e, err := d.s.db.GetEpisode(sh.TMDBID, ref.Season, ref.Episode)
	if err != nil || e == nil {
		d.s.enqueueSeason(sh.TMDBID, ref.Season, path)
		return nil
	}
	season, episode := ref.Season, ref.Episode
	return &models.ItemMetadata{Kind: "episode", TMDBID: sh.TMDBID, Title: e.Name, Year: sh.Year,
		Overview: e.Overview, Runtime: e.Runtime, AirDate: e.AirDate, ShowTitle: sh.Name,
		Season: &season, Episode: &episode,
		Poster: show.Poster, Backdrop: show.Backdrop, Logo: show.Logo, Still: ArtworkURL(e.Still, e.UpdatedAt)}
}

func isSeasonDir(path string) bool {
	_, ok := ParseSeasonFolder(filepath.Base(path))
	return ok
}

func (d *describer) lookup(ref Ref) *database.Lookup {
	key := ref.Kind + "|" + ref.Key
	if l, ok := d.lookups[key]; ok {
		return l
	}
	l, err := d.s.db.GetLookup(ref.Kind, ref.LibraryID, ref.Key)
	if err != nil {
		return nil
	}
	d.lookups[key] = l
	return l
}

func (d *describer) show(id int) *database.Show {
	if sh, ok := d.shows[id]; ok {
		return sh
	}
	sh, _ := d.s.db.GetShow(id)
	d.shows[id] = sh
	return sh
}

// enqueueSeason schedules a season fetch unless one was tried recently.
func (s *Service) enqueueSeason(showID, season int, path string) {
	if existing, err := s.db.GetSeason(showID, season); err == nil && existing != nil && existing.Status != "fetched" &&
		!due(existing.Status, existing.FetchedAt) {
		return
	}
	s.enqueue(job{showID: showID, season: season, notifyPath: path})
}

func splitGenres(genres string) []string {
	if genres == "" {
		return nil
	}
	return strings.Split(genres, ", ")
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
