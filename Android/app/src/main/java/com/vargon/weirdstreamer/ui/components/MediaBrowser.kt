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
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
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
    var dialogFocusRequester by remember { mutableStateOf<FocusRequester?>(null) }

    val currentSource = viewModel.currentSource

    val goBack: () -> Unit = {
        when {
            viewModel.openTitle != null -> viewModel.closeLibraryTitle()
            viewModel.isLibraryView -> viewModel.leaveLibrary()
            else -> viewModel.navigateUp()
        }
    }
    BackHandler(enabled = currentSource != null && dialogFocusRequester == null) {
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
        Clock(Modifier.align(Alignment.TopEnd).padding(top = 14.dp, end = 52.dp))

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(start = 48.dp, top = 48.dp, end = 48.dp)
                .onPreviewKeyEvent { 
                    if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                        if (currentSource != null && dialogFocusRequester == null) {
                            goBack()
                            true
                        } else false
                    } else false
                }
        ) {
            // Header
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown && event.key == Key.DirectionDown) {
                        if (dialogFocusRequester != null) {
                            dialogFocusRequester?.requestFocus()
                            true
                        } else contentEntry.focusTop?.invoke() ?: false
                    } else false
                }
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    if (currentSource != null) {
                        Text(
                            text = "${currentSource.typeId}: ${currentSource.displayName()}",
                            style = MaterialTheme.typography.titleSmall,
                            color = Color.Gray
                        )
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
                        Text(
                            text = "Select Media Source",
                            style = MaterialTheme.typography.headlineMedium
                        )
                    }
                }
                
                if (currentSource != null) {
                    Button(
                        onClick = { viewModel.returnToSourceSelection() },
                        modifier = Modifier.focusRequester(toolbarFocusRequester)
                    ) {
                        Icon(Icons.Default.Home, contentDescription = "Back to Servers")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                    // Replaces the ".." entry: up a folder, or back out of a title or library.
                    Button(onClick = goBack, modifier = Modifier.focusRequester(upFocusRequester)) {
                        Icon(Icons.Default.ArrowUpward, contentDescription = "Up")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                } else {
                    Button(
                        onClick = { showMainAddDialog = true },
                        modifier = Modifier.focusRequester(toolbarFocusRequester)
                    ) {
                        Icon(Icons.Default.Add, contentDescription = "Add Source")
                        Spacer(modifier = Modifier.width(8.dp))
                        Text("Add Source")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                }

                Button(onClick = { viewModel.toggleViewMode() }) {
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
                        Button(onClick = { showRegenerateThumbnailsDialog = true }) {
                            Icon(Icons.Default.Refresh, contentDescription = "Regenerate Thumbnails")
                        }
                        Spacer(modifier = Modifier.width(16.dp))
                    }

                    if (!viewModel.isLibraryView) Button(onClick = { viewModel.toggleContentFilter() }) {
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

                Button(onClick = onOpenSettings) {
                    Icon(Icons.Default.Settings, contentDescription = "Settings")
                }
            }

            Spacer(modifier = Modifier.height(24.dp))

            // Content area
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
                            Button(onClick = { 
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
                        contentEntry = contentEntry,
                        onDialogFocusChanged = { dialogFocusRequester = it }
                    )
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
                viewModel.selectSource(provider, address)
            }
        )
    }

    // Auth Dialog
    if (showAuthDialog && viewModel.authRequiredSource != null) {
        val source = viewModel.authRequiredSource!!
        AuthDialog(
            host = source.displayName(),
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
                        Button(onClick = { showRegenerateThumbnailsDialog = false }) {
                            Text("Cancel")
                        }
                        Spacer(modifier = Modifier.width(16.dp))
                        Button(
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
    if (viewModel.isGridView) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(4),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            viewModel.sourceProviders.forEach { provider ->
                item(span = { GridItemSpan(maxLineSpan) }) {
                    Text(
                        provider.displayName,
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.padding(bottom = 16.dp)
                    )
                }
                
                val servers = serversByProvider[provider].orEmpty()
                if (servers.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Text("No ${provider.displayName}s found. Use 'Add Source' to add manually.", 
                            style = MaterialTheme.typography.bodyMedium,
                            color = Color.Gray,
                            modifier = Modifier.padding(bottom = 32.dp))
                    }
                } else {
                    items(servers) { address ->
                        HostTileItem(address) {
                            viewModel.selectSource(provider, address)
                        }
                    }
                }

                item(span = { GridItemSpan(maxLineSpan) }) {
                    Spacer(modifier = Modifier.height(32.dp))
                    HorizontalDivider()
                    Spacer(modifier = Modifier.height(32.dp))
                }
            }
        }
    } else {
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            viewModel.sourceProviders.forEach { provider ->
                item {
                    Text(
                        provider.displayName,
                        style = MaterialTheme.typography.titleLarge,
                        modifier = Modifier.padding(bottom = 16.dp)
                    )
                }
                
                val servers = serversByProvider[provider].orEmpty()
                if (servers.isEmpty()) {
                    item {
                        Text("No ${provider.displayName}s found. Use 'Add Source' to add manually.", 
                            style = MaterialTheme.typography.bodyMedium,
                            color = Color.Gray,
                            modifier = Modifier.padding(bottom = 32.dp))
                    }
                } else {
                    items(servers) { address ->
                        HostRowItem(address) {
                            viewModel.selectSource(provider, address)
                        }
                    }
                }

                item {
                    Spacer(modifier = Modifier.height(32.dp))
                    HorizontalDivider()
                    Spacer(modifier = Modifier.height(32.dp))
                }
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
    val keyboardController = LocalSoftwareKeyboardController.current

    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        Surface(modifier = Modifier.width(500.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                if (selectedProvider == null) {
                    Text("Select Source Type", style = MaterialTheme.typography.headlineSmall)
                    Spacer(modifier = Modifier.height(16.dp))
                    providers.forEachIndexed { index, provider ->
                        Button(
                            onClick = { selectedProvider = provider },
                            modifier = Modifier
                                .fillMaxWidth()
                                .then(if (index == 0) Modifier.focusRequester(initialFocusRequester) else Modifier)
                        ) {
                            Text(provider.displayName)
                        }
                        Spacer(modifier = Modifier.height(8.dp))
                    }
                    Button(onClick = onDismiss, modifier = Modifier.align(Alignment.End)) {
                        Text("Cancel")
                    }
                } else {
                    val provider = selectedProvider!!
                    Text("Add ${provider.displayName}", style = MaterialTheme.typography.headlineSmall)
                    Spacer(modifier = Modifier.height(16.dp))
                    
                    TextField(
                        value = address,
                        onValueChange = { address = it },
                        label = { Text(provider.addressLabel) },
                        placeholder = { Text(provider.addressPlaceholder) },
                        supportingText = provider.addressHint?.let { hint -> { Text(hint) } },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth().focusRequester(focusRequester),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = {
                            keyboardController?.hide()
                            if (address.isNotBlank()) onAdd(provider, address)
                        })
                    )
                    
                    Spacer(modifier = Modifier.height(24.dp))
                    
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                        Button(onClick = { selectedProvider = null; address = "" }) { Text("Back") }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(onClick = onDismiss) { Text("Cancel") }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(onClick = { if (address.isNotBlank()) onAdd(provider, address) }) {
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
    contentEntry: ContentEntry,
    onDialogFocusChanged: (FocusRequester?) -> Unit
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
    // A card removed from Continue Watching took focus with it: onto the row's first card, or
    // the folder when the row is gone.
    LaunchedEffect(viewModel.continueRemovals) {
        if (viewModel.continueRemovals == 0) return@LaunchedEffect
        withFrameNanos { }
        if (!(showContinue && runCatching { continueRequester.requestFocus() }.isSuccess)) gridEntry.focusTop?.invoke()
    }
    Column(Modifier.fillMaxSize()) {
        if (showContinue) {
            TvContinueRow(viewModel, continueRequester, up = toolbarFocusRequester,
                onDown = { gridEntry.focusTop?.invoke() ?: false },
                modifier = Modifier.padding(start = 8.dp, end = 8.dp, bottom = 12.dp))
        }
        BrowserGrid(
            folderKey = currentPath,
            items = items,
            isGridView = isGridView,
            initialSelection = viewModel.getSelection(currentPath),
            toolbarFocusRequester = toolbarFocusRequester,
            contentEntry = gridEntry,
            takeFocus = !viewModel.toolbarKeepsFocus,
            upFocusRequester = if (showContinue) continueRequester else toolbarFocusRequester,
            onDialogFocusChanged = onDialogFocusChanged,
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
