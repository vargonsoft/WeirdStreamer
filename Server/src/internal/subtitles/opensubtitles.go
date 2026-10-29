package subtitles

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	apiBase   = "https://api.opensubtitles.com/api/v1"
	userAgent = "WeirdStreamer v1"
)

// ErrNoKey is returned when no OpenSubtitles API key is configured.
var ErrNoKey = errors.New("no OpenSubtitles API key is configured")

// Client searches and downloads subtitles from OpenSubtitles.com. Downloads are limited per day
// (for the key, or per IP address), so callers keep what they download.
type Client struct {
	APIKey  string
	BaseURL string // the API's address (tests point it elsewhere)
	HTTP    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{APIKey: apiKey, BaseURL: apiBase, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Query is what to search for: a TMDB match (a movie's id, or an episode's show, season and
// episode) is the most exact; the file's hash finds subtitles made for that very release; the
// title and year are the fallback.
type Query struct {
	TMDBID       int // a movie
	ParentTMDBID int // an episode's show
	Season       int
	Episode      int
	Title        string
	Year         int
	MovieHash    string
	Languages    []string // two-letter codes; empty for all
}

// Result is one subtitle file found.
type Result struct {
	FileID          int     `json:"file_id"`
	Language        string  `json:"language"` // as OpenSubtitles gives it: en, pt-BR…
	Release         string  `json:"release"`
	Downloads       int     `json:"downloads"`
	Rating          float64 `json:"rating"`
	HearingImpaired bool    `json:"hearing_impaired"`
	Foreign         bool    `json:"foreign_parts_only"`
	HashMatch       bool    `json:"hash_match"` // made for this very file
	Trusted         bool    `json:"trusted"`
	MachineMade     bool    `json:"machine_translated"` // AI or machine translated
	FPS             float64 `json:"fps,omitempty"`
	Uploaded        string  `json:"uploaded,omitempty"`
}

// Search finds subtitles, the best first: made for this file, then trusted, then most downloaded.
func (c *Client) Search(ctx context.Context, q Query) ([]Result, error) {
	params := url.Values{}
	switch {
	case q.ParentTMDBID > 0:
		params.Set("parent_tmdb_id", strconv.Itoa(q.ParentTMDBID))
		params.Set("season_number", strconv.Itoa(q.Season))
		params.Set("episode_number", strconv.Itoa(q.Episode))
	case q.TMDBID > 0:
		params.Set("tmdb_id", strconv.Itoa(q.TMDBID))
	case q.Title != "":
		params.Set("query", strings.ToLower(q.Title))
		if q.Year > 0 {
			params.Set("year", strconv.Itoa(q.Year))
		}
		if q.Season > 0 || q.Episode > 0 {
			params.Set("season_number", strconv.Itoa(q.Season))
			params.Set("episode_number", strconv.Itoa(q.Episode))
		}
	}
	if q.MovieHash != "" {
		params.Set("moviehash", q.MovieHash)
	}
	if len(q.Languages) > 0 {
		langs := append([]string(nil), q.Languages...)
		sort.Strings(langs)
		params.Set("languages", strings.ToLower(strings.Join(langs, ",")))
	}
	// The API redirects requests whose parameters aren't in order (Encode sorts them by key).
	var body struct {
		Data []struct {
			Attributes struct {
				Language          string  `json:"language"`
				Release           string  `json:"release"`
				DownloadCount     int     `json:"download_count"`
				Ratings           float64 `json:"ratings"`
				HearingImpaired   bool    `json:"hearing_impaired"`
				ForeignPartsOnly  bool    `json:"foreign_parts_only"`
				MoviehashMatch    bool    `json:"moviehash_match"`
				FromTrusted       bool    `json:"from_trusted"`
				AITranslated      bool    `json:"ai_translated"`
				MachineTranslated bool    `json:"machine_translated"`
				FPS               float64 `json:"fps"`
				UploadDate        string  `json:"upload_date"`
				FeatureDetails    struct {
					FeatureType string `json:"feature_type"` // Movie, Episode, Tvshow
				} `json:"feature_details"`
				Files []struct {
					FileID   int    `json:"file_id"`
					FileName string `json:"file_name"`
				} `json:"files"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.call(ctx, http.MethodGet, "/subtitles?"+params.Encode(), nil, &body); err != nil {
		return nil, err
	}
	// TMDB numbers movies and episodes apart, so a movie's id finds the episode with that id too.
	want := ""
	switch {
	case q.ParentTMDBID > 0:
		want = "episode"
	case q.TMDBID > 0:
		want = "movie"
	}
	var results []Result
	for _, d := range body.Data {
		a := d.Attributes
		if len(a.Files) == 0 {
			continue
		}
		if kind := strings.ToLower(a.FeatureDetails.FeatureType); want != "" && kind != "" && kind != want {
			continue
		}
		release := a.Release
		if release == "" {
			release = a.Files[0].FileName
		}
		results = append(results, Result{FileID: a.Files[0].FileID, Language: a.Language, Release: release,
			Downloads: a.DownloadCount, Rating: a.Ratings, HearingImpaired: a.HearingImpaired,
			Foreign: a.ForeignPartsOnly, HashMatch: a.MoviehashMatch, Trusted: a.FromTrusted,
			MachineMade: a.AITranslated || a.MachineTranslated, FPS: a.FPS, Uploaded: a.UploadDate})
	}
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.HashMatch != b.HashMatch {
			return a.HashMatch
		}
		if a.MachineMade != b.MachineMade {
			return !a.MachineMade
		}
		if a.Trusted != b.Trusted {
			return a.Trusted
		}
		return a.Downloads > b.Downloads
	})
	return results, nil
}

// Quota is what's left of the day's downloads.
type Quota struct {
	Remaining int    `json:"remaining"`
	Reset     string `json:"reset,omitempty"` // when it resets, as OpenSubtitles words it
}

// Download fetches a subtitle file as SRT.
func (c *Client) Download(ctx context.Context, fileID int) ([]byte, Quota, error) {
	var link struct {
		Link      string `json:"link"`
		Remaining int    `json:"remaining"`
		ResetTime string `json:"reset_time"`
		Message   string `json:"message"`
	}
	req, _ := json.Marshal(map[string]any{"file_id": fileID, "sub_format": "srt"})
	err := c.call(ctx, http.MethodPost, "/download", req, &link)
	quota := Quota{Remaining: link.Remaining, Reset: link.ResetTime}
	if err != nil {
		return nil, quota, err
	}
	if link.Link == "" {
		return nil, quota, fmt.Errorf("OpenSubtitles gave no download link: %s", link.Message)
	}
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, link.Link, nil)
	r.Header.Set("User-Agent", userAgent)
	res, err := c.HTTP.Do(r)
	if err != nil {
		return nil, quota, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, quota, fmt.Errorf("downloading the subtitle: %s", res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	return data, quota, err
}

// call makes an API request and decodes its JSON answer into out; an error answer's message
// becomes the error (the daily download limit, for one).
func (c *Client) call(ctx context.Context, method, path string, body []byte, out any) error {
	if c.APIKey == "" {
		return ErrNoKey
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.APIKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode != http.StatusOK {
		var e struct {
			Message string   `json:"message"`
			Errors  []string `json:"errors"`
		}
		json.Unmarshal(data, &e)
		msg := e.Message
		if msg == "" && len(e.Errors) > 0 {
			msg = strings.Join(e.Errors, "; ")
		}
		if msg == "" {
			msg = res.Status
		}
		// Download answers carry the quota even when refused.
		json.Unmarshal(data, out)
		return fmt.Errorf("OpenSubtitles: %s", msg)
	}
	return json.Unmarshal(data, out)
}

// MovieHash is OpenSubtitles' hash of a video file: its size plus the 64-bit little-endian words
// of its first and last 64 KiB. "" for files too small to hash.
func MovieHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	const chunk = 64 * 1024
	if err != nil || info.Size() < chunk {
		return ""
	}
	hash := uint64(info.Size())
	buf := make([]byte, chunk)
	for _, offset := range []int64{0, info.Size() - chunk} {
		if _, err := f.ReadAt(buf, offset); err != nil {
			return ""
		}
		for i := 0; i < chunk; i += 8 {
			hash += binary.LittleEndian.Uint64(buf[i:])
		}
	}
	return fmt.Sprintf("%016x", hash)
}
