package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Compressed and compatible downloads for offline viewing: the server encodes a file to a
// temporary MP4 (one at a time), and the client downloads it with resume support, then deletes
// the job. A compatible copy (Compatibility) keeps the video or the audio as it is where the
// device plays it, and encodes only the rest.

// Download qualities: a target share of the original's size and of its resolution.
var (
	DownloadSizes  = []int{90, 75, 50, 25}
	DownloadScales = []int{100, 75, 50}
)

const (
	downloadAudioBitrate = 128_000
	fullAudioBitrate     = 192_000        // stereo AAC in a full-quality copy
	downloadKeepReady    = 24 * time.Hour // a finished file nobody fetched is removed after this
	maxJobsPerUser       = 50
)

// DownloadJob is one encode for a download.
type DownloadJob struct {
	ID      string `json:"id"`
	Owner   int64  `json:"-"`
	Path    string `json:"path"`
	SizePct int    `json:"size_pct"` // 100: full quality (only for discs, which can't be downloaded as they are)
	Scale   int    `json:"scale"`
	// Kept as they are instead of encoded (full size and resolution only): what the device plays.
	KeepVideo bool    `json:"keep_video,omitempty"`
	KeepAudio bool    `json:"keep_audio,omitempty"`
	Audio     int     `json:"audio"`  // the audio track kept (among the file's), -1 its default
	Status    string  `json:"status"` // queued, encoding, ready, failed
	Progress  float64 `json:"progress"`
	Error     string  `json:"error,omitempty"`
	Size      int64   `json:"size,omitempty"` // bytes, once ready
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Duration  float64 `json:"duration"`

	file    string
	readyAt time.Time
	cancel  context.CancelFunc
	encoder string // what encoded the video, for the log ("" when it was copied)
}

// DownloadSource describes what a file would download as.
type DownloadSource struct {
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Disc     bool    `json:"disc"` // a DVD image: downloads are its main title as MP4, never the image
	// MainTitleUncertain: a title of the disc that couldn't be read may be longer than the one
	// downloaded (DiscInfo.MainTitleUncertain).
	MainTitleUncertain bool `json:"main_title_uncertain,omitempty"`
}

// DownloadManager runs download encodes one at a time.
type DownloadManager struct {
	dir   string
	mu    sync.Mutex
	jobs  map[string]*DownloadJob
	queue chan *DownloadJob
}

// NewDownloadManager keeps encodes in dir and starts its worker. Encodes left from an earlier
// run are removed; nothing else in dir is touched, as it may be shared with other files.
func NewDownloadManager(dir string) *DownloadManager {
	os.MkdirAll(dir, 0755)
	for _, pattern := range []string{"*.mp4", "*.mp4.part"} {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range matches {
			if isJobFile(filepath.Base(f)) {
				os.Remove(f)
			}
		}
	}
	m := &DownloadManager{dir: dir, jobs: map[string]*DownloadJob{}, queue: make(chan *DownloadJob, 1000)}
	go m.work()
	go m.expire()
	return m
}

// Source probes a file for the download options.
func Source(path string) (*DownloadSource, error) {
	path = Playable(path) // a damaged video's fixed copy
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	src := &DownloadSource{Size: st.Size()}
	if IsDiscImage(path) {
		info, err := LoadDiscWithin(path, DiscRequestWait)
		if err != nil {
			return nil, err
		}
		t := info.Title(0)
		src.Disc, src.Duration = true, t.Duration
		src.MainTitleUncertain = info.MainTitleUncertain
		// The main title as square-pixel video, as it's encoded.
		src.Height = t.Height
		aspect := t.Aspect
		if aspect <= 0 {
			aspect = 16.0 / 9
		}
		src.Width = int(float64(t.Height)*aspect+0.5) &^ 1
		// Sizes are shares of the whole image (extras included), which is what the disc weighs.
		return src, nil
	}
	src.Duration, src.Width, src.Height, err = ProbeVideo(path)
	return src, err
}

// Create queues an encode of path for owner: at sizePct of its size and scale of its resolution,
// keeping the video or audio as they are where keepVideo or keepAudio say (full quality only), with
// the audio track audio (among the file's; -1 its default).
func (m *DownloadManager) Create(owner int64, path string, sizePct, scale int, keepVideo, keepAudio bool, audio int) (*DownloadJob, error) {
	if !contains(append([]int{100}, DownloadSizes...), sizePct) || !contains(DownloadScales, scale) {
		return nil, errors.New("unknown quality")
	}
	full := sizePct == 100 && scale == 100
	disc := IsDiscImage(path)
	if (keepVideo || keepAudio) && (!full || disc) {
		return nil, errors.New("only a full-quality copy of a file keeps its video or audio")
	}
	// Keeping both is a copy into MP4, for players that can't open the file's container (MKV on
	// an iPhone); a file already in MP4 is downloaded as it is.
	if full && !disc && keepVideo && keepAudio && isMP4File(path) {
		return nil, errors.New("original files are downloaded directly")
	}
	src, err := Source(path)
	if err != nil {
		return nil, err
	}
	if src.Duration <= 0 {
		return nil, errors.New("the file's length is unknown")
	}
	m.mu.Lock()
	n := 0
	for _, j := range m.jobs {
		if j.Owner == owner {
			n++
		}
	}
	if n >= maxJobsPerUser {
		m.mu.Unlock()
		return nil, errors.New("too many downloads being prepared")
	}
	id := newID()
	job := &DownloadJob{ID: id, Owner: owner, Path: path, SizePct: sizePct, Scale: scale, KeepVideo: keepVideo,
		KeepAudio: keepAudio, Audio: audio, Status: "queued", Duration: src.Duration, file: filepath.Join(m.dir, id+".mp4")}
	m.jobs[id] = job
	m.mu.Unlock()
	m.queue <- job
	return job.snapshot(m), nil
}

