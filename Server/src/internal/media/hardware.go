package media

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// Graphics hardware for streaming transcodes and downloads. At startup the server looks for GPUs and tries a
// short test encode with each encoder ffmpeg has for them; Settings offers those that worked
// (streaming.hardwareEncoder). The GPU only encodes: decoding and filters (scaling, burned-in
// subtitles, 10-bit to 8-bit) stay on the CPU, so every source the CPU transcodes works the same.
// A hardware transcode that fails before making anything is run again on the CPU.

// GPU is a graphics device the server found.
type GPU struct {
	Name   string `json:"name"`             // "NVIDIA GeForce GTX 1060 6GB", "Intel UHD Graphics 630"
	Vendor string `json:"vendor"`           // nvidia, intel, amd, other
	Device string `json:"device,omitempty"` // its render node (/dev/dri/renderD128), where it has one
	Driver string `json:"driver,omitempty"` // its kernel driver ("i915", "amdgpu"; NVIDIA's with its version)
	Memory string `json:"memory,omitempty"` // "6 GB" (NVIDIA's cards; a Mac's memory, which the GPU shares)
}

// HardwareEncoder is a way to encode H.264 on a GPU that passed the test encode.
type HardwareEncoder struct {
	ID     string `json:"id"`               // streaming.hardwareEncoder: nvenc, qsv, vaapi
	Device string `json:"device,omitempty"` // streaming.hardwareDevice, for qsv and vaapi
	Name   string `json:"name"`             // "NVENC", "Quick Sync", "VA-API"
	GPU    string `json:"gpu"`              // the GPU's name
	Vendor string `json:"vendor"`
}

// HardwareProblem is a GPU an encoder was tried on that didn't work, and why, for Settings.
type HardwareProblem struct {
	GPU     string `json:"gpu"`
	Encoder string `json:"encoder"`
	Reason  string `json:"reason"`
}

// HardwareReport is what the server found, for Settings: GET /api/admin/hardware.
type HardwareReport struct {
	Checking bool              `json:"checking"` // the test encodes haven't finished
	GPUs     []GPU             `json:"gpus"`
	Encoders []HardwareEncoder `json:"encoders"`
	Problems []HardwareProblem `json:"problems,omitempty"`
	FFmpeg   string            `json:"ffmpeg,omitempty"` // ffmpeg's version
}

var hardware struct {
	sync.Mutex
	report HardwareReport
	once   sync.Once
}

// DetectHardware looks for GPUs and their encoders in the background (once).
func DetectHardware() {
	hardware.once.Do(func() {
		hardware.Lock()
		hardware.report = HardwareReport{Checking: true, GPUs: []GPU{}, Encoders: []HardwareEncoder{}}
		hardware.Unlock()
		go func() {
			r := detectHardware()
			hardware.Lock()
			hardware.report = r
			hardware.Unlock()
			logHardware(r)
		}()
	})
}

// logHardware writes what the startup check found, and what Settings has chosen, to the log.
func logHardware(r HardwareReport) {
	if len(r.GPUs) == 0 {
		log.Printf("[HARDWARE] No GPU found")
	}
	for _, g := range r.GPUs {
		details := []string{}
		if g.Driver != "" {
			details = append(details, "driver "+g.Driver)
		}
		for _, d := range []string{g.Memory, g.Device} {
			if d != "" {
				details = append(details, d)
			}
		}
		line := "[HARDWARE] GPU: " + g.Name
		if len(details) > 0 {
			line += "; " + strings.Join(details, ", ")
		}
		log.Print(line)
	}
	if r.FFmpeg != "" {
		log.Printf("[HARDWARE] ffmpeg %s", r.FFmpeg)
	}
	for _, e := range r.Encoders {
		log.Printf("[HARDWARE] %s on %s works", e.Name, e.GPU)
	}
	for _, p := range r.Problems {
		log.Printf("[HARDWARE] %s on %s doesn't work: %s", p.Encoder, p.GPU, p.Reason)
	}
	log.Printf("[HARDWARE] %s", HardwareSetting())
}

