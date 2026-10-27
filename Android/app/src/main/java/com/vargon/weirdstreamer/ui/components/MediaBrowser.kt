package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.DisposableEffect
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ViewList
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.FilterList
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.PictureAsPdf
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.VideoLibrary
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.TextField
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.withFrameNanos
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import kotlinx.coroutines.delay
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Button
import androidx.tv.material3.ExperimentalTvMaterial3Api
import androidx.tv.material3.Icon
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.ContentFilter
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.repository.FileItem
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.foundation.layout.heightIn
import androidx.compose.ui.graphics.takeOrElse
import com.vargon.weirdstreamer.repository.AddressCheck
import com.vargon.weirdstreamer.repository.SourceProvider
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun MediaBrowser(viewModel: MediaViewModel, onOpenSettings: () -> Unit) {
    var showMainAddDialog by remember { mutableStateOf(false) }
    var showAuthDialog by remember { mutableStateOf(false) }
    var showRegenerateThumbnailsDialog by remember { mutableStateOf(false) }
    val toolbarFocusRequester = remember { FocusRequester() }
    // Up and left off the content's edges go to the Up button; Down from the toolbar goes back in.
    val upFocusRequester = remember { FocusRequester() }
    val contentEntry = remember { ContentEntry() }

    val currentSource = viewModel.currentSource

    val goBack: () -> Unit = {
        when {
            viewModel.openTitle != null -> viewModel.closeLibraryTitle()
            viewModel.isLibraryView -> viewModel.leaveLibrary()
            else -> viewModel.navigateUp()
        }
    }
    BackHandler(enabled = currentSource != null) {
        goBack()
    }

    Box(modifier = Modifier.fillMaxSize()) {
        // Background image from theme spec
        val themeSpec = WeirdStreamerTheme.spec
        val backdropTitle = viewModel.openTitle?.takeIf { viewModel.isLibraryView && it.backdrop != null }
        if (backdropTitle == null) themeSpec.backgroundImage?.let { bgRes ->
            Image(
                painter = painterResource(id = bgRes),
                contentDescription = null,
                modifier = Modifier
                    .fillMaxSize()
                    .align(Alignment.CenterEnd)
                    .offset { IntOffset(125.dp.roundToPx(), 0) },
                // Alpha on the draw call rather than Modifier.alpha, which needs an offscreen layer.
                alpha = 0.10f,
                contentScale = ContentScale.Crop,
                alignment = Alignment.CenterEnd
            )
        }
        backdropTitle?.let { TitleBackdrop(viewModel, it) }
        // The boxes let a title's backdrop show through (their contents stay opaque).
        val panelAlpha = if (backdropTitle != null) 0.5f else 1f
        // The wordmark and the source on the clock's line, left; the source list shows the
        // wordmark large instead.
        if (currentSource != null) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.align(Alignment.TopStart).padding(top = 14.dp, start = 48.dp)
            ) {
                Wordmark(MaterialTheme.colorScheme.onSurface, Modifier.height(20.dp))
                Spacer(Modifier.width(16.dp))
                Text(
                    text = viewModel.headerLine(currentSource),
                    style = MaterialTheme.typography.titleMedium, // as the clock's
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        }
        Clock(Modifier.align(Alignment.TopEnd).padding(top = 14.dp, end = 52.dp))

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(start = 48.dp, top = 48.dp, end = 48.dp)
                .onPreviewKeyEvent { 
                    if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                        if (currentSource != null) {
                            goBack()
                            true
                        } else false
                    } else false
                }
        ) {
            // Header (in a box, in themes that draw them)
            Panel(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp), alpha = panelAlpha) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown && event.key == Key.DirectionDown) {
                        contentEntry.focusTop?.invoke() ?: false
                    } else false
                }
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    if (currentSource != null) {
                        val display = when {
                            viewModel.openTitle != null -> viewModel.openTitle!!.title
                            viewModel.isLibraryView -> viewModel.libraryListing?.libraryName ?: "Library"
                            else -> viewModel.currentPathDisplayName ?: if (viewModel.currentPath.isNotEmpty()) viewModel.currentPath else "/"
                        }
                        TextWithStartEllipsis(
                            text = display,
                            style = MaterialTheme.typography.headlineMedium,
                            color = MaterialTheme.colorScheme.onSurface,
                            modifier = Modifier.fillMaxWidth().padding(end = 16.dp)
                        )
                    } else {
                        Wordmark(MaterialTheme.colorScheme.onSurface, Modifier.height(60.dp))
                    }
                }
                
                if (currentSource != null) {
                    Button(colors = tvButtonColors(),
                        onClick = { viewModel.returnToSourceSelection() },
                        modifier = Modifier.focusRequester(toolbarFocusRequester)
                    ) {
                        Icon(Icons.Default.Home, contentDescription = "Back to Servers")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                    // Replaces the ".." entry: up a folder, or back out of a title or library.
                    Button(colors = tvButtonColors(), onClick = goBack, modifier = Modifier.focusRequester(upFocusRequester)) {
                        Icon(Icons.Default.ArrowUpward, contentDescription = "Up")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                } else {
                    Button(colors = tvButtonColors(),
                        onClick = { showMainAddDialog = true },
                        modifier = Modifier.focusRequester(toolbarFocusRequester)
                    ) {
                        Icon(Icons.Default.Add, contentDescription = "Add Source")
                        Spacer(modifier = Modifier.width(8.dp))
                        Text("Add Source")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                }

                Button(colors = tvButtonColors(), onClick = { viewModel.toggleViewMode() }) {
                    // Shows the mode it switches to: list -> grid -> library (movie/tv libraries) -> list
                    Icon(
                        when {
                            viewModel.isLibraryView -> Icons.AutoMirrored.Filled.ViewList
                            !viewModel.isGridView -> Icons.Default.GridView
                            viewModel.libraryType != null -> Icons.Default.VideoLibrary
                            else -> Icons.AutoMirrored.Filled.ViewList
                        },
                        contentDescription = "Switch View"
                    )
                }
                Spacer(modifier = Modifier.width(16.dp))

                if (currentSource != null) {
                    if (viewModel.supportsThumbnailRegeneration) {
                        Button(colors = tvButtonColors(), onClick = { showRegenerateThumbnailsDialog = true }) {
                            Icon(Icons.Default.Refresh, contentDescription = "Regenerate Thumbnails")
                        }
                        Spacer(modifier = Modifier.width(16.dp))
                    }

                    if (!viewModel.isLibraryView) Button(colors = tvButtonColors(), onClick = { viewModel.toggleContentFilter() }) {
                        Icon(
                            when (viewModel.contentFilter) {
                                ContentFilter.ALL -> Icons.Default.FilterList
                                ContentFilter.IMAGE -> Icons.Default.Image
                                ContentFilter.VIDEO -> Icons.Default.VideoLibrary
                                ContentFilter.PDF -> Icons.Default.PictureAsPdf
                            },
                            contentDescription = "Content Filter"
                        )
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                }

                Button(colors = tvButtonColors(), onClick = onOpenSettings) {
                    Icon(Icons.Default.Settings, contentDescription = "Settings")
                }
            }
            }

            Spacer(modifier = Modifier.height(24.dp))

            // Content area: a box to the bottom (the source list boxes each server on its own instead)
            val serverList = currentSource == null && !viewModel.isLoading && viewModel.errorMessage == null &&
                viewModel.authRequiredSource == null
            Panel(
                modifier = Modifier.weight(1f).fillMaxWidth(),
                margin = PaddingValues(bottom = 24.dp),
                contentPadding = PaddingValues(if (serverList) 0.dp else 16.dp),
                alpha = if (serverList) 0f else panelAlpha
            ) {
            when {
                viewModel.isLoading -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Text("Loading...", style = MaterialTheme.typography.displaySmall)
                    }
                }
                viewModel.errorMessage != null -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(text = "Error: ${viewModel.errorMessage}", color = Color.Red)
                            Spacer(modifier = Modifier.height(16.dp))
                            Button(colors = tvButtonColors(), onClick = { 
                                currentSource?.let { viewModel.loadPath(it, viewModel.currentPath) }
                            }) {
                                Text("Retry")
                            }
                        }
                    }
                }
                viewModel.authRequiredSource != null -> {
                    LaunchedEffect(Unit) { showAuthDialog = true }
                }
                currentSource == null -> {
                    SourceSelectionScreen(viewModel)
                }
                viewModel.isLibraryView -> {
                    val title = viewModel.openTitle
                    if (title != null) {
                        TitleScreen(viewModel, title)
                    } else {
                        LibraryScreen(viewModel, upFocusRequester, contentEntry)
                    }
                }
                else -> {
                    FileBrowserScreen(
                        viewModel = viewModel,
                        toolbarFocusRequester = upFocusRequester,
                        contentEntry = contentEntry
                    )
                }
            }
            }
        }
    }

    // Main Add Source Dialog
    if (showMainAddDialog) {
        MainAddSourceDialog(
            providers = viewModel.sourceProviders,
            onDismiss = { showMainAddDialog = false },
            onAdd = { provider, address ->
                showMainAddDialog = false
                viewModel.addServer(provider, address)
            }
        )
    }

    // Auth Dialog
    if (showAuthDialog && viewModel.authRequiredSource != null) {
        val source = viewModel.authRequiredSource!!
        AuthDialog(
            host = source.displayName(),
            initialUsername = viewModel.authUsername,
            title = viewModel.authTitle,
            hint = if (source.typeId == "SMB") "Leave both blank for guest access." else null,
            onDismiss = { 
                showAuthDialog = false
                viewModel.returnToSourceSelection()
            },
            onLogin = { user, pass, save ->
                showAuthDialog = false
                viewModel.loadPath(source, viewModel.currentPath, user, pass, save)
            }
        )
    }

    // Regenerate Thumbnails Dialog
    if (showRegenerateThumbnailsDialog) {
        val focusRequester = remember { FocusRequester() }
        androidx.compose.ui.window.Dialog(onDismissRequest = { showRegenerateThumbnailsDialog = false }) {
            Surface(
                modifier = Modifier.width(400.dp),
                shape = MaterialTheme.shapes.medium
            ) {
                Column(modifier = Modifier.padding(24.dp)) {
                    Text("Regenerate Thumbnails", style = MaterialTheme.typography.headlineSmall)
                    Spacer(modifier = Modifier.height(16.dp))
                    Text("Are you sure you want to regenerate all thumbnails in this folder? This may take some time.")
                    Spacer(modifier = Modifier.height(24.dp))
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        Button(colors = tvButtonColors(), onClick = { showRegenerateThumbnailsDialog = false }) {
                            Text("Cancel")
                        }
                        Spacer(modifier = Modifier.width(16.dp))
                        Button(colors = tvButtonColors(),
                            onClick = {
                                showRegenerateThumbnailsDialog = false
                                viewModel.regenerateThumbnails()
                            },
                            modifier = Modifier.focusRequester(focusRequester)
                        ) {
                            Text("Regenerate")
                        }
                    }
                }
            }
        }
        LaunchedEffect(Unit) {
            focusRequester.requestFocus()
        }
    }
}

