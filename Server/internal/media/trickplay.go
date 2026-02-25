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

	// Generate frames every 10 seconds.
	// Stop 1 second before the end to avoid empty output issues with input seeking.
	for i := 0; i <= int(duration)-1; i += 10 {
		if err := tg.generateFrame(videoPath, trickplayVideoDir, float64(i), i, codec); err != nil {
			fmt.Printf("Warning: failed to generate frame at %ds: %v\n", i, err)
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
		codec = strings.TrimSpace(lines[1])
		duration, err = strconv.ParseFloat(strings.TrimSpace(lines[0]), 64)
		if err != nil {
			return 0, "", fmt.Errorf("failed to parse duration: %w", err)
		}
	}

	return duration, codec, nil
}

func (tg *TrickplayGenerator) generateFrame(videoPath, outputDir string, seekTime float64, frameIndex int, codec string) error {
	framePath := filepath.Join(outputDir, fmt.Sprintf("frame-%d.jpg", frameIndex))

	if _, err := os.Stat(framePath); err == nil {
		return nil
	}

	timestamp := fmt.Sprintf("%.3f", seekTime)

	// format=yuvj420p and -strict unofficial are MANDATORY for MJPEG compliance in FFmpeg 7.1.1.
	var args []string
	if codec == "h264" || codec == "hevc" {
		args = append(args, "-bsf:v", codec+"_metadata=matrix_coefficients=1")
	}
	args = append(args, "-ss", timestamp, "-noaccurate_seek", "-i", videoPath, "-vf", "scale=320:-1,format=yuvj420p", "-frames:v", "1", "-strict", "unofficial", "-y", framePath)

	cmd := exec.Command("ffmpeg", args...)
	if err := cmd.Run(); err == nil {
		if info, statErr := os.Stat(framePath); statErr == nil && info.Size() > 0 {
			return nil
		}
	}

	// Simple fallback without BSF or format flags.
	cmdFallback := exec.Command("ffmpeg", "-ss", timestamp, "-i", videoPath, "-vf", "scale=320:-1", "-frames:v", "1", "-strict", "unofficial", "-y", framePath)
	if errFallback := cmdFallback.Run(); errFallback == nil {
		return nil
	}

	return fmt.Errorf("failed to extract frame %d", frameIndex)
}
