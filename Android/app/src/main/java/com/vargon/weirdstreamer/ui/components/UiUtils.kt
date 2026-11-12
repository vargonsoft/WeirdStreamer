package com.vargon.weirdstreamer.ui.components

import java.util.*

data class TranscodeMode(
    val name: String,
    val bitrate: String? = null,
    val resolution: String? = null,
    val isOriginal: Boolean = false,
    val auto: Boolean = false, // keeps what the device plays (StreamChoice)
    val safe: Boolean = false, // keeps nothing: H.264 and stereo AAC, whatever the device says
)

val transcodeModes = listOf(
    TranscodeMode("Original", isOriginal = true),
    TranscodeMode("Compatibility (Auto)", auto = true),
    TranscodeMode("Compatibility (Safe)", safe = true),
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
 * A track chosen for each video on this device, so it plays the same way next time, remembered
 * by its label (and its position, for a file whose tracks were renamed). Index -1: off for
 * subtitles, the file's default for audio.
 */
open class TrackMemory(private val prefsName: String) {
    data class Choice(val index: Int, val label: String?)

    private fun prefs(context: android.content.Context) =
        context.getSharedPreferences(prefsName, android.content.Context.MODE_PRIVATE)

    fun get(context: android.content.Context, video: String): Choice? {
        val stored = prefs(context).getString(video, null) ?: return null
        if (stored == "off") return Choice(-1, null)
        val index = stored.substringBefore('|').toIntOrNull() ?: return null
        return Choice(index, stored.substringAfter('|', "").ifEmpty { null })
    }

    fun set(context: android.content.Context, video: String, index: Int, label: String?) {
        prefs(context).edit().putString(video, if (index < 0) "off" else "$index|${label.orEmpty()}").apply()
    }

    /** Where [choice] is in [labels] now: the same label, else the same position, else none. */
    fun resolve(choice: Choice, labels: List<String>): Int = when {
        choice.index < 0 -> -1
        choice.label != null && labels.indexOf(choice.label) >= 0 -> labels.indexOf(choice.label)
        choice.index < labels.size -> choice.index
        else -> -1
    }
}

/** The subtitle chosen for each video: off, or a track. */
object SubtitleMemory : TrackMemory("subtitle_choices")

/** The audio track chosen for each video (the server's index among its audio tracks); none: the default. */
object AudioMemory : TrackMemory("audio_choices")

/**
 * Names a file's audio tracks as the server lists them, as the web does: "English · Commentary ·
 * AAC · stereo", with the file's default marked.
 */
fun audioLabels(tracks: List<com.vargon.weirdstreamer.api.models.AudioTrackInfo>): List<String> = tracks.map { t ->
    val language = languageName(t.language)
    val title = t.title?.trim()?.takeIf { it.isNotEmpty() && !it.equals(language, ignoreCase = true) }?.take(40)
    val channels = when (t.channels) {
        0 -> null; 1 -> "mono"; 2 -> "stereo"; 6 -> "5.1"; 8 -> "7.1"; else -> "${t.channels} ch"
    }
    val codec = when (t.codec) {
        "eac3" -> "E-AC-3"; "ac3" -> "AC-3"; "truehd" -> "TrueHD"; "opus" -> "Opus"; "vorbis" -> "Vorbis"
        else -> t.codec.uppercase(Locale.US)
    }
    listOfNotNull(language ?: "Track ${t.index + 1}", title, codec, channels).joinToString(" · ") +
        if (t.isDefault) " (default)" else ""
}

/**
 * Names subtitle tracks so ones in the same language can be told apart: "English · SDH · Image",
 * with the file's title for the track, whether it's text or images (Blu-ray PGS, DVD), forced and
 * hearing-impaired marks, and a number for tracks that still read the same.
 */
fun subtitleLabels(formats: List<androidx.media3.common.Format>): List<String> {
    val base = formats.mapIndexed { i, f ->
        val language = f.language?.takeIf { it.isNotBlank() && it != "und" }
            ?.let { Locale.forLanguageTag(it).getDisplayLanguage(Locale.getDefault()).ifBlank { it } }
        val title = f.label?.trim()?.takeIf { it.isNotEmpty() && !it.equals(language, ignoreCase = true) }
        val kind = when (f.sampleMimeType) {
            androidx.media3.common.MimeTypes.APPLICATION_PGS, androidx.media3.common.MimeTypes.APPLICATION_VOBSUB,
            androidx.media3.common.MimeTypes.APPLICATION_DVBSUBS -> "Image"
            androidx.media3.common.MimeTypes.APPLICATION_CEA608, androidx.media3.common.MimeTypes.APPLICATION_CEA708 -> "Closed captions"
            null -> null
            else -> "Text"
        }
        val marks = listOfNotNull(
            if (f.selectionFlags and androidx.media3.common.C.SELECTION_FLAG_FORCED != 0) "Forced" else null,
            if (f.roleFlags and androidx.media3.common.C.ROLE_FLAG_DESCRIBES_MUSIC_AND_SOUND != 0 &&
                title?.contains("SDH", ignoreCase = true) != true) "SDH" else null,
            if (f.roleFlags and androidx.media3.common.C.ROLE_FLAG_COMMENTARY != 0) "Commentary" else null
        )
        listOfNotNull(language ?: "Subtitle ${i + 1}", title?.take(40)) .plus(marks).plus(listOfNotNull(kind)).joinToString(" · ")
    }
    // Number the ones that still read the same: "Chinese · Image (1)", "Chinese · Image (2)".
    val counts = base.groupingBy { it }.eachCount()
    val seen = HashMap<String, Int>()
    return base.map { label ->
        if ((counts[label] ?: 0) < 2) label else "$label (${seen.merge(label, 1, Int::plus)})"
    }
}

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
 * Stereo audio for transcodes (the stream's channels=2): some decoders fail on the 5.1 AAC a
 * transcode of surround audio carries. Set the first time one fails, and kept while the app
 * runs, so later videos start in stereo instead of failing first.
 */
object AudioFallback {
    @Volatile var stereo = false

    /** Whether [error] is the audio decoder failing (the cue to retry in stereo). */
    @androidx.annotation.OptIn(androidx.media3.common.util.UnstableApi::class)
    fun isAudioDecoderFailure(error: androidx.media3.common.PlaybackException): Boolean {
        val decoder = error.errorCode in androidx.media3.common.PlaybackException.ERROR_CODE_DECODER_INIT_FAILED..
            androidx.media3.common.PlaybackException.ERROR_CODE_DECODING_RESOURCES_RECLAIMED
        val format = (error as? androidx.media3.exoplayer.ExoPlaybackException)?.rendererFormat
        return decoder && androidx.media3.common.MimeTypes.isAudio(format?.sampleMimeType)
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

// Keyboards for addresses and usernames. Picking a suggestion in a plain text field adds a space
// after the word ("babylon5 "), which an address can't have; URL and email keyboards add none.
fun addressKeyboard(imeAction: androidx.compose.ui.text.input.ImeAction = androidx.compose.ui.text.input.ImeAction.Default) =
    androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Uri,
        autoCorrectEnabled = false, imeAction = imeAction)

fun usernameKeyboard(imeAction: androidx.compose.ui.text.input.ImeAction = androidx.compose.ui.text.input.ImeAction.Default) =
    androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Email,
        autoCorrectEnabled = false, imeAction = imeAction)

// A password, key or token: the keyboard neither suggests nor learns what's typed (a plain text
// field would offer it back as a suggestion).
fun secretKeyboard(imeAction: androidx.compose.ui.text.input.ImeAction = androidx.compose.ui.text.input.ImeAction.Default) =
    androidx.compose.foundation.text.KeyboardOptions(keyboardType = androidx.compose.ui.text.input.KeyboardType.Password,
        autoCorrectEnabled = false, imeAction = imeAction)
