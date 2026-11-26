package guide

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/fileio"
	"github.com/weirdstreamer/server/internal/media"
)

// Voice-overs on the credits (docs/GUIDE.md): "Stay tuned for…", said by the channel's announcer.
// Piper speaks a line (with the announcer's fixes for words it says wrong), ffmpeg makes it sound
// like a 1980s announce booth, and the result is kept by a hash of the line and everything that
// shaped it, so changing a fix remakes only what it touches. The players play it over the end of
// the program, its sound lowered under it.

//go:embed voices.default.yml
var defaultVoices []byte

// VoicesFile is guide-voices.yml.
type VoicesFile struct {
	Announcers []Announcer `yaml:"announcers" json:"announcers"`
	Lines      []string    `yaml:"lines" json:"lines"`
	LaterLines []string    `yaml:"laterLines" json:"later_lines"`
}

// Announcer is a voice: its Piper model, how it speaks, its sound and its fixes.
type Announcer struct {
	Name            string  `yaml:"name" json:"name"`
	Model           string  `yaml:"model" json:"model"`
	LengthScale     float64 `yaml:"lengthScale" json:"length_scale"`
	NoiseScale      float64 `yaml:"noiseScale" json:"noise_scale"`
	NoiseW          float64 `yaml:"noiseW" json:"noise_w"`
	SentenceSilence float64 `yaml:"sentenceSilence" json:"sentence_silence"`
	Filter          string  `yaml:"filter" json:"filter"`
	Fixes           []Fix   `yaml:"fixes" json:"fixes"`
}

// Fix is what Piper is given for words it says wrong.
type Fix struct {
	Match string  `yaml:"match" json:"match"`
	Say   string  `yaml:"say" json:"say"`
	Pace  float64 `yaml:"pace,omitempty" json:"pace,omitempty"`
}

// Voices reads guide-voices.yml (again when it changes), writing its defaults when it's missing.
type Voices struct {
	mu      sync.Mutex
	file    *VoicesFile
	modTime time.Time
	made    map[string]VoiceOver // by key: the voice-overs known to be in the cache
	render  sync.Mutex           // one Piper at a time
}

// VoiceOver is a voice-over made: its file's key, its length and what it says.
type VoiceOver struct {
	Key       string        `json:"key"`
	Duration  time.Duration `json:"-"`
	Seconds   float64       `json:"seconds"`
	Line      string        `json:"line"`
	Announcer string        `json:"announcer"`
}

// NewVoices makes the voice-overs' reader and maker.
func NewVoices() *Voices { return &Voices{made: map[string]VoiceOver{}} }

func voicesPath() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Guide.VoicesFile
}

// VoiceOverDir is where the voice-overs are kept.
func VoiceOverDir() string {
	if config.GlobalConfig == nil {
		return ""
	}
	return config.GlobalConfig.Guide.VoiceOverPath
}

// Enabled reports whether voice-overs can be made: Piper's command is set.
func (v *Voices) Enabled() bool {
	return config.GlobalConfig != nil && strings.TrimSpace(config.GlobalConfig.Guide.PiperCommand) != ""
}

// File is guide-voices.yml as it is now.
func (v *Voices) File() (*VoicesFile, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.load()
}

