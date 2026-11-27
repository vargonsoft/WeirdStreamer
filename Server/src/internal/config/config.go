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
	Guide     GuideConfig     `json:"guide" yaml:"guide"`
}

// GuideConfig is the Guide's (the live pseudo-channels made from the libraries Show in Guide puts
// there; docs/GUIDE.md).
type GuideConfig struct {
	// The hour prime time starts, in the server's time zone: 20 (8 to 11 pm, the default) or 19
	// (7 to 10 pm). The news hour follows it.
	PrimeTime int `json:"primeTime" yaml:"primeTime"`
	// The US ZIP code the Weather channel's forecast is for (New York City's, 10001, when empty).
	WeatherZip string `json:"weatherZip,omitempty" yaml:"weatherZip,omitempty"`
	// The folder of music for the Weather channel and the Guide channel's cards (default guide-music
	// beside the thumbnails folder). A <track>.txt beside a track holds its credit line.
	MusicPath string `json:"musicPath,omitempty" yaml:"musicPath,omitempty"`
	// Voice-overs on the credits ("Stay tuned for…"), made by Piper: its command (a binary, or
	// "python3 -m piper"), and the folder of its voice models (.onnx with their .onnx.json). Without
	// a command there are none.
	PiperCommand string `json:"piperCommand,omitempty" yaml:"piperCommand,omitempty"`
	PiperVoices  string `json:"piperVoices,omitempty" yaml:"piperVoices,omitempty"`
	// The announcers, their fixes and the lines they say (default guide-voices.yml beside config.yml,
	// written with its defaults when it's missing), and where the voice-overs made are kept (default
	// voiceovers beside the thumbnails folder).
	VoicesFile    string `json:"voicesFile,omitempty" yaml:"voicesFile,omitempty"`
	VoiceOverPath string `json:"voiceOverPath,omitempty" yaml:"voiceOverPath,omitempty"`
	// Channels an admin defines: a type and the libraries they're made from (typed.go). Settings'
	// Guide tab changes these.
	TypedChannels []TypedChannel `json:"typedChannels,omitempty" yaml:"typedChannels,omitempty"`
}

// Prime time's starts (GuideConfig.PrimeTime)
const (
	PrimeTimeEarly   = 19
	PrimeTimeDefault = 20
	PrimeTimeHours   = 3
)

// JobsConfig tunes the trickplay job, which the Jobs page changes live.
type JobsConfig struct {
	TrickplayRunners         int `json:"trickplayRunners" yaml:"trickplayRunners"`                 // videos made at once: 1 to 8 (0: one per disk the libraries are on)
	TrickplayFramesPerRunner int `json:"trickplayFramesPerRunner" yaml:"trickplayFramesPerRunner"` // frames per ffmpeg process: 1 to 50 (0: what fills half the memory)
	// Decode trickplay frames on the CPU even with a hardware encoder chosen (whose GPU decodes them
	// otherwise): a CPU with many cores can make them faster than a small GPU.
	TrickplayCPUOnly bool `json:"trickplayCPUOnly" yaml:"trickplayCPUOnly"`
	// When every background job waits for a disk or share (0: the default): a disk's busy share of its
	// time, in percent, and a network share's read time, in ms; each where someone is playing from it
	// and where no one is (Idle). A job already reading stops at the stop limits worked out from these.
	WaitDiskPercent     int `json:"waitDiskPercent" yaml:"waitDiskPercent"`
	WaitDiskIdlePercent int `json:"waitDiskIdlePercent" yaml:"waitDiskIdlePercent"`
	WaitShareMs         int `json:"waitShareMs" yaml:"waitShareMs"`
	WaitShareIdleMs     int `json:"waitShareIdleMs" yaml:"waitShareIdleMs"`
}

// WaitLimits are the jobs' wait limits, as config.yml has them (0: the default) or in effect.
type WaitLimits struct {
	DiskPercent     int `json:"disk_percent"`      // a disk someone plays from
	DiskIdlePercent int `json:"disk_idle_percent"` // one nobody plays from
	ShareMs         int `json:"share_ms"`          // a share's read time, someone playing from it
	ShareIdleMs     int `json:"share_idle_ms"`     // ... nobody
}

// DefaultWaitLimits are the limits a 0 gives.
var DefaultWaitLimits = WaitLimits{DiskPercent: 50, DiskIdlePercent: 90, ShareMs: 60, ShareIdleMs: 500}

const (
	MinWaitDiskPercent, MaxWaitDiskPercent = 5, 99
	MinWaitShareMs, MaxWaitShareMs         = 5, 10000
)

// Set returns the limits config.yml sets (0: the default).
func (j JobsConfig) Set() WaitLimits {
	return WaitLimits{j.WaitDiskPercent, j.WaitDiskIdlePercent, j.WaitShareMs, j.WaitShareIdleMs}
}

// Effective fills in the defaults.
func (w WaitLimits) Effective() WaitLimits {
	or := func(v, d int) int {
		if v == 0 {
			return d
		}
		return v
	}
	d := DefaultWaitLimits
	return WaitLimits{or(w.DiskPercent, d.DiskPercent), or(w.DiskIdlePercent, d.DiskIdlePercent),
		or(w.ShareMs, d.ShareMs), or(w.ShareIdleMs, d.ShareIdleMs)}
}

