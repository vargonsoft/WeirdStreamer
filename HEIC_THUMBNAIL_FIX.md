# HEIC/HEIF Thumbnail Generation Fix

## Problem
The server was unable to generate thumbnails for HEIC/HEIF image files despite having them listed in the supported image formats configuration.

## Root Cause
Go's standard library `image` package does not include built-in support for HEIC/HEIF formats. When the thumbnail generator attempted to decode HEIC/HEIF files using `image.Decode()`, it failed with a decoding error.

## Solution
Modified the thumbnail generation logic in [Server/internal/media/thumbnail.go](Server/internal/media/thumbnail.go) to:

1. **Detect HEIC/HEIF files** by checking the file extension
2. **Convert to JPEG** using FFmpeg before processing:
   - Use `ffmpeg -c:v mjpeg -q:v 5` for efficient HEIC-to-JPEG conversion
   - Store converted image in temp directory
   - Clean up temp file after thumbnail generation
3. **Proceed with normal thumbnail pipeline** on the converted JPEG

## Changes Made

### File: Server/internal/media/thumbnail.go
- Added HEIC/HEIF detection in the image case handler
- Inserted FFmpeg conversion step for unsupported formats
- Maintained EXIF orientation correction and image resizing for converted images

### File: Server/internal/media/scanner.go
- Updated `getMimeType()` to include MIME types:
  - `.heif` → `image/heif`
  - `.heic` → `image/heic`

## Requirements
- FFmpeg must be installed and available in PATH
- HEIC/HEIF formats already listed in config.json's `supportedImageFormats`

## Testing
1. Build the server: `cd Server && CGO_ENABLED=1 go build -o server ./cmd/server/main.go`
2. Rescan media: `./cli media scan`
3. Verify thumbnails are generated for HEIC/HEIF files in `./run/thumbnails/`

## Benefits
- HEIC/HEIF images (common on iOS devices) now supported
- Thumbnails maintain aspect ratio and quality
- EXIF orientation corrections applied properly
- No external Go dependencies required (uses existing FFmpeg integration)