// HardwareSetting says what encodes transcodes' and downloads' video now, for the log: the
// encoder Settings chose and its limit, or x264 and why.
func HardwareSetting() string {
	const where = " (Settings › Server › Streaming)"
	r := Hardware()
	s := config.StreamingConfig{}
	if config.GlobalConfig != nil {
		s = config.GlobalConfig.Streaming
	}
	if r.Checking {
		return "Hardware encoding: still being checked"
	}
	if s.HardwareEncoder == "" {
		if len(r.Encoders) == 0 {
			return "Hardware encoding: none here works, so x264 on the CPU encodes transcodes and downloads"
		}
		names := []string{}
		for _, e := range r.Encoders {
			names = append(names, e.Name+" on "+e.GPU)
		}
		verb := " works here"
		if len(names) > 1 {
			verb = " work here"
		}
		return "Hardware encoding is off: x264 on the CPU encodes transcodes and downloads, though " +
			strings.Join(names, " and ") + verb + where
	}
	id, device, ok := hardwareChoice()
	if !ok {
		chosen := s.HardwareEncoder
		if s.HardwareDevice != "" {
			chosen += " on " + s.HardwareDevice
		}
		return "Hardware encoding is set to " + chosen + ", which doesn't work here: x264 on the CPU encodes instead" + where
	}
	name := encoderNames[id]
	for _, e := range r.Encoders {
		if e.ID == id && (e.Device == device || id == "nvenc") {
			name += " on " + e.GPU
			if e.Device != "" {
				name += " (" + e.Device + ")"
			}
			break
		}
	}
	limit := "no limit"
	if s.HardwareMaxTranscodes > 0 {
		limit = fmt.Sprintf("at most %d at once, the rest on the CPU", s.HardwareMaxTranscodes)
	}
	return "Hardware encoding: " + name + " encodes transcodes and downloads, " + limit + where
}

// Hardware is what DetectHardware found so far.
func Hardware() HardwareReport {
	DetectHardware()
	hardware.Lock()
	defer hardware.Unlock()
	r := hardware.report
	r.GPUs = slices.Clone(r.GPUs)
	r.Encoders = slices.Clone(r.Encoders)
	r.Problems = slices.Clone(r.Problems)
	return r
}

var encoderNames = map[string]string{"nvenc": "NVENC", "qsv": "Quick Sync", "vaapi": "VA-API", "videotoolbox": "VideoToolbox"}

func detectHardware() HardwareReport {
	r := HardwareReport{GPUs: findGPUs(), Encoders: []HardwareEncoder{}, FFmpeg: ffmpegVersion()}
	has := ffmpegEncoders()
	try := func(id string, g GPU) {
		codec := "h264_" + id
		if !has[codec] {
			r.Problems = append(r.Problems, HardwareProblem{g.Name, encoderNames[id],
				"this server's ffmpeg has no " + codec + " encoder"})
			return
		}
		if err := testEncode(id, g.Device); err != nil {
			r.Problems = append(r.Problems, HardwareProblem{g.Name, encoderNames[id], err.Error()})
			return
		}
		r.Encoders = append(r.Encoders, HardwareEncoder{ID: id, Device: g.Device, Name: encoderNames[id], GPU: g.Name, Vendor: g.Vendor})
	}
	nvidia := false
	for _, g := range r.GPUs {
		switch g.Vendor {
		case "nvidia":
			if !nvidia { // NVENC picks the GPU itself: one entry
				nvidia = true
				try("nvenc", GPU{Name: g.Name, Vendor: g.Vendor})
			}
		case "intel":
			try("qsv", g)
			try("vaapi", g)
		case "amd":
			try("vaapi", g)
		case "apple":
			try("videotoolbox", g)
		}
	}
	return r
}

