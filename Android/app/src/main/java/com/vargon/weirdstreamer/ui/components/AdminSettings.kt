package com.vargon.weirdstreamer.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import com.google.gson.JsonArray
import com.google.gson.JsonObject
import com.google.gson.JsonParser
import com.vargon.weirdstreamer.api.models.AdminStream
import com.vargon.weirdstreamer.api.models.BackupList
import com.vargon.weirdstreamer.api.models.ChangePasswordRequest
import com.vargon.weirdstreamer.api.models.CreateUserRequest
import com.vargon.weirdstreamer.api.models.GuideAdmin
import com.vargon.weirdstreamer.api.models.GuideAdminChannel
import com.vargon.weirdstreamer.api.models.GuideAdminLibrary
import com.vargon.weirdstreamer.api.models.GuideAdminSave
import com.vargon.weirdstreamer.api.models.GuideLibrarySave
import com.vargon.weirdstreamer.api.models.GuideTypedSave
import com.vargon.weirdstreamer.api.models.HardwareReport
import com.vargon.weirdstreamer.api.models.JobLoad
import com.vargon.weirdstreamer.api.models.JobsResponse
import com.vargon.weirdstreamer.api.models.ServerUser
import com.vargon.weirdstreamer.api.models.TrickplaySettingsRequest
import com.vargon.weirdstreamer.repository.ServerRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import retrofit2.Response
import java.util.Locale
import kotlin.math.roundToInt

// Server administration's state and requests, shared by the phone's Settings tabs
// (PhoneServerSettings.kt) and the TV's (TvAdminSettings.kt): each draws its own screens on these.

/** A request's error message from the server, or a generic one. */
internal fun <T> Response<T>.problem(): String? {
    if (isSuccessful) return null
    val body = runCatching { errorBody()?.string() }.getOrNull()
    val message = runCatching { JsonParser.parseString(body).asJsonObject.get("message")?.asString }.getOrNull()
    return message ?: "Request failed (${code()})"
}

/** A request's body and its error (null when it worked). */
internal suspend fun <T> attempt(block: suspend () -> Response<T>): Pair<T?, String?> = try {
    val r = block()
    r.body() to r.problem()
} catch (e: Exception) {
    null to (e.localizedMessage ?: "Couldn't reach the server")
}

/** A message to show under a section: its text, and whether it's an error. */
typealias Notice = Pair<String, Boolean>

/** One media path being edited; [source] keeps the fields Settings doesn't show. */
class PathEdit(val source: JsonObject, name: String, path: String, groups: List<String>, type: String) {
    var name by mutableStateOf(name)
    var path by mutableStateOf(path)
    val groups = mutableStateListOf<String>().apply { addAll(groups) } // the groups that can see it
    var type by mutableStateOf(type)
    // Where its videos in progress and its newest videos show (the web's defaults when unset)
    var continueWatching by mutableStateOf(source.get("continueWatchingMode")?.asString ?: "on")
    var newAdditions by mutableStateOf(source.get("showNewAdditions")?.asString ?: "off")
    // The phone shows it as it is, with Edit, so a stray tap can't change it; one just added opens for editing.
    var editing by mutableStateOf(source.size() == 0)

    /** Turns [group] on or off for this library, keeping the groups' order ([all]). */
    fun toggleGroup(group: String, all: List<String>) {
        if (group in groups) groups.remove(group) else groups.add(group)
        val order = all.filter { it in groups }
        groups.clear(); groups.addAll(order)
    }
}

internal fun jsonNames(names: List<String>) = JsonArray().apply { names.forEach { add(it) } }

internal val libraryTypes = listOf("" to "No type", "movie" to "Movies", "tv" to "TV Shows")

// Where a library's Continue Watching and Recently Added rows show (continueWatchingMode, showNewAdditions),
// and its titles on the Guide's channels (the Guide tab's Show in Guide)
internal val libraryRowModes = listOf("on" to "Home screen and library", "libraryOnly" to "Library only", "off" to "Off")

// The Guide's prime time (guide.primeTime), the news hour after it
internal val primeTimes = listOf("20" to "8 to 11 pm", "19" to "7 to 10 pm")

/** An option's label among [options], or [key] itself. */
internal fun optionLabel(options: List<Pair<String, String>>, key: String) = options.firstOrNull { it.first == key }?.second ?: key

// ---------------------------------------------------------------------------------------------
// The Server tab: media paths, groups, server, streaming, backups and metadata settings, and a scan.

/**
 * The server's settings as Settings edits them (`GET/PUT /api/config`), sent back whole by [save].
 * Groups change on the server at once (and here, so a save matches), as do Back up now and Scan media.
 */
class ServerSettingsModel(val server: ServerRepository, private val scope: CoroutineScope) {
    var config by mutableStateOf<JsonObject?>(null); private set
    var loadError by mutableStateOf<String?>(null); private set
    val paths = mutableStateListOf<PathEdit>()
    var serverName by mutableStateOf("")
    var port by mutableStateOf("")
    var host by mutableStateOf("")
    var http2 by mutableStateOf(false)
    var pauseJobs by mutableStateOf(false)
    // Hardware encoding: what the server found, and the config's choice ("" the CPU, else "id|device")
    var hardware by mutableStateOf<HardwareReport?>(HardwareReport(checking = true)); private set
    var hwEncoder by mutableStateOf("")
    var hwDevice by mutableStateOf("")
    var hwMax by mutableStateOf("0")
    // Backups: the schedule (saved with the rest), and the backups in the folder config.yml names
    var backupSchedule by mutableStateOf("daily")
    var backupDay by mutableStateOf("sunday")
    var backupTime by mutableStateOf("04:00")
    var backupKeep by mutableStateOf("7")
    var backups by mutableStateOf<BackupList?>(null); private set
    var backingUp by mutableStateOf(false); private set
    var backupMessage by mutableStateOf<Notice?>(null)
    var tmdbToken by mutableStateOf("")
    var openSubtitlesKey by mutableStateOf("")
    var language by mutableStateOf("")
    var message by mutableStateOf<Notice?>(null)
    var saving by mutableStateOf(false); private set
    // The defined groups; the first is the default, which new users start in.
    val groupNames = mutableStateListOf<String>()
    var groupMessage by mutableStateOf<Notice?>(null)

