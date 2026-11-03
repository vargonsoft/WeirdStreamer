package config

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Config struct {
	// The access groups: each user is in one, and each library lists those that may see it.
	// Users and libraries may only name groups listed here.
	Groups    []string        `json:"groups" yaml:"groups"`
	Server    ServerConfig    `json:"server" yaml:"server"`
	Database  DatabaseConfig  `json:"database" yaml:"database"`
	Media     MediaConfig     `json:"media" yaml:"media"`
	Auth      AuthConfig      `json:"auth" yaml:"auth"`
	Streaming StreamingConfig `json:"streaming" yaml:"streaming"`
	Metadata  MetadataConfig  `json:"metadata" yaml:"metadata"`
	Jobs      JobsConfig      `json:"jobs" yaml:"jobs"`
}

// JobsConfig tunes the trickplay job, which the Jobs page changes live.
type JobsConfig struct {
	TrickplayRunners         int `json:"trickplayRunners" yaml:"trickplayRunners"`                 // videos made at once: 1 to 8 (0: 3)
	TrickplayFramesPerRunner int `json:"trickplayFramesPerRunner" yaml:"trickplayFramesPerRunner"` // frames per ffmpeg process: 1 to 50 (0: 25)
}

const (
	DefaultTrickplayRunners         = 3
	DefaultTrickplayFramesPerRunner = 25
	MaxTrickplayRunners             = 8  // the pool of runners the job starts with
	MaxTrickplayFrames              = 50 // one process's frames: more ran out of memory
	MaxTrickplayProduct             = 100
)

// The trickplay settings, read by its runners at every step while the Jobs page can change them:
// kept apart from GlobalConfig so the reads are safe. Set when the config loads (0: the default).
var trickplayRunners, trickplayFrames atomic.Int64

// SetTrickplaySettings sets the trickplay settings the job reads (Load does, and tests).
func SetTrickplaySettings(runners, framesPerRunner int) {
	trickplayRunners.Store(int64(runners))
	trickplayFrames.Store(int64(framesPerRunner))
}

// TrickplayRunners is how many videos the trickplay job makes frames for at once (live).
func TrickplayRunners() int {
	if n := int(trickplayRunners.Load()); n != 0 {
		return min(max(n, 1), MaxTrickplayRunners)
	}
	return DefaultTrickplayRunners
}

// TrickplayFramesPerRunner is how many frames one process makes for a runner (live).
func TrickplayFramesPerRunner() int {
	if n := int(trickplayFrames.Load()); n != 0 {
		return min(max(n, 1), MaxTrickplayFrames)
	}
	return DefaultTrickplayFramesPerRunner
}

// UpdateJobs saves new trickplay settings to the config file and reloads them.
func UpdateJobs(runners, frames int) error {
	c := *GlobalConfig
	c.Jobs = JobsConfig{TrickplayRunners: runners, TrickplayFramesPerRunner: frames}
	if err := c.Validate(); err != nil {
		return err
	}
	if err := Save(Path(), &c); err != nil {
		return fmt.Errorf("failed to save configuration")
	}
	return Load(Path())
}

// MetadataConfig configures lookups for "movie" and "tv" libraries.
type MetadataConfig struct {
	TMDBAccessToken string `json:"tmdbAccessToken" yaml:"tmdbAccessToken"` // TMDB API read access token (Bearer)
	Language        string `json:"language" yaml:"language"`               // e.g. "en-US"; empty means en-US
	// OpenSubtitles.com API key, for searching and downloading subtitles (empty: no online search)
	OpenSubtitlesAPIKey string `json:"openSubtitlesApiKey,omitempty" yaml:"openSubtitlesApiKey"`
}

type ServerConfig struct {
	// The server's name, which apps show for it (empty: they show its address)
	Name string `json:"name" yaml:"name"`
	Port int    `json:"port" yaml:"port"`
	Host string `json:"host" yaml:"host"`
	// TLS certificate and private key for HTTPS (with streaming.enableHTTP2). Relative paths are
	// from the working directory. Empty: cert.pem and key.pem.
	CertFile string `json:"certFile,omitempty" yaml:"certFile"`
	KeyFile  string `json:"keyFile,omitempty" yaml:"keyFile"`
	// Where live.sh writes the server's log files, one per start. Relative paths are from Server/.
	LogsPath string `json:"logsPath,omitempty" yaml:"logsPath"`
	// Reverse proxies (addresses or networks, such as an nginx in front) whose X-Forwarded-For or
	// X-Real-IP name the client: the logs, sign-in limits and the Streams page use that address.
	TrustedProxies []string `json:"trustedProxies,omitempty" yaml:"trustedProxies"`
}

