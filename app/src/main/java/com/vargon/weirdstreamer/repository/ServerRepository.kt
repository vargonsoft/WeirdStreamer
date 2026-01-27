package com.vargon.weirdstreamer.repository

import android.util.Base64
import com.vargon.weirdstreamer.CredentialManager
import com.vargon.weirdstreamer.api.ApiClient
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.LoginRequest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class ServerRepository(
    private val serverUrl: String,
    private val credentialManager: CredentialManager
) : MediaRepository {
    
    private val apiService: ApiService = ApiClient.create(serverUrl, credentialManager)
    
    override suspend fun browse(path: String): Result<List<FileItem>> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.browse(path)
            if (response.isSuccessful) {
                val directory = response.body()
                if (directory != null) {
                    val items = mutableListOf<FileItem>()
                    
                    // Add subdirectories
                    directory.dirs?.forEach { dir ->
                        items.add(
                            FileItem(
                                name = dir.name,
                                path = dir.path,
                                isDirectory = true
                            )
                        )
                    }
                    
                    // Add media items
                    directory.items?.forEach { mediaItem ->
                        // For server items, store the streaming URL in the path
                        val streamingPath = getStreamUrl(mediaItem.path)
                        items.add(
                            FileItem(
                                name = mediaItem.name,
                                path = streamingPath,
                                isDirectory = false,
                                size = mediaItem.size,
                                mimeType = mediaItem.mimeType,
                                thumbnailUrl = mediaItem.thumbnailUrl?.let { 
                                    if (it.startsWith("http")) it else "$serverUrl$it" 
                                },
                                duration = mediaItem.duration,
                                width = mediaItem.width,
                                height = mediaItem.height
                            )
                        )
                    }
                    
                    Result.success(items)
                } else {
                    Result.failure(Exception("Empty response from server"))
                }
            } else {
                Result.failure(Exception("Server error: ${response.code()} ${response.message()}"))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override suspend fun authenticate(username: String?, password: String?): Result<Boolean> = withContext(Dispatchers.IO) {
        try {
            if (username.isNullOrBlank() || password.isNullOrBlank()) {
                return@withContext Result.failure(Exception("Username and password required"))
            }
            
            val loginRequest = LoginRequest(username, password)
            val response = apiService.login(loginRequest)
            
            if (response.isSuccessful) {
                val loginResponse = response.body()
                if (loginResponse != null) {
                    // Save token and expiration
                    credentialManager.saveServerToken(
                        serverUrl,
                        loginResponse.token,
                        loginResponse.expiresAt
                    )
                    // Add to saved API servers
                    credentialManager.addApiServer(serverUrl)
                    Result.success(true)
                } else {
                    Result.failure(Exception("Empty login response"))
                }
            } else {
                Result.failure(Exception("Login failed: ${response.code()} ${response.message()}"))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override fun getStreamUrl(path: String): String {
        // Server requires base64-encoded paths
        val encodedPath = Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)
        // Disable auto-transcoding since Android supports most formats natively
        return "$serverUrl/api/media/stream?path=$encodedPath&auto-transcode=0"
    }
    
    override fun getThumbnailUrl(path: String): String? {
        // Thumbnail URL is provided by the API in MediaItem.thumbnailUrl
        // This method might not be needed for server source
        return null
    }

    override fun getDisplayName(path: String): String {
        // Extract filename from path - handle both regular paths and stream URLs
        return if (path.contains("/api/media/stream?path=")) {
            // Decode base64 path to get actual filename
            try {
                val encodedPath = path.substringAfter("path=").substringBefore("&")
                val decodedPath = String(Base64.decode(encodedPath, Base64.NO_WRAP))
                decodedPath.substringAfterLast('/')
            } catch (e: Exception) {
                "stream"
            }
        } else {
            path.substringAfterLast('/')
        }
    }

    override suspend fun getInputStream(path: String): Result<java.io.InputStream> = withContext(Dispatchers.IO) {
        try {
            // Extract the original path from the stream URL if needed
            val actualPath = if (path.contains("/api/media/stream?path=")) {
                try {
                    val encodedPath = path.substringAfter("path=").substringBefore("&")
                    String(Base64.decode(encodedPath, Base64.NO_WRAP))
                } catch (_: Exception) {
                    path
                }
            } else {
                path
            }

            android.util.Log.d("ServerRepository", "getInputStream - actualPath: $actualPath")

            // Encode the path for the API (server expects base64-encoded paths)
            val encodedPath = Base64.encodeToString(actualPath.toByteArray(), Base64.NO_WRAP)
            android.util.Log.d("ServerRepository", "getInputStream - encodedPath: $encodedPath")

            val response = apiService.stream(encodedPath)
            android.util.Log.d("ServerRepository", "getInputStream - response code: ${response.code()}")

            if (response.isSuccessful) {
                val inputStream = response.body()?.byteStream()
                if (inputStream != null) {
                    android.util.Log.d("ServerRepository", "getInputStream - success")
                    Result.success<java.io.InputStream>(inputStream)
                } else {
                    android.util.Log.e("ServerRepository", "getInputStream - empty response body")
                    Result.failure(Exception("Empty response body"))
                }
            } else {
                val errorMsg = "Server error: ${response.code()} ${response.message()}"
                android.util.Log.e("ServerRepository", "getInputStream - $errorMsg")
                Result.failure(Exception(errorMsg))
            }
        } catch (e: Exception) {
            android.util.Log.e("ServerRepository", "getInputStream - exception", e)
            Result.failure(e)
        }
    }
}
