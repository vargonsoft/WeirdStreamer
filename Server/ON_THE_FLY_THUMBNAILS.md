# On-the-Fly Thumbnail Generation

## Overview
Updated the WeirdStreamer server to support on-the-fly thumbnail generation while maintaining backward compatibility with the existing `cli scan` command.

## Changes Made

### 1. New File: `Server/internal/media/thumbnail.go`
Created a new reusable thumbnail generation module that:
- **ThumbnailGenerator** struct with mutex for thread-safe generation
- **GenerateThumbnail(filePath)** - Main function that generates thumbnails on-demand
- Supports video (MP4, MKV, AVI, etc.) via ffmpeg
- Supports images (JPG, PNG, WEBP, etc.) with EXIF orientation correction
- Caches generated thumbnails (doesn't regenerate if already exists)
- Uses the same hash-based directory structure (`/thumbnails/a/b/abc123.jpg`)
- Shared helper functions: `GenerateThumbnailHash()`, `GetThumbnailPath()`
- Image manipulation functions: resize, rotate, flip for EXIF orientation

### 2. Updated: `Server/internal/media/scanner.go`
Refactored to use the shared thumbnail generation code:
- Removed duplicate thumbnail generation logic
- Removed image manipulation functions (now in thumbnail.go)
- Scanner now uses `ThumbnailGenerator` for `generateVideoThumbnail()` and `generateImageThumbnail()`
- Cleaned up imports (removed unused image libraries)
- **`cli scan` still works exactly as before** - pre-generates all thumbnails during scan

### 3. Updated: `Server/internal/handlers/media.go`
Enhanced the thumbnail serving endpoint:
- **ServeThumbnail()** now supports on-the-fly generation
- Flow:
  1. Check if thumbnail file exists → serve it (fast path)
  2. If not found, extract hash from requested filename
  3. Look up source file in database by thumbnail hash
  4. Verify source file still exists
  5. Generate thumbnail on-the-fly using `ThumbnailGenerator`
  6. Serve the newly generated thumbnail
- Adds logging for on-the-fly generation events
- Thread-safe with mutex protection

## Benefits

1. **No Pre-scanning Required**: API clients can request thumbnails immediately without waiting for `cli scan`
2. **Lazy Loading**: Thumbnails are generated only when actually needed
3. **Backward Compatible**: `cli scan` still works for batch pre-generation
4. **Cached**: Generated thumbnails are saved and reused for future requests
5. **Automatic**: No manual intervention needed - thumbnails appear automatically
6. **Efficient**: Only generates what's requested, doesn't waste resources on unused files

## Usage

### API Client Perspective
```
GET /thumbnails/a/b/abc123.jpg
```
- If thumbnail exists: returns immediately
- If not: generates on-the-fly (may take 1-2 seconds first time), then returns
- Subsequent requests: instant (thumbnail is cached)

### CLI Scan (Optional)
```bash
./cli media scan
```
- Pre-generates all thumbnails
- Useful for:
  - Initial server setup
  - Periodic maintenance
  - Ensuring all thumbnails exist before high-traffic periods

## Performance Considerations

- **First Request**: 1-2 seconds (video extraction or image resize)
- **Cached Requests**: Instant (served as static files)
- **Concurrent Requests**: Mutex prevents duplicate generation
- **Memory**: Minimal - one thumbnail at a time
- **Disk**: Same as before - hash-based directory structure prevents filesystem issues

## Example Scenarios

### Scenario 1: New Media Added
1. User uploads new video file
2. Android app browses directory via `/api/media/browse`
3. App requests thumbnail: `GET /thumbnails/1/2/123abc.jpg`
4. Server generates thumbnail on-the-fly (1-2 sec)
5. App displays thumbnail
6. Future requests instant (cached)

### Scenario 2: Pre-scanned Library
1. Admin runs `./cli media scan`
2. All thumbnails pre-generated
3. All API requests instant (thumbnails already exist)
4. No on-the-fly generation needed

### Scenario 3: Missing Thumbnails After Cleanup
1. Admin clears thumbnail directory
2. API requests automatically regenerate as needed
3. No manual intervention required
4. System self-heals

## Testing

```bash
# Build the server
cd Server
make build

# Test on-the-fly generation
# 1. Clear thumbnails (optional)
rm -rf run/thumbnails/*

# 2. Start server
./server

# 3. Request a thumbnail via API (will generate on-the-fly)
curl -H "Authorization: Bearer <token>" \
  https://localhost:9000/thumbnails/a/b/abc123.jpg

# 4. Verify thumbnail was generated
ls run/thumbnails/a/b/

# 5. Second request should be instant (cached)
time curl -H "Authorization: Bearer <token>" \
  https://localhost:9000/thumbnails/a/b/abc123.jpg
```

## Architecture

```
┌─────────────────┐
│  API Request    │
│  /thumbnails/   │
│   a/b/abc.jpg   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ ServeThumbnail  │◄─── Check disk cache
│    Handler      │
└────────┬────────┘
         │ Not found
         ▼
┌─────────────────┐
│  Database       │◄─── Look up source file
│  Lookup (hash)  │     by thumbnail hash
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Thumbnail       │◄─── Generate thumbnail
│  Generator      │     (video or image)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Cache to Disk  │◄─── Save for future requests
│  & Serve File   │
└─────────────────┘
```
