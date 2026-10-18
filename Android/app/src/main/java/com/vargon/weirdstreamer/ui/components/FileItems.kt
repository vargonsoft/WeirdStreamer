package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.SubdirectoryArrowLeft
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.SmbFileItem
import com.vargon.weirdstreamer.repository.FileItem
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import java.util.Locale

// Helper to get icon color based on theme spec
@Composable
private fun getIconTint(icon: ImageVector): Color {
    val spec = WeirdStreamerTheme.spec
    val defaultColor = LocalContentColor.current
    return when (icon) {
        Icons.Default.Folder -> spec.folderIconTint.takeOrElse { defaultColor }
        Icons.Default.Dns -> spec.serverIconTint.takeOrElse { defaultColor }
        Icons.Default.SubdirectoryArrowLeft -> spec.parentIconTint.takeOrElse { defaultColor }
        else -> defaultColor
    }
}

private fun Color.takeOrElse(block: () -> Color): Color = if (this != Color.Unspecified) this else block()

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
            Icon(
                Icons.Default.Dns, 
                contentDescription = null, 
                modifier = Modifier.size(32.dp),
                tint = getIconTint(Icons.Default.Dns)
            )
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
    val isParent = item.name == ".."
    val subtitle = if (!item.isDirectory && !isParent) {
        val sizeStr = formatFileSize(item.size)
        if (item.duration != null && item.duration > 0) {
            "$sizeStr, ${formatDuration(item.duration)}"
        } else {
            sizeStr
        }
    } else null

    TileItem(
        title = item.name,
        subtitle = subtitle,
        icon = when {
            isParent -> Icons.Default.SubdirectoryArrowLeft
            item.isDirectory -> Icons.Default.Folder
            else -> Icons.Default.Description
        },
        modifier = modifier,
        onClick = onClick,
        thumbnailUrl = if (!item.isDirectory && !isParent) item.path else null,
        rotateIcon = if (isParent) 90f else 0f
    )
}

// Overload for new FileItem from repository
@Composable
fun FileTileItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val viewModel: MediaViewModel = viewModel()
    
    LaunchedEffect(item.path) {
        if (!item.isDirectory && !isParent && (item.duration == null || item.duration <= 0) && viewModel.isVideo(item.path)) {
            viewModel.fetchMetadata(item)
        }
    }

    val subtitle = if (!item.isDirectory && !isParent) {
        val sizeStr = formatFileSize(item.size)
        if (item.duration != null && item.duration > 0) {
            "$sizeStr, ${formatDuration(item.duration)}"
        } else {
            sizeStr
        }
    } else null

    TileItem(
        title = item.name,
        subtitle = subtitle,
        icon = when {
            isParent -> Icons.Default.SubdirectoryArrowLeft
            item.isDirectory -> Icons.Default.Folder
            else -> Icons.Default.Description
        },
        modifier = modifier,
        onClick = onClick,
        thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && !isParent) item.path else null,
        rotateIcon = if (isParent) 90f else 0f
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
    rotateIcon: Float = 0f,
    onClick: () -> Unit
) {
    Column(
        modifier = modifier.fillMaxWidth(),
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
            modifier = Modifier.fillMaxWidth().aspectRatio(16f / 9f)
        ) {
            Box(
                contentAlignment = Alignment.Center, 
                modifier = Modifier.fillMaxSize()
            ) {
                var isImageLoaded by remember(thumbnailUrl) { mutableStateOf(false) }
                
                if (!isImageLoaded) {
                    val isLargeIcon = icon == Icons.Default.Folder || icon == Icons.Default.Dns || icon == Icons.Default.SubdirectoryArrowLeft
                    val iconSizeFraction = if (isLargeIcon) 0.8f else 0.5f
                    val iconPadding = if (isLargeIcon) 4.dp else 10.dp
                    
                    Icon(
                        icon, 
                        contentDescription = null, 
                        modifier = Modifier
                            .fillMaxSize(iconSizeFraction)
                            .padding(iconPadding)
                            .graphicsLayer(rotationZ = rotateIcon),
                        tint = getIconTint(icon)
                    )
                }
                
                if (thumbnailUrl != null) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(thumbnailUrl)
                            .crossfade(true)
                            .size(300)
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
        Column(
            modifier = Modifier.height(80.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
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
}

@Preview(name = "Single-line filename", widthDp = 240, heightDp = 220, showBackground = true)
@Composable
private fun SingleLineTileItemPreview() {
    WeirdStreamerTheme {
        TileItem(
            title = "Summer.jpg",
            subtitle = "2.4 MB",
            icon = Icons.Default.Image,
            modifier = Modifier.width(220.dp),
            onClick = {}
        )
    }
}

@Preview(name = "Multi-line filename", widthDp = 240, heightDp = 220, showBackground = true)
@Composable
private fun MultiLineTileItemPreview() {
    WeirdStreamerTheme {
        TileItem(
            title = "A very long summer vacation image filename.jpg",
            subtitle = "2.4 MB",
            icon = Icons.Default.Image,
            modifier = Modifier.width(220.dp),
            onClick = {}
        )
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun FileItemRow(item: SmbFileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".."
    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }
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
                    Icon(
                        icon,
                        contentDescription = null,
                        modifier = Modifier
                            .size(32.dp)
                            .graphicsLayer(rotationZ = if (isParent) 90f else 0f),
                        tint = getIconTint(icon)
                    )
                }
                
                if (!item.isDirectory && !isParent) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(item.path)
                            .crossfade(true)
                            .size(48)
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
                if (!item.isDirectory && !isParent) {
                    val sizeStr = formatFileSize(item.size)
                    val infoStr = if (item.duration != null && item.duration > 0) {
                        "$sizeStr, ${formatDuration(item.duration)}"
                    } else {
                        sizeStr
                    }
                    Text(text = infoStr, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

// Overload for new FileItem from repository
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun FileRowItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val viewModel: MediaViewModel = viewModel()
    
    LaunchedEffect(item.id, item.path, item.duration) {
        if (!item.isDirectory && !isParent && (item.duration == null || item.duration <= 0) && viewModel.isVideo(item.path)) {
            viewModel.fetchMetadata(item)
        }
    }

    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }
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
                val thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && !isParent) item.path else null
                var isImageLoaded by remember(thumbnailUrl) { mutableStateOf(false) }
                
                if (!isImageLoaded) {
                    Icon(
                        icon,
                        contentDescription = null,
                        modifier = Modifier
                            .size(32.dp)
                            .graphicsLayer(rotationZ = if (isParent) 90f else 0f),
                        tint = getIconTint(icon)
                    )
                }
                
                if (thumbnailUrl != null) {
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(thumbnailUrl)
                            .crossfade(false)
                            .size(48)
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
                if (!item.isDirectory && !isParent) {
                    val sizeStr = formatFileSize(item.size)
                    val infoStr = if (item.duration != null && item.duration > 0) {
                        "$sizeStr, ${formatDuration(item.duration)}"
                    } else {
                        sizeStr
                    }
                    Text(text = infoStr, style = MaterialTheme.typography.bodySmall)
                }
            }
        }
    }
}

// Overload for host row with delete action
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun HostRowItem(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
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
            Icon(
                Icons.Default.Dns, 
                contentDescription = null, 
                modifier = Modifier.size(32.dp),
                tint = getIconTint(Icons.Default.Dns)
            )
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

private fun formatDuration(durationInSeconds: Double): String {
    val totalSeconds = durationInSeconds.toInt()
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    
    return if (hours > 0) {
        String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
    } else {
        String.format(Locale.US, "%d:%02d", minutes, seconds)
    }
}
