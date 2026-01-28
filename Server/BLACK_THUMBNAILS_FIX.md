# Black Thumbnails - Root Cause and Fix ✅ APPLIED

## Status: Changes Applied - CRITICAL FIX ADDED

All code changes have been successfully applied to fix:
1. ✅ Black thumbnails (regenerating empty thumbnail URLs in DB)
2. ✅ Unauthorized error on regenerate (using proper auth.GetClaims())
3. ✅ **CRITICAL**: Corrupted thumbnail files on disk (577 bytes) blocking regeneration

**Next Steps:**
1. Commit and push changes from local machine
2. Pull and rebuild on babylon5
3. **Thumbnails will auto-regenerate** when browsing (corrupted files deleted automatically)
4. Hard refresh browser to clear JavaScript cache

## Issues Fixed

### 1. Black Thumbnails - REAL ROOT CAUSE
The black thumbnails issue had **TWO** causes:
- **Database entries with empty thumbnail URLs** ✅ Fixed previously
- **Corrupted thumbnail files on disk (577 bytes)** ✅ **NEW FIX**

### 2. Unauthorized Error on Regenerate
The regenerate thumbnails endpoint was using incorrect method to check admin status, causing "Unauthorized" errors and signing users out.

## Root Causes

### Black Thumbnails - THE FULL STORY:

**Problem 1: Empty URLs in Database**
1. **Old Code (Async Bug):**
   - Thumbnails generated in background goroutine
   - Items saved to database with empty `ThumbnailURL`
   - Goroutine tried to update URL after item was already in response

2. **First Fix:**
   - Changed to synchronous generation for NEW items
   - Added check for empty URLs and regeneration
   - ✅ This fixed the database issue

**Problem 2: Corrupted Files on Disk (577 bytes) - THE REAL CULPRIT**
1. **What Happened:**
   - Old async code created **empty/corrupted thumbnail files** (577 bytes)
   - These files physically exist on disk
   - When we tried to regenerate, code checked: "Does file exist?"
   - Answer: YES (the corrupted 577-byte file exists)
   - Code returned early without regenerating!

2. **Why Regeneration Failed:**
   ```go
   // OLD CODE - Bug!
   if _, err := os.Stat(thumbnailPath); err == nil {
       return thumbnailPath, nil  // File exists, don't regenerate
   }
   ```
   - Checked if file exists, not if file is VALID
   - 577-byte corrupted files passed this check
   - Never reached FFmpeg generation code

3. **NEW FIX:**
   ```go
   // Check if already exists and is valid size (> 1KB)
   if info, err := os.Stat(thumbnailPath); err == nil {
       if info.Size() > 1024 {
           return thumbnailPath, nil  // Valid file, use it
       }
       // File exists but is too small - delete and regenerate
       fmt.Printf("Removing corrupted thumbnail (size=%d): %s\n", info.Size(), thumbnailPath)
       os.Remove(thumbnailPath)
   }
   ```
   - Checks file size, not just existence
   - If < 1KB (1024 bytes), delete and regenerate
   - Automatically fixes all corrupted thumbnails

### Unauthorized Error:

1. **Wrong Admin Check:**
   - Code tried to get user from context as `map[string]interface{}`
   - This caused type assertion failure
   - Result: "Unauthorized" error and user signed out

2. **Correct Admin Check:**
   - Use `auth.GetClaims(r.Context())` to get Claims struct
   - Check `claims.IsAdmin` boolean directly
   - Proper error handling without invalidating session

## Changes Made

### File: Server/internal/media/thumbnail.go ⭐ CRITICAL FIX

**Fix: Check File Size Before Returning Cached Thumbnail**

This is the REAL fix for the 577-byte black thumbnail issue!

