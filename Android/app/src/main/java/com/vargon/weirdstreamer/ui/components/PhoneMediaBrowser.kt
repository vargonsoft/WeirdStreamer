package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.background
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.grid.LazyGridState
import kotlinx.coroutines.launch
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ViewList
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.IntOffset
import com.vargon.weirdstreamer.ContentFilter
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.R
import com.vargon.weirdstreamer.repository.FileItem
import com.vargon.weirdstreamer.repository.MediaSource
import com.vargon.weirdstreamer.repository.SourceProvider
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneMediaBrowser(viewModel: MediaViewModel, onOpenSettings: () -> Unit) {
    var showMainAddDialog by remember { mutableStateOf(false) }
    var showAuthDialog by remember { mutableStateOf(false) }
    var showRegenerateDialog by remember { mutableStateOf(false) }
    var showDownloadFolder by remember { mutableStateOf(false) }
    val currentSource = viewModel.currentSource
    PhoneSourceDialog(viewModel) // download or stream, for a video downloaded here

    // Back closes a title page, then leaves Library view for the server's libraries, else goes up.
    BackHandler(enabled = currentSource != null) {
        when {
            viewModel.openTitle != null -> viewModel.closeLibraryTitle()
            viewModel.isLibraryView -> viewModel.leaveLibrary()
            else -> viewModel.navigateUp()
        }
    }

    Scaffold(
        topBar = {
            PhoneTopBarPanel(above = currentSource?.let { source ->
                {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 8.dp)
                    ) {
                        Wordmark(MaterialTheme.colorScheme.onBackground, Modifier.height(16.dp))
                        Spacer(Modifier.width(12.dp))
                        Text(
                            text = viewModel.headerLine(source),
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.onBackground,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }
            }) {
            TopAppBar(
                windowInsets = panelTopBarInsets(),
                title = {
                    Column {
                        if (currentSource != null) {
                            val display = when {
                                viewModel.openTitle != null -> viewModel.openTitle!!.title
                                viewModel.isLibraryView -> viewModel.libraryListing?.libraryName ?: "Library"
                                else -> viewModel.currentPathDisplayName ?: if (viewModel.currentPath.isNotEmpty()) viewModel.currentPath else "/"
                            }
                            Text(
                                text = display,
                                style = MaterialTheme.typography.titleMedium,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        } else {
                            Wordmark(MaterialTheme.colorScheme.onSurface, Modifier.height(22.dp))
                        }
                    }
                },
                navigationIcon = {
                    if (currentSource != null) {
                        IconButton(onClick = { viewModel.returnToSourceSelection() }) {
                            Icon(Icons.Default.Home, contentDescription = "Home")
                        }
                    }
                },
                actions = {
                    if (currentSource == null) {
                        IconButton(onClick = { showMainAddDialog = true }) {
                            Icon(Icons.Default.Add, contentDescription = "Add Source")
                        }
                    }
                    // Not on the server list, which has one layout.
                    if (currentSource != null) IconButton(onClick = { viewModel.toggleViewMode() }) {
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
                    // Everything else lives in the overflow menu, so the bar fits on a phone.
                    var menuOpen by remember { mutableStateOf(false) }
                    Box {
                        IconButton(onClick = { menuOpen = true }) {
                            Icon(Icons.Default.MoreVert, contentDescription = "More")
                        }
                        DropdownMenu(border = phoneMenuBorder(), expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            if (currentSource != null && !viewModel.isLibraryView && !viewModel.isServerRoot) {
                                Text("Show", style = MaterialTheme.typography.labelMedium,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp))
                                listOf(
                                    Triple(ContentFilter.ALL, "Everything", Icons.Default.FilterList),
                                    Triple(ContentFilter.VIDEO, "Videos", Icons.Default.VideoLibrary),
                                    Triple(ContentFilter.IMAGE, "Images", Icons.Default.Image),
                                    Triple(ContentFilter.PDF, "PDFs", Icons.Default.PictureAsPdf)
                                ).forEach { (filter, label, icon) ->
                                    DropdownMenuItem(
                                        text = { Text(label) },
                                        leadingIcon = { Icon(icon, contentDescription = null) },
                                        trailingIcon = { if (viewModel.contentFilter == filter) Icon(Icons.Default.Check, contentDescription = "Selected") },
                                        onClick = { menuOpen = false; viewModel.chooseContentFilter(filter) }
                                    )
                                }
                                HorizontalDivider()
                            }
                            // Not among the libraries: that would be every library.
                            if (currentSource != null && !viewModel.isLibraryView && !viewModel.isServerRoot && viewModel.connectedServer != null) {
                                DropdownMenuItem(
                                    text = { Text("Download this folder") },
                                    leadingIcon = { Icon(Icons.Default.Download, contentDescription = null) },
                                    onClick = { menuOpen = false; showDownloadFolder = true }
                                )
                            }
                            if (currentSource != null && !viewModel.isLibraryView && viewModel.supportsThumbnailRegeneration) {
                                DropdownMenuItem(
                                    text = { Text("Regenerate thumbnails") },
                                    leadingIcon = { Icon(Icons.Default.Refresh, contentDescription = null) },
                                    onClick = { menuOpen = false; showRegenerateDialog = true }
                                )
                            }
                            DropdownMenuItem(
                                text = { Text("Settings") },
                                leadingIcon = { Icon(Icons.Default.Settings, contentDescription = null) },
                                onClick = { menuOpen = false; onOpenSettings() }
                            )
                        }
                    }
                }
            )
            }
        }
    ) { padding ->
        // The source list boxes each server on its own.
        val serverList = currentSource == null && !viewModel.isLoading && viewModel.errorMessage == null &&
            viewModel.authRequiredSource == null
        PhoneContentPanel(modifier = Modifier.fillMaxSize().padding(padding), boxed = !serverList) {
            // Background image from theme spec
            val themeSpec = WeirdStreamerTheme.spec
            themeSpec.backgroundImage?.let { bgRes ->
                Image(
                    painter = painterResource(id = bgRes),
                    contentDescription = null,
                    modifier = Modifier
                        .fillMaxSize()
                        .align(Alignment.CenterEnd)
                        .offset { IntOffset(125.dp.roundToPx(), 0) }
                        .alpha(0.10f),
                    contentScale = ContentScale.Crop,
                    alignment = Alignment.CenterEnd
                )
            }

            when {
                viewModel.isLoading -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator()
                    }
                }
                // A server that can't be reached: a banner, and what's downloaded from it.
                viewModel.errorMessage != null && viewModel.errorOffline && currentSource?.typeId == "SERVER" -> {
                    PhoneOfflineScreen(viewModel) { currentSource?.let { viewModel.loadPath(it, viewModel.currentPath) } }
                }
                viewModel.errorMessage != null -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(text = "Error: ${viewModel.errorMessage}", color = MaterialTheme.colorScheme.error)
                            Spacer(modifier = Modifier.height(16.dp))
                            Button(shape = AppButtonShape, colors = phoneButtonColors(), onClick = { 
                                currentSource?.let { viewModel.loadPath(it, viewModel.currentPath) }
                            }) {
                                Text("Retry")
                            }
                        }
                    }
                }
                viewModel.authRequiredSource != null -> {
                    showAuthDialog = true
                }
                currentSource == null -> {
                    PhoneSourceSelectionScreen(viewModel)
                }
                viewModel.isLibraryView -> {
                    val title = viewModel.openTitle
                    if (title != null) PhoneTitleScreen(viewModel, title) else PhoneLibraryScreen(viewModel)
                }
                else -> {
                    PhoneFileBrowserScreen(viewModel)
                }
            }
        }
    }

    if (showDownloadFolder) DownloadFolderDialog(viewModel) { showDownloadFolder = false }
    if (showRegenerateDialog) {
        AlertDialog(
            onDismissRequest = { showRegenerateDialog = false },
            title = { Text("Regenerate thumbnails?") },
            text = { Text("Thumbnails for this folder are made again on the server.") },
            confirmButton = {
                PhoneButton(onClick = { showRegenerateDialog = false; viewModel.regenerateThumbnails() }) { Text("Regenerate") }
            },
            dismissButton = { PhoneSecondaryButton(onClick = { showRegenerateDialog = false }) { Text("Cancel") } }
        )
    }

    if (showMainAddDialog) {
        PhoneAddSourceDialog(
            providers = viewModel.sourceProviders,
            onDismiss = { showMainAddDialog = false },
            onAdd = { provider, address ->
                showMainAddDialog = false
                viewModel.addServer(provider, address)
            }
        )
    }

    if (showAuthDialog && viewModel.authRequiredSource != null) {
        val source = viewModel.authRequiredSource!!
        PhoneAuthDialog(
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

    // An account with a PIN on this phone, chosen on the source list
    PhonePinPrompt(viewModel)
}

@Composable
private fun PhoneSourceSelectionScreen(viewModel: MediaViewModel) {
    // Saved servers, read again when one is added or removed.
    val serversByProvider = remember(viewModel.sourceProviders, viewModel.authStateRefreshTrigger) {
        viewModel.sourceProviders.associateWith { it.getServers() }
    }
    // One list of every server, WeirdStreamer servers first; each card says which kind it is.
    val servers = viewModel.sourceProviders.flatMap { p -> serversByProvider[p].orEmpty().map { p to it } }
    LazyColumn(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(vertical = 4.dp)) {
        if (servers.isEmpty()) item {
            Text(
                text = "No servers yet. Use + to add one.",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(16.dp)
            )
        }
        itemsIndexed(servers, key = { _, (p, a) -> "${p.typeId}:$a" }) { i, (provider, address) ->
            if (i > 0 && !hasPanels) HorizontalDivider() // themes without boxes
            val accounts = remember(address, viewModel.authStateRefreshTrigger) { viewModel.accountsOf(provider, address) }
            Panel(Modifier.fillMaxWidth(), margin = PaddingValues(horizontal = 8.dp, vertical = 4.dp),
                contentPadding = PaddingValues(0.dp), clip = true) {
                PhoneServerAccountsRow(viewModel, provider, address, accounts)
            }
        }
    }
}

@Composable
private fun PhoneFileBrowserScreen(viewModel: MediaViewModel) {
    val items = viewModel.getFilteredFileList()
    PhoneResumeDialog(viewModel)
    
    // The A–Z strip: folders' letters, then files'.
    val letters = remember(items) {
        buildIndex(items, { indexLabel(it.name) }, { if (it.isDirectory) 0 else 1 })
    }
    // The server's front page also has what's downloaded from it.
    val downloads = rememberReadyDownloads(viewModel).takeIf { viewModel.atFrontPage }.orEmpty()
    val topRows = viewModel.hasTopRows || downloads.isNotEmpty()
    val continueCount = if (topRows) 1 else 0
    val folder = viewModel.currentPath
    val gridState = remember(folder) { LazyGridState() }
    val listState = remember(folder) { LazyListState() }
    val scope = rememberCoroutineScope()
    // Back at a folder (from one below it or a video), to where it was. Only once its list has
    // loaded: a list that's still empty can't hold a position.
    var scrollRestored by remember(folder) { mutableStateOf(false) }
    val folderLoaded = items.isNotEmpty() && !viewModel.isLoading && viewModel.continueReady
    LaunchedEffect(folder, folderLoaded) {
        if (!folderLoaded || scrollRestored) return@LaunchedEffect
        viewModel.folderScroll[folder]?.let { (index, offset) ->
            if (viewModel.isGridView) gridState.scrollToItem(index, offset) else listState.scrollToItem(index, offset)
        }
        scrollRestored = true
    }
    LaunchedEffect(folder, scrollRestored, viewModel.isGridView) {
        if (!scrollRestored) return@LaunchedEffect
        val grid = viewModel.isGridView
        snapshotFlow {
            if (grid) gridState.firstVisibleItemIndex to gridState.firstVisibleItemScrollOffset
            else listState.firstVisibleItemIndex to listState.firstVisibleItemScrollOffset
        }.collect { viewModel.folderScroll[folder] = it }
    }
    val topLetter by remember(letters, continueCount) {
        derivedStateOf {
            val first = if (viewModel.isGridView) gridState.firstVisibleItemIndex else listState.firstVisibleItemIndex
            letters.entryFor((first - continueCount).coerceAtLeast(0))
        }
    }

    Row(Modifier.fillMaxSize()) {
    if (viewModel.isGridView) {
        LazyVerticalGrid(
            columns = GridCells.Adaptive(120.dp),
            state = gridState,
            modifier = Modifier.weight(1f).fillMaxHeight(),
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            if (topRows) {
                item(span = { GridItemSpan(maxLineSpan) }, key = "continue") {
                    PhoneContinueRow(viewModel, sidePadding = 0.dp, downloads = downloads)
                }
            }
            items(items) { item ->
                PhoneFileTileItem(item) {
                    handlePhoneFileClick(viewModel, item)
                }
            }
        }
    } else {
        // Padded as the grid is, so switching views leaves Continue Watching where it was.
        LazyColumn(state = listState, modifier = Modifier.weight(1f).fillMaxHeight(),
            contentPadding = PaddingValues(vertical = 8.dp)) {
            // The server's front page leads with what's in progress, in every library.
            if (topRows) {
                item(key = "continue") {
                    PhoneContinueRow(viewModel, Modifier.padding(start = 8.dp, end = 8.dp, bottom = 8.dp), sidePadding = 0.dp,
                        downloads = downloads)
                }
            }
            items(items) { item ->
                PhoneFileRowItem(item) {
                    handlePhoneFileClick(viewModel, item)
                }
            }
        }
    }
    if (letters.size > 1) {
        PhoneIndexBar(letters, topLetter, onSelect = { i ->
            val target = letters[i].position + continueCount
            scope.launch {
                if (viewModel.isGridView) gridState.scrollToItem(target) else listState.scrollToItem(target)
            }
        }, Modifier.padding(vertical = 8.dp))
    }
    }
}

/**
 * A server that can't be reached: a banner saying so (with Retry) at the top, then the downloads
 * from it, ready to play offline.
 */
@Composable
private fun PhoneOfflineScreen(viewModel: MediaViewModel, onRetry: () -> Unit) {
    val downloads = rememberReadyDownloads(viewModel)
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(8.dp)) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp))
                .background(MaterialTheme.colorScheme.secondaryContainer).padding(start = 16.dp, end = 8.dp, top = 10.dp, bottom = 10.dp)
        ) {
            Icon(Icons.Default.CloudOff, contentDescription = null, tint = MaterialTheme.colorScheme.onSecondaryContainer)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text("The server is offline", style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSecondaryContainer)
                Text(if (downloads.isEmpty()) "Nothing from it is downloaded to this phone." else "Your downloads still play.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSecondaryContainer)
            }
            PhoneButton(onClick = onRetry) { Text("Retry") }
        }
        PhoneDownloadsRow(viewModel, downloads, sidePadding = 0.dp)
    }
}

