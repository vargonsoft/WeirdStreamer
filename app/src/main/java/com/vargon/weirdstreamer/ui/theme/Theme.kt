package com.vargon.weirdstreamer.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.Composable
import androidx.tv.material3.ExperimentalTvMaterial3Api
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.darkColorScheme
import androidx.tv.material3.lightColorScheme

enum class AppTheme {
    DEFAULT, CINEMA_BLUE, CINEMA_RED
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
                primary = CinemaGold,
                onPrimary = CinemaNavy,
                secondary = CinemaBlueLight,
                onSecondary = CinemaSilver,
                tertiary = CinemaGoldDark,
                background = CinemaNavy,
                onBackground = CinemaSilver,
                surface = CinemaBlue,
                onSurface = CinemaSilver
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
    MaterialTheme(
        colorScheme = colorScheme,
        typography = Typography,
        content = content
    )
}