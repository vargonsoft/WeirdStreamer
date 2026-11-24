package com.vargon.weirdstreamer.ui.components

import android.widget.Toast
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Backup
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Key
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Password
import androidx.compose.material.icons.filled.PersonAdd
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Switch
import androidx.compose.material3.TextField
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
import androidx.compose.ui.focus.focusProperties
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.tv.material3.Border
import androidx.tv.material3.Button
import androidx.tv.material3.ClickableSurfaceDefaults
import androidx.tv.material3.ExperimentalTvMaterial3Api
import androidx.tv.material3.Icon
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Surface
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.api.models.AdminStream
import com.vargon.weirdstreamer.api.models.BackgroundJob
import com.vargon.weirdstreamer.api.models.StorageUse
import com.vargon.weirdstreamer.api.models.TrickplaySettings
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.launch
import java.util.Locale

// The TV Settings' administration tabs (Server, Users, Streams, Jobs) and the Account tab's password
// change, drawn for the remote on the models the phone's use (AdminSettings.kt). Each change saves
// at once (the phone's Server tab has a Save button); the server's answer shows as a toast.

// ---------------------------------------------------------------------------------------------
// Building blocks

@Composable
internal fun TvSectionTitle(text: String) {
    Text(text, style = MaterialTheme.typography.titleLarge, color = WeirdStreamerTheme.spec.headingColor,
        modifier = Modifier.padding(top = 16.dp, bottom = 12.dp))
}

@Composable
internal fun TvNote(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier.padding(bottom = 12.dp))
}

/** Shows each new [notice] as a toast (longer for an error). */
@Composable
internal fun TvNoticeToast(notice: Notice?) {
    val context = LocalContext.current
    LaunchedEffect(notice) {
        notice?.let { (text, isError) ->
            Toast.makeText(context, text, if (isError) Toast.LENGTH_LONG else Toast.LENGTH_SHORT).show()
        }
    }
}

/** A TV dialog's box: its title and rows. */
@Composable
internal fun TvDialogFrame(title: String, onDismiss: () -> Unit, width: Int = 720, content: @Composable ColumnScope.() -> Unit) {
    val spec = WeirdStreamerTheme.spec
    val panel = spec.panelColor.takeOrElse { MaterialTheme.colorScheme.surface }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(
            Modifier.width(width.dp).background(panel, RoundedCornerShape(16.dp)).padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Text(title, style = MaterialTheme.typography.headlineSmall, color = spec.headingColor.takeOrElse { MaterialTheme.colorScheme.onSurface })
            content()
        }
    }
}

/**
 * A dialog's row: a label and its value, filled with the focus color when focused. OK runs
 * [onClick]; with [onStep], Left and Right step the value (shown between ‹ ›).
 */
