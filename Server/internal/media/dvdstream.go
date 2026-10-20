package media

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// DiscOptions picks what to play from a disc image; from the stream URL's query.
type DiscOptions struct {
	Title    int // 0: the main title
	Angle    int // from 1
	Audio    int // audio track index, from 0
	Subtitle int // subtitle track index burned into the picture; -1 for none
}

// ParseDiscOptions reads title, angle, audio and subtitle from a stream request's query.
func ParseDiscOptions(r *http.Request) DiscOptions {
	q := r.URL.Query()
	num := func(key string, def int) int {
		if n, err := strconv.Atoi(q.Get(key)); err == nil {
			return n
		}
		return def
	}
	return DiscOptions{Title: num("title", 0), Angle: num("angle", 1), Audio: num("audio", 0), Subtitle: num("subtitle", -1)}
}

// discBaseBitrate is what percentage bitrates ("80%") are taken of: about a DVD's video bitrate.
const discBaseBitrate = 5_000_000

// serveDisc transcodes a title of a DVD image: its PGC chain joined into one stream, from
// startTime seconds, with the chosen angle and audio track and subtitles burned in.
func (s *Streamer) serveDisc(w http.ResponseWriter, r *http.Request, filePath, startTime, bitrate, resolution string, sess *StreamSession) error {
	args, desc, err := DiscTranscodeArgs(filePath, ParseDiscOptions(r), startTime, bitrate, resolution)
	if err != nil {
		return err
	}
	sess.StartTranscode(desc)
	fmt.Printf("Starting DVD transcode (%s) at %ss: %s\n", desc.Reason, startTime, filePath)
	return s.runTranscode(w, r, filePath, args, sess)
}

// DiscTranscodeArgs builds the ffmpeg arguments for serveDisc, and their description.
func DiscTranscodeArgs(filePath string, opt DiscOptions, startTime, bitrate, resolution string) ([]string, TranscodeInfo, error) {
	info, err := LoadDisc(filePath)
	if err != nil {
		return nil, TranscodeInfo{}, err
	}
	t := info.Title(opt.Title)
	if t == nil {
		return nil, TranscodeInfo{}, fmt.Errorf("no titles on %s", filePath)
	}
	start, _ := strconv.ParseFloat(startTime, 64)
	start = min(max(start, 0), max(t.Duration-1, 0))
	audio := -1
	if len(t.Audio) > 0 {
		audio = min(max(opt.Audio, 0), len(t.Audio)-1)
	}
	subtitle := opt.Subtitle
	if subtitle >= len(t.Subtitles) {
		subtitle = -1
	}
	angle := min(max(opt.Angle, 1), max(t.Angles, 1))

	first, offset := t.SegmentAt(start)
	args := []string{}
	var filters, concatInputs []string
	n := 0
	for i := first; i < len(t.Segments); i++ {
		seek := 0.0
		if i == first {
			seek = offset
		}
		input, trim := t.InputArgs(filePath, i, angle, seek)
		args = append(args, input...)

		video := fmt.Sprintf("[%d:v:0]", n)
		if subtitle >= 0 {
			video += fmt.Sprintf("[%d:s:%d]overlay", n, subtitle)
		} else {
			video += "null"
		}
		if trim > 0 {
			video += fmt.Sprintf(",trim=start=%.3f,setpts=PTS-STARTPTS", trim)
		}
		// Deinterlace where needed, and turn DVD's non-square pixels into square ones.
		video += ",yadif=deint=interlaced,scale='trunc(iw*sar/2)*2':ih,setsar=1"
		if resolution != "" {
			video += ",scale=-2:" + resolution
		}
		filters = append(filters, fmt.Sprintf("%s[v%d]", video, n))
		concatInputs = append(concatInputs, fmt.Sprintf("[v%d]", n))
		if audio >= 0 {
			a := fmt.Sprintf("[%d:a:%d]", n, audio)
			if trim > 0 {
				a += fmt.Sprintf("atrim=start=%.3f,asetpts=PTS-STARTPTS,", trim)
			}
			a += "aresample=48000:async=1,aformat=sample_fmts=fltp:channel_layouts=stereo"
			filters = append(filters, fmt.Sprintf("%s[a%d]", a, n))
			concatInputs = append(concatInputs, fmt.Sprintf("[a%d]", n))
		}
		n++
	}
	audioOut := 0
	if audio >= 0 {
		audioOut = 1
	}
	filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=1:a=%d[vout]%s", strings.Join(concatInputs, ""), n, audioOut,
		map[bool]string{true: "[aout]", false: ""}[audio >= 0]))
	args = append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[vout]")
	if audio >= 0 {
		args = append(args, "-map", "[aout]")
	}

	videoDesc := "H.264 (libx264, preset ultrafast"
	args = append(args, "-c:v", "libx264", "-preset", "ultrafast")
	switch {
	case strings.HasSuffix(bitrate, "%"):
		if pct, err := strconv.ParseFloat(strings.TrimSuffix(bitrate, "%"), 64); err == nil && pct > 0 {
			b := int64(discBaseBitrate * pct / 100)
			args = append(args, "-b:v", strconv.FormatInt(b, 10), "-maxrate", strconv.FormatInt(b, 10),
				"-bufsize", strconv.FormatInt(2*b, 10))
			videoDesc += fmt.Sprintf(", %d kb/s", b/1000)
		}
	case bitrate != "":
		args = append(args, "-b:v", bitrate)
		videoDesc += ", " + bitrate
	default:
		args = append(args, "-crf", "23")
		videoDesc += ", CRF 23"
	}
	if resolution != "" {
		videoDesc += ", " + resolution + "p"
	}
	args = append(args, "-c:a", "aac", "-b:a", "192k",
		"-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")

	reason := fmt.Sprintf("DVD disc image: title %d", t.Number)
	if t.Number == info.MainTitle {
		reason += " (main feature)"
	}
	if t.Angles > 1 {
		reason += fmt.Sprintf(", angle %d of %d", angle, t.Angles)
	}
	if len(t.Segments) > 1 {
		reason += fmt.Sprintf(", %d chained parts", len(t.Segments))
	}
	desc := describeTranscode(args, startTime, bitrate, resolution, reason)
	desc.Video = videoDesc + ", deinterlaced)"
	desc.Audio = "AAC 192 kb/s stereo"
	if audio >= 0 {
		desc.Audio += fmt.Sprintf(" (track %d: %s)", audio+1, describeTrack(t.Audio[audio]))
	}
	if subtitle >= 0 {
		desc.Video += fmt.Sprintf(" + subtitles track %d (%s) burned in", subtitle+1, describeTrack(t.Subtitles[subtitle]))
	}
	desc.Start = start
	return args, desc, nil
}

func describeTrack(t DiscTrack) string {
	parts := []string{}
	if t.Language != "" {
		parts = append(parts, t.Language)
	}
	parts = append(parts, t.Codec)
	if t.Channels > 0 {
		parts = append(parts, fmt.Sprintf("%d ch", t.Channels))
	}
	return strings.Join(parts, ", ")
}
