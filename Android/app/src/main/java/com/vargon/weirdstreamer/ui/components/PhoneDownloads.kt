package com.vargon.weirdstreamer.ui.components

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import coil.compose.AsyncImage
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CodecsInfo
import com.vargon.weirdstreamer.api.models.DownloadOptions
import com.vargon.weirdstreamer.api.models.LibraryFile
import com.vargon.weirdstreamer.downloads.DownloadItem
import com.vargon.weirdstreamer.downloads.DownloadAhead
import com.vargon.weirdstreamer.downloads.DownloadLanguages
import com.vargon.weirdstreamer.downloads.DownloadStore
import com.vargon.weirdstreamer.downloads.DownloadWorker
import com.vargon.weirdstreamer.repository.FileItem
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.withPermit
import java.io.File

/** A server file that can be downloaded: its path on the server, and how it's shown. */
data class DownloadRequest(
    val sourcePath: String,
    val name: String,
    val size: Long,
    val title: String? = null,
    val thumbnailUrl: String? = null
) {
    val isDisc get() = name.endsWith(".iso", ignoreCase = true)

    companion object {
        fun of(item: FileItem, title: String? = null) = item.originalPath?.let {
            DownloadRequest(it, item.name, item.size, title, item.thumbnailUrl)
        }

        fun of(viewModel: MediaViewModel, file: LibraryFile, title: String? = null, still: String? = null) =
            DownloadRequest(file.path, file.name, file.size, title,
                viewModel.serverUrl(file.thumbnailUrl) ?: viewModel.serverUrl(still))
    }
}

/** The download of [request] on this device, kept current. */
@Composable
private fun rememberDownload(viewModel: MediaViewModel, sourcePath: String): DownloadItem? {
    val items by DownloadStore.items.collectAsState()
    val server = viewModel.connectedServer?.address ?: return null
    return items.firstOrNull { it.serverAddress == server && it.sourcePath == sourcePath && DownloadStore.isMine(it) }
}

/**
 * A file's download control: a download icon, a progress circle while it downloads (tap to
 * cancel), a delete button once it's downloaded, or an error icon if it failed.
 */
@Composable
fun DownloadButton(viewModel: MediaViewModel, request: DownloadRequest) {
    val download = rememberDownload(viewModel, request.sourcePath)
    var choosing by remember { mutableStateOf(false) }
    var confirming by remember { mutableStateOf(false) }
    when {
        download == null -> IconButton(onClick = { choosing = true }) {
            Icon(Icons.Default.Download, contentDescription = "Download")
        }
        download.isActive -> IconButton(onClick = { confirming = true }) {
            DownloadProgress(download)
        }
        download.state == DownloadItem.State.DONE -> IconButton(onClick = { confirming = true }) {
            Icon(Icons.Default.Delete, contentDescription = "Delete download")
        }
        else -> IconButton(onClick = { confirming = true }) {
            Icon(Icons.Default.ErrorOutline, contentDescription = "Download failed", tint = MaterialTheme.colorScheme.error)
        }
    }
    if (choosing) DownloadQualityDialog(viewModel, listOf(request)) { choosing = false }
    if (confirming && download != null) RemoveDownloadDialog(download) { confirming = false }
}

/** A progress circle with a stop mark; it spins while waiting. */
@Composable
private fun DownloadProgress(download: DownloadItem) {
    Box(contentAlignment = Alignment.Center, modifier = Modifier.size(28.dp)) {
        if (download.state == DownloadItem.State.QUEUED || download.progress <= 0f) {
            CircularProgressIndicator(modifier = Modifier.size(28.dp), strokeWidth = 3.dp)
        } else {
            CircularProgressIndicator(progress = { download.progress }, modifier = Modifier.size(28.dp), strokeWidth = 3.dp)
        }
        Icon(Icons.Default.Close, contentDescription = "Cancel download", modifier = Modifier.size(14.dp))
    }
}

