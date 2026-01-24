package com.vargon.weirdstreamer

import android.graphics.Bitmap
import android.graphics.drawable.BitmapDrawable
import android.media.MediaDataSource
import android.media.MediaMetadataRetriever
import android.net.Uri
import android.util.Log
import androidx.media3.common.MediaItem
import androidx.media3.common.util.UnstableApi
import androidx.media3.inspector.FrameExtractor
import coil.ImageLoader
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
import okio.buffer
import okio.sink
import java.io.File
import java.io.FileInputStream
import java.io.IOException
import java.util.concurrent.CancellationException

/**
 * Advanced multi-stage video thumbnail decoder:
 * 1. Buffered SMB Random Access (for performance)
 * 2. Media3 FrameExtractor (for stubborn formats like FLV/AVI/WMV)
 * 3. System MediaMetadataRetriever (as final legacy fallback)
 */
@UnstableApi
class VideoThumbnailDecoder(
    private val source: ImageSource,
    private val options: Options
) : Decoder {

    companion object {
        private val MEDIA3_PREFERRED_EXTENSIONS = setOf("flv", "avi", "wmv", "asf", "m4v")
    }

    override suspend fun decode(): DecodeResult? {
        val smbFile = (source.metadata as? SmbMetadata)?.smbFile
        val path = smbFile?.path?.lowercase() ?: ""
        val extension = path.substringAfterLast(".", "")
        val targetTimeMicros = 15_000_000L
        
        var bitmap: Bitmap? = null

        try {
            // Strategy 1: Buffered SMB Random Access (Best for MP4, MKV)
            // We try this first because it's the fastest and most network-efficient.
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
                        try { retriever.release() } catch (e: Exception) {}
                        try { dataSource?.close() } catch (e: Exception) {}
                    }
                }
            }

            // Strategy 2: Media3 FrameExtractor via Temp File (For stubborn formats)
            if (bitmap == null) {
                val tempFile = File.createTempFile("thumb_work_", ".tmp", options.context.cacheDir)
                try {
                    runInterruptible(Dispatchers.IO) {
                        source.source().use { input ->
                            tempFile.outputStream().sink().buffer().use { output ->
                                output.writeAll(input)
                            }
                        }
                    }

                    if (tempFile.length() > 0) {
                        // Try Media3 first for preferred extensions, otherwise try system retriever
                        if (MEDIA3_PREFERRED_EXTENSIONS.contains(extension)) {
                            bitmap = tryExtractWithMedia3(tempFile, targetTimeMicros / 1000)
                        }
                        
                        // Fallback to system retriever for the temp file
                        if (bitmap == null) {
                            bitmap = runInterruptible(Dispatchers.IO) {
                                val retriever = MediaMetadataRetriever()
                                var fis: FileInputStream? = null
                                try {
                                    fis = FileInputStream(tempFile)
                                    retriever.setDataSource(fis.fd)
                                    tryGetFrame(retriever, targetTimeMicros)
                                } catch (e: Exception) {
                                    null
                                } finally {
                                    try { fis?.close() } catch (e: Exception) {}
                                    try { retriever.release() } catch (e: Exception) {}
                                }
                            }
                        }
                    }
                } finally {
                    if (tempFile.exists()) tempFile.delete()
                }
            }
        } catch (e: Exception) {
            if (e is CancellationException || e is InterruptedException) return null
            Log.e("VideoThumbnailDecoder", "Extraction error", e)
        }

        return bitmap?.let {
            DecodeResult(
                drawable = BitmapDrawable(options.context.resources, it),
                isSampled = false
            )
        }
    }

    private suspend fun tryExtractWithMedia3(file: File, targetMillis: Long): Bitmap? {
        return try {
            val mediaItem = MediaItem.fromUri(Uri.fromFile(file))
            FrameExtractor.Builder(options.context, mediaItem).build().use { extractor ->
                extractor.getFrame(targetMillis).await().bitmap
                    ?: extractor.getThumbnail().await().bitmap
            }
        } catch (e: Exception) {
            Log.w("VideoThumbnailDecoder", "Media3 FrameExtractor failed: ${e.message}")
            null
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
        private val buffer = ByteArray(16384)
        private var bufferPos = -1L
        private var bufferSize = 0

        override fun readAt(position: Long, data: ByteArray, offset: Int, size: Int): Int {
            if (closed || raf == null) return -1
            if (position >= bufferPos && position + size <= bufferPos + bufferSize && bufferPos != -1L) {
                System.arraycopy(buffer, (position - bufferPos).toInt(), data, offset, size)
                return size
            }
            return try {
                val raf = raf!!
                if (size >= buffer.size) {
                    raf.seek(position)
                    raf.read(data, offset, size)
                } else {
                    raf.seek(position)
                    bufferPos = position
                    bufferSize = raf.read(buffer, 0, buffer.size)
                    if (bufferSize <= 0) {
                        bufferPos = -1
                        return -1
                    }
                    val bytesToCopy = Math.min(size, bufferSize)
                    System.arraycopy(buffer, 0, data, offset, bytesToCopy)
                    bytesToCopy
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
