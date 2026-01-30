package com.vargon.weirdstreamer.ui.components

import android.app.Activity
import android.os.Build
import android.view.ViewGroup
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.widget.FrameLayout
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowCompat
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import androidx.media3.datasource.okhttp.OkHttpDataSource
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.WeirdStreamerApplication
import kotlinx.coroutines.delay
import java.util.Locale

@androidx.annotation.OptIn(UnstableApi::class)
@Composable
fun PhoneVideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val view = LocalView.current
    val activity = context as? Activity

    // Hide system bars for immersive fullscreen
    DisposableEffect(Unit) {
        val window = activity?.window
        if (window != null) {
            WindowCompat.setDecorFitsSystemWindows(window, false)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                window.insetsController?.let { controller ->
                    controller.hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                    controller.systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                }
            } else {
                @Suppress("DEPRECATION")
                window.decorView.systemUiVisibility = (
                    android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
                    android.view.View.SYSTEM_UI_FLAG_FULLSCREEN or
                    android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                )
            }
        }
        onDispose {
            if (window != null) {
                WindowCompat.setDecorFitsSystemWindows(window, true)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                    window.insetsController?.show(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                } else {
                    @Suppress("DEPRECATION")
                    window.decorView.systemUiVisibility = android.view.View.SYSTEM_UI_FLAG_VISIBLE
                }
            }
        }
    }

    val isSmbSource = remember(path) { path.startsWith("smb://") }
    
    var showControls by remember { mutableStateOf(true) }
    var isPlaying by remember { mutableStateOf(true) }
    var playbackPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var isBuffering by remember { mutableStateOf(true) }

    val exoPlayer = remember(path) {
        ExoPlayer.Builder(context)
            .apply {
                if (isSmbSource) {
                    setMediaSourceFactory(
                        androidx.media3.exoplayer.source.DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(SmbDataSourceFactory(viewModel.credentialManager, viewModel.baseContext))
                    )
                } else {
                    val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                    val okHttpDataSourceFactory = OkHttpDataSource.Factory(okHttpClient)
                        .setUserAgent("WeirdStreamer/1.0")
                    setMediaSourceFactory(
                        androidx.media3.exoplayer.source.DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(okHttpDataSourceFactory)
                    )
                }
            }
            .build().apply {
                setMediaItem(MediaItem.fromUri(path))
                prepare()
                playWhenReady = true
            }
    }

    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlayingChanged: Boolean) {
                isPlaying = isPlayingChanged
            }
            override fun onPlaybackStateChanged(state: Int) {
                isBuffering = state == Player.STATE_BUFFERING
                if (state == Player.STATE_READY) {
                    duration = exoPlayer.duration
                }
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    LaunchedEffect(exoPlayer) {
        while (true) {
            playbackPosition = exoPlayer.currentPosition
            delay(1000)
        }
    }

    LaunchedEffect(showControls) {
        if (showControls) {
            delay(3000)
            showControls = false
        }
    }

    BackHandler { onBack(path) }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null
            ) { showControls = !showControls }
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

        if (isBuffering) {
            CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = Color.White)
        }

        AnimatedVisibility(
            visible = showControls,
            enter = fadeIn(),
            exit = fadeOut()
        ) {
            Box(modifier = Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.4f))) {
                // Top Bar
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(16.dp)
                        .align(Alignment.TopStart),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    IconButton(onClick = { onBack(path) }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = Color.White)
                    }
                    Text(
                        text = viewModel.getDisplayName(path),
                        color = Color.White,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(start = 8.dp)
                    )
                }

                // Center Controls
                Row(
                    modifier = Modifier.align(Alignment.Center),
                    horizontalArrangement = Arrangement.spacedBy(48.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    IconButton(onClick = { exoPlayer.seekTo(exoPlayer.currentPosition - 10000) }) {
                        Icon(Icons.Default.Replay10, contentDescription = "-10s", tint = Color.White, modifier = Modifier.size(48.dp))
                    }
                    IconButton(onClick = { if (isPlaying) exoPlayer.pause() else exoPlayer.play() }) {
                        Icon(
                            if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                            contentDescription = "Play/Pause",
                            tint = Color.White,
                            modifier = Modifier.size(64.dp)
                        )
                    }
                    IconButton(onClick = { exoPlayer.seekTo(exoPlayer.currentPosition + 30000) }) {
                        Icon(Icons.Default.Forward30, contentDescription = "+30s", tint = Color.White, modifier = Modifier.size(48.dp))
                    }
                }

                // Bottom Controls
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(16.dp)
                        .align(Alignment.BottomCenter)
                ) {
                    Slider(
                        value = if (duration > 0) playbackPosition.toFloat() / duration else 0f,
                        onValueChange = { exoPlayer.seekTo((it * duration).toLong()) },
                        colors = SliderDefaults.colors(
                            thumbColor = Color.Red,
                            activeTrackColor = Color.Red,
                            inactiveTrackColor = Color.White.copy(alpha = 0.3f)
                        )
                    )
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Text(formatTime(playbackPosition), color = Color.White, style = MaterialTheme.typography.labelSmall)
                        Text(formatTime(duration), color = Color.White, style = MaterialTheme.typography.labelSmall)
                    }
                }
            }
        }
    }
}

private fun formatTime(ms: Long): String {
    val totalSeconds = ms / 1000
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    return if (hours > 0) {
        String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
    } else {
        String.format(Locale.US, "%02d:%02d", minutes, seconds)
    }
}
