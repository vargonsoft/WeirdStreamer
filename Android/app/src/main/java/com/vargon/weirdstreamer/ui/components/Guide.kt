package com.vargon.weirdstreamer.ui.components

import android.content.Context
import android.view.LayoutInflater
import androidx.annotation.OptIn
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shadow
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import coil.compose.AsyncImage
import com.vargon.weirdstreamer.R
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.models.GuideChannel
import com.vargon.weirdstreamer.api.models.GuideListing
import com.vargon.weirdstreamer.api.models.GuideListings
import com.vargon.weirdstreamer.api.models.GuideNow
import com.vargon.weirdstreamer.downloads.OfflinePlayback
import com.vargon.weirdstreamer.repository.ServerRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.text.DateFormat
import java.util.Date
import java.util.Locale

/*
 * The Guide (docs/GUIDE.md): live channels the server plans, the TV's and the phone's alike. A
 * channel plays only what's on now, from where it is: no seeking, and pausing mutes instead. The
 * TV (TvGuide.kt) and the phone (PhoneGuide.kt) draw their screens on GuideModel and GuidePlayer.
 */

/** What the player plays: a file from [seekMs] (the original) or a stream from where the channel is. */
data class GuideStream(val url: String, val seekMs: Long, val key: String)

/**
 * The Guide's state: the grid, the channel tuned, what it airs and how it plays. [library] is the
 * library whose own Guide it is, null the home screen's.
 */
