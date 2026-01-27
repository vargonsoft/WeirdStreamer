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
import androidx.media3.common.util.UnstableApi
import coil.annotation.ExperimentalCoilApi
import coil.imageLoader
import com.vargon.weirdstreamer.repository.FileItem
import com.vargon.weirdstreamer.repository.MediaRepository
import com.vargon.weirdstreamer.repository.MediaSource
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.repository.SmbRepository
import jcifs.CIFSContext
import jcifs.config.PropertyConfiguration
import jcifs.context.BaseContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbFile
import jcifs.smb.SmbUnsupportedOperationException
import jcifs.smb.SmbAuthException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import java.util.Properties

class MediaViewModel(application: Application) : AndroidViewModel(application) {
    val credentialManager = CredentialManager(application)
    
    var fileList by mutableStateOf<List<FileItem>>(emptyList())
        private set

    var currentSource by mutableStateOf<MediaSource?>(null)
        private set

    var currentPath by mutableStateOf("")
        private set

    var isLoading by mutableStateOf(false)
        private set

    var errorMessage by mutableStateOf<String?>(null)
        private set

    var authRequiredHost by mutableStateOf<String?>(null)
        private set

    var authRequiredServerUrl by mutableStateOf<String?>(null)

    var smbHosts by mutableStateOf<List<String>>(emptyList())
        private set

    var apiServers by mutableStateOf<List<String>>(emptyList())
        private set

    var showHiddenFiles by mutableStateOf(credentialManager.getShowHiddenFiles())
        private set

    private var viewModeMap by mutableStateOf(mapOf<String, Boolean>())

    val isGridView: Boolean
        get() = viewModeMap[currentPath] ?: credentialManager.isGridView(currentPath)

    var selectedVideoPath by mutableStateOf<String?>(null)
        private set

    var selectedImagePath by mutableStateOf<String?>(null)
        private set

    var selectedPdfPath by mutableStateOf<String?>(null)
        private set

    var downloadProgress by mutableStateOf<Float?>(null)
        private set

    private var currentRepository: MediaRepository? = null

    fun toggleViewMode() {
        val current = isGridView
        val newState = !current
        viewModeMap = viewModeMap + (currentPath to newState)
        credentialManager.setGridView(currentPath, newState)
    }

    private var discoveredHosts = emptyList<String>()
    
