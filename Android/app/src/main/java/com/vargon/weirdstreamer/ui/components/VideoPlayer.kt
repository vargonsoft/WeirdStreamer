package com.vargon.weirdstreamer.ui.components

import android.util.Log
import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.activity.compose.BackHandler
import androidx.annotation.OptIn
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.clickable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Api
import androidx.compose.material.icons.filled.FastForward
import androidx.compose.material.icons.filled.FastRewind
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.VideoSettings
import androidx.compose.material.icons.filled.ZoomIn
import androidx.compose.material.icons.filled.ZoomOut
import androidx.compose.material.icons.filled.AspectRatio
import androidx.compose.material.icons.filled.Subtitles
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.ui.Alignment
import androidx.compose.ui.BiasAlignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.*
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.tv.material3.Button
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.Icon
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.OpenSubtitlesClient
import com.vargon.weirdstreamer.SubtitleInfo
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.io.File
import java.util.Locale
import kotlin.math.abs

@OptIn(UnstableApi::class)
@Composable
fun VideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current

    // Determine if this is an SMB or Server source
    val isSmbSource = remember(path) { path.startsWith("smb://") }

    // Transcode state - only applicable for server sources
    var enableTranscode by remember { mutableStateOf(false) }

    // Track the start offset for transcoded streams
    var transcodeStartOffset by remember { mutableLongStateOf(0L) }

    // Store the original full duration (before any transcoding)
    var fullDuration by remember { mutableLongStateOf(0L) }

    // Build the actual video URL with transcode parameter if needed
    val videoUrl = remember(path, enableTranscode) {
        // Reset offset when transcoding is toggled
        transcodeStartOffset = 0L

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
                            .setDataSourceFactory(
                                SmbDataSourceFactory(
                                    viewModel.credentialManager,
                                    viewModel.baseContext
                                )
                            )
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

    // Zoom state for video player
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    var showZoomToolbar by remember { mutableStateOf(false) }

    // Aspect ratio cycling
    val aspectRatios = listOf("Original", "16:9", "21:9", "4:3", "5:4", "1:1")
    val aspectRatioValues = listOf(null, 16f / 9f, 21f / 9f, 4f / 3f, 5f / 4f, 1f)
    var currentAspectRatioIndex by remember { mutableIntStateOf(0) }
    var currentAspectRatio by remember { mutableStateOf<Float?>(null) }
    var toolbarFocusedButton by remember { mutableIntStateOf(0) } // 0 = Zoom, 1 = Zoom+, 2 = Zoom-, 3 = AspectRatio, 4 = Subtitles
    var videoAspectRatio by remember { mutableFloatStateOf(16f / 9f) } // Track actual video aspect ratio
    var videoPaddingPx by remember { mutableIntStateOf(0) } // Size adjustment in pixels (1px in each direction)
    var showSubtitleDialog by remember { mutableStateOf(false) }
    var availableSubtitles by remember { mutableStateOf<List<String>>(emptyList()) }
    var selectedSubtitleIndex by remember { mutableIntStateOf(-1) } // -1 = no subtitles

    // OpenSubtitles download state
    var showDownloadDialog by remember { mutableStateOf(false) }
    var downloadingSubtitles by remember {
        mutableStateOf<List<SubtitleInfo>>(
            emptyList()
        )
    }
    var isSearchingSubtitles by remember { mutableStateOf(false) }
    var downloadStatus by remember { mutableStateOf<String?>(null) }

    val openSubtitlesClient = remember { OpenSubtitlesClient(context) }

    fun selectSubtitleTrack(index: Int) {
        selectedSubtitleIndex = index

        // Find the text track at the given index
        val tracks = exoPlayer.currentTracks
        var subtitleCount = 0

        for (group in tracks.groups) {
            if (group.type == C.TRACK_TYPE_TEXT) {
                if (index == -1) {
                    // Disable subtitles
                    exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters
                            .buildUpon()
                            .clearOverrides()
                            .build()
                    return
                }

                for (i in 0 until group.length) {
                    if (subtitleCount == index) {
                        // Select this group
                        exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters
                                .buildUpon()
                                .setOverrideForType(
                                    TrackSelectionOverride(
                                        group.mediaTrackGroup,
                                        i
                                    )
                                )
                                .build()
                        return
                    }
                    subtitleCount++
                }
            }
        }
    }

    fun searchAndDownloadSubtitles() {
        if (!openSubtitlesClient.hasApiKey()) {
            downloadStatus =
                "OpenSubtitles API key not configured. Please set up API key in Settings."
            return
        }

        isSearchingSubtitles = true
        downloadStatus = "Searching subtitles..."

        // Run search in background
        Thread {
            try {
                // Extract filename from path
                val filename = path.substringAfterLast("/").substringBefore("?")

                // Search by filename first
                val results = openSubtitlesClient.searchByFilename(filename)

                if (results.isNotEmpty()) {
                    downloadingSubtitles = results.take(10) // Show top 10 results
                    downloadStatus = null
                    isSearchingSubtitles = false
                } else {
                    downloadStatus = "No subtitles found"
                    isSearchingSubtitles = false
                }
            } catch (e: Exception) {
                Log.e("VideoPlayer", "Error searching subtitles", e)
                downloadStatus = "Error: ${e.message}"
                isSearchingSubtitles = false
            }
        }.start()
    }

    fun downloadAndApplySubtitle(subtitle: SubtitleInfo) {
        downloadStatus = "Downloading ${subtitle.filename}..."

        Thread {
            try {
                val cacheDir = openSubtitlesClient.getSubtitleCacheDir()
                val outputFile = File(cacheDir, subtitle.filename)

                if (openSubtitlesClient.downloadSubtitle(subtitle, outputFile)) {
                    downloadStatus = "Downloaded successfully"
                    // TODO: Load subtitle file into ExoPlayer
                    // This requires loading the subtitle through MediaItem.SubtitleConfiguration
                } else {
                    downloadStatus = "Download failed"
                }
            } catch (e: Exception) {
                Log.e("VideoPlayer", "Error downloading subtitle", e)
                downloadStatus = "Error: ${e.message}"
            }
        }.start()
    }

    // Fetch metadata for server sources to get accurate duration
    LaunchedEffect(path) {
        if (!isSmbSource) {
            try {
                // Extract the base64 path parameter
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
                        // Convert duration from seconds to milliseconds
                        metadata.duration?.let { durationSecs ->
                            fullDuration = (durationSecs * 1000).toLong()
                            Log.d(
                                "VideoPlayer",
                                "Loaded metadata: duration=${durationSecs}s ($fullDuration ms)"
                            )
                        }
                    }
                }
            } catch (e: Exception) {
                Log.e("VideoPlayer", "Failed to load metadata", e)
            }
        }
    }
    var isSeekingSlider by remember { mutableStateOf(false) }
    var isFocusedOnTranscode by remember { mutableStateOf(false) }
    var playbackPosition by remember { mutableLongStateOf(0L) }
    var duration by remember { mutableLongStateOf(0L) }
    var skipFeedback by remember { mutableStateOf<Pair<Int, Boolean>?>(null) } // value to show, isForward
    var speedFeedback by remember { mutableStateOf<Float?>(null) }

    // --- Skip sequence state ---
    var skipIndex by remember { mutableIntStateOf(0) }
    var lastSkipDirection by remember { mutableStateOf<Int?>(null) } // -1 for left, 1 for right
    var lastSkipTime by remember { mutableLongStateOf(0L) }
    var lastSkipAmount by remember { mutableIntStateOf(0) }
    var skipReversalStep by remember { mutableIntStateOf(0) } // 0: normal, 1: undo, 2: reset
    var basePlaybackPositionBeforeSkip by remember { mutableLongStateOf(0L) }

    val skipIntervals = listOf(5, 15, 30, 60, 120, 300, 600, 1800) // seconds
    val speedIntervals = listOf(1f, 2f, 4f, 6f, 16f, 32f, 64f)
    var currentSpeedIndex by remember { mutableIntStateOf(0) } // 0 is 1f

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
                    val currentDuration = exoPlayer.duration
                    if (fullDuration > 0) {
                        // Use the metadata duration if available
                        duration = fullDuration
                    } else if (enableTranscode) {
                        // Fallback: calculate from transcode offset
                        val calculatedFullDuration = currentDuration + transcodeStartOffset
                        if (calculatedFullDuration > fullDuration) {
                            fullDuration = calculatedFullDuration
                        }
                        duration = fullDuration
                    } else {
                        // Use actual stream duration
                        fullDuration = currentDuration
                        duration = currentDuration
                    }
                } else if (state == Player.STATE_ENDED) {
                    val videos = viewModel.getVideosInCurrentFolder()
                    val currentIndex = videos.indexOf(path)
                    if (currentIndex != -1 && currentIndex < videos.size - 1) {
                        val nextPath = videos[currentIndex + 1]
                        viewModel.playVideo(nextPath)
                    } else {
                        // Replay current video
                        exoPlayer.seekTo(0)
                        exoPlayer.play()
                    }
                }
                isBuffering = state == Player.STATE_BUFFERING || state == Player.STATE_IDLE
                // Update video aspect ratio from player
                val videoSize = exoPlayer.videoSize
                if (videoSize.width > 0 && videoSize.height > 0) {
                    videoAspectRatio = videoSize.width.toFloat() / videoSize.height.toFloat()
                }
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            exoPlayer.release()
        }
    }

    // Progress updates
    LaunchedEffect(exoPlayer, transcodeStartOffset) {
        while (true) {
            playbackPosition = if (enableTranscode) {
                exoPlayer.currentPosition + transcodeStartOffset
            } else {
                exoPlayer.currentPosition
            }
            delay(500)
        }
    }

    // Auto-hide controls
    LaunchedEffect(showControls, lastInteractionTime, isZoomMode) {
        if (showControls && !isZoomMode) {
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

    // Hide speed feedback
    LaunchedEffect(speedFeedback) {
        if (speedFeedback != null) {
            delay(2000)
            speedFeedback = null
        }
    }

    val updateInteraction = {
        lastInteractionTime = System.currentTimeMillis()
        if (!showControls && !isZoomMode) showControls = true
    }

    fun calculatePanLimits(screenWidth: Float, screenHeight: Float): Pair<Float, Float> {
        // Assume 16:9 aspect ratio for video if we don't have exact dimensions
        val videoRatio = 16f / 9f
        val screenRatio = screenWidth / screenHeight

        val fittedWidth: Float
        val fittedHeight: Float
        if (videoRatio > screenRatio) {
            fittedWidth = screenWidth
            fittedHeight = screenWidth / videoRatio
        } else {
            fittedWidth = screenHeight * videoRatio
            fittedHeight = screenHeight
        }

        val scaledWidth = fittedWidth * zoomLevel
        val scaledHeight = fittedHeight * zoomLevel

        val maxPX = (scaledWidth - screenWidth).coerceAtLeast(0f) / 2f
        val maxPY = (scaledHeight - screenHeight).coerceAtLeast(0f) / 2f

        return maxPX to maxPY
    }

    fun performSkip(amount: Int, showControlsAfter: Boolean, useAbsoluteAmount: Boolean = false) {
        if (amount == 0 && !useAbsoluteAmount) {
            skipFeedback = null
            return
        }

        val currentDuration = exoPlayer.duration
        val maxPos = if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration

        if (!isSmbSource && enableTranscode) {
            val actualPosition =
                if (useAbsoluteAmount) basePlaybackPositionBeforeSkip else (exoPlayer.currentPosition + transcodeStartOffset)
            val maxTranscodePos =
                if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration + transcodeStartOffset
            val finalTarget = (actualPosition + amount * 1000).coerceIn(0L, maxTranscodePos)

            val startTimeSeconds = finalTarget / 1000f
            val baseUrlWithPath = if (path.contains("?")) {
                val base = path.substringBefore("?")
                val pathParam = path.substringAfter("path=").substringBefore("&")
                "$base?path=$pathParam"
            } else {
                path
            }
            val newUrl = "$baseUrlWithPath&auto-transcode=0&transcode=1&start=$startTimeSeconds"
            Log.d(
                "VideoPlayer",
                "Transcode seek to $startTimeSeconds sec, URL: $newUrl"
            )
            transcodeStartOffset = finalTarget
            val mediaItem = MediaItem.fromUri(newUrl)
            exoPlayer.setMediaItem(mediaItem)
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true

            skipFeedback = amount to (amount > 0)
        } else {
            val startPos =
                if (useAbsoluteAmount) basePlaybackPositionBeforeSkip else exoPlayer.currentPosition
            val newPos = (startPos + amount * 1000).coerceIn(0L, maxPos)
            exoPlayer.seekTo(newPos)
            skipFeedback = amount to (amount > 0)
        }

        if (showControlsAfter) updateInteraction()
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
        if (showZoomToolbar && !isZoomMode) {
            // Hide toolbar if it's showing and not in zoom mode
            showZoomToolbar = false
        } else if (isZoomMode) {
            if (zoomLevel > 1f) {
                val oldZoom = zoomLevel
                zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f
                panX = (panX * (zoomLevel / oldZoom)).coerceIn(-Float.MAX_VALUE, Float.MAX_VALUE)
                panY = (panY * (zoomLevel / oldZoom)).coerceIn(-Float.MAX_VALUE, Float.MAX_VALUE)
            } else {
                // At 1x, exit zoom mode
                isZoomMode = false
                showZoomToolbar = false
                panX = 0f
                panY = 0f
            }
        } else {
            handleBackAction()
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .then(
                if (isZoomMode) Modifier.border(2.dp, Color.White) else Modifier
            )
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown) {
                    when (event.key) {
                        Key.MediaPlayPause -> {
                            updateInteraction()
                            if (isPlaying) exoPlayer.pause() else exoPlayer.play()
                            true
                        }
                        Key.MediaPlay -> {
                            updateInteraction()
                            if (!isPlaying) exoPlayer.play()
                            true
                        }
                        Key.MediaPause -> {
                            updateInteraction()
                            if (isPlaying) exoPlayer.pause()
                            true
                        }
                        Key.MediaFastForward -> {
                            performSkip(30, true)
                            true
                        }
                        Key.MediaRewind -> {
                            performSkip(-30, true)
                            true
                        }
                        Key.MediaNext -> {
                            val videos = viewModel.getVideosInCurrentFolder()
                            val currentIndex = videos.indexOf(path)
                            if (currentIndex != -1 && currentIndex < videos.size - 1) {
                                viewModel.playVideo(videos[currentIndex + 1])
                            }
                            true
                        }
                        Key.MediaPrevious -> {
                            val videos = viewModel.getVideosInCurrentFolder()
                            val currentIndex = videos.indexOf(path)
                            if (currentIndex > 0) {
                                viewModel.playVideo(videos[currentIndex - 1])
                            }
                            true
                        }
                        Key.DirectionUp -> {
                            if (showZoomToolbar && !isZoomMode) {
                                // Hide toolbar and show playback controls
                                showZoomToolbar = false
                                showControls = true
                                updateInteraction()
                                true
                            } else if (isZoomMode && zoomLevel > 1f) {
                                // Pan up in zoom mode (only when actually zoomed)
                                val (_, maxPY) = calculatePanLimits(
                                    1920f,
                                    1080f
                                )  // Standard TV size
                                panY = (panY + 100f).coerceIn(-maxPY, maxPY)
                                updateInteraction()
                                true
                            } else if (isFocusedOnTranscode) {
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
                            if (isZoomMode && zoomLevel > 1f) {
                                // Pan down in zoom mode (only when actually zoomed)
                                val (_, maxPY) = calculatePanLimits(
                                    1920f,
                                    1080f
                                )  // Standard TV size
                                panY = (panY - 100f).coerceIn(-maxPY, maxPY)
                                updateInteraction()
                                true
                            } else if (isZoomMode) {
                                // In zoom mode at 1x, toggle toolbar
                                showZoomToolbar = !showZoomToolbar
                                true
                            } else if (showZoomToolbar) {
                                // Hide toolbar if it's showing
                                showZoomToolbar = false
                                true
                            } else if (isFocusedOnTranscode) {
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
                            } else if (!showControls && !showZoomToolbar && !isZoomMode) {
                                // Show zoom toolbar when controls are hidden and not in zoom mode
                                showZoomToolbar = true
                                isFocusedOnTranscode = false
                                isSeekingSlider = false
                                true
                            } else {
                                false
                            }
                        }

                        Key.DirectionCenter, Key.Enter -> {
                            if (showZoomToolbar && !isZoomMode) {
                                // Toolbar is shown, handle button press
                                when (toolbarFocusedButton) {
                                    0 -> {
                                        // Zoom button pressed
                                        isZoomMode = true
                                        zoomLevel = 1f
                                        panX = 0f
                                        panY = 0f
                                        showZoomToolbar =
                                            false  // Hide toolbar after entering zoom mode
                                    }

                                    1 -> {
                                        // Zoom+ button pressed
                                        videoPaddingPx += 1
                                    }

                                    2 -> {
                                        // Zoom- button pressed
                                        if (videoPaddingPx > 0) {
                                            videoPaddingPx -= 1
                                        }
                                    }

                                    3 -> {
                                        // Aspect ratio button pressed, cycle to next
                                        currentAspectRatioIndex =
                                            (currentAspectRatioIndex + 1) % aspectRatios.size
                                        currentAspectRatio =
                                            aspectRatioValues[currentAspectRatioIndex]
                                    }

                                    4 -> {
                                        // Subtitles button pressed, show subtitle dialog
                                        showSubtitleDialog = true
                                    }
                                }
                                true
                            } else if (isZoomMode) {
                                // In zoom mode, zoom in
                                if (zoomLevel < 64f) {
                                    val oldZoom = zoomLevel
                                    zoomLevel = if (zoomLevel == 1f) 2f else zoomLevel * 2f
                                    // Adjust pan to keep it within bounds
                                    panX = (panX * (zoomLevel / oldZoom)).coerceIn(
                                        -Float.MAX_VALUE,
                                        Float.MAX_VALUE
                                    )
                                    panY = (panY * (zoomLevel / oldZoom)).coerceIn(
                                        -Float.MAX_VALUE,
                                        Float.MAX_VALUE
                                    )
                                }
                                true
                            } else if (isFocusedOnTranscode) {
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
                            if (showZoomToolbar && !isZoomMode) {
                                // Navigate left in toolbar (5 buttons: 0-4)
                                toolbarFocusedButton = (toolbarFocusedButton - 1).coerceAtLeast(0)
                                true
                            } else if (isZoomMode && zoomLevel > 1f) {
                                // Pan left in zoom mode (only when actually zoomed)
                                val (maxPX, _) = calculatePanLimits(
                                    1920f,
                                    1080f
                                )  // Standard TV size
                                panX = (panX + 100f).coerceIn(-maxPX, maxPX)
                                updateInteraction()
                                true
                            } else if (isFocusedOnTranscode) {
                                // Do nothing on transcode button
                                false
                            } else if (!isPlaying) {
                                // Frame backward
                                exoPlayer.seekTo((exoPlayer.currentPosition - 40L).coerceAtLeast(0L))
                                updateInteraction()
                                true
                            } else if (showControls) {
                                // Fast forward/rewind speed control
                                updateInteraction()
                                isSeekingSlider = true
                                if (currentSpeedIndex > 0) {
                                    currentSpeedIndex = (currentSpeedIndex - 1).coerceAtLeast(0)
                                } else {
                                    // At 1x, perform a normal skip backward
                                    performSkip(-skipIntervals[0], true)
                                }
                                val speed = speedIntervals[currentSpeedIndex]
                                exoPlayer.playbackParameters = PlaybackParameters(speed)
                                speedFeedback = if (speed > 1f) speed else null
                                true
                            } else {
                                val now = System.currentTimeMillis()
                                val direction = -1
                                val withinInterval =
                                    (now - lastSkipTime) <= 1000 && lastSkipDirection == direction && skipReversalStep == 0
                                val opposite = lastSkipDirection == 1 && skipReversalStep == 0

                                if (!withinInterval && !opposite) {
                                    basePlaybackPositionBeforeSkip =
                                        if (enableTranscode && !isSmbSource) {
                                            exoPlayer.currentPosition + transcodeStartOffset
                                        } else {
                                            exoPlayer.currentPosition
                                        }
                                }

                                var skipAmount: Int
                                if (opposite) {
                                    // Reverse direction: first undo last skip, then reset, then start new direction
                                    skipReversalStep++
                                    skipAmount = when (skipReversalStep) {
                                        1 -> 0 // Undo back to base
                                        2 -> 0
                                        else -> {
                                            skipReversalStep = 0
                                            skipIndex = 0
                                            -skipIntervals[skipIndex]
                                        }
                                    }
                                } else {
                                    skipReversalStep = 0
                                    skipIndex = if (withinInterval) (skipIndex + 1).coerceAtMost(
                                        skipIntervals.lastIndex
                                    ) else 0
                                    skipAmount = -skipIntervals[skipIndex]
                                }

                                if (showControls) isSeekingSlider = true
                                performSkip(skipAmount, showControls, useAbsoluteAmount = true)

                                lastSkipAmount = skipAmount
                                lastSkipDirection = direction
                                lastSkipTime = now
                                true
                            }
                        }

                        Key.DirectionRight -> {
                            if (showZoomToolbar && !isZoomMode) {
                                // Navigate right in toolbar (5 buttons: 0-4)
                                toolbarFocusedButton = (toolbarFocusedButton + 1).coerceAtMost(4)
                                true
                            } else if (isZoomMode && zoomLevel > 1f) {
                                // Pan right in zoom mode (only when actually zoomed)
                                val (maxPX, _) = calculatePanLimits(
                                    1920f,
                                    1080f
                                )  // Standard TV size
                                panX = (panX - 100f).coerceIn(-maxPX, maxPX)
                                updateInteraction()
                                true
                            } else if (isFocusedOnTranscode) {
                                // Do nothing on transcode button
                                false
                            } else if (!isPlaying) {
                                // Frame forward
                                exoPlayer.seekTo(exoPlayer.currentPosition + 40L)
                                updateInteraction()
                                true
                            } else if (showControls) {
                                // Fast forward speed control
                                updateInteraction()
                                isSeekingSlider = true
                                currentSpeedIndex = (currentSpeedIndex + 1).coerceAtMost(speedIntervals.lastIndex)
                                val speed = speedIntervals[currentSpeedIndex]
                                exoPlayer.playbackParameters = PlaybackParameters(speed)
                                speedFeedback = if (speed > 1f) speed else null
                                true
                            } else {
                                val now = System.currentTimeMillis()
                                val direction = 1
                                val withinInterval =
                                    (now - lastSkipTime) <= 1000 && lastSkipDirection == direction && skipReversalStep == 0
                                val opposite = lastSkipDirection == -1 && skipReversalStep == 0

                                if (!withinInterval && !opposite) {
                                    basePlaybackPositionBeforeSkip =
                                        if (enableTranscode && !isSmbSource) {
                                            exoPlayer.currentPosition + transcodeStartOffset
                                        } else {
                                            exoPlayer.currentPosition
                                        }
                                }

                                var skipAmount: Int
                                if (opposite) {
                                    // Reverse direction: first undo last skip, then reset, then start new direction
                                    skipReversalStep++
                                    skipAmount = when (skipReversalStep) {
                                        1 -> 0 // Undo back to base
                                        2 -> 0
                                        else -> {
                                            skipReversalStep = 0
                                            skipIndex = 0
                                            skipIntervals[skipIndex]
                                        }
                                    }
                                } else {
                                    skipReversalStep = 0
                                    skipIndex = if (withinInterval) (skipIndex + 1).coerceAtMost(
                                        skipIntervals.lastIndex
                                    ) else 0
                                    skipAmount = skipIntervals[skipIndex]
                                }

                                if (showControls) isSeekingSlider = true
                                performSkip(skipAmount, showControls, useAbsoluteAmount = true)

                                lastSkipAmount = skipAmount
                                lastSkipDirection = direction
                                lastSkipTime = now
                                true
                            }
                        }

                        Key.Backspace -> {
                            if (showZoomToolbar && !isZoomMode) {
                                // Hide toolbar if it's showing and not in zoom mode
                                showZoomToolbar = false
                                true
                            } else if (isZoomMode) {
                                if (zoomLevel > 1f) {
                                    val oldZoom = zoomLevel
                                    zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f
                                    panX = (panX * (zoomLevel / oldZoom)).coerceIn(
                                        -Float.MAX_VALUE,
                                        Float.MAX_VALUE
                                    )
                                    panY = (panY * (zoomLevel / oldZoom)).coerceIn(
                                        -Float.MAX_VALUE,
                                        Float.MAX_VALUE
                                    )
                                } else {
                                    // At 1x, exit zoom mode
                                    isZoomMode = false
                                    showZoomToolbar = false
                                    panX = 0f
                                    panY = 0f
                                }
                                true
                            } else {
                                handleBackAction()
                                true
                            }
                        }

                        else -> false
                    }
                }
                false
            }
            .focusRequester(focusRequester)
            .focusable()
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
                        PlayerView(ctx).apply {
                            player = exoPlayer
                            useController = false
                            // Use FIT mode - it will fit video into container while maintaining video's aspect ratio
                            // We'll then stretch it with transforms to fill the constrained box
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
                        .graphicsLayer(
                            // Size adjustment: videoPaddingPx increases scale (makes video larger)
                            // Each increment adds 0.5% size (can adjust this multiplier as needed)
                            scaleX = (1f + (videoPaddingPx * 0.005f)) * zoomLevel * (if (currentAspectRatio != null && currentAspectRatio!! > videoAspectRatio) {
                                // Selected ratio is wider than video - stretch horizontally
                                (currentAspectRatio!! / videoAspectRatio)
                            } else {
                                1f
                            }),
                            scaleY = (1f + (videoPaddingPx * 0.005f)) * zoomLevel * (if (currentAspectRatio != null && currentAspectRatio!! < videoAspectRatio) {
                                // Selected ratio is taller than video - stretch vertically
                                (videoAspectRatio / currentAspectRatio!!)
                            } else {
                                1f
                            }),
                            translationX = panX,
                            translationY = panY
                        )
                )
            }
        }

        // Loading Spinner
        if (isBuffering) {
            CircularProgressIndicator(
                modifier = Modifier.align(Alignment.Center),
                color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { MaterialTheme.colorScheme.primary }
            )
        }

        // Filename Overlay
        AnimatedVisibility(
            visible = showControls,
            enter = slideInVertically { -it } + fadeIn(),
            exit = slideOutVertically { -it } + fadeOut(),
            modifier = Modifier.align(Alignment.TopStart)
        ) {
            Text(
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
            val feedback = skipFeedback
            val skipIsForward = feedback?.second == true
            AnimatedVisibility(
                visible = feedback != null && feedback.first != 0,
                enter = fadeIn() + scaleIn(),
                exit = fadeOut() + scaleOut(),
                modifier = Modifier.align(if (skipIsForward) Alignment.CenterEnd else Alignment.CenterStart)
            ) {
                val value = feedback?.first ?: 0
                if (value != 0) {
                    val absValue = abs(value)
                    val text = if (absValue >= 60) {
                        val min = absValue / 60
                        val sec = absValue % 60
                        val timeStr = String.format(Locale.US, "%d:%02d", min, sec)
                        if (value > 0) "+$timeStr" else "-$timeStr"
                    } else {
                        if (value > 0) "+$absValue" else "-$absValue"
                    }
                    Text(
                        text = text,
                        style = androidx.tv.material3.MaterialTheme.typography.displayLarge,
                        color = Color.White.copy(alpha = 0.7f),
                        modifier = Modifier.padding(horizontal = 64.dp)
                    )
                }
            }
        }

        // Speed Feedback Overlay
        Box(modifier = Modifier.fillMaxSize()) {
            val speed = speedFeedback
            AnimatedVisibility(
                visible = speed != null,
                enter = fadeIn() + scaleIn(),
                exit = fadeOut() + scaleOut(),
                modifier = Modifier.align(Alignment.Center)
            ) {
                if (speed != null) {
                    Text(
                        text = "${speed}x",
                        style = androidx.tv.material3.MaterialTheme.typography.displayLarge,
                        color = Color.White.copy(alpha = 0.7f),
                        modifier = Modifier.padding(64.dp)
                    )
                }
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
                            shape = ClickableSurfaceDefaults.shape(
                                shape = RoundedCornerShape(8.dp)
                            ),
                            colors = ClickableSurfaceDefaults.colors(
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
                                Icon(
                                    Icons.Default.VideoSettings,
                                    contentDescription = "Toggle Transcode",
                                    tint = if (enableTranscode) Color.Green else Color.White,
                                    modifier = Modifier.size(32.dp)
                                )
                                Spacer(modifier = Modifier.width(8.dp))
                                Text(
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
                    Text(
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

                        val progress =
                            if (duration > 0) playbackPosition.toFloat() / duration else 0f

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

                    Text(
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
                    Icon(
                        Icons.Default.FastRewind,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
                        modifier = Modifier.size(48.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    Icon(
                        if (isPlaying) Icons.Default.Pause else Icons.Default.PlayArrow,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
                        modifier = Modifier.size(64.dp)
                    )
                    Spacer(modifier = Modifier.width(32.dp))
                    Icon(
                        Icons.Default.FastForward,
                        contentDescription = null,
                        tint = if (showControls && !isSeekingSlider && !isFocusedOnTranscode) Color.White else Color.Gray,
                        modifier = Modifier.size(48.dp)
                    )
                }
            }
        }

        // Zoom Toolbar
        AnimatedVisibility(
            visible = !isZoomMode && showZoomToolbar,
            enter = slideInVertically { it } + fadeIn(),
            exit = slideOutVertically { it } + fadeOut(),
            modifier = Modifier.align(Alignment.BottomCenter)
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(Color.Black.copy(alpha = 0.6f))
                    .padding(32.dp),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                // Zoom Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(
                            if (toolbarFocusedButton == 0) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(
                                alpha = 0.3f
                            )
                        )
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    Icon(
                        Icons.Default.Api,
                        contentDescription = "Zoom",
                        tint = if (toolbarFocusedButton == 0) Color.White else Color.White.copy(
                            alpha = 0.7f
                        ),
                        modifier = Modifier.size(if (toolbarFocusedButton == 0) 56.dp else 48.dp)
                    )
                    Spacer(modifier = Modifier.width(12.dp))
                    Text(
                        text = "Zoom",
                        style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                        color = if (toolbarFocusedButton == 0) Color.White else Color.White.copy(
                            alpha = 0.7f
                        )
                    )
                }

                Spacer(modifier = Modifier.width(48.dp))

                // Zoom+ Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(
                            if (toolbarFocusedButton == 1) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(
                                alpha = 0.3f
                            )
                        )
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    Icon(
                        Icons.Default.ZoomIn,
                        contentDescription = "Fill+",
                        tint = if (toolbarFocusedButton == 1) Color.White else Color.White.copy(
                            alpha = 0.7f
                        ),
                        modifier = Modifier.size(if (toolbarFocusedButton == 1) 56.dp else 48.dp)
                    )
                    Spacer(modifier = Modifier.width(12.dp))
                    Text(
                        text = "Fill+",
                        style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                        color = if (toolbarFocusedButton == 1) Color.White else Color.White.copy(
                            alpha = 0.7f
                        )
                    )
                }

                Spacer(modifier = Modifier.width(48.dp))

                // Zoom- Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(
                            if (toolbarFocusedButton == 2) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(
                                alpha = 0.3f
                            )
                        )
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    Icon(
                        Icons.Default.ZoomOut,
                        contentDescription = "Fill-",
                        tint = if (toolbarFocusedButton == 2) Color.White else Color.White.copy(
                            alpha = 0.7f
                        ),
                        modifier = Modifier.size(if (toolbarFocusedButton == 2) 56.dp else 48.dp)
                    )
                    Spacer(modifier = Modifier.width(12.dp))
                    Text(
                        text = "Fill-",
                        style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                        color = if (toolbarFocusedButton == 2) Color.White else Color.White.copy(
                            alpha = 0.7f
                        )
                    )
                }

                Spacer(modifier = Modifier.width(48.dp))

                // Aspect Ratio Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(
                            if (toolbarFocusedButton == 3) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(
                                alpha = 0.3f
                            )
                        )
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    Icon(
                        Icons.Default.AspectRatio,
                        contentDescription = "Aspect Ratio",
                        tint = if (toolbarFocusedButton == 3) Color.White else Color.White.copy(
                            alpha = 0.7f
                        ),
                        modifier = Modifier.size(if (toolbarFocusedButton == 3) 56.dp else 48.dp)
                    )
                    Spacer(modifier = Modifier.width(12.dp))
                    Text(
                        text = aspectRatios[currentAspectRatioIndex],
                        style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                        color = if (toolbarFocusedButton == 3) Color.White else Color.White.copy(
                            alpha = 0.7f
                        )
                    )
                }

                Spacer(modifier = Modifier.width(48.dp))

                // Subtitles Button
                Row(
                    modifier = Modifier
                        .clip(RoundedCornerShape(8.dp))
                        .background(
                            if (toolbarFocusedButton == 4) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(
                                alpha = 0.3f
                            )
                        )
                        .padding(12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.Center
                ) {
                    Icon(
                        Icons.Default.Subtitles,
                        contentDescription = "Subtitles",
                        tint = if (toolbarFocusedButton == 4) Color.White else Color.White.copy(
                            alpha = 0.7f
                        ),
                        modifier = Modifier.size(if (toolbarFocusedButton == 4) 56.dp else 48.dp)
                    )
                    Spacer(modifier = Modifier.width(12.dp))
                    Text(
                        text = "Subtitles",
                        style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                        color = if (toolbarFocusedButton == 4) Color.White else Color.White.copy(
                            alpha = 0.7f
                        )
                    )
                }
            }
        }

        // Subtitle Dialog
        if (showSubtitleDialog) {
            Dialog(
                onDismissRequest = { showSubtitleDialog = false },
                properties = DialogProperties(
                    dismissOnBackPress = true,
                    dismissOnClickOutside = false
                )
            ) {
                Surface(
                    modifier = Modifier
                        .width(500.dp)
                        .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                        .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
                    color = Color.Black.copy(alpha = 0.9f)
                ) {
                    Column(
                        modifier = Modifier.padding(24.dp),
                        verticalArrangement = Arrangement.spacedBy(16.dp)
                    ) {
                        Text(
                            text = "Subtitles",
                            style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                            color = Color.White
                        )

                        // Available Subtitles Section
                        if (availableSubtitles.isNotEmpty()) {
                            Text(
                                text = "Available Subtitles",
                                style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                                color = Color.White.copy(alpha = 0.8f)
                            )

                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .verticalScroll(rememberScrollState()),
                                verticalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                // Option to disable subtitles
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(8.dp))
                                        .background(
                                            if (selectedSubtitleIndex == -1) Color.White.copy(alpha = 0.2f)
                                            else Color.Gray.copy(alpha = 0.2f)
                                        )
                                        .clickable {
                                            selectSubtitleTrack(-1)
                                        }
                                        .padding(12.dp),
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    if (selectedSubtitleIndex == -1) {
                                        Icon(
                                            Icons.Default.PlayArrow,
                                            contentDescription = null,
                                            tint = Color.White,
                                            modifier = Modifier.size(20.dp)
                                        )
                                        Spacer(modifier = Modifier.width(8.dp))
                                    }
                                    Text(
                                        text = "None",
                                        color = Color.White,
                                        style = androidx.tv.material3.MaterialTheme.typography.bodyMedium
                                    )
                                }

                                availableSubtitles.forEachIndexed { index, subtitle ->
                                    Row(
                                        modifier = Modifier
                                            .fillMaxWidth()
                                            .clip(RoundedCornerShape(8.dp))
                                            .background(
                                                if (selectedSubtitleIndex == index) Color.White.copy(
                                                    alpha = 0.2f
                                                )
                                                else Color.Gray.copy(alpha = 0.2f)
                                            )
                                            .clickable { selectSubtitleTrack(index) }
                                            .padding(12.dp),
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        if (selectedSubtitleIndex == index) {
                                            Icon(
                                                Icons.Default.PlayArrow,
                                                contentDescription = null,
                                                tint = Color.White,
                                                modifier = Modifier.size(20.dp)
                                            )
                                            Spacer(modifier = Modifier.width(8.dp))
                                        }
                                        Text(
                                            text = subtitle,
                                            color = Color.White,
                                            style = androidx.tv.material3.MaterialTheme.typography.bodyMedium
                                        )
                                    }
                                }
                            }

                            HorizontalDivider(color = Color.White.copy(alpha = 0.3f), thickness = 1.dp)
                        }

                        // Download from OpenSubtitles Option
                        Text(
                            text = "Download Subtitles",
                            style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                            color = Color.White.copy(alpha = 0.8f)
                        )

                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clip(RoundedCornerShape(8.dp))
                                .background(Color.Blue.copy(alpha = 0.3f))
                                .clickable {
                                    showDownloadDialog = true
                                    searchAndDownloadSubtitles()
                                }
                                .padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            Icon(
                                Icons.Default.PlayArrow,
                                contentDescription = null,
                                tint = Color.White,
                                modifier = Modifier.size(20.dp)
                            )
                            Text(
                                text = "OpenSubtitles.org / .com",
                                color = Color.White,
                                style = androidx.tv.material3.MaterialTheme.typography.bodyMedium
                            )
                        }

                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.End
                        ) {
                            Button(
                                onClick = { showSubtitleDialog = false }
                            ) {
                                Text("Close")
                            }
                        }
                    }
                }
            }
        }
    }

    // Download Subtitles Dialog
    if (showDownloadDialog) {
        Dialog(
            onDismissRequest = {
                showDownloadDialog = false
                downloadingSubtitles = emptyList()
                downloadStatus = null
            },
            properties = DialogProperties(dismissOnBackPress = true, dismissOnClickOutside = false)
        ) {
            Surface(
                modifier = Modifier
                    .width(600.dp)
                    .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                    .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
                color = Color.Black.copy(alpha = 0.9f)
            ) {
                Column(
                    modifier = Modifier.padding(24.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
                ) {
                    Text(
                        text = "Download Subtitles",
                        style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                        color = Color.White
                    )

                    // Show status/loading
                    if (isSearchingSubtitles) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(16.dp),
                            horizontalArrangement = Arrangement.Center
                        ) {
                            CircularProgressIndicator(
                                color = Color.Blue,
                                modifier = Modifier.size(40.dp)
                            )
                            Spacer(modifier = Modifier.width(16.dp))
                            Text(
                                text = "Searching...",
                                color = Color.White,
                                style = androidx.tv.material3.MaterialTheme.typography.bodyMedium
                            )
                        }
                    } else if (!downloadStatus.isNullOrEmpty()) {
                        Text(
                            text = downloadStatus!!,
                            color = if (downloadStatus!!.startsWith("Error")) Color.Red else Color.Yellow,
                            style = androidx.tv.material3.MaterialTheme.typography.bodyMedium
                        )
                    } else if (downloadingSubtitles.isNotEmpty()) {
                        Text(
                            text = "Found ${downloadingSubtitles.size} subtitle(s)",
                            style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                            color = Color.White.copy(alpha = 0.8f)
                        )

                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .heightIn(max = 300.dp)
                                .verticalScroll(rememberScrollState()),
                            verticalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            downloadingSubtitles.forEach { subtitle ->
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(8.dp))
                                        .background(Color.Gray.copy(alpha = 0.2f))
                                        .clickable { downloadAndApplySubtitle(subtitle) }
                                        .padding(12.dp),
                                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                                    verticalAlignment = Alignment.CenterVertically
                                ) {
                                    Column(modifier = Modifier.weight(1f)) {
                                        Text(
                                            text = subtitle.filename,
                                            color = Color.White,
                                            style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                        Text(
                                            text = "${subtitle.language} | Rating: ${
                                                String.format(
                                                    Locale.US,
                                                    "%.1f",
                                                    subtitle.rating.toDouble()
                                                )
                                            } | Downloads: ${subtitle.downloads}",
                                            color = Color.White.copy(alpha = 0.7f),
                                            style = androidx.tv.material3.MaterialTheme.typography.labelSmall
                                        )
                                    }
                                    Icon(
                                        Icons.Default.PlayArrow,
                                        contentDescription = null,
                                        tint = Color.White,
                                        modifier = Modifier.size(20.dp)
                                    )
                                }
                            }
                        }
                    }

                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.End
                    ) {
                        Button(
                            onClick = {
                                showDownloadDialog = false
                                downloadingSubtitles = emptyList()
                                downloadStatus = null
                            }
                        ) {
                            Text("Close")
                        }
                    }
                }
            }
        }
    }

    // Detect subtitle tracks from video
    DisposableEffect(path) {
        // Listen for tracks available
        val listener = object : Player.Listener {
            override fun onTracksChanged(tracks: Tracks) {
                val subtitleTracks = mutableListOf<String>()

                // Find all subtitle/text tracks
                for (group in tracks.groups) {
                    if (group.type == C.TRACK_TYPE_TEXT) {
                        for (i in 0 until group.length) {
                            val format = group.getTrackFormat(i)
                            val language = format.language ?: "Subtitle ${i + 1}"
                            subtitleTracks.add(language)
                        }
                    }
                }

                availableSubtitles = subtitleTracks
                if (subtitleTracks.isEmpty()) {
                    selectedSubtitleIndex = -1
                }
            }
        }

        exoPlayer.addListener(listener)

        // Clean up listener when composable disposes
        onDispose {
            exoPlayer.removeListener(listener)
        }
    }
}

fun formatTime(ms: Long): String {
    val totalSeconds = ms / 1000
    val hours = totalSeconds / 3600
    val minutes = (totalSeconds % 3600) / 60
    val seconds = totalSeconds % 60
    return String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
}
