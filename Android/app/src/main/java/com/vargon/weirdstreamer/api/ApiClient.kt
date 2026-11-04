package com.vargon.weirdstreamer.api

import com.google.gson.GsonBuilder
import com.vargon.weirdstreamer.BuildConfig
import com.vargon.weirdstreamer.CredentialManager
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.gson.GsonConverterFactory
import java.security.cert.X509Certificate
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

object ApiClient {
    
    private val loggingInterceptor = HttpLoggingInterceptor().apply {
        level = if (BuildConfig.DEBUG) HttpLoggingInterceptor.Level.BASIC else HttpLoggingInterceptor.Level.NONE
    }

    // One client per server, so browsing reuses connections instead of a new pool and TLS
    // handshake for every folder.
    private val services = ConcurrentHashMap<String, ApiService>()
    
    // WARNING: This trust manager accepts ALL certificates - use only for development/testing!
    private val trustAllCerts = arrayOf<TrustManager>(object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
    })
    
    private fun getUnsafeOkHttpClient(baseClient: OkHttpClient.Builder): OkHttpClient.Builder {
        return try {
            val sslContext = SSLContext.getInstance("SSL")
            sslContext.init(null, trustAllCerts, java.security.SecureRandom())
            val sslSocketFactory = sslContext.socketFactory
            
            baseClient
                .sslSocketFactory(sslSocketFactory, trustAllCerts[0] as X509TrustManager)
                .hostnameVerifier { _, _ -> true }
        } catch (e: Exception) {
            throw RuntimeException(e)
        }
    }
    
    /** A short-timeout client with the API client's TLS settings, for probing a server address. */
    fun probeClient(): OkHttpClient = getUnsafeOkHttpClient(
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
        getUnsafeOkHttpClient(
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
        
        // Apply unsafe SSL settings to accept self-signed certificates
        // WARNING: Only for development! Remove for production builds
        val okHttpClient = getUnsafeOkHttpClient(okHttpClientBuilder).build()
        
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
