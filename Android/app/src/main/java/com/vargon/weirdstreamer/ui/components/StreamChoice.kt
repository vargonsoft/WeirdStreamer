package com.vargon.weirdstreamer.ui.components

import android.content.Context
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.media.MediaCodecList
import androidx.annotation.OptIn
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.HttpDataSource
import androidx.media3.exoplayer.ExoPlaybackException
import androidx.media3.exoplayer.audio.AudioCapabilities
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.AudioTrackInfo
import com.vargon.weirdstreamer.api.models.CodecsInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.Locale

/** What a stream does with the audio, best first: each is what's left when the last one failed. */
enum class AudioStep(val query: String, val label: String) {
    KEEP("&audiocodec=copy", "original"),
    AC3("&audiocodec=ac3", "AC-3 5.1"),
    EAC3("&audiocodec=eac3", "E-AC-3 5.1"),
    AAC_SURROUND("&audiocodec=aac", "AAC 5.1"),
    STEREO("&audiocodec=aac&channels=2", "stereo"),
}

/** Which side of a video failed to play; an error that doesn't say (a parse error) is neither. */
enum class FailedSide { VIDEO, AUDIO, UNKNOWN }

@OptIn(UnstableApi::class)
fun failedSide(error: PlaybackException): FailedSide {
    if (error.errorCode in PlaybackException.ERROR_CODE_AUDIO_TRACK_INIT_FAILED..
        PlaybackException.ERROR_CODE_AUDIO_TRACK_OFFLOAD_INIT_FAILED) return FailedSide.AUDIO
    val mime = (error as? ExoPlaybackException)?.rendererFormat?.sampleMimeType
    return when {
        MimeTypes.isAudio(mime) -> FailedSide.AUDIO
        MimeTypes.isVideo(mime) -> FailedSide.VIDEO
        else -> FailedSide.UNKNOWN
    }
}

/**
 * The server's reason when it can't read the video at all (gone, or a share refusing it), from its
 * answer marked `X-Stream-Error: unreadable`; null for any other error. No other stream would help.
 */
@OptIn(UnstableApi::class)
fun unreadableReason(error: PlaybackException): String? {
    var cause: Throwable? = error.cause
    while (cause != null && cause !is HttpDataSource.InvalidResponseCodeException) cause = cause.cause
    val http = cause as? HttpDataSource.InvalidResponseCodeException ?: return null
    val marked = http.headerFields.any { (name, values) ->
        name.equals("X-Stream-Error", ignoreCase = true) && "unreadable" in values
    }
    if (!marked) return null
    return runCatching { org.json.JSONObject(String(http.responseBody)).optString("message") }
        .getOrNull()?.takeIf { it.isNotBlank() } ?: "The server can't read this file."
}

/** What the device can do with audio right now: asked at each playback, as outputs come and go. */
@OptIn(UnstableApi::class)
class AudioOutput private constructor(
    private val plays: (String, Int) -> Boolean,
    val ac3: Boolean,
    val eac3: Boolean,
    val aacSurround: Boolean,
) {
    fun plays(track: AudioTrackInfo): Boolean = AUDIO_MIME[track.codec]?.let { plays(it, track.channels) } == true

    companion object {
        // What a stream can carry untouched. TrueHD is only experimental in MP4; others get converted.
        private val AUDIO_MIME = mapOf(
            "aac" to MimeTypes.AUDIO_AAC, "ac3" to MimeTypes.AUDIO_AC3, "eac3" to MimeTypes.AUDIO_E_AC3,
            "mp3" to MimeTypes.AUDIO_MPEG, "opus" to MimeTypes.AUDIO_OPUS, "flac" to MimeTypes.AUDIO_FLAC,
            "dts" to MimeTypes.AUDIO_DTS,
        )

        fun current(context: Context): AudioOutput {
            val attributes = androidx.media3.common.AudioAttributes.DEFAULT
            val capabilities = AudioCapabilities.getCapabilities(context, attributes, null)
            // Sent as it is to whatever is on HDMI: a receiver, a soundbar, the TV.
            fun passedThrough(mime: String, channels: Int) = capabilities.isPassthroughPlaybackSupported(
                Format.Builder().setSampleMimeType(mime).setChannelCount(channels).setSampleRate(48000).build(), attributes)
            val decoders = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos
                .filter { !it.isEncoder }.flatMap { it.supportedTypes.asList() }.map { it.lowercase(Locale.US) }.toSet()
            val manager = context.getSystemService(AudioManager::class.java)
            val multichannel = manager.getDevices(AudioManager.GET_DEVICES_OUTPUTS).any { device ->
                device.type in listOf(AudioDeviceInfo.TYPE_HDMI, AudioDeviceInfo.TYPE_HDMI_ARC, AudioDeviceInfo.TYPE_HDMI_EARC) &&
                    (device.channelCounts.maxOrNull() ?: 0) >= 6
            }
            return AudioOutput(
                plays = { mime, channels -> passedThrough(mime, channels) || mime.lowercase(Locale.US) in decoders },
                ac3 = passedThrough(MimeTypes.AUDIO_AC3, 6),
                eac3 = passedThrough(MimeTypes.AUDIO_E_AC3, 6),
                aacSurround = multichannel && MimeTypes.AUDIO_AAC in decoders,
            )
        }
    }
}

