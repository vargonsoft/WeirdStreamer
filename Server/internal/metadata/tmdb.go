package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	tmdbAPIBase   = "https://api.themoviedb.org/3"
	tmdbImageBase = "https://image.tmdb.org/t/p/"
)

// ErrNotFound is returned when TMDB has no record for a requested ID.
var ErrNotFound = errors.New("tmdb: not found")

// Client is a minimal TMDB v3 client. It is safe for concurrent use and paces its own requests.
type Client struct {
	token     string
	language  string
	apiBase   string
	imageBase string
	http      *http.Client
	apiPace   *time.Ticker // TMDB allows ~40 req/s; stay well under it
	imagePace *time.Ticker
}

func NewClient(token, language string) *Client {
	if language == "" {
		language = "en-US"
	}
	return &Client{
		token:     token,
		language:  language,
		apiBase:   tmdbAPIBase,
		imageBase: tmdbImageBase,
		http:      &http.Client{Timeout: 30 * time.Second},
		apiPace:   time.NewTicker(50 * time.Millisecond),
		imagePace: time.NewTicker(50 * time.Millisecond),
	}
}

// imageLanguages is TMDB's include_image_language value: the configured language plus
// language-neutral images.
func (c *Client) imageLanguages() string {
	return strings.SplitN(c.language, "-", 2)[0] + ",null"
}

type SearchResult struct {
	ID            int      `json:"id"`
	Title         string   `json:"title"`          // movies
	OriginalTitle string   `json:"original_title"` // movies
	ReleaseDate   string   `json:"release_date"`   // movies
	Name          string   `json:"name"`           // tv
	OriginalName  string   `json:"original_name"`  // tv
	FirstAirDate  string   `json:"first_air_date"` // tv
	OriginCountry []string `json:"origin_country"`
	Popularity    float64  `json:"popularity"`
}

type Genre struct {
	Name string `json:"name"`
}

type Person struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	Job         string `json:"job"`
}

type Image struct {
	FilePath    string  `json:"file_path"`
	Language    *string `json:"iso_639_1"`
	VoteAverage float64 `json:"vote_average"`
}

type Images struct {
	Logos []Image `json:"logos"`
}

type MovieDetails struct {
	ID            int     `json:"id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Overview      string  `json:"overview"`
	Tagline       string  `json:"tagline"`
	ReleaseDate   string  `json:"release_date"`
	Runtime       int     `json:"runtime"`
	Genres        []Genre `json:"genres"`
	IMDBID        string  `json:"imdb_id"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	Credits       struct {
		Cast []Person `json:"cast"`
		Crew []Person `json:"crew"`
	} `json:"credits"`
	Images Images `json:"images"`
}

type AggregateCast struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
	Order       int    `json:"order"`
	Roles       []struct {
		Character string `json:"character"`
	} `json:"roles"`
}

type TVDetails struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	OriginalName   string   `json:"original_name"`
	Overview       string   `json:"overview"`
	Tagline        string   `json:"tagline"`
	FirstAirDate   string   `json:"first_air_date"`
	Genres         []Genre  `json:"genres"`
	EpisodeRunTime []int    `json:"episode_run_time"`
	PosterPath     string   `json:"poster_path"`
	BackdropPath   string   `json:"backdrop_path"`
	CreatedBy      []Person `json:"created_by"`
	ExternalIDs    struct {
		IMDBID string `json:"imdb_id"`
	} `json:"external_ids"`
	AggregateCredits struct {
		Cast []AggregateCast `json:"cast"`
	} `json:"aggregate_credits"`
	Images Images `json:"images"`
}

type Episode struct {
	ID            int      `json:"id"`
	SeasonNumber  int      `json:"season_number"`
	EpisodeNumber int      `json:"episode_number"`
	Name          string   `json:"name"`
	Overview      string   `json:"overview"`
	AirDate       string   `json:"air_date"`
	Runtime       int      `json:"runtime"`
	StillPath     string   `json:"still_path"`
	Crew          []Person `json:"crew"`
	GuestStars    []Person `json:"guest_stars"`
}

type Season struct {
	SeasonNumber int       `json:"season_number"`
	Name         string    `json:"name"`
	Overview     string    `json:"overview"`
	PosterPath   string    `json:"poster_path"`
	AirDate      string    `json:"air_date"`
	Episodes     []Episode `json:"episodes"`
}

func (c *Client) SearchMovie(ctx context.Context, title string, year int) ([]SearchResult, error) {
	params := url.Values{"query": {title}, "include_adult": {"false"}}
	if year > 0 {
		params.Set("year", strconv.Itoa(year))
	}
	var resp struct {
		Results []SearchResult `json:"results"`
	}
	err := c.get(ctx, "/search/movie", params, &resp)
	return resp.Results, err
}

func (c *Client) SearchTV(ctx context.Context, title string, year int) ([]SearchResult, error) {
	params := url.Values{"query": {title}, "include_adult": {"false"}}
	if year > 0 {
		params.Set("first_air_date_year", strconv.Itoa(year))
	}
	var resp struct {
		Results []SearchResult `json:"results"`
	}
	err := c.get(ctx, "/search/tv", params, &resp)
	return resp.Results, err
}

func (c *Client) Movie(ctx context.Context, id int) (*MovieDetails, error) {
	var m MovieDetails
	err := c.get(ctx, fmt.Sprintf("/movie/%d", id), url.Values{
		"append_to_response":     {"credits,images"},
		"include_image_language": {c.imageLanguages()},
	}, &m)
	return &m, err
}

func (c *Client) TV(ctx context.Context, id int) (*TVDetails, error) {
	var t TVDetails
	err := c.get(ctx, fmt.Sprintf("/tv/%d", id), url.Values{
		"append_to_response":     {"aggregate_credits,images,external_ids"},
		"include_image_language": {c.imageLanguages()},
	}, &t)
	return &t, err
}

func (c *Client) Season(ctx context.Context, showID, season int) (*Season, error) {
	var s Season
	err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", showID, season), nil, &s)
	return &s, err
}

// DownloadImage saves a TMDB image (imagePath like "/abc.jpg") at size ("w500", "original")
// to dest, writing to a temp file first so readers never see a partial image.
func (c *Client) DownloadImage(ctx context.Context, size, imagePath, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	resp, err := c.do(ctx, c.imagePace, c.imageBase+size+imagePath, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("language", c.language)
	resp, err := c.do(ctx, c.apiPace, c.apiBase+path+"?"+params.Encode(), true)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// do performs a GET with pacing and retries for rate limiting (429) and server errors.
func (c *Client) do(ctx context.Context, pace *time.Ticker, u string, auth bool) (*http.Response, error) {
	backoff := time.Second
	for attempt := 1; ; attempt++ {
		select {
		case <-pace.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		if auth {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		retryAfter := backoff
		switch {
		case err != nil:
			// network error: retry below
		case resp.StatusCode == http.StatusOK:
			return resp, nil
		case resp.StatusCode == http.StatusNotFound:
			resp.Body.Close()
			return nil, ErrNotFound
		case resp.StatusCode == http.StatusTooManyRequests:
			if secs, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && secs > 0 {
				retryAfter = time.Duration(secs) * time.Second
			}
			resp.Body.Close()
			err = fmt.Errorf("tmdb: rate limited")
		case resp.StatusCode >= 500:
			resp.Body.Close()
			err = fmt.Errorf("tmdb: server error %d", resp.StatusCode)
		default:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return nil, fmt.Errorf("tmdb: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		if attempt == 4 {
			return nil, err
		}
		select {
		case <-time.After(retryAfter):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		backoff *= 2
	}
}
