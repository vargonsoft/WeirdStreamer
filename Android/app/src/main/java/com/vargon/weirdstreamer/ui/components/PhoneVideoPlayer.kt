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
import androidx.compose.ui.unit.sp
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
import com.vargon.weirdstreamer.api.models.DiscInfo
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.Locale

@OptIn(ExperimentalMaterial3Api::class)
@androidx.annotation.OptIn(UnstableApi::class)
@Composable
fun PhoneVideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val activity = context as? Activity
    val view = LocalView.current

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
    // A download on this device: played as it is, with nothing asked of or reported to the server.
    val isLocal = remember(path) { path.startsWith("file://") }

    // DVD disc images (.iso) are always transcoded by the server, one title at a time, with the
    // chosen angle and tracks; the disc sheet changes them.
    val isDisc = remember(path) {
        viewModel.getFileItemForPath(path)?.let { item ->
            item.name.endsWith(".iso", ignoreCase = true) || (item.originalPath ?: "").endsWith(".iso", ignoreCase = true)
        } ?: false
    }
    var discInfo by remember { mutableStateOf<DiscInfo?>(null) }
    var discTitle by remember { mutableIntStateOf(0) } // 0: the main title
    var discAngle by remember { mutableIntStateOf(1) }
    var discAudio by remember { mutableIntStateOf(0) }
    var discSubtitle by remember { mutableIntStateOf(-1) }
    var showDiscSheet by remember { mutableStateOf(false) }
    var discRestartAt by remember { mutableStateOf<Long?>(null) } // a new title starts at 0, not where the last was
    val discQuery = if (isDisc) "&title=$discTitle&angle=$discAngle&audio=$discAudio&subtitle=$discSubtitle" else ""

    var showControls by remember { mutableStateOf(true) }
    var isPlaying by remember { mutableStateOf(true) }
    // Starts where the user stopped last time (0 to start over), as the view model says.
    var playbackPosition by remember(path) { mutableLongStateOf(viewModel.playbackStartMs) }
    var duration by remember(path) { mutableLongStateOf(0L) }
    var isBuffering by remember { mutableStateOf(true) }
    var ended by remember(path) { mutableStateOf(false) }

    // Keep screen on while playing or buffering
    DisposableEffect(isPlaying, isBuffering) {
        val shouldKeepScreenOn = isPlaying || isBuffering
        if (shouldKeepScreenOn) {
            view.keepScreenOn = true
        }
        onDispose {
            view.keepScreenOn = false
        }
    }

    // Trickplay state
    var trickplayInfo by remember { mutableStateOf<TrickplayInfo?>(null) }
    val trickplayListState = rememberLazyListState()
    val isTrickplayDragged by trickplayListState.interactionSource.collectIsDraggedAsState()
    val isTrickplayPressed by trickplayListState.interactionSource.collectIsPressedAsState()

    // Seeker state
    val sliderInteractionSource = remember { MutableInteractionSource() }
    val isSliderDragged by sliderInteractionSource.collectIsDraggedAsState()
    val isSliderPressed by sliderInteractionSource.collectIsPressedAsState()

    var lastInteractionTime by remember { mutableLongStateOf(0L) }

    LaunchedEffect(isTrickplayDragged, isTrickplayPressed, trickplayListState.isScrollInProgress, isSliderDragged, isSliderPressed) {
        if (isTrickplayDragged || isTrickplayPressed || trickplayListState.isScrollInProgress || isSliderDragged || isSliderPressed) {
            lastInteractionTime = System.currentTimeMillis()
        }
    }

    // Transcoding: a mode from the TV's list (0: the original file). Discs are always transcoded,
    // and so is a video too big for this phone's hardware decoder (4K on many phones).
    val sourceItem = remember(path) { viewModel.getFileItemForPath(path) }
    val tooBigToDecode = remember(path) {
        !isSmbSource && !isLocal && !VideoDecoders.canPlay(sourceItem?.width, sourceItem?.height)
    }
    var transcodeModeIndex by remember(path) { mutableIntStateOf(if (isDisc || tooBigToDecode) 1 else 0) }
    var enableTranscode by remember { mutableStateOf(isDisc || tooBigToDecode) }
    var showTranscodeMenu by remember { mutableStateOf(false) }
    fun chooseTranscodeMode(index: Int) {
        transcodeModeIndex = index
        enableTranscode = index != 0
    }
    var transcodeStartOffset by remember { mutableLongStateOf(0L) }
    var fullDuration by remember { mutableLongStateOf(0L) }

    // Subtitles from the server, as on the TV and the web: the video's own tracks, then files
    // beside it and downloads, drawn over the video whatever the stream. A picture track is burned
    // into a transcode. SMB files, downloads on this phone and discs have none.
    val serverSubs = remember(path) { if (isSmbSource || isLocal || isDisc) null else ServerSubtitles.forStream(path) }
    val burnIn = serverSubs?.burnIn ?: -1
    // Stereo audio in transcodes, once a surround one has failed to decode (AudioFallback)
    var stereoAudio by remember(path) { mutableStateOf(AudioFallback.stereo) }
    var showSubtitleMenu by remember { mutableStateOf(false) }
    var showSubtitleSearch by remember { mutableStateOf(false) }
    val subtitleVideo = remember(path) { sourceItem?.originalPath ?: path.substringBefore("&") }
    fun chooseServerSubtitle(index: Int) {
        val subs = serverSubs ?: return
        subs.select(index)
        // The original can't show a picture track: it's burned into a transcode.
        if (subs.burnIn >= 0 && !enableTranscode) chooseTranscodeMode(1)
    }
    LaunchedEffect(serverSubs) {
        val subs = serverSubs ?: return@LaunchedEffect
        subs.load()
        val index = SubtitleMemory.get(context, subtitleVideo)?.let { SubtitleMemory.resolve(it, subs.labels) }
            ?: subs.tracks.indexOfFirst { it.forced && it.text }
        if (index >= 0) chooseServerSubtitle(index)
    }
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
        if (!isSmbSource && !isLocal) {
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
                        if (isDisc) {
                            api.getDisc(encodedPath).body()?.let { disc ->
                                discInfo = disc
                                disc.title(discTitle)?.let { fullDuration = (it.duration * 1000).toLong() }
                            }
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
                if (isLocal) {
                    setMediaSourceFactory(androidx.media3.exoplayer.source.DefaultMediaSourceFactory(context))
                } else if (isSmbSource) {
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
            .apply {
                // The server's subtitles are drawn over the video, so the stream's own stay off.
                trackSelectionParameters = if (serverSubs != null) trackSelectionParameters.buildUpon()
                    .setTrackTypeDisabled(androidx.media3.common.C.TRACK_TYPE_TEXT, true).build()
                else subtitlesOffByDefault(trackSelectionParameters)
            }
    }

    // Handle MediaItem updates when path or transcode mode changes
    LaunchedEffect(path, enableTranscode, transcodeModeIndex, discQuery, burnIn, stereoAudio) {
        // Capture current position if available
        val currentPos = discRestartAt?.also { discRestartAt = null } ?: if (exoPlayer.mediaItemCount > 0) {
            if (transcodeStartOffset > 0) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
        } else {
            playbackPosition
        }

        val url = if (!isSmbSource && !isLocal && enableTranscode) {
            val startTimeSeconds = currentPos / 1000f
            transcodeStartOffset = currentPos
            val base = path.substringBefore("?")
            val pathParam = path.substringAfter("path=").substringBefore("&")
            "$base?path=$pathParam&auto-transcode=0&transcode=1&start=$startTimeSeconds$discQuery" +
                transcodeQuery(transcodeModes[transcodeModeIndex]) + (if (burnIn >= 0) "&subtitle=$burnIn" else "") +
                (if (stereoAudio) "&channels=2" else "")
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

            // The phone can't decode the original (too big, or a codec it lacks): have the server
            // transcode it instead, from the same place. A download can't be transcoded: say so.
            override fun onPlayerError(error: androidx.media3.common.PlaybackException) {
                val decoderProblem = error.errorCode in androidx.media3.common.PlaybackException.ERROR_CODE_DECODER_INIT_FAILED..
                    androidx.media3.common.PlaybackException.ERROR_CODE_DECODING_RESOURCES_RECLAIMED
                if (AudioFallback.isAudioDecoderFailure(error) && enableTranscode && !isSmbSource && !isLocal && !stereoAudio) {
                    // The transcode's surround audio didn't decode: again from here, in stereo
                    // (the stream restarts where the player is).
                    AudioFallback.stereo = true
                    stereoAudio = true
                    android.widget.Toast.makeText(context, "This phone can't play the surround audio. Switching to stereo.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else if (decoderProblem && !enableTranscode && !isSmbSource && !isLocal) {
                    playbackPosition = exoPlayer.currentPosition.takeIf { it > 0 } ?: playbackPosition
                    chooseTranscodeMode(1)
                    android.widget.Toast.makeText(context, "This phone can't play the original. Transcoding it.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else {
                    val message = if (decoderProblem && isLocal)
                        "This phone can't play this download. Download a compressed copy at a lower resolution."
                    else "Playback failed: ${error.errorCodeName}"
                    android.widget.Toast.makeText(context, message, android.widget.Toast.LENGTH_LONG).show()
                }
            }

            override fun onPlaybackStateChanged(state: Int) {
                isBuffering = state == Player.STATE_BUFFERING
                if (state == Player.STATE_ENDED && !isSmbSource) {
                    ended = true
                    viewModel.reportPlayback(path, playbackPosition, paused = true, ended = true, discTitle = discTitle)
                }
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
            // Where the user stopped, so it resumes from there (the end was already reported).
            if (!isSmbSource && !ended && exoPlayer.mediaItemCount > 0) {
                val position = if (enableTranscode) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
                viewModel.reportPlayback(path, position, paused = true, discTitle = discTitle)
            }
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
            val newUrl = "$baseUrlWithPath&auto-transcode=0&transcode=1&start=$startTimeSeconds$discQuery" +
                transcodeQuery(transcodeModes[transcodeModeIndex])
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

    // Where the player is, for the server's admin Streams page: every few seconds, and on
    // play/pause (isPlaying restarts the effect).
    LaunchedEffect(exoPlayer, path, isPlaying) {
        if (isSmbSource) return@LaunchedEffect
        while (!ended) { // the end was reported, and a report after it would undo "watched"
            // Downloads played from the phone are recorded locally and synced later.
            viewModel.reportPlayback(path, playbackPosition, !exoPlayer.isPlaying, discTitle = discTitle)
            delay(5000)
        }
    }

    LaunchedEffect(showControls, showSpeedDialog, showTranscodeMenu, showSubtitleMenu, showSubtitleSearch, isTrickplayDragged, isTrickplayPressed, trickplayListState.isScrollInProgress, isSliderDragged, isSliderPressed, lastInteractionTime) {
        // Not while the transcode menu is open: it closes with the controls.
        if (showControls && !showSpeedDialog && !showTranscodeMenu && !showSubtitleMenu && !showSubtitleSearch && !isTrickplayDragged && !isTrickplayPressed && !trickplayListState.isScrollInProgress && !isSliderDragged && !isSliderPressed) {
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

        SubtitleOverlay(serverSubs, position = {
            if (transcodeStartOffset > 0) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
        }, fontSize = 18.sp, bottomPadding = if (showControls) 160.dp else 32.dp)

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
                    
                    // Disc: chapters, angles, tracks, titles
                    if (isDisc) {
                        IconButton(onClick = {
                            lastInteractionTime = System.currentTimeMillis()
                            showDiscSheet = true
                        }) {
                            Icon(Icons.Default.Album, contentDescription = "Disc", tint = Color.White)
                        }
                    }

                    // Transcoding: the TV's list of modes (discs can't play as they are)
                    if (!isSmbSource && !isLocal) {
                        Box {
                            IconButton(onClick = {
                                lastInteractionTime = System.currentTimeMillis()
                                showTranscodeMenu = true
                            }) {
                                Icon(
                                    Icons.Default.VideoSettings,
                                    contentDescription = "Transcode",
                                    tint = if (enableTranscode) Color.Green else Color.White
                                )
                            }
                            DropdownMenu(expanded = showTranscodeMenu, onDismissRequest = { showTranscodeMenu = false }) {
                                transcodeModes.forEachIndexed { index, mode ->
                                    val unavailable = mode.isOriginal && (isDisc || tooBigToDecode)
                                    DropdownMenuItem(
                                        text = {
                                            Text(if (mode.isOriginal && tooBigToDecode) "Original (too big for this phone)" else mode.name)
                                        },
                                        enabled = !unavailable,
                                        trailingIcon = if (index == transcodeModeIndex) {
                                            { Icon(Icons.Default.Check, contentDescription = "Selected") }
                                        } else null,
                                        onClick = {
                                            showTranscodeMenu = false
                                            lastInteractionTime = System.currentTimeMillis()
                                            if (index != transcodeModeIndex) chooseTranscodeMode(index)
                                        }
                                    )
                                }
                            }
                        }
                    }
                    
                    // Subtitles: the server's tracks, and searching OpenSubtitles through it
                    serverSubs?.let { subs ->
                        Box {
                            IconButton(onClick = {
                                lastInteractionTime = System.currentTimeMillis()
                                showSubtitleMenu = true
                            }) {
                                Icon(Icons.Default.ClosedCaption, contentDescription = "Subtitles",
                                    tint = if (subs.selected >= 0) Color.Green else Color.White)
                            }
                            DropdownMenu(expanded = showSubtitleMenu, onDismissRequest = { showSubtitleMenu = false }) {
                                val labels = subs.labels
                                (listOf("Off") + labels).forEachIndexed { i, label ->
                                    val index = i - 1
                                    DropdownMenuItem(
                                        text = { Text(label) },
                                        trailingIcon = if (index == subs.selected) {
                                            { Icon(Icons.Default.Check, contentDescription = "Selected") }
                                        } else null,
                                        onClick = {
                                            showSubtitleMenu = false
                                            lastInteractionTime = System.currentTimeMillis()
                                            chooseServerSubtitle(index)
                                            SubtitleMemory.set(context, subtitleVideo, index, labels.getOrNull(index))
                                        }
                                    )
                                }
                                HorizontalDivider()
                                DropdownMenuItem(
                                    text = { Text("Search online…") },
                                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                                    onClick = { showSubtitleMenu = false; showSubtitleSearch = true }
                                )
                            }
                        }
                    }

                    // Rotation
                    IconButton(onClick = { 
                        lastInteractionTime = System.currentTimeMillis()
                        rotation = (rotation + 90f) % 360f 
                    }) {
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
                        IconButton(onClick = { 
                            lastInteractionTime = System.currentTimeMillis()
                            seekPlayer((playbackPosition - 10000).coerceAtLeast(0)) 
                        }) {
                            Icon(
                                Icons.Default.Replay10,
                                contentDescription = "-10s",
                                tint = Color.White,
                                modifier = Modifier.size(48.dp)
                            )
                        }
                        IconButton(onClick = { 
                            lastInteractionTime = System.currentTimeMillis()
                            if (isPlaying) exoPlayer.pause() else exoPlayer.play() 
                        }) {
                            Icon(
                                if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                                contentDescription = "Play/Pause",
                                tint = Color.White,
                                modifier = Modifier.size(64.dp)
                            )
                        }
                        IconButton(onClick = { 
                            lastInteractionTime = System.currentTimeMillis()
                            seekPlayer((playbackPosition + 30000).coerceAtMost(duration)) 
                        }) {
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
                                lastInteractionTime = System.currentTimeMillis()
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
                                lastInteractionTime = System.currentTimeMillis()
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
                                            lastInteractionTime = System.currentTimeMillis()
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
                                (currentTime - lastInteractionTime) > interactionTimeout) {
                                
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
                        TextButton(shape = AppButtonShape, onClick = {
                            lastInteractionTime = System.currentTimeMillis()
                            currentAspectRatioIndex = (currentAspectRatioIndex + 1) % aspectRatios.size
                            currentAspectRatio = aspectRatioValues[currentAspectRatioIndex]
                        }) {
                            Icon(Icons.Default.AspectRatio, contentDescription = null, tint = Color.White, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(4.dp))
                            Text(aspectRatios[currentAspectRatioIndex], color = Color.White, style = MaterialTheme.typography.labelMedium)
                        }
                        
                        Spacer(Modifier.width(16.dp))
                        
                        // Speed button
                        TextButton(shape = AppButtonShape, onClick = {
                            lastInteractionTime = System.currentTimeMillis()
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
                        interactionSource = sliderInteractionSource,
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
                            TextButton(shape = AppButtonShape, onClick = { showSpeedDialog = false }) {
                                Text("Cancel")
                            }
                        }
                    }
                }
            }
        }
    }

    if (showDiscSheet && discInfo != null) {
        PhoneDiscSheet(
            info = discInfo!!,
            titleNumber = discTitle,
            angle = discAngle,
            audio = discAudio,
            subtitle = discSubtitle,
            positionMs = playbackPosition,
            onDismiss = { showDiscSheet = false },
            onTitle = { n ->
                showDiscSheet = false
                if (n != discInfo?.title(discTitle)?.number) {
                    discInfo?.title(n)?.let { fullDuration = (it.duration * 1000).toLong(); duration = fullDuration }
                    discRestartAt = 0L
                    discAngle = 1; discAudio = 0; discSubtitle = -1
                    discTitle = n
                }
            },
            onChapter = { ms -> showDiscSheet = false; seekPlayer(ms) },
            onAngle = { a -> showDiscSheet = false; discAngle = a },
            onAudio = { a -> showDiscSheet = false; discAudio = a },
            onSubtitle = { sub -> showDiscSheet = false; discSubtitle = sub }
        )
    }

    if (showSubtitleSearch && serverSubs != null) {
        PhoneSubtitleSearchDialog(serverSubs, onDismiss = { showSubtitleSearch = false }, onChosen = { index ->
            showSubtitleSearch = false
            // The download selected itself; an original can't show a picture track.
            if (serverSubs.burnIn >= 0 && !enableTranscode) chooseTranscodeMode(1)
            SubtitleMemory.set(context, subtitleVideo, index, serverSubs.labels.getOrNull(index))
        })
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

/** A DVD's chapters, angles, audio and subtitle tracks, and other titles; picking one restarts the stream there. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PhoneDiscSheet(
    info: DiscInfo,
    titleNumber: Int,
    angle: Int,
    audio: Int,
    subtitle: Int,
    positionMs: Long,
    onDismiss: () -> Unit,
    onTitle: (Int) -> Unit,
    onChapter: (Long) -> Unit,
    onAngle: (Int) -> Unit,
    onAudio: (Int) -> Unit,
    onSubtitle: (Int) -> Unit
) {
    val title = info.title(titleNumber) ?: return
    val positionSec = positionMs / 1000.0
    val currentChapter = title.chapters.indexOfLast { positionSec + 0.5 >= it }.coerceAtLeast(0)
    fun clock(seconds: Double): String {
        val s = seconds.toLong()
        return if (s >= 3600) String.format(Locale.US, "%d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
        else String.format(Locale.US, "%d:%02d", s / 60, s % 60)
    }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        LazyColumn(contentPadding = PaddingValues(bottom = 24.dp)) {
            fun section(name: String) = item(key = "h:$name") {
                Text(name, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.padding(start = 20.dp, top = 14.dp, bottom = 4.dp))
            }
            fun choice(key: String, label: String, selected: Boolean, onClick: () -> Unit) = item(key = key) {
                ListItem(
                    headlineContent = { Text(label) },
                    leadingContent = { RadioButton(selected = selected, onClick = onClick) },
                    modifier = Modifier.clickable(onClick = onClick)
                )
            }
            if (title.chapters.size > 1) {
                section("Chapters")
                title.chapters.forEachIndexed { i, start ->
                    choice("c$i", "${i + 1}   ${clock(start)}", i == currentChapter) { onChapter((start * 1000).toLong()) }
                }
            }
            if (title.angles > 1) {
                section("Angle")
                (1..title.angles).forEach { a -> choice("a$a", "Angle $a of ${title.angles}", a == angle) { onAngle(a) } }
            }
            if (title.audio.size > 1) {
                section("Audio")
                title.audio.forEach { t -> choice("au${t.index}", "${t.index + 1}   ${t.label}", t.index == audio) { onAudio(t.index) } }
            }
            section("Subtitles")
            choice("s-off", "Off", subtitle < 0) { onSubtitle(-1) }
            title.subtitles.forEach { t -> choice("s${t.index}", "${t.index + 1}   ${t.label}", t.index == subtitle) { onSubtitle(t.index) } }
            if (info.titles.size > 1) {
                section("Titles")
                info.titles.sortedBy { it.number }.forEach { t ->
                    val main = if (t.number == info.mainTitle) "  (main)" else ""
                    choice("t${t.number}", "Title ${t.number}   ${clock(t.duration)}$main", t.number == title.number) { onTitle(t.number) }
                }
            }
        }
    }
}

/** Subtitles found online (OpenSubtitles, through the server); choosing one downloads it there. */
@Composable
private fun PhoneSubtitleSearchDialog(subs: ServerSubtitles, onDismiss: () -> Unit, onChosen: (Int) -> Unit) {
    val scope = rememberCoroutineScope()
    var results by remember { mutableStateOf<List<com.vargon.weirdstreamer.api.models.SubtitleHit>>(emptyList()) }
    var note by remember { mutableStateOf<String?>("Searching…") }
    var busy by remember { mutableStateOf(true) }
    LaunchedEffect(subs) {
        try {
            val found = subs.search(deviceSubtitleLanguages())
            results = found.results
            val q = found.query
            val what = listOfNotNull(q?.title, q?.year?.takeIf { it > 0 }?.toString(),
                if ((q?.season ?: 0) > 0 || (q?.episode ?: 0) > 0) "S%02dE%02d".format(q?.season, q?.episode) else null).joinToString(" ")
            note = if (found.results.isEmpty()) "Nothing found for $what in ${q?.languages?.joinToString(", ")}."
                else "${found.results.size} found for $what${if (q?.tmdb == true) " (by its TMDB match)" else ""}."
        } catch (e: Exception) {
            note = e.message ?: "Search failed"
        } finally {
            busy = false
        }
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Subtitles online") },
        text = {
            Column {
                note?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                androidx.compose.foundation.lazy.LazyColumn(Modifier.padding(top = 8.dp).heightIn(max = 420.dp)) {
                    items(results.size) { i ->
                        val hit = results[i]
                        ListItem(
                            headlineContent = { Text(hit.release, maxLines = 2, overflow = TextOverflow.Ellipsis) },
                            supportingContent = { Text(subtitleHitDetails(hit)) },
                            modifier = Modifier.clickable(enabled = !busy) {
                                busy = true
                                note = if (hit.downloaded) "Loading…" else "Downloading…"
                                scope.launch {
                                    try {
                                        onChosen(subs.download(hit))
                                    } catch (e: Exception) {
                                        note = e.message ?: "Download failed"
                                        busy = false
                                    }
                                }
                            }
                        )
                    }
                }
            }
        },
        confirmButton = { TextButton(shape = AppButtonShape, onClick = onDismiss) { Text("Close") } }
    )
}