/** Confirms deleting a download, cancelling one in progress, or retrying a failed one. */
@Composable
private fun RemoveDownloadDialog(download: DownloadItem, onDismiss: () -> Unit) {
    val context = LocalContext.current
    val name = download.title ?: download.name
    when {
        download.state == DownloadItem.State.FAILED -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text("Download failed") },
            text = { Text("$name\n\n${download.error ?: "Unknown error"}") },
            confirmButton = {
                PhoneButton(onClick = {
                    DownloadStore.update(download.id) { it.copy(state = DownloadItem.State.QUEUED, error = null, progress = 0f) }
                    DownloadWorker.schedule(context)
                    onDismiss()
                }) { Text("Retry") }
            },
            dismissButton = {
                PhoneSecondaryButton(onClick = { DownloadStore.remove(download.id); onDismiss() }) { Text("Remove") }
            }
        )
        else -> {
            val done = download.state == DownloadItem.State.DONE
            AlertDialog(
                onDismissRequest = onDismiss,
                title = { Text(if (done) "Delete download?" else "Cancel download?") },
                text = {
                    Text(if (done) "$name will be removed from this phone (${formatBytes(download.bytesTotal)})."
                    else "$name will stop downloading.")
                },
                confirmButton = {
                    PhoneButton(destructive = true, onClick = { DownloadStore.remove(download.id); onDismiss() }) {
                        Text(if (done) "Delete" else "Cancel download")
                    }
                },
                dismissButton = { PhoneSecondaryButton(onClick = onDismiss) { Text(if (done) "Keep" else "Keep downloading") } }
            )
        }
    }
}

/** How a download is made: the file as it is, a full-quality copy this phone plays, or a smaller one. */
private enum class DownloadMode { ORIGINAL, COMPAT, COMPRESSED }

