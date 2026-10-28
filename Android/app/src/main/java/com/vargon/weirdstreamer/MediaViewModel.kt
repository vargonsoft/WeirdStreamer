package com.vargon.weirdstreamer

import android.app.Application
import android.content.Context
import android.util.Base64
import android.util.Log
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.util.UnstableApi
import coil.annotation.ExperimentalCoilApi
import coil.imageLoader
import com.vargon.weirdstreamer.api.ThumbnailUpdateListener
import com.vargon.weirdstreamer.api.models.ContinueItem
import com.vargon.weirdstreamer.api.models.LibraryFile
import com.vargon.weirdstreamer.api.models.Playback
import com.vargon.weirdstreamer.api.models.SyncedPlayback
import com.vargon.weirdstreamer.downloads.DownloadStore
import com.vargon.weirdstreamer.downloads.LocalProgress
import com.vargon.weirdstreamer.api.models.LibraryListing
import com.vargon.weirdstreamer.api.models.TitleDetail
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import com.vargon.weirdstreamer.repository.FileItem
import com.vargon.weirdstreamer.repository.MediaRepository
import com.vargon.weirdstreamer.repository.MediaSource
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.repository.ServerSourceProvider
import com.vargon.weirdstreamer.repository.SmbSourceProvider
import com.vargon.weirdstreamer.repository.SourceProvider
import jcifs.CIFSContext
import jcifs.config.PropertyConfiguration
import jcifs.context.BaseContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.job
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import java.io.File
import java.util.ArrayDeque
import java.util.Properties

enum class ContentFilter {
    ALL, IMAGE, VIDEO, PDF
}

class MediaViewModel(application: Application) : AndroidViewModel(application) {
    val credentialManager = CredentialManager(application)

    val baseContext: CIFSContext by lazy {
        val props = Properties().apply {
            setProperty("jcifs.smb.client.minVersion", "SMB202")
            setProperty("jcifs.smb.client.maxVersion", "SMB311")
            setProperty("jcifs.smb.client.ipcSigningEnforced", "false")
            setProperty("jcifs.netbios.cachePolicy", "0")
            setProperty("jcifs.smb.client.responseTimeout", "60000")
            setProperty("jcifs.smb.client.soTimeout", "65000")
            setProperty("jcifs.smb.client.connTimeout", "10000")
            setProperty("jcifs.smb.client.sessionLimit", "200")
            setProperty("jcifs.smb.client.listCount", "1000")
            setProperty("jcifs.smb.client.maxBufSize", "1048576")
            setProperty("jcifs.smb.client.transactionBuffer", "1048576")
            setProperty("jcifs.smb.client.rcvBufSize", "1048576")
            setProperty("jcifs.smb.client.sndBufSize", "1048576")
            setProperty("jcifs.smb.client.signingEnforced", "false")
            setProperty("jcifs.smb.client.useBatching", "true")
            // Set static hostname to avoid network lookup during BaseContext init
            setProperty("jcifs.netbios.hostname", "WeirdStreamer")
        }
        BaseContext(PropertyConfiguration(props))
    }

    // Abstract source providers - pass baseContext as a lambda to avoid main-thread init. Their
    // order is the source list's and the Add Source dialog's (whose first is the default).
    val sourceProviders: List<SourceProvider> = listOf(
        ServerSourceProvider(credentialManager),
        SmbSourceProvider({ baseContext }, credentialManager)
    )

    var fileList by mutableStateOf<List<FileItem>>(emptyList())
        private set

    private var _sortedFileList by mutableStateOf<List<FileItem>>(emptyList())
    val sortedFileList: List<FileItem>
        get() = _sortedFileList

    private suspend fun updateSortedList() {
        val currentFiles = fileList
        val currentFilter = contentFilter
        val sorted = withContext(Dispatchers.Default) {
            val filtered = when (currentFilter) {
                ContentFilter.ALL -> currentFiles
                ContentFilter.IMAGE -> currentFiles.filter { it.isDirectory || isImage(it.path) }
                ContentFilter.VIDEO -> currentFiles.filter { it.isDirectory || isVideo(it.path) }
                ContentFilter.PDF -> currentFiles.filter { it.isDirectory || isPdf(it.path) }
            }

            filtered.sortedWith { item1, item2 ->
                when {
                    item1.path == "UP" -> -1
                    item2.path == "UP" -> 1
                    item1.isDirectory && !item2.isDirectory -> -1
                    !item1.isDirectory && item2.isDirectory -> 1
                    else -> naturalCompare(item1.name, item2.name)
                }
            }
        }
        _sortedFileList = sorted
    }

    var currentSource by mutableStateOf<MediaSource?>(null)
        private set

    var currentPath by mutableStateOf("")
        private set

    var currentPathDisplayName by mutableStateOf<String?>(null)

    var isLoading by mutableStateOf(false)
        private set

    var errorMessage by mutableStateOf<String?>(null)
        private set

    var authRequiredSource by mutableStateOf<MediaSource?>(null)
        private set

    var showHiddenFiles by mutableStateOf(credentialManager.getShowHiddenFiles())
        private set

    var contentFilter by mutableStateOf(ContentFilter.ALL)
        private set

    var supportsThumbnailRegeneration by mutableStateOf(false)
        private set

    // Used to trigger UI updates when auth status changes (login/logout)
    var authStateRefreshTrigger by mutableIntStateOf(0)
        private set

    private var viewModeMap by mutableStateOf(mapOf<String, Boolean>())

    private val currentViewModeKey: String
        get() {
            val source = currentSource
            return when {
                source == null -> "__source_selection_view__"
                else -> "${source.typeId}_${source.serverAddress}_${currentPath}_view"
            }
        }

    // Read during composition, so avoid hitting EncryptedSharedPreferences more than once per key.
    private val gridViewPrefCache = HashMap<String, Boolean>()

