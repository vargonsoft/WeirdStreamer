package com.vargon.weirdstreamer

import android.media.MediaMetadataRetriever
import android.util.Log
import java.io.File
import java.io.RandomAccessFile

enum class LivePhotoType {
    SAMSUNG, APPLE
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
        
        // 3. Check Apple (HEIC with embedded video track)
        val appleInfo = findAppleLivePhoto(file)
        if (appleInfo != null) return appleInfo
        
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

    private fun findAppleLivePhoto(file: File): LivePhotoInfo? {
        val retriever = MediaMetadataRetriever()
        try {
            retriever.setDataSource(file.absolutePath)
            val hasVideo = retriever.extractMetadata(MediaMetadataRetriever.METADATA_KEY_HAS_VIDEO)
            if (hasVideo == "yes") {
                Log.i(TAG, "Found Apple Live Photo (embedded video track)")
                return LivePhotoInfo(LivePhotoType.APPLE, 0, file.length())
            }
        } catch (_: Exception) {
        } finally {
            retriever.release()
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
