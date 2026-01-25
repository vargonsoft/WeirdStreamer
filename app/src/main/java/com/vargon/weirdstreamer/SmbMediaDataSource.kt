package com.vargon.weirdstreamer

import android.media.MediaDataSource
import jcifs.smb.SmbFile
import jcifs.smb.SmbRandomAccessFile
import java.io.IOException
import kotlin.math.min

class SmbMediaDataSource(smbFile: SmbFile) : MediaDataSource() {
    private var raf: SmbRandomAccessFile? = try {
        smbFile.openRandomAccess("r")
    } catch (e: IOException) {
        null
    }
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
        } catch (e: IOException) {
            -1
        }
    }

    override fun getSize(): Long = try {
        raf?.length() ?: -1L
    } catch (e: IOException) {
        -1L
    }

    override fun close() {
        closed = true
        try {
            raf?.close()
        } catch (e: IOException) {
        }
        raf = null
    }
}
