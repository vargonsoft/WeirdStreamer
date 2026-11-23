package media

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

func TestHardwareArgs(t *testing.T) {
	cpu := "-ss 10 -i in.mkv -c:v libx264 -preset ultrafast -crf 23 -pix_fmt yuv420p -c:a aac -movflags frag_keyframe -f mp4 pipe:1"
	scaled := "-i in.mkv -map 0:v:0 -vf scale=-2:720 -c:v libx264 -preset ultrafast -b:v 4000000 -maxrate 4000000 -pix_fmt yuv420p -f mp4 pipe:1"
	burned := "-i in.mkv -filter_complex [0:v:0]split[v][ref];[0:s:0][ref]scale=rw:rh[s];[v][s]overlay[vout] -map [vout] -c:v libx264 -preset ultrafast -crf 23 -pix_fmt yuv420p -f mp4 pipe:1"
	for _, c := range []struct{ name, args, id, device, want string }{
		{"nvenc quality", cpu, "nvenc", "",
			"-ss 10 -i in.mkv -c:v h264_nvenc -preset p4 -rc vbr -cq 28 -b:v 0 -pix_fmt yuv420p -c:a aac -movflags frag_keyframe -f mp4 pipe:1"},
		{"nvenc bitrate", scaled, "nvenc", "",
			"-i in.mkv -map 0:v:0 -vf scale=-2:720 -c:v h264_nvenc -preset p4 -rc vbr -b:v 4000000 -maxrate 4000000 -pix_fmt yuv420p -f mp4 pipe:1"},
		{"qsv", cpu, "qsv", "/dev/dri/renderD128",
			"-qsv_device /dev/dri/renderD128 -ss 10 -i in.mkv -c:v h264_qsv -preset veryfast -global_quality 24 -pix_fmt nv12 -c:a aac -movflags frag_keyframe -f mp4 pipe:1"},
		{"videotoolbox quality", cpu, "videotoolbox", "",
			"-ss 10 -i in.mkv -c:v h264_videotoolbox -q:v 63 -pix_fmt yuv420p -c:a aac -movflags frag_keyframe -f mp4 pipe:1"},
		{"videotoolbox bitrate", scaled, "videotoolbox", "",
			"-i in.mkv -map 0:v:0 -vf scale=-2:720 -c:v h264_videotoolbox -b:v 4000000 -maxrate 4000000 -pix_fmt yuv420p -f mp4 pipe:1"},
		{"vaapi without filters", cpu, "vaapi", "/dev/dri/renderD129",
			"-vaapi_device /dev/dri/renderD129 -ss 10 -i in.mkv -c:v h264_vaapi -rc_mode CQP -vf format=nv12,hwupload -qp 25 -c:a aac -movflags frag_keyframe -f mp4 pipe:1"},
		{"vaapi after a scale", scaled, "vaapi", "/dev/dri/renderD128",
			"-vaapi_device /dev/dri/renderD128 -i in.mkv -map 0:v:0 -vf scale=-2:720,format=nv12,hwupload -c:v h264_vaapi -rc_mode VBR -b:v 4000000 -maxrate 4000000 -f mp4 pipe:1"},
		{"vaapi after burned-in subtitles", burned, "vaapi", "/dev/dri/renderD128",
			"-vaapi_device /dev/dri/renderD128 -i in.mkv -filter_complex [0:v:0]split[v][ref];[0:s:0][ref]scale=rw:rh[s];[v][s]overlay[vsw];[vsw]format=nv12,hwupload[vout] -map [vout] -c:v h264_vaapi -rc_mode CQP -qp 25 -f mp4 pipe:1"},
	} {
		got := strings.Join(hardwareArgs(strings.Fields(c.args), c.id, c.device), " ")
		if got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
	}
}

func TestEncodesVideo(t *testing.T) {
	if !encodesVideo(strings.Fields("-i a -c:v libx264 -f mp4 pipe:1")) || encodesVideo(strings.Fields("-i a -c:v copy -f mp4 pipe:1")) {
		t.Error("encodesVideo")
	}
}

func TestDownloadsCountTowardGPULimit(t *testing.T) {
	saved := config.GlobalConfig
	defer func() { config.GlobalConfig = saved }()
	config.GlobalConfig = &config.Config{Streaming: config.StreamingConfig{HardwareMaxTranscodes: 1}}
	c := &transcodeCache{spools: map[*transcodeSpool]bool{}}
	if !c.hardwareRoom() {
		t.Fatal("no room with nothing on the GPU")
	}
	downloadsOnGPU.Add(1)
	defer downloadsOnGPU.Add(-1)
	if c.hardwareRoom() {
		t.Error("a download on the GPU didn't count toward the limit")
	}
}

