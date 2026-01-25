package com.vargon.weirdstreamer

import android.util.Log
import android.webkit.MimeTypeMap
import coil.ImageLoader
import coil.annotation.ExperimentalCoilApi
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
import okio.ForwardingSource
import okio.Buffer
import java.io.File
import java.util.Properties
import kotlin.math.min
import kotlinx.coroutines.CancellationException
import java.io.InterruptedIOException

class SmbFetcher(
    private val data: Any,
    private val options: Options,
    private val credentialManager: CredentialManager,
    private val baseContext: CIFSContext
) : Fetcher {

    override suspend fun fetch(): FetchResult? = withContext(Dispatchers.IO) {
        val path = data.toString()
        val host = try { path.removePrefix("smb://").substringBefore("/") } catch (e: Exception) { return@withContext null }
        
        val creds = credentialManager.getCredentials(host)
        val auth = if (creds.first != null && creds.second != null) {
            NtlmPasswordAuthenticator(null, creds.first, creds.second)
        } else {
            NtlmPasswordAuthenticator(null, null, null)
        }
        
        val context = baseContext.withCredentials(auth)
        val smbFile = SmbFile(path, context)
        
        return@withContext try {
            val extension = path.substringAfterLast('.', "").lowercase()
            val mimeType = MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension) ?: when (extension) {
                "flv" -> "video/x-flv"
                "mkv" -> "video/x-matroska"
                "wmv" -> "video/x-ms-wmv"
                "3gp" -> "video/3gpp"
                "mpg", "mpeg" -> "video/mpeg"
                "m4v" -> "video/x-m4v"
                "vob" -> "video/dvd"
                "avi" -> "video/x-msvideo"
                "ts" -> "video/mp2t"
                "webm" -> "video/webm"
                "mov" -> "video/quicktime"
                "asf" -> "video/x-ms-asf"
                else -> null
            }

            val isVideo = mimeType?.startsWith("video/") == true
            val inputStream = smbFile.inputStream
            val rawSource = inputStream.source()
            
            // If it's a video, we wrap it in a limited source.
            // This prevents accidental full download if a decoder doesn't use random access.
            val processedSource = if (isVideo) {
                object : ForwardingSource(rawSource) {
                    private var bytesRead = 0L
                    private val limit = 20 * 1024 * 1024L // Increased to 20MB
                    override fun read(sink: Buffer, byteCount: Long): Long {
                        if (bytesRead >= limit) return -1
                        val toRead = min(byteCount, limit - bytesRead)
                        val read = super.read(sink, toRead)
                        if (read != -1L) bytesRead += read
                        return read
                    }
                }
            } else {
                rawSource
            }
            
            val imageSource = ImageSource(
                source = object : ForwardingSource(processedSource) {
                    private var closed = false
                    override fun close() {
                        if (closed) return
                        closed = true
                        try {
                            super.close()
                        } finally {
                            inputStream.close()
                            smbFile.close()
                            Log.d("SmbFetcher", "Closed SMB resources for: $path")
                        }
                    }
                }.buffer(),
                context = options.context,
                metadata = SmbMetadata(smbFile)
            )
            
            SourceResult(
                source = imageSource,
                mimeType = mimeType,
                dataSource = DataSource.NETWORK
            )
        } catch (e: Exception) {
            smbFile.close()
            if (e !is CancellationException && e !is InterruptedException && e !is InterruptedIOException) {
                Log.e("SmbFetcher", "Failed to fetch: $path", e)
            }
            null
        }
    }

    class Factory(private val credentialManager: CredentialManager) : Fetcher.Factory<Any> {
        private val baseContext: CIFSContext by lazy {
            val props = Properties().apply {
                setProperty("jcifs.smb.client.minVersion", "SMB202")
                setProperty("jcifs.smb.client.maxVersion", "SMB311")
                setProperty("jcifs.smb.client.ipcSigningEnforced", "false")
                setProperty("jcifs.smb.client.connTimeout", "5000")
                setProperty("jcifs.smb.client.responseTimeout", "10000")
            }
            BaseContext(PropertyConfiguration(props))
        }

        override fun create(data: Any, options: Options, imageLoader: ImageLoader): Fetcher? {
            val path = data.toString()
            if (!path.startsWith("smb://")) return null
            return SmbFetcher(data, options, credentialManager, baseContext)
        }
    }
}
