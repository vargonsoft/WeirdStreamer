package com.vargon.weirdstreamer

import android.util.Log
import android.webkit.MimeTypeMap
import coil.ImageLoader
import coil.decode.DataSource
import coil.decode.ImageSource
import coil.fetch.FetchResult
import coil.fetch.Fetcher
import coil.fetch.SourceResult
import coil.request.Options
import jcifs.CIFSContext
import jcifs.config.PropertyConfiguration
import jcifs.context.BaseContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okio.buffer
import okio.source
import java.util.Properties

class SmbFetcher(
    private val data: Any,
    private val options: Options,
    private val credentialManager: CredentialManager,
    private val baseContext: CIFSContext
) : Fetcher {

    override suspend fun fetch(): FetchResult? = withContext(Dispatchers.IO) {
        val path = data.toString()
        val host = try {
            path.removePrefix("smb://").substringBefore("/")
        } catch (e: Exception) {
            return@withContext null
        }
        
        val creds = credentialManager.getCredentials(host)
        val auth = if (creds.first != null && creds.second != null) {
            NtlmPasswordAuthenticator(null, creds.first, creds.second)
        } else {
            NtlmPasswordAuthenticator(null, null, null)
        }
        
        val context = baseContext.withCredentials(auth)
        
        return@withContext try {
            val smbFile = SmbFile(path, context)
            val extension = path.substringAfterLast('.', "").lowercase()
            val mimeType = MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension)
            
            SourceResult(
                source = ImageSource(
                    source = smbFile.inputStream.source().buffer(),
                    context = options.context
                ),
                mimeType = mimeType,
                dataSource = DataSource.NETWORK
            )
        } catch (e: Exception) {
            Log.e("SmbFetcher", "Error fetching: $path", e)
            null
        }
    }

    class Factory(private val credentialManager: CredentialManager) : Fetcher.Factory<Any> {
        
        private val baseContext: CIFSContext by lazy {
            val props = Properties().apply {
                setProperty("jcifs.smb.client.minVersion", "SMB202")
                setProperty("jcifs.smb.client.maxVersion", "SMB311")
                setProperty("jcifs.smb.client.ipcSigningEnforced", "false")
                setProperty("jcifs.smb.client.signingPreferred", "false")
                setProperty("jcifs.smb.client.signingEnforced", "false")
                setProperty("jcifs.smb.client.connTimeout", "3000")
                setProperty("jcifs.smb.client.responseTimeout", "5000")
            }
            BaseContext(PropertyConfiguration(props))
        }

        override fun create(data: Any, options: Options, imageLoader: ImageLoader): Fetcher? {
            val path = data.toString()
            if (!path.startsWith("smb://")) return null
            
            val lower = path.substringBefore("?").lowercase()
            
            // Expanded list of extensions including user requested types
            val isMedia = lower.endsWith(".jpg") || lower.endsWith(".jpeg") || 
                          lower.endsWith(".png") || lower.endsWith(".webp") ||
                          lower.endsWith(".gif") || lower.endsWith(".wmv") ||
                          lower.endsWith(".mkv") || lower.endsWith(".avi") ||
                          lower.endsWith(".mpg") || lower.endsWith(".mpeg") ||
                          lower.endsWith(".mp4") || lower.endsWith(".mov") ||
                          lower.endsWith(".flv") || lower.endsWith(".webm") ||
                          lower.endsWith(".heic") || lower.endsWith(".heif") ||
                          lower.endsWith(".m4v") || lower.endsWith(".vob") ||
                          lower.endsWith(".bmp") || lower.endsWith(".pdf")
            
            if (!isMedia) return null
            
            return SmbFetcher(data, options, credentialManager, baseContext)
        }
    }
}
