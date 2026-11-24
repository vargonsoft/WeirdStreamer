package com.vargon.weirdstreamer.ui.components

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import com.vargon.weirdstreamer.update.AppUpdater
import com.vargon.weirdstreamer.update.UpdateState
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// The App update row in the TV and phone settings, and the banner that says there's an update.

private const val UPDATE_CHECK_EVERY_MS = 6 * 60 * 60 * 1000L
private const val TV_BANNER_MS = 15_000L

/** Looks for a newer app when it opens and on another server, and every few hours for one left running. */
@Composable
internal fun UpdateChecks(viewModel: MediaViewModel) {
    LaunchedEffect(viewModel.currentSource?.serverAddress) {
        while (true) {
            AppUpdater.checkQuietly(updateServers(viewModel))
            delay(UPDATE_CHECK_EVERY_MS)
        }
    }
}

/**
 * Says a new version can be installed from Settings, over the browse screens. On a TV it's only to
 * read, so it never takes focus, and it goes by itself as nothing can close it (it comes back with
 * the browse screens). On a phone a tap opens Settings, and X closes it for this version.
 */
@Composable
internal fun UpdateBanner(isTv: Boolean, onOpenSettings: () -> Unit, modifier: Modifier = Modifier) {
    val release = (AppUpdater.state as? UpdateState.Available)?.release ?: return
    if (release.build == AppUpdater.dismissedBuild) return
    var onScreen by remember(release.build) { mutableStateOf(true) }
    if (isTv) LaunchedEffect(release.build) {
        delay(TV_BANNER_MS)
        onScreen = false
    }
    if (!onScreen) return
    val message = "Version ${release.version} is available. Update in Settings."
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = modifier
            .then(if (isTv) Modifier.padding(bottom = 24.dp) else Modifier.navigationBarsPadding().padding(12.dp))
            .background(Color.Black.copy(alpha = 0.85f), RoundedCornerShape(12.dp))
            .then(if (isTv) Modifier else Modifier.clickable(onClick = onOpenSettings))
            .padding(horizontal = 16.dp, vertical = if (isTv) 12.dp else 4.dp)
    ) {
        Icon(Icons.Default.SystemUpdate, contentDescription = null, tint = WeirdStreamerTheme.spec.headingColor,
            modifier = Modifier.size(if (isTv) 24.dp else 20.dp))
        Spacer(Modifier.width(12.dp))
        Text(message, color = Color.White, fontSize = if (isTv) 18.sp else 14.sp,
            modifier = if (isTv) Modifier else Modifier.weight(1f, fill = false))
        if (!isTv) IconButton(onClick = { AppUpdater.dismissedBuild = release.build }) {
            Icon(Icons.Default.Close, contentDescription = "Close", tint = Color.White, modifier = Modifier.size(18.dp))
        }
    }
}

/**
 * Servers to look for a newer app on, with their tokens: the one in use first, then the other saved
 * ones, each only while an account is signed in to it (servers give the app to no one else). With
 * none, or none offering it, the updater asks GitHub.
 */
internal fun updateServers(viewModel: MediaViewModel): List<Pair<String, String>> {
    val current = viewModel.currentSource?.takeIf { it.typeId == "SERVER" }?.serverAddress
    val credentials = viewModel.credentialManager
    return (listOfNotNull(current) + credentials.getApiServers().sorted()).distinct()
        .mapNotNull { server -> credentials.getServerToken(server)?.let { server to it } }
}

/** Every release's APK, for when neither a server nor GitHub's API answered (the asset's name has its version, so the page). */
private const val RELEASES_PAGE = "github.com/${AppUpdater.GITHUB_REPO}/releases/latest"

/** The row's title and the line under it. */
internal fun updateLabels(state: UpdateState): Pair<String, String> {
    val installed = "Installed: ${AppUpdater.installedVersion}"
    return when (state) {
        UpdateState.Idle -> "Check for updates" to installed
        UpdateState.Checking -> "Checking for updates…" to installed
        UpdateState.UpToDate -> "Up to date" to "$installed. Select to check again."
        UpdateState.NotPublished -> "Get the latest version on GitHub" to "$installed. Neither your servers nor GitHub answered: $RELEASES_PAGE"
        is UpdateState.Available -> "Update to ${state.release.version}" to
            "$installed. ${formatBytes(state.release.size)} from ${state.release.from}"
        is UpdateState.Downloading -> "Downloading ${state.release.version}… ${(state.fraction * 100).toInt()}%" to installed
        UpdateState.Installing -> "Installing…" to "Confirm in the installer. The app closes when it's done: open it again."
        is UpdateState.Failed -> "Update failed" to "${state.message}. Select to try again."
    }
}

/**
 * What selecting the row does: check, or install what the check found. In the view model's scope,
 * so leaving Settings doesn't stop a download.
 */
internal fun onUpdateRow(viewModel: MediaViewModel, context: Context) {
    val scope = viewModel.viewModelScope
    when (val state = AppUpdater.state) {
        is UpdateState.Available -> scope.launch { AppUpdater.install(context.applicationContext, state.release) }
        UpdateState.Checking, is UpdateState.Downloading, UpdateState.Installing -> Unit
        // The releases page in a browser; a TV without one looks again (the row shows the address)
        UpdateState.NotPublished -> try {
            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("https://$RELEASES_PAGE")).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (e: ActivityNotFoundException) {
            scope.launch { AppUpdater.check(updateServers(viewModel)) }
        }
        else -> scope.launch { AppUpdater.check(updateServers(viewModel)) }
    }
}