    private fun applyGroups(list: List<String>) {
        groupNames.clear(); groupNames.addAll(list)
        config?.add("groups", jsonNames(list))
    }

    /** Takes [c] as the settings shown (loaded, saved, or changed by the TV's Hardware encoding). */
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
        c.getAsJsonObject("server")?.let {
            serverName = it.get("name")?.asString ?: ""
            port = it.get("port")?.asString ?: ""; host = it.get("host")?.asString ?: ""
        }
        c.getAsJsonObject("streaming")?.let {
            http2 = it.get("enableHTTP2")?.asBoolean ?: false
            pauseJobs = it.get("pauseJobsWhileStreaming")?.asBoolean ?: false
        }
        hwEncoder = HardwareSettings.encoderOf(c); hwDevice = HardwareSettings.deviceOf(c)
        hwMax = HardwareSettings.maxOf(c).toString()
        c.getAsJsonObject("database")?.let {
            backupSchedule = it.get("backupSchedule")?.asString?.ifEmpty { null } ?: "daily"
            backupDay = it.get("backupDay")?.asString?.ifEmpty { null } ?: "sunday"
            backupTime = it.get("backupTime")?.asString?.ifEmpty { null } ?: "04:00"
            backupKeep = (it.get("backupKeep")?.asInt?.takeIf { k -> k > 0 } ?: 7).toString()
        }
        c.getAsJsonObject("metadata")?.let {
            tmdbToken = it.get("tmdbAccessToken")?.asString ?: ""
            openSubtitlesKey = it.get("openSubtitlesApiKey")?.asString ?: ""
            language = it.get("language")?.asString ?: ""
        }
    }

    suspend fun load() {
        val (c, err) = attempt { server.api.getConfig() }
        if (c != null && err == null) adopt(c) else loadError = err
    }

    suspend fun loadBackups() {
        backups = runCatching { server.api.getBackups().body() }.getOrNull()
    }

    /** The server tries its GPUs' encoders at startup: asked again until that's done. */
    suspend fun watchHardware() {
        while (true) {
            val report = runCatching { server.api.getHardware().body() }.getOrNull()
            hardware = report
            if (report == null || !report.checking) break
            delay(2000)
        }
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
            // Only where set before or changed from the default, so config.yml changes only where it was changed.
            if (o.has("continueWatchingMode") || p.continueWatching != "on") o.addProperty("continueWatchingMode", p.continueWatching)
            if (o.has("showNewAdditions") || p.newAdditions != "off") o.addProperty("showNewAdditions", p.newAdditions)
            array.add(o)
        }
        c.getAsJsonObject("media").add("paths", array)
        c.add("groups", jsonNames(groupNames))
        c.getAsJsonObject("server").apply {
            addProperty("name", serverName.trim())
            port.trim().toIntOrNull()?.let { addProperty("port", it) }
            addProperty("host", host.trim())
        }
        c.getAsJsonObject("streaming").apply {
            addProperty("enableHTTP2", http2)
            addProperty("pauseJobsWhileStreaming", pauseJobs)
        }
        if (hardware?.encoders?.isNotEmpty() == true) {
            HardwareSettings.apply(c, HardwareSettings.key(hwEncoder, hwDevice), hwMax.toIntOrNull() ?: 0)
        }
        val database = c.getAsJsonObject("database") ?: JsonObject().also { c.add("database", it) }
        database.addProperty("backupSchedule", backupSchedule)
        database.addProperty("backupDay", backupDay)
        database.addProperty("backupTime", BackupSettings.time(backupTime))
        database.addProperty("backupKeep", (backupKeep.toIntOrNull() ?: 7).coerceIn(1, 365))
        val metadata = c.getAsJsonObject("metadata") ?: JsonObject().also { c.add("metadata", it) }
        metadata.addProperty("tmdbAccessToken", tmdbToken.trim())
        metadata.addProperty("openSubtitlesApiKey", openSubtitlesKey.trim())
        metadata.addProperty("language", language.trim())
        return c
    }

    private var saveCount = 0
    private var pendingSave: kotlinx.coroutines.Job? = null

    /**
     * Sends the settings; the server's answer (its message, or why not) goes in [message]. Only the
     * latest save's answer is taken, so one sent before it can't bring back what it changed.
     */
    fun save() {
        pendingSave?.cancel()
        val body = buildConfig() ?: return
        val count = ++saveCount
        saving = true
        scope.launch {
            val (result, err) = attempt { server.api.updateConfig(body) }
            if (count != saveCount) return@launch
            saving = false
            if (err != null) {
                message = err to true
            } else {
                result?.getAsJsonObject("config")?.let { adopt(it) }
                message = (result?.get("message")?.asString ?: "Settings saved.") to false
                loadBackups() // the next backup, by the schedule saved
            }
        }
    }

    /** Saves after a moment without another change (the TV saves each change; Left and Right step through values). */
    fun saveSoon() {
        pendingSave?.cancel()
        pendingSave = scope.launch { delay(600); pendingSave = null; save() }
    }

    fun scan() {
        scope.launch {
            val (_, err) = attempt { server.api.scanMedia() }
            message = (err ?: "Scan started.") to (err != null)
        }
    }

    /** Adds a library for [path] (not saved yet); false, with [message] saying why, when it can't. */
    fun addPath(path: String, type: String): Boolean {
        val p = path.trim()
        return when {
            p.isEmpty() -> { message = "Enter a path" to true; false }
            paths.any { it.path == p } -> { message = "That path is already listed" to true; false }
            else -> {
                val name = p.trimEnd('/').substringAfterLast('/').ifEmpty { p }
                paths.add(PathEdit(JsonObject(), name, p, emptyList(), type))
                message = "Added. Save to apply." to false
                true
            }
        }
    }

    fun removePath(p: PathEdit) {
        paths.remove(p)
        message = "Removed. Save to apply." to false
    }

    fun makeDefaultGroup(g: String) {
        scope.launch {
            val (list, err) = attempt { server.api.makeDefaultGroup(g) }
            if (list != null && err == null) { applyGroups(list); groupMessage = "$g is the default group: new users start in it." to false }
            else groupMessage = (err ?: "Couldn't make $g the default") to true
        }
    }

    fun deleteGroup(g: String) {
        scope.launch {
            val (list, err) = attempt { server.api.deleteGroup(g) }
            if (list != null && err == null) { applyGroups(list); groupMessage = "Removed $g." to false }
            else groupMessage = (err ?: "Couldn't remove $g") to true // still in use: says by whom
        }
    }

    /** Adds group [name]; [onAdded] runs once the server has it. */
    fun createGroup(name: String, onAdded: () -> Unit = {}) {
        val n = name.trim()
        if (n.isEmpty()) return
        scope.launch {
            val (list, err) = attempt { server.api.createGroup(JsonObject().apply { addProperty("name", n) }) }
            if (list != null && err == null) { applyGroups(list); groupMessage = "Added $n." to false; onAdded() }
            else groupMessage = (err ?: "Couldn't add $n") to true
        }
    }

    fun renameGroup(old: String, name: String) {
        val new = name.trim()
        if (new.isEmpty() || new == old) return
        scope.launch {
            val (list, err) = attempt { server.api.renameGroup(old, JsonObject().apply { addProperty("name", new) }) }
            if (list != null && err == null) {
                applyGroups(list)
                // Its libraries here follow, as on the server
                paths.forEach { p -> val i = p.groups.indexOf(old); if (i >= 0) p.groups[i] = new }
                groupMessage = "Renamed $old to $new, for its users and libraries." to false
            } else groupMessage = (err ?: "Couldn't rename $old") to true
        }
    }

    fun backUpNow() {
        backingUp = true
        backupMessage = null
        scope.launch {
            val (made, err) = attempt { server.api.backUpNow() }
            backingUp = false
            backupMessage = if (err != null) err to true else "Made ${made?.name} (${formatBytes(made?.size ?: 0)})." to false
            loadBackups()
        }
    }
}

