# Video Selection Persistence Fix

## Problem
When exiting the video player and returning to the file list, the previously played video was not selected/highlighted. This worked for images and PDFs but not for videos.

## Root Cause
The `VideoPlayer` component and `stopPlayback()` method did not follow the same pattern as `ImageViewer`/`closeImage()` and `PdfViewer`/`closePdf()`:
- **ImageViewer** - Passes path via `onBack: (String) -> Unit` → calls `closeImage(lastViewedPath)`
- **PdfViewer** - Passes path via `onBack: (String) -> Unit` → calls `closePdf(lastViewedPath)`
- **VideoPlayer** - Had `onBack: () -> Unit` → called `stopPlayback()` with no path ❌

The selection persistence mechanism requires:
1. Viewer passes the current path when calling `onBack`
2. ViewModel receives the path and calls `recordSelection(currentPath, lastViewedPath)`
3. On return to file list, `FocusableList` uses `getSelection(currentPath)` to restore focus

## Solution

### 1. Updated `stopPlayback()` Method
**File:** `MediaViewModel.kt`

Added optional `lastViewedPath` parameter and selection recording:

```kotlin
fun stopPlayback(lastViewedPath: String? = null) {
    android.util.Log.d("MediaViewModel", "stopPlayback - lastViewedPath: $lastViewedPath, currentPath: $currentPath")
    lastViewedPath?.let { recordSelection(currentPath, it) }
    selectedVideoPath = null
}
```

This matches the pattern used by `closeImage()` and `closePdf()`.

### 2. Updated VideoPlayer Signature
**File:** `ui/components/VideoPlayer.kt`

Changed `onBack` callback to accept a path parameter:

**Before:**
```kotlin
fun VideoPlayer(viewModel: MediaViewModel, path: String, onBack: () -> Unit) {
```

**After:**
```kotlin
fun VideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
```

### 3. Updated VideoPlayer to Pass Path
**File:** `ui/components/VideoPlayer.kt`

Modified `handleBackAction` to pass the video path:

**Before:**
```kotlin
val handleBackAction = {
    // ...
    if (now - lastControlsHideTime > 1000) {
        onBack()  // ❌ No path passed
    }
}
```

**After:**
```kotlin
val handleBackAction = {
    // ...
    if (now - lastControlsHideTime > 1000) {
        onBack(path)  // ✅ Pass video path
    }
}
```

### 4. Updated MainActivity
**File:** `MainActivity.kt`

Updated the VideoPlayer call to pass the path to `stopPlayback`:

**Before:**
```kotlin
VideoPlayer(
    viewModel = viewModel,
    path = selectedVideo,
    onBack = { viewModel.stopPlayback() }  // ❌ No path
)
```

**After:**
```kotlin
VideoPlayer(
    viewModel = viewModel,
    path = selectedVideo,
    onBack = { lastViewedPath -> viewModel.stopPlayback(lastViewedPath) }  // ✅ Pass path
)
```

## How It Works

### Selection Flow for Videos:

1. **User plays a video:**
   ```
   MediaBrowser → handleFileClick(path)
   → viewModel.playVideo(path)
   → selectedVideoPath = path
   → VideoPlayer displays
   ```

2. **User presses back in VideoPlayer:**
   ```
   VideoPlayer → handleBackAction()
   → onBack(path)  // Pass the video path
   → MainActivity → viewModel.stopPlayback(lastViewedPath)
   → recordSelection(currentPath, lastViewedPath)
   → selectionMap[currentPath] = lastViewedPath
   ```

3. **Return to file browser:**
   ```
   FileBrowserScreen → FocusableList(
       items = viewModel.fileList,
       initialSelection = viewModel.getSelection(currentPath)
   )
   → Finds matching FileItem by path
   → Focuses on that video
   ```

### Path Examples:

**Server Video:**
```
Path: https://babylon5:9000/api/media/stream?path=L2hvbWU...X2RlbW8ubW92
Selection recorded: selectionMap["/home/USER/Downloads"] = "https://babylon5:..."
On return: FocusableList finds item with matching path → focuses on it
```

**SMB Video:**
```
Path: smb://server/share/folder/video.mp4
Selection recorded: selectionMap["smb://server/share/folder/"] = "smb://server/share/folder/video.mp4"
On return: FocusableList finds item with matching path → focuses on it
```

## Consistency Achieved

All three viewer types now follow the same pattern:

| Viewer | Callback Signature | Close Method | Selection Recorded |
|--------|-------------------|--------------|-------------------|
| **ImageViewer** | `onBack: (String) -> Unit` | `closeImage(lastViewedPath)` | ✅ |
| **PdfViewer** | `onBack: (String) -> Unit` | `closePdf(lastViewedPath)` | ✅ |
| **VideoPlayer** | `onBack: (String) -> Unit` | `stopPlayback(lastViewedPath)` | ✅ |

## Debug Logging

Added logging to track selection recording:

```kotlin
D/MediaViewModel: stopPlayback - lastViewedPath: https://babylon5:9000/api/media/stream?path=..., 
                  currentPath: /home/USER/Downloads
D/MediaViewModel: recordSelection - parent: /home/USER/Downloads, 
                  child: https://babylon5:9000/api/media/stream?path=...
```

Later when returning to the folder:
```kotlin
D/MediaViewModel: getSelection - parent: /home/USER/Downloads, 
                  selection: https://babylon5:9000/api/media/stream?path=...
```

## Files Modified

1. ✅ `MediaViewModel.kt` - Updated `stopPlayback()` to accept and record path
2. ✅ `ui/components/VideoPlayer.kt` - Changed callback signature and pass path
3. ✅ `MainActivity.kt` - Pass path from VideoPlayer to stopPlayback

## Testing

### Server Video Test:
1. Browse to server source folder with videos
2. Click on a video (e.g., "demo.mov")
3. Watch for a moment
4. Press back
5. **Expected:** "demo.mov" should be highlighted/focused in the file list
6. **Check logs:** Should see `stopPlayback` and `recordSelection` messages

### SMB Video Test:
1. Browse to SMB share folder with videos
2. Click on a video
3. Press back
4. **Expected:** Video should be highlighted/focused
5. Should work the same as server videos

### Multiple Folders Test:
1. Open video in folder A, press back
2. Navigate to folder B, open video, press back
3. Navigate back to folder A
4. **Expected:** Last viewed video in folder A should still be selected
5. Selection is per-folder (keyed by `currentPath`)

## Verification Checklist

- [x] Updated `stopPlayback()` to accept `lastViewedPath`
- [x] Changed VideoPlayer callback signature to `onBack: (String) -> Unit`
- [x] Updated VideoPlayer to pass path when calling onBack
- [x] Updated MainActivity to pass path to stopPlayback
- [x] Added debug logging
- [x] Code compiles without errors
- [ ] Server videos maintain selection after playback (test required)
- [ ] SMB videos maintain selection after playback (test required)
- [ ] Selection is folder-specific (test required)

## Related Fixes

This completes the selection persistence feature across all viewer types:
- ✅ **Images** - Selection persisted (fixed in FILE_SELECTION_FIX.md)
- ✅ **PDFs** - Selection persisted (fixed in FILE_SELECTION_FIX.md)
- ✅ **Videos** - Selection persisted (fixed now)

All viewers now properly record and restore the last viewed file when navigating back to the file browser.
