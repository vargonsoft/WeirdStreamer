package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/bits"
	"regexp"
	"strconv"
	"strings"
)

// Intros and credits, for the players' Skip buttons, found three ways: chapters named for them,
// a season's shared audio (Chromaprint fingerprints of each episode's opening and ending,
// compared episode to episode, as Jellyfin's Intro Skipper does), and, for credits, the picture
// going dark near the end (ffmpeg's blackdetect, for rolling credits on black).

// Segment is a span of a video, in seconds.
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Length is the segment's duration.
func (s Segment) Length() float64 { return s.End - s.Start }

var (
	introChapter   = regexp.MustCompile(`(?i)^\s*(intro|introduction|opening( credits| titles| theme)?|op|title sequence|theme song)\s*$`)
	creditsChapter = regexp.MustCompile(`(?i)^\s*(credits|end credits|closing credits|ending( credits| theme)?|ed|outro|end titles)\s*$`)
)

// ChapterSegments returns a file's chapters named like an intro or credits (nil for none).
func ChapterSegments(ctx context.Context, path string) (intro, credits *Segment, err error) {
	out, err := backgroundCommand(ctx, "ffprobe", "-v", "error", "-print_format", "json", "-show_chapters", path).Output()
	if err != nil {
		return nil, nil, err
	}
	var probe struct {
		Chapters []struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, nil, err
	}
	for _, c := range probe.Chapters {
		start, err1 := strconv.ParseFloat(c.Start, 64)
		end, err2 := strconv.ParseFloat(c.End, 64)
		if err1 != nil || err2 != nil || end <= start {
			continue
		}
		title := c.Tags["title"]
		seg := &Segment{Start: start, End: end}
		switch {
		case intro == nil && introChapter.MatchString(title):
			intro = seg
		case credits == nil && creditsChapter.MatchString(title):
			credits = seg
		}
	}
	return intro, credits, nil
}

// fingerprintStep is how much audio each Chromaprint point stands for, at its default settings.
const fingerprintStep = 0.1238

// Fingerprint returns the Chromaprint points of a file's first audio track over [start, start+length).
func Fingerprint(ctx context.Context, path string, start, length float64) ([]uint32, error) {
	cmd := backgroundCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
		"-ss", fmt.Sprintf("%.3f", start), "-t", fmt.Sprintf("%.3f", length), "-i", path,
		"-map", "0:a:0", "-ac", "1", "-f", "chromaprint", "-fp_format", "raw", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fingerprint %s: %w (%s)", path, err, strings.TrimSpace(stderr.String()))
	}
	points := make([]uint32, len(out)/4)
	for i := range points {
		points[i] = binary.LittleEndian.Uint32(out[i*4:])
	}
	return points, nil
}

// Matching: two points match when at most this many of their 32 bits differ, and a run of
// matches may skip over this many unmatched points (a sound effect over the theme, say).
const (
	maxBitDiff  = 6
	maxGapPoint = 28 // about 3.5 seconds
)

// commonRun finds the longest stretch two fingerprints share, at any alignment: the matching
// points' indices in a and in b, as half-open ranges, and ok when it's at least minPoints long.
// Every alignment is tried (the same audio a fraction of a point apart rarely gives equal points,
// so exact matches can't pick them): a few million comparisons for ten minutes of each.
func commonRun(a, b []uint32, minPoints int) (aStart, aEnd, bStart, bEnd int, ok bool) {
	best := 0
	for shift := -(len(a) - minPoints); shift <= len(b)-minPoints; shift++ {
		// Over the overlap of a[i] and b[i+shift], the longest run of matches with short gaps.
		from := max(0, -shift)
		to := min(len(a), len(b)-shift)
		if to-from <= best {
			continue
		}
		runStart, lastMatch := -1, -1
		for i := from; i < to; i++ {
			if bits.OnesCount32(a[i]^b[i+shift]) > maxBitDiff {
				continue
			}
			if runStart < 0 || i-lastMatch > maxGapPoint {
				runStart = i
			}
			lastMatch = i
			if n := lastMatch + 1 - runStart; n > best {
				best = n
				aStart, aEnd = runStart, lastMatch+1
				bStart, bEnd = runStart+shift, lastMatch+1+shift
			}
		}
	}
	return aStart, aEnd, bStart, bEnd, best >= minPoints
}

// SharedSegment compares the same stretch of two episodes (fingerprints taken from fromA and
// fromB seconds into each) and returns the audio they share, in each file's own time, when it
// lasts between minLen and maxLen seconds.
func SharedSegment(a, b []uint32, fromA, fromB, minLen, maxLen float64) (inA, inB *Segment) {
	as, ae, bs, be, ok := commonRun(a, b, int(minLen/fingerprintStep))
	if !ok {
		return nil, nil
	}
	length := float64(ae-as) * fingerprintStep
	if length > maxLen {
		return nil, nil
	}
	return &Segment{Start: fromA + float64(as)*fingerprintStep, End: fromA + float64(ae)*fingerprintStep},
		&Segment{Start: fromB + float64(bs)*fingerprintStep, End: fromB + float64(be)*fingerprintStep}
}

var blackLine = regexp.MustCompile(`black_start:([\d.]+) black_end:([\d.]+)`)

// Credits on black: stretches at least this long, joined across gaps (a title card, a logo) of
// up to blackGap seconds, and reaching to within blackEnd seconds of the end. A stretch shorter
// than blackLead seconds (a scene fading out) doesn't start them, only joins them.
const (
	blackMinCredits = 12.0
	blackGap        = 12.0
	blackEnd        = 30.0
	blackLead       = 4.0
)

// BlackCredits looks for credits on black in the last `span` seconds of a file: the start of the
// stretch, reaching to near the end, where the picture is nearly all black (white text on black
// still is). Two frames a second are looked at: a still black screen has few keyframes, so
// keyframes alone miss most of it.
func BlackCredits(ctx context.Context, path string, duration, span float64) (*Segment, error) {
	from := max(0, duration-span)
	cmd := backgroundCommand(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-skip_frame", "noref",
		"-ss", fmt.Sprintf("%.3f", from), "-i", path, "-an", "-sn", "-map", "0:v:0",
		"-vf", "fps=2,scale=320:-2,blackdetect=d=0:pic_th=0.90:pix_th=0.03", "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("blackdetect %s: %w", path, err)
	}
	return blackCredits(stderr.String(), from, duration), nil
}

// blackCredits reads blackdetect's output (times from `from` seconds into the file).
func blackCredits(output string, from, duration float64) *Segment {
	type stretch struct{ start, end float64 }
	var spans []stretch
	for _, m := range blackLine.FindAllStringSubmatch(output, -1) {
		s, _ := strconv.ParseFloat(m[1], 64)
		e, _ := strconv.ParseFloat(m[2], 64)
		s, e = from+s, from+e
		if n := len(spans); n > 0 && s-spans[n-1].end <= blackGap && spans[n-1].end-spans[n-1].start >= blackLead {
			spans[n-1].end = e
		} else {
			spans = append(spans, stretch{s, e})
		}
	}
	for _, s := range spans {
		if s.end-s.start >= blackMinCredits && duration-s.end <= blackEnd {
			return &Segment{Start: s.start, End: duration}
		}
	}
	return nil
}
