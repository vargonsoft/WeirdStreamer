package com.vargon.weirdstreamer.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.tv.material3.*
import com.vargon.weirdstreamer.R

/** The app's themes, stored by name (CredentialManager's "app_theme"), in the order offered. */
enum class AppTheme(val label: String) {
    DEFAULT("Default"), // Ember
    LIGHT("Light"),
    DARK("Dark"),
    AUTO("Auto Light/Dark"), // Light or Dark, following the device's dark theme setting
    CINEMA_BLUE("Cinema Blue"),
    CINEMA_RED("Cinema Red"),
    TEAL("Teal"),
    ORANGE("Orange"),
    BROWN("Brown"),
    PURPLE("Purple"),
    GRAY("Gray"),
    YELLOW("Yellow"),
    GREEN("Green"),
    HARBOR_LIGHTS("Harbor Lights"),
    MOSS_AND_ROSE("Moss and Rose"),
    DESERT_BLOOM("Desert Bloom"),
    ARCTIC_BERRY("Arctic Berry"),
    COPPER_SEA("Copper Sea"),
    NEON_ARCADE("Neon Arcade");

    /** The theme to draw: Auto as Light or Dark, from the device's setting. */
    @Composable
    fun resolved(): AppTheme = if (this == AUTO) (if (isSystemInDarkTheme()) DARK else LIGHT) else this

    companion object {
        fun fromKey(key: String?): AppTheme = entries.find { it.name == key } ?: DEFAULT
    }
}

interface WeirdStreamerThemeSpec {
    val folderIconTint: Color
    val serverIconTint: Color
    val parentIconTint: Color
    val backgroundImage: Int?
    val spinnerColor: Color
    // The look shared with the web UI (Unspecified: Material's defaults, no boxes):
    /** The boxes the header and content sit in, over the darker background (`Panel`). */
    val panelColor: Color get() = Color.Unspecified
    /** How opaque the boxes are: below 1 lets a background image show through. */
    val panelAlpha: Float get() = 1f
    /** Buttons and their labels (`tvButtonColors`, `phoneButtonColors`). */
    val buttonColor: Color get() = Color.Unspecified
    val buttonLabel: Color get() = Color.Unspecified
    /** Settings headings (the web's --heading-color). */
    val headingColor: Color get() = Color.Unspecified
    /** A focused TV button (a hovered one on the web). */
    val focusColor: Color get() = Color.Unspecified
    val focusLabel: Color get() = Color.Unspecified
    /** The outline around a focused TV tile or row. */
    val focusOutline: Color get() = Color.Unspecified
    /** Progress lines on thumbnails and posters, and the check once watched (the web's --progress-color). */
    val progressColor: Color get() = Color.Unspecified
    val progressLabel: Color get() = Color.Unspecified
}

/** No theme's colors: the composition's default before a theme is applied. */
object DefaultThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = Color.Unspecified
    override val serverIconTint: Color = Color.Unspecified
    override val parentIconTint: Color = Color.Unspecified
    override val backgroundImage: Int? = null
    override val spinnerColor: Color = Color.Unspecified
}

/** A theme's spec, from its palette: the accent for icons, headings and focus. */
class PaletteThemeSpec(p: ThemePalette, override val backgroundImage: Int? = null) : WeirdStreamerThemeSpec {
    override val folderIconTint = p.accent
    override val serverIconTint = p.accent
    override val parentIconTint = p.accent
    override val spinnerColor = p.accent
    override val panelColor = p.panel
    override val panelAlpha = p.panelAlpha
    override val buttonColor = p.button
    override val buttonLabel = p.buttonLabel
    override val headingColor = p.accent
    override val focusColor = p.focus
    override val focusLabel = p.onFocus
    override val focusOutline = p.focusOutline
    override val progressColor = p.accent
    override val progressLabel = p.onAccent
}

private val emberSpec = PaletteThemeSpec(EmberPalette)
private val lightSpec = PaletteThemeSpec(LightPalette)
private val darkSpec = PaletteThemeSpec(DarkPalette)
private val cinemaBlueSpec = PaletteThemeSpec(CinemaBluePalette, backgroundImage = R.drawable.film_reel)
private val cinemaRedSpec = PaletteThemeSpec(CinemaRedPalette)
private val tealSpec = PaletteThemeSpec(TealPalette)
private val orangeSpec = PaletteThemeSpec(OrangePalette)
private val brownSpec = PaletteThemeSpec(BrownPalette)
private val purpleSpec = PaletteThemeSpec(PurplePalette)
private val graySpec = PaletteThemeSpec(GrayPalette)
private val yellowSpec = PaletteThemeSpec(YellowPalette)
private val greenSpec = PaletteThemeSpec(GreenPalette)
private val harborLightsSpec = PaletteThemeSpec(HarborLightsPalette)
private val mossAndRoseSpec = PaletteThemeSpec(MossAndRosePalette)
private val desertBloomSpec = PaletteThemeSpec(DesertBloomPalette)
private val arcticBerrySpec = PaletteThemeSpec(ArcticBerryPalette)
private val copperSeaSpec = PaletteThemeSpec(CopperSeaPalette)
private val neonArcadeSpec = PaletteThemeSpec(NeonArcadePalette)