@Composable
internal fun TvValueRow(label: String, value: String = "", focus: FocusRequester? = null, destructive: Boolean = false,
                        onStep: ((Int) -> Unit)? = null, onClick: () -> Unit) {
    val spec = WeirdStreamerTheme.spec
    var focused by remember { mutableStateOf(false) }
    val color = when {
        focused -> spec.focusLabel.takeOrElse { Color.Black }
        destructive -> MaterialTheme.colorScheme.error
        else -> MaterialTheme.colorScheme.onSurface
    }
    Row(
        Modifier.fillMaxWidth()
            .then(if (focus != null) Modifier.focusRequester(focus) else Modifier)
            .onFocusChanged { focused = it.isFocused }
            .onPreviewKeyEvent { e ->
                if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                when (e.key) {
                    Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> { onClick(); true }
                    Key.DirectionRight -> onStep?.let { it(1); true } ?: false
                    Key.DirectionLeft -> onStep?.let { it(-1); true } ?: false
                    else -> false
                }
            }
            .focusable()
            .background(if (focused) spec.focusColor.takeOrElse { Color.White } else Color.Transparent, RoundedCornerShape(8.dp))
            .padding(horizontal = 14.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        Text(label, color = color, modifier = Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
        if (value.isNotEmpty()) {
            Text(if (focused && onStep != null) "‹  $value  ›" else value, color = color, style = MaterialTheme.typography.titleMedium)
        }
    }
}

/** Steps [current] by [by] among [options]' keys, round. */
private fun stepOption(options: List<Pair<String, String>>, current: String, by: Int): String {
    val i = options.indexOfFirst { it.first == current }.coerceAtLeast(0)
    return options[((i + by) % options.size + options.size) % options.size].first
}

/** A list to pick one from, the current one focused and ticked. */
@Composable
internal fun TvChoiceDialog(title: String, options: List<Pair<String, String>>, selected: String,
                            onPick: (String) -> Unit, onDismiss: () -> Unit) {
    val start = options.indexOfFirst { it.first == selected }.coerceAtLeast(0)
    val chosen = remember { FocusRequester() }
    val list = rememberLazyListState(initialFirstVisibleItemIndex = (start - 3).coerceAtLeast(0))
    LaunchedEffect(Unit) { runCatching { chosen.requestFocus() } }
    TvDialogFrame(title, onDismiss, width = 520) {
        LazyColumn(state = list, modifier = Modifier.heightIn(max = 420.dp)) {
            itemsIndexed(options) { i, (key, label) ->
                TvValueRow(label, if (key == selected) "✓" else "", focus = if (i == start) chosen else null) {
                    onPick(key); onDismiss()
                }
            }
        }
    }
}

/** What a text entry asks for: the keyboard and whether it's hidden as typed. */
internal enum class TvEntryKind { Plain, Secret, Number, Address, Username }

/** One text value to type: OK on the keyboard or Save keeps it. */
@Composable
internal fun TvTextEntryDialog(title: String, initial: String, kind: TvEntryKind = TvEntryKind.Plain, note: String? = null,
                               onDismiss: () -> Unit, onDone: (String) -> Unit) {
    var value by remember { mutableStateOf(initial) }
    val field = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { field.requestFocus() } }
    val keyboard = when (kind) {
        TvEntryKind.Secret -> secretKeyboard(ImeAction.Done)
        TvEntryKind.Number -> KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Done)
        TvEntryKind.Address -> addressKeyboard(ImeAction.Done)
        TvEntryKind.Username -> usernameKeyboard(ImeAction.Done)
        TvEntryKind.Plain -> KeyboardOptions(imeAction = ImeAction.Done)
    }
    fun done() { onDismiss(); onDone(value) }
    TvDialogFrame(title, onDismiss) {
        note?.let { TvNote(it) }
        TextField(
            value = value,
            onValueChange = { value = if (kind == TvEntryKind.Number) it.filter(Char::isDigit) else it },
            colors = tvTextFieldColors(), singleLine = true,
            visualTransformation = if (kind == TvEntryKind.Secret) PasswordVisualTransformation() else VisualTransformation.None,
            keyboardOptions = keyboard,
            keyboardActions = androidx.compose.foundation.text.KeyboardActions(onDone = { done() }),
            modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown().focusRequester(field)
        )
        TvDialogButtons(confirm = "Save", onDismiss = onDismiss, onConfirm = ::done)
    }
}

/** A dialog's Cancel and main buttons, at its end. */
@Composable
internal fun TvDialogButtons(confirm: String, onDismiss: () -> Unit, enabled: Boolean = true, onConfirm: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(top = 8.dp), horizontalArrangement = Arrangement.End) {
        TvSecondaryButton(onClick = onDismiss) { Text("Cancel") }
        Spacer(Modifier.width(8.dp))
        Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { if (enabled) onConfirm() }) { Text(confirm) }
    }
}

/** Asks before something that can't be undone. */
@Composable
internal fun TvConfirmDialog(title: String, text: String, confirm: String, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    val cancel = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { cancel.requestFocus() } }
    TvDialogFrame(title, onDismiss, width = 560) {
        TvNote(text)
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
            TvSecondaryButton(onClick = onDismiss, modifier = Modifier.focusRequester(cancel)) { Text("Cancel") }
            Spacer(Modifier.width(8.dp))
            Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { onDismiss(); onConfirm() }) { Text(confirm) }
        }
    }
}

/** A box of text to read (a stream, the storage), focusable so the D-pad scrolls to it. */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
internal fun TvInfoCard(focus: FocusRequester? = null, up: FocusRequester? = null, content: @Composable ColumnScope.() -> Unit) {
    Surface(
        onClick = {},
        scale = ClickableSurfaceDefaults.scale(focusedScale = 1f),
        colors = ClickableSurfaceDefaults.colors(
            containerColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.06f),
            focusedContainerColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.06f),
            pressedContainerColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.06f),
        ),
        border = ClickableSurfaceDefaults.border(
            focusedBorder = Border(border = BorderStroke(2.dp, tvFocusBorderColor()), inset = 0.dp, shape = MaterialTheme.shapes.small)
        ),
        shape = ClickableSurfaceDefaults.shape(MaterialTheme.shapes.small),
        modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp)
            .then(if (focus != null) Modifier.focusRequester(focus) else Modifier)
            .focusProperties { up?.let { this.up = it } }
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp), content = content)
    }
}

