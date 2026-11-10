package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

// Settings → Subtitle style, on TV and phone: Android's caption settings or our own, with a preview.

/** The next (or previous) value in a list, wrapping around. */
private fun <T> List<T>.step(current: T, by: Int): T = this[((indexOf(current).coerceAtLeast(0) + by) % size + size) % size]

/** One line for the setting's row in Settings: "Android system", or "Custom · Large · Serif". */
fun subtitleStyleSummary(s: SubtitleStyleSetting): String =
    if (s.system) "Android system (Accessibility → Captions)"
    else listOf("Custom", s.size.label, s.font.label).joinToString(" · ")

/**
 * The options, as rows of label and value: every row changes the setting through [change], and
 * changing any option of the Custom style makes the style Custom.
 */
private class SubtitleStyleRow(val label: String, val value: String, val applies: Boolean, val step: (Int) -> SubtitleStyleSetting)

private fun subtitleStyleRows(s: SubtitleStyleSetting): List<SubtitleStyleRow> {
    val custom = s.copy(system = false)
    return listOf(
        SubtitleStyleRow("Style", if (s.system) "Android system" else "Custom", true) { s.copy(system = !s.system) },
        SubtitleStyleRow("Background", s.background.label, true) { by -> custom.copy(background = SubtitleBackground.entries.step(s.background, by)) },
        SubtitleStyleRow("Translucency", opacityLabel(s.opacity), s.background != SubtitleBackground.NONE && !s.outline) { by ->
            custom.copy(opacity = SUBTITLE_OPACITIES.step(SUBTITLE_OPACITIES.minBy { kotlin.math.abs(it - s.opacity) }, by))
        },
        SubtitleStyleRow("Text", s.foreground.label, true) { by -> custom.copy(foreground = SubtitleForeground.entries.step(s.foreground, by)) },
        SubtitleStyleRow("Text opacity", opacityLabel(s.textOpacity), true) { by ->
            custom.copy(textOpacity = SUBTITLE_OPACITIES.step(SUBTITLE_OPACITIES.minBy { kotlin.math.abs(it - s.textOpacity) }, by))
        },
        SubtitleStyleRow("Outline", if (s.outline) "On" else "Off", s.background != SubtitleBackground.NONE) { custom.copy(outline = !s.outline) },
        SubtitleStyleRow("Font", s.font.label, true) { by -> custom.copy(font = SubtitleFont.entries.step(s.font, by)) },
        SubtitleStyleRow("Size", s.size.label, true) { by -> custom.copy(size = SubtitleSize.entries.step(s.size, by)) },
    )
}

