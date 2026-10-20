package com.vargon.weirdstreamer.api

import com.vargon.weirdstreamer.api.models.Directory
import com.vargon.weirdstreamer.api.models.DiscInfo
import com.vargon.weirdstreamer.api.models.LibraryListing
import com.vargon.weirdstreamer.api.models.TitleDetail
import com.vargon.weirdstreamer.api.models.LoginRequest
import com.vargon.weirdstreamer.api.models.LoginResponse
import com.vargon.weirdstreamer.api.models.MediaItem
import com.vargon.weirdstreamer.api.models.StreamProgress
import com.vargon.weirdstreamer.api.models.TrickplayInfo
import okhttp3.ResponseBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
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
    
    @POST("/api/media/regenerate-thumbnails")
    suspend fun regenerateThumbnails(@Query("path") path: String): Response<Map<String, String>>

    @GET("/api/media/trickplay/{id}")
    suspend fun getTrickplayInfo(@Path("id") id: Long): Response<TrickplayInfo>

    @GET("/api/media/disc")
    suspend fun getDisc(@Query("path") path: String): Response<DiscInfo>

    @POST("/api/streams/progress")
    suspend fun reportStreamProgress(@Body progress: StreamProgress): Response<Unit>

    @GET("/api/library")
    suspend fun getLibrary(@Query("lib") libraryId: String): Response<LibraryListing>

    @GET("/api/library/title")
    suspend fun getLibraryTitle(
        @Query("lib") libraryId: String,
        @Query("kind") kind: String,
        @Query("id") tmdbId: Int
    ): Response<TitleDetail>
}
