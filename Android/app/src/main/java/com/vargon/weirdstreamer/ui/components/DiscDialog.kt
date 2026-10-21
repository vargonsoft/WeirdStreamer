package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.api.models.DiscInfo
import kotlinx.coroutines.android.awaitFrame

/** One choice in the disc dialog. */
private data class DiscRow(val label: String, val selected: Boolean, val onClick: () -> Unit)

/**
 * A DVD's chapters, angles, audio and subtitle tracks, and its other titles. Picking one restarts
 * the stream there; focus starts on the chapter playing.
 */
@Composable
fun DiscDialog(
    show: Boolean,
    info: DiscInfo?,
    titleNumber: Int,
    angle: Int,
    audio: Int,
    subtitle: Int,
    positionMs: Long,
    onTitle: (Int) -> Unit,
    onChapter: (Long) -> Unit,
    onAngle: (Int) -> Unit,
    onAudio: (Int) -> Unit,
    onSubtitle: (Int) -> Unit,
    onDismiss: () -> Unit
) {
    if (!show || info == null) return
    val title = info.title(titleNumber) ?: return
    val positionSec = positionMs / 1000.0
    val currentChapter = title.chapters.indexOfLast { positionSec + 0.5 >= it }.coerceAtLeast(0)

    val sections = buildList<Pair<String, List<DiscRow>>> {
        if (title.chapters.size > 1) add("Chapters" to title.chapters.mapIndexed { i, start ->
            DiscRow("${i + 1}   ${clockText(start)}", i == currentChapter) { onChapter((start * 1000).toLong()) }
        })
        if (title.angles > 1) add("Angle" to (1..title.angles).map { a ->
            DiscRow("Angle $a of ${title.angles}", a == angle) { onAngle(a) }
        })
        if (title.audio.size > 1) add("Audio" to title.audio.map { t ->
            DiscRow("${t.index + 1}   ${t.label}", t.index == audio) { onAudio(t.index) }
        })
        add("Subtitles" to (listOf(DiscRow("Off", subtitle < 0) { onSubtitle(-1) }) + title.subtitles.map { t ->
            DiscRow("${t.index + 1}   ${t.label}", t.index == subtitle) { onSubtitle(t.index) }
        }))
        if (info.titles.size > 1) add("Titles" to info.titles.sortedBy { it.number }.map { t ->
            val main = if (t.number == info.mainTitle) "  (main)" else ""
            DiscRow("Title ${t.number}   ${clockText(t.duration)}$main", t.number == title.number) { onTitle(t.number) }
        })
    }
    // Flattened for the list: headers (null rows) and choices.
    val rows = sections.flatMap { (header, items) -> listOf(header to null) + items.map { header to it } }
    val firstFocus = rows.indexOfFirst { it.second?.selected == true }.coerceAtLeast(1)
    val focusRequester = remember { FocusRequester() }
    val listState = rememberLazyListState(initialFirstVisibleItemIndex = (firstFocus - 2).coerceAtLeast(0))
    LaunchedEffect(Unit) {
        awaitFrame()
        runCatching { focusRequester.requestFocus() }
    }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(
            modifier = Modifier
                .width(460.dp)
                .heightIn(max = 520.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
        ) {
            Column(modifier = Modifier.padding(20.dp)) {
                Text("Disc · Title ${title.number}", style = MaterialTheme.typography.titleLarge, color = Color.White)
                Spacer(Modifier.padding(top = 8.dp))
                LazyColumn(state = listState, verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    itemsIndexed(rows) { index, (header, row) ->
                        if (row == null) {
                            Text(header, style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold,
                                color = Color.White.copy(alpha = 0.6f), modifier = Modifier.padding(top = 10.dp, bottom = 2.dp))
                        } else {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .then(if (index == firstFocus) Modifier.focusRequester(focusRequester) else Modifier)
                                    .tvClickable(shape = RoundedCornerShape(8.dp), focusedScale = 1.02f, borderWidth = 2.dp,
                                        onClick = { row.onClick() })
                                    .padding(horizontal = 12.dp, vertical = 8.dp)
                            ) {
                                Text(if (row.selected) "●" else "○", color = if (row.selected) Color.Green else Color.Gray,
                                    modifier = Modifier.width(24.dp))
                                Text(row.label, color = Color.White, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                        }
                    }
                }
            }
        }
    }
}
