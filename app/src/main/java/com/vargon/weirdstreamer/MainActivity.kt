package com.vargon.weirdstreamer

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ui.components.ImageViewer
import com.vargon.weirdstreamer.ui.components.SettingsScreen
import com.vargon.weirdstreamer.ui.components.SmbBrowser
import com.vargon.weirdstreamer.ui.components.VideoPlayer
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

class MainActivity : ComponentActivity() {
    private val viewModel: SmbViewModel by viewModels()

    @OptIn(ExperimentalTvMaterial3Api::class)
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            WeirdStreamerTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                ) {
                    var showSettings by remember { mutableStateOf(false) }
                    val selectedVideo = viewModel.selectedVideoPath
                    val selectedImage = viewModel.selectedImagePath

                    if (selectedVideo != null) {
                        VideoPlayer(
                            viewModel = viewModel,
                            path = selectedVideo,
                            onBack = { viewModel.stopPlayback() }
                        )
                    } else if (selectedImage != null) {
                        ImageViewer(
                            viewModel = viewModel,
                            initialPath = selectedImage,
                            onBack = { viewModel.closeImage() }
                        )
                    } else if (showSettings) {
                        SettingsScreen(
                            viewModel = viewModel,
                            onBack = { showSettings = false }
                        )
                    } else {
                        SmbBrowser(
                            viewModel = viewModel,
                            onOpenSettings = { showSettings = true }
                        )
                    }
                }
            }
        }
    }
}
