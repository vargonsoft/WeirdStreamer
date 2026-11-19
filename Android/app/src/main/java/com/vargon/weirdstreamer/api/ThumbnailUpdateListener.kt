package com.vargon.weirdstreamer.api

import android.util.Log
import com.google.gson.Gson
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
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
    private var reconnectScope: CoroutineScope? = null
    private val gson = Gson()
    private val maxRetries = 5
    private val baseDelay = 1000L // 1 second
    
    private fun client(): OkHttpClient = ServerTrust.secure(
        OkHttpClient.Builder()
            .connectTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
            .readTimeout(0, java.util.concurrent.TimeUnit.SECONDS) // No timeout for SSE
            .writeTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
    ).build()

    fun connect(): Flow<ThumbnailUpdate> = callbackFlow {
        val client = client()
        var retryCount = 0
        var isClosing = false
        
        fun attemptConnection() {
            if (isClosing) return

            val request = Request.Builder()
                .url("$serverUrl/api/media/thumbnail-updates")
                .header("Authorization", "Bearer $token")
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
                    // Thumbnail updates are unnamed events; named ones (e.g. "metadata") have other payloads.
                    if (type != null && type != "message") return
                    try {
                        val update = gson.fromJson(data, ThumbnailUpdate::class.java)
                        if (update?.path != null && update.thumbnailURL != null) trySend(update)
                    } catch (e: Exception) {
                        Log.e("ThumbnailSSE", "Failed to parse event", e)
                    }
                }
                
                override fun onClosed(eventSource: EventSource) {
                    Log.d("ThumbnailSSE", "Connection closed")
                    // If not closing intentionally, treat as failure to trigger retry
                    if (!isClosing) {
                        handleFailure(eventSource, null)
                    } else {
                        channel.close()
                    }
                }
                
                override fun onFailure(
                    eventSource: EventSource,
                    t: Throwable?,
                    response: okhttp3.Response?
                ) {
                    if (isClosing) {
                        Log.d("ThumbnailSSE", "Failure during shutdown: ${t?.message}")
                        channel.close()
                        return
                    }
                    handleFailure(eventSource, t)
                }

                private fun handleFailure(eventSource: EventSource, t: Throwable?) {
                    Log.e("ThumbnailSSE", "Connection failure (retry $retryCount/$maxRetries): ${t?.message}")
                    
                    // Don't retry if the channel is already closed
                    if (channel.isClosedForSend || isClosing) return

                    if (retryCount < maxRetries) {
                        retryCount++
                        val delayMs = min(baseDelay * (1 shl (retryCount - 1)), 30000L)
                        Log.d("ThumbnailSSE", "Reconnecting in ${delayMs}ms...")

                        reconnectScope?.launch {
                            delay(delayMs)
                            if (!isClosing && !channel.isClosedForSend) {
                                attemptConnection()
                            }
                        }
                    } else {
                        Log.e("ThumbnailSSE", "Max retries reached")
                        channel.close(t)
                    }
                }
            }
            
            // Cancel previous event source if any
            val oldSource = this@ThumbnailUpdateListener.eventSource
            this@ThumbnailUpdateListener.eventSource = null
            oldSource?.cancel()
            
            this@ThumbnailUpdateListener.eventSource = EventSources.createFactory(client).newEventSource(request, listener)
        }
        
        reconnectScope = CoroutineScope(Dispatchers.IO + SupervisorJob())

        // Initial connection
        attemptConnection()
        
        awaitClose {
            Log.d("ThumbnailSSE", "Closing connection in awaitClose")
            isClosing = true
            reconnectScope?.cancel()
            reconnectScope = null
            val toCancel = eventSource
            eventSource = null
            toCancel?.cancel()
        }
    }
    
    fun disconnect() {
        reconnectScope?.cancel()
        reconnectScope = null
        eventSource?.cancel()
        eventSource = null
    }
}
