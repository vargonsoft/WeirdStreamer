package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func jpegOf(t *testing.T, fill func(x, y int) color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, fill(x, y))
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFrameBlank(t *testing.T) {
	cases := []struct {
		name  string
		fill  func(x, y int) color.Color
		blank bool
	}{
		{"black", func(x, y int) color.Color { return color.Black }, true},
		{"white", func(x, y int) color.Color { return color.White }, true},
		{"one blue", func(x, y int) color.Color { return color.RGBA{30, 60, 200, 255} }, true},
		{"grey", func(x, y int) color.Color { return color.Gray{128} }, true},
		{"gradient", func(x, y int) color.Color { return color.Gray{uint8(x * 255 / 320)} }, false},
		{"color bars", func(x, y int) color.Color {
			return []color.Color{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{200, 200, 0, 255}}[x*4/320]
		}, false},
		// Nearly white with a small dark logo: spread enough, but nearly all one color
		{"white card with a logo", func(x, y int) color.Color {
			if x > 130 && x < 190 && y > 70 && y < 110 {
				return color.Gray{20}
			}
			return color.Gray{235}
		}, true},
		// A dark scene with something lit in it still counts as a picture
		{"dark scene", func(x, y int) color.Color {
			if x > 100 && x < 220 && y > 60 && y < 120 {
				return color.Gray{180}
			}
			return color.Gray{uint8(10 + x*60/320)} // a real background's shading, not one flat color
		}, false},
	}
	for _, c := range cases {
		blank, _ := frameBlank(jpegOf(t, c.fill))
		if blank != c.blank {
			t.Errorf("%s: blank %v, want %v", c.name, blank, c.blank)
		}
	}
}

// A video black for its first 100 seconds gets its thumbnail from further in (2 minutes), and one too
// short for a minute gets one nearer the start. One black throughout gets none, as before: its frames
// are JPEGs too small to keep (under 1 KB, which the thumbnail checks take for a broken one).
func TestVideoThumbnailSkipsBlank(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	dir := t.TempDir()
	make := func(name string, args ...string) string {
		path := filepath.Join(dir, name)
		cmd := exec.Command("ffmpeg", append(append([]string{"-v", "error"}, args...), "-c:v", "libx264", "-preset", "ultrafast", "-g", "10", "-y", path)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("making %s: %v %s", name, err, out)
		}
		return path
	}
	blackThenBars := make("late.mkv", "-f", "lavfi", "-i", "color=black:s=160x90:r=5:d=100", "-f", "lavfi", "-i", "testsrc=s=160x90:r=5:d=100",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1")
	allBlack := make("black.mkv", "-f", "lavfi", "-i", "color=black:s=160x90:r=5:d=240")
	short := make("short.mkv", "-f", "lavfi", "-i", "testsrc=s=160x90:r=5:d=20")

	for _, c := range []struct {
		path  string
		blank bool // the thumbnail is still blank (every frame was)
	}{{blackThenBars, false}, {short, false}} {
		part := filepath.Join(dir, "thumb.jpg")
		if !videoThumbnail(c.path, part, "h264") {
			t.Fatalf("%s: no thumbnail", filepath.Base(c.path))
		}
		data, err := os.ReadFile(part)
		if err != nil {
			t.Fatal(err)
		}
		if blank, _ := frameBlank(data); blank != c.blank {
			t.Errorf("%s: thumbnail blank %v, want %v", filepath.Base(c.path), blank, c.blank)
		}
		os.Remove(part)
	}
	if videoThumbnail(allBlack, filepath.Join(dir, "thumb.jpg"), "h264") {
		t.Error("black.mkv: a thumbnail of a black video")
	}
}
