package media

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
)

// Codecs describes a file's container and streams, for players that decide for themselves what
// they can play (the web UI asks its browser).
type Codecs struct {
	Container string          `json:"container"` // ffprobe's format name, such as "matroska,webm"
	Duration  float64         `json:"duration"`  // seconds
	Video     *VideoCodec     `json:"video,omitempty"`
	Audio     []AudioCodec    `json:"audio"`
	Subtitles []SubtitleCodec `json:"subtitles"`
}

type VideoCodec struct {
	Codec   string `json:"codec"` // ffprobe's codec name: h264, hevc, mpeg2video…
	Profile string `json:"profile,omitempty"`
	Level   int    `json:"level,omitempty"` // as ffprobe gives it: 51 for H.264 5.1, 153 for HEVC 5.1
	PixFmt  string `json:"pix_fmt,omitempty"`
}

type AudioCodec struct {
	Index    int    `json:"index"` // among the file's audio tracks: the stream's audio= param
	Codec    string `json:"codec"` // aac, ac3, eac3, dts, truehd, opus…
	Channels int    `json:"channels"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default"`
}

type SubtitleCodec struct {
	Index    int    `json:"index"` // among the file's subtitle tracks
	Codec    string `json:"codec"` // subrip, ass, mov_text, hdmv_pgs_subtitle, dvd_subtitle…
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default"`
	Forced   bool   `json:"forced"`
	// Text tracks are sent to players as WebVTT (ServeSubtitles); picture ones (PGS, DVD, DVB)
	// can only be burned into a transcode.
	Text bool `json:"text"`
	// Subtitle files outside the video, listed after its own tracks: "file" (beside it) or
	// "opensubtitles" (downloaded). Always text.
	External        string `json:"external,omitempty"`
	HearingImpaired bool   `json:"hearing_impaired,omitempty"`
	// Key names the track whatever its place in the list: "stream:N" for the video's own, "file:…"
	// for a subtitle file. OffsetMs is its timing correction, for every player: positive shows the
	// lines later, negative earlier.
	Key      string `json:"key,omitempty"`
	OffsetMs int    `json:"offset_ms,omitempty"`
}

// IsTextSubtitle reports whether ffmpeg can turn a subtitle codec into WebVTT.
func IsTextSubtitle(codec string) bool {
	switch codec {
	case "subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text", "microdvd", "subviewer",
		"subviewer1", "sami", "realtext", "jacosub", "mpl2", "pjs", "vplayer", "stl":
		return true
	}
	return false
}