class GuideModel(
    private val repo: ServerRepository,
    val library: String?,
    private val context: Context,
    private val scope: CoroutineScope,
    /** Settings' "Mark Guide shows watched": a show that plays to its end on the tuned channel is marked watched. */
    private val marksWatched: () -> Boolean = { false },
) {
    var data by mutableStateOf<GuideListings?>(null)
        private set
    var error by mutableStateOf<String?>(null)
        private set
    var channel by mutableStateOf<Int?>(null)
        private set
    var now by mutableStateOf<GuideNow?>(null)
        private set
    /** The program whose details show: the one chosen in the grid, else the channel's. */
    var selected by mutableStateOf<Pair<GuideChannel, GuideListing>?>(null)
    var stream by mutableStateOf<GuideStream?>(null)
        private set
    var muted by mutableStateOf(false)
    var fullscreen by mutableStateOf(false)
    /** Shown for a few seconds after tuning: the channel's number (or the digits being typed). */
    var osd by mutableStateOf<String?>(null)
    var osdLogo by mutableStateOf<GuideChannel?>(null)
    /** Bumped each second, for the time line. */
    var clock by mutableStateOf(0L)
        private set
    /** The Weather channel's forecast, while it's tuned. */
    var weather by mutableStateOf<com.vargon.weirdstreamer.api.models.GuideWeather?>(null)
        private set
    private var weatherAt = 0L
    /** The music under the Weather channel and the Guide channel's cards (its own player). */
    val music: ExoPlayer by lazy { guideExoPlayer(context).also { it.volume = if (muted) 0f else 1f } }
    private var musicUsed = false

    private var skew = 0L // the server's clock less this one's
    private var nowJob: Job? = null
    private var osdJob: Job? = null
    private var digits = ""
    private var digitJob: Job? = null
    private var fellBack = false
    private val prefs = context.getSharedPreferences("guide", Context.MODE_PRIVATE)
    private val lastKey = "channel:" + repo.address + ":" + (library ?: "")

    fun serverNow(): Long = System.currentTimeMillis() + skew

    /** Loads the grid and tunes to the last channel watched here (else the first); keeps the grid current. */
    fun start() {
        scope.launch {
            loadGrid()
            val channels = data?.channels.orEmpty()
            if (channels.isNotEmpty() && channel == null) {
                val last = prefs.getInt(lastKey, -1)
                tune((channels.firstOrNull { it.number == last } ?: channels.first()).number)
            }
            while (isActive) {
                delay(1000)
                clock = serverNow()
                val from = data?.from ?: continue
                if (serverNow() >= from + HALF_HOUR) loadGrid() // keeps the hours loaded
            }
        }
    }

    /** Loads the grid from the current half hour, keeping as many hours as were loaded (at least 3½). */
    suspend fun loadGrid() {
        val from = serverNow() / HALF_HOUR * HALF_HOUR
        val hours = maxOf(3.5, ((data?.to ?: 0L) - from) / 3_600_000.0)
        repo.guideListings(library, hours).onSuccess {
            skew = it.now - System.currentTimeMillis()
            data = it
            error = null
        }.onFailure { if (data == null) error = it.message }
    }

    private var extending = false

    /** Loads the next 3 hours onto the grid (scrolled near its end), up to as far as the plan reaches. */
    fun extendGrid() {
        val d = data ?: return
        if (extending || d.until in 1..d.to) return
        extending = true
        scope.launch {
            repo.guideListings(library, 3.0, d.to).onSuccess { more ->
                val current = data ?: return@onSuccess
                if (current.to != d.to) return@onSuccess // reloaded meanwhile
                val byNumber = more.channels.associateBy { it.number }
                data = current.copy(to = more.to, until = more.until, channels = current.channels.map { c ->
                    val add = byNumber[c.number]?.listings.orEmpty()
                    val have = c.listings.orEmpty()
                    // a program across the boundary comes in both: kept once
                    c.copy(listings = have + add.filter { a -> have.none { it.start == a.start } })
                })
            }
            extending = false
        }
    }

    fun logoUrl(c: GuideChannel, width: Int, color: String = "ffffff") = repo.guideLogoUrl(c, width, color)

    fun posterUrl(poster: String?) = repo.absoluteUrl(poster)

    /** Tunes to a channel: plays what it airs now, from where it is. */
    fun tune(number: Int) {
        val changed = channel != number
        channel = number
        prefs.edit().putInt(lastKey, number).apply()
        if (changed) showOsd("$number", data?.channels?.firstOrNull { it.number == number })
        nowJob?.cancel()
        nowJob = scope.launch { follow(number) }
    }

    /** The next or previous channel in the grid's order. */
    fun surf(step: Int) {
        val channels = data?.channels.orEmpty()
        if (channels.isEmpty()) return
        val i = channels.indexOfFirst { it.number == channel }
        tune(channels[((i + step) % channels.size + channels.size) % channels.size].number)
    }

    /** A digit of a channel's number typed: tunes once the typing stops. */
    fun typeDigit(d: Int) {
        digits = (digits + d).takeLast(3)
        osd = digits
        osdLogo = null
        digitJob?.cancel()
        digitJob = scope.launch {
            delay(1500)
            val n = digits.toIntOrNull()
            digits = ""
            if (n != null && data?.channels.orEmpty().any { it.number == n }) tune(n) else osd = null
        }
    }

    private fun showOsd(text: String, c: GuideChannel?) {
        osd = text
        osdLogo = c
        osdJob?.cancel()
        osdJob = scope.launch {
            delay(4000)
            osd = null
            osdLogo = null
        }
    }

    // Follows the channel: what it airs, and when that ends, what's next.
    private suspend fun follow(number: Int) {
        while (scope.isActive && channel == number) {
            val r = repo.guideNow(number, library)
            val n = r.getOrNull()
            if (n == null) {
                now = null
                stream = null
                error = r.exceptionOrNull()?.message
                delay(30_000)
                continue
            }
            skew = n.serverTime - System.currentTimeMillis()
            // A show that just played to its end on this channel, with the setting on: marked watched
            val ended = now
            if (ended != null && ended.kind == "program" && ended.channel.number == number && ended.channel.kind.isEmpty() &&
                n.start >= ended.end && marksWatched()) {
                scope.launch { repo.guideWatched(number, library, ended.start) }
            }
            now = n
            fellBack = false
            if (n.channel.kind == "weather") scope.launch { loadWeather() }
            // the details follow the channel unless something else on it was picked
            val c = data?.channels?.firstOrNull { it.number == number }
            val l = c?.listings?.firstOrNull { it.start <= n.start && n.start < it.end }
            if (c != null && l != null && (selected?.first?.number != number || (selected?.second?.end ?: 0) <= serverNow())) {
                selected = c to l
            }
            // The Weather channel and the Guide channel's cards are drawn, with music; the rest plays
            if (n.channel.kind == "weather" || n.channel.kind == "guide" && n.kind == "promo") {
                stream = null
                playMusic(n)
            } else {
                stopMusic()
                stream = n.path?.takeIf { n.kind != "bumper" }?.let { streamFor(n, it, transcode = false) }
            }
            delay((n.end - serverNow()).coerceAtLeast(0) + 50)
        }
    }

    /** How the item plays: the file itself, seeked, where this device plays it; else the server's
     *  stream from where the channel is (the video copied when it can be, the audio as it plays). */
    private suspend fun streamFor(n: GuideNow, path: String, transcode: Boolean): GuideStream {
        val key = "${n.channel.number}|${n.start}|$transcode"
        // where the server said it is in the file, plus the time since
        val offset = ((n.offset * 1000).toLong() + serverNow() - n.serverTime).coerceAtLeast(0)
        val codecs = if (transcode) null else repo.codecs(path)
        if (codecs != null && OfflinePlayback.of(context, codecs, 0, 0).plays) {
            return GuideStream(repo.streamUrlFor(path), offset, key)
        }
        val video = codecs?.video
        val copy = !transcode && video?.codec == "h264" && video.pixFmt in listOf("yuv420p", "yuvj420p")
        val track = codecs?.audio?.let { list -> list.firstOrNull { it.isDefault } ?: list.firstOrNull() }
        val audio = if (transcode) AudioStep.STEREO else audioLadder(track, AudioOutput.current(context), small = false).first()
        val start = offset
        val query = buildString {
            append(if (copy) "&transcode=audio" else "&transcode=1")
            append(audio.query)
            if (!copy) VideoDecoders.maxHeight?.let { append("&resolution=$it") }
            if (start > 1000) append("&start=" + String.format(Locale.US, "%.2f", start / 1000.0))
        }
        return GuideStream(repo.streamUrlFor(path, query), 0, key)
    }

    /** The player failed on what it was given: a full transcode from where the channel is, once. */
    fun playerFailed() {
        val n = now ?: return
        val path = n.path ?: return
        if (fellBack) return
        fellBack = true
        scope.launch { stream = streamFor(n, path, transcode = true) }
    }

    /** Plays the item's music from where the clock says (going on if it's already that track). */
    private fun playMusic(n: GuideNow) {
        val m = n.music ?: return stopMusic()
        val url = repo.absoluteUrl(m.url) ?: return
        val at = ((m.offset * 1000).toLong() + serverNow() - n.serverTime).coerceAtLeast(0)
        musicUsed = true
        music.volume = if (muted) 0f else 1f
        if (music.currentMediaItem?.mediaId == url && music.playbackState != Player.STATE_IDLE) {
            if (kotlin.math.abs(music.currentPosition - at) > 3000) music.seekTo(at)
        } else {
            music.setMediaItem(MediaItem.Builder().setUri(url).setMediaId(url).build(), at)
            music.prepare()
        }
        music.playWhenReady = true
    }

    private fun stopMusic() {
        if (!musicUsed) return
        music.stop()
        music.clearMediaItems()
    }

    /** The music's credit line while it plays ("♪ …"), else null. */
    fun musicCredit(): String? = now?.music?.let { m -> (m.credit.ifBlank { m.title }).takeIf { it.isNotBlank() }?.let { "♪ $it" } }

    /** Reads the forecast (the server keeps it 10 minutes; asked again after 5). */
    suspend fun loadWeather() {
        if (weather != null && System.currentTimeMillis() - weatherAt < 5 * 60_000) return
        repo.guideWeather().onSuccess {
            weather = it
            weatherAt = System.currentTimeMillis()
        }.onFailure { if (weather == null) weather = com.vargon.weirdstreamer.api.models.GuideWeather(error = it.message) }
    }

    fun weatherIconUrl(name: String) = repo.weatherIconUrl(name)

    fun weatherRadarUrl() = repo.weatherRadarUrl(serverNow())

    fun stop() {
        nowJob?.cancel()
        stream = null
        now = null
        if (musicUsed) music.release()
    }

    companion object {
        const val HALF_HOUR = 30 * 60 * 1000L
    }
}

