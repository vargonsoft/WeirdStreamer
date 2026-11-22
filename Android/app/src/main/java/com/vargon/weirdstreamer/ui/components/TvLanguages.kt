package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
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
import com.vargon.weirdstreamer.api.models.AccountSettings
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.launch

/** "File default" for "", else the language's name in this device's language. */
fun accountLanguageName(code: String) = if (code.isEmpty()) "File default" else languageName(code) ?: code

/** The TV Settings row's summary: "Audio English · Subtitles File default, on". */
fun accountLanguagesSummary(a: AccountSettings?): String = if (a == null) "Loading…" else
    "Audio ${accountLanguageName(a.audioLanguage)} · Subtitles ${accountLanguageName(a.subtitleLanguage)}, " +
        if (a.subtitlesOn) "shown by default" else "off by default"

/**
 * The TV's Languages dialog: the account's audio and subtitle languages and whether subtitles start
 * on, kept on the server for every device (as the phone's Settings › Account and the web's). Left and
 * Right step through the languages; OK opens the list.
 */
@Composable
fun TvLanguagesDialog(server: ServerRepository, account: AccountSettings, onChanged: (AccountSettings) -> Unit, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var current by remember { mutableStateOf(account) }
    var error by remember { mutableStateOf<String?>(null) }
    var listing by remember { mutableStateOf<Boolean?>(null) } // true: audio, false: subtitles
    val codes = remember(account) { listOf("") + account.languages.orEmpty().sortedBy { accountLanguageName(it).lowercase() } }
    fun save(next: AccountSettings) {
        val before = current
        current = next
        scope.launch {
            val ok = runCatching { server.api.setAccount(next.copy(languages = null)).isSuccessful }.getOrDefault(false)
            if (ok) { error = null; onChanged(next) } else { current = before; error = "Couldn't save the setting" }
        }
    }
    fun step(audio: Boolean, by: Int) {
        val now = if (audio) current.audioLanguage else current.subtitleLanguage
        val code = codes[((codes.indexOf(now).coerceAtLeast(0) + by) % codes.size + codes.size) % codes.size]
        save(if (audio) current.copy(audioLanguage = code) else current.copy(subtitleLanguage = code))
    }

    val spec = WeirdStreamerTheme.spec
    val panel = spec.panelColor.takeOrElse { androidx.tv.material3.MaterialTheme.colorScheme.surface }
    val focus = spec.focusColor.takeOrElse { Color.White }
    val onFocus = spec.focusLabel.takeOrElse { Color.Black }
    val text = androidx.tv.material3.MaterialTheme.colorScheme.onSurface
    // Each row's focus: the first at the start, and the one the list was opened from when it closes.
    val rowFocus = remember { List(3) { FocusRequester() } }
    var lastRow by remember { mutableStateOf(0) }
    LaunchedEffect(listing) { if (listing == null) runCatching { rowFocus[lastRow].requestFocus() } }

    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.width(720.dp).background(panel, RoundedCornerShape(16.dp)).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            androidx.tv.material3.Text("Languages", style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                color = spec.headingColor.takeOrElse { text })
            androidx.tv.material3.Text(
                "Videos start with audio in this language when they have it, and with subtitles in this one if subtitles are shown by default. For your account, on every device.",
                style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                color = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant)
            val rows = listOf(
                Triple("Audio", accountLanguageName(current.audioLanguage), 0),
                Triple("Subtitles", accountLanguageName(current.subtitleLanguage), 1),
                Triple("Show subtitles by default", if (current.subtitlesOn) "On" else "Off", 2),
            )
            rows.forEach { (label, value, i) ->
                var focused by remember { mutableStateOf(false) }
                Row(
                    Modifier.fillMaxWidth()
                        .focusRequester(rowFocus[i])
                        .onFocusChanged { focused = it.isFocused; if (it.isFocused) lastRow = i }
                        .onPreviewKeyEvent { e ->
                            if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                            when (e.key) {
                                Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> {
                                    if (i == 2) save(current.copy(subtitlesOn = !current.subtitlesOn)) else listing = i == 0
                                    true
                                }
                                Key.DirectionRight, Key.DirectionLeft -> {
                                    val by = if (e.key == Key.DirectionRight) 1 else -1
                                    if (i == 2) save(current.copy(subtitlesOn = !current.subtitlesOn)) else step(i == 0, by)
                                    true
                                }
                                else -> false
                            }
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
            error?.let {
                androidx.tv.material3.Text(it, color = androidx.tv.material3.MaterialTheme.colorScheme.error,
                    style = androidx.tv.material3.MaterialTheme.typography.bodyMedium)
            }
        }
    }

    // The whole list, for picking one without stepping through the others.
    listing?.let { audio ->
        val now = if (audio) current.audioLanguage else current.subtitleLanguage
        val start = codes.indexOf(now).coerceAtLeast(0)
        val chosen = remember { FocusRequester() }
        val list = rememberLazyListState(initialFirstVisibleItemIndex = (start - 3).coerceAtLeast(0))
        LaunchedEffect(Unit) { runCatching { chosen.requestFocus() } }
        Dialog(onDismissRequest = { listing = null }, properties = DialogProperties(usePlatformDefaultWidth = false)) {
            Column(Modifier.width(520.dp).background(panel, RoundedCornerShape(16.dp)).padding(24.dp)) {
                androidx.tv.material3.Text(if (audio) "Audio language" else "Subtitle language",
                    style = androidx.tv.material3.MaterialTheme.typography.headlineSmall, color = spec.headingColor.takeOrElse { text },
                    modifier = Modifier.padding(bottom = 12.dp))
                LazyColumn(state = list, modifier = Modifier.heightIn(max = 420.dp)) {
                    itemsIndexed(codes) { i, code ->
                        var focused by remember { mutableStateOf(false) }
                        Row(
                            Modifier.fillMaxWidth()
                                .then(if (i == start) Modifier.focusRequester(chosen) else Modifier)
                                .onFocusChanged { focused = it.isFocused }
                                .onPreviewKeyEvent { e ->
                                    if (e.type == KeyEventType.KeyDown && e.key in listOf(Key.DirectionCenter, Key.Enter, Key.NumPadEnter)) {
                                        save(if (audio) current.copy(audioLanguage = code) else current.copy(subtitleLanguage = code))
                                        listing = null
                                        true
                                    } else false
                                }
                                .focusable()
                                .background(if (focused) focus else Color.Transparent, RoundedCornerShape(8.dp))
                                .padding(horizontal = 14.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            val color = if (focused) onFocus else text
                            androidx.tv.material3.Text(accountLanguageName(code), color = color, modifier = Modifier.weight(1f),
                                style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                            if (code == now) androidx.tv.material3.Text("✓", color = color,
                                style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
                        }
                    }
                }
            }
        }
    }
}
