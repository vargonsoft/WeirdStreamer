package models

import (
	"time"
)

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	Group        string    `json:"group"` // The single group the user belongs to
	CreatedAt    time.Time `json:"created_at"`
}

type MediaItem struct {
	ID           int64     `json:"id"`
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Type         string    `json:"type"` // video, image, document
	Size         int64     `json:"size"`
	MimeType     string    `json:"mime_type"`
	ThumbnailURL string    `json:"thumbnail_url"`
	Duration     float64   `json:"duration"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Metadata *ItemMetadata `json:"metadata,omitempty"` // TMDB info, for files in movie/tv libraries
}

// ItemMetadata is the TMDB summary attached to browse results. Image fields are server URLs.
type ItemMetadata struct {
	Kind      string `json:"kind"` // "movie", "show", "season" or "episode"
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Overview  string `json:"overview,omitempty"`
	Runtime   int    `json:"runtime,omitempty"` // minutes
	AirDate   string `json:"air_date,omitempty"`
	Poster    string `json:"poster,omitempty"`
	Backdrop  string `json:"backdrop,omitempty"`
	Logo      string `json:"logo,omitempty"`
	Still     string `json:"still,omitempty"`
	ShowTitle string `json:"show_title,omitempty"`
	Season    *int   `json:"season,omitempty"`
	Episode   *int   `json:"episode,omitempty"`
}

// MediaDetails is the full record returned by /api/media/details.
type MediaDetails struct {
	ItemMetadata
	Tagline     string       `json:"tagline,omitempty"`
	Genres      []string     `json:"genres,omitempty"`
	ReleaseDate string       `json:"release_date,omitempty"`
	IMDBID      string       `json:"imdb_id,omitempty"`
	Directors   []CreditInfo `json:"directors,omitempty"`
	Writers     []CreditInfo `json:"writers,omitempty"`
	Creators    []CreditInfo `json:"creators,omitempty"`
	Cast        []CreditInfo `json:"cast,omitempty"`
	GuestStars  []CreditInfo `json:"guest_stars,omitempty"`
}

type CreditInfo struct {
	TMDBID    int    `json:"tmdb_id"`
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Profile   string `json:"profile,omitempty"` // headshot URL, when downloaded
}

type TrickplayFrame struct {
	Time int    `json:"time"`
	URL  string `json:"url"`
}

type TrickplayInfo struct {
	Frames []TrickplayFrame `json:"frames"`
}

type Directory struct {
	Path     string        `json:"path"`
	Name     string        `json:"name"`
	Items    []MediaItem   `json:"items"`
	Dirs     []Directory   `json:"dirs"`
	Metadata *ItemMetadata `json:"metadata,omitempty"` // movie, show or season this folder stands for
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      User   `json:"user"`
}

type UserUpdateRequest struct {
	IsAdmin *bool   `json:"is_admin,omitempty"`
	Group   *string `json:"group,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}
