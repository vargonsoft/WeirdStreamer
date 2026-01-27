# PDF Download Fix

## Problem
PDFs were displaying "Failed to download PDF" when trying to open them from the server source.

## Root Cause
The `ServerRepository.getInputStream()` method was passing the **decoded path** directly to `apiService.stream()`, but the server API expects **base64-encoded paths** in the query parameter.

## Solution

### 1. Fixed Path Encoding in ServerRepository
**File:** `app/src/main/java/com/vargon/weirdstreamer/repository/ServerRepository.kt`

**Before:**
```kotlin
val actualPath = if (path.contains("/api/media/stream?path=")) {
    // decode path...
} else {
    path
}

val response = apiService.stream(actualPath)  // ❌ Passing decoded path
```

**After:**
```kotlin
val actualPath = if (path.contains("/api/media/stream?path=")) {
    // decode path...
} else {
    path
}

// Encode the path for the API (server expects base64-encoded paths)
val encodedPath = Base64.encodeToString(actualPath.toByteArray(), Base64.NO_WRAP)

val response = apiService.stream(encodedPath)  // ✅ Passing encoded path
```

### 2. Added Debug Logging
Added comprehensive logging to both `ServerRepository.getInputStream()` and `MediaViewModel.downloadToTempFile()` to help diagnose issues:

```kotlin
android.util.Log.d("ServerRepository", "getInputStream - actualPath: $actualPath")
android.util.Log.d("ServerRepository", "getInputStream - encodedPath: $encodedPath")
android.util.Log.d("ServerRepository", "getInputStream - response code: ${response.code()}")
```

```kotlin
android.util.Log.d("MediaViewModel", "downloadToTempFile - path: $path")
android.util.Log.d("MediaViewModel", "downloadToTempFile - repository: ${repository::class.simpleName}")
android.util.Log.e("MediaViewModel", "downloadToTempFile - failed: ${error?.message}", error)
```

### 3. Improved Error Handling
Enhanced error handling in `downloadToTempFile()`:
- Check if repository exists before attempting download
- Log detailed error messages
- Properly handle null inputStream despite success result

## How It Works

### Flow for Server PDFs:
1. User clicks PDF in file browser
2. `PdfViewer` calls `viewModel.downloadToTempFile(path, context)`
3. Path is like: `https://server/api/media/stream?path=L2ZvbGRlci9maWxlLnBkZg==`
4. `ServerRepository.getInputStream()`:
   - Detects it's a stream URL
   - Decodes base64: `L2ZvbGRlci9maWxlLnBkZg==` → `/folder/file.pdf`
   - Re-encodes for API: `/folder/file.pdf` → `L2ZvbGRlci9maWxlLnBkZg==`
   - Calls `apiService.stream("L2ZvbGRlci9maWxlLnBkZg==")`
5. Server responds with PDF file stream
6. Download writes to temp file
7. PdfRenderer opens the temp file

### Flow for SMB PDFs:
1. User clicks PDF in file browser
2. `PdfViewer` calls `viewModel.downloadToTempFile(path, context)`
3. Path is like: `smb://server/share/folder/file.pdf`
4. `SmbRepository.getInputStream()`:
   - Opens SMB connection with credentials
   - Returns file input stream directly
5. Download writes to temp file
6. PdfRenderer opens the temp file

## Testing

After rebuilding and installing:

### Server PDF Test:
1. Browse to server source
2. Navigate to folder with PDF file
3. Click on PDF file
4. **Expected:** PDF downloads and opens successfully
5. **Check Logs:**
   ```
   D/ServerRepository: getInputStream - actualPath: /folder/file.pdf
   D/ServerRepository: getInputStream - encodedPath: L2ZvbGRlci9maWxlLnBkZg==
   D/ServerRepository: getInputStream - response code: 200
   D/MediaViewModel: downloadToTempFile - success, file size: 123456 bytes
   ```

### SMB PDF Test:
1. Browse to SMB source
2. Navigate to folder with PDF file
3. Click on PDF file
4. **Expected:** PDF downloads and opens successfully
5. **Check Logs:**
   ```
   D/MediaViewModel: downloadToTempFile - path: smb://server/share/file.pdf
   D/MediaViewModel: downloadToTempFile - repository: SmbRepository
   D/MediaViewModel: downloadToTempFile - success, file size: 123456 bytes
   ```

## Error Scenarios

### If you see "Failed to download PDF":

1. **Check logs for ServerRepository errors:**
   ```
   E/ServerRepository: getInputStream - Server error: 401 Unauthorized
   ```
   → **Fix:** Authentication issue, check token validity

2. **Check logs for MediaViewModel errors:**
   ```
   E/MediaViewModel: downloadToTempFile - no repository set
   ```
   → **Fix:** Repository not initialized, check source selection

3. **Check logs for path encoding issues:**
   ```
   E/ServerRepository: getInputStream - Server error: 400 Bad Request
   ```
   → **Fix:** Path encoding issue, verify base64 encoding is correct

## Files Changed

1. `repository/ServerRepository.kt`
   - Fixed: Encode path before calling API
   - Added: Debug logging

2. `MediaViewModel.kt`
   - Enhanced: Error handling and logging
   - Fixed: Removed duplicate exception handling code

## Build & Deploy

```bash
export JAVA_HOME="/Applications/Android Studio.app/Contents/jbr/Contents/Home"
./gradlew clean :app:assembleDebug
./gradlew :app:installDebug
```

## Verification Checklist

- [ ] Build completes successfully
- [ ] APK installed on device
- [ ] SMB PDFs download and open
- [ ] Server PDFs download and open
- [ ] Error messages are logged for debugging
- [ ] Filenames display correctly in PDF viewer
- [ ] Navigation works after closing PDF

## Additional Notes

The fix ensures that:
- Server API receives correctly encoded paths
- Both SMB and Server sources work uniformly
- Errors are logged with enough detail for debugging
- The repository pattern remains clean and extensible
