package com.vargon.weirdstreamer.downloads

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
import com.vargon.weirdstreamer.api.models.CreateDownloadRequest
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
            val item = DownloadStore.nextQueued() ?: return Result.success()
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
        val api = ApiClient.create(start.serverAddress, credentials)
        val http = ApiClient.downloadClient(start.serverAddress, credentials)
        val encodedPath = Base64.encodeToString(start.sourcePath.toByteArray(), Base64.NO_WRAP)
        var item = start
        var width = 0
        var height = 0

        val options = api.downloadOptions(encodedPath).body()
        options?.let { o -> item = DownloadStore.update(item.id) { it.copy(durationSeconds = o.duration) } ?: return@withContext }
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
                    val created = api.createDownload(CreateDownloadRequest(encodedPath, item.sizePct, item.scale))
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
