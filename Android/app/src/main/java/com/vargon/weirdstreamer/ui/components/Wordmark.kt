package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.res.painterResource
import com.vargon.weirdstreamer.R

/**
 * The Weird Streamer wordmark: cream letters where the text around it is light (dark themes),
 * aubergine ones where it's dark (light themes). textColor is the theme's onSurface: the TV and
 * phone screens use different MaterialThemes, and the TV themes draw their own dark backdrop.
 */
@Composable
fun Wordmark(textColor: Color, modifier: Modifier = Modifier) {
    Image(
        painter = painterResource(if (textColor.luminance() < 0.5f) R.drawable.wordmark_on_light else R.drawable.wordmark),
        contentDescription = "Weird Streamer",
        // Sized by its height (the modifier's), keeping the drawable's proportions.
        modifier = modifier.aspectRatio(745f / 95f)
    )
}