/** The first row of a tab still loading, or with nothing to show: the tabs' Down lands on it. */
@Composable
private fun TvPlaceholderRow(text: String, first: FocusRequester, up: FocusRequester) {
    TvSettingsRow(text, focus = first, up = up, onClick = {}) {}
}

@Composable
private fun TvChevron() = Icon(Icons.Default.ChevronRight, contentDescription = null)

@Composable
private fun TvSwitch(on: Boolean) = Switch(checked = on, onCheckedChange = null, colors = tvSwitchColors())

// ---------------------------------------------------------------------------------------------
// Server tab

/** A text value of the Server tab being typed: what it is, and where it goes. */
private class TextEntry(val title: String, val initial: String, val kind: TvEntryKind, val note: String? = null, val onDone: (String) -> Unit)

/** A list of the Server tab being picked from. */
private class ChoiceEntry(val title: String, val options: List<Pair<String, String>>, val selected: String, val onPick: (String) -> Unit)

@Composable
fun TvServerTab(server: ServerRepository, first: FocusRequester, up: FocusRequester) {
    val m = rememberServerSettings(server)
    TvNoticeToast(m.message)
    TvNoticeToast(m.groupMessage)
    TvNoticeToast(m.backupMessage)
    var entry by remember { mutableStateOf<TextEntry?>(null) }
    var choice by remember { mutableStateOf<ChoiceEntry?>(null) }
    var library by remember { mutableStateOf<Int?>(null) } // its index among the paths
    var group by remember { mutableStateOf<String?>(null) }
    var showHardware by remember { mutableStateOf(false) }

    val config = m.config
    if (config == null) {
        TvPlaceholderRow(m.loadError ?: "Loading…", first, up)
        return
    }

    TvSectionTitle("Libraries")
    m.paths.forEachIndexed { i, p ->
        val groups = if (p.groups.isEmpty()) "No groups (no one sees it)" else "Groups: ${p.groups.joinToString(", ")}"
        TvSettingsRow(p.name.ifBlank { "(no name)" }, "${p.path} · ${optionLabel(libraryTypes, p.type)} · $groups",
            focus = if (i == 0) first else null, up = if (i == 0) up else null, onClick = { library = i }) { TvChevron() }
    }
    TvSettingsRow("Add a library", "A folder on the server", focus = if (m.paths.isEmpty()) first else null,
        up = if (m.paths.isEmpty()) up else null, onClick = {
            entry = TextEntry("Add a library", "", TvEntryKind.Address, "The folder's path on the server, such as /srv/media/Movies.") { path ->
                if (m.addPath(path, "")) { m.save(); library = m.paths.lastIndex }
            }
        }) { Icon(Icons.Default.Add, contentDescription = null) }

    TvSectionTitle("Groups")
    TvNote("Each user is in one group, and each library lists the groups that can see it. New users start in the default group.")
    m.groupNames.forEach { g ->
        TvSettingsRow(g, if (g == m.groupNames.firstOrNull()) "The default group" else null, onClick = { group = g }) { TvChevron() }
    }
    TvSettingsRow("Add a group", onClick = {
        entry = TextEntry("Add a group", "", TvEntryKind.Plain) { m.createGroup(it) }
    }) { Icon(Icons.Default.Add, contentDescription = null) }

    TvSectionTitle("Server")
    TvSettingsRow("Name", m.serverName.ifBlank { "Not set" } + " · shown in the apps' server lists", onClick = {
        entry = TextEntry("Server name", m.serverName, TvEntryKind.Plain) { m.serverName = it; m.save() }
    }) { TvChevron() }
    TvSettingsRow("Port", m.port, onClick = {
        entry = TextEntry("Port", m.port, TvEntryKind.Number, "Takes effect when the server restarts.") { m.port = it; m.save() }
    }) { TvChevron() }
    TvSettingsRow("Host", m.host.ifBlank { "Every address" }, onClick = {
        entry = TextEntry("Host", m.host, TvEntryKind.Address, "The address the server listens on; empty for every address.") { m.host = it; m.save() }
    }) { TvChevron() }

    TvSectionTitle("Streaming")
    TvSettingsRow("Enable HTTP/2", onClick = { m.http2 = !m.http2; m.saveSoon() }) { TvSwitch(m.http2) }
    TvSettingsRow("Pause background jobs while anyone is streaming", "Finding intros and credits, trickplay frames, cleanup",
        onClick = { m.pauseJobs = !m.pauseJobs; m.saveSoon() }) { TvSwitch(m.pauseJobs) }
    TvSettingsRow("Hardware encoding", hardwareSummary(m.hardware, config),
        onClick = { if (m.hardware != null) showHardware = true }) { Icon(Icons.Default.Memory, contentDescription = null) }

    TvSectionTitle("Backups")
    TvNote("Both databases (accounts, watch progress, matches, marked intros and credits) and config.yml. " +
        "Restore one with bin/cli backup restore, with the server stopped.")
    TvSettingsRow("Back up", optionLabel(BackupSettings.schedules, m.backupSchedule), onClick = {
        choice = ChoiceEntry("Back up", BackupSettings.schedules, m.backupSchedule) { m.backupSchedule = it; m.save() }
    }) { TvChevron() }
    if (m.backupSchedule == "weekly") {
        TvSettingsRow("On", optionLabel(BackupSettings.days, m.backupDay), onClick = {
            choice = ChoiceEntry("Back up on", BackupSettings.days, m.backupDay) { m.backupDay = it; m.save() }
        }) { TvChevron() }
    }
    if (m.backupSchedule != "off") {
        TvSettingsRow("At", "${m.backupTime} (24-hour, the server's time)", onClick = {
            entry = TextEntry("Back up at", m.backupTime, TvEntryKind.Plain, "24-hour, the server's time, such as 04:00.") {
                if (BackupSettings.validTime(it.trim())) { m.backupTime = it.trim(); m.save() }
                else m.backupMessage = "That isn't a time: use 24-hour, such as 04:00" to true
            }
        }) { TvChevron() }
    }
    TvSettingsRow("Backups kept", "${m.backupKeep}: the newest; the oldest go", onClick = {
        entry = TextEntry("Backups kept", m.backupKeep, TvEntryKind.Number, "1 to 365.") {
            if ((it.toIntOrNull() ?: 0) in 1..365) { m.backupKeep = it; m.save() }
            else m.backupMessage = "Keep 1 to 365 backups" to true
        }
    }) { TvChevron() }
    val list = m.backups
    TvSettingsRow(if (m.backingUp) "Backing up…" else "Back up now",
        if (list == null) "Couldn't read the backups." else BackupSettings.where(list),
        onClick = { if (!m.backingUp) m.backUpNow() }) { Icon(Icons.Default.Backup, contentDescription = null) }
    list?.backups.orEmpty().forEach { b -> TvNote("${BackupSettings.date(b.created)} · ${formatBytes(b.size)}", Modifier.padding(start = 12.dp)) }

    TvSectionTitle("Metadata")
    TvNote("TMDB is used for libraries with a Movies or TV Shows type.")
    TvSettingsRow("TMDB API read access token", if (m.tmdbToken.isBlank()) "Not set" else "Set", onClick = {
        entry = TextEntry("TMDB API read access token", m.tmdbToken, TvEntryKind.Secret) { m.tmdbToken = it; m.save() }
    }) { Icon(Icons.Default.Key, contentDescription = null) }
    TvSettingsRow("OpenSubtitles API key", (if (m.openSubtitlesKey.isBlank()) "Not set" else "Set") + " · for searching subtitles online", onClick = {
        entry = TextEntry("OpenSubtitles API key", m.openSubtitlesKey, TvEntryKind.Secret) { m.openSubtitlesKey = it; m.save() }
    }) { Icon(Icons.Default.Key, contentDescription = null) }
    TvSettingsRow("Language", m.language.ifBlank { "Not set (en-US)" }, onClick = {
        entry = TextEntry("Language", m.language, TvEntryKind.Plain, "For TMDB's titles and overviews, such as en-US.") { m.language = it; m.save() }
    }) { TvChevron() }

    TvSectionTitle("Library")
    TvSettingsRow("Scan media", "Looks for new and changed files in every library", onClick = { m.scan() }) {
        Icon(Icons.Default.Search, contentDescription = null)
    }

    entry?.let { e -> TvTextEntryDialog(e.title, e.initial, e.kind, e.note, onDismiss = { entry = null }, onDone = e.onDone) }
    choice?.let { c -> TvChoiceDialog(c.title, c.options, c.selected, onPick = c.onPick, onDismiss = { choice = null }) }
    library?.let { i ->
        val p = m.paths.getOrNull(i)
        if (p == null) library = null
        else TvLibraryDialog(m, p, onDismiss = { library = null }, onEdit = { entry = it }, onChoose = { choice = it })
    }
    group?.let { g ->
        TvGroupDialog(m, g, onDismiss = { group = null }, onRename = {
            entry = TextEntry("Rename $g", g, TvEntryKind.Plain) { m.renameGroup(g, it) }
        })
    }
    val hw = m.hardware
    if (showHardware && hw != null) {
        TvHardwareDialog(server, hw, config, onChanged = { m.adopt(it) }, onDismiss = { showHardware = false })
    }
}