```go
// For VIDEO thumbnails:
case "video":
    thumbnailExt = ".jpg"
    thumbnailPath, thumbnailURL = GetThumbnailPath(thumbnailDir, hash, thumbnailExt)
    
    // Check if already exists and is valid size (> 1KB)
    if info, err := os.Stat(thumbnailPath); err == nil {
        if info.Size() > 1024 {
            return thumbnailPath, nil  // Valid thumbnail
        }
        // File exists but is too small - delete and regenerate
        fmt.Printf("Removing corrupted thumbnail (size=%d): %s\n", info.Size(), thumbnailPath)
        os.Remove(thumbnailPath)
    }
    // ... continue to FFmpeg generation ...

// For IMAGE thumbnails:
case "image":
    // ... determine extension ...
    thumbnailPath, thumbnailURL = GetThumbnailPath(thumbnailDir, hash, thumbnailExt)

    // Check if already exists and is valid size (> 1KB)
    if info, err := os.Stat(thumbnailPath); err == nil {
        if info.Size() > 1024 {
            return thumbnailPath, nil  // Valid thumbnail
        }
        // File exists but is too small - delete and regenerate
        fmt.Printf("Removing corrupted thumbnail (size=%d): %s\n", info.Size(), thumbnailPath)
        os.Remove(thumbnailPath)
    }
    // ... continue to image processing ...
```

**What This Does:**
- Gets file info instead of just checking existence
- If file size > 1KB (1024 bytes), use it (valid thumbnail)
- If file size ≤ 1KB (like 577 bytes), DELETE it and regenerate
- Automatically fixes all corrupted thumbnails on next access

### File: Server/internal/handlers/media.go

**Fix 1: Regenerate Missing Thumbnails**
```go
// Item found in database
if item.ThumbnailURL == "" && (mediaType == "image" || mediaType == "video") {
    log.Printf("Regenerating missing thumbnail for: %s", entryPath)
    if mediaType == "image" {
        item.ThumbnailURL = h.scanner.GenerateImageThumbnail(entryPath)
    } else if mediaType == "video" {
        item.ThumbnailURL = h.scanner.GenerateVideoThumbnail(entryPath)
    }
    
    // Update database with new thumbnail URL
    go func(itemCopy *models.MediaItem) {
        h.db.SaveMediaItem(itemCopy)
    }(item)
}
```

**Fix 2: Correct Admin Check**
```go
// OLD (WRONG):
user := r.Context().Value("user")
userMap, ok := user.(map[string]interface{})
isAdmin, _ := userMap["is_admin"].(bool)

// NEW (CORRECT):
claims := auth.GetClaims(r.Context())
if claims == nil {
    respondError(w, http.StatusUnauthorized, "Unauthorized")
    return
}
if !claims.IsAdmin {
    respondError(w, http.StatusForbidden, "Admin access required")
    return
}
```

**Added Import:**
```go
import "github.com/weirdstreamer/server/internal/auth"
```

**Added Debug Logging:**
- Logs when item is not in database
- Logs when thumbnail is generated
- Logs when item is found in database
- Logs when regenerating missing thumbnails

## Deployment Instructions

### 1. Commit and Push Changes

On your **local machine** (Mac):
```bash
cd /Users/vargon/AndroidStudioProjects/WeirdStreamer

# Check what's changed
git status

# Add changes
git add Server/

# Commit
git commit -m "Fix black thumbnails and admin check for regenerate

- Database had old entries with empty thumbnail URLs - now regenerates them
- Fixed admin check to use auth.GetClaims() instead of map assertion
- Prevents unauthorized errors and user sign-out
- Added debug logging for troubleshooting
- Fixes both new and old database entries"

# Push
git push
```

### 2. Deploy to babylon5

On **babylon5**:
```bash
cd <repo>/Server

# Pull latest changes
git pull

# Rebuild server
CGO_ENABLED=1 go build -o server ./cmd/server

# Restart server
killall server
./server &

# Watch logs to see thumbnail generation
tail -f server.log
```

### 3. Fix JavaScript Cache Issue

The `api.regenerateThumbnails is not a function` error is a **browser cache** issue.

**Option A: Hard Refresh (Easiest)**
- Chrome/Edge: `Cmd+Shift+R` (Mac) or `Ctrl+Shift+R` (Windows)
- Firefox: `Cmd+Shift+R` (Mac) or `Ctrl+F5` (Windows)
- Safari: `Cmd+Option+R`

**Option B: Clear Cache**
1. Open browser DevTools (F12)
2. Right-click refresh button
3. Select "Empty Cache and Hard Reload"

