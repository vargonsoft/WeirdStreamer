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

class SmbKeyer : Keyer<String> {
    override fun key(data: String, options: Options): String? {
        if (!data.startsWith("smb://")) return null
        // Strip credentials for a stable cache key: smb://user:pass@host/path -> smb://host/path
        return data.replaceFirst(Regex("smb://.*@"), "smb://")
    }
}

class WeirdStreamerApplication : Application(), ImageLoaderFactory {
    override fun onCreate() {
        super.onCreate()
        Log.d("WeirdStreamerApp", "Application created")
    }

    override fun newImageLoader(): ImageLoader {
        Log.d("WeirdStreamerApp", "Creating new ImageLoader")
        val credentialManager = CredentialManager(this)
        return ImageLoader.Builder(this)
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
