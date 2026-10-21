package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
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
    val currentSource = viewModel.currentSource

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
            TopAppBar(
                title = {
                    Column {
                        if (currentSource != null) {
                            Text(
                                text = "${currentSource.typeId}: ${currentSource.displayName()}",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.secondary
                            )
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
                            Text("WeirdStreamer")
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
                    IconButton(onClick = { viewModel.toggleViewMode() }) {
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
                        DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                            if (currentSource != null && !viewModel.isLibraryView) {
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
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
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
                viewModel.errorMessage != null -> {
                    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text(text = "Error: ${viewModel.errorMessage}", color = MaterialTheme.colorScheme.error)
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

    if (showRegenerateDialog) {
        AlertDialog(
            onDismissRequest = { showRegenerateDialog = false },
            title = { Text("Regenerate thumbnails?") },
            text = { Text("Thumbnails for this folder are made again on the server.") },
            confirmButton = {
                TextButton(onClick = { showRegenerateDialog = false; viewModel.regenerateThumbnails() }) { Text("Regenerate") }
            },
            dismissButton = { TextButton(onClick = { showRegenerateDialog = false }) { Text("Cancel") } }
        )
    }

    if (showMainAddDialog) {
        PhoneAddSourceDialog(
            providers = viewModel.sourceProviders,
            onDismiss = { showMainAddDialog = false },
            onAdd = { provider, address ->
                showMainAddDialog = false
                viewModel.selectSource(provider, address)
            }
        )
    }

    if (showAuthDialog && viewModel.authRequiredSource != null) {
        val source = viewModel.authRequiredSource!!
        PhoneAuthDialog(
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
}

@Composable
private fun PhoneSourceSelectionScreen(viewModel: MediaViewModel) {
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        viewModel.sourceProviders.forEach { provider ->
            item {
                Text(
                    text = provider.displayName,
                    style = MaterialTheme.typography.titleLarge,
                    modifier = Modifier.padding(16.dp)
                )
            }
            
            val servers = provider.getServers()
            if (servers.isEmpty()) {
                item {
                    Text(
                        text = "No ${provider.displayName}s found.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
                    )
                }
            } else {
                items(servers) { address ->
                    PhoneHostRowItem(address) {
                        viewModel.selectSource(provider, address)
                    }
                }
            }
            
            item { HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp)) }
        }
    }
}

@Composable
private fun PhoneFileBrowserScreen(viewModel: MediaViewModel) {
    val items = viewModel.getFilteredFileList()
    
    if (viewModel.isGridView) {
        LazyVerticalGrid(
            columns = GridCells.Adaptive(120.dp),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            items(items) { item ->
                PhoneFileTileItem(item) {
                    handlePhoneFileClick(viewModel, item)
                }
            }
        }
    } else {
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            items(items) { item ->
                PhoneFileRowItem(item) {
                    handlePhoneFileClick(viewModel, item)
                }
            }
        }
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
            viewModel.isVideo(item.path) -> viewModel.playVideo(item.path)
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
                    modifier = Modifier.fillMaxWidth()
                )
            }
        },
        confirmButton = {
            if (selectedProvider != null) {
                TextButton(onClick = { if (address.isNotBlank()) onAdd(selectedProvider!!, address) }) {
                    Text("Add")
                }
            }
        },
        dismissButton = {
            TextButton(onClick = {
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
private fun PhoneAuthDialog(host: String, onDismiss: () -> Unit, onLogin: (String, String, Boolean) -> Unit) {
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var savePassword by remember { mutableStateOf(true) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Authentication Required") },
        text = {
            Column {
                Text("Host: $host", style = MaterialTheme.typography.bodySmall)
                Spacer(modifier = Modifier.height(16.dp))
                TextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text("Username") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                Spacer(modifier = Modifier.height(8.dp))
                TextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("Password") },
                    visualTransformation = androidx.compose.ui.text.input.PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.clickable { savePassword = !savePassword }
                ) {
                    Checkbox(checked = savePassword, onCheckedChange = { savePassword = it })
                    Text("Save credentials")
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onLogin(username, password, savePassword) }) {
                Text("Login")
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}
