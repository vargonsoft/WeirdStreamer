package com.vargon.weirdstreamer.ui.components

import android.app.Activity
import android.app.PendingIntent
import android.app.PictureInPictureParams
import android.app.RemoteAction
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.drawable.Icon
import android.os.Build
import android.util.Rational
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.rememberUpdatedState
import androidx.core.content.ContextCompat

/**
 * Picture-in-picture for the phone player: going home while a video plays shrinks it into a
 * window (on its own from Android 12, from [MainActivity]'s onUserLeaveHint before that), with a
 * play/pause button. The player shows only the video and its subtitles there.
 */
object PhonePip {
    private const val ACTION = "com.vargon.weirdstreamer.PIP_PLAY_PAUSE"

    /** Whether going home now should open the window: a video is playing. */
    var wanted = false
        private set

    /** The window's settings as they are now, for entering it by hand (Android 10 and 11). */
    var params: PictureInPictureParams? = null
        private set

    /**
     * Keeps the window's settings current while a video is shown: [playing] decides whether
     * going home opens it, [aspect] (width / height) is its shape, and its button plays or pauses
     * with [onPlayPause].
     */
    @Composable
    fun Track(activity: Activity?, playing: Boolean, aspect: Float, onPlayPause: () -> Unit) {
        activity ?: return
        val playPause = rememberUpdatedState(onPlayPause)
        DisposableEffect(activity) {
            val receiver = object : BroadcastReceiver() {
                override fun onReceive(context: Context, intent: Intent) { playPause.value() }
            }
            ContextCompat.registerReceiver(activity, receiver, IntentFilter(ACTION), ContextCompat.RECEIVER_NOT_EXPORTED)
            onDispose {
                activity.unregisterReceiver(receiver)
                wanted = false
                params = null
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                    activity.setPictureInPictureParams(PictureInPictureParams.Builder().setAutoEnterEnabled(false).build())
                }
            }
        }
        DisposableEffect(playing, aspect) {
            val built = build(activity, playing, aspect)
            wanted = playing
            params = built
            activity.setPictureInPictureParams(built)
            onDispose {}
        }
    }

    private fun build(activity: Activity, playing: Boolean, aspect: Float): PictureInPictureParams {
        val intent = PendingIntent.getBroadcast(activity, 0, Intent(ACTION).setPackage(activity.packageName),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val action = RemoteAction(
            Icon.createWithResource(activity, if (playing) android.R.drawable.ic_media_pause else android.R.drawable.ic_media_play),
            if (playing) "Pause" else "Play", if (playing) "Pause" else "Play", intent)
        // Android takes shapes from 1:2.39 to 2.39:1 only.
        val ratio = if (aspect > 0f) aspect.coerceIn(1 / 2.39f, 2.39f) else 16f / 9f
        return PictureInPictureParams.Builder()
            .setAspectRatio(Rational((ratio * 1000).toInt(), 1000))
            .setActions(listOf(action))
            .apply {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                    setAutoEnterEnabled(playing)
                    setSeamlessResizeEnabled(false) // video: no cross-fade of stretched frames
                }
            }
            .build()
    }
}