/**
 * Asks how to download [requests]: the original file; Compatibility, a full-quality copy that
 * keeps what this phone plays and converts the rest on the server; or a compressed copy at a share
 * of the original's size and resolution. Disc images download as their main title in MP4. Each
 * file is checked against this phone's decoders first: one that won't play here as it is makes
 * Compatibility the choice, and says why.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun DownloadQualityDialog(viewModel: MediaViewModel, requests: List<DownloadRequest>, onDismiss: () -> Unit) {
    val context = LocalContext.current
    val server = viewModel.connectedServer ?: return onDismiss()
    val pending = remember(requests) {
        requests.filter { r -> DownloadStore.find(server.address, r.sourcePath) == null }
    }
    var picked by remember { mutableStateOf<DownloadMode?>(null) }
    var sizePct by remember { mutableIntStateOf(50) }
    var scale by remember { mutableIntStateOf(100) }
    // One file: its details for the estimate.
    var options by remember { mutableStateOf<DownloadOptions?>(null) }
    // Each file's tracks and picture size (discs are converted anyway); null while checking.
    var probed by remember { mutableStateOf<Map<String, Pair<CodecsInfo, DownloadOptions?>>?>(null) }
    LaunchedEffect(pending) {
        pending.singleOrNull()?.let { r ->
            options = server.downloadOptions(r.sourcePath)
        }
        val limit = kotlinx.coroutines.sync.Semaphore(4)
        probed = kotlinx.coroutines.coroutineScope {
            pending.filter { !it.isDisc }.map { r ->
                async {
                    limit.withPermit {
                        val codecs = server.codecs(r.sourcePath) ?: return@withPermit null
                        val size = if (pending.size == 1) options else server.downloadOptions(r.sourcePath)
                        r.sourcePath to (codecs to size)
                    }
                }
            }.awaitAll().filterNotNull().toMap()
        }
    }
    // The audio and subtitle languages to download with: offered when the files have a choice,
    // starting with the last chosen (if changed then) or else what playing would pick.
    val files = probed?.values?.map { it.first }.orEmpty()
    val audioOptions = remember(probed) { DownloadLanguages.audioLanguages(files) }
    val subtitleOptions = remember(probed) { DownloadLanguages.subtitleLanguages(files) }
    var audioLanguage by remember(probed) { mutableStateOf(DownloadLanguages.startAudio(files, audioOptions)) }
    var subtitleLanguage by remember(probed) { mutableStateOf(DownloadLanguages.startSubtitles(files, subtitleOptions)) }
    var languagesChanged by remember { mutableStateOf(false) }
    val askAudio = audioOptions.size > 1
    val askSubtitles = subtitleOptions.isNotEmpty()
    // What this phone can't play as it is, by file, with the audio chosen.
    val checks = remember(probed, audioLanguage) {
        probed?.mapValues { (_, p) ->
            val (codecs, size) = p
            com.vargon.weirdstreamer.downloads.OfflinePlayback.of(context, codecs, size?.width ?: 0, size?.height ?: 0,
                DownloadLanguages.audioTrack(codecs, if (askAudio) audioLanguage else null))
        }
    }
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    val allDiscs = pending.isNotEmpty() && pending.all { it.isDisc }
    val anyDiscs = pending.any { it.isDisc }
    val unplayable = checks?.values?.filter { !it.plays }.orEmpty()
    // Until one is picked: Compatibility when something won't play here as it is.
    val mode = picked ?: if (unplayable.isNotEmpty()) DownloadMode.COMPAT else DownloadMode.ORIGINAL
    val compressed = mode == DownloadMode.COMPRESSED

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(when (pending.size) {
                0 -> "Already downloaded"
                1 -> "Download ${pending[0].name}"
                else -> "Download ${pending.size} files"
            }, style = MaterialTheme.typography.titleMedium, maxLines = 4, overflow = TextOverflow.Ellipsis)
        },
        text = {
            if (pending.isEmpty()) {
                Text(if (requests.size > 1) "All of these files are downloaded or downloading." else "This file is downloaded or downloading.")
                return@AlertDialog
            }
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                QualityChoice(
                    selected = mode == DownloadMode.ORIGINAL,
                    label = if (allDiscs) "Full quality MP4" else "Original",
                    detail = when {
                        allDiscs -> "The disc's main title, converted on the server"
                        anyDiscs -> "Discs download as their main title in MP4"
                        else -> "The file as it is"
                    }
                ) { picked = DownloadMode.ORIGINAL }
                when {
                    allDiscs -> {}
                    checks == null -> Text("Checking what this phone plays…", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 32.dp))
                    unplayable.isNotEmpty() -> Text(
                        if (pending.size == 1) "This phone can't play ${unplayable[0].problem}."
                        else "${unplayable.size} of ${pending.size} files won't play on this phone as they are.",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(start = 32.dp))
                }
                if (!allDiscs) QualityChoice(
                    selected = mode == DownloadMode.COMPAT,
                    label = "Compatibility",
                    detail = compatDetail(if (pending.size == 1) checks?.values?.firstOrNull() else null)
                ) { picked = DownloadMode.COMPAT }
                QualityChoice(selected = compressed, label = "Compressed", detail = "A smaller copy, made on the server first") {
                    picked = DownloadMode.COMPRESSED
                }
                if (compressed) {
                    Text("Size", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 8.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        listOf(90, 75, 50, 25).forEach { pct ->
                            FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = sizePct == pct, border = phoneChipBorder(sizePct == pct), onClick = { sizePct = pct }, label = { Text("$pct%") })
                        }
                    }
                    Text("Resolution", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 4.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        listOf(100 to "Original", 75 to "75%", 50 to "50%").forEach { (s, label) ->
                            FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = scale == s, border = phoneChipBorder(scale == s), onClick = { scale = s }, label = { Text(label) })
                        }
                    }
                }
                if (askAudio) {
                    Text("Audio", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 8.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        audioOptions.forEach { code ->
                            val on = audioLanguage == code
                            FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = on, border = phoneChipBorder(on),
                                onClick = { audioLanguage = code; languagesChanged = true }, label = { Text(languageName(code) ?: code) })
                        }
                    }
                }
                if (askSubtitles) {
                    Text("Subtitles", style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 4.dp))
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        (listOf(DownloadLanguages.OFF) + subtitleOptions).forEach { code ->
                            val on = subtitleLanguage == code
                            FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = on, border = phoneChipBorder(on),
                                onClick = { subtitleLanguage = code; languagesChanged = true },
                                label = { Text(if (code == DownloadLanguages.OFF) "Off" else languageName(code) ?: code) })
                        }
                    }
                }
                val estimate = estimateText(pending, options, compressed, sizePct, scale, sizeKnown = mode != DownloadMode.COMPAT)
                if (estimate.isNotEmpty()) {
                    Text(estimate, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 8.dp))
                }
                if (DownloadStore.wifiOnly) {
                    Text("Downloads use Wi-Fi only. You can change this in Settings › Downloads.",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(top = 4.dp))
                }
            }
        },
        confirmButton = {
            if (pending.isNotEmpty()) {
                PhoneButton(onClick = {
                    // Changed here: the next download starts with these.
                    if (languagesChanged) {
                        if (askAudio) DownloadStore.lastAudioLanguage = audioLanguage
                        if (askSubtitles) DownloadStore.lastSubtitleLanguage = subtitleLanguage
                    }
                    DownloadStore.add(pending.map { r ->
                        DownloadItem(
                            serverAddress = server.address,
                            account = DownloadStore.accountOf(server.address),
                            sourcePath = r.sourcePath,
                            name = r.name,
                            title = r.title,
                            thumbnailUrl = r.thumbnailUrl,
                            sizePct = if (compressed) sizePct else 100,
                            scale = if (compressed) scale else 100,
                            isDisc = r.isDisc,
                            compat = mode == DownloadMode.COMPAT && !r.isDisc,
                            audioLanguage = if (askAudio) audioLanguage else null,
                            subtitleLanguage = if (askSubtitles) subtitleLanguage else null,
                            bytesTotal = r.size
                        )
                    })
                    DownloadWorker.schedule(context)
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                        ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
                    ) {
                        notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                    onDismiss()
                }) { Text("Download") }
            }
        },
        dismissButton = { PhoneSecondaryButton(onClick = onDismiss) { Text(if (pending.isEmpty()) "OK" else "Cancel") } }
    )
}

@Composable
private fun QualityChoice(selected: Boolean, label: String, detail: String, onClick: () -> Unit) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().selectable(selected = selected, onClick = onClick, role = Role.RadioButton)
    ) {
        RadioButton(selected = selected, onClick = null)
        Spacer(Modifier.width(8.dp))
        Column {
            Text(label, style = MaterialTheme.typography.bodyLarge)
            Text(detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** What Compatibility does with a file: converts only what this phone can't play, at full quality. */
