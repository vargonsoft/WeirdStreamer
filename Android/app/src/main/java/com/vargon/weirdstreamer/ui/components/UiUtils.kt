package com.vargon.weirdstreamer.ui.components

import java.util.*

data class TranscodeMode(
    val name: String,
    val bitrate: String? = null,
    val resolution: String? = null,
    val isOriginal: Boolean = false
)

val transcodeModes = listOf(
    TranscodeMode("Original", isOriginal = true),
    TranscodeMode("Compatibility (Auto)"),
    TranscodeMode("Quality -20%", bitrate = "80%"),
    TranscodeMode("Quality -35%", bitrate = "65%"),
    TranscodeMode("Quality -50%", bitrate = "50%"),
    TranscodeMode("720p", resolution = "720"),
    TranscodeMode("480p", resolution = "480"),
    TranscodeMode("360p", resolution = "360")
)

fun formatTime(ms: Long): String {
    val totalSeconds = ms / 1000
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    return String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
}

/**
 * Subtitles start off: a track the file marks "default" isn't turned on by itself (many MKVs mark
 * one), only forced ones are (translations of foreign-language lines). The player's subtitle
 * menu turns one on.
 */
fun subtitlesOffByDefault(params: androidx.media3.common.TrackSelectionParameters): androidx.media3.common.TrackSelectionParameters =
    params.buildUpon()
        .setIgnoredTextSelectionFlags(androidx.media3.common.C.SELECTION_FLAG_DEFAULT)
        .setPreferredTextLanguage(null)
        .setPreferredTextRoleFlags(0)
        .setSelectUndeterminedTextLanguage(false)
        .build()

/**
 * What this device's hardware H.264 decoders can play, so videos beyond them (4K on many phones)
 * are transcoded down to a size they handle instead of failing. Software decoders don't count:
 * they "support" 4K but can't keep up.
 */
object VideoDecoders {
    private val heights = listOf(2160, 1440, 1080, 720, 480, 360)

    private val decoders: List<android.media.MediaCodecInfo.VideoCapabilities> by lazy {
        android.media.MediaCodecList(android.media.MediaCodecList.REGULAR_CODECS).codecInfos
            .filter { info ->
                !info.isEncoder && info.supportedTypes.any { it.equals("video/avc", ignoreCase = true) } &&
                    (android.os.Build.VERSION.SDK_INT < android.os.Build.VERSION_CODES.Q || info.isHardwareAccelerated)
            }
            .mapNotNull { runCatching { it.getCapabilitiesForType("video/avc").videoCapabilities }.getOrNull() }
    }

    /** Whether a [width]x[height] video can be decoded in hardware (true when nothing is known). */
    fun canPlay(width: Int?, height: Int?): Boolean {
        if (width == null || height == null || width <= 0 || height <= 0 || decoders.isEmpty()) return true
        // Either orientation: portrait videos are rotated landscape ones as far as decoders go.
        return decoders.any { it.isSizeSupported(width, height) || it.isSizeSupported(height, width) }
    }

    /** The tallest standard 16:9 height the hardware decodes, for capping transcodes (null: no limit known). */
    val maxHeight: Int? by lazy {
        if (decoders.isEmpty()) null
        else heights.firstOrNull { h -> val w = (h * 16 / 9) and 1.inv(); decoders.any { it.isSizeSupported(w, h) } }
    }
}

/**
 * The query for a transcoded stream in [mode]: its bitrate, and a resolution no taller than the
 * hardware decodes (the server never scales up, so a small video keeps its size).
 */
fun transcodeQuery(mode: TranscodeMode): String = buildString {
    mode.bitrate?.let { append("&bitrate=$it") }
    val height = listOfNotNull(mode.resolution?.toIntOrNull(), VideoDecoders.maxHeight).minOrNull()
    height?.let { append("&resolution=$it") }
}
