package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.api.models.TrickplaySettingsRequest
import com.vargon.weirdstreamer.api.models.TrickplaySettings
import kotlin.math.roundToInt
import androidx.compose.runtime.mutableLongStateOf
import com.vargon.weirdstreamer.api.models.JobLoad
import com.vargon.weirdstreamer.api.models.BackgroundJob
import com.vargon.weirdstreamer.api.models.JobsResponse
import com.vargon.weirdstreamer.api.models.StorageUse
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material.icons.filled.Check
import androidx.compose.foundation.layout.size
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.background
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.graphics.takeOrElse
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import androidx.compose.material3.FilterChip
import androidx.compose.material.icons.filled.Edit
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.PersonAdd
import androidx.compose.material.icons.filled.Save
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.StarBorder
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.foundation.selection.selectable
import androidx.compose.ui.semantics.Role
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.foundation.clickable
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.IntrinsicSize
import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.vargon.weirdstreamer.api.models.AdminStream
import com.vargon.weirdstreamer.api.models.ChangePasswordRequest
import com.vargon.weirdstreamer.repository.normalizePairingCode
import com.vargon.weirdstreamer.api.models.CreateUserRequest
import com.vargon.weirdstreamer.api.models.ServerUser
import com.vargon.weirdstreamer.repository.ServerRepository
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import retrofit2.Response
import java.util.Locale

// Server administration for the phone app: the same settings as the web UI's Settings page, drawn
// on the shared models in AdminSettings.kt (the TV's are TvAdminSettings.kt).

@Composable
private fun SectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(top = 20.dp, bottom = 8.dp))
}

@Composable
private fun Message(text: String?, isError: Boolean) {
    if (text == null) return
    Text(text, color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
        style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(vertical = 8.dp))
}

/** A button label that wraps with tight line spacing, centred. */
@Composable
private fun ButtonLabel(text: String) {
    Text(text, textAlign = TextAlign.Center, style = MaterialTheme.typography.labelLarge.copy(lineHeight = 16.sp))
}

