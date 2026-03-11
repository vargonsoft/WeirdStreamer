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
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.compose.AsyncImagePainter
import coil.request.ImageRequest
import coil.size.Size as CoilSize
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay
import java.io.File

@OptIn(ExperimentalTvMaterial3Api::class)
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

    // For HEIC support
    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }

    val fileName = remember(currentSmbPath) {
        viewModel.getDisplayName(currentSmbPath)
    }

    LaunchedEffect(currentSmbPath) {
        filenameTimerTrigger = System.currentTimeMillis()
        isLoading = true
        errorMessage = null
        imageIntrinsicSize = null
        rotation = 0f

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
                    localImagePath = currentSmbPath
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
                    modifier = Modifier
                        .fillMaxSize()
                        .graphicsLayer(
                            scaleX = zoomLevel,
                            scaleY = zoomLevel,
                            translationX = panX,
                            translationY = panY,
                            rotationZ = rotation
                        )
                )
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
        }
    }

    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }
}
