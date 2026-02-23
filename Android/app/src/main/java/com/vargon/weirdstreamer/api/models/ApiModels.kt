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
    @SerializedName("dirs") val dirs: List<Directory>? = null
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
