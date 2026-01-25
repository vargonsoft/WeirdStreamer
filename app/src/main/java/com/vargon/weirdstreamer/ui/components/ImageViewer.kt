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
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.key.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.compose.AsyncImagePainter
import coil.request.ImageRequest
import com.vargon.weirdstreamer.SmbViewModel
import kotlinx.coroutines.delay
import java.io.File

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun ImageViewer(viewModel: SmbViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val images = remember { viewModel.getImagesInCurrentFolder() }
    var currentIndex by remember { mutableIntStateOf(images.indexOf(initialPath).coerceAtLeast(0)) }
    
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    
    var showFilename by remember { mutableStateOf(false) }
    var filenameTimerTrigger by remember { mutableLongStateOf(0L) }
    var loopFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    
    val focusRequester = remember { FocusRequester() }
    val currentSmbPath = images.getOrNull(currentIndex) ?: initialPath
    
    // For HEIC support on some devices (like Google TV), we download to a temp file
    // because the system's HEIF decoder often requires a seekable FileDescriptor.
    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }

    val fileName = remember(currentSmbPath) {
        currentSmbPath.substringBefore('?').removeSuffix("/").substringAfterLast('/')
    }

    LaunchedEffect(currentSmbPath) {
        filenameTimerTrigger = System.currentTimeMillis()
        isLoading = true
        errorMessage = null
        
        // Cleanup old temp file
        tempFile?.delete()
        tempFile = null
        
        if (viewModel.isImage(currentSmbPath)) {
            val extension = currentSmbPath.substringBefore('?').substringAfterLast('.', "").lowercase()
            if (extension == "heic" || extension == "heif") {
                val file = viewModel.downloadToTempFile(currentSmbPath, context, ".$extension")
                if (file != null) {
                    tempFile = file
                    localImagePath = file.absolutePath
                } else {
                    errorMessage = "Failed to download HEIC for decoding"
                    localImagePath = currentSmbPath // Fallback to stream
                }
            } else {
                localImagePath = currentSmbPath
            }
        } else {
            localImagePath = currentSmbPath
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            tempFile?.delete()
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

    val handleBack = {
        if (isZoomMode) {
            resetZoom()
        } else {
            onBack(currentSmbPath)
        }
    }

    BackHandler { handleBack() }

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
                        Key.DirectionCenter, Key.Enter -> {
                            if (!isZoomMode) {
                                isZoomMode = true
                                if (zoomLevel == 1f) zoomLevel = 2f
                            } else if (zoomLevel < 256f) {
                                zoomLevel *= 2f
                            }
                            true
                        }
                        Key.Backspace -> {
                            if (isZoomMode) {
                                if (zoomLevel > 2f) {
                                    zoomLevel /= 2f
                                } else {
                                    resetZoom()
                                }
                                true
                            } else {
                                onBack(currentSmbPath)
                                true
                            }
                        }
                        Key.DirectionLeft -> {
                            if (isZoomMode) {
                                panX += 0.1f / zoomLevel
                            } else {
                                navigate(false)
                            }
                            true
                        }
                        Key.DirectionRight -> {
                            if (isZoomMode) {
                                panX -= 0.1f / zoomLevel
                            } else {
                                navigate(true)
                            }
                            true
                        }
                        Key.DirectionUp -> {
                            if (isZoomMode) panY += 0.1f / zoomLevel
                            true
                        }
                        Key.DirectionDown -> {
                            if (isZoomMode) panY -= 0.1f / zoomLevel
                            true
                        }
                        else -> false
                    }
                } else false
            }
            .focusRequester(focusRequester)
            .focusable()
    ) {
        localImagePath?.let { path ->
            AsyncImage(
                model = ImageRequest.Builder(LocalContext.current)
                    .data(path)
                    .crossfade(true)
                    .build(),
                contentDescription = null,
                contentScale = if (isZoomMode) ContentScale.None else ContentScale.Fit,
                onState = { state ->
                    isLoading = state is AsyncImagePainter.State.Loading
                    if (state is AsyncImagePainter.State.Error) {
                        errorMessage = state.result.throwable.message ?: "Failed to load image"
                        Log.e("ImageViewer", "Error loading $path: $errorMessage", state.result.throwable)
                    }
                },
                modifier = Modifier
                    .fillMaxSize()
                    .clipToBounds()
                    .graphicsLayer(
                        scaleX = zoomLevel,
                        scaleY = zoomLevel,
                        translationX = panX * 1000f * zoomLevel,
                        translationY = panY * 1000f * zoomLevel
                    )
            )
        }

        if (isLoading) {
            CircularProgressIndicator(
                modifier = Modifier.align(Alignment.Center),
                color = Color.White
            )
        }

        if (errorMessage != null) {
            Column(
                modifier = Modifier.align(Alignment.Center).padding(32.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text("Error", style = MaterialTheme.typography.headlineMedium, color = Color.Red)
                Text(errorMessage!!, style = MaterialTheme.typography.bodyMedium, color = Color.White, textAlign = TextAlign.Center)
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
            Box(
                modifier = Modifier
                    .size(64.dp)
                    .background(Color.Black.copy(alpha = 0.6f), RoundedCornerShape(32.dp)),
                contentAlignment = Alignment.Center
            ) {
                Icon(
                    imageVector = Icons.Default.Refresh,
                    contentDescription = null,
                    tint = Color.White,
                    modifier = Modifier
                        .size(40.dp)
                        .graphicsLayer(scaleX = if (isForward) -1f else 1f)
                )
            }
        }
    }

    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }
}
