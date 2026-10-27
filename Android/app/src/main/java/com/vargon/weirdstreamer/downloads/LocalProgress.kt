package com.vargon.weirdstreamer.downloads

import android.content.Context
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken

/**
 * Progress in downloaded videos played on this phone: kept here (so they resume offline) and
 * sent to their server when it can be reached. The server keeps whichever report is newer. Each
 * entry belongs to the account in use on its server when it was played.
 */
object LocalProgress {
    data class Entry(
        val serverAddress: String,
        val sourcePath: String,
        val position: Double, // seconds
        val ended: Boolean,
        val updatedAt: Long, // Unix milliseconds
        val synced: Boolean = false,
        val account: String? = null
    )

    /** The account in use on a server (set by the app). */
    var accountOf: (server: String) -> String? = { null }

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

    private fun key(server: String, account: String?, path: String) = "$server|@${account.orEmpty()}|$path"
    private fun key(e: Entry) = key(e.serverAddress, e.account, e.sourcePath)

    fun record(server: String, path: String, position: Double, ended: Boolean) = synchronized(lock) {
        val e = Entry(server, path, position, ended, System.currentTimeMillis(), account = accountOf(server))
        entries[key(e)] = e
        save()
    }

    /** The account in use's progress in a video. */
    fun get(server: String, path: String): Entry? = synchronized(lock) {
        entries[key(server, accountOf(server), path)]
    }

    fun unsynced(server: String): List<Entry> = synchronized(lock) {
        val account = accountOf(server)
        entries.values.filter { it.serverAddress == server && !it.synced && it.account == account }
    }

    /** Marks entries sent, unless they changed since (played again meanwhile). */
    fun markSynced(sent: List<Entry>) = synchronized(lock) {
        sent.forEach { e ->
            val k = key(e)
            if (entries[k]?.updatedAt == e.updatedAt) entries[k] = e.copy(synced = true)
        }
        save()
    }

    fun forget(server: String, path: String) = synchronized(lock) {
        if (entries.remove(key(server, accountOf(server), path)) != null) save()
    }

    private fun save() {
        prefs.edit().putString("entries", gson.toJson(entries)).apply()
    }
}