@Composable
private fun SourceSelectionScreen(viewModel: MediaViewModel) {
    // Reading saved servers hits EncryptedSharedPreferences; do it once per auth/server change,
    // not on every recomposition.
    val refreshTrigger = viewModel.authStateRefreshTrigger
    val serversByProvider = remember(viewModel.sourceProviders, refreshTrigger) {
        viewModel.sourceProviders.associateWith { it.getServers() }
    }
    // The accounts signed in to each server, for its row.
    val accounts = remember(serversByProvider, refreshTrigger) {
        serversByProvider.flatMap { (p, list) -> list.map { p to it } }.associateWith { (p, a) -> viewModel.accountsOf(p, a) }
    }
    // Focus starts on the server opened last (on the account in use), else the first saved server
    // (the WeirdStreamer servers come first).
    val firstServer = viewModel.lastServer?.let { last -> serversByProvider.entries.firstOrNull { last in it.value }?.let { it.key to last } }
        ?: viewModel.sourceProviders.firstNotNullOfOrNull { p -> serversByProvider[p].orEmpty().firstOrNull()?.let { p to it } }
    val focusAccount = firstServer?.let { (p, a) -> viewModel.accountInUse(p, a) }
    val firstRequester = remember { FocusRequester() }
    val first = { provider: SourceProvider, address: String ->
        if (firstServer == provider to address) Modifier.focusRequester(firstRequester) else Modifier
    }
    LaunchedEffect(firstServer, viewModel.isGridView) {
        if (firstServer != null) runCatching { firstRequester.requestFocus() }
    }
    // One list of every server, WeirdStreamer servers first; each card says which kind it is.
    val servers = viewModel.sourceProviders.flatMap { p -> serversByProvider[p].orEmpty().map { p to it } }
    LazyColumn(modifier = Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        if (servers.isEmpty()) item {
            Panel(Modifier.fillMaxWidth()) {
                Text("No servers yet. Use Add Source to add one.", style = MaterialTheme.typography.bodyMedium, color = Color.Gray)
            }
        }
        itemsIndexed(servers, key = { _, (p, a) -> "${p.typeId}:$a" }) { i, (provider, address) ->
            if (i > 0 && !hasPanels) HorizontalDivider() // themes without boxes
            Panel(Modifier.fillMaxWidth()) {
                TvServerAccountsRow(viewModel, provider, address, accounts[provider to address].orEmpty(), first(provider, address), focusAccount)
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
private fun MainAddSourceDialog(
    providers: List<SourceProvider>,
    onDismiss: () -> Unit,
    onAdd: (SourceProvider, String) -> Unit
) {
    var selectedProvider by remember { mutableStateOf<SourceProvider?>(null) }
    var address by remember { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }
    val initialFocusRequester = remember { FocusRequester() }
    val addRequester = remember { FocusRequester() }
    val keyboardController = LocalSoftwareKeyboardController.current
    // With Add highlighted, the address it will add (default port, and the scheme the server
    // answers on), worked out for the address as typed; Add uses it.
    var addFocused by remember { mutableStateOf(false) }
    var checked by remember { mutableStateOf<Pair<String, AddressCheck>?>(null) }
    LaunchedEffect(selectedProvider, addFocused, address) {
        val provider = selectedProvider ?: return@LaunchedEffect
        if (!addFocused || address.isBlank() || checked?.first == address) return@LaunchedEffect
        checked = null
        val typed = address
        checked = typed to provider.checkAddress(typed)
    }

    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        Surface(modifier = Modifier.width(500.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                if (selectedProvider == null) {
                    Text("Select Source Type", style = MaterialTheme.typography.headlineSmall)
                    Spacer(modifier = Modifier.height(16.dp))
                    providers.forEachIndexed { index, provider ->
                        Button(colors = tvButtonColors(),
                            onClick = { selectedProvider = provider },
                            modifier = Modifier
                                .fillMaxWidth()
                                .then(if (index == 0) Modifier.focusRequester(initialFocusRequester) else Modifier)
                        ) {
                            Text(provider.displayName)
                        }
                        Spacer(modifier = Modifier.height(8.dp))
                    }
                    Button(colors = tvButtonColors(), onClick = onDismiss, modifier = Modifier.align(Alignment.End)) {
                        Text("Cancel")
                    }
                } else {
                    val provider = selectedProvider!!
                    Text("Add ${provider.displayName}", style = MaterialTheme.typography.headlineSmall)
                    Spacer(modifier = Modifier.height(16.dp))
                    
                    TextField(
                        colors = tvTextFieldColors(),
                        value = address,
                        onValueChange = { address = it },
                        label = { FieldText(provider.addressLabel) },
                        placeholder = { FieldText(provider.addressPlaceholder) },
                        supportingText = provider.addressHint?.let { hint -> { FieldText(hint) } },
                        singleLine = true,
                        // Down from the address goes to Add, not the first button (Back). The field
                        // keeps D-pad keys for itself, so it's caught before it.
                        modifier = Modifier.fillMaxWidth().focusRequester(focusRequester)
                            .onPreviewKeyEvent {
                                if (it.key == Key.DirectionDown && it.type == KeyEventType.KeyDown) {
                                    runCatching { addRequester.requestFocus() }.isSuccess
                                } else false
                            },
                        keyboardOptions = addressKeyboard(ImeAction.Done),
                        // Done (the keyboard's ✓, Enter or OK) closes the keyboard and moves to Add.
                        keyboardActions = KeyboardActions(onDone = {
                            keyboardController?.hide()
                            runCatching { addRequester.requestFocus() }
                        })
                    )
                    
                    Spacer(modifier = Modifier.height(16.dp))
                    // What Add will add, once it's highlighted.
                    val check = checked?.takeIf { it.first == address }?.second
                    val muted = MaterialTheme.colorScheme.onSurfaceVariant
                    Text(
                        text = when {
                            !addFocused || address.isBlank() -> ""
                            check == null -> "Checking ${address.trim()}…"
                            check.answered == false -> "No server answered. Adds ${check.url}"
                            else -> "Adds ${check.url}"
                        },
                        style = MaterialTheme.typography.titleMedium,
                        color = if (check != null && check.answered != false)
                            WeirdStreamerTheme.spec.headingColor.takeOrElse { MaterialTheme.colorScheme.onSurface }
                        else muted,
                        maxLines = 2,
                        modifier = Modifier.heightIn(min = 24.dp)
                    )
                    Spacer(modifier = Modifier.height(16.dp))

                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        Button(colors = tvButtonColors(), onClick = { selectedProvider = null; address = "" }) { Text("Back") }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(colors = tvButtonColors(), onClick = onDismiss) { Text("Cancel") }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(colors = tvButtonColors(), onClick = {
                                if (address.isNotBlank()) onAdd(provider, check?.url ?: address)
                            },
                            modifier = Modifier.focusRequester(addRequester).onFocusChanged { addFocused = it.isFocused }) {
                            Text("Add")
                        }
                    }
                }
            }
        }
    }

    LaunchedEffect(selectedProvider) {
        if (selectedProvider == null) initialFocusRequester.requestFocus()
        else focusRequester.requestFocus()
    }
}

@Composable
private fun FileBrowserScreen(
    viewModel: MediaViewModel,
    toolbarFocusRequester: FocusRequester,
    contentEntry: ContentEntry
) {
    val isGridView = viewModel.isGridView
    val currentPath = viewModel.currentPath
    val onItemClick: (FileItem) -> Unit = remember(viewModel) {
        { item -> handleFileClick(viewModel, item) }
    }
    val onVisibleItemsChanged: (List<FileItem>) -> Unit = remember(viewModel) {
        { visible -> viewModel.requestMetadataFor(visible) }
    }
    // TV has an Up button on the toolbar instead of a ".." entry.
    val allItems = viewModel.getFilteredFileList()
    val items = remember(allItems) { allItems.filter { it.path != "UP" } }
    // The server's front page leads with what's in progress. The row sits above the grid: Down
    // from the toolbar lands on it, and Down from it on the grid.
    val showContinue = viewModel.continueItems.isNotEmpty()
    val continueRequester = remember { FocusRequester() }
    val gridEntry = remember { ContentEntry() }
    DisposableEffect(contentEntry, showContinue) {
        val focusTop: () -> Boolean = {
            if (showContinue) runCatching { continueRequester.requestFocus() }.isSuccess
            else gridEntry.focusTop?.invoke() ?: false
        }
        contentEntry.focusTop = focusTop
        onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
    }
    TvResumeDialog(viewModel)
    // A first visit waits for the row; a return goes straight back to what was selected.
    val settled = continueSettled(viewModel, currentPath)
    if (!settled && viewModel.getSelection(currentPath) == null) return
    // A card removed from Continue Watching took focus with it: onto the row's first card, or
    // the folder when the row is gone.
    LaunchedEffect(viewModel.continueRemovals) {
        if (viewModel.continueRemovals == 0) return@LaunchedEffect
        withFrameNanos { }
        if (!(showContinue && runCatching { continueRequester.requestFocus() }.isSuccess)) gridEntry.focusTop?.invoke()
    }
    run {
        BrowserGrid(
            folderKey = currentPath,
            items = items,
            isGridView = isGridView,
            initialSelection = viewModel.getSelection(currentPath),
            toolbarFocusRequester = toolbarFocusRequester,
            contentEntry = gridEntry,
            takeFocus = !viewModel.toolbarKeepsFocus,
            upFocusRequester = if (showContinue) continueRequester else toolbarFocusRequester,
            // Scrolls away with the folders as focus moves down them.
            headerFocus = if (showContinue) continueRequester else null,
            header = if (showContinue) {
                {
                    TvContinueRow(viewModel, continueRequester, up = toolbarFocusRequester,
                        onDown = { gridEntry.focusTop?.invoke() ?: false },
                        modifier = Modifier.padding(bottom = 12.dp))
                }
            } else null,
            onVisibleItemsChanged = onVisibleItemsChanged
        ) { item, modifier ->
            val thumbnailUrl = remember(item.path, item.thumbnailUrl) { viewModel.thumbnailSourceFor(item) }
            val playback = if (item.isDirectory) null else viewModel.playbackFor(item.path)
            if (isGridView) {
                FileTileItem(item, thumbnailUrl, modifier = modifier, playback = playback) { onItemClick(item) }
            } else {
                FileRowItem(item, thumbnailUrl, modifier = modifier, playback = playback) { onItemClick(item) }
            }
        }
    }
}

@Preview(name = "Media browser image tiles", widthDp = 960, heightDp = 540, showBackground = true)
@Composable
private fun MediaBrowserImageTilesPreview() {
    WeirdStreamerTheme {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(48.dp)
        ) {
            Text(
                text = "SERVER: Media Library",
                style = MaterialTheme.typography.titleSmall,
                color = Color.Gray
            )
            Text(
                text = "/Images",
                style = MaterialTheme.typography.headlineMedium
            )
            Spacer(modifier = Modifier.height(24.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                listOf(
                    "Summer.jpg",
                    "A very long summer vacation image filename.jpg",
                    "Portrait.png",
                    "Family photo from the beach.jpeg"
                ).forEach { name ->
                    TileItem(
                        title = name,
                        subtitle = "2.4 MB",
                        icon = Icons.Default.Image,
                        modifier = Modifier.width(200.dp),
                        onClick = {}
                    )
                }
            }
        }
    }
}

private fun handleFileClick(viewModel: MediaViewModel, item: FileItem) {
    if (item.path == "UP") {
        viewModel.navigateUp()
        return
    }

    if (item.isDirectory) {
        viewModel.recordSelection(viewModel.currentPath, item.path)
        viewModel.currentSource?.let { source ->
            viewModel.currentPathDisplayName = item.name
            viewModel.loadPath(source, item.path)
        }
    } else {
        when {
            viewModel.isVideo(item.path) -> viewModel.openVideo(item)
            viewModel.isImage(item.path) -> viewModel.showImage(item.path)
            viewModel.isPdf(item.path) -> viewModel.showPdf(item.path)
        }
    }
}

/** The current time, in the device's 12/24-hour format, updated on each minute. */
@Composable
private fun Clock(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val format = remember(context) { android.text.format.DateFormat.getTimeFormat(context) }
    var now by remember { mutableStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(60_000 - System.currentTimeMillis() % 60_000)
            now = System.currentTimeMillis()
        }
    }
    Text(
        text = format.format(java.util.Date(now)),
        style = MaterialTheme.typography.titleMedium,
        color = MaterialTheme.colorScheme.onSurface,
        modifier = modifier
    )
}
