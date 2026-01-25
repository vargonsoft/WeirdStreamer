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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.key.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.SmbViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import java.io.File

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun PdfViewer(viewModel: SmbViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    var tempFile by remember { mutableStateOf<File?>(null) }
    var pdfRenderer by remember { mutableStateOf<PdfRenderer?>(null) }
    var pfd by remember { mutableStateOf<ParcelFileDescriptor?>(null) }
    var pageCount by remember { mutableIntStateOf(0) }
    var currentPageIndex by remember { mutableIntStateOf(0) }
    var currentPageBitmap by remember { mutableStateOf<Bitmap?>(null) }
    
    var zoomLevel by remember { mutableFloatStateOf(1f) }
    var panX by remember { mutableFloatStateOf(0f) }
    var panY by remember { mutableFloatStateOf(0f) }
    var isZoomMode by remember { mutableStateOf(false) }
    
    var showOverlays by remember { mutableStateOf(false) }
    var overlayTimerTrigger by remember { mutableLongStateOf(0L) }
    var loopFeedback by remember { mutableStateOf<Pair<Boolean, Long>?>(null) }
    var isLoading by remember { mutableStateOf(true) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    
    val focusRequester = remember { FocusRequester() }
    val fileName = remember(initialPath) {
        initialPath.substringBefore('?').removeSuffix("/").substringAfterLast('/')
    }

    LaunchedEffect(initialPath) {
        isLoading = true
        errorMessage = null
        val file = viewModel.downloadToTempFile(initialPath, context)
        if (file != null) {
            tempFile = file
            try {
                val fd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
                pfd = fd
                val renderer = PdfRenderer(fd)
                pdfRenderer = renderer
                pageCount = renderer.pageCount
                currentPageIndex = 0
            } catch (e: Exception) {
                errorMessage = "Failed to open PDF: ${e.localizedMessage}"
            }
        } else {
            errorMessage = "Failed to download PDF"
        }
        isLoading = false
    }

    LaunchedEffect(currentPageIndex, pdfRenderer) {
        val renderer = pdfRenderer ?: return@LaunchedEffect
        if (pageCount > 0) {
            isLoading = true
            overlayTimerTrigger = System.currentTimeMillis()
            withContext(Dispatchers.IO) {
                try {
                    renderer.openPage(currentPageIndex).use { page ->
                        val width = (page.width * 1.5f).toInt()
                        val height = (page.height * 1.5f).toInt()
                        val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
                        val canvas = Canvas(bitmap)
                        canvas.drawColor(android.graphics.Color.WHITE)
                        page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                        currentPageBitmap = bitmap
                        errorMessage = null // Clear any previous transient errors on success
                    }
                } catch (e: Exception) {
                    errorMessage = "Failed to render page: ${e.localizedMessage}"
                }
            }
            isLoading = false
        }
    }

    LaunchedEffect(errorMessage) {
        if (errorMessage != null && errorMessage!!.contains("Current page not closed")) {
            delay(3000)
            errorMessage = null
        }
    }

    LaunchedEffect(overlayTimerTrigger) {
        if (overlayTimerTrigger > 0) {
            showOverlays = true
            delay(1500)
            showOverlays = false
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            pdfRenderer?.close()
            pfd?.close()
            tempFile?.delete()
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
        if (pageCount > 0) {
            overlayTimerTrigger = System.currentTimeMillis()
            if (pageCount > 1) {
                val oldIndex = currentPageIndex
                if (forward) {
                    currentPageIndex = (currentPageIndex + 1) % pageCount
                    if (currentPageIndex < oldIndex) loopFeedback = true to System.currentTimeMillis()
                } else {
                    currentPageIndex = (currentPageIndex - 1 + pageCount) % pageCount
                    if (currentPageIndex > oldIndex) loopFeedback = false to System.currentTimeMillis()
                }
                resetZoom()
            }
        }
    }

    val handleBack = {
        if (isZoomMode) {
            resetZoom()
        } else {
            onBack(initialPath)
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
                                onBack(initialPath)
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
        currentPageBitmap?.let { bitmap ->
            Image(
                bitmap = bitmap.asImageBitmap(),
                contentDescription = null,
                contentScale = if (isZoomMode) ContentScale.None else ContentScale.Fit,
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
            Box(
                modifier = Modifier
                    .align(Alignment.Center)
                    .background(Color.Black.copy(alpha = 0.5f), RoundedCornerShape(8.dp))
                    .padding(24.dp),
                contentAlignment = Alignment.Center
            ) {
                val progress = viewModel.downloadProgress
                if (progress != null) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        CircularProgressIndicator(
                            progress = { progress },
                            color = Color.White
                        )
                        Spacer(modifier = Modifier.height(8.dp))
                        Text(
                            text = "${(progress * 100).toInt()}%",
                            color = Color.White,
                            style = MaterialTheme.typography.labelLarge
                        )
                    }
                } else {
                    CircularProgressIndicator(color = Color.White)
                }
            }
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
            visible = showOverlays && !isZoomMode,
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

        // Page Number Indicator
        AnimatedVisibility(
            visible = showOverlays && pageCount > 0 && !isZoomMode,
            enter = fadeIn() + slideInVertically { it },
            exit = fadeOut() + slideOutVertically { it },
            modifier = Modifier.align(Alignment.BottomCenter)
        ) {
            Text(
                text = "${currentPageIndex + 1} / $pageCount",
                modifier = Modifier
                    .padding(bottom = 32.dp)
                    .background(Color.Black.copy(alpha = 0.5f), RoundedCornerShape(16.dp))
                    .padding(horizontal = 16.dp, vertical = 8.dp),
                style = MaterialTheme.typography.labelLarge,
                color = Color.White
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