// Get returns owner's job, or nil.
func (m *DownloadManager) Get(owner int64, id string) *DownloadJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j == nil || j.Owner != owner {
		return nil
	}
	c := *j
	return &c
}

// File returns the finished file of owner's job.
func (m *DownloadManager) File(owner int64, id string) (string, bool) {
	j := m.Get(owner, id)
	if j == nil || j.Status != "ready" {
		return "", false
	}
	return j.file, true
}

// Delete cancels or removes owner's job and its file.
func (m *DownloadManager) Delete(owner int64, id string) bool {
	m.mu.Lock()
	j := m.jobs[id]
	if j == nil || j.Owner != owner {
		m.mu.Unlock()
		return false
	}
	delete(m.jobs, id)
	cancel := j.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	os.Remove(j.file)
	os.Remove(j.file + ".part")
	return true
}

func (j *DownloadJob) snapshot(m *DownloadManager) *DownloadJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *j
	return &c
}

func (m *DownloadManager) work() {
	for job := range m.queue {
		m.mu.Lock()
		if m.jobs[job.ID] != job { // deleted while queued
			m.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		job.Status, job.cancel = "encoding", cancel
		m.mu.Unlock()

		err := m.encode(ctx, job)
		cancel()
		// The encode's picture size, read before the job shows as ready (clients read it then).
		var w, h int
		if err == nil {
			_, w, h, _ = ProbeVideo(job.file)
		}
		m.mu.Lock()
		job.cancel = nil
		if err != nil {
			if ctx.Err() == nil {
				job.Status, job.Error, job.readyAt = "failed", err.Error(), time.Now()
				log.Printf("[DOWNLOAD] %s failed: %v", filepath.Base(job.Path), err)
			}
		} else {
			job.Status, job.Progress, job.readyAt = "ready", 1, time.Now()
			if st, err := os.Stat(job.file); err == nil {
				job.Size = st.Size()
			}
			if w > 0 && h > 0 {
				job.Width, job.Height = w, h
			}
		}
		m.mu.Unlock()
		if err == nil {
			log.Printf("[DOWNLOAD] Ready: %s (%s)", filepath.Base(job.Path), job.describe())
		}
	}
}

// describe is the job's quality, for the log.
func (j *DownloadJob) describe() string {
	if j.SizePct == 100 && j.Scale == 100 {
		kept := map[bool]string{true: "kept", false: "encoded"}
		return "full quality, video " + kept[j.KeepVideo] + ", audio " + kept[j.KeepAudio] + j.encodedBy()
	}
	return fmt.Sprintf("%d%% size, %d%% resolution", j.SizePct, j.Scale) + j.encodedBy()
}

func (j *DownloadJob) encodedBy() string {
	if j.encoder == "" {
		return ""
	}
	return ", video by " + j.encoder
}

// mp4Audio reports whether an audio codec can be copied into MP4 as it is.
func mp4Audio(codec string) bool {
	switch codec {
	case "aac", "ac3", "eac3", "mp3", "opus", "flac", "alac":
		return true
	}
	return false
}

// encode writes job's MP4: H.264 at a bitrate that lands near the target size (or at full quality),
// AAC stereo, from the default audio track; what the job keeps is copied instead.
func (m *DownloadManager) encode(ctx context.Context, job *DownloadJob) error {
	src, err := Source(job.Path)
	if err != nil {
		return err
	}
	height := 0
	if job.Scale < 100 && src.Height > 0 {
		height = int(float64(src.Height)*float64(job.Scale)/100+0.5) &^ 1
	}

	var args []string
	keepAudio, hevc := job.KeepAudio, false
	if src.Disc {
		res := ""
		if height > 0 {
			res = strconv.Itoa(height)
		}
		full, _, err := DiscTranscodeArgs(job.Path, DiscOptions{Angle: 1}, "", "", res)
		if err != nil {
			return err
		}
		// Keep the disc's inputs, filters and maps; the output settings are the download's.
		for i, a := range full {
			if a == "-c:v" {
				args = full[:i]
				break
			}
		}
	} else {
		codecs, err := ProbeCodecs(Playable(job.Path))
		if err != nil {
			return err
		}
		track := job.Audio
		if track < 0 || track >= len(codecs.Audio) {
			track = codecs.DefaultAudio()
		}
		args = []string{"-i", Playable(job.Path), "-map", "0:v:0", "-map", fmt.Sprintf("0:a:%d?", track)}
		if height > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", height))
		}
		if len(codecs.Audio) > 0 && !mp4Audio(codecs.Audio[track].Codec) {
			keepAudio = false // DTS, TrueHD…: MP4 can't carry it
		}
		hevc = codecs.Video != nil && codecs.Video.Codec == "hevc"
	}

	switch {
	case job.KeepVideo:
		args = append(args, "-c:v", "copy")
		if hevc {
			args = append(args, "-tag:v", "hvc1") // what Apple and Android players look for in MP4
		}
	case job.SizePct == 100:
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "18", "-pix_fmt", "yuv420p") // full quality
	default:
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p")
		total := float64(src.Size) * 8 * float64(job.SizePct) / 100 / src.Duration
		video := int64(total) - downloadAudioBitrate
		if video < 150_000 {
			video = 150_000
		}
		b := strconv.FormatInt(video, 10)
		args = append(args, "-b:v", b, "-maxrate", strconv.FormatInt(video*3/2, 10), "-bufsize", strconv.FormatInt(video*2, 10))
	}
	if keepAudio {
		args = append(args, "-c:a", "copy")
	} else {
		bitrate := downloadAudioBitrate
		if job.SizePct == 100 {
			bitrate = fullAudioBitrate
		}
		args = append(args, "-c:a", "aac", "-b:a", strconv.Itoa(bitrate), "-ac", "2")
	}
	part := job.file + ".part"
	args = append(args, "-movflags", "+faststart", "-f", "mp4", "-progress", "pipe:1", "-nostats", "-y", part)

	if src.MainTitleUncertain {
		log.Printf("[DOWNLOAD] %s: a title that can't be read may be longer than the one downloaded as the main title", filepath.Base(job.Path))
	}
	// The video encoded on the GPU Settings chose while it has room (streams and downloads share
	// streaming.hardwareMaxTranscodes), else x264. A GPU encode that fails runs again on the CPU.
	if !encodesVideo(args) {
		return m.ffmpeg(ctx, job, src, args, part)
	}
	hw, device, ok := hardwareForDownload()
	if !ok {
		job.encoder = "libx264"
		return m.ffmpeg(ctx, job, src, args, part)
	}
	gpuArgs := hardwareArgs(args, hw, device)
	job.encoder = encoderLabel(gpuArgs)
	err = m.ffmpeg(ctx, job, src, gpuArgs, part)
	downloadsOnGPU.Add(-1)
	if err == nil || ctx.Err() != nil || errors.Is(err, ErrFFmpegStalled) {
		return err
	}
	m.mu.Lock()
	started := job.Progress > 0
	job.Progress = 0
	m.mu.Unlock()
	if !started { // the GPU couldn't encode at all: streams and downloads skip it for a while
		hardwareFailed.Store(time.Now().UnixNano())
	}
	log.Printf("[DOWNLOAD] The GPU encoder failed for %s, so the CPU encodes it: %v", filepath.Base(job.Path), err)
	job.encoder = "libx264, after the GPU failed"
	return m.ffmpeg(ctx, job, src, args, part)
}