/** A time of day as the device shows them ("8:30 PM"). */
fun guideTime(context: Context, ms: Long): String = DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(ms))

/** A day other than today as "Fri, Nov 27 " (before a time), else "". */
fun guideDay(ms: Long): String {
    val day = java.text.SimpleDateFormat("yyyyMMdd", Locale.getDefault())
    if (day.format(Date(ms)) == day.format(Date())) return ""
    return java.text.SimpleDateFormat("EEE, MMM d", Locale.getDefault()).format(Date(ms)) + " "
}

/** A half hour's label in the grid: its time, or the day at midnight ("Fri, Nov 27"). */
fun guideTimeOrDay(context: Context, ms: Long): String {
    val c = java.util.Calendar.getInstance().apply { timeInMillis = ms }
    return if (c.get(java.util.Calendar.HOUR_OF_DAY) == 0 && c.get(java.util.Calendar.MINUTE) == 0)
        java.text.SimpleDateFormat("EEE, MMM d", Locale.getDefault()).format(Date(ms)) else guideTime(context, ms)
}

/**
 * The Guide's player: what the channel airs, or the card between programs. One [player] for the
 * small window and full screen alike, so switching keeps playing.
 */
@OptIn(UnstableApi::class)
@Composable
fun GuidePlayer(model: GuideModel, player: ExoPlayer, modifier: Modifier = Modifier) {
    val stream = model.stream
    LaunchedEffect(stream?.key, stream?.url) {
        if (stream == null) {
            player.stop()
            player.clearMediaItems()
            return@LaunchedEffect
        }
        // Going full screen (or back) places this anew with the same player: what it already plays goes on.
        if (player.currentMediaItem?.mediaId == stream.key && player.playbackState != Player.STATE_IDLE) return@LaunchedEffect
        player.setMediaItem(MediaItem.Builder().setUri(stream.url).setMediaId(stream.key).build(), stream.seekMs)
        player.prepare()
        player.playWhenReady = true
    }
    LaunchedEffect(model.muted) {
        player.volume = if (model.muted) 0f else 1f
        model.music.volume = if (model.muted) 0f else 1f
    }
    DisposableEffect(player) {
        val listener = object : Player.Listener {
            override fun onPlayerError(error: PlaybackException) = model.playerFailed()
            // Live TV doesn't pause: a pause from outside (a headset's button) mutes instead.
            override fun onPlayWhenReadyChanged(playWhenReady: Boolean, reason: Int) {
                if (!playWhenReady && model.stream != null && reason == Player.PLAY_WHEN_READY_CHANGE_REASON_USER_REQUEST) {
                    model.muted = true
                    player.playWhenReady = true
                }
            }
        }
        player.addListener(listener)
        onDispose { player.removeListener(listener) }
    }
    val now = model.now
    // A channel of its own: the Guide channel's picture on top with every channel's listings below;
    // the Weather channel's pages over it all
    val kind = now?.channel?.kind ?: model.data?.channels?.firstOrNull { it.number == model.channel }?.kind ?: ""
    Box(modifier.background(Color.Black), contentAlignment = Alignment.Center) {
        Box(if (kind == "guide") Modifier.fillMaxWidth().fillMaxHeight(0.58f).align(Alignment.TopCenter) else Modifier.fillMaxSize()) {
            AndroidView(
                factory = { ctx ->
                    (LayoutInflater.from(ctx).inflate(R.layout.texture_video_view, null) as PlayerView).apply {
                        useController = false
                        this.player = player
                    }
                },
                update = { it.player = player },
                // the view going (to full screen or back) lets go of the picture, so the new one gets it
                onRelease = { it.player = null },
                modifier = Modifier.fillMaxSize()
            )
            when {
                kind == "weather" -> {}
                kind == "guide" && stream == null -> GuidePromo(model, now)
                stream == null -> GuideCard(model, now)
                kind == "guide" && now?.kind == "preview" -> PreviewCaption(now, Modifier.align(Alignment.BottomStart))
            }
        }
        if (kind == "guide") GuideListings(model, Modifier.fillMaxWidth().fillMaxHeight(0.42f).align(Alignment.BottomCenter))
        if (kind == "weather") WeatherScreen(model, Modifier.fillMaxSize())
        // the channel's number and logo for a few seconds after tuning, as a cable box shows them
        model.osd?.let {
            GuideText(it, Modifier.align(Alignment.TopEnd).padding(12.dp), 28.sp, Color(0xFF7CFC00), mono = true)
        }
        model.osdLogo?.let { c ->
            AsyncImage(model = model.logoUrl(c, 512), contentDescription = null,
                modifier = Modifier.align(Alignment.TopStart).padding(12.dp).fillMaxWidth(0.22f))
        }
    }
}