// ---------------------------------------------------------------------------------------------
// The Guide tab: typed channels, each library's Show in Guide and Use for Guide Filler, the schedule
// (GET/PUT /api/admin/guide), saved apart from the Server tab.

/** A typed channel definition being edited: its type, its libraries and up to how many channels. */
class TypedEdit(val id: String, type: String, libraries: List<String>, max: Int, val number: Int,
                val channels: List<GuideAdminChannel>) {
    var type by mutableStateOf(type)
    val libraries = mutableStateListOf<String>().apply { addAll(libraries) }
    var max by mutableStateOf(max)
}

/** A library's Guide switches being edited. */
class GuideLibraryEdit(val library: GuideAdminLibrary) {
    var showInGuide by mutableStateOf(library.showInGuide.ifEmpty { "off" })
    var guideFiller by mutableStateOf(library.guideFiller)
    val airs get() = library.type == "movie" || library.type == "tv" // only those take Show in Guide
}

class GuideSettingsModel(val server: ServerRepository, private val scope: CoroutineScope) {
    var data by mutableStateOf<GuideAdmin?>(null); private set
    var loadError by mutableStateOf<String?>(null); private set
    val typed = mutableStateListOf<TypedEdit>()
    val libraries = mutableStateListOf<GuideLibraryEdit>()
    var primeTime by mutableStateOf("20")
    var weatherZip by mutableStateOf("")
    var message by mutableStateOf<Notice?>(null)
    var saving by mutableStateOf(false); private set

    fun adopt(g: GuideAdmin) {
        data = g
        primeTime = g.primeTime.takeIf { it > 0 }?.toString() ?: "20"
        weatherZip = g.weatherZip
        typed.clear()
        g.typedChannels.forEach { typed.add(TypedEdit(it.id, it.type, it.libraries, it.max, it.number, it.channels)) }
        libraries.clear()
        g.libraries.forEach { libraries.add(GuideLibraryEdit(it)) }
    }

    suspend fun load() {
        val (g, err) = attempt { server.api.getGuideSettings() }
        if (g != null && err == null) adopt(g) else loadError = err
    }

    val types get() = data?.types?.map { it.id to it.name } ?: emptyList()
    fun typeName(id: String) = optionLabel(types, id)
    fun library(id: String) = data?.libraries?.firstOrNull { it.id == id }
    val canAdd get() = data != null && typed.size < (data?.maxTyped ?: 0)