func (v *Voices) load() (*VoicesFile, error) {
	path := voicesPath()
	if path == "" {
		return parseVoices(defaultVoices)
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := writeAtomic(path, defaultVoices, 0o600); err != nil {
			log.Printf("[VOICES] Couldn't write %s: %v", path, err)
			return parseVoices(defaultVoices)
		}
		log.Printf("[VOICES] Wrote %s with the default announcers", path)
		info, err = os.Stat(path)
	}
	if err != nil {
		return nil, err
	}
	if v.file != nil && info.ModTime().Equal(v.modTime) {
		return v.file, nil
	}
	b, err := fileio.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parseVoices(b)
	if err != nil {
		if v.file != nil {
			log.Printf("[VOICES] %s: %v (keeping what was read before)", path, err)
			return v.file, nil
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	v.file, v.modTime = f, info.ModTime()
	return f, nil
}

func parseVoices(b []byte) (*VoicesFile, error) {
	var f VoicesFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if len(f.Announcers) == 0 || len(f.Lines) == 0 {
		return nil, errors.New("it needs announcers and lines")
	}
	return &f, nil
}

// AnnouncerFor is a channel's announcer: one of the list, by its number.
func (f *VoicesFile) AnnouncerFor(channel int) Announcer {
	return f.Announcers[(channel%len(f.Announcers)+len(f.Announcers))%len(f.Announcers)]
}

// Announcer is an announcer by name.
func (f *VoicesFile) Announcer(name string) (Announcer, bool) {
	for _, a := range f.Announcers {
		if strings.EqualFold(a.Name, name) {
			return a, true
		}
	}
	return Announcer{}, false
}

// piece is part of a line as Piper says it, at a pace (0: the announcer's).
type piece struct {
	Text string
	Pace float64
}

// fixPattern matches a fix's words as whole words, in any case.
func fixPattern(match string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(strings.TrimSpace(match)) + `\b`)
}

// pieces is a line with the announcer's fixes: replaced where they match, and a fix with a pace
// split out to be said on its own.
func (a Announcer) pieces(line string, fixes bool) []piece {
	if !fixes {
		return []piece{{Text: line}}
	}
	out := []piece{{Text: line}}
	for _, f := range a.Fixes {
		if strings.TrimSpace(f.Match) == "" {
			continue
		}
		re := fixPattern(f.Match)
		var next []piece
		for _, p := range out {
			if p.Pace != 0 { // already a fix of its own
				next = append(next, p)
				continue
			}
			if f.Pace == 0 {
				next = append(next, piece{Text: re.ReplaceAllLiteralString(p.Text, f.Say)})
				continue
			}
			at := 0
			for _, m := range re.FindAllStringIndex(p.Text, -1) {
				if m[0] > at {
					next = append(next, piece{Text: p.Text[at:m[0]]})
				}
				next = append(next, piece{Text: f.Say, Pace: f.Pace})
				at = m[1]
			}
			if at < len(p.Text) {
				next = append(next, piece{Text: p.Text[at:]})
			}
		}
		out = next
	}
	var kept []piece
	for _, p := range out {
		if strings.TrimSpace(p.Text) != "" {
			kept = append(kept, p)
		}
	}
	return kept
}

// Key is the voice-over's name in the cache: the line and everything that shapes how it's said.
func (a Announcer) Key(line string, fixes bool) string {
	b, _ := json.Marshal(struct {
		Line   string
		A      Announcer
		Fixes  bool
		Format int
	}{line, a, fixes, 1})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:24]
}

// Made is a voice-over already in the cache (its length read once), or false.
func (v *Voices) Made(a Announcer, line string) (VoiceOver, bool) {
	return v.made1(a.Key(line, true), line, a.Name)
}

func (v *Voices) made1(key, line, announcer string) (VoiceOver, bool) {
	v.mu.Lock()
	vo, ok := v.made[key]
	v.mu.Unlock()
	if ok {
		return vo, true
	}
	path := filepath.Join(VoiceOverDir(), key+".m4a")
	if _, err := os.Stat(path); err != nil {
		return VoiceOver{}, false
	}
	d, err := probeLength(path)
	if err != nil {
		return VoiceOver{}, false
	}
	vo = VoiceOver{Key: key, Duration: d, Seconds: d.Seconds(), Line: line, Announcer: announcer}
	v.mu.Lock()
	v.made[key] = vo
	v.mu.Unlock()
	return vo, true
}

