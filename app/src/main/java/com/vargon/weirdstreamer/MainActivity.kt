package com.vargon.weirdstreamer

import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.tv.material3.*
import com.vargon.weirdstreamer.ui.components.*
import com.vargon.weirdstreamer.ui.theme.AppTheme
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
            val credentialManager = remember { CredentialManager(applicationContext) }
            var currentTheme by remember { 
                mutableStateOf(
                    when (credentialManager.getAppTheme()) {
                        "CINEMA_BLUE" -> AppTheme.CINEMA_BLUE
                        "CINEMA_RED" -> AppTheme.CINEMA_RED
                        else -> AppTheme.DEFAULT
                    }
                )
            }
            
            WeirdStreamerTheme(appTheme = currentTheme) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                ) {
                    var showSettings by remember { mutableStateOf(false) }
                    val selectedVideo = viewModel.selectedVideoPath
                    val selectedImage = viewModel.selectedImagePath
                    val selectedPdf = viewModel.selectedPdfPath

                    // Keep screen on when viewing media (video, image, or PDF)
                    val isViewingMedia = selectedVideo != null || selectedImage != null || selectedPdf != null
                    DisposableEffect(isViewingMedia) {
                        if (isViewingMedia) {
                            window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
                        } else {
                            window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
                        }
                        onDispose {
                            window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
                        }
                    }

                    if (selectedVideo != null) {
                        VideoPlayer(
                            viewModel = viewModel,
                            path = selectedVideo,
                            onBack = { lastViewedPath -> viewModel.stopPlayback(lastViewedPath) }
                        )
                    } else if (selectedImage != null) {
                        ImageViewer(
                            viewModel = viewModel,
                            initialPath = selectedImage,
                            onBack = { lastViewedPath -> viewModel.closeImage(lastViewedPath) }
                        )
                    } else if (selectedPdf != null) {
                        PdfViewer(
                            viewModel = viewModel,
                            initialPath = selectedPdf,
                            onBack = { lastViewedPath -> viewModel.closePdf(lastViewedPath) }
                        )
                    } else if (showSettings) {
                        SettingsScreen(
                            viewModel = viewModel,
                            onBack = { showSettings = false },
                            onThemeChanged = {
                                currentTheme = when (credentialManager.getAppTheme()) {
                                    "CINEMA_BLUE" -> AppTheme.CINEMA_BLUE
                                    "CINEMA_RED" -> AppTheme.CINEMA_RED
                                    else -> AppTheme.DEFAULT
                                }
                            }
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
