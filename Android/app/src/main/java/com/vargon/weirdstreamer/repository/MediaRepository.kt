package com.vargon.weirdstreamer.repository

import java.io.InputStream

/**
 * Represents a specific instance of a media server (e.g., a specific SMB host or API URL).
 */
data class MediaSource(
    val typeId: String,
    val serverAddress: String,
    val serverDisplayName: String = serverAddress
) {
    fun displayName(): String {
        return if (typeId == "SERVER") {
            try {
                val url = java.net.URL(serverAddress)
                url.host + (if (url.port != -1 && url.port != 80 && url.port != 443) ":${url.port}" else "")
            } catch (e: Exception) {
                serverAddress
            }
        } else {
            serverAddress.removePrefix("smb://").removeSuffix("/")
        }
    }
}

/**
 * Common file item interface for all sources.
 */
data class FileItem(
    val name: String,
    val path: String,
    val isDirectory: Boolean,
    val size: Long = 0,
    val mimeType: String? = null,
    val thumbnailUrl: String? = null,
    val duration: Double? = null,
    val width: Int? = null,
    val height: Int? = null,
    val originalPath: String? = null // For SSE matching or original reference
)

/**
 * Repository interface for media browsing.
 */
interface MediaRepository {
    suspend fun browse(path: String): Result<List<FileItem>>
    suspend fun authenticate(username: String?, password: String?): Result<Boolean>
    fun getStreamUrl(path: String): String
    fun getThumbnailUrl(path: String): String?
    fun getDisplayName(path: String): String
    suspend fun getInputStream(path: String): Result<InputStream>
    suspend fun inheritState(other: MediaRepository?)
    
    // Support for server-specific features
    fun supportsThumbnailRegeneration(): Boolean = false
    suspend fun regenerateThumbnails(path: String): Result<Boolean> = Result.failure(UnsupportedOperationException())
    
    // Auth requirements
    fun isAuthRequired(exception: Throwable): Boolean

    // Path logic abstraction
    fun getInitialPath(): String
    fun getParentPath(currentPath: String): String? // returns null if we should go back to server selection
}

/**
 * Abstraction for a type of media source (SMB, WS Server, etc.)
 */
interface SourceProvider {
    val typeId: String
    val displayName: String
    val addressLabel: String
    val addressPlaceholder: String
    
    fun getServers(): List<String>
    fun addServer(address: String)
    fun removeServer(address: String)
    fun createRepository(serverAddress: String): MediaRepository
    
    // Optional discovery
    fun startDiscovery(onServersFound: (List<String>) -> Unit) {}
}
