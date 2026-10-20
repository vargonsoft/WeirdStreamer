package com.vargon.weirdstreamer.repository

import android.util.Base64
import okhttp3.Request
import com.vargon.weirdstreamer.CredentialManager
import com.vargon.weirdstreamer.api.ApiClient
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.StreamProgress
import com.vargon.weirdstreamer.api.models.LibraryFile
import com.vargon.weirdstreamer.api.models.LibraryListing
import com.vargon.weirdstreamer.api.models.LoginRequest
import com.vargon.weirdstreamer.api.models.TitleDetail
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.InputStream

class ServerRepository(
    private val serverUrl: String,
    private val credentialManager: CredentialManager
) : MediaRepository {
    
    private val apiService: ApiService = ApiClient.create(serverUrl, credentialManager)

    private var shareMap = mutableMapOf<String, String>()
    // Library ID -> "movie" or "tv", learned from the root listing and browse responses
    private var libraryTypes = mutableMapOf<String, String>()
    override suspend fun inheritState(other: MediaRepository?) {
        (other as? ServerRepository)?.let { previous ->
            shareMap = previous.shareMap
            libraryTypes = previous.libraryTypes
        }
    }

    /** The library a browse path is in: its first segment. */
    fun libraryIdOf(path: String): String = path.substringBefore("/")

    /** "movie" or "tv" when [path] is inside a library with a type, else null. */
    fun libraryTypeFor(path: String): String? = if (path.isEmpty()) null else libraryTypes[libraryIdOf(path)]

    fun libraryName(libraryId: String): String = shareMap[libraryId] ?: libraryId

    /** Server-relative artwork/thumbnail URLs as absolute URLs. */
    fun absoluteUrl(url: String?): String? = url?.let { if (it.startsWith("http")) it else "$serverUrl$it" }

    /** A Library-view file as a playable [FileItem] (stream URL path, like browse items). */
    fun fileItemFor(file: LibraryFile): FileItem = FileItem(
        name = file.name,
        path = getStreamUrl(file.path),
        isDirectory = false,
        size = file.size,
        thumbnailUrl = absoluteUrl(file.thumbnailUrl),
        duration = file.duration,
        originalPath = file.path,
        id = file.id
    )

    /** Tells the server where the player is in [originalPath], for its admin Streams page. */
    suspend fun reportPlayback(originalPath: String, positionSeconds: Double, paused: Boolean) = withContext(Dispatchers.IO) {
        runCatching {
            val encoded = Base64.encodeToString(originalPath.toByteArray(), Base64.NO_WRAP)
            apiService.reportStreamProgress(StreamProgress(encoded, positionSeconds, paused))
        }
    }

    suspend fun library(libraryId: String): Result<LibraryListing> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.getLibrary(libraryId)
            val body = response.body()
            if (response.isSuccessful && body != null) Result.success(body)
            else Result.failure(Exception("Server error: ${response.code()}"))
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun libraryTitle(libraryId: String, kind: String, tmdbId: Int): Result<TitleDetail> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.getLibraryTitle(libraryId, kind, tmdbId)
            val body = response.body()
            if (response.isSuccessful && body != null) Result.success(body)
            else Result.failure(Exception("Server error: ${response.code()}"))
        } catch (e: Exception) {
            Result.failure(e)
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
                    
                    directory.libraryType?.let { if (path.isNotEmpty()) libraryTypes[libraryIdOf(path)] = it }
                    directory.dirs?.forEach { dir ->
                        if (!dir.id.contains("/") && !dir.name.contains("/")) {
                            shareMap[dir.id] = dir.name
                            dir.libraryType?.let { libraryTypes[dir.id] = it }
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
                                originalPath = mediaItem.path,
                                id = mediaItem.id
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
                    
                    Result.success(
                        FileItem(
                            name = mediaItem.name,
                            path = streamingPath,
                            isDirectory = false,
                            size = mediaItem.size,
                            mimeType = mediaItem.mimeType,
                            thumbnailUrl = item.thumbnailUrl,
                            duration = mediaItem.duration,
                            width = mediaItem.width,
                            height = mediaItem.height,
                            originalPath = mediaItem.path,
                            id = mediaItem.id
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

    override fun supportsTrickplay(): Boolean = true
    override suspend fun getTrickplayInfo(id: Long): Result<TrickplayInfo> = withContext(Dispatchers.IO) {
        try {
            val response = apiService.getTrickplayInfo(id)
            if (response.isSuccessful && response.body() != null) {
                val info = response.body()!!
                // Prepend server URL to frame URLs if they are relative
                val absoluteFrames = info.frames.map { frame ->
                    if (frame.url.startsWith("http")) frame
                    else frame.copy(url = "$serverUrl${if (frame.url.startsWith("/")) "" else "/"}${frame.url}")
                }
                Result.success(TrickplayInfo(absoluteFrames))
            } else {
                Result.failure(Exception("Trickplay not found"))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
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
    override val addressPlaceholder: String = "myserver or https://myserver:9000"
    override val addressHint: String = "The default port is $DEFAULT_PORT. Without http:// or https://, the server is asked which it uses."

    /**
     * "myserver" becomes "https://myserver:9000": a missing port is the default, and a missing
     * scheme is whichever the server answers on, https first (an https server also answers plain
     * http, with an error, so http alone can't tell).
     */
    override suspend fun normalizeAddress(address: String): String = withContext(Dispatchers.IO) {
        val typed = address.trim().trimEnd('/')
        if (typed.contains("://")) return@withContext typed
        val hostPort = if (hasPort(typed)) typed else "$typed:$DEFAULT_PORT"
        val client = ApiClient.probeClient()
        val scheme = listOf("https", "http").firstOrNull { scheme ->
            runCatching {
                client.newCall(Request.Builder().url("$scheme://$hostPort/").head().build()).execute().close()
            }.isSuccess
        } ?: "https"
        "$scheme://$hostPort"
    }

    private fun hasPort(hostPort: String): Boolean {
        val afterBracket = hostPort.substringAfterLast(']') // [ipv6]:port
        val colon = afterBracket.lastIndexOf(':')
        if (colon < 0) return false
        if (!hostPort.startsWith("[") && hostPort.count { it == ':' } > 1) return false // bare IPv6
        return afterBracket.substring(colon + 1).let { it.isNotEmpty() && it.all(Char::isDigit) }
    }

    companion object {
        const val DEFAULT_PORT = 9000
    }
    override fun getServers(): List<String> = credentialManager.getApiServers().toList()
    override fun addServer(address: String) { credentialManager.addApiServer(address) }
    override fun removeServer(address: String) { credentialManager.removeApiServer(address) }
    override fun createRepository(serverAddress: String): MediaRepository = ServerRepository(serverAddress, credentialManager)
}
