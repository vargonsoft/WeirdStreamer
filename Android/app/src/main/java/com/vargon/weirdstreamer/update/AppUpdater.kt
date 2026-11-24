package com.vargon.weirdstreamer.update

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import com.vargon.weirdstreamer.BuildConfig
import com.vargon.weirdstreamer.api.ApiClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.util.concurrent.TimeUnit

/**
 * A build published on a server (deploy-release.sh --web: downloads/WeirdStreamer.json) or on
 * GitHub (a release's APK); [from] is the server's address, or "GitHub".
 */
data class AppRelease(val version: String, val build: Int, val size: Long, val from: String, val apkUrl: String)

sealed interface UpdateState {
    data object Idle : UpdateState
    data object Checking : UpdateState
    data object UpToDate : UpdateState
    data object NotPublished : UpdateState // no server has one, and GitHub couldn't be asked
    data class Available(val release: AppRelease) : UpdateState
    data class Downloading(val release: AppRelease, val fraction: Float) : UpdateState
    data object Installing : UpdateState // the system installer has it
    data class Failed(val message: String) : UpdateState
}

/**
 * Updates the app from the APK a WeirdStreamer server publishes, or else GitHub's latest release:
 * newer when its build number (the git commit count) is above the installed one. The APK goes
 * straight into a PackageInstaller session, and the system asks the user to confirm the install.
 * Servers give it only to accounts signed in, so each is asked with its token.
 */
object AppUpdater {
    const val GITHUB_REPO = "vargonsoft/WeirdStreamer"

    var state by mutableStateOf<UpdateState>(UpdateState.Idle)
        private set

    val installedVersion: String get() = BuildConfig.VERSION_NAME

    /** The build whose banner was closed: the next one shows again. */
    var dismissedBuild by mutableStateOf(0)

    /** A look for a newer build while the app runs, for the banner: leaves an update under way, or found, as it is. */
    suspend fun checkQuietly(servers: List<Pair<String, String>>) {
        if (state in listOf(UpdateState.Idle, UpdateState.UpToDate, UpdateState.NotPublished)) check(servers)
    }

    private fun signedIn(url: String, token: String?) = Request.Builder().url(url)
        .apply { if (token != null) header("Authorization", "Bearer $token") }.build()

    private val client: OkHttpClient by lazy {
        // Saved servers' certificate rules (ServerTrust).
        ApiClient.quickClient().newBuilder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .followRedirects(true)
            .build()
    }

    /** The tokens [check] was given, by server, for downloading the APK from the one it found. */
    private var tokens = emptyMap<String, String>()

    /** Looks for a newer build on [servers], each server's address and token (the first one's wins a tie). */
    suspend fun check(servers: List<Pair<String, String>>) {
        if (state is UpdateState.Checking || state is UpdateState.Downloading) return
        state = UpdateState.Checking
        tokens = servers.associate { (server, token) -> server.trimEnd('/') to token }
        val releases = withContext(Dispatchers.IO) {
            servers.distinctBy { it.first }.mapNotNull { (server, token) ->
                runCatching {
                    val base = server.trimEnd('/')
                    client.newCall(signedIn("$base/downloads/WeirdStreamer.json", token)).execute().use { r ->
                        if (!r.isSuccessful) return@runCatching null
                        val json = JSONObject(r.body!!.string())
                        AppRelease(json.getString("version"), json.getInt("build"), json.optLong("size"), base,
                            "$base/downloads/WeirdStreamer.apk")
                    }
                }.getOrNull()
            }
        }
        // None of the servers offers it (none signed in, or none publishes it): GitHub's latest release
        val offered = releases.ifEmpty { withContext(Dispatchers.IO) { listOfNotNull(runCatching { latestOnGitHub() }.getOrNull()) } }
        val newest = offered.maxByOrNull { it.build }
        state = when {
            newest == null -> UpdateState.NotPublished
            newest.build > BuildConfig.VERSION_CODE -> UpdateState.Available(offered.first { it.build == newest.build })
            else -> UpdateState.UpToDate
        }
    }

    /** The latest GitHub release's APK: its tag is v<version>, whose last part is the build number. */
    private fun latestOnGitHub(): AppRelease? {
        val request = Request.Builder().url("https://api.github.com/repos/$GITHUB_REPO/releases/latest")
            .header("Accept", "application/vnd.github+json").build()
        return client.newCall(request).execute().use { r ->
            if (!r.isSuccessful) return null
            val json = JSONObject(r.body!!.string())
            val version = json.getString("tag_name").removePrefix("v")
            val build = version.substringAfterLast('.').toIntOrNull() ?: return null
            val assets = json.getJSONArray("assets")
            val apk = (0 until assets.length()).map { assets.getJSONObject(it) }
                .firstOrNull { it.getString("name").endsWith(".apk") } ?: return null
            AppRelease(version, build, apk.optLong("size"), "GitHub", apk.getString("browser_download_url"))
        }
    }

    /** Downloads [release] into an install session and hands it to the system installer. */
    suspend fun install(context: Context, release: AppRelease) {
        state = UpdateState.Downloading(release, 0f)
        val installer = context.packageManager.packageInstaller
        var sessionId = -1
        try {
            sessionId = withContext(Dispatchers.IO) {
                val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
                    setAppPackageName(context.packageName)
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                        setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED)
                    }
                }
                val id = installer.createSession(params)
                installer.openSession(id).use { session ->
                    client.newCall(signedIn(release.apkUrl, tokens[release.from])).execute().use { r ->
                        if (!r.isSuccessful) error("${release.from} answered ${r.code}")
                        val body = r.body!!
                        val total = body.contentLength().takeIf { it > 0 } ?: release.size
                        session.openWrite("WeirdStreamer.apk", 0, total.takeIf { it > 0 } ?: -1).use { out ->
                            body.byteStream().use { input ->
                                val buffer = ByteArray(256 * 1024)
                                var done = 0L
                                while (true) {
                                    val n = input.read(buffer)
                                    if (n < 0) break
                                    out.write(buffer, 0, n)
                                    done += n
                                    if (total > 0) state = UpdateState.Downloading(release, (done.toFloat() / total).coerceAtMost(1f))
                                }
                            }
                            session.fsync(out)
                        }
                    }
                }
                id
            }
            val intent = Intent(context, UpdateReceiver::class.java)
            val pending = PendingIntent.getBroadcast(context, sessionId, intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE)
            state = UpdateState.Installing
            installer.openSession(sessionId).use { it.commit(pending.intentSender) }
        } catch (e: Exception) {
            if (sessionId >= 0) runCatching { installer.abandonSession(sessionId) }
            state = UpdateState.Failed(e.message ?: e.javaClass.simpleName)
        }
    }

    internal fun finished(status: Int, message: String?) {
        state = when (status) {
            PackageInstaller.STATUS_SUCCESS -> UpdateState.UpToDate // the app restarts on success anyway
            PackageInstaller.STATUS_FAILURE_ABORTED -> UpdateState.Idle // cancelled
            else -> UpdateState.Failed(message ?: "The install failed ($status)")
        }
    }
}

/** The installer's answers: a confirmation to show, or how the install ended. */
class UpdateReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirm = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
                } else intent.getParcelableExtra(Intent.EXTRA_INTENT)
                confirm?.let { context.startActivity(it.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
            }
            else -> AppUpdater.finished(status, intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE))
        }
    }
}
