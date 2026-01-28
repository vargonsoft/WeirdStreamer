# Server-Side Automatic Transcoding Disabled ✅

## Summary

The server's **automatic transcoding** feature has been **disabled by default**. Videos now stream in their original format unless transcoding is explicitly requested.

## Changes Made

### 1. `Server/internal/handlers/media.go`

**Changed auto-transcode default behavior:**

**Before (Auto-transcode ON by default):**
```go
// Auto-transcoding enabled unless explicitly disabled with ?auto-transcode=0
autoTranscode := r.URL.Query().Get("auto-transcode") != "0"
```

**After (Auto-transcode OFF by default):**
```go
// Auto-transcoding disabled by default, requires ?auto-transcode=1 to enable
autoTranscode := r.URL.Query().Get("auto-transcode") == "1"
```

### 2. `Server/internal/media/streamer.go`

**Updated comments and logging:**
- Clarified that auto-transcode is now opt-in, not opt-out
- Added logging when serving original files
- Improved transcode reason logging

## Behavior Changes

### Before
```
User requests video.mkv
  ↓
Server checks: MKV needs transcoding? YES
  ↓
Server automatically transcodes to MP4
  ↓
Client receives transcoded stream
```

### After
```
User requests video.mkv
  ↓
Server serves ORIGINAL .mkv file
  ↓
Browser attempts native playback
  ↓
If browser can't play: User clicks 🔄 button
  ↓
Request with ?transcode=1
  ↓
Server transcodes to MP4
```

## Query Parameter Behavior

### `?transcode=1` (Force Transcode)
- **Always** transcodes, regardless of format
- Used by the WebUI transcode button (🔄)
- Example: `/api/media/stream?path=xyz&transcode=1`

### `?auto-transcode=1` (Enable Auto-Transcode)
- Enables automatic transcoding for incompatible formats
- Server checks file extension (.mkv, .avi, .3gp, etc.)
- If format needs transcoding, server transcodes automatically
- Example: `/api/media/stream?path=xyz&auto-transcode=1`

### No query parameters (Default)
- **Serves original file** in its native format
- Browser attempts native playback
- This is now the default behavior

## Incompatible Formats (Auto-Transcode List)

When `?auto-transcode=1` is set, these formats are transcoded:
- `.3gp` - 3GPP mobile video
- `.avi` - Audio Video Interleave
- `.wmv` - Windows Media Video
- `.asf` - Advanced Systems Format
- `.flv` - Flash Video
- `.mkv` - Matroska Video

**Note:** Modern browsers often support MKV and other formats natively, so auto-transcoding is no longer needed in most cases.

## WebUI Integration

The WebUI transcode button (🔄) uses `?transcode=1`, which:
- ✅ Works exactly as before
- ✅ Force-transcodes regardless of auto-transcode setting
- ✅ Provides explicit user control

## Server Logs

### Original File Serving (New Default)
```
Serving original file: /path/to/video.mkv
```

### Explicit Transcoding (Button Click)
```
Transcoding /path/to/video.mkv (explicitly requested)
```

### Auto-Transcoding (If Enabled)
```
Auto-transcoding /path/to/video.mkv for browser compatibility
```

## Benefits

✅ **Better performance** - No unnecessary transcoding
✅ **Higher quality** - Original video quality preserved
✅ **Less server load** - CPU/disk only used when needed
✅ **Modern browser support** - Browsers can handle many formats natively
✅ **User control** - Explicit decision via transcode button

## Backward Compatibility

### For Native Apps / Custom Clients

If you have native apps or custom clients that rely on automatic transcoding, add `?auto-transcode=1` to stream URLs:

```javascript
// Old behavior (automatic transcoding)
const url = `/api/media/stream?path=${encodedPath}&auto-transcode=1`;
```

### For Web Browsers

No changes needed! The WebUI transcode button handles transcoding when needed.

## Migration Notes

### If You Want Old Behavior Back

To restore automatic transcoding globally, you could:

1. **Modify the default in code:**
   ```go
   // In media.go, change back to:
   autoTranscode := r.URL.Query().Get("auto-transcode") != "0"
   ```

2. **Or modify client requests:**
   ```javascript
   // Add auto-transcode=1 to all stream URLs
   api.getStreamUrl(path) + '&auto-transcode=1'
   ```

## Testing

After rebuilding the server, test with various formats:

### Test 1: MP4 (Browser Native)
```
Request: /api/media/stream?path=video.mp4
Expected: Serves original MP4 (fast, high quality)
```

### Test 2: MKV (Try Native First)
```
Request: /api/media/stream?path=video.mkv
Expected: Serves original MKV
Browser: Attempts native playback (many modern browsers support this)
```

### Test 3: MKV with Force Transcode
```
Request: /api/media/stream?path=video.mkv&transcode=1
Expected: Transcodes to MP4
```

### Test 4: MKV with Auto-Transcode Enabled
```
Request: /api/media/stream?path=video.mkv&auto-transcode=1
Expected: Auto-detects MKV needs transcoding, transcodes to MP4
```

## Deployment

```bash
# On babylon5
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &
```

## Result

✅ **Server now matches WebUI behavior** - Original files by default
✅ **Automatic transcoding disabled** - Only when explicitly requested
✅ **Better performance** - No CPU/disk overhead for supported formats
✅ **User control** - Transcode button provides explicit fallback
✅ **Modern browser optimized** - Native playback when possible

Videos now stream in their original format, giving browsers the chance to play them natively before falling back to transcoding! 🎬✨
