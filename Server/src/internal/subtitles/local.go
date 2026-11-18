// Package subtitles finds subtitles kept outside a video: files beside it (Movie.en.srt, or in a
// Subs folder) and ones downloaded from OpenSubtitles into the configured subtitlePath.
package subtitles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/weirdstreamer/server/internal/config"
)

// Track is a subtitle file for a video.
type Track struct {
	Path            string
	Codec           string // as ffprobe names it: subrip, ass, webvtt
	Language        string // ISO 639-2, like the video's own tracks ("eng"); "" if unknown
	Title           string // the file's name, or the release a download was made for
	Forced          bool
	HearingImpaired bool
	Source          string // SourceFile or SourceDownload
	FileID          int    // downloads: the OpenSubtitles file id
}

const (
	SourceFile     = "file"          // beside the video
	SourceDownload = "opensubtitles" // downloaded into subtitlePath
)

// Codecs of the subtitle files used, by extension: text formats ffmpeg turns into WebVTT.
var extCodecs = map[string]string{".srt": "subrip", ".ass": "ass", ".ssa": "ssa", ".vtt": "webvtt"}

// subFolders are the folders releases keep subtitles in, beside the video.
var subFolders = []string{"subs", "subtitles", "sub"}

// Sidecars finds the subtitle files that go with a video: in its folder, those named after it
// (Movie.srt, Movie.en.srt, Movie.eng.forced.srt); in a Subs folder beside it, those named after
// it, or every one when the video is the only one in its folder.
func Sidecars(videoPath string, isVideo func(name string) bool) []Track {
	dir := filepath.Dir(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var tracks []Track
	videos := 0
	var folders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			for _, f := range subFolders {
				if strings.EqualFold(name, f) {
					folders = append(folders, filepath.Join(dir, name))
				}
			}
			continue
		}
		if isVideo(name) {
			videos++
		}
		if t, ok := sidecar(filepath.Join(dir, name), base, false); ok {
			tracks = append(tracks, t)
		}
	}
	for _, folder := range folders {
		files, _ := os.ReadDir(folder)
		for _, e := range files {
			if e.IsDir() {
				continue
			}
			if t, ok := sidecar(filepath.Join(folder, e.Name()), base, videos == 1); ok {
				tracks = append(tracks, t)
			}
		}
	}
	sort.SliceStable(tracks, func(i, j int) bool { return tracks[i].Path < tracks[j].Path })
	return tracks
}

// sidecar reads a subtitle file named base[.lang][.forced|.sdh…].ext (any name, with anyName).
func sidecar(path, base string, anyName bool) (Track, bool) {
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(name))
	codec, ok := extCodecs[ext]
	if !ok {
		return Track{}, false
	}
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	var rest string
	switch {
	case strings.EqualFold(stem, base):
	case len(stem) > len(base) && strings.EqualFold(stem[:len(base)], base) && (stem[len(base)] == '.' || stem[len(base)] == '_'):
		rest = stem[len(base)+1:]
	case anyName:
		rest = stem
	default:
		return Track{}, false
	}
	t := Track{Path: path, Codec: codec, Title: name, Source: SourceFile}
	// The tags after the name: a language, and forced or hearing-impaired markers.
	for _, tag := range strings.FieldsFunc(strings.ToLower(rest), func(r rune) bool { return r == '.' || r == '_' || r == ' ' || r == '-' }) {
		switch tag {
		case "forced", "foreign":
			t.Forced = true
		case "sdh", "hi", "cc":
			t.HearingImpaired = true
		default:
			if lang := Language(tag); lang != "" && t.Language == "" {
				t.Language = lang
			}
		}
	}
	return t, true
}

// The downloaded subtitles of a video live in their own folder under subtitlePath, named by a
// hash of the video's path, with an index of what each file is.
type storedIndex struct {
	Video     string           `json:"video"` // its client path, for a person reading the file
	Downloads []storedSubtitle `json:"downloads"`
}

type storedSubtitle struct {
	FileID          int       `json:"file_id"`
	File            string    `json:"file"`
	Language        string    `json:"language"` // ISO 639-2
	Release         string    `json:"release"`
	HearingImpaired bool      `json:"hearing_impaired"`
	Downloaded      time.Time `json:"downloaded"`
}

// storeDir is where a video's downloads go.
func storeDir(root, videoPath string) string {
	h := config.CacheKey(videoPath)
	return filepath.Join(root, h[:2], h)
}

func readIndex(dir string) storedIndex {
	var idx storedIndex
	if b, err := os.ReadFile(filepath.Join(dir, "index.json")); err == nil {
		json.Unmarshal(b, &idx)
	}
	return idx
}

// Downloaded lists a video's downloaded subtitles, oldest first.
func Downloaded(root, videoPath string) []Track {
	if root == "" {
		return nil
	}
	dir := storeDir(root, videoPath)
	var tracks []Track
	for _, d := range readIndex(dir).Downloads {
		path := filepath.Join(dir, d.File)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		tracks = append(tracks, Track{Path: path, Codec: "subrip", Language: d.Language, Title: d.Release,
			HearingImpaired: d.HearingImpaired, Source: SourceDownload, FileID: d.FileID})
	}
	return tracks
}

// Store keeps a downloaded subtitle for a video (an SRT file) and returns it as a track.
func Store(root, videoPath string, r Result, data []byte) (Track, error) {
	if root == "" {
		return Track{}, fmt.Errorf("no subtitlePath is configured")
	}
	dir := storeDir(root, videoPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Track{}, err
	}
	lang := Language(r.Language)
	file := fmt.Sprintf("%d.%s.srt", r.FileID, firstNonEmpty(lang, "und"))
	if err := os.WriteFile(filepath.Join(dir, file), data, 0o644); err != nil {
		return Track{}, err
	}
	idx := readIndex(dir)
	idx.Video, _ = config.ClientPath(videoPath)
	kept := idx.Downloads[:0]
	for _, d := range idx.Downloads {
		if d.FileID != r.FileID {
			kept = append(kept, d)
		}
	}
	idx.Downloads = append(kept, storedSubtitle{FileID: r.FileID, File: file, Language: lang, Release: r.Release,
		HearingImpaired: r.HearingImpaired, Downloaded: time.Now().UTC()})
	b, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "index.json"), b, 0o644); err != nil {
		return Track{}, err
	}
	return Track{Path: filepath.Join(dir, file), Codec: "subrip", Language: lang, Title: r.Release,
		HearingImpaired: r.HearingImpaired, Source: SourceDownload, FileID: r.FileID}, nil
}

// Charset is the character set to read a subtitle file in: UTF-8, else Windows-1252 (what most
// older SRT files are in).
func Charset(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || utf8.Valid(b) {
		return "UTF-8"
	}
	return "CP1252"
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