/** A library's settings: each saved as it changes. */
@Composable
private fun TvLibraryDialog(m: ServerSettingsModel, p: PathEdit, onDismiss: () -> Unit,
                            onEdit: (TextEntry) -> Unit, onChoose: (ChoiceEntry) -> Unit) {
    var removing by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    TvDialogFrame(p.name.ifBlank { "Library" }, onDismiss) {
        TvNote(p.path)
        TvValueRow("Name", p.name, focus = first) {
            onEdit(TextEntry("Library name", p.name, TvEntryKind.Plain) { p.name = it; m.save() })
        }
        TvValueRow("Type", optionLabel(libraryTypes, p.type), onStep = { p.type = stepOption(libraryTypes, p.type, it); m.saveSoon() }) {
            onChoose(ChoiceEntry("Library type", libraryTypes, p.type) { p.type = it; m.save() })
        }
        TvValueRow("Continue Watching", optionLabel(libraryRowModes, p.continueWatching),
            onStep = { p.continueWatching = stepOption(libraryRowModes, p.continueWatching, it); m.saveSoon() }) {
            onChoose(ChoiceEntry("Continue Watching", libraryRowModes, p.continueWatching) { p.continueWatching = it; m.save() })
        }
        TvValueRow("Recently Added", optionLabel(libraryRowModes, p.newAdditions),
            onStep = { p.newAdditions = stepOption(libraryRowModes, p.newAdditions, it); m.saveSoon() }) {
            onChoose(ChoiceEntry("Recently Added", libraryRowModes, p.newAdditions) { p.newAdditions = it; m.save() })
        }
        Text("Groups that can see it", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 8.dp))
        if (m.groupNames.isEmpty()) TvNote("None defined: add one under Groups.")
        m.groupNames.forEach { g ->
            val on = g in p.groups
            TvValueRow(g, if (on) "On" else "Off", onStep = { p.toggleGroup(g, m.groupNames); m.saveSoon() }) {
                p.toggleGroup(g, m.groupNames); m.saveSoon()
            }
        }
        Spacer(Modifier.height(4.dp))
        TvValueRow("Remove this library", destructive = true) { removing = true }
    }
    if (removing) {
        TvConfirmDialog("Remove ${p.name.ifEmpty { "this library" }}?",
            "It's removed from the server's libraries. The files aren't touched.", "Remove",
            onDismiss = { removing = false }) { m.removePath(p); m.save(); onDismiss() }
    }
}

