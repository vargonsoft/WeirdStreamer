package config

import (
	"path/filepath"
	"slices"
	"strings"
)

// MediaType is what a file is by its extension and the configured formats (media's
// supportedVideoFormats, supportedImageFormats and supportedDocFormats): "video", "image",
// "document", or "" for a file the server doesn't serve.
func MediaType(path string) string {
	if GlobalConfig == nil {
		return ""
	}
	m := GlobalConfig.Media
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ext == "":
		return ""
	case slices.Contains(m.SupportedVideoFormats, ext):
		return "video"
	case slices.Contains(m.SupportedImageFormats, ext):
		return "image"
	case slices.Contains(m.SupportedDocFormats, ext):
		return "document"
	}
	return ""
}

// IsVideo reports whether a file has one of the configured video extensions.
func IsVideo(path string) bool { return MediaType(path) == "video" }
