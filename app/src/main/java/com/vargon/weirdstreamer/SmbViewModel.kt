package com.vargon.weirdstreamer

import android.app.Application
import android.content.Context
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import coil.imageLoader
import jcifs.CIFSContext
import jcifs.config.PropertyConfiguration
import jcifs.context.BaseContext
import jcifs.smb.NtlmPasswordAuthenticator
import jcifs.smb.SmbAuthException
import jcifs.smb.SmbFile
import jcifs.smb.SmbUnsupportedOperationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.Properties

class SmbViewModel(application: Application) : AndroidViewModel(application) {
    private val credentialManager = CredentialManager(application)
    
    var fileList by mutableStateOf<List<SmbFileItem>>(emptyList())
        private set

    var currentPath by mutableStateOf("smb://")
        private set

    var isLoading by mutableStateOf(false)
        private set

    var errorMessage by mutableStateOf<String?>(null)
        private set

    var authRequiredHost by mutableStateOf<String?>(null)
        private set

    var allHosts by mutableStateOf<List<String>>(emptyList())
        private set

    var savedServers by mutableStateOf<List<String>>(emptyList())
        private set

    var showHiddenFiles by mutableStateOf(credentialManager.getShowHiddenFiles())
        private set

    // Track view mode per path: path -> isGridView
    private var viewModeMap by mutableStateOf(mapOf<String, Boolean>())

    val isGridView: Boolean
        get() = viewModeMap[currentPath] ?: credentialManager.isGridView(currentPath)

    fun toggleViewMode() {
        val current = isGridView
        val newState = !current
        viewModeMap = viewModeMap + (currentPath to newState)
        credentialManager.setGridView(currentPath, newState)
    }

    private var discoveredHosts = emptyList<String>()
    
    // Selection tracking: parentPath -> childPath or childName
    private val selectionMap = mutableMapOf<String, String>()

    private val baseContext: CIFSContext by lazy {
        val props = Properties().apply {
            setProperty("jcifs.smb.client.minVersion", "SMB202")
            setProperty("jcifs.smb.client.maxVersion", "SMB311")
            setProperty("jcifs.smb.client.ipcSigningEnforced", "false")
            setProperty("jcifs.netbios.cachePolicy", "0")
        }
        BaseContext(PropertyConfiguration(props))
    }

    init {
        refreshHosts()
        restoreLastPath()
    }

    private fun restoreLastPath() {
        val lastPath = credentialManager.getLastPath()
        if (!lastPath.isNullOrEmpty() && lastPath != "smb://") {
            loadPath(lastPath, isAutoRestore = true)
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
        if (currentPath != "smb://") {
            loadPath(currentPath)
        }
    }

    fun deleteServer(host: String) {
        credentialManager.removeSavedServer(host)
        refreshHosts()
    }

    fun clearThumbnailCache(context: Context) {
        context.imageLoader.diskCache?.clear()
        context.imageLoader.memoryCache?.clear()
    }

    fun refreshHosts() {
        val saved = credentialManager.getSavedServers()
        savedServers = saved.toList().sorted()
        allHosts = (saved + discoveredHosts).distinct().sorted()
        discoverHosts()
    }

    private fun discoverHosts() {
        viewModelScope.launch {
            try {
                val found = withContext(Dispatchers.IO) {
                    val context = baseContext.withCredentials(NtlmPasswordAuthenticator(null, null, null))
                    SmbFile("smb://", context).use { workgroup ->
                        workgroup.listFiles().flatMap { server ->
                            try {
                                server.listFiles().map { it.name.removeSuffix("/") }
                            } catch (e: Exception) {
                                emptyList()
                            }
                        }
                    }
                }
                discoveredHosts = found
                val saved = credentialManager.getSavedServers()
                allHosts = (saved + discoveredHosts).distinct().sorted()
            } catch (e: SmbUnsupportedOperationException) {
                // Expected on SMB2+ networks.
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
    }

    fun loadPath(path: String, user: String? = null, pass: String? = null, save: Boolean = false, isAutoRestore: Boolean = false) {
        val normalizedPath = if (path.startsWith("smb://")) path else "smb://$path"
        val finalPath = if (normalizedPath.endsWith("/") || normalizedPath.contains("?")) normalizedPath else "$normalizedPath/"
        val host = finalPath.removePrefix("smb://").substringBefore("/")

        viewModelScope.launch {
            isLoading = true
            errorMessage = null
            try {
                val list = withContext(Dispatchers.IO) {
                    val creds = if (user != null && pass != null) {
                        if (save) credentialManager.saveCredentials(host, user, pass)
                        Pair(user, pass)
                    } else {
                        credentialManager.getCredentials(host)
                    }

                    val auth = if (creds.first != null && creds.second != null) {
                        NtlmPasswordAuthenticator(null, creds.first, creds.second)
                    } else {
                        NtlmPasswordAuthenticator(null, null, null)
                    }
                    
                    val context = baseContext.withCredentials(auth)
                    SmbFile(finalPath, context).use { smbFile ->
                        val remoteItems = smbFile.listFiles()
                            .filter { showHiddenFiles || (!it.isHidden && !it.name.startsWith(".")) }
                            .map {
                                SmbFileItem(
                                    name = it.name.removeSuffix("/"),
                                    path = it.canonicalPath,
                                    isDirectory = it.isDirectory,
                                    size = if (it.isDirectory) 0 else it.length()
                                )
                            }.sortedWith(compareBy({ !it.isDirectory }, { it.name.lowercase() }))
                        
                        if (finalPath != "smb://") {
                            listOf(SmbFileItem("..", "UP", true)) + remoteItems
                        } else {
                            remoteItems
                        }
                    }
                }
                
                credentialManager.addSavedServer(host)
                credentialManager.saveLastPath(finalPath)
                
                fileList = list
                currentPath = finalPath
                authRequiredHost = null
            } catch (e: SmbAuthException) {
                authRequiredHost = host
            } catch (e: Exception) {
                if (isAutoRestore) {
                    currentPath = "smb://"
                    refreshHosts()
                } else {
                    e.printStackTrace()
                    errorMessage = e.localizedMessage ?: "Failed to connect"
                }
            } finally {
                isLoading = false
            }
        }
    }

    fun navigateUp() {
        if (currentPath == "smb://") return
        
        val pathWithoutTrailing = currentPath.removeSuffix("/")
        if (pathWithoutTrailing.count { it == '/' } <= 2) {
            currentPath = "smb://"
            fileList = emptyList()
            refreshHosts()
            return
        }
        
        val parent = pathWithoutTrailing.substringBeforeLast("/") + "/"
        loadPath(parent)
    }
}
