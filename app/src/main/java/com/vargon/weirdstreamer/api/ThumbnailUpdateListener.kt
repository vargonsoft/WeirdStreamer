package com.vargon.weirdstreamer.api

import android.os.Handler
import android.os.Looper
import android.util.Log
import com.google.gson.Gson
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager
import kotlin.math.min

data class ThumbnailUpdate(
    val path: String,
    val thumbnailURL: String
)

class ThumbnailUpdateListener(
    private val serverUrl: String,
    private val token: String
) {
    private var eventSource: EventSource? = null
    private val gson = Gson()
    private val maxRetries = 5
    private val baseDelay = 1000L // 1 second
    private val handler = Handler(Looper.getMainLooper())
    
    // WARNING: This trust manager accepts ALL certificates - use only for development/testing!
    private val trustAllCerts = arrayOf<TrustManager>(object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
    })
    
    private fun getUnsafeOkHttpClient(): OkHttpClient {
        return try {
            val sslContext = SSLContext.getInstance("SSL")
            sslContext.init(null, trustAllCerts, java.security.SecureRandom())
            val sslSocketFactory = sslContext.socketFactory
            
            OkHttpClient.Builder()
                .sslSocketFactory(sslSocketFactory, trustAllCerts[0] as X509TrustManager)
                .hostnameVerifier { _, _ -> true }
                .connectTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
                .readTimeout(0, java.util.concurrent.TimeUnit.SECONDS) // No timeout for SSE
                .writeTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
                .build()
        } catch (e: Exception) {
            throw RuntimeException(e)
        }
    }
    
    fun connect(): Flow<ThumbnailUpdate> = callbackFlow {
        val client = getUnsafeOkHttpClient()
        var retryCount = 0
        var shouldRetry = true
        
        fun attemptConnection() {
            val request = Request.Builder()
                .url("$serverUrl/api/media/thumbnail-updates?token=$token")
                .build()
            
            val listener = object : EventSourceListener() {
                override fun onOpen(eventSource: EventSource, response: okhttp3.Response) {
                    Log.d("ThumbnailSSE", "Connection opened")
                    retryCount = 0 // Reset retry count on successful connection
                }
                
                override fun onEvent(
                    eventSource: EventSource,
                    id: String?,
                    type: String?,
                    data: String
                ) {
                    Log.d("ThumbnailSSE", "Event received: $data")
                    try {
                        val update = gson.fromJson(data, ThumbnailUpdate::class.java)
                        trySend(update)
                    } catch (e: Exception) {
                        Log.e("ThumbnailSSE", "Failed to parse event", e)
                    }
                }
                
                override fun onClosed(eventSource: EventSource) {
                    Log.d("ThumbnailSSE", "Connection closed")
                    shouldRetry = false
                    channel.close()
                }
                
                override fun onFailure(
                    eventSource: EventSource,
                    t: Throwable?,
                    response: okhttp3.Response?
                ) {
                    Log.e("ThumbnailSSE", "Connection failed (retry $retryCount/$maxRetries)", t)
                    
                    if (retryCount < maxRetries && shouldRetry) {
                        retryCount++
                        val delayMs = min(baseDelay * (1 shl (retryCount - 1)), 30000L) // Max 30s
                        Log.d("ThumbnailSSE", "Reconnecting in ${delayMs}ms...")
                        
                        // Schedule reconnection
                        handler.postDelayed({
                            if (shouldRetry && !channel.isClosedForSend) {
                                eventSource?.cancel()
                                attemptConnection()
                            }
                        }, delayMs)
                    } else {
                        Log.e("ThumbnailSSE", "Max retries reached or channel closed")
                        channel.close(t)
                    }
                }
            }
            
            eventSource?.cancel()
            eventSource = EventSources.createFactory(client).newEventSource(request, listener)
        }
        
        // Initial connection
        attemptConnection()
        
        awaitClose {
            Log.d("ThumbnailSSE", "Closing connection")
            shouldRetry = false
            handler.removeCallbacksAndMessages(null)
            eventSource?.cancel()
        }
    }
    
    fun disconnect() {
        eventSource?.cancel()
        eventSource = null
    }
}