    val isGridView: Boolean
        get() {
            val key = currentViewModeKey
            return viewModeMap[key] ?: gridViewPrefCache.getOrPut(key) { credentialManager.isGridView(key) }
        }

    var selectedVideoPath by mutableStateOf<String?>(null)
        private set

    var selectedImagePath by mutableStateOf<String?>(null)
        private set

    var selectedPdfPath by mutableStateOf<String?>(null)
        private set

    var downloadProgress by mutableStateOf<Float?>(null)
        private set

    private var currentRepository: MediaRepository? = null
    
    private var thumbnailListener: ThumbnailUpdateListener? = null
    private var thumbnailListenerJob: Job? = null

    private val selectionMap = mutableStateMapOf<String, String>()

    // Metadata fetching state to avoid redundant calls
    private val metadataJobs = mutableMapOf<String, Job>()
    private val metadataUnavailablePaths = mutableSetOf<String>()
    private val metadataPermits = Semaphore(METADATA_CONCURRENCY)

    // Per-item updates (metadata, SSE thumbnails) are batched so the list is replaced at most
    // once per flush instead of once per response. Sort order never depends on these fields.
    private val pendingItemUpdates = LinkedHashMap<String, (FileItem) -> FileItem>()
    private var itemUpdateFlushJob: Job? = null

    // Natural sort helper function
    private fun naturalCompare(s1: String, s2: String): Int {
        var i = 0
        var j = 0
        while (i < s1.length && j < s2.length) {
            val c1 = s1[i]
            val c2 = s2[j]

            if (c1.isDigit() && c2.isDigit()) {
                var v1 = 0L
                while (i < s1.length && s1[i].isDigit()) {
                    v1 = v1 * 10 + (s1[i] - '0')
                    i++
                }
                var v2 = 0L
                while (j < s2.length && s2[j].isDigit()) {
                    v2 = v2 * 10 + (s2[j] - '0')
                    j++
                }
                if (v1 != v2) return v1.compareTo(v2)
            } else {
                val res = c1.lowercaseChar().compareTo(c2.lowercaseChar())
                if (res != 0) return res
                i++
                j++
            }
        }
        return s1.length - s2.length
    }

    // ---- Library view: movie/tv libraries as titles instead of folders (server sources) ----

    /** "movie" or "tv" when the current folder is in a library with a type. */
    var libraryType by mutableStateOf<String?>(null)
        private set
    var libraryListing by mutableStateOf<LibraryListing?>(null)
        private set
    var libraryLoading by mutableStateOf(false)
        private set
    var libraryError by mutableStateOf<String?>(null)
        private set
    /** The title page being shown, if any. */
    var openTitle by mutableStateOf<TitleDetail?>(null)
        private set
    var titleLoading by mutableStateOf(false)
        private set
    /**
     * Set by the view toggle so the newly shown view leaves focus on the toolbar; cleared by
     * navigation, after which views take focus as usual.
     */
    var toolbarKeepsFocus = false
        private set
    /** Where the poster grid was when a title was opened, to restore it on the way back. */
    var libraryReturn: LibraryReturn? = null

    private var libraryModeMap by mutableStateOf(mapOf<String, Boolean>())
    private val libraryModeCache = HashMap<String, Boolean>()
    // Files of the title being played from, so the player can find the item and its neighbours
    private var playbackQueue: List<FileItem> = emptyList()

    /** The WeirdStreamer server being browsed, if any (for its admin settings). */
    val connectedServer: ServerRepository?
        get() = serverRepository

    private val serverRepository: ServerRepository?
        get() = currentRepository as? ServerRepository

    val currentLibraryId: String?
        get() = if (libraryType != null) serverRepository?.libraryIdOf(currentPath) else null

    private val libraryModeKey: String
        get() = "${currentSource?.serverAddress}|${currentLibraryId}"

    /** A server's top level: its libraries (or shares), never files, so no content filter. */
    val isServerRoot: Boolean
        get() = currentSource != null && currentRepository?.getParentPath(currentPath) == null

    /** Whether the current library is shown as titles (Library view) rather than folders. */
    val isLibraryView: Boolean
        get() {
            if (libraryType == null) return false
            val key = libraryModeKey
            return libraryModeMap[key] ?: libraryModeCache.getOrPut(key) { credentialManager.isLibraryMode(key) }
        }

    private fun setLibraryMode(enabled: Boolean) {
        val key = libraryModeKey
        libraryModeMap = libraryModeMap + (key to enabled)
        credentialManager.setLibraryMode(key, enabled)
    }

    private fun setGridView(grid: Boolean) {
        val key = currentViewModeKey
        viewModeMap = viewModeMap + (key to grid)
        credentialManager.setGridView(key, grid)
    }

    /** Cycles list -> grid -> library (movie/tv libraries only) -> list. */
    fun toggleViewMode() {
        toolbarKeepsFocus = true
        when {
            isLibraryView -> {
                setLibraryMode(false)
                setGridView(false)
            }
            !isGridView -> setGridView(true)
            libraryType != null -> {
                setLibraryMode(true)
                loadLibrary()
            }
            else -> setGridView(false)
        }
        loadContinueWatching()
    }

    fun loadLibrary() {
        val repo = serverRepository ?: return
        val libraryId = currentLibraryId ?: return
        viewModelScope.launch {
            libraryLoading = true
            libraryError = null
            val result = repo.library(libraryId)
            libraryListing = result.getOrNull()
            libraryError = result.exceptionOrNull()?.let { it.localizedMessage ?: "Failed to load library" }
            libraryLoading = false
        }
        loadContinueWatching()
    }

    fun openLibraryTitle(kind: String, tmdbId: Int) {
        toolbarKeepsFocus = false
        val repo = serverRepository ?: return
        val libraryId = currentLibraryId ?: return
        viewModelScope.launch {
            titleLoading = true
            val result = repo.libraryTitle(libraryId, kind, tmdbId)
            openTitle = result.getOrNull()
            if (result.isFailure) libraryError = result.exceptionOrNull()?.localizedMessage
            titleLoading = false
        }
    }

