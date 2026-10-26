package com.vargon.weirdstreamer.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * A theme's colors, all built from its background: boxes a step lighter (darker, in a light
 * theme), buttons a step further, text and secondary text tinted to match, and one accent for
 * focus, headings and icons. The TV and phone color schemes and the theme spec come from it.
 */
data class ThemePalette(
    val isLight: Boolean,
    val background: Color, // the screen, around the boxes
    val panel: Color, // the boxes the header and content sit in (the web's --surface-color)
    val raised: Color, // things raised inside a box: placeholders, tonal chips
    val button: Color, // buttons
    val buttonLabel: Color,
    val text: Color,
    val mutedText: Color, // secondary text
    val accent: Color, // focused buttons, headings, icons, links
    val onAccent: Color, // text on the accent
    val secondary: Color, // a second accent, rarely used
    val panelAlpha: Float = 1f, // below 1: boxes let the background image show through
)

// Ember (the default): the Weird Streamer logo's aubergine, gold and cream
val EmberPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF1C0F1B),
    panel = Color(0xFF321B2F), // the logo's aubergine (the app icon's background)
    raised = Color(0xFF4A2A45),
    button = Color(0xFF6A3A62), // the web's --button-bg
    buttonLabel = Color(0xFFFFF2D8), // the logo's cream
    text = Color(0xFFFFF2D8),
    mutedText = Color(0xFFD8C6B4),
    accent = Color(0xFFF6C445), // the logo's gold
    onAccent = Color(0xFF321B2F),
    secondary = Color(0xFFE0892C),
)

// Dark: Material's dark purple, around a near-black background
val DarkPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF141218),
    panel = Color(0xFF211F26),
    raised = Color(0xFF2B2930),
    button = Color(0xFF4A4458),
    buttonLabel = Color(0xFFE6E1E5),
    text = Color(0xFFE6E1E5),
    mutedText = Color(0xFFCAC4D0),
    accent = Color(0xFFD0BCFF),
    onAccent = Color(0xFF381E72),
    secondary = Color(0xFFCCC2DC),
)

// Light: Material's light purple, white boxes on a pale lavender background
val LightPalette = ThemePalette(
    isLight = true,
    background = Color(0xFFF3EFF7),
    panel = Color(0xFFFFFFFF),
    raised = Color(0xFFECE6F0),
    button = Color(0xFF6750A4),
    buttonLabel = Color(0xFFFFFFFF),
    text = Color(0xFF1D1B20),
    mutedText = Color(0xFF49454F),
    accent = Color(0xFF6750A4),
    onAccent = Color(0xFFFFFFFF),
    secondary = Color(0xFF625B71),
)

// Cinema Blue: a theater's deep navy and gold. The boxes are see-through, over the film reel.
val CinemaBluePalette = ThemePalette(
    isLight = false,
    background = Color(0xFF02013D),
    panel = Color(0xFF0D1150),
    raised = Color(0xFF182066),
    button = Color(0xFF26307F),
    buttonLabel = Color(0xFFE6E9F7),
    text = Color(0xFFE6E9F7),
    mutedText = Color(0xFFA8AED6),
    accent = Color(0xFFF2B83A),
    onAccent = Color(0xFF02013D),
    secondary = Color(0xFF7C8FE0),
    panelAlpha = 0.75f,
)

// Cinema Red: a velvet curtain's deep burgundy and gold
val CinemaRedPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF2B0B10),
    panel = Color(0xFF43141B),
    raised = Color(0xFF561B24),
    button = Color(0xFF72252F),
    buttonLabel = Color(0xFFF6E7E4),
    text = Color(0xFFF6E7E4),
    mutedText = Color(0xFFD4AFAA),
    accent = Color(0xFFF2B83A),
    onAccent = Color(0xFF2B0B10),
    secondary = Color(0xFFE0896F),
)
