package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.unit.dp
import androidx.tv.material3.ExperimentalTvMaterial3Api
import com.vargon.weirdstreamer.SmbFileItem
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.delay

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun <T> FocusableList(
    items: List<T>,
    initialSelection: String?,
    isGridView: Boolean,
    itemContent: @Composable (T, Modifier) -> Unit
) {
    val focusRequesters = remember(items) { List(items.size) { FocusRequester() } }
    val gridState = rememberLazyGridState()
    val listState = rememberLazyListState()
    
    val initialIndex = remember(items, initialSelection) {
        if (initialSelection == null) 0
        else {
            items.indexOfFirst { item ->
                when (item) {
                    is String -> item == initialSelection
                    is SmbFileItem -> item.path == initialSelection || item.name == initialSelection
                    else -> false
                }
            }.let { if (it == -1) 0 else it }
        }
    }

    LaunchedEffect(items, initialIndex, isGridView) {
        if (items.isNotEmpty()) {
            val safeIndex = initialIndex.coerceIn(items.indices)
            if (isGridView) gridState.scrollToItem(safeIndex)
            else listState.scrollToItem(safeIndex)
            
            delay(100)
            awaitFrame()
            
            runCatching {
                focusRequesters[safeIndex].requestFocus()
            }
        }
    }

    if (isGridView) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(4),
            state = gridState,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            itemsIndexed(items) { index, item ->
                itemContent(item, Modifier.focusRequester(focusRequesters[index]))
            }
        }
    } else {
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
            itemsIndexed(items) { index, item ->
                itemContent(item, Modifier.focusRequester(focusRequesters[index]))
            }
        }
    }
}
