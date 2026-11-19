package com.vargon.weirdstreamer.api

import android.content.Context
import android.content.SharedPreferences
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.OkHttpClient
import java.net.Socket
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.concurrent.ConcurrentHashMap
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLEngine
import javax.net.ssl.SSLSocket
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509ExtendedTrustManager
import javax.net.ssl.X509TrustManager

/**
 * A saved server answered with a certificate other than the one it had when it was added, and the
 * new one isn't a valid renewal (issued by a trusted authority for its name).
 */
class CertificateChangedException(host: String) : CertificateException(
    "$host's certificate has changed since it was added, so the app won't connect: someone may be " +
        "posing as the server. If you replaced its certificate, remove the server and add it again."
)

/**
 * Which HTTPS certificates the app accepts. Home servers often have self-signed certificates, so a
 * saved WeirdStreamer server's is trusted as it is, but only that one: the certificate it shows
 * when it's added (or, for a server saved before this, at its first connection) is kept, by its
 * SHA-256. A different one is accepted, and kept instead, only if it passes the usual check (a
 * trusted authority, and the server's name): a renewal such as Let's Encrypt's every few months.
 * Otherwise it's refused ([CertificateChangedException]), so a self-signed certificate can't change
 * without removing the server, which forgets it. Every other host is checked as usual.
 * Keyed by host and port, as the connection names them.
 */
object ServerTrust {
    private lateinit var prefs: SharedPreferences
    private var savedServers: () -> Set<String> = { emptySet() }

    // What probes of addresses not saved yet were shown, for keeping when the server is added
    private val seen = ConcurrentHashMap<String, String>()

    fun init(context: Context, servers: () -> Set<String>) {
        prefs = context.getSharedPreferences("server_certificates", Context.MODE_PRIVATE)
        savedServers = servers
    }

    private fun key(host: String, port: Int) = "${host.lowercase().removeSurrounding("[", "]")}:$port"

    private fun keyOf(address: String): String? =
        address.toHttpUrlOrNull()?.takeIf { it.isHttps }?.let { key(it.host, it.port) }

    private fun isSavedServer(key: String) = savedServers().any { keyOf(it) == key }

    private fun fingerprint(cert: X509Certificate): String =
        MessageDigest.getInstance("SHA-256").digest(cert.encoded).joinToString("") { "%02x".format(it) }

    /**
     * Keeps the certificate a newly added server showed when its address was checked. Returns
     * false if there was none to keep (an http address, or no check): then its first connection
     * keeps it, as a saved server's.
     */
    fun keepOnAdd(address: String): Boolean {
        val key = keyOf(address) ?: return false
        if (prefs.contains(key)) return true
        val fp = seen.remove(key) ?: return false
        prefs.edit().putString(key, fp).apply()
        return true
    }

    /** Forgets a removed server's certificate, so adding it again keeps the one it has then. */
    fun forget(address: String) {
        val key = keyOf(address) ?: return
        if (savedServers().none { keyOf(it) == key }) prefs.edit().remove(key).apply()
    }

    private val system: X509ExtendedTrustManager by lazy {
        val factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        factory.init(null as KeyStore?)
        factory.trustManagers.filterIsInstance<X509ExtendedTrustManager>().first()
    }

    private val trustManager = object : X509ExtendedTrustManager() {
        private fun check(chain: Array<out X509Certificate>, host: String?, port: Int, systemCheck: () -> Unit) {
            if (host == null || chain.isEmpty()) return systemCheck()
            val key = key(host, port)
            val fp = fingerprint(chain[0])
            val pinned = prefs.getString(key, null)
            when {
                pinned != null -> if (pinned != fp) {
                    // A renewal passes the usual check: a trusted authority, for this server's name
                    // (the hostname verifier lets kept servers through, so the name is checked here).
                    val renewed = runCatching { systemCheck() }.isSuccess &&
                        okhttp3.internal.tls.OkHostnameVerifier.verify(host, chain[0])
                    if (!renewed) throw CertificateChangedException(host)
                    prefs.edit().putString(key, fp).commit()
                }
                isSavedServer(key) -> prefs.edit().putString(key, fp).commit() // first connection
                else -> systemCheck()
            }
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String, socket: Socket?) {
            val session = (socket as? SSLSocket)?.handshakeSession
            check(chain, session?.peerHost, session?.peerPort ?: -1) { system.checkServerTrusted(chain, authType, socket) }
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String, engine: SSLEngine?) =
            check(chain, engine?.peerHost, engine?.peerPort ?: -1) { system.checkServerTrusted(chain, authType, engine) }

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) =
            system.checkServerTrusted(chain, authType)

        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String, socket: Socket?) =
            system.checkClientTrusted(chain, authType, socket)

        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String, engine: SSLEngine?) =
            system.checkClientTrusted(chain, authType, engine)

        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) =
            system.checkClientTrusted(chain, authType)

        override fun getAcceptedIssuers(): Array<X509Certificate> = system.acceptedIssuers
    }

    // A saved server's certificate is its own, so its name needn't match (self-signed ones often
    // don't, or the server is reached by its IP address); other hosts' must.
    private val hostnameVerifier = HostnameVerifier { host, session ->
        val key = key(host, session.peerPort)
        prefs.contains(key) || isSavedServer(key) || HttpsURLConnection.getDefaultHostnameVerifier().verify(host, session)
    }

    private val sslContext: SSLContext by lazy {
        SSLContext.getInstance("TLS").apply { init(null, arrayOf(trustManager), null) }
    }

    /** [builder] with these rules: every client that talks to servers uses them. */
    fun secure(builder: OkHttpClient.Builder): OkHttpClient.Builder =
        builder.sslSocketFactory(sslContext.socketFactory, trustManager).hostnameVerifier(hostnameVerifier)

    // Checking an address before it's added: whatever certificate it has (only /api/info is read,
    // nothing is sent), remembered for keepOnAdd.
    private val probeTrust = object : X509ExtendedTrustManager() {
        private fun remember(chain: Array<out X509Certificate>, host: String?, port: Int) {
            if (host != null && chain.isNotEmpty()) seen[key(host, port)] = fingerprint(chain[0])
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String, socket: Socket?) {
            val session = (socket as? SSLSocket)?.handshakeSession
            remember(chain, session?.peerHost, session?.peerPort ?: -1)
        }

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String, engine: SSLEngine?) =
            remember(chain, engine?.peerHost, engine?.peerPort ?: -1)

        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) {}
        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String, socket: Socket?) {}
        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String, engine: SSLEngine?) {}
        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) {}
        override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
    }

    /** [builder] for checking an address that isn't saved yet: see [keepOnAdd]. */
    fun probing(builder: OkHttpClient.Builder): OkHttpClient.Builder {
        val context = SSLContext.getInstance("TLS").apply { init(null, arrayOf(probeTrust), null) }
        return builder.sslSocketFactory(context.socketFactory, probeTrust as X509TrustManager).hostnameVerifier { _, _ -> true }
    }
}
