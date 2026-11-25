package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestThemeFile(t *testing.T) {
	dir := t.TempDir()
	if ThemeFile(dir) != "" {
		t.Fatal("a theme in an empty folder")
	}
	for _, name := range []string{"theme.mp3", "Theme.FLAC", "themes.wav", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644)
	}
	if got := filepath.Base(ThemeFile(dir)); got != "Theme.FLAC" {
		t.Errorf("picked %s, want the FLAC (any case) over the MP3", got)
	}
}

func TestThemes(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	root := t.TempDir()
	saved := config.GlobalConfig
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{ThemePath: filepath.Join(root, "themes")}}
	t.Cleanup(func() { config.GlobalConfig = saved })
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("ffmpeg", append([]string{"-v", "error", "-y"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("ffmpeg: %v %s", err, out)
		}
	}

	// A file every player takes is sent as it is; an Ogg one is converted once
	mp3 := filepath.Join(root, "theme.mp3")
	run("-f", "lavfi", "-i", "sine=d=5", mp3)
	if got, err := ServableTheme(ctx, mp3); err != nil || got != mp3 {
		t.Errorf("MP3: %s %v", got, err)
	}
	ogg := filepath.Join(root, "theme.ogg")
	run("-f", "lavfi", "-i", "sine=d=5", "-c:a", "libvorbis", ogg)
	converted, err := ServableTheme(ctx, ogg)
	if err != nil || filepath.Ext(converted) != ".m4a" || filepath.Dir(filepath.Dir(converted)) != filepath.Join(root, "themes") {
		t.Fatalf("Ogg: %s %v", converted, err)
	}

	// A movie whose opening and credits share a 30-second piece of music: that's its theme. Its
	// sound is melodies (random notes; Chromaprint describes pitch, so tones or noise would all look
	// alike): the theme at 60 s and again when the credits start at 240 s, other melodies between.
	theme := melody(t, root, "piece", 30, 7)
	movie := filepath.Join(root, "movie.mkv")
	run("-i", melody(t, root, "a", 60, 1), "-i", theme, "-i", melody(t, root, "b", 150, 2),
		"-i", theme, "-i", melody(t, root, "c", 30, 3),
		"-filter_complex", "[0][1][2][3][4]concat=n=5:v=0:a=1[a]", "-map", "[a]", "-c:a", "aac", movie)
	credits := &Segment{Start: 240, End: 300}
	span := MovieThemeSpan(ctx, movie, credits, 300)
	if span == nil || math.Abs(span.Start-240) > 2 || math.Abs(span.End-270) > 3 {
		t.Fatalf("the shared theme: %+v, want about 240–270", span)
	}
	cut, err := CutTheme(ctx, movie, span.Start, span.End-span.Start)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := CutTheme(ctx, movie, span.Start, span.End-span.Start); again != cut {
		t.Error("the same cut made twice")
	}
	if d := probeDuration(t, cut); math.Abs(d-(span.End-span.Start)) > 1 {
		t.Errorf("cut is %.1f s long", d)
	}

	// Nothing shared: the start of the credits
	plain := filepath.Join(root, "plain.mkv")
	run("-i", melody(t, root, "plain", 200, 4), "-c:a", "aac", plain)
	if span := MovieThemeSpan(ctx, plain, &Segment{Start: 150, End: 200}, 200); span == nil || span.Start != 150 || span.End != 200 {
		t.Errorf("credits fallback: %+v", span)
	}
}

func probeDuration(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var d float64
	if _, err := fmt.Sscan(string(out), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// melody writes seconds of random notes (a quarter second each, seeded) as a WAV file.
func melody(t *testing.T, dir, name string, seconds int, seed int64) string {
	t.Helper()
	const rate = 22050
	r := rand.New(rand.NewSource(seed))
	n := seconds * rate
	pcm := make([]byte, 44+2*n)
	copy(pcm, "RIFF")
	binary.LittleEndian.PutUint32(pcm[4:], uint32(36+2*n))
	copy(pcm[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(pcm[16:], 16)
	binary.LittleEndian.PutUint16(pcm[20:], 1)
	binary.LittleEndian.PutUint16(pcm[22:], 1)
	binary.LittleEndian.PutUint32(pcm[24:], rate)
	binary.LittleEndian.PutUint32(pcm[28:], rate*2)
	binary.LittleEndian.PutUint16(pcm[32:], 2)
	binary.LittleEndian.PutUint16(pcm[34:], 16)
	copy(pcm[36:], "data")
	binary.LittleEndian.PutUint32(pcm[40:], uint32(2*n))
	freq := 0.0
	for i := 0; i < n; i++ {
		if i%(rate/4) == 0 {
			freq = 220 * math.Pow(2, float64(r.Intn(24))/12)
		}
		v := 0.5*math.Sin(2*math.Pi*freq*float64(i)/rate) + 0.25*math.Sin(4*math.Pi*freq*float64(i)/rate)
		binary.LittleEndian.PutUint16(pcm[44+2*i:], uint16(int16(v*12000)))
	}
	path := filepath.Join(dir, name+".wav")
	if err := os.WriteFile(path, pcm, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
