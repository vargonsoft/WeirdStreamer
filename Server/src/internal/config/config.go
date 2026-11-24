package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

type Config struct {
	// The access groups: each user is in one, and each library lists those that may see it.
	// Users and libraries may only name groups listed here. The first is the default for new users.
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
	// Backups of both databases and config.yml (internal/backup): the folder (default: BackupDir), when (off, daily or weekly; default daily), at what time
	// ("04:00"), on what day for weekly ones (default sunday), and how many are kept (default 7).
	BackupPath     string `json:"backupPath,omitempty" yaml:"backupPath"`
	BackupSchedule string `json:"backupSchedule,omitempty" yaml:"backupSchedule"`
	BackupTime     string `json:"backupTime,omitempty" yaml:"backupTime"`
	BackupDay      string `json:"backupDay,omitempty" yaml:"backupDay"`
	BackupKeep     int    `json:"backupKeep,omitempty" yaml:"backupKeep"`
}

// Backup defaults and limits.
const (
	DefaultBackupSchedule = "daily"
	DefaultBackupTime     = "04:00"
	DefaultBackupDay      = "sunday"
	DefaultBackupKeep     = 7
	MaxBackupKeep         = 365
)

// BackupSchedules are database.backupSchedule's values; BackupDays its days.
var (
	BackupSchedules = []string{"off", "daily", "weekly"}
	BackupDays      = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
)