// VoiceOverFile is a voice-over's file by its key (a key is hex only).
func VoiceOverFile(key string) (string, bool) {
	if len(key) != 24 || strings.Trim(key, "0123456789abcdef") != "" || VoiceOverDir() == "" {
		return "", false
	}
	path := filepath.Join(VoiceOverDir(), key+".m4a")
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// Make makes a voice-over (with the announcer's fixes, or without them for the tester), or gives
// the one made before.
func (v *Voices) Make(ctx context.Context, a Announcer, line string, fixes bool) (VoiceOver, error) {
	key := a.Key(line, fixes)
	if vo, ok := v.made1(key, line, a.Name); ok {
		return vo, nil
	}
	if !v.Enabled() {
		return VoiceOver{}, errors.New("Piper isn't set up (guide.piperCommand)")
	}
	v.render.Lock()
	defer v.render.Unlock()
	if vo, ok := v.made1(key, line, a.Name); ok { // made while this one waited
		return vo, nil
	}
	dir := VoiceOverDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return VoiceOver{}, err
	}
	tmp, err := os.MkdirTemp(media.TempDir(), "weirdstreamer-voice-")
	if err != nil {
		return VoiceOver{}, err
	}
	defer os.RemoveAll(tmp)
	var wavs []string
	for i, p := range a.pieces(line, fixes) {
		wav := filepath.Join(tmp, strconv.Itoa(i)+".wav")
		if _, err := runPiper(ctx, a, p, wav); err != nil {
			return VoiceOver{}, err
		}
		wavs = append(wavs, wav)
	}
	if len(wavs) == 0 {
		return VoiceOver{}, errors.New("nothing to say")
	}
	// the pieces joined, then the announce booth's sound, as AAC
	args := []string{"-hide_banner", "-loglevel", "error", "-y"}
	for _, w := range wavs {
		args = append(args, "-i", w)
	}
	chain := ""
	for i := range wavs {
		chain += fmt.Sprintf("[%d:a]", i)
	}
	chain += fmt.Sprintf("concat=n=%d:v=0:a=1[joined]", len(wavs))
	filter := strings.TrimSpace(a.Filter)
	if filter == "" {
		filter = "anull"
	}
	chain += ";[joined]" + filter + ",aresample=44100[out]"
	part := filepath.Join(tmp, "out.m4a")
	args = append(args, "-filter_complex", chain, "-map", "[out]", "-ac", "1", "-c:a", "aac", "-b:a", "96k", part)
	if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return VoiceOver{}, fmt.Errorf("ffmpeg: %v: %s", err, lastLines(out))
	}
	final := filepath.Join(dir, key+".m4a")
	if err := fileio.Rename(part, final); err != nil {
		if b, rerr := os.ReadFile(part); rerr == nil { // the temp folder on another disk
			err = writeAtomic(final, b, 0o600)
		}
		if err != nil {
			return VoiceOver{}, err
		}
	}
	vo, ok := v.made1(key, line, a.Name)
	if !ok {
		return VoiceOver{}, errors.New("the voice-over wasn't readable")
	}
	return vo, nil
}

// runPiper says a piece into wav, giving Piper's phonemes for it.
func runPiper(ctx context.Context, a Announcer, p piece, wav string) (string, error) {
	cmdline := strings.Fields(config.GlobalConfig.Guide.PiperCommand)
	if len(cmdline) == 0 {
		return "", errors.New("Piper isn't set up (guide.piperCommand)")
	}
	model := a.Model
	if !strings.Contains(model, string(filepath.Separator)) {
		model = filepath.Join(config.GlobalConfig.Guide.PiperVoices, model)
	}
	if !strings.HasSuffix(model, ".onnx") {
		model += ".onnx"
	}
	pace := a.LengthScale
	if p.Pace > 0 {
		pace = p.Pace
	}
	args := append(cmdline[1:], "-m", model, "-f", wav, "--debug")
	num := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	if pace > 0 {
		args = append(args, "--length-scale", num(pace))
	}
	if a.NoiseScale > 0 {
		args = append(args, "--noise-scale", num(a.NoiseScale))
	}
	if a.NoiseW > 0 {
		args = append(args, "--noise-w-scale", num(a.NoiseW))
	}
	if a.SentenceSilence > 0 {
		args = append(args, "--sentence-silence", num(a.SentenceSilence))
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdline[0], args...)
	cmd.Stdin = strings.NewReader(p.Text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("piper: %v: %s", err, lastLines(stderr.Bytes()))
	}
	return phonemesFrom(stderr.String()), nil
}

var phonemesLine = regexp.MustCompile(`phonemes=(\[\[.*\]\])`)

// phonemesFrom is the phonemes Piper's --debug output gives: "stˈeɪ tˈuːnd …".
func phonemesFrom(debug string) string {
	var out []string
	for _, m := range phonemesLine.FindAllStringSubmatch(debug, -1) {
		var sentences [][]string
		if json.Unmarshal([]byte(strings.ReplaceAll(m[1], "'", `"`)), &sentences) != nil {
			continue
		}
		for _, s := range sentences {
			out = append(out, strings.Join(s, ""))
		}
	}
	return strings.Join(out, " ")
}

