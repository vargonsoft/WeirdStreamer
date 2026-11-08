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
    val focusOutline: Color = Color.White, // the outline around a focused tile or row (TV)
    val focus: Color = accent, // a focused button (TV), when the accent wouldn't stand out
    val onFocus: Color = onAccent,
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
    // Focus in a deep purple: white wouldn't show on the white boxes, nor the buttons' purple on them
    focusOutline = Color(0xFF381E72),
    focus = Color(0xFF381E72),
    onFocus = Color(0xFFFFFFFF),
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

// Teal: deep sea teal with aqua
val TealPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF0B1E1E),
    panel = Color(0xFF123130),
    raised = Color(0xFF1B4443),
    button = Color(0xFF1F5F5B),
    buttonLabel = Color(0xFFE4F5F3),
    text = Color(0xFFE4F5F3),
    mutedText = Color(0xFFA6CDC9),
    accent = Color(0xFF5EEAD4),
    onAccent = Color(0xFF0B2423),
    secondary = Color(0xFFF2A65A),
)

// Orange: burnt orange with tangerine
val OrangePalette = ThemePalette(
    isLight = false,
    background = Color(0xFF1D110A),
    panel = Color(0xFF331E10),
    raised = Color(0xFF4A2D18),
    button = Color(0xFF8A4A1B),
    buttonLabel = Color(0xFFFFF0E0),
    text = Color(0xFFFFF0E0),
    mutedText = Color(0xFFE2C2A2),
    accent = Color(0xFFFFA24A),
    onAccent = Color(0xFF2B1607),
    secondary = Color(0xFFF6C445),
)

// Brown: coffee brown with caramel
val BrownPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF18120D),
    panel = Color(0xFF2D2219),
    raised = Color(0xFF423226),
    button = Color(0xFF6B4D36),
    buttonLabel = Color(0xFFF5EADC),
    text = Color(0xFFF5EADC),
    mutedText = Color(0xFFCDB9A2),
    accent = Color(0xFFE3B676),
    onAccent = Color(0xFF2D2219),
    secondary = Color(0xFFC77C4A),
)

// Purple: violet with lavender
val PurplePalette = ThemePalette(
    isLight = false,
    background = Color(0xFF140E22),
    panel = Color(0xFF23193B),
    raised = Color(0xFF322556),
    button = Color(0xFF4F3C8A),
    buttonLabel = Color(0xFFEEE8FF),
    text = Color(0xFFEEE8FF),
    mutedText = Color(0xFFBCB1DD),
    accent = Color(0xFFC3A6FF),
    onAccent = Color(0xFF1E1436),
    secondary = Color(0xFFF29FC5),
)

// Gray: charcoal with silver
val GrayPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF151617),
    panel = Color(0xFF232527),
    raised = Color(0xFF313437),
    button = Color(0xFF4A4E53),
    buttonLabel = Color(0xFFEFF1F3),
    text = Color(0xFFEFF1F3),
    mutedText = Color(0xFFB3B8BD),
    accent = Color(0xFFDDE2E7),
    onAccent = Color(0xFF232527),
    secondary = Color(0xFF8FB8E8),
)

// Yellow: dark olive with sunflower
val YellowPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF181606),
    panel = Color(0xFF2B2809),
    raised = Color(0xFF3F3A10),
    button = Color(0xFF6A6015),
    buttonLabel = Color(0xFFFFF9DA),
    text = Color(0xFFFFF9DA),
    mutedText = Color(0xFFDAD19E),
    accent = Color(0xFFFFE04D),
    onAccent = Color(0xFF2B2809),
    secondary = Color(0xFFF29B38),
)

// Green: forest green with lime
val GreenPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF0D190F),
    panel = Color(0xFF162B19),
    raised = Color(0xFF213F26),
    button = Color(0xFF2E6A37),
    buttonLabel = Color(0xFFE8F6E9),
    text = Color(0xFFE8F6E9),
    mutedText = Color(0xFFB0D2B3),
    accent = Color(0xFF93E27D),
    onAccent = Color(0xFF12261A),
    secondary = Color(0xFFE8C55A),
)

// Harbor Lights: navy boxes, brick buttons and lamp gold
val HarborLightsPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF0C1626),
    panel = Color(0xFF14243D),
    raised = Color(0xFF322A39),
    button = Color(0xFF8C3B2E),
    buttonLabel = Color(0xFFF3EAD8),
    text = Color(0xFFF3EAD8),
    mutedText = Color(0xFFBBB8B1),
    accent = Color(0xFFF2B544),
    onAccent = Color(0xFF14243D),
    secondary = Color(0xFFB0786A),
)

// Moss and Rose: forest boxes, dusty rose buttons and cream gold
val MossAndRosePalette = ThemePalette(
    isLight = false,
    background = Color(0xFF0F1F18),
    panel = Color(0xFF1D372B),
    raised = Color(0xFF393A36),
    button = Color(0xFF8E4458),
    buttonLabel = Color(0xFFF4EEDF),
    text = Color(0xFFF4EEDF),
    mutedText = Color(0xFFBEC0B2),
    accent = Color(0xFFF1D49B),
    onAccent = Color(0xFF1D372B),
    secondary = Color(0xFFB28087),
)

// Desert Bloom: plum-brown boxes, terracotta buttons and turquoise
val DesertBloomPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF1E0F17),
    panel = Color(0xFF38202B),
    raised = Color(0xFF542C2B),
    button = Color(0xFFA84E2A),
    buttonLabel = Color(0xFFFBEDE3),
    text = Color(0xFFFBEDE3),
    mutedText = Color(0xFFCABAB5),
    accent = Color(0xFF4FD1C5),
    onAccent = Color(0xFF1E0F17),
    secondary = Color(0xFFC5866B),
)

// Arctic Berry: slate blue boxes, raspberry buttons and ice mint
val ArcticBerryPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF10172A),
    panel = Color(0xFF1F2A44),
    raised = Color(0xFF402B4A),
    button = Color(0xFFA32E5B),
    buttonLabel = Color(0xFFEEF2FA),
    text = Color(0xFFEEF2FA),
    mutedText = Color(0xFFBAC0CC),
    accent = Color(0xFF9BE7D8),
    onAccent = Color(0xFF10172A),
    secondary = Color(0xFFBD7393),
)

// Copper Sea: deep teal boxes, copper buttons and sand
val CopperSeaPalette = ThemePalette(
    isLight = false,
    background = Color(0xFF082221),
    panel = Color(0xFF0F3B3A),
    raised = Color(0xFF324238),
    button = Color(0xFF9C5530),
    buttonLabel = Color(0xFFF1EBDD),
    text = Color(0xFFF1EBDD),
    mutedText = Color(0xFFB8BFB4),
    accent = Color(0xFFEAD7A1),
    onAccent = Color(0xFF0F3B3A),
    secondary = Color(0xFFBA8A6D),
)

// Neon Arcade: charcoal boxes, electric violet buttons and lime
val NeonArcadePalette = ThemePalette(
    isLight = false,
    background = Color(0xFF111116),
    panel = Color(0xFF1D1D26),
    raised = Color(0xFF2C234D),
    button = Color(0xFF5B36C2),
    buttonLabel = Color(0xFFF1EEFF),
    text = Color(0xFFF1EEFF),
    mutedText = Color(0xFFBCBAC9),
    accent = Color(0xFFB8F04A),
    onAccent = Color(0xFF16161E),
    secondary = Color(0xFF9076D7),
)
