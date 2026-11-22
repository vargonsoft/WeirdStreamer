package media

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/weirdstreamer/server/internal/config"
)

// Graphics hardware for streaming transcodes. At startup the server looks for GPUs and tries a
// short test encode with each encoder ffmpeg has for them; Settings offers those that worked
// (streaming.hardwareEncoder). The GPU only encodes: decoding and filters (scaling, burned-in
// subtitles, 10-bit to 8-bit) stay on the CPU, so every source the CPU transcodes works the same.
// A hardware transcode that fails before making anything is run again on the CPU.

// GPU is a graphics device the server found.
type GPU struct {
	Name   string `json:"name"`             // "NVIDIA GeForce GTX 1060 6GB", "Intel UHD Graphics 630"
	Vendor string `json:"vendor"`           // nvidia, intel, amd, other
	Device string `json:"device,omitempty"` // its render node (/dev/dri/renderD128), where it has one
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
			names := []string{}
			for _, e := range r.Encoders {
				names = append(names, e.Name+" on "+e.GPU)
			}
			if len(names) == 0 {
				names = append(names, "none")
			}
			log.Printf("[HARDWARE] %d GPU(s); hardware encoders: %s", len(r.GPUs), strings.Join(names, ", "))
			for _, p := range r.Problems {
				log.Printf("[HARDWARE] %s on %s doesn't work: %s", p.Encoder, p.GPU, p.Reason)
			}
		}()
	})
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

var encoderNames = map[string]string{"nvenc": "NVENC", "qsv": "Quick Sync", "vaapi": "VA-API"}

func detectHardware() HardwareReport {
	r := HardwareReport{GPUs: findGPUs(), Encoders: []HardwareEncoder{}}
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
		}
	}
	return r
}

// findGPUs lists the GPUs: each DRM render node's (Linux), and NVIDIA's by nvidia-smi.
func findGPUs() []GPU {
	gpus := []GPU{}
	nodes, _ := filepath.Glob("/sys/class/drm/renderD*")
	slices.Sort(nodes)
	for _, node := range nodes {
		vendor := readTrim(filepath.Join(node, "device", "vendor"))
		g := GPU{Device: "/dev/dri/" + filepath.Base(node), Vendor: vendorName(vendor)}
		g.Name = pciName(filepath.Join(node, "device"))
		if g.Name == "" {
			g.Name = vendorTitle[g.Vendor] + " GPU (" + filepath.Base(node) + ")"
		}
		gpus = append(gpus, g)
	}
	// NVIDIA's own name for its cards ("NVIDIA GeForce GTX 1060 6GB"), and a card without a
	// render node (other systems).
	if out, err := commandOutput(5*time.Second, "nvidia-smi", "--query-gpu=name", "--format=csv,noheader"); err == nil {
		names := strings.Split(strings.TrimSpace(out), "\n")
		i := 0
		for j := range gpus {
			if gpus[j].Vendor == "nvidia" && i < len(names) {
				gpus[j].Name = strings.TrimSpace(names[i])
				i++
			}
		}
		for ; i < len(names); i++ {
			if n := strings.TrimSpace(names[i]); n != "" {
				gpus = append(gpus, GPU{Name: n, Vendor: "nvidia"})
			}
		}
	}
	return gpus
}

var vendorTitle = map[string]string{"nvidia": "NVIDIA", "intel": "Intel", "amd": "AMD", "other": "Other"}

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
	vendor, device := fields[3], fields[5]
	vendor = strings.TrimSuffix(strings.TrimSuffix(vendor, " Corporation"), ", Inc.")
	if i, j := strings.Index(device, "["), strings.LastIndex(device, "]"); i >= 0 && j > i {
		device = device[i+1 : j] // the marketing name
	}
	return strings.TrimSpace(vendor + " " + device)
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

// hardwareChoice is the hardware encoder streaming transcodes use now: the one Settings chose, if
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
	case "vaapi":
		if bitrate {
			return []string{"-c:v", "h264_vaapi", "-rc_mode", "VBR"}
		}
		return []string{"-c:v", "h264_vaapi", "-rc_mode", "CQP"}
	}
	return []string{"-c:v", "libx264", "-preset", "ultrafast"}
}

// encoderQualityArgs is x264's -crf (constant quality) in id's terms.
func encoderQualityArgs(id, crf string) []string {
	switch id {
	case "nvenc":
		return []string{"-cq", crf, "-b:v", "0"}
	case "qsv":
		return []string{"-global_quality", crf}
	case "vaapi":
		return []string{"-qp", crf}
	}
	return []string{"-crf", crf}
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
			default:
				return args[i+1]
			}
		}
	}
	return ""
}
