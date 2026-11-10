package com.vargon.weirdstreamer.downloads

import android.content.Context
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File
import java.util.UUID

/** One file downloaded (or being downloaded) for offline viewing. */
data class DownloadItem(
    val id: String = UUID.randomUUID().toString(),
    val serverAddress: String,
    val account: String?,            // the account it was downloaded in, the only one that sees it
    val sourcePath: String,          // the file's path on the server
    val name: String,                // its file name
    val title: String? = null,       // what it is: "Example Movie (2008)", "Example Show S01E02 · ..."
    val thumbnailUrl: String? = null,
    val sizePct: Int = 100,          // 100 = original
    val scale: Int = 100,            // resolution, percent of the original's
    val isDisc: Boolean = false,     // a DVD image, downloaded as its main title in MP4
    val state: State = State.QUEUED,
    val stage: String? = null,       // what's happening now, for display
    val progress: Float = 0f,        // 0..1 within the stage
    val bytesDone: Long = 0,
    val bytesTotal: Long = 0,
    val jobId: String? = null,       // the server's encode, for compressed copies
    val localFile: String? = null,
    val thumbFile: String? = null,
    val width: Int = 0,
    val height: Int = 0,
    val error: String? = null,
    val createdAt: Long = System.currentTimeMillis(),
    val durationSeconds: Double = 0.0, // the video's length, for progress played from the phone
    // Compatibility: full quality, the video and audio kept as they are where this phone plays
    // them, the rest encoded on the server. One with nothing to encode becomes an original.
    val compat: Boolean = false,
    val subtitles: Boolean = false,  // its subtitle tracks are saved (subtitleDir), or there are none
    // When its subtitles, skip times and trickplay frames were last saved from the server (0: never)
    val extrasAt: Long = 0
) {
    enum class State { QUEUED, PREPARING, DOWNLOADING, DONE, FAILED }

    val isActive get() = state == State.QUEUED || state == State.PREPARING || state == State.DOWNLOADING
    val isOriginal get() = sizePct == 100 && scale == 100 && !isDisc && !compat

    /** "Original", "Compatibility", "Full quality", or "50% size · 75% resolution". */
    val qualityLabel: String
        get() = when {
            isOriginal -> "Original"
            compat -> "Compatibility"
            sizePct == 100 && scale == 100 -> "Full quality"
            else -> listOfNotNull(
                if (sizePct < 100) "$sizePct% size" else "Full quality",
                if (scale < 100) "$scale% resolution" else null
            ).joinToString(" · ")
        }
}

/** Which episode of which show a download is: its show's name (lowercased) and its numbers. */
data class DownloadEpisode(val show: String, val season: Int, val episode: Int)

// "Example Show S01E02 · Example Episode", the title a library download gets
private val TITLE_EPISODE = Regex("""^(.+?) S(\d+)E(\d+)(?: · |$)""")
// "Example Show.S01E02.720p.mkv", "Example Show - s1e2.mkv", "S01E02.mkv"
private val NAME_EPISODE = Regex("""^(.*?)[\s._-]*[Ss](\d{1,2})[\s._-]?[Ee](\d{1,3})""")
private val SEASON_FOLDER = Regex("""(?i)^(season|series|s)[\s._-]*\d+$""")

/**
 * The episode a download is, from its title (library downloads) or else its file name; a file
 * named only for its episode ("S01E02.mkv") takes its show from its folders. Null for a movie.
 */
val DownloadItem.episode: DownloadEpisode?
    get() {
        title?.let { TITLE_EPISODE.find(it) }?.let { m ->
            return DownloadEpisode(showKey(m.groupValues[1]), m.groupValues[2].toInt(), m.groupValues[3].toInt())
        }
        val m = NAME_EPISODE.find(name) ?: return null
        val show = showKey(m.groupValues[1]).ifEmpty {
            val folders = sourcePath.split('/').dropLast(1).drop(1).filter { it.isNotEmpty() } // without its library's ID
            showKey(folders.lastOrNull { !SEASON_FOLDER.matches(it) } ?: "")
        }
        return DownloadEpisode(show, m.groupValues[2].toInt(), m.groupValues[3].toInt())
    }

private fun showKey(name: String) = name.replace(Regex("""[\s._-]+"""), " ").trim().lowercase()

/**
 * The Downloads list's order: what's downloading or failed first, then the rest, each newest first,
 * except that a show's episodes stay together in episode order, where its newest one would be.
 */
