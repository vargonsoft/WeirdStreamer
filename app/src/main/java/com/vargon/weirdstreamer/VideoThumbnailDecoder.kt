package com.vargon.weirdstreamer

import android.graphics.Bitmap
import android.graphics.drawable.BitmapDrawable
import android.media.MediaDataSource
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMetadataRetriever
import android.net.Uri
import android.util.Log
import android.webkit.MimeTypeMap
import androidx.media3.common.MediaItem
import androidx.media3.common.util.UnstableApi
import androidx.media3.inspector.FrameExtractor
import coil.ImageLoader
import coil.annotation.ExperimentalCoilApi
import coil.decode.DecodeResult
import coil.decode.Decoder
import coil.decode.ImageSource
import coil.fetch.SourceResult
import coil.request.Options
import jcifs.smb.SmbFile
import jcifs.smb.SmbRandomAccessFile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.guava.await
import kotlinx.coroutines.runInterruptible
import kotlinx.coroutines.withContext
import okio.buffer
import okio.sink
import java.io.File
import java.io.FileInputStream
import java.io.IOException
import java.io.InterruptedIOException
import java.util.concurrent.CancellationException
import kotlin.math.min

/**
 * Advanced multi-stage video thumbnail decoder:
 * 1. Buffered SMB Random Access (for performance)
 * 2. System MediaMetadataRetriever via Persistent Temp File (to avoid ENOENT race)
 * 3. Media3 FrameExtractor (for stubborn formats like FLV/AVI/WMV)
 */
