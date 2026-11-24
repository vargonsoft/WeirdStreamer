package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A side-by-side 3D video marked in both its MKV and its H.264 SEI: ffmpeg 8 can't decode it as it
// is, but with the SEI left out on input (the file untouched) it gets a thumbnail and transcodes.
// An ordinary H.264 video isn't taken for one.
func TestStereoMarkedTwice(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	sbs := filepath.Join(dir, "Film 3D.mkv")
	plain := filepath.Join(dir, "Film.mkv")
	for _, c := range []struct {
		path  string
		extra []string
	}{
		{sbs, []string{"-x264-params", "frame-packing=3", "-metadata:s:v:0", "stereo_mode=left_right"}},
		{plain, nil},
	} {
		args := append([]string{"-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=320x180:r=10:d=90",
			"-c:v", "libx264", "-preset", "ultrafast"}, c.extra...)
		if out, err := exec.Command("ffmpeg", append(args, c.path)...).CombinedOutput(); err != nil {
			t.Fatalf("making %s: %v %s", c.path, err, out)
		}
	}
	if !stereoMarked(sbs) || stereoMarked(plain) {
		t.Fatalf("3D: %v, plain: %v", stereoMarked(sbs), stereoMarked(plain))
	}
	if exec.Command("ffmpeg", "-v", "error", "-ss", "60", "-i", sbs, "-frames:v", "1", "-f", "null", "-").Run() == nil {
		t.Log("this ffmpeg decodes it as it is (no doubled marker problem); checking the filter is harmless")
	}
	part := filepath.Join(dir, "thumb.jpg")
	if !videoThumbnail(sbs, part, "h264") {
		t.Fatal("no thumbnail for the 3D video")
	}
	os.Remove(part)
	// A transcode as the streamer builds it (the filter before -i)
	args := append(inputFilters(sbs, ""), "-v", "error", "-ss", "30", "-i", sbs, "-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-f", "null", "-")
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("transcoding the 3D video: %v %s", err, out)
	}
	// The file isn't touched
	if !stereoMarked(sbs) {
		t.Fatal("the 3D video changed")
	}
}
