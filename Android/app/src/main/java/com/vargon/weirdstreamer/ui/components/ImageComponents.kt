package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*

@Composable
fun ImageOverlays(
    showFilename: Boolean,
    filename: String,
    isZoomMode: Boolean,
    loopFeedback: Pair<Boolean, Long>?,
    showPauseIcon: Boolean,
    isLivePhoto: Boolean,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        // Filename Overlay
        AnimatedVisibility(
            visible = showFilename && !isZoomMode,
            enter = fadeIn() + slideInVertically(),
            exit = fadeOut() + slideOutVertically(),
            modifier = Modifier.align(Alignment.TopCenter)
        ) {
            Text(
                text = filename,
                modifier = Modifier
                    .padding(top = 32.dp)
                    .background(Color.Black.copy(alpha = 0.5f), RoundedCornerShape(16.dp))
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                style = MaterialTheme.typography.titleMedium,
                color = Color.White,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis
            )
        }

        // Loop Feedback
        AnimatedVisibility(
            visible = loopFeedback != null,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier
                .align(Alignment.TopEnd)
                .padding(32.dp)
        ) {
            val isForward = loopFeedback?.first == true
            Icon(
                Icons.Default.Refresh,
                contentDescription = null,
                modifier = Modifier
                    .size(48.dp)
                    .graphicsLayer(rotationY = if (isForward) 0f else 180f),
                tint = Color.White.copy(alpha = 0.5f)
            )
        }

        if (showPauseIcon && isLivePhoto) {
            Icon(
                imageVector = Icons.Default.Pause,
                contentDescription = "Paused",
                tint = Color.White.copy(alpha = 0.5f),
                modifier = Modifier
                    .size(64.dp)
                    .align(Alignment.Center)
            )
        }
    }
}
