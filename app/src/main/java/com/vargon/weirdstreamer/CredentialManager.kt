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
}
