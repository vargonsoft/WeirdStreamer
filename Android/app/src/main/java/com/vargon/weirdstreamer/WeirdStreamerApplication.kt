package com.vargon.weirdstreamer

import android.app.Application
import android.util.Log
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.decode.GifDecoder
import coil.decode.VideoFrameDecoder
import coil.decode.SvgDecoder
import coil.disk.DiskCache
import coil.key.Keyer
import coil.memory.MemoryCache
import coil.request.Options
import okhttp3.OkHttpClient

class SmbKeyer : Keyer<String> {
    override fun key(data: String, options: Options): String? {
        if (!data.startsWith("smb://")) return null
        // Strip credentials for a stable cache key: smb://user:pass@host/path -> smb://host/path
        return data.replaceFirst(Regex("smb://.*@"), "smb://")
    }
}

class WeirdStreamerApplication : Application(), ImageLoaderFactory {

    companion object {
        private var instance: WeirdStreamerApplication? = null
        private val SERVER_AUTH_PATHS = listOf("/api/", "/thumbnails/", "/trickplay/", "/artwork/")

        fun getOkHttpClient(): OkHttpClient {
            return instance?.okHttpClient ?: throw IllegalStateException("Application not initialized")
        }
    }

    private lateinit var okHttpClient: OkHttpClient

    override fun onCreate() {
        super.onCreate()
        instance = this
        // Photos and PDFs a viewer downloaded (temp_), and motion clips (clip_, temp_video_ before):
        // only in use while a viewer is open, so any left from before are stale.
        Thread {
            cacheDir.listFiles { f -> f.isFile && (f.name.startsWith("temp_") || f.name.startsWith("clip_")) }?.forEach { it.delete() }
        }.start()
        com.vargon.weirdstreamer.downloads.DownloadStore.init(this)
        com.vargon.weirdstreamer.downloads.LocalProgress.init(this)
        val accounts = CredentialManager(this)
        com.vargon.weirdstreamer.api.ServerTrust.init(this) { accounts.getApiServers() }
        com.vargon.weirdstreamer.downloads.LocalProgress.accountOf = { accounts.accountInUse(it) }
        com.vargon.weirdstreamer.downloads.DownloadStore.accountOf = { accounts.accountInUse(it) }
        // Downloads to finish, or finished ones due to save their subtitles, skip times and trickplay frames.
        if (com.vargon.weirdstreamer.downloads.DownloadStore.items.value.any {
                it.isActive || com.vargon.weirdstreamer.downloads.DownloadStore.needsExtras(it)
            }) {
            com.vargon.weirdstreamer.downloads.DownloadWorker.schedule(this)
        }
        Log.d("WeirdStreamerApp", "Application created")
    }

    override fun newImageLoader(): ImageLoader {
        Log.d("WeirdStreamerApp", "Creating new ImageLoader")
        val credentialManager = CredentialManager(this)
        
        // Server thumbnails, artwork and streams need the auth header; certificates as ServerTrust says.
        okHttpClient = com.vargon.weirdstreamer.api.ServerTrust.secure(OkHttpClient.Builder())
            .addInterceptor { chain ->
                val request = chain.request()
                val url = request.url.toString()
                
                // Server API, thumbnail and trickplay requests all need the auth header
                val serverPath = SERVER_AUTH_PATHS.firstOrNull { url.contains(it) }
                if (serverPath != null) {
                    val serverUrl = url.substringBefore(serverPath)
                    val token = credentialManager.getServerToken(serverUrl)
                        ?: credentialManager.serverTokenForUrl(request.url)
                    
                    if (token != null) {
                        val authenticatedRequest = request.newBuilder()
                            .header("Authorization", "Bearer $token")
                            .build()
                        chain.proceed(authenticatedRequest)
                    } else {
                        chain.proceed(request)
                    }
                } else {
                    chain.proceed(request)
                }
            }
            .build()
        
        return ImageLoader.Builder(this)
            .okHttpClient(okHttpClient)
            // Opaque thumbnails don't need ARGB_8888; halves bitmap memory (fewer GCs on TV).
            .allowRgb565(true)
            .crossfade(false)
            .memoryCache {
                MemoryCache.Builder(this)
                    .maxSizePercent(0.25)
                    .build()
            }
            .diskCache {
                DiskCache.Builder()
                    .directory(this.cacheDir.resolve("image_cache"))
                    .maxSizePercent(0.05) // Increased to 5% for better thumbnail persistence
                    .build()
            }
            .components {
                add(SmbKeyer())
                add(VideoThumbnailDecoder.Factory())
                add(VideoFrameDecoder.Factory())
                add(GifDecoder.Factory())
                add(SvgDecoder.Factory())
                add(SmbFetcher.Factory(credentialManager))
            }
            .build()
    }
}
