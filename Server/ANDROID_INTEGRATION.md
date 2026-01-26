# Android Client Integration Guide

This guide helps you integrate the Media Streaming Server with your Android app.

## Method 1: Using OpenAPI Generator (Recommended)

### Generate Kotlin/Java Client

1. Install OpenAPI Generator:
```bash
npm install -g @openapitools/openapi-generator-cli
# or
brew install openapi-generator
```

2. Generate Retrofit client:
```bash
openapi-generator-cli generate \
  -i api/openapi.yaml \
  -g kotlin \
  -o android-client \
  --library jvm-retrofit2 \
  --additional-properties=packageName=com.example.mediastreamer
```

3. Add generated files to your Android project under `app/src/main/java/`

### Add Dependencies to build.gradle

```gradle
dependencies {
    // Retrofit for API calls
    implementation 'com.squareup.retrofit2:retrofit:2.9.0'
    implementation 'com.squareup.retrofit2:converter-gson:2.9.0'
    implementation 'com.squareup.retrofit2:adapter-rxjava3:2.9.0'
    
    // OkHttp for HTTP client
    implementation 'com.squareup.okhttp3:okhttp:4.11.0'
    implementation 'com.squareup.okhttp3:logging-interceptor:4.11.0'
    
    // Gson for JSON parsing
    implementation 'com.google.code.gson:gson:2.10.1'
    
    // ExoPlayer for video streaming
    implementation 'com.google.android.exoplayer:exoplayer:2.19.1'
    
    // Coil or Glide for image loading
    implementation 'io.coil-kt:coil:2.5.0'
    
    // Kotlin Coroutines
    implementation 'org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3'
}
```

## Method 2: Manual Integration

### 1. Create API Service Interface

```kotlin
package com.example.mediastreamer.api

import okhttp3.ResponseBody
import retrofit2.Call
import retrofit2.Response
import retrofit2.http.*

interface MediaApiService {
    
    @POST("/api/auth/login")
    suspend fun login(@Body request: LoginRequest): Response<LoginResponse>
    
    @GET("/api/media")
    suspend fun getMediaList(
        @Query("type") type: String? = null
    ): Response<List<MediaItem>>
    
    @GET("/api/media/browse")
    suspend fun browseDirectory(
        @Query("path") path: String? = null
    ): Response<Directory>
    
    @Streaming
    @GET("/api/media/stream")
    fun streamMedia(@Query("path") path: String): Call<ResponseBody>
    
    @POST("/api/media/scan")
    suspend fun scanMedia(): Response<Map<String, String>>
}
```

### 2. Define Data Models

```kotlin
package com.example.mediastreamer.models

import com.google.gson.annotations.SerializedName

data class LoginRequest(
    val username: String,
    val password: String
)

data class LoginResponse(
    val token: String,
    @SerializedName("expires_at")
    val expiresAt: Long,
    val user: User
)

data class User(
    val id: Long,
    val username: String,
    @SerializedName("is_admin")
    val isAdmin: Boolean,
    @SerializedName("created_at")
    val createdAt: String
)

data class MediaItem(
    val id: Long,
    val path: String,
    val name: String,
    val type: String, // "video", "image", "document"
    val size: Long,
    @SerializedName("mime_type")
    val mimeType: String,
    @SerializedName("thumbnail_url")
    val thumbnailUrl: String?,
    val duration: Double?,
    val width: Int?,
    val height: Int?,
    @SerializedName("created_at")
    val createdAt: String,
    @SerializedName("updated_at")
    val updatedAt: String
)

data class Directory(
    val path: String,
    val name: String,
    val items: List<MediaItem>?,
    val dirs: List<Directory>?
)

data class ErrorResponse(
    val error: String,
    val message: String?
)
```

### 3. Create Retrofit Client