    fun closeLibraryTitle() {
        openTitle = null
    }

    /** Leaves Library view for the server's list of libraries. */
    fun leaveLibrary() {
        val source = currentSource ?: return
        val repo = currentRepository ?: return
        openTitle = null
        currentPathDisplayName = null
        loadPath(source, repo.getInitialPath())
    }

    /** Server-relative artwork and thumbnail URLs as absolute URLs. */
    fun serverUrl(url: String?): String? = serverRepository?.absoluteUrl(url)

    /** Plays a Library-view file; [queue] (the title's files) becomes the player's previous/next list. */
    /**
     * Reports the player's position in a server file (by its player path) to the server, for its
     * Streams page and the user's progress. A download played from the phone records it here and
     * sends it to its server when it can. [discTitle]: the disc title playing (0: main title).
     */
    fun reportPlayback(path: String, positionMs: Long, paused: Boolean, ended: Boolean = false, discTitle: Int = 0) {
        if (path.startsWith("file://")) {
            val download = DownloadStore.items.value.firstOrNull { it.localFile == path.removePrefix("file://") } ?: return
            LocalProgress.record(download.serverAddress, download.sourcePath, positionMs / 1000.0, ended)
            val repo = serverRepository?.takeIf { it.address == download.serverAddress } ?: return
            viewModelScope.launch { syncLocalProgress(repo) }
            return
        }
        val repo = serverRepository ?: return
        val originalPath = getFileItemForPath(path)?.originalPath ?: return
        viewModelScope.launch { repo.reportPlayback(originalPath, positionMs / 1000.0, paused, ended, discTitle) }
    }

    // ---- Playback progress: resume points, watched marks and Continue Watching ----

    /** Where the player should start the video it's about to open, in ms; it reads this once. */
    var playbackStartMs = 0L
        private set

    /** Videos in progress on the current server, for the Continue Watching row. */
    var continueItems by mutableStateOf<List<ContinueItem>>(emptyList())
        private set

    /** False while the Continue Watching row is being asked for, so the TV can start focus on it. */
    var continueReady by mutableStateOf(true)
        private set

    /** The user's progress in a video by its player path (server results, or offline play). */
    fun playbackFor(path: String): Playback? {
        val item = getFileItemForPath(path) ?: return null
        return newestProgress(item)
    }

    private fun newestProgress(item: FileItem): Playback? {
        val server = item.playback
        val local = serverRepository?.let { repo -> item.originalPath?.let { LocalProgress.get(repo.address, it) } }
        if (local == null || (server != null && server.updatedAt >= local.updatedAt)) return server
        return Playback(
            position = if (local.ended) 0.0 else local.position,
            duration = server?.duration ?: item.duration ?: 0.0,
            watched = local.ended || server?.watched == true,
            updatedAt = local.updatedAt
        )
    }

    /** Where to resume a video, in seconds (0: from the start). */
    fun resumeSeconds(path: String): Double = playbackFor(path)?.resumeAt ?: 0.0

    /** Sends progress in downloads played offline to the server it came from. */
    private suspend fun syncLocalProgress(repo: ServerRepository) {
        val pending = LocalProgress.unsynced(repo.address)
        if (pending.isEmpty()) return
        val items = pending.map {
            SyncedPlayback(Base64.encodeToString(it.sourcePath.toByteArray(), Base64.NO_WRAP), it.position, it.ended, it.updatedAt)
        }
        if (repo.syncPlayback(items).isSuccess) LocalProgress.markSynced(pending)
    }

    fun loadContinueWatching() {
        val repo = serverRepository ?: return
        // Library view shows its own library's; the server's front page shows every library's.
        val libraryId = if (isLibraryView) currentLibraryId else null
        // Whenever the server answers, progress played offline goes to it.
        viewModelScope.launch { syncLocalProgress(repo) }
        if (!isLibraryView && repo.getParentPath(currentPath) != null) {
            continueItems = emptyList()
            continueReady = true
            return
        }
        continueReady = false
        viewModelScope.launch {
            // Offline, the row keeps what it had.
            try {
                repo.continueWatching(libraryId).getOrNull()?.let { continueItems = it }
            } finally {
                continueReady = true
            }
        }
    }

    /** Plays a Continue Watching entry from where it stopped. */
    fun playContinue(item: ContinueItem) {
        val repo = serverRepository ?: return
        val file = repo.fileItemFor(item)
        playbackQueue = listOf(file)
        playVideo(file.path)
    }

    fun removeFromContinue(item: ContinueItem) {
        val repo = serverRepository ?: return
        continueItems = continueItems - item
        viewModelScope.launch { repo.removeFromContinue(item.path) }
    }

    /** Marks videos (server paths) watched or unwatched, then shows it. */
    /** A playing video's intro and credits (by its player path), or null (SMB, downloads, offline). */
    suspend fun segmentsFor(path: String): com.vargon.weirdstreamer.api.models.MediaSegments? {
        val repo = serverRepository ?: return null
        val original = getFileItemForPath(path)?.originalPath ?: return null
        return repo.segments(original)
    }

    /**
     * Skip Credits: this video counts as finished, then the next plays (the queue's, else the
     * show's next episode). False when there's none, for the player to close.
     */
    fun playAfterCredits(path: String, positionMs: Long, next: ContinueItem?): Boolean {
        reportPlayback(path, positionMs, paused = true, ended = true)
        val videos = getVideosInCurrentFolder()
        val i = videos.indexOf(path)
        if (i != -1 && i < videos.size - 1) {
            playVideo(videos[i + 1])
            return true
        }
        if (next != null) {
            playContinue(next)
            return true
        }
        return false
    }

