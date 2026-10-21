package com.vargon.weirdstreamer.ui.components

import android.util.Log
import android.view.LayoutInflater
import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.activity.compose.BackHandler
import androidx.annotation.OptIn
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalContext
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
import androidx.tv.material3.*
import com.vargon.weirdstreamer.api.models.DiscInfo
import com.vargon.weirdstreamer.SmbDataSourceFactory
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.OpenSubtitlesClient
import com.vargon.weirdstreamer.SubtitleInfo
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.delay
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.io.File

@OptIn(UnstableApi::class)
@Composable
fun VideoPlayer(viewModel: MediaViewModel, path: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    // Determine if this is an SMB or Server source
    val isSmbSource = remember(path) { path.startsWith("smb://") }

    // DVD disc images (.iso) are always transcoded by the server, one title at a time, with the
    // chosen angle and tracks; the disc dialog changes them.
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
    var showDiscDialog by remember { mutableStateOf(false) }
    val discQuery = if (isDisc) "&title=$discTitle&angle=$discAngle&audio=$discAudio&subtitle=$discSubtitle" else ""

    // Transcode state
    // A video too big for this box's hardware decoder (4K on a 1080p box) starts transcoded,
    // like a disc; it's decided per video, so the next one starts on its own terms.
    val tooBigToDecode = remember(path) {
        val item = viewModel.getFileItemForPath(path)
        !isSmbSource && !VideoDecoders.canPlay(item?.width, item?.height)
    }
    var selectedTranscodeModeIndex by remember(path) { mutableIntStateOf(if (isDisc || tooBigToDecode) 1 else 0) }
    val currentTranscodeMode = transcodeModes[selectedTranscodeModeIndex]

    // Where the user stopped last time (0 to start over), as the view model says.
    val startAtMs = remember(path) { viewModel.playbackStartMs }
    var resumePending by remember(path) { mutableStateOf(startAtMs > 0) }
    var ended by remember(path) { mutableStateOf(false) }

    // Track the start offset for transcoded streams (discs start their stream at the resume point)
    var transcodeStartOffset by remember(path) { mutableLongStateOf(if (isDisc && !isSmbSource) startAtMs else 0L) }

    // Store the original full duration (before any transcoding)
    var fullDuration by remember { mutableLongStateOf(0L) }

    // Build the actual video URL with transcode parameters
    val videoUrl = remember(path, selectedTranscodeModeIndex, discQuery) {
        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            var url = if (path.contains("?")) "$path&transcode=1" else "$path?transcode=1"
            url += transcodeQuery(currentTranscodeMode) // no taller than the hardware decodes
            url += discQuery
            if (transcodeStartOffset > 0) {
                url += "&start=${transcodeStartOffset / 1000f}"
            }
            url
        } else {
            path
        }
    }

    val exoPlayer = remember(videoUrl) {
        val loadControl = DefaultLoadControl.Builder()
            .setBufferDurationsMs(50_000, 120_000, 5_000, 10_000)
            .build()

        val player = ExoPlayer.Builder(context)
            .setLoadControl(loadControl)
            .apply {
                if (isSmbSource) {
                    setMediaSourceFactory(
                        DefaultMediaSourceFactory(context)
                            .setDataSourceFactory(SmbDataSourceFactory(viewModel.credentialManager, viewModel.baseContext))
                    )
                } else {
                    val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                    val okHttpDataSourceFactory = OkHttpDataSource.Factory(okHttpClient).setUserAgent("WeirdStreamer/1.0")
                    setMediaSourceFactory(DefaultMediaSourceFactory(context).setDataSourceFactory(okHttpDataSourceFactory))
                }
            }
            .build()

        player.apply {
            trackSelectionParameters = subtitlesOffByDefault(trackSelectionParameters)
            val mediaItem = MediaItem.fromUri(videoUrl)
            // A direct stream seeks to the resume point; a transcoded one starts there.
            if (resumePending && currentTranscodeMode.isOriginal) setMediaItem(mediaItem, startAtMs) else setMediaItem(mediaItem)
            resumePending = false
            prepare()
            playWhenReady = true
        }
    }

    var isPlaying by remember { mutableStateOf(true) }
    var isPaused by remember { mutableStateOf(false) }
    var isBuffering by remember { mutableStateOf(true) }
    var showControls by remember { mutableStateOf(false) }
    var lastInteractionTime by remember { mutableLongStateOf(System.currentTimeMillis()) }
    var lastControlsHideTime by remember { mutableLongStateOf(0L) }

    // Trickplay state
    var trickplayInfo by remember { mutableStateOf<TrickplayInfo?>(null) }
    var isTrickplayVisible by remember { mutableStateOf(false) }
    var selectedTrickplayIndex by remember { mutableIntStateOf(-1) }
    var trickplayScrollRequest by remember { mutableIntStateOf(-1) }
    var lastConfirmEventTime by remember { mutableLongStateOf(0L) }

    // Zoom state
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    var showZoomToolbar by remember { mutableStateOf(false) }
    var rotation by remember { mutableFloatStateOf(0f) }

    // Aspect ratio
    val aspectRatios = listOf("Original", "16:9", "21:9", "4:3", "5:4", "1:1")
    val aspectRatioValues = listOf(null, 16f / 9f, 21f / 9f, 4f / 3f, 5f / 4f, 1f)
    var currentAspectRatioIndex by remember { mutableIntStateOf(0) }
    var currentAspectRatio by remember { mutableStateOf<Float?>(null) }
    var toolbarFocusedButton by remember { mutableIntStateOf(0) }
    var videoAspectRatio by remember { mutableFloatStateOf(16f / 9f) }
    var videoPaddingPx by remember { mutableIntStateOf(0) }
    
    // Dialog states
    var showSubtitleDialog by remember { mutableStateOf(false) }
    var showSpeedDialog by remember { mutableStateOf(false) }
    var showTranscodeDialog by remember { mutableStateOf(false) }
    var showDownloadDialog by remember { mutableStateOf(false) }
    
    var availableSubtitles by remember { mutableStateOf<List<String>>(emptyList()) }
    var selectedSubtitleIndex by remember { mutableIntStateOf(-1) }
    var downloadingSubtitles by remember { mutableStateOf<List<SubtitleInfo>>(emptyList()) }
    var isSearchingSubtitles by remember { mutableStateOf(false) }
    var downloadStatus by remember { mutableStateOf<String?>(null) }

    val openSubtitlesClient = remember { OpenSubtitlesClient(context) }

    // This video's remembered subtitle choice is applied once per player (a transcode switch
    // makes a new player, whose tracks arrive again).
    val subtitleVideo = remember(path) { viewModel.getFileItemForPath(path)?.originalPath ?: path.substringBefore("&") }
    var subtitlesRestoredFor by remember { mutableStateOf<Any?>(null) }

    fun selectSubtitleTrack(index: Int) {
        selectedSubtitleIndex = index
        val tracks = exoPlayer.currentTracks
        var subtitleCount = 0

        for (group in tracks.groups) {
            if (group.type == C.TRACK_TYPE_TEXT) {
                if (index == -1) {
                    // Off: back to no subtitles (forced ones aside), not the file's default track.
                    exoPlayer.trackSelectionParameters = subtitlesOffByDefault(
                        exoPlayer.trackSelectionParameters.buildUpon().clearOverridesOfType(C.TRACK_TYPE_TEXT).build())
                    return
                }

                for (i in 0 until group.length) {
                    if (subtitleCount == index) {
                        exoPlayer.trackSelectionParameters = exoPlayer.trackSelectionParameters.buildUpon()
                                .setOverrideForType(TrackSelectionOverride(group.mediaTrackGroup, i)).build()
                        return
                    }
                    subtitleCount++
                }
            }
        }
    }

    fun searchAndDownloadSubtitles() {
        if (!openSubtitlesClient.hasApiKey()) {
            downloadStatus = "OpenSubtitles API key not configured."
            return
        }
        isSearchingSubtitles = true
        downloadStatus = "Searching subtitles..."
        Thread {
            try {
                val filename = path.substringAfterLast("/").substringBefore("?")
                val results = openSubtitlesClient.searchByFilename(filename)
                if (results.isNotEmpty()) {
                    downloadingSubtitles = results.take(10)
                    downloadStatus = null
                    isSearchingSubtitles = false
                } else {
                    downloadStatus = "No subtitles found"
                    isSearchingSubtitles = false
                }
            } catch (e: Exception) {
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
                } else {
                    downloadStatus = "Download failed"
                }
            } catch (e: Exception) {
                downloadStatus = "Error: ${e.message}"
            }
        }.start()
    }

    // Fetch metadata
    LaunchedEffect(path) {
        if (!isSmbSource) {
            try {
                val encodedPath = path.substringAfter("path=").substringBefore("&")
                val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                val retrofit = Retrofit.Builder().baseUrl(path.substringBefore("/api"))
                    .client(okHttpClient).addConverterFactory(GsonConverterFactory.create()).build()
                val api = retrofit.create(ApiService::class.java)
                val response = api.getMetadata(encodedPath)
                if (response.isSuccessful) {
                    response.body()?.let { metadata ->
                        metadata.duration?.let { fullDuration = (it * 1000).toLong() }
                        if (isDisc) {
                            api.getDisc(encodedPath).body()?.let { disc ->
                                discInfo = disc
                                disc.title(discTitle)?.let { fullDuration = (it.duration * 1000).toLong() }
                            }
                        }
                        viewModel.getFileItemForPath(path)?.let { viewModel.fetchTrickplayInfo(it) }?.let { trickplayInfo = it }
                    }
                }
            } catch (e: Exception) { Log.e("VideoPlayer", "Failed to load metadata", e) }
        }
    }

    var isSeekingSlider by remember { mutableStateOf(false) }
    var playbackPosition by remember(path) { mutableLongStateOf(startAtMs) }
    var duration by remember { mutableLongStateOf(0L) }
    var skipFeedback by remember { mutableStateOf<Pair<Int, Boolean>?>(null) }
    var speedFeedback by remember { mutableStateOf<Float?>(null) }
    var frameFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }

    // Skip sequence state
    var skipIndex by remember { mutableIntStateOf(0) }
    var lastSkipDirection by remember { mutableStateOf<Int?>(null) }
    var lastSkipTime by remember { mutableLongStateOf(0L) }
    var lastSkipAmount by remember { mutableIntStateOf(0) }
    var skipReversalStep by remember { mutableIntStateOf(0) }
    var basePlaybackPositionBeforeSkip by remember { mutableLongStateOf(0L) }

    val skipIntervals = listOf(5, 15, 30, 60, 120, 300, 600, 1800)
    val speedIntervals = listOf(0.1f, 0.25f, 0.5f, 1f, 2f, 4f, 6f, 8f, 10f, 12f, 16f, 32f, 64f)
    var currentSpeedIndex by remember { mutableIntStateOf(speedIntervals.indexOf(1f)) }

    val focusRequester = remember { FocusRequester() }
    val filename = remember(path) { viewModel.getDisplayName(path) }

    // Sync playback state
    DisposableEffect(exoPlayer) {
        val listener = object : Player.Listener {
            override fun onIsPlayingChanged(isPlayingChanged: Boolean) {
                isPlaying = isPlayingChanged
                if (isPlaying) isPaused = false
            }
            // The box can't decode the original (too big, or a codec it lacks): have the server
            // transcode it instead, from the same place.
            override fun onPlayerError(error: androidx.media3.common.PlaybackException) {
                val decoderProblem = error.errorCode in androidx.media3.common.PlaybackException.ERROR_CODE_DECODER_INIT_FAILED..
                    androidx.media3.common.PlaybackException.ERROR_CODE_DECODING_RESOURCES_RECLAIMED
                if (decoderProblem && currentTranscodeMode.isOriginal && !isSmbSource) {
                    transcodeStartOffset = playbackPosition
                    selectedTranscodeModeIndex = 1
                    android.widget.Toast.makeText(context, "This device can't play the original. Transcoding it.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else {
                    android.widget.Toast.makeText(context, "Playback failed: ${error.errorCodeName}",
                        android.widget.Toast.LENGTH_LONG).show()
                }
            }

            override fun onPlaybackStateChanged(state: Int) {
                if (state == Player.STATE_READY) {
                    val currentDuration = exoPlayer.duration
                    if (fullDuration > 0) duration = fullDuration
                    else if (!currentTranscodeMode.isOriginal) {
                        val calculated = currentDuration + transcodeStartOffset
                        if (calculated > fullDuration) fullDuration = calculated
                        duration = fullDuration
                    } else { fullDuration = currentDuration; duration = currentDuration }
                } else if (state == Player.STATE_ENDED) {
                    if (!isSmbSource && !ended) {
                        ended = true
                        viewModel.reportPlayback(path, playbackPosition, paused = true, ended = true, discTitle = discTitle)
                    }
                    val videos = viewModel.getVideosInCurrentFolder()
                    val currentIndex = videos.indexOf(path)
                    if (currentIndex != -1 && currentIndex < videos.size - 1) viewModel.playVideo(videos[currentIndex + 1])
                    else { exoPlayer.seekTo(0); exoPlayer.play() }
                }
                isBuffering = state == Player.STATE_BUFFERING || state == Player.STATE_IDLE
                val videoSize = exoPlayer.videoSize
                if (videoSize.width > 0 && videoSize.height > 0) videoAspectRatio = videoSize.width.toFloat() / videoSize.height.toFloat()
            }
            override fun onTracksChanged(tracks: Tracks) {
                val formats = mutableListOf<androidx.media3.common.Format>()
                for (group in tracks.groups) {
                    if (group.type == C.TRACK_TYPE_TEXT) {
                        for (i in 0 until group.length) formats.add(group.getTrackFormat(i))
                    }
                }
                val subtitleTracks = subtitleLabels(formats)
                availableSubtitles = subtitleTracks
                if (subtitleTracks.isNotEmpty() && subtitlesRestoredFor !== exoPlayer && !isDisc) {
                    subtitlesRestoredFor = exoPlayer
                    val choice = SubtitleMemory.get(context, subtitleVideo)
                    val index = choice?.let { SubtitleMemory.resolve(it, subtitleTracks) } ?: -1
                    if (index >= 0) selectSubtitleTrack(index) else selectedSubtitleIndex = -1
                }
                if (subtitleTracks.isEmpty()) selectedSubtitleIndex = -1
            }
        }
        exoPlayer.addListener(listener)
        onDispose {
            exoPlayer.removeListener(listener)
            // Where the user stopped, so it resumes from there (the end was already reported).
            // (playbackPosition, kept current twice a second, is already absolute for transcodes.)
            if (!isSmbSource && !ended && exoPlayer.playbackState != Player.STATE_IDLE) {
                viewModel.reportPlayback(path, playbackPosition, paused = true, discTitle = discTitle)
            }
            exoPlayer.release()
        }
    }

    LaunchedEffect(exoPlayer, transcodeStartOffset) {
        while (true) {
            playbackPosition = if (!currentTranscodeMode.isOriginal) exoPlayer.currentPosition + transcodeStartOffset
                               else exoPlayer.currentPosition
            delay(500)
        }
    }

    // Where the player is, for the server's admin Streams page: every few seconds, and on
    // play/pause (isPlaying restarts the effect).
    val isPlayingNow = isPlaying
    LaunchedEffect(exoPlayer, path, isPlayingNow) {
        if (isSmbSource) return@LaunchedEffect
        while (!ended) { // a report after the end would undo "watched"
            viewModel.reportPlayback(path, playbackPosition, !exoPlayer.isPlaying, discTitle = discTitle)
            delay(5000)
        }
    }

    LaunchedEffect(showControls, lastInteractionTime, isZoomMode) {
        if (showControls && !isZoomMode) {
            val wait = 3000L - (System.currentTimeMillis() - lastInteractionTime)
            if (wait > 0) delay(wait)
            if (!isTrickplayVisible) {
                showControls = false
                lastControlsHideTime = System.currentTimeMillis()
                isSeekingSlider = false
            }
        }
    }

    LaunchedEffect(skipFeedback) { if (skipFeedback != null) { delay(2000); skipFeedback = null } }
    LaunchedEffect(speedFeedback) { if (speedFeedback != null) { delay(2000); speedFeedback = null } }
    LaunchedEffect(frameFeedback) { if (frameFeedback != null) { delay(500); frameFeedback = null } }

    val updateInteraction = {
        lastInteractionTime = System.currentTimeMillis()
        if (!showControls && !isZoomMode) showControls = true
    }

    fun showTrickplayAtCurrentPosition() {
        val frames = trickplayInfo?.frames ?: return
        selectedTrickplayIndex = frames.indexOfFirst {
            it.time * 1000 >= playbackPosition
        }.coerceAtLeast(0)
        trickplayScrollRequest = selectedTrickplayIndex
        isTrickplayVisible = true
        isSeekingSlider = false
        updateInteraction()
    }

    fun calculatePanLimits(screenWidth: Float, screenHeight: Float): Pair<Float, Float> {
        val videoRatio = 16f / 9f
        val screenRatio = screenWidth / screenHeight
        val fittedWidth = if (videoRatio > screenRatio) screenWidth else screenHeight * videoRatio
        val fittedHeight = if (videoRatio > screenRatio) screenWidth / videoRatio else screenHeight
        val scaledWidth = fittedWidth * zoomLevel
        val scaledHeight = fittedHeight * zoomLevel
        val maxPX = (scaledWidth - screenWidth).coerceAtLeast(0f) / 2f
        val maxPY = (scaledHeight - screenHeight).coerceAtLeast(0f) / 2f
        return maxPX to maxPY
    }

    fun performSkip(amount: Int, showControlsAfter: Boolean, useAbsoluteAmount: Boolean = false) {
        if (amount == 0 && !useAbsoluteAmount) { skipFeedback = null; return }
        val currentDuration = exoPlayer.duration
        val maxPos = if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration

        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            val actual = if (useAbsoluteAmount) basePlaybackPositionBeforeSkip else (exoPlayer.currentPosition + transcodeStartOffset)
            val finalTarget = (actual + amount * 1000).coerceIn(0L, if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration + transcodeStartOffset)
            val startTime = finalTarget / 1000f
            var newUrl = if (path.contains("?")) path.substringBefore("?") + "?path=" + path.substringAfter("path=").substringBefore("&") + "&transcode=1"
                         else "$path?transcode=1"
            currentTranscodeMode.bitrate?.let { newUrl += "&bitrate=$it" }
            currentTranscodeMode.resolution?.let { newUrl += "&resolution=$it" }
            newUrl += discQuery
            newUrl += "&start=$startTime"
            transcodeStartOffset = finalTarget
            exoPlayer.setMediaItem(MediaItem.fromUri(newUrl))
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true
            skipFeedback = amount to (amount > 0)
        } else {
            val start = if (useAbsoluteAmount) basePlaybackPositionBeforeSkip else exoPlayer.currentPosition
            val newPos = (start + amount * 1000).coerceIn(0L, maxPos)
            exoPlayer.seekTo(newPos)
            skipFeedback = amount to (amount > 0)
        }
        if (showControlsAfter) updateInteraction()
    }

    fun seekToTrickplayTarget(target: Long) {
        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            val startTime = target / 1000f
            var newUrl = if (path.contains("?")) {
                path.substringBefore("?") + "?path=" + path.substringAfter("path=").substringBefore("&") + "&transcode=1"
            } else {
                "$path?transcode=1"
            }
            currentTranscodeMode.bitrate?.let { newUrl += "&bitrate=$it" }
            currentTranscodeMode.resolution?.let { newUrl += "&resolution=$it" }
            newUrl += discQuery
            newUrl += "&start=$startTime"
            transcodeStartOffset = target
            exoPlayer.setMediaItem(MediaItem.fromUri(newUrl))
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true
        } else {
            exoPlayer.seekTo(target)
            exoPlayer.play()
        }
        updateInteraction()
    }

    val handleBackAction = {
        val now = System.currentTimeMillis()
        if (isTrickplayVisible) {
            isTrickplayVisible = false
            selectedTrickplayIndex = -1
            trickplayScrollRequest = -1
            updateInteraction()
        }
        else if (showControls) { showControls = false; lastControlsHideTime = now; isSeekingSlider = false }
        else if (now - lastControlsHideTime > 1000) onBack(path)
    }

    BackHandler {
        if (showZoomToolbar && !isZoomMode) showZoomToolbar = false
        else if (isZoomMode) {
            if (zoomLevel > 1f) {
                val old = zoomLevel
                zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f
                panX *= (zoomLevel / old); panY *= (zoomLevel / old)
            } else { isZoomMode = false; showZoomToolbar = false; panX = 0f; panY = 0f }
        } else handleBackAction()
    }

    Box(
        modifier = Modifier.fillMaxSize().background(Color.Black)
            .then(if (isZoomMode) Modifier.border(2.dp, Color.White) else Modifier)
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown) {
                    when (event.key) {
                        Key.MediaPlayPause -> {
                            updateInteraction()
                            if (isPlaying) { exoPlayer.pause(); isPaused = true } else { exoPlayer.play(); isPaused = false }
                            true
                        }
                        Key.MediaPlay -> { updateInteraction(); if (!isPlaying) exoPlayer.play(); true }
                        Key.MediaPause -> { updateInteraction(); if (isPlaying) { exoPlayer.pause(); isPaused = true }; true }
                        Key.MediaFastForward -> { performSkip(30, true); true }
                        Key.MediaRewind -> { performSkip(-30, true); true }
                        Key.MediaNext -> {
                            val videos = viewModel.getVideosInCurrentFolder()
                            val idx = videos.indexOf(path)
                            if (idx != -1 && idx < videos.size - 1) viewModel.playVideo(videos[idx + 1])
                            true
                        }
                        Key.MediaPrevious -> {
                            val videos = viewModel.getVideosInCurrentFolder()
                            val idx = videos.indexOf(path)
                            if (idx > 0) viewModel.playVideo(videos[idx - 1])
                            true
                        }
                        Key.DirectionUp -> {
                            if (showZoomToolbar && !isZoomMode) { showZoomToolbar = false; showControls = true; updateInteraction(); true }
                            else if (isZoomMode && zoomLevel > 1f) {
                                val (_, maxPY) = calculatePanLimits(1920f, 1080f)
                                panY = (panY + 100f).coerceIn(-maxPY, maxPY); updateInteraction(); true
                            }                             else if (isTrickplayVisible) { isTrickplayVisible = false; selectedTrickplayIndex = -1; updateInteraction(); true }
                            else if (isSeekingSlider) {
                                if (trickplayInfo != null) showTrickplayAtCurrentPosition()
                                else updateInteraction()
                                true
                            } else if (showControls && !isSeekingSlider) { isSeekingSlider = true; updateInteraction(); true }
                            else if (trickplayInfo != null) {
                                showTrickplayAtCurrentPosition()
                                true
                            } else { updateInteraction(); false }
                        }
                        Key.DirectionDown -> {
                            if (isZoomMode && zoomLevel > 1f) {
                                val (_, maxPY) = calculatePanLimits(1920f, 1080f)
                                panY = (panY - 100f).coerceIn(-maxPY, maxPY); updateInteraction(); true
                            } else if (isZoomMode) { showZoomToolbar = !showZoomToolbar; true }
                            else if (showZoomToolbar) { showZoomToolbar = false; true }
                            else if (isTrickplayVisible) { isTrickplayVisible = false; selectedTrickplayIndex = -1; isSeekingSlider = true; updateInteraction(); true }
                            else if (isSeekingSlider) { isSeekingSlider = false; updateInteraction(); true }
                            else if (showControls) { showControls = false; lastControlsHideTime = System.currentTimeMillis(); true }
                            else if (!showControls && !showZoomToolbar && !isZoomMode) { showZoomToolbar = true; isSeekingSlider = false; true }
                            else false
                        }
                        Key.DirectionCenter, Key.Enter -> {
                            val now = System.currentTimeMillis()
                            if (now - lastConfirmEventTime < 300L) {
                                true
                            } else {
                                lastConfirmEventTime = now
                            if (isTrickplayVisible) {
                                val frames = trickplayInfo?.frames
                                if (frames != null && selectedTrickplayIndex >= 0 && selectedTrickplayIndex < frames.size) {
                                    val target = frames[selectedTrickplayIndex].time.toLong() * 1000
                                    seekToTrickplayTarget(target)
                                }
                                true
                            } else if (showZoomToolbar && !isZoomMode) {
                                when (toolbarFocusedButton) {
                                    0 -> showTranscodeDialog = true
                                    1 -> { isZoomMode = true; zoomLevel = 1f; panX = 0f; panY = 0f; showZoomToolbar = false }
                                    2 -> showSpeedDialog = true
                                    3 -> videoPaddingPx += 1
                                    4 -> if (videoPaddingPx > 0) videoPaddingPx -= 1
                                    5 -> {
                                        currentAspectRatioIndex = (currentAspectRatioIndex + 1) % aspectRatios.size
                                        currentAspectRatio = aspectRatioValues[currentAspectRatioIndex]
                                    }
                                    6 -> showSubtitleDialog = true
                                    7 -> rotation = (rotation + 90f) % 360f
                                    8 -> showDiscDialog = true
                                }
                                true
                            } else if (isZoomMode) {
                                if (zoomLevel < 64f) {
                                    val old = zoomLevel; zoomLevel = if (zoomLevel == 1f) 2f else zoomLevel * 2f
                                    panX *= (zoomLevel / old); panY *= (zoomLevel / old)
                                }
                                true
                            } else if (isTrickplayVisible) {
                                if (selectedTrickplayIndex != -1) {
                                    val target = trickplayInfo!!.frames[selectedTrickplayIndex].time.toLong() * 1000
                                    seekToTrickplayTarget(target)
                                }
                                true
                            } else {
                                if (showControls) updateInteraction()
                                if (isPlaying) { exoPlayer.pause(); isPaused = true } else { exoPlayer.play(); isPaused = false }
                                true
                            }
                            }
                        }
                        Key.DirectionLeft -> {
                            if (showZoomToolbar && !isZoomMode) { toolbarFocusedButton = (toolbarFocusedButton - 1).coerceAtLeast(0); true }
                            else if (isZoomMode && zoomLevel > 1f) {
                                val (maxPX, _) = calculatePanLimits(1920f, 1080f)
                                panX = (panX + 100f).coerceIn(-maxPX, maxPX); updateInteraction(); true
                            } else if (isTrickplayVisible) {
                                if (selectedTrickplayIndex > 0) {
                                    selectedTrickplayIndex--
                                    trickplayScrollRequest = selectedTrickplayIndex
                                }
                                true
                            }
                            else if (!isPlaying && isPaused) { exoPlayer.seekTo((exoPlayer.currentPosition - 40L).coerceAtLeast(0L)); frameFeedback = false to System.currentTimeMillis(); updateInteraction(); true }
                            else if (showControls) { updateInteraction(); performSkip(-30, true); true }
                            else {
                                val now = System.currentTimeMillis(); val dir = -1
                                val within = (now - lastSkipTime) <= 1000 && lastSkipDirection == dir && skipReversalStep == 0
                                val opp = lastSkipDirection == 1 && skipReversalStep == 0
                                if (!within && !opp) basePlaybackPositionBeforeSkip = if (!currentTranscodeMode.isOriginal && !isSmbSource) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
                                var amount: Int
                                if (opp) {
                                    skipReversalStep++
                                    amount = when (skipReversalStep) { 1, 2 -> 0; else -> { skipReversalStep = 0; skipIndex = 0; -skipIntervals[skipIndex] } }
                                } else { skipReversalStep = 0; skipIndex = if (within) (skipIndex + 1).coerceAtMost(skipIntervals.lastIndex) else 0; amount = -skipIntervals[skipIndex] }
                                if (showControls) isSeekingSlider = true
                                performSkip(amount, showControls, useAbsoluteAmount = true)
                                lastSkipAmount = amount; lastSkipDirection = dir; lastSkipTime = now; true
                            }
                        }
                        Key.DirectionRight -> {
                            if (showZoomToolbar && !isZoomMode) { toolbarFocusedButton = (toolbarFocusedButton + 1).coerceAtMost(if (isDisc) 8 else 7); true }
                            else if (isZoomMode && zoomLevel > 1f) {
                                val (maxPX, _) = calculatePanLimits(1920f, 1080f)
                                panX = (panX - 100f).coerceIn(-maxPX, maxPX); updateInteraction(); true
                            } else if (isTrickplayVisible) {
                                if (selectedTrickplayIndex < trickplayInfo!!.frames.size - 1) {
                                    selectedTrickplayIndex++
                                    trickplayScrollRequest = selectedTrickplayIndex
                                }
                                true
                            }
                            else if (!isPlaying && isPaused) { exoPlayer.seekTo(exoPlayer.currentPosition + 40L); frameFeedback = true to System.currentTimeMillis(); updateInteraction(); true }
                            else if (showControls) { updateInteraction(); performSkip(30, true); true }
                            else {
                                val now = System.currentTimeMillis(); val dir = 1
                                val within = (now - lastSkipTime) <= 1000 && lastSkipDirection == dir && skipReversalStep == 0
                                val opp = lastSkipDirection == -1 && skipReversalStep == 0
                                if (!within && !opp) basePlaybackPositionBeforeSkip = if (!currentTranscodeMode.isOriginal && !isSmbSource) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
                                var amount: Int
                                if (opp) {
                                    skipReversalStep++
                                    amount = when (skipReversalStep) { 1, 2 -> 0; else -> { skipReversalStep = 0; skipIndex = 0; skipIntervals[skipIndex] } }
                                } else { skipReversalStep = 0; skipIndex = if (within) (skipIndex + 1).coerceAtMost(skipIntervals.lastIndex) else 0; amount = skipIntervals[skipIndex] }
                                if (showControls) isSeekingSlider = true
                                performSkip(amount, showControls, useAbsoluteAmount = true)
                                lastSkipAmount = amount; lastSkipDirection = dir; lastSkipTime = now; true
                            }
                        }
                        Key.Backspace -> {
                            if (showZoomToolbar && !isZoomMode) { showZoomToolbar = false; true }
                            else if (isZoomMode) {
                                if (zoomLevel > 1f) {
                                    val old = zoomLevel; zoomLevel = if (zoomLevel <= 2f) 1f else zoomLevel / 2f
                                    panX *= (zoomLevel / old); panY *= (zoomLevel / old)
                                } else { isZoomMode = false; showZoomToolbar = false; panX = 0f; panY = 0f }
                                true
                            } else { handleBackAction(); true }
                        }
                        else -> false
                    }
                }
                false
            }.focusRequester(focusRequester).focusable()
    ) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Box(modifier = Modifier.then(if (currentAspectRatio != null) Modifier.aspectRatio(currentAspectRatio!!) else Modifier.fillMaxSize()), contentAlignment = Alignment.Center) {
                AndroidView(
                    factory = { ctx ->
                        LayoutInflater.from(ctx).inflate(R.layout.texture_video_view, null).let { it as PlayerView }.apply {
                            player = exoPlayer; useController = false; resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                            layoutParams = FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
                        }
                    },
                    update = { view -> view.player = exoPlayer },
                    modifier = Modifier.fillMaxSize().graphicsLayer {
                        rotationZ = rotation
                        val isRotated = (rotation / 90f).toInt() % 2 != 0
                        val rotationScale = if (isRotated && size.width != 0f && size.height != 0f) minOf(size.width / size.height, size.height / size.width) else 1f
                        val stretchX = if (currentAspectRatio != null && currentAspectRatio!! > videoAspectRatio) currentAspectRatio!! / videoAspectRatio else 1f
                        val stretchY = if (currentAspectRatio != null && currentAspectRatio!! <= videoAspectRatio) videoAspectRatio / currentAspectRatio!! else 1f
                        val commonScale = (1f + (videoPaddingPx * 0.005f)) * zoomLevel * rotationScale
                        scaleX = commonScale * stretchX; scaleY = commonScale * stretchY
                        translationX = panX; translationY = panY
                    }
                )
            }
        }

        if (isBuffering) CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { MaterialTheme.colorScheme.primary })

        FilenameOverlay(show = showControls, filename = filename, modifier = Modifier.align(Alignment.TopStart))
        SkipFeedbackOverlay(feedback = skipFeedback)
        SpeedFeedbackOverlay(speed = speedFeedback)
        FrameFeedbackOverlay(feedback = frameFeedback)

        TrickplayOverlay(
            visible = isTrickplayVisible,
            trickplayInfo = trickplayInfo,
            playbackPosition = playbackPosition,
            selectedTrickplayIndex = selectedTrickplayIndex,
            scrollRequestIndex = trickplayScrollRequest,
            updateInteraction = updateInteraction,
            modifier = Modifier.align(Alignment.BottomCenter)
        )

        PlayerControls(
            show = showControls,
            isPlaying = isPlaying,
            playbackPosition = playbackPosition,
            duration = duration,
            isSeekingSlider = isSeekingSlider,
            isTrickplayVisible = isTrickplayVisible,
            modifier = Modifier.align(Alignment.BottomCenter)
        )

        PlayerZoomToolbar(
            visible = showZoomToolbar,
            isZoomMode = isZoomMode,
            toolbarFocusedButton = toolbarFocusedButton,
            currentTranscodeModeName = currentTranscodeMode.name,
            currentAspectRatioName = aspectRatios[currentAspectRatioIndex],
            isSmbSource = isSmbSource,
            isDisc = isDisc,
            onButtonFocused = { toolbarFocusedButton = it },
            onButtonClicked = { id ->
                when (id) {
                    0 -> showTranscodeDialog = true
                    1 -> { isZoomMode = true; zoomLevel = 1f; panX = 0f; panY = 0f; showZoomToolbar = false }
                    2 -> showSpeedDialog = true
                    3 -> videoPaddingPx += 1
                    4 -> if (videoPaddingPx > 0) videoPaddingPx -= 1
                    5 -> { currentAspectRatioIndex = (currentAspectRatioIndex + 1) % aspectRatios.size; currentAspectRatio = aspectRatioValues[currentAspectRatioIndex] }
                    6 -> showSubtitleDialog = true
                    7 -> rotation = (rotation + 90f) % 360f
                    8 -> showDiscDialog = true
                }
            },
            modifier = Modifier.align(Alignment.BottomCenter)
        )

        SpeedDialog(show = showSpeedDialog, currentSpeedIndex = currentSpeedIndex, speedIntervals = speedIntervals, onDismiss = { showSpeedDialog = false }, onSpeedSelected = { idx, speed -> exoPlayer.playbackParameters = PlaybackParameters(speed); speedFeedback = speed; currentSpeedIndex = idx; showSpeedDialog = false })
        TranscodeDialog(show = showTranscodeDialog, selectedModeIndex = selectedTranscodeModeIndex, transcodeModes = transcodeModes, onDismiss = { showTranscodeDialog = false }, onModeSelected = { idx -> if (selectedTranscodeModeIndex != idx && !((isDisc || tooBigToDecode) && transcodeModes[idx].isOriginal)) { transcodeStartOffset = playbackPosition; selectedTranscodeModeIndex = idx }; showTranscodeDialog = false })
        DiscDialog(
            show = showDiscDialog,
            info = discInfo,
            titleNumber = discTitle,
            angle = discAngle,
            audio = discAudio,
            subtitle = discSubtitle,
            positionMs = playbackPosition,
            onTitle = { n ->
                showDiscDialog = false
                if (n != (discInfo?.title(discTitle)?.number ?: n)) {
                    transcodeStartOffset = 0
                    discInfo?.title(n)?.let { fullDuration = (it.duration * 1000).toLong(); duration = fullDuration }
                    discAngle = 1; discAudio = 0; discSubtitle = -1
                    discTitle = n
                }
            },
            onChapter = { ms -> showDiscDialog = false; seekToTrickplayTarget(ms) },
            onAngle = { a -> showDiscDialog = false; if (a != discAngle) { transcodeStartOffset = playbackPosition; discAngle = a } },
            onAudio = { a -> showDiscDialog = false; if (a != discAudio) { transcodeStartOffset = playbackPosition; discAudio = a } },
            onSubtitle = { s -> showDiscDialog = false; if (s != discSubtitle) { transcodeStartOffset = playbackPosition; discSubtitle = s } },
            onDismiss = { showDiscDialog = false }
        )
        SubtitleDialog(show = showSubtitleDialog, availableSubtitles = availableSubtitles, selectedSubtitleIndex = selectedSubtitleIndex, onDismiss = { showSubtitleDialog = false }, onSubtitleSelected = {
            selectSubtitleTrack(it)
            if (!isDisc) SubtitleMemory.set(context, subtitleVideo, it, availableSubtitles.getOrNull(it))
        }, onDownloadRequested = { showDownloadDialog = true; searchAndDownloadSubtitles() })
        DownloadSubtitlesDialog(show = showDownloadDialog, isSearching = isSearchingSubtitles, downloadStatus = downloadStatus, downloadingSubtitles = downloadingSubtitles, onDismiss = { showDownloadDialog = false; downloadingSubtitles = emptyList(); downloadStatus = null }, onSubtitleSelected = { downloadAndApplySubtitle(it) })
    }
}
