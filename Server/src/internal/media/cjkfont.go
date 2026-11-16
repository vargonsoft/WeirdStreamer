package media

// CJK fonts for styled (ASS) subtitles. A player's own fonts (the browser's, the phone's, the
// video's embedded ones) rarely hold Chinese, Japanese or Korean characters, and a CJK font is
// 20 MB or more. So the server makes a subset of one for the characters a subtitle track uses
// (a few KB, with fontTools' pyftsubset, which it runs once per set of characters and keeps in
// <subtitlePath>/cjk), and the players add it to what they draw with. Without a CJK font or the
// subsetter, the lines show as boxes, as before.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// baseText is what every subset holds besides the text's CJK characters: basic and Latin-1
// letters and punctuation. The players draw a subtitle's Latin letters from the subset too (their
// default font is the subset's family), so one font covers a line that mixes both.
var baseText = func() string {
	var b []rune
	for r := rune(0x20); r <= 0x7E; r++ {
		b = append(b, r)
	}
	for r := rune(0xA0); r <= 0xFF; r++ {
		b = append(b, r)
	}
	return string(b)
}()

// cjkFontCandidates are where Noto Sans CJK is installed on the usual systems, first found wins.
var cjkFontCandidates = []string{
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf",
	"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.otf",
}

// IsCJK reports whether r is a Chinese, Japanese or Korean character, or the punctuation and
// full-width forms written with them (which a Latin font lacks too).
func IsCJK(r rune) bool {
	switch {
	case r >= 0x3000 && r <= 0x303F: // CJK symbols and punctuation
	case r >= 0x3040 && r <= 0x30FF: // hiragana, katakana
	case r >= 0x31F0 && r <= 0x31FF: // katakana extensions
	case r >= 0x3130 && r <= 0x318F, r >= 0x1100 && r <= 0x11FF: // Hangul jamo
	case r >= 0x3400 && r <= 0x4DBF, r >= 0x4E00 && r <= 0x9FFF: // CJK ideographs
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul syllables
	case r >= 0xF900 && r <= 0xFAFF: // CJK compatibility ideographs
	case r >= 0xFF00 && r <= 0xFFEF: // full-width forms
	case r >= 0x20000 && r <= 0x2FA1F: // CJK extensions
	default:
		return false
	}
	return true
}

// CJKCharacters lists the distinct CJK characters in text, sorted.
func CJKCharacters(text string) string {
	seen := map[rune]bool{}
	for _, r := range text {
		if IsCJK(r) {
			seen[r] = true
		}
	}
	runes := make([]rune, 0, len(seen))
	for r := range seen {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	return string(runes)
}

// cjkFontFile is the CJK font the subsets come from: the config's, else the first installed
// candidate. "" when there's none.
func cjkFontFile() string {
	if p := config.GlobalConfig.Media.CJKFontPath; p != "" {
		return p
	}
	for _, p := range cjkFontCandidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// subsetSlots lets one subsetter run at a time (each reads the whole font). A waiting request gives
// up when its own context ends, rather than queueing behind the others for nothing.
var subsetSlots = make(chan struct{}, 1)

// maxCachedSubsets bounds the subset cache: the oldest files go first.
const maxCachedSubsets = 500

// SubsetCJK returns a subset of the CJK font holding the CJK characters of text, and the base Latin
// (baseText). Subsets are kept in <subtitlePath>/cjk, so each set is made once.
// ErrTooManyCharacters: the text has more distinct CJK characters than a subset is made for.
var ErrTooManyCharacters = errors.New("too many CJK characters")

// maxCJKCharacters bounds a subset (and its cache entry): far more than any subtitle track's.
const maxCJKCharacters = 8000

// ErrNoFont: no CJK font is installed or configured (media.cjkFontPath).
var ErrNoFont = errors.New("no CJK font")

// ErrNoSubsetter: the subsetter (media.subsetCommand) isn't installed.
var ErrNoSubsetter = errors.New("no subsetter")

func SubsetCJK(ctx context.Context, text string) ([]byte, error) {
	cjk := CJKCharacters(text)
	if len([]rune(cjk)) > maxCJKCharacters {
		return nil, ErrTooManyCharacters
	}
	chars := baseText + cjk
	font := cjkFontFile()
	if font == "" {
		return nil, ErrNoFont
	}
	// The key covers the font's file (replacing it makes new subsets), not just its path.
	info, err := os.Stat(font)
	if err != nil {
		return nil, ErrNoFont
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d\x00%s", font, info.Size(), info.ModTime().UnixNano(), chars)))
	name := hex.EncodeToString(sum[:16]) + ".ttf"
	var cacheFile string
	if root := config.GlobalConfig.Media.SubtitlePath; root != "" {
		cacheFile = filepath.Join(root, "cjk", name)
		if b, err := os.ReadFile(cacheFile); err == nil {
			return b, nil
		}
	}

	select {
	case subsetSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-subsetSlots }()
	if cacheFile != "" {
		if b, err := os.ReadFile(cacheFile); err == nil { // made while this one waited
			return b, nil
		}
	}
	command := config.GlobalConfig.Media.SubsetCommand
	if command == "" {
		command = "pyftsubset"
	}
	// The characters go in a file, not an argument: a long list would exceed the command line's limit.
	textFile, err := tempFileName("cjk-text-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(textFile)
	if err := os.WriteFile(textFile, []byte(chars), 0o600); err != nil {
		return nil, err
	}
	out, err := os.CreateTemp(TempDir(), tempPrefix+"cjk-*.ttf")
	if err != nil {
		return nil, err
	}
	out.Close()
	defer os.Remove(out.Name())
	args := []string{font}
	if strings.HasSuffix(strings.ToLower(font), ".ttc") || strings.HasSuffix(strings.ToLower(font), ".otc") {
		args = append(args, "--font-number=0") // the Japanese set, of the collection's
	}
	args = append(args, "--text-file="+textFile, "--layout-features=*", "--output-file="+out.Name())
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exErr *exec.Error
		if errors.As(err, &exErr) {
			return nil, ErrNoSubsetter
		}
		// The details (which may name paths) go to the server's log, not to clients.
		log.Printf("[SUBTITLES] pyftsubset failed: %v: %s", err, lastLines(stderr.String(), 3))
		return nil, errors.New("the subsetter failed")
	}
	b, err := os.ReadFile(out.Name())
	if err != nil {
		return nil, err
	}
	if cacheFile != "" {
		if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err == nil {
			tmp := cacheFile + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				os.Rename(tmp, cacheFile)
				pruneSubsets(filepath.Dir(cacheFile))
			}
		}
	}
	return b, nil
}

// pruneSubsets removes the oldest cached subsets beyond maxCachedSubsets (the subset just made is
// the newest, so it stays).
func pruneSubsets(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= maxCachedSubsets {
		return
	}
	type file struct {
		name string
		mod  time.Time
	}
	var files []file
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() && strings.HasSuffix(e.Name(), ".ttf") {
			files = append(files, file{e.Name(), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for i := 0; i < len(files)-maxCachedSubsets; i++ {
		os.Remove(filepath.Join(dir, files[i].name))
	}
}
