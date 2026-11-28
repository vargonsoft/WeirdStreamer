package media

import (
	"context"
	"log"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// HDR video (PQ: HDR10, HDR10+ and Dolby Vision with an HDR10 base; HLG) encoded as the 8-bit
// H.264 of a transcode or download keeps its HDR signal in SDR's place unless it's tone mapped:
// the picture comes out washed out and grey. So its encodes go through toneMapFilter, which
// brings it to BT.709 SDR: zscale (from zimg) and ffmpeg's tonemap with the Mobius curve, which
// keeps the SDR range's brightness and compresses only the highlights past it (Hable dimmed the
// whole picture: white came out at 60%).
//
// On the CPU it about doubles the cost of a 4K encode (10 s of 4K in 7 s here, 13 cores);
// scaled down first, a 1080p one costs less than an untouched 4K one. Dolby Vision profile 5 has
// no HDR10 base (its colors are IPT, which only libplacebo reads): it isn't helped.

// toneMapFilter turns HDR (PQ or HLG) into BT.709 SDR, ending in 8-bit 4:2:0.
const toneMapFilter = "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=mobius:param=0.5:desat=0," +
	"zscale=t=bt709:m=bt709:r=tv,format=yuv420p"

// HDR reports whether the video carries an HDR transfer (PQ or HLG).
func (v *VideoCodec) HDR() bool {
	return v != nil && (v.ColorTransfer == "smpte2084" || v.ColorTransfer == "arib-std-b67")
}

var toneMapCheck struct {
	once sync.Once
	ok   bool
}

// canToneMap reports whether ffmpeg has the filters toneMapFilter needs (zscale needs zimg, which
// some builds lack). Without them HDR is encoded as before, washed out, and the log says so once.
func canToneMap() bool {
	toneMapCheck.once.Do(func() {
		out, err := commandOutput(15*time.Second, "ffmpeg", "-hide_banner", "-filters")
		if err != nil {
			return
		}
		has := map[string]bool{}
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) >= 2 {
				has[f[1]] = true
			}
		}
		toneMapCheck.ok = has["zscale"] && has["tonemap"]
		if !toneMapCheck.ok {
			log.Printf("[TRANSCODE] ffmpeg has no zscale (zimg) or tonemap filter: HDR videos are transcoded without tone mapping (washed out)")
		}
	})
	return toneMapCheck.ok
}

// hdrFilter is toneMapFilter for an HDR video when ffmpeg can run it, else "". zscale refuses a
// frame whose matrix or primaries aren't marked ("no path between colorspaces"), so those an HDR
// video leaves out are taken as HDR's own, BT.2020.
func hdrFilter(v *VideoCodec) string {
	if !v.HDR() || !canToneMap() {
		return ""
	}
	var set []string
	if !known(v.ColorSpace) {
		set = append(set, "colorspace=bt2020nc")
	}
	if !known(v.ColorPrimaries) {
		set = append(set, "color_primaries=bt2020")
	}
	if len(set) > 0 {
		return "setparams=" + strings.Join(set, ":") + "," + toneMapFilter
	}
	return toneMapFilter
}

// known reports whether ffprobe named a color property ("unknown" and "reserved" are none).
func known(s string) bool { return s != "" && s != "unknown" && s != "reserved" }

// joinFilters chains the filters given, leaving out empty ones.
func joinFilters(filters ...string) string {
	var out []string
	for _, f := range filters {
		if f != "" {
			out = append(out, f)
		}
	}
	return strings.Join(out, ",")
}

// On the GPU: libplacebo over Vulkan tone maps HDR for transcodes and downloads whose video a
// hardware encoder makes (the CPU's arguments, which run when the GPU fails, keep toneMapFilter).
// It matched the SDR grade of the same footage better than toneMapFilter (Jellyfin's test videos:
// a mean difference of 3 against 7 levels), took a tenth of the CPU (115% against 1229% for 4K
// with NVENC on a GTX 1060, as fast), and reads Dolby Vision profile 5, whose colors the CPU
// filters can't: so those videos are tone mapped only on the GPU. Frames go up to the first Vulkan
// GPU after the CPU's decode and other filters and come back down for the encoder. That works for
// NVENC and Quick Sync, which take frames from memory, but not for VA-API, whose frames must go up
// to its own device afterwards: ffmpeg's filters have one device to upload to, and on ws's Meteor
// Lake neither mapping VA-API frames to Vulkan nor uploading to a Vulkan device derived from VA-API's
// worked, nor deriving VA-API from Vulkan. Intel's own tonemap_vaapi refuses HDR10 without mastering
// display data. So VA-API's transcodes tone map on the CPU: choose Quick Sync for HDR on Intel. On
// ws (10 s of 4K HDR10): libplacebo with Quick Sync 7.9 s and 142% CPU; the CPU's with VA-API
// 12.2 s and 581%, slower than playing.

