package com.vargon.weirdstreamer.ui.components

import android.app.Activity
import android.os.Build
import android.util.Log
import android.view.LayoutInflater
import android.view.ViewGroup
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.widget.FrameLayout
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsDraggedAsState
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.RotateRight
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowCompat
import androidx.media3.common.MediaItem
import androidx.media3.common.Player
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import androidx.media3.datasource.okhttp.OkHttpDataSource
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.delay
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.Locale

@androidx.annotation.OptIn(UnstableApi::class)
@Composable
fun PhoneVideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val activity = context as? Activity

    // Hide system bars for immersive fullscreen
    DisposableEffect(Unit) {
        val window = activity?.window
        if (window != null) {
            WindowCompat.setDecorFitsSystemWindows(window, false)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                window.insetsController?.let { controller ->
                    controller.hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                    controller.systemBarsBehavior =
                        WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
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
    var playbackPosition by remember(path) { mutableLongStateOf(0L) }
    var duration by remember(path) { mutableLongStateOf(0L) }
    var isBuffering by remember { mutableStateOf(true) }

    // Trickplay state
    var trickplayInfo by remember { mutableStateOf<TrickplayInfo?>(null) }
    val trickplayListState = rememberLazyListState()
    val isTrickplayDragged by trickplayListState.interactionSource.collectIsDraggedAsState()
    val isTrickplayPressed by trickplayListState.interactionSource.collectIsPressedAsState()
    var lastTrickplayInteractionTime by remember { mutableLongStateOf(0L) }

    LaunchedEffect(isTrickplayDragged, isTrickplayPressed, trickplayListState.isScrollInProgress) {
        if (isTrickplayDragged || isTrickplayPressed || trickplayListState.isScrollInProgress) {
            lastTrickplayInteractionTime = System.currentTimeMillis()
        }
    }

    // New states
    var enableTranscode by remember { mutableStateOf(false) }
    var transcodeStartOffset by remember { mutableLongStateOf(0L) }
    var fullDuration by remember { mutableLongStateOf(0L) }
    var rotation by remember { mutableFloatStateOf(0f) }
    
    // Pinch to zoom state
    var zoomScale by remember { mutableFloatStateOf(1f) }
    var zoomOffsetX by remember { mutableFloatStateOf(0f) }
    var zoomOffsetY by remember { mutableFloatStateOf(0f) }

    val speedIntervals = listOf(0.1f, 0.25f, 0.5f, 1f, 2f, 4f, 6f, 8f, 10f, 12f, 16f, 32f, 64f)
    var currentSpeedIndex by remember { mutableIntStateOf(speedIntervals.indexOf(1f)) }
    var showSpeedDialog by remember { mutableStateOf(false) }
    
    val aspectRatios = listOf("Original", "16:9", "21:9", "4:3", "5:4", "1:1")
    val aspectRatioValues = listOf(null, 16f / 9f, 21f / 9f, 4f / 3f, 5f / 4f, 1f)
    var currentAspectRatioIndex by remember { mutableIntStateOf(0) }
    var currentAspectRatio by remember { mutableStateOf<Float?>(null) }
    var videoAspectRatio by remember { mutableFloatStateOf(16f / 9f) }

    // Fetch metadata for duration and trickplay
    LaunchedEffect(path) {
        if (!isSmbSource) {
            try {
                val encodedPath = path.substringAfter("path=").substringBefore("&")
                val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                val retrofit = Retrofit.Builder()
                    .baseUrl(path.substringBefore("/api"))
                    .client(okHttpClient)
                    .addConverterFactory(GsonConverterFactory.create())
                    .build()
                val api = retrofit.create(ApiService::class.java)
                val response = api.getMetadata(encodedPath)
                if (response.isSuccessful) {
                    response.body()?.let { metadata ->
                        metadata.duration?.let { durationSecs ->
                            fullDuration = (durationSecs * 1000).toLong()
                        }
                        
                        // Fetch trickplay info
                        val item = viewModel.getFileItemForPath(path)
                        if (item != null) {
                            trickplayInfo = viewModel.fetchTrickplayInfo(item)
                        }
                    }
                }
            } catch (e: Exception) {
                Log.e("PhoneVideoPlayer", "Failed to load metadata", e)
            }
        }
    }

    val exoPlayer = remember(path) {
        ExoPlayer.Builder(context)
            .apply {
                if (isSmbSource) {
                    setMediaSourceFactory(
                        androidx.media3.exoplayer.source.DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(
                                SmbDataSourceFactory(
                                    viewModel.credentialManager,
                                    viewModel.baseContext
                                )
                            )
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
            .build()
    }

    // Handle MediaItem updates when path or transcode mode changes
    LaunchedEffect(path, enableTranscode) {
        // Capture current position if available
        val currentPos = if (exoPlayer.mediaItemCount > 0) {
            if (transcodeStartOffset > 0) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
        } else {
            playbackPosition
        }

        val url = if (!isSmbSource && enableTranscode) {
            val startTimeSeconds = currentPos / 1000f
            transcodeStartOffset = currentPos
            val base = path.substringBefore("?")
            val pathParam = path.substringAfter("path=").substringBefore("&")
            "$base?path=$pathParam&auto-transcode=0&transcode=1&start=$startTimeSeconds"
        } else {
            transcodeStartOffset = 0L
            path
        }

        val mediaItem = MediaItem.fromUri(url)
        // If enabling transcode, the start position is handled by the URL 'start' param
        // If disabling transcode, we need to seek the player to the previous absolute position
        if (!isSmbSource && !enableTranscode && currentPos > 0) {
            exoPlayer.setMediaItem(mediaItem, currentPos)
        } else {
            exoPlayer.setMediaItem(mediaItem)
        }
        
        exoPlayer.playbackParameters = PlaybackParameters(speedIntervals[currentSpeedIndex])
        exoPlayer.prepare()
        exoPlayer.play()
    }

    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlayingChanged: Boolean) {
                isPlaying = isPlayingChanged
            }

            override fun onPlaybackStateChanged(state: Int) {
                isBuffering = state == Player.STATE_BUFFERING
                if (state == Player.STATE_READY) {
                    val currentDuration = exoPlayer.duration
                    if (fullDuration > 0) {
                        duration = fullDuration
                    } else if (enableTranscode) {
                        val calculatedFullDuration = currentDuration + transcodeStartOffset
                        if (calculatedFullDuration > fullDuration) {
                            fullDuration = calculatedFullDuration
                        }
                        duration = fullDuration
                    } else {
                        duration = currentDuration
                        fullDuration = currentDuration
                    }
                    
                    val videoSize = exoPlayer.videoSize
                    if (videoSize.width > 0 && videoSize.height > 0) {
                        videoAspectRatio = videoSize.width.toFloat() / videoSize.height.toFloat()
                    }
                }
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    fun seekPlayer(targetPos: Long) {
        if (!isSmbSource && enableTranscode) {
            val startTimeSeconds = targetPos / 1000f
            val baseUrlWithPath = if (path.contains("?")) {
                val base = path.substringBefore("?")
                val pathParam = path.substringAfter("path=").substringBefore("&")
                "$base?path=$pathParam"
            } else {
                path
            }
            val newUrl = "$baseUrlWithPath&auto-transcode=0&transcode=1&start=$startTimeSeconds"
            transcodeStartOffset = targetPos
            val mediaItem = MediaItem.fromUri(newUrl)
            exoPlayer.setMediaItem(mediaItem)
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true
        } else {
            exoPlayer.seekTo(targetPos)
        }
    }

    LaunchedEffect(exoPlayer) {
        while (true) {
            // Only update position from player if it has a media item loaded and is in a valid state
            if (exoPlayer.mediaItemCount > 0 && (exoPlayer.playbackState == Player.STATE_READY || exoPlayer.playbackState == Player.STATE_BUFFERING)) {
                playbackPosition = if (enableTranscode) {
                    exoPlayer.currentPosition + transcodeStartOffset
                } else {
                    exoPlayer.currentPosition
                }
            }
            delay(1000)
        }
    }

    LaunchedEffect(showControls, showSpeedDialog) {
        if (showControls && !showSpeedDialog) {
            delay(5000)
            if (!showSpeedDialog) showControls = false
        }
    }

    BackHandler { 
        if (showSpeedDialog) {
            showSpeedDialog = false
        } else if (zoomScale > 1f) {
            // Reset zoom on back if zoomed
            zoomScale = 1f
            zoomOffsetX = 0f
            zoomOffsetY = 0f
        } else {
            onBack(path) 
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .pointerInput(Unit) {
                detectTransformGestures { _, pan, zoom, _ ->
                    zoomScale = (zoomScale * zoom).coerceIn(1f, 10f)
                    if (zoomScale > 1f) {
                        zoomOffsetX += pan.x
                        zoomOffsetY += pan.y
                    } else {
                        zoomOffsetX = 0f
                        zoomOffsetY = 0f
                    }
                }
            }
            .clickable(
                interactionSource = remember { MutableInteractionSource() },
                indication = null
            ) { showControls = !showControls }
    ) {
        Box(
            modifier = Modifier.fillMaxSize(),
            contentAlignment = Alignment.Center
        ) {
            Box(
                modifier = Modifier
                    .then(
                        if (currentAspectRatio != null) {
                            Modifier.aspectRatio(currentAspectRatio!!)
                        } else {
                            Modifier.fillMaxSize()
                        }
                    ),
                contentAlignment = Alignment.Center
            ) {
                AndroidView(
                    factory = { ctx ->
                        // Inflate layout with PlayerView configured to use TextureView
                        val view = LayoutInflater.from(ctx).inflate(R.layout.texture_video_view, null) as PlayerView
                        view.apply {
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
                    modifier = Modifier
                        .fillMaxSize()
                        .graphicsLayer {
                            this.rotationZ = rotation
                            
                            val isRotated = (rotation / 90f).toInt() % 2 != 0
                            
                            // 1. Handle base scaling for rotation to prevent squishing and overflow
                            val rotationScale = if (isRotated && size.width != 0f && size.height != 0f) {
                                minOf(size.width / size.height, size.height / size.width)
                            } else {
                                1f
                            }
                            
                            // 2. Handle aspect ratio stretching
                            val stretchScaleX: Float
                            val stretchScaleY: Float
                            if (currentAspectRatio != null) {
                                if (currentAspectRatio!! > videoAspectRatio) {
                                    stretchScaleX = currentAspectRatio!! / videoAspectRatio
                                    stretchScaleY = 1f
                                } else {
                                    stretchScaleX = 1f
                                    stretchScaleY = videoAspectRatio / currentAspectRatio!!
                                }
                            } else {
                                stretchScaleX = 1f
                                stretchScaleY = 1f
                            }
                            
                            // 3. Combine all scales including manual pinch-to-zoom
                            val commonScale = zoomScale * rotationScale
                            this.scaleX = commonScale * stretchScaleX
                            this.scaleY = commonScale * stretchScaleY

                            this.translationX = zoomOffsetX
                            this.translationY = zoomOffsetY
                        }
                )
            }
        }

        // Loading Spinner
        if (isBuffering) {
            CircularProgressIndicator(
                modifier = Modifier.align(Alignment.Center),
                color = Color.White
            )
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
                        Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = "Back",
                            tint = Color.White
                        )
                    }
                    Text(
                        text = viewModel.getDisplayName(path),
                        color = Color.White,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(start = 8.dp).weight(1f)
                    )
                    
                    // Transcode toggle
                    if (!isSmbSource) {
                        IconButton(onClick = { enableTranscode = !enableTranscode }) {
                            Icon(
                                Icons.Default.VideoSettings,
                                contentDescription = "Transcode",
                                tint = if (enableTranscode) Color.Green else Color.White
                            )
                        }
                    }
                    
                    // Rotation
                    IconButton(onClick = { rotation = (rotation + 90f) % 360f }) {
                        Icon(
                            Icons.AutoMirrored.Filled.RotateRight,
                            contentDescription = "Rotate",
                            tint = Color.White
                        )
                    }
                }

                // Center Controls
                Column(
                    modifier = Modifier.align(Alignment.Center),
                    horizontalAlignment = Alignment.CenterHorizontally
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(48.dp)
                    ) {
                        IconButton(onClick = { seekPlayer((playbackPosition - 10000).coerceAtLeast(0)) }) {
                            Icon(
                                Icons.Default.Replay10,
                                contentDescription = "-10s",
                                tint = Color.White,
                                modifier = Modifier.size(48.dp)
                            )
                        }
                        IconButton(onClick = { if (isPlaying) exoPlayer.pause() else exoPlayer.play() }) {
                            Icon(
                                if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                                contentDescription = "Play/Pause",
                                tint = Color.White,
                                modifier = Modifier.size(64.dp)
                            )
                        }
                        IconButton(onClick = { seekPlayer((playbackPosition + 30000).coerceAtMost(duration)) }) {
                            Icon(
                                Icons.Default.Forward30,
                                contentDescription = "+30s",
                                tint = Color.White,
                                modifier = Modifier.size(48.dp)
                            )
                        }
                    }
                    
                    // Frame-by-frame controls when paused
                    AnimatedVisibility(
                        visible = !isPlaying,
                        enter = fadeIn() + expandVertically(),
                        exit = fadeOut() + shrinkVertically()
                    ) {
                        Row(
                            modifier = Modifier.padding(top = 16.dp),
                            horizontalArrangement = Arrangement.spacedBy(64.dp)
                        ) {
                            IconButton(onClick = { 
                                // Approx 1 frame back at 24fps
                                exoPlayer.seekTo((exoPlayer.currentPosition - 40L).coerceAtLeast(0L))
                            }) {
                                Icon(
                                    Icons.Default.SkipPrevious,
                                    contentDescription = "Frame Back",
                                    tint = Color.White,
                                    modifier = Modifier.size(32.dp)
                                )
                            }
                            IconButton(onClick = { 
                                // Approx 1 frame forward at 24fps
                                exoPlayer.seekTo(exoPlayer.currentPosition + 40L)
                            }) {
                                Icon(
                                    Icons.Default.SkipNext,
                                    contentDescription = "Frame Forward",
                                    tint = Color.White,
                                    modifier = Modifier.size(32.dp)
                                )
                            }
                        }
                    }
                }

                // Bottom Controls
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(16.dp)
                        .align(Alignment.BottomCenter)
                ) {
                    // Trickplay Frames
                    if (trickplayInfo != null) {
                        LazyRow(
                            state = trickplayListState,
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(80.dp)
                                .padding(bottom = 8.dp),
                            horizontalArrangement = Arrangement.spacedBy(4.dp),
                            contentPadding = PaddingValues(horizontal = 8.dp)
                        ) {
                            itemsIndexed(trickplayInfo!!.frames) { _, frame ->
                                Box(
                                    modifier = Modifier
                                        .width(100.dp)
                                        .fillMaxHeight()
                                        .clip(RoundedCornerShape(4.dp))
                                        .clickable {
                                            lastTrickplayInteractionTime = System.currentTimeMillis()
                                            seekPlayer(frame.time.toLong() * 1000)
                                        }
                                ) {
                                    AsyncImage(
                                        model = ImageRequest.Builder(LocalContext.current)
                                            .data(frame.url)
                                            .crossfade(true)
                                            .build(),
                                        contentDescription = null,
                                        contentScale = ContentScale.Crop,
                                        modifier = Modifier.fillMaxSize()
                                    )
                                    Text(
                                        text = formatTime(frame.time.toLong() * 1000),
                                        style = MaterialTheme.typography.labelSmall,
                                        color = Color.White,
                                        modifier = Modifier
                                            .align(Alignment.BottomEnd)
                                            .background(Color.Black.copy(alpha = 0.6f))
                                            .padding(horizontal = 2.dp)
                                    )
                                }
                            }
                        }
                        
                        // Auto-scroll trickplay list to current position
                        // Only if the user hasn't interacted with it recently
                        LaunchedEffect(playbackPosition) {
                            val frames = trickplayInfo?.frames ?: return@LaunchedEffect
                            val currentTime = System.currentTimeMillis()
                            val interactionTimeout = 8000L
                            
                            if (!isTrickplayDragged && !isTrickplayPressed && !trickplayListState.isScrollInProgress && 
                                (currentTime - lastTrickplayInteractionTime) > interactionTimeout) {
                                
                                val index = frames.indexOfFirst { it.time * 1000 >= playbackPosition }
                                if (index != -1) {
                                    trickplayListState.animateScrollToItem(index)
                                }
                            }
                        }
                    }

                    Row(
                        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
                        horizontalArrangement = Arrangement.End,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        // Aspect Ratio button
                        TextButton(onClick = {
                            currentAspectRatioIndex = (currentAspectRatioIndex + 1) % aspectRatios.size
                            currentAspectRatio = aspectRatioValues[currentAspectRatioIndex]
                        }) {
                            Icon(Icons.Default.AspectRatio, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(4.dp))
                            Text(aspectRatios[currentAspectRatioIndex], color = Color.White, style = MaterialTheme.typography.labelMedium)
                        }
                        
                        Spacer(Modifier.width(16.dp))
                        
                        // Speed button
                        TextButton(onClick = {
                            showSpeedDialog = true
                        }) {
                            Icon(Icons.Default.Speed, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(4.dp))
                            Text("${speedIntervals[currentSpeedIndex]}x", color = Color.White, style = MaterialTheme.typography.labelMedium)
                        }
                    }

                    Slider(
                        value = if (duration > 0) playbackPosition.toFloat() / duration else 0f,
                        onValueChange = { 
                            val targetPos = (it * duration).toLong()
                            seekPlayer(targetPos)
                        },
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
                        Text(
                            formatTime(playbackPosition),
                            color = Color.White,
                            style = MaterialTheme.typography.labelSmall
                        )
                        Text(
                            formatTime(duration),
                            color = Color.White,
                            style = MaterialTheme.typography.labelSmall
                        )
                    }
                }
            }
        }

        // Speed Selection Dialog
        if (showSpeedDialog) {
            Dialog(onDismissRequest = { showSpeedDialog = false }) {
                Surface(
                    modifier = Modifier
                        .width(280.dp)
                        .heightIn(max = 450.dp),
                    shape = RoundedCornerShape(16.dp),
                    color = MaterialTheme.colorScheme.surface
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            text = "Playback Speed",
                            style = MaterialTheme.typography.titleLarge,
                            modifier = Modifier.padding(bottom = 16.dp)
                        )
                        
                        LazyColumn(
                            modifier = Modifier.weight(1f, fill = false),
                            verticalArrangement = Arrangement.spacedBy(4.dp)
                        ) {
                            itemsIndexed(speedIntervals) { index, speed ->
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clickable {
                                            currentSpeedIndex = index
                                            exoPlayer.playbackParameters = PlaybackParameters(speed)
                                            showSpeedDialog = false
                                        }
                                        .padding(vertical = 12.dp, horizontal = 8.dp),
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    RadioButton(
                                        selected = currentSpeedIndex == index,
                                        onClick = {
                                            currentSpeedIndex = index
                                            exoPlayer.playbackParameters = PlaybackParameters(speed)
                                            showSpeedDialog = false
                                        }
                                    )
                                    Spacer(Modifier.width(12.dp))
                                    Text(
                                        text = "${speed}x",
                                        style = MaterialTheme.typography.bodyLarge
                                    )
                                }
                            }
                        }
                        
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(top = 16.dp),
                            horizontalArrangement = Arrangement.End
                        ) {
                            TextButton(onClick = { showSpeedDialog = false }) {
                                Text("Cancel")
                            }
                        }
                    }
                }
            }
        }
    }

    fun formatTime(ms: Long): String {
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
}
