package com.vargon.weirdstreamer.ui.theme

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.Color

@Composable
fun WeirdStreamerPhoneTheme(
    appTheme: AppTheme = AppTheme.DEFAULT,
    content: @Composable () -> Unit,
) {
    val p = appTheme.palette()
    // The screen is the background and the boxes (Panel) are the panel color. Lists, tab rows and
    // top bars are transparent (surface) so they show the box they sit in, see-through or not;
    // dialogs, menus and sheets (the surface containers) get the box's color, opaque.
    val colorScheme = if (p.isLight) {
        lightColorScheme(
            primary = p.accent, onPrimary = p.onAccent,
            primaryContainer = p.raised, onPrimaryContainer = p.text,
            secondary = p.secondary, onSecondary = p.background,
            secondaryContainer = p.buttonSecondary, onSecondaryContainer = p.buttonSecondaryLabel, // tonal buttons: the quieter ones
            tertiary = p.accent, onTertiary = p.onAccent,
            background = p.background, onBackground = p.text,
            surface = Color.Transparent, onSurface = p.text,
            surfaceVariant = p.raised, onSurfaceVariant = p.mutedText,
            surfaceTint = Color.Transparent,
            inverseSurface = p.text, inverseOnSurface = p.background,
            outline = p.mutedText, outlineVariant = p.raised,
            surfaceContainerLowest = p.panel, surfaceContainerLow = p.panel,
            surfaceContainer = p.panel, surfaceContainerHigh = p.panel,
            surfaceContainerHighest = p.raised,
        )
    } else {
        darkColorScheme(
            primary = p.accent, onPrimary = p.onAccent,
            primaryContainer = p.raised, onPrimaryContainer = p.text,
            secondary = p.secondary, onSecondary = p.background,
            secondaryContainer = p.buttonSecondary, onSecondaryContainer = p.buttonSecondaryLabel, // tonal buttons: the quieter ones
            tertiary = p.accent, onTertiary = p.onAccent,
            background = p.background, onBackground = p.text,
            surface = Color.Transparent, onSurface = p.text,
            surfaceVariant = p.raised, onSurfaceVariant = p.mutedText,
            surfaceTint = Color.Transparent,
            inverseSurface = p.text, inverseOnSurface = p.background,
            outline = p.mutedText, outlineVariant = p.raised,
            surfaceContainerLowest = p.panel, surfaceContainerLow = p.panel,
            surfaceContainer = p.panel, surfaceContainerHigh = p.panel,
            surfaceContainerHighest = p.raised,
        )
    }

    CompositionLocalProvider(LocalThemeSpec provides appTheme.spec()) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = if (p.monospace) PhoneMonospaceTypography else MaterialTheme.typography,
            content = content
        )
    }
}
