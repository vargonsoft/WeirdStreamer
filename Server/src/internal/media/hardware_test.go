package media

import (
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