fun downloadsInOrder(items: List<DownloadItem>): List<DownloadItem> =
    items.partition { it.isActive || it.state == DownloadItem.State.FAILED }.toList().flatMap { part ->
        part.groupBy { d -> d.episode?.let { d.serverAddress to it.show } ?: d.id }.values
            .sortedByDescending { group -> group.maxOf { it.createdAt } }
            .flatMap { group -> group.sortedWith(compareBy({ it.episode?.season }, { it.episode?.episode })) }
    }

/**
 * The file a saved subtitle track is in, in its download's subtitleDir: a file's by its key (its
 * place in the list moves when another is added before it), the video's own by their index.
 */
fun savedSubtitleName(t: com.vargon.weirdstreamer.api.models.SubtitleTrack) =
    (if (t.external != null && t.key != null) t.key.replace(Regex("[^A-Za-z0-9]"), "-") else "${t.index}") +
        if (t.text) ".vtt" else ".sup"

/** How long a download's subtitles, skip times and trickplay frames go before they're saved again. */
const val EXTRAS_REFRESH_MS = 12 * 60 * 60 * 1000L

/**
 * The downloads on this device: kept in app storage (only this app can see them), listed in a
 * JSON index, and observed by the UI.
 */
object DownloadStore {
    private val gson = Gson()
    private lateinit var dir: File
    private lateinit var index: File
    private lateinit var prefs: android.content.SharedPreferences
    private lateinit var appContext: Context
    private val lock = Any()
    private val _items = MutableStateFlow<List<DownloadItem>>(emptyList())
    val items: StateFlow<List<DownloadItem>> = _items.asStateFlow()

    fun init(context: Context) {
        synchronized(lock) {
            if (::dir.isInitialized) return
            appContext = context.applicationContext
            dir = File(context.filesDir, "downloads").apply { mkdirs() }
            File(dir, "thumbs").mkdirs()
            index = File(dir, "index.json")
            prefs = context.getSharedPreferences("downloads", Context.MODE_PRIVATE)
            _items.value = runCatching {
                gson.fromJson<List<DownloadItem>>(index.readText(), object : TypeToken<List<DownloadItem>>() {}.type)
            }.getOrNull().orEmpty()
                // A download interrupted by the app being killed carries on from where it was.
                .map { if (it.state == DownloadItem.State.PREPARING || it.state == DownloadItem.State.DOWNLOADING) it.copy(state = DownloadItem.State.QUEUED) else it }
        }
    }

    val directory: File get() = dir

    /** The account in use on a server (set by the app), even with its sign-in expired. */
    var accountOf: (server: String) -> String? = { null }

    /**
     * Whether a download is the account in use's: each account sees only its own. Downloads
     * from before they were kept per account have none, and every account sees them.
     */
    fun isMine(item: DownloadItem) = item.account == null || item.account == accountOf(item.serverAddress)

    /** The downloads the account in use on each server sees. */
    fun mine(items: List<DownloadItem>) = items.filter(::isMine)

    /** Downloads only use Wi-Fi (unmetered networks) unless this is turned off. */
    var wifiOnly: Boolean
        get() = prefs.getBoolean("wifi_only", true)
        set(value) { prefs.edit().putBoolean("wifi_only", value).apply() }

    /** The account in use's download of a server file. */
    fun find(serverAddress: String, sourcePath: String): DownloadItem? =
        _items.value.firstOrNull { it.serverAddress == serverAddress && it.sourcePath == sourcePath && isMine(it) }

    fun get(id: String): DownloadItem? = _items.value.firstOrNull { it.id == id }

    fun nextQueued(): DownloadItem? = _items.value.filter { it.state == DownloadItem.State.QUEUED }.minByOrNull { it.createdAt }

    /** Adds downloads, skipping files their account already downloaded or is downloading. */
    fun add(items: List<DownloadItem>): Int = synchronized(lock) {
        val fresh = items.filter { n ->
            _items.value.none { it.serverAddress == n.serverAddress && it.sourcePath == n.sourcePath && (it.account == null || it.account == n.account) }
        }
        if (fresh.isNotEmpty()) save(_items.value + fresh)
        fresh.size
    }

    fun update(id: String, change: (DownloadItem) -> DownloadItem): DownloadItem? = synchronized(lock) {
        var updated: DownloadItem? = null
        save(_items.value.map { if (it.id == id) change(it).also { u -> updated = u } else it })
        updated
    }

