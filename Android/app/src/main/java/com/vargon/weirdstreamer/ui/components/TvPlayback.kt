package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import kotlinx.coroutines.delay
import androidx.compose.runtime.getValue
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
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Icon
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.MediaViewModel

/** Progress lines and checks on TV: white reads on every theme's dark backgrounds. */
/**
 * TV: the rows above a folder or library grid: Continue Watching, the videos in progress, and
 * Recently Added, the last to arrive in the library. Up goes to the toolbar ([up]); Down from the last
 * row hands focus to the grid ([onDown]). [topRequester] focuses the top row and [bottomRequester]
 * the bottom one (the grid's Up), each on the card last focused there, or its first.
 */
@Composable
fun TvContinueRow(
    viewModel: MediaViewModel,
    topRequester: FocusRequester,
    bottomRequester: FocusRequester,
    up: FocusRequester,
    onDown: () -> Boolean,
    modifier: Modifier = Modifier
) {
    val inProgress = viewModel.continueItems
    val newlyAdded = viewModel.newItems
    val both = inProgress.isNotEmpty() && newlyAdded.isNotEmpty()
    Column(modifier = modifier) {
        if (inProgress.isNotEmpty()) TvVideoRow(viewModel, "Continue Watching", inProgress,
            if (both) listOf(topRequester) else listOf(topRequester, bottomRequester), up,
            onDown = { if (both) bottomRequester.tryFocus() else onDown() })
        if (newlyAdded.isNotEmpty()) TvVideoRow(viewModel, "Recently Added", newlyAdded,
            if (both) listOf(bottomRequester) else listOf(topRequester, bottomRequester),
            if (both) topRequester else up, onDown,
            modifier = if (both) Modifier.padding(top = 16.dp) else Modifier)
    }
}

/** Focuses what this requester is on; false when nothing is (not composed, or can't take focus). */
fun FocusRequester.tryFocus(): Boolean = runCatching { requestFocus() }.getOrDefault(false)

