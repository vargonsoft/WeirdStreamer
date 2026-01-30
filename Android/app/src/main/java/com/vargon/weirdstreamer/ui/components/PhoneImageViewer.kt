package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.gestures.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.MediaViewModel
import java.io.File

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneImageViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val images = remember { viewModel.getImagesInCurrentFolder() }
    val initialIndex = remember { images.indexOf(initialPath).coerceAtLeast(0) }
    val pagerState = rememberPagerState(initialPage = initialIndex) { images.size }
    
    val currentPath = images.getOrNull(pagerState.currentPage) ?: initialPath
    val fileName = remember(currentPath) { viewModel.getDisplayName(currentPath) }

    var isZoomed by remember { mutableStateOf(false) }

    LaunchedEffect(pagerState.currentPage) {
        isZoomed = false
    }

    BackHandler { onBack(currentPath) }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(fileName, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    IconButton(onClick = { onBack(currentPath) }) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = Color.Black.copy(alpha = 0.5f),
                    titleContentColor = Color.White,
                    navigationIconContentColor = Color.White
                )
            )
        },
        containerColor = Color.Black
    ) { padding ->
        HorizontalPager(
            state = pagerState,
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
            userScrollEnabled = !isZoomed,
            beyondViewportPageCount = 1
        ) { pageIndex ->
            ZoomableImage(
                viewModel = viewModel,
                path = images[pageIndex],
                onZoomChanged = { zoomed ->
                    if (pageIndex == pagerState.currentPage) {
                        isZoomed = zoomed
                    }
                }
            )
        }
    }
}

@Composable
fun ZoomableImage(viewModel: MediaViewModel, path: String, onZoomChanged: (Boolean) -> Unit) {
    val context = LocalContext.current
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(Offset.Zero) }
    
    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }
    var isLoading by remember { mutableStateOf(true) }

    LaunchedEffect(path) {
        isLoading = true
        tempFile?.delete()
        tempFile = null
        
        val extension = path.substringBefore('?').substringAfterLast('.', "").lowercase()
        if (extension == "heic" || extension == "heif") {
            val file = viewModel.downloadToTempFile(path, context, ".$extension")
            if (file != null) {
                tempFile = file
                localImagePath = file.absolutePath
            } else {
                localImagePath = path
            }
        } else {
            localImagePath = path
        }
        isLoading = false
    }

    DisposableEffect(path) {
        onDispose { tempFile?.delete() }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .pointerInput(path) {
                awaitEachGesture {
                    awaitFirstDown(requireUnconsumed = false)
                    do {
                        val event = awaitPointerEvent()
                        val isPinch = event.changes.size > 1
                        
                        // If we are zoomed in or currently pinching, consume the event to handle zoom/pan
                        if (scale > 1f || isPinch) {
                            val zoomChange = event.calculateZoom()
                            val panChange = event.calculatePan()
                            
                            val newScale = (scale * zoomChange).coerceIn(1f, 5f)
                            if (newScale != scale) {
                                scale = newScale
                                onZoomChanged(scale > 1f)
                            }
                            
                            if (scale > 1f) {
                                offset += panChange
                            } else {
                                offset = Offset.Zero
                            }
                            
                            event.changes.forEach { change ->
                                if (change.positionChanged()) {
                                    change.consume()
                                }
                            }
                        }
                    } while (event.changes.any { it.pressed })
                }
            }
    ) {
        localImagePath?.let { p ->
            AsyncImage(
                model = ImageRequest.Builder(LocalContext.current)
                    .data(p)
                    .crossfade(true)
                    .build(),
                contentDescription = null,
                contentScale = ContentScale.Fit,
                modifier = Modifier
                    .fillMaxSize()
                    .graphicsLayer(
                        scaleX = scale,
                        scaleY = scale,
                        translationX = offset.x,
                        translationY = offset.y
                    )
            )
        }

        if (isLoading) {
            CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = Color.White)
        }
    }
}
