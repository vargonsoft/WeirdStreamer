package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.SubtitleDownloadRequest
import com.vargon.weirdstreamer.api.models.SubtitleHit
import com.vargon.weirdstreamer.api.models.SubtitleSearch
import com.vargon.weirdstreamer.api.models.SubtitleTrack
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.net.URLDecoder
import java.util.Locale

/** One subtitle line on screen, from startMs to endMs (in the file's time). */
data class SubtitleCue(val startMs: Long, val endMs: Long, val text: AnnotatedString)

/**
 * Subtitles for a video played from a WeirdStreamer server, as the web player has them: the
 * video's own tracks, then files beside it and downloads from OpenSubtitles. Text tracks come
 * from the server as WebVTT, a couple of minutes at a time, and are drawn by [SubtitleOverlay], so
 * they show the same whatever the stream is (a transcode has none of its own); picture tracks
 * (PGS, DVD) can only be burned into a transcode ([burnIn]).
 */
class ServerSubtitles(private val api: ApiService, private val encodedPath: String) {
    var tracks by mutableStateOf<List<SubtitleTrack>>(emptyList())
        private set
    var selected by mutableIntStateOf(-1)
        private set
    var loaded by mutableStateOf(false)
        private set

    val labels: List<String> get() = serverSubtitleLabels(tracks)
    val selectedTrack: SubtitleTrack? get() = tracks.getOrNull(selected)

    /** The picture track to burn into the stream (subtitle=N), or -1. */
    val burnIn: Int get() = selectedTrack?.takeIf { !it.text }?.index ?: -1

    /** The selected track's timing correction (ms; positive shows its lines later). */
    val offsetMs: Int get() = selectedTrack?.offsetMs ?: 0

    private val saver = kotlinx.coroutines.CoroutineScope(Dispatchers.IO + kotlinx.coroutines.SupervisorJob())
    private var saving: kotlinx.coroutines.Job? = null

    /**
     * Moves the selected track's lines by [deltaMs] (negative: earlier), at once, and saves the
     * correction for every player a moment after the last change.
     */
    fun shift(deltaMs: Int) {
        val track = selectedTrack?.takeIf { it.text } ?: return
        val moved = track.copy(offsetMs = track.offsetMs + deltaMs)
        tracks = tracks.toMutableList().also { it[selected] = moved }
        val key = moved.key ?: return
        saving?.cancel()
        saving = saver.launch {
            delay(600)
            runCatching { api.setSubtitleOffset(com.vargon.weirdstreamer.api.models.SubtitleOffsetRequest(encodedPath, key, moved.offsetMs)) }
        }
    }

    // The cues loaded for the selected track: from fromMs to untilMs.
    private var cues = emptyList<SubtitleCue>()
    private var fromMs = 0L
    private var untilMs = 0L
    private var generation = 0 // a new choice drops cues still arriving for the old one

    suspend fun load() {
        val got = withContext(Dispatchers.IO) { runCatching { api.getCodecs(encodedPath).body() }.getOrNull() }
        got?.subtitles?.let { tracks = it }
        loaded = true
    }

    fun select(index: Int) {
        selected = if (index in tracks.indices) index else -1
        cues = emptyList()
        fromMs = 0
        untilMs = 0
        generation++
    }

    /** Loads the cues around [positionMs], and the next window before it's needed. */
    suspend fun keepAhead(playerMs: Long) {
        val positionMs = playerMs - offsetMs // in the subtitles' own time
        val track = selectedTrack?.takeIf { it.text } ?: return
        val outside = positionMs < fromMs || positionMs > untilMs
        if (!outside && positionMs < untilMs - AHEAD_MS) return
        val start = if (outside) (positionMs - 10_000).coerceAtLeast(0) else untilMs
        val gen = generation
        val body = withContext(Dispatchers.IO) {
            runCatching {
                api.getSubtitles(encodedPath, track.index, start / 1000.0, WINDOW_MS / 1000.0).body()?.string()
            }.getOrNull()
        } ?: return
        if (gen != generation) return
        val window = parseWebVtt(body)
        if (outside) {
            cues = window
            fromMs = start
        } else {
            // The windows overlap a little: skip lines already had (by time and text; two different
            // lines can start together, two speakers, say).
            val known = cues.mapTo(HashSet()) { it.startMs to it.text.text }
            cues = cues + window.filter { (it.startMs to it.text.text) !in known }
        }
        untilMs = start + WINDOW_MS
    }

