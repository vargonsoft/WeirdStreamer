package com.vargon.weirdstreamer.repository

import com.vargon.weirdstreamer.CredentialManager
import jcifs.CIFSContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbAuthException
import jcifs.smb.SmbFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class SmbRepository(
    private val baseContext: CIFSContext,
    private val credentialManager: CredentialManager,
    private val showHiddenFiles: Boolean = false
) : MediaRepository {
    
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
        // SMB authentication is handled per-request, so we just save credentials
        Result.success(true)
    }
    
    override fun getStreamUrl(path: String): String {
        // SMB paths are used directly
        return path
    }
    
    override fun getThumbnailUrl(path: String): String? {
        // SMB thumbnails are generated locally
        return null
    }
    
    private fun guessMimeType(filename: String): String {
        val ext = filename.substringAfterLast('.', "").lowercase()
        return when (ext) {
            "mp4", "mkv", "avi", "mov", "webm", "flv" -> "video/$ext"
            "jpg", "jpeg" -> "image/jpeg"
            "png" -> "image/png"
            "gif" -> "image/gif"
            "webp" -> "image/webp"
            "pdf" -> "application/pdf"
            else -> "application/octet-stream"
        }
    }
}