/** The card between programs: "We'll be right back" or "Up next", with the channel's logo. */
@Composable
private fun GuideCard(model: GuideModel, now: GuideNow?) {
    val context = LocalContext.current
    val color = runCatching { Color(android.graphics.Color.parseColor(now?.channel?.color ?: "#6A3A62")) }
        .getOrDefault(Color(0xFF6A3A62))
    BoxWithConstraints(
        Modifier.fillMaxSize().background(Brush.radialGradient(listOf(color, Color(0xFF0B0B1E)))),
        contentAlignment = Alignment.Center
    ) {
        val w = maxWidth.value
        val next = now?.next
        Row(Modifier.fillMaxSize().padding((w * 0.05f).dp), verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy((w * 0.04f).dp)) {
            next?.poster?.let {
                AsyncImage(model = model.posterUrl(it), contentDescription = null, contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxHeight(0.8f).width((w * 0.22f).dp))
            }
            Column(verticalArrangement = Arrangement.spacedBy((w * 0.01f).dp)) {
                now?.channel?.let { c ->
                    AsyncImage(model = model.logoUrl(c, 512), contentDescription = null,
                        modifier = Modifier.width((w * 0.28f).dp))
                }
                when {
                    now == null -> GuideText("Please stand by", fontSize = (w * 0.05f).sp)
                    next == null -> GuideText("Programming will resume shortly", fontSize = (w * 0.04f).sp)
                    else -> {
                        val long = now.nextStart - model.serverNow() > 90_000
                        GuideText(if (long) "WE'LL BE RIGHT BACK" else "UP NEXT ON ${now.channel.number} ${now.channel.name.uppercase()}",
                            fontSize = (w * 0.032f).sp, color = Color(0xFFFFD23F))
                        GuideText(next.title, fontSize = (w * 0.055f).sp)
                        GuideText(listOfNotNull(next.episodeLabel, "at " + guideTime(context, now.nextStart)).joinToString(" · "),
                            fontSize = (w * 0.028f).sp)
                    }
                }
            }
        }
    }
}