/** The TV's dialog: the preview beside the options; Left/Right (or OK) change the one focused. */
@Composable
fun TvSubtitleStyleDialog(onDismiss: () -> Unit) {
    val context = LocalContext.current
    var setting by remember { mutableStateOf(SubtitleStyleStore.get(context)) }
    fun change(s: SubtitleStyleSetting) { setting = s; SubtitleStyleStore.set(context, s) }
    val spec = WeirdStreamerTheme.spec
    val panel = spec.panelColor.takeOrElse { androidx.tv.material3.MaterialTheme.colorScheme.surface }
    val focus = spec.focusColor.takeOrElse { Color.White }
    val onFocus = spec.focusLabel.takeOrElse { Color.Black }
    val text = androidx.tv.material3.MaterialTheme.colorScheme.onSurface
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.width(900.dp).background(panel, RoundedCornerShape(16.dp)).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            androidx.tv.material3.Text("Subtitle style", style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                color = spec.headingColor.takeOrElse { text })
            Row(horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                Column(Modifier.weight(1f)) {
                    SubtitleStylePreview(setting, fontSize = 20.sp)
                    Spacer(Modifier.padding(top = 8.dp))
                    androidx.tv.material3.Text(
                        "Text subtitles only: picture ones (Blu-ray, DVD) keep their own look." +
                            if (setting.system) " Android's style is set in Settings → Accessibility → Captions." else "",
                        style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                        color = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Column(Modifier.weight(1.1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    subtitleStyleRows(setting).forEachIndexed { i, row ->
                        var focused by remember { mutableStateOf(false) }
                        val dimmed = !row.applies || (setting.system && i > 0)
                        Row(
                            Modifier.fillMaxWidth()
                                .then(if (i == 0) Modifier.focusRequester(first) else Modifier)
                                .onFocusChanged { focused = it.isFocused }
                                .onPreviewKeyEvent { e ->
                                    if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                                    when (e.key) {
                                        Key.DirectionRight, Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> { change(row.step(1)); true }
                                        Key.DirectionLeft -> { change(row.step(-1)); true }
                                        else -> false
                                    }
                                }
                                .focusable()
                                .background(if (focused) focus else Color.Transparent, RoundedCornerShape(8.dp))
                                .padding(horizontal = 14.dp, vertical = 8.dp)
                                .alpha(if (dimmed && !focused) 0.5f else 1f),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            val color = if (focused) onFocus else text
                            androidx.tv.material3.Text(row.label, color = color, modifier = Modifier.weight(1f),
                                style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                            androidx.tv.material3.Text(if (focused) "‹  ${row.value}  ›" else row.value, color = color,
                                style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                        }
                    }
                }
            }
        }
    }
}

/** The phone's dialog: the preview above the options, each a menu of its values. */
@Composable
fun PhoneSubtitleStyleDialog(onDismiss: () -> Unit) {
    val context = LocalContext.current
    var setting by remember { mutableStateOf(SubtitleStyleStore.get(context)) }
    fun change(s: SubtitleStyleSetting) { setting = s; SubtitleStyleStore.set(context, s) }
    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { androidx.compose.material3.Text("Subtitle style") },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                SubtitleStylePreview(setting, fontSize = 16.sp)
                androidx.compose.material3.Text(
                    "Text subtitles only: picture ones (Blu-ray, DVD) keep their own look." +
                        if (setting.system) " Android's style is set in Settings → Accessibility → Caption preferences." else "",
                    style = androidx.compose.material3.MaterialTheme.typography.bodySmall,
                    color = androidx.compose.material3.MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(vertical = 6.dp))
                subtitleStyleRows(setting).forEachIndexed { i, row ->
                    val options = phoneChoices(setting, i)
                    var open by remember { mutableStateOf(false) }
                    Box {
                        androidx.compose.material3.ListItem(
                            headlineContent = { androidx.compose.material3.Text(row.label) },
                            trailingContent = { androidx.compose.material3.Text(row.value) },
                            colors = androidx.compose.material3.ListItemDefaults.colors(containerColor = Color.Transparent),
                            modifier = Modifier.alpha(if (!row.applies || (setting.system && i > 0)) 0.5f else 1f)
                                .clickable { open = true }
                        )
                        androidx.compose.material3.DropdownMenu(border = phoneMenuBorder(), expanded = open, onDismissRequest = { open = false }) {
                            options.forEach { (label, value) ->
                                androidx.compose.material3.DropdownMenuItem(
                                    text = { androidx.compose.material3.Text(label) },
                                    onClick = { open = false; change(value) }
                                )
                            }
                        }
                    }
                }
            }
        },
        confirmButton = { PhoneButton(onClick = onDismiss) { androidx.compose.material3.Text("Done") } }
    )
}

/** The values a phone menu offers for row [i] of subtitleStyleRows, each as the setting it makes. */
private fun phoneChoices(s: SubtitleStyleSetting, i: Int): List<Pair<String, SubtitleStyleSetting>> {
    val c = s.copy(system = false)
    return when (i) {
        0 -> listOf("Android system" to s.copy(system = true), "Custom" to c)
        1 -> SubtitleBackground.entries.map { it.label to c.copy(background = it) }
        2 -> SUBTITLE_OPACITIES.map { opacityLabel(it) to c.copy(opacity = it) }
        3 -> SubtitleForeground.entries.map { it.label to c.copy(foreground = it) }
        4 -> SUBTITLE_OPACITIES.map { opacityLabel(it) to c.copy(textOpacity = it) }
        5 -> listOf("Off" to c.copy(outline = false), "On" to c.copy(outline = true))
        6 -> SubtitleFont.entries.map { it.label to c.copy(font = it) }
        else -> SubtitleSize.entries.map { it.label to c.copy(size = it) }
    }
}
