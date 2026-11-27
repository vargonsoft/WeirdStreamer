package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material.icons.filled.LiveTv
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
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
import androidx.lifecycle.viewmodel.compose.viewModel
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.MediaViewModel
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

@OptIn(androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
fun PhoneFileRowItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val viewModel: MediaViewModel = viewModel()
    var menuOpen by remember { mutableStateOf(false) }
    val playback = if (item.isDirectory || isParent) null else viewModel.playbackFor(item.path)
    val fixTarget = fixMatchTarget(viewModel, item, isParent)

    LaunchedEffect(item.path) {
        if (!item.isDirectory && !isParent && (item.duration == null || item.duration <= 0) && viewModel.isVideo(item)) {
            viewModel.fetchMetadata(item)
        }
    }

    val icon = when {
        HomeTiles.isGuide(item) -> Icons.Default.LiveTv // the home screen's Guide
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }

    ListItem(
        headlineContent = { Text(item.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
        supportingContent = if (!item.isDirectory && !isParent) {
            {
                val sizeStr = formatFileSize(item.size)
                val infoStr = if (item.duration != null && item.duration > 0) {
                    "$sizeStr, ${formatDuration(item.duration)}"
                } else {
                    sizeStr
                }
                val left = timeLeftText(playback, item.duration)
                Column {
                    // A video's format, above its size and length
                    item.format?.takeIf { it.isNotEmpty() }?.let { Text(it, maxLines = 1, overflow = TextOverflow.Ellipsis) }
                    Text(if (left != null) "$infoStr · $left" else infoStr)
                }
            }
        } else null,
        // Videos on a WeirdStreamer server can be downloaded for offline viewing.
        trailingContent = if (!item.isDirectory && !isParent && viewModel.connectedServer != null && viewModel.isVideo(item)) {
            { DownloadRequest.of(item)?.let { DownloadButton(viewModel, it) } }
        } else null,
        leadingContent = {
            Box(modifier = Modifier.size(40.dp), contentAlignment = Alignment.Center) {
                var isImageLoaded by remember { mutableStateOf(false) }
                val folderThumbnail = if (item.isDirectory && !isParent) item.thumbnailUrl else null
                val thumbnailUrl = if (item.isDirectory) null else item.thumbnailUrl ?: if (!isParent) item.path else null

                if (item.isDirectory && !isParent && !HomeTiles.isGuide(item)) {
                    FolderPreview(folderThumbnail, getPhoneIconTint(Icons.Default.Folder), Modifier.fillMaxSize(), requestSize = 80, kind = item.libraryType)
                } else if (!isImageLoaded) {
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
                            .size(48)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        onState = { state ->
                            isImageLoaded = state is coil.compose.AsyncImagePainter.State.Success
                        },
                        modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(4.dp))
                    )
                }
                // Videos have filmstrip edges, telling them from photos in the same folder.
                if (!item.isDirectory && !isParent && viewModel.isVideo(item)) {
                    Box(Modifier.matchParentSize().clip(RoundedCornerShape(4.dp)).filmstrip())
                }
                PlaybackOverlay(playback, item.duration, barHeight = 3.dp, badgeSize = 14.dp)
                // Long press: Play from start, and Mark watched (server videos); Fix match (admins).
                if (playback != null || (item.originalPath != null && viewModel.isVideo(item)) || fixTarget != null) {
                    DropdownMenu(border = phoneMenuBorder(), expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        if ((playback?.resumeAt ?: 0.0) > 0) {
                            DropdownMenuItem(text = { Text("Play from start") }, onClick = {
                                menuOpen = false
                                viewModel.playVideo(item.path, fromStart = true)
                            })
                        }
                        item.originalPath?.let { original ->
                            val watched = playback?.watched == true
                            DropdownMenuItem(text = { Text(if (watched) "Mark unwatched" else "Mark watched") }, onClick = {
                                menuOpen = false
                                viewModel.setWatched(listOf(original), !watched)
                            })
                        }
                        fixTarget?.let { (path, isDir) ->
                            DropdownMenuItem(text = { Text("Fix match") }, onClick = {
                                menuOpen = false
                                viewModel.openMatch(path, isDir, item.name)
                            })
                        }
                    }
                }
            }
        },
        modifier = modifier.combinedClickable(onClick = onClick, onLongClick = {
            if ((!item.isDirectory && !isParent && item.originalPath != null && viewModel.isVideo(item)) || fixTarget != null) menuOpen = true
        })
    )
}

