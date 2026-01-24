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
import okio.ForwardingSource
import java.io.File
import java.util.Properties

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
                "pdf" -> "application/pdf"
                else -> null
            }

            val isVideo = mimeType?.startsWith("video/") == true
            
            val imageSource = if (isVideo) {
                // Videos still need a small local proxy for reliable frame extraction
                val tempFile = File.createTempFile("smb_video_thumb_", ".$extension", options.context.cacheDir)
                try {
                    smbFile.inputStream.use { input ->
                        tempFile.outputStream().use { output ->
                            val buffer = ByteArray(8192)
                            var totalRead = 0L
                            val maxToRead = 2 * 1024 * 1024L // 2MB is enough
                            while (totalRead < maxToRead) {
                                val read = input.read(buffer)
                                if (read == -1) break
                                output.write(buffer, 0, read)
                                totalRead += read
                            }
                        }
                    }
                } finally {
                    smbFile.close() // Close network handle immediately for videos
                }
                
                ImageSource(
                    source = object : ForwardingSource(tempFile.source()) {
                        override fun close() {
                            try {
                                super.close()
                            } finally {
                                if (tempFile.exists()) tempFile.delete()
                                Log.i("SmbFetcher", "CLOSED video proxy for: $path")
                            }
                        }
                    }.buffer(),
                    context = options.context
                )
            } else {
                // Images stream directly
                val inputStream = smbFile.inputStream
                ImageSource(
                    source = object : ForwardingSource(inputStream.source()) {
                        private var closed = false
                        override fun close() {
                            if (closed) return
                            closed = true
                            try {
                                super.close()
                            } finally {
                                inputStream.close()
                                smbFile.close() // Close network handle for images
                                Log.i("SmbFetcher", "CLOSED image stream for: $path")
                            }
                        }
                    }.buffer(),
                    context = options.context
                )
            }
            
            SourceResult(
                source = imageSource,
                mimeType = mimeType,
                dataSource = DataSource.NETWORK
            )
        } catch (e: Exception) {
            smbFile.close()
            Log.e("SmbFetcher", "Failed to fetch: $path", e)
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
