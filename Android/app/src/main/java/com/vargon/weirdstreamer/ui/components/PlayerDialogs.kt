package com.vargon.weirdstreamer.ui.components

import android.util.Log
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Search
import java.util.Locale
import kotlinx.coroutines.delay
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Surface
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.foundation.focusable
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.tv.material3.*
import com.vargon.weirdstreamer.api.models.SubtitleHit
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import java.io.File
import java.util.*

@Composable
fun SpeedDialog(
    show: Boolean,
    currentSpeedIndex: Int,
    speedIntervals: List<Float>,
    onDismiss: () -> Unit,
    onSpeedSelected: (Int, Float) -> Unit
) {
    if (!show) return
    var pendingSpeedIndex by remember { mutableIntStateOf(currentSpeedIndex) }
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(300.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionUp -> {
                                if (pendingSpeedIndex > 0) pendingSpeedIndex--
                                true
                            }
                            Key.DirectionDown -> {
                                if (pendingSpeedIndex < speedIntervals.size - 1) pendingSpeedIndex++
                                true
                            }
                            Key.DirectionCenter, Key.Enter -> {
                                onSpeedSelected(pendingSpeedIndex, speedIntervals[pendingSpeedIndex])
                                true
                            }
                            Key.Escape, Key.Back, Key.Backspace -> {
                                onDismiss()
                                true
                            }
                            else -> false
                        }
                    } else false
                },
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(modifier = Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text("Playback Speed", style = MaterialTheme.typography.headlineSmall, color = Color.White)
                Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    speedIntervals.forEachIndexed { index, speed ->
                        Row(
                            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp))
                                .background(if (pendingSpeedIndex == index) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(alpha = 0.2f))
                                .clickable { pendingSpeedIndex = index }
                                .height(48.dp).padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            if (pendingSpeedIndex == index) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, tint = Color.White, modifier = Modifier.size(20.dp))
                                Spacer(modifier = Modifier.width(8.dp))
                            }
                            Text("${speed}x", color = Color.White, style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun TranscodeDialog(
    show: Boolean,
    selectedModeIndex: Int,
    transcodeModes: List<TranscodeMode>,
    status: String? = null, // what the stream does now
    onDismiss: () -> Unit,
    onModeSelected: (Int) -> Unit
) {
    if (!show) return
    var pendingModeIndex by remember { mutableIntStateOf(selectedModeIndex) }
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(400.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
                .onPreviewKeyEvent { event ->
                    if (event.type == KeyEventType.KeyDown) {
                        when (event.key) {
                            Key.DirectionUp -> {
                                if (pendingModeIndex > 0) pendingModeIndex--
                                true
                            }
                            Key.DirectionDown -> {
                                if (pendingModeIndex < transcodeModes.size - 1) pendingModeIndex++
                                true
                            }
                            Key.DirectionCenter, Key.Enter -> {
                                onModeSelected(pendingModeIndex)
                                true
                            }
                            Key.Escape, Key.Back, Key.Backspace -> {
                                onDismiss()
                                true
                            }
                            else -> false
                        }
                    } else false
                },
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(modifier = Modifier.padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Text("Transcoding Options", style = MaterialTheme.typography.headlineSmall, color = Color.White)
                if (status != null) Text(status, style = MaterialTheme.typography.bodySmall, color = Color.White.copy(alpha = 0.8f))
                // Keeps the highlighted mode on screen as Up/Down move it (the list can be taller
                // than the TV's screen).
                val listScroll = rememberScrollState()
                val rowPx = with(androidx.compose.ui.platform.LocalDensity.current) { 56.dp.roundToPx() }
                LaunchedEffect(pendingModeIndex) {
                    listScroll.animateScrollTo((pendingModeIndex * rowPx - listScroll.viewportSize / 2 + rowPx / 2).coerceAtLeast(0))
                }
                Column(modifier = Modifier.fillMaxWidth().verticalScroll(listScroll), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    transcodeModes.forEachIndexed { index, mode ->
                        Row(
                            modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp))
                                .background(if (pendingModeIndex == index) Color.White.copy(alpha = 0.2f) else Color.Gray.copy(alpha = 0.2f))
                                .clickable { pendingModeIndex = index }
                                .height(48.dp).padding(12.dp),
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            if (pendingModeIndex == index) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, tint = Color.White, modifier = Modifier.size(20.dp))
                                Spacer(modifier = Modifier.width(8.dp))
                            }
                            Text(mode.name, color = Color.White, style = MaterialTheme.typography.bodyMedium)
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun SubtitleDialog(
    show: Boolean,
    availableSubtitles: List<String>,
    selectedSubtitleIndex: Int,
    onDismiss: () -> Unit,
    onSubtitleSelected: (Int) -> Unit,
    // Search OpenSubtitles through the server (videos from a WeirdStreamer server only)
    onDownloadRequested: (() -> Unit)?,
    // The text track's timing ("In sync", "250 ms earlier"), which opens the timing panel; null: none
    timing: String? = null,
    onTiming: () -> Unit = {},
) {
    if (!show) return
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(
            dismissOnBackPress = true,
            dismissOnClickOutside = false
        )
    ) {
        Surface(
            modifier = Modifier
                .width(680.dp)
                .heightIn(max = 600.dp) // the TV's 1080p screen is 540dp tall: the list scrolls inside
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text(
                    text = "Subtitles",
                    style = MaterialTheme.typography.headlineSmall,
                    color = Color.White
                )

                // The row the remote is on fills white; the subtitle in use has a check.
                val currentRequester = remember { FocusRequester() }
                LaunchedEffect(Unit) { runCatching { currentRequester.requestFocus() } }
                if (availableSubtitles.isNotEmpty()) {
                    Text(
                        text = "Available Subtitles",
                        style = MaterialTheme.typography.bodyLarge,
                        color = Color.White.copy(alpha = 0.8f)
                    )

                    // A long list (Blu-rays carry dozens) scrolls inside its share of the dialog, so
                    // the download row and Close stay on screen below it.
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f, fill = false)
                            .verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        SubtitleRow("None", selected = selectedSubtitleIndex == -1,
                            requester = if (selectedSubtitleIndex == -1) currentRequester else null) { onSubtitleSelected(-1) }
                        availableSubtitles.forEachIndexed { index, subtitle ->
                            SubtitleRow(subtitle, selected = selectedSubtitleIndex == index,
                                requester = if (selectedSubtitleIndex == index) currentRequester else null) { onSubtitleSelected(index) }
                        }
                    }

                    HorizontalDivider(color = Color.White.copy(alpha = 0.3f), thickness = 1.dp)
                }

                if (timing != null) {
                    SubtitleRow("Timing: $timing", selected = false, icon = Icons.Default.Schedule, requester = null) { onTiming() }
                }
                if (onDownloadRequested != null) {
                    SubtitleRow("Search online (OpenSubtitles)", selected = false, icon = Icons.Default.Search,
                        requester = if (availableSubtitles.isEmpty()) currentRequester else null) { onDownloadRequested() }
                } else if (availableSubtitles.isEmpty()) {
                    Text("This video has no subtitles.", color = Color.White.copy(alpha = 0.8f),
                        style = MaterialTheme.typography.bodyLarge)
                }

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.End
                ) {
                    TvSecondaryButton(
                        onClick = onDismiss
                    ) {
                        Text("Close")
                    }
                }
            }
        }
    }
}

