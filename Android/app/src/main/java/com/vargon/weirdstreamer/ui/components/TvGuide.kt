package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.nativeKeyCode
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.ExperimentalTvMaterial3Api
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import coil.compose.AsyncImage
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.GuideChannel
import com.vargon.weirdstreamer.api.models.GuideListing
import kotlinx.coroutines.launch

/**
 * TV: the Guide. The channel plays top left (OK makes it full screen), the program's details beside
 * it, and the grid below: channels down, half hours across, moved through with the D-pad. A program
 * shows its details when focused; OK on one on now tunes to its channel. Full screen, Up and Down
 * change channel, OK shows the channel banner (the channel and its program; Up and Down then show the
 * other channels' without tuning, OK tunes to the one shown, Back hides it), the number keys tune,
 * Play/Pause mutes (live TV doesn't pause), Back returns.
 */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TvGuideScreen(viewModel: MediaViewModel) {
    val context = LocalContext.current
    val repo = viewModel.guideServer ?: run { viewModel.closeGuide(); return }
    val scope = rememberCoroutineScope()
    val model = remember { GuideModel(repo, viewModel.guideOpen?.ifEmpty { null }, context.applicationContext, scope) { viewModel.guideMarksWatched } }
    val player = remember { guideExoPlayer(context) }
    LaunchedEffect(Unit) { model.start() }
    DisposableEffect(Unit) {
        onDispose {
            model.stop()
            player.release()
        }
    }
    val playerFocus = remember { FocusRequester() }

    // Keys anywhere in the Guide: the number keys tune, Play/Pause and Mute mute.
    val keys = Modifier.onPreviewKeyEvent { e ->
        if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
        val digit = e.key.nativeKeyCode - android.view.KeyEvent.KEYCODE_0
        when {
            digit in 0..9 -> { model.typeDigit(digit); true }
            e.key == Key.MediaPlayPause || e.key == Key.MediaPause || e.key == Key.MediaPlay || e.key == Key.VolumeMute ->
                { model.muted = !model.muted; true }
            e.key == Key.ChannelUp -> { model.surf(1); true }
            e.key == Key.ChannelDown -> { model.surf(-1); true }
            else -> false
        }
    }

    if (model.fullscreen) {
        BackHandler { model.hideBanner(); model.fullscreen = false }
        BackHandler(enabled = model.banner != null) { model.hideBanner() } // Back first hides the banner
        val focus = remember { FocusRequester() }
        Box(Modifier.fillMaxSize()) {
            GuidePlayer(model, player, keys.fillMaxSize().focusRequester(focus).focusable()
                .onKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onKeyEvent false
                    val banner = model.banner
                    when (e.key) {
                        // with the banner up, the channels' programs; without it, the channels themselves
                        Key.DirectionUp -> { if (banner != null) model.moveBanner(-1) else model.surf(-1); true }
                        Key.DirectionDown -> { if (banner != null) model.moveBanner(1) else model.surf(1); true }
                        Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> {
                            when {
                                banner == null -> model.channel?.let { model.showBanner(it) }
                                banner != model.channel -> { model.hideBanner(); model.tune(banner) }
                                else -> model.hideBanner()
                            }
                            true
                        }
                        else -> false
                    }
                })
            model.banner?.let { n ->
                model.data?.channels?.firstOrNull { it.number == n }?.let { c ->
                    TvChannelBanner(model, c, Modifier.align(Alignment.BottomCenter))
                }
            }
        }
        LaunchedEffect(Unit) { focus.tryFocus() }
        return
    }
    BackHandler { viewModel.closeGuide() }

    val textColor = MaterialTheme.colorScheme.onSurface
    val logoColor = remember(textColor) { guideLogoColor(textColor) }
    Column(keys.fillMaxSize().background(MaterialTheme.colorScheme.background).padding(horizontal = 32.dp, vertical = 20.dp)) {
        Row(Modifier.fillMaxWidth().fillMaxHeight(0.44f), horizontalArrangement = Arrangement.spacedBy(24.dp)) {
            var playerFocused by remember { mutableStateOf(false) }
            GuidePlayer(model, player, Modifier.fillMaxHeight().aspectRatio(16f / 9f)
                .clip(RoundedCornerShape(8.dp))
                .focusRequester(playerFocus)
                .onFocusChanged { playerFocused = it.isFocused }
                .tvClickable(RoundedCornerShape(8.dp), 1f, 3.dp) { model.fullscreen = true })
            TvGuideDetails(model, Modifier.weight(1f).fillMaxHeight())
        }
        Spacer(Modifier.height(14.dp))
        TvGuideGrid(model, logoColor, playerFocus, Modifier.weight(1f))
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
/** Full screen's channel banner, along the bottom: the channel's logo, number and name, and what it airs now (its
 *  title, episode, times and description; the tuned channel's from what plays, another's from the grid), and the
 *  time at its lower right. */
@Composable
private fun TvChannelBanner(model: GuideModel, c: GuideChannel, modifier: Modifier) {
    val context = LocalContext.current
    val tuned = c.number == model.channel
    val listing = model.listingNow(c)
    val info = (if (tuned) model.now?.program else null) ?: listing?.info
    Row(modifier.fillMaxWidth().background(Brush.verticalGradient(0f to Color.Transparent, 0.25f to Color(0xE6000000), 1f to Color(0xF2000000)))
        .padding(start = 48.dp, end = 48.dp, top = 96.dp, bottom = 32.dp),
        horizontalArrangement = Arrangement.spacedBy(28.dp), verticalAlignment = Alignment.Bottom) {
        Column(Modifier.width(220.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            AsyncImage(model = model.logoUrl(c, 512), contentDescription = null, modifier = Modifier.fillMaxWidth())
            GuideText("${c.number}  ${c.name}", fontSize = 22.sp, color = Color(0xFF7CFC00), mono = true)
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            GuideText((info?.title ?: "Off the air") + (info?.year?.let { " ($it)" } ?: ""), fontSize = 30.sp, color = Color(0xFFFFD23F))
            info?.episodeLabel?.let { GuideText(it, fontSize = 20.sp) }
            if (listing != null) GuideText(guideFacts(context, c, listing), fontSize = 18.sp, color = Color(0xFFDDDDDD))
            (info?.episodeOverview?.takeIf { it.isNotBlank() } ?: info?.overview)?.let {
                androidx.compose.material3.Text(it, color = Color.White, fontSize = 18.sp, maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
            // the hints, and the time at the lower right: the banner grows upward with its program's text, so the
            // bottom line stays put as Up and Down go through the channels
            Row(verticalAlignment = Alignment.CenterVertically) {
                GuideText(if (tuned) "▲▼ other channels · Back to close" else "OK to watch · ▲▼ other channels · Back to close",
                    Modifier.weight(1f), fontSize = 15.sp, color = Color(0xFFAAAAAA))
                GuideText(guideTime(context, model.clock.takeIf { it > 0 } ?: model.serverNow()), fontSize = 26.sp, mono = true) // clock ticks each second
            }
        }
    }
}

@Composable
private fun TvGuideDetails(model: GuideModel, modifier: Modifier) {
    val context = LocalContext.current
    val (c, l) = model.selected ?: run {
        Box(modifier) {
            Text(model.error ?: if (model.data == null) "Loading the Guide…" else "", style = MaterialTheme.typography.titleMedium)
        }
        return
    }
    val info = l.info
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(16.dp)) {
        info?.poster?.let {
            AsyncImage(model = model.posterUrl(it), contentDescription = null,
                modifier = Modifier.fillMaxHeight().aspectRatio(2f / 3f).clip(RoundedCornerShape(6.dp)))
        }
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text((info?.title ?: "Off the air") + (info?.year?.let { " ($it)" } ?: ""),
                style = MaterialTheme.typography.headlineSmall, color = guideAccent(MaterialTheme.colorScheme.primary),
                maxLines = 2, overflow = TextOverflow.Ellipsis)
            info?.episodeLabel?.let { Text(it, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold) }
            Text(guideFacts(context, c, l), style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            ReportVoiceOver(model, c.number) { label, onClick -> TvSecondaryButton(onClick = onClick) { Text(label) } }
            (info?.episodeOverview?.takeIf { it.isNotBlank() } ?: info?.overview)?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

private val ChannelWidth = 210.dp
private val RowHeight = 56.dp

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
private fun TvGuideGrid(model: GuideModel, logoColor: String, playerFocus: FocusRequester, modifier: Modifier) {
    val context = LocalContext.current
    val data = model.data ?: return
    val channels = data.channels
    if (channels.isEmpty()) {
        Text("There are no channels yet: the Guide is planned a minute after the server starts.", modifier = modifier)
        return
    }
    // the tuned channel's program on now takes focus first
    val entry = remember { FocusRequester() }
    var focusedOnce by remember { mutableStateOf(false) }
    val listState = rememberLazyListState(initialFirstVisibleItemIndex =
        channels.indexOfFirst { it.number == model.channel }.coerceAtLeast(0))
    val across = rememberScrollState() // every row's half hours (and the times over them) scroll together
    BoxWithConstraints(modifier) {
    // 3½ hours fit across; the rest scrolls in as focus moves right, loaded as it gets near the end
    val halfHour = (maxWidth - ChannelWidth - 4.dp) / 7
    val width = halfHour * ((data.to - data.from).toFloat() / GuideModel.HALF_HOUR)
    fun x(ms: Long) = halfHour * ((ms - data.from).toFloat() / GuideModel.HALF_HOUR)
    Column(Modifier.fillMaxSize()) {
        // the half hours across the top (the day at midnight)
        Row(Modifier.fillMaxWidth().height(30.dp)) {
            // the day at the grid's left edge, as it scrolls
            val density = androidx.compose.ui.platform.LocalDensity.current
            val leftEdge = data.from + (across.value / with(density) { halfHour.toPx() } * GuideModel.HALF_HOUR).toLong()
            Text(DateFormatHelper.day(leftEdge), Modifier.width(ChannelWidth).padding(start = 8.dp),
                style = MaterialTheme.typography.labelLarge)
            Box(Modifier.weight(1f).fillMaxHeight().padding(start = 4.dp).horizontalScroll(across, enabled = false)) {
                Box(Modifier.width(width).fillMaxHeight()) {
                    var t = data.from
                    while (t < data.to) {
                        Text(guideTimeOrDay(context, t), Modifier.offset(x = x(t)).padding(start = 6.dp),
                            style = MaterialTheme.typography.labelLarge)
                        t += GuideModel.HALF_HOUR
                    }
                }
            }
        }
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
            items(channels, key = { it.number }) { c ->
                TvGuideRow(model, c, data.from, data.to, logoColor, across, width, ::x,
                    entry = if (c.number == model.channel) entry else null,
                    up = if (c == channels.first()) playerFocus else null)
            }
        }
    }
    // the time line: one line from the half hours down to the last channel, over the programs; it
    // moves with their scroll and hides behind the channels' column
    val now = model.clock.takeIf { it > 0 } ?: model.serverNow()
    val density = androidx.compose.ui.platform.LocalDensity.current
    val lineX = with(density) { (ChannelWidth + 4.dp + x(now)).roundToPx() } - across.value
    if (now in data.from..data.to && lineX >= with(density) { (ChannelWidth + 4.dp).roundToPx() })
        Box(Modifier.offset { androidx.compose.ui.unit.IntOffset(lineX, 0) }.width(2.dp)
            .height(minOf(maxHeight, 30.dp + RowHeight * channels.size))
            .background(guideAccent(MaterialTheme.colorScheme.primary)))
    }
    LaunchedEffect(model.channel, data) {
        if (!focusedOnce && model.channel != null) {
            kotlinx.coroutines.delay(100)
            focusedOnce = entry.tryFocus()
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
private fun TvGuideRow(model: GuideModel, c: GuideChannel, from: Long, to: Long, logoColor: String,
                       across: androidx.compose.foundation.ScrollState, width: androidx.compose.ui.unit.Dp,
                       x: (Long) -> androidx.compose.ui.unit.Dp, entry: FocusRequester?, up: FocusRequester?) {
    val tuned = model.channel == c.number
    Row(Modifier.fillMaxWidth().height(RowHeight).padding(vertical = 3.dp)) {
        // its logo, number and name
        Row(Modifier.width(ChannelWidth).fillMaxHeight()
            .background(if (tuned) MaterialTheme.colorScheme.primary.copy(alpha = 0.18f) else MaterialTheme.colorScheme.surface,
                RoundedCornerShape(6.dp))
            // OK on the tuned channel goes full screen (the picture is hard to reach with the arrows)
            .tvClickable(RoundedCornerShape(6.dp), 1f, 2.dp) { if (tuned) model.fullscreen = true else model.tune(c.number) }
            .padding(horizontal = 6.dp),
            verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AsyncImage(model = model.logoUrl(c, 256, logoColor), contentDescription = c.name,
                modifier = Modifier.size(width = 88.dp, height = 44.dp))
            Column {
                Text("${c.number}", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Black)
                Text(c.name, style = MaterialTheme.typography.labelMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
        Box(Modifier.weight(1f).fillMaxHeight().padding(start = 4.dp).horizontalScroll(across)) {
            Box(Modifier.width(width).fillMaxHeight()) {
                val now = model.serverNow()
                c.listings.orEmpty().forEach { l ->
                    val start = maxOf(l.start, from)
                    val end = minOf(l.end, to)
                    if (end <= start) return@forEach
                    val onNow = l.start <= now && now < l.end
                    var m = Modifier.offset(x = x(start)).width(x(end) - x(start) - 3.dp).fillMaxHeight()
                    if (onNow && entry != null) m = m.focusRequester(entry)
                    if (up != null) m = m.focusProperties { this.up = up }
                    Box(m
                        .background(if (onNow) MaterialTheme.colorScheme.surfaceVariant else MaterialTheme.colorScheme.surface,
                            RoundedCornerShape(6.dp))
                        .onFocusChanged {
                            if (it.isFocused) {
                                model.selected = c to l
                                // near the end of what's loaded: the next hours
                                if (l.end > to - 2 * GuideModel.HALF_HOUR) model.extendGrid()
                            }
                        }
                        .tvClickable(RoundedCornerShape(6.dp), 1f, 2.dp) {
                            if (onNow && model.channel != c.number) model.tune(c.number)
                            else if (onNow) model.fullscreen = true
                        }
                        .padding(horizontal = 8.dp, vertical = 4.dp),
                        contentAlignment = Alignment.CenterStart) {
                        Column {
                            Text(l.info?.title ?: "Off the air", style = MaterialTheme.typography.titleSmall,
                                fontWeight = FontWeight.Bold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            val second = l.info?.episodeLabel ?: l.info?.takeIf { it.kind == "movie" }?.year?.toString()
                            second?.let {
                                Text(it, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
            }
        }
    }
}

/** The grid's corner: the day ("Thu, Nov 26"). */
private object DateFormatHelper {
    fun day(ms: Long): String = java.text.SimpleDateFormat("EEE, MMM d", java.util.Locale.getDefault()).format(java.util.Date(ms))
}
