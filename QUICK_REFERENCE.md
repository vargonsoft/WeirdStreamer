# Quick Reference: MediaRepository Interface

## Interface Definition

```kotlin
interface MediaRepository {
    suspend fun browse(path: String): Result<List<FileItem>>
    suspend fun authenticate(username: String?, password: String?): Result<Boolean>
    fun getStreamUrl(path: String): String
    fun getThumbnailUrl(path: String): String?
    fun getDisplayName(path: String): String  // ✨ NEW
    suspend fun getInputStream(path: String): Result<InputStream>  // ✨ NEW
}
```

## Usage Examples

### Getting Display Name
```kotlin
// In any viewer
val filename = viewModel.getDisplayName(path)
// Works for both:
// - SMB: "smb://server/share/folder/file.jpg" → "file.jpg"
// - Server: "/api/media/stream?path=<base64>" → "file.jpg" (decoded)
```

### Downloading Files
```kotlin
// In ViewModel
val tempFile = downloadToTempFile(path, context, ".pdf")
// Works for both SMB and Server sources
// Uses repository.getInputStream() internally

// IMPORTANT: For Server sources, paths must be base64-encoded when calling the API
// ServerRepository handles this automatically in getInputStream()
```

### Video Streaming
```kotlin
// In VideoPlayer
val isSmbSource = path.startsWith("smb://")
ExoPlayer.Builder(context).apply {
    if (isSmbSource) {
        setMediaSourceFactory(/* SMB DataSource */)
    }
    // else: uses default HTTP DataSource
}
```

## Implementation Pattern

### For New Source Types
```kotlin
class MyNewRepository : MediaRepository {
    override fun getDisplayName(path: String): String {
        // Extract filename from your path format
        return path.substringAfterLast('/')
    }
    
    override suspend fun getInputStream(path: String): Result<InputStream> {
        return try {
            val stream = /* your download logic */
            Result.success(stream)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    // ... implement other methods
}
```

## Key Files

| File | Purpose |
|------|---------|
| `MediaRepository.kt` | Interface definition |
| `SmbRepository.kt` | SMB implementation |
| `ServerRepository.kt` | Server API implementation |
| `MediaViewModel.kt` | Repository manager |
| `*Viewer.kt` | UI consumers |

## Testing

```kotlin
// Mock for unit tests
val mockRepository = object : MediaRepository {
    override fun getDisplayName(path: String) = "test.jpg"
    override suspend fun getInputStream(path: String) = 
        Result.success(ByteArrayInputStream(byteArrayOf()))
    // ...
}

viewModel.currentRepository = mockRepository
```

## Common Patterns

### Filename Display
```kotlin
// ❌ OLD - fragile, doesn't handle base64
val name = path.substringAfterLast('/')

// ✅ NEW - robust, handles all sources
val name = viewModel.getDisplayName(path)
```

### File Download
```kotlin
// ❌ OLD - SMB-only
SmbFile(path, context).inputStream

// ✅ NEW - works with any source
val result = repository.getInputStream(path)
if (result.isSuccess) {
    result.getOrNull()?.use { stream -> /* process */ }
}
```

### Source Detection
```kotlin
// ❌ OLD - type checking
when (source) {
    is MediaSource.Smb -> /* SMB code */
    is MediaSource.Server -> /* Server code */
}

// ✅ NEW - polymorphism
repository.getDisplayName(path)  // works for all
```

## Troubleshooting

### PDF Downloads Fail
**Symptom:** "Failed to download PDF" error

**Common Causes:**
1. **Server path not base64-encoded** - ServerRepository must encode paths before calling API
2. **No repository set** - Check logs: "no repository set"
3. **Authentication failed** - Check logs: "401 Unauthorized"

**Solution:**
```kotlin
// ServerRepository.getInputStream() must encode the path:
val encodedPath = Base64.encodeToString(actualPath.toByteArray(), Base64.NO_WRAP)
val response = apiService.stream(encodedPath)  // ✅ Encoded
```

### Check Logs
```kotlin
// Enable verbose logging
android.util.Log.setProperty("log.tag.ServerRepository", "DEBUG")
android.util.Log.setProperty("log.tag.MediaViewModel", "DEBUG")

// Look for these log messages:
// D/ServerRepository: getInputStream - actualPath: /path/to/file.pdf
// D/ServerRepository: getInputStream - encodedPath: L2Zvb...
// D/ServerRepository: getInputStream - response code: 200
// D/MediaViewModel: downloadToTempFile - success, file size: 123456 bytes
```

