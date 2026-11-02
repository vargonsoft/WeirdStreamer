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

/** A build published on a server (deploy-release.sh --web: downloads/WeirdStreamer.json). */
data class AppRelease(val version: String, val build: Int, val size: Long, val server: String) {
    val apkUrl get() = "$server/downloads/WeirdStreamer.apk"
}

sealed interface UpdateState {
    data object Idle : UpdateState
    data object Checking : UpdateState
    data object UpToDate : UpdateState
    data object NotPublished : UpdateState // no server has one
    data class Available(val release: AppRelease) : UpdateState
    data class Downloading(val release: AppRelease, val fraction: Float) : UpdateState
    data object Installing : UpdateState // the system installer has it
    data class Failed(val message: String) : UpdateState
}

/**
 * Updates the app from the APK a WeirdStreamer server publishes: newer when its build number
 * (the git commit count) is above the installed one. The APK goes straight into a
 * PackageInstaller session, and the system asks the user to confirm the install.
 */
object AppUpdater {
    var state by mutableStateOf<UpdateState>(UpdateState.Idle)
        private set

    val installedVersion: String get() = BuildConfig.VERSION_NAME

    /** The build whose banner was closed: the next one shows again. */
    var dismissedBuild by mutableStateOf(0)

    /** A look for a newer build while the app runs, for the banner: leaves an update under way, or found, as it is. */
    suspend fun checkQuietly(servers: List<String>) {
        if (state in listOf(UpdateState.Idle, UpdateState.UpToDate, UpdateState.NotPublished)) check(servers)
    }

    private val client: OkHttpClient by lazy {
        // The API client's TLS settings; no read timeout for the APK itself.
        ApiClient.probeClient().newBuilder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .followRedirects(true)
            .build()
    }

    /** Looks for a newer build on [servers] (the first one's wins a tie). */
    suspend fun check(servers: List<String>) {
        if (state is UpdateState.Checking || state is UpdateState.Downloading) return
        state = UpdateState.Checking
        val releases = withContext(Dispatchers.IO) {
            servers.distinct().mapNotNull { server ->
                runCatching {
                    val base = server.trimEnd('/')
                    client.newCall(Request.Builder().url("$base/downloads/WeirdStreamer.json").build()).execute().use { r ->
                        if (!r.isSuccessful) return@runCatching null
                        val json = JSONObject(r.body!!.string())
                        AppRelease(json.getString("version"), json.getInt("build"), json.optLong("size"), base)
                    }
                }.getOrNull()
            }
        }
        val newest = releases.maxByOrNull { it.build }
        state = when {
            newest == null -> UpdateState.NotPublished
            newest.build > BuildConfig.VERSION_CODE -> UpdateState.Available(releases.first { it.build == newest.build })
            else -> UpdateState.UpToDate
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
                    client.newCall(Request.Builder().url(release.apkUrl).build()).execute().use { r ->
                        if (!r.isSuccessful) error("The server answered ${r.code}")
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
