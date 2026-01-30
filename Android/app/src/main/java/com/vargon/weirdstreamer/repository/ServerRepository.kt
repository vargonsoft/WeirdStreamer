package com.vargon.weirdstreamer.repository

import android.util.Base64
import com.vargon.weirdstreamer.CredentialManager
import com.vargon.weirdstreamer.api.ApiClient
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.LoginRequest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.InputStream

class ServerRepository(
    private val serverUrl: String,
    private val credentialManager: CredentialManager
) : MediaRepository {
    
    private val apiService: ApiService = ApiClient.create(serverUrl, credentialManager)

    private var shareMap = mutableMapOf<String, String>()
    override suspend fun inheritState(other: MediaRepository?) {
        (other as? ServerRepository)?.let { previous ->
            shareMap = previous.shareMap
        }
    }

    override suspend fun browse(path: String): Result<List<FileItem>> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.browse(path)
            if (response.isSuccessful) {
                val directory = response.body()
                if (directory != null) {
                    val items = mutableListOf<FileItem>()
                    val cacheVersion = credentialManager.getThumbnailCacheVersion(serverUrl, path)
                    
                    directory.dirs?.forEach { dir ->
                        if (!dir.id.contains("/") && !dir.name.contains("/")) {
                            shareMap[dir.id] = dir.name
                        }
                        items.add(FileItem(name = dir.name, path = dir.id, isDirectory = true))
                    }
                    
                    directory.items?.forEach { mediaItem ->
                        val streamingPath = getStreamUrl(mediaItem.path)
                        items.add(
                            FileItem(
                                name = mediaItem.name,
                                path = streamingPath,
                                isDirectory = false,
                                size = mediaItem.size,
                                mimeType = mediaItem.mimeType,
                                thumbnailUrl = mediaItem.thumbnailUrl?.let { 
                                    val baseUrl = if (it.startsWith("http")) it else "$serverUrl$it"
                                    if (cacheVersion > 0) "$baseUrl?v=$cacheVersion" else baseUrl
                                },
                                duration = mediaItem.duration,
                                width = mediaItem.width,
                                height = mediaItem.height,
                                originalPath = mediaItem.path
                            )
                        )
                    }
                    Result.success(items)
                } else Result.failure(Exception("Empty response"))
            } else Result.failure(Exception("Server error: ${response.code()}"))
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override suspend fun getMetadata(item: FileItem): Result<FileItem> = withContext(Dispatchers.IO) {
        try {
            val path = item.path
            // Check if this is a streaming URL and extract original path
            val actualPath = if (path.contains("/api/media/stream?path=")) {
                try {
                    val encodedPath = path.substringAfter("path=").substringBefore("&")
                    String(Base64.decode(encodedPath, Base64.NO_WRAP))
                } catch (e: Exception) { path }
            } else path

            // Use Base64 encoding for the path when calling metadata API, matching stream API requirements
            val encodedPathForApi = Base64.encodeToString(actualPath.toByteArray(), Base64.NO_WRAP)
            val response = apiService.getMetadata(encodedPathForApi)
            if (response.isSuccessful) {
                val mediaItem = response.body()
                if (mediaItem != null) {
                    val streamingPath = getStreamUrl(mediaItem.path)
                    
                    // Maintain thumbnail cache versioning
                    val parentPath = actualPath.substringBeforeLast("/", "")
                    val cacheVersion = credentialManager.getThumbnailCacheVersion(serverUrl, parentPath)
                    
                    Result.success(
                        FileItem(
                            name = mediaItem.name,
                            path = streamingPath,
                            isDirectory = false,
                            size = mediaItem.size,
                            mimeType = mediaItem.mimeType,
                            // Preserve the cache-buster versioning for thumbnails
                            thumbnailUrl = item.thumbnailUrl,
//                            thumbnailUrl = mediaItem.thumbnailUrl?.let {
//                                val baseUrl = if (it.startsWith("http")) it else "$serverUrl$it"
//                                if (cacheVersion > 0) "$baseUrl?v=$cacheVersion" else baseUrl
//                            },
                            duration = mediaItem.duration,
                            width = mediaItem.width,
                            height = mediaItem.height,
                            originalPath = mediaItem.path
                        )
                    )
                } else Result.failure(Exception("Empty body"))
            } else Result.failure(Exception("Metadata failed"))
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override suspend fun authenticate(username: String?, password: String?): Result<Boolean> = withContext(Dispatchers.IO) {
        try {
            if (username.isNullOrBlank() || password.isNullOrBlank()) return@withContext Result.failure(Exception("Required"))
            val response = apiService.login(LoginRequest(username, password))
            if (response.isSuccessful && response.body() != null) {
                credentialManager.saveServerToken(serverUrl, response.body()!!.token, response.body()!!.expiresAt)
                credentialManager.addApiServer(serverUrl)
                Result.success(true)
            } else Result.failure(Exception("Login failed"))
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override fun getStreamUrl(path: String): String {
        val encodedPath = Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)
        return "$serverUrl/api/media/stream?path=$encodedPath&auto-transcode=0"
    }
    
    override fun getThumbnailUrl(path: String): String? = null

    override fun getDisplayName(path: String): String {
        if (path.isEmpty()) return "/"
        shareMap[path]?.let {
            return it
        }
        return if (path.contains("/api/media/stream?path=")) {
            try {
                val encodedPath = path.substringAfter("path=").substringBefore("&")
                String(Base64.decode(encodedPath, Base64.NO_WRAP)).substringAfterLast('/')
            } catch (e: Exception) { "stream" }
        } else path.substringAfterLast('/')
    }

    override suspend fun getInputStream(path: String): Result<InputStream> = withContext(Dispatchers.IO) {
        try {
            val actualPath = if (path.contains("/api/media/stream?path=")) {
                try {
                    val encodedPath = path.substringAfter("path=").substringBefore("&")
                    String(Base64.decode(encodedPath, Base64.NO_WRAP))
                } catch (_: Exception) { path }
            } else path
            val encodedPath = Base64.encodeToString(actualPath.toByteArray(), Base64.NO_WRAP)
            val response = apiService.stream(encodedPath)
            if (response.isSuccessful && response.body() != null) Result.success(response.body()!!.byteStream())
            else Result.failure(Exception("Stream failed"))
        } catch (e: Exception) { Result.failure(e) }
    }

    override fun supportsThumbnailRegeneration(): Boolean = true
    override suspend fun regenerateThumbnails(path: String): Result<Boolean> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.regenerateThumbnails(path)
            if (response.isSuccessful) {
                credentialManager.incrementThumbnailCacheVersion(serverUrl, path)
                Result.success(true)
            } else Result.failure(Exception("Failed"))
        } catch (e: Exception) { Result.failure(e) }
    }

    override fun isAuthRequired(exception: Throwable): Boolean = exception.message?.contains("401") == true
    
    override fun getInitialPath(): String = ""
    
    override fun getParentPath(currentPath: String): String? {
        if (currentPath.isEmpty()) return null
        return currentPath.substringBeforeLast("/", "")
    }
    
    fun getToken(): String? = credentialManager.getServerToken(serverUrl)
}

class ServerSourceProvider(
    private val credentialManager: CredentialManager
) : SourceProvider {
    override val typeId: String = "SERVER"
    override val displayName: String = "WeirdStreamer Server"
    override val addressLabel: String = "Server URL"
    override val addressPlaceholder: String = "http://192.168.1.100:8080"
    override fun getServers(): List<String> = credentialManager.getApiServers().toList()
    override fun addServer(address: String) { credentialManager.addApiServer(address) }
    override fun removeServer(address: String) { credentialManager.removeApiServer(address) }
    override fun createRepository(serverAddress: String): MediaRepository = ServerRepository(serverAddress, credentialManager)
}