// Phonemes is how Piper spells a line in an announcer's voice (with or without the fixes).
func (v *Voices) Phonemes(ctx context.Context, a Announcer, line string, fixes bool) (string, error) {
	if !v.Enabled() {
		return "", errors.New("Piper isn't set up (guide.piperCommand)")
	}
	tmp, err := os.MkdirTemp(media.TempDir(), "weirdstreamer-voice-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	var out []string
	for i, p := range a.pieces(line, fixes) {
		ph, err := runPiper(ctx, a, p, filepath.Join(tmp, strconv.Itoa(i)+".wav"))
		if err != nil {
			return "", err
		}
		out = append(out, ph)
	}
	return strings.Join(out, " | "), nil
}

func probeLength(path string) (time.Duration, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, err
	}
	secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * float64(time.Second)), nil
}

func lastLines(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return strings.Join(lines, " / ")
}

// AddFix saves a fix for an announcer in guide-voices.yml, keeping the file's comments: after its
// other fixes (one with the same words is replaced).
func (v *Voices) AddFix(announcer string, fix Fix) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, err := v.load(); err != nil {
		return err
	}
	path := voicesPath()
	b, err := fileio.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 {
		return errors.New("the voices file is empty")
	}
	announcers := mapValue(doc.Content[0], "announcers")
	if announcers == nil || announcers.Kind != yaml.SequenceNode {
		return errors.New("the voices file has no announcers")
	}
	var target *yaml.Node
	for _, a := range announcers.Content {
		if n := mapValue(a, "name"); n != nil && strings.EqualFold(n.Value, announcer) {
			target = a
		}
	}
	if target == nil {
		return fmt.Errorf("no announcer %q", announcer)
	}
	fixes := mapValue(target, "fixes")
	if fixes == nil {
		target.Content = append(target.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "fixes"},
			&yaml.Node{Kind: yaml.SequenceNode})
		fixes = target.Content[len(target.Content)-1]
	}
	item := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k, val string) {
		item.Content = append(item.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Value: val})
	}
	add("match", fix.Match)
	add("say", fix.Say)
	if fix.Pace > 0 {
		add("pace", strconv.FormatFloat(fix.Pace, 'f', -1, 64))
	}
	replaced := false
	for i, f := range fixes.Content {
		if m := mapValue(f, "match"); m != nil && strings.EqualFold(strings.TrimSpace(m.Value), strings.TrimSpace(fix.Match)) {
			fixes.Content[i], replaced = item, true
		}
	}
	if !replaced {
		fixes.Content = append(fixes.Content, item)
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if _, err := parseVoices(out.Bytes()); err != nil {
		return err
	}
	if err := writeAtomic(path, out.Bytes(), 0o600); err != nil {
		return err
	}
	v.file = nil // read again
	return nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// SpokenTime is a time as an announcer says it: "9", "9:30".
func SpokenTime(t time.Time) string {
	h := t.Hour() % 12
	if h == 0 {
		h = 12
	}
	if t.Minute() == 0 {
		return strconv.Itoa(h)
	}
	return fmt.Sprintf("%d:%02d", h, t.Minute())
}

// VoiceOverLine is what the announcer says over a program's credits: from the programs after it
// on the channel (next, and one within three hours of it), a line picked for the program. ok is
// false when nothing follows it.
func (f *VoicesFile) VoiceOverLine(channel int, programStart time.Time, next *ProgramInfo, later *ProgramInfo, laterStart time.Time) (string, bool) {
	if next == nil || strings.TrimSpace(next.Title) == "" {
		return "", false
	}
	lines := f.Lines
	if later != nil && len(f.LaterLines) > 0 {
		lines = f.LaterLines
	}
	line := lines[hash(strconv.Itoa(channel)+"|"+strconv.FormatInt(programStart.Unix(), 10))%uint64(len(lines))]
	r := strings.NewReplacer("{next}", next.Title, "{channel}", strconv.Itoa(channel))
	line = r.Replace(line)
	if later != nil {
		line = strings.NewReplacer("{later}", later.Title, "{later_time}", SpokenTime(laterStart)).Replace(line)
	}
	return line, true
}

// writeAtomic writes a file whole: to a temporary file beside it, then renamed into place.
func writeAtomic(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".weirdstreamer-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := fileio.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
