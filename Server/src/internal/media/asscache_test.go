package media

import (
	"strings"
	"testing"
)

func TestAssWindow(t *testing.T) {
	script := `[Script Info]
PlayResX: 1280

[V4+ Styles]
Format: Name, Fontname
Style: Default,Arial

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.02,0:02:00.02,Default,,0,0,0,,Whole video
Dialogue: 0,0:00:10.00,0:00:12.00,Default,,0,0,0,,Before
Dialogue: 0,0:00:28.00,0:00:31.50,Default,,0,0,0,,Across the start, with, commas
Dialogue: 0,0:00:40.00,0:00:43.00,Default,,0,0,0,,Inside
Dialogue: 0,0:00:50.00,0:00:52.00,Default,,0,0,0,,At the end
Dialogue: 0,0:01:30.00,0:01:32.00,Default,,0,0,0,,After
`
	got := string(AssWindow([]byte(script), 30, 20))
	for _, want := range []string{"[Script Info]", "Style: Default,Arial", "[Events]",
		"Format: Layer, Start, End", "Whole video", "Across the start, with, commas", "Inside"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, not := range []string{"Before", "At the end", "After"} {
		if strings.Contains(got, not) {
			t.Errorf("has %q, outside the window", not)
		}
	}
	if all := string(AssWindow([]byte(script), 0, 0)); strings.Count(all, "Dialogue:") != 6 {
		t.Errorf("duration 0: want every line, got:\n%s", all)
	}
	if v, ok := assTime("1:02:03.45"); !ok || v < 3723.44 || v > 3723.46 {
		t.Errorf("assTime: %v %v", v, ok)
	}
}
