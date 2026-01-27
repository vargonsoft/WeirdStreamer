# Video File Detection Fix for Server Sources

## Problem
When clicking on video files from WS Server sources, nothing happened. The logs showed:
```
D/MediaBrowser: handleFileClick: path=https://babylon5:9000/api/media/stream?path=L2hvbWU...X2RlbW8ubW92, isDirectory=false
D/MediaBrowser: File type check: isVideo=false, isImage=false, isPdf=false
W/MediaBrowser: Unknown file type: https://babylon5:9000/api/media/stream?path=...
```

## Root Cause
The video file type detection in `MediaBrowser.kt` was using inline `path.endsWith()` checks:
```kotlin
val isVideo = path.endsWith(".mp4", true) || 
              path.endsWith(".mkv", true) ||
              path.endsWith(".avi", true) ||
              path.endsWith(".mov", true) ||
              path.endsWith(".webm", true)
```

For server URLs, the path is a full streaming URL with base64-encoded actual path:
```
https://babylon5:9000/api/media/stream?path=L2hvbWUvdmFyZ29uL0Rvd25sb2Fkcy8xZDQ0MTA1Ny04NmUxLTQ4YzAtOWU1Mi1hNzlmNmZjNmI1NjZfX2RlbW8ubW92
```

This doesn't end with `.mov` - it ends with the base64 string, so `isVideo` was always `false`.

## Solution

### 1. Added `isVideo()` Method to MediaViewModel
**File:** `MediaViewModel.kt`

Created a new `isVideo()` method that uses the existing `getActualFilename()` helper to decode base64 paths before checking the extension:

```kotlin
fun isVideo(path: String): Boolean {
    val actualPath = getActualFilename(path)
    val cleanPath = actualPath.substringBefore('?').removeSuffix("/")
    val ext = cleanPath.substringAfterLast('.', "").lowercase()
    val result = ext in setOf("mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg")
    android.util.Log.d("MediaViewModel", "isVideo - path: $path, actualPath: $actualPath, ext: $ext, result: $result")
    return result
}
```

### 2. Updated MediaBrowser to Use ViewModel Method
**File:** `ui/components/MediaBrowser.kt`

**Before:**
```kotlin
val isVideo = path.endsWith(".mp4", true) || 
              path.endsWith(".mkv", true) ||
              path.endsWith(".avi", true) ||
              path.endsWith(".mov", true) ||
              path.endsWith(".webm", true)
```

**After:**
```kotlin
val isVideo = viewModel.isVideo(path)
```

## How It Works

### For Server URLs:
1. **Input path:** `https://babylon5:9000/api/media/stream?path=L2hvbWUvdmFyZ29uL0Rvd25sb2Fkcy8xZDQ0MTA1Ny04NmUxLTQ4YzAtOWU1Mi1hNzlmNmZjNmI1NjZfX2RlbW8ubW92`
2. **getActualFilename()** detects it's a server URL and decodes the base64:
   - `L2hvbWUvdmFyZ29uL0Rvd25sb2Fkcy8xZDQ0MTA1Ny04NmUxLTQ4YzAtOWU1Mi1hNzlmNmZjNmI1NjZfX2RlbW8ubW92`
   - Decodes to: `/home/USER/Downloads/1d441057-86e1-48c0-9e52-a79f6fc6b566__demo.mov`
3. **Extract extension:** `.mov`
4. **Check against video extensions:** `mov` is in the set → returns `true`

### For SMB URLs:
1. **Input path:** `smb://server/share/folder/video.mp4`
2. **getActualFilename()** returns path as-is (no base64 to decode)
3. **Extract extension:** `.mp4`
4. **Check against video extensions:** `mp4` is in the set → returns `true`

## Consistency with Other File Types

This fix makes video detection consistent with image and PDF detection, which were already using the ViewModel methods:
- ✅ `viewModel.isImage(path)` - Already worked correctly
- ✅ `viewModel.isPdf(path)` - Already worked correctly
- ✅ `viewModel.isVideo(path)` - **NOW FIXED**

All three methods now:
1. Use `getActualFilename()` to decode server paths
2. Extract the file extension from the decoded path
3. Check against a set of known extensions

## Video Extensions Supported

Added comprehensive video format support:
- `mp4`, `mkv`, `avi`, `mov`, `webm` - Common formats
- `flv`, `wmv`, `m4v` - Additional formats
- `mpg`, `mpeg` - MPEG formats

## Debug Logging

Added logging to track the path decoding process:
```
D/MediaViewModel: isVideo - path: https://babylon5:9000/api/media/stream?path=L2hvbWU..., 
                  actualPath: /home/USER/Downloads/demo.mov, 
                  ext: mov, 
                  result: true
```

This helps verify:
1. The original path being checked
2. The decoded actual path
3. The extracted extension
4. The final result

## Testing

### Server Video Test:
1. Browse to server source
2. Navigate to folder with video files (`.mp4`, `.mov`, `.mkv`, etc.)
3. Click on a video file
4. **Expected:** Video should start playing in VideoPlayer
5. **Check logs:** Should see `isVideo=true` and then "Playing video: ..."

### SMB Video Test:
1. Browse to SMB share
2. Navigate to folder with video files
3. Click on a video file
4. **Expected:** Video should start playing (should still work as before)

## Files Modified

1. ✅ `MediaViewModel.kt` - Added `isVideo()` method with base64 decoding
2. ✅ `ui/components/MediaBrowser.kt` - Use `viewModel.isVideo()` instead of inline checks

## Verification Checklist

- [x] Added isVideo() method to MediaViewModel
- [x] Updated MediaBrowser to use viewModel.isVideo()
- [x] Added debug logging
- [x] Code compiles without errors
- [ ] Server videos play correctly (test required)
- [ ] SMB videos still work (test required)
- [ ] All video formats detected (.mp4, .mkv, .mov, etc.)

## Related Fixes

This completes the file type detection trio:
- **Images** - Fixed in initial implementation (was already using ViewModel method)
- **PDFs** - Fixed in initial implementation (was already using ViewModel method)
- **Videos** - Fixed now (was using inline checks, now uses ViewModel method)

All file types now properly handle both SMB and Server sources with base64-encoded paths.
