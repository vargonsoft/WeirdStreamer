package metadata

import (
	"testing"

	"github.com/weirdstreamer/server/internal/config"
)

// useVideoFormats sets the configured video extensions for a test (file names are parsed without
// them), and puts the config back after it.
func useVideoFormats(t *testing.T, exts ...string) {
	prev := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = prev })
	config.GlobalConfig = &config.Config{Media: config.MediaConfig{SupportedVideoFormats: exts}}
}
