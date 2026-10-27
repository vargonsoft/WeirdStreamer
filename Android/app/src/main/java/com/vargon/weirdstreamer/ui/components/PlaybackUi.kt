package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.ContinueItem
import com.vargon.weirdstreamer.api.models.Playback
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import java.util.Locale

// Playback progress, shared by the phone and TV screens: a progress line along the bottom of a
// thumbnail or poster, a check once watched, and the Continue Watching row.

/** A progress line at the bottom of the box and a check in its corner once watched. */
@Composable
fun BoxScope.WatchProgressOverlay(
    fraction: Float,
    watched: Boolean,
    barHeight: Dp = 4.dp,
    badgeSize: Dp = 22.dp,
    accent: Color = WeirdStreamerTheme.spec.progressColor.takeOrElse { MaterialTheme.colorScheme.primary },
    onAccent: Color = WeirdStreamerTheme.spec.progressLabel.takeOrElse { MaterialTheme.colorScheme.onPrimary }
) {
    if (fraction > 0f) {
        Box(
            Modifier.align(Alignment.BottomStart).fillMaxWidth().height(barHeight)
                .background(Color.Black.copy(alpha = 0.55f))
        ) {
            Box(Modifier.fillMaxHeight().fillMaxWidth(fraction.coerceIn(0f, 1f)).background(accent))
        }
    }
    if (watched) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier.align(Alignment.BottomEnd).padding(end = 6.dp, bottom = barHeight + 6.dp)
                .size(badgeSize).clip(CircleShape).background(accent)
        ) {
            Icon(Icons.Default.Check, contentDescription = "Watched", tint = onAccent,
                modifier = Modifier.size(badgeSize * 0.7f))
        }
    }
}

/** [WatchProgressOverlay] for one video's [playback]. */
@Composable
fun BoxScope.PlaybackOverlay(
    playback: Playback?,
    duration: Double?,
    barHeight: Dp = 4.dp,
    badgeSize: Dp = 22.dp,
    accent: Color = WeirdStreamerTheme.spec.progressColor.takeOrElse { MaterialTheme.colorScheme.primary },
    onAccent: Color = WeirdStreamerTheme.spec.progressLabel.takeOrElse { MaterialTheme.colorScheme.onPrimary }
) {
    WatchProgressOverlay(playback?.fraction(duration) ?: 0f, playback?.watched == true, barHeight, badgeSize, accent, onAccent)
}

/** "1:23:45" or "12:34". */
fun clockText(seconds: Double): String {
    val s = seconds.toLong().coerceAtLeast(0)
    val h = s / 3600
    val m = (s % 3600) / 60
    val sec = s % 60
    return if (h > 0) String.format(Locale.US, "%d:%02d:%02d", h, m, sec) else String.format(Locale.US, "%d:%02d", m, sec)
}

/** "23 min left", for a video in progress. */
fun timeLeftText(playback: Playback?, duration: Double?): String? {
    val p = playback ?: return null
    val total = p.duration.takeIf { it > 0 } ?: duration ?: return null
    if (p.resumeAt <= 0 || total <= 0) return null
    return "${((total - p.position) / 60).toInt().coerceAtLeast(1)} min left"
}

/** Phone: Continue Watching, a row of videos in progress (nothing when there are none). */
@Composable
fun PhoneContinueRow(viewModel: MediaViewModel, modifier: Modifier = Modifier, sidePadding: Dp = 16.dp) {
    val items = viewModel.continueItems
    if (items.isEmpty()) return
    Column(modifier = modifier) {
        Text("Continue Watching", style = MaterialTheme.typography.titleMedium,
            modifier = Modifier.padding(start = sidePadding, end = sidePadding, top = 12.dp, bottom = 8.dp))
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            contentPadding = PaddingValues(horizontal = sidePadding)
        ) {
            items(items, key = { it.path }) { item -> PhoneContinueCard(viewModel, item) }
        }
    }
}