/** A group's actions: make it the default, rename it, remove it (applied on the server at once). */
@Composable
private fun TvGroupDialog(m: ServerSettingsModel, g: String, onDismiss: () -> Unit, onRename: () -> Unit) {
    var removing by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    val isDefault = g == m.groupNames.firstOrNull()
    TvDialogFrame(g, onDismiss, width = 560) {
        if (isDefault) TvNote("The default group: new users start in it.")
        if (!isDefault) TvValueRow("Make it the default group", focus = first) { m.makeDefaultGroup(g); onDismiss() }
        TvValueRow("Rename", focus = if (isDefault) first else null) { onDismiss(); onRename() }
        TvValueRow("Remove", destructive = true) { removing = true }
    }
    if (removing) {
        TvConfirmDialog("Remove $g?", "Only a group no user is in can be removed.", "Remove",
            onDismiss = { removing = false }) { m.deleteGroup(g); onDismiss() }
    }
}

// ---------------------------------------------------------------------------------------------
// Users tab

@Composable
fun TvUsersTab(server: ServerRepository, first: FocusRequester, up: FocusRequester) {
    val m = rememberUsers(server)
    TvNoticeToast(m.message)
    var creating by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<String?>(null) } // a username

    TvSettingsRow("Create a user", focus = first, up = up, onClick = { creating = true }) {
        Icon(Icons.Default.PersonAdd, contentDescription = null)
    }
    TvSectionTitle("Users")
    val list = m.users
    if (list == null) TvNote("Loading…")
    list?.forEach { u ->
        val tags = listOfNotNull("Administrator".takeIf { u.isAdmin }, "You".takeIf { u.username == m.me })
        TvSettingsRow(u.username + if (tags.isEmpty()) "" else " · ${tags.joinToString(" · ")}",
            "Group: ${groupShown(u.group ?: "", m.groups)}", onClick = { editing = u.username }) { TvChevron() }
    }

    if (creating) TvCreateUserDialog(m, onDismiss = { creating = false })
    editing?.let { name ->
        val user = m.users?.firstOrNull { it.username == name }
        if (user == null) editing = null
        else TvUserDialog(m, user, onDismiss = { editing = null })
    }
}