// findGPUs lists the GPUs: each DRM render node's (Linux), NVIDIA's by nvidia-smi, and a Mac's own
// (VideoToolbox, which needs no device).
func findGPUs() []GPU {
	gpus := []GPU{}
	if runtime.GOOS == "darwin" {
		name := "This Mac's graphics"
		if chip, err := commandOutput(5*time.Second, "sysctl", "-n", "machdep.cpu.brand_string"); err == nil && strings.HasPrefix(chip, "Apple") {
			name = strings.TrimSpace(chip) + " GPU" // "Apple M1 GPU"
		}
		g := GPU{Name: name, Vendor: "apple"}
		if out, err := commandOutput(5*time.Second, "sysctl", "-n", "hw.memsize"); err == nil {
			if b, err := strconv.ParseFloat(strings.TrimSpace(out), 64); err == nil && b > 0 {
				g.Memory = fmt.Sprintf("%.0f GB shared", b/(1<<30))
			}
		}
		return append(gpus, g)
	}
	nodes, _ := filepath.Glob("/sys/class/drm/renderD*")
	slices.Sort(nodes)
	for _, node := range nodes {
		vendor := readTrim(filepath.Join(node, "device", "vendor"))
		g := GPU{Device: "/dev/dri/" + filepath.Base(node), Vendor: vendorName(vendor)}
		if driver, err := os.Readlink(filepath.Join(node, "device", "driver")); err == nil {
			g.Driver = filepath.Base(driver) // i915, xe, amdgpu, nvidia
		}
		g.Name = pciName(filepath.Join(node, "device"))
		if g.Name == "" {
			g.Name = vendorTitle[g.Vendor] + " GPU (" + filepath.Base(node) + ")"
		}
		gpus = append(gpus, g)
	}
	// NVIDIA's own name for its cards ("NVIDIA GeForce GTX 1060 6GB"), and a card without a
	// render node (other systems).
	// Its driver's version and the card's memory too.
	if out, err := commandOutput(5*time.Second, "nvidia-smi", "--query-gpu=name,driver_version,memory.total", "--format=csv,noheader,nounits"); err == nil {
		cards := []GPU{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Split(line, ",")
			if len(f) < 3 || strings.TrimSpace(f[0]) == "" {
				continue
			}
			c := GPU{Name: strings.TrimSpace(f[0]), Vendor: "nvidia", Driver: "nvidia " + strings.TrimSpace(f[1])}
			if mib, err := strconv.ParseFloat(strings.TrimSpace(f[2]), 64); err == nil && mib > 0 {
				c.Memory = fmt.Sprintf("%.0f GB", mib/1024)
			}
			cards = append(cards, c)
		}
		i := 0
		for j := range gpus {
			if gpus[j].Vendor == "nvidia" && i < len(cards) {
				gpus[j].Name, gpus[j].Driver, gpus[j].Memory = cards[i].Name, cards[i].Driver, cards[i].Memory
				i++
			}
		}
		gpus = append(gpus, cards[i:]...)
	}
	return gpus
}

var vendorTitle = map[string]string{"nvidia": "NVIDIA", "intel": "Intel", "amd": "AMD", "apple": "Apple", "other": "Other"}

func vendorName(pciVendor string) string {
	switch strings.ToLower(pciVendor) {
	case "0x10de":
		return "nvidia"
	case "0x8086":
		return "intel"
	case "0x1002", "0x1022":
		return "amd"
	}
	return "other"
}

// pciName is lspci's name for the PCI device at dir ("" without lspci).
func pciName(dir string) string {
	slot := ""
	for _, line := range strings.Split(readTrim(filepath.Join(dir, "uevent")), "\n") {
		if v, ok := strings.CutPrefix(line, "PCI_SLOT_NAME="); ok {
			slot = v
		}
	}
	if slot == "" {
		return ""
	}
	out, err := commandOutput(5*time.Second, "lspci", "-mm", "-s", slot)
	if err != nil {
		return ""
	}
	// `00:02.0 "VGA compatible controller" "Intel Corporation" "CoffeeLake-S GT2 [UHD Graphics 630]" …`
	fields := strings.Split(out, "\"")
	if len(fields) < 6 {
		return ""
	}
	return gpuName(pciVendor(fields[3]), fields[5])
}

// pciVendor is lspci's vendor name, shortened: "Intel Corporation" is "Intel", "Advanced Micro
// Devices, Inc. [AMD/ATI]" is "AMD".
func pciVendor(v string) string {
	if i, j := strings.Index(v, "["), strings.LastIndex(v, "]"); i >= 0 && j > i {
		v, _, _ = strings.Cut(v[i+1:j], "/")
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(v, " Corporation"), ", Inc."))
}

