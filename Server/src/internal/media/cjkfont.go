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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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

var subsetMu sync.Mutex // one subsetter at a time: each reads the whole font

// SubsetCJK returns a subset of the CJK font holding the CJK characters of text, and the base Latin
// (baseText). Subsets are kept in <subtitlePath>/cjk, so each set is made once.
func SubsetCJK(ctx context.Context, text string) ([]byte, error) {
	chars := baseText + CJKCharacters(text)
	font := cjkFontFile()
	if font == "" {
		return nil, errors.New("no CJK font: set media.cjkFontPath, or install Noto Sans CJK")
	}
	sum := sha256.Sum256([]byte(font + "\x00" + chars))
	name := hex.EncodeToString(sum[:16]) + ".ttf"
	var cacheFile string
	if root := config.GlobalConfig.Media.SubtitlePath; root != "" {
		cacheFile = filepath.Join(root, "cjk", name)
		if b, err := os.ReadFile(cacheFile); err == nil {
			return b, nil
		}
	}

	subsetMu.Lock()
	defer subsetMu.Unlock()
	if cacheFile != "" {
		if b, err := os.ReadFile(cacheFile); err == nil { // made while this one waited
			return b, nil
		}
	}
	command := config.GlobalConfig.Media.SubsetCommand
	if command == "" {
		command = "pyftsubset"
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
	args = append(args, "--text="+chars, "--layout-features=*", "--output-file="+out.Name())
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exErr *exec.Error
		if errors.As(err, &exErr) {
			return nil, fmt.Errorf("the subsetter %q isn't installed (media.subsetCommand, or fontTools' pyftsubset)", command)
		}
		return nil, fmt.Errorf("the subsetter failed: %v: %s", err, lastLines(stderr.String(), 2))
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
			}
		}
	}
	return b, nil
}