```kotlin
package com.example.mediastreamer.network

import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.concurrent.TimeUnit

object RetrofitClient {
    private const val BASE_URL = "http://your-server-ip:8080/"
    
    private var authToken: String? = null
    
    fun setAuthToken(token: String?) {
        authToken = token
    }
    
    private val loggingInterceptor = HttpLoggingInterceptor().apply {
        level = HttpLoggingInterceptor.Level.BODY
    }
    
    private val authInterceptor = Interceptor { chain ->
        val request = chain.request().newBuilder()
        authToken?.let {
            request.addHeader("Authorization", "Bearer $it")
        }
        chain.proceed(request.build())
    }
    
    private val okHttpClient = OkHttpClient.Builder()
        .addInterceptor(authInterceptor)
        .addInterceptor(loggingInterceptor)
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()
    
    private val retrofit = Retrofit.Builder()
        .baseUrl(BASE_URL)
        .client(okHttpClient)
        .addConverterFactory(GsonConverterFactory.create())
        .build()
    
    val apiService: MediaApiService = retrofit.create(MediaApiService::class.java)
}
```

### 4. Create Repository

```kotlin
package com.example.mediastreamer.repository

import com.example.mediastreamer.models.*
import com.example.mediastreamer.network.RetrofitClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class MediaRepository {
    
    private val api = RetrofitClient.apiService
    
    suspend fun login(username: String, password: String): Result<LoginResponse> {
        return withContext(Dispatchers.IO) {
            try {
                val response = api.login(LoginRequest(username, password))
                if (response.isSuccessful && response.body() != null) {
                    val loginResponse = response.body()!!
                    RetrofitClient.setAuthToken(loginResponse.token)
                    Result.success(loginResponse)
                } else {
                    Result.failure(Exception("Login failed"))
                }
            } catch (e: Exception) {
                Result.failure(e)
            }
        }
    }
    
    suspend fun getMediaList(type: String? = null): Result<List<MediaItem>> {
        return withContext(Dispatchers.IO) {
            try {
                val response = api.getMediaList(type)
                if (response.isSuccessful && response.body() != null) {
                    Result.success(response.body()!!)
                } else {
                    Result.failure(Exception("Failed to fetch media"))
                }
            } catch (e: Exception) {
                Result.failure(e)
            }
        }
    }
    
    suspend fun browseDirectory(path: String? = null): Result<Directory> {
        return withContext(Dispatchers.IO) {
            try {
                val response = api.browseDirectory(path)
                if (response.isSuccessful && response.body() != null) {
                    Result.success(response.body()!!)
                } else {
                    Result.failure(Exception("Failed to browse directory"))
                }
            } catch (e: Exception) {
                Result.failure(e)
            }
        }
    }
    
    fun getStreamUrl(path: String): String {
        return "${RetrofitClient.BASE_URL}api/media/stream?path=${path}"
    }
    
    fun getThumbnailUrl(thumbnailUrl: String): String {
        return "${RetrofitClient.BASE_URL}${thumbnailUrl.trimStart('/')}"
    }
}
```

### 5. Video Player Activity

```kotlin
package com.example.mediastreamer.ui

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import com.google.android.exoplayer2.ExoPlayer
import com.google.android.exoplayer2.MediaItem
import com.google.android.exoplayer2.ui.PlayerView
import com.example.mediastreamer.databinding.ActivityVideoPlayerBinding

class VideoPlayerActivity : AppCompatActivity() {
    
    private lateinit var binding: ActivityVideoPlayerBinding
    private var player: ExoPlayer? = null
    
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityVideoPlayerBinding.inflate(layoutInflater)
        setContentView(binding.root)
        
        val videoUrl = intent.getStringExtra("VIDEO_URL") ?: return
        
        player = ExoPlayer.Builder(this).build().apply {
            binding.playerView.player = this
            
            val mediaItem = MediaItem.Builder()
                .setUri(videoUrl)
                .build()
            
            setMediaItem(mediaItem)
            prepare()
            playWhenReady = true
        }
    }
    
    override fun onStop() {
        super.onStop()
        player?.release()
    }
}
```

