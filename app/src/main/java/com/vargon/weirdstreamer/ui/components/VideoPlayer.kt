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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.FastForward
import androidx.compose.material.icons.filled.FastRewind
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.VideoSettings
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import androidx.media3.datasource.okhttp.OkHttpDataSource
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.WeirdStreamerApplication
import kotlinx.coroutines.delay
import java.util.Locale

@OptIn(UnstableApi::class)
@Composable
fun VideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current

    // Determine if this is an SMB or Server source
    val isSmbSource = remember(path) { path.startsWith("smb://") }

    // Transcode state - only applicable for server sources
    var enableTranscode by remember { mutableStateOf(false) }

    // Build the actual video URL with transcode parameter if needed
    val videoUrl = remember(path, enableTranscode) {
        if (!isSmbSource && enableTranscode) {
            // Add transcode=1 parameter to the URL
            if (path.contains("?")) {
                "$path&transcode=1"
            } else {
                "$path?transcode=1"
            }
        } else {
            path
        }
    }

    val exoPlayer = remember(videoUrl) {
        val loadControl = DefaultLoadControl.Builder()
            .setBufferDurationsMs(
                50_000, // minBufferMs (increased from 32s to 50s)
                120_000, // maxBufferMs (increased from 50s to 120s)
                5_000, // bufferForPlaybackMs
                10_000 // bufferForPlaybackAfterRebufferMs
            )
            .build()

        val player = ExoPlayer.Builder(context)
            .setLoadControl(loadControl)
            .apply {
                if (isSmbSource) {
                    // Use SMB DataSource for SMB paths
                    setMediaSourceFactory(
                        DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(SmbDataSourceFactory(viewModel.credentialManager, viewModel.baseContext))
                    )
                } else {
                    // Use OkHttp DataSource for server URLs (handles SSL and auth)
                    val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                    val okHttpDataSourceFactory = OkHttpDataSource.Factory(okHttpClient)
                        .setUserAgent("WeirdStreamer/1.0")

                    setMediaSourceFactory(
                        DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(okHttpDataSourceFactory)
                    )
                }
            }
            .build()

        player.apply {
            val mediaItem = MediaItem.fromUri(videoUrl)
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
    var isFocusedOnTranscode by remember { mutableStateOf(false) }
    var playbackPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var skipFeedback by remember { mutableStateOf<Pair<Int, Boolean>?>(null) } // value to show, isForward
    
    val focusRequester = remember { FocusRequester() }
    
    val filename = remember(path) {
        viewModel.getDisplayName(path)
    }

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
            val timeToWait = 3000L - (System.currentTimeMillis() - lastInteractionTime)
            if (timeToWait > 0) {
                delay(timeToWait)
            }
            showControls = false
            lastControlsHideTime = System.currentTimeMillis()
            isSeekingSlider = false
            isFocusedOnTranscode = false
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
            isFocusedOnTranscode = false
        } else {
            // If back is pressed within 1 second of controls disappearing, ignore it.
            if (now - lastControlsHideTime > 1000) {
                onBack(path)
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
                            if (isFocusedOnTranscode) {
                                // Already on transcode button, do nothing or hide controls
                                false
                            } else if (isSeekingSlider && !isSmbSource) {
                                // Move from slider to transcode button
                                isFocusedOnTranscode = true
                                isSeekingSlider = false
                                updateInteraction()
                                true
                            } else if (showControls && !isSeekingSlider && !isSmbSource) {
                                // Move from playback controls to slider
                                isSeekingSlider = true
                                updateInteraction()
                                true
                            } else if (showControls && !isSeekingSlider && isSmbSource) {
                                // For SMB, just go to slider (no transcode button)
                                isSeekingSlider = true
                                updateInteraction()
                                true
                            } else {
                                updateInteraction()
                                false
                            }
                        }
                        Key.DirectionDown -> {
                            if (isFocusedOnTranscode) {
                                // Move from transcode button to slider
                                isFocusedOnTranscode = false
                                isSeekingSlider = true
                                updateInteraction()
                                true
                            } else if (isSeekingSlider) {
                                // Move from slider to playback controls
                                isSeekingSlider = false
                                updateInteraction()
                                true
                            } else if (showControls) {
                                showControls = false
                                lastControlsHideTime = System.currentTimeMillis()
                                isFocusedOnTranscode = false
                                true
                            } else {
                                false
                            }
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            if (isFocusedOnTranscode) {
                                // Toggle transcode
                                enableTranscode = !enableTranscode
                                updateInteraction()
                                true
                            } else {
                                updateInteraction()
                                if (isPlaying) exoPlayer.pause() else exoPlayer.play()
                                true
                            }
                        }
                        Key.DirectionLeft -> {
                            if (isFocusedOnTranscode) {
                                // Do nothing on transcode button
                                false
                            } else if (isSeekingSlider) {
                                performSkip(-30)
                                true
                            } else {
                                performSkip(-5)
                                true
                            }
                        }
                        Key.DirectionRight -> {
                            if (isFocusedOnTranscode) {
                                // Do nothing on transcode button
                                false
                            } else if (isSeekingSlider) {
                                performSkip(30)
                                true
                            } else {
                                performSkip(5)
                                true
                            }
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
            update = { view ->
                // Update the player reference when exoPlayer changes (e.g., when transcoding is toggled)
                view.player = exoPlayer
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

        // Filename Overlay
        AnimatedVisibility(
            visible = showControls,
            enter = slideInVertically { -it } + fadeIn(),
            exit = slideOutVertically { -it } + fadeOut(),
            modifier = Modifier.align(Alignment.TopStart)
        ) {
            androidx.tv.material3.Text(
                text = filename,
                style = androidx.tv.material3.MaterialTheme.typography.headlineMedium,
                color = Color.White,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier
                    .padding(32.dp)
                    .background(Color.Black.copy(alpha = 0.4f), RoundedCornerShape(8.dp))
                    .padding(horizontal = 16.dp, vertical = 8.dp)
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
                // Transcode button - only show for server sources, above the slider
                if (!isSmbSource) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.Start
                    ) {
                        androidx.tv.material3.Surface(
                            onClick = {
                                enableTranscode = !enableTranscode
                                updateInteraction()
                            },
                            shape = androidx.tv.material3.ClickableSurfaceDefaults.shape(
                                shape = RoundedCornerShape(8.dp)
                            ),
                            colors = androidx.tv.material3.ClickableSurfaceDefaults.colors(
                                containerColor = if (isFocusedOnTranscode)
                                    Color.White.copy(alpha = 0.2f)
                                else
                                    Color.Transparent,
                                focusedContainerColor = Color.White.copy(alpha = 0.3f)
                            ),
                            modifier = Modifier.padding(bottom = 8.dp)
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                androidx.tv.material3.Icon(
                                    Icons.Default.VideoSettings,
                                    contentDescription = "Toggle Transcode",
                                    tint = if (enableTranscode) Color.Green else Color.White,
                                    modifier = Modifier.size(32.dp)
                                )
                                Spacer(modifier = Modifier.width(8.dp))
                                androidx.tv.material3.Text(
                                    text = if (enableTranscode) "Transcoding ON" else "Transcoding OFF",
                                    style = androidx.tv.material3.MaterialTheme.typography.bodyMedium,
                                    color = if (enableTranscode) Color.Green else Color.White
                                )
                            }
                        }
                    }
                }

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

                // Playback controls
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    androidx.tv.material3.Icon(
                        Icons.Default.FastRewind,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
                        modifier = Modifier.size(48.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    androidx.tv.material3.Icon(
                        if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
                        modifier = Modifier.size(64.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    androidx.tv.material3.Icon(
                        Icons.Default.FastForward,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
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
