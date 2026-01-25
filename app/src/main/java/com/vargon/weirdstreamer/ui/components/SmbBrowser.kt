package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ViewList
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material.icons.filled.Settings
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.SmbViewModel

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SmbBrowser(viewModel: SmbViewModel, onOpenSettings: () -> Unit) {
    var showAddressDialog by remember { mutableStateOf(false) }
    val serverAddress by remember { mutableStateOf("smb://") }

    BackHandler(enabled = viewModel.currentPath != "smb://") {
        viewModel.navigateUp()
    }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(48.dp)
            .onPreviewKeyEvent { 
                if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                    if (viewModel.currentPath != "smb://") {
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
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = viewModel.currentPath,
                style = MaterialTheme.typography.headlineMedium,
                modifier = Modifier.weight(1f),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
            Button(onClick = { viewModel.toggleViewMode() }) {
                Icon(
                    if (viewModel.isGridView) Icons.AutoMirrored.Filled.ViewList else Icons.Default.GridView,
                    contentDescription = "Switch View"
                )
            }
            Spacer(modifier = Modifier.width(16.dp))
            Button(onClick = { showAddressDialog = true }) {
                Text("Manual Address")
            }
            Spacer(modifier = Modifier.width(16.dp))
            Button(onClick = onOpenSettings) {
                Icon(Icons.Default.Settings, contentDescription = "Settings")
            }
        }

        Spacer(modifier = Modifier.height(24.dp))

        if (viewModel.isLoading) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Text("Loading...", style = MaterialTheme.typography.displaySmall)
            }
        } else if (viewModel.errorMessage != null) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(text = "Error: ${viewModel.errorMessage}", color = Color.Red)
                    Spacer(modifier = Modifier.height(16.dp))
                    Button(onClick = { viewModel.loadPath(viewModel.currentPath) }) {
                        Text("Retry")
                    }
                }
            }
        } else if (viewModel.currentPath == "smb://") {
            FocusableList(
                items = viewModel.allHosts,
                initialSelection = viewModel.getSelection(viewModel.currentPath),
                isGridView = viewModel.isGridView
            ) { host, modifier ->
                if (viewModel.isGridView) {
                    HostTileItem(host, modifier = modifier) {
                        viewModel.recordSelection(viewModel.currentPath, host)
                        viewModel.loadPath("smb://$host/")
                    }
                } else {
                    HostItemRow(host, modifier = modifier) {
                        viewModel.recordSelection(viewModel.currentPath, host)
                        viewModel.loadPath("smb://$host/")
                    }
                }
            }
        } else {
            FocusableList(
                items = viewModel.fileList,
                initialSelection = viewModel.getSelection(viewModel.currentPath),
                isGridView = viewModel.isGridView
            ) { item, modifier ->
                if (viewModel.isGridView) {
                    FileTileItem(item, modifier = modifier) {
                        if (item.name == "..") {
                            viewModel.navigateUp()
                        } else if (item.isDirectory) {
                            viewModel.recordSelection(viewModel.currentPath, item.path)
                            viewModel.loadPath(item.path)
                        }
                    }
                } else {
                    FileItemRow(item, modifier = modifier) {
                        if (item.name == "..") {
                            viewModel.navigateUp()
                        } else if (item.isDirectory) {
                            viewModel.recordSelection(viewModel.currentPath, item.path)
                            viewModel.loadPath(item.path)
                        }
                    }
                }
            }
        }
    }

    if (showAddressDialog) {
        ManualAddressDialog(
            initialAddress = serverAddress,
            onDismiss = { showAddressDialog = false },
            onConnect = { address ->
                viewModel.loadPath(address)
                showAddressDialog = false
            }
        )
    }

    viewModel.authRequiredHost?.let { host ->
        AuthDialog(
            host = host,
            onDismiss = { /* Handle cancel */ },
            onLogin = { user, pass, save ->
                viewModel.loadPath("smb://$host/", user, pass, save)
            }
        )
    }
}
