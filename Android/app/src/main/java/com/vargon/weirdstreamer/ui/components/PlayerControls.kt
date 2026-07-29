package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.RotateRight
import androidx.compose.material.icons.filled.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.BiasAlignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*

@Composable
fun PlayerControls(
    show: Boolean,
    isPlaying: Boolean,
    playbackPosition: Long,
    duration: Long,
    isSeekingSlider: Boolean,
    isTrickplayVisible: Boolean,
    modifier: Modifier = Modifier
) {
    AnimatedVisibility(
        visible = show,
        enter = slideInVertically { it } + fadeIn(),
        exit = slideOutVertically { it } + fadeOut(),
        modifier = modifier
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(Color.Black.copy(alpha = 0.6f))
                .padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            PlayerControlsProgress(
                playbackPosition = playbackPosition,
                duration = duration,
                isSeekingSlider = isSeekingSlider
            )

            Spacer(modifier = Modifier.height(16.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.Center
            ) {
                Icon(
                    Icons.Default.FastRewind,
                    contentDescription = null,
                    tint = if (!isSeekingSlider && !isTrickplayVisible) Color.White else Color.Gray,
                    modifier = Modifier.size(48.dp)
                )
                Spacer(modifier = Modifier.width(32.dp))
                Icon(
                    if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                    contentDescription = null,
                    tint = if (!isSeekingSlider && !isTrickplayVisible) Color.White else Color.Gray,
                    modifier = Modifier.size(64.dp)
                )
                Spacer(modifier = Modifier.width(32.dp))
                Icon(
                    Icons.Default.FastForward,
                    contentDescription = null,
                    tint = if (!isSeekingSlider && !isTrickplayVisible) Color.White else Color.Gray,
                    modifier = Modifier.size(48.dp)
                )
            }
        }
    }
}

@Composable
fun PlayerControlsProgress(
    playbackPosition: Long,
    duration: Long,
    isSeekingSlider: Boolean,
    modifier: Modifier = Modifier
) {
    Row(
        modifier = modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(
            text = formatTime(playbackPosition),
            style = MaterialTheme.typography.bodySmall,
            color = Color.White
        )

        Spacer(modifier = Modifier.width(16.dp))

        Box(
            modifier = Modifier
                .weight(1f)
                .height(24.dp),
            contentAlignment = Alignment.CenterStart
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(4.dp)
                    .background(Color.Gray.copy(alpha = 0.5f))
            )

            val progress =
                if (duration > 0) playbackPosition.toFloat() / duration else 0f

            Box(
                modifier = Modifier
                    .fillMaxWidth(progress)
                    .height(4.dp)
                    .background(Color.Red)
            )

            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .align(Alignment.CenterStart)
            ) {
                Box(
                    modifier = Modifier
                        .align(BiasAlignment(2 * progress - 1, 0f))
                        .size(if (isSeekingSlider) 16.dp else 12.dp)
                        .clip(CircleShape)
                        .background(if (isSeekingSlider) Color.White else Color.Red)
                )
            }
        }

        Spacer(modifier = Modifier.width(16.dp))

        Text(
            text = if (duration > 0) formatTime(duration) else "0:00:00",
            style = MaterialTheme.typography.bodySmall,
            color = Color.White
        )
    }
}

@Composable
fun PlayerZoomToolbar(
    visible: Boolean,
    isZoomMode: Boolean,
    toolbarFocusedButton: Int, // Still needed for now, but will migrate to focus
    currentTranscodeModeName: String,
    currentAspectRatioName: String,
    isSmbSource: Boolean,
    onButtonFocused: (Int) -> Unit,
    onButtonClicked: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    AnimatedVisibility(
        visible = !isZoomMode && visible,
        enter = slideInVertically { it } + fadeIn(),
        exit = slideOutVertically { it } + fadeOut(),
        modifier = modifier
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(Color.Black.copy(alpha = 0.6f))
                .padding(16.dp),
            horizontalArrangement = Arrangement.Center,
            verticalAlignment = Alignment.CenterVertically
        ) {
            val buttons = mutableListOf<Int>()
            if (!isSmbSource) buttons.add(0) // Transcode
            buttons.addAll(listOf(1, 2, 3, 4, 5, 6, 7))

            buttons.forEach { id ->
                val isFocused = toolbarFocusedButton == id
                ToolbarButton(
                    id = id,
                    isFocused = isFocused,
                    currentTranscodeModeName = currentTranscodeModeName,
                    currentAspectRatioName = currentAspectRatioName,
                    onFocused = { onButtonFocused(id) },
                    onClick = { onButtonClicked(id) }
                )
                if (id != buttons.last()) Spacer(modifier = Modifier.width(12.dp))
            }
        }
    }
}

@Composable
fun ToolbarButton(
    id: Int,
    isFocused: Boolean,
    currentTranscodeModeName: String,
    currentAspectRatioName: String,
    onFocused: () -> Unit,
    onClick: () -> Unit
) {
    Column(
        modifier = Modifier
            .width(100.dp)
            .clip(RoundedCornerShape(8.dp))
            .background(
                if (isFocused) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(alpha = 0.3f)
            )
            .onFocusChanged { if (it.isFocused) onFocused() }
            .clickable { onClick() }
            .padding(8.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center
    ) {
        val icon = when (id) {
            0 -> Icons.Default.VideoSettings
            1 -> Icons.Default.Api
            2 -> Icons.Default.Speed
            3 -> Icons.Default.ZoomIn
            4 -> Icons.Default.ZoomOut
            5 -> Icons.Default.AspectRatio
            6 -> Icons.Default.Subtitles
            7 -> Icons.AutoMirrored.Filled.RotateRight
            else -> Icons.Default.QuestionMark
        }

        val text = when (id) {
            0 -> currentTranscodeModeName
            1 -> "Zoom"
            2 -> "Speed"
            3 -> "Fill+"
            4 -> "Fill-"
            5 -> currentAspectRatioName
            6 -> "Subtitles"
            7 -> "Rotate"
            else -> ""
        }

        val tint = if (id == 0 && currentTranscodeModeName != "Original") Color.Green
                   else (if (isFocused) Color.White else Color.White.copy(alpha = 0.7f))

        Icon(
            icon,
            contentDescription = text,
            tint = tint,
            modifier = Modifier.size(if (isFocused) 48.dp else 40.dp)
        )
        Spacer(modifier = Modifier.height(4.dp))
        Text(
            text = text,
            style = MaterialTheme.typography.labelSmall,
            color = tint,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}
