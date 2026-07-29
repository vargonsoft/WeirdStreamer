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