/**
 * Subtitles found online (OpenSubtitles, through the server), best first; choosing one downloads
 * it to the server, which keeps it for next time.
 */
@Composable
fun OnlineSubtitlesDialog(
    show: Boolean,
    isSearching: Boolean,
    status: String?,
    note: String?,
    results: List<SubtitleHit>,
    onDismiss: () -> Unit,
    onChosen: (SubtitleHit) -> Unit
) {
    if (!show) return
    Dialog(
        onDismissRequest = onDismiss,
        properties = DialogProperties(dismissOnBackPress = true, dismissOnClickOutside = false)
    ) {
        Surface(
            modifier = Modifier
                .width(760.dp)
                .heightIn(max = 600.dp)
                .background(Color.Black.copy(alpha = 0.9f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp)),
            color = Color.Black.copy(alpha = 0.9f)
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text("Subtitles online", style = MaterialTheme.typography.headlineSmall, color = Color.White)
                val line = status ?: note
                if (isSearching || line != null) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        if (isSearching) {
                            CircularProgressIndicator(color = Color.White, modifier = Modifier.size(24.dp))
                            Spacer(Modifier.width(12.dp))
                        }
                        Text(line ?: "Searching…", color = Color.White.copy(alpha = 0.8f), style = MaterialTheme.typography.bodyLarge)
                    }
                }
                val firstRequester = remember { FocusRequester() }
                LaunchedEffect(results) { if (results.isNotEmpty()) runCatching { firstRequester.requestFocus() } }
                if (results.isNotEmpty()) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .weight(1f, fill = false)
                            .verticalScroll(rememberScrollState()),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        results.forEachIndexed { i, hit ->
                            SubtitleRow(hit.release, selected = false, icon = Icons.Default.Download,
                                requester = if (i == 0) firstRequester else null,
                                detail = subtitleHitDetails(hit)) { onChosen(hit) }
                        }
                    }
                }
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    TvSecondaryButton(onClick = onDismiss) { Text("Close") }
                }
            }
        }
    }
}