Layout file `activity_video_player.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<androidx.constraintlayout.widget.ConstraintLayout 
    xmlns:android="http://schemas.android.com/apk/res/android"
    xmlns:app="http://schemas.android.com/apk/res-auto"
    android:layout_width="match_parent"
    android:layout_height="match_parent">
    
    <com.google.android.exoplayer2.ui.PlayerView
        android:id="@+id/playerView"
        android:layout_width="match_parent"
        android:layout_height="match_parent"
        app:layout_constraintTop_toTopOf="parent"
        app:layout_constraintBottom_toBottomOf="parent"
        app:layout_constraintStart_toStartOf="parent"
        app:layout_constraintEnd_toEndOf="parent" />
        
</androidx.constraintlayout.widget.ConstraintLayout>
```

### 6. Image Loading with Coil

```kotlin
import coil.load
import coil.request.CachePolicy

imageView.load(repository.getThumbnailUrl(mediaItem.thumbnailUrl ?: "")) {
    crossfade(true)
    placeholder(R.drawable.placeholder)
    error(R.drawable.error_image)
    memoryCachePolicy(CachePolicy.ENABLED)
    diskCachePolicy(CachePolicy.ENABLED)
}
```

### 7. ViewModel Example

```kotlin
package com.example.mediastreamer.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.example.mediastreamer.models.MediaItem
import com.example.mediastreamer.repository.MediaRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

class MediaViewModel : ViewModel() {
    
    private val repository = MediaRepository()
    
    private val _mediaItems = MutableStateFlow<List<MediaItem>>(emptyList())
    val mediaItems: StateFlow<List<MediaItem>> = _mediaItems
    
    private val _isLoading = MutableStateFlow(false)
    val isLoading: StateFlow<Boolean> = _isLoading
    
    fun login(username: String, password: String) {
        viewModelScope.launch {
            _isLoading.value = true
            repository.login(username, password).onSuccess {
                loadMedia()
            }.onFailure {
                // Handle error
            }
            _isLoading.value = false
        }
    }
    
    fun loadMedia(type: String? = null) {
        viewModelScope.launch {
            _isLoading.value = true
            repository.getMediaList(type).onSuccess { items ->
                _mediaItems.value = items
            }.onFailure {
                // Handle error
            }
            _isLoading.value = false
        }
    }
    
    fun getStreamUrl(path: String) = repository.getStreamUrl(path)
    
    fun getThumbnailUrl(url: String) = repository.getThumbnailUrl(url)
}
```

## Permissions

Add to `AndroidManifest.xml`:

```xml
<uses-permission android:name="android.permission.INTERNET" />
<uses-permission android:name="android.permission.ACCESS_NETWORK_STATE" />

<!-- If using HTTP instead of HTTPS -->
<application
    android:usesCleartextTraffic="true"
    ...>
```

## ProGuard Rules

Add to `proguard-rules.pro`:

```proguard
# Retrofit
-dontwarn retrofit2.**
-keep class retrofit2.** { *; }
-keepattributes Signature
-keepattributes Exceptions

# Gson
-keep class com.google.gson.** { *; }
-keep class com.example.mediastreamer.models.** { *; }

# OkHttp
-dontwarn okhttp3.**
-keep class okhttp3.** { *; }
```

## Testing

```kotlin
class MediaApiTest {
    private lateinit var repository: MediaRepository
    
    @Before
    fun setup() {
        repository = MediaRepository()
    }
    
    @Test
    fun testLogin() = runBlocking {
        val result = repository.login("admin", "password")
        assertTrue(result.isSuccess)
    }
}
```

## Best Practices

1. **Use HTTPS in production** - Enable TLS on the server
2. **Token Storage** - Use EncryptedSharedPreferences for token storage
3. **Error Handling** - Implement proper error handling and user feedback
4. **Caching** - Use Room database for offline caching
5. **Background Downloads** - Use WorkManager for downloading media
6. **Network Security** - Add network security config for certificate pinning

## Troubleshooting

**Issue: Connection refused**
- Ensure server is running
- Check firewall settings
- Use correct IP address (not localhost on real device)

**Issue: Cleartext traffic not permitted**
- Add `usesCleartextTraffic="true"` for HTTP or use HTTPS

**Issue: Video won't play**
- Check video codec compatibility (use H.264)
- Ensure proper authentication headers
- Verify network connectivity

For more help, see the main README.md
