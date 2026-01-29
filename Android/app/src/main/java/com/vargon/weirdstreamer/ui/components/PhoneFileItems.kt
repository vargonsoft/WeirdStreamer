package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.SubdirectoryArrowLeft
import androidx.compose.material3.*
import androidx.compose.runtime.*
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
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.repository.FileItem
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import java.util.Locale

@Composable
private fun getPhoneIconTint(icon: ImageVector): Color {
    val spec = WeirdStreamerTheme.spec
    val defaultColor = LocalContentColor.current
    return when (icon) {
        Icons.Default.Folder -> if (spec.folderIconTint != Color.Unspecified) spec.folderIconTint else defaultColor
        Icons.Default.Dns -> if (spec.serverIconTint != Color.Unspecified) spec.serverIconTint else defaultColor
        Icons.Default.SubdirectoryArrowLeft -> if (spec.parentIconTint != Color.Unspecified) spec.parentIconTint else defaultColor
        else -> defaultColor
    }
}

@Composable
fun PhoneHostRowItem(host: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(host) },
        leadingContent = {
            Icon(
                Icons.Default.Dns,
                contentDescription = null,
                tint = getPhoneIconTint(Icons.Default.Dns)
            )
        },
        modifier = modifier.clickable(onClick = onClick)
    )
}

@Composable
fun PhoneFileRowItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }

    ListItem(
        headlineContent = { Text(item.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = if (!item.isDirectory && !isParent) {
            { Text(formatFileSize(item.size)) }
        } else null,
        leadingContent = {
            Box(modifier = Modifier.size(40.dp), contentAlignment = Alignment.Center) {
                var isImageLoaded by remember { mutableStateOf(false) }
                val thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && !isParent) item.path else null

                if (!isImageLoaded) {
                    Icon(
                        icon,
                        contentDescription = null,
                        modifier = Modifier
                            .size(24.dp)
                            .graphicsLayer(rotationZ = if (isParent) 90f else 0f),
                        tint = getPhoneIconTint(icon)
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
                        onState = { state ->
                            isImageLoaded = state is coil.compose.AsyncImagePainter.State.Success
                        },
                        modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(4.dp))
                    )
                }
            }
        },
        modifier = modifier.clickable(onClick = onClick)
    )
}

@Composable
fun PhoneFileTileItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }

    Card(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(1f)
            .clickable(onClick = onClick),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.3f))
    ) {
        Column(
            modifier = Modifier.fillMaxSize().padding(8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center
        ) {
            Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                var isImageLoaded by remember { mutableStateOf(false) }
                val thumbnailUrl = item.thumbnailUrl ?: if (!item.isDirectory && !isParent) item.path else null

                if (!isImageLoaded) {
                    val isLargeIcon = item.isDirectory || isParent
                    Icon(
                        icon,
                        contentDescription = null,
                        modifier = Modifier
                            .size(if (isLargeIcon) 48.dp else 32.dp)
                            .graphicsLayer(rotationZ = if (isParent) 90f else 0f),
                        tint = getPhoneIconTint(icon)
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
                        onState = { state ->
                            isImageLoaded = state is coil.compose.AsyncImagePainter.State.Success
                        },
                        modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(8.dp))
                    )
                }
            }
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = item.name,
                style = MaterialTheme.typography.labelMedium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                textAlign = TextAlign.Center
            )
        }
    }
}

private fun formatFileSize(size: Long): String {
    if (size <= 0) return "0 B"
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val digitGroups = (Math.log10(size.toDouble()) / Math.log10(1024.0)).toInt()
    return String.format(Locale.US, "%.1f %s", size / Math.pow(1024.0, digitGroups.toDouble()), units[digitGroups])
}
