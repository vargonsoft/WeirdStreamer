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
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
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
import com.vargon.weirdstreamer.api.models.SubtitleHit
import com.vargon.weirdstreamer.WeirdStreamerApplication
import com.vargon.weirdstreamer.api.ApiService
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import com.vargon.weirdstreamer.api.models.forDiscTitle
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.delay
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.io.File

/** How long after a skip one the other way goes back to where the skipping started. */
private const val SKIP_REVERSAL_WINDOW_MS = 2000L

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

    // Subtitles from the server, as on the web: the video's own tracks, then files beside it and
    // downloads, drawn over the video whatever the stream (SubtitleOverlay). A picture track is
    // burned into a transcode. SMB files and discs keep the player's own tracks.
    val serverSubs = remember(path) { if (isSmbSource || isDisc) null else ServerSubtitles.forStream(path) }
    val burnIn = serverSubs?.burnIn ?: -1
    // Stereo audio in transcodes, once a surround one has failed to decode (AudioFallback)
    var stereoAudio by remember(path) { mutableStateOf(AudioFallback.stereo) }

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

    // How a transcode treats the video and the audio (StreamChoice): copyStartMs is the start
    // asked for when it copies the video, transcodeStartOffset where the stream really begins.
    val choice = remember(path) { if (isSmbSource || isDisc) null else StreamChoice.forStream(path) }
    val copiesVideo = choice?.copiesVideo == true
    val audioStep = choice?.audio
    var copyStartMs by remember(path) { mutableLongStateOf(0L) }
    var restartJob by remember(path) { mutableStateOf<kotlinx.coroutines.Job?>(null) }
    var originalByHand by remember(path) { mutableStateOf(false) } // plays as it is, even silent
    var switching by remember(path) { mutableStateOf(false) }

    fun transcodedUrl(startMs: Long): String {
        var url = path + (if (path.contains("?")) "&" else "?")
        url += choice?.videoQuery(currentTranscodeMode)
            ?: ("transcode=1" + transcodeQuery(currentTranscodeMode)) // no taller than the hardware decodes
        url += discQuery
        if (burnIn >= 0) url += "&subtitle=$burnIn"
        val audio = choice?.audio
        if (audio != null) url += audio.query else if (stereoAudio) url += "&channels=2"
        if (startMs > 0) url += "&start=${StreamChoice.streamStart(startMs)}"
        return url
    }

    // Build the actual video URL with transcode parameters
    val videoUrl = remember(path, selectedTranscodeModeIndex, discQuery, burnIn, stereoAudio, copiesVideo, audioStep) {
        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            transcodedUrl(if (copiesVideo) copyStartMs else transcodeStartOffset)
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
            // The server's subtitles are drawn over the video, so the stream's own stay off.
            trackSelectionParameters = if (serverSubs != null) trackSelectionParameters.buildUpon()
                .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true).build()
            else subtitlesOffByDefault(trackSelectionParameters)
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
    var playbackFailed by remember { mutableStateOf(false) }
    var failureReason by remember { mutableStateOf<String?>(null) }
    var showControls by remember { mutableStateOf(false) }
    var lastInteractionTime by remember { mutableLongStateOf(System.currentTimeMillis()) }
    var lastControlsHideTime by remember { mutableLongStateOf(0L) }

    // Trickplay state
    var loadedTrickplay by remember { mutableStateOf<TrickplayInfo?>(null) }
    // A disc's frames are for its main feature: none while another title plays.
    val trickplayInfo = loadedTrickplay?.forDiscTitle(discTitle, discInfo?.mainTitle)
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
    var showTimingDialog by remember { mutableStateOf(false) }
    var showSpeedDialog by remember { mutableStateOf(false) }
    var showTranscodeDialog by remember { mutableStateOf(false) }
    var showDownloadDialog by remember { mutableStateOf(false) }
    
    var availableSubtitles by remember { mutableStateOf<List<String>>(emptyList()) }
    var selectedSubtitleIndex by remember { mutableIntStateOf(-1) }
    var subtitleResults by remember { mutableStateOf<List<SubtitleHit>>(emptyList()) }
    var subtitleSearchNote by remember { mutableStateOf<String?>(null) }
    var isSearchingSubtitles by remember { mutableStateOf(false) }
    var downloadStatus by remember { mutableStateOf<String?>(null) }

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
                        viewModel.getFileItemForPath(path)?.let { viewModel.fetchTrickplayInfo(it) }?.let { loadedTrickplay = it }
                    }
                }
            } catch (e: Exception) { Log.e("VideoPlayer", "Failed to load metadata", e) }
        }
    }

    var isSeekingSlider by remember { mutableStateOf(false) }
    var playbackPosition by remember(path) { mutableLongStateOf(startAtMs) }

    /** Starts [plan] at [atMs] in mode [index]: all at once, for one new stream. */
    suspend fun startPlan(plan: StreamChoice.Plan?, index: Int, atMs: Long) {
        val begins = if (plan?.copiesVideo == true) choice?.startOf(atMs) ?: atMs else atMs
        copyStartMs = atMs
        transcodeStartOffset = begins
        if (plan != null) choice?.apply(plan)
        selectedTranscodeModeIndex = index
        switching = false
    }

    /** Switches to transcode mode [index] at [atMs]; [failed] is what the original failed on, if it did. */
    fun chooseTranscode(index: Int, atMs: Long, failed: FailedSide? = null) {
        scope.launch {
            val mayCopy = (serverSubs?.burnIn ?: -1) < 0 && !tooBigToDecode
            startPlan(choice?.plan(context, transcodeModes[index], mayCopy, failed), index, atMs)
        }
    }

    // A video too big for the hardware starts transcoded, and its audio can still be kept.
    LaunchedEffect(path) { if (tooBigToDecode && choice != null) chooseTranscode(1, startAtMs) }

    /** Chooses a server subtitle track: a picture one is burned into a transcode, from here. */
    fun chooseServerSubtitle(index: Int, atMs: Long = playbackPosition) {
        val subs = serverSubs ?: return
        val before = subs.burnIn
        subs.select(index)
        if (subs.burnIn == before) return
        if (subs.burnIn >= 0) choice?.stopCopying()
        if (currentTranscodeMode.isOriginal) {
            if (subs.burnIn < 0) return
            transcodeStartOffset = atMs
            chooseTranscode(1, atMs) // Compatibility (Auto)
        } else {
            transcodeStartOffset = atMs
            if (subs.burnIn >= 0) chooseTranscode(selectedTranscodeModeIndex, atMs)
        }
    }

    fun searchSubtitlesOnline() {
        val subs = serverSubs ?: return
        isSearchingSubtitles = true
        downloadStatus = null
        subtitleResults = emptyList()
        scope.launch {
            try {
                val found = subs.search(deviceSubtitleLanguages())
                subtitleResults = found.results
                val q = found.query
                val what = listOfNotNull(q?.title, q?.year?.takeIf { it > 0 }?.toString(),
                    if ((q?.season ?: 0) > 0 || (q?.episode ?: 0) > 0) "S%02dE%02d".format(q?.season, q?.episode) else null).joinToString(" ")
                subtitleSearchNote = if (found.results.isEmpty()) "Nothing found for $what in ${q?.languages?.joinToString(", ")}."
                    else "${found.results.size} found for $what${if (q?.tmdb == true) " (by its TMDB match)" else ""}. Best matches first."
            } catch (e: Exception) {
                downloadStatus = e.message ?: "Search failed"
            } finally {
                isSearchingSubtitles = false
            }
        }
    }

    fun downloadSubtitle(hit: SubtitleHit) {
        val subs = serverSubs ?: return
        downloadStatus = if (hit.downloaded) "Loading…" else "Downloading…"
        scope.launch {
            try {
                val before = subs.burnIn
                val index = subs.download(hit)
                // The download selected itself; a burned-in picture track before it ends here.
                if (before >= 0 && !currentTranscodeMode.isOriginal) transcodeStartOffset = playbackPosition
                SubtitleMemory.set(context, subtitleVideo, index, subs.labels.getOrNull(index))
                showDownloadDialog = false
                subtitleResults = emptyList()
                downloadStatus = null
            } catch (e: Exception) {
                downloadStatus = e.message ?: "Download failed"
            }
        }
    }

    // The server's subtitle tracks, and the one chosen for this video last time (or its forced
    // subtitles, as the web player does).
    LaunchedEffect(serverSubs) {
        val subs = serverSubs ?: return@LaunchedEffect
        subs.load()
        val labels = subs.labels
        val index = SubtitleMemory.get(context, subtitleVideo)?.let { SubtitleMemory.resolve(it, labels) }
            ?: subs.tracks.indexOfFirst { it.forced && it.text }
        if (index >= 0) chooseServerSubtitle(index, if (playbackPosition > 0) playbackPosition else startAtMs)
    }

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
    val playerTitle by produceState<String?>(null, path) { value = viewModel.playerTitleFor(path) }

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
                unreadableReason(error)?.let { reason ->
                    // The server can't read the file: no transcode would help either.
                    playbackFailed = true
                    failureReason = reason
                    android.widget.Toast.makeText(context, reason, android.widget.Toast.LENGTH_LONG).show()
                    return
                }
                val next = if (currentTranscodeMode.isOriginal) null else choice?.nextAfter(failedSide(error))
                if (next != null) {
                    scope.launch {
                        startPlan(next, selectedTranscodeModeIndex, playbackPosition)
                        android.widget.Toast.makeText(context, choice?.status(currentTranscodeMode), android.widget.Toast.LENGTH_SHORT).show()
                    }
                } else if (AudioFallback.isAudioDecoderFailure(error) && !currentTranscodeMode.isOriginal && !isSmbSource && !stereoAudio) {
                    // The transcode's surround audio didn't decode: again from here, in stereo.
                    AudioFallback.stereo = true
                    transcodeStartOffset = playbackPosition
                    stereoAudio = true
                    android.widget.Toast.makeText(context, "This device can't play the surround audio. Switching to stereo.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else if (currentTranscodeMode.isOriginal && !isSmbSource) {
                    // Whatever failed (decoder, container, audio output), the server may manage it.
                    chooseTranscode(1, playbackPosition, failedSide(error))
                    android.widget.Toast.makeText(context, "This device can't play the original. Switching to Compatibility.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else {
                    playbackFailed = true
                    android.widget.Toast.makeText(context, "Playback failed: ${error.errorCodeName}",
                        android.widget.Toast.LENGTH_LONG).show()
                }
            }

            override fun onPlaybackStateChanged(state: Int) {
                if (state != Player.STATE_IDLE) { playbackFailed = false; failureReason = null }
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
                    viewModel.playNextAfterEnd(path, replay = { exoPlayer.seekTo(0); exoPlayer.play() }, close = { onBack(path) })
                }
                isBuffering = state == Player.STATE_BUFFERING || state == Player.STATE_IDLE
                val videoSize = exoPlayer.videoSize
                if (videoSize.width > 0 && videoSize.height > 0) videoAspectRatio = videoSize.width.toFloat() / videoSize.height.toFloat()
            }
            override fun onTracksChanged(tracks: Tracks) {
                // A track with no decoder gives no error, only silence or no picture.
                val missing = unplayableSide(tracks)
                if (missing != null && !switching && !isSmbSource && !originalByHand) {
                    if (currentTranscodeMode.isOriginal) { switching = true; chooseTranscode(1, playbackPosition, missing) }
                    else choice?.nextAfter(missing)?.let { next ->
                        switching = true
                        scope.launch { startPlan(next, selectedTranscodeModeIndex, playbackPosition) }
                    }
                }
                if (serverSubs != null) return // the server's subtitles, not the stream's
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

    /** Restarts a server stream at [targetMs]; a copied video begins at the keyframe before it. */
    fun restartStream(targetMs: Long) {
        fun play(beginsMs: Long) {
            transcodeStartOffset = beginsMs
            exoPlayer.setMediaItem(MediaItem.fromUri(transcodedUrl(targetMs)))
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true
        }
        restartJob?.cancel()
        if (choice?.copiesVideo == true) restartJob = scope.launch { play(choice?.startOf(targetMs) ?: targetMs) }
        else play(targetMs)
    }

    fun performSkip(amount: Int, showControlsAfter: Boolean, useAbsoluteAmount: Boolean = false) {
        if (amount == 0 && !useAbsoluteAmount) { skipFeedback = null; return }
        val currentDuration = exoPlayer.duration
        val maxPos = if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration

        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            val actual = if (useAbsoluteAmount) basePlaybackPositionBeforeSkip else (exoPlayer.currentPosition + transcodeStartOffset)
            val finalTarget = (actual + amount * 1000).coerceIn(0L, if (currentDuration == C.TIME_UNSET) Long.MAX_VALUE else currentDuration + transcodeStartOffset)
            restartStream(finalTarget)
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
            restartStream(target)
        } else {
            exoPlayer.seekTo(target)
            exoPlayer.play()
        }
        updateInteraction()
    }

    // Skip Intro / Skip Credits (SkipSegments.kt): the intro to its end, the credits to the next video.
    fun seekAbsolute(target: Long) {
        if (!isSmbSource && !currentTranscodeMode.isOriginal) {
            restartStream(target)
        } else {
            exoPlayer.seekTo(target)
        }
    }
    val doSkip: (SkipPrompt, com.vargon.weirdstreamer.api.models.MediaSegments) -> Unit = { prompt, segments ->
        if (!prompt.credits) seekAbsolute((prompt.segment.end * 1000).toLong())
        else {
            ended = true // reported as finished just below; no report after it
            if (!viewModel.playAfterCredits(path, playbackPosition, segments.next)) onBack(path)
        }
    }
    val skip = if (isSmbSource || isDisc) null else rememberSkipPrompt(viewModel, path, playbackPosition, showControls, doSkip)

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
                            if (skip != null && !showControls && !showZoomToolbar && !isTrickplayVisible && !isZoomMode) {
                                doSkip(skip.first, skip.second) // the Skip button shows as focused
                                true
                            } else if (isTrickplayVisible) {
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
                                val opp = (now - lastSkipTime) <= SKIP_REVERSAL_WINDOW_MS && lastSkipDirection == 1 && skipReversalStep == 0
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
                                val opp = (now - lastSkipTime) <= SKIP_REVERSAL_WINDOW_MS && lastSkipDirection == -1 && skipReversalStep == 0
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

        if (playbackFailed) PlaybackFailedMessage(Modifier.align(Alignment.Center), failureReason)
        else if (isBuffering) CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { MaterialTheme.colorScheme.primary })

        FilenameOverlay(show = showControls, filename = filename, title = playerTitle, modifier = Modifier.align(Alignment.TopStart))
        skip?.let { (prompt, _) ->
            TvSkipButton(prompt, Modifier.align(Alignment.BottomEnd)
                .padding(end = 56.dp, bottom = if (showControls || showZoomToolbar) 210.dp else 64.dp))
        }
        SkipFeedbackOverlay(feedback = skipFeedback)
        SpeedFeedbackOverlay(speed = speedFeedback)
        SubtitleOverlay(serverSubs, position = {
            if (!currentTranscodeMode.isOriginal) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
        }, fontSize = 26.sp, bottomPadding = if (showControls || showZoomToolbar) 190.dp else 48.dp, videoAspect = {
            exoPlayer.videoSize.let { if (it.height > 0) it.width * it.pixelWidthHeightRatio / it.height else 0f }
        })

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
        TranscodeDialog(show = showTranscodeDialog, selectedModeIndex = selectedTranscodeModeIndex, transcodeModes = transcodeModes, status = choice?.status(currentTranscodeMode), onDismiss = { showTranscodeDialog = false }, onModeSelected = { idx -> if (selectedTranscodeModeIndex != idx && !((isDisc || tooBigToDecode) && transcodeModes[idx].isOriginal)) { if (transcodeModes[idx].isOriginal) { originalByHand = true; choice?.clear(); transcodeStartOffset = playbackPosition; selectedTranscodeModeIndex = idx } else chooseTranscode(idx, playbackPosition) }; showTranscodeDialog = false })
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
        val subtitleLabels = serverSubs?.labels ?: availableSubtitles
        SubtitleDialog(show = showSubtitleDialog, availableSubtitles = subtitleLabels,
            selectedSubtitleIndex = serverSubs?.selected ?: selectedSubtitleIndex, onDismiss = { showSubtitleDialog = false }, onSubtitleSelected = {
            if (serverSubs != null) chooseServerSubtitle(it) else selectSubtitleTrack(it)
            if (!isDisc) SubtitleMemory.set(context, subtitleVideo, it, subtitleLabels.getOrNull(it))
        }, onDownloadRequested = if (serverSubs == null) null else ({ showSubtitleDialog = false; showDownloadDialog = true; searchSubtitlesOnline() }),
            timing = serverSubs?.takeIf { it.selectedTrack?.text == true }?.let { subtitleOffsetText(it.offsetMs) },
            onTiming = { showSubtitleDialog = false; showTimingDialog = true })
        SubtitleTimingDialog(show = showTimingDialog && serverSubs != null, offsetMs = serverSubs?.offsetMs ?: 0,
            onShift = { serverSubs?.shift(it) }, onDismiss = { showTimingDialog = false })
        OnlineSubtitlesDialog(show = showDownloadDialog, isSearching = isSearchingSubtitles, status = downloadStatus,
            note = subtitleSearchNote, results = subtitleResults,
            onDismiss = { showDownloadDialog = false; subtitleResults = emptyList(); downloadStatus = null; subtitleSearchNote = null },
            onChosen = { downloadSubtitle(it) })
    }
}
