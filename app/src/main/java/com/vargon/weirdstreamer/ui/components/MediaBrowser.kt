package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ViewList
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.TextField
import androidx.compose.runtime.*
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
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.repository.MediaSource

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun MediaBrowser(viewModel: MediaViewModel, onOpenSettings: () -> Unit) {
    var showAddServerDialog by remember { mutableStateOf(false) }
    var showAuthDialog by remember { mutableStateOf(false) }

    val currentSource = viewModel.currentSource

    BackHandler(enabled = currentSource != null) {
        viewModel.navigateUp()
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(48.dp)
            .onPreviewKeyEvent { 
                if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                    if (currentSource != null) {
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
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = when (currentSource) {
                    is MediaSource.Smb -> "SMB: ${viewModel.currentPath}"
                    is MediaSource.Server -> "Server: ${currentSource.displayName()}${if (viewModel.currentPath.isNotEmpty()) " / ${viewModel.currentPath}" else ""}"
                    null -> "Select Media Source"
                },
                style = MaterialTheme.typography.headlineMedium,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            
            if (currentSource != null) {
                Button(onClick = { viewModel.returnToSourceSelection() }) {
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
            } else {
                // Only show Add Source button on server selection screen
                Button(onClick = { showAddServerDialog = true }) {
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
                FileBrowserScreen(viewModel)
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
    
    Column(modifier = Modifier.fillMaxSize()) {
        // SMB Shares Section
        Text(
            "SMB Shares",
            style = MaterialTheme.typography.titleLarge,
            modifier = Modifier.padding(bottom = 16.dp)
        )
        
        Box(modifier = Modifier.weight(1f)) {
            if (viewModel.smbHosts.isEmpty()) {
                Text("No SMB shares found. Use 'Add Source' to add manually.", 
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color.Gray,
                    modifier = Modifier.padding(top = 16.dp))
            } else {
                FocusableList(
                    items = viewModel.smbHosts,
                    initialSelection = null,
                    isGridView = viewModel.isGridView
                ) { host, modifier ->
                    if (viewModel.isGridView) {
                        HostTileItem(host, modifier = modifier) {
                            viewModel.selectSmbSource(host)
                        }
                    } else {
                        HostRowItem(host, modifier = modifier, onDelete = {
                            viewModel.deleteServer(host)
                        }) {
                            viewModel.selectSmbSource(host)
                        }
                    }
                }
            }
        }

        Spacer(modifier = Modifier.height(32.dp))
        HorizontalDivider()
        Spacer(modifier = Modifier.height(32.dp))

        // API Servers Section
        Text(
            "API Servers",
            style = MaterialTheme.typography.titleLarge,
            modifier = Modifier.padding(bottom = 16.dp)
        )
        
        Box(modifier = Modifier.weight(1f)) {
            if (viewModel.apiServers.isEmpty()) {
                Text("No API servers configured. Use 'Add Source' to add a server.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color.Gray,
                    modifier = Modifier.padding(top = 16.dp))
            } else {
                FocusableList(
                    items = viewModel.apiServers,
                    initialSelection = null,
                    isGridView = viewModel.isGridView
                ) { serverUrl, modifier ->
                    if (viewModel.isGridView) {
                        HostTileItem(serverUrl, modifier = modifier) {
                            viewModel.selectServerSource(serverUrl)
                        }
                    } else {
                        HostRowItem(serverUrl, modifier = modifier, onDelete = {
                            viewModel.deleteApiServer(serverUrl)
                        }) {
                            viewModel.selectServerSource(serverUrl)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun FileBrowserScreen(viewModel: MediaViewModel) {
    FocusableList(
        items = viewModel.fileList,
        initialSelection = viewModel.getSelection(viewModel.currentPath),
        isGridView = viewModel.isGridView
    ) { item, modifier ->
        if (viewModel.isGridView) {
            FileTileItem(item, modifier = modifier) {
                handleFileClick(viewModel, item.path, item.isDirectory)
            }
        } else {
            FileRowItem(item, modifier = modifier) {
                handleFileClick(viewModel, item.path, item.isDirectory)
            }
        }
    }
}

private fun handleFileClick(viewModel: MediaViewModel, path: String, isDirectory: Boolean) {
    android.util.Log.d("MediaBrowser", "handleFileClick: path=$path, isDirectory=$isDirectory")
    
    if (path == "UP") {
        viewModel.navigateUp()
        return
    }

    if (isDirectory) {
        viewModel.recordSelection(viewModel.currentPath, path)
        viewModel.currentSource?.let { source ->
            when (source) {
                is MediaSource.Smb -> viewModel.loadPath(source, path)
                is MediaSource.Server -> {
                    // Extract just the path part from the full path
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
    val keyboardController = LocalSoftwareKeyboardController.current

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
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
                        modifier = Modifier.fillMaxWidth()
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
        }
    }
}