@OptIn(androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
fun PhoneFileTileItem(item: FileItem, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val isParent = item.name == ".." || item.path == "UP"
    val viewModel: MediaViewModel = viewModel()
    val playback = if (item.isDirectory || isParent) null else viewModel.playbackFor(item.path)

    LaunchedEffect(item.path) {
        if (!item.isDirectory && !isParent && (item.duration == null || item.duration <= 0) && viewModel.isVideo(item)) {
            viewModel.fetchMetadata(item)
        }
    }

    val icon = when {
        isParent -> Icons.Default.SubdirectoryArrowLeft
        item.isDirectory -> Icons.Default.Folder
        else -> Icons.Default.Description
    }

    // Long press: Fix match (admins)
    val fixTarget = fixMatchTarget(viewModel, item, isParent)
    var menuOpen by remember { mutableStateOf(false) }
    Card(
        modifier = modifier
            .fillMaxWidth()
            .aspectRatio(1f)
            .combinedClickable(onClick = onClick, onLongClick = { if (fixTarget != null) menuOpen = true }),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.3f))
    ) {
        Column(
            modifier = Modifier.fillMaxSize().padding(8.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center
        ) {
            Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                var isImageLoaded by remember { mutableStateOf(false) }
                val folderThumbnail = if (item.isDirectory && !isParent) item.thumbnailUrl else null
                val thumbnailUrl = if (item.isDirectory) null else item.thumbnailUrl ?: if (!isParent) item.path else null

                if (item.isDirectory && !isParent) {
                    FolderPreview(folderThumbnail, getPhoneIconTint(Icons.Default.Folder), Modifier.fillMaxHeight(0.9f), requestSize = 200, kind = item.libraryType)
                } else if (!isImageLoaded) {
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
                            .size(80)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Crop,
                        onState = { state ->
                            isImageLoaded = state is coil.compose.AsyncImagePainter.State.Success
                        },
                        modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(8.dp))
                    )
                }
                if (!item.isDirectory && !isParent && viewModel.isVideo(item)) {
                    Box(Modifier.matchParentSize().clip(RoundedCornerShape(8.dp)).filmstrip())
                }
                PlaybackOverlay(playback, item.duration, barHeight = 3.dp, badgeSize = 18.dp)
                fixTarget?.let { (path, isDir) ->
                    DropdownMenu(border = phoneMenuBorder(), expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        DropdownMenuItem(text = { Text("Fix match") }, onClick = {
                            menuOpen = false
                            viewModel.openMatch(path, isDir, item.name)
                        })
                    }
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
            if (!item.isDirectory && !isParent) {
                val sizeStr = formatFileSize(item.size)
                val infoStr = if (item.duration != null && item.duration > 0) {
                    "$sizeStr, ${formatDuration(item.duration)}"
                } else {
                    sizeStr
                }
                Text(
                    text = infoStr,
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    textAlign = TextAlign.Center
                )
            }
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

/** What Fix match opens for a folder-view item (its client path, and whether it's a folder), or null. */
internal fun fixMatchTarget(viewModel: MediaViewModel, item: FileItem, isParent: Boolean): Pair<String, Boolean>? = when {
    isParent || !viewModel.canFixMatches -> null
    item.isDirectory -> if (item.fixable) item.path to true else null
    item.originalPath != null && viewModel.isVideo(item) -> item.originalPath to false
    else -> null
}

/** The home screen's Guide: a tile like a library's folder, after the TV Shows and Movies libraries. */
@Composable
fun PhoneGuideTile(modifier: Modifier = Modifier, onClick: () -> Unit) {
    Card(
        modifier = modifier.fillMaxWidth().aspectRatio(1f).combinedClickable(onClick = onClick),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.3f))
    ) {
        Column(Modifier.fillMaxSize().padding(8.dp), horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center) {
            Box(Modifier.weight(1f), contentAlignment = Alignment.Center) {
                Icon(Icons.Default.LiveTv, contentDescription = null, tint = getPhoneIconTint(Icons.Default.Folder),
                    modifier = Modifier.fillMaxHeight(0.8f).aspectRatio(1f))
            }
            Spacer(modifier = Modifier.height(4.dp))
            Text("Guide", style = MaterialTheme.typography.labelMedium, maxLines = 1)
        }
    }
}
