# Thumbnail Fixes and Regeneration Feature

## Summary of Changes

This update fixes black thumbnails and adds an admin-only "Regenerate Thumbnails" button to the web UI.

## Issues Fixed

### 1. Black Thumbnails Issue
**Problem:** Thumbnails were appearing black in the web UI even though files were showing up.

**Root Cause:** Thumbnail generation was happening asynchronously in a goroutine, but the response was being sent before thumbnails were generated. The goroutine was trying to modify `item.ThumbnailURL` after the item had already been copied to the response.

**Solution:** Changed thumbnail generation to be synchronous on first browse:
- Thumbnails are now generated **immediately** when browsing a folder for the first time
- The `ThumbnailURL` is set before adding the item to the response
- Database save happens in background to avoid blocking the response
- Subsequent browses use the cached thumbnails from the database (fast)

**Code Change in `Server/internal/handlers/media.go`:**
```go
// Before: Async generation (item already in response)
go func(path string, mType string) {
    thumbnailURL = h.scanner.GenerateImageThumbnail(path)
    item.ThumbnailURL = thumbnailURL  // Too late!
}(entryPath, mediaType)

// After: Sync generation (thumbnail set before response)
if mediaType == "image" {
    item.ThumbnailURL = h.scanner.GenerateImageThumbnail(entryPath)
} else if mediaType == "video" {
    item.ThumbnailURL = h.scanner.GenerateVideoThumbnail(entryPath)
}

// Save to database in background
go func(itemCopy *models.MediaItem) {
    h.db.SaveMediaItem(itemCopy)
}(item)
```

### 2. Regenerate Thumbnails Feature
**Purpose:** Allow admins to force regeneration of all thumbnails in a folder (useful for corrupted or outdated thumbnails).

**Features:**
- ✅ Admin-only (requires `is_admin: true`)
- ✅ Shows only when browsing a folder (not on root)
- ✅ Confirmation dialog before regenerating
- ✅ Deletes existing thumbnails and regenerates them
- ✅ Runs in background (non-blocking)
- ✅ Visual feedback (button disables during operation)

## Files Modified

### Server (Go)

**1. Server/internal/handlers/media.go**
- Fixed `BrowseDirectory` to generate thumbnails synchronously
- Added `RegenerateThumbnails` endpoint handler
- Endpoint: `POST /api/media/regenerate-thumbnails?path=<folder-path>`
- Admin-only: Checks `is_admin` claim from JWT token
- Deletes existing thumbnails and regenerates all media files in folder

**2. Server/cmd/server/main.go**
- Added route: `api.HandleFunc("/media/regenerate-thumbnails", mediaHandler.RegenerateThumbnails).Methods("POST")`

### Web UI (JavaScript/HTML)

**3. Server/web/js/api.js**
- Added `regenerateThumbnails(path)` method to API client

**4. Server/web/index.html**
- Added button: `<button id="regenerate-thumbnails-btn" class="btn" style="display: none;">🔄 Regenerate Thumbnails</button>`

**5. Server/web/js/app.js**
- Added DOM element reference for `regenerateThumbnailsBtn`
- Added event listener in `setupEventListeners()`
- Added `handleRegenerateThumbnails()` handler function
- Added `updateRegenerateThumbnailsButton()` visibility logic
- Updated `showMainScreen()` to call `updateRegenerateThumbnailsButton()`
- Updated `loadMedia()` to call `updateRegenerateThumbnailsButton()` after loading

## How It Works

### Thumbnail Generation Flow

**First Browse (Not in Database):**
1. User browses folder
2. Server reads filesystem
3. For each media file:
   - Generate thumbnail **immediately** (blocks response)
   - Set `item.ThumbnailURL` 
   - Add item to response
   - Save to database in background
4. Response sent with thumbnail URLs
5. Web UI displays thumbnails

**Subsequent Browse (In Database):**
1. User browses same folder
2. Server loads items from database (fast)
3. Thumbnails already have URLs
4. Response sent immediately
5. No thumbnail generation needed

### Regenerate Thumbnails Flow

**When User Clicks Button:**
1. Confirmation dialog appears
2. User confirms
3. POST request to `/api/media/regenerate-thumbnails?path=<folder>`
4. Server validates:
   - User is authenticated
   - User is admin
   - Path is valid
5. Server starts background goroutine:
   - Reads directory
   - For each media file:
     - Delete existing thumbnail file
     - Regenerate thumbnail
     - Update database
6. Response sent immediately (202 status)
7. User refreshes page after a few moments to see new thumbnails

## API Endpoint

### POST /api/media/regenerate-thumbnails

**Authentication:** Required (JWT Bearer token)
**Authorization:** Admin only (`is_admin: true`)

**Query Parameters:**
- `path` (required): Relative path to folder (e.g., `Downloads`)