    /** Has the players skip a show's intros and credits by themselves, then shows the change. */
    fun setAutoSkip(tmdbId: Int, on: Boolean) {
        val repo = serverRepository ?: return
        viewModelScope.launch {
            if (repo.setAutoSkip(tmdbId, on).isSuccess) {
                openTitle = openTitle?.takeIf { it.tmdbId == tmdbId }?.copy(autoSkip = on) ?: openTitle
            }
        }
    }

    fun setWatched(originalPaths: List<String>, watched: Boolean) {
        val repo = serverRepository ?: return
        viewModelScope.launch {
            if (repo.setWatched(originalPaths, watched).isSuccess) {
                originalPaths.forEach { LocalProgress.forget(repo.address, it) }
                refreshProgress()
            }
        }
    }

    /**
     * Reloads what's on screen for its new progress, quietly (no spinner, scroll kept): the title
     * page, the library, or the folder, and Continue Watching.
     */
    fun refreshProgress() {
        val repo = serverRepository ?: return
        viewModelScope.launch {
            syncLocalProgress(repo)
            val libraryId = currentLibraryId
            openTitle?.let { t ->
                if (libraryId != null) repo.libraryTitle(libraryId, t.kind, t.tmdbId).getOrNull()?.let { fresh ->
                    if (openTitle?.tmdbId == fresh.tmdbId) openTitle = fresh
                }
            }
            if (isLibraryView && libraryId != null) {
                repo.library(libraryId).getOrNull()?.let { if (libraryListing?.libraryId == it.libraryId) libraryListing = it }
            } else {
                val path = currentPath
                repo.browse(path).getOrNull()?.let { fresh ->
                    if (currentPath != path) return@let
                    val byPath = fresh.associateBy { it.path }
                    fileList = fileList.map { old -> byPath[old.path]?.let { old.copy(playback = it.playback) } ?: old }
                    updateSortedList()
                }
            }
            playbackQueue = playbackQueue.map { q -> fileList.firstOrNull { it.path == q.path } ?: q }
            loadContinueWatching()
        }
    }

    fun playLibraryFile(file: LibraryFile, queue: List<LibraryFile>, fromStart: Boolean = false) {
        val repo = serverRepository ?: return
        playbackQueue = queue.map { repo.fileItemFor(it) }
        playVideo(repo.fileItemFor(file).path, fromStart)
    }


    fun chooseContentFilter(filter: ContentFilter) {
        if (filter == contentFilter) return
        contentFilter = filter
        viewModelScope.launch { updateSortedList() }
    }

    fun toggleContentFilter() {
        contentFilter = when (contentFilter) {
            ContentFilter.ALL -> ContentFilter.IMAGE
            ContentFilter.IMAGE -> ContentFilter.VIDEO
            ContentFilter.VIDEO -> ContentFilter.PDF
            ContentFilter.PDF -> ContentFilter.ALL
        }
        viewModelScope.launch {
            updateSortedList()
        }
    }

    fun getFilteredFileList(): List<FileItem> = sortedFileList

    fun recordSelection(parentPath: String, childPath: String) {
        selectionMap[parentPath] = childPath
    }

    fun getSelection(parentPath: String): String? {
        return selectionMap[parentPath]
    }

    fun updateShowHiddenFiles(show: Boolean) {
        showHiddenFiles = show
        credentialManager.setShowHiddenFiles(show)
        currentSource?.let { loadPath(it, currentPath) }
    }

    @androidx.annotation.OptIn(UnstableApi::class)
    @OptIn(ExperimentalCoilApi::class)
    fun clearThumbnailCache(context: Context) {
        context.imageLoader.diskCache?.clear()
        context.imageLoader.memoryCache?.clear()
        val thumbCacheDir = File(context.cacheDir, "thumbnails")
        if (thumbCacheDir.exists()) thumbCacheDir.deleteRecursively()
        VideoThumbnailDecoder.clearSessionCache()
    }

