package com.vargon.weirdstreamer.api

import com.vargon.weirdstreamer.api.models.Directory
import com.vargon.weirdstreamer.api.models.LoginRequest
import com.vargon.weirdstreamer.api.models.LoginResponse
import com.vargon.weirdstreamer.api.models.MediaItem
import okhttp3.ResponseBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Query
import retrofit2.http.Streaming

interface ApiService {
    
    @POST("/api/auth/login")
    suspend fun login(@Body request: LoginRequest): Response<LoginResponse>
    
    @GET("/api/media")
    suspend fun getMedia(): Response<List<MediaItem>>
    
    @GET("/api/media/browse")
    suspend fun browse(@Query("path") path: String): Response<Directory>
    
    @GET("/api/media/metadata")
    suspend fun getMetadata(@Query("path") path: String): Response<MediaItem>
    
    @GET("/api/media/stream")
    suspend fun stream(@Query("path") path: String): Response<ResponseBody>

    @GET("/api/media/stream")
    @Streaming
    suspend fun streamFile(@Query("path") path: String): Response<ResponseBody>
}
