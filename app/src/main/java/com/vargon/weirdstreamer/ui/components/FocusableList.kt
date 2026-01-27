package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed as gridItemsIndexed
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
    val focusRequesters = remember(items) { List(items.size) { FocusRequester() } }
    val gridState = rememberLazyGridState()
    val listState = rememberLazyListState()
    val dialogFocusRequester = remember { FocusRequester() }

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
    var showCharacterSelector by remember { mutableStateOf(false) }

    // Notify parent when dialog state changes
    LaunchedEffect(showCharacterSelector) {
        onRequestDialogFocus(if (showCharacterSelector) dialogFocusRequester else null)
    }

    // Debug: Track state changes
    LaunchedEffect(showCharacterSelector) {
        android.util.Log.d("FocusableList", "LaunchedEffect: showCharacterSelector changed to: $showCharacterSelector")
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

    // Get unique first characters for character selector - separated by folders and files
    val (folderChars, fileChars) = remember(items) {
        val folders = mutableSetOf<Char>()
        val files = mutableSetOf<Char>()

        items.forEach { item ->
            val firstChar = getFirstChar(item)?.uppercaseChar()
            if (firstChar != null) {
                if (isDirectory(item)) {
                    folders.add(firstChar)
                } else {
                    files.add(firstChar)
                }
            }
        }

        Pair(folders.sorted(), files.sorted())
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

    // Handle character selection from dialog
    fun jumpToCharacter(char: Char, isFolder: Boolean) {
        val targetIndex = items.indexOfFirst { item ->
            val matchesChar = getFirstChar(item)?.uppercaseChar() == char.uppercaseChar()
            val matchesType = isDirectory(item) == isFolder
            matchesChar && matchesType
        }

        if (targetIndex != -1) {
            jumpLabel = char.toString().uppercase()
            showJumpChar = true
            scope.launch {
                if (isGridView) {
                    gridState.scrollToItem(targetIndex)
                } else {
                    listState.scrollToItem(targetIndex)
                }
                delay(50)
                awaitFrame()
                focusRequesters[targetIndex].requestFocus()
            }
        }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        if (isGridView) {
            LazyVerticalGrid(
                columns = GridCells.Fixed(4),
                state = gridState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(8.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                gridItemsIndexed(items) { index, item ->
                    itemContent(item, Modifier
                        .focusRequester(focusRequesters[index])
                        .onFocusChanged { if (it.isFocused) focusedIndex = index }
                    )
                }
            }
        } else {
            // List view - handle Left/Right for toolbar focus and character dialog
            LazyColumn(
                state = listState,
                modifier = Modifier
                    .fillMaxSize()
                    .onPreviewKeyEvent { event ->
                        if (event.type == KeyEventType.KeyDown && items.isNotEmpty()) {
                            when (event.key) {
                                Key.DirectionLeft -> {
                                    // Move focus to toolbar
                                    android.util.Log.d("FocusableList", "Left arrow pressed - requesting toolbar focus")
                                    onRequestToolbarFocus()
                                    true
                                }
                                Key.DirectionRight -> {
                                    // Show character selector dialog
                                    val hasChars = folderChars.isNotEmpty() || fileChars.isNotEmpty()
                                    android.util.Log.d("FocusableList", "Right arrow pressed - folderChars: ${folderChars.size}, fileChars: ${fileChars.size}, hasChars: $hasChars")
                                    if (hasChars) {
                                        android.util.Log.d("FocusableList", "Setting showCharacterSelector = true")
                                        showCharacterSelector = true
                                        true
                                    } else {
                                        android.util.Log.d("FocusableList", "No characters available, not showing dialog")
                                        false
                                    }
                                }
                                else -> false
                            }
                        } else {
                            false
                        }
                    }
            ) {
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

        // Character Selector Dialog - INSIDE Box for proper z-ordering on TV
        android.util.Log.d("FocusableList", "Checking showCharacterSelector: $showCharacterSelector")
        if (showCharacterSelector) {
            android.util.Log.d("FocusableList", "Showing CharacterSelectorDialog - folders: ${folderChars.size}, files: ${fileChars.size}")
            CharacterSelectorDialog(
                folderCharacters = folderChars,
                fileCharacters = fileChars,
                dialogFocusRequester = dialogFocusRequester,
                onCharacterSelected = { char, isFolder ->
                    android.util.Log.d("FocusableList", "Character selected: $char, isFolder: $isFolder")
                    showCharacterSelector = false
                    jumpToCharacter(char, isFolder)
                },
                onDismiss = {
                    android.util.Log.d("FocusableList", "Dialog dismissed")
                    showCharacterSelector = false
                }
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
    android.util.Log.d("CharacterSelectorDialog", "Dialog composing - folders: ${folderCharacters.size}, files: ${fileCharacters.size}")

    // Create focus requesters for both sections
    val allCharacters = folderCharacters + fileCharacters
    val focusRequesters = remember(folderCharacters, fileCharacters) {
        List(allCharacters.size) { FocusRequester() }
    }

    LaunchedEffect(Unit) {
        android.util.Log.d("CharacterSelectorDialog", "LaunchedEffect: Attempting to focus first character")
        delay(100)
        if (focusRequesters.isNotEmpty()) {
            focusRequesters[0].requestFocus()
            android.util.Log.d("CharacterSelectorDialog", "Focus requested on first character")
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.85f))
            .focusRequester(dialogFocusRequester)
            .onPreviewKeyEvent { event ->
                // Handle both KeyDown and KeyUp to prevent propagation
                when (event.key) {
                    Key.Back, Key.Backspace -> {
                        if (event.type == KeyEventType.KeyDown) {
                            onDismiss()
                        }
                        true // Always consume Back/Backspace events
                    }
                    Key.DirectionUp -> {
                        // If we're on the first row, allow focus to move to toolbar
                        false
                    }
                    else -> false
                }
            },
        contentAlignment = Alignment.Center
    ) {
        Surface(
            shape = RoundedCornerShape(16.dp),
            colors = SurfaceDefaults.colors(
                containerColor = Color(0xFF2D2D2D),
                contentColor = Color.White
            ),
            modifier = Modifier
                .fillMaxWidth(0.85f)
                .fillMaxHeight(0.9f)
                .padding(24.dp)
        ) {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(24.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text(
                    text = "Jump to Section",
                    style = MaterialTheme.typography.headlineMedium,
                    modifier = Modifier.padding(bottom = 16.dp)
                )

                // Scrollable column with both sections
                LazyColumn(
                    modifier = Modifier.weight(1f),
                    contentPadding = PaddingValues(8.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    // Folders section
                    if (folderCharacters.isNotEmpty()) {
                        item {
                            Column {
                                Text(
                                    text = "Folders",
                                    style = MaterialTheme.typography.titleMedium,
                                    color = Color(0xFFFFD700), // Gold color for folders
                                    modifier = Modifier.padding(bottom = 8.dp)
                                )

                                // Grid of folder characters
                                val folderRows = (folderCharacters.size + 6) / 7
                                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                    for (row in 0 until folderRows) {
                                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                            for (col in 0 until 7) {
                                                val index = row * 7 + col
                                                if (index < folderCharacters.size) {
                                                    val char = folderCharacters[index]
                                                    val focusRequester = focusRequesters[index]

                                                    Button(
                                                        onClick = {
                                                            onCharacterSelected(char, true)
                                                            onDismiss()
                                                        },
                                                        modifier = Modifier
                                                            .size(64.dp)
                                                            .focusRequester(focusRequester)
                                                    ) {
                                                        Box(
                                                            modifier = Modifier.fillMaxSize(),
                                                            contentAlignment = Alignment.Center
                                                        ) {
                                                            Text(
                                                                text = char.toString(),
                                                                style = MaterialTheme.typography.titleLarge,
                                                                textAlign = TextAlign.Center
                                                            )
                                                        }
                                                    }
                                                } else {
                                                    // Empty spacer
                                                    Spacer(modifier = Modifier.size(64.dp))
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }

                    // Files section
                    if (fileCharacters.isNotEmpty()) {
                        item {
                            Column {
                                Text(
                                    text = "Files",
                                    style = MaterialTheme.typography.titleMedium,
                                    color = Color(0xFF87CEEB), // Sky blue color for files
                                    modifier = Modifier.padding(bottom = 8.dp)
                                )

                                // Grid of file characters
                                val fileRows = (fileCharacters.size + 6) / 7
                                val folderOffset = folderCharacters.size
                                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                    for (row in 0 until fileRows) {
                                        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                                            for (col in 0 until 7) {
                                                val index = row * 7 + col
                                                if (index < fileCharacters.size) {
                                                    val char = fileCharacters[index]
                                                    val focusRequester = focusRequesters[folderOffset + index]

                                                    Button(
                                                        onClick = {
                                                            onCharacterSelected(char, false)
                                                            onDismiss()
                                                        },
                                                        modifier = Modifier
                                                            .size(64.dp)
                                                            .focusRequester(focusRequester)
                                                    ) {
                                                        Box(
                                                            modifier = Modifier.fillMaxSize(),
                                                            contentAlignment = Alignment.Center
                                                        ) {
                                                            Text(
                                                                text = char.toString(),
                                                                style = MaterialTheme.typography.titleLarge,
                                                                textAlign = TextAlign.Center
                                                            )
                                                        }
                                                    }
                                                } else {
                                                    // Empty spacer
                                                    Spacer(modifier = Modifier.size(64.dp))
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                Spacer(modifier = Modifier.height(16.dp))

                Text(
                    text = "Press Back to cancel",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color.White.copy(alpha = 0.7f)
                )
            }
        }
    }
}
