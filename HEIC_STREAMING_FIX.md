# HEIC/HEIF Full-Size Image Streaming Fix

## Problem
HEIC/HEIF images (common on iOS devices) could not be viewed at full-size when clicking on them in the WebUI or Android app, since browsers and the Android app don't natively support these formats.

## Solution
Added an auto-conversion feature that detects HEIC/HEIF files during streaming and converts them to JPEG on-the-fly before serving to clients.

## Changes Made

### 1. Configuration (`Server/internal/config/config.go`)
- Added `AutoConvertImages` boolean field to `StreamingConfig` struct
- Default: `true` (auto-conversion enabled)

### 2. Configuration Files
Updated both config files to include the new setting:
- [Server/config.json](Server/config.json)
- [Server/config.example.json](Server/config.example.json)

```json
"streaming": {
  "chunkSize": 1048576,
  "enableHTTP2": true,
  "autoConvertImages": true
}
```

### 3. Stream Handler (`Server/internal/handlers/media.go`)
Modified `StreamMedia` handler to pass the `autoConvertImages` config to the streamer:
```go
if err := h.streamer.ServeContent(w, r, path, item.MimeType, forceTranscode, autoTranscode, startTime, config.GlobalConfig.Streaming.AutoConvertImages); err != nil {
```

### 4. Streaming Service (`Server/internal/media/streamer.go`)
- Updated `ServeContent()` method signature to accept `autoConvertImages` parameter
- Added early detection: if file is HEIC/HEIF and auto-conversion enabled, calls new conversion method
- Added `convertAndServeImage()` method that:
  - Uses FFmpeg to convert HEIC/HEIF to JPEG on-the-fly
  - Streams converted output directly to client
  - Sets proper Content-Type and cache headers
  - Handles client disconnects gracefully

## How It Works

### Streaming Flow for HEIC/HEIF
```
Client Request (HEIC file)
         ↓
StreamMedia Handler (gets config)
         ↓
ServeContent() checks: autoConvertImages && (.heic/.heif)
         ↓
convertAndServeImage() calls FFmpeg
         ↓
FFmpeg converts HEIC → JPEG (pipe:1 output)
         ↓
Server streams JPEG to client as image/jpeg
         ↓
Browser/App displays image
```

## Configuration

### Disable Auto-Conversion
Edit `config.json` to disable auto-conversion:
```json
"streaming": {
  "autoConvertImages": false
}
```

When disabled, users will still see thumbnails (which use thumbnails), but full-size viewing will fail or attempt browser native handling.

## Performance Considerations
- FFmpeg conversion happens on-the-fly (not cached) for immediate availability
- Quality settings: `-c:v mjpeg -q:v 5` balances quality and speed
- Uses client request context for proper cancellation handling
- Respects browser range requests through standard HTTP headers

## Browser/App Compatibility
- **WebUI**: JPEG output supported by all browsers
- **Android**: ExoPlayer and standard image loading support JPEG
- **iOS**: Native Safari supports JPEG

## Dependencies
- FFmpeg installed and in PATH (already required for video transcoding)
- No additional Go dependencies needed

## Testing
1. Rebuild server: `cd Server && CGO_ENABLED=1 go build -o server ./cmd/server/main.go`
2. Start server with updated config
3. View HEIC/HEIF images:
   - Check logs for: "Converting HEIC/HEIF image to JPEG on-the-fly"
   - Should display in browsers/apps without errors

## Troubleshooting
- **Images show blank**: Check FFmpeg availability and permissions
- **Slow loading**: Normal for first view (FFmpeg conversion takes ~1-2s per image)
- **"Failed to stream media" error**: Check server logs for FFmpeg errors
