package com.vargon.weirdstreamer

import android.content.Context
import android.util.Base64
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.PBEKeySpec

/**
 * The PINs that guard a server's accounts on this device: opening an account from the source list
 * asks for its PIN when it has one. Each is set on the device it guards (Settings › Account), so a
 * phone's is digits and a TV's a code of D-pad directions (U, D, L, R), at least [MIN_LENGTH] of
 * either. Kept as a salted PBKDF2 hash. After [FREE_TRIES] wrong ones in a row, each try waits
 * (30 s, doubling to an hour at most) until one is right. Forgotten, the account's password opens
 * it instead, which clears the PIN.
 */
class AccountPins private constructor(context: Context) {
    private val prefs = context.getSharedPreferences("account_pins", Context.MODE_PRIVATE)

    enum class Kind { Digits, Directions }

    sealed interface Check {
        data object Right : Check
        /** Wrong; [lockedUntil] (epoch ms) is when the next try counts, 0 when it can be tried now. */
        data class Wrong(val lockedUntil: Long) : Check
        /** Not tried: still waiting after earlier wrong ones. */
        data class Locked(val until: Long) : Check
    }

    private fun key(address: String, username: String) = "$address|$username"

    fun has(address: String, username: String) = prefs.contains("pin:" + key(address, username))

    fun kind(address: String, username: String): Kind? =
        prefs.getString("pin:" + key(address, username), null)?.substringBefore(':')?.let { k -> Kind.entries.firstOrNull { it.name == k } }

    fun set(address: String, username: String, code: String, kind: Kind) {
        val salt = ByteArray(16).also { SecureRandom().nextBytes(it) }
        val stored = "${kind.name}:${encode(salt)}:${encode(hash(code, salt))}"
        prefs.edit().putString("pin:" + key(address, username), stored).remove("fails:" + key(address, username))
            .remove("locked:" + key(address, username)).apply()
    }

    fun clear(address: String, username: String) {
        val k = key(address, username)
        prefs.edit().remove("pin:$k").remove("fails:$k").remove("locked:$k").apply()
    }

    /** When the next try counts (epoch ms), or 0. */
    fun lockedUntil(address: String, username: String): Long =
        prefs.getLong("locked:" + key(address, username), 0).takeIf { it > System.currentTimeMillis() } ?: 0

    fun check(address: String, username: String, code: String): Check {
        val k = key(address, username)
        val stored = prefs.getString("pin:$k", null) ?: return Check.Right
        lockedUntil(address, username).takeIf { it > 0 }?.let { return Check.Locked(it) }
        val parts = stored.split(':')
        val right = parts.size == 3 && MessageDigest.isEqual(hash(code, decode(parts[1])), decode(parts[2]))
        if (right) {
            prefs.edit().remove("fails:$k").remove("locked:$k").apply()
            return Check.Right
        }
        val fails = prefs.getInt("fails:$k", 0) + 1
        val until = if (fails < FREE_TRIES) 0L
            else System.currentTimeMillis() + (30_000L shl (fails - FREE_TRIES).coerceAtMost(7)).coerceAtMost(3_600_000L)
        prefs.edit().putInt("fails:$k", fails).putLong("locked:$k", until).apply()
        return Check.Wrong(until)
    }

    private fun hash(code: String, salt: ByteArray): ByteArray =
        SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256")
            .generateSecret(PBEKeySpec(code.toCharArray(), salt, 20_000, 256)).encoded

    private fun encode(b: ByteArray) = Base64.encodeToString(b, Base64.NO_WRAP)
    private fun decode(s: String) = Base64.decode(s, Base64.NO_WRAP)

    companion object {
        const val MIN_LENGTH = 4
        const val FREE_TRIES = 5

        @Volatile private var instance: AccountPins? = null
        fun get(context: Context): AccountPins =
            instance ?: synchronized(this) { instance ?: AccountPins(context.applicationContext).also { instance = it } }

        /** "0:30", "2:00": how long until [until]. */
        fun wait(until: Long): String {
            val s = ((until - System.currentTimeMillis() + 999) / 1000).coerceAtLeast(0)
            return "${s / 60}:${(s % 60).toString().padStart(2, '0')}"
        }
    }
}
