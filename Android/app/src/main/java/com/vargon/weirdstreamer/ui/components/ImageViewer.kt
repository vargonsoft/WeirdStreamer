package com.vargon.weirdstreamer.ui.components

import android.util.Log
import android.view.LayoutInflater
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
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
import com.vargon.weirdstreamer.R
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

    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }
    var livePhotoInfo by remember { mutableStateOf<LivePhotoInfo?>(null) }
    var showVideo by remember { mutableStateOf(false) }
    var isVideoPlaying by remember { mutableStateOf(false) }
    var isPaused by remember { mutableStateOf(false) }
    var showPauseIcon by remember { mutableStateOf(false) }

    val fileName = remember(currentSmbPath) { viewModel.getDisplayName(currentSmbPath) }
    val exoPlayer = remember {
        ExoPlayer.Builder(context).build().apply {
            repeatMode = Player.REPEAT_MODE_OFF
            addListener(object : Player.Listener {
                override fun onPlaybackStateChanged(state: Int) { if (state == Player.STATE_ENDED) isVideoPlaying = false }
                override fun onPlayerError(error: androidx.media3.common.PlaybackException) { isVideoPlaying = false }
            })
        }
    }

    DisposableEffect(Unit) { onDispose { exoPlayer.release(); tempFile?.delete() } }

    LaunchedEffect(currentSmbPath) {
        filenameTimerTrigger = System.currentTimeMillis(); isLoading = true; errorMessage = null; imageIntrinsicSize = null
        rotation = 0f; livePhotoInfo = null; showVideo = false; isVideoPlaying = false; isPaused = false; showPauseIcon = false
        exoPlayer.stop(); exoPlayer.clearMediaItems(); tempFile?.delete(); tempFile = null

        if (viewModel.isImage(currentSmbPath)) {
            val extension = viewModel.getFileExtension(currentSmbPath)
            val file = viewModel.downloadToTempFile(currentSmbPath, context, ".$extension")
            if (file != null) {
                tempFile = file; localImagePath = file.absolutePath
                val info = LivePhotoUtil.findLivePhotoInfo(file)
                if (info != null) {
                    livePhotoInfo = info
                    if (info.type == LivePhotoType.SAMSUNG) {
                        try {
                            val videoFile = File(context.cacheDir, "temp_video_${System.currentTimeMillis()}.mp4")
                            file.inputStream().use { input ->
                                var toSkip = info.videoOffset
                                while (toSkip > 0) { val skipped = input.skip(toSkip); if (skipped <= 0) break; toSkip -= skipped }
                                videoFile.outputStream().use { output ->
                                    val buffer = ByteArray(64 * 1024); var bytesRead = 0; var totalRead = 0L
                                    while (totalRead < info.videoLength && input.read(buffer).also { bytesRead = it } != -1) {
                                        val toWrite = minOf(bytesRead.toLong(), info.videoLength - totalRead).toInt()
                                        output.write(buffer, 0, toWrite); totalRead += bytesRead
                                    }
                                }
                            }
                            if (videoFile.exists() && videoFile.length() > 0) exoPlayer.setMediaItem(MediaItem.fromUri(videoFile.absolutePath))
                            else livePhotoInfo = null
                        } catch (e: Exception) { livePhotoInfo = null }
                    } else exoPlayer.setMediaItem(MediaItem.fromUri(file.absolutePath))
                    if (livePhotoInfo != null) exoPlayer.prepare()
                }
            } else localImagePath = currentSmbPath
        } else localImagePath = currentSmbPath
        isLoading = false
    }

    LaunchedEffect(livePhotoInfo, isPaused) {
        if (livePhotoInfo != null && !isPaused) {
            while (true) {
                isVideoPlaying = true; exoPlayer.seekTo(0)
                while (exoPlayer.playbackState == Player.STATE_BUFFERING) { if (isPaused) break; delay(10) }
                if (isPaused) break
                showVideo = true; exoPlayer.play()
                while (isVideoPlaying) { if (isPaused) break; delay(100) }
                if (isPaused) break
                showVideo = false; delay(3000)
                if (isPaused) break
            }
        } else if (isPaused) exoPlayer.pause()
    }

    LaunchedEffect(isPaused) { if (isPaused) { showPauseIcon = true; delay(2000); showPauseIcon = false } else showPauseIcon = false }
    LaunchedEffect(filenameTimerTrigger) { if (filenameTimerTrigger > 0) { showFilename = true; delay(1500); showFilename = false } }
    LaunchedEffect(loopFeedback) { if (loopFeedback != null) { delay(1500); loopFeedback = null } }

    val resetZoom = { zoomLevel = 1f; panX = 0f; panY = 0f; rotation = 0f; isZoomMode = false }
    val navigate = { forward: Boolean ->
        val old = currentIndex
        if (images.isNotEmpty()) {
            if (images.size == 1) filenameTimerTrigger = System.currentTimeMillis()
            else {
                if (forward) { currentIndex = (currentIndex + 1) % images.size; if (currentIndex < old) loopFeedback = true to System.currentTimeMillis() }
                else { currentIndex = (currentIndex - 1 + images.size) % images.size; if (currentIndex > old) loopFeedback = false to System.currentTimeMillis() }
                resetZoom()
            }
        }
    }

    BoxWithConstraints(modifier = Modifier.fillMaxSize().background(Color.Black).clipToBounds().then(if (isZoomMode) Modifier.border(2.dp, Color.White) else Modifier)) {
        val w = constraints.maxWidth.toFloat(); val h = constraints.maxHeight.toFloat()
        val imageRatio = imageIntrinsicSize?.let { if (it.width > 0f && it.height > 0f) it.width / it.height else null } ?: (w / h)
        val containerRatio = w / h
        val fittedWidth = if (imageRatio > containerRatio) w else h * imageRatio
        val fittedHeight = if (imageRatio > containerRatio) w / imageRatio else h

        val zoomOut = {
            if (isZoomMode) {
                if (zoomLevel > 1f) {
                    val old = zoomLevel; zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f
                    val nsw = fittedWidth * zoomLevel; val nsh = fittedHeight * zoomLevel
                    val nmpX = (nsw - w).coerceAtLeast(0f) / 2f; val nmpY = (nsh - h).coerceAtLeast(0f) / 2f
                    panX = (panX * (zoomLevel / old)).coerceIn(-nmpX, nmpX)
                    panY = (panY * (zoomLevel / old)).coerceIn(-nmpY, nmpY)
                } else isZoomMode = false
            } else onBack(currentSmbPath)
        }

        BackHandler { zoomOut() }

        Box(
            modifier = Modifier.fillMaxSize()
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionCenter, Key.Enter -> {
                                if (!isZoomMode) { isZoomMode = true; zoomLevel = 1f }
                                else if (zoomLevel < 64f) {
                                    val old = zoomLevel; zoomLevel = if (zoomLevel == 1f) 2f else zoomLevel * 2f
                                    val nsw = fittedWidth * zoomLevel; val nsh = fittedHeight * zoomLevel
                                    val nmpX = (nsw - w).coerceAtLeast(0f) / 2f; val nmpY = (nsh - h).coerceAtLeast(0f) / 2f
                                    panX = (panX * (zoomLevel / old)).coerceIn(-nmpX, nmpX)
                                    panY = (panY * (zoomLevel / old)).coerceIn(-nmpY, nmpY)
                                }
                                true
                            }
                            Key.MediaPlayPause, Key.Spacebar -> { if (livePhotoInfo != null) { isPaused = !isPaused; true } else false }
                            Key.Backspace -> { zoomOut(); true }
                            Key.DirectionLeft -> { if (isZoomMode) { panX = (panX + 100f).coerceAtMost((fittedWidth * zoomLevel - w).coerceAtLeast(0f) / 2f) } else navigate(false); true }
                            Key.DirectionRight -> { if (isZoomMode) { panX = (panX - 100f).coerceAtLeast(-(fittedWidth * zoomLevel - w).coerceAtLeast(0f) / 2f) } else navigate(true); true }
                            Key.DirectionUp -> { if (isZoomMode) { panY = (panY + 100f).coerceAtMost((fittedHeight * zoomLevel - h).coerceAtLeast(0f) / 2f) } else rotation += 90f; true }
                            Key.DirectionDown -> { if (isZoomMode) { panY = (panY - 100f).coerceAtLeast(-(fittedHeight * zoomLevel - h).coerceAtLeast(0f) / 2f) } else rotation -= 90f; true }
                            else -> false
                        }
                    } else false
                }.focusRequester(focusRequester).focusable()
        ) {
            Box(modifier = Modifier.fillMaxSize().graphicsLayer(scaleX = zoomLevel, scaleY = zoomLevel, translationX = panX, translationY = panY, rotationZ = rotation)) {
                localImagePath?.let { path ->
                    AsyncImage(
                        model = ImageRequest.Builder(LocalContext.current).data(path).crossfade(true).size(CoilSize.ORIGINAL).build(),
                        contentDescription = null, contentScale = ContentScale.Fit,
                        onState = { state ->
                            isLoading = state is AsyncImagePainter.State.Loading
                            if (state is AsyncImagePainter.State.Success) imageIntrinsicSize = state.painter.intrinsicSize
                            if (state is AsyncImagePainter.State.Error) errorMessage = state.result.throwable.message ?: "Failed to load image"
                        },
                        modifier = Modifier.fillMaxSize()
                    )
                }
                if (livePhotoInfo != null) {
                    AndroidView(
                        factory = { ctx ->
                            LayoutInflater.from(ctx).inflate(R.layout.texture_video_view, null).let { it as PlayerView }.apply { player = exoPlayer; useController = false; resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT }
                        },
                        modifier = Modifier.fillMaxSize().alpha(if (showVideo) 1f else 0f)
                    )
                }
            }

            if (isLoading) CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { MaterialTheme.colorScheme.primary })
            if (errorMessage != null) {
                Column(modifier = Modifier.align(Alignment.Center).padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("Error", style = MaterialTheme.typography.headlineMedium, color = Color.Red)
                    Text(errorMessage!!, style = MaterialTheme.typography.bodyMedium, color = Color.White, textAlign = TextAlign.Center)
                }
            }

            ImageOverlays(
                showFilename = showFilename,
                filename = fileName,
                isZoomMode = isZoomMode,
                loopFeedback = loopFeedback,
                showPauseIcon = showPauseIcon,
                isLivePhoto = livePhotoInfo != null
            )
        }
    }

    LaunchedEffect(Unit) { focusRequester.requestFocus() }
}
