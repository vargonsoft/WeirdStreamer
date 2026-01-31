package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.*
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.core.view.WindowCompat
import android.app.Activity
import android.os.Build
import android.view.WindowInsets
import android.view.WindowInsetsController
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.ui.input.pointer.pointerInput
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File
import androidx.compose.ui.unit.dp

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhonePdfViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val activity = context as? Activity
    var immersiveMode by remember { mutableStateOf(false) }

    // Toggle immersive mode on tap
    fun setImmersiveMode(enabled: Boolean) {
        val window = activity?.window
        if (window != null) {
            WindowCompat.setDecorFitsSystemWindows(window, !enabled)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
                window.insetsController?.let { controller ->
                    if (enabled) {
                        controller.hide(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                        controller.systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                    } else {
                        controller.show(WindowInsets.Type.statusBars() or WindowInsets.Type.navigationBars())
                    }
                }
            } else {
                @Suppress("DEPRECATION")
                window.decorView.systemUiVisibility = if (enabled) {
                    android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY or
                    android.view.View.SYSTEM_UI_FLAG_FULLSCREEN or
                    android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                } else {
                    android.view.View.SYSTEM_UI_FLAG_VISIBLE
                }
            }
        }
    }

    DisposableEffect(immersiveMode) {
        setImmersiveMode(immersiveMode)
        onDispose { setImmersiveMode(false) }
    }

    val scope = rememberCoroutineScope()
    var tempFile by remember { mutableStateOf<File?>(null) }
    var pdfRenderer by remember { mutableStateOf<PdfRenderer?>(null) }
    var pageCount by remember { mutableIntStateOf(0) }
    var isThumbnailView by remember { mutableStateOf(false) }
    var isLoading by remember { mutableStateOf(true) }
    
    val rendererMutex = remember { Mutex() }
    val pagerState = rememberPagerState { pageCount }
    var isZoomed by remember { mutableStateOf(false) }

    LaunchedEffect(initialPath) {
        val file = viewModel.downloadToTempFile(initialPath, context)
        if (file != null) {
            tempFile = file
            try {
                val pfd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
                pdfRenderer = PdfRenderer(pfd)
                pageCount = pdfRenderer!!.pageCount
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
        isLoading = false
    }

    LaunchedEffect(pagerState.currentPage) {
        isZoomed = false
    }

    DisposableEffect(Unit) {
        onDispose {
            pdfRenderer?.close()
            tempFile?.delete()
        }
    }

    BackHandler {
        if (isThumbnailView) {
            isThumbnailView = false
        } else {
            onBack(initialPath)
        }
    }

    Scaffold(
        topBar = {
            if (!immersiveMode) {
                TopAppBar(
                    title = { Text(viewModel.getDisplayName(initialPath), maxLines = 1) },
                    navigationIcon = {
                        IconButton(onClick = { if (isThumbnailView) isThumbnailView = false else onBack(initialPath) }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                        }
                    },
                    actions = {
                        IconButton(onClick = { isThumbnailView = !isThumbnailView }) {
                            Icon(Icons.Default.GridView, contentDescription = "Thumbnails")
                        }
                    }
                )
            }
        },
        bottomBar = {
            if (!isThumbnailView && pageCount > 0) {
                BottomAppBar {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        TextButton(
                            onClick = { scope.launch { pagerState.animateScrollToPage(pagerState.currentPage - 1) } }, 
                            enabled = pagerState.currentPage > 0
                        ) {
                            Text("Previous")
                        }
                        Text("${pagerState.currentPage + 1} / $pageCount")
                        TextButton(
                            onClick = { scope.launch { pagerState.animateScrollToPage(pagerState.currentPage + 1) } }, 
                            enabled = pagerState.currentPage < pageCount - 1
                        ) {
                            Text("Next")
                        }
                    }
                }
            }
        }
    ) { padding ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .pointerInput(Unit) {
                    detectTapGestures(onTap = { immersiveMode = !immersiveMode })
                }
                .padding(padding)
        ) {
            if (isThumbnailView) {
                LazyVerticalGrid(
                    columns = GridCells.Adaptive(100.dp),
                    contentPadding = PaddingValues(8.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(pageCount) { index ->
                        Box(
                            modifier = Modifier
                                .aspectRatio(0.7f)
                                .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(8.dp))
                                .clip(RoundedCornerShape(8.dp))
                                .clickable {
                                    scope.launch {
                                        pagerState.scrollToPage(index)
                                        isThumbnailView = false
                                    }
                                },
                            contentAlignment = Alignment.Center
                        ) {
                            Text((index + 1).toString())
                        }
                    }
                }
            } else {
                HorizontalPager(
                    state = pagerState,
                    modifier = Modifier.fillMaxSize(),
                    userScrollEnabled = !isZoomed,
                    beyondViewportPageCount = 1
                ) { pageIndex ->
                    ZoomablePdfPage(
                        pdfRenderer = pdfRenderer,
                        pageIndex = pageIndex,
                        rendererMutex = rendererMutex,
                        onZoomChanged = { zoomed ->
                            if (pageIndex == pagerState.currentPage) {
                                isZoomed = zoomed
                            }
                        }
                    )
                }
            }

            if (isLoading) {
                CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = MaterialTheme.colorScheme.primary)
            }
        }
    }
}