    /** Whether a typed channel other than [t] has the library (a library is one's at most). */
    fun usedElsewhere(t: TypedEdit, id: String) = typed.any { it !== t && id in it.libraries }
    /** Whether any typed channel has the library: then it's in no regular channel. */
    fun inTyped(id: String) = typed.any { id in it.libraries }

    /** The groups that see [t]'s channels: those that can open every one of its libraries (null: none chosen). */
    fun groupsFor(t: TypedEdit): List<String>? {
        var out: List<String>? = null
        t.libraries.forEach { id ->
            val g = library(id)?.groups ?: emptyList()
            out = out?.filter { it in g } ?: g
        }
        return out
    }

    fun toggleLibrary(t: TypedEdit, id: String) {
        if (id in t.libraries) t.libraries.remove(id) else if (!usedElsewhere(t, id)) t.libraries.add(id)
    }

    fun addTyped(): TypedEdit? {
        val d = data ?: return null
        if (!canAdd) return null
        return TypedEdit("", d.types.firstOrNull()?.id ?: "latenight", emptyList(), d.maxChannels, 0, emptyList()).also { typed.add(it) }
    }

    fun removeTyped(t: TypedEdit) { typed.remove(t) }

    private var saveCount = 0
    private var pendingSave: kotlinx.coroutines.Job? = null

    /** Sends the tab; the server answers with it as saved (new typed channels numbered), or why not. */
    fun save() {
        pendingSave?.cancel()
        val zip = weatherZip.trim()
        if (zip.isNotEmpty() && !Regex("\\d{5}").matches(zip)) { message = "The weather ZIP code is five digits." to true; return }
        val body = GuideAdminSave(primeTime.toIntOrNull() ?: 20, zip,
            libraries.map { GuideLibrarySave(it.library.id, if (it.showInGuide == "off") "" else it.showInGuide, it.guideFiller) },
            typed.map { GuideTypedSave(it.id, it.type, it.libraries.toList(), it.max) })
        val count = ++saveCount
        saving = true
        scope.launch {
            val (g, err) = attempt { server.api.saveGuideSettings(body) }
            if (count != saveCount) return@launch
            saving = false
            if (g != null && err == null) { adopt(g); message = "Saved. The Guide plans again now." to false }
            else message = (err ?: "Couldn't save") to true
        }
    }

    /** Saves after a moment without another change (the TV saves each change). */
    fun saveSoon() {
        pendingSave?.cancel()
        pendingSave = scope.launch { delay(600); pendingSave = null; save() }
    }

    var replanning by mutableStateOf(false); private set

    /** Plans every channel again from its next program, the next 6 hours too (asked first). */
    fun replan() {
        replanning = true
        scope.launch {
            val (r, err) = attempt { server.api.replanGuide() }
            replanning = false
            message = if (err != null) err to true else (r?.get("message")?.asString ?: "Planned again.") to false
            if (err == null) load() // the channels' names and numbers now
        }
    }
}

/** What Plan again from now asks before it runs. */
internal const val REPLAN_QUESTION = "What each channel airs now stays. Everything after it, the next 6 hours too, is planned " +
    "anew, so the Guide everyone sees changes."

@Composable
fun rememberGuideSettings(server: ServerRepository): GuideSettingsModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address) { GuideSettingsModel(server, scope) }
    LaunchedEffect(model) { model.load() }
    return model
}

@Composable
fun rememberServerSettings(server: ServerRepository): ServerSettingsModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address) { ServerSettingsModel(server, scope) }
    LaunchedEffect(model) { model.load() }
    LaunchedEffect(model) { model.loadBackups() }
    LaunchedEffect(model) { model.watchHardware() }
    return model
}

// ---------------------------------------------------------------------------------------------
// The Users tab: create users, change their group or admin status, delete them.

class UsersModel(val server: ServerRepository, private val scope: CoroutineScope) {
    val me: String? = server.signedInAs().first
    var users by mutableStateOf<List<ServerUser>?>(null); private set
    var groups by mutableStateOf<List<String>>(emptyList()); private set
    var message by mutableStateOf<Notice?>(null)
    // The user being made
    var newName by mutableStateOf("")
    var newPassword by mutableStateOf("")
    var newGroup by mutableStateOf("")
    var newAdmin by mutableStateOf(false)

    suspend fun reload() {
        val (list, err) = attempt { server.api.getUsers() }
        if (err != null) message = err to true else users = list
        attempt { server.api.getGroups() }.first?.let { groups = it }
        if (newGroup !in groups) newGroup = groups.firstOrNull() ?: ""
    }

    fun update(user: ServerUser, changes: JsonObject) {
        scope.launch {
            val (_, err) = attempt { server.api.updateUser(user.username, changes) }
            message = (err ?: "Updated ${user.username}.") to (err != null)
            reload()
        }
    }

    fun setGroup(user: ServerUser, group: String) = update(user, JsonObject().apply { addProperty("group", group) })
    fun setAdmin(user: ServerUser, admin: Boolean) = update(user, JsonObject().apply { addProperty("is_admin", admin) })

    /** Creates the user in the fields; [onCreated] runs once it's made (the fields are cleared). */
    fun create(onCreated: () -> Unit = {}) {
        if (newName.isBlank() || newPassword.isEmpty()) { message = "Enter a username and password" to true; return }
        if (newGroup !in groups) { message = "Choose a group for the user" to true; return }
        scope.launch {
            val name = newName.trim()
            val (_, err) = attempt { server.api.createUser(CreateUserRequest(name, newPassword, newAdmin, newGroup.trim())) }
            if (err == null) {
                message = "Created $name." to false; newName = ""; newPassword = ""; newAdmin = false
                newGroup = groups.firstOrNull() ?: "" // back to the default group
                onCreated()
            } else message = err to true
            reload()
        }
    }

