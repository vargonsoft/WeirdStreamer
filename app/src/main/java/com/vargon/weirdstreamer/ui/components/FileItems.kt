package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Folder
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.SmbFileItem
import com.vargon.weirdstreamer.repository.FileItem
import java.util.Locale

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

@Composable
fun HostTileItem(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    TileItem(
        title = host,
        icon = Icons.Default.Dns,
        modifier = modifier,
        onClick = onClick
    )
}

@Composable
fun FileTileItem(item: SmbFileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    TileItem(
        title = item.name,
        subtitle = if (!item.isDirectory && item.name != "..") formatFileSize(item.size) else null,
        icon = if (item.isDirectory) Icons.Default.Folder else Icons.Default.Description,
        modifier = modifier,
        onClick = onClick,
        thumbnailUrl = if (!item.isDirectory && item.name != "..") item.path else null
    )
}

// Overload for new FileItem from repository
@Composable
fun FileTileItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    TileItem(
        title = item.name,
        subtitle = if (!item.isDirectory && item.name != "..") formatFileSize(item.size) else null,
        icon = if (item.isDirectory) Icons.Default.Folder else Icons.Default.Description,
        modifier = modifier,
        onClick = onClick,
        thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && item.name != "..") item.path else null
    )
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TileItem(
    title: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
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
                modifier = Modifier.fillMaxSize()
            ) {
                var isImageLoaded by remember(thumbnailUrl) { mutableStateOf(false) }
                
                if (!isImageLoaded) {
                    Icon(
                        icon, 
                        contentDescription = null, 
                        modifier = Modifier.fillMaxSize(0.5f).padding(10.dp)
                    )
                }
                
                if (thumbnailUrl != null) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(thumbnailUrl)
                            .crossfade(true)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        onSuccess = { isImageLoaded = true },
                        modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.small)
                    )
                }
            }
        }
        Spacer(modifier = Modifier.height(12.dp))
        Text(
            text = title,
            style = MaterialTheme.typography.titleMedium,
            maxLines = 3,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center
        )
        if (subtitle != null) {
            Text(
                text = subtitle,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center
            )
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
            Box(modifier = Modifier.size(48.dp), contentAlignment = Alignment.Center) {
                var isImageLoaded by remember(item.path) { mutableStateOf(false) }
                
                if (!isImageLoaded) {
                    Icon(icon, contentDescription = null, modifier = Modifier.size(32.dp))
                }
                
                if (!item.isDirectory && item.name != "..") {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(item.path)
                            .crossfade(true)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        onSuccess = { isImageLoaded = true },
                        modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.extraSmall)
                    )
                }
            }
            Spacer(modifier = Modifier.width(16.dp))
            Column {
                Text(text = item.name, style = MaterialTheme.typography.bodyLarge)
                if (!item.isDirectory && item.name != "..") {
                    Text(text = formatFileSize(item.size), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

// Overload for new FileItem from repository
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun FileRowItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
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
            Box(modifier = Modifier.size(48.dp), contentAlignment = Alignment.Center) {
                val thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && item.name != "..") item.path else null
                var isImageLoaded by remember(thumbnailUrl) { mutableStateOf(false) }
                
                if (!isImageLoaded) {
                    Icon(icon, contentDescription = null, modifier = Modifier.size(32.dp))
                }
                
                if (thumbnailUrl != null) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(thumbnailUrl)
                            .crossfade(true)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        onSuccess = { isImageLoaded = true },
                        modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.extraSmall)
                    )
                }
            }
            Spacer(modifier = Modifier.width(16.dp))
            Column {
                Text(text = item.name, style = MaterialTheme.typography.bodyLarge)
                if (!item.isDirectory && item.name != "..") {
                    Text(text = formatFileSize(item.size), style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

// Overload for host row with delete action
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun HostRowItem(host: String, modifier: Modifier = Modifier, onDelete: () -> Unit = {}, onClick: () -> Unit) {
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
            Text(text = host, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        }
    }
}

private fun formatFileSize(size: Long): String {
    if (size <= 0) return "0 B"
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val digitGroups = (Math.log10(size.toDouble()) / Math.log10(1024.0)).toInt()
    return String.format(Locale.US, "%.1f %s", size / Math.pow(1024.0, digitGroups.toDouble()), units[digitGroups])
}
