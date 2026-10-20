package media

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// How long a stream stays listed after its last request or player report.
const (
	streamIdleTimeout = 90 * time.Second
	reportFresh       = 30 * time.Second // a player report this recent says what the player is doing
	transcodeStall    = 8 * time.Second  // ffmpeg making no progress this long is waiting on the client
)

// StreamTracker keeps the video streams being played, for the admin Streams page. A stream is
// one user playing one file from one client: the many range requests a player makes for a file,
// or the transcodes it starts when seeking, all count as the same stream.
type StreamTracker struct {
	mu       sync.Mutex
	sessions map[string]*StreamSession
	nextID   int
	now      func() time.Time
}

func NewStreamTracker() *StreamTracker {
	return &StreamTracker{sessions: map[string]*StreamSession{}, now: time.Now}
}

// StreamInfo is a snapshot of one stream.
type StreamInfo struct {
	ID           string    `json:"id"`
	User         string    `json:"user"`
	ClientIP     string    `json:"client_ip"`
	Client       string    `json:"client"` // the player, from its user agent
	UserAgent    string    `json:"user_agent"`
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	Title        string    `json:"title,omitempty"` // TMDB title, for files in movie and TV libraries
	Size         int64     `json:"size"`
	Duration     float64   `json:"duration"` // seconds
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	// State is "playing" or "paused" (from player reports), else "streaming" while data is being
	// sent, or "idle".
	State     string  `json:"state"`
	Mode      string  `json:"mode"` // "direct" or "transcode"
	Requests  int     `json:"requests"`
	Active    int     `json:"active_requests"`
	BytesSent int64   `json:"bytes_sent"`
	ServedTo  float64 `json:"served_to"` // seconds of the file sent so far (direct play: from the byte offset)
	// Position and Paused are what the player last reported, at PositionAt.
	Position   *float64       `json:"position,omitempty"`
	Paused     bool           `json:"paused"`
	PositionAt *time.Time     `json:"position_at,omitempty"`
	Transcode  *TranscodeInfo `json:"transcode,omitempty"`
}

// TranscodeInfo describes a stream's ffmpeg transcode and its progress.
type TranscodeInfo struct {
	Reason      string   `json:"reason"` // why the file is transcoded
	Start       float64  `json:"start"`  // seconds into the file the transcode started at
	Video       string   `json:"video"`  // output video settings
	Audio       string   `json:"audio"`
	Container   string   `json:"container"`
	Resolution  string   `json:"resolution,omitempty"` // requested output height
	Bitrate     string   `json:"bitrate,omitempty"`    // requested video bitrate
	SourceVideo string   `json:"source_video,omitempty"`
	SourceAudio string   `json:"source_audio,omitempty"`
	Args        []string `json:"args"`

	OutTime    float64   `json:"out_time"` // seconds transcoded since Start
	Speed      float64   `json:"speed"`    // x realtime
	FPS        float64   `json:"fps"`
	Frame      int64     `json:"frame"`
	OutBitrate float64   `json:"out_bitrate"` // kbit/s
	Stalled    bool      `json:"stalled"`     // ffmpeg is waiting for the client to read (paused, or far ahead)
	StartedAt  time.Time `json:"started_at"`
	lastChange time.Time
}

// StreamSession is a tracked stream; its methods are safe for concurrent use.
type StreamSession struct {
	t      *StreamTracker
	key    string
	info   StreamInfo
	probed bool
}

// Begin records a request for a stream (created with init the first time) and returns its
// session; call End when the request finishes.
func (t *StreamTracker) Begin(key string, init func(*StreamInfo)) *StreamSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.session(key, init)
	s.info.Requests++
	s.info.Active++
	s.info.LastActivity = t.now()
	return s
}

// session returns the session for key, creating it with init; t.mu must be held.
func (t *StreamTracker) session(key string, init func(*StreamInfo)) *StreamSession {
	s := t.sessions[key]
	if s == nil {
		t.nextID++
		s = &StreamSession{t: t, key: key}
		s.info.ID = strconv.Itoa(t.nextID)
		s.info.StartedAt = t.now()
		s.info.Mode = "direct"
		if init != nil {
			init(&s.info)
		}
		t.sessions[key] = s
	}
	return s
}

// Report records what a player says it is doing: its position (seconds) and whether it's paused.
func (t *StreamTracker) Report(key string, init func(*StreamInfo), position float64, paused bool) *StreamSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.session(key, init)
	now := t.now()
	s.info.Position, s.info.Paused, s.info.PositionAt = &position, paused, &now
	s.info.LastActivity = now
	return s
}