@Composable
private fun TvCreateUserDialog(m: UsersModel, onDismiss: () -> Unit) {
    val nameField = remember { FocusRequester() }
    var choosingGroup by remember { mutableStateOf(false) }
    var newGroup by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) { runCatching { nameField.requestFocus() } }
    TvDialogFrame("Create a user", onDismiss) {
        TextField(m.newName, { m.newName = it }, label = { FieldText("Username") }, singleLine = true, colors = tvTextFieldColors(),
            keyboardOptions = usernameKeyboard(ImeAction.Next), modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown().focusRequester(nameField))
        TextField(m.newPassword, { m.newPassword = it }, label = { FieldText("Password") }, singleLine = true, colors = tvTextFieldColors(),
            visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(ImeAction.Done),
            modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown())
        val groupOptions = m.groups.map { it to it }
        TvValueRow("Group", groupShown(m.newGroup, m.groups),
            onStep = { if (groupOptions.isNotEmpty()) m.newGroup = stepOption(groupOptions, m.newGroup, it) }) { choosingGroup = true }
        TvValueRow("Administrator", if (m.newAdmin) "Yes" else "No", onStep = { m.newAdmin = !m.newAdmin }) { m.newAdmin = !m.newAdmin }
        TvDialogButtons("Create user", onDismiss = onDismiss) { m.create(onCreated = onDismiss) }
    }
    if (choosingGroup) {
        TvChoiceDialog("Group", m.groups.map { it to it } + ("\u0000new" to "New group…"), m.newGroup,
            onPick = { if (it == "\u0000new") newGroup = true else m.newGroup = it }, onDismiss = { choosingGroup = false })
    }
    if (newGroup) {
        TvTextEntryDialog("New group", "", onDismiss = { newGroup = false }) { m.createGroup(it) }
    }
}

@Composable
private fun TvUserDialog(m: UsersModel, user: com.vargon.weirdstreamer.api.models.ServerUser, onDismiss: () -> Unit) {
    val isMe = user.username == m.me
    var choosingGroup by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    val groupOptions = m.groups.map { it to it }
    TvDialogFrame(user.username, onDismiss, width = 600) {
        TvValueRow("Group", groupShown(user.group ?: "", m.groups), focus = first,
            onStep = { if (groupOptions.isNotEmpty()) m.setGroup(user, stepOption(groupOptions, user.group ?: "", it)) }) { choosingGroup = true }
        if (isMe) {
            TvNote("You're an administrator: another administrator can change that, or delete your account.")
        } else {
            TvValueRow("Administrator", if (user.isAdmin) "Yes" else "No", onStep = { m.setAdmin(user, !user.isAdmin) }) {
                m.setAdmin(user, !user.isAdmin)
            }
            TvValueRow("Delete ${user.username}", destructive = true) { deleting = true }
        }
    }
    if (choosingGroup) {
        TvChoiceDialog("Group", groupOptions, user.group ?: "", onPick = { if (it != user.group) m.setGroup(user, it) },
            onDismiss = { choosingGroup = false })
    }
    if (deleting) {
        TvConfirmDialog("Delete ${user.username}?", "They won't be able to sign in any more.", "Delete",
            onDismiss = { deleting = false }) { m.delete(user); onDismiss() }
    }
}

