package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Folder
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.vector.rememberVectorPainter
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import coil.request.ImageRequest

/**
 * A folder's icon, square and as large as [modifier]'s box allows, with [url] (the server's pick:
 * the folder's thumbnail.png, else a photo or video from inside it) in its front; null draws the
 * plain folder at the same size. The icon's body is x 2-22 and y 6-20 of its 24 units, so the
 * picture sits in its lower part, as on the web.
 */
@Composable
fun FolderPreview(url: String?, tint: Color, modifier: Modifier = Modifier, requestSize: Int? = null) {
    val context = LocalContext.current
    val request = remember(url, requestSize) {
        ImageRequest.Builder(context).data(url).crossfade(false)
            .apply { requestSize?.let { size(it) } }
            .build()
    }
    BoxWithConstraints(modifier.aspectRatio(1f)) {
        Image(
            rememberVectorPainter(Icons.Default.Folder),
            contentDescription = null,
            modifier = Modifier.fillMaxSize(),
            contentScale = ContentScale.FillBounds,
            colorFilter = ColorFilter.tint(tint)
        )
        if (url != null) AsyncImage(
            model = request,
            contentDescription = null,
            contentScale = ContentScale.Crop,
            modifier = Modifier
                .offset(x = maxWidth * 0.14f, y = maxHeight * 0.34f)
                .size(maxWidth * 0.72f, maxHeight * 0.44f)
                .clip(RoundedCornerShape(minOf(maxWidth, maxHeight) * 0.04f))
        )
    }
}

private val filmBase = Color(0xFF111111)
private val filmHole = Color(0xD9F0ECE4)

/**
 * Sprocket strips down the left and right edges, telling a folder's videos from its photos.
 * Drawn behind the content, so it goes on a box of its own laid over the thumbnail.
 */
fun Modifier.filmstrip(): Modifier = drawBehind {
    val strip = (size.width * 0.06f).coerceIn(4.dp.toPx(), 14.dp.toPx())
    val holeWidth = strip * 0.45f
    val holeHeight = strip * 0.5f
    val pitch = holeHeight * 2
    for (x in listOf(0f, size.width - strip)) {
        drawRect(filmBase, Offset(x, 0f), Size(strip, size.height))
        var y = pitch * 0.3f
        while (y + holeHeight <= size.height) {
            drawRoundRect(filmHole, Offset(x + (strip - holeWidth) / 2, y), Size(holeWidth, holeHeight),
                CornerRadius(holeWidth * 0.2f))
            y += pitch
        }
    }
}
