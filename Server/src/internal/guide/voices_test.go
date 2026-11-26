package guide

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

func TestFixesMakePieces(t *testing.T) {
	a := Announcer{Fixes: []Fix{
		{Match: "next on Channel", Say: "next on [[ tʃˈæːnəl ]]"},
		{Match: "Spartacus", Say: "Spartacus", Pace: 1.3},
	}}
	got := a.pieces("Stay tuned for Spartacus, next on channel 15.", true)
	if len(got) != 3 || got[0].Text != "Stay tuned for " || got[1].Pace != 1.3 ||
		got[2].Text != ", next on [[ tʃˈæːnəl ]] 15." {
		t.Fatalf("%+v", got)
	}
	if plain := a.pieces("Only on Channel 15.", true); len(plain) != 1 || plain[0].Text != "Only on Channel 15." {
		t.Fatalf("a fix for a phrase changed another: %+v", plain)
	}
	if a.Key("x", true) == a.Key("x", false) {
		t.Fatal("with and without fixes share a key")
	}
}

func TestVoiceOverLine(t *testing.T) {
	f, err := parseVoices(defaultVoices)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 26, 20, 0, 0, 0, time.Local)
	line, ok := f.VoiceOverLine(15, at, &ProgramInfo{Title: "Spartacus"}, &ProgramInfo{Title: "Annie"}, at.Add(90*time.Minute))
	if !ok || !strings.Contains(line, "Spartacus") || !strings.Contains(line, "Channel 15") || !strings.Contains(line, "9:30, Annie") {
		t.Fatalf("%q", line)
	}
	if _, ok := f.VoiceOverLine(15, at, nil, nil, time.Time{}); ok {
		t.Fatal("a line with nothing next")
	}
	if f.AnnouncerFor(15).Name == f.AnnouncerFor(16).Name {
		t.Fatal("neighbouring channels share an announcer")
	}
}

// With PIPER_TEST=<the piper command>|<its voices folder>, a real voice-over.
func TestMakeVoiceOver(t *testing.T) {
	spec := os.Getenv("PIPER_TEST")
	if spec == "" {
		t.Skip("PIPER_TEST not set")
	}
	cmd, voices, _ := strings.Cut(spec, "|")
	dir := t.TempDir()
	config.GlobalConfig = &config.Config{}
	config.GlobalConfig.Guide.PiperCommand, config.GlobalConfig.Guide.PiperVoices = cmd, voices
	config.GlobalConfig.Guide.VoiceOverPath = dir
	config.GlobalConfig.Guide.VoicesFile = filepath.Join(dir, "guide-voices.yml")
	v := NewVoices()
	f, err := v.File()
	if err != nil {
		t.Fatal(err)
	}
	a := f.Announcers[0]
	line := "Stay tuned for Spartacus, next on Channel 15. And at 9, Annie."
	vo, err := v.Make(context.Background(), a, line, true)
	if err != nil {
		t.Fatal(err)
	}
	if vo.Duration < 2*time.Second || vo.Duration > 15*time.Second {
		t.Fatalf("length %v", vo.Duration)
	}
	if again, err := v.Make(context.Background(), a, line, true); err != nil || again.Key != vo.Key {
		t.Fatal("not taken from the cache")
	}
	ph, err := v.Phonemes(context.Background(), a, line, true)
	if err != nil || !strings.Contains(ph, "tʃˈæːnəl") {
		t.Fatalf("phonemes %q %v", ph, err)
	}
	if err := v.AddFix(a.Name, Fix{Match: "Annie", Say: "[[ ˈæni ]]"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(config.GlobalConfig.Guide.VoicesFile)
	if !strings.Contains(string(b), "Annie") || !strings.Contains(string(b), "# The Guide's voice-overs") {
		t.Fatalf("fix not saved, or the comments lost:\n%s", b)
	}
	f, _ = v.File()
	if len(f.Announcers[0].Fixes) != 2 {
		t.Fatalf("fixes %+v", f.Announcers[0].Fixes)
	}
	src, _ := VoiceOverFile(vo.Key)
	data, _ := os.ReadFile(src)
	if out := os.Getenv("PIPER_TEST_OUT"); out != "" { // to listen to
		os.WriteFile(out, data, 0o644)
	}
}
