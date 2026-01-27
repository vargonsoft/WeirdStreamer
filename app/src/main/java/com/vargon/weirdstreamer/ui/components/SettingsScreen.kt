package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.DeleteSweep
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Palette
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Switch
import androidx.compose.runtime.*
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
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.delay

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SettingsScreen(viewModel: MediaViewModel, onBack: () -> Unit, onThemeChanged: (() -> Unit)? = null) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current
    val backFocus = remember { FocusRequester() }
    val themeFocus = remember { FocusRequester() }
    val hiddenFilesFocus = remember { FocusRequester() }
    val clearCacheFocus = remember { FocusRequester() }

    LaunchedEffect(Unit) {
        delay(100)
        backFocus.requestFocus()
    }

    val currentTheme = viewModel.credentialManager.getAppTheme()
    var themeMenuExpanded by remember { mutableStateOf(false) }
    val themeButton1Focus = remember { FocusRequester() }
    val themeButton2Focus = remember { FocusRequester() }
    val themeButton3Focus = remember { FocusRequester() }

    LaunchedEffect(themeMenuExpanded) {
        if (themeMenuExpanded) {
            delay(50)
            themeButton1Focus.requestFocus()
        }
    }

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
        LazyColumn(modifier = Modifier.fillMaxSize()) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Button(
                        onClick = onBack,
                        modifier = Modifier
                            .focusRequester(backFocus)
                            .focusProperties { 
                                down = themeFocus
                            }
                    ) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                    Spacer(modifier = Modifier.width(16.dp))
                    Text("Settings", style = MaterialTheme.typography.headlineMedium)
                }
                Spacer(modifier = Modifier.height(32.dp))
            }

            // Theme Selector
            item {
                Surface(
                    onClick = { themeMenuExpanded = !themeMenuExpanded },
                    scale = ClickableSurfaceDefaults.scale(focusedScale = 1.02f),
                    colors = ClickableSurfaceDefaults.colors(
                        containerColor = Color.Transparent,
                        focusedContainerColor = Color.Transparent,
                        pressedContainerColor = Color.Transparent
                    ),
                    border = ClickableSurfaceDefaults.border(
                        focusedBorder = Border(
                            border = BorderStroke(2.dp, Color.White),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(themeFocus)
                        .focusProperties {
                            up = backFocus
                            down = hiddenFilesFocus
                        }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(Icons.Default.Palette, contentDescription = null, modifier = Modifier.size(24.dp))
                        Spacer(modifier = Modifier.width(16.dp))
                        Column(modifier = Modifier.weight(1f)) {
                            Text("Theme", style = MaterialTheme.typography.titleMedium)
                            Text(
                                when (currentTheme) {
                                    "CINEMA_BLUE" -> "Cinema Blue"
                                    "CINEMA_RED" -> "Cinema Red"
                                    else -> "Default"
                                },
                                style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                        }
                    }
                }
                
                Spacer(modifier = Modifier.height(16.dp))
            }

            // Theme Menu Buttons (when expanded)
            if (themeMenuExpanded) {
                item {
                    Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                        val themes = listOf(
                            Triple("DEFAULT", "Default", themeButton1Focus),
                            Triple("CINEMA_BLUE", "Cinema Blue", themeButton2Focus),
                            Triple("CINEMA_RED", "Cinema Red", themeButton3Focus)
                        )
                        
                        themes.forEachIndexed { index, (themeKey, themeName, focusReq) ->
                            Button(
                                onClick = {
                                    viewModel.credentialManager.setAppTheme(themeKey)
                                    onThemeChanged?.invoke()
                                    themeMenuExpanded = false
                                },
                                colors = if (currentTheme == themeKey) {
                                    ButtonDefaults.colors()
                                } else {
                                    ButtonDefaults.colors(
                                        containerColor = Color.Transparent,
                                        contentColor = MaterialTheme.colorScheme.onSurface
                                    )
                                },
                                modifier = Modifier
                                    .focusRequester(focusReq)
                                    .focusProperties {
                                        up = themeFocus
                                        down = hiddenFilesFocus
                                        left = if (index > 0) themes[index - 1].third else FocusRequester.Default
                                        right = if (index < themes.size - 1) themes[index + 1].third else FocusRequester.Default
                                    }
                            ) {
                                Text(themeName)
                            }
                        }
                    }
                    Spacer(modifier = Modifier.height(16.dp))
                }
            }

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
                            border = BorderStroke(2.dp, Color.White),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(hiddenFilesFocus)
                        .focusProperties {
                            up = themeFocus
                            down = clearCacheFocus
                        }
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Show hidden files", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                        Switch(checked = viewModel.showHiddenFiles, onCheckedChange = null)
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
                            border = BorderStroke(2.dp, Color.White),
                            inset = 0.dp,
                            shape = MaterialTheme.shapes.small
                        )
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(clearCacheFocus)
                        .focusProperties {
                            up = hiddenFilesFocus
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
                Spacer(modifier = Modifier.height(24.dp))
                HorizontalDivider()
                Spacer(modifier = Modifier.height(24.dp))
            }

            // SMB Shares Section
            item {
                Text("SMB Shares", style = MaterialTheme.typography.headlineSmall)
                Spacer(modifier = Modifier.height(16.dp))
            }

            items(viewModel.smbHosts) { host ->
                Surface(
                    onClick = { /* Could auto-connect here if desired */ },
                    modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(Icons.Default.Dns, contentDescription = null)
                        Spacer(modifier = Modifier.width(16.dp))
                        Text(host, modifier = Modifier.weight(1f))
                        Button(onClick = { viewModel.deleteServer(host) }) {
                            Icon(Icons.Default.Delete, contentDescription = "Delete")
                        }
                    }
                }
            }

            item {
                Spacer(modifier = Modifier.height(24.dp))
                HorizontalDivider()
                Spacer(modifier = Modifier.height(24.dp))
            }
            
            // API Servers Section
            item {
                Text("API Servers", style = MaterialTheme.typography.headlineSmall)
                Spacer(modifier = Modifier.height(16.dp))
            }

            items(viewModel.apiServers) { serverUrl ->
                Surface(
                    onClick = { /* Could auto-connect here if desired */ },
                    modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Icon(Icons.Default.Dns, contentDescription = null)
                        Spacer(modifier = Modifier.width(16.dp))
                        Text(serverUrl, modifier = Modifier.weight(1f))
                        Button(onClick = { viewModel.deleteApiServer(serverUrl) }) {
                            Icon(Icons.Default.Delete, contentDescription = "Delete")
                        }
                    }
                }
            }
        }
    }
}