/**
 * The audio for a stream in the order to try it: the track as it is if the device plays it, the
 * best 5.1 its output takes, then stereo. A small stream keeps only audio that stays small.
 */
fun audioLadder(track: AudioTrackInfo?, output: AudioOutput, small: Boolean): List<AudioStep> = buildList {
    if (track != null && output.plays(track) && !(small && track.codec in setOf("flac", "dts"))) add(AudioStep.KEEP)
    if (track != null && track.channels > 2) {
        if (output.ac3 && track.codec != "ac3") add(AudioStep.AC3)
        if (output.eac3 && track.codec != "eac3") add(AudioStep.EAC3)
        if (output.aacSurround && !AudioFallback.stereo) add(AudioStep.AAC_SURROUND)
    }
    add(AudioStep.STEREO)
}

/**
 * How a video from the server is streamed when it isn't the original, deciding the video and the
 * audio apart, as the web does. Compatibility (Auto) keeps what the device plays: it copies 8-bit
 * 4:2:0 H.264 (transcode=audio) and takes the audio from [audioLadder]. Compatibility (Safe) keeps
 * nothing. What fails steps down: a copied video gets a transcode, audio the next on its ladder.
 */
class StreamChoice(private val api: ApiService, private val encodedPath: String) {
    /** A decision for a stream, applied once the player knows where the stream will start.
     *  [track] is the audio track it plays (the server's index; -1: the file's default). */
    class Plan(val copiesVideo: Boolean, val audio: AudioStep?, val ladder: List<AudioStep>, val track: Int)

    var copiesVideo by mutableStateOf(false)
        private set
    var audio by mutableStateOf<AudioStep?>(null)
        private set
    /** The audio track chosen (the server's index among the file's audio tracks), -1 for the default. */
    var audioTrack by mutableStateOf(-1)
        private set
    /** The file's audio tracks, once the server has said. */
    var audioTracks by mutableStateOf<List<AudioTrackInfo>>(emptyList())
        private set
    private var ladder = emptyList<AudioStep>()
    private var videoFailed = false
    private var codecs: CodecsInfo? = null

    private suspend fun codecs(): CodecsInfo? = codecs ?: withContext(Dispatchers.IO) {
        runCatching { api.getCodecs(encodedPath).body() }.getOrNull()
    }?.also { codecs = it; audioTracks = it.audio.orEmpty() }

    /** Asks the server for the file's tracks (once). */
    suspend fun load() { codecs() }

    private fun defaultTrack() = codecs?.audio?.let { list -> list.firstOrNull { it.isDefault } ?: list.firstOrNull() }

    private fun track(index: Int = audioTrack) = codecs?.audio?.getOrNull(index) ?: defaultTrack()

    /** The track the list shows as playing: the one chosen, else the default. */
    val shownTrack: Int get() = if (audioTrack >= 0) audioTrack else defaultTrack()?.index ?: -1

    /** Whether the original file can play audio track [index] on this device: the player switches to it itself. */
    fun originalPlays(context: Context, index: Int): Boolean {
        val track = codecs?.audio?.getOrNull(index) ?: return true
        return track === defaultTrack() || AudioOutput.current(context).plays(track)
    }

    /** Chooses audio track [index] for the original file, which plays it itself (no new stream). */
    fun chooseForOriginal(index: Int) { audioTrack = index }

    /** The stream for [mode]; [failed] is what the original just failed on, if it did. */
    suspend fun plan(context: Context, mode: TranscodeMode, mayCopy: Boolean, failed: FailedSide? = null, track: Int = audioTrack): Plan {
        val info = codecs()
        if (failed == FailedSide.VIDEO) videoFailed = true
        val video = info?.video
        val copy = mode.auto && mayCopy && !videoFailed && video != null && video.codec == "h264" &&
            video.pixFmt in listOf("yuv420p", "yuvj420p")
        val all = when {
            info == null -> emptyList()
            mode.safe -> listOf(AudioStep.STEREO)
            else -> audioLadder(track(track), AudioOutput.current(context), small = mode.bitrate != null || mode.resolution != null)
        }
        val steps = if (failed == FailedSide.AUDIO) all - AudioStep.KEEP else all
        return Plan(copy, steps.firstOrNull(), steps, track)
    }

    /** The next thing to try after [failed]; null when nothing is left. */
    fun nextAfter(failed: FailedSide): Plan? {
        if (failed != FailedSide.AUDIO && copiesVideo) {
            videoFailed = true
            return Plan(false, audio, ladder, audioTrack)
        }
        if (failed == FailedSide.VIDEO) return null
        val next = ladder.getOrNull(ladder.indexOf(audio) + 1) ?: return null
        if (audio == AudioStep.AAC_SURROUND) AudioFallback.stereo = true
        return Plan(copiesVideo, next, ladder, audioTrack)
    }

    fun apply(plan: Plan) {
        copiesVideo = plan.copiesVideo
        audio = plan.audio
        ladder = plan.ladder
        audioTrack = plan.track
    }

