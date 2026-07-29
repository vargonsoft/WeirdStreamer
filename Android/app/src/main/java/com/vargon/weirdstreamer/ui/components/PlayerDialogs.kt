package com.vargon.weirdstreamer.ui.components

import android.util.Log
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.tv.material3.*
import com.vargon.weirdstreamer.OpenSubtitlesClient
import com.vargon.weirdstreamer.SubtitleInfo
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import java.io.File
import java.util.*

@Composable
fun SpeedDialog(
    show: Boolean,
    currentSpeedIndex: Int,
    speedIntervals: List<Float>,
    onDismiss: () -> Unit,
    onSpeedSelected: (Int, Float) -> Unit
) {
    if (!show) return
    var pendingSpeedIndex by remember { mutableIntStateOf(currentSpeedIndex) }
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(300.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionUp -> {
                                if (pendingSpeedIndex > 0) pendingSpeedIndex--
                                true
                            }
                            Key.DirectionDown -> {
                                if (pendingSpeedIndex < speedIntervals.size - 1) pendingSpeedIndex++
                                true
                            }
                            Key.DirectionCenter, Key.Enter -> {
                                onSpeedSelected(pendingSpeedIndex, speedIntervals[pendingSpeedIndex])
                                true
                            }
                            Key.Escape, Key.Back, Key.Backspace -> {
                                onDismiss()
                                true
                            }
                            else -> false
                        }
                    } else false
                },
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(modifier = Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text("Playback Speed", style = MaterialTheme.typography.headlineSmall, color = Color.White)
                Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    speedIntervals.forEachIndexed { index, speed ->
                        Row(
                            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp))
                                .background(if (pendingSpeedIndex == index) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(alpha = 0.2f))
                                .clickable { pendingSpeedIndex = index }
                                .height(48.dp).padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            if (pendingSpeedIndex == index) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, tint = Color.White, modifier = Modifier.size(20.dp))
                                Spacer(modifier = Modifier.width(8.dp))
                            }
                            Text("${speed}x", color = Color.White, style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun TranscodeDialog(
    show: Boolean,
    selectedModeIndex: Int,
    transcodeModes: List<TranscodeMode>,
    onDismiss: () -> Unit,
    onModeSelected: (Int) -> Unit
) {
    if (!show) return
    var pendingModeIndex by remember { mutableIntStateOf(selectedModeIndex) }
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(400.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionUp -> {
                                if (pendingModeIndex > 0) pendingModeIndex--
                                true
                            }
                            Key.DirectionDown -> {
                                if (pendingModeIndex < transcodeModes.size - 1) pendingModeIndex++
                                true
                            }
                            Key.DirectionCenter, Key.Enter -> {
                                onModeSelected(pendingModeIndex)
                                true
                            }
                            Key.Escape, Key.Back, Key.Backspace -> {
                                onDismiss()
                                true
                            }
                            else -> false
                        }
                    } else false
                },
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(modifier = Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text("Transcoding Options", style = MaterialTheme.typography.headlineSmall, color = Color.White)
                Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    transcodeModes.forEachIndexed { index, mode ->
                        Row(
                            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp))
                                .background(if (pendingModeIndex == index) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(alpha = 0.2f))
                                .clickable { pendingModeIndex = index }
                                .height(48.dp).padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            if (pendingModeIndex == index) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, tint = Color.White, modifier = Modifier.size(20.dp))
                                Spacer(modifier = Modifier.width(8.dp))
                            }
                            Text(mode.name, color = Color.White, style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun SubtitleDialog(
    show: Boolean,
    availableSubtitles: List<String>,
    selectedSubtitleIndex: Int,
    onDismiss: () -> Unit,
    onSubtitleSelected: (Int) -> Unit,
    onDownloadRequested: () -> Unit
) {
    if (!show) return
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(500.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                Text(
                    text = "Subtitles",
                    style = MaterialTheme.typography.headlineSmall,
                    color = Color.White
                )

                if (availableSubtitles.isNotEmpty()) {
                    Text(
                        text = "Available Subtitles",
                        style = MaterialTheme.typography.bodyLarge,
                        color = Color.White.copy(alpha = 0.8f)
                    )

                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(
                                    if (selectedSubtitleIndex == -1) Color.White.copy(alpha = 0.2f)
                                    else Color.Gray.copy(alpha = 0.2f)
                                )
                                .clickable {
                                    onSubtitleSelected(-1)
                                }
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            if (selectedSubtitleIndex == -1) {
                                Icon(
                                    Icons.Default.PlayArrow,
                                    contentDescription = null,
                                    tint = Color.White,
                                    modifier = Modifier.size(20.dp)
                                )
                                Spacer(modifier = Modifier.width(8.dp))
                            }
                            Text(
                                text = "None",
                                color = Color.White,
                                style = MaterialTheme.typography.bodyMedium
                            )
                        }

                        availableSubtitles.forEachIndexed { index, subtitle ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(
                                        if (selectedSubtitleIndex == index) Color.White.copy(
                                            alpha = 0.2f
                                        )
                                        else Color.Gray.copy(alpha = 0.2f)
                                    )
                                    .clickable { onSubtitleSelected(index) }
                                    .padding(12.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                if (selectedSubtitleIndex == index) {
                                    Icon(
                                        Icons.Default.PlayArrow,
                                        contentDescription = null,
                                        tint = Color.White,
                                        modifier = Modifier.size(20.dp)
                                    )
                                    Spacer(modifier = Modifier.width(8.dp))
                                }
                                Text(
                                    text = subtitle,
                                    color = Color.White,
                                    style = MaterialTheme.typography.bodyMedium
                                )
                            }
                        }
                    }

                    HorizontalDivider(color = Color.White.copy(alpha = 0.3f), thickness = 1.dp)
                }

                Text(
                    text = "Download Subtitles",
                    style = MaterialTheme.typography.bodyLarge,
                    color = Color.White.copy(alpha = 0.8f)
                )

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(8.dp))
                        .background(Color.Blue.copy(alpha = 0.3f))
                        .clickable {
                            onDownloadRequested()
                        }
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    Icon(
                        Icons.Default.PlayArrow,
                        contentDescription = null,
                        tint = Color.White,
                        modifier = Modifier.size(20.dp)
                        )
                    Text(
                        text = "OpenSubtitles.org / .com",
                        color = Color.White,
                        style = MaterialTheme.typography.bodyMedium
                    )
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End
                ) {
                    Button(
                        onClick = onDismiss
                    ) {
                        Text("Close")
                    }
                }
            }
        }
    }
}

