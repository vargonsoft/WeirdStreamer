package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.PersonAdd
import androidx.compose.material.icons.filled.Save
import androidx.compose.material.icons.filled.Search
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
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import com.vargon.weirdstreamer.api.models.CreateUserRequest
import com.vargon.weirdstreamer.api.models.ServerUser
import com.vargon.weirdstreamer.repository.ServerRepository
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import retrofit2.Response
import java.util.Locale

// Server administration for the phone app: the same settings as the web UI's Settings page.

/** A request's error message from the server, or a generic one. */
private fun <T> Response<T>.problem(): String? {
    if (isSuccessful) return null
    val body = runCatching { errorBody()?.string() }.getOrNull()
    val message = runCatching { JsonParser.parseString(body).asJsonObject.get("message")?.asString }.getOrNull()
    return message ?: "Request failed (${code()})"
}

private suspend fun <T> attempt(block: suspend () -> Response<T>): Pair<T?, String?> = try {
    val r = block()
    r.body() to r.problem()
} catch (e: Exception) {
    null to (e.localizedMessage ?: "Couldn't reach the server")
}

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

/** One media path being edited; [source] keeps the fields this screen doesn't show. */
private class PathEdit(val source: JsonObject, name: String, path: String, groups: List<String>, type: String) {
    var name by mutableStateOf(name)
    var path by mutableStateOf(path)
    val groups = mutableStateListOf<String>().apply { addAll(groups) } // the groups that can see it
    var type by mutableStateOf(type)
}

private fun jsonNames(names: List<String>) = JsonArray().apply { names.forEach { add(it) } }

