package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
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
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.PictureAsPdf
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.VideoLibrary
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.TextField
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
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
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.window.Dialog
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ContentFilter
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.R
import com.vargon.weirdstreamer.repository.MediaSource
import com.vargon.weirdstreamer.ui.theme.AppTheme

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun MediaBrowser(viewModel: MediaViewModel, onOpenSettings: () -> Unit) {
    var showAddServerDialog by remember { mutableStateOf(false) }
    var showAuthDialog by remember { mutableStateOf(false) }
    var showRegenerateThumbnailsDialog by remember { mutableStateOf(false) }
    val toolbarFocusRequester = remember { FocusRequester() }
    var dialogFocusRequester by remember { mutableStateOf<FocusRequester?>(null) }

    val currentSource = viewModel.currentSource

    BackHandler(enabled = currentSource != null && dialogFocusRequester == null) {
        viewModel.navigateUp()
    }

    Box(modifier = Modifier.fillMaxSize()) {
        // Background image for Cinema Blue theme - on the right side
        if (MaterialTheme.colorScheme.primary == com.vargon.weirdstreamer.ui.theme.CinemaGold) {
            Image(
                painter = painterResource(id = R.drawable.film_reel),
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

        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(48.dp)
                .onPreviewKeyEvent { 
                if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                    // Don't handle if dialog is showing
                    if (currentSource != null && dialogFocusRequester == null) {
                        viewModel.navigateUp()
                        true
                    } else {
                        false
                    }
                } else {
                    false
                }
            }
    ) {
        // Header with current path and controls
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.onPreviewKeyEvent { event ->
                // Handle down navigation - if dialog is open, focus it instead of file list
                if (event.type == KeyEventType.KeyDown && event.key == Key.DirectionDown) {
                    if (dialogFocusRequester != null) {
                        dialogFocusRequester?.requestFocus()
                        true
                    } else {
                        false
                    }
                } else {
                    false
                }
            }
        ) {
            // Display server/source and path in stacked layout
            Column(modifier = Modifier.weight(1f)) {
                when (currentSource) {
                    is MediaSource.Smb -> {
                        val pathWithoutScheme = viewModel.currentPath.removePrefix("smb://")
                        val firstSlashIndex = pathWithoutScheme.indexOf('/')
                        val serverName = if (firstSlashIndex > 0) {
                            pathWithoutScheme.substring(0, firstSlashIndex)
                        } else {
                            pathWithoutScheme
                        }
                        val relativePath = if (firstSlashIndex > 0) {
                            pathWithoutScheme.substring(firstSlashIndex)
                        } else {
                            "/"
                        }
                        
                        Text(
                            text = "SMB: $serverName",
                            style = MaterialTheme.typography.titleSmall,
                            color = Color.Gray
                        )
                        if (relativePath.isNotEmpty() && relativePath != "/") {
                            TextWithStartEllipsis(
                                text = relativePath.removePrefix("/"),
                                style = MaterialTheme.typography.headlineMedium,
                                color = MaterialTheme.colorScheme.onSurface,
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(end = 16.dp)
                            )
                        } else {
                            Text(
                                text = "/",
                                style = MaterialTheme.typography.headlineMedium
                            )
                        }
                    }
                    is MediaSource.Server -> {
                        Text(
                            text = "Server: ${currentSource.displayName()}",
                            style = MaterialTheme.typography.titleSmall,
                            color = Color.Gray
                        )
                        // If we have a friendly display name for the current path (root ID), show it.
                        val display = viewModel.currentPathDisplayName ?: if (viewModel.currentPath.isNotEmpty()) viewModel.currentPath else "/"
                        TextWithStartEllipsis(
                            text = display,
                            style = MaterialTheme.typography.headlineMedium,
                            color = MaterialTheme.colorScheme.onSurface,
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(end = 16.dp)
                        )
                    }
                    null -> {
                        Text(
                            text = "Select Media Source",
                            style = MaterialTheme.typography.headlineMedium
                        )
                    }
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
                Button(onClick = { viewModel.toggleViewMode() }) {
                    Icon(
                        if (viewModel.isGridView) Icons.AutoMirrored.Filled.ViewList else Icons.Default.GridView,
                        contentDescription = "Switch View"
                    )
                }
                Spacer(modifier = Modifier.width(16.dp))
                Button(onClick = { viewModel.toggleContentFilter() }) {
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
                
                // Show regenerate button only for Server sources when in a folder
                if (currentSource is MediaSource.Server && viewModel.currentPath.isNotEmpty()) {
                    Spacer(modifier = Modifier.width(16.dp))
                    Button(onClick = { showRegenerateThumbnailsDialog = true }) {
                        Icon(Icons.Default.Refresh, contentDescription = "Regenerate Thumbnails")
                    }
                }
                
                Spacer(modifier = Modifier.width(16.dp))
            } else {
                // Only show Add Source button on server selection screen
                Button(
                    onClick = { showAddServerDialog = true },
                    modifier = Modifier.focusRequester(toolbarFocusRequester)
                ) {
                    Icon(Icons.Default.Add, contentDescription = "Add Source")
                    Spacer(modifier = Modifier.width(8.dp))
                    Text("Add Source")
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
            viewModel.authRequiredHost != null -> {
                showAuthDialog = true
            }
            currentSource == null -> {
                // Source selection screen
                SourceSelectionScreen(viewModel)
            }
            else -> {
                // File browser
                FileBrowserScreen(
                    viewModel = viewModel,
                    toolbarFocusRequester = toolbarFocusRequester,
                    onDialogFocusChanged = { focusRequester ->
                        dialogFocusRequester = focusRequester
                    }
                )
            }
        }
        }
    }

    // Add Server Dialog
    if (showAddServerDialog) {
        AddServerDialog(
            onDismiss = { showAddServerDialog = false },
            onAddSmb = { host ->
                showAddServerDialog = false
                viewModel.selectSmbSource(host)
            },
            onAddServer = { serverUrl ->
                showAddServerDialog = false
                viewModel.selectServerSource(serverUrl)
            }
        )
    }

    // Auth Dialog for SMB
    if (showAuthDialog && viewModel.authRequiredHost != null) {
        AuthDialog(
            host = viewModel.authRequiredHost!!,
            onDismiss = { 
                showAuthDialog = false
                viewModel.returnToSourceSelection()
            },
            onLogin = { user, pass, save ->
                showAuthDialog = false
                currentSource?.let { viewModel.loadPath(it, viewModel.currentPath, user, pass, save) }
            }
        )
    }
    
    // Regenerate Thumbnails Confirmation Dialog
    if (showRegenerateThumbnailsDialog) {
        ConfirmDialog(
            title = "Regenerate Thumbnails",
            message = "Regenerate all thumbnails for this folder? This will delete and recreate all thumbnails.",
            onDismiss = { showRegenerateThumbnailsDialog = false },
            onConfirm = {
                showRegenerateThumbnailsDialog = false
                viewModel.regenerateThumbnails()
            }
        )
    }
    
    // Auth Dialog for Server API
    if (viewModel.authRequiredServerUrl != null) {
        AuthDialog(
            host = viewModel.authRequiredServerUrl!!,
            onDismiss = { 
                viewModel.authRequiredServerUrl = null
                viewModel.returnToSourceSelection()
            },
            onLogin = { user, pass, save ->
                val serverUrl = viewModel.authRequiredServerUrl!!
                viewModel.authRequiredServerUrl = null
                viewModel.selectServerSource(serverUrl, user, pass)
            }
        )
    }
}

@Composable
private fun SourceSelectionScreen(viewModel: MediaViewModel) {
    // Refresh sources when this screen is shown
    LaunchedEffect(Unit) {
        viewModel.refreshSources()
    }
    
    android.util.Log.d("SourceSelection", "SMB hosts: ${viewModel.smbHosts.size}, API servers: ${viewModel.apiServers.size}")
    android.util.Log.d("SourceSelection", "API servers list: ${viewModel.apiServers}")
    
    LazyColumn(modifier = Modifier.fillMaxSize()) {
        // SMB Shares Section
        item {
            Text(
                "SMB Shares",
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(bottom = 16.dp)
            )
        }
        
        if (viewModel.smbHosts.isEmpty()) {
            item {
                Text("No SMB shares found. Use 'Add Source' to add manually.", 
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color.Gray,
                    modifier = Modifier.padding(top = 16.dp, bottom = 32.dp))
            }
        } else {
            items(viewModel.smbHosts) { host ->
                if (viewModel.isGridView) {
                    HostTileItem(host) {
                        viewModel.selectSmbSource(host)
                    }
                } else {
                    HostRowItem(host) {
                        viewModel.selectSmbSource(host)
                    }
                }
            }
        }

        item {
            Spacer(modifier = Modifier.height(32.dp))
            HorizontalDivider()
            Spacer(modifier = Modifier.height(32.dp))
        }

        // API Servers Section
        item {
            Text(
                "API Servers",
                style = MaterialTheme.typography.titleLarge,
                modifier = Modifier.padding(bottom = 16.dp)
            )
        }
        
        if (viewModel.apiServers.isEmpty()) {
            item {
                Text("No API servers configured. Use 'Add Source' to add a server.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color.Gray,
                    modifier = Modifier.padding(top = 16.dp))
            }
        } else {
            items(viewModel.apiServers) { serverUrl ->
                if (viewModel.isGridView) {
                    HostTileItem(serverUrl) {
                        viewModel.selectServerSource(serverUrl)
                    }
                } else {
                    HostRowItem(serverUrl) {
                        viewModel.selectServerSource(serverUrl)
                    }
                }
            }
        }
    }
}

@Composable
private fun FileBrowserScreen(
    viewModel: MediaViewModel,
    toolbarFocusRequester: FocusRequester,
    onDialogFocusChanged: (FocusRequester?) -> Unit
) {
    var dialogFocusRequester by remember { mutableStateOf<FocusRequester?>(null) }

    // Notify parent when dialog focus changes
    LaunchedEffect(dialogFocusRequester) {
        onDialogFocusChanged(dialogFocusRequester)
    }

    FocusableList(
        items = viewModel.getFilteredFileList(),
        initialSelection = viewModel.getSelection(viewModel.currentPath),
        isGridView = viewModel.isGridView,
        onRequestToolbarFocus = {
            toolbarFocusRequester.requestFocus()
        },
        onRequestDialogFocus = { focusRequester ->
            dialogFocusRequester = focusRequester
        }
    ) { item, modifier ->
        if (viewModel.isGridView) {
            FileTileItem(item, modifier = modifier) {
                handleFileClick(viewModel, item.path, item.isDirectory, item.name)
            }
        } else {
            FileRowItem(item, modifier = modifier) {
                handleFileClick(viewModel, item.path, item.isDirectory, item.name)
            }
        }
    }
}

private fun handleFileClick(viewModel: MediaViewModel, path: String, isDirectory: Boolean, name: String) {
    android.util.Log.d("MediaBrowser", "handleFileClick: path=$path, isDirectory=$isDirectory")
    
    if (path == "UP") {
        viewModel.navigateUp()
        return
    }

    if (isDirectory) {
        viewModel.recordSelection(viewModel.currentPath, path)
        viewModel.currentSource?.let { source ->
            when (source) {
                is MediaSource.Smb -> {
                    viewModel.currentPathDisplayName = null
                    viewModel.loadPath(source, path)
                }
                is MediaSource.Server -> {
                    // Preserve the friendly name for this server root (name) so the UI can show it
                    viewModel.currentPathDisplayName = name
                    viewModel.loadPath(source, path)
                }
            }
        }
    } else {
        val isVideo = viewModel.isVideo(path)
        val isImage = viewModel.isImage(path)
        val isPdf = viewModel.isPdf(path)
        
        android.util.Log.d("MediaBrowser", "File type check: isVideo=$isVideo, isImage=$isImage, isPdf=$isPdf")
        
        when {
            isVideo -> {
                android.util.Log.d("MediaBrowser", "Playing video: $path")
                viewModel.playVideo(path)
            }
            isImage -> {
                android.util.Log.d("MediaBrowser", "Showing image: $path")
                viewModel.showImage(path)
            }
            isPdf -> {
                android.util.Log.d("MediaBrowser", "Showing PDF: $path")
                viewModel.showPdf(path)
            }
            else -> {
                android.util.Log.w("MediaBrowser", "Unknown file type: $path")
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
private fun AddServerDialog(
    onDismiss: () -> Unit,
    onAddSmb: (String) -> Unit,
    onAddServer: (String) -> Unit
) {
    var selectedType by remember { mutableStateOf<Int?>(null) } // null = not selected, 0 = SMB, 1 = Server
    var address by remember { mutableStateOf("") }
    val focusRequester = remember { FocusRequester() }
    val smbButtonFocusRequester = remember { FocusRequester() }
    val keyboardController = LocalSoftwareKeyboardController.current

    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        Surface(modifier = Modifier.width(500.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text("Add Media Source", style = MaterialTheme.typography.headlineSmall)
                
                Spacer(modifier = Modifier.height(16.dp))
                
                if (selectedType == null) {
                    // Step 1: Select source type
                    Text("Select source type:", style = MaterialTheme.typography.bodyLarge)
                    Spacer(modifier = Modifier.height(16.dp))
                    
                    Button(
                        onClick = { selectedType = 0 },
                        modifier = Modifier
                            .fillMaxWidth()
                            .focusRequester(smbButtonFocusRequester)
                    ) {
                        Text("SMB Share")
                    }
                    Spacer(modifier = Modifier.height(12.dp))
                    Button(
                        onClick = { selectedType = 1 },
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("WeirdStreamer Server")
                    }
                    Spacer(modifier = Modifier.height(24.dp))
                    Button(onClick = onDismiss, modifier = Modifier.align(Alignment.End)) {
                        Text("Cancel")
                    }
                } else {
                    // Step 2: Enter address
                    Text(
                        if (selectedType == 0) "Enter SMB Host" else "Enter Server URL",
                        style = MaterialTheme.typography.bodyLarge
                    )
                    Spacer(modifier = Modifier.height(16.dp))
                    
                    TextField(
                        value = address,
                        onValueChange = { address = it },
                        label = { Text(if (selectedType == 0) "Host (e.g., 192.168.1.100)" else "URL (e.g., http://192.168.1.100:8080)") },
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .focusRequester(focusRequester),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                        keyboardActions = KeyboardActions(onDone = {
                            keyboardController?.hide()
                            if (address.isNotBlank()) {
                                when (selectedType) {
                                    0 -> onAddSmb(address)
                                    1 -> onAddServer(address)
                                }
                            }
                        })
                    )
                    
                    Spacer(modifier = Modifier.height(24.dp))
                    
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.End
                    ) {
                        Button(onClick = { selectedType = null; address = "" }) {
                            Text("Back")
                        }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(onClick = onDismiss) {
                            Text("Cancel")
                        }
                        Spacer(modifier = Modifier.width(8.dp))
                        Button(onClick = {
                            if (address.isNotBlank()) {
                                when (selectedType) {
                                    0 -> onAddSmb(address)
                                    1 -> onAddServer(address)
                                }
                            }
                        }) {
                            Text("Add")
                        }
                    }
                }
            }
        }
    }

    LaunchedEffect(selectedType) {
        if (selectedType != null) {
            focusRequester.requestFocus()
        } else {
            smbButtonFocusRequester.requestFocus()
        }
    }
}

@Composable
private fun ConfirmDialog(
    title: String,
    message: String,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit
) {
    val cancelFocusRequester = remember { FocusRequester() }
    
    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        Surface(
            shape = MaterialTheme.shapes.medium,
            tonalElevation = 8.dp,
            modifier = Modifier.padding(16.dp)
        ) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text(
                    text = title,
                    style = MaterialTheme.typography.headlineSmall
                )
                Spacer(modifier = Modifier.height(16.dp))
                Text(
                    text = message,
                    style = MaterialTheme.typography.bodyMedium
                )
                Spacer(modifier = Modifier.height(24.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End
                ) {
                    Button(
                        onClick = onDismiss,
                        modifier = Modifier.focusRequester(cancelFocusRequester)
                    ) {
                        Text("Cancel")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                    Button(onClick = onConfirm) {
                        Text("Confirm")
                    }
                }
            }
        }
    }
    
    LaunchedEffect(Unit) {
        cancelFocusRequester.requestFocus()
    }
}
