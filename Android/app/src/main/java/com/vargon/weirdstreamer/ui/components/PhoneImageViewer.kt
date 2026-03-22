package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.gestures.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.core.view.WindowCompat
import android.app.Activity
import android.os.Build
import android.util.Log
import android.view.LayoutInflater
import android.view.WindowInsets
import android.view.WindowInsetsController
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.LivePhotoInfo
import com.vargon.weirdstreamer.LivePhotoType
import com.vargon.weirdstreamer.LivePhotoUtil
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.delay
import java.io.File
import kotlin.OptIn

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneImageViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val activity = context as? Activity
    var immersiveMode by remember { mutableStateOf(false) }

    // Toggle immersive mode on tap
    fun setImmersiveMode(enabled: Boolean) {
        val window = activity?.window
        if (window != null) {
            WindowCompat.setDecorFitsSystemWindows(window, !enabled)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                window.insetsController?.let { controller ->
                    if (enabled) {
                        controller.hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                        controller.systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                    } else {
                        controller.show(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                    }
                }
            } else {
                @Suppress("DEPRECATION")
                window.decorView.systemUiVisibility = if (enabled) {
                    android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
                    android.view.View.SYSTEM_UI_FLAG_FULLSCREEN or
                    android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                } else {
                    android.view.View.SYSTEM_UI_FLAG_VISIBLE
                }
            }
        }
    }

    DisposableEffect(immersiveMode) {
        setImmersiveMode(immersiveMode)
        onDispose { setImmersiveMode(false) }
    }

    val images = remember { viewModel.getImagesInCurrentFolder() }
    val initialIndex = remember { images.indexOf(initialPath).coerceAtLeast(0) }
    val pagerState = rememberPagerState(initialPage = initialIndex) { images.size }
    
    val currentPath = images.getOrNull(pagerState.currentPage) ?: initialPath
    val fileName = remember(currentPath) { viewModel.getDisplayName(currentPath) }

    var isZoomed by remember { mutableStateOf(false) }
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var resetZoomTrigger by remember { mutableIntStateOf(0) }

    LaunchedEffect(pagerState.currentPage) {
        isZoomed = false
        zoomLevel = 1f
    }

    val handleBack = {
        if (zoomLevel > 1f) {
            resetZoomTrigger++
        } else {
            onBack(currentPath)
        }
    }

    BackHandler { handleBack() }

    Scaffold(
        topBar = {
            if (!immersiveMode) {
                TopAppBar(
                    title = { Text(fileName, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = {
                        IconButton(onClick = { handleBack() }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = Color.Black.copy(alpha = 0.5f),
                        titleContentColor = Color.White,
                        navigationIconContentColor = Color.White
                    )
                )
            }
        },
        containerColor = Color.Black
    ) { padding ->
        HorizontalPager(
            state = pagerState,
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            userScrollEnabled = !isZoomed,
            beyondViewportPageCount = 1
        ) { pageIndex ->
            ZoomableImage(
                viewModel = viewModel,
                path = images[pageIndex],
                resetTrigger = if (pageIndex == pagerState.currentPage) resetZoomTrigger else 0,
                onZoomChanged = { zoomed, level ->
                    if (pageIndex == pagerState.currentPage) {
                        isZoomed = zoomed
                        zoomLevel = level
                    }
                },
                onTap = {
                    immersiveMode = !immersiveMode
                }
            )
        }
    }
}

@OptIn(UnstableApi::class)
@Composable
fun ZoomableImage(
    viewModel: MediaViewModel,
    path: String,
    resetTrigger: Int,
    onZoomChanged: (Boolean, Float) -> Unit,
    onTap: () -> Unit
) {
    val context = LocalContext.current
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(Offset.Zero) }
    
    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }
    var isLoading by remember { mutableStateOf(true) }

    // Live Photo state
    var livePhotoInfo by remember { mutableStateOf<LivePhotoInfo?>(null) }
    var showVideo by remember { mutableStateOf(false) }
    var isVideoPlaying by remember { mutableStateOf(false) }
    var isPaused by remember { mutableStateOf(false) }
    var showPauseIcon by remember { mutableStateOf(false) }

    val exoPlayer = remember {
        ExoPlayer.Builder(context).build().apply {
            repeatMode = Player.REPEAT_MODE_OFF
            addListener(object : Player.Listener {
                override fun onPlaybackStateChanged(state: Int) {
                    if (state == Player.STATE_ENDED) {
                        isVideoPlaying = false
                    }
                }
            })
        }
    }

    LaunchedEffect(path) {
        isLoading = true
        livePhotoInfo = null
        showVideo = false
        isVideoPlaying = false
        isPaused = false
        showPauseIcon = false
        exoPlayer.stop()
        exoPlayer.clearMediaItems()
        
        tempFile?.delete()
        tempFile = null
        
        if (viewModel.isImage(path)) {
            val extension = viewModel.getFileExtension(path)
            Log.i("ZoomableImage", "Loading image: $path (ext: $extension)")
            
            // Always download to check for Live Photo metadata/appended video
            val file = viewModel.downloadToTempFile(path, context, ".$extension")
            if (file != null) {
                tempFile = file
                localImagePath = file.absolutePath
                
                val info = LivePhotoUtil.findLivePhotoInfo(file)
                if (info != null) {
                    Log.i("ZoomableImage", "Found Live Photo info: $info")
                    livePhotoInfo = info
                    if (info.type == LivePhotoType.SAMSUNG) {
                        try {
                            val videoFile = File(context.cacheDir, "temp_video_${System.currentTimeMillis()}.mp4")
                            file.inputStream().use { input ->
                                // Reliable skip
                                var toSkip = info.videoOffset
                                while (toSkip > 0) {
                                    val skipped = input.skip(toSkip)
                                    if (skipped <= 0) break
                                    toSkip -= skipped
                                }
                                
                                videoFile.outputStream().use { output ->
                                    val buffer = ByteArray(64 * 1024)
                                    var bytesRead = 0
                                    var totalRead = 0L
                                    while (totalRead < info.videoLength && input.read(buffer).also { bytesRead = it } != -1) {
                                        val toWrite = minOf(bytesRead.toLong(), info.videoLength - totalRead).toInt()
                                        output.write(buffer, 0, toWrite)
                                        totalRead += bytesRead
                                    }
                                }
                            }
                            if (videoFile.exists() && videoFile.length() > 0) {
                                Log.i("ZoomableImage", "Extracted video to ${videoFile.absolutePath} (${videoFile.length()} bytes)")
                                exoPlayer.setMediaItem(MediaItem.fromUri(videoFile.absolutePath))
                            } else {
                                Log.e("ZoomableImage", "Extracted video file is empty or missing")
                                livePhotoInfo = null
                            }
                        } catch (e: Exception) {
                            Log.e("ZoomableImage", "Error extracting Samsung video", e)
                        }
                    } else {
                        Log.i("ZoomableImage", "Using whole file as video for Apple/Generic")
                        exoPlayer.setMediaItem(MediaItem.fromUri(file.absolutePath))
                    }
                    if (livePhotoInfo != null) {
                        exoPlayer.prepare()
                    }
                } else {
                    Log.i("ZoomableImage", "No Live Photo info found for file")
                }
            } else {
                Log.w("ZoomableImage", "Failed to download image to temp file")
                localImagePath = path
            }
        } else {
            localImagePath = path
        }
        isLoading = false
    }

    // Live Photo Loop Logic
    LaunchedEffect(livePhotoInfo, isPaused) {
        if (livePhotoInfo != null && !isPaused) {
            Log.i("ZoomableImage", "Starting Live Photo loop")
            while (true) {
                isVideoPlaying = true
                exoPlayer.seekTo(0)
                // Ensure the video is fully prepared and ready to play
                while (exoPlayer.playbackState == Player.STATE_BUFFERING) {
                    if (isPaused) break
                    delay(10)
                }
                if (isPaused) break

                showVideo = true
                exoPlayer.play()
                
                while (isVideoPlaying) {
                    if (isPaused) break
                    delay(100)
                }
                if (isPaused) break
                
                showVideo = false
                Log.d("ZoomableImage", "Video ended, showing still for 3s")
                delay(3000)
                if (isPaused) break
            }
        } else if (isPaused) {
            exoPlayer.pause()
        }
    }

    LaunchedEffect(isPaused) {
        if (isPaused) {
            showPauseIcon = true
            delay(2000)
            showPauseIcon = false
        } else {
            showPauseIcon = false
        }
    }

    LaunchedEffect(resetTrigger) {
        if (resetTrigger > 0) {
            scale = 1f
            offset = Offset.Zero
            onZoomChanged(false, 1f)
        }
    }

    DisposableEffect(path) {
        onDispose {
            exoPlayer.release()
            tempFile?.delete()
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .pointerInput(path) {
                detectTapGestures(
                    onTap = {
                        if (livePhotoInfo != null) {
                            isPaused = !isPaused
                            if (!isPaused) {
                                // Resume will be handled by LaunchedEffect
                            } else {
                                exoPlayer.pause()
                            }
                        }
                        onTap()
                    }
                )
            }
            .pointerInput(path) {
                awaitEachGesture {
                    awaitFirstDown(requireUnconsumed = false)
                    do {
                        val event = awaitPointerEvent()
                        val isPinch = event.changes.size > 1
                        
                        if (scale > 1f || isPinch) {
                            val zoomChange = event.calculateZoom()
                            val panChange = event.calculatePan()
                            
                            val newScale = (scale * zoomChange).coerceIn(1f, 5f)
                            if (newScale != scale) {
                                scale = newScale
                                onZoomChanged(scale > 1f, scale)
                            }
                            
                            if (scale > 1f) {
                                offset += panChange
                            } else {
                                offset = Offset.Zero
                            }
                            
                            event.changes.forEach { change ->
                                if (change.positionChanged()) {
                                    change.consume()
                                }
                            }
                        }
                    } while (event.changes.any { it.pressed })
                }
            }
    ) {
        Box(
            modifier = Modifier
                .fillMaxSize()
                .graphicsLayer(
                    scaleX = scale,
                    scaleY = scale,
                    translationX = offset.x,
                    translationY = offset.y
                )
        ) {
            localImagePath?.let { p ->
                AsyncImage(
                    model = ImageRequest.Builder(LocalContext.current)
                        .data(p)
                        .crossfade(true)
                        .build(),
                    contentDescription = null,
                    contentScale = ContentScale.Fit,
                    modifier = Modifier.fillMaxSize()
                )
            }

            if (livePhotoInfo != null) {
                AndroidView(
                    factory = { ctx ->
                        val view = LayoutInflater.from(ctx).inflate(R.layout.texture_video_view, null) as PlayerView
                        view.apply {
                            player = exoPlayer
                            useController = false
                            resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                        }
                    },
                    modifier = Modifier.fillMaxSize().alpha(if (showVideo) 1f else 0f)
                )
            }
        }

        if (isLoading) {
            CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = MaterialTheme.colorScheme.primary)
        }
        
        if (showPauseIcon && livePhotoInfo != null) {
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
