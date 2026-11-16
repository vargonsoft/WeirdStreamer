package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntSize
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.ConcurrentLinkedQueue
import java.util.concurrent.Executors
import kotlin.math.roundToInt

/**
 * An ASS/SSA track drawn by libass ([AssRenderer]), styled, positioned and animated, over the
 * picture: [position] is the file's time (ms), [videoAspect] the picture's shape; [modifier] can
 * give it the picture's own transform (zoom, rotation), to stay on it. The script and
 * the video's fonts come from [ServerSubtitles]. Each frame the position is checked; libass draws
 * on its own thread into the back of two bitmaps, which is shown when the subtitles changed.
 */
@Composable
fun AssSubtitle(subtitles: ServerSubtitles, position: () -> Long, videoAspect: () -> Float,
                modifier: Modifier = Modifier.fillMaxSize()) {
    val currentPosition by rememberUpdatedState(position)
    val currentAspect by rememberUpdatedState(videoAspect)
    var area by remember { mutableStateOf(IntSize.Zero) }
    var shown by remember { mutableStateOf<Pair<ImageBitmap, IntOffset>?>(null) }

    Canvas(modifier.onSizeChanged { area = it }) {
        shown?.let { (image, at) -> drawImage(image, dstOffset = at, dstSize = IntSize(image.width, image.height)) }
    }

    LaunchedEffect(subtitles, subtitles.selected, area) {
        shown = null
        if (area.width == 0 || area.height == 0) return@LaunchedEffect
        val thread = Executors.newSingleThreadExecutor { Thread(it, "AssRenderer") }.asCoroutineDispatcher()
        val renderer = withContext(thread) { runCatching { AssRenderer.create() }.getOrNull() }
        if (renderer == null) {
            thread.close()
            return@LaunchedEffect
        }
        try {
            val fonts = subtitles.videoFonts()
            withContext(thread) {
                fonts.forEach { (name, data) -> renderer.addFont(name, data) }
                subtitles.cjkFontsSoFar().forEach { renderer.addFont("cjk.ttf", it) }
            }
            // CJK subsets made while this renderer runs (their fetches run beside the drawing).
            val madeLater = ConcurrentLinkedQueue<ByteArray>()
            var version = -1
            var size = IntSize.Zero
            val bitmaps = arrayOfNulls<Bitmap>(2)
            var back = 0
            while (true) {
                withFrameNanos { }
                // The picture's part of the area (inside any bars), as the video's shape gives it.
                val aspect = currentAspect().takeIf { it > 0 } ?: (16f / 9)
                val width = minOf(area.width.toFloat(), area.height * aspect)
                val picture = IntSize(width.roundToInt(), (width / aspect).roundToInt())
                val at = IntOffset((area.width - picture.width) / 2, (area.height - picture.height) / 2)
                val timeMs = currentPosition() - subtitles.offsetMs
                val v = subtitles.assVersion
                val script = if (v != version) subtitles.assScript() else null
                if (script != null) launch { subtitles.fetchCjkFont(script)?.let { madeLater.add(it) } }
                val drawn = withContext(thread) {
                    var force = false
                    if (script != null) {
                        renderer.setScript(script)
                        version = v
                        force = true
                    }
                    while (true) {
                        val font = madeLater.poll() ?: break
                        renderer.addFont("cjk.ttf", font)
                        force = true
                    }
                    if (picture != size) {
                        size = picture
                        renderer.setSize(picture.width, picture.height, (1080 * aspect).roundToInt(), 1080)
                        // New ones (the old may still be on screen: left to the garbage collector).
                        for (i in bitmaps.indices) bitmaps[i] = Bitmap.createBitmap(picture.width, picture.height, Bitmap.Config.ARGB_8888)
                        force = true
                    }
                    val bitmap = bitmaps[back]!!
                    if (renderer.render(timeMs, bitmap, force)) bitmap else null
                }
                if (drawn != null) {
                    shown = drawn.asImageBitmap() to at
                    back = 1 - back
                }
            }
        } finally {
            shown = null
            withContext(NonCancellable + thread) { renderer.close() }
            thread.close()
        }
    }
}
