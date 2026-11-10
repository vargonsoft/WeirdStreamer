package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vargon.weirdstreamer.ui.theme.AppTheme
import com.vargon.weirdstreamer.ui.theme.ThemePalette
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import com.vargon.weirdstreamer.ui.theme.palette

// The theme pickers (TV and phone Settings, Themes): a tile per theme, drawn in its own colors like
// the web UI's: its background, a box with a heading and a line of text, and a button.

/** A theme's preview. Auto shows Light and Dark side by side, the two it follows. */
@Composable
fun ThemePreview(theme: AppTheme, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(5.dp)
    if (theme == AppTheme.AUTO) {
        Row(modifier.clip(shape)) {
            PalettePreview(AppTheme.LIGHT.palette(), Modifier.weight(1f).fillMaxSize(), half = true)
            PalettePreview(AppTheme.DARK.palette(), Modifier.weight(1f).fillMaxSize(), half = true)
        }
    } else {
        PalettePreview(theme.palette(), modifier.clip(shape))
    }
}

@Composable
private fun PalettePreview(p: ThemePalette, modifier: Modifier, half: Boolean = false) {
    Box(modifier.background(p.background).padding(7.dp)) {
        Box(Modifier.fillMaxSize().clip(RoundedCornerShape(4.dp)).background(p.panel)) {
            Box(Modifier.offset(7.dp, 8.dp).fillMaxWidth(0.4f).height(4.dp).clip(RoundedCornerShape(2.dp)).background(p.accent))
            Box(Modifier.offset(7.dp, 17.dp).fillMaxWidth(0.55f).height(4.dp).clip(RoundedCornerShape(2.dp)).background(p.text.copy(alpha = 0.6f)))
            // Half a tile (Auto's) has no room for the button's label: just the button.
            if (half) Box(Modifier.align(Alignment.BottomEnd).padding(5.dp).size(16.dp, 10.dp).clip(RoundedCornerShape(3.dp)).background(p.button))
            else androidx.compose.material3.Text(
                "Aa", color = p.buttonLabel, fontSize = 10.sp, lineHeight = 14.sp, maxLines = 1, softWrap = false,
                modifier = Modifier.align(Alignment.BottomEnd).padding(5.dp)
                    .clip(RoundedCornerShape(3.dp)).background(p.button).padding(horizontal = 6.dp),
            )
        }
    }
}

/** The theme in use's outline color for the picked tile: the accent, as the web's. */
@Composable
private fun pickedColor(): Color = WeirdStreamerTheme.spec.headingColor.takeOrElse { androidx.compose.material3.MaterialTheme.colorScheme.primary }

/** The phone's theme picker: a grid of tiles; a tap picks the theme ([onPick]) at once. */
@Composable
fun PhoneThemePicker(current: AppTheme, onPick: (AppTheme) -> Unit, modifier: Modifier = Modifier) {
    val picked = pickedColor()
    LazyVerticalGrid(
        columns = GridCells.Adaptive(104.dp),
        modifier = modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(12.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(AppTheme.entries) { theme ->
            val selected = theme == current
            Column(
                Modifier
                    .clip(RoundedCornerShape(8.dp))
                    .border(2.dp, if (selected) picked else Color.Transparent, RoundedCornerShape(8.dp))
                    .clickable { onPick(theme) }
                    .padding(6.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                ThemePreview(theme, Modifier.fillMaxWidth().height(58.dp))
                Spacer(Modifier.height(6.dp))
                ThemeName(theme, selected, androidx.compose.material3.MaterialTheme.typography.bodyMedium, androidx.compose.material3.MaterialTheme.colorScheme.onSurface)
            }
        }
    }
}

/**
 * The TV's theme picker: rows of [columns] tiles; OK picks the theme ([onPick]) at once. Up from
 * the first row goes to [up] (the tabs); [currentFocus] is the picked theme's tile, for coming
 * into the picker on it.
 */
@Composable
fun TvThemePicker(
    current: AppTheme,
    onPick: (AppTheme) -> Unit,
    up: FocusRequester,
    currentFocus: FocusRequester,
    columns: Int = 7,
    previewHeight: Dp = 54.dp,
) {
    val picked = pickedColor()
    val shape = RoundedCornerShape(8.dp)
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        AppTheme.entries.chunked(columns).forEachIndexed { r, row ->
            Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                row.forEach { theme ->
                    val selected = theme == current
                    Column(
                        Modifier
                            .weight(1f)
                            .then(if (selected) Modifier.focusRequester(currentFocus) else Modifier)
                            .focusProperties { if (r == 0) this.up = up }
                            .tvClickable(shape = shape, focusedScale = 1.06f, borderWidth = 3.dp) { onPick(theme) }
                            .border(2.dp, if (selected) picked else Color.Transparent, shape)
                            .padding(8.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        ThemePreview(theme, Modifier.fillMaxWidth().height(previewHeight))
                        Spacer(Modifier.height(6.dp))
                        ThemeName(theme, selected, androidx.tv.material3.MaterialTheme.typography.bodySmall, androidx.tv.material3.MaterialTheme.colorScheme.onSurface)
                    }
                }
                // A short last row keeps the tiles' width.
                repeat(columns - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

@Composable
private fun ThemeName(theme: AppTheme, selected: Boolean, style: androidx.compose.ui.text.TextStyle, color: Color) {
    androidx.compose.material3.Text(
        if (theme == AppTheme.DEFAULT) "Default (Ember)" else theme.label,
        style = style,
        color = color,
        fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal,
        textAlign = TextAlign.Center,
        maxLines = 2,
    )
}
