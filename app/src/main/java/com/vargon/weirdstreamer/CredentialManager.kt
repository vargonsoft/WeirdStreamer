package com.vargon.weirdstreamer

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

class CredentialManager(context: Context) {
    private val masterKey = MasterKey.Builder(context)
        .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
        .build()

    private val sharedPreferences: SharedPreferences = EncryptedSharedPreferences.create(
        context,
        "smb_credentials",
        masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )

    fun saveCredentials(host: String, user: String, pass: String) {
        sharedPreferences.edit().apply {
            putString("${host}_user", user)
            putString("${host}_pass", pass)
            apply()
        }
    }

    fun getCredentials(host: String): Pair<String?, String?> {
        val user = sharedPreferences.getString("${host}_user", null)
        val pass = sharedPreferences.getString("${host}_pass", null)
        return Pair(user, pass)
    }

    fun clearCredentials(host: String) {
        sharedPreferences.edit().apply {
            remove("${host}_user")
            remove("${host}_pass")
            apply()
        }
    }

    fun addSavedServer(address: String) {
        val servers = getSavedServers().toMutableSet()
        servers.add(address)
        sharedPreferences.edit().putStringSet("saved_servers", servers).apply()
    }

    fun removeSavedServer(address: String) {
        val servers = getSavedServers().toMutableSet()
        servers.remove(address)
        sharedPreferences.edit().putStringSet("saved_servers", servers).apply()
        clearCredentials(address)
    }

    fun getSavedServers(): Set<String> {
        return sharedPreferences.getStringSet("saved_servers", emptySet()) ?: emptySet()
    }

    fun setShowHiddenFiles(show: Boolean) {
        sharedPreferences.edit().putBoolean("show_hidden_files", show).apply()
    }

    fun getShowHiddenFiles(): Boolean {
        return sharedPreferences.getBoolean("show_hidden_files", false)
    }

    fun saveLastPath(path: String) {
        sharedPreferences.edit().putString("last_path", path).apply()
    }

    fun getLastPath(): String? {
        return sharedPreferences.getString("last_path", null)
    }

    fun setGridView(path: String, isGrid: Boolean) {
        sharedPreferences.edit().putBoolean("grid_view_$path", isGrid).apply()
    }

    fun isGridView(path: String): Boolean {
        return sharedPreferences.getBoolean("grid_view_$path", false)
    }

    // Server API token management
    fun saveServerToken(serverUrl: String, token: String, expiresAt: Long) {
        sharedPreferences.edit().apply {
            putString("${serverUrl}_token", token)
            putLong("${serverUrl}_expires", expiresAt)
            apply()
        }
    }

    fun getServerToken(serverUrl: String): String? {
        val token = sharedPreferences.getString("${serverUrl}_token", null)
        val expiresAt = sharedPreferences.getLong("${serverUrl}_expires", 0)
        
        // Check if token is expired
        if (token != null && System.currentTimeMillis() / 1000 < expiresAt) {
            return token
        }
        return null
    }

    fun clearServerToken(serverUrl: String) {
        sharedPreferences.edit().apply {
            remove("${serverUrl}_token")
            remove("${serverUrl}_expires")
            apply()
        }
    }

    fun addApiServer(serverUrl: String) {
        val servers = getApiServers().toMutableSet()
        servers.add(serverUrl)
        sharedPreferences.edit().putStringSet("api_servers", servers).apply()
    }

    fun removeApiServer(serverUrl: String) {
        val servers = getApiServers().toMutableSet()
        servers.remove(serverUrl)
        sharedPreferences.edit().putStringSet("api_servers", servers).apply()
        clearServerToken(serverUrl)
    }

    fun getApiServers(): Set<String> {
        return sharedPreferences.getStringSet("api_servers", emptySet()) ?: emptySet()
    }

    fun setAppTheme(theme: String) {
        sharedPreferences.edit().putString("app_theme", theme).apply()
    }

    fun getAppTheme(): String {
        return sharedPreferences.getString("app_theme", "DEFAULT") ?: "DEFAULT"
    }

    // Thumbnail cache versioning - increment when thumbnails are regenerated
    fun getThumbnailCacheVersion(serverUrl: String, path: String): Long {
        return sharedPreferences.getLong("thumb_ver_${serverUrl}_${path}", 0)
    }

    fun incrementThumbnailCacheVersion(serverUrl: String, path: String) {
        val currentVersion = getThumbnailCacheVersion(serverUrl, path)
        sharedPreferences.edit().putLong("thumb_ver_${serverUrl}_${path}", currentVersion + 1).apply()
    }
}
