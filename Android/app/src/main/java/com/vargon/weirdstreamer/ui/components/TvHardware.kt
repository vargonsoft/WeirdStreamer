package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.google.gson.JsonObject
import com.vargon.weirdstreamer.api.models.HardwareReport
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.launch

/** The TV Settings row's summary: the chosen encoder, or what the server has. */
fun hardwareSummary(report: HardwareReport?, config: JsonObject?): String = when {
    report == null || config == null -> "Loading…"
    report.checking -> "Checking the server's graphics hardware…"
    report.encoders.isNullOrEmpty() -> "No usable GPU: transcodes are encoded by the CPU"
    else -> {
        val chosen = HardwareSettings.chosenKey(report, HardwareSettings.encoderOf(config), HardwareSettings.deviceOf(config))
        val label = HardwareSettings.choices(report, HardwareSettings.encoderOf(config), HardwareSettings.deviceOf(config))
            .firstOrNull { it.first == chosen }?.second ?: "The CPU (x264)"
        if (chosen.isEmpty()) "Transcodes on the CPU (x264)"
        else "$label · ${HardwareSettings.limitLabel(HardwareSettings.maxOf(config))}"
    }
}

/**
 * The TV's Hardware encoding dialog (admins): the server's graphics hardware and, when it has a
 * working encoder, which one encodes transcodes and how many at once. Left and Right change a
 * row's value, saved on the server at once (the config sent back whole, as the phone's Save does).
 */
@Composable
fun TvHardwareDialog(server: ServerRepository, report: HardwareReport, config: JsonObject,
                     onChanged: (JsonObject) -> Unit, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var current by remember { mutableStateOf(config) }
    var error by remember { mutableStateOf<String?>(null) }
    val choices = remember(report) {
        HardwareSettings.choices(report, HardwareSettings.encoderOf(config), HardwareSettings.deviceOf(config))
    }
    val chosen = HardwareSettings.chosenKey(report, HardwareSettings.encoderOf(current), HardwareSettings.deviceOf(current))
    val max = HardwareSettings.maxOf(current)
    val usable = !report.checking && !report.encoders.isNullOrEmpty()

    fun save(choice: String, limit: Int) {
        val before = current
        val next = current.deepCopy().also { HardwareSettings.apply(it, choice, limit) }
        current = next
        scope.launch {
            val ok = runCatching { server.api.updateConfig(next).isSuccessful }.getOrDefault(false)
            if (ok) { error = null; onChanged(next) } else { current = before; error = "Couldn't save the setting" }
        }
    }

    val spec = WeirdStreamerTheme.spec
    val panel = spec.panelColor.takeOrElse { androidx.tv.material3.MaterialTheme.colorScheme.surface }
    val focus = spec.focusColor.takeOrElse { Color.White }
    val onFocus = spec.focusLabel.takeOrElse { Color.Black }
    val text = androidx.tv.material3.MaterialTheme.colorScheme.onSurface
    val muted = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant
    val first = remember { FocusRequester() }
    LaunchedEffect(usable) { runCatching { first.requestFocus() } }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.width(760.dp).background(panel, RoundedCornerShape(16.dp)).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            androidx.tv.material3.Text("Hardware encoding", style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                color = spec.headingColor.takeOrElse { text })
            androidx.tv.material3.Text(HardwareSettings.status(report),
                style = androidx.tv.material3.MaterialTheme.typography.bodySmall, color = muted)
            val rows = buildList {
                if (usable) add(Triple("Encode transcodes on", choices.firstOrNull { it.first == chosen }?.second ?: "The CPU (x264)", 0))
                if (usable && chosen.isNotEmpty()) add(Triple("Transcodes on the GPU at once", HardwareSettings.limitLabel(max), 1))
            }
            rows.forEachIndexed { n, (label, value, kind) ->
                var focused by remember { mutableStateOf(false) }
                Row(
                    Modifier.fillMaxWidth()
                        .then(if (n == 0) Modifier.focusRequester(first) else Modifier)
                        .onFocusChanged { focused = it.isFocused }
                        .onPreviewKeyEvent { e ->
                            if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                            val by = when (e.key) {
                                Key.DirectionRight, Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> 1
                                Key.DirectionLeft -> -1
                                else -> return@onPreviewKeyEvent false
                            }
                            if (kind == 0) {
                                val i = choices.indexOfFirst { it.first == chosen }.coerceAtLeast(0)
                                save(choices[((i + by) % choices.size + choices.size) % choices.size].first, max)
                            } else {
                                save(chosen, (max + by).coerceIn(0, 32))
                            }
                            true
                        }
                        .focusable()
                        .background(if (focused) focus else Color.Transparent, RoundedCornerShape(8.dp))
                        .padding(horizontal = 14.dp, vertical = 8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    val color = if (focused) onFocus else text
                    androidx.tv.material3.Text(label, color = color, modifier = Modifier.weight(1f),
                        style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                    androidx.tv.material3.Text(if (focused) "‹  $value  ›" else value, color = color,
                        style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                }
            }
            if (usable) {
                androidx.tv.material3.Text(HardwareSettings.note(report, chosen),
                    style = androidx.tv.material3.MaterialTheme.typography.bodySmall, color = muted)
            } else {
                // Nothing to change: a focusable line so Back and OK close it.
                var focused by remember { mutableStateOf(false) }
                androidx.tv.material3.Text("Close", color = if (focused) onFocus else text,
                    style = androidx.tv.material3.MaterialTheme.typography.titleMedium,
                    modifier = Modifier.focusRequester(first).onFocusChanged { focused = it.isFocused }
                        .onPreviewKeyEvent { e ->
                            if (e.type == KeyEventType.KeyDown && e.key in listOf(Key.DirectionCenter, Key.Enter, Key.NumPadEnter)) { onDismiss(); true } else false
                        }
                        .focusable()
                        .background(if (focused) focus else Color.Transparent, RoundedCornerShape(8.dp))
                        .padding(horizontal = 14.dp, vertical = 8.dp))
            }
            error?.let {
                androidx.tv.material3.Text(it, color = androidx.tv.material3.MaterialTheme.colorScheme.error,
                    style = androidx.tv.material3.MaterialTheme.typography.bodyMedium)
            }
        }
    }
}
