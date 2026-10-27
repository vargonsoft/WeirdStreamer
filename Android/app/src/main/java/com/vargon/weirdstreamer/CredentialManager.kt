package com.vargon.weirdstreamer

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import java.util.concurrent.ConcurrentHashMap

class CredentialManager(context: Context) {
    private class CachedToken(val token: String?, val expiresAt: Long)

    companion object {
        // Tokens are read for every API and thumbnail request; decrypting them each time is
        // expensive. Shared across instances so a login/logout through one is seen by all.
        private val tokenCache = ConcurrentHashMap<String, CachedToken>()
    }

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

    // Library view (titles instead of folders) per movie/tv library; on unless turned off
    fun setLibraryMode(key: String, enabled: Boolean) {
        sharedPreferences.edit().putBoolean("library_mode_$key", enabled).apply()
    }

    fun isLibraryMode(key: String): Boolean {
        return sharedPreferences.getBoolean("library_mode_$key", true)
    }

    fun setGridView(path: String, isGrid: Boolean) {
        sharedPreferences.edit().putBoolean("grid_view_$path", isGrid).apply()
    }

    fun isGridView(path: String): Boolean {
        return sharedPreferences.getBoolean("grid_view_$path", false)
    }

    // Server API token management
    fun saveServerToken(serverUrl: String, token: String, expiresAt: Long) {
        tokenCache[serverUrl] = CachedToken(token, expiresAt)
        sharedPreferences.edit().apply {
            putString("${serverUrl}_token", token)
            putLong("${serverUrl}_expires", expiresAt)
            apply()
        }
    }

    fun getServerToken(serverUrl: String): String? {
        val cached = tokenCache.getOrPut(serverUrl) {
            CachedToken(
                sharedPreferences.getString("${serverUrl}_token", null),
                sharedPreferences.getLong("${serverUrl}_expires", 0)
            )
        }

        // Check if token is expired
        if (cached.token != null && System.currentTimeMillis() / 1000 < cached.expiresAt) {
            return cached.token
        }
        return null
    }

    /**
     * The account signed in to a server: the username in its token (a JWT, whose middle part is
     * the claims as base64url JSON), so sign-ins from before this was shown have it too. Null when
     * signed out.
     */
    fun getServerAccount(serverUrl: String): String? {
        val token = getServerToken(serverUrl) ?: return null
        return runCatching {
            val claims = android.util.Base64.decode(token.split(".")[1],
                android.util.Base64.URL_SAFE or android.util.Base64.NO_PADDING or android.util.Base64.NO_WRAP)
            org.json.JSONObject(String(claims)).optString("username").ifEmpty { null }
        }.getOrNull()
    }

    fun clearServerToken(serverUrl: String) {
        tokenCache.remove(serverUrl)
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