const (
	defaultCertFile = "cert.pem"
	defaultKeyFile  = "key.pem"
)

// TLSFiles returns the certificate and key files to serve HTTPS with.
func (s ServerConfig) TLSFiles() (certFile, keyFile string) {
	certFile, keyFile = s.CertFile, s.KeyFile
	if certFile == "" {
		certFile = defaultCertFile
	}
	if keyFile == "" {
		keyFile = defaultKeyFile
	}
	return certFile, keyFile
}

type DatabaseConfig struct {
	UserPath  string `json:"userPath" yaml:"userPath"`
	MediaPath string `json:"mediaPath" yaml:"mediaPath"`
}

type MediaPath struct {
	Name   string   `json:"name" yaml:"name"`
	Path   string   `json:"path" yaml:"path"`
	ID     string   `json:"id,omitempty" yaml:"id,omitempty"`
	Groups []string `json:"groups" yaml:"groups"`                 // Groups that have access to this library
	Type   string   `json:"type,omitempty" yaml:"type,omitempty"` // Optional: LibraryTypeMovie or LibraryTypeTV
	// Where this library's videos in progress show on Continue Watching: ContinueOn (the default,
	// also when empty), ContinueLibraryOnly or ContinueOff. Their progress is kept and shown
	// either way.
	ContinueWatchingMode string `json:"continueWatchingMode,omitempty" yaml:"continueWatchingMode,omitempty"`
	// Where this library's last videos to arrive show on Recently Added: ContinueOn (the home
	// screen's row, which blends the libraries, and the library's own), ContinueLibraryOnly, or
	// ContinueOff (the default, also when empty).
	ShowNewAdditions string `json:"showNewAdditions,omitempty" yaml:"showNewAdditions,omitempty"`
}

// Continue Watching modes (MediaPath.ContinueWatchingMode)
const (
	ContinueOn          = "on"          // on the home screen's Continue Watching and the library's own
	ContinueLibraryOnly = "libraryOnly" // only on the library's own page, not the home screen's
	ContinueOff         = "off"         // nowhere
)

// OnContinueWatching reports whether the library's videos in progress belong on a Continue
// Watching list: the library's own page's (libraryPage) or the home screen's, which blends all
// libraries.
func (p MediaPath) OnContinueWatching(libraryPage bool) bool {
	switch p.ContinueWatchingMode {
	case ContinueOff:
		return false
	case ContinueLibraryOnly:
		return libraryPage
	}
	return true
}

// OnNewAdditions reports whether the library's new videos belong on a Recently Added row: the
// library's own page's (libraryPage) or the home screen's, which blends libraries.
func (p MediaPath) OnNewAdditions(libraryPage bool) bool {
	switch p.ShowNewAdditions {
	case ContinueOn:
		return true
	case ContinueLibraryOnly:
		return libraryPage
	}
	return false
}

// Library types. A library has at most one type; untyped libraries are plain folders.
const (
	LibraryTypeMovie = "movie"
	LibraryTypeTV    = "tv"
)

// HasGroup reports whether name is one of the defined groups.
func (c *Config) HasGroup(name string) bool {
	for _, g := range c.Groups {
		if g == name {
			return true
		}
	}
	return false
}

// CheckGroupName reports why name can't be a new group's name, or nil.
func (c *Config) CheckGroupName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("a group needs a name")
	}
	if name != strings.TrimSpace(name) || strings.Contains(name, ",") {
		return fmt.Errorf("group %q: no commas, and no spaces at either end", name)
	}
	for _, g := range c.Groups {
		if strings.EqualFold(g, name) {
			return fmt.Errorf("the group %q already exists", g)
		}
	}
	return nil
}