// gpuName is a GPU's name from lspci's vendor and device: the marketing name in the device's
// brackets, with the vendor in front unless it's there already, and the chip's codename after a
// name without a model number ("CoffeeLake-S GT2 [UHD Graphics 630]" is "Intel UHD Graphics 630",
// "Meteor Lake-P [Intel Graphics]" is "Intel Graphics (Meteor Lake-P)").
func gpuName(vendor, device string) string {
	name, codename := device, ""
	if i, j := strings.Index(device, "["), strings.LastIndex(device, "]"); i >= 0 && j > i {
		name, codename = device[i+1:j], strings.TrimSpace(device[:i])
	}
	name = strings.TrimSpace(name)
	if vendor != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(vendor)) {
		name = vendor + " " + name
	}
	if codename != "" && !strings.ContainsAny(name, "0123456789") {
		name += " (" + codename + ")"
	}
	return name
}

func readTrim(path string) string {
	b, _ := os.ReadFile(path)
	return strings.TrimSpace(string(b))
}

func commandOutput(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// ffmpegVersion is ffmpeg's version ("8.0.1-3ubuntu2"), "" without ffmpeg.
func ffmpegVersion() string {
	out, err := commandOutput(15*time.Second, "ffmpeg", "-hide_banner", "-version")
	if err != nil {
		return ""
	}
	// "ffmpeg version 8.0.1-3ubuntu2 Copyright (c) 2000-2025 the FFmpeg developers"
	if f := strings.Fields(out); len(f) >= 3 && f[0] == "ffmpeg" && f[1] == "version" {
		return f[2]
	}
	return ""
}

// ffmpegEncoders are the encoders this server's ffmpeg has.
func ffmpegEncoders() map[string]bool {
	has := map[string]bool{}
	out, err := commandOutput(15*time.Second, "ffmpeg", "-hide_banner", "-encoders")
	if err != nil {
		return has
	}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) >= 2 && len(f[0]) == 6 {
			has[f[1]] = true
		}
	}
	return has
}

// testEncode encodes a second of a test pattern as a transcode would, with encoder id on device.
func testEncode(id, device string) error {
	args := []string{"-hide_banner", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-frames:v", "30", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "23", "-pix_fmt", "yuv420p", "-f", "null", "-"}
	args = hardwareArgs(args, id, device)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("the test encode took too long")
	}
	// ffmpeg's last line says why ("No capable devices found", "Failed to initialise VAAPI…")
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	reason := strings.TrimSpace(lines[len(lines)-1])
	if reason == "" {
		reason = err.Error()
	}
	return fmt.Errorf("the test encode failed: %s", reason)
}

// hardwareChoice is the hardware encoder transcodes and downloads use now: the one Settings chose, if
// the test found it working.
func hardwareChoice() (id, device string, ok bool) {
	if config.GlobalConfig == nil {
		return "", "", false
	}
	s := config.GlobalConfig.Streaming
	if s.HardwareEncoder == "" {
		return "", "", false
	}
	for _, e := range Hardware().Encoders {
		if e.ID == s.HardwareEncoder && (e.Device == s.HardwareDevice || e.ID == "nvenc") {
			return e.ID, e.Device, true
		}
	}
	return "", "", false
}

// hardwareArgs turns a CPU transcode's ffmpeg arguments (libx264, -crf or -b:v, -pix_fmt yuv420p,
// filters in -vf or a -filter_complex ending in [vout]) into the same transcode encoded by id.
// Everything else (decoding, filters, audio, output) stays as it was.
func hardwareArgs(args []string, id, device string) []string {
	var pre, out []string
	bitrate := slices.Contains(args, "-b:v")
	hasFilter := slices.Contains(args, "-vf") || slices.Contains(args, "-filter_complex")
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case a == "-c:v" && next == "libx264":
			out = append(out, encoderCodecArgs(id, bitrate)...)
			if id == "vaapi" && !hasFilter {
				out = append(out, "-vf", "format=nv12,hwupload") // VA-API encodes frames on the GPU
			}
			i++
		case a == "-preset" || a == "-crf":
			if a == "-crf" {
				out = append(out, encoderQualityArgs(id, next)...)
			}
			i++
		case a == "-pix_fmt":
			switch id {
			case "qsv":
				out = append(out, "-pix_fmt", "nv12")
			case "vaapi": // the filter uploads the frames
			default:
				out = append(out, a, next)
			}
			i++
		case a == "-vf" && id == "vaapi":
			out = append(out, a, next+",format=nv12,hwupload")
			i++
		case a == "-filter_complex" && id == "vaapi" && strings.Contains(next, "[vout]"):
			out = append(out, a, strings.Replace(next, "[vout]", "[vsw];[vsw]format=nv12,hwupload[vout]", 1))
			i++
		default:
			out = append(out, a)
		}
	}
	switch id {
	case "vaapi":
		pre = []string{"-vaapi_device", device}
	case "qsv":
		if device != "" {
			pre = []string{"-qsv_device", device}
		}
	}
	return append(pre, out...)
}

