package com.vargon.weirdstreamer.repository

import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import android.util.Base64
import okhttp3.Request
import com.vargon.weirdstreamer.CredentialManager
import com.vargon.weirdstreamer.api.ApiClient
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.StreamProgress
import com.vargon.weirdstreamer.api.models.ContinueItem
import com.vargon.weirdstreamer.api.models.MediaSegments
import com.vargon.weirdstreamer.api.models.AutoSkipRequest
import com.vargon.weirdstreamer.api.models.PlaybackSync
import com.vargon.weirdstreamer.api.models.SyncedPlayback
import com.vargon.weirdstreamer.api.models.WatchedRequest
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
    /** The address for building media URLs (thumbnails, streams, trickplay frames). */
    private val requestBase = normalizedServerUrl(serverUrl)

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
    /**
     * Every video under a folder (browse path), for downloading a whole folder or show: its own
     * videos and those of its subfolders, up to [limit].
     */
    suspend fun videosUnder(path: String, limit: Int = 5000): List<FileItem> {
        val found = mutableListOf<FileItem>()
        val pending = ArrayDeque(listOf(path))
        while (pending.isNotEmpty() && found.size < limit) {
            val items = browse(pending.removeFirst()).getOrNull() ?: continue
            items.forEach { item ->
                when {
                    item.isDirectory && item.path != "UP" && item.name != ".." -> pending.add(item.path)
                    !item.isDirectory && isDownloadableVideo(item.name) -> found.add(item)
                }
            }
        }
        return found.take(limit)
    }

    private fun isDownloadableVideo(name: String) =
        name.substringAfterLast('.', "").lowercase() in setOf(
            "mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "3gp", "3gpp", "ts", "mp2", "divx", "xvid", "iso")

    /** The server's API, for the admin settings screens. */
    val api: ApiService get() = apiService

    val address: String get() = serverUrl

    /** What [path] (a server path) would download as, for the quality dialog's estimate. */
    suspend fun downloadOptions(path: String): com.vargon.weirdstreamer.api.models.DownloadOptions? = runCatching {
        apiService.downloadOptions(Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)).body()
    }.getOrNull()

    /** A file's container, video, audio and subtitle tracks; null when the server can't say. */
    suspend fun codecs(path: String): com.vargon.weirdstreamer.api.models.CodecsInfo? = runCatching {
        apiService.getCodecs(Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)).body()
    }.getOrNull()

    /** Who the saved login is, from its token's claims: username and whether they're an admin. */
    fun signedInAs(): Pair<String?, Boolean> {
        val token = credentialManager.getServerToken(serverUrl) ?: return null to false
        return runCatching {
            val payload = token.split(".")[1]
            val json = String(Base64.decode(payload, Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING))
            val claims = com.google.gson.JsonParser.parseString(json).asJsonObject
            claims.get("username")?.asString to (claims.get("is_admin")?.asBoolean ?: false)
        }.getOrDefault(null to false)
    }

    fun absoluteUrl(url: String?): String? = url?.let { if (it.startsWith("http")) it else "$requestBase$it" }

    /** A Library-view file as a playable [FileItem] (stream URL path, like browse items). */
    fun fileItemFor(file: LibraryFile): FileItem = FileItem(
        name = file.name,
        path = getStreamUrl(file.path),
        isDirectory = false,
        size = file.size,
        thumbnailUrl = absoluteUrl(file.thumbnailUrl),
        duration = file.duration,
        originalPath = file.path,
        id = file.id,
        playback = file.playback
    )

    private fun encode(path: String) = Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)

    /**
     * Tells the server where the player is in [originalPath], for its admin Streams page and the
     * user's progress; [ended] when it played to the end, [discTitle] for discs (0: main title).
     */
    suspend fun reportPlayback(originalPath: String, positionSeconds: Double, paused: Boolean, ended: Boolean = false,
                               discTitle: Int = 0) = withContext(Dispatchers.IO) {
        runCatching {
            apiService.reportStreamProgress(StreamProgress(encode(originalPath), positionSeconds, paused, ended, discTitle))
        }
    }

    /** Marks server files watched or unwatched, or a whole title by [library], [kind] and [tmdbId]. */
    suspend fun setWatched(originalPaths: List<String>, watched: Boolean, library: String? = null, kind: String? = null,
                           tmdbId: Int? = null): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val r = apiService.setWatched(WatchedRequest(originalPaths.map(::encode), watched, library, kind, tmdbId))
            if (!r.isSuccessful) throw Exception("Server error: ${r.code()}")
        }
    }

    /** A video's intro and credits (by its library path), or null when the server can't say. */
    suspend fun segments(originalPath: String): MediaSegments? = withContext(Dispatchers.IO) {
        runCatching { apiService.getSegments(encode(originalPath)).body() }.getOrNull()
    }

    /** Has the players skip a show's intros and credits by themselves (or not). */
    suspend fun setAutoSkip(tmdbId: Int, on: Boolean): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val r = apiService.setAutoSkip(AutoSkipRequest(tmdbId, on))
            if (!r.isSuccessful) throw Exception("Server error: ${r.code()}")
        }
    }

    /** Videos in progress, most recent first, in [libraryId] or every library. */
    suspend fun continueWatching(libraryId: String? = null): Result<List<ContinueItem>> = withContext(Dispatchers.IO) {
        runCatching {
            val r = apiService.continueWatching(libraryId)
            r.body()?.takeIf { r.isSuccessful } ?: throw Exception("Server error: ${r.code()}")
        }
    }

    /** The videos that arrived last in [libraryId], or blended for the home screen; empty where showNewAdditions is off. */
    suspend fun newAdditions(libraryId: String?): Result<List<ContinueItem>> = withContext(Dispatchers.IO) {
        runCatching {
            val r = apiService.newAdditions(libraryId)
            r.body()?.takeIf { r.isSuccessful } ?: throw Exception("Server error: ${r.code()}")
        }
    }

    suspend fun removeFromContinue(originalPath: String): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching { apiService.removeFromContinue(encode(originalPath)); Unit }
    }

    /** Sends progress recorded offline; the server keeps whichever report is newer. */
    suspend fun syncPlayback(items: List<SyncedPlayback>): Result<Unit> = withContext(Dispatchers.IO) {
        runCatching {
            val r = apiService.syncPlayback(PlaybackSync(items))
            if (!r.isSuccessful) throw Exception("Server error: ${r.code()}")
        }
    }

    /** A continue-watching entry as a playable [FileItem]. */
    fun fileItemFor(item: ContinueItem): FileItem = FileItem(
        name = item.name,
        path = getStreamUrl(item.path),
        isDirectory = false,
        size = item.size,
        thumbnailUrl = absoluteUrl(item.thumbnailUrl),
        duration = item.duration,
        originalPath = item.path,
        id = item.id,
        playback = item.playback
    )

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
                                    val baseUrl = if (it.startsWith("http")) it else "$requestBase$it"
                                    if (cacheVersion > 0) "$baseUrl?v=$cacheVersion" else baseUrl
                                },
                                duration = mediaItem.duration,
                                width = mediaItem.width,
                                height = mediaItem.height,
                                originalPath = mediaItem.path,
                                id = mediaItem.id,
                                playback = mediaItem.playback
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
            } else if (response.code() == 429) {
                // Too many wrong passwords: the server says how long to wait (Retry-After, or
                // retry_after in the body, in seconds)
                val seconds = response.headers()["Retry-After"]?.toIntOrNull()
                    ?: runCatching { org.json.JSONObject(response.errorBody()?.string() ?: "").optInt("retry_after") }.getOrNull()
                    ?: 0
                Result.failure(Exception("Too many failed attempts. Try again ${retryWait(seconds)}."))
            } else Result.failure(Exception("Login failed"))
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override fun getStreamUrl(path: String): String {
        val encodedPath = Base64.encodeToString(path.toByteArray(), Base64.NO_WRAP)
        return "$requestBase/api/media/stream?path=$encodedPath&auto-transcode=0"
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
                    else frame.copy(url = "$requestBase${if (frame.url.startsWith("/")) "" else "/"}${frame.url}")
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

/** When a locked-out login can be tried again, as the web shows it: "in 4m 10s", "in 30s", or "later" if unknown. */
internal fun retryWait(seconds: Int): String = when {
    seconds <= 0 -> "later"
    seconds >= 60 -> "in ${seconds / 60}m ${seconds % 60}s"
    else -> "in ${seconds}s"
}

/**
 * A server address as requests are made to it: lower-case scheme and host, no default port and no
 * trailing slash ("Https://Host:443/" becomes "https://host"). OkHttp rewrites request URLs this
 * way, and Coil only fetches lower-case http(s), so URLs built from the address as typed (say, for
 * a reverse proxy on 443) can fail to load or to find their sign-in.
 */
fun normalizedServerUrl(address: String): String =
    address.trim().toHttpUrlOrNull()?.toString()?.trimEnd('/') ?: address

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
    override suspend fun checkAddress(address: String): AddressCheck = withContext(Dispatchers.IO) {
        val typed = address.trim().trimEnd('/')
        if (typed.contains("://")) return@withContext AddressCheck(typed, null)
        val hostPort = if (hasPort(typed)) typed else "$typed:$DEFAULT_PORT"
        val client = ApiClient.probeClient()
        val scheme = listOf("https", "http").firstOrNull { scheme ->
            runCatching {
                client.newCall(Request.Builder().url("$scheme://$hostPort/").head().build()).execute().close()
            }.isSuccess
        }
        AddressCheck("${scheme ?: "https"}://$hostPort", scheme != null)
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
