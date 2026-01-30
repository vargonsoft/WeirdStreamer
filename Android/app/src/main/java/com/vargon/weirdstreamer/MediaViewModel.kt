package com.vargon.weirdstreamer

import android.app.Application
import android.content.Context
import android.util.Base64
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import androidx.media3.common.MediaItem
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import coil.annotation.ExperimentalCoilApi
import coil.imageLoader
import com.vargon.weirdstreamer.api.ThumbnailUpdateListener
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

    // --- ExoPlayer management for phone video playback ---
    private var exoPlayer: ExoPlayer? = null
    private var exoPlayerPath: String? = null
    private var exoPlayerContext: Context? = null

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
    }

    fun getFilteredFileList(): List<FileItem> {
        return when (contentFilter) {
            ContentFilter.ALL -> fileList
            ContentFilter.IMAGE -> fileList.filter { (!it.isDirectory && isImage(it.path)) }
            ContentFilter.VIDEO -> fileList.filter { (!it.isDirectory && isVideo(it.path)) }
            ContentFilter.PDF -> fileList.filter { (!it.isDirectory && isPdf(it.path)) }
        }
    }

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

                if (user != null && pass != null) {
                    val authResult = repository.authenticate(user, pass)
                    if (authResult.isFailure) throw authResult.exceptionOrNull() ?: Exception("Authentication failed")
                    
                    if (save && source.typeId == "SMB") {
                        credentialManager.saveCredentials(source.serverAddress, user, pass)
                    }
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
        if (item.isDirectory || item.path == "UP" || item.duration != null || pendingMetadataPaths.contains(item.path)) return
        
        pendingMetadataPaths.add(item.path)
        viewModelScope.launch {
            val result = repo.getMetadata(item)
            if (result.isSuccess) {
                val updatedItem = result.getOrNull()
                if (updatedItem != null) {
                    fileList = fileList.map {
                        if (it.path == item.path) updatedItem else it 
                    }
                }
            }
            pendingMetadataPaths.remove(item.path)
        }
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
        stopThumbnailUpdates()
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

    fun isImage(path: String): Boolean {
        val ext = getActualFilename(path).substringBefore('?').removeSuffix("/").substringAfterLast('.', "").lowercase()
        return ext in setOf("jpg", "jpeg", "png", "webp", "bmp", "gif", "jfif", "heif", "heic")
    }

    fun isVideo(path: String): Boolean {
        val ext = getActualFilename(path).substringBefore('?').removeSuffix("/").substringAfterLast('.', "").lowercase()
        return ext in setOf("mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "3gp", "3gpp", "ts", "mp2")
    }

    fun isPdf(path: String): Boolean {
        val ext = getActualFilename(path).substringBefore('?').removeSuffix("/").substringAfterLast('.', "").lowercase()
        return ext == "pdf"
    }

    fun getDisplayName(path: String): String {
        return currentRepository?.getDisplayName(path) ?: path.substringAfterLast('/')
    }

    fun getImagesInCurrentFolder(): List<String> {
        return fileList.filter { !it.isDirectory && isImage(it.path) }.map { it.path }
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
            thumbnailListener?.connect()?.collect { update ->
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
    
    // --- ExoPlayer management for phone video playback ---
    @androidx.annotation.OptIn(UnstableApi::class)
    fun getOrCreateExoPlayer(context: Context, path: String): ExoPlayer {
        if (exoPlayer == null || exoPlayerPath != path || exoPlayerContext != context) {
            exoPlayer?.release()
            val isSmbSource = path.startsWith("smb://")
            val builder = ExoPlayer.Builder(context)
            if (isSmbSource) {
                builder.setMediaSourceFactory(
                    DefaultMediaSourceFactory(context)
                        .setDataSourceFactory(SmbDataSourceFactory(credentialManager, baseContext))
                )
            } else {
                val okHttpClient = WeirdStreamerApplication.getOkHttpClient()
                val okHttpDataSourceFactory = OkHttpDataSource.Factory(okHttpClient)
                    .setUserAgent("WeirdStreamer/1.0")
                builder.setMediaSourceFactory(
                    DefaultMediaSourceFactory(context)
                        .setDataSourceFactory(okHttpDataSourceFactory)
                )
            }
            exoPlayer = builder.build().apply {
                setMediaItem(MediaItem.fromUri(path))
                prepare()
                playWhenReady = true
            }
            exoPlayerPath = path
            exoPlayerContext = context
        }
        return exoPlayer!!
    }

    fun releaseExoPlayer() {
        exoPlayer?.release()
        exoPlayer = null
        exoPlayerPath = null
        exoPlayerContext = null
    }

    override fun onCleared() {
        super.onCleared()
        stopThumbnailUpdates()
        releaseExoPlayer()
    }
}
