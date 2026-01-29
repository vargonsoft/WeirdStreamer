package com.vargon.weirdstreamer.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider

@Composable
fun WeirdStreamerPhoneTheme(
    isInDarkTheme: Boolean = isSystemInDarkTheme(),
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val colorScheme = when (appTheme) {
        AppTheme.CINEMA_BLUE -> {
            darkColorScheme(
                primary = CinemaGold,
                onPrimary = CinemaBlueDeep,
                secondary = CinemaBlueLight,
                onSecondary = CinemaSilver,
                tertiary = CinemaGoldDark,
                background = CinemaBlueDeep,
                onBackground = CinemaSilver,
                surface = CinemaBlueMid,
                onSurface = CinemaSilver,
                surfaceVariant = CinemaBlue
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
                surface = CinemaRed,
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
            content = content
        )
    }
}
