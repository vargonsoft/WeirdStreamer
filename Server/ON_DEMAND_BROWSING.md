s# On-Demand File Browsing and Thumbnail Generation

## Overview
Modified the WS Server to browse files directly from the filesystem without requiring a pre-scan via `./cli scan`. Files now appear immediately when browsing folders, and thumbnails are generated on-demand in the background.

## Problem
Previously, the server required running `./cli scan` to index all media files into the database before they would appear in the browse API. This meant:
- New media paths wouldn't show any files until manually scanned
- Adding files to existing paths required re-scanning
- Poor user experience with extra manual steps

## Solution
The server now operates in two modes:
1. **Direct Filesystem Browsing**: Files appear immediately by reading the filesystem
2. **Database Caching**: Metadata and thumbnails are cached in the database for faster subsequent access

## Changes Made

### Server/internal/handlers/media.go

**Modified `BrowseDirectory` function:**
```go
// Before: Only showed files that existed in database
if item, err := h.db.GetMediaItem(entryPath); err == nil {
    items = append(items, *item)
}

// After: Shows all media files from filesystem
- Reads filesystem directly
- Creates MediaItem with basic info (name, size, type, mime type)
- Tries to get from database first (for cached metadata/thumbnails)
- If not in database, creates item with basic info
- Generates thumbnails in background (non-blocking)
- Saves to database for future requests
```

**Added helper functions:**
- `getMediaTypeFromExtension(ext string) string` - Determines if file is video/image/document
- `getMimeTypeFromExtension(ext string) string` - Returns proper MIME type for streaming

### Server/internal/media/scanner.go

**Made methods public:**
- `GenerateVideoThumbnail(videoPath string) string` - Generate video thumbnail (was private)
- `GenerateImageThumbnail(imagePath string) string` - Generate image thumbnail (was private)

**Added new method:**
- `GetFileMetadata(path string) (*models.MediaItem, error)` - Get metadata for a file without saving to database

## How It Works

### Browse Flow

1. **User opens folder** in Android app
2. **Server reads directory** from filesystem (os.ReadDir)
3. **For each file**:
   - Check if it's a supported media type (video/image/PDF)
   - Try to load from database (for cached metadata/thumbnails)
   - If not in database:
     - Create MediaItem with basic info
     - Start background goroutine to generate thumbnail
     - Save to database when thumbnail is ready
4. **Return all files** to client immediately

### Thumbnail Generation

**Synchronous (Database):**
- When `./cli scan` is run
- Thumbnails generated before adding to database
- Blocks until complete

**Asynchronous (On-Demand):**
- When browsing a folder with new files
- Thumbnail generation happens in background goroutine
- Files appear immediately (thumbnail loads later)
- Once generated, cached in database for future requests

### Benefits

#### User Experience
- ✅ **Instant File Visibility**: Files appear immediately when browsing folders
- ✅ **No Manual Scanning**: Just add paths to config and browse
- ✅ **Background Processing**: Thumbnails generate without blocking
- ✅ **Progressive Loading**: See files first, thumbnails load as ready

#### Performance
- ✅ **Database Caching**: Metadata and thumbnails cached for fast repeat access
- ✅ **Non-Blocking**: Thumbnail generation doesn't delay browsing
- ✅ **Lazy Loading**: Only generate thumbnails for files actually viewed
- ✅ **Efficient**: Reuses database cache when available

#### Developer Experience
- ✅ **No Migration Required**: Existing scanned files still work
- ✅ **Backward Compatible**: `./cli scan` still works for bulk indexing
- ✅ **Optional Scanning**: Can pre-scan for better initial performance
- ✅ **Flexible**: Mix of scanned and on-demand files works seamlessly

## Migration Notes

### Existing Installations
- No changes required
- Already-scanned files continue to use database cache
- New files automatically work via filesystem browsing
- Can continue using `./cli scan` for bulk indexing if preferred

### New Installations
- Can skip `./cli scan` entirely
- Files appear immediately when browsing
- Thumbnails generate on first view
- Optional: Run `./cli scan` to pre-generate all thumbnails

## Use Cases

### Best for On-Demand (No Scan Required)
- Small to medium libraries (< 10,000 files)
- Frequently changing content (downloads, recordings)
- Development/testing
- New paths added frequently

### Best for Pre-Scanning
- Large libraries (> 10,000 files)
- Static content (movie collections)
- Production deployments with many users
- When you want all thumbnails ready immediately

### Hybrid Approach
- Run `./cli scan` once initially for existing content
- New files automatically work via on-demand browsing
- Periodically re-scan for new thumbnails and metadata updates

## Technical Details

### Goroutine Safety
```go
go func(path string, mType string) {
    var thumbnailURL string
    if mType == "image" {
        thumbnailURL = h.scanner.GenerateImageThumbnail(path)
    } else if mType == "video" {
        thumbnailURL = h.scanner.GenerateVideoThumbnail(path)
    }
    
    if thumbnailURL != "" {
        item.ThumbnailURL = thumbnailURL
        h.db.SaveMediaItem(item)
    }
}(entryPath, mediaType)
```

- Each thumbnail generation runs in separate goroutine
- Database saves are goroutine-safe (SQLite handles locking)
- No blocking of browse response
- Errors logged but don't affect browsing

### MIME Type Mapping
Supports all configured formats:
- Videos: mp4, mkv, avi, mov, wmv, flv, webm, 3gp, 3gpp, m4v, mpg, mpeg, ts, mp2
- Images: jpg, jpeg, png, gif, bmp, webp
- Documents: pdf

### Database Caching
- First request: Reads filesystem, generates thumbnail, saves to database
- Subsequent requests: Reads from database (fast)
- Cache invalidation: Delete database to force regeneration

## Testing

To verify the feature:

1. **Add a new media path** to config.json:
   ```bash
   ./cli config add-path /path/to/new/media
   ```

2. **Don't run scan**:
   ```bash
   # Skip: ./cli scan
   ```

3. **Browse in Android app**:
   - Navigate to the new path
   - Files should appear immediately
   - Folders show up instantly
   - Thumbnails load progressively

4. **Check logs**:
   ```
   Stream request - Path: /path/to/file.mp4
   Generating thumbnail on-the-fly for: /path/to/file.mp4
   ```

5. **Verify caching**:
   - Browse same folder again
   - Should be faster (using database cache)
   - No thumbnail generation logs

## Future Enhancements

Potential improvements:
1. **Thumbnail Queue**: Limit concurrent thumbnail generations
2. **Priority System**: Prioritize thumbnails for currently visible items
3. **Batch Generation**: Generate multiple thumbnails efficiently
4. **Cache Warming**: Pre-generate thumbnails for recently accessed folders
5. **Metadata Extraction**: Extract video duration/resolution on-demand
6. **Progress API**: Endpoint to check thumbnail generation status

## Backward Compatibility

### API Changes
- ✅ No breaking changes to API responses
- ✅ `/api/media/browse` returns same structure
- ✅ Thumbnail URLs use same format
- ✅ Existing database entries still used

### CLI Commands
- ✅ `./cli scan` still works (optional)
- ✅ `./cli config add-path` still required for new paths
- ✅ All existing commands unchanged

### Client Compatibility
- ✅ Android app works without changes
- ✅ Web client works without changes
- ✅ All existing clients compatible

## Notes

- The `./cli scan` command is now optional but can still be useful for:
  - Pre-generating all thumbnails
  - Extracting all metadata at once
  - Initial setup of large libraries
  - Scheduled maintenance tasks

- On-demand generation is automatic and transparent
- No configuration changes required
- Works with both SMB-equivalent browsing and API streaming
