# Video Streaming SSL/Auth Fix for Server Sources

## Problem
When trying to play videos from WS Server sources with HTTPS, ExoPlayer failed with an SSL certificate validation error:

```
androidx.media3.exoplayer.ExoPlaybackException: Source error
Caused by: androidx.media3.datasource.HttpDataSource$HttpDataSourceException: 
  javax.net.ssl.SSLHandshakeException: java.security.cert.CertPathValidatorException: 
  Trust anchor for certification path not found.
```

## Root Cause
ExoPlayer was using its default `DefaultHttpDataSource` which:
1. **Does not trust self-signed certificates** - Validates SSL certificates against the system trust store
2. **Does not add authentication headers** - Doesn't have access to the JWT tokens stored in the app

The `WeirdStreamerApplication` has an `OkHttpClient` configured with:
- SSL certificate validation disabled (for self-signed certs)
- Authentication interceptor (adds JWT tokens)

However, ExoPlayer was not using this custom `OkHttpClient`.

## Solution

### 1. Added media3-datasource-okhttp Dependency
**Files:** `gradle/libs.versions.toml`, `app/build.gradle.kts`

Added the OkHttp DataSource extension for Media3/ExoPlayer:
```toml
media3-datasource-okhttp = { group = "androidx.media3", name = "media3-datasource-okhttp", version.ref = "media3" }
```

This allows ExoPlayer to use OkHttp for HTTP streaming instead of the default HttpURLConnection-based DataSource.

### 2. Exposed OkHttpClient as Singleton
**File:** `WeirdStreamerApplication.kt`

Made the configured `OkHttpClient` accessible globally:

```kotlin
class WeirdStreamerApplication : Application(), ImageLoaderFactory {
    
    companion object {
        private var instance: WeirdStreamerApplication? = null
        
        fun getOkHttpClient(): OkHttpClient {
            return instance?.okHttpClient ?: throw IllegalStateException("Application not initialized")
        }
    }
    
    private lateinit var okHttpClient: OkHttpClient
    
    override fun onCreate() {
        super.onCreate()
        instance = this
        // ...
    }
    
    override fun newImageLoader(): ImageLoader {
        // ... create okHttpClient with SSL bypass and auth interceptor
        okHttpClient = OkHttpClient.Builder()
            .sslSocketFactory(...)
            .hostnameVerifier { _, _ -> true }
            .addInterceptor { /* auth headers */ }
            .build()
        // ...
    }
}
```

### 3. Updated VideoPlayer to Use OkHttp DataSource
**File:** `ui/components/VideoPlayer.kt`

For server URLs (non-SMB), configure ExoPlayer to use the OkHttp DataSource:

```kotlin
val exoPlayer = remember(path) {
    val player = ExoPlayer.Builder(context)
        .setLoadControl(loadControl)
        .apply {
            if (isSmbSource) {
                // SMB: Use custom SMB DataSource
                setMediaSourceFactory(
                    DefaultMediaSourceFactory(context)
                        .setDataSourceFactory(SmbDataSourceFactory(...))
                )
            } else {
                // Server: Use OkHttp DataSource with SSL bypass and auth
                val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                val okHttpDataSourceFactory = OkHttpDataSource.Factory(okHttpClient)
                    .setUserAgent("WeirdStreamer/1.0")
                
                setMediaSourceFactory(
                    DefaultMediaSourceFactory(context)
                        .setDataSourceFactory(okHttpDataSourceFactory)
                )
            }
        }
        .build()
}
```

## How It Works

### For Server Videos (HTTPS):
1. User clicks video from server source
2. `VideoPlayer` detects it's not SMB (`!path.startsWith("smb://")`)
3. Gets the shared `OkHttpClient` from `WeirdStreamerApplication`
4. Creates `OkHttpDataSource.Factory` with that client
5. ExoPlayer uses OkHttp for all HTTP requests, which:
   - **Bypasses SSL validation** (accepts self-signed certificates)
   - **Adds JWT auth headers** via the interceptor
6. Video streams successfully

### For SMB Videos:
1. User clicks video from SMB source
2. `VideoPlayer` detects it's SMB (`path.startsWith("smb://")`)
3. Uses custom `SmbDataSourceFactory`
4. Video streams via SMB protocol (unchanged behavior)

## Benefits

### SSL Certificate Handling
- ✅ Works with self-signed certificates (common for local servers)
- ✅ Works with valid SSL certificates
- ✅ Consistent with image/thumbnail loading (uses same OkHttpClient)

### Authentication
- ✅ JWT tokens automatically added to video streaming requests
- ✅ Reuses existing auth infrastructure from `WeirdStreamerApplication`
- ✅ No need to manually pass tokens to ExoPlayer

### Performance
- ✅ Reuses connection pool from OkHttpClient
- ✅ HTTP/2 support (if server supports it)
- ✅ Efficient buffering and caching

## Files Modified

1. ✅ `gradle/libs.versions.toml` - Added media3-datasource-okhttp dependency
2. ✅ `app/build.gradle.kts` - Added dependency to app module
3. ✅ `WeirdStreamerApplication.kt` - Exposed OkHttpClient as singleton
4. ✅ `ui/components/VideoPlayer.kt` - Use OkHttp DataSource for server videos

## Testing

### Server Video with HTTPS Test:
1. Configure server with HTTPS (self-signed cert is fine)
2. Browse to server source
3. Click on a video file
4. **Expected:** Video plays successfully without SSL errors
5. **Check logs:** Should NOT see `SSLHandshakeException`

### Server Video with HTTP Test:
1. Configure server with HTTP (no SSL)
2. Browse to server source
3. Click on a video file
4. **Expected:** Video plays successfully
5. Works with both HTTP and HTTPS

### SMB Video Test:
1. Browse to SMB share
2. Click on a video file
3. **Expected:** Video still plays (no regression)
4. SMB videos use separate DataSource

## Verification Checklist

- [x] Added media3-datasource-okhttp dependency
- [x] Exposed OkHttpClient from WeirdStreamerApplication
- [x] Updated VideoPlayer to use OkHttp DataSource for server videos
- [x] Code compiles without errors
- [ ] HTTPS videos play without SSL errors (test required)
- [ ] HTTP videos still work (test required)
- [ ] SMB videos still work (test required)
- [ ] Authentication headers sent with video requests (check server logs)

## Related Components

This fix completes the server streaming support:
- **Images** - Use Coil with OkHttpClient (already working)
- **PDFs** - Use ServerRepository.getInputStream() with OkHttp (fixed earlier)
- **Videos** - Use ExoPlayer with OkHttp DataSource (fixed now)

All server media access now uses the same `OkHttpClient` instance with:
- SSL certificate validation bypass
- JWT authentication interceptor
- Connection pooling and HTTP/2 support

## Security Note

⚠️ **Production Deployment:** For production use, consider:
1. Using valid SSL certificates instead of disabling validation
2. Implementing proper certificate pinning
3. The current SSL bypass is appropriate for:
   - Local network servers with self-signed certs
   - Development/testing environments
   - Trusted private networks

For public internet deployment, use proper SSL certificates from a trusted CA.
