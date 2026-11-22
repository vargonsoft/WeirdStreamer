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
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
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
import com.vargon.weirdstreamer.api.models.forDiscTitle
import com.vargon.weirdstreamer.api.models.progressTitle
import com.vargon.weirdstreamer.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
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
    var playbackFailed by remember { mutableStateOf(false) }
    var failureReason by remember { mutableStateOf<String?>(null) }
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
    var loadedTrickplay by remember { mutableStateOf<TrickplayInfo?>(null) }
    // A disc's frames are for its main feature: none while another title plays.
    val trickplayInfo = loadedTrickplay?.forDiscTitle(discTitle, discInfo?.mainTitle)
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
    var enableTranscode by remember(path) { mutableStateOf(isDisc || tooBigToDecode) }
    var showTranscodeMenu by remember { mutableStateOf(false) }
    fun chooseTranscodeMode(index: Int) {
        transcodeModeIndex = index
        enableTranscode = index != 0
    }
    // How a transcode treats the video and the audio (StreamChoice)
    val scope = rememberCoroutineScope()
    val choice = remember(path) { if (isSmbSource || isLocal || isDisc) null else StreamChoice.forStream(path) }
    var originalByHand by remember(path) { mutableStateOf(false) } // plays as it is, even silent
    var switching by remember(path) { mutableStateOf(false) }
    var seekJob by remember(path) { mutableStateOf<kotlinx.coroutines.Job?>(null) }
    var transcodeStartOffset by remember { mutableLongStateOf(0L) }
    var fullDuration by remember { mutableLongStateOf(0L) }

    // Subtitles from the server, as on the TV and the web: the video's own tracks, then files
    // beside it and downloads, drawn over the video whatever the stream. A picture track is burned
    // into a transcode. A download on this phone has the tracks saved with it; SMB files and discs have none.
    val serverSubs = remember(path) {
        when {
            isLocal -> ServerSubtitles.forDownload(path)
            isSmbSource || isDisc -> null
            else -> ServerSubtitles.forStream(path)
        }
    }
    val burnIn = serverSubs?.burnIn ?: -1
    // Stereo audio in transcodes, once a surround one has failed to decode (AudioFallback)
    var stereoAudio by remember(path) { mutableStateOf(AudioFallback.stereo) }

    /** Switches to transcode mode [index]; [failed] is what the original failed on, if it did;
     *  [track] is the audio track to play (else the one playing). */
    fun chooseTranscode(index: Int, failed: FailedSide? = null, track: Int? = null) {
        scope.launch {
            val mayCopy = (serverSubs?.burnIn ?: -1) < 0 && !tooBigToDecode
            choice?.plan(context, transcodeModes[index], mayCopy, failed, track ?: choice.audioTrack)?.let { choice.apply(it) }
            chooseTranscodeMode(index)
            switching = false
        }
    }

    fun transcodedUrl(startMs: Long): String {
        val base = path.substringBefore("?") + "?path=" + path.substringAfter("path=").substringBefore("&") + "&auto-transcode=0"
        val mode = transcodeModes[transcodeModeIndex]
        val video = "&" + (choice?.videoQuery(mode) ?: ("transcode=1" + transcodeQuery(mode)))
        val audio = choice?.audio?.query ?: if (stereoAudio) "&channels=2" else ""
        return base + video + choice?.trackQuery().orEmpty() + "&start=${StreamChoice.streamStart(startMs)}$discQuery" +
            (if (burnIn >= 0) "&subtitle=$burnIn" else "") + audio
    }

    var showSubtitleMenu by remember { mutableStateOf(false) }
    var showAudioMenu by remember { mutableStateOf(false) }
    var showTiming by remember { mutableStateOf(false) }
    var showSubtitleSearch by remember { mutableStateOf(false) }
    val subtitleVideo = remember(path) {
        // A download remembers its choice with the video on the server.
        sourceItem?.originalPath ?: (if (isLocal) com.vargon.weirdstreamer.downloads.DownloadStore.ofLocalPath(path)?.sourcePath else null)
            ?: path.substringBefore("&")
    }
    // The audio track chosen for this video last time. A video too big for the hardware starts
    // transcoded (and its audio can still be kept); the original plays a track the phone decodes
    // itself, and the server converts one it doesn't (Compatibility (Auto)).
    LaunchedEffect(path) {
        val c = choice ?: return@LaunchedEffect
        c.load()
        val saved = AudioMemory.get(context, subtitleVideo)?.let { AudioMemory.resolve(it, audioLabels(c.audioTracks)) } ?: c.preferredTrack
        when {
            tooBigToDecode -> chooseTranscode(1, track = saved)
            saved < 0 -> {}
            c.originalPlays(context, saved) -> c.chooseForOriginal(saved)
            else -> chooseTranscode(1, track = saved)
        }
    }

    /** Chooses audio track [index] (the server's) from the Audio menu, and keeps it for next time. */
    fun chooseAudio(index: Int) {
        val c = choice ?: return
        AudioMemory.set(context, subtitleVideo, index, audioLabels(c.audioTracks).getOrNull(index))
        if (index == c.shownTrack && c.audioTrack >= 0) return
        when {
            enableTranscode -> chooseTranscode(transcodeModeIndex, track = index)
            c.originalPlays(context, index) -> c.chooseForOriginal(index)
            else -> { originalByHand = false; chooseTranscode(1, track = index) }
        }
    }

    fun chooseServerSubtitle(index: Int) {
        val subs = serverSubs ?: return
        subs.select(index)
        // The original can't show a picture track: it's burned into a transcode.
        if (subs.burnIn >= 0) {
            choice?.stopCopying()
            chooseTranscode(if (enableTranscode) transcodeModeIndex else 1)
        }
    }
    LaunchedEffect(serverSubs) {
        val subs = serverSubs ?: return@LaunchedEffect
        subs.load()
        val index = SubtitleMemory.get(context, subtitleVideo)?.let { SubtitleMemory.resolve(it, subs.labels) }
            ?: subs.preferred.takeIf { it >= 0 } ?: subs.tracks.indexOfFirst { it.forced && it.text }
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
        if (isLocal) { // a download: the frames saved with it
            loadedTrickplay = withContext(Dispatchers.IO) {
                com.vargon.weirdstreamer.downloads.DownloadStore.ofLocalPath(path)?.let { com.vargon.weirdstreamer.downloads.DownloadStore.savedTrickplay(it) }
            }
        }
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
                            loadedTrickplay = viewModel.fetchTrickplayInfo(item)
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

    // The original plays the chosen audio track itself, once the player has the file's tracks.
    var audioAppliedFor by remember { mutableStateOf<Pair<Any?, Int>?>(null) }
    fun applyOriginalAudio() {
        if (isLocal) { // a download kept whole plays the audio track it was downloaded with
            val d = com.vargon.weirdstreamer.downloads.DownloadStore.ofLocalPath(path) ?: return
            val track = d.audioTrack ?: return
            if (exoPlayer.currentTracks.groups.none { it.type == androidx.media3.common.C.TRACK_TYPE_AUDIO }) return
            if (audioAppliedFor?.first == exoPlayer.currentMediaItem) return
            audioAppliedFor = exoPlayer.currentMediaItem to track
            selectAudioTrack(exoPlayer, track, d.audioTracks ?: 0) // a copy with one track: nothing to choose
            return
        }
        val c = choice ?: return
        if (enableTranscode || exoPlayer.currentTracks.groups.none { it.type == androidx.media3.common.C.TRACK_TYPE_AUDIO }) return
        val key = exoPlayer.currentMediaItem to c.audioTrack
        if (audioAppliedFor == key) return
        audioAppliedFor = key
        val plays = c.audioTrack < 0 || c.originalPlays(context, c.audioTrack)
        if (plays && selectAudioTrack(exoPlayer, c.audioTrack, c.audioTracks.size)) return
        // The phone can't decode it, or the player's tracks aren't the server's: the server plays it.
        if (!originalByHand && !switching) { switching = true; chooseTranscode(1) }
    }
    LaunchedEffect(choice?.audioTrack) { applyOriginalAudio() }

    // The audio track a stream from the server carries (the original plays the chosen one itself)
    val streamAudioTrack = if (enableTranscode) choice?.audioTrack ?: -1 else -1

    // Handle MediaItem updates when path or transcode mode changes
    LaunchedEffect(path, enableTranscode, transcodeModeIndex, discQuery, burnIn, stereoAudio, choice?.copiesVideo, choice?.audio, streamAudioTrack) {
        // Capture current position if available
        val currentPos = discRestartAt?.also { discRestartAt = null } ?: if (exoPlayer.mediaItemCount > 0) {
            if (transcodeStartOffset > 0) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
        } else {
            playbackPosition
        }

        val url = if (!isSmbSource && !isLocal && enableTranscode) {
            transcodeStartOffset = if (choice?.copiesVideo == true) choice.startOf(currentPos) else currentPos
            transcodedUrl(currentPos)
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
                unreadableReason(error)?.let { reason ->
                    // The server can't read the file: no transcode would help either.
                    playbackFailed = true
                    failureReason = reason
                    android.widget.Toast.makeText(context, reason, android.widget.Toast.LENGTH_LONG).show()
                    return
                }
                val decoderProblem = error.errorCode in androidx.media3.common.PlaybackException.ERROR_CODE_DECODER_INIT_FAILED..
                    androidx.media3.common.PlaybackException.ERROR_CODE_DECODING_RESOURCES_RECLAIMED
                val next = if (enableTranscode) choice?.nextAfter(failedSide(error)) else null
                if (next != null) {
                    playbackPosition = exoPlayer.currentPosition.takeIf { it > 0 }?.plus(transcodeStartOffset) ?: playbackPosition
                    choice?.apply(next)
                    android.widget.Toast.makeText(context, choice?.status(transcodeModes[transcodeModeIndex]),
                        android.widget.Toast.LENGTH_SHORT).show()
                } else if (AudioFallback.isAudioDecoderFailure(error) && enableTranscode && !isSmbSource && !isLocal && !stereoAudio) {
                    // The transcode's surround audio didn't decode: again from here, in stereo
                    // (the stream restarts where the player is).
                    AudioFallback.stereo = true
                    stereoAudio = true
                    android.widget.Toast.makeText(context, "This phone can't play the surround audio. Switching to stereo.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else if (!enableTranscode && !isSmbSource && !isLocal) {
                    // Whatever failed (decoder, container, audio output), the server may manage it.
                    playbackPosition = exoPlayer.currentPosition.takeIf { it > 0 } ?: playbackPosition
                    chooseTranscode(1, failedSide(error))
                    android.widget.Toast.makeText(context, "This phone can't play the original. Switching to Compatibility.",
                        android.widget.Toast.LENGTH_SHORT).show()
                } else {
                    val message = if (decoderProblem && isLocal)
                        "This phone can't play this download. Download a compressed copy at a lower resolution."
                    else "Playback failed: ${error.errorCodeName}"
                    playbackFailed = true
                    android.widget.Toast.makeText(context, message, android.widget.Toast.LENGTH_LONG).show()
                }
            }

            override fun onTracksChanged(tracks: androidx.media3.common.Tracks) {
                applyOriginalAudio()
                // A track with no decoder gives no error, only silence or no picture.
                val missing = unplayableSide(tracks)
                if (missing == null || switching || isSmbSource || isLocal || originalByHand) return
                if (!enableTranscode) { switching = true; chooseTranscode(1, missing) }
                else choice?.nextAfter(missing)?.let { choice.apply(it) }
            }

            override fun onPlaybackStateChanged(state: Int) {
                isBuffering = state == Player.STATE_BUFFERING
                if (state != Player.STATE_IDLE) { playbackFailed = false; failureReason = null }
                if (state == Player.STATE_ENDED) {
                    if (!isSmbSource) {
                        ended = true
                        viewModel.reportPlayback(path, playbackPosition, paused = true, ended = true, discTitle = discInfo.progressTitle(discTitle))
                    }
                    viewModel.playNextAfterEnd(path, replay = { exoPlayer.seekTo(0); exoPlayer.play() }, close = { onBack(path) })
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
                viewModel.reportPlayback(path, position, paused = true, discTitle = discInfo.progressTitle(discTitle))
            }
            exoPlayer.release()
        }
    }

    fun seekPlayer(targetPos: Long) {
        if (!isSmbSource && enableTranscode) {
            fun play(askMs: Long, beginsMs: Long) {
                transcodeStartOffset = beginsMs
                exoPlayer.setMediaItem(MediaItem.fromUri(transcodedUrl(askMs)))
                exoPlayer.prepare()
                exoPlayer.playWhenReady = true
            }
            seekJob?.cancel()
            val from = playbackPosition
            if (choice?.copiesVideo == true) seekJob = scope.launch {
                val (ask, begins) = choice.jump(from, targetPos)
                play(ask, begins)
            }
            else play(targetPos, targetPos)
        } else {
            exoPlayer.seekTo(targetPos)
        }
    }

    // Swipes' indicator (below), also shown by resume()
    var swipeIndicator by remember { mutableStateOf<SwipeIndicator?>(null) }
    var swipeReleased by remember { mutableIntStateOf(0) } // hides the indicator a moment after
    var indicatorMs by remember { mutableLongStateOf(700L) } // that moment: longer for resume()'s

    // Unpausing: with Settings' "Rewind 5 seconds when resuming", go back up to 5 seconds first,
    // shown as a swipe back would be.
    fun resume() {
        if (viewModel.rewindOnResume && !exoPlayer.playWhenReady) { // paused, not just buffering
            val at = if (!isSmbSource && enableTranscode) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition
            val back = minOf(RESUME_REWIND_S * 1000, at / 1000 * 1000)
            if (back > 0) {
                seekPlayer(at - back)
                swipeIndicator = SwipeIndicator.Seek(seekGestureText(-back, at - back))
                indicatorMs = 1500L
                swipeReleased++
            }
        }
        exoPlayer.play()
    }

    // Skip Intro / Skip Credits (SkipSegments.kt): the intro to its end, the credits to the next video.
    val doSkip: (SkipPrompt, com.vargon.weirdstreamer.api.models.MediaSegments) -> Unit = { prompt, segments ->
        if (!prompt.credits) seekPlayer((prompt.segment.end * 1000).toLong())
        else {
            ended = true // reported as finished just below; no report after it
            if (!viewModel.playAfterCredits(path, playbackPosition, segments.next)) onBack(path)
        }
    }
    val skip = if (isSmbSource || isDisc) null else rememberSkipPrompt(viewModel, path, playbackPosition, showControls, doSkip)

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
            viewModel.reportPlayback(path, playbackPosition, !exoPlayer.isPlaying, discTitle = discInfo.progressTitle(discTitle))
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

    // Going home while it plays shrinks it into a window (PhonePip), of the picture's shape.
    val inPip = viewModel.inPictureInPicture
    PhonePip.Track(activity, playing = isPlaying && !playbackFailed,
        aspect = (currentAspectRatio ?: videoAspectRatio).let { if ((rotation / 90f).toInt() % 2 != 0) 1f / it else it },
        onPlayPause = { if (exoPlayer.isPlaying) exoPlayer.pause() else resume() })

    // The picture's transform (rotation, the aspect's stretch, pinch-zoom), for the video and the
    // ASS subtitles placed on it.
    val videoTransform: androidx.compose.ui.graphics.GraphicsLayerScope.() -> Unit = {
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
    val subtitlePosition = { if (transcodeStartOffset > 0) exoPlayer.currentPosition + transcodeStartOffset else exoPlayer.currentPosition }
    val videoShape = { exoPlayer.videoSize.let { if (it.height > 0) it.width * it.pixelWidthHeightRatio / it.height else 0f } }

    BackHandler { 
        if (showTiming) {
            showTiming = false
        } else if (showSpeedDialog) {
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

    // Swipes (PhoneGestures.kt): sideways seeks, applied on release; up and down set the
    // brightness (left half) or the volume (right half) as the finger moves.
    var swipeSeekFrom by remember { mutableLongStateOf(0L) }
    var swipeSeekDelta by remember { mutableFloatStateOf(0f) }
    var swipeLevel by remember { mutableFloatStateOf(0f) }
    val swipeSeek by rememberUpdatedState<(Long) -> Unit> { seekPlayer(it) }
    val gesturesOn by rememberUpdatedState(!inPip && !showSpeedDialog)
    fun swipeSeekTarget() = (swipeSeekFrom + swipeSeekDelta.toLong()).coerceIn(0L, duration.coerceAtLeast(0L))
    // The player's brightness lasts while it's open; leaving it gives the system's back.
    DisposableEffect(activity) { onDispose { activity?.setPlayerBrightness(null) } }
    LaunchedEffect(swipeReleased) {
        if (swipeReleased > 0) { delay(indicatorMs); swipeIndicator = null; indicatorMs = 700L }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black)
            .phonePlayerGestures(
                enabled = { gesturesOn },
                zoomed = { zoomScale > 1f },
                onTransform = { pan, zoom ->
                    zoomScale = (zoomScale * zoom).coerceIn(1f, 10f)
                    if (zoomScale > 1f) {
                        zoomOffsetX += pan.x
                        zoomOffsetY += pan.y
                    } else {
                        zoomOffsetX = 0f
                        zoomOffsetY = 0f
                    }
                },
                onSwipeStart = { kind ->
                    swipeReleased = 0
                    when (kind) {
                        PlayerSwipe.Seek -> { swipeSeekFrom = playbackPosition; swipeSeekDelta = 0f }
                        PlayerSwipe.Brightness -> swipeLevel = activity?.playerBrightness() ?: 0.5f
                        PlayerSwipe.Volume -> swipeLevel = context.mediaVolume()
                    }
                },
                onSwipe = { kind, move, size ->
                    when (kind) {
                        PlayerSwipe.Seek -> {
                            if (duration > 0) {
                                // The screen's width is 90 seconds.
                                swipeSeekDelta += move.x / size.width.coerceAtLeast(1) * 90_000f
                                val target = swipeSeekTarget()
                                swipeSeekDelta = (target - swipeSeekFrom).toFloat()
                                swipeIndicator = SwipeIndicator.Seek(seekGestureText(target - swipeSeekFrom, target))
                            }
                        }
                        else -> {
                            // The height of the screen goes from none to full.
                            swipeLevel = (swipeLevel - move.y / size.height.coerceAtLeast(1)).coerceIn(0f, 1f)
                            val shown = if (kind == PlayerSwipe.Volume) {
                                context.setMediaVolume(swipeLevel)
                            } else {
                                activity?.setPlayerBrightness(swipeLevel)
                                swipeLevel
                            }
                            swipeIndicator = SwipeIndicator.Level(kind == PlayerSwipe.Volume, shown)
                        }
                    }
                },
                onSwipeEnd = { kind, cancelled ->
                    if (kind == PlayerSwipe.Seek) {
                        val target = swipeSeekTarget()
                        if (!cancelled && duration > 0 && target != swipeSeekFrom) swipeSeek(target)
                        swipeIndicator = null
                    } else { indicatorMs = 700L; swipeReleased++ }
                }
            )
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
                        .graphicsLayer(videoTransform)
                )
                // ASS subtitles are placed on the picture: drawn here, moved with it.
                serverSubs?.takeIf { it.styled }?.let { subs ->
                    AssSubtitle(subs, position = subtitlePosition, videoAspect = videoShape,
                        modifier = Modifier.fillMaxSize().graphicsLayer(videoTransform))
                }
            }
        }

        SubtitleOverlay(serverSubs, position = subtitlePosition, fontSize = if (inPip) 9.sp else phoneSubtitleFontSize(),
            bottomPadding = if (inPip) 4.dp else if (showControls) 160.dp else 32.dp, videoAspect = videoShape,
            drawsStyled = false)

        // Loading Spinner
        if (playbackFailed) {
            PlaybackFailedMessage(Modifier.align(Alignment.Center), failureReason)
        } else if (isBuffering) {
            CircularProgressIndicator(
                modifier = Modifier.align(Alignment.Center),
                color = Color.White
            )
        }

        AnimatedVisibility(
            visible = showControls && !inPip,
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
                    val playerTitle by produceState<String?>(null, path) { value = viewModel.playerTitleFor(path) }
                    Column(modifier = Modifier.padding(start = 8.dp).weight(1f)) {
                        Text(
                            text = playerTitle ?: viewModel.getDisplayName(path),
                            color = Color.White,
                            style = MaterialTheme.typography.titleMedium,
                            maxLines = 2,
                            overflow = TextOverflow.Ellipsis
                        )
                        if (playerTitle != null) {
                            Text(
                                text = viewModel.getDisplayName(path),
                                color = Color.White.copy(alpha = 0.8f),
                                style = MaterialTheme.typography.bodySmall,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                    
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
                            DropdownMenu(border = phoneMenuBorder(), expanded = showTranscodeMenu, onDismissRequest = { showTranscodeMenu = false }) {
                                choice?.status(transcodeModes[transcodeModeIndex])?.let { status ->
                                    Text(status, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                                }
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
                                            if (index != transcodeModeIndex) {
                                                if (mode.isOriginal) { originalByHand = true; choice?.clear(); chooseTranscodeMode(index) }
                                                else chooseTranscode(index)
                                            }
                                        }
                                    )
                                }
                            }
                        }
                    }
                    
                    // Audio: the file's tracks, as the server lists them
                    choice?.takeIf { it.audioTracks.size > 1 }?.let { c ->
                        Box {
                            IconButton(onClick = {
                                lastInteractionTime = System.currentTimeMillis()
                                showAudioMenu = true
                            }) {
                                Icon(Icons.Default.Audiotrack, contentDescription = "Audio", tint = Color.White)
                            }
                            DropdownMenu(border = phoneMenuBorder(), expanded = showAudioMenu, onDismissRequest = { showAudioMenu = false }) {
                                audioLabels(c.audioTracks).forEachIndexed { i, label ->
                                    val track = c.audioTracks[i]
                                    DropdownMenuItem(
                                        text = { Text(label) },
                                        trailingIcon = if (track.index == c.shownTrack) {
                                            { Icon(Icons.Default.Check, contentDescription = "Selected") }
                                        } else null,
                                        onClick = {
                                            showAudioMenu = false
                                            lastInteractionTime = System.currentTimeMillis()
                                            chooseAudio(track.index)
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
                            DropdownMenu(border = phoneMenuBorder(), expanded = showSubtitleMenu, onDismissRequest = { showSubtitleMenu = false }) {
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
                                if (subs.selectedTrack?.text == true) {
                                    DropdownMenuItem(
                                        text = { Text("Timing: ${subtitleOffsetText(subs.offsetMs)}") },
                                        leadingIcon = { Icon(Icons.Default.Schedule, contentDescription = null) },
                                        onClick = { showSubtitleMenu = false; showTiming = true }
                                    )
                                }
                                if (subs.online) DropdownMenuItem(
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
                            if (isPlaying) exoPlayer.pause() else resume()
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
                    
                    // Frame by frame: always there (shown only while paused, they flashed in during
                    // a seek's buffering and moved the buttons above); a step pauses the video.
                    Row(
                        modifier = Modifier.padding(top = 16.dp),
                        horizontalArrangement = Arrangement.spacedBy(64.dp)
                    ) {
                        IconButton(onClick = { 
                            lastInteractionTime = System.currentTimeMillis()
                            exoPlayer.pause()
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
                            exoPlayer.pause()
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

        // Subtitle timing, at the top so the subtitles below show the change
        if (showTiming && serverSubs != null) {
            PhoneSubtitleTiming(serverSubs.offsetMs, onShift = { serverSubs.shift(it) }, onClose = { showTiming = false },
                modifier = Modifier.align(Alignment.TopCenter).padding(top = 24.dp, start = 12.dp, end = 12.dp))
        }

        // Over the picture's lower right, above the controls when they show
        if (!inPip) skip?.let { (prompt, segments) ->
            PhoneSkipButton(prompt, onClick = { doSkip(prompt, segments) },
                modifier = Modifier.align(Alignment.BottomEnd).padding(end = 20.dp, bottom = if (showControls) 150.dp else 40.dp))
        }

        swipeIndicator?.let { PhoneSwipeIndicator(it, Modifier.align(Alignment.Center)) }

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
                            PhoneSecondaryButton(onClick = { showSpeedDialog = false }) {
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
            if (serverSubs.burnIn >= 0) {
                choice?.stopCopying()
                chooseTranscode(if (enableTranscode) transcodeModeIndex else 1)
            }
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
        confirmButton = { PhoneSecondaryButton(onClick = onDismiss) { Text("Close") } }
    )
}

/**
 * Subtitle timing on the phone: "Already said" when the line was spoken before its subtitle showed
 * (subtitles earlier), "Not said yet" when the subtitle came first (later), each by the step chosen.
 */
@Composable
private fun PhoneSubtitleTiming(offsetMs: Int, onShift: (Int) -> Unit, onClose: () -> Unit, modifier: Modifier = Modifier) {
    var step by remember { mutableIntStateOf(250) }
    androidx.compose.material3.Surface(modifier = modifier, shape = AppButtonShape, color = Color.Black.copy(alpha = 0.85f),
        border = phoneMenuBorder()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp),
            horizontalAlignment = Alignment.CenterHorizontally) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("Subtitle timing: ${subtitleOffsetText(offsetMs)}", color = Color.White, modifier = Modifier.weight(1f, fill = false))
                IconButton(onClick = onClose) { Icon(Icons.Default.Close, contentDescription = "Close", tint = Color.White) }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { onShift(-step) }, shape = AppButtonShape, colors = phoneButtonColors()) { Text("Already said") }
                Button(onClick = { onShift(step) }, shape = AppButtonShape, colors = phoneButtonColors()) { Text("Not said yet") }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                SUBTITLE_STEPS.forEach { ms ->
                    FilterChip(colors = phoneChipColors(), selected = ms == step, border = phoneChipBorder(ms == step), onClick = { step = ms }, shape = AppButtonShape,
                        label = { Text(subtitleStepText(ms)) })
                }
            }
        }
    }
}