/** Text over the picture: white (or [color]) with a dark shadow, for any background. */
@Composable
fun GuideText(text: String, modifier: Modifier = Modifier, fontSize: TextUnit = 16.sp, color: Color = Color.White, mono: Boolean = false) {
    androidx.compose.material3.Text(
        text, modifier = modifier, maxLines = 2, overflow = TextOverflow.Ellipsis,
        style = TextStyle(color = color, fontSize = fontSize, fontWeight = FontWeight.Bold,
            fontFamily = if (mono) FontFamily.Monospace else FontFamily.Default,
            shadow = Shadow(Color.Black.copy(alpha = 0.7f), androidx.compose.ui.geometry.Offset(3f, 3f), 2f))
    )
}

/** The Guide's ExoPlayer, signed in to the server. */
@OptIn(UnstableApi::class)
fun guideExoPlayer(context: Context): ExoPlayer =
    ExoPlayer.Builder(context)
        .setMediaSourceFactory(
            androidx.media3.exoplayer.source.DefaultMediaSourceFactory(context)
                .setDataSourceFactory(OkHttpDataSource.Factory(WeirdStreamerApplication.getOkHttpClient()).setUserAgent("WeirdStreamer/1.0"))
        )
        .build()

/** A program's time slot, its rating and genres, for the details ("8:00 – 8:30 PM · TV-PG · Comedy"). */
fun guideFacts(context: Context, c: GuideChannel, l: GuideListing): String = listOfNotNull(
    guideDay(l.start) + guideTime(context, l.start) + " – " + guideTime(context, l.end),
    "Channel ${c.number} · ${c.name}",
    l.info?.rating?.takeIf { it.isNotBlank() },
    l.info?.genres?.takeIf { it.isNotEmpty() }?.joinToString(", "),
).joinToString(" · ")

/** The Guide's accent (titles, the time line): the theme's heading color, else [fallback]. */
@Composable
fun guideAccent(fallback: Color): Color =
    com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme.spec.headingColor.let { if (it == Color.Unspecified) fallback else it }

/** A channel's logo color for the theme's text: "rrggbb". */
fun guideLogoColor(c: Color): String = String.format(Locale.US, "%02x%02x%02x",
    (c.red * 255).toInt(), (c.green * 255).toInt(), (c.blue * 255).toInt())
