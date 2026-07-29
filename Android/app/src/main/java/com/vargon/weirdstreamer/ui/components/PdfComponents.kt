package com.vargon.weirdstreamer.ui.components

import android.util.Log
import android.graphics.Bitmap
import androidx.compose.animation.*
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.LazyGridState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.*
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.android.awaitFrame
import kotlinx.coroutines.launch

@Composable
fun PdfThumbnailGrid(
    pageCount: Int,
    currentPageIndex: Int,
    thumbHeight: Dp,
    gridState: LazyGridState,
    columnCount: Int,
    thumbCache: Map<Int, Bitmap>,
    onPageSelected: (Int) -> Unit,
    onRenderThumbnail: (Int) -> Unit,
    modifier: Modifier = Modifier
) {
    val focusRequesters = remember(pageCount) { List(pageCount) { FocusRequester() } }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) {
        gridState.scrollToItem(currentPageIndex)
        kotlinx.coroutines.delay(100)
        if (currentPageIndex in focusRequesters.indices) {
            focusRequesters[currentPageIndex].requestFocus()
        }
    }

    LazyVerticalGrid(
        columns = GridCells.Fixed(columnCount),
        state = gridState,
        modifier = modifier.fillMaxSize().padding(16.dp),
        contentPadding = PaddingValues(16.dp),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        items(pageCount) { index ->
            val thumbBitmap = thumbCache[index]

            LaunchedEffect(index) {
                if (thumbBitmap == null) {
                    onRenderThumbnail(index)
                }
            }

            Surface(
                onClick = { onPageSelected(index) },
                scale = ClickableSurfaceDefaults.scale(focusedScale = 1.05f),
                border = ClickableSurfaceDefaults.border(
                    focusedBorder = Border(
                        border = BorderStroke(4.dp, Color.White),
                        inset = 0.dp,
                        shape = MaterialTheme.shapes.medium
                    )
                ),
                modifier = Modifier
                    .height(thumbHeight)
                    .aspectRatio(0.75f)
                    .focusRequester(focusRequesters[index])
                    .onPreviewKeyEvent { event ->
                        if (event.type == KeyEventType.KeyDown) {
                            val targetIndex = when (event.key) {
                                Key.DirectionUp -> index - columnCount
                                Key.DirectionDown -> index + columnCount
                                Key.DirectionLeft -> index - 1
                                Key.DirectionRight -> index + 1
                                else -> null
                            }

                            if (targetIndex != null) {
                                if (targetIndex in 0 until pageCount) {
                                    Log.d("FocusNav", "PDF Mapping $index -> $targetIndex")
                                    scope.launch {
                                        gridState.scrollToItem(targetIndex)
                                        awaitFrame()
                                        focusRequesters[targetIndex].requestFocus()
                                    }
                                    return@onPreviewKeyEvent true
                                }
                            }
                        }
                        false
                    },
                shape = ClickableSurfaceDefaults.shape(shape = MaterialTheme.shapes.medium),
                colors = ClickableSurfaceDefaults.colors(
                    containerColor = Color.Transparent,
                    focusedContainerColor = Color.Transparent,
                    pressedContainerColor = Color.Transparent
                )
            ) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(10.dp)
                        .background(Color.DarkGray, MaterialTheme.shapes.small)
                        .clip(MaterialTheme.shapes.small)
                ) {
                    if (thumbBitmap != null) {
                        Image(
                            bitmap = thumbBitmap.asImageBitmap(),
                            contentDescription = null,
                            contentScale = ContentScale.Fit,
                            modifier = Modifier.fillMaxSize()
                        )
                    } else {
                        CircularProgressIndicator(color = WeirdStreamerTheme.spec.spinnerColor.takeOrElse { Color.Blue }, modifier = Modifier.size(24.dp))
                    }

                    Box(
                        modifier = Modifier
                            .align(Alignment.BottomEnd)
                            .padding(8.dp)
                            .background(Color.Black.copy(alpha = 0.6f), RoundedCornerShape(4.dp))
                            .padding(horizontal = 6.dp, vertical = 2.dp)
                    ) {
                        Text(
                            text = (index + 1).toString(),
                            color = Color.White,
                            fontSize = 12.sp,
                            fontWeight = FontWeight.Bold
                        )
                    }
                }
            }
        }
    }
}

@Composable
fun PdfOverlays(
    show: Boolean,
    fileName: String,
    rotation: Float,
    isTwoPageView: Boolean,
    pageCount: Int,
    currentPageIndex: Int,
    isZoomMode: Boolean,
    loopFeedback: Pair<Boolean, Long>?,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        // Filename Overlay
        AnimatedVisibility(
            visible = show && !isZoomMode,
            enter = fadeIn() + slideInVertically(),
            exit = fadeOut() + slideOutVertically(),
            modifier = Modifier.align(Alignment.TopCenter)
        ) {
            Text(
                text = fileName + if (rotation != 0f && !isTwoPageView) " (${rotation.toInt()}°)" else "",
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
            visible = show && pageCount > 0 && !isZoomMode,
            enter = fadeIn() + slideInVertically { it },
            exit = fadeOut() + slideOutVertically { it },
            modifier = Modifier.align(Alignment.BottomCenter)
        ) {
            val pageText = if (isTwoPageView && currentPageIndex + 1 < pageCount) {
                "${currentPageIndex + 1}-${currentPageIndex + 2} / $pageCount"
            } else {
                "${currentPageIndex + 1} / $pageCount"
            }
            Text(
                text = (if (isTwoPageView) "Two-Page View: " else "") + pageText,
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
}