@Composable
private fun PhoneContinueCard(viewModel: MediaViewModel, item: ContinueItem) {
    val (title, subtitle) = item.titleLines
    Column(modifier = Modifier.width(200.dp).clickable { viewModel.playContinue(item) }) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier.fillMaxWidth().aspectRatio(16f / 9f).clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(32.dp))
            continueImage(viewModel, item)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
            PlaybackOverlay(item.playback, item.duration)
            IconButton(
                onClick = { viewModel.removeFromContinue(item) },
                modifier = Modifier.align(Alignment.TopEnd).size(36.dp)
            ) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier.size(26.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.6f))
                ) {
                    Icon(Icons.Default.Close, contentDescription = "Remove from Continue Watching", tint = Color.White,
                        modifier = Modifier.size(16.dp))
                }
            }
        }
        Text(title, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(top = 6.dp))
        subtitle?.let {
            Text(it, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        timeLeftText(item.playback, item.duration)?.let {
            Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** The picture for a Continue Watching entry: the episode still, the backdrop, or the thumbnail. */
fun continueImage(viewModel: MediaViewModel, item: ContinueItem): String? =
    viewModel.serverUrl(item.metadata?.still) ?: viewModel.serverUrl(item.metadata?.backdrop)
        ?: viewModel.serverUrl(item.thumbnailUrl)

/** Phone folder view: Resume or Play from start, for a partly watched video. */
@Composable
fun PhoneResumeDialog(viewModel: MediaViewModel) {
    val item = viewModel.resumeChoice ?: return
    val resumeAt = viewModel.resumeSeconds(item.path)
    androidx.compose.material3.AlertDialog(
        onDismissRequest = { viewModel.chooseResume(null) },
        title = { Text(item.name, style = MaterialTheme.typography.titleMedium, maxLines = 3, overflow = TextOverflow.Ellipsis) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                timeLeftText(viewModel.playbackFor(item.path), item.duration)?.let {
                    Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(bottom = 8.dp))
                }
                androidx.compose.material3.FilledTonalButton(shape = AppButtonShape, onClick = { viewModel.chooseResume(false) },
                    modifier = Modifier.fillMaxWidth()) {
                    Icon(androidx.compose.material.icons.Icons.Default.PlayArrow, contentDescription = null)
                    androidx.compose.foundation.layout.Spacer(Modifier.width(8.dp))
                    Text("Resume from ${clockText(resumeAt)}")
                }
                androidx.compose.material3.OutlinedButton(shape = AppButtonShape, onClick = { viewModel.chooseResume(true) },
                    modifier = Modifier.fillMaxWidth()) {
                    Icon(androidx.compose.material.icons.Icons.Default.Replay, contentDescription = null)
                    androidx.compose.foundation.layout.Spacer(Modifier.width(8.dp))
                    Text("Play from start")
                }
            }
        },
        confirmButton = {},
        dismissButton = { androidx.compose.material3.TextButton(shape = AppButtonShape, onClick = { viewModel.chooseResume(null) }) { Text("Cancel") } }
    )
}

/**
 * A season's progress as a ring in [color]: dimmed and dashed while none of it is watched, then a
 * solid track with an arc for the share watched ([fraction], 0 to 1), whole once it all is.
 */
@Composable
fun SeasonProgressRing(fraction: Float, color: Color, modifier: Modifier = Modifier, size: Dp = 18.dp) {
    androidx.compose.foundation.Canvas(modifier.size(size)) {
        val width = this.size.minDimension * 0.14f
        val inset = width / 2
        val arcSize = androidx.compose.ui.geometry.Size(this.size.width - width, this.size.height - width)
        val topLeft = androidx.compose.ui.geometry.Offset(inset, inset)
        if (fraction <= 0f) {
            // Six long dashes, evenly around from the top (the circumference divides into them exactly), as on the web
            val segment = (Math.PI * arcSize.width).toFloat() / 6
            drawArc(color.copy(alpha = 0.45f), -90f, 360f, false, topLeft, arcSize,
                style = androidx.compose.ui.graphics.drawscope.Stroke(width,
                    pathEffect = androidx.compose.ui.graphics.PathEffect.dashPathEffect(floatArrayOf(segment * 0.7f, segment * 0.3f))))
        } else {
            drawArc(color.copy(alpha = 0.3f), 0f, 360f, false, topLeft, arcSize,
                style = androidx.compose.ui.graphics.drawscope.Stroke(width))
            drawArc(color, -90f, 360f * fraction.coerceAtMost(1f), false, topLeft, arcSize,
                style = androidx.compose.ui.graphics.drawscope.Stroke(width, cap = androidx.compose.ui.graphics.StrokeCap.Round))
        }
    }
}

/** "10 Episodes", or "1 Episode". */
fun episodeCountText(count: Int) = if (count == 1) "1 Episode" else "$count Episodes"

/**
 * Where a show picks up, as (season index, episode index): the episode played most recently if it's
 * unfinished, else the one after it (on into the next season, skipping specials unless it's one);
 * [justPlayed] (back from the player) counts as the most recent. Null when none has been played.
 */
fun upNext(seasons: List<com.vargon.weirdstreamer.api.models.LibrarySeason>, justPlayed: String?): Pair<Int, Int>? {
    data class At(val season: Int, val episode: Int, val ep: com.vargon.weirdstreamer.api.models.LibraryEpisode, val number: Int)
    val all = seasons.flatMapIndexed { si, s -> s.episodes.mapIndexed { ei, e -> At(si, ei, e, s.season) } }
    val played = { a: At -> a.ep.files.any { it.playback?.let { p -> p.watched || p.resumeAt > 0 } == true } }
    val latest = all.indexOfFirst { a -> justPlayed != null && a.ep.files.any { it.path == justPlayed } }.takeIf { it >= 0 }
        ?: all.indices.filter { played(all[it]) }.maxByOrNull { i -> all[i].ep.files.maxOf { it.playback?.updatedAt ?: 0L } }
        ?: return null
    val at = all[latest]
    val finished = at.ep.files.any { it.playback?.watched == true } && at.ep.files.none { (it.playback?.resumeAt ?: 0.0) > 0 && it.playback?.watched != true }
    val next = if (!finished) at else
        all.drop(latest + 1).firstOrNull { it.number != 0 || at.number == 0 } ?: at
    return next.season to next.episode
}
