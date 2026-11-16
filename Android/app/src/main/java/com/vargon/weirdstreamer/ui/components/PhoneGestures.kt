package com.vargon.weirdstreamer.ui.components

import android.app.Activity
import android.content.Context
import android.media.AudioManager
import android.provider.Settings
import android.view.WindowManager
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.calculatePan
import androidx.compose.foundation.gestures.calculateZoom
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.BrightnessMedium
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.input.pointer.positionChange
import androidx.compose.ui.input.pointer.positionChanged
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.dp
import java.util.Locale
import kotlin.math.abs
import kotlin.math.roundToInt

/** What a one-finger swipe on the phone player's picture does. */
enum class PlayerSwipe { Seek, Brightness, Volume }

/**
 * The phone player's touch gestures, in one detector so they can't fight over a drag:
 * - two fingers pinch-zoom and pan (`onTransform`), as before, at any time; a second finger
 *   during a swipe cancels the swipe and zooms instead;
 * - one finger on a zoomed picture pans it;
 * - one finger on an unzoomed picture swipes: mostly sideways seeks, up or down on the left half
 *   is brightness and on the right half volume (`onSwipe` gets the move and the area's size).
 * A tap is left to the clickable after it (it's cancelled once a drag takes over), and drags a
 * child takes (the seek bar, the trickplay row) are left alone.
 */
fun Modifier.phonePlayerGestures(
    enabled: () -> Boolean,
    zoomed: () -> Boolean,
    onTransform: (pan: Offset, zoom: Float) -> Unit,
    onSwipeStart: (PlayerSwipe) -> Unit,
    onSwipe: (PlayerSwipe, Offset, IntSize) -> Unit,
    onSwipeEnd: (PlayerSwipe, cancelled: Boolean) -> Unit,
): Modifier = pointerInput(Unit) {
    awaitEachGesture {
        val down = awaitFirstDown(requireUnconsumed = false)
        val slop = viewConfiguration.touchSlop
        var mode = 0 // 0 undecided, 1 pinch, 2 one-finger pan, 3 swipe
        var swipe: PlayerSwipe? = null
        var moved = Offset.Zero
        do {
            val event = awaitPointerEvent()
            val pressed = event.changes.count { it.pressed }
            if (pressed > 1 && mode != 1) {
                swipe?.let { onSwipeEnd(it, true) }
                swipe = null
                mode = 1
            }
            when (mode) {
                1, 2 -> {
                    val zoom = if (mode == 1) event.calculateZoom() else 1f
                    val pan = event.calculatePan()
                    if (zoom != 1f || pan != Offset.Zero) onTransform(pan, zoom)
                    event.changes.forEach { if (it.positionChanged()) it.consume() }
                }
                3 -> event.changes.firstOrNull { it.id == down.id }?.let { change ->
                    onSwipe(swipe!!, change.positionChange(), size)
                    change.consume()
                }
                else -> {
                    val change = event.changes.firstOrNull { it.id == down.id } ?: break
                    if (change.isConsumed) break // a child's drag (seek bar, trickplay row)
                    moved += change.positionChange()
                    if (moved.getDistance() > slop) {
                        if (zoomed()) {
                            mode = 2
                            onTransform(moved, 1f)
                        } else if (enabled()) {
                            val kind = when {
                                abs(moved.x) > abs(moved.y) -> PlayerSwipe.Seek
                                down.position.x < size.width / 2 -> PlayerSwipe.Brightness
                                else -> PlayerSwipe.Volume
                            }
                            swipe = kind
                            mode = 3
                            onSwipeStart(kind)
                        } else break
                        change.consume()
                    }
                }
            }
        } while (event.changes.any { it.pressed })
        swipe?.let { onSwipeEnd(it, false) }
    }
}

/** The window's brightness, 0 to 1: the player's own once set, else the system's. */
fun Activity.playerBrightness(): Float {
    val own = window.attributes.screenBrightness
    if (own >= 0f) return own
    return Settings.System.getInt(contentResolver, Settings.System.SCREEN_BRIGHTNESS, 128) / 255f
}

fun Activity.setPlayerBrightness(level: Float?) {
    val attributes = window.attributes
    attributes.screenBrightness = level?.coerceIn(0.01f, 1f) ?: WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE
    window.attributes = attributes
}

/** Media volume, 0 to 1. */
fun Context.mediaVolume(): Float {
    val audio = getSystemService(Context.AUDIO_SERVICE) as AudioManager
    val max = audio.getStreamMaxVolume(AudioManager.STREAM_MUSIC).coerceAtLeast(1)
    return audio.getStreamVolume(AudioManager.STREAM_MUSIC).toFloat() / max
}

/** Sets the media volume to the nearest step of `level` (0 to 1), without the system's slider; returns that step's level. */
fun Context.setMediaVolume(level: Float): Float {
    val audio = getSystemService(Context.AUDIO_SERVICE) as AudioManager
    val max = audio.getStreamMaxVolume(AudioManager.STREAM_MUSIC).coerceAtLeast(1)
    val step = (level.coerceIn(0f, 1f) * max).roundToInt()
    if (!audio.isVolumeFixed && step != audio.getStreamVolume(AudioManager.STREAM_MUSIC)) {
        audio.setStreamVolume(AudioManager.STREAM_MUSIC, step, 0)
    }
    return audio.getStreamVolume(AudioManager.STREAM_MUSIC).toFloat() / max
}

/** A seek's "+0:30 / 12:34": the jump, then where it lands. */
fun seekGestureText(deltaMs: Long, targetMs: Long): String =
    (if (deltaMs < 0) "−" else "+") + clockText(abs(deltaMs)) + " / " + clockText(targetMs)

private fun clockText(ms: Long): String {
    val s = ms / 1000
    return if (s >= 3600) String.format(Locale.US, "%d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
    else String.format(Locale.US, "%d:%02d", s / 60, s % 60)
}

/** What a swipe shows while it lasts: the seek's text, or the brightness or volume level. */
sealed class SwipeIndicator {
    data class Seek(val text: String) : SwipeIndicator()
    data class Level(val volume: Boolean, val level: Float) : SwipeIndicator()
}

@Composable
fun PhoneSwipeIndicator(indicator: SwipeIndicator, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .background(Color.Black.copy(alpha = 0.65f), RoundedCornerShape(12.dp))
            .padding(horizontal = 20.dp, vertical = 14.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        when (indicator) {
            is SwipeIndicator.Seek -> Text(indicator.text, color = Color.White, style = MaterialTheme.typography.headlineSmall)
            is SwipeIndicator.Level -> {
                val percent = (indicator.level * 100).roundToInt()
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(
                        when {
                            !indicator.volume -> Icons.Default.BrightnessMedium
                            percent == 0 -> Icons.AutoMirrored.Filled.VolumeOff
                            else -> Icons.AutoMirrored.Filled.VolumeUp
                        },
                        contentDescription = if (indicator.volume) "Volume" else "Brightness",
                        tint = Color.White,
                        modifier = Modifier.size(28.dp)
                    )
                    Spacer(Modifier.width(10.dp))
                    Text("$percent%", color = Color.White, style = MaterialTheme.typography.titleMedium)
                }
                LinearProgressIndicator(
                    progress = { indicator.level.coerceIn(0f, 1f) },
                    modifier = Modifier.padding(top = 10.dp).width(160.dp),
                    color = Color.White,
                    trackColor = Color.White.copy(alpha = 0.3f),
                    strokeCap = StrokeCap.Round,
                )
            }
        }
    }
}
