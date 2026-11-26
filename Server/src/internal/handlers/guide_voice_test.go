package handlers

import "testing"

func TestVoiceOverTiming(t *testing.T) {
	// credits known: a second in, squeezed from just before
	if at, sq, ok := voiceOverTiming(1800, 1740, 6); !ok || at != 1741 || sq != 1740.5 {
		t.Fatalf("credits: %v %v %v", at, sq, ok)
	}
	// short credits: earlier, so it still ends 5 seconds before the end
	if at, _, ok := voiceOverTiming(1800, 1792, 6); !ok || at != 1789 {
		t.Fatalf("short credits: %v %v", at, ok)
	}
	// credits unknown: ending 5 seconds before the end, not squeezed
	if at, sq, ok := voiceOverTiming(1800, -1, 6); !ok || at != 1789 || sq != 0 {
		t.Fatalf("no credits: %v %v %v", at, sq, ok)
	}
	// too short a program for it: none
	if _, _, ok := voiceOverTiming(8, -1, 6); ok {
		t.Fatal("an 8-second program got one")
	}
}
