package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.GridView
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import java.io.File

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhonePdfViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    var tempFile by remember { mutableStateOf<File?>(null) }
    var pdfRenderer by remember { mutableStateOf<PdfRenderer?>(null) }
    var pageCount by remember { mutableIntStateOf(0) }
    var currentPageIndex by remember { mutableIntStateOf(0) }
    var currentPageBitmap by remember { mutableStateOf<Bitmap?>(null) }
    var isThumbnailView by remember { mutableStateOf(false) }
    var isLoading by remember { mutableStateOf(true) }
    
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(androidx.compose.ui.geometry.Offset.Zero) }
    
    val rendererMutex = remember { Mutex() }

    LaunchedEffect(initialPath) {
        val file = viewModel.downloadToTempFile(initialPath, context)
        if (file != null) {
            tempFile = file
            val pfd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
            val renderer = PdfRenderer(pfd)
            pdfRenderer = renderer
            pageCount = renderer.pageCount
        }
        isLoading = false
    }

    LaunchedEffect(currentPageIndex, pdfRenderer, isThumbnailView) {
        if (pdfRenderer != null && !isThumbnailView) {
            isLoading = true
            withContext(Dispatchers.Default) {
                rendererMutex.withLock {
                    pdfRenderer!!.openPage(currentPageIndex).use { page ->
                        val bitmap = Bitmap.createBitmap(page.width * 2, page.height * 2, Bitmap.Config.ARGB_8888)
                        val canvas = Canvas(bitmap)
                        canvas.drawColor(android.graphics.Color.WHITE)
                        page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                        currentPageBitmap = bitmap
                    }
                }
            }
            isLoading = false
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            pdfRenderer?.close()
            tempFile?.delete()
        }
    }

    Scaffold(
        topBar = {
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
        },
        bottomBar = {
            if (!isThumbnailView && pageCount > 0) {
                BottomAppBar {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        TextButton(onClick = { if (currentPageIndex > 0) currentPageIndex-- }, enabled = currentPageIndex > 0) {
                            Text("Previous")
                        }
                        Text("${currentPageIndex + 1} / $pageCount")
                        TextButton(onClick = { if (currentPageIndex < pageCount - 1) currentPageIndex++ }, enabled = currentPageIndex < pageCount - 1) {
                            Text("Next")
                        }
                    }
                }
            }
        }
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
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
                                .background(MaterialTheme.colorScheme.surfaceVariant)
                                .clickable {
                                    currentPageIndex = index
                                    isThumbnailView = false
                                },
                            contentAlignment = Alignment.Center
                        ) {
                            Text((index + 1).toString())
                        }
                    }
                }
            } else {
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .pointerInput(Unit) {
                            detectTransformGestures { _, pan, zoom, _ ->
                                scale = (scale * zoom).coerceIn(1f, 5f)
                                offset += pan
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
                }
            }

            if (isLoading) {
                CircularProgressIndicator(modifier = Modifier.align(Alignment.Center))
            }
        }
    }
}
