package com.vargon.weirdstreamer.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider

@Composable
fun WeirdStreamerPhoneTheme(
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val colorScheme = when (appTheme.resolved()) {
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
            darkColorScheme(
                primary = EmberGold,
                onPrimary = EmberAubergine,
                primaryContainer = EmberPlum,
                onPrimaryContainer = EmberCream,
                secondary = EmberAmber,
                onSecondary = EmberNight,
                secondaryContainer = EmberButton, // tonal buttons, like the filled ones
                onSecondaryContainer = EmberCream,
                tertiary = EmberGold,
                background = EmberNight, // behind the boxes (Panel)
                onBackground = EmberCream,
                surface = EmberAubergine, // the boxes: lists, tab rows and top bars inside them blend in
                onSurface = EmberCream,
                surfaceVariant = EmberPlum,
                onSurfaceVariant = EmberCreamMuted,
                surfaceContainer = EmberAubergine,
                surfaceContainerHigh = EmberAubergine,
                surfaceContainerHighest = EmberPlum,
                outline = EmberCreamMuted,
                outlineVariant = EmberPlum
            )
        }
        AppTheme.DARK -> darkColorScheme(primary = Purple80, secondary = PurpleGrey80, tertiary = Pink80)
        AppTheme.LIGHT, AppTheme.AUTO -> lightColorScheme(primary = Purple40, secondary = PurpleGrey40, tertiary = Pink40)
    }

    CompositionLocalProvider(LocalThemeSpec provides appTheme.spec()) {
        MaterialTheme(
            colorScheme = colorScheme,
            content = content
        )
    }
}
