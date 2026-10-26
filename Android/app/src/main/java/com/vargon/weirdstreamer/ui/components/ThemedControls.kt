package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.isSpecified
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

// The theme's own buttons and boxes (Ember's, shared with the web UI). Themes that don't set them
// keep Material's defaults and no boxes.

/**
 * A rounded box in the theme's panel color, like the web UI's header and content boxes: margin
 * around it, contentPadding inside it, and alpha below 1 to let a backdrop show through. Without a
 * panel color it's a plain Box, with neither margin nor padding, so other themes' layouts stay put.
 */
@Composable
fun Panel(
    modifier: Modifier = Modifier,
    margin: PaddingValues = PaddingValues(0.dp),
    contentPadding: PaddingValues = PaddingValues(16.dp),
    alpha: Float = 1f,
    clip: Boolean = false, // for scrolling content (the phone); the TV's focused tiles grow past the edge
    content: @Composable BoxScope.() -> Unit,
) {
    val color = WeirdStreamerTheme.spec.panelColor
    val opacity = minOf(alpha, WeirdStreamerTheme.spec.panelAlpha) // the theme's, or less over a backdrop
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier = if (color.isSpecified) {
            modifier.padding(margin).then(if (clip) Modifier.clip(shape) else Modifier)
                .background(color.copy(alpha = opacity), shape).padding(contentPadding)
        } else modifier,
        content = content,
    )
}

/** A phone screen's top bar, in a box in themes that draw them: its TopAppBar takes panelTopBarInsets(). */
@Composable
fun PhoneTopBarPanel(content: @Composable () -> Unit) {
    if (hasPanels) {
        Panel(Modifier.statusBarsPadding(), margin = PaddingValues(start = 8.dp, top = 8.dp, end = 8.dp),
            contentPadding = PaddingValues(0.dp), clip = true) { content() }
    } else content()
}

/** The top bar's insets: none inside its box (the box keeps clear of the status bar). */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun panelTopBarInsets(): WindowInsets = if (hasPanels) WindowInsets(0) else androidx.compose.material3.TopAppBarDefaults.windowInsets

/** A phone screen's content, in a box in themes that draw them (else a plain Box). */
@Composable
fun PhoneContentPanel(modifier: Modifier = Modifier, content: @Composable BoxScope.() -> Unit) =
    Panel(modifier, margin = PaddingValues(8.dp), contentPadding = PaddingValues(0.dp), clip = true, content = content)

/** Whether the theme draws boxes (Panel), for layouts that change around them. */
val hasPanels: Boolean
    @Composable get() = WeirdStreamerTheme.spec.panelColor.isSpecified

/** TV button colors: the theme's button and focus colors, or containerColor/contentColor given. */
@Composable
fun tvButtonColors(
    containerColor: Color = Color.Unspecified,
    contentColor: Color = Color.Unspecified,
): androidx.tv.material3.ButtonColors {
    val spec = WeirdStreamerTheme.spec
    val defaults = androidx.tv.material3.ButtonDefaults.colors()
    if (!spec.buttonColor.isSpecified) {
        return if (containerColor.isSpecified || contentColor.isSpecified) {
            androidx.tv.material3.ButtonDefaults.colors(
                containerColor = containerColor.takeOrElse(androidx.tv.material3.MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.8f)),
                contentColor = contentColor.takeOrElse(androidx.tv.material3.MaterialTheme.colorScheme.onSurface.copy(alpha = 0.8f)),
            )
        } else defaults
    }
    return androidx.tv.material3.ButtonDefaults.colors(
        containerColor = containerColor.takeOrElse(spec.buttonColor),
        contentColor = contentColor.takeOrElse(spec.buttonLabel),
        focusedContainerColor = spec.focusColor,
        focusedContentColor = spec.focusLabel,
    )
}

/** Whether the theme has its own button colors (then a selected button is the plain themed one). */
val hasThemedButtons: Boolean
    @Composable get() = WeirdStreamerTheme.spec.buttonColor.isSpecified

/** Filled phone button colors: the theme's button colors, or Material's. */
@Composable
fun phoneButtonColors(): androidx.compose.material3.ButtonColors {
    val spec = WeirdStreamerTheme.spec
    return if (spec.buttonColor.isSpecified) {
        androidx.compose.material3.ButtonDefaults.buttonColors(containerColor = spec.buttonColor, contentColor = spec.buttonLabel)
    } else androidx.compose.material3.ButtonDefaults.buttonColors()
}

/**
 * TV text field colors from the theme. The fields are the phone's Material 3 ones, which would
 * otherwise draw with Material's light defaults (a pale box under the theme's cream text).
 */
@Composable
fun tvTextFieldColors(): androidx.compose.material3.TextFieldColors {
    val scheme = androidx.tv.material3.MaterialTheme.colorScheme
    val accent = WeirdStreamerTheme.spec.focusColor.takeOrElse(scheme.tertiary)
    return androidx.compose.material3.TextFieldDefaults.colors(
        focusedContainerColor = scheme.surfaceVariant,
        unfocusedContainerColor = scheme.surfaceVariant,
        focusedTextColor = scheme.onSurface,
        unfocusedTextColor = scheme.onSurface,
        focusedLabelColor = accent,
        unfocusedLabelColor = scheme.onSurfaceVariant,
        focusedPlaceholderColor = scheme.onSurfaceVariant.copy(alpha = 0.4f),
        unfocusedPlaceholderColor = scheme.onSurfaceVariant.copy(alpha = 0.4f),
        focusedSupportingTextColor = scheme.onSurfaceVariant,
        unfocusedSupportingTextColor = scheme.onSurfaceVariant,
        focusedIndicatorColor = accent,
        unfocusedIndicatorColor = scheme.onSurfaceVariant,
        cursorColor = accent,
        selectionColors = androidx.compose.foundation.text.selection.TextSelectionColors(accent, accent.copy(alpha = 0.4f)),
    )
}

/** Text for a TV text field's label, placeholder or hint, in the field's colors (a TV Text has the TV theme's). */
@Composable
fun FieldText(text: String) = androidx.compose.material3.Text(text)

private fun Color.takeOrElse(other: Color) = if (isSpecified) this else other
