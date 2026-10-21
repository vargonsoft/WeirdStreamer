package media

import (
	"bufio"
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

// Compressed downloads for offline viewing: the server encodes a file to a temporary MP4 (one at
// a time), and the client downloads it with resume support, then deletes the job.

// Download qualities: a target share of the original's size and of its resolution.
var (
	DownloadSizes  = []int{90, 75, 50, 25}
	DownloadScales = []int{100, 75, 50}
)

const (
	downloadAudioBitrate = 128_000
	downloadKeepReady    = 24 * time.Hour // a finished file nobody fetched is removed after this
	maxJobsPerUser       = 50
)

// DownloadJob is one encode for a download.
type DownloadJob struct {
	ID       string  `json:"id"`
	Owner    int64   `json:"-"`
	Path     string  `json:"path"`
	SizePct  int     `json:"size_pct"` // 100: full quality (only for discs, which can't be downloaded as they are)
	Scale    int     `json:"scale"`
	Status   string  `json:"status"` // queued, encoding, ready, failed
	Progress float64 `json:"progress"`
	Error    string  `json:"error,omitempty"`
	Size     int64   `json:"size,omitempty"` // bytes, once ready
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	Duration float64 `json:"duration"`

	file    string
	readyAt time.Time
	cancel  context.CancelFunc
}

// DownloadSource describes what a file would download as.
type DownloadSource struct {
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	Disc     bool    `json:"disc"` // a DVD image: downloads are its main title as MP4, never the image
}

// DownloadManager runs download encodes one at a time.
type DownloadManager struct {
	dir   string
	mu    sync.Mutex
	jobs  map[string]*DownloadJob
	queue chan *DownloadJob
}

// NewDownloadManager keeps encodes in dir (emptied on start) and starts its worker.
func NewDownloadManager(dir string) *DownloadManager {
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0755)
	m := &DownloadManager{dir: dir, jobs: map[string]*DownloadJob{}, queue: make(chan *DownloadJob, 1000)}
	go m.work()
	go m.expire()
	return m
}

// Source probes a file for the download options.
func Source(path string) (*DownloadSource, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	src := &DownloadSource{Size: st.Size()}
	if IsDiscImage(path) {
		info, err := LoadDisc(path)
		if err != nil {
			return nil, err
		}
		t := info.Title(0)
		src.Disc, src.Duration = true, t.Duration
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

// Create queues an encode of path for owner.
func (m *DownloadManager) Create(owner int64, path string, sizePct, scale int) (*DownloadJob, error) {
	if !contains(append([]int{100}, DownloadSizes...), sizePct) || !contains(DownloadScales, scale) {
		return nil, errors.New("unknown quality")
	}
	if sizePct == 100 && scale == 100 && !IsDiscImage(path) {
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
	job := &DownloadJob{ID: id, Owner: owner, Path: path, SizePct: sizePct, Scale: scale, Status: "queued",
		Duration: src.Duration, file: filepath.Join(m.dir, id+".mp4")}
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
		}
		m.mu.Unlock()
		if err == nil {
			if _, w, h, perr := ProbeVideo(job.file); perr == nil {
				m.mu.Lock()
				job.Width, job.Height = w, h
				m.mu.Unlock()
			}
			log.Printf("[DOWNLOAD] Ready: %s (%d%% size, %d%% resolution)", filepath.Base(job.Path), job.SizePct, job.Scale)
		}
	}
}

// encode writes job's MP4: H.264 at a bitrate that lands near the target size, AAC stereo.
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
	if src.Disc {
		res := ""
		if height > 0 {
			res = strconv.Itoa(height)
		}
		full, _, err := DiscTranscodeArgs(job.Path, DiscOptions{Angle: 1, Subtitle: -1}, "", "", res)
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
		args = []string{"-i", job.Path, "-map", "0:v:0", "-map", "0:a:0?"}
		if height > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", height))
		}
	}

	args = append(args, "-c:v", "libx264", "-preset", "veryfast")
	if job.SizePct == 100 {
		args = append(args, "-crf", "18") // full quality (discs)
	} else {
		total := float64(src.Size) * 8 * float64(job.SizePct) / 100 / src.Duration
		video := int64(total) - downloadAudioBitrate
		if video < 150_000 {
			video = 150_000
		}
		b := strconv.FormatInt(video, 10)
		args = append(args, "-b:v", b, "-maxrate", strconv.FormatInt(video*3/2, 10), "-bufsize", strconv.FormatInt(video*2, 10))
	}
	part := job.file + ".part"
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", strconv.Itoa(downloadAudioBitrate), "-ac", "2",
		"-movflags", "+faststart", "-f", "mp4", "-progress", "pipe:1", "-nostats", "-y", part)

	cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-loglevel", "error", "-nostdin"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
			if us, err := strconv.ParseFloat(v, 64); err == nil && us > 0 {
				m.mu.Lock()
				job.Progress = min(us/1e6/src.Duration, 0.99)
				m.mu.Unlock()
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		os.Remove(part)
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
