# Async Thumbnail Generation with Real-Time Updates

## Summary

Thumbnail generation is now **non-blocking** with real-time progress updates via Server-Sent Events (SSE). Users can browse folders immediately, and thumbnails appear automatically as they're generated in the background.

## Changes Made

### Backend (Go Server)

#### 1. `Server/internal/handlers/media.go`

**Added SSE Infrastructure:**
- `thumbnailClient` struct for managing SSE connections
- `thumbnailClients` map to track connected clients
- `clientsMux` for thread-safe client management

**Modified Browse Handler:**
- Thumbnail generation now happens **in background goroutines**
- Browse request returns immediately with items (no waiting for thumbnails)
- When thumbnail is ready, `notifyThumbnailReady()` sends SSE event to all clients

**New Methods:**
- `ThumbnailUpdates(w http.ResponseWriter, r *http.Request)` - SSE endpoint for thumbnail notifications
- `notifyThumbnailReady(filePath, thumbnailURL string)` - Broadcasts thumbnail completion to all connected clients

**Before (Blocking):**
```go
// Generate thumbnail immediately - blocks the request
if mediaType == "image" {
    item.ThumbnailURL = h.scanner.GenerateImageThumbnail(entryPath)
}
```

**After (Non-Blocking):**
```go
// Generate thumbnail in background - returns immediately
if mediaType == "image" {
    go func(itemCopy *models.MediaItem) {
        thumbnailURL := h.scanner.GenerateImageThumbnail(itemCopy.Path)
        itemCopy.ThumbnailURL = thumbnailURL
        h.db.SaveMediaItem(itemCopy)
        h.notifyThumbnailReady(itemCopy.Path, thumbnailURL)
    }(item)
}
```

#### 2. `Server/cmd/server/main.go`

**Added Route:**
```go
api.HandleFunc("/media/thumbnail-updates", mediaHandler.ThumbnailUpdates).Methods("GET")
```

### Frontend (Web UI)

#### 1. `Server/web/js/api.js`

**Added SSE Support:**
- `eventSource` - EventSource connection for SSE
- `thumbnailCallbacks` - Array of callback functions
- `connectThumbnailUpdates()` - Establishes SSE connection
- `disconnectThumbnailUpdates()` - Closes SSE connection
- `onThumbnailUpdate(callback)` - Registers callback for thumbnail events
- `clearThumbnailCallbacks()` - Clears all callbacks

**Auto-Reconnect:**
- If SSE connection drops, automatically reconnects after 5 seconds

#### 2. `Server/web/js/app.js`

**SSE Connection Management:**
- Connects to SSE when showing main screen
- Disconnects on logout (cleanup)
- Registers `handleThumbnailUpdate()` callback

**Thumbnail Update Handler:**
```javascript
function handleThumbnailUpdate(data) {
    // Find media item by path
    // Update thumbnail image with new URL
    // Add cache buster to force reload
    // Show subtle flash animation
}
```

**Placeholder Support:**
- Items without thumbnails show "Loading..." placeholder
- Placeholder has pulsing animation
- Automatically replaced when real thumbnail arrives

**Media Element Updates:**
- Added `data-path` attribute to media items for matching SSE updates
- Changed thumbnail display to show placeholder when `thumbnail_url` is empty

#### 3. `Server/web/css/style.css`

**New Animations:**
- `thumbnailUpdate` - Subtle flash when thumbnail loads (0.5s fade)
- `pulse` - Breathing animation for placeholders (2s loop)

**New Classes:**
- `.placeholder` - Thumbnail placeholder styling (semi-transparent, grayscale)
- `.loading-icon` - Centered icon overlay on placeholders

## User Experience

### Before (Blocking)
1. User clicks on folder
2. **Browser freezes** while waiting for all thumbnails to generate
3. Can take 10-30+ seconds for folders with many videos
4. Folder finally appears with all thumbnails loaded

### After (Non-Blocking)
1. User clicks on folder
2. **Folder appears instantly** with placeholder thumbnails
3. Real thumbnails **pop in one by one** as they're generated
4. User can browse, navigate, or play media immediately
5. Smooth, responsive experience

## Technical Details

### Server-Sent Events (SSE)

**Why SSE instead of WebSockets?**
- Simpler (one-way server-to-client)
- Works with HTTP/2
- Automatic reconnection
- Built-in browser support (EventSource API)
- No additional dependencies

**Authentication:**
- EventSource API doesn't support custom headers
- Solution: Pass JWT token as query parameter
- Server validates token before establishing SSE connection
- URL format: `/api/media/thumbnail-updates?token=<JWT>`

**SSE Message Format:**
```
data: {"path":"/path/to/video.mp4","thumbnailURL":"/thumbnails/a/b/abc123.jpg"}
```

### Progressive Enhancement

**Works without JavaScript:**
- Thumbnails still generate in background
- Next page refresh shows completed thumbnails
- Graceful degradation

**Works with slow connections:**
- Folder loads immediately
- Thumbnails arrive as bandwidth allows
- No timeout issues

### Performance

**Server:**
- Background goroutines for thumbnail generation
- Non-blocking database writes
- Efficient client notification (channel-based)

**Client:**
- Single SSE connection (not one per file)
- Efficient DOM updates (find by data-path)
- Cache busters prevent stale images

## Testing

### Test Scenario 1: New Folder
1. Navigate to folder with 50 videos (none cached)
2. Folder should appear in < 500ms
3. Thumbnails should appear progressively over next 30-60 seconds
4. Each thumbnail shows with subtle flash animation

### Test Scenario 2: Cached Folder
1. Navigate to previously browsed folder
2. All thumbnails should appear immediately (from database)
3. No SSE updates needed

### Test Scenario 3: Partial Cache
1. Navigate to folder with mix of cached and new files
2. Cached thumbnails appear immediately
3. New thumbnails appear progressively
4. Only new files show loading placeholders

### Test Scenario 4: Multiple Tabs
1. Open same folder in multiple browser tabs
2. Thumbnails should update in **all tabs simultaneously**
3. Each tab maintains own SSE connection

## Configuration

No configuration needed! The feature works automatically when:
- Server is running
- User is authenticated
- Browsing a folder with media files

## Troubleshooting

### Thumbnails not appearing
- Check browser console for SSE connection errors
- Verify `/api/media/thumbnail-updates` endpoint is accessible
- Check server logs for thumbnail generation errors

### SSE connection drops
- Auto-reconnects after 5 seconds
- Check proxy/load balancer timeout settings
- SSE connections should not timeout (keep-alive)

### Multiple updates for same file
- Normal during regeneration
- Each update replaces previous thumbnail
- Cache buster prevents showing stale images

## Future Enhancements

Possible improvements:
- Progress indicator showing X/Y thumbnails generated
- Priority generation (visible items first)
- Batch notifications (reduce SSE traffic)
- Thumbnail generation queue status
- Pause/resume thumbnail generation

## Deployment

```bash
# On babylon5
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &

# Hard refresh browser to load new JavaScript
# Cmd+Shift+R (Mac) or Ctrl+Shift+R (Windows/Linux)
```

## Summary

✅ **Non-blocking** - Folders load instantly
✅ **Real-time updates** - Thumbnails appear as they're generated
✅ **Progressive enhancement** - Works without SSE (degrades gracefully)
✅ **Efficient** - Single SSE connection, background processing
✅ **User-friendly** - Loading placeholders, smooth animations
✅ **Scalable** - Multiple tabs, multiple users, multiple folders

Browse folders are now **fast and responsive** regardless of thumbnail generation time!
