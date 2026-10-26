package com.vargon.weirdstreamer.ui.components

import android.content.Context
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.update.AppUpdater
import com.vargon.weirdstreamer.update.UpdateState
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.launch

// The App update row in the TV and phone settings.

/** Servers to look for a newer app on: the one in use first, then the other saved ones. */
internal fun updateServers(viewModel: MediaViewModel): List<String> {
    val current = viewModel.currentSource?.takeIf { it.typeId == "SERVER" }?.serverAddress
    return listOfNotNull(current) + viewModel.credentialManager.getApiServers().sorted()
}

/** The row's title and the line under it. */
internal fun updateLabels(state: UpdateState): Pair<String, String> {
    val installed = "Installed: ${AppUpdater.installedVersion}"
    return when (state) {
        UpdateState.Idle -> "Check for updates" to installed
        UpdateState.Checking -> "Checking for updates…" to installed
        UpdateState.UpToDate -> "Up to date" to "$installed. Select to check again."
        UpdateState.NotPublished -> "No update published" to "$installed. None of your servers offers the app."
        is UpdateState.Available -> "Update to ${state.release.version}" to
            "$installed. ${formatBytes(state.release.size)} from ${state.release.server}"
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
        else -> scope.launch { AppUpdater.check(updateServers(viewModel)) }
    }
}