// libplaceboFilter makes BT.709 SDR of whatever HDR the frames carry (PQ, HLG, Dolby Vision).
const libplaceboFilter = "libplacebo=format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv"

// gpuToneMapFilter is the filter chain: from the CPU's frames to the CPU's frames.
const gpuToneMapFilter = "hwupload," + libplaceboFilter + ",hwdownload,format=yuv420p"

// gpuToneMapDevice is the ffmpeg options that give gpuToneMapFilter its Vulkan device.
var gpuToneMapDevice = []string{"-init_hw_device", "vulkan=tonemap", "-filter_hw_device", "tonemap"}

// needsGPUToneMap reports whether a video is tone mapped on the GPU when its encode runs there:
// HDR, or Dolby Vision profile 5 (no HDR10 base, so not HDR() by its transfer).
func needsGPUToneMap(v *VideoCodec) bool {
	return v.HDR() || (v != nil && v.DolbyVision == 5)
}

// gpuToneMap turns a hardware transcode's arguments (hardwareArgs') for filePath into ones that
// tone map on the GPU, when the encoder passed the startup check: the CPU's toneMapFilter is
// replaced by gpuToneMapFilter (a Dolby Vision profile 5 video, which has none, gets it first in
// its filters). Otherwise they're returned as they are.
func gpuToneMap(args []string, filePath, id string) []string {
	if !encoderToneMaps(id) {
		return args
	}
	c, err := ProbeCodecs(filePath)
	if err != nil || !needsGPUToneMap(c.Video) {
		return args
	}
	return withGPUToneMap(args)
}

// withGPUToneMap is gpuToneMap's change to the arguments.
func withGPUToneMap(args []string) []string {
	gpu := gpuToneMapFilter
	out := slices.Clone(args)
	done := false
	for i := 0; i+1 < len(out) && !done; i++ {
		switch out[i] {
		case "-vf":
			if strings.Contains(out[i+1], toneMapFilter) {
				out[i+1] = strings.Replace(out[i+1], toneMapFilter, gpu, 1)
			} else {
				out[i+1] = gpu + "," + out[i+1]
			}
			done = true
		case "-filter_complex":
			if strings.Contains(out[i+1], toneMapFilter) {
				out[i+1] = strings.Replace(out[i+1], toneMapFilter, gpu, 1)
			} else if strings.HasPrefix(out[i+1], "[0:v:0]") {
				out[i+1] = "[0:v:0]" + gpu + "," + strings.TrimPrefix(out[i+1], "[0:v:0]")
			} else {
				return args // a graph this doesn't know: the CPU's tone mapping (if any) stays
			}
			done = true
		}
	}
	if !done {
		at := slices.Index(out, "-c:v")
		if at < 0 {
			return args
		}
		out = slices.Insert(out, at, "-vf", gpu)
	}
	return append(slices.Clone(gpuToneMapDevice), out...)
}

// encoderToneMaps reports whether the hardware encoder id tone maps on the GPU (the startup check).
func encoderToneMaps(id string) bool {
	for _, e := range Hardware().Encoders {
		if e.ID == id && e.ToneMap != "" {
			return true
		}
	}
	return false
}

// vulkanDevice finds the device ffmpeg's Vulkan picked in its verbose output: its name, and its
// kind ("integrated", "discrete", "software"…).
var vulkanDevice = regexp.MustCompile(`Device \d+ selected: (.+) \((\w+)\) \(0x[0-9a-f]+\)`)

// gpuToneMapCheck runs gpuToneMapFilter on a moment of made-up HDR10 and says which GPU did it for
// the encoder id: "" for VA-API (see above), when it failed, or when Vulkan picked a software
// device (llvmpipe).
func gpuToneMapCheck(id string) string {
	if id == "vaapi" || id == "videotoolbox" {
		return ""
	}
	args := []string{"-hide_banner", "-v", "verbose"}
	args = append(args, gpuToneMapDevice...)
	args = append(args, "-f", "lavfi", "-i",
		"testsrc2=size=256x144:rate=10:duration=0.3,format=yuv420p10le,setparams=color_trc=smpte2084:color_primaries=bt2020:colorspace=bt2020nc",
		"-vf", gpuToneMapFilter, "-f", "null", "-")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	name := ""
	for _, line := range strings.Split(string(out), "\n") {
		if m := vulkanDevice.FindStringSubmatch(line); m != nil {
			if m[2] == "software" || m[2] == "cpu" || strings.Contains(strings.ToLower(m[1]), "llvmpipe") {
				return ""
			}
			name = m[1]
		}
	}
	return name // "" when ffmpeg named no device
}
