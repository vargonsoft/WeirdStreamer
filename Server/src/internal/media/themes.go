package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/fileio"
)

// Theme music for title pages: a theme file in the show's or movie's folder (theme.flac,
// theme.wav, theme.m4a…, as Plex and Jellyfin take them), else cut from the videos: a show's
// intro, or a movie's main theme (the music its opening and its credits share) or the start of its
// credits. Cuts, and theme files the players can't all play, are kept as AAC in media.themePath.

// themeExts are the theme file types, the best first (one folder with several plays the best).
var themeExts = []string{".flac", ".wav", ".m4a", ".aac", ".mp3", ".opus", ".ogg", ".oga", ".wma"}

// playedAsIs are the theme file types every player takes (the iPhone's can't play Ogg or WMA).
var playedAsIs = map[string]bool{".flac": true, ".wav": true, ".m4a": true, ".aac": true, ".mp3": true}

// ThemeFile is a folder's theme file (named theme.<ext>, any case), the best kind there is, or "".
func ThemeFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	found := map[string]string{}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		ext := filepath.Ext(name)
		if !e.IsDir() && strings.TrimSuffix(name, ext) == "theme" {
			found[ext] = filepath.Join(dir, e.Name())
		}
	}
	for _, ext := range themeExts {
		if p := found[ext]; p != "" {
			return p
		}
	}
	return ""
}

// Lengths of cut themes: a show's intro up to this; a movie's credits from their start this long.
const (
	maxThemeLength     = 120.0
	creditsThemeLength = 60.0
	themeOpening       = 600.0 // a movie's start searched for its theme: its first ten minutes
	minSharedTheme     = 20.0  // the shortest music the opening and the credits share that counts
)

func themeDir() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Media.ThemePath
}

// One cut or conversion of a theme at a time each (two requests for one wait for the first).
var themeLocks = struct {
	sync.Mutex
	by map[string]*sync.Mutex
}{by: map[string]*sync.Mutex{}}

func themeLock(key string) *sync.Mutex {
	themeLocks.Lock()
	defer themeLocks.Unlock()
	if themeLocks.by[key] == nil {
		themeLocks.by[key] = &sync.Mutex{}
	}
	return themeLocks.by[key]
}

// themeKey names a cached theme by what it was made from: the file (its path, size and time) and
// what part of it.
func themeKey(path string, parts ...any) string {
	h := sha256.New()
	fmt.Fprint(h, config.CacheKey(path))
	if info, err := os.Stat(path); err == nil {
		fmt.Fprint(h, info.Size(), info.ModTime().UnixNano())
	}
	for _, p := range parts {
		fmt.Fprintf(h, "|%v", p)
	}
	return hex.EncodeToString(h.Sum(nil))[:40]
}

// ErrNoThemeDir is returned when media.themePath isn't set (cuts and conversions need it).
var ErrNoThemeDir = errors.New("no media.themePath")

// cachedTheme makes the cached file for key with ffmpeg (args before the output) unless it's there.
func cachedTheme(ctx context.Context, key string, args []string) (string, error) {
	dir := themeDir()
	if dir == "" {
		return "", ErrNoThemeDir
	}
	out := filepath.Join(dir, key[:2], key+".m4a")
	lock := themeLock(key)
	lock.Lock()
	defer lock.Unlock()
	if info, err := os.Stat(out); err == nil && info.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o700); err != nil {
		return "", err
	}
	part := out + ".part"
	defer os.Remove(part)
	cmd := backgroundCommand(ctx, "ffmpeg", append(append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}, args...),
		"-vn", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-f", "mp4", part)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg: %v %s", err, lastLine(stderr.String()))
	}
	if info, err := os.Stat(part); err != nil || info.Size() == 0 {
		return "", errors.New("ffmpeg made no audio")
	}
	return out, fileio.Rename(part, out)
}

// ServableTheme is a theme file as the players get it: itself when they all play its type, else
// converted to AAC once.
func ServableTheme(ctx context.Context, file string) (string, error) {
	if playedAsIs[strings.ToLower(filepath.Ext(file))] {
		return file, nil
	}
	return cachedTheme(ctx, themeKey(file, "converted"), []string{"-i", file, "-map", "0:a:0"})
}

// CutTheme is part of a video's audio, cut out (faded in and out) as a theme.
func CutTheme(ctx context.Context, video string, start, length float64) (string, error) {
	length = min(length, maxThemeLength)
	if length < 3 {
		return "", errors.New("too short for a theme")
	}
	fadeOut := max(0, length-2)
	return cachedTheme(ctx, themeKey(video, fmt.Sprintf("%.2f", start), fmt.Sprintf("%.2f", length)), []string{
		"-ss", fmt.Sprintf("%.3f", start), "-t", fmt.Sprintf("%.3f", length), "-i", Playable(video),
		"-map", "0:a:0", "-af", fmt.Sprintf("afade=t=in:d=0.5,afade=t=out:st=%.3f:d=2", fadeOut)})
}

// Found main themes by movie (file, size, time, credits), so each movie is searched once.
var movieThemes = struct {
	sync.Mutex
	found map[string]*Segment // nil: searched, none
}{found: map[string]*Segment{}}

// MovieThemeSpan is the part of a movie to play as its theme: the music its opening (its first
// ten minutes) and its credits share, which is usually its main theme, else the start of its
// credits. nil when its credits aren't known.
func MovieThemeSpan(ctx context.Context, video string, credits *Segment, duration float64) *Segment {
	if credits == nil || credits.Start <= 0 {
		return nil
	}
	fallback := &Segment{Start: credits.Start, End: credits.Start + creditsThemeLength}
	if duration > credits.Start {
		fallback.End = min(fallback.End, duration)
	}
	key := themeKey(video, fmt.Sprintf("%.1f", credits.Start))
	movieThemes.Lock()
	found, searched := movieThemes.found[key]
	movieThemes.Unlock()
	if !searched {
		found = sharedTheme(ctx, video, credits, duration)
		if ctx.Err() != nil {
			return fallback
		}
		movieThemes.Lock()
		movieThemes.found[key] = found
		movieThemes.Unlock()
	}
	if found != nil {
		return found
	}
	return fallback
}

// sharedTheme looks for the music a movie's opening and its credits share, in the credits' time.
func sharedTheme(ctx context.Context, video string, credits *Segment, duration float64) *Segment {
	openLen := min(themeOpening, credits.Start/2)
	creditsLen := themeOpening
	if duration > credits.Start {
		creditsLen = min(creditsLen, duration-credits.Start)
	}
	if openLen < minSharedTheme || creditsLen < minSharedTheme {
		return nil
	}
	opening, err := Fingerprint(ctx, video, 0, openLen)
	if err != nil {
		return nil
	}
	end, err := Fingerprint(ctx, video, credits.Start, creditsLen)
	if err != nil {
		return nil
	}
	_, inCredits := SharedSegment(opening, end, 0, credits.Start, minSharedTheme, themeOpening)
	return inCredits
}