// BackupDir is where backups go: backupPath, else "backups" beside the users database's folder
// when that's a data folder (./var/data/users.db: ./var/backups), else inside it.
func (d DatabaseConfig) BackupDir() string {
	if d.BackupPath != "" {
		return d.BackupPath
	}
	dir := filepath.Dir(d.UserPath)
	if filepath.Base(dir) == "data" {
		return filepath.Join(filepath.Dir(dir), "backups")
	}
	return filepath.Join(dir, "backups")
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

// DefaultGroup is the group new users start in: the first one listed ("" with none).
func (c *Config) DefaultGroup() string {
	if len(c.Groups) == 0 {
		return ""
	}
	return c.Groups[0]
}

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
	d := &c.Database
	d.BackupSchedule = strings.ToLower(strings.TrimSpace(d.BackupSchedule))
	if d.BackupSchedule == "" {
		d.BackupSchedule = DefaultBackupSchedule
	}
	if !slices.Contains(BackupSchedules, d.BackupSchedule) {
		return fmt.Errorf("database.backupSchedule must be one of %s", strings.Join(BackupSchedules, ", "))
	}
	if d.BackupTime = strings.TrimSpace(d.BackupTime); d.BackupTime == "" {
		d.BackupTime = DefaultBackupTime
	}
	if _, err := time.Parse("15:04", d.BackupTime); err != nil {
		return fmt.Errorf("database.backupTime must be a time of day, as 04:00")
	}
	if d.BackupDay = strings.ToLower(strings.TrimSpace(d.BackupDay)); d.BackupDay == "" {
		d.BackupDay = DefaultBackupDay
	}
	if !slices.Contains(BackupDays, d.BackupDay) {
		return fmt.Errorf("database.backupDay must be a day of the week, as sunday")
	}
	if d.BackupKeep == 0 {
		d.BackupKeep = DefaultBackupKeep
	}
	if d.BackupKeep < 1 || d.BackupKeep > MaxBackupKeep {
		return fmt.Errorf("database.backupKeep must be 1 to %d", MaxBackupKeep)
	}
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
	if e := c.Streaming.HardwareEncoder; e != "" && !slices.Contains(HardwareEncoders, e) {
		return fmt.Errorf("streaming.hardwareEncoder must be empty or one of %s", strings.Join(HardwareEncoders, ", "))
	}
	if c.Streaming.HardwareMaxTranscodes < 0 {
		return fmt.Errorf("streaming.hardwareMaxTranscodes can't be negative")
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
	TempPath              string      `json:"tempPath,omitempty" yaml:"tempPath"`                     // Short-lived work files (HEIC conversions)
	RepairPath            string      `json:"repairPath,omitempty" yaml:"repairPath"`                 // Fixed copies of damaged videos (a broken seek index), played in their place
	TrashPath             string      `json:"trashPath,omitempty" yaml:"trashPath"`                   // Library files replaced by fixed copies, kept 30 days
	CJKFontPath           string      `json:"cjkFontPath,omitempty" yaml:"cjkFontPath"`               // A CJK font for styled subtitles' Chinese, Japanese and Korean (default: Noto Sans CJK, where it's installed)
	SubsetCommand         string      `json:"subsetCommand,omitempty" yaml:"subsetCommand"`           // fontTools' pyftsubset, which makes that font's subset for each video (default "pyftsubset")
	SupportedVideoFormats []string    `json:"supportedVideoFormats" yaml:"supportedVideoFormats"`
	SupportedImageFormats []string    `json:"supportedImageFormats" yaml:"supportedImageFormats"`
	SupportedDocFormats   []string    `json:"supportedDocFormats" yaml:"supportedDocFormats"`
	IgnoreMacFiles        bool        `json:"ignoreMacFiles" yaml:"ignoreMacFiles"`
}

type AuthConfig struct {
	JWTSecret        string `json:"jwtSecret,omitempty" yaml:"jwtSecret"`
	TokenExpiryHours int    `json:"tokenExpiryHours" yaml:"tokenExpiryHours"`
}

type StreamingConfig struct {
	EnableHTTP2       bool `json:"enableHTTP2" yaml:"enableHTTP2"`
	AutoConvertImages bool `json:"autoConvertImages" yaml:"autoConvertImages"`
	// Background jobs (intros and credits, trickplay, cleanup) wait while anyone is streaming,
	// whatever the machine's load: a failsafe
	PauseJobsWhileStreaming bool `json:"pauseJobsWhileStreaming,omitempty" yaml:"pauseJobsWhileStreaming"`
	// The graphics hardware that encodes the video of transcodes and downloads: "" (the CPU, x264), "nvenc"
	// (NVIDIA), "qsv" (Intel Quick Sync), "vaapi" (VA-API: AMD or Intel) or "videotoolbox" (Apple
	// Silicon Macs). Settings offers what
	// the server found working (media.Hardware).
	HardwareEncoder string `json:"hardwareEncoder,omitempty" yaml:"hardwareEncoder"`
	// The device for qsv and vaapi: a render node such as /dev/dri/renderD128
	HardwareDevice string `json:"hardwareDevice,omitempty" yaml:"hardwareDevice"`
	// At most this many transcodes and downloads on the hardware at once (0: no limit); the rest
	// use the CPU
	HardwareMaxTranscodes int `json:"hardwareMaxTranscodes,omitempty" yaml:"hardwareMaxTranscodes"`
}

// HardwareEncoders are the streaming.hardwareEncoder values ("" is the CPU).
var HardwareEncoders = []string{"nvenc", "qsv", "vaapi", "videotoolbox"}

var GlobalConfig *Config

// loadedPath is the file GlobalConfig was last loaded from, so saves go back to it.
var loadedPath string

// Path returns the file the current configuration was loaded from.
func Path() string {
	return loadedPath
}

// ExampleJWTSecret is config.example.yml's placeholder secret, which the server refuses.
const ExampleJWTSecret = "change-this-to-a-random-secret-key"

// CheckJWTSecret refuses a blank JWT secret or config.example.yml's placeholder: with either,
// anyone could make sign-in tokens for any user.
func (c *Config) CheckJWTSecret() error {
	switch s := strings.TrimSpace(c.Auth.JWTSecret); {
	case s == "":
		return errors.New("auth.jwtSecret is blank")
	case s == ExampleJWTSecret:
		return errors.New("auth.jwtSecret is still config.example.yml's placeholder")
	}
	return nil
}