    fun delete(user: ServerUser) {
        scope.launch {
            val (_, err) = attempt { server.api.deleteUser(user.username) }
            message = (err ?: "Deleted ${user.username}.") to (err != null)
            reload()
        }
    }

    /** Adds group [name] and picks it for the user being made. */
    fun createGroup(name: String) {
        val new = name.trim()
        if (new.isEmpty()) return
        scope.launch {
            val (list, err) = attempt { server.api.createGroup(JsonObject().apply { addProperty("name", new) }) }
            if (list != null && err == null) { groups = list; newGroup = new } else message = (err ?: "Couldn't add $new") to true
        }
    }
}

@Composable
fun rememberUsers(server: ServerRepository): UsersModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address) { UsersModel(server, scope) }
    LaunchedEffect(model) { model.reload() }
    return model
}

/** A group's name as a menu shows it: a group that isn't one any more (or none) says so until changed. */
internal fun groupShown(group: String, groups: List<String>) = when {
    group in groups -> group
    group.isEmpty() -> "Choose a group"
    else -> "$group (not a group)"
}

// ---------------------------------------------------------------------------------------------
// The Account tab's password change.

class PasswordChangeModel(val server: ServerRepository, private val scope: CoroutineScope) {
    var current by mutableStateOf("")
    var password by mutableStateOf("")
    var confirm by mutableStateOf("")
    var message by mutableStateOf<Notice?>(null)

    /** Changes the password in the fields; [onChanged] runs once it's done (the fields are cleared). */
    fun change(onChanged: () -> Unit = {}) {
        when {
            current.isEmpty() -> message = "Enter your current password" to true
            password.isEmpty() -> message = "Enter a new password" to true
            password != confirm -> message = "The passwords don't match" to true
            else -> scope.launch {
                val (_, err) = attempt { server.api.changePassword(ChangePasswordRequest(current, password)) }
                message = (err ?: "Password changed.") to (err != null)
                if (err == null) { current = ""; password = ""; confirm = ""; onChanged() }
            }
        }
    }
}

@Composable
fun rememberPasswordChange(server: ServerRepository): PasswordChangeModel {
    val scope = rememberCoroutineScope()
    return remember(server.address) { PasswordChangeModel(server, scope) }
}

// ---------------------------------------------------------------------------------------------
// The Streams tab: what's playing on the server, asked every few seconds.

class StreamsModel {
    var streams by mutableStateOf<List<AdminStream>?>(null); internal set
    var error by mutableStateOf<String?>(null); internal set
}

@Composable
fun rememberStreams(server: ServerRepository): StreamsModel {
    val model = remember(server.address) { StreamsModel() }
    LaunchedEffect(model) {
        while (true) {
            val (list, err) = attempt { server.api.getStreams() }
            model.error = err
            if (list != null) model.streams = list
            delay(3000)
        }
    }
    return model
}

/** A position as a clock: "1:02:03", "4:05". */
internal fun streamClock(seconds: Double): String {
    val t = seconds.toLong().coerceAtLeast(0)
    return if (t >= 3600) String.format(Locale.US, "%d:%02d:%02d", t / 3600, (t % 3600) / 60, t % 60)
    else String.format(Locale.US, "%d:%02d", t / 60, t % 60)
}

/** "Playing · Transcoding". */
internal fun streamState(s: AdminStream) =
    "${s.state.replaceFirstChar { it.uppercase() }} · ${if (s.mode == "transcode") "Transcoding" else "Direct play"}"

/** "12:34 / 45:00", "(estimated)" when the player hasn't said. */
internal fun streamPosition(s: AdminStream): String {
    val position = s.position ?: s.servedTo
    return "${streamClock(position)} / ${streamClock(s.duration)}" + if (s.position == null) " (estimated)" else ""
}

/** The lines under a stream's state: the file, its size and the transcode's details. */
internal fun streamDetails(s: AdminStream): List<String> = buildList {
    add(s.name)
    if (s.width > 0) add("Source ${s.width}×${s.height}")
    s.transcode?.let { t ->
        add(t.reason)
        add("Video: ${t.video}")
        add("Audio: ${t.audio}")
        t.sourceVideo?.let { add("Source: $it") }
        add(if (t.stalled) "Transcoder waiting for the player"
            else String.format(Locale.US, "Transcoder %.2f× realtime · %.0f fps", t.speed, t.fps))
    }
}

// ---------------------------------------------------------------------------------------------
// The Jobs tab: the server's background jobs, with what each is doing, its last and next run
// (a countdown) and Run now / Pause. Asked every few seconds; [now] ticks in between.

class JobsModel(val server: ServerRepository, private val scope: CoroutineScope) {
    var data by mutableStateOf<JobsResponse?>(null); internal set
    var error by mutableStateOf<String?>(null); internal set
    var now by mutableLongStateOf(System.currentTimeMillis()); internal set

    fun control(id: String, action: String) {
        scope.launch {
            val (d, err) = attempt { server.api.controlJob(id, action) }
            if (d != null) data = d
            error = err
        }
    }

    /** Saves trickplay's runners and frames per runner (0: automatic): an error to show, or null. */
    suspend fun saveTrickplay(runners: Int, frames: Int): String? = saveTrickplay(TrickplaySettingsRequest(runners, frames))