    /** The line to show at [positionMs], if any. */
    fun cueAt(playerMs: Long): AnnotatedString? {
        if (selectedTrack?.text != true) return null
        val positionMs = playerMs - offsetMs // in the subtitles' own time
        val at = cues.filter { positionMs >= it.startMs && positionMs < it.endMs }
        return when (at.size) {
            0 -> null
            1 -> at[0].text
            else -> buildAnnotatedString { at.forEachIndexed { i, c -> if (i > 0) append("\n"); append(c.text) } }
        }
    }

    /** OpenSubtitles results for the video; throws with the server's message. */
    suspend fun search(languages: List<String>): SubtitleSearch = withContext(Dispatchers.IO) {
        val res = api.searchSubtitles(encodedPath, languages.joinToString(","))
        res.body() ?: error(serverMessage(res.errorBody()?.string()) ?: "Search failed (${res.code()})")
    }

    /** Downloads (or reuses) a result and selects it; throws with the server's message. */
    suspend fun download(hit: SubtitleHit): Int {
        val got = withContext(Dispatchers.IO) {
            val res = api.downloadSubtitle(SubtitleDownloadRequest(encodedPath, hit))
            res.body() ?: error(serverMessage(res.errorBody()?.string()) ?: "Download failed (${res.code()})")
        }
        tracks = got.subtitles
        select(got.index)
        return got.index
    }

    companion object {
        private const val WINDOW_MS = 120_000L
        private const val AHEAD_MS = 40_000L

        /** For a stream URL from the server (…/api/media/stream?path=…): null for other sources. */
        fun forStream(streamUrl: String): ServerSubtitles? {
            if (!streamUrl.contains("/api/media/stream") || !streamUrl.contains("path=")) return null
            val encoded = URLDecoder.decode(streamUrl.substringAfter("path=").substringBefore("&"), "UTF-8")
            val api = Retrofit.Builder().baseUrl(streamUrl.substringBefore("/api") + "/")
                .client(WeirdStreamerApplication.getOkHttpClient())
                .addConverterFactory(GsonConverterFactory.create()).build()
                .create(ApiService::class.java)
            return ServerSubtitles(api, encoded)
        }

        private fun serverMessage(body: String?): String? =
            body?.let { runCatching { com.google.gson.JsonParser.parseString(it).asJsonObject.get("message")?.asString }.getOrNull() }
    }
}

/** Two-letter language codes to search OpenSubtitles in: this device's languages. */
fun deviceSubtitleLanguages(): List<String> {
    val locales = androidx.core.os.LocaleListCompat.getAdjustedDefault()
    return (0 until locales.size()).mapNotNull { locales[it]?.language?.takeIf { l -> l.length == 2 } }
        .distinct().ifEmpty { listOf("en") }
}

/** Reads WebVTT cues: their times, and their text with italics kept and other tags dropped. */
fun parseWebVtt(body: String): List<SubtitleCue> {
    val cues = ArrayList<SubtitleCue>()
    for (block in body.replace("\r\n", "\n").split(Regex("\n{2,}"))) {
        val lines = block.split("\n")
        val timing = lines.indexOfFirst { it.contains("-->") }
        if (timing < 0) continue
        val (a, b) = lines[timing].split("-->").map { it.trim().substringBefore(' ') }
        val start = vttTime(a) ?: continue
        val end = vttTime(b) ?: continue
        val text = lines.drop(timing + 1).joinToString("\n").trim()
        if (text.isNotEmpty()) cues.add(SubtitleCue(start, end, styledCue(text)))
    }
    return cues
}

private fun vttTime(s: String): Long? {
    val parts = s.split(":")
    val seconds = parts.last().toDoubleOrNull() ?: return null
    val minutes = parts.getOrNull(parts.size - 2)?.toLongOrNull() ?: 0
    val hours = if (parts.size >= 3) parts[0].toLongOrNull() ?: 0 else 0
    return ((hours * 3600 + minutes * 60) * 1000) + (seconds * 1000).toLong()
}

private val TAG = Regex("<(/?)([a-zA-Z]+)[^>]*>|\\{\\\\[^}]*\\}")

private fun styledCue(text: String): AnnotatedString = buildAnnotatedString {
    var italic = 0
    var at = 0
    fun put(s: String) {
        val plain = s.replace("&amp;", "&").replace("&lt;", "<").replace("&gt;", ">").replace("&nbsp;", " ")
        if (italic > 0) pushStyle(SpanStyle(fontStyle = FontStyle.Italic))
        append(plain)
        if (italic > 0) pop()
    }
    for (m in TAG.findAll(text)) {
        put(text.substring(at, m.range.first))
        at = m.range.last + 1
        if (m.groupValues[2].equals("i", ignoreCase = true)) {
            italic = if (m.groupValues[1] == "/") (italic - 1).coerceAtLeast(0) else italic + 1
        }
    }
    put(text.substring(at))
}