@Composable
fun AdminOnly(message: String = "Only administrators can change these settings.") {
    Box(Modifier.fillMaxSize().padding(32.dp), contentAlignment = Alignment.Center) {
        Text(message, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** The groups that can see a library: chips for each defined group. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun LibraryGroups(all: List<String>, p: PathEdit) {
    val selected = p.groups
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(0.dp)) {
        Text("Groups:", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.align(Alignment.CenterVertically))
        if (all.isEmpty()) Text("none defined (add one under Groups)", style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.align(Alignment.CenterVertically))
        all.forEach { g ->
            val on = g in selected
            FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = on, border = phoneChipBorder(on), label = { Text(g) },
                leadingIcon = if (on) { { Icon(Icons.Default.Check, contentDescription = null, modifier = Modifier.size(18.dp)) } } else null,
                onClick = { p.toggleGroup(g, all) })
        }
    }
}

/** A line of a library's summary: its label in a column of their own, then its value. */
@Composable
private fun LibrarySummaryRow(label: String, value: @Composable () -> Unit) {
    Row(Modifier.padding(vertical = 3.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(96.dp).padding(end = 8.dp))
        value()
    }
}

// ---------------------------------------------------------------------------------------------
// Server tab: media paths, server, streaming and metadata settings, and a library scan.

@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun ServerSettingsTab(server: ServerRepository, onPlay: (path: String, name: String, duration: Double) -> Unit = { _, _, _ -> }) {
    val m = rememberServerSettings(server)
    val damaged = rememberDamagedVideos(server)
    var replacing by remember { mutableStateOf<com.vargon.weirdstreamer.api.models.DamagedVideo?>(null) }
    var emptying by remember { mutableStateOf(false) }
    var restoring by remember { mutableStateOf<com.vargon.weirdstreamer.api.models.TrashItem?>(null) }
    var showSubtitlesKey by remember { mutableStateOf(false) }
    var showToken by remember { mutableStateOf(false) }
    var newPath by remember { mutableStateOf("") }
    var newType by remember { mutableStateOf("") }
    var removing by remember { mutableStateOf<PathEdit?>(null) }
    var newGroupName by remember { mutableStateOf("") }
    var renaming by remember { mutableStateOf<String?>(null) }
    val paths = m.paths
    val groupNames = m.groupNames

    val loaded = m.config
    when {
        loaded == null && m.loadError != null -> AdminOnly(m.loadError!!)
        loaded == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> LazyColumn(contentPadding = PaddingValues(16.dp)) {
            item { SectionTitle("Media paths") }
            itemsIndexed(paths, key = { i, p -> "$i:${p.source.get("id")?.asString}" }) { _, p ->
                Card(modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)) {
                    if (!p.editing) Column(Modifier.padding(12.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(p.name.ifBlank { "(no name)" }, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                            IconButton(onClick = { p.editing = true }, modifier = Modifier.size(36.dp)) {
                                Icon(Icons.Default.Edit, contentDescription = "Edit", modifier = Modifier.size(20.dp))
                            }
                        }
                        Text(p.path, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        HorizontalDivider(Modifier.padding(vertical = 10.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.35f))
                        LibrarySummaryRow("Type") { Text(libraryTypes.firstOrNull { it.first == p.type }?.second ?: "No type", style = MaterialTheme.typography.bodyMedium) }
                        LibrarySummaryRow("Groups") {
                            if (p.groups.isEmpty()) Text("None (no one sees it)", style = MaterialTheme.typography.bodyMedium)
                            else FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                                p.groups.forEach { g ->
                                    Text(g, style = MaterialTheme.typography.labelMedium, color = WeirdStreamerTheme.spec.buttonLabel.takeOrElse { MaterialTheme.colorScheme.onSecondaryContainer },
                                        modifier = Modifier.clip(RoundedCornerShape(50)).background(WeirdStreamerTheme.spec.buttonColor.takeOrElse { MaterialTheme.colorScheme.secondaryContainer })
                                            .padding(horizontal = 10.dp, vertical = 2.dp))
                                }
                            }
                        }
                        LibrarySummaryRow("Continue Watching") { Text(libraryRowModes.firstOrNull { it.first == p.continueWatching }?.second ?: p.continueWatching, style = MaterialTheme.typography.bodyMedium) }
                        LibrarySummaryRow("Recently Added") { Text(libraryRowModes.firstOrNull { it.first == p.newAdditions }?.second ?: p.newAdditions, style = MaterialTheme.typography.bodyMedium) }
                    } else Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(p.name, { p.name = it }, label = { Text("Name") }, singleLine = true,
                            modifier = Modifier.fillMaxWidth())
                        LibraryGroups(groupNames, p)
                        LibraryOptionMenu("Library type", libraryTypes, p.type) { p.type = it }
                        LibraryOptionMenu("Continue Watching", libraryRowModes, p.continueWatching) { p.continueWatching = it }
                        LibraryOptionMenu("Recently Added", libraryRowModes, p.newAdditions) { p.newAdditions = it }
                        Text(p.path, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Spacer(Modifier.weight(1f))
                            IconButton(onClick = { removing = p }) {
                                Icon(Icons.Default.Delete, contentDescription = "Remove", tint = MaterialTheme.colorScheme.error)
                            }
                            // Back to the summary: Save applies the change, as before.
                            PhoneSecondaryButton(onClick = { p.editing = false }) {
                                Icon(Icons.Default.Check, contentDescription = null, modifier = Modifier.size(18.dp))
                                Spacer(Modifier.width(6.dp)); Text("Done")
                            }
                        }
                    }
                }
            }
            item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(newPath, { newPath = it }, label = { Text("New path on the server") },
                        placeholder = { Text("/path/to/media") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    LibraryOptionMenu("Library type", libraryTypes, newType) { newType = it }
                    OutlinedButton(shape = AppButtonShape, onClick = {
                        if (m.addPath(newPath, newType)) { newPath = ""; newType = "" }
                    }) {
                        Icon(Icons.Default.Add, contentDescription = null)
                        Spacer(Modifier.width(6.dp)); Text("Add path")
                    }
                }
            }
            item { SectionTitle("Groups") }
            item {
                Text("Each user is in one group, and each library lists the groups that can see it. " +
                    "New users start in the default group (the star makes another one the default). Changes here apply right away.", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(groupNames.toList(), key = { "group:$it" }) { g ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(g, style = MaterialTheme.typography.bodyLarge)
                    if (g == groupNames.firstOrNull()) { // new users start in it
                        Text("default", style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(start = 8.dp))
                    }
                    Spacer(Modifier.weight(1f))
                    if (g != groupNames.firstOrNull()) {
                        IconButton(onClick = { m.makeDefaultGroup(g) }) { Icon(Icons.Default.StarBorder, contentDescription = "Make $g the default group") }
                    }
                    IconButton(onClick = { renaming = g }) { Icon(Icons.Default.Edit, contentDescription = "Rename $g") }
                    IconButton(onClick = { m.deleteGroup(g) }) { Icon(Icons.Default.Delete, contentDescription = "Remove $g", tint = MaterialTheme.colorScheme.error) }
                }
            }
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(newGroupName, { newGroupName = it }, label = { Text("New group") }, singleLine = true,
                        modifier = Modifier.weight(1f))
                    OutlinedButton(shape = AppButtonShape, onClick = { m.createGroup(newGroupName) { newGroupName = "" } }) { Icon(Icons.Default.Add, contentDescription = null); Spacer(Modifier.width(6.dp)); Text("Add") }
                }
            }
            item { Message(m.groupMessage?.first, m.groupMessage?.second ?: false) }
            item { SectionTitle("Server") }
            item {
                OutlinedTextField(m.serverName, { m.serverName = it }, label = { Text("Name") }, singleLine = true,
                    supportingText = { Text("Shown in the apps' server lists") }, modifier = Modifier.fillMaxWidth())
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    OutlinedTextField(m.port, { m.port = it.filter(Char::isDigit) }, label = { Text("Port") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.weight(1f))
                    OutlinedTextField(m.host, { m.host = it }, label = { Text("Host") }, singleLine = true, keyboardOptions = addressKeyboard(), modifier = Modifier.weight(2f))
                }
            }
            item { SectionTitle("Streaming") }
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enable HTTP/2", modifier = Modifier.weight(1f))
                    Switch(checked = m.http2, onCheckedChange = { m.http2 = it })
                }
                // Background jobs run while the server has room to spare; this has them wait whenever anyone streams.
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("Pause background jobs while anyone is streaming")
                        Text("Finding intros and credits, trickplay frames, cleanup", style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Switch(checked = m.pauseJobs, onCheckedChange = { m.pauseJobs = it })
                }
                Text("Hardware encoding", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 16.dp, bottom = 4.dp))
                val report = m.hardware
                if (report == null) {
                    Text("Couldn't read the server's graphics m.hardware.", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    Text(HardwareSettings.status(report), style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                    if (!report.checking && !report.encoders.isNullOrEmpty()) {
                        val chosen = HardwareSettings.chosenKey(report, m.hwEncoder, m.hwDevice)
                        Column(Modifier.padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            LibraryOptionMenu("Encode transcodes on", HardwareSettings.choices(report, m.hwEncoder, m.hwDevice), chosen) {
                                m.hwEncoder = it.substringBefore('|', ""); m.hwDevice = if (it.isEmpty()) "" else it.substringAfter('|', "")
                            }
                            if (chosen.isNotEmpty()) {
                                OutlinedTextField(m.hwMax, { m.hwMax = it.filter(Char::isDigit) }, label = { Text("Transcodes on the GPU at once") },
                                    supportingText = { Text("0: no limit; the rest use the CPU") }, singleLine = true,
                                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                            }
                            Text(HardwareSettings.note(report, chosen), style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
            item { SectionTitle("Backups") }
            item {
                Text("Both databases (accounts, watch progress, matches, marked intros and credits) and config.yml. " +
                    "Restore one with bin/cli backup restore, with the server stopped.",
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Column(Modifier.padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    LibraryOptionMenu("Back up", BackupSettings.schedules, m.backupSchedule) { m.backupSchedule = it }
                    if (m.backupSchedule == "weekly") {
                        LibraryOptionMenu("On", BackupSettings.days, m.backupDay) { m.backupDay = it }
                    }
                    if (m.backupSchedule != "off") {
                        OutlinedTextField(m.backupTime, { m.backupTime = it.filter { ch -> ch.isDigit() || ch == ':' }.take(5) },
                            label = { Text("At (24-hour, the server's time)") }, placeholder = { Text("04:00") }, singleLine = true,
                            isError = !BackupSettings.validTime(m.backupTime),
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                    }
                    OutlinedTextField(m.backupKeep, { m.backupKeep = it.filter(Char::isDigit).take(3) }, label = { Text("Backups kept") },
                        supportingText = { Text("The newest; the oldest go") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.fillMaxWidth())
                }
                val list = m.backups
                Text(if (list == null) "Couldn't read the m.backups." else BackupSettings.where(list),
                    style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 8.dp))
                Message(m.backupMessage?.first, m.backupMessage?.second ?: false)
                PhoneSecondaryButton(onClick = { m.backUpNow() }, enabled = !m.backingUp, modifier = Modifier.padding(top = 4.dp)) {
                    ButtonLabel(if (m.backingUp) "Backing up…" else "Back up now")
                }
                list?.backups.orEmpty().forEach { b ->
                    Text("${BackupSettings.date(b.created)} · ${formatBytes(b.size)}", style = MaterialTheme.typography.bodySmall,
                        modifier = Modifier.padding(top = 4.dp))
                }
            }
            item { SectionTitle("Damaged videos") }
            item {
                Text(DAMAGED_ABOUT, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (damaged.videos?.isEmpty() == true) Text("None found.", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp))
                damaged.loadError?.let { Message("Couldn't read the damaged videos: $it", true) }
            }
            items(damaged.videos.orEmpty(), key = { "damaged:${it.path}" }) { v ->
                Card(modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
                    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        server.absoluteUrl(v.thumbnailUrl)?.let { thumb ->
                            coil.compose.AsyncImage(model = thumb, contentDescription = null,
                                contentScale = androidx.compose.ui.layout.ContentScale.Crop,
                                modifier = Modifier.fillMaxWidth().then(Modifier.aspectRatio(16f / 9f)).clip(RoundedCornerShape(6.dp)))
                        }
                        Text(v.name, style = MaterialTheme.typography.titleMedium)
                        Text(damagedWhere(v), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(damagedSizes(v), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            PhoneSecondaryButton(onClick = { damaged.play(v, original = true) { onPlay(v.path, "${v.name} (original)", 0.0) } }) {
                                Text("Play original")
                            }
                            if (v.fixable) {
                                PhoneButton(onClick = { damaged.fix(v) }) { Text(if (v.state == "failed") "Try fixing again" else "Fix") }
                            }
                            if (v.fixed) {
                                PhoneButton(onClick = { damaged.play(v, original = false) { onPlay(v.path, "${v.name} (fixed copy)", v.duration) } }) {
                                    Text("Play fixed copy")
                                }
                                PhoneButton(onClick = { replacing = v }) { Text("Replace with fixed copy") }
                            }
                        }
                    }
                }
            }
            item {
                Message(damaged.message?.first, damaged.message?.second ?: false)
                Text("Trash", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 8.dp))
                Text(trashText(damaged.trash), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(damaged.trash.items.orEmpty(), key = { "trash:${it.id}" }) { t ->
                Card(modifier = Modifier.fillMaxWidth().padding(top = 8.dp)) {
                    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        server.absoluteUrl(t.thumbnailUrl)?.let { thumb ->
                            coil.compose.AsyncImage(model = thumb, contentDescription = null,
                                contentScale = androidx.compose.ui.layout.ContentScale.Crop,
                                modifier = Modifier.fillMaxWidth().then(Modifier.aspectRatio(16f / 9f)).clip(RoundedCornerShape(6.dp)))
                        }
                        Text(t.name, style = MaterialTheme.typography.titleMedium)
                        Text(trashWhere(t), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(trashDates(t), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        if (t.restorable) {
                            PhoneSecondaryButton(onClick = { restoring = t }) { Text("Restore") }
                        }
                    }
                }
            }
            item {
                if (damaged.trash.files > 0) {
                    PhoneSecondaryButton(onClick = { emptying = true }, modifier = Modifier.padding(top = 8.dp)) { Text("Empty the trash now") }
                }
            }
            item { SectionTitle("Metadata") }
            item {
                Text("TMDB is used for libraries with a Movies or TV Shows type.", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(
                    m.tmdbToken, { m.tmdbToken = it }, label = { Text("TMDB API read access token") }, singleLine = true,
                    visualTransformation = if (showToken) VisualTransformation.None else PasswordVisualTransformation(),
                    keyboardOptions = secretKeyboard(),
                    trailingIcon = {
                        IconButton(onClick = { showToken = !showToken }) {
                            Icon(if (showToken) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                contentDescription = if (showToken) "Hide token" else "Show token")
                        }
                    },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp)
                )
                OutlinedTextField(
                    m.openSubtitlesKey, { m.openSubtitlesKey = it }, label = { Text("OpenSubtitles API key") }, singleLine = true,
                    supportingText = { Text("For searching subtitles online") },
                    visualTransformation = if (showSubtitlesKey) VisualTransformation.None else PasswordVisualTransformation(),
                    keyboardOptions = secretKeyboard(),
                    trailingIcon = {
                        IconButton(onClick = { showSubtitlesKey = !showSubtitlesKey }) {
                            Icon(if (showSubtitlesKey) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                contentDescription = if (showSubtitlesKey) "Hide key" else "Show key")
                        }
                    },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp)
                )
                OutlinedTextField(m.language, { m.language = it }, label = { Text("Language") }, placeholder = { Text("en-US") },
                    singleLine = true, modifier = Modifier.padding(top = 8.dp))
            }
            item {
                Message(m.message?.first, m.message?.second ?: false)
                // Same width and height, whether or not a label wraps.
                Row(
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    modifier = Modifier.padding(top = 8.dp).fillMaxWidth().height(IntrinsicSize.Min)
                ) {
                    Button(shape = AppButtonShape, colors = phoneButtonColors(), enabled = !m.saving, modifier = Modifier.weight(1f).fillMaxHeight(), onClick = { m.save() }) {
                        Icon(Icons.Default.Save, contentDescription = null)
                        Spacer(Modifier.width(6.dp)); ButtonLabel(if (m.saving) "Saving…" else "Save settings")
                    }
                    OutlinedButton(shape = AppButtonShape, modifier = Modifier.weight(1f).fillMaxHeight(), onClick = { m.scan() }) {
                        Icon(Icons.Default.Search, contentDescription = null)
                        Spacer(Modifier.width(6.dp)); ButtonLabel("Scan media")
                    }
                }
            }
        }
    }

    renaming?.let { old ->
        var name by remember(old) { mutableStateOf(old) }
        AlertDialog(
            onDismissRequest = { renaming = null },
            title = { Text("Rename $old") },
            text = { OutlinedTextField(name, { name = it }, label = { Text("New name") }, singleLine = true) },
            confirmButton = {
                PhoneButton(onClick = {
                    renaming = null
                    m.renameGroup(old, name)
                }) { Text("Rename") }
            },
            dismissButton = { PhoneSecondaryButton(onClick = { renaming = null }) { Text("Cancel") } }
        )
    }

    replacing?.let { v ->
        AlertDialog(
            onDismissRequest = { replacing = null },
            title = { Text("Replace ${v.name}?") },
            text = { Text("The fixed copy goes into its folder as ${replacedName(v.name)}, with its watch progress, and the original goes to the trash (deleted after ${damaged.trash.days} days).") },
            confirmButton = { PhoneButton(onClick = { damaged.replace(v); replacing = null }) { Text("Replace") } },
            dismissButton = { PhoneSecondaryButton(onClick = { replacing = null }) { Text("Cancel") } }
        )
    }
    restoring?.let { t ->
        AlertDialog(
            onDismissRequest = { restoring = null },
            title = { Text("Restore ${t.name}?") },
            text = { Text(restoreText(t)) },
            confirmButton = { PhoneButton(onClick = { damaged.restore(t); restoring = null }) { Text("Restore") } },
            dismissButton = { PhoneSecondaryButton(onClick = { restoring = null }) { Text("Cancel") } }
        )
    }
    if (emptying) {
        AlertDialog(
            onDismissRequest = { emptying = false },
            title = { Text("Empty the trash?") },
            text = { Text("Every replaced original in it is deleted now. They can't be brought back.") },
            confirmButton = { PhoneButton(destructive = true, onClick = { damaged.emptyTrash(); emptying = false }) { Text("Empty the trash") } },
            dismissButton = { PhoneSecondaryButton(onClick = { emptying = false }) { Text("Cancel") } }
        )
    }

    removing?.let { p ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text("Remove ${p.name.ifEmpty { "this path" }}?") },
            text = { Text("It's removed from the server's libraries when you save. The files aren't touched.") },
            confirmButton = { PhoneButton(destructive = true, onClick = { m.removePath(p); removing = null }) { Text("Remove") } },
            dismissButton = { PhoneSecondaryButton(onClick = { removing = null }) { Text("Cancel") } }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LibraryOptionMenu(label: String, options: List<Pair<String, String>>, value: String, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }) {
        OutlinedTextField(
            value = options.firstOrNull { it.first == value }?.second ?: options.first().second,
            onValueChange = {}, readOnly = true, label = { Text(label) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = open) },
            modifier = Modifier.menuAnchor(MenuAnchorType.PrimaryNotEditable).fillMaxWidth()
        )
        ExposedDropdownMenu(border = phoneMenuBorder(), expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (key, text) ->
                DropdownMenuItem(text = { Text(text) }, onClick = { onChange(key); open = false })
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Users tab: create users, change their group or admin status, delete them.

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsersTab(server: ServerRepository) {
    val m = rememberUsers(server)
    var deleting by remember { mutableStateOf<ServerUser?>(null) }
    var creatingGroup by remember { mutableStateOf(false) }

    LazyColumn(contentPadding = PaddingValues(16.dp)) {
        item { SectionTitle("Create a user") }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(m.newName, { m.newName = it }, label = { Text("Username") }, singleLine = true, keyboardOptions = usernameKeyboard(), modifier = Modifier.fillMaxWidth())
                OutlinedTextField(m.newPassword, { m.newPassword = it }, label = { Text("Password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(), modifier = Modifier.fillMaxWidth())
                GroupMenu(m.newGroup, m.groups, Modifier.fillMaxWidth(), onNew = { creatingGroup = true }) { m.newGroup = it }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = m.newAdmin, onCheckedChange = { m.newAdmin = it })
                    Text("Administrator")
                }
                Button(shape = AppButtonShape, colors = phoneButtonColors(), onClick = { m.create() }) {
                    Icon(Icons.Default.PersonAdd, contentDescription = null)
                    Spacer(Modifier.width(6.dp)); Text("Create user")
                }
            }
        }
        item { Message(m.message?.first, m.message?.second ?: false) }
        item { SectionTitle("Users") }
        val list = m.users
        if (list == null) {
            item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
        } else {
            itemsIndexed(list, key = { _, u -> u.username }) { _, user ->
                val isMe = user.username == m.me
                Card(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp)) {
                    Column(Modifier.padding(12.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(user.username, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                            if (user.isAdmin) AssistChip(shape = AppButtonShape, onClick = {}, label = { Text("Admin") }, modifier = Modifier.padding(start = 8.dp))
                            if (isMe) AssistChip(shape = AppButtonShape, onClick = {}, label = { Text("You") }, modifier = Modifier.padding(start = 6.dp))
                            Spacer(Modifier.weight(1f))
                            if (!isMe) IconButton(onClick = { deleting = user }) {
                                Icon(Icons.Default.Delete, contentDescription = "Delete ${user.username}", tint = MaterialTheme.colorScheme.error)
                            }
                        }
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            GroupMenu(user.group ?: "", m.groups, modifier = Modifier.weight(1f)) { g -> m.setGroup(user, g) }
                            Spacer(Modifier.width(8.dp))
                            Text("Admin")
                            Checkbox(checked = user.isAdmin, enabled = !isMe, onCheckedChange = { admin -> m.setAdmin(user, admin) })
                        }
                    }
                }
            }
        }
    }

    if (creatingGroup) {
        var name by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { creatingGroup = false },
            title = { Text("New group") },
            text = { OutlinedTextField(name, { name = it }, label = { Text("Name") }, singleLine = true) },
            confirmButton = {
                PhoneButton(onClick = { creatingGroup = false; m.createGroup(name) }) { Text("Add") }
            },
            dismissButton = { PhoneSecondaryButton(onClick = { creatingGroup = false }) { Text("Cancel") } }
        )
    }

    deleting?.let { user ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text("Delete ${user.username}?") },
            text = { Text("They won't be able to sign in any more.") },
            confirmButton = {
                PhoneButton(destructive = true, onClick = { deleting = null; m.delete(user) }) { Text("Delete") }
            },
            dismissButton = { PhoneSecondaryButton(onClick = { deleting = null }) { Text("Cancel") } }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GroupMenu(group: String, groups: List<String>, modifier: Modifier, onNew: (() -> Unit)? = null, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val shown = groupShown(group, groups)
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }, modifier = modifier) {
        OutlinedTextField(
            value = shown, onValueChange = {}, readOnly = true, label = { Text("Group") },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = open) },
            modifier = Modifier.menuAnchor(MenuAnchorType.PrimaryNotEditable).fillMaxWidth()
        )
        ExposedDropdownMenu(border = phoneMenuBorder(), expanded = open, onDismissRequest = { open = false }) {
            groups.forEach { g ->
                DropdownMenuItem(text = { Text(g) }, onClick = { open = false; if (g != group) onChange(g) })
            }
            onNew?.let { DropdownMenuItem(text = { Text("New group…") }, onClick = { open = false; it() }) }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Account tab: who is signed in, and changing their password.

@Composable
fun AccountTab(server: ServerRepository) {
    val (me, isAdmin) = remember(server.address) { server.signedInAs() }
    val pw = rememberPasswordChange(server)
    LazyColumn(contentPadding = PaddingValues(16.dp)) {
        item {
            SectionTitle("Signed in")
            Text("${me ?: "Unknown"}${if (isAdmin) " (administrator)" else ""} on ${server.address}",
                style = MaterialTheme.typography.bodyMedium)
        }
        item { SectionTitle("Sign in on a TV") }
        item { SignInOnTv(server) }
        item { SectionTitle("Languages") }
        item { AccountLanguages(server) }
        if (me != null) {
            item { SectionTitle("PIN on this phone") }
            item { PhonePinSettings(server.address, me) }
        }
        item { SectionTitle("Change password") }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(pw.current, { pw.current = it }, label = { Text("Current password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(), modifier = Modifier.fillMaxWidth())
                OutlinedTextField(pw.password, { pw.password = it }, label = { Text("New password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(), modifier = Modifier.fillMaxWidth())
                OutlinedTextField(pw.confirm, { pw.confirm = it }, label = { Text("Confirm password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(), modifier = Modifier.fillMaxWidth())
                Message(pw.message?.first, pw.message?.second ?: false)
                Button(shape = AppButtonShape, colors = phoneButtonColors(), onClick = { pw.change() }) { Text("Change password") }
            }
        }
    }
}

/**
 * The account's languages for audio and for subtitles, and whether subtitles start on, kept on the
 * server for every device: videos start with tracks in them when they have them (the server picks the
 * tracks). "File default" is as the file says.
 */
@Composable
private fun AccountLanguages(server: ServerRepository) {
    val scope = rememberCoroutineScope()
    var account by remember(server.address) { mutableStateOf<com.vargon.weirdstreamer.api.models.AccountSettings?>(null) }
    var message by remember { mutableStateOf<String?>(null) }
    var choosing by remember { mutableStateOf<Boolean?>(null) } // true: audio, false: subtitles
    LaunchedEffect(server.address) {
        val (got, err) = attempt { server.api.getAccount() }
        account = got
        message = err
    }
    fun nameOf(code: String) = if (code.isEmpty()) "File default" else languageName(code) ?: code
    Column {
        Text("Videos start with audio in this language when they have it, and with subtitles in this one if subtitles are shown by default. On every device.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        val a = account
        listOf(true to "Audio", false to "Subtitles").forEach { (audio, label) ->
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 8.dp)) {
                Text(label, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.width(96.dp))
                OutlinedButton(onClick = { choosing = audio }, enabled = a != null, shape = AppButtonShape) {
                    Text(a?.let { nameOf(if (audio) it.audioLanguage else it.subtitleLanguage) } ?: "Loading…")
                }
            }
        }
        fun save(next: com.vargon.weirdstreamer.api.models.AccountSettings) = scope.launch {
            val (_, err) = attempt { server.api.setAccount(next.copy(languages = null)) }
            if (err == null) account = next
            message = err
        }
        if (a != null) Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth().padding(top = 8.dp)
            .clickable { save(a.copy(subtitlesOn = !a.subtitlesOn)) }) {
            Text("Show subtitles by default", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            Switch(checked = a.subtitlesOn, onCheckedChange = { save(a.copy(subtitlesOn = it)) })
        }
        Message(message, true)
    }
    val a = account
    val audio = choosing
    if (audio != null && a != null) {
        val current = if (audio) a.audioLanguage else a.subtitleLanguage
        val options = listOf("") + a.languages.orEmpty().sortedBy { nameOf(it).lowercase() }
        AlertDialog(
            onDismissRequest = { choosing = null },
            title = { Text(if (audio) "Audio language" else "Subtitle language") },
            text = {
                LazyColumn {
                    items(options) { code ->
                        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()
                            .selectable(selected = code == current, role = Role.RadioButton) {
                                choosing = null
                                val next = if (audio) a.copy(audioLanguage = code) else a.copy(subtitleLanguage = code)
                                scope.launch {
                                    val (_, err) = attempt { server.api.setAccount(next.copy(languages = null)) }
                                    if (err == null) account = next
                                    message = err
                                }
                            }.padding(vertical = 4.dp)) {
                            RadioButton(selected = code == current, onClick = null)
                            Spacer(Modifier.width(8.dp))
                            Text(nameOf(code))
                        }
                    }
                }
            },
            confirmButton = {},
            dismissButton = { PhoneSecondaryButton(onClick = { choosing = null }) { Text("Cancel") } }
        )
    }
}

/** The code a TV shows on its sign-in screen, entered here, signs that TV in as this account. */
@Composable
private fun SignInOnTv(server: ServerRepository) {
    val scope = rememberCoroutineScope()
    var code by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    var busy by remember { mutableStateOf(false) }
    fun claim() {
        val clean = normalizePairingCode(code)
        if (clean.length != 5) { message = "Enter the 5-character code the TV shows." to true; return }
        if (busy) return
        busy = true
        scope.launch {
            val result = server.claimPairing(clean)
            busy = false
            message = if (result.isSuccess) { code = ""; "The TV is signing in." to false }
                else (result.exceptionOrNull()?.message ?: "Couldn't sign the TV in") to true
        }
    }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Enter the code the TV shows on its sign-in screen.", style = MaterialTheme.typography.bodyMedium)
        OutlinedTextField(code, { code = it.uppercase().take(8) }, label = { Text("Code") }, singleLine = true,
            keyboardOptions = pairingCodeKeyboard(),
            keyboardActions = androidx.compose.foundation.text.KeyboardActions(onDone = { claim() }),
            modifier = Modifier.fillMaxWidth())
        Message(message?.first, message?.second ?: false)
        Button(shape = AppButtonShape, colors = phoneButtonColors(), enabled = !busy, onClick = { claim() }) {
            Text("Sign in on the TV")
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Streams tab (admins): what's playing on the server, refreshed every few seconds.

@Composable
fun StreamsTab(server: ServerRepository) {
    val m = rememberStreams(server)
    val list = m.streams
    when {
        list == null && m.error != null -> AdminOnly(m.error!!)
        list == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        list.isEmpty() -> AdminOnly("Nothing is playing.")
        else -> LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            itemsIndexed(list, key = { _, s -> s.id }) { _, s -> StreamCard(s) }
        }
    }
}

@Composable
private fun StreamCard(s: AdminStream) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(s.title ?: s.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            Text(streamState(s), color = streamStateColor(s.state), style = MaterialTheme.typography.labelLarge)
            Text("${s.user} · ${s.client} · ${s.clientIp}", style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (s.duration > 0) {
                LinearProgressIndicator(progress = { ((s.position ?: s.servedTo) / s.duration).toFloat().coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
            }
            Text(streamPosition(s), style = MaterialTheme.typography.bodySmall)
            HorizontalDivider(Modifier.padding(vertical = 4.dp))
            streamDetails(s).forEach { Text(it, style = MaterialTheme.typography.bodySmall) }
        }
    }
}

/** A stream's state in color: playing green, paused amber. */
@Composable
internal fun streamStateColor(state: String) = when (state) {
    "playing" -> Color(0xFF2E7D32); "paused" -> Color(0xFFB7791F); else -> MaterialTheme.colorScheme.onSurfaceVariant
}

/** A job's state in color: running green, waiting amber, paused red. */
@Composable
internal fun jobStateColor(state: String) = when (state) {
    "running" -> Color(0xFF2E7D32); "waiting" -> Color(0xFFB7791F); "paused" -> Color(0xFFC62828)
    else -> MaterialTheme.colorScheme.onSurfaceVariant
}

// ---------------------------------------------------------------------------------------------
// Jobs tab (admins): the server's background jobs (JobsModel), with Run now / Pause.

@Composable
fun JobsTab(server: ServerRepository) {
    val m = rememberJobs(server)
    val d = m.data
    when {
        d == null && m.error != null -> AdminOnly(m.error!!)
        d == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            item {
                Text(JOBS_ABOUT, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                d.load?.let { Text(loadText(it), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 6.dp)) }
                m.error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            }
            d.trickplay?.let { tp -> item { TrickplaySettingsBox(tp, m::saveTrickplay) } }
            itemsIndexed(d.jobs, key = { _, j -> j.id }) { _, j -> JobCard(j, m.now, m::control) }
            item { StorageCard(d.storage.orEmpty()) }
        }
    }
}

// Trickplay's runners and frames per runner, saved to the server's config.yml (an empty field is
// automatic, sent as 0). The tab refreshes every few seconds, so the fields only follow the server while
// they aren't being edited. save returns an error to show, or null.
@Composable
private fun TrickplaySettingsBox(tp: TrickplaySettings, save: suspend (Int, Int) -> String?) {
    val scope = rememberCoroutineScope()
    var runners by remember { mutableStateOf(trickplayField(tp.autoRunners, tp.runners)) }
    var frames by remember { mutableStateOf(trickplayField(tp.autoFrames, tp.framesPerRunner)) }
    var edited by remember { mutableStateOf(false) }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    LaunchedEffect(tp.runners, tp.framesPerRunner, tp.autoRunners, tp.autoFrames) {
        if (!edited) {
            runners = trickplayField(tp.autoRunners, tp.runners)
            frames = trickplayField(tp.autoFrames, tp.framesPerRunner)
        }
    }
    val r = runners.toIntOrNull() ?: 0 // 0: automatic
    val f = frames.toIntOrNull() ?: 0
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Trickplay", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(runners, { runners = it.filter(Char::isDigit); edited = true }, label = { Text("Runners") },
                    placeholder = { Text(trickplayAutoRunners(tp)) }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.weight(1f))
                OutlinedTextField(frames, { frames = it.filter(Char::isDigit); edited = true }, label = { Text("Frames per runner") },
                    placeholder = { Text(trickplayAutoFrames(runners, tp)) }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.weight(1f))
            }
            Text(trickplayNote(runners, frames, tp), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Button(shape = AppButtonShape, colors = phoneButtonColors(), onClick = {
                    scope.launch {
                        val err = save(r, f)
                        if (err == null) {
                            edited = false
                            message = "Saved. Applies from the next batch." to false
                        } else message = err to true
                    }
                }) { Text("Save") }
                message?.let { (text, isError) ->
                    Text(text, style = MaterialTheme.typography.bodySmall,
                        color = if (isError) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

// What the server's data folders hold (kept current as things are added and removed), each readable
// and in bytes, with their total.
@Composable
private fun StorageCard(storage: List<StorageUse>) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text("Storage", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            if (storage.isEmpty()) {
                Text("Counting…", style = MaterialTheme.typography.bodySmall)
                return@Column
            }
            @Composable
            fun line(name: String, bytes: Long, bold: Boolean) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(name, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f),
                        fontWeight = if (bold) FontWeight.Bold else null)
                    Column(horizontalAlignment = Alignment.End) {
                        Text(formatBytes(bytes).ifEmpty { "0 B" }, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold)
                        Text(String.format(Locale.US, "%,d bytes", bytes), style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
            storage.forEach { line(it.name, it.bytes, false) }
            HorizontalDivider()
            line("Total", storage.sumOf { it.bytes }, true)
        }
    }
}

@Composable
private fun JobCard(j: BackgroundJob, now: Long, control: (String, String) -> Unit) {
    val started = epochMillis(j.started)
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(j.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                Text(j.state.replaceFirstChar { it.uppercase() }, color = jobStateColor(j.state), style = MaterialTheme.typography.labelLarge)
            }
            Text(j.about, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (started != null) {
                if (j.total > 0) {
                    LinearProgressIndicator(progress = { (j.done.toFloat() / j.total).coerceIn(0f, 1f) },
                        modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
                }
                Text(jobProgress(j, started, now), style = MaterialTheme.typography.bodySmall)
                j.detail?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                // What each runner is on, for a job that runs several at once.
                j.runners?.forEachIndexed { i, file ->
                    if (file.isNotEmpty()) Text("Runner ${i + 1}: $file", style = MaterialTheme.typography.bodySmall)
                }
            }
            if (j.state == "waiting" && !j.note.isNullOrEmpty()) {
                Text("Waiting: ${j.note}", style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold)
            }
            Text(jobLastRun(j, now), style = MaterialTheme.typography.bodySmall)
            Text(jobNextRun(j, now), style = MaterialTheme.typography.bodySmall)
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
                Button(shape = AppButtonShape, colors = phoneButtonColors(), enabled = j.canRun && started == null,
                    onClick = { control(j.id, "run") }) { Text("Run now") }
                Button(shape = AppButtonShape, colors = phoneButtonColors(),
                    onClick = { control(j.id, if (j.paused) "resume" else "pause") }) { Text(if (j.paused) "Resume" else "Pause") }
            }
        }
    }
}
