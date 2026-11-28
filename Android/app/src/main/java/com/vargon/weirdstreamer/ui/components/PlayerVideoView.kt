package com.vargon.weirdstreamer.ui.components

import android.view.LayoutInflater
import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.compose.runtime.Composable
import androidx.compose.runtime.key
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import androidx.media3.common.Player
import androidx.media3.ui.AspectRatioFrameLayout
import androidx.media3.ui.PlayerView
import com.vargon.weirdstreamer.R

/**
 * The players' picture (TV and phone). It's a SurfaceView, the only kind of view whose frames reach the display
 * as they are: HDR10, HLG and Dolby Vision need it for the TV to switch into its HDR or Dolby Vision mode. A
 * TextureView's frames are composed as SDR graphics, so HDR video in it came out dark.
 *
 * A SurfaceView follows the modifier's scaling and moving (zoom, pan, fill, aspect) but not its rotation, so
 * while the picture is turned ([rotated]) it's drawn in a TextureView instead, where HDR looks dim again.
 */
@Composable
fun PlayerVideoView(player: Player, rotated: Boolean, modifier: Modifier = Modifier) {
    key(rotated) {
        AndroidView(
            factory = { ctx ->
                val layout = if (rotated) R.layout.texture_video_view else R.layout.surface_video_view
                (LayoutInflater.from(ctx).inflate(layout, null) as PlayerView).apply {
                    this.player = player
                    useController = false
                    resizeMode = AspectRatioFrameLayout.RESIZE_MODE_FIT
                    layoutParams = FrameLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
                }
            },
            // A new player (another transcode mode) gets the view.
            update = { view -> view.player = player },
            // The view switched away from lets go of the player (which keeps the new view's surface).
            onRelease = { view -> view.player = null },
            modifier = modifier,
        )
    }
}
