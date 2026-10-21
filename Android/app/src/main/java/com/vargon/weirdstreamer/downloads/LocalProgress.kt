package com.vargon.weirdstreamer.downloads

import android.content.Context
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken

/**
 * Progress in downloaded videos played on this phone: kept here (so they resume offline) and
 * sent to their server when it can be reached. The server keeps whichever report is newer.
 */
object LocalProgress {
    data class Entry(
        val serverAddress: String,
        val sourcePath: String,
        val position: Double, // seconds
        val ended: Boolean,
        val updatedAt: Long, // Unix milliseconds
        val synced: Boolean = false
    )

    private val gson = Gson()
    private lateinit var prefs: android.content.SharedPreferences
    private val lock = Any()
    private var entries: MutableMap<String, Entry> = mutableMapOf()

    fun init(context: Context) = synchronized(lock) {
        if (::prefs.isInitialized) return
        prefs = context.getSharedPreferences("local_progress", Context.MODE_PRIVATE)
        entries = runCatching {
            gson.fromJson<Map<String, Entry>>(prefs.getString("entries", null),
                object : TypeToken<Map<String, Entry>>() {}.type)
        }.getOrNull()?.toMutableMap() ?: mutableMapOf()
    }

    private fun key(server: String, path: String) = "$server|$path"

    fun record(server: String, path: String, position: Double, ended: Boolean) = synchronized(lock) {
        entries[key(server, path)] = Entry(server, path, position, ended, System.currentTimeMillis())
        save()
    }

    fun get(server: String, path: String): Entry? = synchronized(lock) { entries[key(server, path)] }

    fun unsynced(server: String): List<Entry> = synchronized(lock) {
        entries.values.filter { it.serverAddress == server && !it.synced }
    }

    /** Marks entries sent, unless they changed since (played again meanwhile). */
    fun markSynced(sent: List<Entry>) = synchronized(lock) {
        sent.forEach { e ->
            val k = key(e.serverAddress, e.sourcePath)
            if (entries[k]?.updatedAt == e.updatedAt) entries[k] = e.copy(synced = true)
        }
        save()
    }

    fun forget(server: String, path: String) = synchronized(lock) {
        if (entries.remove(key(server, path)) != null) save()
    }

    private fun save() {
        prefs.edit().putString("entries", gson.toJson(entries)).apply()
    }
}