// ---------------------------------------------------------------------------------------------
// Streams tab

@Composable
fun TvStreamsTab(server: ServerRepository, first: FocusRequester, up: FocusRequester) {
    val m = rememberStreams(server)
    val list = m.streams
    when {
        list == null -> TvPlaceholderRow(m.error ?: "Loading…", first, up)
        list.isEmpty() -> TvPlaceholderRow("Nothing is playing.", first, up)
        else -> list.forEachIndexed { i, s ->
            TvInfoCard(focus = if (i == 0) first else null, up = if (i == 0) up else null) { TvStreamText(s) }
        }
    }
}

@Composable
private fun TvStreamText(s: AdminStream) {
    Text(s.title ?: s.name, style = MaterialTheme.typography.titleMedium)
    Text(streamState(s), color = streamStateColor(s.state), style = MaterialTheme.typography.labelLarge)
    Text("${s.user} · ${s.client} · ${s.clientIp}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    if (s.duration > 0) {
        androidx.compose.material3.LinearProgressIndicator(
            progress = { ((s.position ?: s.servedTo) / s.duration).toFloat().coerceIn(0f, 1f) },
            color = WeirdStreamerTheme.spec.progressColor.takeOrElse { MaterialTheme.colorScheme.primary },
            modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp))
    }
    Text(streamPosition(s), style = MaterialTheme.typography.bodyMedium)
    streamDetails(s).forEach { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
}

// ---------------------------------------------------------------------------------------------
// Jobs tab

@Composable
fun TvJobsTab(server: ServerRepository, first: FocusRequester, up: FocusRequester) {
    val m = rememberJobs(server)
    var trickplay by remember { mutableStateOf(false) }
    val d = m.data
    if (d == null) {
        TvPlaceholderRow(m.error ?: "Loading…", first, up)
        return
    }
    TvNote(JOBS_ABOUT)
    d.load?.let { TvNote(loadText(it)) }
    m.error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
    val tp = d.trickplay
    var firstUsed = false
    if (tp != null) {
        firstUsed = true
        TvSettingsRow("Trickplay", "${tp.runners} runners × ${tp.framesPerRunner} frames per runner", focus = first, up = up,
            onClick = { trickplay = true }) { TvChevron() }
    }
    d.jobs.forEachIndexed { i, j ->
        val isFirst = !firstUsed && i == 0
        TvJobCard(j, m.now, m::control, focus = if (isFirst) first else null, up = if (isFirst) up else null)
    }
    TvInfoCard(focus = if (!firstUsed && d.jobs.isEmpty()) first else null) { TvStorageText(d.storage.orEmpty()) }
    if (trickplay && tp != null) TvTrickplayDialog(tp, m::saveTrickplay, onDismiss = { trickplay = false })
}

@Composable
private fun TvJobCard(j: BackgroundJob, now: Long, control: (String, String) -> Unit, focus: FocusRequester?, up: FocusRequester?) {
    val started = epochMillis(j.started)
    Column(Modifier.fillMaxWidth().padding(bottom = 20.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(j.name, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            Text(j.state.replaceFirstChar { it.uppercase() }, color = jobStateColor(j.state), style = MaterialTheme.typography.labelLarge)
        }
        Text(j.about, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (started != null) {
            if (j.total > 0) {
                androidx.compose.material3.LinearProgressIndicator(
                    progress = { (j.done.toFloat() / j.total).coerceIn(0f, 1f) },
                    color = WeirdStreamerTheme.spec.progressColor.takeOrElse { MaterialTheme.colorScheme.primary },
                    modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp))
            }
            Text(jobProgress(j, started, now), style = MaterialTheme.typography.bodySmall)
            j.detail?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
            j.runners?.forEachIndexed { i, file ->
                if (file.isNotEmpty()) Text("Runner ${i + 1}: $file", style = MaterialTheme.typography.bodySmall)
            }
        }
        if (j.state == "waiting" && !j.note.isNullOrEmpty()) Text("Waiting: ${j.note}", style = MaterialTheme.typography.bodySmall)
        Text(jobLastRun(j, now), style = MaterialTheme.typography.bodySmall)
        Text(jobNextRun(j, now), style = MaterialTheme.typography.bodySmall)
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.padding(top = 6.dp)) {
            val canRun = j.canRun && started == null
            // A job that can't run now keeps its button, so the row's focus doesn't jump: it says why.
            Button(shape = tvButtonShape(), colors = tvButtonColors(),
                onClick = { if (canRun) control(j.id, "run") },
                modifier = Modifier.then(if (focus != null) Modifier.focusRequester(focus) else Modifier)
                    .focusProperties { up?.let { this.up = it } }) {
                Text(if (started != null) "Running" else "Run now")
            }
            TvSecondaryButton(onClick = { control(j.id, if (j.paused) "resume" else "pause") }) { Text(if (j.paused) "Resume" else "Pause") }
        }
    }
}

