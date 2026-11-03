package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.repository.FileItem
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

private const val GRID_COLUMNS = 4

/** Lets the toolbar hand focus down to whichever content is showing. */
class ContentEntry {
    /** Focuses the first item of the top row on screen; false if there is nothing to focus. */
    var focusTop: (() -> Boolean)? = null
}

internal const val GRID_HEADER = "grid-header"

/** The key of the first item in the topmost fully visible row of a lazy grid, skipping [skip]. */
internal fun LazyGridState.topRowKey(skip: (Any) -> Boolean = { false }): Any? {
    val visible = layoutInfo.visibleItemsInfo.filterNot { skip(it.key) }
    // Items are in index order, so the first one fully on screen starts its row.
    return (visible.firstOrNull { it.offset.y >= 0 } ?: visible.firstOrNull())?.key
}

/**
 * Folder contents as a native lazy grid (or a one-column list). D-pad focus movement and
 * scrolling are left to Compose: on TV devices lazy layouts already keep the focused item at a
 * fixed pivot and animate toward it, like the system launcher.
 *
 * @param folderKey identifies the folder; scroll and focus restoration reset when it changes.
 * @param onVisibleItemsChanged called (debounced) with the items on screen, for lazy metadata loading.
 */
@OptIn(FlowPreview::class, ExperimentalFoundationApi::class)
@Composable
fun BrowserGrid(
    folderKey: Any,
    items: List<FileItem>,
    isGridView: Boolean,
    initialSelection: String?,
    toolbarFocusRequester: FocusRequester,
    contentEntry: ContentEntry,
    takeFocus: Boolean,
    // Where Up from the top row goes (a row above the grid, else the toolbar)
    upFocusRequester: FocusRequester = toolbarFocusRequester,
    // A full-width row above the items that scrolls with them (Continue Watching)
    header: (@Composable () -> Unit)? = null,
    // Focuses the header's first item: on a fresh visit it takes focus when it arrives
    headerFocus: FocusRequester? = null,
    onVisibleItemsChanged: (List<FileItem>) -> Unit,
    itemContent: @Composable (FileItem, Modifier) -> Unit
) {
    val columns = if (isGridView) GRID_COLUMNS else 1
    val headerCount = if (header != null) 1 else 0
    val gridState = remember(folderKey) { LazyGridState() }
    val scope = rememberCoroutineScope()
    val currentItems by rememberUpdatedState(items)

    // Requesters of the currently composed items, by path. Plain (non-state) bookkeeping, so
    // focus moves never trigger recomposition.
    val requesters = remember(folderKey) { HashMap<String, FocusRequester>() }
    val focusedIndex = remember(folderKey) { intArrayOf(-1) }
    val initialPath = remember(folderKey) {
        items.firstOrNull { it.path == initialSelection || it.name == initialSelection }?.path
            ?: items.firstOrNull()?.path
    }

    fun tryFocus(path: String): Boolean =
        requesters[path]?.let { runCatching { it.requestFocus() }.getOrDefault(false) } ?: false

    suspend fun focusItem(path: String) {
        val index = currentItems.indexOfFirst { it.path == path }
        if (index < 0) return
        val lazyIndex = index + headerCount
        val visible = gridState.layoutInfo.visibleItemsInfo
        if (visible.none { it.index == lazyIndex }) gridState.scrollToItem(lazyIndex)
        // Wait for the targeted item to be composed.
        repeat(3) {
            withFrameNanos { }
            if (tryFocus(path)) return
        }
    }

    /**
     * Moves focus exactly one row, staying in the same column. Compose's own focus search can't
     * be used here: when a held key outruns the scroll animation, the next row isn't composed
     * yet, the grid extends one item at a time, and focus lands on the row's first (or last)
     * item instead.
     */
    fun moveVertically(rows: Int): Boolean {
        val list = currentItems
        val from = focusedIndex[0]
        if (from !in list.indices) return false
        var target = from + rows * columns
        if (target < 0) {
            if (header == null) return false // top row: focusProperties sends it to the toolbar
            // Up to the rows above, scrolled out of composition if the grid went past them.
            if (!upFocusRequester.tryFocus()) scope.launch {
                gridState.scrollToItem(0)
                withFrameNanos { }
                upFocusRequester.tryFocus()
            }
            return true
        }
        if (target >= list.size) {
            if (from / columns == (list.size - 1) / columns) return true // already on the last row
            target = list.size - 1
        }
        val path = list[target].path
        if (!tryFocus(path) && !gridState.isScrollInProgress) {
            // Not composed and nothing in flight to bring it in: scroll to it explicitly.
            scope.launch { focusItem(path) }
        }
        // While a scroll is still catching up, the key is dropped; the next repeat retries.
        return true
    }

    DisposableEffect(contentEntry, folderKey) {
        val focusTop = {
            val path = gridState.topRowKey { it == GRID_HEADER } as? String
            if (path != null && !tryFocus(path)) scope.launch { focusItem(path) }
            path != null
        }
        contentEntry.focusTop = focusTop
        onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
    }

    // A fresh visit (nothing opened here before), and whether a key was pressed in the grid
    // since: until then the header, when there is one, takes focus as it arrives.
    val freshVisit = remember(folderKey) { initialSelection == null }
    val keyPressed = remember(folderKey) { booleanArrayOf(false) }
    val gridFocused = remember { booleanArrayOf(false) }

    // Not keyed on isGridView: the list/grid toggle is on the toolbar, which keeps focus.
    LaunchedEffect(folderKey) {
        if (takeFocus) initialPath?.let { focusItem(it) }
    }
    // The header (Continue Watching) usually arrives just after the items. On a fresh visit its
    // first item takes focus (the grid's first item had it, and the header would push it off
    // screen); otherwise, while focus is in the top row, the header shows whole above it.
    LaunchedEffect(folderKey, headerCount) {
        if (headerCount == 0) return@LaunchedEffect
        withFrameNanos { }
        if (headerFocus != null && takeFocus && freshVisit && !keyPressed[0] && gridFocused[0]) {
            gridState.scrollToItem(0)
            withFrameNanos { }
            if (runCatching { headerFocus.requestFocus() }.getOrDefault(false)) return@LaunchedEffect
        }
        if (focusedIndex[0] in 0 until columns) gridState.scrollToItem(0)
    }

    LaunchedEffect(gridState, columns) {
        snapshotFlow {
            val visible = gridState.layoutInfo.visibleItemsInfo
            if (visible.isEmpty()) null
            else (visible.first().index - headerCount).coerceAtLeast(0) to (visible.last().index - headerCount).coerceAtLeast(0)
        }
            .distinctUntilChanged()
            .debounce(250)
            .collect { range ->
                val list = currentItems
                if (range == null || list.isEmpty()) return@collect
                // Include one row past the screen so metadata is usually there before it scrolls in.
                val first = range.first.coerceIn(list.indices)
                val last = (range.second + columns).coerceIn(list.indices)
                onVisibleItemsChanged(list.subList(first, last + 1))
            }
    }

    // The A–Z bar: folders' letters, then files'. Right from an item (from a row's last tile in
    // grid view) moves onto it; Up/Down scroll letter by letter and Left goes to the letter's
    // first item.
    val letters = remember(items) {
        buildIndex(items, { indexLabel(it.name) }, { if (it.isDirectory) 0 else 1 })
    }
    val showBar = letters.size > 1
    val barRequester = remember { FocusRequester() }
    var barFocused by remember { mutableStateOf(false) }
    var barLetter by remember(folderKey) { mutableStateOf(0) }
    // Unfocused, the bar marks the letter at the top of the screen.
    val topLetter by remember(letters, headerCount) {
        derivedStateOf { letters.entryFor((gridState.firstVisibleItemIndex - headerCount).coerceAtLeast(0)) }
    }

    // Moving along a row to a tile already fully on screen doesn't scroll. The TV's own scrolling
    // (focus at a fixed point) would, and in the first row that fights the scroll keeping the
    // header above it in view, bouncing the grid on every move.
    val alongRow = remember { booleanArrayOf(false) }
    val tvScrolling = LocalBringIntoViewSpec.current
    val scrolling = remember(tvScrolling) {
        object : BringIntoViewSpec {
            override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float =
                if (alongRow[0] && offset >= 0 && offset + size <= containerSize) 0f
                else tvScrolling.calculateScrollDistance(offset, size, containerSize)
        }
    }

    Row(modifier = Modifier.fillMaxSize()) {
        CompositionLocalProvider(LocalBringIntoViewSpec provides scrolling) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(columns),
            state = gridState,
            modifier = Modifier
                .weight(1f)
                .fillMaxHeight()
                .onFocusChanged { gridFocused[0] = it.hasFocus }
                .onPreviewKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    keyPressed[0] = true
                    alongRow[0] = event.key == Key.DirectionLeft || event.key == Key.DirectionRight
                    when (event.key) {
                        Key.DirectionDown -> isGridView && moveVertically(1)
                        Key.DirectionUp -> isGridView && moveVertically(-1)
                        else -> false
                    }
                },
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(if (isGridView) 10.dp else 0.dp)
        ) {
            if (header != null) {
                item(key = GRID_HEADER, span = { GridItemSpan(maxLineSpan) }, contentType = GRID_HEADER) {
                    // While the header has focus, row-by-row moves are its own (Down leaves it).
                    Box(Modifier.onFocusChanged { if (it.hasFocus) focusedIndex[0] = -1 }) { header() }
                }
            }
            itemsIndexed(
                items = items,
                key = { _, item -> item.path },
                contentType = { _, item -> if (item.isDirectory) 0 else 1 }
            ) { index, item ->
                val requester = remember { FocusRequester() }
                DisposableEffect(item.path) {
                    requesters[item.path] = requester
                    onDispose { if (requesters[item.path] === requester) requesters.remove(item.path) }
                }
                var itemModifier: Modifier = Modifier
                    .focusRequester(requester)
                    .onFocusChanged {
                        if (!it.hasFocus) return@onFocusChanged
                        focusedIndex[0] = index
                        // In the top row, show as much of the header above it as fits with the
                        // item still whole (after the scroll that brings the item into view).
                        // Scrolling to the very top can push the item off the bottom when the
                        // header and a row don't both fit.
                        if (headerCount > 0 && index < columns) {
                            scope.launch {
                                withFrameNanos { }
                                val info = gridState.layoutInfo
                                val shown = info.visibleItemsInfo.firstOrNull { it.index == index + headerCount }
                                    ?: return@launch
                                // Room below the item: scrolling up by it keeps the item in view
                                // (negative: the item is cut off, so scroll down by that much).
                                val room = info.viewportEndOffset - info.afterContentPadding -
                                    (shown.offset.y + shown.size.height)
                                if (room != 0) gridState.animateScrollBy(-room.toFloat())
                            }
                        }
                    }
                val isFirstColumn = index % columns == 0
                val isFirstRow = index < columns
                // The row's last tile (any row in list view): Right goes to the A–Z bar, or nowhere.
                val isRowEnd = index % columns == columns - 1 || index == items.lastIndex
                if (isFirstColumn || isFirstRow || isRowEnd) {
                    // The lazy grid doesn't hand focus searches off its edges reliably, so the
                    // edges are explicit: left/up go to the toolbar, right from the row end to the bar.
                    itemModifier = itemModifier.focusProperties {
                        if (isFirstColumn) left = toolbarFocusRequester
                        if (isFirstRow) up = upFocusRequester
                        if (isRowEnd) right = if (showBar) barRequester else FocusRequester.Cancel
                    }
                }
                itemContent(item, itemModifier)
            }
        }

        }

        if (showBar) {
            TvIndexBar(
                entries = letters,
                current = if (barFocused) barLetter else topLetter,
                focusRequester = barRequester,
                onFocused = {
                    barFocused = true
                    barLetter = letters.entryFor(focusedIndex[0].coerceAtLeast(0))
                },
                onMove = { i ->
                    barLetter = i
                    scope.launch { gridState.scrollToItem(letters[i].position + headerCount) }
                },
                onPick = { i ->
                    barFocused = false
                    scope.launch { focusItem(currentItems[letters[i].position].path) }
                },
                modifier = Modifier
                    .padding(start = 4.dp, top = 8.dp, bottom = 8.dp)
                    .onFocusChanged { if (!it.hasFocus) barFocused = false }
            )
        }
    }
}