private fun compatDetail(check: com.vargon.weirdstreamer.downloads.OfflinePlayback?): String = when {
    check == null -> "Full quality, converting only what this phone can't play"
    check.plays -> "Plays here as it is: the same as the original"
    else -> listOf(
        if (check.video) "Video kept" else "Video converted to H.264",
        if (check.audio) "audio kept" else "audio converted to stereo AAC"
    ).joinToString(" · ") + ", on the server first"
}

/** "About 1.2 GB · 1280×720", from the original's size and the chosen quality. */
private fun estimateText(
    pending: List<DownloadRequest>, options: DownloadOptions?, compressed: Boolean, sizePct: Int, scale: Int,
    sizeKnown: Boolean = true
): String {
    val total = options?.size ?: pending.sumOf { it.size }
    val parts = mutableListOf<String>()
    when {
        !sizeKnown -> {} // a converted copy's size isn't known ahead
        compressed && total > 0 -> parts += "About ${formatBytes(total * sizePct / 100)}"
        // A disc's full-quality MP4 size isn't known ahead; an original is its size.
        !compressed && pending.none { it.isDisc } && total > 0 -> parts += formatBytes(total)
    }
    options?.takeIf { it.width > 0 && it.height > 0 }?.let { o ->
        val s = if (compressed) scale else 100
        val h = (o.height * s / 100) and 1.inv()
        val w = (o.width * s / 100) and 1.inv()
        parts += "${w}×${h}"
    }
    return parts.joinToString(" · ")
}

/**
 * Settings › Downloads: what's on this phone, what's downloading, and the Wi-Fi setting: the
 * downloads of the server and account open.
 */