@Composable
fun DownloadSubtitlesDialog(
    show: Boolean,
    isSearching: Boolean,
    downloadStatus: String?,
    downloadingSubtitles: List<SubtitleInfo>,
    onDismiss: () -> Unit,
    onSubtitleSelected: (SubtitleInfo) -> Unit
) {
    if (!show) return
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(dismissOnBackPress = true, dismissOnClickOutside = false)
    ) {
        Surface(
            modifier = Modifier
                .width(600.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                Text(
                    text = "Download Subtitles",
                    style = MaterialTheme.typography.headlineSmall,
                    color = Color.White
                )

                if (isSearching) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(16.dp),
                        horizontalArrangement = Arrangement.Center
                    ) {
                        CircularProgressIndicator(
                            color = Color.Blue,
                            modifier = Modifier.size(40.dp)
                        )
                        Spacer(modifier = Modifier.width(16.dp))
                        Text(
                            text = "Searching...",
                            color = Color.White,
                            style = MaterialTheme.typography.bodyMedium
                        )
                    }
                } else if (!downloadStatus.isNullOrEmpty()) {
                    Text(
                        text = downloadStatus,
                        color = if (downloadStatus.startsWith("Error")) Color.Red else Color.Yellow,
                        style = MaterialTheme.typography.bodyMedium
                    )
                } else if (downloadingSubtitles.isNotEmpty()) {
                    Text(
                        text = "Found ${downloadingSubtitles.size} subtitle(s)",
                        style = MaterialTheme.typography.bodyLarge,
                        color = Color.White.copy(alpha = 0.8f)
                    )

                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .heightIn(max = 300.dp)
                            .verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        downloadingSubtitles.forEach { subtitle ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(Color.Gray.copy(alpha = 0.2f))
                                    .clickable { onSubtitleSelected(subtitle) }
                                    .padding(12.dp),
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        text = subtitle.filename,
                                        color = Color.White,
                                        style = MaterialTheme.typography.bodySmall,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = "${subtitle.language} | Rating: ${
                                            String.format(
                                                Locale.US,
                                                "%.1f",
                                                subtitle.rating.toDouble()
                                            )
                                        } | Downloads: ${subtitle.downloads}",
                                        color = Color.White.copy(alpha = 0.7f),
                                        style = MaterialTheme.typography.labelSmall
                                    )
                                }
                                Icon(
                                    Icons.Default.PlayArrow,
                                    contentDescription = null,
                                    tint = Color.White,
                                    modifier = Modifier.size(20.dp)
                                )
                            }
                        }
                    }
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End
                ) {
                    Button(
                        onClick = onDismiss
                    ) {
                        Text("Close")
                    }
                }
            }
        }
    }
}
