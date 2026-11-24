package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Pin
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.vargon.weirdstreamer.AccountPins
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.accountLabel
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay

// Account PINs (AccountPins): asked for when an account with one is opened from the source list,
// and set in Settings › Account. A phone's are digits; a TV's a code of D-pad directions, of which
// the screen shows only that a direction was pressed (a dot flashes), never which.

/** A wait after wrong PINs, counting down; null when there's none. */
@Composable
private fun lockMessage(until: Long): String? {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(until) {
        while (until > System.currentTimeMillis()) { now = System.currentTimeMillis(); delay(500) }
        now = System.currentTimeMillis()
    }
    return if (until > now) "Too many wrong tries. Try again in ${AccountPins.wait(until)}." else null
}

private fun AccountPins.Check.lockedUntil(): Long = when (this) {
    is AccountPins.Check.Wrong -> lockedUntil
    is AccountPins.Check.Locked -> until
    AccountPins.Check.Right -> 0
}

// ---------------------------------------------------------------------------------------------
// Phone

/** The PIN asked for when an account with one is opened (MediaViewModel.pinRequest). */
@Composable
fun PhonePinPrompt(viewModel: MediaViewModel) {
    val request = viewModel.pinRequest ?: return
    var pin by remember(request) { mutableStateOf("") }
    var wrong by remember(request) { mutableStateOf(false) }
    var lockedUntil by remember(request) { mutableLongStateOf(viewModel.pins.lockedUntil(request.address, request.username)) }
    val locked = lockMessage(lockedUntil)
    val field = remember { FocusRequester() }
    LaunchedEffect(request) { runCatching { field.requestFocus() } }
    fun submit() {
        if (pin.isEmpty() || locked != null) return
        val result = viewModel.pinEntered(pin)
        if (result !is AccountPins.Check.Right) { wrong = result is AccountPins.Check.Wrong; lockedUntil = result.lockedUntil(); pin = "" }
    }
    androidx.compose.material3.AlertDialog(
        onDismissRequest = { viewModel.cancelPin() },
        title = { androidx.compose.material3.Text("${accountLabel(request.username)}'s PIN") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                androidx.compose.material3.OutlinedTextField(
                    pin, { pin = it.filter(Char::isDigit); wrong = false }, label = { androidx.compose.material3.Text("PIN") },
                    singleLine = true, visualTransformation = PasswordVisualTransformation(), enabled = locked == null,
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { submit() }),
                    modifier = Modifier.fillMaxWidth().focusRequester(field)
                )
                val note = locked ?: if (wrong) "That PIN isn't right." else null
                note?.let { androidx.compose.material3.Text(it, color = androidx.compose.material3.MaterialTheme.colorScheme.error) }
                androidx.compose.material3.TextButton(onClick = { viewModel.usePasswordInstead() }) {
                    androidx.compose.material3.Text("Forgot it? Use the password instead")
                }
            }
        },
        confirmButton = { PhoneButton(onClick = ::submit) { androidx.compose.material3.Text("Open") } },
        dismissButton = { PhoneSecondaryButton(onClick = { viewModel.cancelPin() }) { androidx.compose.material3.Text("Cancel") } }
    )
}

/** Settings › Account on a phone: this account's PIN here (set, change or remove it). */
@Composable
fun PhonePinSettings(address: String, username: String) {
    val pins = AccountPins.get(LocalContext.current)
    var version by remember { mutableIntStateOf(0) }
    val has = remember(version) { pins.has(address, username) }
    var editing by remember { mutableStateOf<Boolean?>(null) } // true: set or change, false: remove
    var done by remember { mutableStateOf<String?>(null) }
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        androidx.compose.material3.Text(
            if (has) "Opening ${accountLabel(username)} on this phone asks for its PIN."
            else "Anyone using this phone can open ${accountLabel(username)}. A PIN of ${AccountPins.MIN_LENGTH} or more digits keeps it to you.",
            style = androidx.compose.material3.MaterialTheme.typography.bodyMedium)
        done?.let { androidx.compose.material3.Text(it, color = androidx.compose.material3.MaterialTheme.colorScheme.primary) }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            PhoneButton(onClick = { editing = true; done = null }) { androidx.compose.material3.Text(if (has) "Change PIN" else "Set a PIN") }
            if (has) PhoneSecondaryButton(onClick = { editing = false; done = null }) { androidx.compose.material3.Text("No PIN") }
        }
    }
    editing?.let { setting ->
        PhonePinEditDialog(pins, address, username, has, setting, onDismiss = { editing = null }) { message ->
            editing = null; version++; done = message
        }
    }
}

