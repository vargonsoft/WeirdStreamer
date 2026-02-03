# HEIC/HEIF Image Cropping Fix

## Problem
When converting HEIC/HEIF images to JPEG for viewing, the output was appearing heavily cropped rather than showing the full image.

## Root Cause
The FFmpeg command was using incorrect codec and format parameters:
- Original: `ffmpeg -i input.heic -c:v libjpeg ...` 
- Issues:
  1. `libjpeg` encoder is not available in standard FFmpeg builds
  2. Missing proper output format specification for single image output
  3. No `-update` flag to specify single frame output (vs image sequence)

## Solution
Updated both thumbnail generation and streaming conversion to use proper FFmpeg parameters:

```bash
ffmpeg -i input.heic -c:v mjpeg -q:v 5 -f image2pipe -update 1 pipe:1
```

**Parameters explained:**
- `-c:v mjpeg` - Motion JPEG codec (widely available, suitable for single images)
- `-q:v 5` - Quality level 5 (excellent quality, fast encoding)
- `-f image2pipe` - Output format for piping single images
- `-update 1` - Write single frame instead of image sequence

## Files Changed

### 1. Server/internal/media/thumbnail.go
- HEIC→JPEG conversion for thumbnail generation
- Uses same MJPEG parameters with file output

### 2. Server/internal/media/streamer.go
- HEIC→JPEG conversion for full-size image streaming
- Pipes output directly to HTTP response

## Verification
Tested with actual HEIC file from `/home/USER/Downloads/jSF1EBDp.heic`:
- Original dimensions: 450x600
- Converted JPEG dimensions: 450x600 ✓
- No cropping confirmed

## Build Status
✅ Compilation successful
✅ All FFmpeg commands validated
✅ Dimensions preserved in conversion

## Testing Instructions
1. Rebuild: `cd Server && CGO_ENABLED=1 go build -o server ./cmd/server/main.go`
2. Start server with config `"autoConvertImages": true`
3. View HEIC/HEIF images via WebUI or Android app
4. Verify full image is displayed (not cropped)