@Composable
private fun TvStorageText(storage: List<StorageUse>) {
    Text("Storage", style = MaterialTheme.typography.titleMedium)
    if (storage.isEmpty()) { Text("Counting…", style = MaterialTheme.typography.bodySmall); return }
    @Composable
    fun line(name: String, bytes: Long, total: Boolean) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(name, style = if (total) MaterialTheme.typography.titleSmall else MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            Text(formatBytes(bytes).ifEmpty { "0 B" }, style = MaterialTheme.typography.bodyMedium)
            Text(String.format(Locale.US, "%,d bytes", bytes), style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(200.dp).padding(start = 16.dp))
        }
    }
    storage.forEach { line(it.name, it.bytes, false) }
    line("Total", storage.sumOf { it.bytes }, true)
}

@Composable
private fun TvTrickplayDialog(tp: TrickplaySettings, save: suspend (Int, Int) -> String?, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var runners by remember { mutableStateOf(tp.runners.toString()) }
    var frames by remember { mutableStateOf(tp.framesPerRunner.toString()) }
    var error by remember { mutableStateOf<String?>(null) }
    val field = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { field.requestFocus() } }
    val r = runners.toIntOrNull() ?: 0
    val f = frames.toIntOrNull() ?: 0
    val context = LocalContext.current
    TvDialogFrame("Trickplay", onDismiss) {
        val number = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Next)
        TextField(runners, { runners = it.filter(Char::isDigit) }, label = { FieldText("Runners") }, singleLine = true,
            colors = tvTextFieldColors(), keyboardOptions = number,
            modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown().focusRequester(field))
        TextField(frames, { frames = it.filter(Char::isDigit) }, label = { FieldText("Frames per runner") }, singleLine = true,
            colors = tvTextFieldColors(), keyboardOptions = number, modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown())
        TvNote(trickplayNote(r, f, tp))
        error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
        TvDialogButtons("Save", onDismiss = onDismiss, enabled = r > 0 && f > 0) {
            scope.launch {
                val err = save(r, f)
                if (err == null) {
                    Toast.makeText(context, "Saved. Applies from the next batch.", Toast.LENGTH_SHORT).show()
                    onDismiss()
                } else error = err
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Account tab: changing the password

@Composable
fun TvChangePasswordRow(server: ServerRepository) {
    var open by remember { mutableStateOf(false) }
    TvSettingsRow("Change password", onClick = { open = true }) { Icon(Icons.Default.Password, contentDescription = null) }
    if (open) TvChangePasswordDialog(server, onDismiss = { open = false })
}

@Composable
private fun TvChangePasswordDialog(server: ServerRepository, onDismiss: () -> Unit) {
    val pw = rememberPasswordChange(server)
    val context = LocalContext.current
    val field = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { field.requestFocus() } }
    TvDialogFrame("Change password", onDismiss) {
        @Composable
        fun secret(value: String, label: String, set: (String) -> Unit, modifier: Modifier = Modifier) =
            TextField(value, set, label = { FieldText(label) }, singleLine = true, colors = tvTextFieldColors(),
                visualTransformation = PasswordVisualTransformation(), keyboardOptions = secretKeyboard(ImeAction.Next),
                modifier = Modifier.fillMaxWidth().tvFieldLeavesOnUpDown().then(modifier))
        secret(pw.current, "Current password", { pw.current = it }, Modifier.focusRequester(field))
        secret(pw.password, "New password", { pw.password = it })
        secret(pw.confirm, "Confirm password", { pw.confirm = it })
        pw.message?.takeIf { it.second }?.let { Text(it.first, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
        TvDialogButtons("Change password", onDismiss = onDismiss) { pw.change(onChanged = {
            Toast.makeText(context, "Password changed.", Toast.LENGTH_SHORT).show()
            onDismiss()
        }) }
    }
}
