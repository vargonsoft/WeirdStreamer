package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextOverflow

/**
 * A Text composable that ellipsizes at the start (beginning) instead of the end.
 * Equivalent to Android XML's ellipsize="start".
 * 
 * When the text is too long to fit, it shows "…" at the beginning and displays
 * as much of the end of the text as possible.
 * 
 * @param text The text to display
 * @param style The text style to apply
 * @param color The color to apply to the text (default: Color.Unspecified)
 * @param modifier The modifier to apply to the text
 */
@Composable
fun TextWithStartEllipsis(
    text: String,
    style: TextStyle,
    color: Color = Color.Unspecified,
    modifier: Modifier = Modifier
) {
    BoxWithConstraints(modifier = modifier) {
        val textMeasurer = rememberTextMeasurer()
        val maxWidthPx = with(androidx.compose.ui.platform.LocalDensity.current) { maxWidth.toPx() }
        
        val displayText = remember(text, maxWidthPx, style) {
            val measured = textMeasurer.measure(
                text = text,
                style = style,
                maxLines = 1,
                softWrap = false
            )
            
            if (measured.size.width > maxWidthPx) {
                // Binary search to find how much of the end we can show
                var start = 0
                var end = text.length - 1
                var bestCutPoint = text.length / 2
                
                while (start <= end) {
                    val mid = (start + end) / 2
                    val testText = "…" + text.substring(mid)
                    val testMeasured = textMeasurer.measure(
                        text = testText,
                        style = style,
                        maxLines = 1,
                        softWrap = false
                    )
                    
                    if (testMeasured.size.width <= maxWidthPx) {
                        bestCutPoint = mid
                        end = mid - 1
                    } else {
                        start = mid + 1
                    }
                }
                
                "…" + text.substring(bestCutPoint)
            } else {
                text
            }
        }
        
        Text(
            text = displayText,
            style = style,
            color = color,
            maxLines = 1,
            overflow = TextOverflow.Clip,
            softWrap = false
        )
    }
}