// NeedsProbe reports, once per stream, that its duration isn't known yet (the file hasn't been
// probed), so the caller can probe it and FillIn the result.
func (s *StreamSession) NeedsProbe() bool {
	if s == nil {
		return false
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	if s.probed || (s.info.Duration > 0 && s.info.Width > 0) {
		return false
	}
	s.probed = true
	return true
}

// FillIn updates the stream's details.
func (s *StreamSession) FillIn(update func(*StreamInfo)) {
	if s == nil {
		return
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	update(&s.info)
}

// End records that one of the stream's requests finished.
func (s *StreamSession) End() {
	if s == nil {
		return
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	s.info.Active--
	s.info.LastActivity = s.t.now()
	if s.info.Transcode != nil && s.info.Active == 0 {
		s.info.Mode = "direct" // a later request may play the file directly; a transcode sets it again
		s.info.Transcode = nil
	}
}

// Sent records n more bytes sent; offset is how far into the file they reach (-1 if unknown).
func (s *StreamSession) Sent(n int, offset int64) {
	if s == nil || n <= 0 {
		return
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	s.info.BytesSent += int64(n)
	s.info.LastActivity = s.t.now()
	if offset >= 0 && s.info.Size > 0 && s.info.Duration > 0 {
		s.info.ServedTo = float64(offset) / float64(s.info.Size) * s.info.Duration
	}
}

// StartTranscode marks the stream as transcoding with the given settings.
func (s *StreamSession) StartTranscode(info TranscodeInfo) {
	if s == nil {
		return
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	info.StartedAt, info.lastChange = s.t.now(), s.t.now()
	s.info.Mode, s.info.Transcode = "transcode", &info
}

var (
	ffTime    = regexp.MustCompile(`time=\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	ffSpeed   = regexp.MustCompile(`speed=\s*([\d.]+)x`)
	ffFPS     = regexp.MustCompile(`fps=\s*([\d.]+)`)
	ffFrame   = regexp.MustCompile(`frame=\s*(\d+)`)
	ffBitrate = regexp.MustCompile(`bitrate=\s*([\d.]+)kbits/s`)
	ffInput   = regexp.MustCompile(`Stream #0:\d+[^:]*: (Video|Audio): ([^\n\r]+)`)
)

// FFmpegOutput parses ffmpeg's stderr (progress lines, input stream descriptions) into the
// stream's transcode progress.
func (s *StreamSession) FFmpegOutput(chunk string) {
	if s == nil {
		return
	}
	s.t.mu.Lock()
	defer s.t.mu.Unlock()
	tr := s.info.Transcode
	if tr == nil {
		return
	}
	for _, m := range ffInput.FindAllStringSubmatch(chunk, -1) {
		desc := strings.TrimSpace(m[2])
		if m[1] == "Video" && tr.SourceVideo == "" {
			tr.SourceVideo = desc
		} else if m[1] == "Audio" && tr.SourceAudio == "" {
			tr.SourceAudio = desc
		}
	}
	// A chunk may hold several progress updates; the last one is current.
	if m := lastMatch(ffTime, chunk); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		mi, _ := strconv.ParseFloat(m[2], 64)
		sec, _ := strconv.ParseFloat(m[3], 64)
		if out := h*3600 + mi*60 + sec; out != tr.OutTime {
			tr.OutTime, tr.lastChange = out, s.t.now()
		}
	}
	if m := lastMatch(ffSpeed, chunk); m != nil {
		tr.Speed, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := lastMatch(ffFPS, chunk); m != nil {
		tr.FPS, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := lastMatch(ffFrame, chunk); m != nil {
		tr.Frame, _ = strconv.ParseInt(m[1], 10, 64)
	}
	if m := lastMatch(ffBitrate, chunk); m != nil {
		tr.OutBitrate, _ = strconv.ParseFloat(m[1], 64)
	}
}

func lastMatch(re *regexp.Regexp, s string) []string {
	all := re.FindAllStringSubmatch(s, -1)
	if len(all) == 0 {
		return nil
	}
	return all[len(all)-1]
}

// List returns the current streams, oldest first, dropping ones that have gone quiet.
func (t *StreamTracker) List() []StreamInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	out := []StreamInfo{}
	for key, s := range t.sessions {
		if s.info.Active <= 0 && now.Sub(s.info.LastActivity) > streamIdleTimeout {
			delete(t.sessions, key)
			continue
		}
		info := s.info
		switch {
		case info.PositionAt != nil && now.Sub(*info.PositionAt) < reportFresh && info.Paused:
			info.State = "paused"
		case info.PositionAt != nil && now.Sub(*info.PositionAt) < reportFresh:
			info.State = "playing"
		case info.Active > 0:
			info.State = "streaming"
		default:
			info.State = "idle"
		}
		if s.info.Transcode != nil {
			tr := *s.info.Transcode
			tr.Stalled = now.Sub(tr.lastChange) > transcodeStall
			if tr.Stalled {
				tr.Speed, tr.FPS = 0, 0 // ffmpeg's figures are averages that keep decaying while it waits
			}
			info.Transcode = &tr
			info.ServedTo = tr.Start + tr.OutTime
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.Before(out[j].StartedAt) })
	return out
}

// ClientName names a player from its user agent.
func ClientName(ua string) string {
	switch {
	case strings.HasPrefix(ua, "WeirdStreamer/"), strings.HasPrefix(ua, "okhttp/"):
		return "WeirdStreamer app" // its player, and its API client (progress reports)
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "Firefox/"):
		return "Firefox"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	case strings.Contains(ua, "VLC"):
		return "VLC"
	case ua == "":
		return "Unknown"
	}
	if i := strings.IndexAny(ua, " /"); i > 0 {
		return ua[:i]
	}
	return ua
}

// ProbeVideo reads a video file's duration (seconds) and first video stream's size.
func ProbeVideo(path string) (duration float64, width, height int, err error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "format=duration:stream=width,height", "-of", "json", path).Output()
	if err != nil {
		return 0, 0, 0, err
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return 0, 0, 0, err
	}
	duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if len(probe.Streams) > 0 {
		width, height = probe.Streams[0].Width, probe.Streams[0].Height
	}
	return duration, width, height, nil
}