/** The groups that can see a library: chips for each defined group. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun LibraryGroups(all: List<String>, selected: MutableList<String>) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(0.dp)) {
        Text("Groups:", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.align(Alignment.CenterVertically))
        if (all.isEmpty()) Text("none defined (add one under Groups)", style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.align(Alignment.CenterVertically))
        all.forEach { g ->
            val on = g in selected
            FilterChip(selected = on, label = { Text(g) }, onClick = {
                if (on) selected.remove(g) else selected.add(g)
                val order = all.filter { it in selected } // keep the groups' order
                selected.clear(); selected.addAll(order)
            })
        }
    }
}

private val libraryTypes = listOf("" to "No type", "movie" to "Movies", "tv" to "TV Shows")

// ---------------------------------------------------------------------------------------------
// Server tab: media paths, server, streaming and metadata settings, and a library scan.

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ServerSettingsTab(server: ServerRepository) {
    val scope = rememberCoroutineScope()
    var config by remember { mutableStateOf<JsonObject?>(null) }
    var loadError by remember { mutableStateOf<String?>(null) }
    val paths = remember { mutableStateListOf<PathEdit>() }
    var port by remember { mutableStateOf("") }
    var host by remember { mutableStateOf("") }
    var http2 by remember { mutableStateOf(false) }
    var chunkSize by remember { mutableStateOf("") }
    var tmdbToken by remember { mutableStateOf("") }
    var openSubtitlesKey by remember { mutableStateOf("") }
    var showSubtitlesKey by remember { mutableStateOf(false) }
    var showToken by remember { mutableStateOf(false) }
    var language by remember { mutableStateOf("") }
    var newPath by remember { mutableStateOf("") }
    var newType by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    var saving by remember { mutableStateOf(false) }
    var removing by remember { mutableStateOf<PathEdit?>(null) }
    // The defined groups. Changes go to the server right away, and into this screen's settings
    // (which a Save sends back whole, so they must match).
    val groupNames = remember { mutableStateListOf<String>() }
    var newGroupName by remember { mutableStateOf("") }
    var renaming by remember { mutableStateOf<String?>(null) }
    var groupMessage by remember { mutableStateOf<Pair<String, Boolean>?>(null) }

    fun applyGroups(list: List<String>) {
        groupNames.clear(); groupNames.addAll(list)
        config?.add("groups", jsonNames(list))
    }

    fun adopt(c: JsonObject) {
        config = c
        val media = c.getAsJsonObject("media")
        paths.clear()
        media?.getAsJsonArray("paths")?.forEach { e ->
            val o = e.asJsonObject
            paths.add(PathEdit(o, o.get("name")?.asString ?: "", o.get("path")?.asString ?: "",
                o.getAsJsonArray("groups")?.map { it.asString } ?: emptyList(),
                o.get("type")?.asString ?: ""))
        }
        groupNames.clear()
        c.getAsJsonArray("groups")?.forEach { groupNames.add(it.asString) }
        c.getAsJsonObject("server")?.let { port = it.get("port")?.asString ?: ""; host = it.get("host")?.asString ?: "" }
        c.getAsJsonObject("streaming")?.let {
            http2 = it.get("enableHTTP2")?.asBoolean ?: false
            chunkSize = it.get("chunkSize")?.asString ?: ""
        }
        c.getAsJsonObject("metadata")?.let {
            tmdbToken = it.get("tmdbAccessToken")?.asString ?: ""
            openSubtitlesKey = it.get("openSubtitlesApiKey")?.asString ?: ""
            language = it.get("language")?.asString ?: ""
        }
    }

    LaunchedEffect(server.address) {
        val (c, err) = attempt { server.api.getConfig() }
        if (c != null && err == null) adopt(c) else loadError = err
    }

    fun buildConfig(): JsonObject? {
        val c = config?.deepCopy() ?: return null
        val array = JsonArray()
        paths.forEach { p ->
            val o = p.source.deepCopy()
            o.addProperty("name", p.name.trim())
            o.addProperty("path", p.path.trim())
            o.add("groups", jsonNames(p.groups))
            if (p.type.isEmpty()) o.remove("type") else o.addProperty("type", p.type)
            array.add(o)
        }
        c.getAsJsonObject("media").add("paths", array)
        c.add("groups", jsonNames(groupNames))
        c.getAsJsonObject("server").apply {
            port.trim().toIntOrNull()?.let { addProperty("port", it) }
            addProperty("host", host.trim())
        }
        c.getAsJsonObject("streaming").apply {
            addProperty("enableHTTP2", http2)
            chunkSize.trim().toIntOrNull()?.let { addProperty("chunkSize", it) }
        }
        val metadata = c.getAsJsonObject("metadata") ?: JsonObject().also { c.add("metadata", it) }
        metadata.addProperty("tmdbAccessToken", tmdbToken.trim())
        metadata.addProperty("openSubtitlesApiKey", openSubtitlesKey.trim())
        metadata.addProperty("language", language.trim())
        return c
    }

    val loaded = config
    when {
        loaded == null && loadError != null -> AdminOnly(loadError!!)
        loaded == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        else -> LazyColumn(contentPadding = PaddingValues(16.dp)) {
            item { SectionTitle("Groups") }
            item {
                Text("Each user is in one group, and each library lists the groups that can see it. " +
                    "Changes here apply right away.", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(groupNames.toList(), key = { "group:$it" }) { g ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(g, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
                    IconButton(onClick = { renaming = g }) { Icon(Icons.Default.Edit, contentDescription = "Rename $g") }
                    IconButton(onClick = {
                        scope.launch {
                            val (list, err) = attempt { server.api.deleteGroup(g) }
                            if (list != null && err == null) { applyGroups(list); groupMessage = "Removed $g." to false }
                            else groupMessage = (err ?: "Couldn't remove $g") to true // still in use: says by whom
                        }
                    }) { Icon(Icons.Default.Delete, contentDescription = "Remove $g", tint = MaterialTheme.colorScheme.error) }
                }
            }
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(newGroupName, { newGroupName = it }, label = { Text("New group") }, singleLine = true,
                        modifier = Modifier.weight(1f))
                    OutlinedButton(onClick = {
                        val name = newGroupName.trim()
                        if (name.isEmpty()) return@OutlinedButton
                        scope.launch {
                            val (list, err) = attempt { server.api.createGroup(JsonObject().apply { addProperty("name", name) }) }
                            if (list != null && err == null) { applyGroups(list); newGroupName = ""; groupMessage = "Added $name." to false }
                            else groupMessage = (err ?: "Couldn't add $name") to true
                        }
                    }) { Icon(Icons.Default.Add, contentDescription = null); Spacer(Modifier.width(6.dp)); Text("Add") }
                }
            }
            item { Message(groupMessage?.first, groupMessage?.second ?: false) }
            item { SectionTitle("Media paths") }
            itemsIndexed(paths, key = { i, p -> "$i:${p.source.get("id")?.asString}" }) { _, p ->
                Card(modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)) {
                    Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(p.name, { p.name = it }, label = { Text("Name") }, singleLine = true,
                            modifier = Modifier.fillMaxWidth())
                        LibraryGroups(groupNames, p.groups)
                        LibraryTypeMenu(p.type) { p.type = it }
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(p.path, style = MaterialTheme.typography.bodySmall,
                                color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                            IconButton(onClick = { removing = p }) {
                                Icon(Icons.Default.Delete, contentDescription = "Remove", tint = MaterialTheme.colorScheme.error)
                            }
                        }
                    }
                }
            }
            item {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(newPath, { newPath = it }, label = { Text("New path on the server") },
                        placeholder = { Text("/path/to/media") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                    LibraryTypeMenu(newType) { newType = it }
                    OutlinedButton(onClick = {
                        val path = newPath.trim()
                        when {
                            path.isEmpty() -> message = "Enter a path" to true
                            paths.any { it.path == path } -> message = "That path is already listed" to true
                            else -> {
                                val name = path.trimEnd('/').substringAfterLast('/').ifEmpty { path }
                                paths.add(PathEdit(JsonObject(), name, path, emptyList(), newType))
                                newPath = ""; newType = ""; message = "Added. Save to apply." to false
                            }
                        }
                    }) {
                        Icon(Icons.Default.Add, contentDescription = null)
                        Spacer(Modifier.width(6.dp)); Text("Add path")
                    }
                }
            }
            item { SectionTitle("Server") }
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    OutlinedTextField(port, { port = it.filter(Char::isDigit) }, label = { Text("Port") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number), modifier = Modifier.weight(1f))
                    OutlinedTextField(host, { host = it }, label = { Text("Host") }, singleLine = true, modifier = Modifier.weight(2f))
                }
            }
            item { SectionTitle("Streaming") }
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Enable HTTP/2", modifier = Modifier.weight(1f))
                    Switch(checked = http2, onCheckedChange = { http2 = it })
                }
                OutlinedTextField(chunkSize, { chunkSize = it.filter(Char::isDigit) }, label = { Text("Chunk size (bytes)") },
                    singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                    modifier = Modifier.fillMaxWidth())
            }
            item { SectionTitle("Metadata") }
            item {
                Text("TMDB is used for libraries with a Movies or TV Shows type.", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(
                    tmdbToken, { tmdbToken = it }, label = { Text("TMDB API read access token") }, singleLine = true,
                    visualTransformation = if (showToken) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = {
                        IconButton(onClick = { showToken = !showToken }) {
                            Icon(if (showToken) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                contentDescription = if (showToken) "Hide token" else "Show token")
                        }
                    },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp)
                )
                OutlinedTextField(
                    openSubtitlesKey, { openSubtitlesKey = it }, label = { Text("OpenSubtitles API key") }, singleLine = true,
                    supportingText = { Text("For searching subtitles online") },
                    visualTransformation = if (showSubtitlesKey) VisualTransformation.None else PasswordVisualTransformation(),
                    trailingIcon = {
                        IconButton(onClick = { showSubtitlesKey = !showSubtitlesKey }) {
                            Icon(if (showSubtitlesKey) Icons.Default.VisibilityOff else Icons.Default.Visibility,
                                contentDescription = if (showSubtitlesKey) "Hide key" else "Show key")
                        }
                    },
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp)
                )
                OutlinedTextField(language, { language = it }, label = { Text("Language") }, placeholder = { Text("en-US") },
                    singleLine = true, modifier = Modifier.padding(top = 8.dp))
            }
            item {
                Message(message?.first, message?.second ?: false)
                // Same width and height, whether or not a label wraps.
                Row(
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                    modifier = Modifier.padding(top = 8.dp).fillMaxWidth().height(IntrinsicSize.Min)
                ) {
                    Button(colors = phoneButtonColors(), enabled = !saving, modifier = Modifier.weight(1f).fillMaxHeight(), onClick = {
                        val body = buildConfig() ?: return@Button
                        saving = true
                        scope.launch {
                            val (result, err) = attempt { server.api.updateConfig(body) }
                            saving = false
                            if (err != null) {
                                message = err to true
                            } else {
                                result?.getAsJsonObject("config")?.let { adopt(it) }
                                message = (result?.get("message")?.asString ?: "Settings saved.") to false
                            }
                        }
                    }) {
                        Icon(Icons.Default.Save, contentDescription = null)
                        Spacer(Modifier.width(6.dp)); ButtonLabel(if (saving) "Saving…" else "Save settings")
                    }
                    OutlinedButton(modifier = Modifier.weight(1f).fillMaxHeight(), onClick = {
                        scope.launch {
                            val (_, err) = attempt { server.api.scanMedia() }
                            message = (err ?: "Scan started.") to (err != null)
                        }
                    }) {
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
                TextButton(onClick = {
                    val new = name.trim()
                    renaming = null
                    if (new.isEmpty() || new == old) return@TextButton
                    scope.launch {
                        val (list, err) = attempt { server.api.renameGroup(old, JsonObject().apply { addProperty("name", new) }) }
                        if (list != null && err == null) {
                            applyGroups(list)
                            // Its libraries here follow, as on the server
                            paths.forEach { p -> val i = p.groups.indexOf(old); if (i >= 0) p.groups[i] = new }
                            groupMessage = "Renamed $old to $new, for its users and libraries." to false
                        } else groupMessage = (err ?: "Couldn't rename $old") to true
                    }
                }) { Text("Rename") }
            },
            dismissButton = { TextButton(onClick = { renaming = null }) { Text("Cancel") } }
        )
    }

    removing?.let { p ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text("Remove ${p.name.ifEmpty { "this path" }}?") },
            text = { Text("It's removed from the server's libraries when you save. The files aren't touched.") },
            confirmButton = { TextButton(onClick = { paths.remove(p); removing = null; message = "Removed. Save to apply." to false }) { Text("Remove") } },
            dismissButton = { TextButton(onClick = { removing = null }) { Text("Cancel") } }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun LibraryTypeMenu(type: String, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }) {
        OutlinedTextField(
            value = libraryTypes.firstOrNull { it.first == type }?.second ?: "No type",
            onValueChange = {}, readOnly = true, label = { Text("Library type") },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = open) },
            modifier = Modifier.menuAnchor(MenuAnchorType.PrimaryNotEditable).fillMaxWidth()
        )
        ExposedDropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            libraryTypes.forEach { (value, label) ->
                DropdownMenuItem(text = { Text(label) }, onClick = { onChange(value); open = false })
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Users tab: create users, change their group or admin status, delete them.

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsersTab(server: ServerRepository) {
    val scope = rememberCoroutineScope()
    val me = remember(server.address) { server.signedInAs().first }
    var users by remember { mutableStateOf<List<ServerUser>?>(null) }
    var groups by remember { mutableStateOf<List<String>>(emptyList()) }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    var deleting by remember { mutableStateOf<ServerUser?>(null) }
    var newName by remember { mutableStateOf("") }
    var newPassword by remember { mutableStateOf("") }
    var newGroup by remember { mutableStateOf("") }
    var newAdmin by remember { mutableStateOf(false) }
    var creatingGroup by remember { mutableStateOf(false) }

    suspend fun reload() {
        val (list, err) = attempt { server.api.getUsers() }
        if (err != null) message = err to true else users = list
        attempt { server.api.getGroups() }.first?.let { groups = it }
        if (newGroup !in groups) newGroup = groups.firstOrNull() ?: ""
    }
    LaunchedEffect(server.address) { reload() }

    fun update(user: ServerUser, changes: JsonObject) {
        scope.launch {
            val (_, err) = attempt { server.api.updateUser(user.username, changes) }
            message = (err ?: "Updated ${user.username}.") to (err != null)
            reload()
        }
    }

    LazyColumn(contentPadding = PaddingValues(16.dp)) {
        item { SectionTitle("Create a user") }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(newName, { newName = it }, label = { Text("Username") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                OutlinedTextField(newPassword, { newPassword = it }, label = { Text("Password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                GroupMenu(newGroup, groups, Modifier.fillMaxWidth(), onNew = { creatingGroup = true }) { newGroup = it }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(checked = newAdmin, onCheckedChange = { newAdmin = it })
                    Text("Administrator")
                }
                Button(colors = phoneButtonColors(), onClick = {
                    if (newName.isBlank() || newPassword.isEmpty()) {
                        message = "Enter a username and password" to true; return@Button
                    }
                    if (newGroup !in groups) {
                        message = "Choose a group for the user" to true; return@Button
                    }
                    scope.launch {
                        val (_, err) = attempt {
                            server.api.createUser(CreateUserRequest(newName.trim(), newPassword, newAdmin, newGroup.trim()))
                        }
                        if (err == null) { message = "Created ${newName.trim()}." to false; newName = ""; newPassword = ""; newAdmin = false }
                        else message = err to true
                        reload()
                    }
                }) {
                    Icon(Icons.Default.PersonAdd, contentDescription = null)
                    Spacer(Modifier.width(6.dp)); Text("Create user")
                }
            }
        }
        item { Message(message?.first, message?.second ?: false) }
        item { SectionTitle("Users") }
        val list = users
        if (list == null) {
            item { LinearProgressIndicator(Modifier.fillMaxWidth()) }
        } else {
            itemsIndexed(list, key = { _, u -> u.username }) { _, user ->
                val isMe = user.username == me
                Card(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp)) {
                    Column(Modifier.padding(12.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(user.username, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
                            if (user.isAdmin) AssistChip(onClick = {}, label = { Text("Admin") }, modifier = Modifier.padding(start = 8.dp))
                            if (isMe) AssistChip(onClick = {}, label = { Text("You") }, modifier = Modifier.padding(start = 6.dp))
                            Spacer(Modifier.weight(1f))
                            if (!isMe) IconButton(onClick = { deleting = user }) {
                                Icon(Icons.Default.Delete, contentDescription = "Delete ${user.username}", tint = MaterialTheme.colorScheme.error)
                            }
                        }
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            GroupMenu(user.group ?: "", groups, modifier = Modifier.weight(1f)) { g ->
                                update(user, JsonObject().apply { addProperty("group", g) })
                            }
                            Spacer(Modifier.width(8.dp))
                            Text("Admin")
                            Checkbox(checked = user.isAdmin, enabled = !isMe, onCheckedChange = { admin ->
                                update(user, JsonObject().apply { addProperty("is_admin", admin) })
                            })
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
                TextButton(onClick = {
                    val new = name.trim()
                    creatingGroup = false
                    if (new.isEmpty()) return@TextButton
                    scope.launch {
                        val (list, err) = attempt { server.api.createGroup(JsonObject().apply { addProperty("name", new) }) }
                        if (list != null && err == null) { groups = list; newGroup = new } else message = (err ?: "Couldn't add $new") to true
                    }
                }) { Text("Add") }
            },
            dismissButton = { TextButton(onClick = { creatingGroup = false }) { Text("Cancel") } }
        )
    }

    deleting?.let { user ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text("Delete ${user.username}?") },
            text = { Text("They won't be able to sign in any more.") },
            confirmButton = {
                TextButton(onClick = {
                    deleting = null
                    scope.launch {
                        val (_, err) = attempt { server.api.deleteUser(user.username) }
                        message = (err ?: "Deleted ${user.username}.") to (err != null)
                        reload()
                    }
                }) { Text("Delete", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text("Cancel") } }
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GroupMenu(group: String, groups: List<String>, modifier: Modifier, onNew: (() -> Unit)? = null, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    // The defined groups only; a group that isn't one any more (or none) shows as such until changed
    val shown = when {
        group in groups -> group
        group.isEmpty() -> "Choose a group"
        else -> "$group (not a group)"
    }
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }, modifier = modifier) {
        OutlinedTextField(
            value = shown, onValueChange = {}, readOnly = true, label = { Text("Group") },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded = open) },
            modifier = Modifier.menuAnchor(MenuAnchorType.PrimaryNotEditable).fillMaxWidth()
        )
        ExposedDropdownMenu(expanded = open, onDismissRequest = { open = false }) {
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
    val scope = rememberCoroutineScope()
    val (me, isAdmin) = remember(server.address) { server.signedInAs() }
    var current by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var confirm by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    LazyColumn(contentPadding = PaddingValues(16.dp)) {
        item {
            SectionTitle("Signed in")
            Text("${me ?: "Unknown"}${if (isAdmin) " (administrator)" else ""} on ${server.address}",
                style = MaterialTheme.typography.bodyMedium)
        }
        item { SectionTitle("Change password") }
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(current, { current = it }, label = { Text("Current password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                OutlinedTextField(password, { password = it }, label = { Text("New password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                OutlinedTextField(confirm, { confirm = it }, label = { Text("Confirm password") }, singleLine = true,
                    visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                Message(message?.first, message?.second ?: false)
                Button(colors = phoneButtonColors(), onClick = {
                    when {
                        current.isEmpty() -> message = "Enter your current password" to true
                        password.isEmpty() -> message = "Enter a new password" to true
                        password != confirm -> message = "The passwords don't match" to true
                        else -> scope.launch {
                            val (_, err) = attempt { server.api.changePassword(ChangePasswordRequest(current, password)) }
                            message = (err ?: "Password changed.") to (err != null)
                            if (err == null) { current = ""; password = ""; confirm = "" }
                        }
                    }
                }) { Text("Change password") }
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Streams tab (admins): what's playing on the server, refreshed every few seconds.

@Composable
fun StreamsTab(server: ServerRepository) {
    var streams by remember { mutableStateOf<List<AdminStream>?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(server.address) {
        while (true) {
            val (list, err) = attempt { server.api.getStreams() }
            error = err
            if (list != null) streams = list
            delay(3000)
        }
    }
    val list = streams
    when {
        list == null && error != null -> AdminOnly(error!!)
        list == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        list.isEmpty() -> AdminOnly("Nothing is playing.")
        else -> LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            itemsIndexed(list, key = { _, s -> s.id }) { _, s -> StreamCard(s) }
        }
    }
}

@Composable
private fun StreamCard(s: AdminStream) {
    fun clock(seconds: Double): String {
        val t = seconds.toLong().coerceAtLeast(0)
        return if (t >= 3600) String.format(Locale.US, "%d:%02d:%02d", t / 3600, (t % 3600) / 60, t % 60)
        else String.format(Locale.US, "%d:%02d", t / 60, t % 60)
    }
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(s.title ?: s.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.SemiBold)
            val stateColor = when (s.state) {
                "playing" -> Color(0xFF2E7D32); "paused" -> Color(0xFFB7791F); else -> MaterialTheme.colorScheme.onSurfaceVariant
            }
            Text("${s.state.replaceFirstChar { it.uppercase() }} · ${if (s.mode == "transcode") "Transcoding" else "Direct play"}",
                color = stateColor, style = MaterialTheme.typography.labelLarge)
            Text("${s.user} · ${s.client} · ${s.clientIp}", style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            val position = s.position ?: s.servedTo
            if (s.duration > 0) {
                LinearProgressIndicator(progress = { (position / s.duration).toFloat().coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
            }
            Text("${clock(position)} / ${clock(s.duration)}" + if (s.position == null) " (estimated)" else "",
                style = MaterialTheme.typography.bodySmall)
            HorizontalDivider(Modifier.padding(vertical = 4.dp))
            Text(s.name, style = MaterialTheme.typography.bodySmall)
            if (s.width > 0) Text("Source ${s.width}×${s.height}", style = MaterialTheme.typography.bodySmall)
            s.transcode?.let { t ->
                Text(t.reason, style = MaterialTheme.typography.bodySmall)
                Text("Video: ${t.video}", style = MaterialTheme.typography.bodySmall)
                Text("Audio: ${t.audio}", style = MaterialTheme.typography.bodySmall)
                t.sourceVideo?.let { Text("Source: $it", style = MaterialTheme.typography.bodySmall) }
                Text(if (t.stalled) "Transcoder waiting for the player"
                    else String.format(Locale.US, "Transcoder %.2f× realtime · %.0f fps", t.speed, t.fps),
                    style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}
