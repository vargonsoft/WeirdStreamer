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
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

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
        Log.d("WeirdStreamerApp", "Application created")
    }

    override fun newImageLoader(): ImageLoader {
        Log.d("WeirdStreamerApp", "Creating new ImageLoader")
        val credentialManager = CredentialManager(this)
        
        // Create OkHttp client with SSL bypass and auth for server thumbnails
        val trustAllCerts = arrayOf<TrustManager>(object : X509TrustManager {
            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
            override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
            override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
        })
        
        val sslContext = SSLContext.getInstance("SSL")
        sslContext.init(null, trustAllCerts, java.security.SecureRandom())
        val sslSocketFactory = sslContext.socketFactory
        
        okHttpClient = OkHttpClient.Builder()
            .sslSocketFactory(sslSocketFactory, trustAllCerts[0] as X509TrustManager)
            .hostnameVerifier { _, _ -> true }
            .addInterceptor { chain ->
                val request = chain.request()
                val url = request.url.toString()
                
                // Server API, thumbnail and trickplay requests all need the auth header
                val serverPath = SERVER_AUTH_PATHS.firstOrNull { url.contains(it) }
                if (serverPath != null) {
                    val serverUrl = url.substringBefore(serverPath)
                    val token = credentialManager.getServerToken(serverUrl)
                    
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