@Composable
fun ZoomablePdfPage(
    pdfRenderer: PdfRenderer?,
    pageIndex: Int,
    rendererMutex: Mutex,
    onZoomChanged: (Boolean) -> Unit
) {
    var currentPageBitmap by remember { mutableStateOf<Bitmap?>(null) }
    var isLoading by remember { mutableStateOf(false) }
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(Offset.Zero) }

    val currentOnZoomChanged by rememberUpdatedState(onZoomChanged)

    LaunchedEffect(pdfRenderer, pageIndex) {
        if (pdfRenderer != null) {
            isLoading = true
            withContext(Dispatchers.Default) {
                rendererMutex.withLock {
                    try {
                        pdfRenderer.openPage(pageIndex).use { page ->
                            val bitmap = Bitmap.createBitmap(page.width * 2, page.height * 2, Bitmap.Config.ARGB_8888)
                            val canvas = Canvas(bitmap)
                            canvas.drawColor(android.graphics.Color.WHITE)
                            page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                            currentPageBitmap = bitmap
                        }
                    } catch (e: Exception) {
                        e.printStackTrace()
                    }
                }
            }
            isLoading = false
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .pointerInput(Unit) {
                awaitEachGesture {
                    awaitFirstDown(requireUnconsumed = false)
                    do {
                        val event = awaitPointerEvent()
                        val isPinch = event.changes.size > 1
                        if (scale > 1f || isPinch) {
                            val zoomChange = event.calculateZoom()
                            val panChange = event.calculatePan()
                            val newScale = (scale * zoomChange).coerceIn(1f, 5f)
                            if (newScale != scale) {
                                scale = newScale
                                currentOnZoomChanged(scale > 1f)
                            }
                            if (scale > 1f) {
                                offset += panChange
                            } else {
                                offset = Offset.Zero
                            }
                            event.changes.forEach { change ->
                                if (change.position != change.previousPosition) {
                                    change.consume()
                                }
                            }
                        }
                    } while (event.changes.any { it.pressed })
                }
            }
    ) {
        currentPageBitmap?.let { bitmap ->
            Image(
                bitmap = bitmap.asImageBitmap(),
                contentDescription = null,
                modifier = Modifier
                    .fillMaxSize()
                    .graphicsLayer(
                        scaleX = scale,
                        scaleY = scale,
                        translationX = offset.x,
                        translationY = offset.y
                    ),
                contentScale = ContentScale.Fit
            )
        }

        if (isLoading) {
            CircularProgressIndicator(modifier = Modifier.align(Alignment.Center), color = MaterialTheme.colorScheme.primary)
        }
    }
}
