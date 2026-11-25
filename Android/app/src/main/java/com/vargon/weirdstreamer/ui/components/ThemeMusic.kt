package com.vargon.weirdstreamer.ui.components

import android.media.MediaPlayer
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.viewModelScope
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.TitleDetail
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

// Theme music on title pages (Settings: "Play theme music on title pages", this device's, on by
// default, at Low, Medium or High): a show's or movie's theme (the server's GET /api/library/theme: its folder's theme file,
// or cut from an intro or the credits) plays once, quietly, while its page is open, and fades out
// when the page closes or a video plays. It's downloaded through the app's client first (which
// trusts the saved server's certificate), then played from the file.

/** The theme's volume for the setting's level (of the device's volume). */
internal fun themeVolume(level: String) = when (level) { "low" -> 0.15f; "high" -> 0.5f; else -> 0.3f }

/** The setting's levels, as (level, label). */
internal val THEME_VOLUMES = listOf("low" to "Low", "medium" to "Medium", "high" to "High")

/** Plays [title]'s theme while its page is open (TV and phone title pages). */
@Composable
fun TitleThemeMusic(viewModel: MediaViewModel, title: TitleDetail) {
    val context = LocalContext.current
    val key = "${title.kind}:${title.tmdbId}"
    var player by remember(key) { mutableStateOf<MediaPlayer?>(null) }
    var started by remember(key) { mutableStateOf(false) } // once per visit
    val wanted = title.theme && viewModel.themeMusic && viewModel.selectedVideoPath == null
    LaunchedEffect(key, wanted) {
        if (!wanted) {
            player?.let { fadeOut(viewModel, it, themeVolume(viewModel.themeVolume)) }
            player = null
            return@LaunchedEffect
        }
        if (started) return@LaunchedEffect
        started = true
        val api = viewModel.connectedServer?.api ?: return@LaunchedEffect
        val lib = viewModel.currentLibraryId ?: return@LaunchedEffect
        val file = File(context.cacheDir, "theme-music")
        val ok = withContext(Dispatchers.IO) {
            runCatching {
                val response = api.getTheme(lib, title.kind, title.tmdbId)
                val body = response.body()
                if (!response.isSuccessful || body == null) return@runCatching false
                body.byteStream().use { input -> file.outputStream().use { input.copyTo(it) } }
                true
            }.getOrDefault(false)
        }
        if (!ok) return@LaunchedEffect
        val mp = withContext(Dispatchers.IO) {
            runCatching { MediaPlayer().apply { setDataSource(file.path); themeVolume(viewModel.themeVolume).let { setVolume(it, it) }; prepare() } }.getOrNull()
        } ?: return@LaunchedEffect
        mp.setOnCompletionListener { it.release(); if (player === it) player = null }
        mp.start()
        player = mp
    }
    // A new volume applies to the theme playing
    LaunchedEffect(viewModel.themeVolume) {
        val v = themeVolume(viewModel.themeVolume)
        runCatching { player?.setVolume(v, v) }
    }
    DisposableEffect(key) {
        onDispose { player?.let { fadeOut(viewModel, it, themeVolume(viewModel.themeVolume)) }; player = null }
    }
}

/** Fades [mp] out over a second and a half, then releases it (outlasting the page that played it). */
private fun fadeOut(viewModel: MediaViewModel, mp: MediaPlayer, from: Float) {
    viewModel.viewModelScope.launch {
        runCatching {
            if (mp.isPlaying) {
                for (i in 14 downTo 0) {
                    val v = from * i / 15f
                    mp.setVolume(v, v)
                    delay(100)
                }
            }
        }
        runCatching { mp.stop() }
        runCatching { mp.release() }
    }
}