/** A search result's language and marks: "English · 140,606 downloads · trusted · SDH". */
fun subtitleHitDetails(hit: SubtitleHit): String = listOfNotNull(
    languageName(hit.languageCode) ?: hit.language,
    "%,d downloads".format(Locale.getDefault(), hit.downloads),
    if (hit.hashMatch) "made for this file" else null,
    if (hit.trusted) "trusted" else null,
    if (hit.hearingImpaired) "SDH" else null,
    if (hit.foreignPartsOnly) "foreign parts only" else null,
    if (hit.machineTranslated) "machine translated" else null,
    if (hit.downloaded) "already downloaded" else null,
).joinToString(" · ")

/**
 * A subtitle choice: solid white with dark text while the remote is on it, a check when it's the
 * one in use, so both read at a glance on a TV.
 */
@Composable
private fun SubtitleRow(
    label: String,
    selected: Boolean,
    requester: FocusRequester?,
    icon: androidx.compose.ui.graphics.vector.ImageVector? = null,
    detail: String? = null,
    onClick: () -> Unit
) {
    var focused by remember { mutableStateOf(false) }
    val content = if (focused) Color.Black else Color.White
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(
                when {
                    focused -> Color.White
                    selected -> Color.White.copy(alpha = 0.25f)
                    else -> Color.Gray.copy(alpha = 0.2f)
                }
            )
            .then(if (requester != null) Modifier.focusRequester(requester) else Modifier)
            .onFocusChanged { focused = it.hasFocus }
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        icon?.let {
            Icon(it, contentDescription = null, tint = content, modifier = Modifier.size(22.dp))
            Spacer(Modifier.width(10.dp))
        }
        Column(Modifier.weight(1f)) {
            Text(label, color = content, style = MaterialTheme.typography.bodyLarge,
                fontWeight = if (selected) androidx.compose.ui.text.font.FontWeight.Bold else null)
            detail?.let { Text(it, color = content.copy(alpha = 0.75f), style = MaterialTheme.typography.bodySmall) }
        }
        if (selected) {
            Icon(Icons.Default.Check, contentDescription = "In use", tint = content, modifier = Modifier.size(24.dp))
        }
    }
}

