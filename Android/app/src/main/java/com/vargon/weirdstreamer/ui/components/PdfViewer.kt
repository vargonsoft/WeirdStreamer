package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun PdfViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val configuration = LocalConfiguration.current
    val screenHeight = configuration.screenHeightDp.dp
    val scope = rememberCoroutineScope()
    
    var tempFile by remember { mutableStateOf<File?>(null) }
    var pdfRenderer by remember { mutableStateOf<PdfRenderer?>(null) }
    var pfd by remember { mutableStateOf<ParcelFileDescriptor?>(null) }
    var pageCount by remember { mutableIntStateOf(0) }
    var currentPageIndex by remember { mutableIntStateOf(0) }
    var currentPageBitmap by remember { mutableStateOf<Bitmap?>(null) }
    var nextPageBitmap by remember { mutableStateOf<Bitmap?>(null) }
    
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    
    var rotation by remember { mutableFloatStateOf(0f) }
    var isTwoPageView by remember { mutableStateOf(false) }
    var isThumbnailView by remember { mutableStateOf(false) }
    
    var showOverlays by remember { mutableStateOf(false) }
    var overlayTimerTrigger by remember { mutableLongStateOf(0L) }
    var loopFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    
    val rendererMutex = remember { Mutex() }
    val thumbCache = remember { mutableStateMapOf<Int, Bitmap>() }
    var retryTrigger by remember { mutableIntStateOf(0) }
    
    val mainFocusRequester = remember { FocusRequester() }
    val gridState = rememberLazyGridState()
    val fileName = remember(initialPath) { viewModel.getDisplayName(initialPath) }

    LaunchedEffect(initialPath) {
        isLoading = true; errorMessage = null
        val file = viewModel.downloadToTempFile(initialPath, context)
        if (file != null) {
            tempFile = file
            try {
                val fd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
                pfd = fd; val renderer = PdfRenderer(fd)
                pdfRenderer = renderer; pageCount = renderer.pageCount; currentPageIndex = 0
            } catch (e: Exception) { errorMessage = "Failed to open PDF: ${e.localizedMessage}" }
        } else { errorMessage = "Failed to download PDF" }
        isLoading = false
    }

    LaunchedEffect(currentPageIndex, isTwoPageView, pdfRenderer, retryTrigger, isThumbnailView) {
        val renderer = pdfRenderer ?: return@LaunchedEffect
        val targetIndex = currentPageIndex
        val targetTwoPage = isTwoPageView
        if (pageCount > 0 && !isThumbnailView) {
            delay(150); isLoading = true; overlayTimerTrigger = System.currentTimeMillis()
            withContext(Dispatchers.Default) {
                try {
                    if (targetIndex != currentPageIndex) return@withContext
                    val firstBitmap = rendererMutex.withLock {
                        renderer.openPage(targetIndex).use { page ->
                            val bitmap = Bitmap.createBitmap((page.width * 1.5f).toInt(), (page.height * 1.5f).toInt(), Bitmap.Config.ARGB_8888)
                            Canvas(bitmap).apply { drawColor(android.graphics.Color.WHITE); page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY) }
                            bitmap
                        }
                    }
                    val secondBitmap = if (targetTwoPage && targetIndex + 1 < pageCount) {
                        rendererMutex.withLock {
                            renderer.openPage(targetIndex + 1).use { page ->
                                val bitmap = Bitmap.createBitmap((page.width * 1.5f).toInt(), (page.height * 1.5f).toInt(), Bitmap.Config.ARGB_8888)
                                Canvas(bitmap).apply { drawColor(android.graphics.Color.WHITE); page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY) }
                                bitmap
                            }
                        }
                    } else null
                    if (targetIndex == currentPageIndex && targetTwoPage == isTwoPageView) { currentPageBitmap = firstBitmap; nextPageBitmap = secondBitmap; errorMessage = null }
                } catch (e: Exception) {
                    if (targetIndex == currentPageIndex) {
                        val msg = e.localizedMessage ?: "Unknown error"
                        if (msg.contains("Current page not closed")) { delay(200); retryTrigger++ }
                        else { errorMessage = "Failed to render page: $msg" }
                    }
                }
            }
            if (targetIndex == currentPageIndex) isLoading = false
        }
    }

    LaunchedEffect(errorMessage) { if (errorMessage != null && errorMessage!!.contains("Current page not closed")) { delay(3000); errorMessage = null } }
    LaunchedEffect(overlayTimerTrigger) { if (overlayTimerTrigger > 0) { showOverlays = true; delay(1500); showOverlays = false } }
    DisposableEffect(Unit) { onDispose { pdfRenderer?.close(); pfd?.close(); tempFile?.delete(); thumbCache.values.forEach { it.recycle() }; thumbCache.clear() } }
    LaunchedEffect(loopFeedback) { if (loopFeedback != null) { delay(1500); loopFeedback = null } }

    val resetZoom = { zoomLevel = 1f; panX = 0f; panY = 0f; isZoomMode = false }
    val navigate = { forward: Boolean ->
        if (pageCount > 0) {
            overlayTimerTrigger = System.currentTimeMillis()
            if (pageCount > 1) {
                val old = currentPageIndex
                if (forward) { currentPageIndex = (currentPageIndex + 1) % pageCount; if (currentPageIndex < old) loopFeedback = true to System.currentTimeMillis() }
                else { currentPageIndex = (currentPageIndex - 1 + pageCount) % pageCount; if (currentPageIndex > old) loopFeedback = false to System.currentTimeMillis() }
                resetZoom()
            }
        }
    }

    val zoomOut = {
        if (isZoomMode) { if (zoomLevel > 2f) zoomLevel /= 2f else resetZoom() }
        else if (isThumbnailView) isThumbnailView = false
        else if (isTwoPageView) isTwoPageView = false
        else onBack(initialPath)
    }

    BackHandler { zoomOut() }

    Box(
        modifier = Modifier.fillMaxSize().background(Color.Black)
            .then(if (isZoomMode) Modifier.border(2.dp, Color.White) else Modifier)
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown) {
                    when (event.key) {
                        Key.DirectionCenter, Key.Enter -> {
                            if (!isThumbnailView) {
                                if (!isZoomMode) { isZoomMode = true; if (zoomLevel == 1f) zoomLevel = 2f }
                                else if (zoomLevel < 256f) zoomLevel *= 2f
                                true
                            } else false
                        }
                        Key.Backspace -> { zoomOut(); true }
                        Key.DirectionLeft -> { if (isZoomMode) { panX += 0.1f / zoomLevel; true } else if (!isThumbnailView) { navigate(false); true } else false }
                        Key.DirectionRight -> { if (isZoomMode) { panX -= 0.1f / zoomLevel; true } else if (!isThumbnailView) { navigate(true); true } else false }
                        Key.DirectionUp -> {
                            if (isZoomMode) { panY += 0.1f / zoomLevel; true } else if (!isThumbnailView) {
                                if (isTwoPageView) isThumbnailView = true else isTwoPageView = true
                                overlayTimerTrigger = System.currentTimeMillis(); true
                            } else false
                        }
                        Key.DirectionDown -> {
                            if (isZoomMode) { panY -= 0.1f / zoomLevel; true } else if (!isThumbnailView) {
                                if (isTwoPageView) isTwoPageView = false else rotation = (rotation + 90f) % 360f
                                overlayTimerTrigger = System.currentTimeMillis(); true
                            } else false
                        }
                        else -> false
                    }
                } else false
            }.focusRequester(mainFocusRequester).focusable()
    ) {
        if (isThumbnailView) {
            val thumbHeight = screenHeight * 0.4f
            val itemMinWidth = 150.dp
            val spacing = 16.dp
            val gridPadding = 16.dp * 2
            val columnCount = ((configuration.screenWidthDp.dp - gridPadding + spacing) / (itemMinWidth + spacing)).toInt().coerceAtLeast(1)

            PdfThumbnailGrid(
                pageCount = pageCount,
                currentPageIndex = currentPageIndex,
                thumbHeight = thumbHeight,
                gridState = gridState,
                columnCount = columnCount,
                thumbCache = thumbCache,
                onPageSelected = { idx -> currentPageIndex = idx; isThumbnailView = false; isTwoPageView = false },
                onRenderThumbnail = { index ->
                    if (pdfRenderer != null) {
                        scope.launch {
                            withContext(Dispatchers.Default) {
                                try {
                                    val bitmap = rendererMutex.withLock {
                                        pdfRenderer!!.openPage(index).use { page ->
                                            val scale = 0.3f
                                            val b = Bitmap.createBitmap((page.width * scale).toInt(), (page.height * scale).toInt(), Bitmap.Config.ARGB_8888)
                                            Canvas(b).apply { drawColor(android.graphics.Color.WHITE); page.render(b, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY) }
                                            b
                                        }
                                    }
                                    thumbCache[index] = bitmap
                                } catch (_: Exception) {}
                            }
                        }
                    }
                }
            )
        } else {
            Box(
                modifier = Modifier.fillMaxSize().clipToBounds()
                    .graphicsLayer(scaleX = zoomLevel, scaleY = zoomLevel, translationX = panX * 1000f * zoomLevel, translationY = panY * 1000f * zoomLevel, rotationZ = if (!isTwoPageView) rotation else 0f),
                contentAlignment = Alignment.Center
            ) {
                if (isTwoPageView) {
                    Row(modifier = Modifier.fillMaxSize(), horizontalArrangement = Arrangement.spacedBy(0.dp, Alignment.CenterHorizontally), verticalAlignment = Alignment.CenterVertically) {
                        Box(modifier = Modifier.weight(1f).fillMaxHeight(), contentAlignment = Alignment.CenterEnd) { currentPageBitmap?.let { Image(bitmap = it.asImageBitmap(), contentDescription = null, contentScale = ContentScale.Fit, modifier = Modifier.fillMaxHeight()) } }
                        Box(modifier = Modifier.weight(1f).fillMaxHeight(), contentAlignment = Alignment.CenterStart) { nextPageBitmap?.let { Image(bitmap = it.asImageBitmap(), contentDescription = null, contentScale = ContentScale.Fit, modifier = Modifier.fillMaxHeight()) } }
                    }
                } else {
                    currentPageBitmap?.let { Image(bitmap = it.asImageBitmap(), contentDescription = null, contentScale = if (isZoomMode) ContentScale.None else ContentScale.Fit, modifier = Modifier.fillMaxSize()) }
                }
            }
        }

        if (isLoading && !isThumbnailView) {
            Box(modifier = Modifier.align(Alignment.Center).background(Color.Black.copy(alpha = 0.5f), RoundedCornerShape(8.dp)).padding(24.dp), contentAlignment = Alignment.Center) {
                val progress = viewModel.downloadProgress
                if (progress != null) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        CircularProgressIndicator(progress = { progress }, color = MaterialTheme.colorScheme.primary)
                        Spacer(modifier = Modifier.height(8.dp))
                        Text(text = "${(progress * 100).toInt()}%", color = Color.White, style = MaterialTheme.typography.labelLarge)
                    }
                } else CircularProgressIndicator(color = MaterialTheme.colorScheme.primary)
            }
        }

        if (errorMessage != null && !isThumbnailView) {
            Column(modifier = Modifier.align(Alignment.Center).padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Text("Error", style = MaterialTheme.typography.headlineMedium, color = Color.Red)
                Text(errorMessage!!, style = MaterialTheme.typography.bodyMedium, color = Color.White, textAlign = TextAlign.Center)
            }
        }

        PdfOverlays(
            show = showOverlays,
            fileName = fileName,
            rotation = rotation,
            isTwoPageView = isTwoPageView,
            pageCount = pageCount,
            currentPageIndex = currentPageIndex,
            isZoomMode = isZoomMode,
            loopFeedback = loopFeedback
        )
    }

    LaunchedEffect(Unit) { mainFocusRequester.requestFocus() }
}