val LocalThemeSpec = staticCompositionLocalOf<WeirdStreamerThemeSpec> { DefaultThemeSpec }

object WeirdStreamerTheme {
    val spec: WeirdStreamerThemeSpec
        @Composable
        @ReadOnlyComposable
        get() = LocalThemeSpec.current
}

/** The palette a theme draws with (Auto: Light or Dark, from the device's setting). */
@Composable
fun AppTheme.palette(): ThemePalette = when (resolved()) {
    AppTheme.DEFAULT -> EmberPalette
    AppTheme.LIGHT, AppTheme.AUTO -> LightPalette
    AppTheme.DARK -> DarkPalette
    AppTheme.CINEMA_BLUE -> CinemaBluePalette
    AppTheme.CINEMA_RED -> CinemaRedPalette
    AppTheme.TEAL -> TealPalette
    AppTheme.ORANGE -> OrangePalette
    AppTheme.BROWN -> BrownPalette
    AppTheme.PURPLE -> PurplePalette
    AppTheme.GRAY -> GrayPalette
    AppTheme.YELLOW -> YellowPalette
    AppTheme.GREEN -> GreenPalette
    AppTheme.HARBOR_LIGHTS -> HarborLightsPalette
    AppTheme.MOSS_AND_ROSE -> MossAndRosePalette
    AppTheme.DESERT_BLOOM -> DesertBloomPalette
    AppTheme.ARCTIC_BERRY -> ArcticBerryPalette
    AppTheme.COPPER_SEA -> CopperSeaPalette
    AppTheme.NEON_ARCADE -> NeonArcadePalette
}

/** The spec (icon tints, boxes, buttons, background image) that goes with a theme, on TV and phone. */
@Composable
fun AppTheme.spec(): WeirdStreamerThemeSpec = when (resolved()) {
    AppTheme.DEFAULT -> emberSpec
    AppTheme.LIGHT, AppTheme.AUTO -> lightSpec
    AppTheme.DARK -> darkSpec
    AppTheme.CINEMA_BLUE -> cinemaBlueSpec
    AppTheme.CINEMA_RED -> cinemaRedSpec
    AppTheme.TEAL -> tealSpec
    AppTheme.ORANGE -> orangeSpec
    AppTheme.BROWN -> brownSpec
    AppTheme.PURPLE -> purpleSpec
    AppTheme.GRAY -> graySpec
    AppTheme.YELLOW -> yellowSpec
    AppTheme.GREEN -> greenSpec
    AppTheme.HARBOR_LIGHTS -> harborLightsSpec
    AppTheme.MOSS_AND_ROSE -> mossAndRoseSpec
    AppTheme.DESERT_BLOOM -> desertBloomSpec
    AppTheme.ARCTIC_BERRY -> arcticBerrySpec
    AppTheme.COPPER_SEA -> copperSeaSpec
    AppTheme.NEON_ARCADE -> neonArcadeSpec
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun WeirdStreamerTheme(
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val p = appTheme.palette()
    // The screen is the background; buttons, focus and text come from the palette too
    // (buttons through tvButtonColors).
    val colorScheme = if (p.isLight) {
        lightColorScheme(
            primary = p.button, onPrimary = p.buttonLabel,
            secondary = p.secondary, onSecondary = p.background,
            tertiary = p.accent, onTertiary = p.onAccent,
            background = p.background, onBackground = p.text,
            surface = p.background, onSurface = p.text,
            surfaceVariant = p.raised, onSurfaceVariant = p.mutedText,
            inverseSurface = p.text, inverseOnSurface = p.background,
            border = p.mutedText, borderVariant = p.raised,
        )
    } else {
        darkColorScheme(
            primary = p.button, onPrimary = p.buttonLabel,
            secondary = p.secondary, onSecondary = p.background,
            tertiary = p.accent, onTertiary = p.onAccent,
            background = p.background, onBackground = p.text,
            surface = p.background, onSurface = p.text,
            surfaceVariant = p.raised, onSurfaceVariant = p.mutedText,
            inverseSurface = p.text, inverseOnSurface = p.background,
            border = p.mutedText, borderVariant = p.raised,
        )
    }

    CompositionLocalProvider(LocalThemeSpec provides appTheme.spec()) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = Typography,
            content = content
        )
    }
}
