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