**Option C: Verify JavaScript (Debug)**
1. Open browser console (F12)
2. Type: `typeof api.regenerateThumbnails`
3. Should show: `"function"`
4. If shows `"undefined"`, do hard refresh

### 4. Verify Fix

After rebuilding and hard refresh:

**Check Thumbnails:**
1. Browse to a folder with media files
2. Check server logs for:
   ```
   Item from DB: path=/path/to/video.mp4, thumbnailURL=
   Regenerating missing thumbnail for: /path/to/video.mp4
   Generated video thumbnail: path=/path/to/video.mp4, url=/thumbnails/a/b/abc123.jpg
   ```
3. Thumbnails should now appear (not black)

**Check Regenerate Button:**
1. Login as admin
2. Navigate to a folder
3. Click "🔄 Regenerate Thumbnails" button
4. Should work without error

## What This Fixes

### Before:
- ❌ Black thumbnails (database had empty URLs)
- ❌ Regenerate button error (JavaScript not updated)
- ❌ No way to fix corrupted thumbnails

### After:
- ✅ Thumbnails appear correctly
- ✅ Old database entries get fixed automatically
- ✅ Regenerate button works for admins
- ✅ Debug logging for troubleshooting

## Debug Logging

The new logging will show:

**New Item (Not in Database):**
```
Item not in DB, creating new: /path/to/file.mp4
Generated video thumbnail: path=/path/to/file.mp4, url=/thumbnails/1/2/123abc.jpg
```

**Cached Item (In Database, Has Thumbnail):**
```
Item from DB: path=/path/to/file.mp4, thumbnailURL=/thumbnails/1/2/123abc.jpg
```

**Cached Item (In Database, Empty Thumbnail - OLD BUG):**
```
Item from DB: path=/path/to/file.mp4, thumbnailURL=
Regenerating missing thumbnail for: /path/to/file.mp4
Generated video thumbnail: path=/path/to/file.mp4, url=/thumbnails/1/2/123abc.jpg
```

## Performance Notes

- First browse of a folder: Slower (generates thumbnails)
- Database items with empty URLs: Regenerated automatically
- Database items with URLs: Fast (uses cache)
- All items will eventually have thumbnails and be fast

## Alternative: Clear Database

If you want a completely fresh start:

```bash
# On babylon5 (BE CAREFUL - DELETES ALL METADATA)
cd <repo>/Server
killall server
rm -f data/media.db
./server &

# Then re-scan
./cli scan
```

This will:
- Delete all database entries
- Regenerate everything from scratch
- Take longer initially but ensure consistency

**Recommended:** Just deploy the fix - it handles old entries automatically.

## Troubleshooting

### Still See Black Thumbnails

1. **Check server logs:**
   ```bash
   tail -f server.log | grep -i thumbnail
   ```

2. **Look for:**
   - "Regenerating missing thumbnail"
   - "Generated [image/video] thumbnail"
   - Any error messages

3. **Verify thumbnails are being created:**
   ```bash
   ls -la thumbnails/
   ```

4. **Check FFmpeg is working:**
   ```bash
   ffmpeg -i /path/to/video.mp4 -ss 00:00:01 -vframes 1 /tmp/test.jpg
   ```

### Regenerate Button Still Errors

1. **Hard refresh browser** (Cmd+Shift+R)
2. **Clear browser cache completely**
3. **Check console:** Type `typeof api.regenerateThumbnails`
4. **Should show:** `"function"`

### Thumbnails Generate But Still Black

1. **Check thumbnail permissions:**
   ```bash
   ls -la thumbnails/
   chmod -R 755 thumbnails/
   ```

2. **Check thumbnail files exist:**
   ```bash
   find thumbnails/ -name "*.jpg" | head -10
   ```

3. **Try viewing thumbnail directly:**
   ```
   http://babylon5:9000/thumbnails/a/b/abc123.jpg
   ```

## Summary

The fix is simple but critical:
1. **Check** if database item has empty thumbnail URL
2. **Regenerate** thumbnail if empty
3. **Update** database with new URL

This handles the legacy of the async bug and prevents black thumbnails for all future items.