    /** Back to the original file, keeping the audio track chosen. */
    fun clear() = apply(Plan(false, null, emptyList(), audioTrack))

    /** The stream's audio track parameter (audio=N), after the video's. */
    fun trackQuery(): String = if (audioTrack >= 0) "&audio=$audioTrack" else ""

    /** Stops copying, for burned-in subtitles: the stream restarts with a new plan. */
    fun stopCopying() { copiesVideo = false }

    /** The URL parameters for the stream's video, after the mode's own. */
    fun videoQuery(mode: TranscodeMode): String =
        if (copiesVideo) "transcode=audio" else "transcode=1" + transcodeQuery(mode)

    /** What the stream does, for the Transcode menu and the toast when it changes by itself. */
    fun status(mode: TranscodeMode): String? {
        val track = track()
        val original = track?.let { t -> t.codec.uppercase(Locale.US) + if (t.channels > 2) " ${channelsName(t.channels)}" else "" }
        val keptAudio = "original" + (original?.let { " ($it)" } ?: "")
        return when {
            mode.isOriginal -> if (codecs == null) null else "Video: original · Audio: $keptAudio"
            else -> "Video: " + (if (copiesVideo) "copied" else "transcoded to H.264") + " · Audio: " + when (audio) {
                null -> "converted to AAC"
                AudioStep.KEEP -> keptAudio
                else -> "converted to ${audio?.label}"
            }
        }
    }

    private fun channelsName(channels: Int) = if (channels >= 8) "7.1" else if (channels >= 6) "5.1" else "$channels.0"

    /** Where a stream asked to start at [ms] begins: the keyframe the server's seek lands on, up to seconds earlier. */
    suspend fun startOf(ms: Long): Long {
        if (ms <= 0) return 0
        val point = withContext(Dispatchers.IO) {
            runCatching { api.getSeekPoint(encodedPath, streamStart(ms)).body() }.getOrNull()
        }
        // Below 0 is no time at all (an older server reading an AVI's missing timestamp).
        return point?.let { (it.start * 1000).toLong() }?.takeIf { it >= 0 } ?: ms
    }

    /** Where to ask a stream to start for a jump from [fromMs] to [targetMs], and where it then begins.
     *  A jump forward that the keyframe before [targetMs] would undo (keyframes far apart, a short
     *  jump) goes on to the next keyframe instead, further than asked: otherwise it never moves. */
    suspend fun jump(fromMs: Long, targetMs: Long): Pair<Long, Long> {
        val begins = startOf(targetMs)
        if (targetMs <= fromMs || begins > fromMs) return targetMs to begins
        val next = withContext(Dispatchers.IO) {
            runCatching { api.getSeekPoint(encodedPath, streamStart(targetMs), next = 1).body() }.getOrNull()
        } ?: return targetMs to begins
        val ask = next.ask?.toDoubleOrNull()?.let { Math.round(it * 1000) } ?: return targetMs to begins
        val nextBegins = (next.start * 1000).toLong()
        return if (nextBegins > fromMs) ask to nextBegins else targetMs to begins
    }

    companion object {
        /** For a stream URL from the server: null for other sources. */
        fun forStream(streamUrl: String): StreamChoice? =
            ServerSubtitles.streamApi(streamUrl)?.let { (api, encoded) -> StreamChoice(api, encoded) }

        /** A stream's start= param; the seek point is asked for with the same value. */
        fun streamStart(ms: Long): String = String.format(Locale.US, "%.2f", ms / 1000.0)
    }
}

/** Whether tracks of [type] are in the stream but none can be played: no error, just silence or no picture. */
fun unplayable(tracks: androidx.media3.common.Tracks, type: Int): Boolean =
    tracks.containsType(type) && !tracks.isTypeSupported(type, true)

/** What the player can't play in [tracks] although no error says so; null: nothing. */
fun unplayableSide(tracks: androidx.media3.common.Tracks): FailedSide? = when {
    unplayable(tracks, C.TRACK_TYPE_VIDEO) -> FailedSide.VIDEO
    unplayable(tracks, C.TRACK_TYPE_AUDIO) -> FailedSide.AUDIO
    else -> null
}

/**
 * Has the player play the original file's audio track [index] (the server's index among the file's
 * audio tracks; -1: the player's own choice). The player lists a file's audio tracks in the order
 * the server does; when it sees another number of them, nothing changes and this returns false.
 */
fun selectAudioTrack(player: androidx.media3.common.Player, index: Int, serverCount: Int): Boolean {
    val params = player.trackSelectionParameters.buildUpon().clearOverridesOfType(C.TRACK_TYPE_AUDIO)
    if (index < 0) {
        player.trackSelectionParameters = params.build()
        return true
    }
    val groups = player.currentTracks.groups.filter { it.type == C.TRACK_TYPE_AUDIO }
    if (groups.size != serverCount || index >= groups.size) return false
    player.trackSelectionParameters = params
        .setOverrideForType(androidx.media3.common.TrackSelectionOverride(groups[index].mediaTrackGroup, 0)).build()
    return true
}
