package com.vargon.weirdstreamer

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.drawable.BitmapDrawable
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
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.guava.await
import kotlinx.coroutines.runInterruptible
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import okio.buffer
import okio.sink
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.io.InterruptedIOException
import java.util.Collections
import java.util.concurrent.CancellationException

/**
 * Advanced multi-stage video thumbnail decoder with session-aware disk caching:
 * 1. Disk Cache: Fast initial display and offline fallback.
 * 2. SMB Refresh: Fresh extraction on new app launches (session-based).
 * 3. Multi-Engine: System MediaMetadataRetriever (Local/SMB) or Media3 FrameExtractor.
 */
@UnstableApi
@OptIn(ExperimentalCoilApi::class)
class VideoThumbnailDecoder(
    private val source: ImageSource,
    private val options: Options
) : Decoder {

    companion object {
        private val MEDIA3_PREFERRED_EXTENSIONS = setOf("flv", "avi", "wmv", "asf")
        private const val TAG = "VideoThumbnailDecoder"
        
        // Tracks paths refreshed in the current app session to avoid redundant network calls.
        private val sessionRefreshedPaths = Collections.synchronizedSet(mutableSetOf<String>())
        
        fun clearSessionCache() {
            sessionRefreshedPaths.clear()
        }

        // Stable key for disk cache that survives credential changes
        fun getStableCacheKey(path: String): String {
            val normalized = path.replaceFirst(Regex("smb://.*@"), "smb://").removeSuffix("/")
            return normalized.hashCode().toString()
        }
    }

    override suspend fun decode(): DecodeResult? {
        val smbFile = (source.metadata as? SmbMetadata)?.smbFile
        val path = smbFile?.path ?: source.fileOrNull()?.toFile()?.absolutePath ?: ""
        if (path.isEmpty()) return null
        
        val extension = path.substringAfterLast(".", "").lowercase()
        val targetTimeMicros = 10_000_000L
        
        val cacheKey = getStableCacheKey(path)
        val thumbCacheDir = File(options.context.cacheDir, "thumbnails")
        val thumbCacheFile = File(thumbCacheDir, "$cacheKey.webp")
        
        // 1. Check disk cache first.
        if (thumbCacheFile.exists()) {
            val bitmap = loadFromDisk(thumbCacheFile)
            if (bitmap != null) return toResult(bitmap)
        }

        var bitmap: Bitmap? = null
        val isRefreshed = sessionRefreshedPaths.contains(path)

        // 2. Fresh Extraction: If not refreshed this session (and not found in cache), extract from SMB
        if (!isRefreshed) {
            try {
                bitmap = withTimeoutOrNull(20000L) {
                    tryFreshExtraction(smbFile, path, extension, targetTimeMicros, cacheKey)
                }
                
                if (bitmap != null) {
                    saveToDisk(thumbCacheFile, bitmap)
                    sessionRefreshedPaths.add(path)
                }
            } catch (e: Exception) {
                if (e is CancellationException || e is InterruptedException || e is InterruptedIOException) return null
                Log.w(TAG, "Fresh extraction failed/timed out for $path")
            }
        }

        return bitmap?.let { toResult(it) }
    }

    private suspend fun tryFreshExtraction(
        smbFile: jcifs.smb.SmbFile?,
        path: String,
        extension: String,
        targetTimeMicros: Long,
        cacheKey: String
    ): Bitmap? {
        var bitmap: Bitmap? = null

        // Strategy A: Buffered SMB Random Access (Fastest for SMB)
        if (smbFile != null && !MEDIA3_PREFERRED_EXTENSIONS.contains(extension)) {
            bitmap = runInterruptible(Dispatchers.IO) {
                val retriever = MediaMetadataRetriever()
                var dataSource: SmbMediaDataSource? = null
                try {
                    dataSource = SmbMediaDataSource(smbFile)
                    retriever.setDataSource(dataSource)
                    tryGetFrame(retriever, targetTimeMicros)
                } catch (_: Exception) {
                    null
                } finally {
                    try { dataSource?.close() } catch (_: Exception) {}
                    try { retriever.close() } catch (_: Exception) {}
                }
            }
        }

        // Strategy B: Persistent Temp File (System or Media3)
        if (bitmap == null) {
            val tempFile = File(options.context.cacheDir, "v_thumb_$cacheKey.tmp")
            try {
                runInterruptible(Dispatchers.IO) {
                    source.source().use { input ->
                        tempFile.outputStream().sink().buffer().use { output ->
                            output.writeAll(input)
                        }
                    }
                }

                if (tempFile.exists() && tempFile.length() > 0) {
                    // B1: System Retriever
                    bitmap = runInterruptible(Dispatchers.IO) {
                        val retriever = MediaMetadataRetriever()
                        var fis: FileInputStream? = null
                        try {
                            fis = FileInputStream(tempFile)
                            retriever.setDataSource(fis.fd)
                            tryGetFrame(retriever, targetTimeMicros)
                        } catch (_: Exception) {
                            null
                        } finally {
                            try { fis?.close() } catch (_: Exception) {}
                            try { retriever.close() } catch (_: Exception) {}
                        }
                    }

                    // B2: Media3
                    if (bitmap == null && MEDIA3_PREFERRED_EXTENSIONS.contains(extension)) {
                        if (isLikelyDecodableByMedia3(tempFile)) {
                            bitmap = tryExtractWithMedia3(tempFile, targetTimeMicros / 1000)
                        }
                    }
                }
            } finally {
                if (tempFile.exists()) tempFile.delete()
            }
        }
        return bitmap
    }

    private fun toResult(bitmap: Bitmap): DecodeResult {
        return DecodeResult(
            drawable = BitmapDrawable(options.context.resources, bitmap),
            isSampled = false
        )
    }

    private suspend fun loadFromDisk(file: File): Bitmap? = withContext(Dispatchers.IO) {
        try {
            BitmapFactory.decodeFile(file.absolutePath)
        } catch (_: Exception) {
            null
        }
    }

    private suspend fun saveToDisk(file: File, bitmap: Bitmap) = withContext(Dispatchers.IO) {
        try {
            val parent = file.parentFile
            if (parent != null && !parent.exists()) parent.mkdirs()
            FileOutputStream(file).use { out ->
                bitmap.compress(Bitmap.CompressFormat.WEBP, 80, out)
            }
        } catch (e: Exception) {
            Log.e(TAG, "Failed to save thumbnail to disk: $file", e)
        }
    }

    private fun isLikelyDecodableByMedia3(file: File): Boolean {
        val extractor = MediaExtractor()
        try {
            extractor.setDataSource(file.absolutePath)
            for (i in 0 until extractor.trackCount) {
                val format = extractor.getTrackFormat(i)
                val mime = format.getString(MediaFormat.KEY_MIME) ?: ""
                if (mime.startsWith("video/")) {
                    return !(mime == "video/x-vnd.on2.vp6" || mime.contains("sorenson") || mime.contains("spark"))
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
                
                val ext = file.extension.lowercase()
                val mimeType = when (ext) {
                    "wmv" -> "video/x-ms-wmv"
                    "asf" -> "video/x-ms-asf"
                    "avi" -> "video/x-msvideo"
                    "flv" -> "video/x-flv"
                    else -> MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext)
                }
                
                val mediaItem = MediaItem.Builder()
                    .setUri(Uri.fromFile(file))
                    .apply { if (mimeType != null) setMimeType(mimeType) }
                    .build()
                
                extractor = FrameExtractor.Builder(options.context.applicationContext, mediaItem).build()
                
                try {
                    extractor.getFrame(targetMillis).await().bitmap
                } catch (_: Exception) {
                    try {
                        extractor.getThumbnail().await().bitmap
                    } catch (_: Exception) {
                        null
                    }
                }
            } catch (e: Throwable) {
                if (e !is CancellationException) {
                    Log.w(TAG, "Media3 FrameExtractor failed for ${file.name}: ${e.message}")
                }
                null
            } finally {
                extractor?.let {
                    try { it.close() } catch (_: Exception) {}
                }
            }
        }
    }

    private fun tryGetFrame(retriever: MediaMetadataRetriever, targetMicros: Long): Bitmap? {
        return try {
            retriever.getFrameAtTime(targetMicros, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                ?: retriever.getFrameAtTime(1_000_000L, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                ?: retriever.getFrameAtTime(0L, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
        } catch (_: Exception) {
            null
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
