package com.vargon.weirdstreamer.ui.components

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.drawOutline
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.layout
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch

/**
 * Lightweight replacement for tv-material's clickable Surface: D-pad clickable, with a focus
 * scale and border. Focus state is only read in the layer/draw phases, so moving focus never
 * recomposes the item — it only redraws the two items involved.
 */
@Composable
fun Modifier.tvClickable(
    shape: Shape,
    focusedScale: Float,
    borderWidth: Dp,
    borderColor: Color = Color.White,
    onClick: () -> Unit
): Modifier {
    val scope = rememberCoroutineScope()
    val scale = remember { Animatable(1f) }
    var focused by remember { mutableStateOf(false) }
    val interactionSource = remember { MutableInteractionSource() }

    return this
        .onFocusChanged { state ->
            if (state.isFocused == focused) return@onFocusChanged
            focused = state.isFocused
            scope.launch { scale.animateTo(if (state.isFocused) focusedScale else 1f, tween(FOCUS_ANIM_MS)) }
        }
        .clickable(interactionSource = interactionSource, indication = null, onClick = onClick)
        // Scale inside the focus target: a layer outside it would change the bounds the lazy
        // grid brings into view, making it scroll a few pixels on every focus move.
        .graphicsLayer {
            scaleX = scale.value
            scaleY = scale.value
        }
        .drawWithContent {
            drawContent()
            if (focused) {
                val outline = shape.createOutline(size, layoutDirection, this)
                drawOutline(outline, borderColor, style = Stroke(width = borderWidth.toPx()))
            }
        }
}

private const val FOCUS_ANIM_MS = 120

/** How far a focused item grows past its edges: tvClickable's scale (up to 1.06) and border. */
val FocusBleed = 14.dp

/**
 * Room at a scrolling row's ends for its focused item to grow into, instead of the row clipping
 * its outline: the row is drawn [bleed] wider on each side. Give its contentPadding the same
 * [bleed] horizontally, so its items stay where they were. Its parent needs that much room (the
 * boxes' padding is 16dp).
 */
fun Modifier.focusBleed(bleed: Dp = FocusBleed): Modifier = layout { measurable, constraints ->
    val px = bleed.roundToPx()
    val wider = if (constraints.hasBoundedWidth) {
        constraints.copy(minWidth = constraints.minWidth + 2 * px, maxWidth = constraints.maxWidth + 2 * px)
    } else constraints
    val placeable = measurable.measure(wider)
    layout(placeable.width - 2 * px, placeable.height) { placeable.place(-px, 0) }
}
