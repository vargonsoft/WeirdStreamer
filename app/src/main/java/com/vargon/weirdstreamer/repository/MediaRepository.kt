package com.vargon.weirdstreamer.repository

// Sealed class to represent different media sources
sealed class MediaSource {
    data class Smb(
        val host: String,
        val share: String,
        val path: String = ""
    ) : MediaSource() {
        fun fullPath(): String = "smb://$host/$share/$path"
    }
    
    data class Server(
        val serverUrl: String,
        val path: String = ""
    ) : MediaSource() {
        fun displayName(): String {
            return try {
                val url = java.net.URL(serverUrl)
                url.host + (if (url.port != -1 && url.port != 80 && url.port != 443) ":${url.port}" else "")
            } catch (e: Exception) {
                serverUrl
            }
        }
    }
}

// Common file item interface for both sources
data class FileItem(
    val name: String,
    val path: String,
    val isDirectory: Boolean,
    val size: Long = 0,
    val mimeType: String? = null,
    val thumbnailUrl: String? = null,
    val duration: Double? = null,
    val width: Int? = null,
    val height: Int? = null
)

// Repository interface for media browsing
interface MediaRepository {
    suspend fun browse(path: String): Result<List<FileItem>>
    suspend fun authenticate(username: String?, password: String?): Result<Boolean>
    fun getStreamUrl(path: String): String
    fun getThumbnailUrl(path: String): String?
    fun getDisplayName(path: String): String
    suspend fun getInputStream(path: String): Result<java.io.InputStream>
}