    /** Removes a download and its files. */
    fun remove(id: String) = synchronized(lock) {
        val item = get(id) ?: return
        item.localFile?.let { File(it).delete() }
        item.thumbFile?.let { File(it).delete() }
        partFile(item).delete()
        subtitleDir(item).deleteRecursively()
        trickplayDir(item).deleteRecursively()
        segmentsFile(item).delete()
        save(_items.value.filter { it.id != id })
    }

    fun partFile(item: DownloadItem) = File(dir, item.id + ".part")

    fun finalFile(item: DownloadItem): File {
        val ext = if (item.isOriginal) item.name.substringAfterLast('.', "mp4").lowercase() else "mp4"
        return File(dir, "${item.id}.$ext")
    }

    fun thumbFile(item: DownloadItem) = File(File(dir, "thumbs"), item.id + ".jpg")

    /** A download's subtitle tracks: tracks.json (the server's list of them) and one file each. */
    fun subtitleDir(item: DownloadItem) = File(File(dir, "subtitles"), item.id)

    /** A download's trickplay frames, named as on the server (frame-<seconds>.jpg). */
    fun trickplayDir(item: DownloadItem) = File(File(dir, "trickplay"), item.id)

    /** A download's intro and credits, as the server sent them (MediaSegments). */
    fun segmentsFile(item: DownloadItem) = File(File(dir, "segments"), item.id + ".json")

    /** The intro and credits saved with a download, if any. */
    fun savedSegments(item: DownloadItem): com.vargon.weirdstreamer.api.models.MediaSegments? = runCatching {
        gson.fromJson(segmentsFile(item).readText(), com.vargon.weirdstreamer.api.models.MediaSegments::class.java)
    }.getOrNull()

    /** The trickplay frames saved with a download (file:// URLs), if any. */
    fun savedTrickplay(item: DownloadItem): com.vargon.weirdstreamer.api.models.TrickplayInfo? {
        val frames = trickplayDir(item).listFiles().orEmpty().mapNotNull { f ->
            f.name.removePrefix("frame-").removeSuffix(".jpg").toIntOrNull()?.takeIf { f.name.startsWith("frame-") && f.name.endsWith(".jpg") }
                ?.let { com.vargon.weirdstreamer.api.models.TrickplayFrame(it, "file://${f.absolutePath}") }
        }.sortedBy { it.time }
        return if (frames.isEmpty()) null else com.vargon.weirdstreamer.api.models.TrickplayInfo(frames)
    }

    /** Whether a finished download's subtitles, skip times and trickplay frames are due to be saved (again). */
    fun needsExtras(item: DownloadItem, now: Long = System.currentTimeMillis()) =
        item.state == DownloadItem.State.DONE && (!item.subtitles || now - item.extrasAt > EXTRAS_REFRESH_MS)

    /** Has the downloads of a server file save their extras again, now (a subtitle was added to it). */
    fun extrasChanged(sourcePath: String) {
        synchronized(lock) {
            if (_items.value.none { it.sourcePath == sourcePath && it.state == DownloadItem.State.DONE }) return
            save(_items.value.map { if (it.sourcePath == sourcePath) it.copy(extrasAt = 0) else it })
        }
        DownloadWorker.schedule(appContext)
    }

    /** The finished download playing from [path] (a file:// URL), if it's one. */
    fun ofLocalPath(path: String): DownloadItem? =
        _items.value.firstOrNull { it.localFile != null && path == "file://${it.localFile}" }

    /** The downloaded episode of the same show (and server) that comes after [item], if there is one. */
    fun nextEpisode(item: DownloadItem): DownloadItem? {
        val ep = item.episode ?: return null
        return _items.value.asSequence()
            .filter { it.serverAddress == item.serverAddress && it.account == item.account && it.state == DownloadItem.State.DONE }
            .filter { it.localFile?.let { f -> File(f).exists() } == true }
            .mapNotNull { d -> d.episode?.takeIf { it.show == ep.show }?.let { d to it } }
            .filter { (_, e) -> e.season > ep.season || (e.season == ep.season && e.episode > ep.episode) }
            .minWithOrNull(compareBy({ it.second.season }, { it.second.episode }))?.first
    }

    fun bytesUsed(): Long = _items.value.sumOf { d -> d.localFile?.let { File(it).length() } ?: 0L }

    private fun save(list: List<DownloadItem>) {
        _items.value = list
        runCatching {
            val tmp = File(dir, "index.json.tmp")
            tmp.writeText(gson.toJson(list))
            tmp.renameTo(index)
        }
    }
}
