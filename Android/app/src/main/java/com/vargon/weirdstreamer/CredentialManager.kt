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

    // SMB servers keep their accounts' usernames and passwords (SMB has no tokens); the one chosen
    // last is the host's credentials, which browsing, thumbnails and playback use. A blank username
    // is guest access.
    private fun smbAccounts(host: String): List<Pair<String, String>> {
        val json = sharedPreferences.getString("${host}_smb_accounts", null) ?: return emptyList()
        return runCatching {
            val array = org.json.JSONArray(json)
            (0 until array.length()).map { i -> array.getJSONObject(i).let { it.getString("username") to it.getString("password") } }
        }.getOrDefault(emptyList())
    }

    private fun saveSmbAccounts(host: String, accounts: List<Pair<String, String>>) {
        val array = org.json.JSONArray()
        accounts.forEach { (u, p) -> array.put(org.json.JSONObject().put("username", u).put("password", p)) }
        sharedPreferences.edit().putString("${host}_smb_accounts", array.toString()).apply()
    }

    /** An SMB server's accounts ("" is guest access); they don't expire. */
    fun getSmbAccounts(host: String): List<ServerAccount> = smbAccounts(host).map { ServerAccount(it.first, true) }

    /** Makes an SMB account the host's credentials. */
    fun useSmbAccount(host: String, username: String): Boolean {
        val (user, pass) = smbAccounts(host).firstOrNull { it.first == username } ?: return false
        if (user.isEmpty()) clearCredentials(host) else saveCredentials(host, user, pass)
        return true
    }

    /** Keeps an SMB account that signed in (replacing its old password). */
    fun addSmbAccount(host: String, username: String, password: String) {
        saveSmbAccounts(host, smbAccounts(host).filter { it.first != username } + (username to password))
    }

    fun signOutSmbAccount(host: String, username: String) {
        saveSmbAccounts(host, smbAccounts(host).filter { it.first != username })
        if ((getCredentials(host).first ?: "") == username) clearCredentials(host)
    }

    /** The SMB account in use: its username, "" for guest. */
    fun getSmbAccount(host: String): String = getCredentials(host).first ?: ""

    fun addSavedServer(address: String) {
        val servers = getSavedServers().toMutableSet()
        servers.add(address)
        sharedPreferences.edit().putStringSet("saved_servers", servers).apply()
    }

    fun removeSavedServer(address: String) {
        val servers = getSavedServers().toMutableSet()
        servers.remove(address)
        sharedPreferences.edit().putStringSet("saved_servers", servers).remove("${address}_smb_accounts").remove("${address}_name").apply()
        clearCredentials(address)
    }

    /** A WeirdStreamer server's name, as it last said (null if never asked; blank if it has none). */
    fun getServerName(address: String): String? = sharedPreferences.getString("${address}_name", null)

    fun setServerName(address: String, name: String) {
        sharedPreferences.edit().putString("${address}_name", name).apply()
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
    /** Signs in an account: it joins the server's accounts (replacing an older sign-in) and becomes the one in use. */
    fun saveServerToken(serverUrl: String, token: String, expiresAt: Long) {
        val username = usernameIn(token)
        if (username != null) {
            val accounts = storedAccounts(serverUrl).filter { it.username != username } + StoredAccount(username, token, expiresAt)
            saveAccounts(serverUrl, accounts)
        }
        setActiveToken(serverUrl, token, expiresAt)
    }

    private fun setActiveToken(serverUrl: String, token: String, expiresAt: Long) {
        tokenCache[serverUrl] = CachedToken(token, expiresAt)
        sharedPreferences.edit().apply {
            putString("${serverUrl}_token", token)
            putLong("${serverUrl}_expires", expiresAt)
            apply()
        }
    }

    // Each server keeps the accounts signed in to it on this device, each with its own token; the
    // one chosen last is in use (its token is the server's token, which every request sends).
    private class StoredAccount(val username: String, val token: String, val expiresAt: Long)

    /** An account signed in to a server; [signedIn] is false once its sign-in has expired. */
    data class ServerAccount(val username: String, val signedIn: Boolean)

    private fun storedAccounts(serverUrl: String): List<StoredAccount> {
        val json = sharedPreferences.getString("${serverUrl}_accounts", null) ?: return emptyList()
        return runCatching {
            val array = org.json.JSONArray(json)
            (0 until array.length()).map { i ->
                val o = array.getJSONObject(i)
                StoredAccount(o.getString("username"), o.getString("token"), o.getLong("expiresAt"))
            }
        }.getOrDefault(emptyList())
    }

    private fun saveAccounts(serverUrl: String, accounts: List<StoredAccount>) {
        val array = org.json.JSONArray()
        accounts.forEach { array.put(org.json.JSONObject().put("username", it.username).put("token", it.token).put("expiresAt", it.expiresAt)) }
        sharedPreferences.edit().putString("${serverUrl}_accounts", array.toString()).apply()
    }

    /** The accounts signed in to a server, in the order they were first added. */
    fun getServerAccounts(serverUrl: String): List<ServerAccount> {
        val now = System.currentTimeMillis() / 1000
        return storedAccounts(serverUrl).map { ServerAccount(it.username, now < it.expiresAt) }
    }

    /** Makes [username] the account in use on a server. False when its sign-in has expired. */
    fun useServerAccount(serverUrl: String, username: String): Boolean {
        val account = storedAccounts(serverUrl).firstOrNull { it.username == username } ?: return false
        if (System.currentTimeMillis() / 1000 >= account.expiresAt) return false
        setActiveToken(serverUrl, account.token, account.expiresAt)
        return true
    }

    /** Signs an account out of a server and off this device's list; the server is then signed out if it was in use. */
    fun signOutServerAccount(serverUrl: String, username: String) {
        saveAccounts(serverUrl, storedAccounts(serverUrl).filter { it.username != username })
        if (getServerAccount(serverUrl) == username) clearActiveToken(serverUrl)
    }

    private fun usernameIn(token: String): String? = runCatching {
        val claims = android.util.Base64.decode(token.split(".")[1],
            android.util.Base64.URL_SAFE or android.util.Base64.NO_PADDING or android.util.Base64.NO_WRAP)
        org.json.JSONObject(String(claims)).optString("username").ifEmpty { null }
    }.getOrNull()

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
    fun getServerAccount(serverUrl: String): String? = getServerToken(serverUrl)?.let(::usernameIn)

    /** Signs out every account of a server (when it's removed). */
    fun clearServerToken(serverUrl: String) {
        sharedPreferences.edit().remove("${serverUrl}_accounts").apply()
        clearActiveToken(serverUrl)
    }

    private fun clearActiveToken(serverUrl: String) {
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