// ffmpeg runs a download's encode with args, writing part, and renames it to the job's file.
func (m *DownloadManager) ffmpeg(ctx context.Context, job *DownloadJob, src *DownloadSource, args []string, part string) error {
	run, stop := context.WithCancel(ctx)
	defer stop()
	var watch *stallWatch
	if src.Disc {
		watch = watchStalls(stop) // a stuck DVD demuxer: see watchdog.go
		defer watch.done()
	}
	cmd := exec.CommandContext(run, "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	watchProgressLines(stdout, watch, func(line string) {
		if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
			if us, err := strconv.ParseFloat(v, 64); err == nil && us > 0 {
				m.mu.Lock()
				job.Progress = min(us/1e6/src.Duration, 0.99)
				m.mu.Unlock()
			}
		}
	})
	if err := cmd.Wait(); err != nil {
		os.Remove(part)
		if watch.stalled() {
			return ErrFFmpegStalled
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return fmt.Errorf("ffmpeg: %v %s", err, msg)
	}
	return os.Rename(part, job.file)
}

// expire removes finished files nobody fetched.
func (m *DownloadManager) expire() {
	for range time.Tick(time.Hour) {
		m.mu.Lock()
		var old []*DownloadJob
		for id, j := range m.jobs {
			if (j.Status == "ready" || j.Status == "failed") && time.Since(j.readyAt) > downloadKeepReady {
				old = append(old, j)
				delete(m.jobs, id)
			}
		}
		m.mu.Unlock()
		for _, j := range old {
			os.Remove(j.file)
		}
	}
}

// isJobFile reports whether name is an encode's file: its 24-hex-digit job ID plus .mp4 or .mp4.part.
func isJobFile(name string) bool {
	id := strings.TrimSuffix(strings.TrimSuffix(name, ".part"), ".mp4")
	if len(id) != 24 || len(id)+4 > len(name) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func newID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func contains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// isMP4File reports whether a file is in an MP4 (or QuickTime) container, by its extension.
func isMP4File(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov":
		return true
	}
	return false
}
