# File Selection Persistence Fix

## Problem
When leaving a viewer (ImageViewer, PdfViewer), the previously selected file was not being highlighted when returning to the file browser. This worked for SMB files but not for WS Server files.

## Root Causes

### 1. MainActivity Not Passing Last Viewed Path
**File:** `MainActivity.kt`

The `onBack` callbacks for ImageViewer and PdfViewer were not using the path parameter that the viewers provide.

**Before:**
```kotlin
ImageViewer(
    viewModel = viewModel,
    initialPath = selectedImage,
    onBack = { viewModel.closeImage() }  // ❌ Not using the path parameter
)
```

**After:**
```kotlin
ImageViewer(
    viewModel = viewModel,
    initialPath = selectedImage,
    onBack = { lastViewedPath -> viewModel.closeImage(lastViewedPath) }  // ✅ Passing the path
)
```

### 2. FocusableList Not Recognizing FileItem Type
**File:** `ui/components/FocusableList.kt`

The `FocusableList` component only handled `SmbFileItem` type, but `MediaViewModel` uses the new `FileItem` type from the repository pattern.

**Before:**
```kotlin
items.indexOfFirst { item ->
    when (item) {
        is String -> item == initialSelection
        is SmbFileItem -> item.path == initialSelection || item.name == initialSelection
        else -> false  // ❌ FileItem falls through to false
    }
}
```

**After:**
```kotlin
items.indexOfFirst { item ->
    when (item) {
        is String -> item == initialSelection
        is SmbFileItem -> item.path == initialSelection || item.name == initialSelection
        is FileItem -> item.path == initialSelection || item.name == initialSelection  // ✅ Added
        else -> false
    }
}
```

## Solution Details

### Files Modified

1. **MainActivity.kt**
   - Updated `ImageViewer` onBack to pass `lastViewedPath`
   - Updated `PdfViewer` onBack to pass `lastViewedPath`

2. **MediaViewModel.kt**
   - Added logging to `recordSelection()` to track what's being stored
   - Added logging to `getSelection()` to track what's being retrieved
   - Added logging to `closeImage()` and `closePdf()` to verify paths

3. **FocusableList.kt**
   - Added support for `FileItem` type in `initialIndex` calculation
   - Added support for `FileItem` type in `isDirectory()` helper
   - Added support for `FileItem` type in `getName()` helper

## How It Works

### Selection Recording Flow:

1. **User opens a file:**
   ```
   MediaBrowser → handleFileClick(path="/api/media/stream?path=...")
   → viewModel.showImage(path)
   → selectedImagePath = path
   ```

2. **ImageViewer displays the file**

3. **User presses back:**
   ```
   ImageViewer → onBack(currentSmbPath)
   → MainActivity → viewModel.closeImage(lastViewedPath)
   → recordSelection(currentPath="/folder", childPath="/api/media/stream?path=...")
   → selectionMap["/folder"] = "/api/media/stream?path=..."
   ```

4. **Return to file browser:**
   ```
   FileBrowserScreen → FocusableList(
       items = viewModel.fileList,
       initialSelection = viewModel.getSelection("/folder")
   )
   → getSelection returns "/api/media/stream?path=..."
   → FocusableList finds matching FileItem by comparing item.path
   → Focuses on that item
   ```

### Path Matching

For **Server files**, the path stored in `FileItem.path` is the full streaming URL:
```
https://server.com/api/media/stream?path=L2ZvbGRlci9maWxlLmpwZw==
```

This same URL is:
1. Passed to `showImage(path)`
2. Passed back via `onBack(lastViewedPath)`  
3. Stored in `selectionMap[currentPath] = lastViewedPath`
4. Retrieved and matched against `FileItem.path`

For **SMB files**, the path is the SMB URL:
```
smb://server/share/folder/file.jpg
```

## Debug Logging

Added comprehensive logging to track the selection mechanism:

```kotlin
// When recording selection:
D/MediaViewModel: closeImage - lastViewedPath: https://server/api/media/stream?path=..., currentPath: /folder
D/MediaViewModel: recordSelection - parent: /folder, child: https://server/api/media/stream?path=...

// When retrieving selection:
D/MediaViewModel: getSelection - parent: /folder, selection: https://server/api/media/stream?path=...
```

## Testing

### Server Files Test:
1. Browse to server source
2. Navigate to a folder with multiple images
3. Click on an image (e.g., "photo2.jpg")
4. Press back
5. **Expected:** "photo2.jpg" should be highlighted/focused
6. **Check logs:** Should see recordSelection and getSelection messages

### SMB Files Test:
1. Browse to SMB share
2. Navigate to a folder with multiple PDFs
3. Click on a PDF (e.g., "document3.pdf")
4. Press back
5. **Expected:** "document3.pdf" should be highlighted/focused
6. **Check logs:** Should see recordSelection and getSelection messages

### Navigation Test:
1. Open an image
2. Navigate through multiple images using arrow keys
3. Press back
4. **Expected:** Last viewed image should be focused
5. Navigate to different folder and back
6. **Expected:** Selection should be remembered per folder

## Verification Checklist

- [x] MainActivity passes lastViewedPath to close methods
- [x] FocusableList handles FileItem type
- [x] Debug logging added for troubleshooting
- [x] Code compiles without errors
- [ ] Selection persists for Server images (test required)
- [ ] Selection persists for Server PDFs (test required)
- [ ] Selection persists for SMB files (should still work)
- [ ] Selection is folder-specific (test required)

## Additional Notes

### Why Full URLs Work
Even though server paths are full URLs (with base URL and base64 encoding), the selection matching works because:
1. The same full URL is stored in `FileItem.path` during browse
2. The same full URL is passed through the viewer flow
3. String comparison matches them exactly

### Per-Folder Memory
The selection is keyed by `currentPath`:
- Server: `/folder` → remembers last file in `/folder`
- SMB: `smb://server/share/folder/` → remembers last file in that folder

This means navigating between folders maintains separate selection memory for each folder.

### Alternative Approach (Not Used)
We could have normalized paths to just filenames for selection, but that would break if multiple folders have files with the same name. The current approach using full paths is more robust.