**Request:**
```bash
curl -X POST "http://localhost:8080/api/media/regenerate-thumbnails?path=Downloads" \
  -H "Authorization: Bearer <admin-token>"
```

**Response (Success):**
```json
{
  "status": "regeneration started",
  "path": "Downloads"
}
```

**Response (Error - Not Admin):**
```json
{
  "error": "Admin access required"
}
```

## Testing

### Test Thumbnail Generation

1. **Build and restart server:**
   ```bash
   cd Server
   make build
   killall server && ./server
   ```

2. **Browse to folder with media files** in web UI
3. **Thumbnails should appear** (not black)
4. **Check server logs:**
   ```
   Generating thumbnail for: /path/to/video.mp4
   ```

5. **Browse same folder again** - should be instant (using cache)

### Test Regenerate Thumbnails

1. **Login as admin** in web UI
2. **Navigate to a folder** (e.g., Downloads)
3. **"🔄 Regenerate Thumbnails" button should appear** in header
4. **Click the button**
5. **Confirm in dialog**
6. **Wait a few moments** (check server logs for progress)
7. **Refresh the page** - thumbnails should be regenerated

### Verify Button Visibility

**Should Show:**
- ✅ User is admin
- ✅ Currently in a folder (not root)

**Should Hide:**
- ❌ User is not admin (non-admin users)
- ❌ On root directory listing
- ❌ Not logged in

## Performance Considerations

### First Browse (With Thumbnail Generation)
- **Images:** Fast (~50-200ms per image)
- **Videos:** Slower (~1-3 seconds per video)
- **Large folders:** May take several seconds to load
- **Trade-off:** Slower first load, but thumbnails visible immediately

### Subsequent Browse (Cached)
- **All files:** Fast (~10-50ms)
- **Database lookup** only, no thumbnail generation
- Same performance as before

### Optimization Tips

For large libraries:
1. **Pre-scan:** Run `./cli scan` to pre-generate all thumbnails
2. **Batch operations:** Regenerate thumbnails during off-peak hours
3. **Selective regeneration:** Only regenerate folders with issues

## Troubleshooting

### Thumbnails Still Black

1. **Check FFmpeg is installed:**
   ```bash
   ffmpeg -version
   ```

2. **Check permissions on thumbnail directory:**
   ```bash
   ls -la ./thumbnails
   chmod 755 ./thumbnails
   ```

3. **Check server logs for errors:**
   ```bash
   tail -f server.log | grep -i thumbnail
   ```

4. **Manually test thumbnail generation:**
   ```bash
   ffmpeg -i /path/to/video.mp4 -ss 00:00:01.000 -vframes 1 -vf "scale=320:-1" /tmp/test.jpg
   ```

### Regenerate Button Not Showing

1. **Verify you're logged in as admin:**
   ```javascript
   // In browser console
   JSON.parse(localStorage.getItem('user'))
   // Should show: { username: "admin", is_admin: true }
   ```

2. **Navigate to a folder** (not root directory)

3. **Check browser console for errors**

### Regeneration Not Working

1. **Check you're admin** (see above)

2. **Check server logs:**
   ```bash
   tail -f server.log | grep -i regenerat
   ```

3. **Verify path is valid:**
   ```bash
   # Server validates path is within media directories
   ```

## Migration from Previous Version

**No database migration required!**

The changes are backward-compatible:
- Existing database entries work as-is
- Already-generated thumbnails are reused
- New files get thumbnails on first browse
- Can still use `./cli scan` for bulk generation

## Performance Impact

**Before (Async Generation):**
- Browse folder: Instant response
- Thumbnails: Black initially, appear later (never)
- Database: Items saved with thumbnails eventually

**After (Sync Generation):**
- Browse folder: Slower response (generates thumbnails)
- Thumbnails: Appear immediately in response
- Database: Items saved in background

**Trade-off Analysis:**
- First browse: **Slower** (1-3s for videos, <1s for images)
- Subsequent browse: **Same speed** (uses cache)
- User experience: **Better** (thumbnails visible immediately)

## Best Practices

1. **Pre-scan large libraries:**
   ```bash
   ./cli scan  # Generate all thumbnails in advance
   ```

2. **Regenerate selectively:**
   - Only regenerate folders with corrupt thumbnails
   - Don't regenerate entire library unless necessary

3. **Monitor server load:**
   - Thumbnail generation is CPU/IO intensive
   - Limit concurrent folder browses during regeneration

4. **Schedule maintenance:**
   - Use `./cli scan` during off-peak hours
   - Regenerate thumbnails overnight if needed

## Related Documentation

- [Server/ON_DEMAND_BROWSING.md](ON_DEMAND_BROWSING.md) - On-demand file browsing
- [Server/REBUILD_CHECKLIST.md](REBUILD_CHECKLIST.md) - Rebuild instructions
- [Server/test_browse.sh](test_browse.sh) - Test script for browsing
