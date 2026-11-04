package com.vargon.weirdstreamer

import android.util.Log
import com.vargon.weirdstreamer.repository.FileItem
import java.io.File
import java.io.RandomAccessFile

enum class LivePhotoType {
    SAMSUNG, // the video inside the photo (Samsung and Google motion photos), at videoOffset
    APPLE    // a separate video file (an Apple Live Photo's .MOV)
}

data class LivePhotoInfo(
    val type: LivePhotoType,
    val videoOffset: Long,
    val videoLength: Long
)

object LivePhotoUtil {
    private const val TAG = "LivePhotoUtil"
    private val SAMSUNG_MARKER = "MotionPhoto_Data".toByteArray(Charsets.US_ASCII)
    private val XMP_OFFSET_MARKER = "MicroVideoOffset=\"".toByteArray(Charsets.US_ASCII)
    private val XMP_OFFSET_MARKER_ALT = "GCamera:MicroVideoOffset=\"".toByteArray(Charsets.US_ASCII)

    fun findLivePhotoInfo(file: File): LivePhotoInfo? {
        if (!file.exists()) return null
        Log.i(TAG, "Analyzing file for Live Photo: ${file.name} (${file.length()} bytes)")
        
        // 1. Check for Samsung marker formats (v1 and v2)
        val samsungInfo = findSamsungMotionPhoto(file)
        if (samsungInfo != null) return samsungInfo
        
        // 2. Check XMP metadata (common in Google Camera and newer Samsung)
        val xmpInfo = findXmpMicroVideo(file)
        if (xmpInfo != null) return xmpInfo
        
        Log.i(TAG, "No Live Photo markers found in ${file.name}")
        return null
    }

