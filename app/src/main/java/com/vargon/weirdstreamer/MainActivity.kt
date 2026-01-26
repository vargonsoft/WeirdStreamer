package com.vargon.weirdstreamer

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ui.components.*
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

class MainActivity : ComponentActivity() {
    // Use the new MediaViewModel that supports both SMB and Server API
    private val viewModel: MediaViewModel by viewModels()
    // Uncomment below to use legacy SMB-only mode:
    // private val smbViewModel: SmbViewModel by viewModels()

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
                    val selectedPdf = viewModel.selectedPdfPath

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
                    } else if (selectedPdf != null) {
                        PdfViewer(
                            viewModel = viewModel,
                            initialPath = selectedPdf,
                            onBack = { viewModel.closePdf() }
                        )
                    } else if (showSettings) {
                        SettingsScreen(
                            viewModel = viewModel,
                            onBack = { showSettings = false }
                        )
                    } else {
                        MediaBrowser(
                            viewModel = viewModel,
                            onOpenSettings = { showSettings = true }
                        )
                        // For legacy SMB-only mode, use:
                        // SmbBrowser(viewModel = smbViewModel, onOpenSettings = { showSettings = true })
                    }
                }
            }
        }
    }
}
