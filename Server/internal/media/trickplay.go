package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/weirdstreamer/server/internal/config"
	"github.com/weirdstreamer/server/internal/database"
)

type TrickplayGenerator struct {
	db *database.DB
}

func NewTrickplayGenerator(db *database.DB) *TrickplayGenerator {
	return &TrickplayGenerator{
		db: db,
	}
}

func (tg *TrickplayGenerator) Generate(videoPath string) error {
	fmt.Printf("Generating trickplay frames for %s\n", videoPath)

	duration, codec, err := tg.getVideoInfo(videoPath)
	if err != nil {
		return fmt.Errorf("failed to get video info: %w", err)
	}

	trickplayDir := config.GlobalConfig.Media.TrickplayPath
	if trickplayDir == "" {
		return fmt.Errorf("trickplay path is not configured")
	}

	hash := GenerateThumbnailHash(videoPath)
	trickplayVideoDir := filepath.Join(trickplayDir, hash)

	if err := os.MkdirAll(trickplayVideoDir, 0755); err != nil {
		return fmt.Errorf("failed to create trickplay directory: %w", err)
	}

	// Generate frames every 10 seconds
	for i := 0; i <= int(duration); i += 10 {
		if err := tg.generateFrame(videoPath, trickplayVideoDir, i, codec); err != nil {
			fmt.Printf("Warning: failed to generate frame at %ds: %v\n", i, err)
		}
	}
	// Ensure the very last frame is generated
	if int(duration)%10 != 0 {
		if err := tg.generateFrame(videoPath, trickplayVideoDir, int(duration), codec); err != nil {
			fmt.Printf("Warning: failed to generate last frame: %v\n", err)
		}
	}

	fmt.Printf("Finished generating trickplay frames for %s\n", videoPath)

	return nil
}

func (tg *TrickplayGenerator) GenerateForDirectory(dirPath string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && isSupportedVideo(path) {
			if err := tg.Generate(path); err != nil {
				fmt.Printf("Error generating trickplay frames for %s: %v\n", path, err)
			}
		}
		return nil
	})
}

func isSupportedVideo(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, videoExt := range config.GlobalConfig.Media.SupportedVideoFormats {
		if ext == videoExt {
			return true
		}
	}
	return false
}

func (tg *TrickplayGenerator) getVideoInfo(videoPath string) (float64, string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name:format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("ffprobe failed: %s", string(output))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return 0, "", fmt.Errorf("ffprobe returned insufficient data")
	}

	codec := strings.TrimSpace(lines[0])
	duration, err := strconv.ParseFloat(strings.TrimSpace(lines[1]), 64)
	if err != nil {
		// Try swapped order
		codec = strings.TrimSpace(lines[1])
		duration, err = strconv.ParseFloat(strings.TrimSpace(lines[0]), 64)
		if err != nil {
			return 0, "", fmt.Errorf("failed to parse duration from ffprobe: %w", err)
		}
	}

	return duration, codec, nil
}

func (tg *TrickplayGenerator) generateFrame(videoPath, outputDir string, timeInSeconds int, codec string) error {
	framePath := filepath.Join(outputDir, fmt.Sprintf("frame-%d.jpg", timeInSeconds))

	if _, err := os.Stat(framePath); err == nil {
		return nil
	}

	// Format timestamp as HH:MM:SS.mmm
	h := timeInSeconds / 3600
	m := (timeInSeconds % 3600) / 60
	s := timeInSeconds % 60
	timestamp := fmt.Sprintf("%02d:%02d:%02d.000", h, m, s)

	// Using the specific command structure confirmed to work for problematic Android recordings:
	// ffmpeg [BSF] [Seek] [-noaccurate_seek] -i [Input] [Filters] [Output]
	var args []string
	if codec == "h264" {
		args = append(args, "-bsf:v", "h264_metadata=matrix_coefficients=1")
	} else if codec == "hevc" {
		args = append(args, "-bsf:v", "hevc_metadata=matrix_coefficients=1")
	}

	args = append(args, "-ss", timestamp, "-noaccurate_seek", "-i", videoPath)
	args = append(args, "-vf", "scale=320:-1", "-frames:v", "1", "-q:v", "5", "-y", framePath)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("ffmpeg failed: %s", string(output))
	}

	return nil
}
