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
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
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

    // Abstract source providers - pass baseContext as a lambda to avoid main-thread init
    val sourceProviders: List<SourceProvider> = listOf(
        SmbSourceProvider({ baseContext }, credentialManager),
        ServerSourceProvider(credentialManager)
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

    val isGridView: Boolean
        get() = viewModeMap[currentViewModeKey] ?: credentialManager.isGridView(currentViewModeKey)

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
    private val pendingMetadataPaths = mutableSetOf<String>()
    private val metadataJobs = mutableMapOf<String, Job>()

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

    fun toggleViewMode() {
        val key = currentViewModeKey
        val newState = !isGridView
        viewModeMap = viewModeMap + (key to newState)
        credentialManager.setGridView(key, newState)
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
        val cleanAddress = if (provider.typeId == "SMB") {
            address.removePrefix("smb://").substringBefore("/").trim()
        } else address
        
        val source = MediaSource(provider.typeId, cleanAddress)
        
        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            try {
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
        currentSource = source
        currentPath = path
        metadataJobs.values.forEach { it.cancel() }
        metadataJobs.clear()
        pendingMetadataPaths.clear()

        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            authRequiredSource = null
            
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
                    
                    if (save && source.typeId == "SMB") {
                        credentialManager.saveCredentials(source.serverAddress, user, pass)
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
                
                if (result.isSuccess) {
                    val items = result.getOrNull() ?: emptyList()
                    
                    // Auto-save successful servers
                    provider.addServer(source.serverAddress)

                    // Unified "Share level" logic
                    fileList = if (repository.getParentPath(path) != null || path != repository.getInitialPath()) {
                        listOf(FileItem("..", "UP", true)) + items
                    } else {
                        // At the Share level root, always show .. to go back to Server Selection
                        listOf(FileItem("..", "UP", true)) + items.filter { it.name != ".." }
                    }
                    updateSortedList()
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
                errorMessage = e.localizedMessage ?: "Failed to load content"
            } finally {
                isLoading = false
            }
        }
    }

    fun fetchMetadata(item: FileItem) {
        val repo = currentRepository ?: return
        if (item.isDirectory || item.path == "UP" || (item.duration != null && item.duration > 0) || pendingMetadataPaths.contains(item.path)) return
        
        pendingMetadataPaths.add(item.path)
        val job = viewModelScope.launch {
            try {
                val result = repo.getMetadata(item)
                if (result.isSuccess) {
                    val updatedItem = result.getOrNull()
                    if (updatedItem != null) {
                        fileList = fileList.map {
                            if (it.path == item.path) updatedItem else it
                        }
                        viewModelScope.launch {
                            updateSortedList()
                        }
                    }
                }
            } finally {
                pendingMetadataPaths.remove(item.path)
                metadataJobs.remove(item.path)
            }
        }
        metadataJobs[item.path] = job
    }

    fun cancelMetadata(path: String) {
        metadataJobs.remove(path)?.cancel()
        pendingMetadataPaths.remove(path)
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
        fileList = emptyList()
        currentPathDisplayName = null
        supportsThumbnailRegeneration = false
        stopThumbnailUpdates()
    }

    fun logout(serverUrl: String) {
        credentialManager.clearServerToken(serverUrl)
        authStateRefreshTrigger++
        if (currentSource?.serverAddress == serverUrl && currentSource?.typeId == "SERVER") {
            returnToSourceSelection()
        }
    }

    fun removeServer(provider: SourceProvider, address: String) {
        provider.removeServer(address)
        authStateRefreshTrigger++
        if (currentSource?.serverAddress == address && currentSource?.typeId == provider.typeId) {
            returnToSourceSelection()
        }
    }

    fun playVideo(path: String) { selectedVideoPath = path }
    fun stopPlayback(lastViewedPath: String? = null) {
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedVideoPath = null
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

    fun isImage(path: String): Boolean {
        val ext = getFileExtension(path)
        return ext in setOf("jpg", "jpeg", "png", "webp", "bmp", "gif", "jfif", "heif", "heic")
    }

    fun isVideo(path: String): Boolean {
        val ext = getFileExtension(path)
        return ext in setOf("mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "3gp", "3gpp", "ts", "mp2")
    }

    fun isPdf(path: String): Boolean {
        val ext = getFileExtension(path)
        return ext == "pdf"
    }

    fun getDisplayName(path: String): String {
        return currentRepository?.getDisplayName(path) ?: path.substringAfterLast('/')
    }

    fun getImagesInCurrentFolder(): List<String> {
        return getFilteredFileList().filter { !it.isDirectory && isImage(it.path) }.map { it.path }
    }

    fun getVideosInCurrentFolder(): List<String> {
        return getFilteredFileList().filter { !it.isDirectory && isVideo(it.path) }.map { it.path }
    }

    fun getFileItemForPath(path: String): FileItem? {
        return fileList.find { it.path == path }
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
            downloadProgress = 0f
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
                            downloadProgress = totalRead.toFloat() / 1000000f
                        }
                    }
                }
                downloadProgress = null
                tempFile
            } else {
                downloadProgress = null
                null
            }
        } catch (e: Exception) {
            downloadProgress = null
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
                    fileList = fileList.map { item ->
                        if (item.originalPath == update.path) {
                            val fullThumbnailUrl = if (update.thumbnailURL.startsWith("http")) update.thumbnailURL else "$serverUrl${update.thumbnailURL}"
                            item.copy(thumbnailUrl = "$fullThumbnailUrl?t=${System.currentTimeMillis()}")
                        } else item
                    }
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
}
