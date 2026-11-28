package media

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An HDR10 video is found HDR, and an encode through toneMapFilter comes out as 8-bit BT.709 SDR.
func TestToneMapHDR10(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	if !canToneMap() {
		t.Skip("ffmpeg has no zscale or tonemap")
	}
	dir := t.TempDir()
	hdr := filepath.Join(dir, "hdr10.mkv")
	out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:log-level=error",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", hdr).CombinedOutput()
	if err != nil {
		t.Skipf("can't make an HDR10 clip (libx265?): %v %s", err, out)
	}
	c, err := ProbeCodecs(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Video.HDR() || hdrFilter(c.Video) == "" {
		t.Fatalf("HDR10 not found HDR: %+v", c.Video)
	}

	sdr := filepath.Join(dir, "sdr.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", hdr, "-vf", hdrFilter(c.Video),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", sdr).CombinedOutput(); err != nil {
		t.Fatalf("tone mapping failed: %v %s", err, out)
	}
	probe, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=pix_fmt,color_transfer,color_primaries", "-of", "json", sdr).Output()
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Streams []struct {
			PixFmt    string `json:"pix_fmt"`
			Transfer  string `json:"color_transfer"`
			Primaries string `json:"color_primaries"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(probe, &p); err != nil || len(p.Streams) != 1 {
		t.Fatalf("probe: %v %s", err, probe)
	}
	if s := p.Streams[0]; s.PixFmt != "yuv420p" || s.Transfer != "bt709" || s.Primaries != "bt709" {
		t.Errorf("tone mapped video is %+v, want 8-bit BT.709", s)
	}
	sdrCodecs, err := ProbeCodecs(sdr)
	if err != nil {
		t.Fatal(err)
	}
	if sdrCodecs.Video.HDR() || hdrFilter(sdrCodecs.Video) != "" {
		t.Errorf("SDR video found HDR: %+v", sdrCodecs.Video)
	}
}

// An HDR video that marks only its transfer is tone mapped too (zscale needs the matrix and
// primaries, taken as BT.2020).
func TestToneMapUnmarkedMatrix(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	if !canToneMap() {
		t.Skip("ffmpeg has no zscale or tonemap")
	}
	dir := t.TempDir()
	hdr := filepath.Join(dir, "pq-only.mkv")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24", "-t", "1",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "transfer=smpte2084:log-level=error", "-color_trc", "smpte2084", hdr).CombinedOutput(); err != nil {
		t.Skipf("can't make an HDR clip (libx265?): %v %s", err, out)
	}
	c, err := ProbeCodecs(hdr)
	if err != nil {
		t.Fatal(err)
	}
	f := hdrFilter(c.Video)
	if !strings.HasPrefix(f, "setparams=") {
		t.Fatalf("filter %q for %+v doesn't set the missing matrix and primaries", f, c.Video)
	}
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", hdr, "-vf", f, "-c:v", "libx264", "-preset", "ultrafast",
		"-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("tone mapping failed: %v %s", err, out)
	}
}

// Burned-in subtitles go over the filtered video (tone mapped, scaled), not under the filters.
func TestMapArgsFiltersBeforeSubtitles(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "a.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(dir, "subs.mkv")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10",
		"-i", srt, "-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-c:s", "srt", video).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	args := TrackOptions{Audio: -1, Subtitle: 0}.mapArgs(video, "scale=-2:720")
	graph := ""
	for i, a := range args {
		if a == "-filter_complex" {
			graph = args[i+1]
		}
	}
	if !strings.HasPrefix(graph, "[0:v:0]scale=-2:720,split[v][ref];[0:s:0][ref]scale=rw:rh[s];[v][s]overlay") {
		t.Errorf("graph %q doesn't filter the video before the subtitles go over it", graph)
	}
}

// A hardware transcode of HDR swaps the CPU's tone mapping for libplacebo's, with its Vulkan
// device; a Dolby Vision profile 5 video (no CPU tone mapping) gets it before the encoder; an encoder
// without GPU tone mapping (VA-API, a failed check) keeps the CPU's.
func TestGPUToneMapArgs(t *testing.T) {
	hdr := filepath.Join(t.TempDir(), "hdr.mkv")
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-t", "1",
		"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le",
		"-x265-params", "colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:log-level=error", "-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", hdr).CombinedOutput(); err != nil {
		t.Skipf("can't make an HDR clip (libx265?): %v %s", err, out)
	}
	hardware.once.Do(func() {}) // no detection: the report below is what was found
	hardware.Lock()
	saved := hardware.report
	hardware.report = HardwareReport{Encoders: []HardwareEncoder{{ID: "nvenc", ToneMap: "Some GPU"}, {ID: "vaapi", Device: "/dev/dri/renderD128"}, {ID: "qsv"}}}
	hardware.Unlock()
	t.Cleanup(func() { hardware.Lock(); hardware.report = saved; hardware.Unlock() })

	cpu := []string{"-i", hdr, "-c:v", "libx264", "-preset", "ultrafast", "-crf", "23", "-vf", "scale=-2:720," + toneMapFilter, "-pix_fmt", "yuv420p", "-f", "mp4", "pipe:1"}
	join := func(a []string) string { return strings.Join(a, " ") }

	nv := join(gpuToneMap(hardwareArgs(cpu, "nvenc", ""), hdr, "nvenc"))
	if !strings.HasPrefix(nv, "-init_hw_device vulkan=tonemap -filter_hw_device tonemap ") ||
		!strings.Contains(nv, "-vf scale=-2:720,hwupload,"+libplaceboFilter+",hwdownload,format=yuv420p ") || strings.Contains(nv, "zscale") {
		t.Errorf("NVENC: %s", nv)
	}
	if va := join(gpuToneMap(hardwareArgs(cpu, "vaapi", "/dev/dri/renderD128"), hdr, "vaapi")); !strings.Contains(va, toneMapFilter+",format=nv12,hwupload") {
		t.Errorf("VA-API lost the CPU's tone mapping: %s", va)
	}
	if qsv := gpuToneMap(hardwareArgs(cpu, "qsv", ""), hdr, "qsv"); !strings.Contains(join(qsv), toneMapFilter) {
		t.Errorf("Quick Sync without GPU tone mapping lost the CPU's: %s", join(qsv))
	}
	sdr := []string{"-i", hdr, "-c:v", "libx264", "-crf", "23", "-pix_fmt", "yuv420p", "pipe:1"}
	// A video the CPU leaves alone that needs the GPU's (Dolby Vision profile 5) gets it before the encoder.
	if got := join(withGPUToneMap(sdr)); !strings.Contains(got, "-vf hwupload,"+libplaceboFilter+",hwdownload,format=yuv420p -c:v libx264") {
		t.Errorf("inserted: %s", got)
	}
}