// Validate checks values that can't be expressed by the file's structure alone.
func (c *Config) Validate() error {
	c.Server.Name = strings.TrimSpace(c.Server.Name)
	if c.Jobs.TrickplayRunners == 0 {
		c.Jobs.TrickplayRunners = DefaultTrickplayRunners
	}
	if c.Jobs.TrickplayFramesPerRunner == 0 {
		c.Jobs.TrickplayFramesPerRunner = DefaultTrickplayFramesPerRunner
	}
	if r, f := c.Jobs.TrickplayRunners, c.Jobs.TrickplayFramesPerRunner; r < 1 || r > MaxTrickplayRunners || f < 1 || f > MaxTrickplayFrames {
		return fmt.Errorf("trickplay runners must be 1 to %d, and frames per runner 1 to %d", MaxTrickplayRunners, MaxTrickplayFrames)
	} else if r*f > MaxTrickplayProduct {
		return fmt.Errorf("runners times frames per runner can't exceed %d (about %d GB of memory)", MaxTrickplayProduct, MaxTrickplayProduct*110/1000)
	}
	// Groups: each a valid name, none twice (in any case)
	groups := c.Groups
	c.Groups = nil
	for _, g := range groups {
		if err := c.CheckGroupName(g); err != nil {
			c.Groups = groups
			return err
		}
		c.Groups = append(c.Groups, g)
	}
	if c.Groups == nil {
		c.Groups = []string{}
	}
	for i := range c.Media.Paths {
		p := &c.Media.Paths[i]
		for _, g := range p.Groups {
			if !c.HasGroup(g) {
				return fmt.Errorf("media path %q: the group %q isn't defined (groups: %s)", p.Name, g, strings.Join(c.Groups, ", "))
			}
		}
		switch p.Type {
		case "", LibraryTypeMovie, LibraryTypeTV:
		default:
			return fmt.Errorf("media path %q: invalid type %q (allowed: %q, %q, or none)", p.Name, p.Type, LibraryTypeMovie, LibraryTypeTV)
		}
		var err error
		if p.ContinueWatchingMode, err = canonicalMode(p.ContinueWatchingMode); err != nil {
			return fmt.Errorf("media path %q: invalid continueWatchingMode %w", p.Name, err)
		}
		if p.ShowNewAdditions, err = canonicalMode(p.ShowNewAdditions); err != nil {
			return fmt.Errorf("media path %q: invalid showNewAdditions %w", p.Name, err)
		}
	}
	return nil
}

// canonicalMode reads an on/libraryOnly/off option in any capitalization, written back in the
// canonical one. Empty stays empty (the option's default), so saving doesn't add it to the file.
func canonicalMode(mode string) (string, error) {
	switch {
	case mode == "":
		return "", nil
	case strings.EqualFold(mode, ContinueOn):
		return ContinueOn, nil
	case strings.EqualFold(mode, ContinueLibraryOnly):
		return ContinueLibraryOnly, nil
	case strings.EqualFold(mode, ContinueOff):
		return ContinueOff, nil
	}
	return "", fmt.Errorf("%q (allowed: %q, %q, %q)", mode, ContinueOn, ContinueLibraryOnly, ContinueOff)
}

type MediaConfig struct {
	Paths                 []MediaPath `json:"paths" yaml:"paths"`
	ThumbnailPath         string      `json:"thumbnailPath" yaml:"thumbnailPath"`
	TrickplayPath         string      `json:"trickplayPath,omitempty" yaml:"trickplayPath"`
	ArtworkPath           string      `json:"artworkPath,omitempty" yaml:"artworkPath"`               // Downloaded TMDB posters, backdrops, etc.
	DownloadPath          string      `json:"downloadPath,omitempty" yaml:"downloadPath"`             // Temporary encodes for offline downloads
	TranscodeCachePath    string      `json:"transcodeCachePath,omitempty" yaml:"transcodeCachePath"` // Transcodes while they play, so a client can resume
	SubtitlePath          string      `json:"subtitlePath,omitempty" yaml:"subtitlePath"`             // Subtitles downloaded from OpenSubtitles
	SupportedVideoFormats []string    `json:"supportedVideoFormats" yaml:"supportedVideoFormats"`
	SupportedImageFormats []string    `json:"supportedImageFormats" yaml:"supportedImageFormats"`
	SupportedDocFormats   []string    `json:"supportedDocFormats" yaml:"supportedDocFormats"`
	IgnoreMacFiles        bool        `json:"ignoreMacFiles" yaml:"ignoreMacFiles"`
}

type AuthConfig struct {
	JWTSecret        string `json:"jwtSecret" yaml:"jwtSecret"`
	TokenExpiryHours int    `json:"tokenExpiryHours" yaml:"tokenExpiryHours"`
}

type StreamingConfig struct {
	ChunkSize         int  `json:"chunkSize" yaml:"chunkSize"`
	EnableHTTP2       bool `json:"enableHTTP2" yaml:"enableHTTP2"`
	AutoConvertImages bool `json:"autoConvertImages" yaml:"autoConvertImages"`
	// Background jobs (intros and credits, trickplay, cleanup) wait while anyone is streaming,
	// whatever the machine's load: a failsafe
	PauseJobsWhileStreaming bool `json:"pauseJobsWhileStreaming,omitempty" yaml:"pauseJobsWhileStreaming"`
}

var GlobalConfig *Config

// loadedPath is the file GlobalConfig was last loaded from, so saves go back to it.
var loadedPath string

// Path returns the file the current configuration was loaded from.
func Path() string {
	return loadedPath
}
