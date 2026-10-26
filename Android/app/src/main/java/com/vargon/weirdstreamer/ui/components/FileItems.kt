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
fun FileTileItem(item: FileItem, thumbnailUrl: String?, modifier: Modifier = Modifier,
                 playback: com.vargon.weirdstreamer.api.models.Playback? = null, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val subtitle = remember(item.isDirectory, isParent, item.size, item.duration) {
        if (!item.isDirectory && !isParent) fileInfoText(item.size, item.duration) else null
    }

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
        thumbnailUrl = thumbnailUrl,
        rotateIcon = if (isParent) 90f else 0f,
        progress = playback?.fraction(item.duration) ?: 0f,
        watched = playback?.watched == true
    )
}

@Composable
fun TileItem(
    title: String,
    icon: ImageVector,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    thumbnailUrl: String? = null,
    rotateIcon: Float = 0f,
    progress: Float = 0f,
    watched: Boolean = false,
    onClick: () -> Unit
) {
    Column(
        modifier = modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(16f / 9f)
                .tvClickable(
                    shape = MaterialTheme.shapes.medium,
                    focusedScale = 1.05f,
                    borderWidth = 4.dp,
                    onClick = onClick
                )
        ) {
            // The icon sits under the thumbnail, which covers it once loaded; no load-state
            // bookkeeping (and no extra recomposition) needed.
            val isLargeIcon = icon == Icons.Default.Folder || icon == Icons.Default.Dns || icon == Icons.Default.SubdirectoryArrowLeft
            Icon(
                icon,
                contentDescription = null,
                modifier = Modifier
                    .fillMaxSize(if (isLargeIcon) 0.8f else 0.5f)
                    .padding(if (isLargeIcon) 4.dp else 10.dp)
                    .graphicsLayer(rotationZ = rotateIcon),
                tint = getIconTint(icon)
            )

            if (thumbnailUrl != null) {
                ThumbnailImage(
                    url = thumbnailUrl,
                    modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.small)
                )
            }
            WatchProgressOverlay(progress, watched, barHeight = 5.dp, badgeSize = 28.dp)
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

/** Thumbnail sized by its layout constraints, with the request built once per URL. */
@Composable
private fun ThumbnailImage(url: String, modifier: Modifier) {
    val context = LocalContext.current
    val request = remember(url) {
        ImageRequest.Builder(context)
            .data(url)
            .crossfade(false)
            .build()
    }
    AsyncImage(
        model = request,
        contentDescription = null,
        contentScale = ContentScale.Crop,
        modifier = modifier
    )
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

@Composable
fun FileRowItem(item: FileItem, thumbnailUrl: String?, modifier: Modifier = Modifier,
                playback: com.vargon.weirdstreamer.api.models.Playback? = null, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val infoText = remember(item.isDirectory, isParent, item.size, item.duration) {
        if (!item.isDirectory && !isParent) fileInfoText(item.size, item.duration) else null
    }
    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp)
            .tvClickable(shape = MaterialTheme.shapes.small, focusedScale = 1.02f, borderWidth = 2.dp, onClick = onClick)
            .padding(12.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Box(modifier = Modifier.size(48.dp), contentAlignment = Alignment.Center) {
            Icon(
                icon,
                contentDescription = null,
                modifier = Modifier
                    .size(32.dp)
                    .graphicsLayer(rotationZ = if (isParent) 90f else 0f),
                tint = getIconTint(icon)
            )
            if (thumbnailUrl != null) {
                ThumbnailImage(
                    url = thumbnailUrl,
                    modifier = Modifier.fillMaxSize().clip(MaterialTheme.shapes.extraSmall)
                )
            }
            PlaybackOverlay(playback, item.duration, barHeight = 3.dp, badgeSize = 16.dp)
        }
        Spacer(modifier = Modifier.width(16.dp))
        Column {
            Text(text = item.name, style = MaterialTheme.typography.bodyLarge)
            if (infoText != null) {
                val left = timeLeftText(playback, item.duration)
                Text(text = if (left != null) "$infoText · $left" else infoText, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

// Overload for host row with delete action
@Composable
fun HostRowItem(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp)
            .tvClickable(shape = MaterialTheme.shapes.small, focusedScale = 1.02f, borderWidth = 2.dp, onClick = onClick)
            .padding(12.dp),
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

private fun fileInfoText(size: Long, duration: Double?): String {
    val sizeStr = formatFileSize(size)
    return if (duration != null && duration > 0) "$sizeStr, ${formatDuration(duration)}" else sizeStr
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