/**
 * Subtitle timing on the TV, near the top so the subtitles below show the change: Left when the
 * line was already said before its subtitle showed (subtitles earlier), Right when it's not said
 * yet (later); Up and Down change how far each press moves them. OK or Back closes it.
 */
// The theme's gold and aubergine, for the side a press just moved the subtitles
private val TimingFlash = Color(0xFFF6C445)
private val TimingFlashText = Color(0xFF321B2F)

@Composable
fun SubtitleTimingDialog(show: Boolean, offsetMs: Int, onShift: (Int) -> Unit, onDismiss: () -> Unit) {
    if (!show) return
    var step by remember { mutableIntStateOf(250) }
    // The side of the last left or right press (-1 earlier, 1 later), lit briefly. presses changes
    // on each one, so a quick second press restarts the flash.
    var flashSide by remember { mutableIntStateOf(0) }
    var presses by remember { mutableIntStateOf(0) }
    LaunchedEffect(presses) {
        if (presses > 0) {
            delay(200)
            flashSide = 0
        }
    }
    val requester = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { requester.requestFocus() } }
    androidx.compose.ui.window.Popup(
        alignment = Alignment.TopCenter,
        onDismissRequest = onDismiss,
        properties = androidx.compose.ui.window.PopupProperties(focusable = true)
    ) {
        Column(
            modifier = Modifier
                .padding(top = 40.dp)
                .width(760.dp)
                .background(Color.Black.copy(alpha = 0.85f), RoundedCornerShape(16.dp))
                .border(2.dp, Color.White, RoundedCornerShape(16.dp))
                .focusRequester(requester)
                .onPreviewKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    val i = SUBTITLE_STEPS.indexOf(step)
                    when (e.key) {
                        Key.DirectionLeft -> { onShift(-step); flashSide = -1; presses++; true }
                        Key.DirectionRight -> { onShift(step); flashSide = 1; presses++; true }
                        Key.DirectionUp -> { step = SUBTITLE_STEPS[(i + 1).coerceAtMost(SUBTITLE_STEPS.size - 1)]; true }
                        Key.DirectionDown -> { step = SUBTITLE_STEPS[(i - 1).coerceAtLeast(0)]; true }
                        Key.DirectionCenter, Key.Enter, Key.Back -> { onDismiss(); true }
                        else -> false
                    }
                }
                .focusable()
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text("Subtitle timing", style = MaterialTheme.typography.headlineSmall, color = Color.White)
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                val litLeft = flashSide == -1
                Box(Modifier.weight(1f).clip(RoundedCornerShape(8.dp))
                    .background(if (litLeft) TimingFlash else Color.Transparent).padding(horizontal = 12.dp, vertical = 6.dp),
                    contentAlignment = Alignment.CenterStart) {
                    Text("‹  Already said", color = if (litLeft) TimingFlashText else Color.White,
                        style = MaterialTheme.typography.titleMedium)
                }
                Text(subtitleOffsetText(offsetMs), color = Color.White, style = MaterialTheme.typography.titleLarge)
                val litRight = flashSide == 1
                Box(Modifier.weight(1f).clip(RoundedCornerShape(8.dp))
                    .background(if (litRight) TimingFlash else Color.Transparent).padding(horizontal = 12.dp, vertical = 6.dp),
                    contentAlignment = Alignment.CenterEnd) {
                    Text("Not said yet  ›", color = if (litRight) TimingFlashText else Color.White,
                        style = MaterialTheme.typography.titleMedium)
                }
            }
            Text("Each press: ${subtitleStepText(step)}   (▲ ▼ to change)", color = Color.White.copy(alpha = 0.8f),
                style = MaterialTheme.typography.bodyMedium)
            Text("Already said: the line was spoken before its subtitle showed. Not said yet: the subtitle showed first.",
                color = Color.White.copy(alpha = 0.6f), style = MaterialTheme.typography.bodySmall)
        }
    }
}
