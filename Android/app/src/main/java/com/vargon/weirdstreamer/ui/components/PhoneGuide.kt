package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.gestures.detectVerticalDragGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.GuideChannel
import kotlinx.coroutines.launch

/**
 * Phone: the Guide. The channel plays at the top (a tap makes it full screen, where a swipe up or
 * down changes channel and a tap comes back), the program's details under it, and the grid below,
 * its half hours scrolling sideways past the channels. A tap on a program shows its details, and
 * on one on now tunes to its channel. Live TV doesn't pause: the speaker button mutes.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneGuideScreen(viewModel: MediaViewModel) {
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

    if (model.fullscreen) {
        BackHandler { model.fullscreen = false }
        // full screen is sideways, as the player's; upright again after
        val activity = context as? android.app.Activity
        DisposableEffect(Unit) {
            val before = activity?.requestedOrientation
            activity?.requestedOrientation = android.content.pm.ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            onDispose { if (before != null) activity.requestedOrientation = before }
        }
        val density = LocalDensity.current
        Box(Modifier.fillMaxSize().background(Color.Black)) {
            GuidePlayer(model, player, Modifier.fillMaxSize()
                .pointerInput(Unit) { detectTapGestures { model.fullscreen = false } }
                .pointerInput(Unit) {
                    var dragged = 0f
                    detectVerticalDragGestures(onDragStart = { dragged = 0f }, onDragEnd = {
                        val step = with(density) { 60.dp.toPx() }
                        if (dragged < -step) model.surf(1) else if (dragged > step) model.surf(-1)
                    }) { _, dy -> dragged += dy }
                })
            MuteButton(model, Modifier.align(Alignment.BottomEnd).padding(16.dp))
        }
        return
    }
    BackHandler { viewModel.closeGuide() }

    val textColor = MaterialTheme.colorScheme.onSurface
    val logoColor = remember(textColor) { guideLogoColor(textColor) }
    Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
        PhoneTopBarPanel {
            TopAppBar(
                title = { Text("Guide") },
                navigationIcon = {
                    IconButton(onClick = { viewModel.closeGuide() }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Color.Transparent),
            )
        }
        Box(Modifier.fillMaxWidth().aspectRatio(16f / 9f)) {
            GuidePlayer(model, player, Modifier.fillMaxSize().clickable { model.fullscreen = true })
            Row(Modifier.align(Alignment.BottomEnd).padding(8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                MuteButton(model)
                RoundButton({ model.fullscreen = true }) {
                    Icon(Icons.Filled.Fullscreen, contentDescription = "Full screen", tint = Color.White)
                }
            }
        }
        PhoneGuideDetails(model, Modifier.fillMaxWidth().heightIn(max = 170.dp).padding(12.dp))
        PhoneGuideGrid(model, logoColor, Modifier.weight(1f))
    }
}

@Composable
private fun MuteButton(model: GuideModel, modifier: Modifier = Modifier) {
    RoundButton({ model.muted = !model.muted }, modifier) {
        Icon(if (model.muted) Icons.AutoMirrored.Filled.VolumeOff else Icons.AutoMirrored.Filled.VolumeUp,
            contentDescription = if (model.muted) "Unmute" else "Mute", tint = Color.White)
    }
}

@Composable
private fun RoundButton(onClick: () -> Unit, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Box(modifier.size(40.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.6f)).clickable(onClick = onClick),
        contentAlignment = Alignment.Center) { content() }
}

@Composable
private fun PhoneGuideDetails(model: GuideModel, modifier: Modifier) {
    val context = LocalContext.current
    val (c, l) = model.selected ?: run {
        Text(model.error ?: if (model.data == null) "Loading the Guide…" else "", modifier)
        return
    }
    val info = l.info
    Row(modifier, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        info?.poster?.let {
            AsyncImage(model = model.posterUrl(it), contentDescription = null,
                modifier = Modifier.width(72.dp).aspectRatio(2f / 3f).clip(RoundedCornerShape(4.dp)))
        }
        Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text((info?.title ?: "Off the air") + (info?.year?.let { " ($it)" } ?: ""),
                style = MaterialTheme.typography.titleMedium, color = guideAccent(MaterialTheme.colorScheme.primary), fontWeight = FontWeight.Bold)
            info?.episodeLabel?.let { Text(it, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold) }
            Text(guideFacts(context, c, l), style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            (info?.episodeOverview?.takeIf { it.isNotBlank() } ?: info?.overview)?.let {
                Text(it, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

private val PhoneChannelWidth = 112.dp
private val PhoneHalfHour = 120.dp
private val PhoneRowHeight = 64.dp

@Composable
private fun PhoneGuideGrid(model: GuideModel, logoColor: String, modifier: Modifier) {
    val context = LocalContext.current
    val data = model.data ?: return
    val channels = data.channels
    if (channels.isEmpty()) {
        Text("There are no channels yet: the Guide is planned a minute after the server starts.", modifier.padding(12.dp))
        return
    }
    val halfHours = ((data.to - data.from).toFloat() / GuideModel.HALF_HOUR)
    val width = PhoneHalfHour * halfHours
    val across = rememberScrollState() // every row's half hours scroll together
    // scrolled near the end of what's loaded: the next hours
    LaunchedEffect(across) {
        androidx.compose.runtime.snapshotFlow { across.maxValue - across.value }.collect { left ->
            if (across.maxValue > 0 && left < 600) model.extendGrid()
        }
    }
    val listState = rememberLazyListState(initialFirstVisibleItemIndex =
        channels.indexOfFirst { it.number == model.channel }.coerceAtLeast(0))
    fun x(ms: Long) = PhoneHalfHour * ((ms - data.from).toFloat() / GuideModel.HALF_HOUR)
    androidx.compose.foundation.layout.BoxWithConstraints(modifier) {
    Column(Modifier.fillMaxSize()) {
        Row(Modifier.fillMaxWidth().height(28.dp).background(MaterialTheme.colorScheme.surface)) {
            // the day at the grid's left edge, as it scrolls
            val leftEdge = data.from + (across.value / with(LocalDensity.current) { PhoneHalfHour.toPx() } * GuideModel.HALF_HOUR).toLong()
            Text(java.text.SimpleDateFormat("EEE, MMM d", java.util.Locale.getDefault()).format(java.util.Date(leftEdge)),
                Modifier.width(PhoneChannelWidth).padding(start = 8.dp, top = 4.dp), style = MaterialTheme.typography.labelMedium)
            Box(Modifier.weight(1f).fillMaxHeight().horizontalScroll(across)) {
                Box(Modifier.width(width).fillMaxHeight()) {
                    var t = data.from
                    while (t < data.to) {
                        Text(guideTimeOrDay(context, t), Modifier.offset(x = x(t)).padding(start = 6.dp, top = 4.dp),
                            style = MaterialTheme.typography.labelMedium)
                        t += GuideModel.HALF_HOUR
                    }
                }
            }
        }
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
            items(channels, key = { it.number }) { c ->
                PhoneGuideRow(model, c, data.from, data.to, logoColor, across, width, ::x)
            }
        }
    }
    // the time line: one line from the half hours down to the last channel, over the programs; it
    // moves with their sideways scroll and hides behind the channels' column
    val now = model.clock.takeIf { it > 0 } ?: model.serverNow()
    if (now in data.from..data.to) {
        val density = LocalDensity.current
        val lineX = with(density) { (PhoneChannelWidth + x(now)).roundToPx() } - across.value
        if (lineX >= with(density) { PhoneChannelWidth.roundToPx() })
            Box(Modifier.offset { androidx.compose.ui.unit.IntOffset(lineX, 0) }.width(2.dp)
                .height(minOf(maxHeight, 28.dp + PhoneRowHeight * channels.size))
                .background(guideAccent(MaterialTheme.colorScheme.primary)))
    }
    }
}

@Composable
private fun PhoneGuideRow(model: GuideModel, c: GuideChannel, from: Long, to: Long, logoColor: String,
                          across: androidx.compose.foundation.ScrollState, width: androidx.compose.ui.unit.Dp,
                          x: (Long) -> androidx.compose.ui.unit.Dp) {
    val tuned = model.channel == c.number
    Row(Modifier.fillMaxWidth().height(PhoneRowHeight).padding(vertical = 2.dp)) {
        Column(Modifier.width(PhoneChannelWidth).fillMaxHeight()
            .background(if (tuned) MaterialTheme.colorScheme.primary.copy(alpha = 0.18f) else MaterialTheme.colorScheme.surface)
            .clickable { model.tune(c.number) }
            .padding(horizontal = 4.dp, vertical = 2.dp),
            verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
            // the logo takes what the name leaves, so a large font size never pushes the name out
            AsyncImage(model = model.logoUrl(c, 256, logoColor), contentDescription = c.name,
                modifier = Modifier.weight(1f).width(80.dp))
            Text("${c.number} ${c.name}", style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Box(Modifier.weight(1f).fillMaxHeight().horizontalScroll(across)) {
            Box(Modifier.width(width).fillMaxHeight()) {
                val now = model.serverNow()
                c.listings.orEmpty().forEach { l ->
                    val start = maxOf(l.start, from)
                    val end = minOf(l.end, to)
                    if (end <= start) return@forEach
                    val onNow = l.start <= now && now < l.end
                    val picked = model.selected?.let { it.first.number == c.number && it.second.start == l.start } == true
                    Box(Modifier.offset(x = x(start)).width(x(end) - x(start) - 2.dp).fillMaxHeight()
                        .padding(start = 2.dp)
                        .background(when {
                            picked -> MaterialTheme.colorScheme.primary.copy(alpha = 0.35f)
                            onNow -> MaterialTheme.colorScheme.surfaceVariant
                            else -> MaterialTheme.colorScheme.surface
                        }, RoundedCornerShape(4.dp))
                        .clickable {
                            model.selected = c to l
                            if (onNow && model.channel != c.number) model.tune(c.number)
                        }
                        .padding(horizontal = 6.dp, vertical = 2.dp),
                        contentAlignment = Alignment.CenterStart) {
                        Column {
                            Text(l.info?.title ?: "Off the air", style = MaterialTheme.typography.bodySmall,
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
