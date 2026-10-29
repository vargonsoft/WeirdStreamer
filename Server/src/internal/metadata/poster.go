package metadata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// ErrNotFetched is returned when a title's details haven't been downloaded yet.
var ErrNotFetched = errors.New("this title's details haven't been downloaded yet; try again in a moment")

// ErrBadPoster is returned for a poster path that isn't a TMDB image path.
var ErrBadPoster = errors.New("not a TMDB image path")

// tmdbImagePath matches TMDB image paths like "/abc123.jpg", so only TMDB images are fetched.
var tmdbImagePath = regexp.MustCompile(`^/[A-Za-z0-9_-]+\.(jpg|jpeg|png|webp)$`)

// PosterOption is one poster offered in the fix-match dialog.
type PosterOption struct {
	Path     string `json:"path"`     // TMDB image path, what SetPoster takes
	URL      string `json:"url"`      // preview image URL
	Language string `json:"language"` // ISO 639-1 code, "" for posters without text
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// PosterList is a title's posters and which one it uses.
type PosterList struct {
	Current string         `json:"current"` // TMDB path of the poster in use
	Manual  bool           `json:"manual"`  // the poster was picked by hand
	Posters []PosterOption `json:"posters"`
}

// titleKind is TMDB's name for a library type ("movie" or "tv").
func titleKind(kind string) string {
	if kind == config.LibraryTypeTV {
		return "tv"
	}
	return "movie"
}

// Posters lists every poster TMDB has for a title (kind "movie" or "tv"): the configured
// language first, then posters without text, then the rest, best rated first within each.
func (s *Service) Posters(ctx context.Context, kind string, id int) (*PosterList, error) {
	client := s.tmdb()
	if client == nil {
		return nil, ErrNoToken
	}
	source, manual, _, err := s.titlePoster(kind, id)
	if err != nil {
		return nil, err
	}
	if source == "" {
		// Stored before poster sources were recorded: the poster is TMDB's default.
		source, _ = s.defaultPoster(ctx, client, kind, id)
	}
	images, err := client.Posters(ctx, titleKind(kind), id)
	if err != nil {
		return nil, err
	}
	lang := strings.SplitN(client.language, "-", 2)[0]
	rank := func(img PosterImage) int {
		switch {
		case img.Language != nil && *img.Language == lang:
			return 0
		case img.Language == nil:
			return 1
		}
		return 2
	}
	sort.SliceStable(images, func(i, j int) bool {
		if ri, rj := rank(images[i]), rank(images[j]); ri != rj {
			return ri < rj
		}
		return images[i].VoteAverage > images[j].VoteAverage
	})

	list := &PosterList{Current: source, Manual: manual, Posters: []PosterOption{}}
	for _, img := range images {
		opt := PosterOption{Path: img.FilePath, URL: client.imageBase + "w185" + img.FilePath,
			Width: img.Width, Height: img.Height}
		if img.Language != nil {
			opt.Language = *img.Language
		}
		list.Posters = append(list.Posters, opt)
	}
	return list, nil
}

// SetPoster makes a TMDB image (any title's) a title's poster; an empty source goes back to
// TMDB's default poster. notifyPath is told when the new poster is in place.
func (s *Service) SetPoster(ctx context.Context, kind string, id int, source, notifyPath string) error {
	client := s.tmdb()
	if client == nil {
		return ErrNoToken
	}
	if source != "" && !tmdbImagePath.MatchString(source) {
		return ErrBadPoster
	}
	_, _, oldPoster, err := s.titlePoster(kind, id)
	if err != nil {
		return err
	}
	manual := source != ""
	if !manual {
		if source, err = s.defaultPoster(ctx, client, kind, id); err != nil {
			return err
		}
	}

	dir := fmt.Sprintf("%s/%d", titleKind(kind), id)
	s.markActive(fmt.Sprintf("%s:%d", titleKind(kind), id), true)
	defer s.markActive(fmt.Sprintf("%s:%d", titleKind(kind), id), false)
	poster := ""
	if source != "" {
		if poster = s.download(ctx, client, "w500", source, dir+"/poster"); poster == "" {
			return fmt.Errorf("could not download poster %s", source)
		}
	}
	if oldPoster != "" && oldPoster != poster {
		// A poster of another image type (.png vs .jpg) leaves the old file behind.
		os.Remove(filepath.Join(artworkDir(), filepath.FromSlash(oldPoster)))
	}

	table := "movies"
	if kind == config.LibraryTypeTV {
		table = "shows"
	}
	if err := s.db.SetTitlePoster(table, id, poster, source, manual, time.Now()); err != nil {
		return err
	}
	if s.notify != nil && notifyPath != "" {
		s.notify(notifyPath)
	}
	return nil
}

// titlePoster returns a stored title's poster source, whether it was picked by hand, and its
// artwork-relative poster file.
func (s *Service) titlePoster(kind string, id int) (source string, manual bool, poster string, err error) {
	if kind == config.LibraryTypeTV {
		sh, err := s.db.GetShow(id)
		if err != nil || sh == nil {
			return "", false, "", orNotFetched(err)
		}
		return sh.PosterSource, sh.PosterManual, sh.Poster, nil
	}
	m, err := s.db.GetMovie(id)
	if err != nil || m == nil {
		return "", false, "", orNotFetched(err)
	}
	return m.PosterSource, m.PosterManual, m.Poster, nil
}

func orNotFetched(err error) error {
	if err != nil {
		return err
	}
	return ErrNotFetched
}

// defaultPoster asks TMDB for a title's own poster.
func (s *Service) defaultPoster(ctx context.Context, client *Client, kind string, id int) (string, error) {
	if kind == config.LibraryTypeTV {
		d, err := client.TV(ctx, id)
		if err != nil {
			return "", err
		}
		return d.PosterPath, nil
	}
	d, err := client.Movie(ctx, id)
	if err != nil {
		return "", err
	}
	return d.PosterPath, nil
}