@UnstableApi
@OptIn(ExperimentalCoilApi::class)
class VideoThumbnailDecoder(
    private val source: ImageSource,
    private val options: Options
) : Decoder {

    companion object {
        // Extensions where Media3 is generally more robust than the system retriever.
        private val MEDIA3_PREFERRED_EXTENSIONS = setOf("flv", "avi", "wmv", "asf")
        private const val TAG = "VideoThumbnailDecoder"
    }

    override suspend fun decode(): DecodeResult? {
        val smbFile = (source.metadata as? SmbMetadata)?.smbFile
        val path = smbFile?.path ?: ""
        val extension = path.substringAfterLast(".", "").lowercase()
        val targetTimeMicros = 10_000_000L
        
        var bitmap: Bitmap? = null

        try {
            // Strategy 1: Buffered SMB Random Access (Fastest, avoids full download)
            if (smbFile != null && !MEDIA3_PREFERRED_EXTENSIONS.contains(extension)) {
                bitmap = runInterruptible(Dispatchers.IO) {
                    val retriever = MediaMetadataRetriever()
                    var dataSource: SmbMediaDataSource? = null
                    try {
                        dataSource = SmbMediaDataSource(smbFile)
                        retriever.setDataSource(dataSource)
                        tryGetFrame(retriever, targetTimeMicros)
                    } catch (e: Exception) {
                        null
                    } finally {
                        try { retriever.release() } catch (_: Exception) {}
                        try { dataSource?.close() } catch (_: Exception) {}
                    }
                }
            }

            // Strategy 2: Persistent Temp File (to solve ENOENT issues with Media3/System)
            if (bitmap == null) {
                val fileHash = path.hashCode().toString()
                val tempFile = File(options.context.cacheDir, "v_thumb_$fileHash.tmp")
                
                if (!tempFile.exists() || tempFile.length() == 0L) {
                    runInterruptible(Dispatchers.IO) {
                        try {
                            source.source().use { input ->
                                tempFile.outputStream().sink().buffer().use { output ->
                                    output.writeAll(input)
                                }
                            }
                        } catch (e: Exception) {
                            if (tempFile.exists()) tempFile.delete()
                            throw e
                        }
                    }
                }

                if (tempFile.exists() && tempFile.length() > 0) {
                    // A. Try system retriever on local file (very stable)
                    var hasVideo = false
                    var retrieverSuccess = false
                    bitmap = runInterruptible(Dispatchers.IO) {
                        val retriever = MediaMetadataRetriever()
                        var fis: FileInputStream? = null
                        try {
                            fis = FileInputStream(tempFile)
                            retriever.setDataSource(fis.fd)
                            retrieverSuccess = true
                            hasVideo = retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_HAS_VIDEO) == "yes"
                            tryGetFrame(retriever, targetTimeMicros)
                        } catch (e: Exception) {
                            null
                        } finally {
                            try { fis?.close() } catch (_: Exception) {}
                            try { retriever.release() } catch (_: Exception) {}
                        }
                    }

                    // B. Try Media3 as last resort
                    if (bitmap == null && (MEDIA3_PREFERRED_EXTENSIONS.contains(extension))) {
                        // Crucial: Only use Media3 if the file is likely valid to avoid internal NPEs
                        if (isLikelyDecodableByMedia3(tempFile)) {
                            bitmap = tryExtractWithMedia3(tempFile, targetTimeMicros / 1000)
                        }
                    }
                }
            }
        } catch (e: Exception) {
            if (e is CancellationException || e is InterruptedException || e is InterruptedIOException) return null
            Log.e(TAG, "Extraction error for $path", e)
        }

        return bitmap?.let {
            DecodeResult(
                drawable = BitmapDrawable(options.context.resources, it),
                isSampled = false
            )
        }
    }

    /**
     * Pre-validates the file using the lighter MediaExtractor to avoid triggering 
     * internal Media3 bugs that cause NPEs on the main thread.
     */
    private fun isLikelyDecodableByMedia3(file: File): Boolean {
        val extractor = MediaExtractor()
        try {
            extractor.setDataSource(file.absolutePath)
            for (i in 0 until extractor.trackCount) {
                val format = extractor.getTrackFormat(i)
                val mime = format.getString(MediaFormat.KEY_MIME) ?: ""
                if (mime.startsWith("video/")) {
                    // Avoid Sorenson Spark (often in FLV) which Media3's FrameExtractor chokes on
                    if (mime == "video/x-vnd.on2.vp6" || mime.contains("sorenson") || mime.contains("spark")) {
                        return false
                    }
                    return true
                }
            }
        } catch (_: Exception) {
        } finally {
            try { extractor.release() } catch (_: Exception) {}
        }
        return false
    }

    private suspend fun tryExtractWithMedia3(file: File, targetMillis: Long): Bitmap? {
        return withContext(Dispatchers.Main) {
            var extractor: FrameExtractor? = null
            try {
                if (!file.exists()) return@withContext null
                
                val extension = file.extension.lowercase()
                val mimeType = when (extension) {
                    "wmv" -> "video/x-ms-wmv"
                    "asf" -> "video/x-ms-asf"
                    "avi" -> "video/x-msvideo"
                    "flv" -> "video/x-flv"
                    else -> MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension)
                }
                
                val mediaItem = MediaItem.Builder()
                    .setUri(Uri.fromFile(file))
                    .apply { if (mimeType != null) setMimeType(mimeType) }
                    .build()
                
                extractor = FrameExtractor.Builder(options.context.applicationContext, mediaItem).build()
                
                try {
                    extractor.getFrame(targetMillis).await().bitmap
                } catch (e: Exception) {
                    try {
                        extractor.getThumbnail().await().bitmap
                    } catch (inner: Exception) {
                        null
                    }
                }
            } catch (e: Throwable) {
                if (e !is CancellationException) {
                    Log.w(TAG, "Media3 FrameExtractor failed for ${file.name}: ${e.message}")
                }
                null
            } finally {
                val finalExtractor = extractor
                if (finalExtractor != null) {
                    // Close immediately on the main thread to stop any pending callbacks
                    try {
                        finalExtractor.close()
                    } catch (_: Exception) {}
                }
            }
        }
    }

    private fun tryGetFrame(retriever: MediaMetadataRetriever, targetMicros: Long): Bitmap? {
        return try {
            retriever.getFrameAtTime(targetMicros, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                ?: retriever.getFrameAtTime(1_000_000L, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                ?: retriever.getFrameAtTime(0L, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
        } catch (e: Exception) {
            null
        }
    }

    private class SmbMediaDataSource(smbFile: SmbFile) : MediaDataSource() {
        private var raf: SmbRandomAccessFile? = try { smbFile.openRandomAccess("r") } catch (e: IOException) { null }
        private var closed = false
        private val buffer = ByteArray(64 * 1024) 
        private var bufferPos = -1L
        private var bufferSize = 0

        override fun readAt(position: Long, data: ByteArray, offset: Int, size: Int): Int {
            if (closed || raf == null) return -1
            if (size <= 0) return 0

            if (bufferPos != -1L && position >= bufferPos && position < bufferPos + bufferSize) {
                val available = (bufferPos + bufferSize - position).toInt()
                val toCopy = min(size, available)
                System.arraycopy(buffer, (position - bufferPos).toInt(), data, offset, toCopy)
                return toCopy
            }

            return try {
                val currentRaf = raf!!
                if (size >= buffer.size) {
                    currentRaf.seek(position)
                    currentRaf.read(data, offset, size)
                } else {
                    currentRaf.seek(position)
                    bufferPos = position
                    bufferSize = currentRaf.read(buffer, 0, buffer.size)
                    if (bufferSize <= 0) {
                        bufferPos = -1
                        return -1
                    }
                    val toCopy = min(size, bufferSize)
                    System.arraycopy(buffer, 0, data, offset, toCopy)
                    toCopy
                }
            } catch (e: IOException) { -1 }
        }

        override fun getSize(): Long = try { raf?.length() ?: -1L } catch (e: IOException) { -1L }
        override fun close() {
            closed = true
            try { raf?.close() } catch (e: IOException) {}
            raf = null
        }
    }

    class Factory : Decoder.Factory {
        override fun create(result: SourceResult, options: Options, imageLoader: ImageLoader): Decoder? {
            val mimeType = result.mimeType
            if (mimeType != null && mimeType.startsWith("video/")) {
                return VideoThumbnailDecoder(result.source, options)
            }
            return null
        }
    }
}
