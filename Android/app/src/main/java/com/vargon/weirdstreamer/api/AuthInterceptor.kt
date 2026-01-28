package com.vargon.weirdstreamer.api

import com.vargon.weirdstreamer.CredentialManager
import okhttp3.Interceptor
import okhttp3.Response

class AuthInterceptor(
    private val credentialManager: CredentialManager,
    private val serverUrl: String
) : Interceptor {
    
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        
        // Skip adding token for login endpoint
        if (request.url.encodedPath.endsWith("/api/auth/login")) {
            return chain.proceed(request)
        }
        
        // Get token from credential manager
        val token = credentialManager.getServerToken(serverUrl)
        
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
