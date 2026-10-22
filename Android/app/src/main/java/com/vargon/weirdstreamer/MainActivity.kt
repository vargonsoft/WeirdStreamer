package com.vargon.weirdstreamer

import android.content.pm.PackageManager
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.tv.material3.ExperimentalTvMaterial3Api
import com.vargon.weirdstreamer.ui.components.*
import com.vargon.weirdstreamer.ui.theme.AppTheme
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerPhoneTheme
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

class MainActivity : ComponentActivity() {
    private val viewModel: MediaViewModel by viewModels()

    private val isTv by lazy {
        packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)
    }

    @OptIn(ExperimentalTvMaterial3Api::class, ExperimentalMaterial3Api::class)
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            val credentialManager = remember { CredentialManager(applicationContext) }
            var currentTheme by remember { 
                mutableStateOf(AppTheme.fromKey(credentialManager.getAppTheme()))
            }
            
            if (isTv) {
                WeirdStreamerTheme(appTheme = currentTheme) {
                    androidx.tv.material3.Surface(
                        modifier = Modifier.fillMaxSize()
                    ) {
                        MainContent(currentTheme) { theme -> currentTheme = theme }
                    }
                }
            } else {
                WeirdStreamerPhoneTheme(appTheme = currentTheme) {
                    androidx.compose.material3.Surface(
                        modifier = Modifier.fillMaxSize(),
                        color = androidx.compose.material3.MaterialTheme.colorScheme.background
                    ) {
                        MainContent(currentTheme) { theme -> currentTheme = theme }
                    }
                }
            }
        }
    }

    @OptIn(ExperimentalTvMaterial3Api::class, ExperimentalMaterial3Api::class)
    @Composable
    private fun MainContent(currentTheme: AppTheme, onThemeChanged: (AppTheme) -> Unit) {
        var showSettings by remember { mutableStateOf(false) }
        val selectedVideo = viewModel.selectedVideoPath
        val selectedImage = viewModel.selectedImagePath
        val selectedPdf = viewModel.selectedPdfPath

        // Keep screen on when viewing media
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

        when {
            selectedVideo != null -> {
                if (isTv) {
                    VideoPlayer(
                        viewModel = viewModel,
                        path = selectedVideo,
                        onBack = { lastViewedPath -> viewModel.stopPlayback(lastViewedPath) }
                    )
                } else {
                    PhoneVideoPlayer(
                        viewModel = viewModel,
                        path = selectedVideo,
                        onBack = { lastViewedPath -> viewModel.stopPlayback(lastViewedPath) }
                    )
                }
            }
            selectedImage != null -> {
                if (isTv) {
                    ImageViewer(
                        viewModel = viewModel,
                        initialPath = selectedImage,
                        onBack = { lastViewedPath -> viewModel.closeImage(lastViewedPath) }
                    )
                } else {
                    PhoneImageViewer(
                        viewModel = viewModel,
                        initialPath = selectedImage,
                        onBack = { lastViewedPath -> viewModel.closeImage(lastViewedPath) }
                    )
                }
            }
            selectedPdf != null -> {
                if (isTv) {
                    PdfViewer(
                        viewModel = viewModel,
                        initialPath = selectedPdf,
                        onBack = { lastViewedPath -> viewModel.closePdf(lastViewedPath) }
                    )
                } else {
                    PhonePdfViewer(
                        viewModel = viewModel,
                        initialPath = selectedPdf,
                        onBack = { lastViewedPath -> viewModel.closePdf(lastViewedPath) }
                    )
                }
            }
            showSettings -> {
                val credentialManager = remember { CredentialManager(applicationContext) }
                val onThemeUpdate = {
                    onThemeChanged(AppTheme.fromKey(credentialManager.getAppTheme()))
                }
                
                if (isTv) {
                    SettingsScreen(
                        viewModel = viewModel,
                        onBack = { showSettings = false },
                        onThemeChanged = onThemeUpdate
                    )
                } else {
                    PhoneSettingsScreen(
                        viewModel = viewModel,
                        onBack = { showSettings = false },
                        onThemeChanged = onThemeUpdate
                    )
                }
            }
            else -> {
                if (isTv) {
                    MediaBrowser(
                        viewModel = viewModel,
                        onOpenSettings = { showSettings = true }
                    )
                } else {
                    PhoneMediaBrowser(
                        viewModel = viewModel,
                        onOpenSettings = { showSettings = true }
                    )
                }
            }
        }
    }
}
