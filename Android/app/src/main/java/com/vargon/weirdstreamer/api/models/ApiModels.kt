package com.vargon.weirdstreamer.api.models

import com.google.gson.annotations.SerializedName

data class User(
    @SerializedName("id") val id: Long,
    @SerializedName("username") val username: String,
    @SerializedName("created_at") val createdAt: String,
    @SerializedName("is_admin") val isAdmin: Boolean
)

data class MediaItem(
    @SerializedName("id") val id: Long,
    @SerializedName("path") val path: String,
    @SerializedName("name") val name: String,
    @SerializedName("type") val type: String, // video, image, document
    @SerializedName("size") val size: Long,
    @SerializedName("mime_type") val mimeType: String,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    @SerializedName("duration") val duration: Double? = null,
    @SerializedName("width") val width: Int? = null,
    @SerializedName("height") val height: Int? = null,
    @SerializedName("created_at") val createdAt: String,
    @SerializedName("updated_at") val updatedAt: String
)

data class TrickplayFrame(
    @SerializedName("time") val time: Int,
    @SerializedName("url") val url: String
)

data class TrickplayInfo(
    @SerializedName("frames") val frames: List<TrickplayFrame>
)

data class Directory(
    // `path` JSON key now carries the root ID or the relative id-based path
    @SerializedName("path") val id: String,
    @SerializedName("name") val name: String,
    @SerializedName("items") val items: List<MediaItem>? = null,
    @SerializedName("dirs") val dirs: List<Directory>? = null,
    // "movie" or "tv" for libraries with a type (on root entries and on browse responses)
    @SerializedName("library_type") val libraryType: String? = null
)

// Library view: a movie or TV library as titles instead of folders

data class LibraryListing(
    @SerializedName("library_id") val libraryId: String,
    @SerializedName("library_name") val libraryName: String,
    @SerializedName("library_type") val libraryType: String,
    @SerializedName("titles") val titles: List<LibraryTitle> = emptyList(),
    @SerializedName("unmatched") val unmatched: List<LibraryFile> = emptyList()
)

data class LibraryTitle(
    @SerializedName("kind") val kind: String, // "movie" or "show"
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("file_count") val fileCount: Int = 0
)

data class LibraryFile(
    @SerializedName("id") val id: Long? = null,
    @SerializedName("path") val path: String,
    @SerializedName("name") val name: String,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("duration") val duration: Double? = null,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    @SerializedName("count") val count: Int? = null,
    // The movie a TV-library file that isn't an episode was matched to by hand
    @SerializedName("match") val match: FileMatch? = null
)

/** What a file downloads as (GET /api/downloads/options). */
data class DownloadOptions(
    @SerializedName("size") val size: Long = 0,
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0,
    @SerializedName("disc") val disc: Boolean = false,
    @SerializedName("sizes") val sizes: List<Int> = listOf(90, 75, 50, 25),
    @SerializedName("scales") val scales: List<Int> = listOf(100, 75, 50)
)

data class CreateDownloadRequest(
    @SerializedName("path") val path: String, // base64 server path
    @SerializedName("size_pct") val sizePct: Int,
    @SerializedName("scale") val scale: Int
)

/** A server-side encode for a compressed download. */
data class DownloadJob(
    @SerializedName("id") val id: String,
    @SerializedName("status") val status: String, // queued, encoding, ready, failed
    @SerializedName("progress") val progress: Double = 0.0,
    @SerializedName("error") val error: String? = null,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0
)

/** A user account on the server, as admins see it. */
data class ServerUser(
    @SerializedName("id") val id: Long,
    @SerializedName("username") val username: String,
    @SerializedName("is_admin") val isAdmin: Boolean,
    @SerializedName("group") val group: String? = null
)

data class CreateUserRequest(
    @SerializedName("username") val username: String,
    @SerializedName("password") val password: String,
    @SerializedName("is_admin") val isAdmin: Boolean,
    @SerializedName("group") val group: String
)

data class ChangePasswordRequest(
    @SerializedName("old_password") val oldPassword: String,
    @SerializedName("new_password") val newPassword: String
)

/** A stream playing on the server, from its admin Streams list. */
data class AdminStream(
    @SerializedName("id") val id: String,
    @SerializedName("user") val user: String = "",
    @SerializedName("client") val client: String = "",
    @SerializedName("client_ip") val clientIp: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("title") val title: String? = null,
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("state") val state: String = "",
    @SerializedName("mode") val mode: String = "",
    @SerializedName("bytes_sent") val bytesSent: Long = 0,
    @SerializedName("served_to") val servedTo: Double = 0.0,
    @SerializedName("position") val position: Double? = null,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0,
    @SerializedName("transcode") val transcode: AdminTranscode? = null
)

