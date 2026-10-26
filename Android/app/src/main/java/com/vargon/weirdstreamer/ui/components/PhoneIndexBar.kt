package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.min
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

private val BAR_PADDING = 8.dp

/**
 * The phone's A–Z strip down the right edge, like the contacts app's: tap a letter, or drag
 * along it, to jump there ([onSelect]), with the letter in a bubble beside your finger. Otherwise
 * it marks the letter at the top of the screen ([current]).
 */
@Composable
fun PhoneIndexBar(entries: List<IndexEntry>, current: Int, onSelect: (Int) -> Unit, modifier: Modifier = Modifier) {
    var touched by remember { mutableStateOf(-1) }
    val haptics = LocalHapticFeedback.current
    val select by rememberUpdatedState(onSelect)
    val spec = WeirdStreamerTheme.spec
    val accent = spec.focusColor.takeOrElse { MaterialTheme.colorScheme.primary }
    val onAccent = spec.focusLabel.takeOrElse { MaterialTheme.colorScheme.onPrimary }
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    val divider = entries.dividerBefore()
    val slots = entries.size + if (divider >= 0) 1 else 0

    BoxWithConstraints(modifier.width(24.dp).fillMaxHeight()) {
        val slot: Dp = min(20.dp, (maxHeight - BAR_PADDING * 2) / slots.coerceAtLeast(1))
        val slotPx = with(LocalDensity.current) { slot.toPx() }
        val padPx = with(LocalDensity.current) { BAR_PADDING.toPx() }
        // The entry under a touch at [y] (the divider counts as the letter after it).
        fun entryAt(y: Float): Int {
            var s = ((y - padPx) / slotPx).toInt().coerceIn(0, slots - 1)
            if (divider >= 0 && s > divider) s -= 1
            return s.coerceIn(0, entries.lastIndex)
        }
        fun touch(y: Float) {
            val i = entryAt(y)
            if (i != touched) {
                touched = i
                haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
                select(i)
            }
        }
        val shown = if (touched >= 0) touched else current
        Column(
            Modifier
                .fillMaxWidth()
                .fillMaxHeight()
                .background(if (touched >= 0) MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.7f) else Color.Transparent,
                    RoundedCornerShape(12.dp))
                .pointerInput(entries) {
                    // A touch jumps at once, and follows the finger until it lifts.
                    awaitEachGesture {
                        val down = awaitFirstDown()
                        down.consume()
                        touch(down.position.y)
                        while (true) {
                            val change = awaitPointerEvent().changes.firstOrNull { it.id == down.id } ?: break
                            if (!change.pressed) break
                            change.consume()
                            touch(change.position.y)
                        }
                        touched = -1
                    }
                }
                .padding(vertical = BAR_PADDING),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            val fontSize = with(LocalDensity.current) { (slot * 0.6f).toSp() }
            entries.forEachIndexed { i, e ->
                if (i == divider) {
                    Box(Modifier.height(slot), contentAlignment = Alignment.Center) {
                        Box(Modifier.width(10.dp).height(1.dp).background(muted.copy(alpha = 0.6f)))
                    }
                }
                Box(Modifier.height(slot), contentAlignment = Alignment.Center) {
                    Text(
                        e.label,
                        fontSize = fontSize,
                        fontWeight = if (i == shown) FontWeight.Bold else FontWeight.Normal,
                        color = if (i == shown) accent else muted,
                        maxLines = 1,
                    )
                }
            }
        }
        if (touched in entries.indices) {
            val slotIndex = touched + if (divider in 0..touched) 1 else 0
            val bubble = 56.dp
            val y = (BAR_PADDING + slot * slotIndex + slot / 2 - bubble / 2)
                .coerceIn(0.dp, (maxHeight - bubble).coerceAtLeast(0.dp))
            Box(
                Modifier
                    .offset(x = -(bubble + 16.dp), y = y)
                    .wrapContentSize(Alignment.TopStart, unbounded = true) // wider than the strip
                    .size(bubble)
                    .background(accent, RoundedCornerShape(50, 50, 0, 50)),
                contentAlignment = Alignment.Center
            ) {
                Text(entries[touched].label, style = MaterialTheme.typography.headlineMedium, color = onAccent)
            }
        }
    }
}
