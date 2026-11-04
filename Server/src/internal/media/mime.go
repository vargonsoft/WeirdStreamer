package media

import (
	"path/filepath"
	"strings"
)

// mimeTypes are the types files are served and saved with, by extension. Which files are served
// at all is the config's (config.MediaType); an extension missing here is served as
// application/octet-stream.
var mimeTypes = map[string]string{
	// Video
	".mp4":  "video/mp4",
	".mkv":  "video/x-matroska",
	".avi":  "video/x-msvideo",
	".divx": "video/x-msvideo", // DivX and Xvid files are AVI containers
	".xvid": "video/x-msvideo",
	".iso":  "application/x-iso9660-image", // DVD images, played transcoded
	".mov":  "video/quicktime",
	".wmv":  "video/x-ms-wmv",
	".asf":  "video/x-ms-asf",
	".flv":  "video/x-flv",
	".webm": "video/webm",
	".3gp":  "video/3gpp",
	".3gpp": "video/3gpp",
	".m4v":  "video/x-m4v",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".ts":   "video/mp2t",
	".mp2":  "video/mpeg",
	// Image
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".bmp":  "image/bmp",
	".webp": "image/webp",
	".heif": "image/heif",
	".heic": "image/heic",
	// Document
	".pdf": "application/pdf",
}

// MimeType is the MIME type of a file by its extension.
func MimeType(path string) string {
	if t, ok := mimeTypes[strings.ToLower(filepath.Ext(path))]; ok {
		return t
	}
	return "application/octet-stream"
}