func (w WaitLimits) validate() error {
	for _, p := range []int{w.DiskPercent, w.DiskIdlePercent} {
		if p != 0 && (p < MinWaitDiskPercent || p > MaxWaitDiskPercent) {
			return fmt.Errorf("a disk's wait limit must be %d to %d%% (0: the default)", MinWaitDiskPercent, MaxWaitDiskPercent)
		}
	}
	for _, ms := range []int{w.ShareMs, w.ShareIdleMs} {
		if ms != 0 && (ms < MinWaitShareMs || ms > MaxWaitShareMs) {
			return fmt.Errorf("a share's wait limit must be %d to %d ms (0: the default)", MinWaitShareMs, MaxWaitShareMs)
		}
	}
	if e := w.Effective(); e.DiskIdlePercent < e.DiskPercent || e.ShareIdleMs < e.ShareMs {
		return fmt.Errorf("the limit where no one is playing can't be lower than the one where someone is")
	}
	return nil
}

var waitLimits atomic.Pointer[WaitLimits]

// SetWaitLimits sets the limits the jobs read (Load does, and tests).
func SetWaitLimits(w WaitLimits) { waitLimits.Store(&w) }

// CurrentWaitLimits are the jobs' wait limits in effect (live: the Jobs page changes them).
func CurrentWaitLimits() WaitLimits {
	if w := waitLimits.Load(); w != nil {
		return w.Effective()
	}
	return DefaultWaitLimits
}

const (
	MaxTrickplayRunners = 8  // the pool of runners the job starts with
	MaxTrickplayFrames  = 50 // one process's frames: more ran out of memory
	MaxTrickplayProduct = 100
)

// The trickplay settings, read by its runners at every step while the Jobs page can change them:
// kept apart from GlobalConfig so the reads are safe. Set when the config loads (0: automatic).
var trickplayRunners, trickplayFrames atomic.Int64

var trickplayCPUOnly atomic.Bool

// TrickplayCPUOnly says trickplay decodes on the CPU even with a hardware encoder chosen (live).
func TrickplayCPUOnly() bool { return trickplayCPUOnly.Load() }

// SetTrickplayCPUOnly sets it (Load does, and tests).
func SetTrickplayCPUOnly(on bool) { trickplayCPUOnly.Store(on) }

// SetTrickplaySettings sets the trickplay settings the job reads (Load does, and tests).
func SetTrickplaySettings(runners, framesPerRunner int) {
	trickplayRunners.Store(int64(runners))
	trickplayFrames.Store(int64(framesPerRunner))
}

// TrickplaySettings are the trickplay job's settings as config.yml has them: 0 for automatic, which
// media.TrickplayRunners and media.TrickplayFramesPerRunner work out (live).
func TrickplaySettings() (runners, framesPerRunner int) {
	r, f := int(trickplayRunners.Load()), int(trickplayFrames.Load())
	if r != 0 {
		r = min(max(r, 1), MaxTrickplayRunners)
	}
	if f != 0 {
		f = min(max(f, 1), MaxTrickplayFrames)
	}
	return r, f
}

// UpdateJobs saves new jobs settings (trickplay's, the wait limits) to the config file and reloads them.
func UpdateJobs(jobs JobsConfig) error {
	c := *GlobalConfig
	c.Jobs = jobs
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
	// Where this library's titles are on the Guide's channels: ContinueOn (the Guide on the home
	// screen's Libraries row, and on the library's own page), ContinueLibraryOnly (only a Guide of its
	// own on its page) or ContinueOff (the default, also when empty).
	ShowInGuide string `json:"showInGuide,omitempty" yaml:"showInGuide,omitempty"`
	// The Guide may take this library's short videos to fill the gaps between programs. They stay
	// the library's: it's still browsed and played as usual.
	GuideFiller bool `json:"guideFiller,omitempty" yaml:"guideFiller,omitempty"`
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

// InGuide reports whether the library's titles are on the Guide's channels: the home screen's Guide
// (libraryPage false), which blends the libraries, or the library's own (true).
func (p MediaPath) InGuide(libraryPage bool) bool {
	switch p.ShowInGuide {
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
// LibraryByID is the library with the ID.
func (c *Config) LibraryByID(id string) (MediaPath, bool) {
	for _, lib := range c.Media.Paths {
		if lib.ID == id {
			return lib, true
		}
	}
	return MediaPath{}, false
}

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
	switch c.Guide.PrimeTime {
	case 0:
		c.Guide.PrimeTime = PrimeTimeDefault
	case PrimeTimeEarly, PrimeTimeDefault:
	default:
		return fmt.Errorf("guide.primeTime must be %d (7 to 10 pm) or %d (8 to 11 pm)", PrimeTimeEarly, PrimeTimeDefault)
	}
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
	if r, f := c.Jobs.TrickplayRunners, c.Jobs.TrickplayFramesPerRunner; r < 0 || r > MaxTrickplayRunners || f < 0 || f > MaxTrickplayFrames {
		return fmt.Errorf("trickplay runners must be 1 to %d, and frames per runner 1 to %d (0: automatic)", MaxTrickplayRunners, MaxTrickplayFrames)
	} else if r > 0 && f > 0 && r*f > MaxTrickplayProduct {
		return fmt.Errorf("runners times frames per runner can't exceed %d (about %d GB of memory)", MaxTrickplayProduct, MaxTrickplayProduct*110/1000)
	}
	if err := c.Jobs.Set().validate(); err != nil {
		return err
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
		if p.ShowInGuide, err = canonicalMode(p.ShowInGuide); err != nil {
			return fmt.Errorf("media path %q: invalid showInGuide %w", p.Name, err)
		}
	}
	return c.validateTypedChannels()
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
	ThemePath             string      `json:"themePath,omitempty" yaml:"themePath"`                   // Theme music cut from shows' intros and movies' credits (and theme files converted)
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
	// The graphics hardware that encodes the video of transcodes and downloads, and decodes trickplay's
	// frames (media.trickplayDecoder): "" (the CPU, x264), "nvenc"
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
