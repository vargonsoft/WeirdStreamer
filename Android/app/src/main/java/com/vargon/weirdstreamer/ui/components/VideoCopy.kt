package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.api.ApiService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.Locale

/**
 * Compatibility (Auto) keeps a video as it is when it can, as on the web: the server copies 8-bit
 * 4:2:0 H.264 into a stream with AAC audio (transcode=audio), and a player gives it a full
 * transcode only if that fails too.
 */
class VideoCopy(private val api: ApiService, private val encodedPath: String) {
    private var copyable: Boolean? = null

    /** Whether the server would copy this video; a failed check gives it a full transcode. */
    suspend fun possible(): Boolean = copyable ?: withContext(Dispatchers.IO) {
        val video = runCatching { api.getCodecs(encodedPath).body() }.getOrNull()?.video
            ?: return@withContext false
        (video.codec == "h264" && video.pixFmt in listOf("yuv420p", "yuvj420p")).also { copyable = it }
    }

    /** Where a stream asked to start at [ms] begins: the keyframe the server's seek lands on, up to seconds earlier. */
    suspend fun startOf(ms: Long): Long {
        if (ms <= 0) return 0
        val point = withContext(Dispatchers.IO) {
            runCatching { api.getSeekPoint(encodedPath, streamStart(ms)).body() }.getOrNull()
        }
        return point?.let { (it.start * 1000).toLong() } ?: ms
    }

    companion object {
        /** For a stream URL from the server: null for other sources. */
        fun forStream(streamUrl: String): VideoCopy? =
            ServerSubtitles.streamApi(streamUrl)?.let { (api, encoded) -> VideoCopy(api, encoded) }

        /** A stream's start= param; the seek point is asked for with the same value. */
        fun streamStart(ms: Long): String = String.format(Locale.US, "%.2f", ms / 1000.0)
    }
}
