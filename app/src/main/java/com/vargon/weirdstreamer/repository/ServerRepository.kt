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
                        items.add(
                            FileItem(
                                name = mediaItem.name,
                                path = mediaItem.path,
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
        return "$serverUrl/api/media/stream?path=$encodedPath"
    }
    
    override fun getThumbnailUrl(path: String): String? {
        // Thumbnail URL is provided by the API in MediaItem.thumbnailUrl
        // This method might not be needed for server source
        return null
    }
}
