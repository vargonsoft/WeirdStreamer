package com.vargon.weirdstreamer.api

import com.vargon.weirdstreamer.CredentialManager
import okhttp3.Interceptor
import okhttp3.Response

class AuthInterceptor(
    private val credentialManager: CredentialManager,
    private val serverUrl: String,
    private val account: String? = null // signs in as this account, not the one in use
) : Interceptor {
    
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        
        // Skip adding token for signing in (password, or a TV's code)
        val path = request.url.encodedPath
        if (path.endsWith("/api/auth/login") || path.contains("/api/auth/pairing")) {
            return chain.proceed(request)
        }
        
        // Get token from credential manager
        val token = if (account != null) credentialManager.tokenOf(serverUrl, account) else credentialManager.getServerToken(serverUrl)
        
        if (token != null) {
            // Add Authorization header with Bearer token
            val authenticatedRequest = request.newBuilder()
                .header("Authorization", "Bearer $token")
                .build()
            return chain.proceed(authenticatedRequest)
        }
        
        return chain.proceed(request)
    }
}