    /** Has trickplay decode on the GPU, or on the CPU: an error to show, or null. */
    suspend fun saveTrickplayGpu(useGpu: Boolean): String? = saveTrickplay(TrickplaySettingsRequest(useGpu = useGpu))

    private suspend fun saveTrickplay(request: TrickplaySettingsRequest): String? {
        val (resp, err) = attempt { server.api.setTrickplaySettings(request) }
        if (resp != null) data = resp
        return err
    }
}

@Composable
fun rememberJobs(server: ServerRepository): JobsModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address) { JobsModel(server, scope) }
    LaunchedEffect(model) {
        while (true) {
            val (d, err) = attempt { server.api.getJobs() }
            model.error = err
            if (d != null) model.data = d
            delay(5000)
        }
    }
    LaunchedEffect(model) {
        while (true) {
            model.now = System.currentTimeMillis()
            delay(1000)
        }
    }
    return model
}

internal const val JOBS_ABOUT = "Jobs run at low priority, so playback gets the CPU and disks first. They wait while a disk, network share or mergerfs pool " +
    "is busy (or, with the setting under Streaming, while anyone is streaming). Pause holds a job until you resume it, " +
    "or the server restarts."

/** What a trickplay field shows: its number, or nothing when it's automatic. */
internal fun trickplayField(auto: Boolean, value: Int) = if (auto) "" else value.toString()

/** The runners and frames per runner the fields give: an empty one is automatic, worked out as the server does
 *  (one runner per disk the libraries are on; the frames that fill half the memory for those runners). */
internal fun trickplayEffective(runners: String, frames: String, tp: com.vargon.weirdstreamer.api.models.TrickplaySettings): Pair<Int, Int> {
    val r = runners.toIntOrNull()?.takeIf { it > 0 } ?: tp.autoRunnersValue.takeIf { it > 0 } ?: tp.runners
    val f = frames.toIntOrNull()?.takeIf { it > 0 } ?: tp.autoFramesFor?.getOrNull(r - 1) ?: tp.framesPerRunner
    return r to f
}

/** A trickplay field's placeholder: what automatic gives. */
internal fun trickplayAutoRunners(tp: com.vargon.weirdstreamer.api.models.TrickplaySettings) =
    "Auto: ${tp.autoRunnersValue.takeIf { it > 0 } ?: tp.runners}"

internal fun trickplayAutoFrames(runners: String, tp: com.vargon.weirdstreamer.api.models.TrickplaySettings) =
    "Auto: ${trickplayEffective(runners, "", tp).second}"

/** What the trickplay fields give (empty ones automatic) with 4K videos: [trickplayMemoryTable]. */
internal fun trickplayTable(runners: String, frames: String, tp: com.vargon.weirdstreamer.api.models.TrickplaySettings): TrickplayTable {
    val (r, f) = trickplayEffective(runners, frames, tp)
    return trickplayMemoryTable(r, f, tp.memory)
}

/** The limits, and what automatic means. */
internal fun trickplayLimits(tp: com.vargon.weirdstreamer.api.models.TrickplaySettings) =
    "Runners 1 to ${tp.maxRunners}, frames per runner 1 to ${tp.maxFrames}, at most ${tp.maxProduct} frames at once. " +
        "Empty is automatic: a runner per disk the libraries are on, and frames filling half the server's memory with 4K videos."

/** A row of the trickplay memory table: what it's about, what the settings need with 4K videos, what the server has,
 *  and whether that fits ([warn] when it doesn't). */
data class TrickplayRow(val what: String, val needs: String, val has: String, val note: String, val warn: Boolean)

data class TrickplayTable(val summary: String, val rows: List<TrickplayRow>)

/** What [r] runners of [f] frames take with 4K videos (each frame in a batch holds a decoder of its own), as the
 *  server measured it for the decoder it uses ([m].decoder: the CPU, an Intel or AMD GPU whose buffers are the
 *  server's memory, or an NVIDIA card with memory of its own), against the server's memory and swap, and the card's:
 *  a summary line and a row each. The server uses the settings as they are. As the web's trickplayMemoryTable. */
internal fun trickplayMemoryTable(r: Int, f: Int, m: com.vargon.weirdstreamer.api.models.TrickplayMemory?): TrickplayTable {
    fun gb(bytes: Long) = String.format(Locale.US, "%.1f GB", bytes / 1073741824.0)
    val on = when (m?.decoder) {
        null -> ""
        "vaapi" -> ", decoded on the GPU"
        "cuda" -> ", decoded on the graphics card"
        else -> ", decoded on the CPU"
    }
    val summary = "$r × $f = ${r * f} frames at once$on."
    if (m == null || m.frameBytes4k <= 0) return TrickplayTable(summary, emptyList())
    val rows = mutableListOf<TrickplayRow>()
    val ram = r * (m.processBytes4k + f * m.frameBytes4k)
    val swap = m.swapBytes
    val what = if (m.decoder == "vaapi") "Memory (with the GPU's buffers)" else "Memory"
    rows += when {
        m.totalBytes <= 0 -> TrickplayRow(what, gb(ram), "?", "", false)
        else -> {
            val has = gb(m.totalBytes) + if (swap > 0) " + ${gb(swap)} swap" else ""
            when {
                ram <= m.totalBytes -> TrickplayRow(what, gb(ram), has, "Fits", false)
                ram <= m.totalBytes + swap -> TrickplayRow(what, gb(ram), has, "Uses swap", true)
                else -> TrickplayRow(what, gb(ram), has, "Too much: frames are made one at a time", true)
            }
        }
    }
    if (m.cardFrameBytes4k > 0) {
        val card = r * (m.cardProcessBytes4k + f * m.cardFrameBytes4k)
        rows += when {
            m.cardBytes <= 0 -> TrickplayRow("Graphics card", gb(card), "?", "", false)
            card <= m.cardBytes -> TrickplayRow("Graphics card", gb(card), gb(m.cardBytes), "Fits", false)
            else -> TrickplayRow("Graphics card", gb(card), gb(m.cardBytes), "Too much: batches are made smaller", true)
        }
    }
    return TrickplayTable(summary, rows)
}

