package com.vargon.weirdstreamer.ui.theme

import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import androidx.tv.material3.ExperimentalTvMaterial3Api
import androidx.tv.material3.Typography

// Set of Material typography styles to start with
@OptIn(ExperimentalTvMaterial3Api::class)
val Typography = Typography(
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 16.sp,
        lineHeight = 24.sp,
        letterSpacing = 0.5.sp
    )
    /* Other default text styles to override
    titleLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 22.sp,
        lineHeight = 28.sp,
        letterSpacing = 0.sp
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 16.sp,
        letterSpacing = 0.5.sp
    )
    */
)
// The terminal themes: every style in a monospace font
private val mono = TextStyle(fontFamily = FontFamily.Monospace)

@OptIn(ExperimentalTvMaterial3Api::class)
val MonospaceTypography = Typography.run {
    Typography(
        displayLarge = displayLarge.merge(mono), displayMedium = displayMedium.merge(mono), displaySmall = displaySmall.merge(mono),
        headlineLarge = headlineLarge.merge(mono), headlineMedium = headlineMedium.merge(mono), headlineSmall = headlineSmall.merge(mono),
        titleLarge = titleLarge.merge(mono), titleMedium = titleMedium.merge(mono), titleSmall = titleSmall.merge(mono),
        bodyLarge = bodyLarge.merge(mono), bodyMedium = bodyMedium.merge(mono), bodySmall = bodySmall.merge(mono),
        labelLarge = labelLarge.merge(mono), labelMedium = labelMedium.merge(mono), labelSmall = labelSmall.merge(mono),
    )
}

val PhoneMonospaceTypography = androidx.compose.material3.Typography().run {
    androidx.compose.material3.Typography(
        displayLarge = displayLarge.merge(mono), displayMedium = displayMedium.merge(mono), displaySmall = displaySmall.merge(mono),
        headlineLarge = headlineLarge.merge(mono), headlineMedium = headlineMedium.merge(mono), headlineSmall = headlineSmall.merge(mono),
        titleLarge = titleLarge.merge(mono), titleMedium = titleMedium.merge(mono), titleSmall = titleSmall.merge(mono),
        bodyLarge = bodyLarge.merge(mono), bodyMedium = bodyMedium.merge(mono), bodySmall = bodySmall.merge(mono),
        labelLarge = labelLarge.merge(mono), labelMedium = labelMedium.merge(mono), labelSmall = labelSmall.merge(mono),
    )
}
