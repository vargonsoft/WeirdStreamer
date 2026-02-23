package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

	duration, err := tg.getVideoDuration(videoPath)
	if err != nil {
		return fmt.Errorf("failed to get video duration: %w", err)
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

	// Generate first frame
	if err := tg.generateFrame(videoPath, trickplayVideoDir, 0); err != nil {
		fmt.Printf("Warning: failed to generate first frame: %v\n", err)
	}

	// Generate frames every 10 seconds
	for i := 10; i < int(duration); i += 10 {
		if err := tg.generateFrame(videoPath, trickplayVideoDir, i); err != nil {
			// Log error and continue
			fmt.Printf("Warning: failed to generate frame at %ds: %v\n", i, err)
		}
	}

	// Generate last frame
	if err := tg.generateFrame(videoPath, trickplayVideoDir, int(duration)); err != nil {
		fmt.Printf("Warning: failed to generate last frame: %v\n", err)
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

func (tg *TrickplayGenerator) getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %s", string(output))
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

func (tg *TrickplayGenerator) generateFrame(videoPath, outputDir string, timeInSeconds int) error {
	framePath := filepath.Join(outputDir, fmt.Sprintf("frame-%d.jpg", timeInSeconds))

	// Resume: if frame already exists, skip it
	if _, err := os.Stat(framePath); err == nil {
		return nil
	}
	
	timestamp := time.Duration(timeInSeconds) * time.Second
	
	cmd := exec.Command("ffmpeg",
		"-ss", timestamp.String(),
		"-i", videoPath,
		"-vframes", "1",
		"-q:v", "5", // Image quality (1-31, lower is better)
		"-vf", "scale=320:-1",
		"-y",
		framePath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed for frame at %ds: %s", timeInSeconds, string(output))
	}

	return nil
}
// Force rewrite to fix newline issues
