package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.*
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.R
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import java.util.*
import kotlin.math.abs

@Composable
fun FilenameOverlay(
    show: Boolean,
    filename: String,
    modifier: Modifier = Modifier
) {
    AnimatedVisibility(
        visible = show,
        enter = slideInVertically { -it } + fadeIn(),
        exit = slideOutVertically { -it } + fadeOut(),
        modifier = modifier
    ) {
        Text(
            text = filename,
            style = MaterialTheme.typography.headlineMedium,
            color = Color.White,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier
                .padding(32.dp)
                .background(Color.Black.copy(alpha = 0.4f), RoundedCornerShape(8.dp))
                .padding(horizontal = 16.dp, vertical = 8.dp)
        )
    }
}

@Composable
fun SkipFeedbackOverlay(
    feedback: Pair<Int, Boolean>?,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        val skipIsForward = feedback?.second == true
        AnimatedVisibility(
            visible = feedback != null && feedback.first != 0,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier.align(if (skipIsForward) Alignment.CenterEnd else Alignment.CenterStart)
        ) {
            val value = feedback?.first ?: 0
            if (value != 0) {
                val absValue = abs(value)
                val text = if (absValue >= 60) {
                    val min = absValue / 60
                    val sec = absValue % 60
                    val timeStr = String.format(Locale.US, "%d:%02d", min, sec)
                    if (value > 0) "+$timeStr" else "-$timeStr"
                } else {
                    if (value > 0) "+$absValue" else "-$absValue"
                }
                Text(
                    text = text,
                    style = MaterialTheme.typography.displayLarge,
                    color = Color.White.copy(alpha = 0.7f),
                    modifier = Modifier.padding(horizontal = 64.dp)
                )
            }
        }
    }
}

@Composable
fun SpeedFeedbackOverlay(
    speed: Float?,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        AnimatedVisibility(
            visible = speed != null,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier.align(Alignment.Center)
        ) {
            if (speed != null) {
                Text(
                    text = "${speed}x",
                    style = MaterialTheme.typography.displayLarge,
                    color = Color.White.copy(alpha = 0.7f),
                    modifier = Modifier.padding(64.dp)
                )
            }
        }
    }
}

@Composable
fun FrameFeedbackOverlay(
    feedback: Pair<Boolean, Long>?,
    modifier: Modifier = Modifier
) {
    Box(modifier = modifier.fillMaxSize()) {
        AnimatedVisibility(
            visible = feedback != null,
            enter = fadeIn() + scaleIn(),
            exit = fadeOut() + scaleOut(),
            modifier = Modifier.align(Alignment.Center)
        ) {
            if (feedback != null) {
                Icon(
                    painter = painterResource(id = if (feedback.first) R.drawable.ic_frame_forward else R.drawable.ic_frame_backward),
                    contentDescription = null,
                    tint = Color.White.copy(alpha = 0.7f),
                    modifier = Modifier.size(128.dp)
                )
            }
        }
    }
}

@Composable
fun TrickplayOverlay(
    visible: Boolean,
    trickplayInfo: TrickplayInfo?,
    playbackPosition: Long,
    selectedTrickplayIndex: Int,
    scrollRequestIndex: Int,
    updateInteraction: () -> Unit,
    modifier: Modifier = Modifier
) {
    val trickplayListState = rememberLazyListState()

    AnimatedVisibility(
        visible = visible && trickplayInfo != null,
        enter = slideInVertically { it } + fadeIn(),
        exit = slideOutVertically { it } + fadeOut(),
        modifier = modifier.padding(bottom = 160.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .background(Color.Black.copy(alpha = 0.8f))
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = if (selectedTrickplayIndex != -1 && trickplayInfo != null) 
                    formatTime(trickplayInfo.frames[selectedTrickplayIndex].time.toLong() * 1000) 
                    else "",
                style = MaterialTheme.typography.headlineSmall,
                color = Color.White,
                modifier = Modifier.padding(bottom = 16.dp)
            )

            BoxWithConstraints(modifier = Modifier.fillMaxWidth()) {
                val itemWidth = 160.dp
                val contentPadding = maxWidth / 2 - itemWidth / 2

                LazyRow(
                    state = trickplayListState,
                    modifier = Modifier.fillMaxWidth().height(120.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    contentPadding = PaddingValues(horizontal = contentPadding)
                ) {
                    if (trickplayInfo != null) {
                        itemsIndexed(trickplayInfo.frames) { index, frame ->
                            Box(
                                modifier = Modifier
                                    .width(itemWidth)
                                    .fillMaxHeight()
                                    .clip(RoundedCornerShape(8.dp))
                                    .border(
                                        width = if (selectedTrickplayIndex == index) 4.dp else 0.dp,
                                        color = if (selectedTrickplayIndex == index) Color.White else Color.Transparent,
                                        shape = RoundedCornerShape(8.dp)
                                    )
                            ) {
                                AsyncImage(
                                    model = ImageRequest.Builder(LocalContext.current)
                                        .data(frame.url)
                                        .crossfade(true)
                                        .build(),
                                    contentDescription = null,
                                    contentScale = ContentScale.Crop,
                                    modifier = Modifier.fillMaxSize()
                                )
                                
                                Text(
                                    text = formatTime(frame.time.toLong() * 1000),
                                    style = MaterialTheme.typography.labelSmall,
                                    color = Color.White,
                                    modifier = Modifier
                                        .align(Alignment.BottomEnd)
                                        .background(Color.Black.copy(alpha = 0.6f))
                                        .padding(horizontal = 4.dp, vertical = 2.dp)
                                )
                            }
                        }
                    }
                }
                
                LaunchedEffect(visible, trickplayInfo, scrollRequestIndex) {
                    if (visible && scrollRequestIndex >= 0) {
                        trickplayListState.scrollToItem(scrollRequestIndex)
                    }
                }
            }
        }
    }
}
