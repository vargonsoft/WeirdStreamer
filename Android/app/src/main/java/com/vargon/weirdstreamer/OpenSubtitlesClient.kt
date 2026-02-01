package com.vargon.weirdstreamer

import android.content.Context
import android.net.Uri
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.net.URLEncoder
import java.util.zip.GZIPInputStream

data class SubtitleInfo(
    val id: String,
    val filename: String,
    val language: String,
    val rating: Float,
    val downloads: Int,
    val url: String
)

class OpenSubtitlesClient(private val context: Context) {
    private val httpClient = OkHttpClient()
    private val apiBaseUrl = "https://api.opensubtitles.com/api/v1"
    private var apiKey = getApiKeyFromConfig()

    companion object {
        private const val CONFIG_API_KEY = "opensubtitles_api_key"
        
        fun setApiKey(context: Context, apiKey: String) {
            val prefs = context.getSharedPreferences("opensubtitles_config", Context.MODE_PRIVATE)
            prefs.edit().putString(CONFIG_API_KEY, apiKey).apply()
        }
    }

    private fun getApiKeyFromConfig(): String {
        try {
            val prefs = context.getSharedPreferences("opensubtitles_config", Context.MODE_PRIVATE)
            return prefs.getString(CONFIG_API_KEY, "") ?: ""
        } catch (e: Exception) {
            return ""
        }
    }

    fun hasApiKey(): Boolean = apiKey.isNotEmpty()

    /**
     * Search for subtitles by video hash (OpenSubtitles.com format)
     */
    fun searchByHash(
        videoHash: String,
        languages: String = "en"
    ): List<SubtitleInfo> {
        if (!hasApiKey()) {
            android.util.Log.w("OpenSubtitles", "API key not set")
            return emptyList()
        }
        
        try {
            val query = "$apiBaseUrl/subtitles?query=$videoHash&languages=$languages"
            val request = Request.Builder()
                .url(query)
                .addHeader("api-key", apiKey)
                .build()

            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) return emptyList()

                val body = response.body?.string() ?: return emptyList()
                val json = JSONObject(body)
                val data = json.optJSONArray("data") ?: return emptyList()

                return parseSubtitleResults(data)
            }
        } catch (e: Exception) {
            android.util.Log.e("OpenSubtitles", "Error searching by hash", e)
            return emptyList()
        }
    }

    /**
     * Search for subtitles by video filename
     */
    fun searchByFilename(
        filename: String,
        languages: String = "en"
    ): List<SubtitleInfo> {
        if (!hasApiKey()) {
            android.util.Log.w("OpenSubtitles", "API key not set")
            return emptyList()
        }
        
        try {
            val encodedFilename = URLEncoder.encode(filename, "UTF-8")
            val query = "$apiBaseUrl/subtitles?query=$encodedFilename&languages=$languages"
            val request = Request.Builder()
                .url(query)
                .addHeader("api-key", apiKey)
                .build()

            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) return emptyList()

                val body = response.body?.string() ?: return emptyList()
                val json = JSONObject(body)
                val data = json.optJSONArray("data") ?: return emptyList()

                return parseSubtitleResults(data)
            }
        } catch (e: Exception) {
            android.util.Log.e("OpenSubtitles", "Error searching by filename", e)
            return emptyList()
        }
    }

    /**
     * Download subtitle file
     */
    fun downloadSubtitle(
        subtitle: SubtitleInfo,
        outputFile: File
    ): Boolean {
        return try {
            val request = Request.Builder()
                .url(subtitle.url)
                .addHeader("api-key", apiKey)
                .build()

            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) return false

                val inputStream = response.body?.byteStream() ?: return false
                
                // Handle gzip compression if needed
                val decompressed = if (response.header("content-encoding") == "gzip") {
                    GZIPInputStream(inputStream)
                } else {
                    inputStream
                }

                FileOutputStream(outputFile).use { output ->
                    decompressed.copyTo(output)
                }

                true
            }
        } catch (e: Exception) {
            android.util.Log.e("OpenSubtitles", "Error downloading subtitle", e)
            false
        }
    }

    /**
     * Get subtitle cache directory
     */
    fun getSubtitleCacheDir(): File {
        val cacheDir = File(context.cacheDir, "subtitles")
        if (!cacheDir.exists()) {
            cacheDir.mkdirs()
        }
        return cacheDir
    }

    private fun parseSubtitleResults(data: JSONArray): List<SubtitleInfo> {
        val results = mutableListOf<SubtitleInfo>()

        for (i in 0 until data.length()) {
            try {
                val item = data.getJSONObject(i)
                val attributes = item.optJSONObject("attributes") ?: continue

                val subtitleInfo = SubtitleInfo(
                    id = item.optString("id", ""),
                    filename = attributes.optString("filename", "subtitle.srt"),
                    language = attributes.optString("language", "en"),
                    rating = attributes.optDouble("rating", 0.0).toFloat(),
                    downloads = attributes.optInt("download_count", 0),
                    url = attributes.optString("url", "")
                )

                if (subtitleInfo.id.isNotEmpty() && subtitleInfo.url.isNotEmpty()) {
                    results.add(subtitleInfo)
                }
            } catch (e: Exception) {
                android.util.Log.w("OpenSubtitles", "Error parsing subtitle item", e)
            }
        }

        return results
    }

    /**
     * Calculate video hash (for OpenSubtitles compatibility)
     * This is a simple implementation - for production, use the proper algorithm
     */
    fun calculateVideoHash(videoFile: File): String? {
        return try {
            if (!videoFile.exists() || videoFile.length() < 128 * 1024) return null

            val file = java.io.RandomAccessFile(videoFile, "r")
            val buffer = ByteArray(65536)

            // Read hash from beginning
            file.seek(0)
            file.read(buffer)
            var hash = 0L
            for (i in 0 until 8192) {
                hash += (buffer[i * 8].toLong() and 0xFF)
            }

            // Read hash from end
            file.seek(file.length() - 65536)
            file.read(buffer)
            for (i in 0 until 8192) {
                hash += (buffer[i * 8].toLong() and 0xFF)
            }

            file.close()

            String.format("%016x", hash)
        } catch (e: Exception) {
            android.util.Log.e("OpenSubtitles", "Error calculating video hash", e)
            null
        }
    }
}