@Composable
fun DownloadsTab(viewModel: MediaViewModel) {
    val context = LocalContext.current
    val all by DownloadStore.items.collectAsState()
    val server = viewModel.connectedServer?.address
    val items = remember(all, server) { DownloadStore.mine(all).filter { it.serverAddress == server } }
    var wifiOnly by remember { mutableStateOf(DownloadStore.wifiOnly) }
    var ahead by remember { mutableStateOf(DownloadStore.downloadAhead) }
    var confirm by remember { mutableStateOf<DownloadItem?>(null) }
    val sorted = remember(items) { com.vargon.weirdstreamer.downloads.downloadsInOrder(items) }

    LazyColumn(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        item(key = "wifi") {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth().clickable {
                    wifiOnly = !wifiOnly
                    DownloadStore.wifiOnly = wifiOnly
                    DownloadWorker.schedule(context, restart = true)
                }.padding(horizontal = 16.dp, vertical = 12.dp)
            ) {
                Column(Modifier.weight(1f)) {
                    Text("Download on Wi-Fi only", style = MaterialTheme.typography.bodyLarge)
                    Text(if (wifiOnly) "Downloads wait for Wi-Fi" else "Downloads also use mobile data",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = wifiOnly, onCheckedChange = {
                    wifiOnly = it
                    DownloadStore.wifiOnly = it
                    DownloadWorker.schedule(context, restart = true)
                })
            }
        }
        item(key = "ahead") {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth().clickable {
                    ahead = !ahead
                    DownloadStore.downloadAhead = ahead
                }.padding(horizontal = 16.dp, vertical = 12.dp)
            ) {
                Column(Modifier.weight(1f)) {
                    Text("Download ahead", style = MaterialTheme.typography.bodyLarge)
                    Text(if (ahead) "Watching an episode downloads the next ${DownloadAhead.MIN_EPISODES} to ${DownloadAhead.MAX_EPISODES} " +
                            "(more on a faster connection), removed once you've watched past them"
                        else "Off: only what you download yourself",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = ahead, onCheckedChange = {
                    ahead = it
                    DownloadStore.downloadAhead = it
                })
            }
        }
        item(key = "used") {
            DownloadStorage(items)
            HorizontalDivider(Modifier.padding(top = 8.dp))
        }
        if (items.isEmpty()) {
            item(key = "empty") {
                Text("Nothing downloaded yet. Use the download button next to a file, or ⋮ › Download this folder.",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(16.dp))
            }
        }
        items(sorted, key = { it.id }) { d ->
            DownloadRow(d,
                onPlay = { viewModel.playDownload(d) },
                onRemove = { confirm = d })
        }
    }
    confirm?.let { d -> RemoveDownloadDialog(items.firstOrNull { it.id == d.id } ?: d) { confirm = null } }
}

/**
 * The phone's storage as a bar (downloads, everything else, free) with the totals: what [items]
 * (the account's downloads) take on disk, partial files and subtitles included, and how much of
 * the phone is used. Other accounts' downloads count as everything else.
 */
@Composable
private fun DownloadStorage(items: List<DownloadItem>) {
    val storage = remember(items) {
        val ours = items.sumOf { d ->
            listOfNotNull(d.localFile?.let(::File), d.thumbFile?.let(::File), DownloadStore.partFile(d)).sumOf { it.length() } +
                DownloadStore.subtitleDir(d).walkTopDown().filter { it.isFile }.sumOf { it.length() }
        }
        val stat = android.os.StatFs(DownloadStore.directory.path)
        Triple(ours, stat.totalBytes, stat.availableBytes)
    }
    val (ours, total, free) = storage
    val usedByOthers = (total - free - ours).coerceAtLeast(0)
    val done = items.count { it.state == DownloadItem.State.DONE }
    Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text("Storage", style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
        if (total > 0) {
            Row(
                modifier = Modifier.fillMaxWidth().height(10.dp).clip(RoundedCornerShape(5.dp))
                    .background(MaterialTheme.colorScheme.surfaceVariant)
            ) {
                // Downloads get a sliver even when tiny, so they're visible.
                val oursWeight = if (ours > 0) maxOf(ours.toFloat() / total, 0.01f) else 0f
                val othersWeight = usedByOthers.toFloat() / total
                if (oursWeight > 0f) Box(Modifier.weight(oursWeight).fillMaxHeight().background(MaterialTheme.colorScheme.primary))
                if (othersWeight > 0f) Box(Modifier.weight(othersWeight).fillMaxHeight().background(MaterialTheme.colorScheme.outline))
                val freeWeight = 1f - oursWeight - othersWeight
                if (freeWeight > 0f) Spacer(Modifier.weight(freeWeight))
            }
        }
        StorageLegend(MaterialTheme.colorScheme.primary, "Downloads",
            "${formatBytes(ours).ifEmpty { "0 B" }} · $done ${if (done == 1) "file" else "files"}")
        StorageLegend(MaterialTheme.colorScheme.outline, "Everything else", formatBytes(usedByOthers))
        StorageLegend(MaterialTheme.colorScheme.surfaceVariant, "Free", "${formatBytes(free)} of ${formatBytes(total)}")
    }
}

