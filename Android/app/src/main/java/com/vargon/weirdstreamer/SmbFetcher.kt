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
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import okio.Buffer
import okio.FileSystem
import okio.ForwardingSource
import okio.Path.Companion.toOkioPath
import okio.buffer
import okio.source
import java.io.File
import java.io.InterruptedIOException
import java.util.Properties
import kotlin.math.min

class SmbFetcher(
    private val data: Any,
    private val options: Options,
    private val credentialManager: CredentialManager,
    private val baseContext: CIFSContext,
    private val imageLoader: ImageLoader
) : Fetcher {

    companion object {
        private const val TAG = "SmbFetcher"

        fun getStableKey(path: String): String {
            return path.replaceFirst(Regex("smb://.*@"), "smb://").removeSuffix("/")
        }
    }

    @androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
    @OptIn(ExperimentalCoilApi::class)
    override suspend fun fetch(): FetchResult? = withContext(Dispatchers.IO) {
        val path = data.toString()
        val diskCacheKey = options.diskCacheKey ?: getStableKey(path)
        val diskCache = imageLoader.diskCache

        // 1. FAST PATH: Check Coil's DiskCache for images
        if (diskCache != null && options.diskCachePolicy.readEnabled) {
            val snapshot = diskCache[diskCacheKey]
            if (snapshot != null) {
                Log.d(TAG, "Disk cache HIT for $path (key: $diskCacheKey)")
                return@withContext SourceResult(
                    source = ImageSource(
                        file = snapshot.data,
                        diskCacheKey = diskCacheKey,
                        closeable = snapshot
                    ),
                    mimeType = getMimeType(path),
                    dataSource = DataSource.DISK
                )
            } else {
                Log.d(TAG, "Disk cache MISS for $path (key: $diskCacheKey)")
            }
        }

        // 2. Fetch from SMB
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
            if (!smbFile.exists() || !smbFile.isFile) {
                smbFile.close()
                return@withContext null
            }

            val mimeType = getMimeType(path)
            val isVideo = mimeType?.startsWith("video/") == true
            val inputStream = smbFile.inputStream

            // For non-video images, we'll stream it into the DiskCache manually
            if (!isVideo && diskCache != null && options.diskCachePolicy.writeEnabled) {
                val editor = diskCache.edit(diskCacheKey)
                if (editor != null) {
                    try {
                        FileSystem.SYSTEM.write(editor.data) {
                            inputStream.source().use { source ->
                                writeAll(source)
                            }
                        }
                        editor.commit()
                        Log.d(TAG, "Successfully cached $path (key: $diskCacheKey)")

                        // Verification check
                        val snapshot = diskCache[diskCacheKey]
                        if (snapshot != null) {
                            Log.d(TAG, "VERIFIED: Just saved $diskCacheKey and it is now retrievable!")
                            return@withContext SourceResult(
                                source = ImageSource(
                                    file = snapshot.data,
                                    diskCacheKey = diskCacheKey,
                                    closeable = snapshot
                                ),
                                mimeType = mimeType,
                                dataSource = DataSource.NETWORK
                            )
                        } else {
                            Log.e(TAG, "VERIFICATION FAILED: Just saved $diskCacheKey but it is NOT retrievable!")
                        }
                    } catch (e: Exception) {
                        Log.e(TAG, "Failed to write to disk cache for $path", e)
                        editor.abort()
                    } finally {
                        inputStream.close()
                        smbFile.close()
                    }
                }
            }

            // Fallback for videos or when cache write fails
            val rawSource = if (!isVideo) smbFile.inputStream.source().buffer() else inputStream.source().buffer()
            
            val processedSource = if (isVideo) {
                object : ForwardingSource(rawSource) {
                    private var bytesRead = 0L
                    private val limit = 20 * 1024 * 1024L // 20MB
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
                            if (isVideo) inputStream.close()
                            smbFile.close()
                            Log.d(TAG, "SMB Stream Closed: $path")
                        }
                    }
                }.buffer(),
                context = options.context,
                metadata = SmbMetadata(smbFile)
            )
            
            SourceResult(
                source = imageSource,
                mimeType = mimeType,
                dataSource = if (isVideo) DataSource.DISK else DataSource.NETWORK
            )
        } catch (e: Exception) {
            smbFile.close()
            if (e !is CancellationException && e !is InterruptedException && e !is InterruptedIOException) {
                Log.e(TAG, "Failed to fetch: $path", e)
            }
            null
        }
    }

    private fun getMimeType(path: String): String? {
        val extension = path.substringBefore('?').substringAfterLast('.', "").lowercase()
        return MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension) ?: when (extension) {
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
            "jpg", "jpeg" -> "image/jpeg"
            "png" -> "image/png"
            "webp" -> "image/webp"
            "heic" -> "image/heic"
            "heif" -> "image/heif"
            else -> null
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
                setProperty("jcifs.smb.client.disableRequestValidation", "true")
            }
            BaseContext(PropertyConfiguration(props))
        }

        override fun create(data: Any, options: Options, imageLoader: ImageLoader): Fetcher? {
            val path = data.toString()
            if (!path.startsWith("smb://")) return null
            return SmbFetcher(data, options, credentialManager, baseContext, imageLoader)
        }
    }
}