data class AdminTranscode(
    @SerializedName("reason") val reason: String = "",
    @SerializedName("video") val video: String = "",
    @SerializedName("audio") val audio: String = "",
    @SerializedName("speed") val speed: Double = 0.0,
    @SerializedName("fps") val fps: Double = 0.0,
    @SerializedName("stalled") val stalled: Boolean = false,
    @SerializedName("source_video") val sourceVideo: String? = null
)

/** A DVD disc image's titles, for the player's disc controls. */
data class DiscInfo(
    @SerializedName("titles") val titles: List<DiscTitle> = emptyList(),
    @SerializedName("main_title") val mainTitle: Int = 1
) {
    fun title(number: Int): DiscTitle? =
        titles.firstOrNull { it.number == (if (number <= 0) mainTitle else number) } ?: titles.firstOrNull()
}

data class DiscTitle(
    @SerializedName("number") val number: Int,
    @SerializedName("duration") val duration: Double, // seconds
    @SerializedName("angles") val angles: Int = 1,
    @SerializedName("chapters") val chapters: List<Double> = emptyList(), // start times, seconds
    @SerializedName("audio") val audio: List<DiscTrack> = emptyList(),
    @SerializedName("subtitles") val subtitles: List<DiscTrack> = emptyList()
)

data class DiscTrack(
    @SerializedName("index") val index: Int,
    @SerializedName("language") val language: String? = null,
    @SerializedName("codec") val codec: String = "",
    @SerializedName("channels") val channels: Int = 0
) {
    val label: String
        get() = listOfNotNull(
            language?.takeIf { it.isNotBlank() } ?: "Unknown",
            codec.takeIf { it.isNotBlank() },
            when (channels) { 0 -> null; 2 -> "Stereo"; 6 -> "5.1"; else -> "$channels ch" }
        ).joinToString(" · ")
}

/** The player's position, reported for the server's admin Streams page. */
data class StreamProgress(
    @SerializedName("path") val path: String, // base64 server path
    @SerializedName("position") val position: Double, // seconds
    @SerializedName("paused") val paused: Boolean
)

data class FileMatch(
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null
)

data class CreditInfo(
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("name") val name: String,
    @SerializedName("character") val character: String? = null,
    @SerializedName("profile") val profile: String? = null
)

data class LibraryEpisode(
    @SerializedName("season") val season: Int,
    @SerializedName("episode") val episode: Int,
    @SerializedName("title") val title: String,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("air_date") val airDate: String? = null,
    @SerializedName("runtime") val runtime: Int? = null,
    @SerializedName("still") val still: String? = null,
    @SerializedName("files") val files: List<LibraryFile> = emptyList()
)

data class LibrarySeason(
    @SerializedName("season") val season: Int,
    @SerializedName("name") val name: String,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("episodes") val episodes: List<LibraryEpisode> = emptyList()
)

data class TitleDetail(
    @SerializedName("kind") val kind: String, // "movie" or "show"
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("runtime") val runtime: Int? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("backdrop") val backdrop: String? = null,
    @SerializedName("logo") val logo: String? = null,
    @SerializedName("tagline") val tagline: String? = null,
    @SerializedName("genres") val genres: List<String>? = null,
    @SerializedName("directors") val directors: List<CreditInfo>? = null,
    @SerializedName("writers") val writers: List<CreditInfo>? = null,
    @SerializedName("creators") val creators: List<CreditInfo>? = null,
    @SerializedName("cast") val cast: List<CreditInfo>? = null,
    @SerializedName("files") val files: List<LibraryFile>? = null,
    @SerializedName("seasons") val seasons: List<LibrarySeason>? = null,
    @SerializedName("other_files") val otherFiles: List<LibraryFile>? = null
)

data class LoginRequest(
    @SerializedName("username") val username: String,
    @SerializedName("password") val password: String
)

data class LoginResponse(
    @SerializedName("token") val token: String,
    @SerializedName("expires_at") val expiresAt: Long,
    @SerializedName("user") val user: User
)

data class ErrorResponse(
    @SerializedName("error") val error: String,
    @SerializedName("message") val message: String? = null
)
