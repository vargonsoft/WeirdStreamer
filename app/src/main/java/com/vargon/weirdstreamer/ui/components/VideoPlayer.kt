package com.vargon.weirdstreamer.ui.components

import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.activity.compose.BackHandler
import androidx.annotation.OptIn
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.FastForward
import androidx.compose.material.icons.filled.FastRewind
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.BiasAlignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.SmbViewModel
import kotlinx.coroutines.delay
import java.util.Locale

@OptIn(UnstableApi::class)
@Composable
fun VideoPlayer(viewModel: SmbViewModel, path: String, onBack: () -> Unit) {
    val context = LocalContext.current
    val exoPlayer = remember {
        ExoPlayer.Builder(context)
            .setMediaSourceFactory(
                DefaultMediaSourceFactory(context)
                    .setDataSourceFactory(SmbDataSourceFactory(viewModel.credentialManager, viewModel.baseContext))
            )
            .build()
            .apply {
                val mediaItem = MediaItem.fromUri(path)
                setMediaItem(mediaItem)
                prepare()
                playWhenReady = true
            }
    }

    var isPlaying by remember { mutableStateOf(true) }
    var isBuffering by remember { mutableStateOf(true) }
    var showControls by remember { mutableStateOf(false) }
    var lastInteractionTime by remember { mutableLongStateOf(System.currentTimeMillis()) }
    var lastControlsHideTime by remember { mutableLongStateOf(0L) }
    var isSeekingSlider by remember { mutableStateOf(false) }
    var playbackPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var skipFeedback by remember { mutableStateOf<Pair<Int, Boolean>?>(null) } // value to show, isForward
    
    val focusRequester = remember { FocusRequester() }

    // Sync playback state
    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlayingChanged: Boolean) {
                isPlaying = isPlayingChanged
            }
            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_READY) {
                    duration = exoPlayer.duration
                }
                isBuffering = state == Player.STATE_BUFFERING || state == Player.STATE_IDLE
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    // Progress updates
    LaunchedEffect(exoPlayer) {
        while (true) {
            playbackPosition = exoPlayer.currentPosition
            delay(500)
        }
    }

    // Auto-hide controls
    LaunchedEffect(showControls, lastInteractionTime) {
        if (showControls) {
            val timeToWait = 6000L - (System.currentTimeMillis() - lastInteractionTime)
            if (timeToWait > 0) {
                delay(timeToWait)
            }
            showControls = false
            lastControlsHideTime = System.currentTimeMillis()
            isSeekingSlider = false
        }
    }

    // Hide skip feedback
    LaunchedEffect(skipFeedback) {
        if (skipFeedback != null) {
            delay(2000)
            skipFeedback = null
        }
    }

    val updateInteraction = {
        lastInteractionTime = System.currentTimeMillis()
        if (!showControls) showControls = true
    }

    val performSkip = { amount: Int ->
        val currentDuration = exoPlayer.duration
        val maxPos = if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration
        val newPos = (exoPlayer.currentPosition + amount * 1000).coerceIn(0L, maxPos)
        exoPlayer.seekTo(newPos)
        skipFeedback = amount to (amount > 0)
        updateInteraction()
    }

    val handleBackAction = {
        val now = System.currentTimeMillis()
        if (showControls) {
            showControls = false
            lastControlsHideTime = now
            isSeekingSlider = false
        } else {
            // If back is pressed within 1 second of controls disappearing, ignore it.
            if (now - lastControlsHideTime > 1000) {
                onBack()
            }
        }
    }

    BackHandler {
        handleBackAction()
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown) {
                    when (event.key) {
                        Key.DirectionUp -> {
                            if (showControls && !isSeekingSlider) {
                                isSeekingSlider = true
                            } else {
                                updateInteraction()
                            }
                            true
                        }
                        Key.DirectionDown -> {
                            if (isSeekingSlider) {
                                isSeekingSlider = false
                            } else if (showControls) {
                                showControls = false
                                lastControlsHideTime = System.currentTimeMillis()
                            }
                            true
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            updateInteraction()
                            if (isPlaying) exoPlayer.pause() else exoPlayer.play()
                            true
                        }
                        Key.DirectionLeft -> {
                            if (isSeekingSlider) {
                                performSkip(-30)
                            } else {
                                performSkip(-5)
                            }
                            true
                        }
                        Key.DirectionRight -> {
                            if (isSeekingSlider) {
                                performSkip(30)
                            } else {
                                performSkip(5)
                            }
                            true
                        }
                        Key.Backspace -> {
                            handleBackAction()
                            true
                        }
                        else -> false
                    }
                } else false
            }
            .focusRequester(focusRequester)
            .focusable()
    ) {
        AndroidView(
            factory = { ctx ->
                PlayerView(ctx).apply {
                    player = exoPlayer
                    useController = false
                    resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                    layoutParams = FrameLayout.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT
                    )
                }
            },
            modifier = Modifier.fillMaxSize()
        )

        // Loading Spinner
        if (isBuffering) {
            CircularProgressIndicator(
                modifier = Modifier.align(Alignment.Center),
                color = Color.Red
            )
        }

        // Skip Feedback Overlay
        Box(modifier = Modifier.fillMaxSize()) {
            AnimatedVisibility(
                visible = skipFeedback != null,
                enter = fadeIn() + scaleIn(),
                exit = fadeOut() + scaleOut(),
                modifier = Modifier.align(if (skipFeedback?.second == true) Alignment.CenterEnd else Alignment.CenterStart)
            ) {
                val value = skipFeedback?.first ?: 0
                val text = if (value > 0) "+$value" else "$value"
                androidx.tv.material3.Text(
                    text = text,
                    style = androidx.tv.material3.MaterialTheme.typography.displayLarge,
                    color = Color.White.copy(alpha = 0.7f),
                    modifier = Modifier.padding(horizontal = 64.dp)
                )
            }
        }

        // Controls Overlay
        AnimatedVisibility(
            visible = showControls,
            enter = slideInVertically { it } + fadeIn(),
            exit = slideOutVertically { it } + fadeOut(),
            modifier = Modifier.align(Alignment.BottomCenter)
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(Color.Black.copy(alpha = 0.6f))
                    .padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                // Progress Slider and Timestamps
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    androidx.tv.material3.Text(
                        text = formatTime(playbackPosition),
                        style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                        color = Color.White
                    )
                    
                    Spacer(modifier = Modifier.width(16.dp))
                    
                    Box(
                        modifier = Modifier
                            .weight(1f)
                            .height(24.dp),
                        contentAlignment = Alignment.CenterStart
                    ) {
                        // Background track
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(4.dp)
                                .background(Color.Gray.copy(alpha = 0.5f))
                        )
                        
                        val progress = if (duration > 0) playbackPosition.toFloat() / duration else 0f
                        
                        // Progress bar
                        Box(
                            modifier = Modifier
                                .fillMaxWidth(progress)
                                .height(4.dp)
                                .background(Color.Red)
                        )

                        // Thumb
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
                    
                    androidx.tv.material3.Text(
                        text = if (duration > 0) formatTime(duration) else "0:00:00",
                        style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                        color = Color.White
                    )
                }
                
                Spacer(modifier = Modifier.height(16.dp))

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    androidx.tv.material3.Icon(
                        Icons.Default.FastRewind,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider) Color.White else Color.Gray,
                        modifier = Modifier.size(48.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    androidx.tv.material3.Icon(
                        if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider) Color.White else Color.Gray,
                        modifier = Modifier.size(64.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    androidx.tv.material3.Icon(
                        Icons.Default.FastForward,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider) Color.White else Color.Gray,
                        modifier = Modifier.size(48.dp)
                    )
                }
            }
        }
    }

    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }
}

private fun formatTime(ms: Long): String {
    val totalSeconds = ms / 1000
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    return String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
}
