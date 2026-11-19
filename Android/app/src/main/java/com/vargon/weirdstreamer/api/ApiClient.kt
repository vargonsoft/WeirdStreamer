package com.vargon.weirdstreamer.api

import com.google.gson.GsonBuilder
import com.vargon.weirdstreamer.BuildConfig
import com.vargon.weirdstreamer.CredentialManager
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

object ApiClient {
    
    private val loggingInterceptor = HttpLoggingInterceptor().apply {
        level = if (BuildConfig.DEBUG) HttpLoggingInterceptor.Level.BASIC else HttpLoggingInterceptor.Level.NONE
    }

    // One client per server, so browsing reuses connections instead of a new pool and TLS
    // handshake for every folder.
    private val services = ConcurrentHashMap<String, ApiService>()
    
    /**
     * A short-timeout client for checking an address before it's saved: any certificate, which
     * [ServerTrust.keepOnAdd] keeps when the server is added.
     */
    fun probeClient(): OkHttpClient = ServerTrust.probing(
        OkHttpClient.Builder()
            .connectTimeout(3, java.util.concurrent.TimeUnit.SECONDS)
            .readTimeout(3, java.util.concurrent.TimeUnit.SECONDS)
            .followRedirects(false)
    ).build()

    /** A short-timeout client for saved servers (their name, updates), with [ServerTrust]'s rules. */
    fun quickClient(): OkHttpClient = ServerTrust.secure(
        OkHttpClient.Builder()
            .connectTimeout(3, java.util.concurrent.TimeUnit.SECONDS)
            .readTimeout(3, java.util.concurrent.TimeUnit.SECONDS)
            .followRedirects(false)
    ).build()

    /**
     * An HTTP client that signs in to [serverUrl], for large file downloads (no read timeout): as
     * [account] when given, else the account in use.
     */
    fun downloadClient(serverUrl: String, credentialManager: CredentialManager, account: String? = null): OkHttpClient =
        ServerTrust.secure(
            OkHttpClient.Builder()
                .addInterceptor(AuthInterceptor(credentialManager, serverUrl, account))
                .connectTimeout(30, TimeUnit.SECONDS)
                .readTimeout(2, TimeUnit.MINUTES)
        ).build()

    /** The API of [serverUrl], signed in as [account] when given, else as the account in use. */
    fun create(
        serverUrl: String,
        credentialManager: CredentialManager,
        account: String? = null
    ): ApiService = services.getOrPut(if (account == null) serverUrl else "$serverUrl|@$account") {
        build(serverUrl, credentialManager, account)
    }

    private fun build(
        serverUrl: String,
        credentialManager: CredentialManager,
        account: String?
    ): ApiService {
        val authInterceptor = AuthInterceptor(credentialManager, serverUrl, account)
        
        val okHttpClientBuilder = OkHttpClient.Builder()
            .addInterceptor(authInterceptor)
            .addInterceptor(loggingInterceptor)
            .connectTimeout(30, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
        
        val okHttpClient = ServerTrust.secure(okHttpClientBuilder).build()
        
        val gson = GsonBuilder()
            .setLenient()
            .create()
        
        val retrofit = Retrofit.Builder()
            .baseUrl(ensureTrailingSlash(serverUrl))
            .client(okHttpClient)
            .addConverterFactory(GsonConverterFactory.create(gson))
            .build()
        
        return retrofit.create(ApiService::class.java)
    }
    
    private fun ensureTrailingSlash(url: String): String {
        return if (url.endsWith("/")) url else "$url/"
    }
}
