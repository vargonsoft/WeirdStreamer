package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.sizeIn
import androidx.compose.foundation.layout.wrapContentSize
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.unit.dp
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.repository.FileItem
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

private const val GRID_COLUMNS = 4

/** Lets the toolbar hand focus down to whichever content is showing. */
class ContentEntry {
    /** Focuses the first item of the top row on screen; false if there is nothing to focus. */
    var focusTop: (() -> Boolean)? = null
}

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
@OptIn(FlowPreview::class)
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
    onDialogFocusChanged: (FocusRequester?) -> Unit,
    onVisibleItemsChanged: (List<FileItem>) -> Unit,
    itemContent: @Composable (FileItem, Modifier) -> Unit
) {
    val columns = if (isGridView) GRID_COLUMNS else 1
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
        val visible = gridState.layoutInfo.visibleItemsInfo
        if (visible.none { it.index == index }) gridState.scrollToItem(index)
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
        if (target < 0) return false // top row: focusProperties sends it to the toolbar
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
            val path = gridState.topRowKey() as? String
            if (path != null && !tryFocus(path)) scope.launch { focusItem(path) }
            path != null
        }
        contentEntry.focusTop = focusTop
        onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
    }

    // Not keyed on isGridView: the list/grid toggle is on the toolbar, which keeps focus.
    LaunchedEffect(folderKey) {
        if (takeFocus) initialPath?.let { focusItem(it) }
    }

    LaunchedEffect(gridState, columns) {
        snapshotFlow {
            val visible = gridState.layoutInfo.visibleItemsInfo
            if (visible.isEmpty()) null else visible.first().index to visible.last().index
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

    // Jump-to-letter (list view: press Right)
    var showCharacterSelector by remember { mutableStateOf(false) }
    var jumpLabel by remember { mutableStateOf("") }
    var showJumpChar by remember { mutableStateOf(false) }
    val dialogFocusRequester = remember { FocusRequester() }

    LaunchedEffect(showCharacterSelector) {
        onDialogFocusChanged(if (showCharacterSelector) dialogFocusRequester else null)
    }
    LaunchedEffect(showJumpChar) {
        if (showJumpChar) {
            delay(1500)
            showJumpChar = false
        }
    }

    val (folderChars, fileChars) = remember(items) {
        val folders = sortedSetOf<Char>()
        val files = sortedSetOf<Char>()
        items.forEach { item ->
            if (item.name == "..") return@forEach
            val c = item.name.firstOrNull()?.uppercaseChar() ?: return@forEach
            if (item.isDirectory) folders.add(c) else files.add(c)
        }
        folders.toList() to files.toList()
    }

    fun jumpToCharacter(char: Char, isFolder: Boolean) {
        val target = items.firstOrNull { item ->
            item.name != ".." && item.isDirectory == isFolder &&
                item.name.firstOrNull()?.uppercaseChar() == char.uppercaseChar()
        } ?: return
        jumpLabel = char.uppercase()
        showJumpChar = true
        scope.launch { focusItem(target.path) }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(columns),
            state = gridState,
            modifier = Modifier
                .fillMaxSize()
                .onPreviewKeyEvent { event ->
                    if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    when (event.key) {
                        Key.DirectionDown -> isGridView && moveVertically(1)
                        Key.DirectionUp -> isGridView && moveVertically(-1)
                        Key.DirectionRight -> if (!isGridView && (folderChars.isNotEmpty() || fileChars.isNotEmpty())) {
                            showCharacterSelector = true
                            true
                        } else false
                        else -> false
                    }
                },
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(if (isGridView) 10.dp else 0.dp)
        ) {
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
                    .onFocusChanged { if (it.hasFocus) focusedIndex[0] = index }
                val isFirstColumn = index % columns == 0
                val isFirstRow = index < columns
                val isRowEnd = isGridView && (index % columns == columns - 1 || index == items.lastIndex)
                if (isFirstColumn || isFirstRow || isRowEnd) {
                    // The lazy grid doesn't hand focus searches off its edges reliably, so the
                    // edges are explicit: left/up go to the toolbar, right stops at the row end.
                    itemModifier = itemModifier.focusProperties {
                        if (isFirstColumn) left = toolbarFocusRequester
                        if (isFirstRow) up = upFocusRequester
                        if (isRowEnd) right = FocusRequester.Cancel
                    }
                }
                itemContent(item, itemModifier)
            }
        }

        AnimatedVisibility(
            visible = showJumpChar,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier.align(Alignment.TopEnd).padding(48.dp)
        ) {
            Box(
                modifier = Modifier.wrapContentSize().sizeIn(minWidth = 100.dp, minHeight = 100.dp)
                    .background(Color.Black.copy(alpha = 0.8f), RoundedCornerShape(16.dp)).padding(24.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(text = jumpLabel, style = MaterialTheme.typography.displayLarge, color = Color.White)
            }
        }

        if (showCharacterSelector) {
            CharacterSelectorDialog(
                folderCharacters = folderChars,
                fileCharacters = fileChars,
                dialogFocusRequester = dialogFocusRequester,
                onCharacterSelected = { char, isFolder ->
                    showCharacterSelector = false
                    jumpToCharacter(char, isFolder)
                },
                onDismiss = { showCharacterSelector = false }
            )
        }
    }
}
