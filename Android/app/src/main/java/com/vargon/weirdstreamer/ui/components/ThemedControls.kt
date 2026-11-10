package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Column
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

/**
 * A phone screen's top bar, in a box in themes that draw them: its TopAppBar takes panelTopBarInsets().
 * [above] goes over it, on the background (the open server and user).
 */
@Composable
fun PhoneTopBarPanel(above: (@Composable () -> Unit)? = null, content: @Composable () -> Unit) {
    if (hasPanels) {
        Column(Modifier.statusBarsPadding()) {
            above?.invoke()
            Panel(margin = PaddingValues(start = 8.dp, top = if (above != null) 4.dp else 8.dp, end = 8.dp),
                contentPadding = PaddingValues(0.dp), clip = true) { content() }
        }
    } else if (above != null) {
        // The column takes the status bar, so the TopAppBar's own insets leave nothing more.
        Column(Modifier.statusBarsPadding()) { above(); content() }
    } else content()
}

/** The top bar's insets: none inside its box (the box keeps clear of the status bar). */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
fun panelTopBarInsets(): WindowInsets = if (hasPanels) WindowInsets(0) else androidx.compose.material3.TopAppBarDefaults.windowInsets

/** A phone screen's content, in a box in themes that draw them (else a plain Box). */
@Composable
fun PhoneContentPanel(modifier: Modifier = Modifier, boxed: Boolean = true, content: @Composable BoxScope.() -> Unit) =
    if (boxed) Panel(modifier, margin = PaddingValues(8.dp), contentPadding = PaddingValues(0.dp), clip = true, content = content)
    else Box(modifier, content = content) // the content draws its own boxes

/** Whether the theme draws boxes (Panel), for layouts that change around them. */
val hasPanels: Boolean
    @Composable get() = WeirdStreamerTheme.spec.panelColor.isSpecified

/** A phone pop-up menu's edge, so it stands apart from a dialog or bar of the same colour. */
@Composable
fun phoneMenuBorder() = androidx.compose.foundation.BorderStroke(1.dp, androidx.compose.material3.MaterialTheme.colorScheme.outline)

/** Buttons' shape on TV and phone: square corners, as on the web (its .btn's 5px). */
val AppButtonShape = RoundedCornerShape(5.dp)

/** A TV Button's shape (pass as shape =). */
@Composable
fun tvButtonShape() = androidx.tv.material3.ButtonDefaults.shape(AppButtonShape)

/** A TV IconButton's shape (pass as shape =). */
@Composable
fun tvIconButtonShape() = androidx.tv.material3.IconButtonDefaults.shape(AppButtonShape)

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

/** TV colors for a quieter button (Cancel, Back, Close): the theme's secondary pair; focused, the focus colors like any button. */
@Composable
fun tvSecondaryButtonColors(): androidx.tv.material3.ButtonColors {
    val spec = WeirdStreamerTheme.spec
    if (!spec.secondaryButtonColor.isSpecified) return tvButtonColors()
    return androidx.tv.material3.ButtonDefaults.colors(
        containerColor = spec.secondaryButtonColor,
        contentColor = spec.secondaryButtonLabel,
        focusedContainerColor = spec.focusColor,
        focusedContentColor = spec.focusLabel,
    )
}

/** A quieter TV button's outline: the theme's button color, gone while focused (the focus color fills it). */
@Composable
fun tvSecondaryButtonBorder(): androidx.tv.material3.ButtonBorder {
    val spec = WeirdStreamerTheme.spec
    val outline = if (spec.buttonColor.isSpecified)
        androidx.tv.material3.Border(androidx.compose.foundation.BorderStroke(1.dp, spec.buttonColor), shape = AppButtonShape)
    else androidx.tv.material3.Border.None
    return androidx.tv.material3.ButtonDefaults.border(border = outline, focusedBorder = androidx.tv.material3.Border.None)
}

/** A TV dialog's quieter button (Cancel, Back, Close), beside its main one in [tvButtonColors]. */
@Composable
fun TvSecondaryButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit,
) = androidx.tv.material3.Button(onClick = onClick, modifier = modifier, shape = tvButtonShape(),
    colors = tvSecondaryButtonColors(), border = tvSecondaryButtonBorder(), content = content)

/** Phone colors for a quieter button: the theme's secondary pair, or Material's tonal one. */
@Composable
fun phoneSecondaryButtonColors(): androidx.compose.material3.ButtonColors {
    val spec = WeirdStreamerTheme.spec
    return if (spec.secondaryButtonColor.isSpecified) {
        androidx.compose.material3.ButtonDefaults.buttonColors(containerColor = spec.secondaryButtonColor, contentColor = spec.secondaryButtonLabel)
    } else androidx.compose.material3.ButtonDefaults.filledTonalButtonColors()
}

/**
 * A phone dialog's main button (OK, Save, Login): filled in the theme's button color, or in
 * Material's error colors when it deletes or removes something ([destructive], like the web's
 * red Remove buttons).
 */
@Composable
fun PhoneButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    destructive: Boolean = false,
    content: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit,
) {
    val scheme = androidx.compose.material3.MaterialTheme.colorScheme
    androidx.compose.material3.Button(onClick = onClick, modifier = modifier, enabled = enabled, shape = AppButtonShape,
        colors = if (destructive) androidx.compose.material3.ButtonDefaults.buttonColors(containerColor = scheme.errorContainer, contentColor = scheme.onErrorContainer)
            else phoneButtonColors(),
        content = content)
}

/** A phone dialog's quieter button (Cancel, Keep, Close): the theme's secondary pair, outlined in its button color. */
@Composable
fun PhoneSecondaryButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    content: @Composable androidx.compose.foundation.layout.RowScope.() -> Unit,
) {
    val spec = WeirdStreamerTheme.spec
    androidx.compose.material3.Button(onClick = onClick, modifier = modifier, enabled = enabled, shape = AppButtonShape,
        colors = phoneSecondaryButtonColors(),
        border = if (spec.buttonColor.isSpecified) androidx.compose.foundation.BorderStroke(1.dp, spec.buttonColor) else null,
        content = content)
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
