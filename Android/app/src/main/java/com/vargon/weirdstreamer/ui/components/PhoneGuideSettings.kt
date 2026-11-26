package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Save
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.repository.ServerRepository

// The phone's Guide tab (admins): typed channels, each library's Show in Guide and Use for Guide
// Filler, and the schedule, on the shared GuideSettingsModel (AdminSettings.kt); saved with its button.

internal fun libraryKind(type: String) = when (type) { "movie" -> "Movies"; "tv" -> "TV Shows"; else -> "Folders" }

/** What a typed channel's card says of who sees it, and of its numbers and channels. */
internal fun typedSummary(m: GuideSettingsModel, t: TypedEdit): Pair<String, String> {
    val groups = m.groupsFor(t)
    val seen = when {
        groups == null -> "Choose its libraries."
        groups.isEmpty() -> "No group can open all of these libraries: no one will see it."
        else -> "Seen by: ${groups.joinToString(", ")}."
    }
    val numbers = if (t.number > 0) "Numbers ${t.number}–${t.number + 9}. " else "Numbered when saved. "
    val chans = t.channels.joinToString(", ") { "${it.number} ${it.name}" }
    return seen to numbers + if (chans.isNotEmpty()) "Now: $chans." else "Its channels are made at the next planning, as its content allows."
}

@Composable
fun GuideSettingsTab(server: ServerRepository) {
    val m = rememberGuideSettings(server)
    val data = m.data
    when {
        data == null && m.loadError != null -> AdminOnly(m.loadError!!)
        data == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> LazyColumn(contentPadding = PaddingValues(16.dp)) {
            item {
                SectionTitle("Typed channels")
                Text("Channels made only from the libraries you pick, of one type, numbered from 100. Only the groups that can open " +
                    "every one of their libraries see them, and those libraries are in no other channel. Late Night airs everything " +
                    "in its libraries, all day; the other types take their TMDB titles of that genre.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(m.typed, key = { System.identityHashCode(it) }) { t ->
                Card(Modifier.fillMaxWidth().padding(top = 10.dp)) {
                    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Box(Modifier.weight(1f)) { LibraryOptionMenu("Type", m.types, t.type) { t.type = it } }
                            IconButton(onClick = { m.removeTyped(t) }) {
                                Icon(Icons.Default.Delete, contentDescription = "Remove", tint = MaterialTheme.colorScheme.error)
                            }
                        }
                        LibraryOptionMenu("Channels", (1..data.maxChannels).map { "$it" to "Up to $it" }, "${t.max}") { t.max = it.toInt() }
                        Text("Libraries", style = MaterialTheme.typography.labelLarge)
                        data.libraries.forEach { l ->
                            val other = m.usedElsewhere(t, l.id)
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Checkbox(checked = l.id in t.libraries, enabled = !other || l.id in t.libraries,
                                    onCheckedChange = { m.toggleLibrary(t, l.id) })
                                Column {
                                    Text(l.name)
                                    val groups = l.groups.joinToString(", ").ifEmpty { "no group" }
                                    Text("${libraryKind(l.type)} · $groups" + if (other) " · another typed channel has it" else "",
                                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                        }
                        val (seen, numbers) = typedSummary(m, t)
                        Text(seen, style = MaterialTheme.typography.bodySmall,
                            color = if (m.groupsFor(t).isNullOrEmpty()) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(numbers, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
            item {
                OutlinedButton(onClick = { m.addTyped() }, enabled = m.canAdd, shape = AppButtonShape, modifier = Modifier.padding(top = 10.dp)) {
                    Icon(Icons.Default.Add, contentDescription = null)
                    Spacer(Modifier.width(6.dp)); ButtonLabel("Add typed channels")
                }
            }
            item {
                SectionTitle("Libraries")
                Text("Show in Guide puts a movie or TV library's titles on the channels, by network and genre. Use for Guide Filler " +
                    "lends its videos of 15 minutes or less to the gaps between programs.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(m.libraries, key = { it.library.id }) { l ->
                Column(Modifier.padding(top = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(l.library.name, style = MaterialTheme.typography.titleSmall)
                    when {
                        m.inTyped(l.library.id) -> Text("On its typed channels", style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                        l.airs -> LibraryOptionMenu("Show in Guide", libraryRowModes, l.showInGuide) { l.showInGuide = it }
                        else -> Text("${libraryKind(l.library.type)}: only typed channels (Late Night) take it",
                            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("Use for Guide Filler", modifier = Modifier.weight(1f))
                        Switch(checked = l.guideFiller, onCheckedChange = { l.guideFiller = it })
                    }
                    HorizontalDivider()
                }
            }
            item {
                SectionTitle("Schedule")
                LibraryOptionMenu("Prime time (then the news hour)", primeTimes, m.primeTime) { m.primeTime = it }
                OutlinedTextField(m.weatherZip, { v -> m.weatherZip = v.filter { it.isDigit() }.take(5) },
                    label = { Text("Weather ZIP code") }, placeholder = { Text("10001") }, singleLine = true,
                    supportingText = { Text("The Weather channel's forecast is the National Weather Service's for it (US only)") },
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
                Text(guideExtras(data.musicTracks, data.voiceOvers), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            item {
                Message(m.message?.first, m.message?.second ?: false)
                Button(shape = AppButtonShape, colors = phoneButtonColors(), enabled = !m.saving, onClick = { m.save() },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
                    Icon(Icons.Default.Save, contentDescription = null)
                    Spacer(Modifier.width(6.dp)); ButtonLabel(if (m.saving) "Saving…" else "Save Guide settings")
                }
            }
        }
    }
}

/** The music and voice-overs the server has, for the Schedule section. */
internal fun guideExtras(tracks: Int, voiceOvers: Boolean): String {
    val music = if (tracks > 0) "Music: $tracks track${if (tracks == 1) "" else "s"}." else "Music: none (the setup's Guide step adds it)."
    val voices = if (voiceOvers) "Voice-overs: on." else "Voice-overs: off (Piper isn't set up; the setup's Guide step adds it)."
    return "$music $voices"
}
