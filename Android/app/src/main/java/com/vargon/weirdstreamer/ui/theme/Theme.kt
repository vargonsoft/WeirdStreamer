package com.vargon.weirdstreamer.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.tv.material3.*
import com.vargon.weirdstreamer.R

enum class AppTheme {
    DEFAULT, CINEMA_BLUE, CINEMA_RED
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

object CinemaBlueThemeSpec : WeirdStreamerThemeSpec {
    override val folderIconTint: Color = CinemaGold
    override val serverIconTint: Color = CinemaGold
    override val parentIconTint: Color = CinemaGold
    override val backgroundImage: Int? = R.drawable.film_reel
    override val spinnerColor: Color = CinemaBlueDeep

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

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun WeirdStreamerTheme(
    isInDarkTheme: Boolean = isSystemInDarkTheme(),
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val colorScheme = when (appTheme) {
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
            if (isInDarkTheme) {
                darkColorScheme(
                    primary = Purple80,
                    secondary = PurpleGrey80,
                    tertiary = Pink80
                )
            } else {
                lightColorScheme(
                    primary = Purple40,
                    secondary = PurpleGrey40,
                    tertiary = Pink40
                )
            }
        }
    }

    val themeSpec = when (appTheme) {
        AppTheme.CINEMA_BLUE -> CinemaBlueThemeSpec
        AppTheme.CINEMA_RED -> CinemaRedThemeSpec
        else -> DefaultThemeSpec
    }

    CompositionLocalProvider(LocalThemeSpec provides themeSpec) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = Typography,
            content = content
        )
    }
}
