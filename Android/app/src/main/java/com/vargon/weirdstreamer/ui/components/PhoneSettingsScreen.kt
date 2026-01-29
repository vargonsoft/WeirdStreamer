package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DeleteSweep
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Palette
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.MediaViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneSettingsScreen(viewModel: MediaViewModel, onBack: () -> Unit, onThemeChanged: (() -> Unit)? = null) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current
    val currentTheme = viewModel.credentialManager.getAppTheme()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Settings") },
                navigationIcon = {
                    IconButton(onClick = onBack) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                }
            )
        }
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
        ) {
            // Theme Section
            item {
                Text(
                    "Appearance",
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.padding(16.dp)
                )
            }

            item {
                var showThemeDialog by remember { mutableStateOf(false) }
                ListItem(
                    headlineContent = { Text("Theme") },
                    supportingContent = {
                        Text(
                            when (currentTheme) {
                                "CINEMA_BLUE" -> "Cinema Blue"
                                "CINEMA_RED" -> "Cinema Red"
                                else -> "Default"
                            }
                        )
                    },
                    leadingContent = { Icon(Icons.Default.Palette, contentDescription = null) },
                    modifier = Modifier.clickable { showThemeDialog = true }
                )

                if (showThemeDialog) {
                    AlertDialog(
                        onDismissRequest = { showThemeDialog = false },
                        title = { Text("Select Theme") },
                        text = {
                            Column {
                                listOf(
                                    "DEFAULT" to "Default",
                                    "CINEMA_BLUE" to "Cinema Blue",
                                    "CINEMA_RED" to "Cinema Red"
                                ).forEach { (key, name) ->
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clickable {
                                                viewModel.credentialManager.setAppTheme(key)
                                                onThemeChanged?.invoke()
                                                showThemeDialog = false
                                            }
                                            .padding(vertical = 12.dp)
                                    ) {
                                        RadioButton(selected = currentTheme == key, onClick = null)
                                        Spacer(modifier = Modifier.width(8.dp))
                                        Text(name)
                                    }
                                }
                            }
                        },
                        confirmButton = {
                            TextButton(onClick = { showThemeDialog = false }) { Text("Close") }
                        }
                    )
                }
            }

            item {
                ListItem(
                    headlineContent = { Text("Show hidden files") },
                    trailingContent = {
                        Switch(
                            checked = viewModel.showHiddenFiles,
                            onCheckedChange = { viewModel.updateShowHiddenFiles(it) }
                        )
                    }
                )
            }

            item {
                ListItem(
                    headlineContent = { Text("Clear Thumbnail Cache") },
                    trailingContent = { Icon(Icons.Default.DeleteSweep, contentDescription = null) },
                    modifier = Modifier.clickable { viewModel.clearThumbnailCache(context) }
                )
            }

            // Dynamic Provider Sections
            viewModel.sourceProviders.forEach { provider ->
                item {
                    HorizontalDivider(modifier = Modifier.padding(vertical = 8.dp))
                    Text(
                        provider.displayName,
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.primary,
                        modifier = Modifier.padding(16.dp)
                    )
                }

                val servers = provider.getServers()
                if (servers.isEmpty()) {
                    item {
                        Text(
                            "No ${provider.displayName}s saved.",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
                        )
                    }
                } else {
                    items(servers) { address ->
                        ListItem(
                            headlineContent = { Text(address) },
                            leadingContent = { Icon(Icons.Default.Dns, contentDescription = null) },
                            trailingContent = {
                                IconButton(onClick = { provider.removeServer(address) }) {
                                    Icon(Icons.Default.Delete, contentDescription = "Delete")
                                }
                            }
                        )
                    }
                }
            }
        }
    }
}
