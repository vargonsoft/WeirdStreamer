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
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Memory
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

/** The TV Settings' tabs. Account needs a server open; Server, an admin's account on it. */
private enum class TvSettingsTab(val title: String) { General("General"), Account("Account"), Sources("Sources"), Server("Server"), Themes("Themes") }

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SettingsScreen(viewModel: MediaViewModel, onBack: () -> Unit, onThemeChanged: (() -> Unit)? = null) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current
    val backFocus = remember { FocusRequester() }
    // Tabs beside the title: this TV's own settings, the account's, the saved sources, the
    // server's (admins) and the themes.
    var tabName by androidx.compose.runtime.saveable.rememberSaveable { mutableStateOf(TvSettingsTab.General.name) }
    val tabFocus = remember { TvSettingsTab.entries.associateWith { FocusRequester() } }
    val currentThemeFocus = remember { FocusRequester() }
    val hiddenFilesFocus = remember { FocusRequester() }
    val rewindFocus = remember { FocusRequester() }
    val clearCacheFocus = remember { FocusRequester() }
    val updateFocus = remember { FocusRequester() }
    val subtitleStyleFocus = remember { FocusRequester() }
    val firstSourceFocus = remember { FocusRequester() }
    var showSubtitleStyle by remember { mutableStateOf(false) }
    if (showSubtitleStyle) TvSubtitleStyleDialog(onDismiss = { showSubtitleStyle = false })
    // The account's languages (on the server), with a server open.
    val languagesFocus = remember { FocusRequester() }
    val languageServer = viewModel.connectedServer
    var account by remember(languageServer?.address) { mutableStateOf<com.vargon.weirdstreamer.api.models.AccountSettings?>(null) }
    var showLanguages by remember { mutableStateOf(false) }
    LaunchedEffect(languageServer?.address) {
        account = languageServer?.let { s -> runCatching { s.api.getAccount().body() }.getOrNull() }
    }
    // Hardware encoding (admins): the server's GPUs and the transcode encoder, with a server open.
    val hardwareFocus = remember { FocusRequester() }
    val hardwareServer = languageServer?.takeIf { it.signedInAs().second }
    var hardware by remember(hardwareServer?.address) { mutableStateOf<com.vargon.weirdstreamer.api.models.HardwareReport?>(null) }
    var serverConfig by remember(hardwareServer?.address) { mutableStateOf<com.google.gson.JsonObject?>(null) }
    var showHardware by remember { mutableStateOf(false) }
    LaunchedEffect(hardwareServer?.address) {
        val s = hardwareServer ?: return@LaunchedEffect
        serverConfig = runCatching { s.api.getConfig().body() }.getOrNull()
        while (true) { // the server tries its GPUs at startup: asked again until that's done
            val report = runCatching { s.api.getHardware().body() }.getOrNull() ?: break
            hardware = report
            if (!report.checking) break
            kotlinx.coroutines.delay(2000)
        }
    }
    val hw = hardware
    val hwConfig = serverConfig
    if (showHardware && hardwareServer != null && hw != null && hwConfig != null) {
        TvHardwareDialog(hardwareServer, hw, hwConfig, onChanged = { serverConfig = it }, onDismiss = { showHardware = false })
    }
    val a = account
    if (showLanguages && languageServer != null && a != null) {
        TvLanguagesDialog(languageServer, a, onChanged = { account = it }, onDismiss = { showLanguages = false })
    }
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
    val tabs = TvSettingsTab.entries.filter {
        when (it) {
            TvSettingsTab.Account -> languageServer != null
            TvSettingsTab.Server -> hardwareServer != null
            else -> true
        }
    }
    val tab = TvSettingsTab.valueOf(tabName).takeIf { it in tabs } ?: TvSettingsTab.General
    val hasSources = viewModel.sourceProviders.any { it.getServers().isNotEmpty() }
    val firstBelowTabs = when (tab) {
        TvSettingsTab.General -> hiddenFilesFocus
        TvSettingsTab.Account -> languagesFocus
        TvSettingsTab.Sources -> if (hasSources) firstSourceFocus else FocusRequester.Default
        TvSettingsTab.Server -> hardwareFocus
        TvSettingsTab.Themes -> currentThemeFocus
    }
    val tabUp = tabFocus.getValue(tab)

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
                    tabs.forEachIndexed { i, t ->
                        if (i > 0) Spacer(modifier = Modifier.width(12.dp))
                        Button(shape = tvButtonShape(),
                            onClick = { tabName = t.name },
                            colors = if (tab == t) tvButtonColors()
                                else tvButtonColors(containerColor = Color.Transparent, contentColor = MaterialTheme.colorScheme.onSurface),
                            modifier = Modifier.focusRequester(tabFocus.getValue(t)).focusProperties { down = firstBelowTabs }
                        ) {
                            Text(t.title)
                        }
                    }
                }
                Spacer(modifier = Modifier.height(32.dp))
            }

            when (tab) {
            TvSettingsTab.Themes -> item {
                TvThemePicker(AppTheme.fromKey(currentTheme), onPick = {
                    viewModel.credentialManager.setAppTheme(it.name)
                    currentTheme = it.name
                    onThemeChanged?.invoke()
                }, up = tabUp, currentFocus = currentThemeFocus)
            }

            // This TV's own settings
            TvSettingsTab.General -> {
                item {
                    TvSettingsRow("Show hidden files", focus = hiddenFilesFocus, up = tabUp, down = rewindFocus,
                        onClick = { viewModel.updateShowHiddenFiles(!viewModel.showHiddenFiles) }) {
                        Switch(checked = viewModel.showHiddenFiles, onCheckedChange = null, colors = tvSwitchColors())
                    }
                    TvSettingsRow("Rewind 5 seconds when resuming", focus = rewindFocus, up = hiddenFilesFocus, down = subtitleStyleFocus,
                        onClick = { viewModel.updateRewindOnResume(!viewModel.rewindOnResume) }) {
                        Switch(checked = viewModel.rewindOnResume, onCheckedChange = null, colors = tvSwitchColors())
                    }
                    // Subtitle style: Android's caption settings, or our own
                    TvSettingsRow("Subtitle style", subtitleStyleSummary(SubtitleStyleStore.get(context)), subtitleStyleFocus,
                        up = rewindFocus, down = clearCacheFocus, onClick = { showSubtitleStyle = true }) {
                        Icon(Icons.Default.ClosedCaption, contentDescription = null)
                    }
                    TvSettingsRow("Clear thumbnail cache", focus = clearCacheFocus, up = subtitleStyleFocus, down = updateFocus,
                        onClick = { viewModel.clearThumbnailCache(context) }) {
                        Icon(Icons.Default.DeleteSweep, contentDescription = null)
                    }
                    // App update, from the APK a server publishes to its signed-in accounts, or GitHub's
                    val (title, detail) = updateLabels(AppUpdater.state)
                    TvSettingsRow(title, detail, updateFocus, up = clearCacheFocus,
                        titleColor = if (AppUpdater.state is UpdateState.Available) WeirdStreamerTheme.spec.headingColor else Color.Unspecified,
                        onClick = { onUpdateRow(viewModel, context) }) {
                        Icon(Icons.Default.SystemUpdate, contentDescription = null)
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

            // The account in use on the open server: its languages, and signing another TV in
            TvSettingsTab.Account -> item {
                TvSettingsRow("Languages", accountLanguagesSummary(account), languagesFocus, up = tabUp,
                    onClick = { if (account != null) showLanguages = true }) {
                    Icon(Icons.Default.Language, contentDescription = null)
                }
                Spacer(modifier = Modifier.height(8.dp))
                HorizontalDivider()
                Spacer(modifier = Modifier.height(24.dp))
                // The account in use signs another TV in, with the code that TV shows.
                languageServer?.let { TvSignInTvSection(it) }
            }

            // The open server's settings (admins)
            TvSettingsTab.Server -> item {
                TvSettingsRow("Hardware encoding", hardwareSummary(hardware, serverConfig), hardwareFocus, up = tabUp,
                    onClick = { if (hardware != null && serverConfig != null) showHardware = true }) {
                    Icon(Icons.Default.Memory, contentDescription = null)
                }
            }

            // The saved servers and shares
            TvSettingsTab.Sources -> {
                var first = true
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
                        val focusFirst = first
                        first = false
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
                                        ),
                                        modifier = if (focusFirst && address == servers.first())
                                            Modifier.focusRequester(firstSourceFocus).focusProperties { up = tabUp } else Modifier
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
            }
        }
        }
    }
}

/**
 * A Settings row: a title (and a line under it) with something at the end (a switch, an icon),
 * the whole row one button, bordered when focused. [up] and [down] chain the focus down a tab.
 */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
private fun TvSettingsRow(
    title: String,
    detail: String? = null,
    focus: FocusRequester,
    up: FocusRequester? = null,
    down: FocusRequester? = null,
    titleColor: Color = Color.Unspecified,
    onClick: () -> Unit,
    trailing: @Composable () -> Unit
) {
    Surface(
        onClick = onClick,
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
            .focusRequester(focus)
            .focusProperties {
                up?.let { this.up = it }
                down?.let { this.down = it }
            }
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.titleMedium, color = titleColor)
                if (detail != null) {
                    Text(detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            trailing()
        }
    }
    Spacer(modifier = Modifier.height(16.dp))
}