private fun handlePhoneFileClick(viewModel: MediaViewModel, item: FileItem) {
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
            viewModel.isVideo(item) -> viewModel.openVideo(item)
            viewModel.isImage(item.path) -> viewModel.showImage(item.path)
            viewModel.isPdf(item.path) -> viewModel.showPdf(item.path)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PhoneAddSourceDialog(
    providers: List<SourceProvider>,
    onDismiss: () -> Unit,
    onAdd: (SourceProvider, String) -> Unit
) {
    var selectedProvider by remember { mutableStateOf<SourceProvider?>(null) }
    var address by remember { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (selectedProvider == null) "Select Source Type" else "Add ${selectedProvider!!.displayName}") },
        text = {
            if (selectedProvider == null) {
                Column {
                    providers.forEach { provider ->
                        ListItem(
                            headlineContent = { Text(provider.displayName) },
                            modifier = Modifier.clickable { selectedProvider = provider }
                        )
                    }
                }
            } else {
                val provider = selectedProvider!!
                TextField(
                    value = address,
                    onValueChange = { address = it },
                    label = { Text(provider.addressLabel) },
                    placeholder = { Text(provider.addressPlaceholder) },
                    supportingText = provider.addressHint?.let { hint -> { Text(hint) } },
                    singleLine = true,
                    keyboardOptions = addressKeyboard(),
                    modifier = Modifier.fillMaxWidth()
                )
            }
        },
        confirmButton = {
            if (selectedProvider != null) {
                PhoneButton(onClick = { if (address.isNotBlank()) onAdd(selectedProvider!!, address) }) {
                    Text("Add")
                }
            }
        },
        dismissButton = {
            PhoneSecondaryButton(onClick = {
                if (selectedProvider == null) onDismiss()
                else selectedProvider = null
            }) {
                Text(if (selectedProvider == null) "Cancel" else "Back")
            }
        }
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PhoneAuthDialog(host: String, onDismiss: () -> Unit, initialUsername: String? = null, title: String? = null, hint: String? = null, onLogin: (String, String, Boolean) -> Unit) {
    var username by remember { mutableStateOf(initialUsername.orEmpty()) }
    var password by remember { mutableStateOf("") }
    var savePassword by remember { mutableStateOf(true) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title ?: "Authentication Required") },
        text = {
            Column {
                Text("Host: $host", style = MaterialTheme.typography.bodySmall)
                Spacer(modifier = Modifier.height(16.dp))
                TextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text("Username") },
                    keyboardOptions = usernameKeyboard(),
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                Spacer(modifier = Modifier.height(8.dp))
                TextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("Password") },
                    visualTransformation = androidx.compose.ui.text.input.PasswordVisualTransformation(),
                    keyboardOptions = secretKeyboard(),
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                hint?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 8.dp))
                }
            }
        },
        confirmButton = {
            PhoneButton(onClick = { onLogin(username, password, savePassword) }) {
                Text("Login")
            }
        },
        dismissButton = {
            PhoneSecondaryButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}