@Composable
private fun PhonePinEditDialog(pins: AccountPins, address: String, username: String, has: Boolean, setting: Boolean,
                               onDismiss: () -> Unit, onDone: (String) -> Unit) {
    var current by remember { mutableStateOf("") }
    var pin by remember { mutableStateOf("") }
    var confirm by remember { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    var lockedUntil by remember { mutableLongStateOf(pins.lockedUntil(address, username)) }
    val locked = if (has) lockMessage(lockedUntil) else null
    fun save() {
        if (locked != null) return
        if (has) {
            val result = pins.check(address, username, current)
            if (result !is AccountPins.Check.Right) {
                lockedUntil = result.lockedUntil(); current = ""
                error = if (result is AccountPins.Check.Wrong) "The current PIN isn't right." else null
                return
            }
        }
        if (!setting) { pins.clear(address, username); onDone("No PIN: anyone using this phone can open ${accountLabel(username)}."); return }
        when {
            pin.length < AccountPins.MIN_LENGTH -> error = "Use ${AccountPins.MIN_LENGTH} or more digits."
            pin != confirm -> error = "The PINs don't match."
            else -> { pins.set(address, username, pin, AccountPins.Kind.Digits); onDone("PIN saved.") }
        }
    }
    val digits = KeyboardOptions(keyboardType = KeyboardType.NumberPassword, imeAction = ImeAction.Next)
    @Composable
    fun field(value: String, label: String, set: (String) -> Unit) = androidx.compose.material3.OutlinedTextField(
        value, { set(it.filter(Char::isDigit)); error = null }, label = { androidx.compose.material3.Text(label) }, singleLine = true,
        visualTransformation = PasswordVisualTransformation(), keyboardOptions = digits, modifier = Modifier.fillMaxWidth())
    androidx.compose.material3.AlertDialog(
        onDismissRequest = onDismiss,
        title = { androidx.compose.material3.Text(if (!setting) "Remove the PIN" else if (has) "Change the PIN" else "Set a PIN") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (has) field(current, "Current PIN") { current = it }
                if (setting) {
                    field(pin, "New PIN (${AccountPins.MIN_LENGTH} or more digits)") { pin = it }
                    field(confirm, "New PIN again") { confirm = it }
                }
                (locked ?: error)?.let { androidx.compose.material3.Text(it, color = androidx.compose.material3.MaterialTheme.colorScheme.error) }
            }
        },
        confirmButton = { PhoneButton(onClick = ::save, destructive = !setting) { androidx.compose.material3.Text(if (setting) "Save" else "Remove") } },
        dismissButton = { PhoneSecondaryButton(onClick = onDismiss) { androidx.compose.material3.Text("Cancel") } }
    )
}

// ---------------------------------------------------------------------------------------------
// TV

/**
 * Takes a code of D-pad directions: each press flashes the dot (never which direction), OK ends it,
 * Back clears what's been pressed (or, with nothing pressed, leaves). Holding OK runs [onHoldOk].
 * The OK that opened it is ignored: only one pressed while it's open counts.
 */
