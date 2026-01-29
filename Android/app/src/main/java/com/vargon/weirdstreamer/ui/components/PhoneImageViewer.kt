package com.vargon.weirdstreamer.ui.components

import androidx.activity.compose.BackHandler
import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectTransformGestures
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.delay
import java.io.File

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PhoneImageViewer(viewModel: MediaViewModel, initialPath: String, onBack: (String) -> Unit) {
    val context = LocalContext.current
    val images = remember { viewModel.getImagesInCurrentFolder() }
    var currentIndex by remember { mutableIntStateOf(images.indexOf(initialPath).coerceAtLeast(0)) }
    
    var scale by remember { mutableFloatStateOf(1f) }
    var offset by remember { mutableStateOf(androidx.compose.ui.geometry.Offset.Zero) }
    
    val currentPath = images.getOrNull(currentIndex) ?: initialPath
    val fileName = remember(currentPath) { viewModel.getDisplayName(currentPath) }

    var localImagePath by remember { mutableStateOf<String?>(null) }
    var tempFile by remember { mutableStateOf<File?>(null) }
    var isLoading by remember { mutableStateOf(true) }

    LaunchedEffect(currentPath) {
        isLoading = true
        tempFile?.delete()
        tempFile = null
        
        val extension = currentPath.substringBefore('?').substringAfterLast('.', "").lowercase()
        if (extension == "heic" || extension == "heif") {
            val file = viewModel.downloadToTempFile(currentPath, context, ".$extension")
            if (file != null) {
                tempFile = file
                localImagePath = file.absolutePath
            } else {
                localImagePath = currentPath
            }
        } else {
            localImagePath = currentPath
        }
        isLoading = false
    }

    DisposableEffect(Unit) {
        onDispose { tempFile?.delete() }
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
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .pointerInput(Unit) {
                    detectTransformGestures { _, pan, zoom, _ ->
                        scale = (scale * zoom).coerceIn(1f, 5f)
                        offset += pan
                    }
                }
        ) {
            localImagePath?.let { path ->
                AsyncImage(
                    model = ImageRequest.Builder(LocalContext.current)
                        .data(path)
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
}
