package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.accountLabel
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DeleteSweep
import androidx.compose.material.icons.filled.Palette
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material.icons.filled.ClosedCaption
import androidx.compose.ui.graphics.Color
import com.vargon.weirdstreamer.update.AppUpdater
import com.vargon.weirdstreamer.update.UpdateState
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.ui.theme.AppTheme
import com.vargon.weirdstreamer.MediaViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneSettingsScreen(viewModel: MediaViewModel, onBack: () -> Unit, onThemeChanged: (() -> Unit)? = null) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current
    var currentTheme by remember { mutableStateOf(viewModel.credentialManager.getAppTheme()) }

    // Observe auth state changes
    val authStateRefresh = viewModel.authStateRefreshTrigger

    Scaffold(
        topBar = {
            PhoneTopBarPanel {
                TopAppBar(
                    windowInsets = panelTopBarInsets(),
                    title = { Text("Settings") },
                    navigationIcon = {
                        IconButton(onClick = onBack) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                        }
                    }
                )
            }
        }
    ) { padding ->
        // The server being browsed can be managed here, as in the web UI's Settings.
        val server = viewModel.connectedServer
        val isAdmin = remember(server?.address, authStateRefresh) { server?.signedInAs()?.second ?: false }
        // This phone's and the account's tabs first, the server's (admins) after them.
        val tabs = buildList {
            add("App")
            // Account and Downloads are the account's: there's none until a server's account is open.
            if (server != null) addAll(listOf("Account", "Downloads"))
            add("Themes")
            if (server != null && isAdmin) addAll(listOf("Server", "Users", "Streams", "Jobs"))
        }
        // In the view model: playing a download from its tab replaces this screen, and coming back
        // returns to the tab (by name, as the list changes with the server).
        val selectedTab = tabs.indexOf(viewModel.settingsTab).coerceAtLeast(0)
        val tab = tabs[selectedTab]
        PhoneContentPanel(modifier = Modifier.fillMaxSize().padding(padding)) {
        Column(modifier = Modifier.fillMaxSize()) {
        if (tabs.size > 1) {
            PrimaryScrollableTabRow(selectedTabIndex = selectedTab.coerceIn(tabs.indices), edgePadding = 8.dp) {
                tabs.forEachIndexed { index, name ->
                    Tab(selected = index == selectedTab, onClick = { viewModel.settingsTab = name }, text = { Text(name) })
                }
            }
        }
        if (server == null && tab == "App") {
            Text(
                "Open a WeirdStreamer server to manage it here.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)
            )
        }
        when (tab) {
        "Server" -> ServerSettingsTab(server!!)
        "Users" -> UsersTab(server!!)
        "Streams" -> StreamsTab(server!!)
        "Jobs" -> JobsTab(server!!)
        "Account" -> AccountTab(server!!)
        "Downloads" -> DownloadsTab(viewModel)
        "Themes" -> PhoneThemePicker(AppTheme.fromKey(currentTheme), onPick = {
            viewModel.credentialManager.setAppTheme(it.name)
            currentTheme = it.name
            onThemeChanged?.invoke()
        })
        else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
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
                    headlineContent = { Text("Rewind 5 seconds when resuming") },
                    supportingContent = { Text("Playing a paused video goes back 5 seconds first") },
                    trailingContent = {
                        Switch(
                            checked = viewModel.rewindOnResume,
                            onCheckedChange = { viewModel.updateRewindOnResume(it) }
                        )
                    },
                    modifier = Modifier.clickable { viewModel.updateRewindOnResume(!viewModel.rewindOnResume) }
                )
            }

            // Subtitle style: Android's caption settings, or our own
            item {
                var showSubtitleStyle by remember { mutableStateOf(false) }
                ListItem(
                    headlineContent = { Text("Subtitle style") },
                    supportingContent = { Text(subtitleStyleSummary(SubtitleStyleStore.get(context))) },
                    trailingContent = { Icon(Icons.Default.ClosedCaption, contentDescription = null) },
                    modifier = Modifier.clickable { showSubtitleStyle = true }
                )
                if (showSubtitleStyle) PhoneSubtitleStyleDialog(onDismiss = { showSubtitleStyle = false })
            }

            item {
                ListItem(
                    headlineContent = { Text("Clear thumbnail cache") },
                    trailingContent = { Icon(Icons.Default.DeleteSweep, contentDescription = null) },
                    modifier = Modifier.clickable { viewModel.clearThumbnailCache(context) }
                )
            }

            // App update, from the APK a server publishes to its signed-in accounts: only with one signed in
            if (updateServers(viewModel).isNotEmpty()) item {
                val (title, detail) = updateLabels(AppUpdater.state)
                ListItem(
                    headlineContent = {
                        Text(title, color = if (AppUpdater.state is UpdateState.Available) MaterialTheme.colorScheme.primary else Color.Unspecified)
                    },
                    supportingContent = { Text(detail) },
                    trailingContent = { Icon(Icons.Default.SystemUpdate, contentDescription = null) },
                    modifier = Modifier.clickable { onUpdateRow(viewModel, context) }
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
                        // A server's accounts are chosen on the source list; here they're only listed.
                        val users = authStateRefresh.let {
                            viewModel.accountsOf(provider, address).map { accountLabel(it.username) }
                        }
                        var confirmRemove by remember { mutableStateOf(false) }
                        ListItem(
                            headlineContent = { Text(address) },
                            supportingContent = { Text(serverUsersText(users)) },
                            leadingContent = { Icon(serverIcon(provider), contentDescription = null) },
                            trailingContent = {
                                IconButton(onClick = { confirmRemove = true }) {
                                    Icon(Icons.Default.Delete, contentDescription = "Remove")
                                }
                            }
                        )
                        if (confirmRemove) {
                            AlertDialog(
                                onDismissRequest = { confirmRemove = false },
                                title = { Text("Remove server?") },
                                text = { Text(removeServerWarning(address, users)) },
                                confirmButton = {
                                    PhoneButton(destructive = true, onClick = {
                                        confirmRemove = false
                                        val isCurrent = address == viewModel.currentSource?.serverAddress
                                        viewModel.removeServer(provider, address)
                                        if (isCurrent) onBack()
                                    }) { Text("Remove") }
                                },
                                dismissButton = { PhoneSecondaryButton(onClick = { confirmRemove = false }) { Text("Cancel") } }
                            )
                        }
                    }
                }
            }
        }
        }
        }
        }
    }
}
