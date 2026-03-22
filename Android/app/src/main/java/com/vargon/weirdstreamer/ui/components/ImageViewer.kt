package com.vargon.weirdstreamer.ui.components

import android.util.Log
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.compose.AsyncImagePainter
import coil.request.ImageRequest
import coil.size.Size as CoilSize
import com.vargon.weirdstreamer.LivePhotoInfo
import com.vargon.weirdstreamer.LivePhotoType
import com.vargon.weirdstreamer.LivePhotoUtil
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay
import java.io.File
import kotlin.OptIn

@OptIn(UnstableApi::class, ExperimentalTvMaterial3Api::class)
@Composable
fun ImageViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val images = remember { viewModel.getImagesInCurrentFolder() }
    var currentIndex by remember { mutableIntStateOf(images.indexOf(initialPath).coerceAtLeast(0)) }

    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var rotation by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }

    var showFilename by remember { mutableStateOf(false) }
    var filenameTimerTrigger by remember { mutableLongStateOf(0L) }
    var loopFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var imageIntrinsicSize by remember { mutableStateOf<Size?>(null) }

    val focusRequester = remember { FocusRequester() }
    val currentSmbPath = images.getOrNull(currentIndex) ?: initialPath

    // For HEIC and Live Photo support
    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }
    var livePhotoInfo by remember { mutableStateOf<LivePhotoInfo?>(null) }
    var showVideo by remember { mutableStateOf(false) }
    var isVideoPlaying by remember { mutableStateOf(false) }
    var isPaused by remember { mutableStateOf(false) }
    var showPauseIcon by remember { mutableStateOf(false) }

    val fileName = remember(currentSmbPath) {
        viewModel.getDisplayName(currentSmbPath)
    }

    val exoPlayer = remember {
        ExoPlayer.Builder(context).build().apply {
            repeatMode = Player.REPEAT_MODE_OFF
            addListener(object : Player.Listener {
                override fun onPlaybackStateChanged(state: Int) {
                    if (state == Player.STATE_ENDED) {
                        isVideoPlaying = false
                    }
                }
                override fun onPlayerError(error: androidx.media3.common.PlaybackException) {
                    Log.e("ImageViewer", "ExoPlayer error: ${error.message}", error)
                    isVideoPlaying = false
                }
            })
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            exoPlayer.release()
            tempFile?.delete()
        }
    }

    LaunchedEffect(currentSmbPath) {
        filenameTimerTrigger = System.currentTimeMillis()
        isLoading = true
        errorMessage = null
        imageIntrinsicSize = null
        rotation = 0f
        livePhotoInfo = null
        showVideo = false
        isVideoPlaying = false
        isPaused = false
        showPauseIcon = false
        exoPlayer.stop()
        exoPlayer.clearMediaItems()

        tempFile?.delete()
        tempFile = null

        if (viewModel.isImage(currentSmbPath)) {
            val extension = viewModel.getFileExtension(currentSmbPath)
            Log.i("ImageViewer", "Loading image: $currentSmbPath (ext: $extension)")
            
            // Download to check for Live Photo metadata/appended video
            val file = viewModel.downloadToTempFile(currentSmbPath, context, ".$extension")
            if (file != null) {
                tempFile = file
                localImagePath = file.absolutePath
                
                // Check if it's a live photo
                val info = LivePhotoUtil.findLivePhotoInfo(file)
                if (info != null) {
                    Log.i("ImageViewer", "Found Live Photo info: $info")
                    livePhotoInfo = info
                    if (info.type == LivePhotoType.SAMSUNG) {
                        // Create a separate temp file for the video part of Samsung Motion Photo
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
                                Log.i("ImageViewer", "Extracted video to ${videoFile.absolutePath} (${videoFile.length()} bytes)")
                                exoPlayer.setMediaItem(MediaItem.fromUri(videoFile.absolutePath))
                            } else {
                                Log.e("ImageViewer", "Extracted video file is empty or missing")
                                livePhotoInfo = null
                            }
                        } catch (e: Exception) {
                            Log.e("ImageViewer", "Error extracting Samsung video", e)
                        }
                    } else {
                        Log.i("ImageViewer", "Using whole file as video for Apple/Generic")
                        exoPlayer.setMediaItem(MediaItem.fromUri(file.absolutePath))
                    }
                    if (livePhotoInfo != null) {
                        exoPlayer.prepare()
                    }
                } else {
                    Log.i("ImageViewer", "No Live Photo info found for file")
                }
            } else {
                Log.w("ImageViewer", "Failed to download image to temp file")
                if (extension == "heic" || extension == "heif") {
                    errorMessage = "Failed to download HEIC for decoding"
                }
                localImagePath = currentSmbPath
            }
        } else {
            localImagePath = currentSmbPath
        }
        isLoading = false
    }

    // Live Photo Loop Logic
    LaunchedEffect(livePhotoInfo, isPaused) {
        if (livePhotoInfo != null && !isPaused) {
            Log.i("ImageViewer", "Starting Live Photo loop")
            while (true) {
                // Play video
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
                
                // Wait for video to finish
                while (isVideoPlaying) {
                    if (isPaused) break
                    delay(100)
                }
                if (isPaused) break
                
                // Show still image for 3 seconds
                showVideo = false
                Log.d("ImageViewer", "Video ended, showing still for 3s")
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

    LaunchedEffect(filenameTimerTrigger) {
        if (filenameTimerTrigger > 0) {
            showFilename = true
            delay(1500)
            showFilename = false
        }
    }

    LaunchedEffect(loopFeedback) {
        if (loopFeedback != null) {
            delay(1500)
            loopFeedback = null
        }
    }

    val resetZoom = {
        zoomLevel = 1f
        panX = 0f
        panY = 0f
        rotation = 0f
        isZoomMode = false
    }

    val navigate = { forward: Boolean ->
        val oldIndex = currentIndex
        if (images.isNotEmpty()) {
            if (images.size == 1) {
                filenameTimerTrigger = System.currentTimeMillis()
            } else {
                if (forward) {
                    currentIndex = (currentIndex + 1) % images.size
                    if (currentIndex < oldIndex) loopFeedback = true to System.currentTimeMillis()
                } else {
                    currentIndex = (currentIndex - 1 + images.size) % images.size
                    if (currentIndex > oldIndex) loopFeedback = false to System.currentTimeMillis()
                }
                resetZoom()
            }
        }
    }

    BoxWithConstraints(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .clipToBounds() // Clip everything inside this box
            .then(
                if (isZoomMode) Modifier.border(2.dp, Color.White) else Modifier
            )
    ) {
        val w = constraints.maxWidth.toFloat()
        val h = constraints.maxHeight.toFloat()

        val imageRatio = imageIntrinsicSize?.let {
            if (it.width > 0f && it.height > 0f) it.width / it.height else null
        } ?: (w / h)
        val containerRatio = w / h

        val fittedWidth: Float
        val fittedHeight: Float
        if (imageRatio > containerRatio) {
            fittedWidth = w
            fittedHeight = w / imageRatio
        } else {
            fittedWidth = h * imageRatio
            fittedHeight = h
        }

        val zoomOut = {
            if (isZoomMode) {
                if (zoomLevel > 1f) {
                    val oldZoom = zoomLevel
                    zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f

                    val newScaledWidth = fittedWidth * zoomLevel
                    val newScaledHeight = fittedHeight * zoomLevel
                    val newMaxPX = (newScaledWidth - w).coerceAtLeast(0f) / 2f
                    val newMaxPY = (newScaledHeight - h).coerceAtLeast(0f) / 2f

                    panX = (panX * (zoomLevel / oldZoom)).coerceIn(
                        -newMaxPX,
                        newMaxPX
                    )
                    panY = (panY * (zoomLevel / oldZoom)).coerceIn(
                        -newMaxPY,
                        newMaxPY
                    )
                } else {
                    isZoomMode = false
                }
            } else {
                onBack(currentSmbPath)
            }
        }

        BackHandler { zoomOut() }

        Box(
            modifier = Modifier
                .fillMaxSize()
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionCenter, Key.Enter -> {
                                if (!isZoomMode) {
                                    isZoomMode = true
                                    zoomLevel = 1f
                                } else if (zoomLevel < 64f) {
                                    val oldZoom = zoomLevel
                                    zoomLevel = if (zoomLevel == 1f) 2f else zoomLevel * 2f

                                    val newScaledWidth = fittedWidth * zoomLevel
                                    val newScaledHeight = fittedHeight * zoomLevel
                                    val newMaxPX = (newScaledWidth - w).coerceAtLeast(0f) / 2f
                                    val newMaxPY = (newScaledHeight - h).coerceAtLeast(0f) / 2f

                                    panX =
                                        (panX * (zoomLevel / oldZoom)).coerceIn(
                                            -newMaxPX,
                                            newMaxPX
                                        )
                                    panY =
                                        (panY * (zoomLevel / oldZoom)).coerceIn(
                                            -newMaxPY,
                                            newMaxPY
                                        )
                                }
                                true
                            }

                            Key.MediaPlayPause, Key.Spacebar -> {
                                if (livePhotoInfo != null) {
                                    isPaused = !isPaused
                                    true
                                } else false
                            }

                            Key.Backspace -> {
                                zoomOut()
                                true
                            }

                            Key.DirectionLeft -> {
                                if (isZoomMode) {
                                    val scaledWidth = fittedWidth * zoomLevel
                                    val maxPX = (scaledWidth - w).coerceAtLeast(0f) / 2f
                                    panX = (panX + 100f).coerceAtMost(maxPX)
                                } else {
                                    navigate(false)
                                }
                                true
                            }

                            Key.DirectionRight -> {
                                if (isZoomMode) {
                                    val scaledWidth = fittedWidth * zoomLevel
                                    val maxPX = (scaledWidth - w).coerceAtLeast(0f) / 2f
                                    panX = (panX - 100f).coerceAtLeast(-maxPX)
                                } else {
                                    navigate(true)
                                }
                                true
                            }

                            Key.DirectionUp -> {
                                if (isZoomMode) {
                                    val scaledHeight = fittedHeight * zoomLevel
                                    val maxPY = (scaledHeight - h).coerceAtLeast(0f) / 2f
                                    panY = (panY + 100f).coerceAtMost(maxPY)
                                } else {
                                    rotation += 90f
                                }
                                true
                            }

                            Key.DirectionDown -> {
                                if (isZoomMode) {
                                    val scaledHeight = fittedHeight * zoomLevel
                                    val maxPY = (scaledHeight - h).coerceAtLeast(0f) / 2f
                                    panY = (panY - 100f).coerceAtLeast(-maxPY)
                                } else {
                                    rotation -= 90f
                                }
                                true
                            }

                            else -> false
                        }
                    } else false
                }
                .focusRequester(focusRequester)
                .focusable()
        ) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .graphicsLayer(
                        scaleX = zoomLevel,
                        scaleY = zoomLevel,
                        translationX = panX,
                        translationY = panY,
                        rotationZ = rotation
                    )
            ) {
                localImagePath?.let { path ->
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current)
                            .data(path)
                            .crossfade(true)
                            .size(CoilSize.ORIGINAL)
                            .build(),
                        contentDescription = null,
                        contentScale = ContentScale.Fit,
                        onState = { state ->
                            isLoading = state is AsyncImagePainter.State.Loading
                            if (state is AsyncImagePainter.State.Success) {
                                imageIntrinsicSize = state.painter.intrinsicSize
                            }
                            if (state is AsyncImagePainter.State.Error) {
                                errorMessage = state.result.throwable.message ?: "Failed to load image"
                                Log.e(
                                    "ImageViewer",
                                    "Error loading $path: $errorMessage",
                                    state.result.throwable
                                )
                            }
                        },
                        modifier = Modifier.fillMaxSize()
                    )
                }

                if (livePhotoInfo != null) {
                    AndroidView(
                        factory = { ctx ->
                            PlayerView(ctx).apply {
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
                CircularProgressIndicator(
                    modifier = Modifier.align(Alignment.Center),
                    color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { MaterialTheme.colorScheme.primary }
                )
            }

            if (errorMessage != null) {
                Column(
                    modifier = Modifier.align(Alignment.Center).padding(32.dp),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Text(
                        "Error",
                        style = MaterialTheme.typography.headlineMedium,
                        color = Color.Red
                    )
                    Text(
                        errorMessage!!,
                        style = MaterialTheme.typography.bodyMedium,
                        color = Color.White,
                        textAlign = TextAlign.Center
                    )
                }
            }

            // Filename Overlay
            AnimatedVisibility(
                visible = showFilename && !isZoomMode,
                enter = fadeIn() + slideInVertically(),
                exit = fadeOut() + slideOutVertically(),
                modifier = Modifier.align(Alignment.TopCenter)
            ) {
                Text(
                    text = fileName,
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
                    if (isForward) Icons.Default.Refresh else Icons.Default.Refresh, // Should be Flip or similar
                    contentDescription = null,
                    modifier = Modifier
                        .size(48.dp)
                        .graphicsLayer(rotationY = if (isForward) 0f else 180f),
                    tint = Color.White.copy(alpha = 0.5f)
                )
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

    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }
}