@Composable
private fun TvDirectionCode(title: String, guide: String, message: String?, isError: Boolean, enabled: Boolean,
                            onHoldOk: (() -> Unit)?, onCancel: () -> Unit, onDone: (String) -> Unit) {
    val spec = WeirdStreamerTheme.spec
    val panel = spec.panelColor.takeOrElse { androidx.tv.material3.MaterialTheme.colorScheme.surface }
    val accent = spec.focusColor.takeOrElse { Color.White }
    val text = androidx.tv.material3.MaterialTheme.colorScheme.onSurface
    var code by remember { mutableStateOf("") }
    var flashes by remember { mutableIntStateOf(0) }
    var lit by remember { mutableStateOf(false) }
    var okDown by remember { mutableStateOf(false) }
    var held by remember { mutableStateOf(false) }
    var cleared by remember { mutableStateOf(false) }
    LaunchedEffect(flashes) { if (flashes > 0) { lit = true; delay(250); lit = false } }
    LaunchedEffect(message) { code = "" } // a new try starts empty
    val focus = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
    // Back reaches the dialog as a dismiss request: it clears what's been pressed, or leaves.
    fun back() { if (code.isNotEmpty()) { code = ""; cleared = true } else onCancel() }
    Dialog(onDismissRequest = ::back, properties = DialogProperties(usePlatformDefaultWidth = false, dismissOnClickOutside = false)) {
        Column(
            Modifier.width(720.dp).background(panel, RoundedCornerShape(16.dp)).padding(32.dp)
                .focusRequester(focus)
                .onPreviewKeyEvent { e ->
                    val down = e.type == KeyEventType.KeyDown
                    when (e.key) {
                        Key.DirectionUp, Key.DirectionDown, Key.DirectionLeft, Key.DirectionRight -> {
                            if (down && enabled) {
                                code += when (e.key) { Key.DirectionUp -> "U"; Key.DirectionDown -> "D"; Key.DirectionLeft -> "L"; else -> "R" }
                                cleared = false
                                flashes++
                            }
                            true
                        }
                        Key.DirectionCenter, Key.Enter, Key.NumPadEnter -> {
                            if (down) {
                                if (e.nativeKeyEvent.repeatCount == 0) okDown = true
                                else if (okDown && !held && onHoldOk != null) { held = true; onHoldOk() }
                            } else if (okDown) {
                                okDown = false
                                if (held) held = false
                                else if (enabled && code.isNotEmpty()) { val c = code; code = ""; onDone(c) }
                            }
                            true
                        }
                        else -> false
                    }
                }
                .focusable(),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            androidx.tv.material3.Text(title, style = androidx.tv.material3.MaterialTheme.typography.headlineSmall,
                color = spec.headingColor.takeOrElse { text })
            androidx.tv.material3.Text(guide, style = androidx.tv.material3.MaterialTheme.typography.bodyLarge, color = text,
                textAlign = TextAlign.Center)
            Box(Modifier.height(56.dp), contentAlignment = Alignment.Center) {
                Box(Modifier.size(28.dp).background(if (lit) accent else accent.copy(alpha = 0.15f), CircleShape))
            }
            val line = message ?: if (cleared) "Cleared: start again." else null
            androidx.tv.material3.Text(line ?: " ", style = androidx.tv.material3.MaterialTheme.typography.bodyLarge,
                color = if (isError && line == message) androidx.tv.material3.MaterialTheme.colorScheme.error else text, textAlign = TextAlign.Center)
            androidx.tv.material3.Text(
                "Back clears what you've pressed; with nothing pressed, it closes." +
                    if (onHoldOk != null) " Hold OK to sign in with the password instead." else "",
                style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                color = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
        }
    }
}

/** The TV's code asked for when an account with one is opened (MediaViewModel.pinRequest). */
@Composable
fun TvPinPrompt(viewModel: MediaViewModel) {
    val request = viewModel.pinRequest ?: return
    var wrong by remember(request) { mutableStateOf(false) }
    var tries by remember(request) { mutableIntStateOf(0) }
    var lockedUntil by remember(request) { mutableLongStateOf(viewModel.pins.lockedUntil(request.address, request.username)) }
    val locked = lockMessage(lockedUntil)
    TvDirectionCode(
        title = "${accountLabel(request.username)}'s code",
        guide = "Press the arrows of the code, then OK.",
        // A new message each wrong try, so the line changes even when the words don't
        message = locked ?: if (wrong) "That code isn't right." + "\u200B".repeat(tries % 2) else null,
        isError = locked != null || wrong,
        enabled = locked == null,
        onHoldOk = { viewModel.usePasswordInstead() },
        onCancel = { viewModel.cancelPin() },
    ) { code ->
        val result = viewModel.pinEntered(code)
        if (result !is AccountPins.Check.Right) {
            wrong = result is AccountPins.Check.Wrong; tries++; lockedUntil = result.lockedUntil()
        }
    }
}

/** Settings › Account on a TV: this account's code here (set, change or remove it). */
@Composable
fun TvPinSettingsRow(address: String, username: String) {
    val context = LocalContext.current
    val pins = AccountPins.get(context)
    var version by remember { mutableIntStateOf(0) }
    val has = remember(version) { pins.has(address, username) }
    var menu by remember { mutableStateOf(false) }
    var flow by remember { mutableStateOf<Boolean?>(null) } // true: set or change, false: remove
    TvSettingsRow("Code to open ${accountLabel(username)}",
        if (has) "On: opening ${accountLabel(username)} on this TV asks for its code" else "Off: anyone using this TV can open ${accountLabel(username)}",
        onClick = { if (has) menu = true else flow = true }) {
        androidx.tv.material3.Icon(Icons.Default.Pin, contentDescription = null)
    }
    if (menu) {
        TvDialogFrame("Code to open ${accountLabel(username)}", onDismiss = { menu = false }, width = 560) {
            val first = remember { FocusRequester() }
            LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
            TvValueRow("Change the code", focus = first) { menu = false; flow = true }
            TvValueRow("Remove the code", destructive = true) { menu = false; flow = false }
        }
    }
    flow?.let { setting ->
        TvPinEditFlow(pins, address, username, has, setting, onDismiss = { flow = null }) { message ->
            flow = null; version++
            android.widget.Toast.makeText(context, message, android.widget.Toast.LENGTH_SHORT).show()
        }
    }
}

/** Setting, changing or removing a TV's code: the current one first (if any), then the new one twice. */
@Composable
private fun TvPinEditFlow(pins: AccountPins, address: String, username: String, has: Boolean, setting: Boolean,
                          onDismiss: () -> Unit, onDone: (String) -> Unit) {
    // 0: the current code, 1: the new one, 2: the new one again
    var step by remember { mutableIntStateOf(if (has) 0 else 1) }
    var first by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<String?>(null) }
    var isError by remember { mutableStateOf(false) }
    var lockedUntil by remember { mutableLongStateOf(pins.lockedUntil(address, username)) }
    val locked = if (step == 0) lockMessage(lockedUntil) else null
    val name = accountLabel(username)
    val (title, guide) = when (step) {
        0 -> "$name's current code" to "Press the arrows of the current code, then OK."
        1 -> "A new code for $name" to "Press ${AccountPins.MIN_LENGTH} or more arrows (up, down, left and right, in any order), then OK. " +
            "The screen shows only that one was pressed."
        else -> "The new code again" to "Press the same arrows again, then OK."
    }
    androidx.compose.runtime.key(step) {
        TvDirectionCode(title, guide, locked ?: message, isError || locked != null, enabled = locked == null,
            onHoldOk = null, onCancel = onDismiss) { code ->
            when (step) {
                0 -> {
                    val result = pins.check(address, username, code)
                    if (result is AccountPins.Check.Right) {
                        if (!setting) { pins.clear(address, username); onDone("Code removed: anyone using this TV can open $name.") }
                        else { step = 1; message = null; isError = false }
                    } else {
                        lockedUntil = result.lockedUntil()
                        message = if (result is AccountPins.Check.Wrong) "That isn't the current code." else null; isError = true
                    }
                }
                1 -> if (code.length < AccountPins.MIN_LENGTH) {
                    message = "That's ${code.length}: use ${AccountPins.MIN_LENGTH} or more."; isError = true
                } else { first = code; step = 2; message = null; isError = false }
                else -> if (code == first) {
                    pins.set(address, username, code, AccountPins.Kind.Directions)
                    onDone("Code saved: opening $name on this TV asks for it.")
                } else { step = 1; first = ""; message = "They didn't match: start again."; isError = true }
            }
        }
    }
}
