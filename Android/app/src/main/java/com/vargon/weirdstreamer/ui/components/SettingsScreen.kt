package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.accountLabel
import com.vargon.weirdstreamer.repository.SourceProvider
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DeleteSweep
import androidx.compose.material.icons.filled.Palette
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Switch
import androidx.compose.runtime.*
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material.icons.filled.ClosedCaption
import com.vargon.weirdstreamer.update.AppUpdater
import com.vargon.weirdstreamer.update.UpdateState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ui.theme.AppTheme
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.delay

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SettingsScreen(viewModel: MediaViewModel, onBack: () -> Unit, onThemeChanged: (() -> Unit)? = null) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current
    val backFocus = remember { FocusRequester() }
    // Two tabs, as on the phone: General (this list) and Themes (the theme picker).
    var tab by androidx.compose.runtime.saveable.rememberSaveable { mutableIntStateOf(0) }
    val generalTabFocus = remember { FocusRequester() }
    val themesTabFocus = remember { FocusRequester() }
    val currentThemeFocus = remember { FocusRequester() }
    val hiddenFilesFocus = remember { FocusRequester() }
    val clearCacheFocus = remember { FocusRequester() }
    val updateFocus = remember { FocusRequester() }
    val subtitleStyleFocus = remember { FocusRequester() }
    var showSubtitleStyle by remember { mutableStateOf(false) }
    if (showSubtitleStyle) TvSubtitleStyleDialog(onDismiss = { showSubtitleStyle = false })
    // Remove asks first, naming the accounts that go with the server.
    var removeChoice by remember { mutableStateOf<Triple<SourceProvider, String, List<String>>?>(null) }
    removeChoice?.let { (provider, address, users) ->
        TvRemoveServerDialog(address, users, onDismiss = { removeChoice = null }) {
            removeChoice = null
            val isCurrent = address == viewModel.currentSource?.serverAddress
            viewModel.removeServer(provider, address)
            if (isCurrent) onBack()
        }
    }

    // Observe auth state changes
    val authStateRefresh = viewModel.authStateRefreshTrigger

    LaunchedEffect(Unit) {
        delay(100)
        backFocus.requestFocus()
    }

    var currentTheme by remember { mutableStateOf(viewModel.credentialManager.getAppTheme()) }
    val firstBelowTabs = if (tab == 0) hiddenFilesFocus else currentThemeFocus

    Box(
        modifier = Modifier
            .fillMaxSize()
            .padding(48.dp)
            .onPreviewKeyEvent { 
                if (it.key == Key.Backspace && it.type == KeyEventType.KeyUp) {
                    onBack()
                    true
                } else {
                    false
                }
            }
    ) {
        // In a box, like the web UI's settings page, in themes that draw them
        Panel(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 24.dp, end = 24.dp)) {
        // The list's own padding, not the panel's: it clips at its edges, and focused buttons grow.
        LazyColumn(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(vertical = 24.dp)) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Button(shape = tvButtonShape(), colors = tvButtonColors(),
                        onClick = onBack,
                        modifier = Modifier
                            .focusRequester(backFocus)
                            .focusProperties { 
                                down = firstBelowTabs
                            }
                    ) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                    Text("Settings", style = MaterialTheme.typography.headlineMedium, color = WeirdStreamerTheme.spec.headingColor)
                    Spacer(modifier = Modifier.weight(1f))
                    listOf("General" to generalTabFocus, "Themes" to themesTabFocus).forEachIndexed { i, (name, focus) ->
                        if (i > 0) Spacer(modifier = Modifier.width(12.dp))
                        Button(shape = tvButtonShape(),
                            onClick = { tab = i },
                            colors = if (tab == i) tvButtonColors()
                                else tvButtonColors(containerColor = Color.Transparent, contentColor = MaterialTheme.colorScheme.onSurface),
                            modifier = Modifier.focusRequester(focus).focusProperties { down = firstBelowTabs }
                        ) {
                            Text(name)
                        }
                    }
                }
                Spacer(modifier = Modifier.height(32.dp))
            }

            if (tab == 1) item {
                TvThemePicker(AppTheme.fromKey(currentTheme), onPick = {
                    viewModel.credentialManager.setAppTheme(it.name)
                    currentTheme = it.name
                    onThemeChanged?.invoke()
                }, up = themesTabFocus, currentFocus = currentThemeFocus)
            }

            if (tab == 0) {
            // Hidden Files Toggle
            item {
                Surface(
                    onClick = { viewModel.updateShowHiddenFiles(!viewModel.showHiddenFiles) },
                    scale = ClickableSurfaceDefaults.scale(focusedScale = 1.02f),
                    colors = ClickableSurfaceDefaults.colors(
                        containerColor = Color.Transparent,
                        focusedContainerColor = Color.Transparent,
                        pressedContainerColor = Color.Transparent
                    ),
                    border = ClickableSurfaceDefaults.border(
                        focusedBorder = Border(
                            border = BorderStroke(2.dp, tvFocusBorderColor()),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(hiddenFilesFocus)
                        .focusProperties {
                            up = generalTabFocus
                            down = clearCacheFocus
                        }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Show hidden files", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                        Switch(checked = viewModel.showHiddenFiles, onCheckedChange = null, colors = tvSwitchColors())
                    }
                }
                Spacer(modifier = Modifier.height(16.dp))
            }

            // Clear Cache
            item {
                Surface(
                    onClick = { viewModel.clearThumbnailCache(context) },
                    scale = ClickableSurfaceDefaults.scale(focusedScale = 1.02f),
                    colors = ClickableSurfaceDefaults.colors(
                        containerColor = Color.Transparent,
                        focusedContainerColor = Color.Transparent,
                        pressedContainerColor = Color.Transparent
                    ),
                    border = ClickableSurfaceDefaults.border(
                        focusedBorder = Border(
                            border = BorderStroke(2.dp, tvFocusBorderColor()),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(clearCacheFocus)
                        .focusProperties {
                            up = hiddenFilesFocus
                            down = subtitleStyleFocus
                        }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Clear Thumbnail Cache", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                        Icon(Icons.Default.DeleteSweep, contentDescription = null)
                    }
                }
                Spacer(modifier = Modifier.height(16.dp))
                // Subtitle style: Android's caption settings, or our own
                Surface(
                    onClick = { showSubtitleStyle = true },
                    scale = ClickableSurfaceDefaults.scale(focusedScale = 1.02f),
                    colors = ClickableSurfaceDefaults.colors(
                        containerColor = Color.Transparent,
                        focusedContainerColor = Color.Transparent,
                        pressedContainerColor = Color.Transparent
                    ),
                    border = ClickableSurfaceDefaults.border(
                        focusedBorder = Border(
                            border = BorderStroke(2.dp, tvFocusBorderColor()),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(subtitleStyleFocus)
                        .focusProperties { up = clearCacheFocus; down = updateFocus }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Subtitle style", style = MaterialTheme.typography.titleMedium)
                            Text(subtitleStyleSummary(SubtitleStyleStore.get(context)), style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Icon(Icons.Default.ClosedCaption, contentDescription = null)
                    }
                }
                Spacer(modifier = Modifier.height(16.dp))
                // App update, from the APK a server publishes
                Surface(
                    onClick = { onUpdateRow(viewModel, context) },
                    scale = ClickableSurfaceDefaults.scale(focusedScale = 1.02f),
                    colors = ClickableSurfaceDefaults.colors(
                        containerColor = Color.Transparent,
                        focusedContainerColor = Color.Transparent,
                        pressedContainerColor = Color.Transparent
                    ),
                    border = ClickableSurfaceDefaults.border(
                        focusedBorder = Border(
                            border = BorderStroke(2.dp, tvFocusBorderColor()),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(updateFocus)
                        .focusProperties { up = subtitleStyleFocus }
                ) {
                    val (title, detail) = updateLabels(AppUpdater.state)
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(title, style = MaterialTheme.typography.titleMedium,
                                color = if (AppUpdater.state is UpdateState.Available) WeirdStreamerTheme.spec.headingColor else Color.Unspecified)
                            Text(detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        Icon(Icons.Default.SystemUpdate, contentDescription = null)
                    }
                }
                Spacer(modifier = Modifier.height(24.dp))
                HorizontalDivider()
                Spacer(modifier = Modifier.height(24.dp))
            }

            // Dynamic Provider Sections
            viewModel.sourceProviders.forEach { provider ->
                item {
                    Text(provider.displayName, style = MaterialTheme.typography.headlineSmall, color = WeirdStreamerTheme.spec.headingColor)
                    Spacer(modifier = Modifier.height(16.dp))
                }

                val servers = provider.getServers()
                if (servers.isEmpty()) {
                    item {
                        Text("No ${provider.displayName}s saved.", style = MaterialTheme.typography.bodyMedium, color = Color.Gray)
                    }
                } else {
                    items(servers) { address ->
                        Box(
                            modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 12.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Icon(serverIcon(provider), contentDescription = null, modifier = Modifier.size(24.dp))
                                Spacer(modifier = Modifier.width(16.dp))
                                // A server's accounts are chosen on the source list; here they're only listed.
                                val users = authStateRefresh.let {
                                    viewModel.accountsOf(provider, address).map { accountLabel(it.username) }
                                }
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(address, style = MaterialTheme.typography.titleMedium)
                                    Text(serverUsersText(users),
                                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                Button(shape = tvButtonShape(), 
                                    onClick = { removeChoice = Triple(provider, address, users) },
                                    colors = ButtonDefaults.colors(
                                        containerColor = Color.Red.copy(alpha = 0.2f),
                                        contentColor = Color.Red
                                    )
                                ) {
                                    Icon(Icons.Default.Delete, contentDescription = null)
                                    Spacer(modifier = Modifier.width(8.dp))
                                    Text("Remove")
                                }
                            }
                        }
                    }
                }

                item {
                    Spacer(modifier = Modifier.height(24.dp))
                    HorizontalDivider()
                    Spacer(modifier = Modifier.height(24.dp))
                }
            }

            }

            // Required by TMDB's terms for apps that show its data.
            item {
                Text(
                    "Movie and TV information and images from TMDB (themoviedb.org). " +
                        "This product uses the TMDB API but is not endorsed or certified by TMDB.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color.Gray,
                    modifier = Modifier.padding(vertical = 16.dp)
                )
            }
        }
        }
    }
}
