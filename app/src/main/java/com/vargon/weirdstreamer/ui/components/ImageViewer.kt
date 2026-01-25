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
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.compose.AsyncImagePainter
import coil.request.ImageRequest
import com.vargon.weirdstreamer.SmbViewModel
import kotlinx.coroutines.delay

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun ImageViewer(viewModel: SmbViewModel, initialPath: String, onBack: (String) -> Unit) {
    val images = remember { viewModel.getImagesInCurrentFolder() }
    var currentIndex by remember { mutableIntStateOf(images.indexOf(initialPath).coerceAtLeast(0)) }
    
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    
    var loopFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    
    val focusRequester = remember { FocusRequester() }
    val currentPath = images.getOrNull(currentIndex) ?: initialPath

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
            if (forward) {
                currentIndex = (currentIndex + 1) % images.size
                if (currentIndex < oldIndex) loopFeedback = true to System.currentTimeMillis()
            } else {
                currentIndex = (currentIndex - 1 + images.size) % images.size
                if (currentIndex > oldIndex) loopFeedback = false to System.currentTimeMillis()
            }
            resetZoom()
            errorMessage = null
            isLoading = true
        }
    }

    val handleBack = {
        if (isZoomMode) {
            resetZoom()
        } else {
            onBack(currentPath)
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
                                onBack(currentPath)
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
        AsyncImage(
            model = ImageRequest.Builder(LocalContext.current)
                .data(currentPath)
                .crossfade(true)
                .build(),
            contentDescription = null,
            contentScale = if (isZoomMode) ContentScale.None else ContentScale.Fit,
            onState = { state ->
                isLoading = state is AsyncImagePainter.State.Loading
                if (state is AsyncImagePainter.State.Error) {
                    errorMessage = state.result.throwable.message ?: "Failed to load image"
                    Log.e("ImageViewer", "Error loading $currentPath: $errorMessage", state.result.throwable)
                }
            },
            modifier = Modifier
                .fillMaxSize()
                .clipToBounds()
                .graphicsLayer(
                    scaleX = zoomLevel,
                    scaleY = zoomLevel,
                    translationX = panX * 1000f * zoomLevel, // Adjusted scaling for pan
                    translationY = panY * 1000f * zoomLevel
                )
        )

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
