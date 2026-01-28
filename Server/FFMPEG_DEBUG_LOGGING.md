# FFmpeg Thumbnail Debug Logging

## Problem
You weren't seeing FFmpeg execution in logs because debug output was going to stdout (`fmt.Printf`) instead of stderr (`log.Printf`).

## Changes Applied

### All debug output now uses `log.Printf` with `[THUMBNAIL]` or `[SCANNER]` prefixes

**Files Changed:**
1. `Server/internal/media/thumbnail.go` - Added `log` import and replaced all `fmt.Printf` with `log.Printf`
2. `Server/internal/media/scanner.go` - Replaced warning `fmt.Printf` with `log.Printf`

## What You'll See in Logs Now

### When browsing a directory with new media files:

```
[SCANNER] Calling GenerateThumbnail for video: /path/to/video.mp4
[THUMBNAIL] GenerateThumbnail called for: /path/to/video.mp4
[THUMBNAIL] Media type determined: video (ext: .mp4)
[THUMBNAIL] Generating video thumbnail: source=/path/to/video.mp4, output=/thumbnails/a/b/abc123.jpg
```

### If FFmpeg succeeds:

```
[THUMBNAIL] Successfully generated video thumbnail: /thumbnails/a/b/abc123.jpg (15234 bytes)
[SCANNER] Successfully generated thumbnail URL: /thumbnails/a/b/abc123.jpg
```

### If FFmpeg fails:

```
[THUMBNAIL] FFmpeg error for /path/to/video.mp4: exit status 1
Output: [full FFmpeg error output here]
[SCANNER] WARNING: Failed to generate video thumbnail for /path/to/video.mp4: ffmpeg failed: exit status 1 (output: ...)
```

### If thumbnail file is too small (577 bytes):

```
[THUMBNAIL] Removing corrupted thumbnail (size=577): /thumbnails/a/b/abc123.jpg
[THUMBNAIL] Generating video thumbnail: source=/path/to/video.mp4, output=/thumbnails/a/b/abc123.jpg
[THUMBNAIL] FFmpeg created thumbnail but size too small: 577 bytes for /thumbnails/a/b/abc123.jpg
[SCANNER] WARNING: Failed to generate video thumbnail for /path/to/video.mp4: thumbnail too small: 577 bytes
```

### If video is too short (< 1 second):

```
[THUMBNAIL] FFmpeg failed with seek, trying without seek for: /path/to/short.mp4
[THUMBNAIL] Successfully generated video thumbnail: /thumbnails/c/d/cd456.jpg (8923 bytes)
```

## Key Indicators

### ✅ Working correctly:
- You see `[THUMBNAIL] Generating video thumbnail`
- FFmpeg completes without errors
- File size is > 1KB (typically 5KB-50KB)
- You see `Successfully generated video thumbnail`

### ❌ FFmpeg not being called:
- You DON'T see `[THUMBNAIL] Generating video thumbnail`
- Likely means file already exists and passed size check
- Or `GenerateThumbnail` not being called at all

### ⚠️ FFmpeg failing:
- You see `[THUMBNAIL] FFmpeg error`
- Check the FFmpeg output for details
- Common issues:
  - Video codec not supported
  - File corrupted
  - FFmpeg not installed/not in PATH
  - Permission issues

### ⚠️ 577-byte files:
- You see `thumbnail but size too small: 577 bytes`
- Means FFmpeg created file header but failed to extract frame
- Check FFmpeg error output to see why

## How to View Logs

### If running server directly:
```bash
./server 2>&1 | tee server.log
```

### If running in background:
```bash
./server > server.log 2>&1 &
tail -f server.log
```

### Filter for thumbnail-related logs only:
```bash
tail -f server.log | grep -E '\[THUMBNAIL\]|\[SCANNER\]'
```

### Check for FFmpeg errors:
```bash
grep -i "ffmpeg error" server.log
```

## Deploy Steps

```bash
# On your Mac
cd /Users/vargon/AndroidStudioProjects/WeirdStreamer
git add Server/
git commit -m "Add comprehensive FFmpeg debug logging"
git push

# On babylon5
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &

# Watch logs in real-time
tail -f server.log

# Or filter for thumbnails only
tail -f server.log | grep -E '\[THUMBNAIL\]|\[SCANNER\]'
```

## What to Test

1. **Browse a directory with NEW video files** (not in database yet)
   - Should see full thumbnail generation flow
   - Will reveal if FFmpeg is running and what errors it produces

2. **Browse same directory again**
   - Should see "Item from DB" logs
   - Should NOT regenerate if thumbnail is valid (> 1KB)

3. **Look for 577-byte files**
   - Check: `find /path/to/thumbnails -type f -size -1024c`
   - These will be automatically deleted and regenerated on next browse

4. **Check FFmpeg is installed**
   ```bash
   which ffmpeg
   ffmpeg -version
   ```

## Expected Root Cause

Based on 577-byte files, likely one of:

1. **FFmpeg not found** - Server can't find ffmpeg executable
2. **Video codec issue** - FFmpeg can't decode the video format
3. **Seek failure** - Video too short or corrupted, can't seek to 1 second
4. **Permission issue** - FFmpeg can't read source file or write thumbnail

The new logging will reveal exactly which one!