internal fun loadText(l: JobLoad): String {
    fun pct(f: Double) = "${(f * 100).roundToInt()}%"
    val parts = mutableListOf<String>()
    // Jobs wait at the lower limit on a disk or share someone is playing from.
    if (l.available) {
        parts += listOf("CPU ${pct(l.cpu)}",
            "Disk ${pct(l.disk)} (jobs wait above ${pct(l.maxDisk)} on a disk someone is playing from, ${pct(l.relaxedDisk)} otherwise)")
        val shares = l.shares.orEmpty()
        if (shares.isNotEmpty()) {
            // Libraries on network shares or FUSE pools (mergerfs), whose disks can't be told apart
            parts += "Read times ${shares.joinToString(", ") { "${it.library} ${it.ms.roundToInt()} ms" }} " +
                "(above ${l.maxShareMs.roundToInt()} ms where someone is playing, ${l.relaxedShareMs.roundToInt()} ms otherwise)"
        }
    } else parts += "No load readings: jobs wait while anyone is streaming"
    if (l.streaming) parts += "Someone is streaming"
    if (l.pauseWhileStreaming) parts += "Jobs pause while anyone streams"
    return parts.joinToString(" · ")
}

internal fun epochMillis(time: String?): Long? =
    time?.let { runCatching { java.time.OffsetDateTime.parse(it).toInstant().toEpochMilli() }.getOrNull() }

/** A span of time: "2h 05m 09s", "4m 09s", "12s" (days as "3d 4h"). */
internal fun jobDuration(ms: Long): String {
    val s = (ms / 1000).coerceAtLeast(0)
    val d = s / 86400; val h = s % 86400 / 3600; val m = s % 3600 / 60; val sec = s % 60
    return when {
        d > 0 -> "${d}d ${h}h"
        h > 0 -> String.format(Locale.US, "%dh %02dm %02ds", h, m, sec)
        m > 0 -> String.format(Locale.US, "%dm %02ds", m, sec)
        else -> "${sec}s"
    }
}

internal fun jobWhen(at: Long, now: Long): String {
    val ago = now - at
    return when {
        ago < 60_000 -> "just now"
        ago < 86_400_000 -> jobDuration(ago).replace(Regex(" \\d+s$"), "") + " ago"
        else -> java.text.DateFormat.getDateTimeInstance(java.text.DateFormat.SHORT, java.text.DateFormat.SHORT).format(java.util.Date(at))
    }
}

/** A job's progress line while it runs: "1,234 of 5,000 · running for 4m 09s". */
internal fun jobProgress(j: com.vargon.weirdstreamer.api.models.BackgroundJob, started: Long, now: Long): String {
    val count = if (j.total > 0) String.format(Locale.US, "%,d of %,d · ", j.done, j.total) else ""
    return "${count}running for ${jobDuration(now - started)}"
}

/** "Last run: 2h ago, took 4m 09s · result". */
internal fun jobLastRun(j: com.vargon.weirdstreamer.api.models.BackgroundJob, now: Long): String {
    val lastEnd = epochMillis(j.lastEnd)
    val lastStart = epochMillis(j.lastStart)
    return if (lastEnd == null) "Last run: not since the server started"
    else "Last run: ${jobWhen(lastEnd, now)}, took ${jobDuration(lastEnd - (lastStart ?: lastEnd))}" +
        (j.lastResult?.takeIf { it.isNotEmpty() }?.let { " · $it" } ?: "")
}

/** "Next run: in 4m 09s" (or after this one, any moment, its schedule), "(paused)" when paused. */
internal fun jobNextRun(j: com.vargon.weirdstreamer.api.models.BackgroundJob, now: Long): String {
    val next = epochMillis(j.next)
    return when {
        epochMillis(j.started) != null -> "Next run: after this one"
        next != null -> if (next > now) "Next run: in ${jobDuration(next - now)}" else "Next run: any moment"
        else -> "Runs: ${j.schedule ?: "when asked"}"
    } + if (j.paused) " (paused)" else ""
}

// ---------------------------------------------------------------------------------------------
// Damaged videos (the Server tab): videos with a broken seek index, which the server plays from
// fixed copies; each can be played as it is or fixed, and replaced by its fixed copy. And the trash
// of replaced originals, each of which can be restored (its replace undone).

class DamagedVideosModel(val server: ServerRepository, private val scope: CoroutineScope) {
    var videos by mutableStateOf<List<com.vargon.weirdstreamer.api.models.DamagedVideo>?>(null); private set
    var trash by mutableStateOf(com.vargon.weirdstreamer.api.models.TrashReport()); private set
    var loadError by mutableStateOf<String?>(null); private set
    var message by mutableStateOf<Notice?>(null)

    private fun body(path: String, original: Boolean? = null) = JsonObject().apply {
        addProperty("path", android.util.Base64.encodeToString(path.toByteArray(), android.util.Base64.NO_WRAP))
        original?.let { addProperty("original", it) }
    }