// ProbeCodecs reads the codecs of a file's first video stream and of all its audio streams.
func ProbeCodecs(path string) (*Codecs, error) {
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=format_name,duration:stream=codec_type,codec_name,profile,level,pix_fmt,channels:stream_tags=language,title:stream_disposition=default,forced,attached_pic",
		"-of", "json", path).Output()
	if err != nil {
		return nil, err
	}
	var probe struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType   string            `json:"codec_type"`
			CodecName   string            `json:"codec_name"`
			Profile     string            `json:"profile"`
			Level       int               `json:"level"`
			PixFmt      string            `json:"pix_fmt"`
			Channels    int               `json:"channels"`
			Tags        map[string]string `json:"tags"`
			Disposition map[string]int    `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, err
	}
	c := &Codecs{Container: probe.Format.FormatName, Audio: []AudioCodec{}, Subtitles: []SubtitleCodec{}}
	c.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	c.Duration = orPackets(path, c.Duration)
	for _, s := range probe.Streams {
		switch s.CodecType {
		case "video":
			if c.Video == nil && s.Disposition["attached_pic"] == 0 { // not cover art
				c.Video = &VideoCodec{Codec: s.CodecName, Profile: s.Profile, Level: max(s.Level, 0), PixFmt: s.PixFmt}
			}
		case "audio":
			c.Audio = append(c.Audio, AudioCodec{Index: len(c.Audio), Codec: s.CodecName, Channels: s.Channels,
				Language: s.Tags["language"], Title: s.Tags["title"], Default: s.Disposition["default"] == 1})
		case "subtitle":
			c.Subtitles = append(c.Subtitles, SubtitleCodec{Index: len(c.Subtitles), Codec: s.CodecName,
				Language: s.Tags["language"], Title: s.Tags["title"], Default: s.Disposition["default"] == 1,
				Forced: s.Disposition["forced"] == 1, Text: IsTextSubtitle(s.CodecName)})
		}
	}
	return c, nil
}

// DefaultAudio is the audio track a player picks on its own: the first one marked default, or
// else the first one.
func (c *Codecs) DefaultAudio() int {
	for i, a := range c.Audio {
		if a.Default {
			return i
		}
	}
	return 0
}

// BrowserVideo reports whether browsers decode the video as it is: 8-bit 4:2:0 H.264.
func (c *Codecs) BrowserVideo() bool {
	return c.Video != nil && c.Video.Codec == "h264" &&
		(c.Video.PixFmt == "yuv420p" || c.Video.PixFmt == "yuvj420p")
}

// TrackOptions picks a file's tracks for a transcode: Audio is an index among its audio tracks
// (-1: the default one), Subtitle one among its subtitle tracks to burn in (-1: none). Stereo
// downmixes the audio (channels=2): for players whose decoder fails on surround AAC. AudioCodec
// (audiocodec=) is what a player that knows its output wants done with the audio: "copy" keeps
// it, "ac3", "eac3" and "aac" convert it to 5.1 (aac with channels=2: to stereo); "" is the
// stream's own default.
type TrackOptions struct {
	Audio, Subtitle int
	Stereo          bool
	AudioCodec      string
}

// StereoRequested reports whether a stream request asks for stereo audio (channels=2).
func StereoRequested(r *http.Request) bool { return r.URL.Query().Get("channels") == "2" }

// ParseTrackOptions reads the audio=, subtitle= and channels= params of a stream request.
func ParseTrackOptions(r *http.Request) TrackOptions {
	q := r.URL.Query()
	num := func(key string) int {
		if n, err := strconv.Atoi(q.Get(key)); err == nil && n >= 0 {
			return n
		}
		return -1
	}
	codec := q.Get("audiocodec")
	switch codec {
	case "copy", "ac3", "eac3", "aac":
	default:
		codec = ""
	}
	return TrackOptions{Audio: num("audio"), Subtitle: num("subtitle"), Stereo: StereoRequested(r), AudioCodec: codec}
}

// audioArgs are ffmpeg's audio arguments and what they do, for the Streams page. stereo is the
// stream's default when the player asks for nothing: a copied video's audio is always stereo.
func (t TrackOptions) audioArgs(stereo bool) ([]string, string) {
	switch t.AudioCodec {
	case "copy":
		return []string{"-c:a", "copy"}, "Copied"
	case "ac3", "eac3":
		return []string{"-c:a", t.AudioCodec, "-ac", "6", "-b:a", "640k"},
			strings.ToUpper(t.AudioCodec) + " 640 kb/s 5.1"
	case "aac":
		if !t.Stereo {
			return []string{"-c:a", "aac", "-ac", "6", "-b:a", "384k"}, "AAC 384 kb/s 5.1"
		}
	}
	if stereo || t.Stereo {
		return []string{"-c:a", "aac", "-ac", "2", "-b:a", "192k"}, "AAC 192 kb/s stereo"
	}
	return []string{"-c:a", "aac", "-b:a", "192k"}, "AAC 192 kb/s"
}

// movFlags are the stream's MP4 flags. ffmpeg can only write the header of copied audio, AC-3 and
// E-AC-3 once it has seen their first packets, which delay_moov lets it do.
func (t TrackOptions) movFlags() string {
	switch t.AudioCodec {
	case "copy", "ac3", "eac3":
		return "frag_keyframe+empty_moov+delay_moov"
	}
	return "frag_keyframe+empty_moov"
}

// audioIndex is the audio track to play: the chosen one if the file has it, else the default.
func (t TrackOptions) audioIndex(c *Codecs) int {
	if t.Audio >= 0 && t.Audio < len(c.Audio) {
		return t.Audio
	}
	return c.DefaultAudio()
}

// mapArgs maps the first video stream (with the chosen subtitles burned in, then the video
// filter vf, if any) and the chosen audio track.
func (t TrackOptions) mapArgs(filePath, vf string) []string {
	c, err := ProbeCodecs(filePath)
	if err != nil {
		c = &Codecs{}
	}
	var args []string
	if t.Subtitle >= 0 && t.Subtitle < len(c.Subtitles) {
		// Subtitle pictures are drawn for their own canvas (1080p on 4K discs): scaled to the video.
		graph := fmt.Sprintf("[0:v:0]split[v][ref];[0:s:%d][ref]scale=rw:rh[s];[v][s]overlay", t.Subtitle)
		if vf != "" {
			graph += "," + vf
		}
		args = append(args, "-filter_complex", graph+"[vout]", "-map", "[vout]")
	} else {
		args = append(args, "-map", "0:v:0")
		if vf != "" {
			args = append(args, "-vf", vf)
		}
	}
	if len(c.Audio) > 0 {
		args = append(args, "-map", "0:a:"+strconv.Itoa(t.audioIndex(c)))
	}
	return args
}

// SeekPoint is where a stream that copies the video starts when asked to start at start (seconds,
// as given to ffmpeg's -ss): the keyframe ffmpeg's seek lands on, up to several seconds earlier,
// as the file's index (an MKV's cues) goes. ffmpeg itself finds it, seeking as the stream does
// (ffprobe's seek differs near keyframes), and prints that first frame's time.
func SeekPoint(filePath, start string) (float64, error) {
	out, err := exec.Command("ffmpeg", "-v", "error", "-noaccurate_seek", "-ss", start, "-i", filePath,
		"-map", "0:v:0", "-c", "copy", "-copyts", "-start_at_zero", "-frames:v", "1", "-f", "framecrc", "-").Output()
	if err != nil {
		return 0, err
	}
	return parseFrameTime(string(out))
}

// parseFrameTime reads the first frame's time from ffmpeg's framecrc output: a "#tb 0: 1/1000"
// time base line, then "stream, dts, pts, duration, size, crc" lines.
func parseFrameTime(out string) (float64, error) {
	num, den := 0.0, 0.0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "#tb 0:") {
			fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(line, "#tb 0:")), "%g/%g", &num, &den)
			continue
		}
		fields := strings.Split(line, ",")
		if strings.HasPrefix(line, "#") || len(fields) < 3 || den == 0 {
			continue
		}
		pts, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return 0, err
		}
		return pts * num / den, nil
	}
	return 0, fmt.Errorf("no frame in ffmpeg's output")
}

// transcodeAudioAndServe streams the file with its video copied and its default audio track
// encoded to AAC, for players that can decode the video but not the audio (browsers and E-AC-3,
// AC-3, DTS or TrueHD). Video that can't simply be copied gets a full transcode instead.
func (s *Streamer) transcodeAudioAndServe(w http.ResponseWriter, r *http.Request, filePath string, startTime string, tracks TrackOptions, sess *StreamSession) error {
	codecs, err := ProbeCodecs(filePath)
	if err != nil || !codecs.BrowserVideo() {
		return s.transcodeAndServe(w, r, filePath, startTime, "", "",
			"The player can't play the audio (and the video can't be copied as it is)", tracks, sess)
	}
	args := []string{}
	if startTime != "" {
		// The copied video starts at the keyframe the seek lands on (SeekPoint). Without
		// -noaccurate_seek the audio would start at startTime exactly, after seconds of silence.
		args = append(args, "-noaccurate_seek", "-ss", startTime)
	}
	args = append(args, "-i", filePath, "-map", "0:v:0")
	reason := "The player can't play the audio"
	if len(codecs.Audio) > 0 {
		n := tracks.audioIndex(codecs)
		args = append(args, "-map", "0:a:"+strconv.Itoa(n))
		reason = fmt.Sprintf("The player can't play %s audio", strings.ToUpper(codecs.Audio[n].Codec))
		if tracks.Audio >= 0 {
			reason = fmt.Sprintf("The player asked for audio track %d (%s)", n+1, codecs.Audio[n].Codec)
		}
	}
	audio, audioInfo := tracks.audioArgs(true) // browsers play surround as stereo anyway
	args = append(args, "-c:v", "copy")
	args = append(args, audio...)
	args = append(args,
		"-avoid_negative_ts", "make_zero",
		"-movflags", tracks.movFlags(),
		"-f", "mp4",
		"pipe:1",
	)
	fmt.Printf("Transcoding audio only for: %s\n", filePath)
	info := TranscodeInfo{Reason: reason, Args: args, Container: "Fragmented MP4",
		Video: "Copied (H.264)", Audio: audioInfo}
	info.Start, _ = strconv.ParseFloat(startTime, 64)
	sess.StartTranscode(info)
	return s.runTranscode(w, r, filePath, args, sess)
}

// SubtitleArgs are the ffmpeg arguments for ServeSubtitles: the cues from about start (where the
// seek lands) to start+duration (duration 0: to the end), at their times in the file. Subtitles
// are spread through the whole file, so a player asks for them a few minutes at a time rather
// than have all of it read at once.
func SubtitleArgs(filePath string, track int, start, duration float64) []string {
	args := []string{"-v", "error"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	args = append(args, "-i", filePath, "-copyts", "-start_at_zero")
	if duration > 0 {
		args = append(args, "-to", strconv.FormatFloat(start+duration, 'f', 3, 64))
	}
	return append(args, "-map", "0:s:"+strconv.Itoa(track), "-c:s", "webvtt", "-f", "webvtt", "pipe:1")
}

// SubtitleFileArgs are SubtitleArgs for a subtitle file (beside the video, or downloaded), read in
// charset. A subtitle file can't seek: the cues before start are dropped as they're read instead.
func SubtitleFileArgs(filePath, charset string, start, duration float64) []string {
	args := []string{"-v", "error", "-sub_charenc", charset, "-i", filePath, "-copyts"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	if duration > 0 {
		args = append(args, "-to", strconv.FormatFloat(start+duration, 'f', 3, 64))
	}
	return append(args, "-map", "0:s:0", "-c:s", "webvtt", "-f", "webvtt", "pipe:1")
}

// ServeSubtitles streams a text subtitle track as WebVTT (see SubtitleArgs), cue by cue as ffmpeg
// reads them.
func ServeSubtitles(w http.ResponseWriter, r *http.Request, filePath string, track int, start, duration float64) error {
	return serveWebVTT(w, r, SubtitleArgs(filePath, track, start, duration))
}

// ServeSubtitleFile streams a subtitle file as WebVTT, as ServeSubtitles does a video's track.
func ServeSubtitleFile(w http.ResponseWriter, r *http.Request, filePath, charset string, start, duration float64) error {
	return serveWebVTT(w, r, SubtitleFileArgs(filePath, charset, start, duration))
}

func serveWebVTT(w http.ResponseWriter, r *http.Request, args []string) error {
	cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 16*1024)
	for {
		n, err := stdout.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	cmd.Wait()
	return nil
}
