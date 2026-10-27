package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.MediaSegments
import com.vargon.weirdstreamer.api.models.SegmentSpan
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay

// Skip Intro / Skip Credits: the server finds each video's intro and credits (chapters, a
// season's shared audio, dark frames) or an admin marks them. The players offer to skip for the
// first 10 seconds of either (and while their controls show during it); with the show's Auto
// Skip on, they skip by themselves. Skipping credits goes on to the next episode, else closes.

/** The part of a video the player is in, to skip: the intro, or the credits. */
data class SkipPrompt(val credits: Boolean, val segment: SegmentSpan) {
    val label: String get() = if (credits) "Skip Credits" else "Skip Intro"
}

private const val SKIP_SHOWN_MS = 10_000L

/**
 * The Skip button for a player at [positionMs] (absolute, in a transcode too): the prompt and the
 * video's segments while it should show, else null. With the show's Auto Skip on, [onSkip] runs
 * by itself instead, once per part.
 */
@Composable
fun rememberSkipPrompt(
    viewModel: MediaViewModel,
    path: String,
    positionMs: Long,
    controlsShown: Boolean,
    onSkip: (SkipPrompt, MediaSegments) -> Unit,
): Pair<SkipPrompt, MediaSegments>? {
    val segments by produceState<MediaSegments?>(null, path) { value = viewModel.segmentsFor(path) }
    val s = segments
    val t = positionMs / 1000.0
    val prompt = when {
        s == null -> null
        s.intro != null && t >= s.intro.start && t < s.intro.end - 0.5 -> SkipPrompt(false, s.intro)
        s.credits != null && t >= s.credits.start && t < s.credits.end - 0.5 -> SkipPrompt(true, s.credits)
        else -> null
    }
    var fresh by remember(prompt) { mutableStateOf(true) }
    val autoSkipped = remember(path) { mutableSetOf<SkipPrompt>() }
    val skip by rememberUpdatedState(onSkip)
    LaunchedEffect(prompt) {
        if (prompt == null || s == null) return@LaunchedEffect
        if (s.autoSkip && autoSkipped.add(prompt)) {
            skip(prompt, s)
            return@LaunchedEffect
        }
        delay(SKIP_SHOWN_MS)
        fresh = false
    }
    return if (prompt != null && s != null && (fresh || controlsShown)) prompt to s else null
}

/** The TV's Skip button: drawn focused, since OK presses it while the controls are hidden. */
@Composable
fun TvSkipButton(prompt: SkipPrompt, modifier: Modifier = Modifier) {
    val spec = WeirdStreamerTheme.spec
    Box(
        modifier
            .background(spec.focusColor.takeOrElse { Color.White }, AppButtonShape)
            .padding(horizontal = 28.dp, vertical = 14.dp)
    ) {
        androidx.tv.material3.Text(prompt.label, color = spec.focusLabel.takeOrElse { Color.Black },
            style = androidx.tv.material3.MaterialTheme.typography.titleMedium)
    }
}

/** The phone's Skip button. */
@Composable
fun PhoneSkipButton(prompt: SkipPrompt, onClick: () -> Unit, modifier: Modifier = Modifier) {
    androidx.compose.material3.Button(onClick = onClick, shape = AppButtonShape, colors = phoneButtonColors(), modifier = modifier) {
        androidx.compose.material3.Text(prompt.label)
    }
}
