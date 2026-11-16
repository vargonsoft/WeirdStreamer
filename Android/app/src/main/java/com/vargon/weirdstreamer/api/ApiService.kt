package com.vargon.weirdstreamer.api

import com.vargon.weirdstreamer.api.models.JobsResponse
import com.vargon.weirdstreamer.api.models.Directory
import com.vargon.weirdstreamer.api.models.DiscInfo
import com.vargon.weirdstreamer.api.models.MediaSegments
import com.vargon.weirdstreamer.api.models.AutoSkipRequest
import com.vargon.weirdstreamer.api.models.CodecsInfo
import com.vargon.weirdstreamer.api.models.SeekPoint
import com.vargon.weirdstreamer.api.models.SubtitleSearch
import com.vargon.weirdstreamer.api.models.SubtitleDownload
import com.vargon.weirdstreamer.api.models.SubtitleDownloadRequest
import com.vargon.weirdstreamer.api.models.LibraryListing
import com.vargon.weirdstreamer.api.models.TitleDetail
import com.vargon.weirdstreamer.api.models.LoginRequest
import com.vargon.weirdstreamer.api.models.LoginResponse
import com.vargon.weirdstreamer.api.models.PairingClaim
import com.vargon.weirdstreamer.api.models.PairingCode
import com.vargon.weirdstreamer.api.models.PairingStatus
import com.vargon.weirdstreamer.api.models.MediaItem
import com.vargon.weirdstreamer.api.models.StreamProgress
import com.vargon.weirdstreamer.api.models.ContinueItem
import com.vargon.weirdstreamer.api.models.PlaybackSync
import com.vargon.weirdstreamer.api.models.SyncedPlayback
import com.vargon.weirdstreamer.api.models.WatchedRequest
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

    // Signing a TV in with a code: the TV makes one and asks after it; a signed-in account claims it.
    @POST("/api/auth/pairing")
    suspend fun createPairing(): Response<PairingCode>

    @GET("/api/auth/pairing/{code}")
    suspend fun pollPairing(@Path("code") code: String): Response<PairingStatus>

    @POST("/api/pairing/claim")
    suspend fun claimPairing(@Body request: PairingClaim): Response<JsonObject>
    
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

    /** A file's trickplay frames by its path, for downloads (which keep the path, not the ID). */
    @GET("/api/media/trickplay")
    suspend fun getTrickplayByPath(@Query("path") path: String): Response<TrickplayInfo>

    @GET("/api/media/disc")
    suspend fun getDisc(@Query("path") path: String): Response<DiscInfo>

    // Subtitles: a video's tracks (its own, then files beside it and downloads), a text track as
    // WebVTT (a window at a time), and OpenSubtitles search and download through the server.
    @GET("/api/media/codecs")
    suspend fun getCodecs(@Query("path") path: String): Response<CodecsInfo>

    @GET("/api/media/seekpoint")
    suspend fun getSeekPoint(@Query("path") path: String, @Query("start") start: String,
                             @Query("next") next: Int? = null): Response<SeekPoint>

    @GET("/api/media/subtitles")
    suspend fun getSubtitles(@Query("path") path: String, @Query("track") track: Int,
                             @Query("start") start: Double, @Query("duration") duration: Double,
                             @Query("format") format: String? = null): Response<ResponseBody>

    /** A font embedded in a video, for its ASS/SSA tracks: font is its index from the codecs report. */
    @GET("/api/media/font")
    suspend fun getFont(@Query("path") path: String, @Query("font") font: Int): Response<ResponseBody>

    /** A subset of the server's CJK font for the Chinese, Japanese and Korean characters in [body]'s text. */
    @POST("/api/media/subtitle-font")
    suspend fun getSubtitleFont(@Body body: com.vargon.weirdstreamer.api.models.SubtitleFontRequest): Response<ResponseBody>

    @PUT("/api/media/subtitle-offset")
    suspend fun setSubtitleOffset(@Body request: com.vargon.weirdstreamer.api.models.SubtitleOffsetRequest): Response<Unit>

    @GET("/api/media/subtitles/search")
    suspend fun searchSubtitles(@Query("path") path: String, @Query("languages") languages: String): Response<SubtitleSearch>

    @POST("/api/media/subtitles/download")
    suspend fun downloadSubtitle(@Body request: SubtitleDownloadRequest): Response<SubtitleDownload>

    // Server administration (admins), as in the web UI's Settings. The config is kept as raw
    // JSON, so fields this app doesn't edit go back to the server untouched.
    @GET("/api/config")
    suspend fun getConfig(): Response<JsonObject>

    @PUT("/api/config")
    suspend fun updateConfig(@Body config: JsonObject): Response<JsonObject>

    @GET("/api/groups")
    suspend fun getGroups(): Response<List<String>>

    // The defined groups: each returns the new list ({"name": …} bodies)
    @POST("/api/groups")
    suspend fun createGroup(@Body body: JsonObject): Response<List<String>>

    @PUT("/api/groups/{name}")
    suspend fun renameGroup(@Path("name") name: String, @Body body: JsonObject): Response<List<String>>

    /** Makes a group the default for new users: it moves to the front of the list. */
    @PUT("/api/groups/{name}/default")
    suspend fun makeDefaultGroup(@Path("name") name: String): Response<List<String>>

    @DELETE("/api/groups/{name}")
    suspend fun deleteGroup(@Path("name") name: String): Response<List<String>>

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

    // Settings → Jobs: the background jobs, and Run now / Pause / Resume (action: run, pause, resume)
    @GET("/api/admin/jobs")
    suspend fun getJobs(): Response<JobsResponse>

    @POST("/api/admin/jobs/{id}/{action}")
    suspend fun controlJob(@Path("id") id: String, @Path("action") action: String): Response<JobsResponse>

    @POST("/api/admin/jobs/trickplay/settings")
    suspend fun setTrickplaySettings(@Body request: com.vargon.weirdstreamer.api.models.TrickplaySettingsRequest): Response<JobsResponse>

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

    @POST("/api/playback/watched")
    suspend fun setWatched(@Body request: WatchedRequest): Response<Unit>

    @GET("/api/media/segments")
    suspend fun getSegments(@Query("path") path: String): Response<MediaSegments>

    @GET("/api/media/upcoming")
    suspend fun upcoming(@Query("path") path: String, @Query("count") count: Int): Response<com.vargon.weirdstreamer.api.models.UpcomingEpisodes>

    @PUT("/api/library/autoskip")
    suspend fun setAutoSkip(@Body request: AutoSkipRequest): Response<Unit>

    @POST("/api/playback/sync")
    suspend fun syncPlayback(@Body request: PlaybackSync): Response<Unit>

    @GET("/api/library/new")
    suspend fun newAdditions(@Query("lib") libraryId: String?): Response<List<ContinueItem>>

    @GET("/api/library/new?by=library")
    suspend fun newAdditionsByLibrary(): Response<List<com.vargon.weirdstreamer.api.models.LibraryAdditions>>

    @GET("/api/playback/continue")
    suspend fun continueWatching(@Query("lib") libraryId: String? = null): Response<List<ContinueItem>>

    @DELETE("/api/playback/continue")
    suspend fun removeFromContinue(@Query("path") path: String): Response<Unit>

    @GET("/api/library")
    suspend fun getLibrary(@Query("lib") libraryId: String): Response<LibraryListing>

    @GET("/api/library/title")
    suspend fun getLibraryTitle(
        @Query("lib") libraryId: String,
        @Query("kind") kind: String,
        @Query("id") tmdbId: Int
    ): Response<TitleDetail>
}
