package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
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
 * change channel, the number keys tune, Play/Pause mutes (live TV doesn't pause), Back returns.
 */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TvGuideScreen(viewModel: MediaViewModel) {
    val context = LocalContext.current
    val repo = viewModel.guideServer ?: run { viewModel.closeGuide(); return }
    val scope = rememberCoroutineScope()
    val model = remember { GuideModel(repo, viewModel.guideOpen?.ifEmpty { null }, context.applicationContext, scope) }
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
        BackHandler { model.fullscreen = false }
        val focus = remember { FocusRequester() }
        GuidePlayer(model, player, keys.fillMaxSize().focusRequester(focus).focusable()
            .onKeyEvent { e ->
                if (e.type != KeyEventType.KeyDown) return@onKeyEvent false
                when (e.key) {
                    Key.DirectionUp -> { model.surf(-1); true }
                    Key.DirectionDown -> { model.surf(1); true }
                    Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> {
                        model.channel?.let { n -> model.osd = "$n"; model.osdLogo = model.data?.channels?.firstOrNull { it.number == n } }
                        true
                    }
                    else -> false
                }
            })
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
            // Mark watched above the description, which takes what room is left
            val now = model.serverNow()
            if (info != null && model.channel == c.number && l.start <= now && now < l.end) {
                var label by remember(l.start) { mutableStateOf("Mark watched") }
                val scope = rememberCoroutineScope()
                TvSecondaryButton(onClick = {
                    scope.launch { label = if (model.markWatched().isSuccess) "Watched" else "Couldn't mark it watched" }
                }) { Text(label) }
            }
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
    val span = (data.to - data.from).toFloat()
    // the tuned channel's program on now takes focus first
    val entry = remember { FocusRequester() }
    var focusedOnce by remember { mutableStateOf(false) }
    val listState = rememberLazyListState(initialFirstVisibleItemIndex =
        channels.indexOfFirst { it.number == model.channel }.coerceAtLeast(0))
    BoxWithConstraints(modifier) {
    Column(Modifier.fillMaxSize()) {
        // the half hours across the top
        Row(Modifier.fillMaxWidth().height(30.dp)) {
            Text(DateFormatHelper.day(data.from), Modifier.width(ChannelWidth).padding(start = 8.dp),
                style = MaterialTheme.typography.labelLarge)
            BoxWithConstraints(Modifier.weight(1f).fillMaxHeight().padding(start = 4.dp)) { // as the rows' programs
                val w = maxWidth
                var t = data.from
                while (t < data.to) {
                    Text(guideTime(context, t), Modifier.offset(x = w * ((t - data.from) / span)).padding(start = 6.dp),
                        style = MaterialTheme.typography.labelLarge)
                    t += GuideModel.HALF_HOUR
                }
            }
        }
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
            items(channels, key = { it.number }) { c ->
                TvGuideRow(model, c, data.from, data.to, span, logoColor,
                    entry = if (c.number == model.channel) entry else null,
                    up = if (c == channels.first()) playerFocus else null)
            }
        }
    }
    // the time line: one line from the half hours down to the last channel, over the programs
    val programs = maxWidth - ChannelWidth - 4.dp
    val f = ((model.clock.takeIf { it > 0 } ?: model.serverNow()) - data.from) / span
    if (f in 0f..1f) Box(Modifier.offset(x = ChannelWidth + 4.dp + programs * f).width(2.dp)
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
private fun TvGuideRow(model: GuideModel, c: GuideChannel, from: Long, to: Long, span: Float, logoColor: String,
                       entry: FocusRequester?, up: FocusRequester?) {
    val tuned = model.channel == c.number
    Row(Modifier.fillMaxWidth().height(RowHeight).padding(vertical = 3.dp)) {
        // its logo, number and name
        Row(Modifier.width(ChannelWidth).fillMaxHeight()
            .background(if (tuned) MaterialTheme.colorScheme.primary.copy(alpha = 0.18f) else MaterialTheme.colorScheme.surface,
                RoundedCornerShape(6.dp))
            .tvClickable(RoundedCornerShape(6.dp), 1f, 2.dp) { model.tune(c.number) }
            .padding(horizontal = 6.dp),
            verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            AsyncImage(model = model.logoUrl(c, 256, logoColor), contentDescription = c.name,
                modifier = Modifier.size(width = 88.dp, height = 44.dp))
            Column {
                Text("${c.number}", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Black)
                Text(c.name, style = MaterialTheme.typography.labelMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
        BoxWithConstraints(Modifier.weight(1f).fillMaxHeight().padding(start = 4.dp)) {
            val w = maxWidth
            val now = model.serverNow()
            c.listings.orEmpty().forEach { l ->
                val start = maxOf(l.start, from)
                val end = minOf(l.end, to)
                if (end <= start) return@forEach
                val onNow = l.start <= now && now < l.end
                var m = Modifier.offset(x = w * ((start - from) / span)).width(w * ((end - start) / span) - 3.dp).fillMaxHeight()
                if (onNow && entry != null) m = m.focusRequester(entry)
                if (up != null) m = m.focusProperties { this.up = up }
                Box(m
                    .background(if (onNow) MaterialTheme.colorScheme.surfaceVariant else MaterialTheme.colorScheme.surface,
                        RoundedCornerShape(6.dp))
                    .onFocusChanged { if (it.isFocused) model.selected = c to l }
                    .tvClickable(RoundedCornerShape(6.dp), 1f, 2.dp) {
                        if (onNow && model.channel != c.number) model.tune(c.number)
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

/** The grid's corner: the day ("Thu, Nov 26"). */
private object DateFormatHelper {
    fun day(ms: Long): String = java.text.SimpleDateFormat("EEE, MMM d", java.util.Locale.getDefault()).format(java.util.Date(ms))
}
