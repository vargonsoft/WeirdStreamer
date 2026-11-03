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
    val subtitles: Boolean = false   // its subtitle tracks are saved (subtitleDir), or there are none
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

/** The file a saved subtitle track is in, in its download's subtitleDir. */
fun savedSubtitleName(t: com.vargon.weirdstreamer.api.models.SubtitleTrack) = "${t.index}." + if (t.text) "vtt" else "sup"

/**
 * The downloads on this device: kept in app storage (only this app can see them), listed in a
 * JSON index, and observed by the UI.
 */
object DownloadStore {
    private val gson = Gson()
    private lateinit var dir: File
    private lateinit var index: File
    private lateinit var prefs: android.content.SharedPreferences
    private val lock = Any()
    private val _items = MutableStateFlow<List<DownloadItem>>(emptyList())
    val items: StateFlow<List<DownloadItem>> = _items.asStateFlow()

    fun init(context: Context) {
        synchronized(lock) {
            if (::dir.isInitialized) return
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

    /** Downloads only use Wi-Fi (unmetered networks) unless this is turned off. */
    var wifiOnly: Boolean
        get() = prefs.getBoolean("wifi_only", true)
        set(value) { prefs.edit().putBoolean("wifi_only", value).apply() }

    fun find(serverAddress: String, sourcePath: String): DownloadItem? =
        _items.value.firstOrNull { it.serverAddress == serverAddress && it.sourcePath == sourcePath }

    fun get(id: String): DownloadItem? = _items.value.firstOrNull { it.id == id }

    fun nextQueued(): DownloadItem? = _items.value.filter { it.state == DownloadItem.State.QUEUED }.minByOrNull { it.createdAt }

    /** Adds downloads, skipping files already downloaded or downloading. */
    fun add(items: List<DownloadItem>): Int = synchronized(lock) {
        val fresh = items.filter { n -> _items.value.none { it.serverAddress == n.serverAddress && it.sourcePath == n.sourcePath } }
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

    /** The finished download playing from [path] (a file:// URL), if it's one. */
    fun ofLocalPath(path: String): DownloadItem? =
        _items.value.firstOrNull { it.localFile != null && path == "file://${it.localFile}" }

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