    private fun findSamsungMotionPhoto(file: File): LivePhotoInfo? {
        try {
            RandomAccessFile(file, "r").use { raf ->
                val size = raf.length()
                if (size < 100) return null
                
                // Search the file for "MotionPhoto_Data"
                // Search up to 15MB from the end, which covers almost all cases
                val searchRange = size.coerceAtMost(15 * 1024 * 1024)
                val startSearch = size - searchRange
                raf.seek(startSearch)
                val buffer = ByteArray(searchRange.toInt())
                raf.readFully(buffer)
                
                Log.i(TAG, "Searching for 'MotionPhoto_Data' in last ${buffer.size} bytes...")
                
                var posInSearch = findLastInByteArray(buffer, SAMSUNG_MARKER)
                while (posInSearch != -1) {
                    val absPos = startSearch + posInSearch
                    Log.i(TAG, "Found Samsung marker candidate at position: $absPos")
                    
                    // --- Case 1: Samsung v1 (Marker + Length + Video) ---
                    // The video starts some bytes after the marker. Usually there's a 4-byte length.
                    // We look for 'ftyp' which starts an MP4 file (usually at offset 4 or 8)
                    val afterStart = posInSearch + SAMSUNG_MARKER.size
                    val afterEnd = (afterStart + 128).coerceAtMost(buffer.size)
                    for (i in afterStart until afterEnd - 4) {
                        if (buffer[i] == 'f'.toByte() && buffer[i+1] == 't'.toByte() && 
                            buffer[i+2] == 'y'.toByte() && buffer[i+3] == 'p'.toByte()) {
                            
                            val videoOffset = (i - 4).toLong() + startSearch
                            val videoLength = size - videoOffset
                            
                            // Check if the 4 bytes before ftyp (the size field of ftyp box) look reasonable
                            if (videoLength > 0) {
                                Log.i(TAG, "Detected Samsung v1 via ftyp: offset=$videoOffset, length=$videoLength")
                                return LivePhotoInfo(LivePhotoType.SAMSUNG, videoOffset, videoLength)
                            }
                        }
                    }
                    
                    // --- Case 2: Samsung v2 (Video + Marker + Length) ---
                    // The marker is in a trailer AFTER the video. The length is usually 4 bytes after marker.
                    if (afterStart + 4 <= buffer.size) {
                        raf.seek(absPos + SAMSUNG_MARKER.size)
                        val len1 = raf.readInt()
                        val len2 = Integer.reverseBytes(len1)
                        for (len in listOf(len1.toLong() and 0xFFFFFFFFL, len2.toLong() and 0xFFFFFFFFL)) {
                            if (len > 0 && len < size) {
                                val videoOffset = absPos - len
                                if (videoOffset >= 0) {
                                    // Verify 'ftyp' at the calculated offset
                                    val ftypPos = (videoOffset + 4).toInt()
                                    if (ftypPos + 4 <= size && ftypPos >= startSearch && ftypPos + 4 <= startSearch + buffer.size) {
                                        val bufPos = (ftypPos - startSearch).toInt()
                                        if (buffer[bufPos] == 'f'.toByte() && buffer[bufPos+1] == 't'.toByte() && 
                                            buffer[bufPos+2] == 'y'.toByte() && buffer[bufPos+3] == 'p'.toByte()) {
                                            Log.i(TAG, "Detected Samsung v2 via ftyp: offset=$videoOffset, length=$len")
                                            return LivePhotoInfo(LivePhotoType.SAMSUNG, videoOffset, len)
                                        }
                                    } else if (videoOffset >= 0 && videoOffset + 12 <= size) {
                                        // Fallback: use RandomAccessFile to check ftyp if it's outside our buffer
                                        raf.seek(videoOffset + 4)
                                        val ftyp = ByteArray(4)
                                        raf.readFully(ftyp)
                                        if (ftyp.contentEquals("ftyp".toByteArray())) {
                                            Log.i(TAG, "Detected Samsung v2 (fallback) via ftyp: offset=$videoOffset, length=$len")
                                            return LivePhotoInfo(LivePhotoType.SAMSUNG, videoOffset, len)
                                        }
                                    }
                                }
                            }
                        }
                    }
                    
                    // Move backwards and keep searching
                    if (posInSearch > 0) {
                        posInSearch = findLastInByteArray(buffer, SAMSUNG_MARKER, posInSearch - 1)
                    } else {
                        posInSearch = -1
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error in findSamsungMotionPhoto", e)
        }
        return null
    }

    private fun findXmpMicroVideo(file: File): LivePhotoInfo? {
        try {
            RandomAccessFile(file, "r").use { raf ->
                val size = raf.length()
                // XMP is usually in the first 2MB
                val searchRange = (2 * 1024 * 1024).coerceAtMost(size.toInt())
                val buffer = ByteArray(searchRange)
                raf.readFully(buffer)
                
                var markerIndex = findInByteArray(buffer, XMP_OFFSET_MARKER)
                var markerSize = XMP_OFFSET_MARKER.size
                if (markerIndex == -1) {
                    markerIndex = findInByteArray(buffer, XMP_OFFSET_MARKER_ALT)
                    markerSize = XMP_OFFSET_MARKER_ALT.size
                }
                
                if (markerIndex != -1) {
                    val start = markerIndex + markerSize
                    var end = start
                    while (end < buffer.size && (buffer[end].toInt() and 0xFF).toChar().isDigit()) {
                        end++
                    }
                    if (end > start) {
                        val offsetFromEnd = String(buffer, start, end - start).toLong()
                        if (offsetFromEnd > 0 && offsetFromEnd < size) {
                            val videoOffset = size - offsetFromEnd
                            Log.i(TAG, "Found XMP MicroVideo: offset=$videoOffset, length=$offsetFromEnd")
                            return LivePhotoInfo(LivePhotoType.SAMSUNG, videoOffset, offsetFromEnd)
                        }
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error in findXmpMicroVideo", e)
        }
        return null
    }

    private val LIVE_STILLS = setOf("heic", "heif", "jpg", "jpeg")

    /**
     * Apple Live Photos in an SMB folder's files, as the server pairs them in its listings: an
     * iPhone saves each as a photo and a .MOV of the same name (IMG_1234.HEIC, IMG_1234.MOV),
     * marked as a Live Photo's ([isLive], [isLivePhotoVideo]). The photo gets the video as its
     * liveVideo, and the video leaves the list. Any other .MOV stays.
     */
    fun pairAppleLivePhotos(items: List<FileItem>, isLive: (FileItem) -> Boolean): List<FileItem> {
        fun key(name: String) = name.substringBeforeLast('.').lowercase()
        fun ext(name: String) = name.substringAfterLast('.', "").lowercase()
        val movs = items.filter { !it.isDirectory && ext(it.name) == "mov" }.associateBy { key(it.name) }
        if (movs.isEmpty()) return items
        val paired = mutableSetOf<FileItem>()
        val withVideos = items.map { item ->
            val mov = if (!item.isDirectory && ext(item.name) in LIVE_STILLS) movs[key(item.name)] else null
            if (mov != null && mov !in paired && isLive(mov)) item.copy(liveVideo = mov.path).also { paired.add(mov) } else item
        }
        return withVideos.filter { it !in paired }
    }

    private val LIVE_KEY = "com.apple.quicktime.content.identifier".toByteArray(Charsets.US_ASCII)

    /**
     * Whether a QuickTime file is a Live Photo's video: its index (the moov box, at the start or
     * the end) has the content identifier an iPhone writes. [read] fills a buffer from a position.
     */
    fun isLivePhotoVideo(size: Long, read: (pos: Long, buf: ByteArray) -> Unit): Boolean = try {
        val moov = readMoov(size, read)
        moov != null && findInByteArray(moov, LIVE_KEY) != -1
    } catch (e: Exception) {
        false
    }

    private fun readMoov(size: Long, read: (Long, ByteArray) -> Unit): ByteArray? {
        var pos = 0L
        while (pos + 8 <= size) {
            val h = ByteArray(8).also { read(pos, it) }
            var boxSize = java.nio.ByteBuffer.wrap(h).int.toLong() and 0xFFFFFFFFL
            var headerSize = 8L
            when (boxSize) {
                0L -> boxSize = size - pos
                1L -> { val l = ByteArray(8).also { read(pos + 8, it) }; boxSize = java.nio.ByteBuffer.wrap(l).long; headerSize = 16 }
            }
            if (boxSize < headerSize || pos + boxSize > size) return null
            if (String(h, 4, 4, Charsets.US_ASCII) == "moov") {
                if (boxSize > 8L * 1024 * 1024) return null
                return ByteArray((boxSize - headerSize).toInt()).also { read(pos + headerSize, it) }
            }
            pos += boxSize
        }
        return null
    }

    private fun findInByteArray(data: ByteArray, pattern: ByteArray): Int {
        if (pattern.isEmpty()) return -1
        for (i in 0 until data.size - pattern.size) {
            var found = true
            for (j in pattern.indices) {
                if (data[i + j] != pattern[j]) {
                    found = false
                    break
                }
            }
            if (found) return i
        }
        return -1
    }

    private fun findLastInByteArray(data: ByteArray, pattern: ByteArray, startIndex: Int = data.size - pattern.size): Int {
        if (pattern.isEmpty() || startIndex < 0) return -1
        val actualStart = startIndex.coerceAtMost(data.size - pattern.size)
        for (i in actualStart downTo 0) {
            var found = true
            for (j in pattern.indices) {
                if (data[i + j] != pattern[j]) {
                    found = false
                    break
                }
            }
            if (found) return i
        }
        return -1
    }
}
