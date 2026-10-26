package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.min
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

/**
 * One letter on an A–Z bar: [label], the [position] of the first item it jumps to, and its
 * [run] (folders, then files; titles, then unmatched files). Each run has its own letters, in
 * the order they're shown.
 */
data class IndexEntry(val label: String, val position: Int, val run: Int)

private val ARTICLES = listOf("the ", "a ", "an ")

/**
 * The letter a name is filed under: its first letter, or # for digits and symbols. With
 * [skipArticles], a leading "The", "A" or "An" is skipped, as the server sorts library titles.
 */
fun indexLabel(name: String, skipArticles: Boolean = false): String? {
    var n = name.trim()
    if (n.isEmpty() || n == "..") return null
    if (skipArticles) {
        val lower = n.lowercase()
        ARTICLES.firstOrNull { lower.startsWith(it) && n.length > it.length }?.let { n = n.substring(it.length).trimStart() }
    }
    val c = n.first()
    return if (c.isLetter()) c.uppercase() else "#"
}

/**
 * The bar's entries for items in display order: a letter's first item in each run. A letter that
 * comes back later in the same run (when the sort isn't strictly by it) keeps its first place.
 */
fun <T> buildIndex(items: List<T>, label: (T) -> String?, run: (T) -> Int = { 0 }): List<IndexEntry> {
    val entries = ArrayList<IndexEntry>()
    val seen = HashSet<Pair<Int, String>>()
    items.forEachIndexed { i, item ->
        val l = label(item) ?: return@forEachIndexed
        val r = run(item)
        if (seen.add(r to l)) entries.add(IndexEntry(l, i, r))
    }
    return entries
}

/** The entry an item at [position] is under (the last one starting at or before it). */
fun List<IndexEntry>.entryFor(position: Int): Int {
    if (isEmpty()) return -1
    var found = 0
    for (i in indices) if (this[i].position <= position) found = i
    return found
}

/** Where a divider goes on the bar: before the first entry of the second run, else -1. */
internal fun List<IndexEntry>.dividerBefore(): Int =
    indices.firstOrNull { it > 0 && this[it].run != this[it - 1].run } ?: -1

internal val TV_INDEX_BAR_WIDTH = 32.dp

/**
 * The TV's A–Z bar, down the right side of a grid or list. Right from an item focuses it, on the
 * letter that item is under; Up and Down move between letters ([onMove] scrolls there as it goes,
 * with a popup letter beside the bar), and Left, OK or Back go to that letter's first item
 * ([onPick]). Unfocused, it marks the letter at the top of the screen ([current]).
 */
@Composable
fun TvIndexBar(
    entries: List<IndexEntry>,
    current: Int,
    focusRequester: FocusRequester,
    onFocused: () -> Unit,
    onMove: (Int) -> Unit,
    onPick: (Int) -> Unit,
    modifier: Modifier = Modifier,
) {
    var focused by remember { mutableStateOf(false) }
    val spec = WeirdStreamerTheme.spec
    val accent = spec.focusColor.takeOrElse { MaterialTheme.colorScheme.primary }
    val onAccent = spec.focusLabel.takeOrElse { MaterialTheme.colorScheme.onPrimary }
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    val divider = entries.dividerBefore()

    BoxWithConstraints(
        modifier
            .width(TV_INDEX_BAR_WIDTH)
            .fillMaxHeight()
            .background(if (focused) MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.6f) else Color.Transparent,
                RoundedCornerShape(16.dp))
            .onFocusChanged {
                if (it.isFocused != focused) {
                    focused = it.isFocused
                    if (it.isFocused) onFocused()
                }
            }
            .onPreviewKeyEvent { event ->
                if (event.type != KeyEventType.KeyDown) {
                    // Back acts on release, like the rest of the app: take both halves.
                    return@onPreviewKeyEvent event.key == Key.Back
                }
                when (event.key) {
                    Key.DirectionUp -> { if (current > 0) onMove(current - 1); true }
                    Key.DirectionDown -> { if (current < entries.lastIndex) onMove(current + 1); true }
                    Key.DirectionLeft, Key.DirectionCenter, Key.Enter, Key.NumPadEnter, Key.Back -> {
                        onPick(current); true
                    }
                    Key.DirectionRight -> true
                    else -> false
                }
            }
            .focusRequester(focusRequester)
            .focusable()
    ) {
        // Letters share the height, up to a comfortable size; a divider between the two runs.
        val slots = entries.size + if (divider >= 0) 1 else 0
        val slot: Dp = min(24.dp, (maxHeight - 16.dp) / slots.coerceAtLeast(1))
        val fontSize = with(LocalDensity.current) { (slot * 0.62f).toSp() }
        Column(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            entries.forEachIndexed { i, e ->
                if (i == divider) {
                    Box(Modifier.height(slot), contentAlignment = Alignment.Center) {
                        Box(Modifier.width(14.dp).height(1.dp).background(muted.copy(alpha = 0.6f)))
                    }
                }
                val isCurrent = i == current
                Box(
                    Modifier.size(slot).then(
                        if (isCurrent && focused) Modifier.background(accent, CircleShape) else Modifier
                    ),
                    contentAlignment = Alignment.Center
                ) {
                    Text(
                        e.label,
                        fontSize = fontSize,
                        fontWeight = if (isCurrent) FontWeight.Bold else FontWeight.Normal,
                        color = when {
                            isCurrent && focused -> onAccent
                            isCurrent -> accent
                            else -> muted
                        },
                        maxLines = 1,
                    )
                }
            }
        }
        // The popup letter, beside the current one.
        if (focused && current in entries.indices) {
            val slotIndex = current + if (divider in 0..current) 1 else 0
            val popup = 72.dp
            val y = (8.dp + slot * slotIndex + slot / 2 - popup / 2).coerceIn(0.dp, (maxHeight - popup).coerceAtLeast(0.dp))
            Box(
                Modifier
                    .offset(x = -(popup + 12.dp), y = y)
                    .wrapContentSize(Alignment.TopStart, unbounded = true) // wider than the bar
                    .size(popup)
                    .background(accent, RoundedCornerShape(16.dp)),
                contentAlignment = Alignment.Center
            ) {
                Text(entries[current].label, style = MaterialTheme.typography.displayMedium, color = onAccent)
            }
        }
    }
}
