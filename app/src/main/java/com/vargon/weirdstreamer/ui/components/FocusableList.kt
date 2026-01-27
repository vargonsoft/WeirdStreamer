package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.*
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.SmbFileItem
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun <T> FocusableList(
    items: List<T>,
    initialSelection: String?,
    isGridView: Boolean,
    itemContent: @Composable (T, Modifier) -> Unit
) {
    val scope = rememberCoroutineScope()
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
                    is com.vargon.weirdstreamer.repository.FileItem -> item.path == initialSelection || item.name == initialSelection
                    else -> false
                }
            }.let { if (it == -1) 0 else it }
        }
    }

    var focusedIndex by remember(items) { mutableIntStateOf(initialIndex) }
    var showJumpChar by remember { mutableStateOf(false) }
    var jumpLabel by remember { mutableStateOf("") }

    LaunchedEffect(showJumpChar) {
        if (showJumpChar) {
            delay(1500)
            showJumpChar = false
        }
    }

    fun isDirectory(item: T): Boolean = when (item) {
        is SmbFileItem -> item.isDirectory
        is com.vargon.weirdstreamer.repository.FileItem -> item.isDirectory
        else -> false
    }

    fun getName(item: T): String = when (item) {
        is String -> item
        is SmbFileItem -> item.name
        is com.vargon.weirdstreamer.repository.FileItem -> item.name
        else -> ""
    }

    fun getFirstChar(item: T): Char? {
        val name = getName(item)
        if (name == "..") return null
        return name.firstOrNull()?.lowercaseChar()
    }

    LaunchedEffect(items, initialIndex, isGridView) {
        if (items.isNotEmpty()) {
            val safeIndex = initialIndex.coerceIn(items.indices)
            if (isGridView) gridState.scrollToItem(safeIndex)
            else listState.scrollToItem(safeIndex)
            
            delay(100)
            awaitFrame()
            
            runCatching {
                if (safeIndex in focusRequesters.indices) {
                    focusRequesters[safeIndex].requestFocus()
                }
            }
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        val listModifier = Modifier
            .fillMaxSize()
            .onPreviewKeyEvent { event ->
                if (!isGridView && event.type == KeyEventType.KeyDown && items.isNotEmpty()) {
                    when (event.key) {
                        Key.DirectionRight -> {
                            val currentItem = items.getOrNull(focusedIndex) ?: return@onPreviewKeyEvent false
                            val currentChar = getFirstChar(currentItem)
                            
                            val targetIndex = (focusedIndex + 1 until items.size).firstOrNull { 
                                val char = getFirstChar(items[it])
                                char != null && char != currentChar
                            }
                            
                            if (targetIndex != null) {
                                val targetChar = getFirstChar(items[targetIndex])
                                if (targetChar != null) {
                                    jumpLabel = targetChar.toString().uppercase()
                                    showJumpChar = true
                                    scope.launch {
                                        listState.scrollToItem(targetIndex)
                                        delay(50)
                                        awaitFrame()
                                        focusRequesters[targetIndex].requestFocus()
                                    }
                                    return@onPreviewKeyEvent true
                                }
                            }
                        }
                        Key.DirectionLeft -> {
                            val currentItem = items.getOrNull(focusedIndex) ?: return@onPreviewKeyEvent false
                            val currentChar = getFirstChar(currentItem)
                            
                            val prevDiffIndex = (focusedIndex - 1 downTo 0).firstOrNull {
                                val char = getFirstChar(items[it])
                                char != null && char != currentChar
                            }
                            
                            if (prevDiffIndex != null) {
                                val targetChar = getFirstChar(items[prevDiffIndex])
                                if (targetChar != null) {
                                    val firstOfGroup = (0..prevDiffIndex).first {
                                        getFirstChar(items[it]) == targetChar
                                    }
                                    jumpLabel = targetChar.toString().uppercase()
                                    showJumpChar = true
                                    scope.launch {
                                        listState.scrollToItem(firstOfGroup)
                                        delay(50)
                                        awaitFrame()
                                        focusRequesters[firstOfGroup].requestFocus()
                                    }
                                    return@onPreviewKeyEvent true
                                }
                            }
                        }
                    }
                }
                false
            }

        if (isGridView) {
            LazyVerticalGrid(
                columns = GridCells.Fixed(4),
                state = gridState,
                modifier = Modifier.fillMaxSize(), // Grid doesn't use jump logic
                contentPadding = PaddingValues(8.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                itemsIndexed(items) { index, item ->
                    itemContent(item, Modifier
                        .focusRequester(focusRequesters[index])
                        .onFocusChanged { if (it.isFocused) focusedIndex = index }
                    )
                }
            }
        } else {
            LazyColumn(state = listState, modifier = listModifier) {
                itemsIndexed(items) { index, item ->
                    itemContent(item, Modifier
                        .focusRequester(focusRequesters[index])
                        .onFocusChanged { if (it.isFocused) focusedIndex = index }
                    )
                }
            }
        }

        // Jump Character Popup
        AnimatedVisibility(
            visible = showJumpChar,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(48.dp)
        ) {
            Box(
                modifier = Modifier
                    .wrapContentSize()
                    .sizeIn(minWidth = 100.dp, minHeight = 100.dp)
                    .background(Color.Black.copy(alpha = 0.8f), RoundedCornerShape(16.dp))
                    .padding(24.dp),
                contentAlignment = Alignment.Center
            ) {
                Text(
                    text = jumpLabel,
                    style = if (jumpLabel.length > 1) MaterialTheme.typography.headlineMedium else MaterialTheme.typography.displayLarge,
                    color = Color.White
                )
            }
        }
    }
}
