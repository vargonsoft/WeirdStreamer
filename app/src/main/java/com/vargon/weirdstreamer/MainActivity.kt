package com.vargon.weirdstreamer

import android.os.Bundle
import android.view.KeyEvent
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ViewList
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.Checkbox
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Switch
import androidx.compose.material3.TextField
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.compose.SubcomposeAsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.delay

class MainActivity : ComponentActivity() {
    private val viewModel: SmbViewModel by viewModels()

    @OptIn(ExperimentalTvMaterial3Api::class)
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WeirdStreamerTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                ) {
                    var showSettings by remember { mutableStateOf(false) }
                    
                    if (showSettings) {
                        SettingsScreen(
                            viewModel = viewModel,
                            onBack = { showSettings = false }
                        )
                    } else {
                        SmbBrowser(
                            viewModel = viewModel,
                            onOpenSettings = { showSettings = true }
                        )
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SmbBrowser(viewModel: SmbViewModel, onOpenSettings: () -> Unit) {
    var showAddressDialog by remember { mutableStateOf(false) }
    var serverAddress by remember { mutableStateOf("smb://") }

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
                        } else {
                            // File opening logic
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

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun <T> FocusableList(
    items: List<T>,
    initialSelection: String?,
    isGridView: Boolean,
    itemContent: @Composable (T, Modifier) -> Unit
) {
    val focusRequesters = remember(items) { List(items.size) { FocusRequester() } }
    val gridState = rememberLazyGridState()
    val listState = rememberLazyListState()
    
    val initialIndex = remember(items, initialSelection) {
        if (initialSelection == null) 0
        else {
            items.indexOfFirst { item ->
                when (item) {
                    is String -> item == initialSelection
                    is SmbFileItem -> item.path == initialSelection || item.name == initialSelection
                    else -> false
                }
            }.let { if (it == -1) 0 else it }
        }
    }

    LaunchedEffect(items, initialIndex, isGridView) {
        if (items.isNotEmpty()) {
            val safeIndex = initialIndex.coerceIn(items.indices)
            // Ensure the item is target for composition
            if (isGridView) gridState.scrollToItem(safeIndex)
            else listState.scrollToItem(safeIndex)
            
            // Wait for composition and attachment
            delay(100)
            awaitFrame()
            
            runCatching {
                focusRequesters[safeIndex].requestFocus()
            }
        }
    }

    if (isGridView) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(4),
            state = gridState,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(8.dp),
            horizontalArrangement = Arrangement.spacedBy(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            itemsIndexed(items) { index, item ->
                itemContent(item, Modifier.focusRequester(focusRequesters[index]))
            }
        }
    } else {
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize()) {
            itemsIndexed(items) { index, item ->
                itemContent(item, Modifier.focusRequester(focusRequesters[index]))
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun SettingsScreen(viewModel: SmbViewModel, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    val context = LocalContext.current

    Column(
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
        Row(verticalAlignment = Alignment.CenterVertically) {
            Button(onClick = onBack) {
                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
            }
            Spacer(modifier = Modifier.width(16.dp))
            Text("Settings", style = MaterialTheme.typography.headlineMedium)
        }

        Spacer(modifier = Modifier.height(24.dp))

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
            modifier = Modifier.fillMaxWidth()
        ) {
            Row(
                modifier = Modifier.padding(12.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text("Show hidden files", modifier = Modifier.weight(1f))
                Switch(
                    checked = viewModel.showHiddenFiles,
                    onCheckedChange = null
                )
            }
        }

        Spacer(modifier = Modifier.height(16.dp))

        Button(onClick = { viewModel.clearThumbnailCache(context) }) {
            Text("Clear Thumbnail Cache")
        }

        Spacer(modifier = Modifier.height(24.dp))
        HorizontalDivider()
        Spacer(modifier = Modifier.height(24.dp))

        Text("Saved Servers", style = MaterialTheme.typography.headlineSmall)
        Spacer(modifier = Modifier.height(16.dp))

        LazyColumn(modifier = Modifier.fillMaxSize()) {
            items(viewModel.savedServers) { host ->
                Surface(
                    onClick = { /* Could auto-connect here if desired */ },
                    modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)
                ) {
                    Row(
                        modifier = Modifier.padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Default.Dns, contentDescription = null)
                        Spacer(modifier = Modifier.width(16.dp))
                        Text(host, modifier = Modifier.weight(1f))
                        Button(onClick = { viewModel.deleteServer(host) }) {
                            Icon(Icons.Default.Delete, contentDescription = "Delete")
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun HostItemRow(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
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
                border = BorderStroke(2.dp, Color.White),
                inset = 0.dp,
                shape = MaterialTheme.shapes.small
            )
        ),
        modifier = modifier.fillMaxWidth().padding(vertical = 4.dp)
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(Icons.Default.Dns, contentDescription = null, modifier = Modifier.size(32.dp))
            Spacer(modifier = Modifier.width(16.dp))
            Text(text = host, style = MaterialTheme.typography.bodyLarge)
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun HostTileItem(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    TileItem(
        title = host,
        icon = Icons.Default.Dns,
        modifier = modifier,
        onClick = onClick
    )
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun FileTileItem(item: SmbFileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    TileItem(
        title = item.name,
        icon = if (item.isDirectory) Icons.Default.Folder else Icons.Default.Description,
        modifier = modifier,
        onClick = onClick,
        thumbnailUrl = if (!item.isDirectory) item.path else null
    )
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TileItem(
    title: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    thumbnailUrl: String? = null,
    onClick: () -> Unit
) {
    Column(
        modifier = modifier.width(200.dp),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Surface(
            onClick = onClick,
            scale = ClickableSurfaceDefaults.scale(focusedScale = 1.05f),
            colors = ClickableSurfaceDefaults.colors(
                containerColor = Color.Transparent,
                focusedContainerColor = Color.Transparent,
                pressedContainerColor = Color.Transparent
            ),
            border = ClickableSurfaceDefaults.border(
                focusedBorder = Border(
                    border = BorderStroke(4.dp, Color.White),
                    inset = 0.dp,
                    shape = MaterialTheme.shapes.medium
                )
            ),
            modifier = Modifier.aspectRatio(16f / 9f)
        ) {
            Box(
                contentAlignment = Alignment.Center, 
                modifier = Modifier.fillMaxSize().padding(10.dp)
            ) {
                Icon(icon, contentDescription = null, modifier = Modifier.fillMaxSize(0.7f))
                if (thumbnailUrl != null) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(thumbnailUrl)
                            .crossfade(true)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.small)
                    )
                }
            }
        }
        Spacer(modifier = Modifier.height(12.dp))
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun AuthDialog(host: String, onDismiss: () -> Unit, onLogin: (String, String, Boolean) -> Unit) {
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var savePassword by remember { mutableStateOf(true) }
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Surface(onClick = {}, modifier = Modifier.width(400.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text("Authentication Required", style = MaterialTheme.typography.headlineSmall)
                Text("Host: $host", style = MaterialTheme.typography.bodySmall)
                Spacer(modifier = Modifier.height(16.dp))
                
                TextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text("Username") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                    keyboardActions = KeyboardActions(onNext = { focusManager.moveFocus(FocusDirection.Down) })
                )
                
                Spacer(modifier = Modifier.height(8.dp))
                
                TextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("Password") },
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Password,
                        imeAction = ImeAction.Done
                    ),
                    keyboardActions = KeyboardActions(onDone = {
                        keyboardController?.hide()
                        onLogin(username, password, savePassword)
                    })
                )
                
                Spacer(modifier = Modifier.height(8.dp))
                
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = savePassword, onCheckedChange = { savePassword = it })
                    Text("Save credentials")
                }
                
                Spacer(modifier = Modifier.height(24.dp))

                Button(
                    onClick = { onLogin(username, password, savePassword) },
                    modifier = Modifier.align(Alignment.End)
                ) {
                    Text("Login")
                }
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun ManualAddressDialog(initialAddress: String, onDismiss: () -> Unit, onConnect: (String) -> Unit) {
    var address by remember { mutableStateOf(initialAddress) }
    val keyboardController = LocalSoftwareKeyboardController.current

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Surface(onClick = {}, modifier = Modifier.width(500.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text("Manual Connection", style = MaterialTheme.typography.headlineSmall)
                Spacer(modifier = Modifier.height(16.dp))
                TextField(
                    value = address,
                    onValueChange = { address = it },
                    modifier = Modifier.fillMaxWidth(),
                    label = { Text("smb://host/share/") },
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { 
                        onConnect(address)
                        keyboardController?.hide()
                    }),
                    singleLine = true
                )
                Spacer(modifier = Modifier.height(24.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    Button(onClick = onDismiss) { Text("Cancel") }
                    Spacer(modifier = Modifier.width(8.dp))
                    Button(onClick = { onConnect(address) }) { Text("Connect") }
                }
            }
        }
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun FileItemRow(item: SmbFileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val icon = if (item.isDirectory) Icons.Default.Folder else Icons.Default.Description
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
                border = BorderStroke(2.dp, Color.White),
                inset = 0.dp,
                shape = MaterialTheme.shapes.small
            )
        ),
        modifier = modifier.fillMaxWidth().padding(vertical = 4.dp)
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Box(modifier = Modifier.size(48.dp).padding(8.dp), contentAlignment = Alignment.Center) {
                Icon(icon, contentDescription = null, modifier = Modifier.size(32.dp))
                if (!item.isDirectory && item.name != "..") {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(item.path)
                            .crossfade(true)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.extraSmall)
                    )
                }
            }
            Spacer(modifier = Modifier.width(16.dp))
            Column {
                Text(text = item.name, style = MaterialTheme.typography.bodyLarge)
                if (!item.isDirectory && item.name != "..") {
                    Text(text = "${item.size / 1024} KB", style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}
