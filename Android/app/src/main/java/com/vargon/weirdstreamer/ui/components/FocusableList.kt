package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
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
import androidx.compose.ui.text.style.TextAlign
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
    onRequestToolbarFocus: () -> Unit = {},
    onRequestDialogFocus: (FocusRequester?) -> Unit = {},
    itemContent: @Composable (T, Modifier) -> Unit
) {
    val scope = rememberCoroutineScope()
    val columns = 4
    val focusRequesters = remember { mutableMapOf<String, FocusRequester>() }
    val listState = rememberLazyListState()
    val dialogFocusRequester = remember { FocusRequester() }

    val initialIndex = remember(initialSelection) {
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

    var showJumpChar by remember { mutableStateOf(false) }
    var jumpLabel by remember { mutableStateOf("") }
    var showCharacterSelector by remember { mutableStateOf(false) }
    var focusedIndex by remember(initialSelection) { mutableIntStateOf(initialIndex) }
    var navigationJob by remember { mutableStateOf<kotlinx.coroutines.Job?>(null) }

    LaunchedEffect(showCharacterSelector) {
        onRequestDialogFocus(if (showCharacterSelector) dialogFocusRequester else null)
    }

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

    val (folderChars, fileChars) = remember(items) {
        val folders = mutableSetOf<Char>()
        val files = mutableSetOf<Char>()
        items.forEach { item ->
            val firstChar = getFirstChar(item)?.uppercaseChar()
            if (firstChar != null) {
                if (isDirectory(item)) folders.add(firstChar) else files.add(firstChar)
            }
        }
        Pair(folders.sorted(), files.sorted())
    }

    LaunchedEffect(initialSelection, isGridView) {
        if (items.isNotEmpty()) {
            val safeIndex = initialIndex.coerceIn(items.indices)
            if (isGridView) listState.scrollToItem(safeIndex / columns) else listState.scrollToItem(safeIndex)
            delay(100); awaitFrame()
            focusRequesters.getOrPut(getName(items[safeIndex])) { FocusRequester() }.requestFocus()
        }
    }

    fun jumpToCharacter(char: Char, isFolder: Boolean) {
        val targetIndex = items.indexOfFirst { item ->
            getFirstChar(item)?.uppercaseChar() == char.uppercaseChar() && isDirectory(item) == isFolder
        }
        if (targetIndex != -1) {
            jumpLabel = char.toString().uppercase(); showJumpChar = true
            scope.launch {
                if (isGridView) listState.scrollToItem(targetIndex / columns) else listState.scrollToItem(targetIndex)
                delay(50); awaitFrame()
                focusRequesters.getOrPut(getName(items[targetIndex])) { FocusRequester() }.requestFocus()
            }
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        if (isGridView) {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(8.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                items((items.size + columns - 1) / columns) { row ->
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        repeat(columns) { column ->
                            val index = row * columns + column
                            if (index < items.size) {
                                val item = items[index]
                                val key = getName(item)
                                val requester = focusRequesters.getOrPut(key) { FocusRequester() }
                                Box(Modifier.width(200.dp).height(200.dp)) {
                                    itemContent(item, Modifier
                                        .focusRequester(requester)
                                        .onPreviewKeyEvent { event ->
                                            if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                                            val target = when (event.key) {
                                                Key.DirectionLeft -> if (column > 0) index - 1 else -1
                                                Key.DirectionRight -> if (column < columns - 1) index + 1 else -1
                                                Key.DirectionUp -> index - columns
                                                Key.DirectionDown -> index + columns
                                                else -> return@onPreviewKeyEvent false
                                            }
                                            if (target < 0) {
                                                if (event.key == Key.DirectionLeft) onRequestToolbarFocus()
                                                true
                                            } else if (target < items.size) {
                                                focusedIndex = target
                                                navigationJob?.cancel()
                                                navigationJob = scope.launch {
                                                    val targetKey = getName(items[target])
                                                    if (target / columns != index / columns) listState.scrollToItem(target / columns)
                                                    focusRequesters.getOrPut(targetKey) { FocusRequester() }.requestFocus()
                                                }
                                                true
                                            } else true
                                        }
                                        .onFocusChanged { if (it.isFocused) focusedIndex = index }
                                    )
                                }
                            } else {
                                Spacer(Modifier.width(200.dp).height(200.dp))
                            }
                        }
                    }
                }
            }
        } else {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize()
                    .onPreviewKeyEvent { event ->
                        if (event.type == KeyEventType.KeyDown && items.isNotEmpty()) {
                            when (event.key) {
                                Key.DirectionLeft -> { onRequestToolbarFocus(); true }
                                Key.DirectionRight -> {
                                    if (folderChars.isNotEmpty() || fileChars.isNotEmpty()) { showCharacterSelector = true; true } 
                                    else false
                                }
                                else -> false
                            }
                        } else false
                    }
            ) {
                itemsIndexed(
                    items = items,
                    key = { _, item ->
                        when (item) {
                            is SmbFileItem -> item.path
                            is com.vargon.weirdstreamer.repository.FileItem -> item.path
                            else -> item.hashCode()
                        }
                    }
                ) { index, item ->
                    val key = getName(item)
                    itemContent(item, Modifier.focusRequester(focusRequesters.getOrPut(key) { FocusRequester() }))
                }
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
                Text(text = jumpLabel, style = if (jumpLabel.length > 1) MaterialTheme.typography.headlineMedium else MaterialTheme.typography.displayLarge, color = Color.White)
            }
        }

        if (showCharacterSelector) {
            CharacterSelectorDialog(
                folderCharacters = folderChars,
                fileCharacters = fileChars,
                dialogFocusRequester = dialogFocusRequester,
                onCharacterSelected = { char, isFolder -> showCharacterSelector = false; jumpToCharacter(char, isFolder) },
                onDismiss = { showCharacterSelector = false }
            )
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun CharacterSelectorDialog(
    folderCharacters: List<Char>,
    fileCharacters: List<Char>,
    dialogFocusRequester: FocusRequester,
    onCharacterSelected: (Char, Boolean) -> Unit,
    onDismiss: () -> Unit
) {
    BackHandler { onDismiss() }
    val allCharacters = folderCharacters + fileCharacters
    val focusRequesters = remember(folderCharacters, fileCharacters) { List(allCharacters.size) { FocusRequester() } }
    LaunchedEffect(Unit) { delay(100); if (focusRequesters.isNotEmpty()) focusRequesters[0].requestFocus() }

    Box(
        modifier = Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.85f))
            .focusRequester(dialogFocusRequester)
            .onPreviewKeyEvent { event ->
                when (event.key) {
                    Key.Back, Key.Backspace -> { if (event.type == KeyEventType.KeyDown) onDismiss(); true }
                    else -> false
                }
            },
        contentAlignment = Alignment.Center
    ) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            colors = SurfaceDefaults.colors(containerColor = Color(0xFF2D2D2D), contentColor = Color.White),
            modifier = Modifier.fillMaxWidth(0.85f).fillMaxHeight(0.9f).padding(24.dp)
        ) {
            Column(modifier = Modifier.fillMaxSize().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Text(text = "Jump to Section", style = MaterialTheme.typography.headlineMedium, modifier = Modifier.padding(bottom = 16.dp))
                LazyColumn(modifier = Modifier.weight(1f), contentPadding = PaddingValues(8.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                    if (folderCharacters.isNotEmpty()) {
                        item {
                            Column {
                                Text(text = "Folders", style = MaterialTheme.typography.titleMedium, color = Color(0xFFFFD700), modifier = Modifier.padding(bottom = 8.dp))
                                val folderRows = (folderCharacters.size + 6) / 7
                                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                    for (row in 0 until folderRows) {
                                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                            for (col in 0 until 7) {
                                                val idx = row * 7 + col
                                                if (idx < folderCharacters.size) {
                                                    Button(colors = tvButtonColors(), onClick = { onCharacterSelected(folderCharacters[idx], true); onDismiss() }, modifier = Modifier.size(64.dp).focusRequester(focusRequesters[idx])) {
                                                        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { Text(text = folderCharacters[idx].toString(), style = MaterialTheme.typography.titleLarge, textAlign = TextAlign.Center) }
                                                    }
                                                } else Spacer(modifier = Modifier.size(64.dp))
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                    if (fileCharacters.isNotEmpty()) {
                        item {
                            Column {
                                Text(text = "Files", style = MaterialTheme.typography.titleMedium, color = Color(0xFF87CEEB), modifier = Modifier.padding(bottom = 8.dp))
                                val fileRows = (fileCharacters.size + 6) / 7
                                val folderOffset = folderCharacters.size
                                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                    for (row in 0 until fileRows) {
                                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                            for (col in 0 until 7) {
                                                val idx = row * 7 + col
                                                if (idx < fileCharacters.size) {
                                                    Button(colors = tvButtonColors(), onClick = { onCharacterSelected(fileCharacters[idx], false); onDismiss() }, modifier = Modifier.size(64.dp).focusRequester(focusRequesters[folderOffset + idx])) {
                                                        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { Text(text = fileCharacters[idx].toString(), style = MaterialTheme.typography.titleLarge, textAlign = TextAlign.Center) }
                                                    }
                                                } else Spacer(modifier = Modifier.size(64.dp))
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                Spacer(modifier = Modifier.height(16.dp))
                Text(text = "Press Back to cancel", style = MaterialTheme.typography.bodyMedium, color = Color.White.copy(alpha = 0.7f))
            }
        }
    }
}
