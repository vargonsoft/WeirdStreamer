# Server Rebuild Checklist - On-Demand File Browsing

## After pulling the latest code changes, follow these steps:

### 1. Verify Go Version
```bash
go version
# Should be Go 1.24.0 or higher
```

### 2. Clean Previous Build
```bash
cd /path/to/WeirdStreamer/Server
rm -f server cli
```

### 3. Download Dependencies
```bash
go mod download
go mod tidy
```

### 4. Rebuild Server and CLI
```bash
make build
# OR manually:
CGO_ENABLED=1 go build -o server ./cmd/server
CGO_ENABLED=1 go build -o cli ./cmd/cli
```

### 5. Restart the Server
```bash
# Stop the old server first
killall server  # or however you stop it

# Start the new server
./server
```

### 6. Test File Browsing

**Via Web UI:**
1. Open browser to `http://your-server:8080`
2. Login with your credentials
3. Browse to a folder with media files
4. Files should appear immediately (no scan required)

**Via Android App:**
1. Navigate to a WS Server folder
2. Files should appear in the list
3. Thumbnails will load progressively

### 7. Verify Changes Are Active

Check server logs for these messages when browsing:
```
# When opening a folder with new files (not yet scanned):
Stream request - Path: /path/to/file.mp4

# Background thumbnail generation:
Generating thumbnail for: /path/to/file.mp4
```

### 8. Optional: Pre-scan for Performance

If you have a large library and want all thumbnails ready:
```bash
./cli scan
```

But this is now **optional** - files will appear without scanning!

## Troubleshooting

### Files still not showing?

1. **Check config.json has the correct paths:**
   ```bash
   cat config.json | grep -A 5 '"paths"'
   ```

2. **Verify the server binary timestamp:**
   ```bash
   ls -lah server
   # Should show recent date/time (after rebuild)
   ```

3. **Check server logs for errors:**
   ```bash
   tail -f /path/to/server.log
   # or if running in foreground, watch the console output
   ```

4. **Test with curl:**
   ```bash
   # Get auth token first
   TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"admin","password":"yourpassword"}' | jq -r .token)
   
   # Browse root
   curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/media/browse
   
   # Should see folders listed
   
   # Browse a specific folder
   curl -H "Authorization: Bearer $TOKEN" "http://localhost:8080/api/media/browse?path=FolderName"
   
   # Should see files and subdirectories
   ```

### Still having issues?

Check that these files have the latest changes:
- `internal/handlers/media.go` - Should have `getMediaTypeFromExtension()` method
- `internal/media/scanner.go` - Should have public `GenerateImageThumbnail()` and `GenerateVideoThumbnail()` methods

```bash
# Verify changes are present:
grep -n "getMediaTypeFromExtension" internal/handlers/media.go
grep -n "GenerateImageThumbnail" internal/media/scanner.go
grep -n "GenerateVideoThumbnail" internal/media/scanner.go
```

## Expected Behavior

### Before Changes
- Folders appeared empty until `./cli scan` was run
- New paths showed no files
- Adding files required re-scanning

### After Changes
- Files appear immediately when browsing
- No scan required for new paths
- Thumbnails generate in background
- Database caches for fast repeat access
- `./cli scan` is optional (for bulk pre-generation)

## Key Code Changes Summary

1. **BrowseDirectory** now reads filesystem directly instead of only database
2. **Thumbnail generation** happens on-demand in background goroutines
3. **Database caching** stores metadata/thumbnails for future requests
4. **Supported formats** include: mp4, mkv, avi, mov, wmv, flv, webm, 3gp, 3gpp, m4v, mpg, mpeg, ts, mp2, jpg, jpeg, png, gif, bmp, webp, pdf

## Success Indicators

✅ Files appear in web UI without running scan
✅ Android app shows files in folder list
✅ Thumbnails load (may take a few seconds for videos)
✅ Second visit to same folder is faster (uses cache)
✅ Server logs show background thumbnail generation