func encoderCodecArgs(id string, bitrate bool) []string {
	switch id {
	case "nvenc":
		return []string{"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr"}
	case "qsv":
		return []string{"-c:v", "h264_qsv", "-preset", "veryfast"}
	case "videotoolbox":
		return []string{"-c:v", "h264_videotoolbox"}
	case "vaapi":
		if bitrate {
			return []string{"-c:v", "h264_vaapi", "-rc_mode", "VBR"}
		}
		return []string{"-c:v", "h264_vaapi", "-rc_mode", "CQP"}
	}
	return []string{"-c:v", "libx264", "-preset", "ultrafast"}
}

// qualityOffset is what each encoder adds to x264's CRF for the same size and look as x264
// veryfast: measured on Tears of Steel (1080p) at CRF 18 in 2026-11, NVENC on an RTX 4070 Ti SUPER
// (+5; a GTX 1060 needed +6), VA-API and Quick Sync on Meteor Lake. Taken 1:1, the GPU's files
// came out 1.3 to 2 times the size.
var qualityOffset = map[string]int{"nvenc": 5, "qsv": 1, "vaapi": 2}

// videoToolboxQuality is VideoToolbox's -q:v (1 to 100, higher is better; Apple Silicon only) for
// x264's CRF: 75 for CRF 18 and 2.5 per CRF step, measured as qualityOffset was, on an M1.
func videoToolboxQuality(crf int) int {
	return max(1, min(100, 75-(crf-18)*5/2))
}

// encoderQualityArgs is x264's -crf (constant quality) in id's terms.
func encoderQualityArgs(id, crf string) []string {
	q := crf
	if n, err := strconv.Atoi(crf); err == nil {
		q = strconv.Itoa(min(n+qualityOffset[id], 51))
		if id == "videotoolbox" {
			q = strconv.Itoa(videoToolboxQuality(n))
		}
	}
	switch id {
	case "nvenc":
		return []string{"-cq", q, "-b:v", "0"}
	case "qsv":
		return []string{"-global_quality", q}
	case "vaapi":
		return []string{"-qp", q}
	case "videotoolbox":
		return []string{"-q:v", q}
	}
	return []string{"-crf", crf}
}

// downloadsOnGPU counts the download encodes running on the GPU, which count toward
// streaming.hardwareMaxTranscodes with the streams.
var downloadsOnGPU atomic.Int32

// hardwareForDownload is the GPU encoder for a download's video when Settings chose one and it has
// room, counting the download in downloadsOnGPU: the caller subtracts it when the encode ends.
func hardwareForDownload() (id, device string, ok bool) {
	id, device, ok = hardwareChoice()
	if !ok {
		return "", "", false
	}
	transcodes.mu.Lock()
	defer transcodes.mu.Unlock()
	if !transcodes.hardwareRoom() {
		return "", "", false
	}
	downloadsOnGPU.Add(1)
	return id, device, true
}

// encoderLabel names the encoder in ffmpeg's arguments, for the Streams page.
func encoderLabel(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-c:v" {
			switch args[i+1] {
			case "h264_nvenc":
				return "NVIDIA NVENC"
			case "h264_qsv":
				return "Intel Quick Sync"
			case "h264_vaapi":
				return "VA-API"
			case "h264_videotoolbox":
				return "Apple VideoToolbox"
			default:
				return args[i+1]
			}
		}
	}
	return ""
}
