package com.vargon.weirdstreamer.downloads

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody.Companion.toRequestBody
import com.vargon.weirdstreamer.repository.normalizedServerUrl
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.pm.ServiceInfo
import android.os.Build
import android.util.Base64
import androidx.core.app.NotificationCompat
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.ForegroundInfo
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.vargon.weirdstreamer.CredentialManager
import com.vargon.weirdstreamer.api.ApiClient
import com.vargon.weirdstreamer.api.models.CodecsInfo
import com.vargon.weirdstreamer.api.models.CreateDownloadRequest
import com.vargon.weirdstreamer.ui.components.ServerSubtitles
import com.vargon.weirdstreamer.ui.components.deviceSubtitleLanguages
import com.google.gson.Gson
import java.util.Locale
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.net.URLEncoder
import java.util.concurrent.TimeUnit

/**
 * Works through the download queue one file at a time, in the background (a foreground service
 * with a notification), on Wi-Fi unless the user allows mobile data. Interrupted downloads resume
 * from where they stopped; WorkManager retries after network errors.
 */
class DownloadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    companion object {
        private const val WORK_NAME = "downloads"
        private const val CHANNEL = "downloads"
        private const val NOTIFICATION_ID = 4101

        /** Starts (or continues) the queue; [restart] re-applies the network setting. */
        fun schedule(context: Context, restart: Boolean = false) {
            val request = OneTimeWorkRequestBuilder<DownloadWorker>()
                .setConstraints(
                    Constraints.Builder()
                        .setRequiredNetworkType(if (DownloadStore.wifiOnly) NetworkType.UNMETERED else NetworkType.CONNECTED)
                        .build()
                )
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(context).enqueueUniqueWork(
                WORK_NAME,
                if (restart) ExistingWorkPolicy.REPLACE else ExistingWorkPolicy.APPEND_OR_REPLACE,
                request
            )
        }
    }

    private val credentials = CredentialManager(context)

    override suspend fun doWork(): Result {
        DownloadStore.init(applicationContext)
        while (true) {
            val item = DownloadStore.nextQueued() ?: return refreshExtras()
            setForeground(foregroundInfo(item, 0f))
            try {
                download(item)
            } catch (e: CancellationException) {
                throw e
            } catch (e: IOException) {
                // Network trouble: back to the queue, and WorkManager tries again later.
                DownloadStore.update(item.id) { it.copy(state = DownloadItem.State.QUEUED, stage = "Waiting to retry") }
                return Result.retry()
            } catch (e: Exception) {
                DownloadStore.update(item.id) { it.copy(state = DownloadItem.State.FAILED, error = e.message ?: "Download failed") }
            }
        }
    }

    private fun stillWanted(id: String) = DownloadStore.get(id) != null

    private suspend fun download(start: DownloadItem) = withContext(Dispatchers.IO) {
        // Downloaded as the account it's for, whichever is in use now.
        if (start.account != null && credentials.tokenOf(start.serverAddress, start.account) == null) {
            throw IllegalStateException("Sign in to the server as ${start.account} again, then retry")
        }
        val api = ApiClient.create(start.serverAddress, credentials, start.account)
        val http = ApiClient.downloadClient(start.serverAddress, credentials, start.account)
        val encodedPath = Base64.encodeToString(start.sourcePath.toByteArray(), Base64.NO_WRAP)
        var item = start
        var width = 0
        var height = 0

        val options = api.downloadOptions(encodedPath).body()
        options?.let { o -> item = DownloadStore.update(item.id) { it.copy(durationSeconds = o.duration) } ?: return@withContext }
        val codecs = api.getCodecs(encodedPath).body()
        // Compatibility keeps what this phone plays; with nothing to convert, it's the original.
        var keep: OfflinePlayback? = null
        if (item.compat && !item.isDisc) {
            keep = codecs?.let { OfflinePlayback.of(applicationContext, it, options?.width ?: 0, options?.height ?: 0) }
            if (keep?.plays == true) item = DownloadStore.update(item.id) { it.copy(compat = false) } ?: return@withContext
        }
        val url: String
        if (item.isOriginal) {
            options?.let { width = it.width; height = it.height }
            url = "${normalizedServerUrl(item.serverAddress)}/api/media/stream?path=${URLEncoder.encode(encodedPath, "UTF-8")}&auto-transcode=0"
        } else {
            // The server encodes the copy first; its progress is shown as "Preparing".
            var jobId = item.jobId
            while (true) {
                if (!stillWanted(item.id)) { jobId?.let { runCatching { api.deleteDownload(it) } }; return@withContext }
                if (jobId == null) {
                    val created = api.createDownload(CreateDownloadRequest(encodedPath, item.sizePct, item.scale,
                        keepVideo = keep?.video == true, keepAudio = keep?.audio == true))
                    val job = created.body() ?: throw IllegalStateException(errorText(created.errorBody()?.string()) ?: "The server couldn't prepare the file")
                    jobId = job.id
                    item = DownloadStore.update(item.id) { it.copy(jobId = job.id) } ?: return@withContext
                }
                val response = api.getDownload(jobId)
                if (response.code() == 404) { // the server restarted and lost the job: start over
                    jobId = null
                    DownloadStore.update(item.id) { it.copy(jobId = null) }
                    continue
                }
                val job = response.body() ?: throw IOException("Couldn't check the server")
                when (job.status) {
                    "ready" -> { width = job.width; height = job.height; break }
                    "failed" -> throw IllegalStateException(job.error ?: "The server couldn't prepare the file")
                }
                DownloadStore.update(item.id) {
                    it.copy(state = DownloadItem.State.PREPARING, progress = job.progress.toFloat(),
                        stage = if (job.status == "queued") "Waiting for the server" else "Preparing on the server")
                }
                setForeground(foregroundInfo(item, job.progress.toFloat()))
                delay(2000)
            }
            url = "${normalizedServerUrl(item.serverAddress)}/api/downloads/$jobId/file"
        }

        // Fetch the file, resuming a partial one.
        val part = DownloadStore.partFile(item)
        var done = if (part.exists()) part.length() else 0L
        val request = Request.Builder().url(url).apply { if (done > 0) header("Range", "bytes=$done-") }.build()
        http.newCall(request).execute().use { response ->
            if (response.code == 416) { // already complete
            } else if (!response.isSuccessful) {
                throw if (response.code >= 500) IOException("Server error ${response.code}") else IllegalStateException("Download refused (${response.code})")
            } else {
                if (response.code != 206) done = 0 // no resume: start over
                val total = done + (response.body?.contentLength()?.takeIf { it > 0 } ?: 0L)
                val started = System.currentTimeMillis()
                val from = done
                FileOutputStream(part, done > 0).use { out ->
                    val input = response.body!!.byteStream()
                    val buffer = ByteArray(256 * 1024)
                    var lastUpdate = 0L
                    while (true) {
                        val n = input.read(buffer)
                        if (n < 0) break
                        out.write(buffer, 0, n)
                        done += n
                        val now = System.currentTimeMillis()
                        if (now - lastUpdate > 700) {
                            lastUpdate = now
                            if (!stillWanted(item.id)) {
                                part.delete()
                                item.jobId?.let { runCatching { api.deleteDownload(it) } }
                                return@withContext
                            }
                            val progress = if (total > 0) done.toFloat() / total else 0f
                            DownloadStore.update(item.id) {
                                it.copy(state = DownloadItem.State.DOWNLOADING, stage = "Downloading", progress = progress,
                                    bytesDone = done, bytesTotal = total)
                            }
                            setForeground(foregroundInfo(item, progress))
                        }
                    }
                }
                DownloadStore.recordSpeed(done - from, System.currentTimeMillis() - started)
            }
        }
        if (!stillWanted(item.id)) {
            part.delete()
            item.jobId?.let { runCatching { api.deleteDownload(it) } }
            return@withContext
        }

        val final = DownloadStore.finalFile(item)
        if (!part.renameTo(final)) throw IllegalStateException("Couldn't save the file")
        val thumb = item.thumbnailUrl?.let { fetchThumbnail(http, it, DownloadStore.thumbFile(item)) }
        DownloadStore.update(item.id) {
            it.copy(state = DownloadItem.State.DONE, stage = null, progress = 1f, localFile = final.absolutePath,
                thumbFile = thumb?.absolutePath, width = width, height = height, bytesDone = final.length(),
                bytesTotal = final.length(), error = null)
        }
        // The server's copy isn't needed any more.
        item.jobId?.let { runCatching { api.deleteDownload(it) } }
        // Best effort: what isn't saved now is on a later run (refreshExtras).
        DownloadStore.get(item.id)?.let { done ->
            try { saveExtras(done, codecs) } catch (e: CancellationException) { throw e } catch (e: Exception) { }
        }
    }

    /**
     * Saves the subtitles, skip times and trickplay frames of finished downloads that don't have
     * them yet (downloaded before this app saved them, or while the server couldn't send them),
     * and again every [EXTRAS_REFRESH_MS] and when a subtitle was added to the video, for what
     * changed on the server since. A server that can't be reached is left until next time.
     */
    private suspend fun refreshExtras(): Result {
        val away = mutableSetOf<String>()
        for (item in DownloadStore.items.value.filter { DownloadStore.needsExtras(it) }) {
            if (item.serverAddress in away) continue
            try {
                saveExtras(item, null)
            } catch (e: CancellationException) {
                throw e
            } catch (e: IOException) {
                away += item.serverAddress // offline, or the server is away
            } catch (e: Exception) {
                continue // this file only
            }
        }
        return Result.success()
    }

    /**
     * Saves [item]'s subtitles, intro and credits, and trickplay frames. Each goes on without the
     * others; any but a network error (thrown) leaves it to try again next run.
     */
    private suspend fun saveExtras(item: DownloadItem, known: CodecsInfo?) {
        var failed = false
        for (step in listOf<suspend () -> Unit>({ saveSubtitles(item, known) }, { saveSegments(item) }, { saveTrickplay(item) })) {
            try { step() } catch (e: CancellationException) { throw e } catch (e: IOException) { throw e } catch (e: Exception) { failed = true }
        }
        if (!failed) DownloadStore.update(item.id) { it.copy(subtitles = true, extrasAt = System.currentTimeMillis()) }
    }

    /** Saves [item]'s intro and credits (none: an empty answer), for Skip Intro and Skip Credits offline. */
    private suspend fun saveSegments(item: DownloadItem) = withContext(Dispatchers.IO) {
        val encodedPath = Base64.encodeToString(item.sourcePath.toByteArray(), Base64.NO_WRAP)
        val r = ApiClient.create(item.serverAddress, credentials, item.account).getSegments(encodedPath)
        if (r.code() >= 500) throw IOException("Server error ${r.code()}")
        val segments = r.body()
        if (segments == null) { DownloadStore.segmentsFile(item).delete(); return@withContext }
        val file = DownloadStore.segmentsFile(item).apply { parentFile?.mkdirs() }
        File(file.path + ".part").apply { writeText(Gson().toJson(segments)) }.renameTo(file)
    }

    /**
     * Saves [item]'s trickplay frames (small JPEGs, one every 10 seconds), fetching only those it
     * doesn't have and dropping those the server no longer has. None yet on the server, or a
     * server from before it sent them by path: nothing, until they're made.
     */
    private suspend fun saveTrickplay(item: DownloadItem) = withContext(Dispatchers.IO) {
        val encodedPath = Base64.encodeToString(item.sourcePath.toByteArray(), Base64.NO_WRAP)
        val r = ApiClient.create(item.serverAddress, credentials, item.account).getTrickplayByPath(encodedPath)
        if (r.code() > 500) throw IOException("Server error ${r.code()}") // 500: it has no trickplay folder
        val frames = r.body()?.frames.orEmpty()
        val dir = DownloadStore.trickplayDir(item).apply { mkdirs() }
        val names = frames.map { it.url.substringAfterLast('/') }.toSet()
        dir.listFiles().orEmpty().filter { it.name !in names }.forEach { it.delete() }
        val http = ApiClient.downloadClient(item.serverAddress, credentials, item.account)
        val base = normalizedServerUrl(item.serverAddress)
        frames.forEachIndexed { i, frame ->
            if (i % 50 == 0 && !stillWanted(item.id)) return@withContext
            val file = File(dir, frame.url.substringAfterLast('/'))
            if (file.length() > 0) return@forEachIndexed
            val url = if (frame.url.startsWith("http")) frame.url else base + (if (frame.url.startsWith("/")) "" else "/") + frame.url
            http.newCall(Request.Builder().url(url).build()).execute().use { f ->
                if (!f.isSuccessful) return@forEachIndexed // this frame only
                val tmp = File(dir, file.name + ".part")
                tmp.outputStream().use { f.body!!.byteStream().copyTo(it) }
                tmp.renameTo(file)
            }
        }
        if (!stillWanted(item.id)) dir.deleteRecursively()
    }

    /**
     * Saves [item]'s subtitle tracks beside it, for playing offline: the ones the players draw
     * (text, as WebVTT; ASS/SSA whole, styled, with the video's fonts; and PGS pictures), in this
     * phone's languages, forced ones and files the server has beside the video. Each is read from
     * the server whole, as it sends a window.
     */
    private suspend fun saveSubtitles(item: DownloadItem, known: CodecsInfo?) = withContext(Dispatchers.IO) {
        val encodedPath = Base64.encodeToString(item.sourcePath.toByteArray(), Base64.NO_WRAP)
        val info = known ?: ApiClient.create(item.serverAddress, credentials, item.account).getCodecs(encodedPath).body()
            ?: throw IllegalStateException("The server couldn't read the file's subtitles")
        val languages = deviceSubtitleLanguages().mapNotNull { runCatching { Locale(it).isO3Language }.getOrNull() }.toSet()
        val wanted = info.subtitles.orEmpty().filter { t ->
            (t.text || t.codec == "hdmv_pgs_subtitle") &&
                (t.forced || t.external != null || t.language.isNullOrEmpty() || t.language == "und" || t.language in languages)
        }
        val dir = DownloadStore.subtitleDir(item).apply { mkdirs() }
        val http = ApiClient.downloadClient(item.serverAddress, credentials, item.account)
        val saved = wanted.filter { t ->
            val file = File(dir, savedSubtitleName(t))
            if (file.length() > 0) return@filter true
            val url = "${normalizedServerUrl(item.serverAddress)}/api/media/subtitles?path=${URLEncoder.encode(encodedPath, "UTF-8")}" +
                "&track=${t.index}&start=0&duration=0" + when {
                    ServerSubtitles.isStyled(t) -> "&format=ass"
                    t.text -> ""
                    else -> "&format=sup"
                }
            http.newCall(Request.Builder().url(url).build()).execute().use { r ->
                if (!r.isSuccessful) return@filter false // this track only
                val tmp = File(dir, file.name + ".part")
                tmp.outputStream().use { r.body!!.byteStream().copyTo(it) }
                tmp.renameTo(file)
            }
        }
        // The fonts the ASS tracks name (embedded in the video), when any are saved.
        val fonts = if (saved.none { ServerSubtitles.isStyled(it) }) emptyList() else info.fonts.orEmpty().filter { f ->
            val file = File(dir, savedFontName(f))
            if (file.length() > 0) return@filter true
            val url = "${normalizedServerUrl(item.serverAddress)}/api/media/font?path=${URLEncoder.encode(encodedPath, "UTF-8")}&font=${f.index}"
            http.newCall(Request.Builder().url(url).build()).execute().use { r ->
                if (!r.isSuccessful) return@filter false
                val tmp = File(dir, file.name + ".part")
                tmp.outputStream().use { r.body!!.byteStream().copyTo(it) }
                tmp.renameTo(file)
            }
        }
        // The CJK font for the saved ASS tracks' Chinese, Japanese and Korean characters (the server's
        // subset), so they're drawn offline too: "font-cjk.ttf", one per download.
        val cjkFile = File(dir, "font-cjk.ttf")
        val styledText = saved.filter { ServerSubtitles.isStyled(it) }
            .joinToString("\n") { runCatching { File(dir, savedSubtitleName(it)).readText() }.getOrDefault("") }
        val cjkChars = styledText.filter { ServerSubtitles.isCjk(it) }.toSet().joinToString("")
        if (styledText.isNotEmpty()) {
            val body = Gson().toJson(mapOf("text" to cjkChars))
                .toRequestBody("application/json".toMediaType())
            runCatching {
                http.newCall(Request.Builder().url("${normalizedServerUrl(item.serverAddress)}/api/media/subtitle-font").post(body).build()).execute().use { r ->
                    if (!r.isSuccessful) return@use
                    val tmp = File(dir, cjkFile.name + ".part")
                    tmp.outputStream().use { r.body!!.byteStream().copyTo(it) }
                    tmp.renameTo(cjkFile)
                }
            }
        }
        // Files of tracks gone from the server (or saved under their index before): not needed.
        val keep = saved.map(::savedSubtitleName).toSet() + fonts.map(::savedFontName) + "font-cjk.ttf" + "tracks.json"
        File(dir, "tracks.json").writeText(Gson().toJson(saved))
        dir.listFiles().orEmpty().filter { it.name !in keep }.forEach { it.delete() }
    }

    private fun fetchThumbnail(http: okhttp3.OkHttpClient, url: String, dest: File): File? = runCatching {
        http.newCall(Request.Builder().url(url).build()).execute().use { r ->
            if (!r.isSuccessful) return null
            dest.outputStream().use { r.body!!.byteStream().copyTo(it) }
        }
        dest
    }.getOrNull()

    private fun errorText(body: String?): String? = runCatching {
        com.google.gson.JsonParser.parseString(body).asJsonObject.get("message")?.asString
    }.getOrNull()

    private fun foregroundInfo(item: DownloadItem, progress: Float): ForegroundInfo {
        val manager = applicationContext.getSystemService(NotificationManager::class.java)
        if (manager.getNotificationChannel(CHANNEL) == null) {
            manager.createNotificationChannel(NotificationChannel(CHANNEL, "Downloads", NotificationManager.IMPORTANCE_LOW))
        }
        val queued = DownloadStore.items.value.count { it.isActive }
        val notification = NotificationCompat.Builder(applicationContext, CHANNEL)
            .setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle(item.title ?: item.name)
            .setContentText(if (queued > 1) "$queued downloads left" else "Downloading for offline viewing")
            .setProgress(100, (progress * 100).toInt(), progress <= 0f)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .build()
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ForegroundInfo(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        } else {
            ForegroundInfo(NOTIFICATION_ID, notification)
        }
    }
}