    suspend fun load() {
        val (list, err) = attempt { server.api.getDamaged() }
        loadError = err
        list?.let { videos = it.videos.orEmpty() }
        attempt { server.api.getTrash() }.first?.let { trash = it }
    }

    /** Has the server give this account's players the original as it is (or the fixed copy), then [then] plays it. */
    fun play(video: com.vargon.weirdstreamer.api.models.DamagedVideo, original: Boolean, then: () -> Unit) {
        scope.launch {
            val (_, err) = attempt { server.api.playDamaged(body(video.path, original)) }
            if (err != null) message = err to true else then()
        }
    }

    /** Makes its fixed copy, in the server's background: the list says fixing until it's done. */
    fun fix(video: com.vargon.weirdstreamer.api.models.DamagedVideo) {
        scope.launch {
            val (_, err) = attempt { server.api.fixDamaged(body(video.path)) }
            message = (err ?: "Making the fixed copy of ${video.name}…") to (err != null)
            load()
        }
    }

    fun replace(video: com.vargon.weirdstreamer.api.models.DamagedVideo) {
        scope.launch {
            val (reply, err) = attempt { server.api.replaceDamaged(body(video.path)) }
            message = if (err != null) err to true
                else "Replaced: ${replacedName(video.name)} is in its place, and the original is in the trash." to false
            load()
        }
    }

    fun emptyTrash() {
        scope.launch {
            val (reply, err) = attempt { server.api.emptyTrash() }
            message = (err ?: "The trash is empty.") to (err != null)
            reply?.let { trash = it }
        }
    }

    /** Undoes its replace: the original back in its folder with its progress, listed as damaged again (fixed). */
    fun restore(item: com.vargon.weirdstreamer.api.models.TrashItem) {
        scope.launch {
            val (_, err) = attempt { server.api.restoreTrash(JsonObject().apply { addProperty("id", item.id) }) }
            message = (err ?: "Restored: ${item.name} is back in its folder.") to (err != null)
            load()
        }
    }
}

@Composable
fun rememberDamagedVideos(server: ServerRepository): DamagedVideosModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address) { DamagedVideosModel(server, scope) }
    LaunchedEffect(model) { model.load() }
    // While a copy is being made, asked again until it's done
    val fixing = model.videos?.any { it.fixing } == true
    LaunchedEffect(model, fixing) {
        while (fixing) {
            delay(3000)
            model.load()
        }
    }
    return model
}

/** The name a damaged video gets when its fixed copy replaces it: its own, as .mkv. */
internal fun replacedName(name: String) = name.substringBeforeLast('.', name) + ".mkv"

internal const val DAMAGED_ABOUT = "Videos players can't jump in (a broken seek index), found by scans. Fix makes a fixed copy (the " +
    "same video and audio, copied untouched into an indexed file, about as big as the video), which the server then plays in its " +
    "place. Play each to compare, then replace the original with the fixed copy (as .mkv, keeping its watch progress); the " +
    "original goes to the trash, where Restore puts it back."

/** "Original: 316 MB, says 2:07:52 long · Fixed copy: 298 MB, 1:14:32", or why it couldn't be fixed. */
internal fun damagedSizes(v: com.vargon.weirdstreamer.api.models.DamagedVideo) = when (v.state) {
    "fixed" -> "Original: ${formatBytes(v.size)}, says ${streamClock(v.claimed)} long · Fixed copy: ${formatBytes(v.fixedSize)}, ${streamClock(v.duration)}"
    "fixing" -> "${formatBytes(v.size)} · Making the fixed copy…"
    "failed" -> "${formatBytes(v.size)} · Couldn't be fixed: ${v.error ?: "unknown"}"
    else -> "${formatBytes(v.size)} · Not fixed: Fix makes a copy of about its size"
}

internal fun damagedWhere(v: com.vargon.weirdstreamer.api.models.DamagedVideo) =
    listOf(v.library, v.folder).filter { it.isNotEmpty() }.joinToString(" › ") + " · " + v.reason

internal fun trashText(t: com.vargon.weirdstreamer.api.models.TrashReport) =
    if (t.files > 0) "The trash holds ${t.files} replaced original${if (t.files == 1) "" else "s"} (${formatBytes(t.bytes)}), each deleted ${t.days} days after it went in."
    else "The trash is empty. Replaced originals are kept there ${t.days} days."

/** "Movies › Old" (where it was in its library). */
internal fun trashWhere(t: com.vargon.weirdstreamer.api.models.TrashItem) =
    listOf(t.library, t.folder).filter { it.isNotEmpty() }.joinToString(" › ").ifEmpty { "Not in a library" }

/** "11 MB · Replaced 11/24/26 by x.mkv · Deleted 12/24/26" */
internal fun trashDates(t: com.vargon.weirdstreamer.api.models.TrashItem): String {
    val day = { iso: String ->
        epochMillis(iso)?.let { java.text.DateFormat.getDateInstance(java.text.DateFormat.SHORT).format(java.util.Date(it)) } ?: "?"
    }
    return "${formatBytes(t.size)} · Replaced ${day(t.trashedAt)}" +
        (if (t.replacement.isNotEmpty()) " by ${t.replacement}" else "") + " · Deleted ${day(t.deletesAt)}"
}

/** What Restore does, for its confirmation. */
internal fun restoreText(t: com.vargon.weirdstreamer.api.models.TrashItem) =
    "It goes back to its folder, " + (if (t.replacement.isNotEmpty()) "${t.replacement} goes back to the fixed copies, " else "") +
        "and its watch progress goes back to it. It's listed with the damaged videos again."
