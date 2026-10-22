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
    CINEMA_RED("Cinema Red");

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
}

object DefaultThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = Color.Unspecified
    override val serverIconTint: Color = Color.Unspecified
    override val parentIconTint: Color = Color.Unspecified
    override val backgroundImage: Int? = null
    override val spinnerColor: Color = Color.Unspecified
}

object EmberThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = EmberGold
    override val serverIconTint: Color = EmberGold
    override val parentIconTint: Color = EmberGold
    override val backgroundImage: Int? = null
    override val spinnerColor: Color = EmberGold
}

object CinemaBlueThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = CinemaGold
    override val serverIconTint: Color = CinemaGold
    override val parentIconTint: Color = CinemaGold
    override val backgroundImage: Int? = R.drawable.film_reel
    override val spinnerColor: Color = CinemaBlue

}

object CinemaRedThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = Color.Unspecified
    override val serverIconTint: Color = Color.Unspecified
    override val parentIconTint: Color = Color.Unspecified
    override val backgroundImage: Int? = null
    override val spinnerColor: Color = CinemaBurgundy
}

val LocalThemeSpec = staticCompositionLocalOf<WeirdStreamerThemeSpec> { DefaultThemeSpec }

object WeirdStreamerTheme {
    val spec: WeirdStreamerThemeSpec
        @Composable
        @ReadOnlyComposable
        get() = LocalThemeSpec.current
}

/** The theme spec (icon tints, backdrop) that goes with each theme, on the TV and the phone. */
fun AppTheme.spec(): WeirdStreamerThemeSpec = when (this) {
    AppTheme.DEFAULT -> EmberThemeSpec
    AppTheme.CINEMA_BLUE -> CinemaBlueThemeSpec
    AppTheme.CINEMA_RED -> CinemaRedThemeSpec
    AppTheme.LIGHT, AppTheme.DARK, AppTheme.AUTO -> DefaultThemeSpec
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun WeirdStreamerTheme(
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val colorScheme = when (appTheme.resolved()) {
        AppTheme.CINEMA_BLUE -> {
            darkColorScheme(
                primary = CinemaBlueMid,
                onPrimary = CinemaSilver,
                secondary = CinemaBlueLight,
                onSecondary = CinemaSilver,
                tertiary = CinemaGold,
                background = CinemaBlueDeep,
                onBackground = CinemaSilver,
                surface = CinemaBlueDeep,
                onSurface = CinemaSilver,
                surfaceVariant = CinemaBlueMid
            )
        }
        AppTheme.CINEMA_RED -> {
            darkColorScheme(
                primary = CinemaRedGold,
                onPrimary = CinemaBurgundy,
                secondary = CinemaRedLight,
                onSecondary = CinemaRedSilver,
                tertiary = CinemaRedGoldDark,
                background = CinemaBurgundy,
                onBackground = CinemaRedSilver,
                surface = CinemaBurgundy,
                onSurface = CinemaRedSilver
            )
        }
        AppTheme.DEFAULT -> {
            darkColorScheme(
                primary = EmberPlum,
                onPrimary = EmberCream,
                secondary = EmberAmber,
                onSecondary = EmberNight,
                tertiary = EmberGold,
                background = EmberNight,
                onBackground = EmberCream,
                surface = EmberNight,
                onSurface = EmberCream,
                surfaceVariant = EmberPlum,
                onSurfaceVariant = EmberCreamMuted
            )
        }
        AppTheme.DARK -> darkColorScheme(primary = Purple80, secondary = PurpleGrey80, tertiary = Pink80)
        AppTheme.LIGHT, AppTheme.AUTO -> lightColorScheme(primary = Purple40, secondary = PurpleGrey40, tertiary = Pink40)
    }

    CompositionLocalProvider(LocalThemeSpec provides appTheme.spec()) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = Typography,
            content = content
        )
    }
}