@Composable
private fun StorageLegend(color: androidx.compose.ui.graphics.Color, label: String, value: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(10.dp).clip(RoundedCornerShape(2.dp)).background(color))
        Spacer(Modifier.width(8.dp))
        Text(label, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
        Text(value, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun DownloadRow(d: DownloadItem, onPlay: () -> Unit, onRemove: () -> Unit) {
    val done = d.state == DownloadItem.State.DONE
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().clickable(enabled = done, onClick = onPlay).padding(horizontal = 16.dp, vertical = 8.dp)
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .width(112.dp)
                .aspectRatio(16f / 9f)
                .clip(RoundedCornerShape(6.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(28.dp))
            val thumb = d.thumbFile?.let { File(it) }?.takeIf { it.exists() } ?: d.thumbnailUrl
            if (thumb != null) {
                AsyncImage(model = thumb, contentDescription = null, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
            }
            if (done) {
                Icon(Icons.Default.PlayArrow, contentDescription = "Play", modifier = Modifier.size(32.dp),
                    tint = androidx.compose.ui.graphics.Color.White)
                // Progress played on this phone (offline or not).
                com.vargon.weirdstreamer.downloads.LocalProgress.get(d.serverAddress, d.sourcePath)?.let { p ->
                    val total = d.durationSeconds
                    WatchProgressOverlay(
                        if (!p.ended && p.position >= 10 && total > 0) (p.position / total).toFloat() else 0f,
                        p.ended, barHeight = 3.dp, badgeSize = 18.dp)
                }
            }
        }
        Spacer(Modifier.width(12.dp))
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            d.title?.let { Text(it, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis) }
            Text(d.name, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            val facts = listOfNotNull(
                if (d.ahead) "Downloaded ahead" else null,
                d.qualityLabel,
                if (d.width > 0 && d.height > 0) "${d.width}×${d.height}" else null,
                if (done) formatBytes(d.bytesTotal) else null
            ).filter { it.isNotEmpty() }
            Text(facts.joinToString(" · "), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            when (d.state) {
                DownloadItem.State.DONE -> {}
                DownloadItem.State.FAILED -> Text(d.error ?: "Download failed", style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.error, maxLines = 3, overflow = TextOverflow.Ellipsis)
                else -> {
                    val status = when (d.state) {
                        DownloadItem.State.QUEUED -> d.stage ?: "Waiting"
                        DownloadItem.State.PREPARING -> "${d.stage ?: "Preparing"} · ${(d.progress * 100).toInt()}%"
                        else -> "Downloading · ${formatBytes(d.bytesDone)} of ${formatBytes(d.bytesTotal)}"
                    }
                    Text(status, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
                    if (d.state == DownloadItem.State.QUEUED) {
                        LinearProgressIndicator(modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
                    } else {
                        LinearProgressIndicator(progress = { d.progress }, modifier = Modifier.fillMaxWidth().padding(top = 4.dp))
                    }
                }
            }
        }
        IconButton(onClick = onRemove) {
            when {
                d.isActive -> Icon(Icons.Default.Close, contentDescription = "Cancel download")
                d.state == DownloadItem.State.FAILED -> Icon(Icons.Default.ErrorOutline, contentDescription = "Retry or remove",
                    tint = MaterialTheme.colorScheme.error)
                else -> Icon(Icons.Default.Delete, contentDescription = "Delete download")
            }
        }
    }
}

/** Downloads every video in the folder being browsed (and its subfolders). */
@Composable
fun DownloadFolderDialog(viewModel: MediaViewModel, onDismiss: () -> Unit) {
    val server = viewModel.connectedServer ?: return onDismiss()
    var found by remember { mutableStateOf<List<DownloadRequest>?>(null) }
    LaunchedEffect(viewModel.currentPath) {
        found = server.videosUnder(viewModel.currentPath).mapNotNull { DownloadRequest.of(it) }
    }
    val requests = found
    when {
        requests == null -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text("Finding videos…") },
            text = { Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) { CircularProgressIndicator() } },
            confirmButton = {},
            dismissButton = { PhoneSecondaryButton(onClick = onDismiss) { Text("Cancel") } }
        )
        requests.isEmpty() -> AlertDialog(
            onDismissRequest = onDismiss,
            title = { Text("No videos here") },
            text = { Text("This folder has no videos to download.") },
            confirmButton = { PhoneButton(onClick = onDismiss) { Text("OK") } }
        )
        else -> DownloadQualityDialog(viewModel, requests, onDismiss)
    }
}
