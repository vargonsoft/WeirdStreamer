package com.vargon.weirdstreamer.repository

import com.vargon.weirdstreamer.CredentialManager
import jcifs.CIFSContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbAuthException
import jcifs.smb.SmbFile
import jcifs.smb.SmbUnsupportedOperationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.InputStream

class SmbRepository(
    private val baseContext: CIFSContext,
    private val credentialManager: CredentialManager,
    private val serverAddress: String,
) : MediaRepository {

    override suspend fun inheritState(other: MediaRepository?) {
        // No-op
    }

    override suspend fun browse(path: String): Result<List<FileItem>> = withContext(Dispatchers.IO) {
        try {
            val normalizedPath = if (path.endsWith("/") || path.contains("?")) path else "$path/"
            val host = normalizedPath.removePrefix("smb://").substringBefore("/")
            
            val creds = credentialManager.getCredentials(host)
            val auth = if (creds.first != null && creds.second != null) {
                NtlmPasswordAuthenticator(null, creds.first, creds.second)
            } else {
                NtlmPasswordAuthenticator(null, null, null)
            }
            
            val context = baseContext.withCredentials(auth)
            SmbFile(normalizedPath, context).use { smbFile ->
                val allFiles = smbFile.listFiles()
                // Settings → Show hidden files, as it is now
                val showHiddenFiles = credentialManager.getShowHiddenFiles()
                val items = allFiles.mapNotNull { file ->
                    val isHidden = file.isHidden || file.name.startsWith(".")
                    val item = if (showHiddenFiles || !isHidden) {
                        FileItem(
                            name = file.name.removeSuffix("/"),
                            path = file.canonicalPath,
                            isDirectory = file.isDirectory,
                            size = if (file.isDirectory) 0 else file.length(),
                            mimeType = guessMimeType(file.name)
                        )
                    } else null
                    file.close()
                    item
                }.sortedWith(compareBy({ !it.isDirectory }, { it.name.lowercase() }))
                
                Result.success(items)
            }
        } catch (e: SmbAuthException) {
            Result.failure(e)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }
    
    override suspend fun authenticate(username: String?, password: String?): Result<Boolean> = withContext(Dispatchers.IO) {
        Result.success(true)
    }
    
    override fun getStreamUrl(path: String): String = path
    override fun getThumbnailUrl(path: String): String? = null
    override fun getDisplayName(path: String): String = path.substringAfterLast('/').substringBefore('?')

    override suspend fun getInputStream(path: String): Result<InputStream> = withContext(Dispatchers.IO) {
        try {
            val host = path.removePrefix("smb://").substringBefore("/")
            val creds = credentialManager.getCredentials(host)
            val auth = if (creds.first != null && creds.second != null) {
                NtlmPasswordAuthenticator(null, creds.first, creds.second)
            } else {
                NtlmPasswordAuthenticator(null, null, null)
            }

            val context = baseContext.withCredentials(auth)
            val smbFile = SmbFile(path, context)
            Result.success(smbFile.inputStream)
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    override fun isAuthRequired(exception: Throwable): Boolean = exception is SmbAuthException

    override fun getInitialPath(): String = "smb://$serverAddress/"

    override fun getParentPath(currentPath: String): String? {
        if (currentPath == "smb://" || currentPath.isEmpty()) return null
        
        val pathWithoutTrailing = currentPath.removeSuffix("/")
        if (pathWithoutTrailing.count { it == '/' } <= 2) return null
        
        return pathWithoutTrailing.substringBeforeLast("/") + "/"
    }

    private fun guessMimeType(filename: String): String {
        val ext = filename.substringAfterLast('.', "").lowercase()
        return when (ext) {
            "mp4", "mkv", "avi", "mov", "webm", "flv" -> "video/$ext"
            "divx", "xvid" -> "video/x-msvideo"
            "jpg", "jpeg" -> "image/jpeg"
            "png" -> "image/png"
            "gif" -> "image/gif"
            "webp" -> "image/webp"
            "pdf" -> "application/pdf"
            else -> "application/octet-stream"
        }
    }
}

class SmbSourceProvider(
    private val baseContextLoader: () -> CIFSContext,
    private val credentialManager: CredentialManager
) : SourceProvider {
    override val typeId: String = "SMB"
    override val displayName: String = "SMB Share"
    override val addressLabel: String = "SMB Host"
    override val addressPlaceholder: String = "192.168.1.100"

    override fun getServers(): List<String> = credentialManager.getSavedServers().toList()
    override fun addServer(address: String) { credentialManager.addSavedServer(address) }
    override fun removeServer(address: String) { credentialManager.removeSavedServer(address) }
    override fun createRepository(serverAddress: String): MediaRepository = SmbRepository(baseContextLoader(), credentialManager, serverAddress)
}
