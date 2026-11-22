package com.vargon.weirdstreamer.downloads

import android.content.Context
import android.media.MediaCodecInfo.CodecProfileLevel
import android.media.MediaCodecList
import com.vargon.weirdstreamer.api.models.AudioTrackInfo
import com.vargon.weirdstreamer.api.models.CodecsInfo
import com.vargon.weirdstreamer.api.models.VideoCodec
import com.vargon.weirdstreamer.ui.components.AudioOutput
import java.util.Locale

/**
 * Whether this device plays a file as it is, offline: a download has no server to transcode it.
 * The video needs a decoder for its codec, its bit depth and its size; its audio track (the default, or the one chosen) a
 * decoder (or an output that takes it as it is).
 */
class OfflinePlayback(val video: Boolean, val audio: Boolean, private val videoName: String?, private val audioName: String?) {
    val plays get() = video && audio

    /** "its video (HEVC 10-bit)", "its audio (E-AC-3 5.1)" or both, for "This phone can't play …". */
    val problem: String?
        get() = listOfNotNull(
            if (!video) "its video" + (videoName?.let { " ($it)" } ?: "") else null,
            if (!audio) "its audio" + (audioName?.let { " ($it)" } ?: "") else null
        ).joinToString(" or ").ifEmpty { null }

    companion object {
        private val VIDEO_MIME = mapOf(
            "h264" to "video/avc", "hevc" to "video/hevc", "vp9" to "video/x-vnd.on2.vp9", "vp8" to "video/x-vnd.on2.vp8",
            "av1" to "video/av01", "mpeg4" to "video/mp4v-es", "mpeg2video" to "video/mpeg2", "h263" to "video/3gpp",
        )
        private val VIDEO_NAME = mapOf(
            "h264" to "H.264", "hevc" to "HEVC", "vp9" to "VP9", "vp8" to "VP8", "av1" to "AV1", "mpeg4" to "MPEG-4",
            "mpeg2video" to "MPEG-2", "vc1" to "VC-1", "h263" to "H.263",
        )
        private val AUDIO_NAME = mapOf(
            "aac" to "AAC", "ac3" to "AC-3", "eac3" to "E-AC-3", "dts" to "DTS", "truehd" to "TrueHD", "mp3" to "MP3",
            "opus" to "Opus", "flac" to "FLAC", "vorbis" to "Vorbis", "pcm_s16le" to "PCM",
        )

        private val decoders by lazy {
            MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos.filter { !it.isEncoder }
        }

        /** [audioTrack]: the audio track it's downloaded with (the server's index), -1 the default. */
        fun of(context: Context, codecs: CodecsInfo, width: Int, height: Int, audioTrack: Int = -1): OfflinePlayback {
            val track = codecs.audio?.let { list -> list.firstOrNull { it.index == audioTrack } ?: list.firstOrNull { it.isDefault } ?: list.firstOrNull() }
            val video = codecs.video
            return OfflinePlayback(
                video = video == null || playsVideo(video, width, height),
                audio = track == null || AudioOutput.current(context).plays(track),
                videoName = video?.let { v ->
                    (VIDEO_NAME[v.codec] ?: v.codec.uppercase(Locale.US)) + if (tenBit(v)) " 10-bit" else ""
                },
                audioName = track?.let { audioName(it) }
            )
        }

        private fun audioName(t: AudioTrackInfo): String =
            (AUDIO_NAME[t.codec] ?: t.codec.uppercase(Locale.US)) + when {
                t.channels >= 8 -> " 7.1"
                t.channels >= 6 -> " 5.1"
                else -> ""
            }

        private fun tenBit(v: VideoCodec) = v.pixFmt?.let { "10" in it || "12" in it } == true

        /** Whether a decoder takes [v]'s codec, its bit depth (as a profile) and a [width]×[height] picture. */
        fun playsVideo(v: VideoCodec, width: Int, height: Int): Boolean {
            val mime = VIDEO_MIME[v.codec] ?: return false
            val profile = if (!tenBit(v)) null else when (v.codec) {
                "hevc" -> CodecProfileLevel.HEVCProfileMain10
                "h264" -> CodecProfileLevel.AVCProfileHigh10
                "vp9" -> CodecProfileLevel.VP9Profile2
                "av1" -> CodecProfileLevel.AV1ProfileMain10
                else -> return false
            }
            return decoders.any { info ->
                info.supportedTypes.any { it.equals(mime, ignoreCase = true) } && runCatching {
                    val caps = info.getCapabilitiesForType(mime)
                    (profile == null || caps.profileLevels.any { it.profile == profile }) &&
                        (width <= 0 || height <= 0 || caps.videoCapabilities.isSizeSupported(width, height) ||
                            caps.videoCapabilities.isSizeSupported(height, width))
                }.getOrDefault(false)
            }
        }
    }
}
