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

	// An Apple Live Photo's video: the .MOV of the same name beside this photo, which folder
	// listings then leave out (media.PairLivePhotos)
	LiveVideo string `json:"live_video,omitempty"`

	Metadata *ItemMetadata `json:"metadata,omitempty"` // TMDB info, for files in movie/tv libraries
	Playback *Playback     `json:"playback,omitempty"` // the signed-in user's progress, for videos
}

// Playback is where a user is in a video. Times are in seconds; UpdatedAt is Unix milliseconds.
type Playback struct {
	Position  float64 `json:"position"` // where to resume; 0 when watched to the end
	Duration  float64 `json:"duration,omitempty"`
	Watched   bool    `json:"watched"`
	UpdatedAt int64   `json:"updated_at"`
	// On Continue Watching or taken off it (database.ContinueListed, ContinueRemoved; 0 neither)
	ContinueState int `json:"-"`
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
	Title  int              `json:"title,omitempty"` // a disc image's frames: the title they're for
}

type Directory struct {
	Path     string        `json:"path"`
	Name     string        `json:"name"`
	Items    []MediaItem   `json:"items"`
	Dirs     []Directory   `json:"dirs"`
	Metadata *ItemMetadata `json:"metadata,omitempty"` // movie, show or season this folder stands for
	// The folder stands for one title, so its match can be fixed by hand (collections can't)
	Fixable bool `json:"fixable,omitempty"`
	// The library's type ("movie", "tv") on browse responses, so clients know metadata applies
	LibraryType string `json:"library_type,omitempty"`
	// A picture to show inside the folder's icon: its thumbnail.png, else a photo's or video's
	// thumbnail from inside it, else its poster. A library shows only its own thumbnail.png.
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Web: the web UI's sign-in, whose token goes only into the HttpOnly session cookie
	Web bool `json:"web,omitempty"`
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
	// RetryAfter is the seconds to wait before trying again (set with 429 Too Many Requests).
	RetryAfter int `json:"retry_after,omitempty"`
}

// LibraryListing is a movie or TV library as a list of titles (Library view).
type LibraryListing struct {
	LibraryID   string         `json:"library_id"`
	LibraryName string         `json:"library_name"`
	LibraryType string         `json:"library_type"`
	Titles      []LibraryTitle `json:"titles"`
	Unmatched   []LibraryFile  `json:"unmatched"` // files (or, for TV, shows) without a match
	// Movie libraries: the TMDB collections (film series) its movies belong to, by name
	Collections []LibraryCollection `json:"collections,omitempty"`
}

// LibraryCollection is a collection (a film series) in a movie library's listing.
type LibraryCollection struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Poster string `json:"poster,omitempty"`
	Count  int    `json:"count"` // its movies in the library
	Total  int    `json:"total"` // its films on TMDB
}

// CollectionDetail is a collection's page: every film TMDB lists in it, by release date, those the
// library has as its titles (with progress), the rest marked missing.
type CollectionDetail struct {
	ID       int            `json:"id"`
	Name     string         `json:"name"`
	Overview string         `json:"overview,omitempty"`
	Poster   string         `json:"poster,omitempty"`
	Backdrop string         `json:"backdrop,omitempty"`
	Parts    []LibraryTitle `json:"parts"`
}

// LibraryTitle is one movie or show in a library listing.
type LibraryTitle struct {
	Kind      string `json:"kind"` // "movie" or "show"
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Poster    string `json:"poster,omitempty"`
	FileCount int    `json:"file_count"`
	// The signed-in user's progress: files watched, and for a movie, how far into it they are
	Watched  int      `json:"watched,omitempty"`
	Progress float64  `json:"progress,omitempty"` // 0..1
	// Movies: the TMDB collection it belongs to (0 for none)
	CollectionID int `json:"collection_id,omitempty"`
	// On a collection's page: a film of it the library doesn't have (not playable)
	Missing bool     `json:"missing,omitempty"`
	Paths   []string `json:"-"`
}

// LibraryFile is a playable file on a title page or in the Unmatched section.
type LibraryFile struct {
	ID           int64   `json:"id,omitempty"` // media item ID (trickplay), once the file is known
	Path         string  `json:"path"`
	Name         string  `json:"name"`
	Size         int64   `json:"size,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
	ThumbnailURL string  `json:"thumbnail_url,omitempty"`
	Count        int     `json:"count,omitempty"` // unmatched TV shows: how many files
	// Match is the movie a TV library file that isn't an episode was matched to by hand.
	Match    *ItemMetadata `json:"match,omitempty"`
	Playback *Playback     `json:"playback,omitempty"`
}

// TitleDetail is a title page: the full metadata plus the library's files for it.
type TitleDetail struct {
	MediaDetails
	Files      []LibraryFile   `json:"files,omitempty"`       // movies
	Seasons    []LibrarySeason `json:"seasons,omitempty"`     // shows: seasons with episodes on disk
	OtherFiles []LibraryFile   `json:"other_files,omitempty"` // shows: files that aren't episodes
	AutoSkip   bool            `json:"auto_skip,omitempty"`   // shows: the user has intros and credits skipped by themselves
}

type LibrarySeason struct {
	Season   int              `json:"season"`
	Name     string           `json:"name"`
	Poster   string           `json:"poster,omitempty"`
	Episodes []LibraryEpisode `json:"episodes"`
}

type LibraryEpisode struct {
	Season   int           `json:"season"`
	Episode  int           `json:"episode"`
	Title    string        `json:"title"`
	Overview string        `json:"overview,omitempty"`
	AirDate  string        `json:"air_date,omitempty"`
	Runtime  int           `json:"runtime,omitempty"`
	Still    string        `json:"still,omitempty"`
	Files    []LibraryFile `json:"files"`
}