// ISO 639-2/B codes Java's Locale doesn't name, by their 639-2/T forms (which it does).
private val bibliographic = mapOf("fre" to "fra", "ger" to "deu", "chi" to "zho", "dut" to "nld", "cze" to "ces",
    "gre" to "ell", "per" to "fas", "rum" to "ron", "slo" to "slk", "ice" to "isl", "may" to "msa", "baq" to "eus",
    "arm" to "hye", "geo" to "kat", "mac" to "mkd", "alb" to "sqi", "bur" to "mya", "tib" to "bod", "wel" to "cym")

/** A language code (eng, fre, en…) as a name in this device's language: "English", "French". */
fun languageName(code: String?): String? {
    val c = code?.trim()?.lowercase()?.takeIf { it.isNotEmpty() && it != "und" } ?: return null
    val name = Locale.forLanguageTag(bibliographic[c] ?: c).getDisplayLanguage(Locale.getDefault())
    return name.ifBlank { c }.replaceFirstChar { it.titlecase(Locale.getDefault()) }
}

/**
 * Names the server's subtitle tracks like the player's own (subtitleLabels): language, title,
 * marks (Forced, SDH), where it's from (a file beside the video, a download), and Image for picture
 * tracks; a number for ones that still read the same.
 */
fun serverSubtitleLabels(tracks: List<SubtitleTrack>): List<String> {
    val base = tracks.mapIndexed { i, t ->
        val language = languageName(t.language)
        val title = t.title?.trim()?.takeIf { it.isNotEmpty() && !it.equals(language, ignoreCase = true) }?.take(40)
        listOfNotNull(
            language ?: "Subtitle ${i + 1}", title,
            if (t.forced) "Forced" else null,
            if (t.hearingImpaired && title?.contains("SDH", ignoreCase = true) != true) "SDH" else null,
            when (t.external) { "opensubtitles" -> "Downloaded"; "file" -> "File"; else -> null },
            if (!t.text) "Image" else null,
        ).joinToString(" · ")
    }
    val counts = base.groupingBy { it }.eachCount()
    val seen = HashMap<String, Int>()
    return base.map { label -> if ((counts[label] ?: 0) < 2) label else "$label (${seen.merge(label, 1, Int::plus)})" }
}

/**
 * Draws the selected text subtitle over the video, at [position] (the file's time, in ms: a
 * transcode's start offset included), loading the cues ahead as playback goes on; [bottomPadding]
 * keeps it above the player's controls while they show.
 */
@Composable
fun SubtitleOverlay(subtitles: ServerSubtitles?, position: () -> Long, fontSize: TextUnit, modifier: Modifier = Modifier,
                    bottomPadding: Dp = 48.dp) {
    if (subtitles == null) return
    val currentPosition by rememberUpdatedState(position)
    var text by remember { mutableStateOf<AnnotatedString?>(null) }
    val selected = subtitles.selected
    LaunchedEffect(subtitles, selected) {
        while (true) {
            subtitles.keepAhead(currentPosition())
            delay(1000)
        }
    }
    LaunchedEffect(subtitles, selected) {
        while (true) {
            text = subtitles.cueAt(currentPosition())
            delay(100)
        }
    }
    val shown = text ?: return
    // In this device's subtitle style (Settings → Subtitle style); fontSize is Normal's.
    val style = resolvedSubtitleStyle()
    Box(modifier.fillMaxSize().padding(bottom = bottomPadding, start = 48.dp, end = 48.dp), contentAlignment = Alignment.BottomCenter) {
        StyledSubtitleText(shown, style, fontSize)
    }
}

/** How far the subtitles are moved, in words: "In sync", "250 ms earlier", "1.5 s later". */
fun subtitleOffsetText(ms: Int): String {
    if (ms == 0) return "In sync"
    val a = kotlin.math.abs(ms)
    val amount = if (a >= 1000) "%.2f".format(Locale.US, a / 1000.0).trimEnd('0').trimEnd('.') + " s" else "$a ms"
    return amount + if (ms < 0) " earlier" else " later"
}

/** The steps the timing moves by. */
val SUBTITLE_STEPS = listOf(50, 250, 500, 1000)
fun subtitleStepText(ms: Int) = if (ms >= 1000) "${ms / 1000} s" else if (ms == 500) "0.5 s" else "$ms ms"