func TestVideoToolboxQuality(t *testing.T) {
	for crf, want := range map[int]int{18: 75, 20: 70, 23: 63, 0: 100, 51: 1} {
		if got := videoToolboxQuality(crf); got != want {
			t.Errorf("CRF %d: got %d, want %d", crf, got, want)
		}
	}
}

func TestGPUName(t *testing.T) {
	for _, c := range [][3]string{
		{"Intel", "CoffeeLake-S GT2 [UHD Graphics 630]", "Intel UHD Graphics 630"},
		{"Intel", "Meteor Lake-P [Intel Graphics]", "Intel Graphics (Meteor Lake-P)"},
		{pciVendor("Advanced Micro Devices, Inc. [AMD/ATI]"), "Navi 22 [Radeon RX 6700/6700 XT/6750 XT / 6800M/6850M XT]", "AMD Radeon RX 6700/6700 XT/6750 XT / 6800M/6850M XT"},
		{pciVendor("Intel Corporation"), "Meteor Lake-P [Intel Graphics]", "Intel Graphics (Meteor Lake-P)"},
		{"NVIDIA", "GP106 [GeForce GTX 1060 6GB]", "NVIDIA GeForce GTX 1060 6GB"},
		{"Intel", "Device 7d55", "Intel Device 7d55"},
	} {
		if got := gpuName(c[0], c[1]); got != c[2] {
			t.Errorf("%s %q: got %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestHardwareLimitKeepsToTheDriver(t *testing.T) {
	saved := config.GlobalConfig
	defer func() { config.GlobalConfig = saved }()
	hardware.once.Do(func() {}) // no detection: the report below is what was found
	hardware.Lock()
	savedReport := hardware.report
	hardware.report = HardwareReport{Encoders: []HardwareEncoder{{ID: "nvenc", Name: "NVENC", Sessions: 8}}}
	hardware.Unlock()
	defer func() { hardware.Lock(); hardware.report = savedReport; hardware.Unlock() }()
	for _, c := range []struct{ setting, want int }{{0, 8}, {12, 8}, {3, 3}} {
		config.GlobalConfig = &config.Config{Streaming: config.StreamingConfig{HardwareEncoder: "nvenc", HardwareMaxTranscodes: c.setting}}
		if got := hardwareLimit(); got != c.want {
			t.Errorf("setting %d: limit %d, want %d", c.setting, got, c.want)
		}
	}
}

func TestFindGPUsSkipsVirtualFunctions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("render nodes are Linux's")
	}
	savedClass, savedDev := drmClass, devDRI
	defer func() { drmClass, devDRI = savedClass, savedDev }()
	// A GPU (00:02.0) with two SR-IOV virtual functions (00:02.1, 00:02.2), each a render node.
	root := t.TempDir()
	drmClass, devDRI = filepath.Join(root, "class"), filepath.Join(root, "dev")
	pci := filepath.Join(root, "pci")
	for i, n := range []string{"renderD128", "renderD129", "renderD130"} {
		dev := filepath.Join(pci, fmt.Sprintf("0000:00:02.%d", i))
		os.MkdirAll(dev, 0o755)
		os.WriteFile(filepath.Join(dev, "vendor"), []byte("0x8086\n"), 0o644)
		if i > 0 {
			os.Symlink(filepath.Join(pci, "0000:00:02.0"), filepath.Join(dev, "physfn"))
		}
		os.MkdirAll(filepath.Join(drmClass, n), 0o755)
		os.Symlink(dev, filepath.Join(drmClass, n, "device"))
	}
	nodes := func(dev ...string) []string { // the render nodes found, with these device files there
		os.RemoveAll(devDRI)
		os.MkdirAll(devDRI, 0o755)
		for _, d := range dev {
			os.WriteFile(filepath.Join(devDRI, d), nil, 0o644)
		}
		var got []string // nvidia-smi may add this machine's own cards: only render nodes count
		for _, g := range findGPUs() {
			if g.Device != "" {
				got = append(got, filepath.Base(g.Device))
			}
		}
		return got
	}
	if got := nodes("renderD128", "renderD129", "renderD130"); !slices.Equal(got, []string{"renderD128"}) {
		t.Errorf("the host: %v, want the GPU only", got)
	}
	if got := nodes("renderD130"); !slices.Equal(got, []string{"renderD130"}) {
		t.Errorf("a container given a virtual function: %v, want it", got)
	}
}

func TestFFmpegMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := ffmpegEncoders(); err == nil || !strings.Contains(err.Error(), "isn't installed") {
		t.Fatalf("got %v, want ffmpeg isn't installed", err)
	}
}