@Composable
private fun TvVideoRow(
    viewModel: MediaViewModel,
    heading: String,
    items: List<com.vargon.weirdstreamer.api.models.ContinueItem>,
    requesters: List<FocusRequester>,
    up: FocusRequester,
    onDown: () -> Boolean,
    modifier: Modifier = Modifier
) {
    Column(modifier = modifier) {
        Text(heading, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(bottom = 10.dp))
        // The requesters are on the row, not its first card, which scrolls away (and out of
        // composition) along the row: focusing the row goes to the card last focused in it, or
        // the first. The view model keeps that card, as the grid scrolls the whole row out of
        // composition too (Compose's focusRestorer then lost track of it and picked another).
        val last = viewModel.rowFocus[heading]
        val entryIndex = items.indexOfFirst { it.path == last }.coerceAtLeast(0)
        val entry = remember { FocusRequester() }
        val rowState = rememberLazyListState(initialFirstVisibleItemIndex = entryIndex)
        LazyRow(
            state = rowState,
            horizontalArrangement = Arrangement.spacedBy(20.dp),
            contentPadding = PaddingValues(horizontal = FocusBleed, vertical = 8.dp),
            modifier = requesters.fold(Modifier.focusBleed()) { m, r -> m.focusRequester(r) }
                .focusProperties { onEnter = { entry.tryFocus() } }
                .onPreviewKeyEvent { e ->
                    e.type == KeyEventType.KeyDown && e.key == Key.DirectionDown && onDown()
                }
        ) {
            itemsIndexed(items, key = { _, it -> it.path }) { index, item ->
                val (title, subtitle) = item.titleLines
                Column(modifier = Modifier.width(260.dp)) {
                    // A press resumes; holding OK opens Resume / Play from start / Remove. The key
                    // repeats while held: the first repeat opens the menu, and the release that
                    // follows is swallowed so it doesn't also play.
                    var held by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(false) }
                    Box(
                        contentAlignment = Alignment.Center,
                        modifier = Modifier
                            .fillMaxWidth()
                            .aspectRatio(16f / 9f)
                            .then(if (index == entryIndex) Modifier.focusRequester(entry) else Modifier)
                            .onFocusChanged { if (it.isFocused) viewModel.rowFocus[heading] = item.path }
                            .focusProperties { this.up = up }
                            .onPreviewKeyEvent { e ->
                                val select = e.key == Key.DirectionCenter || e.key == Key.Enter || e.key == Key.NumPadEnter
                                when {
                                    !select || item.addedAt != null -> false // Recently Added has nothing to remove
                                    e.type == KeyEventType.KeyDown && e.nativeKeyEvent.repeatCount > 0 -> {
                                        if (!held) {
                                            held = true
                                            viewModel.openContinueMenu(item)
                                        }
                                        true
                                    }
                                    e.type == KeyEventType.KeyUp && held -> {
                                        held = false
                                        true
                                    }
                                    else -> false
                                }
                            }
                            .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.06f, borderWidth = 3.dp,
                                onClick = { viewModel.playContinue(item) })
                            .clip(MaterialTheme.shapes.medium)
                            .background(MaterialTheme.colorScheme.surfaceVariant)
                    ) {
                        Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(40.dp))
                        continueImage(viewModel, item)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                        PlaybackOverlay(item.playback, item.duration, barHeight = 5.dp)
                    }
                    Text(title, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(top = 8.dp))
                    listOfNotNull(subtitle, continueNote(item)).forEach {
                        Text(it, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
    }
}

/** TV folder view: Resume or Play from start, for a partly watched video. */
@Composable
fun TvResumeDialog(viewModel: MediaViewModel) {
    val item = viewModel.resumeChoice ?: return
    val resumeAt = viewModel.resumeSeconds(item.path)
    val resumeRequester = androidx.compose.runtime.remember { FocusRequester() }
    // Opened by holding OK, the button is still down: its repeats and its release would press
    // Resume. OK counts only after a fresh press (a new key down that isn't a repeat).
    var armed by androidx.compose.runtime.remember(item.path) { androidx.compose.runtime.mutableStateOf(false) }
    androidx.compose.ui.window.Dialog(onDismissRequest = { viewModel.chooseResume(null) }) {
        androidx.tv.material3.Surface(shape = MaterialTheme.shapes.large, modifier = Modifier.onPreviewKeyEvent { e ->
            val select = e.key == Key.DirectionCenter || e.key == Key.Enter || e.key == Key.NumPadEnter
            if (!select || armed) return@onPreviewKeyEvent false
            if (e.type == KeyEventType.KeyDown && e.nativeKeyEvent.repeatCount == 0) {
                armed = true
                false
            } else true // a held key's repeat or release: swallowed
        }) {
            Column(modifier = Modifier.padding(28.dp).width(420.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)) {
                // From Continue Watching: its title ("Example Show" and the episode); else the file name.
                val (title, subtitle) = viewModel.continueChoice?.titleLines ?: (item.name to null)
                Text(title, style = MaterialTheme.typography.titleLarge, maxLines = 3, overflow = TextOverflow.Ellipsis)
                subtitle?.let { Text(it, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis) }
                timeLeftText(viewModel.playbackFor(item.path), item.duration)?.let {
                    Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                androidx.tv.material3.Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { viewModel.chooseResume(false) },
                    modifier = Modifier.fillMaxWidth().focusRequester(resumeRequester)) {
                    Icon(androidx.compose.material.icons.Icons.Default.PlayArrow, contentDescription = null)
                    androidx.compose.foundation.layout.Spacer(Modifier.width(8.dp))
                    Text("Resume from ${clockText(resumeAt)}")
                }
                androidx.tv.material3.Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { viewModel.chooseResume(true) }, modifier = Modifier.fillMaxWidth()) {
                    Icon(androidx.compose.material.icons.Icons.Default.Replay, contentDescription = null)
                    androidx.compose.foundation.layout.Spacer(Modifier.width(8.dp))
                    Text("Play from start")
                }
                if (viewModel.continueChoice != null) {
                    androidx.tv.material3.Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { viewModel.removeChoice() }, modifier = Modifier.fillMaxWidth()) {
                        Icon(androidx.compose.material.icons.Icons.Default.Close, contentDescription = null)
                        androidx.compose.foundation.layout.Spacer(Modifier.width(8.dp))
                        Text("Remove from Continue Watching")
                    }
                }
            }
        }
    }
    androidx.compose.runtime.LaunchedEffect(item.path) {
        androidx.compose.runtime.withFrameNanos { }
        runCatching { resumeRequester.requestFocus() }
    }
}

/**
 * Whether a page with a Continue Watching row can show: once the row has loaded (or after a few
 * seconds without an answer), so focus starts on the row instead of jumping there from the folders.
 */
@Composable
fun continueSettled(viewModel: MediaViewModel, page: Any?): Boolean {
    var waited by remember(page) { mutableStateOf(false) }
    LaunchedEffect(page) { delay(3_000); waited = true }
    return viewModel.continueReady || viewModel.hasTopRows || waited
}
