package com.vargon.weirdstreamer

import android.app.Application
import android.util.Log
import coil.ImageLoader
import coil.ImageLoaderFactory
import coil.decode.GifDecoder
import coil.decode.VideoFrameDecoder
import coil.decode.SvgDecoder

class WeirdStreamerApplication : Application(), ImageLoaderFactory {
    override fun onCreate() {
        super.onCreate()
        Log.d("WeirdStreamerApp", "Application created")
    }

    override fun newImageLoader(): ImageLoader {
        Log.d("WeirdStreamerApp", "Creating new ImageLoader")
        val credentialManager = CredentialManager(this)
        return ImageLoader.Builder(this)
            .components {
                add(VideoFrameDecoder.Factory())
                add(GifDecoder.Factory())
                add(SvgDecoder.Factory())
                add(SmbFetcher.Factory(credentialManager))
            }
            .build()
    }
}
