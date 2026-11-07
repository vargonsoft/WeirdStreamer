package media

import (
	"math"
	"math/rand"
	"testing"
)

func randomPoints(r *rand.Rand, n int) []uint32 {
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.Uint32()
	}
	return out
}

// noisy flips a few bits of each point, as a re-encode of the same audio would.
func noisy(r *rand.Rand, points []uint32) []uint32 {
	out := make([]uint32, len(points))
	for i, p := range points {
		for k := 0; k < r.Intn(4); k++ {
			p ^= 1 << r.Intn(32)
		}
		out[i] = p
	}
	return out
}

func TestSharedSegmentFindsTheCommonTheme(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	theme := randomPoints(r, 300) // about 37 seconds
	// Episode A: the theme 100 points in; episode B: 250 points in, slightly different audio.
	a := append(append(randomPoints(r, 100), theme...), randomPoints(r, 400)...)
	b := append(append(randomPoints(r, 250), noisy(r, theme)...), randomPoints(r, 300)...)

	inA, inB := SharedSegment(a, b, 0, 10, 15, 150)
	if inA == nil || inB == nil {
		t.Fatal("no shared segment found")
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 1.0 }
	if !near(inA.Start, 100*fingerprintStep) || !near(inA.End, 400*fingerprintStep) {
		t.Errorf("in A: %.1f–%.1f, want %.1f–%.1f", inA.Start, inA.End, 100*fingerprintStep, 400*fingerprintStep)
	}
	if !near(inB.Start, 10+250*fingerprintStep) || !near(inB.End, 10+550*fingerprintStep) {
		t.Errorf("in B: %.1f–%.1f, want %.1f–%.1f", inB.Start, inB.End, 10+250*fingerprintStep, 10+550*fingerprintStep)
	}
}

func TestSharedSegmentNeedsEnough(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	theme := randomPoints(r, 60) // about 7 seconds: too short for an intro
	a := append(append(randomPoints(r, 100), theme...), randomPoints(r, 100)...)
	b := append(append(randomPoints(r, 50), theme...), randomPoints(r, 100)...)
	if inA, _ := SharedSegment(a, b, 0, 0, 15, 150); inA != nil {
		t.Errorf("found %v in unrelated audio", inA)
	}
	if inA, _ := SharedSegment(randomPoints(r, 500), randomPoints(r, 500), 0, 0, 15, 150); inA != nil {
		t.Errorf("found %v in random audio", inA)
	}
}

func TestChapterNames(t *testing.T) {
	for _, s := range []string{"Intro", "Opening", "OP", "Opening Credits", "title sequence"} {
		if !introChapter.MatchString(s) {
			t.Errorf("%q should be an intro", s)
		}
	}
	for _, s := range []string{"Credits", "End Credits", "ED", "Ending", "Outro"} {
		if !creditsChapter.MatchString(s) {
			t.Errorf("%q should be credits", s)
		}
	}
	for _, s := range []string{"Chapter 1", "The Opening Night", "Credits Roll Over The City Skyline At Dusk"} {
		if introChapter.MatchString(s) || creditsChapter.MatchString(s) {
			t.Errorf("%q should be neither", s)
		}
	}
}

// Suits S07E02's ending: about 16 seconds of credits on black, a company card, black to the end.
func TestBlackCreditsShortRun(t *testing.T) {
	out := `[blackdetect @ 0x1] black_start:148.5 black_end:149.5 black_duration:1
[blackdetect @ 0x1] black_start:276 black_end:292.5 black_duration:16.5
[blackdetect @ 0x1] black_start:294 black_end:294.5 black_duration:0.5
[blackdetect @ 0x1] black_start:298 black_end:299.5 black_duration:1.5`
	got := blackCredits(out, 2196, 2496)
	if got == nil || got.Start != 2472 || got.End != 2496 {
		t.Fatalf("got %+v, want credits 2472–2496", got)
	}
	// Suits S06E04: a scene fades out, one more shot, then the credits.
	if got := blackCredits(`black_start:39 black_end:41.5
black_start:46.5 black_end:72
black_start:79 black_end:79.5`, 2441, 2521); got == nil || got.Start != 2487.5 {
		t.Fatalf("got %+v, want credits from 2487.5", got)
	}
	// A short fade to black near the end isn't credits.
	if got := blackCredits(`black_start:290 black_end:299.5`, 2196, 2496); got != nil {
		t.Fatalf("a 9.5-second fade became credits: %+v", got)
	}
	// Nor is black that ends long before the end.
	if got := blackCredits(`black_start:100 black_end:160`, 2196, 2496); got != nil {
		t.Fatalf("black far from the end became credits: %+v", got)
	}
}