    private val selectionMap = mutableStateMapOf<String, String>()

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
        }
        BaseContext(PropertyConfiguration(props))
    }

    init {
        refreshSources()
    }

    fun recordSelection(parentPath: String, childPath: String) {
        android.util.Log.d("MediaViewModel", "recordSelection - parent: $parentPath, child: $childPath")
        selectionMap[parentPath] = childPath
    }

    fun getSelection(parentPath: String): String? {
        val selection = selectionMap[parentPath]
        android.util.Log.d("MediaViewModel", "getSelection - parent: $parentPath, selection: $selection")
        return selection
    }

    fun updateShowHiddenFiles(show: Boolean) {
        showHiddenFiles = show
        credentialManager.setShowHiddenFiles(show)
        currentSource?.let { loadPath(it, currentPath) }
    }

    fun deleteServer(host: String) {
        credentialManager.removeSavedServer(host)
        refreshSources()
    }

    fun deleteApiServer(serverUrl: String) {
        credentialManager.removeApiServer(serverUrl)
        refreshSources()
    }

    @androidx.annotation.OptIn(UnstableApi::class)
    @OptIn(ExperimentalCoilApi::class)
    fun clearThumbnailCache(context: Context) {
        context.imageLoader.diskCache?.clear()
        context.imageLoader.memoryCache?.clear()

        val thumbCacheDir = File(context.cacheDir, "thumbnails")
        if (thumbCacheDir.exists()) {
            thumbCacheDir.deleteRecursively()
        }

        VideoThumbnailDecoder.clearSessionCache()
    }

    fun refreshSources() {
        val savedSmb = credentialManager.getSavedServers()
        val savedApi = credentialManager.getApiServers()
        android.util.Log.d("MediaViewModel", "refreshSources - SMB: $savedSmb, API: $savedApi")
        smbHosts = (savedSmb + discoveredHosts).distinct().sorted()
        apiServers = savedApi.toList().sorted()
        android.util.Log.d("MediaViewModel", "refreshSources - smbHosts: $smbHosts, apiServers: $apiServers")
        discoverHosts()
    }

    private fun discoverHosts() {
        viewModelScope.launch {
            try {
                val found = withContext(Dispatchers.IO) {
                    val context = baseContext.withCredentials(NtlmPasswordAuthenticator(null, null, null))
                    SmbFile("smb://", context).use { workgroup ->
                        workgroup.listFiles().flatMap { server ->
                            server.use { s ->
                                try {
                                    s.listFiles().map { 
                                        val name = it.name.removeSuffix("/")
                                        it.close()
                                        name
                                    }
                                } catch (_: Exception) {
                                    emptyList()
                                }
                            }
                        }
                    }
                }
                discoveredHosts = found
                val saved = credentialManager.getSavedServers()
                smbHosts = (saved + discoveredHosts).distinct().sorted()
            } catch (_: SmbUnsupportedOperationException) {
                // Expected
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
    }

    fun selectSmbSource(host: String) {
        // Extract just the hostname, removing any path components or smb:// prefix
        val cleanHost = host.removePrefix("smb://").substringBefore("/").trim()
        android.util.Log.d("MediaViewModel", "selectSmbSource - input: '$host', cleaned: '$cleanHost'")
        val source = MediaSource.Smb(cleanHost, "", "")
        loadPath(source, "smb://$cleanHost/")
    }

    fun selectServerSource(serverUrl: String, username: String? = null, password: String? = null) {
        val source = MediaSource.Server(serverUrl, "")
        loadPath(source, "", username, password, save = false)
    }

    fun loadPath(source: MediaSource, path: String, user: String? = null, pass: String? = null, save: Boolean = false) {
        currentSource = source
        currentPath = path

        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            authRequiredHost = null
            
            try {
                val repository: MediaRepository = when (source) {
                    is MediaSource.Smb -> {
                        if (user != null && pass != null && save) {
                            val host = source.host
                            credentialManager.saveCredentials(host, user, pass)
                            credentialManager.addSavedServer(host)
                        }
                        SmbRepository(baseContext, credentialManager, showHiddenFiles)
                    }
                    is MediaSource.Server -> {
                        val repo = ServerRepository(source.serverUrl, credentialManager)
                        if (user != null && pass != null) {
                            val result = repo.authenticate(user, pass)
                            if (result.isFailure) {
                                throw result.exceptionOrNull() ?: Exception("Authentication failed")
                            }
                        }
                        repo
                    }
                }

                // Store the repository for later use
                currentRepository = repository

                val browsePath = when (source) {
                    is MediaSource.Smb -> {
                        val normalizedPath = if (path.startsWith("smb://")) path else "smb://$path"
                        if (normalizedPath.endsWith("/") || normalizedPath.contains("?")) normalizedPath else "$normalizedPath/"
                    }
                    is MediaSource.Server -> path
                }

                val result = repository.browse(browsePath)
                
                if (result.isSuccess) {
                    val items = result.getOrNull() ?: emptyList()
                    
                    // Save SMB host to saved servers list after successful browse
                    if (source is MediaSource.Smb) {
                        val host = browsePath.removePrefix("smb://").substringBefore("/")
                        android.util.Log.d("MediaViewModel", "Saving SMB host after successful browse: $host from path: $browsePath")
                        credentialManager.addSavedServer(host)
                    }

                    // Add parent directory entry for non-root paths
                    fileList = if (shouldShowParentDir(source, path)) {
                        listOf(FileItem("..", "UP", true)) + items
                    } else {
                        items
                    }
                    
                    // Refresh sources list to show newly added servers
                    if (source is MediaSource.Server) {
                        refreshSources()
                    }
                } else {
                    val exception = result.exceptionOrNull()
                    if (exception is SmbAuthException) {
                        authRequiredHost = (source as? MediaSource.Smb)?.host
                    } else if (source is MediaSource.Server && exception?.message?.contains("401") == true) {
                        // Server returned 401 - authentication required
                        authRequiredServerUrl = source.serverUrl
                    } else {
                        throw exception ?: Exception("Unknown error")
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

    private fun shouldShowParentDir(source: MediaSource, path: String): Boolean {
        return when (source) {
            is MediaSource.Smb -> {
                val pathWithoutTrailing = path.removeSuffix("/")
                pathWithoutTrailing.count { it == '/' } > 2
            }
            is MediaSource.Server -> path.isNotEmpty()
        }
    }

    fun navigateUp() {
        val source = currentSource ?: return
        
        when (source) {
            is MediaSource.Smb -> {
                if (currentPath == "smb://") return
                
                val pathWithoutTrailing = currentPath.removeSuffix("/")
                if (pathWithoutTrailing.count { it == '/' } <= 2) {
                    returnToSourceSelection()
                } else {
                    val parent = pathWithoutTrailing.substringBeforeLast("/") + "/"
                    loadPath(source, parent)
                }
            }
            is MediaSource.Server -> {
                if (currentPath.isEmpty()) {
                    returnToSourceSelection()
                } else {
                    val parent = currentPath.substringBeforeLast("/", "")
                    loadPath(source, parent)
                }
            }
        }
    }

    fun returnToSourceSelection() {
        android.util.Log.d("MediaViewModel", "returnToSourceSelection - clearing source, path, error, and file list")
        currentSource = null
        currentPath = ""
        errorMessage = null
        fileList = emptyList()
        refreshSources()
    }

    fun playVideo(path: String) {
        selectedVideoPath = path
    }

    fun stopPlayback(lastViewedPath: String? = null) {
        android.util.Log.d("MediaViewModel", "stopPlayback - lastViewedPath: $lastViewedPath, currentPath: $currentPath")
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedVideoPath = null
    }

    fun showImage(path: String) {
        android.util.Log.d("MediaViewModel", "showImage called with path: $path")
        selectedImagePath = path
        android.util.Log.d("MediaViewModel", "selectedImagePath set to: $selectedImagePath")
    }

    fun closeImage(lastViewedPath: String? = null) {
        android.util.Log.d("MediaViewModel", "closeImage - lastViewedPath: $lastViewedPath, currentPath: $currentPath")
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedImagePath = null
    }

    fun showPdf(path: String) {
        selectedPdfPath = path
    }

    fun closePdf(lastViewedPath: String? = null) {
        android.util.Log.d("MediaViewModel", "closePdf - lastViewedPath: $lastViewedPath, currentPath: $currentPath")
        lastViewedPath?.let { recordSelection(currentPath, it) }
        selectedPdfPath = null
    }

    private fun getActualFilename(path: String): String {
        // If it's a server URL with base64-encoded path, decode it
        if (path.contains("/api/media/stream?path=")) {
            val base64Path = path.substringAfter("path=")
            return try {
                String(Base64.decode(base64Path, Base64.NO_WRAP))
            } catch (e: Exception) {
                path
            }
        }
        return path
    }

    fun isImage(path: String): Boolean {
        val actualPath = getActualFilename(path)
        val cleanPath = actualPath.substringBefore('?').removeSuffix("/")
        val ext = cleanPath.substringAfterLast('.', "").lowercase()
        return ext in setOf("jpg", "jpeg", "png", "webp", "bmp", "gif", "jfif", "heif", "heic")
    }

    fun isVideo(path: String): Boolean {
        val actualPath = getActualFilename(path)
        val cleanPath = actualPath.substringBefore('?').removeSuffix("/")
        val ext = cleanPath.substringAfterLast('.', "").lowercase()
        val result = ext in setOf("mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "3gp", "3gpp")
        android.util.Log.d("MediaViewModel", "isVideo - path: $path, actualPath: $actualPath, ext: $ext, result: $result")
        return result
    }

    fun isPdf(path: String): Boolean {
        val actualPath = getActualFilename(path)
        val cleanPath = actualPath.substringBefore('?').removeSuffix("/")
        val ext = cleanPath.substringAfterLast('.', "").lowercase()
        return ext == "pdf"
    }

    fun getImagesInCurrentFolder(): List<String> {
        return fileList.filter { !it.isDirectory && isImage(it.path) }.map { it.path }
    }

    fun getDisplayName(path: String): String {
        return currentRepository?.getDisplayName(path) ?: path.substringAfterLast('/')
    }

    suspend fun downloadToTempFile(path: String, context: Context, suffix: String = ".pdf"): File? = withContext(Dispatchers.IO) {
        try {
            downloadProgress = 0f
            val repository = currentRepository

            if (repository == null) {
                android.util.Log.e("MediaViewModel", "downloadToTempFile - no repository set")
                downloadProgress = null
                return@withContext null
            }

            android.util.Log.d("MediaViewModel", "downloadToTempFile - path: $path, repository: ${repository::class.simpleName}")

            val tempFile = File(context.cacheDir, "temp_${System.currentTimeMillis()}$suffix")
            val result = repository.getInputStream(path)

            if (result.isSuccess) {
                val inputStream = result.getOrNull()
                if (inputStream == null) {
                    android.util.Log.e("MediaViewModel", "downloadToTempFile - inputStream is null despite success")
                    downloadProgress = null
                    return@withContext null
                }

                android.util.Log.d("MediaViewModel", "downloadToTempFile - downloading to ${tempFile.absolutePath}")

                inputStream.use { input ->
                    tempFile.outputStream().use { output ->
                        val buffer = ByteArray(64 * 1024)
                        var bytesRead: Int
                        var totalRead = 0L

                        while (input.read(buffer).also { bytesRead = it } != -1) {
                            output.write(buffer, 0, bytesRead)
                            totalRead += bytesRead
                            downloadProgress = totalRead.toFloat() / 1000000f // Approximate progress
                        }
                    }
                }

                android.util.Log.d("MediaViewModel", "downloadToTempFile - success, file size: ${tempFile.length()} bytes")
                downloadProgress = null
                tempFile
            } else {
                val error = result.exceptionOrNull()
                android.util.Log.e("MediaViewModel", "downloadToTempFile - failed: ${error?.message}", error)
                downloadProgress = null
                null
            }
        } catch (e: Exception) {
            android.util.Log.e("MediaViewModel", "downloadToTempFile - exception", e)
            downloadProgress = null
            null
        }
    }
}