    fun selectSource(provider: SourceProvider, address: String) {
        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            try {
                val cleanAddress = if (provider.typeId == "SMB") {
                    address.removePrefix("smb://").substringBefore("/").trim()
                } else provider.normalizeAddress(address)
                val source = MediaSource(provider.typeId, cleanAddress)
                // Ensure repository creation (and potential BaseContext init) happens off-main-thread
                val initialPath = withContext(Dispatchers.IO) {
                    provider.createRepository(cleanAddress).getInitialPath()
                }
                loadPath(source, initialPath)
                // If we successfully loaded, trigger a refresh of auth state in UI
                authStateRefreshTrigger++
            } catch (e: Exception) {
                e.printStackTrace()
                errorMessage = e.localizedMessage ?: "Failed to initialize source"
                isLoading = false
            }
        }
    }

    fun loadPath(source: MediaSource, path: String, user: String? = null, pass: String? = null, save: Boolean = false) {
        toolbarKeepsFocus = false
        currentSource = source
        currentPath = path
        metadataJobs.values.forEach { it.cancel() }
        metadataJobs.clear()
        metadataUnavailablePaths.clear()
        discardItemUpdates()

        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            authRequiredSource = null
            authTitle = null
            
            try {
                val provider = sourceProviders.find { it.typeId == source.typeId } ?: throw Exception("Unknown source type")
                val repository = withContext(Dispatchers.IO) { provider.createRepository(source.serverAddress) }
                repository.inheritState(currentRepository)
                currentRepository = repository

                // If we're at the root level (no parent), don't show thumbnail regeneration button
                val parent = repository.getParentPath(path)
                supportsThumbnailRegeneration = parent != null && repository.supportsThumbnailRegeneration()

                if (user != null && pass != null) {
                    val authResult = repository.authenticate(user, pass)
                    if (authResult.isFailure) throw authResult.exceptionOrNull() ?: Exception("Authentication failed")
                    
                    // SMB signs in by browsing with the account's credentials (blank: guest).
                    if (source.typeId == "SMB") {
                        if (user.isBlank()) credentialManager.clearCredentials(source.serverAddress)
                        else credentialManager.saveCredentials(source.serverAddress, user.trim(), pass)
                    }
                    // Trigger UI update for auth status
                    authStateRefreshTrigger++
                }

                // Handle server-specific SSE for thumbnails
                if (repository is ServerRepository) {
                    val token = repository.getToken()
                    if (token != null) startThumbnailUpdates(source.serverAddress, token)
                }

                val result = repository.browse(path)
                // Left (Back to the source list, or another folder) while it loaded: drop it.
                if (currentSource != source || currentPath != path) return@launch
                
                if (result.isSuccess) {
                    val items = result.getOrNull() ?: emptyList()
                    
                    // Auto-save successful servers
                    provider.addServer(source.serverAddress)
                    // An SMB account that signed in joins the server's accounts.
                    if (source.typeId == "SMB" && user != null && pass != null) {
                        credentialManager.addSmbAccount(source.serverAddress, user.trim(), if (user.isBlank()) "" else pass)
                        authStateRefreshTrigger++
                    }

                    // Unified "Share level" logic
                    fileList = if (repository.getParentPath(path) != null || path != repository.getInitialPath()) {
                        listOf(FileItem("..", "UP", true)) + items
                    } else {
                        // At the Share level root, always show .. to go back to Server Selection
                        listOf(FileItem("..", "UP", true)) + items.filter { it.name != ".." }
                    }
                    updateSortedList()
                    val newLibraryType = (repository as? ServerRepository)?.libraryTypeFor(path)
                    if (newLibraryType == null || libraryListing?.libraryId != (repository as ServerRepository).libraryIdOf(path)) {
                        libraryListing = null
                        openTitle = null
                    }
                    libraryType = newLibraryType
                    if (isLibraryView && libraryListing == null) loadLibrary()
                    loadContinueWatching()
                } else {
                    val exception = result.exceptionOrNull() ?: Exception("Unknown error")
                    if (repository.isAuthRequired(exception)) {
                        authRequiredSource = source
                    } else {
                        throw exception
                    }
                }
            } catch (e: Exception) {
                e.printStackTrace()
                if (currentSource == source) errorMessage = e.localizedMessage ?: "Failed to load content"
            } finally {
                isLoading = false
            }
        }
    }

    fun fetchMetadata(item: FileItem) {
        val repo = currentRepository ?: return
        val path = item.path
        if (item.isDirectory || path == "UP" || (item.duration != null && item.duration > 0) ||
            metadataJobs.containsKey(path) || path in metadataUnavailablePaths) return

        val job = viewModelScope.launch(start = CoroutineStart.LAZY) {
            try {
                val result = metadataPermits.withPermit { repo.getMetadata(item) }
                val updatedItem = result.getOrNull()
                if (updatedItem != null) {
                    queueItemUpdate(path) { current -> updatedItem.copy(thumbnailUrl = current.thumbnailUrl, playback = current.playback) }
                } else {
                    metadataUnavailablePaths.add(path)
                }
            } finally {
                if (metadataJobs[path] === coroutineContext.job) metadataJobs.remove(path)
            }
        }
        metadataJobs[path] = job
        job.start()
    }

    /**
     * Fetches missing video metadata for the items currently on screen, and drops queued
     * requests for items that scrolled away.
     */
    fun requestMetadataFor(visibleItems: List<FileItem>) {
        val visiblePaths = visibleItems.mapTo(HashSet()) { it.path }
        metadataJobs.keys.filter { it !in visiblePaths }.forEach { cancelMetadata(it) }
        visibleItems.forEach { item ->
            if (!item.isDirectory && isVideo(item.path)) fetchMetadata(item)
        }
    }

    fun cancelMetadata(path: String) {
        metadataJobs.remove(path)?.cancel()
    }

    private fun queueItemUpdate(path: String, transform: (FileItem) -> FileItem) {
        val previous = pendingItemUpdates[path]
        pendingItemUpdates[path] = if (previous == null) transform else { item -> transform(previous(item)) }
        if (pendingItemUpdates.size >= METADATA_BATCH_SIZE) {
            flushItemUpdates()
        } else if (itemUpdateFlushJob == null) {
            itemUpdateFlushJob = viewModelScope.launch {
                delay(METADATA_FLUSH_INTERVAL_MS)
                itemUpdateFlushJob = null
                flushItemUpdates()
            }
        }
    }

    private fun flushItemUpdates() {
        itemUpdateFlushJob?.cancel()
        itemUpdateFlushJob = null
        if (pendingItemUpdates.isEmpty()) return
        val updates = HashMap(pendingItemUpdates)
        pendingItemUpdates.clear()
        fileList = fileList.map { item -> updates[item.path]?.invoke(item) ?: item }
        _sortedFileList = _sortedFileList.map { item -> updates[item.path]?.invoke(item) ?: item }
    }

    private fun discardItemUpdates() {
        itemUpdateFlushJob?.cancel()
        itemUpdateFlushJob = null
        pendingItemUpdates.clear()
    }

    fun navigateUp() {
        val source = currentSource ?: return
        val repository = currentRepository ?: return
        
        val parent = repository.getParentPath(currentPath)
        if (parent == null) {
            returnToSourceSelection()
        } else {
            // Set display name to last segment of parent path, or "/" if root
            currentPathDisplayName = getDisplayName(parent)
            loadPath(source, parent)
        }
    }

    fun returnToSourceSelection() {
        currentSource = null
        currentPath = ""
        errorMessage = null
        // A sign-in cancelled: else the browser still asks for it and opens the dialog again.
        authRequiredSource = null
        authTitle = null
        fileList = emptyList()
        currentPathDisplayName = null
        supportsThumbnailRegeneration = false
        libraryType = null
        libraryListing = null
        openTitle = null
        discardItemUpdates()
        stopThumbnailUpdates()
    }

    // ---- Accounts: each WeirdStreamer server keeps the accounts signed in to it on this device ----

    /** The username the sign-in dialog starts with: an account whose sign-in expired. */
    var authUsername by mutableStateOf<String?>(null)
        private set

    /** The server opened last, so the source list can return to its account. */
    var lastServer by mutableStateOf<String?>(null)
        private set

    /** The account in use on the current server, for the header. */
    val currentAccount: String?
        get() = currentSource?.let {
            authStateRefreshTrigger.let { _ ->
                when (it.typeId) {
                    "SERVER" -> credentialManager.getServerAccount(it.serverAddress)
                    "SMB" -> accountLabel(credentialManager.getSmbAccount(it.serverAddress))
                    else -> null
                }
            }
        }

    // WeirdStreamer servers' names, as they last said, so a server shows its name when it's offline
    private val serverNames = mutableStateMapOf<String, String>()

    /** The name a WeirdStreamer server gave itself, or null when it has none (or hasn't said). */
    fun serverName(address: String): String? =
        (serverNames[address] ?: credentialManager.getServerName(address))?.takeIf { it.isNotBlank() }

    /** Asks a WeirdStreamer server for its name (no sign-in needed), keeping what it says. */
    fun refreshServerName(address: String) {
        viewModelScope.launch {
            val name = withContext(Dispatchers.IO) {
                runCatching {
                    val request = okhttp3.Request.Builder().url(address.trimEnd('/') + "/api/info").build()
                    com.vargon.weirdstreamer.api.ApiClient.probeClient().newCall(request).execute().use { response ->
                        if (response.isSuccessful) org.json.JSONObject(response.body?.string() ?: "").optString("name").trim() else null
                    }
                }.getOrNull()
            } ?: return@launch
            credentialManager.setServerName(address, name)
            serverNames[address] = name
        }
    }

    /** The header's line for an open server: its name (else its address) and the account in use. */
    fun headerLine(source: MediaSource): String {
        val name = (if (source.typeId == "SERVER") serverName(source.serverAddress) else null) ?: source.displayName()
        return name + (currentAccount?.let { " · $it" } ?: "")
    }

    /** A server's accounts on this device, for its row on the source list and in Settings. */
    fun accountsOf(provider: SourceProvider, address: String): List<CredentialManager.ServerAccount> =
        if (provider.typeId == "SMB") credentialManager.getSmbAccounts(address) else credentialManager.getServerAccounts(address)

    /** The account in use on a server: the one its row focuses. */
    fun accountInUse(provider: SourceProvider, address: String): String? =
        if (provider.typeId == "SMB") credentialManager.getSmbAccount(address) else credentialManager.getServerAccount(address)

    /** Saves a server without signing in to it; its accounts are added on its row. */
    fun addServer(provider: SourceProvider, address: String) {
        viewModelScope.launch {
            val clean = if (provider.typeId == "SMB") address.trim().removePrefix("smb://").substringBefore("/")
                else provider.normalizeAddress(address)
            if (clean.isBlank()) return@launch
            provider.addServer(clean)
            lastServer = clean
            authStateRefreshTrigger++
        }
    }

    /** Opens a server as one of its accounts, or asks for its password when its sign-in has expired. */
    fun chooseServerAccount(provider: SourceProvider, address: String, username: String) {
        lastServer = address
        currentRepository = null // the account's own libraries, not the last account's
        continueItems = emptyList() // nor the last account's videos in progress
        selectionMap.clear() // a fresh start, on Continue Watching
        val ready = if (provider.typeId == "SMB") credentialManager.useSmbAccount(address, username)
            else credentialManager.useServerAccount(address, username)
        if (ready) {
            authStateRefreshTrigger++
            selectSource(provider, address)
        } else askToSignIn(provider, address, username)
    }

    /** Signs another account in to a server, then opens it. */
    fun addServerAccount(provider: SourceProvider, address: String) {
        lastServer = address
        currentRepository = null
        askToSignIn(provider, address, null)
    }

    /** The sign-in dialog's title, when it's for an account chosen on the source list. */
    var authTitle by mutableStateOf<String?>(null)
        private set

    private fun askToSignIn(provider: SourceProvider, address: String, username: String?) {
        authUsername = username
        authTitle = if (username == null) "Add user" else "Sign in again"
        currentPath = if (provider.typeId == "SMB") "smb://$address/" else "" // where signing in opens
        authRequiredSource = MediaSource(provider.typeId, address)
    }

    fun signOutServerAccount(provider: SourceProvider, address: String, username: String) {
        if (provider.typeId == "SMB") credentialManager.signOutSmbAccount(address, username)
        else credentialManager.signOutServerAccount(address, username)
        authStateRefreshTrigger++
    }

    fun removeServer(provider: SourceProvider, address: String) {
        provider.removeServer(address)
        authStateRefreshTrigger++
        if (currentSource?.serverAddress == address && currentSource?.typeId == provider.typeId) {
            returnToSourceSelection()
        }
    }

    // The server file a local copy is playing in place of, to restore the selection afterwards
    private var localPlaybackOf: String? = null

    /**
     * Plays a video, from its downloaded copy when there is one: from where the user stopped,
     * or from the beginning with [fromStart].
     */
    fun playVideo(path: String, fromStart: Boolean = false) {
        playbackStartMs = if (fromStart || path.startsWith("smb://")) 0L else (resumeSeconds(path) * 1000).toLong()
        val local = localCopyOf(path)
        localPlaybackOf = if (local != null) path else null
        selectedVideoPath = local ?: path
    }

    /** A partly watched video picked in folder view: the UI asks Resume or Play from start. */
    var resumeChoice by mutableStateOf<FileItem?>(null)
        private set

    /** The Continue Watching entry the choice was opened from (held on TV), which it can remove. */
    var continueChoice by mutableStateOf<ContinueItem?>(null)
        private set

    /** TV: holding a Continue Watching card asks Resume, Play from start, or Remove. */
    fun openContinueMenu(item: ContinueItem) {
        val repo = serverRepository ?: return
        val file = repo.fileItemFor(item)
        playbackQueue = listOf(file)
        continueChoice = item
        resumeChoice = file
    }

    /** Answers the choice with Remove from Continue Watching. */
    fun removeChoice() {
        continueChoice?.let { removeFromContinue(it) }
        continueChoice = null
        resumeChoice = null
        continueRemovals++
    }

    /** Counts removals from the TV menu, so the screen can put focus back (the card is gone). */
    var continueRemovals by mutableIntStateOf(0)
        private set

    /** Folder view: plays a video, first asking whether to resume when it's partly watched. */
    fun openVideo(item: FileItem) {
        if (resumeSeconds(item.path) > 0) resumeChoice = item else playVideo(item.path)
    }

    /** Answers [resumeChoice]: resume, start over, or (null) cancel. */
    fun chooseResume(fromStart: Boolean?) {
        val item = resumeChoice ?: return
        resumeChoice = null
        continueChoice = null
        if (fromStart != null) playVideo(item.path, fromStart)
    }

    /** Plays a download from the Downloads list, resuming where it was left. */
    fun playDownload(item: com.vargon.weirdstreamer.downloads.DownloadItem) {
        val file = item.localFile ?: return
        // The newest of what the server said (if the file is on screen) and what was played here.
        val known = (fileList + playbackQueue).firstOrNull { it.originalPath == item.sourcePath }
            ?.takeIf { serverRepository?.address == item.serverAddress }
        val resumeAt = known?.let { newestProgress(it)?.resumeAt } ?: LocalProgress.get(item.serverAddress, item.sourcePath)
            ?.takeIf { !it.ended && it.position >= Playback.MIN_RESUME_SECONDS }?.position ?: 0.0
        playbackStartMs = (resumeAt * 1000).toLong()
        localPlaybackOf = null
        selectedVideoPath = "file://$file"
    }

    /** The finished download of a server file (by its player path), as a file:// URL. */
    fun localCopyOf(path: String): String? {
        val repo = serverRepository ?: return null
        val original = getFileItemForPath(path)?.originalPath ?: return null
        return downloadOf(repo.address, original)
    }

    private fun downloadOf(server: String, original: String): String? =
        com.vargon.weirdstreamer.downloads.DownloadStore.find(server, original)
            ?.takeIf { it.state == com.vargon.weirdstreamer.downloads.DownloadItem.State.DONE }
            ?.localFile?.takeIf { java.io.File(it).exists() }?.let { "file://$it" }

    /** The server path of the video played last, so a title page comes back to its season and episode. */
    var lastPlayedFile by mutableStateOf<String?>(null)
        private set

    fun stopPlayback(lastViewedPath: String? = null) {
        val viewed = if (lastViewedPath?.startsWith("file://") == true) localPlaybackOf else lastViewedPath
        viewed?.let { recordSelection(currentPath, it) }
        viewed?.let { v -> lastPlayedFile = (playbackQueue.firstOrNull { it.path == v } ?: getFileItemForPath(v))?.originalPath }
        localPlaybackOf = null
        selectedVideoPath = null
        // Back from the player, the folder selects what was played (not the toolbar, after a view toggle).
        toolbarKeepsFocus = false
        // The player's last report needs a moment to land before the new progress is fetched.
        viewModelScope.launch {
            delay(400)
            refreshProgress()
        }
    }

    fun showImage(path: String) { selectedImagePath = path }
    fun closeImage(lastViewedPath: String? = null) {
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedImagePath = null
    }

    fun showPdf(path: String) { selectedPdfPath = path }
    fun closePdf(lastViewedPath: String? = null) {
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedPdfPath = null
    }

    private fun getActualFilename(path: String): String {
        if (path.contains("/api/media/stream?path=")) {
            val base64Path = path.substringAfter("path=").substringBefore("&")
            return try { String(Base64.decode(base64Path, Base64.NO_WRAP)) } catch (e: Exception) { path }
        }
        return path
    }

    fun getFileExtension(path: String): String {
        return getActualFilename(path).substringBefore('?').removeSuffix("/").substringAfterLast('.', "").lowercase()
    }

    fun isImage(path: String): Boolean = getFileExtension(path) in IMAGE_EXTENSIONS

    fun isVideo(path: String): Boolean = getFileExtension(path) in VIDEO_EXTENSIONS

    fun isPdf(path: String): Boolean {
        val ext = getFileExtension(path)
        return ext == "pdf"
    }

    /**
     * Returns what should be loaded as a thumbnail for [item], or null if nothing should be loaded.
     * - Server-provided thumbnails are always preferred.
     * - SMB images/videos are loaded directly (decoded/extracted by SmbFetcher/VideoThumbnailDecoder).
     * - HTTP images are loaded directly, but HTTP videos are never used as a thumbnail source, since
     *   Coil would download the entire video file just to extract one frame.
     * - Other files (PDFs, archives, ...) never trigger a network request.
     */
    fun thumbnailSourceFor(item: FileItem): String? {
        item.thumbnailUrl?.let { return it }
        if (item.isDirectory || item.path == "UP" || item.name == "..") return null
        val isSmb = item.path.startsWith("smb://")
        return when {
            isImage(item.path) -> item.path
            isSmb && isVideo(item.path) -> item.path
            else -> null
        }
    }

    fun getDisplayName(path: String): String {
        if (path.startsWith("file://")) {
            // A download: what it is, not its file in app storage.
            val file = path.removePrefix("file://")
            com.vargon.weirdstreamer.downloads.DownloadStore.items.value.firstOrNull { it.localFile == file }
                ?.let { return it.title ?: it.name }
        }
        return currentRepository?.getDisplayName(path) ?: path.substringAfterLast('/')
    }

    fun getImagesInCurrentFolder(): List<String> {
        return getFilteredFileList().filter { !it.isDirectory && isImage(it.path) }.map { it.path }
    }

    fun getVideosInCurrentFolder(): List<String> {
        if (playbackQueue.any { it.path == selectedVideoPath }) return playbackQueue.map { it.path }
        return getFilteredFileList().filter { !it.isDirectory && isVideo(it.path) }.map { it.path }
    }

    fun getFileItemForPath(path: String): FileItem? {
        return fileList.find { it.path == path } ?: playbackQueue.find { it.path == path }
    }

    suspend fun fetchTrickplayInfo(id: Long): TrickplayInfo? {
        val repo = currentRepository ?: return null
        if (!repo.supportsTrickplay()) return null
        
        Log.d("MediaViewModel", "Fetching trickplay info for ID: $id")
        val result = repo.getTrickplayInfo(id)
        if (result.isFailure) {
            Log.e("MediaViewModel", "Failed to fetch trickplay info: ${result.exceptionOrNull()?.message}")
        }
        return result.getOrNull()
    }

    suspend fun fetchTrickplayInfo(item: FileItem): TrickplayInfo? {
        return item.id?.let { fetchTrickplayInfo(it) }
    }

    suspend fun downloadToTempFile(path: String, context: Context, suffix: String = ".pdf"): File? = withContext(Dispatchers.IO) {
        try {
            withContext(Dispatchers.Main) { downloadProgress = 0f }
            val repository = currentRepository ?: return@withContext null
            val tempFile = File(context.cacheDir, "temp_${System.currentTimeMillis()}$suffix")
            val result = repository.getInputStream(path)

            if (result.isSuccess) {
                result.getOrNull()?.use { input ->
                    tempFile.outputStream().use { output ->
                        val buffer = ByteArray(64 * 1024)
                        var bytesRead: Int
                        var totalRead = 0L
                        while (input.read(buffer).also { bytesRead = it } != -1) {
                            output.write(buffer, 0, bytesRead)
                            totalRead += bytesRead
                            if (totalRead % (256 * 1024) == 0L) {
                                withContext(Dispatchers.Main) {
                                    downloadProgress = totalRead.toFloat() / 1000000f
                                }
                            }
                        }
                    }
                }
                withContext(Dispatchers.Main) { downloadProgress = null }
                tempFile
            } else {
                withContext(Dispatchers.Main) { downloadProgress = null }
                null
            }
        } catch (e: Exception) {
            withContext(Dispatchers.Main) { downloadProgress = null }
            null
        }
    }
    
    fun startThumbnailUpdates(serverUrl: String, token: String) {
        stopThumbnailUpdates()
        thumbnailListener = ThumbnailUpdateListener(serverUrl, token)
        thumbnailListenerJob = viewModelScope.launch {
            thumbnailListener?.connect()
                ?.catch { e ->
                    Log.e("MediaViewModel", "Thumbnail update flow error: ${e.message}")
                }
                ?.collect { update ->
                    // Library view thumbnails are generated when a title page is first opened.
                    openTitle?.let { title -> openTitle = title.withThumbnail(update.path, update.thumbnailURL) }
                    val target = fileList.firstOrNull { it.originalPath == update.path } ?: return@collect
                    val fullThumbnailUrl = if (update.thumbnailURL.startsWith("http")) update.thumbnailURL else "$serverUrl${update.thumbnailURL}"
                    val thumbnailUrl = "$fullThumbnailUrl?t=${System.currentTimeMillis()}"
                    queueItemUpdate(target.path) { it.copy(thumbnailUrl = thumbnailUrl) }
                }
        }
    }
    
    fun stopThumbnailUpdates() {
        thumbnailListenerJob?.cancel()
        thumbnailListenerJob = null
        thumbnailListener?.disconnect()
        thumbnailListener = null
    }
    
    fun regenerateThumbnails() {
        val repo = currentRepository ?: return
        if (!repo.supportsThumbnailRegeneration() || currentPath.isEmpty()) return
        
        viewModelScope.launch {
            isLoading = true
            val result = repo.regenerateThumbnails(currentPath)
            isLoading = false
            if (result.isFailure) errorMessage = "Failed to regenerate thumbnails: ${result.exceptionOrNull()?.message}"
        }
    }

    override fun onCleared() {
        super.onCleared()
        stopThumbnailUpdates()
    }

    companion object {
        private val NATURAL_TOKEN_REGEX = Regex("(\\d+)|(\\D+)")
        private val IMAGE_EXTENSIONS = setOf("jpg", "jpeg", "png", "webp", "bmp", "gif", "jfif", "heif", "heic")
        private val VIDEO_EXTENSIONS = setOf("mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "3gp", "3gpp", "ts", "mp2", "divx", "xvid", "iso")
        private const val METADATA_BATCH_SIZE = 12
        private const val METADATA_FLUSH_INTERVAL_MS = 500L
        private const val METADATA_CONCURRENCY = 4
    }
}

/** The poster grid's focused title and scroll position in a library. */
data class LibraryReturn(val libraryId: String, val titleKey: String, val index: Int, val offset: Int)

/** [this] with [path]'s thumbnail set, or [this] itself when [path] isn't one of its files. */
private fun TitleDetail.withThumbnail(path: String, thumbnailUrl: String): TitleDetail {
    var changed = false
    fun List<LibraryFile>.update() = map { file ->
        if (file.path == path && file.thumbnailUrl != thumbnailUrl) {
            changed = true
            file.copy(thumbnailUrl = thumbnailUrl)
        } else file
    }
    val updated = copy(
        files = files?.update(),
        otherFiles = otherFiles?.update(),
        seasons = seasons?.map { season ->
            season.copy(episodes = season.episodes.map { it.copy(files = it.files.update()) })
        }
    )
    return if (changed) updated else this
}

/** How an account is shown: its username, or Guest for SMB guest access. */
fun accountLabel(username: String) = username.ifEmpty { "Guest" }
