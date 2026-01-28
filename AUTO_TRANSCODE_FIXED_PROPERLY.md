# Auto-Transcode Fixed - WebUI Sends auto-transcode=0 ✅

## Summary

Fixed the automatic transcoding logic **properly** by:
- ✅ **Server**: Auto-transcode remains ON by default (the correct behavior)
- ✅ **WebUI**: Explicitly sends `auto-transcode=0` to disable it

This is the right approach - the server's "auto-transcode" feature works as designed (automatic), and the WebUI opts out to let browsers attempt native playback first.

## Changes Made

### 1. Server - Reverted to Correct Default

**`Server/internal/handlers/media.go`:**
```go
// Auto-transcoding ENABLED by default (as it should be)
// Clients can disable with ?auto-transcode=0
autoTranscode := r.URL.Query().Get("auto-transcode") != "0"
```

**`Server/internal/media/streamer.go`:**
```go
// Comments updated to reflect auto-transcode is ON by default
```

### 2. WebUI - Opts Out of Auto-Transcode

**`Server/web/js/api.js`:**
```javascript
getStreamUrl(path, forceTranscode = false) {
    const encodedPath = btoa(path);
    let url = `${API_BASE}/api/media/stream?path=${encodedPath}`;
    
    // Disable auto-transcoding for WebUI
    // Let browser attempt native playback first
    url += '&auto-transcode=0';
    
    if (forceTranscode) {
        url += '&transcode=1';  // User clicked 🔄 button
    }
    
    // ... add token ...
    return url;
}
```

## How It Works Now

### Default Behavior (Other Clients)
```
Native app requests: /api/media/stream?path=video.mkv
  ↓
No auto-transcode parameter
  ↓
Server default: autoTranscode = true
  ↓
Server checks: MKV needs transcoding? YES
  ↓
Server auto-transcodes to MP4
```

### WebUI Behavior
```
WebUI requests: /api/media/stream?path=video.mkv&auto-transcode=0
  ↓
auto-transcode=0 present
  ↓
Server: autoTranscode = false
  ↓
Server serves ORIGINAL .mkv file
  ↓
Browser attempts native playback
  ↓
If fails: User clicks 🔄 button → adds &transcode=1
```

## Why This Is The Right Solution

✅ **Server default makes sense** - Auto-transcode is "automatic" by default
✅ **Backward compatible** - Existing clients get automatic transcoding
✅ **WebUI optimized** - Modern browsers get to try native playback
✅ **Explicit control** - WebUI opts out, other clients get auto behavior
✅ **Named correctly** - "auto-transcode" actually works automatically!

## Query Parameter Behavior

| Request | Result |
|---------|--------|
| No params | Auto-transcode ON (server default) |
| `?auto-transcode=0` | Auto-transcode OFF (WebUI default) |
| `?auto-transcode=1` | Auto-transcode ON (explicit) |
| `?transcode=1` | Force transcode (always, regardless of auto) |

## Use Cases

### WebUI (Modern Browsers)
- Sends `auto-transcode=0`
- Tries native playback first
- Transcode button (🔄) available if needed
- Adds `transcode=1` when button clicked

### Native Apps / Other Clients
- Don't send auto-transcode parameter
- Get automatic transcoding (server default)
- MKV/AVI/3GP automatically become MP4
- Works without client-side logic

### Android App (Can Use Native Playback)
- Sends `auto-transcode=0` like WebUI
- Uses ExoPlayer's native codec support
- Transcode button available if needed

## Files Changed

- ✅ `Server/internal/handlers/media.go` - Reverted to auto-transcode ON by default
- ✅ `Server/internal/media/streamer.go` - Updated comments
- ✅ `Server/web/js/api.js` - WebUI sends `auto-transcode=0`

## Deployment

### Server (Required)
```bash
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &
```

### WebUI (Required)
Hard refresh browser:
- Mac: **Cmd+Shift+R**
- Windows/Linux: **Ctrl+Shift+R**

## Testing

### Test WebUI Behavior
1. Open WebUI
2. Click any video (especially MKV/AVI)
3. Check browser network tab - should see `auto-transcode=0` in URL
4. Video should attempt native playback
5. If fails, click 🔄 button
6. Should see `transcode=1` added to URL

### Test Native Client Behavior
```bash
# Simulate native client (no auto-transcode param)
curl "https://babylon5:9000/api/media/stream?path=<base64>&token=<token>"
# Should auto-transcode incompatible formats
```

## Result

🎉 **Problem solved correctly!**

- ✅ Server's "auto-transcode" feature works as designed (automatic by default)
- ✅ WebUI opts out to enable modern browser native playback
- ✅ Transcode button provides explicit fallback
- ✅ Other clients get automatic transcoding without extra logic
- ✅ Everyone's happy!

The server keeps its smart automatic behavior for clients that need it, while the WebUI opts out to leverage modern browser capabilities. Best of both worlds! 🌟
