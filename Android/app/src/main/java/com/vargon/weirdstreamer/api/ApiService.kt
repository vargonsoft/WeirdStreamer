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
import com.google.gson.JsonObject
import com.vargon.weirdstreamer.api.models.AdminStream
import com.vargon.weirdstreamer.api.models.CreateDownloadRequest
import com.vargon.weirdstreamer.api.models.DownloadJob
import com.vargon.weirdstreamer.api.models.DownloadOptions
import com.vargon.weirdstreamer.api.models.ChangePasswordRequest
import com.vargon.weirdstreamer.api.models.CreateUserRequest
import com.vargon.weirdstreamer.api.models.ServerUser
import okhttp3.ResponseBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.PUT
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

    // Server administration (admins), as in the web UI's Settings. The config is kept as raw
    // JSON, so fields this app doesn't edit go back to the server untouched.
    @GET("/api/config")
    suspend fun getConfig(): Response<JsonObject>

    @PUT("/api/config")
    suspend fun updateConfig(@Body config: JsonObject): Response<JsonObject>

    @GET("/api/groups")
    suspend fun getGroups(): Response<List<String>>

    @GET("/api/users")
    suspend fun getUsers(): Response<List<ServerUser>>

    @POST("/api/users")
    suspend fun createUser(@Body request: CreateUserRequest): Response<JsonObject>

    @PUT("/api/users")
    suspend fun updateUser(@Query("username") username: String, @Body changes: JsonObject): Response<JsonObject>

    @DELETE("/api/users")
    suspend fun deleteUser(@Query("username") username: String): Response<JsonObject>

    @POST("/api/users/change-password")
    suspend fun changePassword(@Body request: ChangePasswordRequest): Response<JsonObject>

    @POST("/api/media/scan")
    suspend fun scanMedia(): Response<JsonObject>

    @GET("/api/admin/streams")
    suspend fun getStreams(): Response<List<AdminStream>>

    // Offline downloads: originals come from /api/media/stream; compressed copies are encoded
    // by a server job first and fetched from /api/downloads/{id}/file.
    @GET("/api/downloads/options")
    suspend fun downloadOptions(@Query("path") path: String): Response<DownloadOptions>

    @POST("/api/downloads")
    suspend fun createDownload(@Body request: CreateDownloadRequest): Response<DownloadJob>

    @GET("/api/downloads/{id}")
    suspend fun getDownload(@Path("id") id: String): Response<DownloadJob>

    @DELETE("/api/downloads/{id}")
    suspend fun deleteDownload(@Path("id") id: String): Response<Unit>

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
