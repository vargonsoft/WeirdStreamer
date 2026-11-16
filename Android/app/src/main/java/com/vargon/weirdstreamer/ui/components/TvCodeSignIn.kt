package com.vargon.weirdstreamer.ui.components

import android.os.SystemClock
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.TextField
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.tv.material3.*
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.repository.normalizePairingCode
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// Signing a TV in with a code: the TV shows a short code, and an account signed in on a phone or
// the web enters it (Settings, My Account › Sign in on a TV). The server hands the TV that account's
// token, which is saved as a password sign-in saves its own.

/** How long a code is shown before the TV makes a new one (the server's limit too). */
private const val CODE_LIFETIME_MS = 5 * 60_000L

/** How often the TV asks whether its code has been entered. */
private const val POLL_MS = 2_000L

/**
 * The TV's sign-in by code: makes a code, shows it large, and asks the server every 2 seconds
 * whether it was entered, while this is on screen. An approved code signs the TV in
 * ([onSignedIn], with the token saved); an expired one, or one shown 5 minutes, is replaced.
 */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TvCodeSignInDialog(host: String, server: ServerRepository, onSignedIn: () -> Unit, onUsePassword: () -> Unit) {
    var code by remember { mutableStateOf<String?>(null) }
    var deadline by remember { mutableLongStateOf(0L) }
    var problem by remember { mutableStateOf<String?>(null) }
    var now by remember { mutableLongStateOf(SystemClock.elapsedRealtime()) }
    val passwordFocus = remember { FocusRequester() }
    val signedIn by rememberUpdatedState(onSignedIn)

    BackHandler(onBack = onUsePassword)

    // Cancelled when the dialog leaves the screen.
    LaunchedEffect(server) {
        while (true) {
            code = null
            val made = server.createPairing()
            val pairing = made.getOrNull()
            if (pairing == null) {
                problem = made.exceptionOrNull()?.localizedMessage ?: "Couldn't get a code"
                delay(15_000)
                continue
            }
            problem = null
            code = pairing.code
            deadline = SystemClock.elapsedRealtime() + CODE_LIFETIME_MS
            while (SystemClock.elapsedRealtime() < deadline) {
                delay(POLL_MS)
                // A failed request (the network, say) is tried again at the next poll.
                val status = server.pollPairing(pairing.code).getOrNull() ?: continue
                when (status.status) {
                    "approved" -> {
                        val token = status.token ?: break
                        server.signInWithToken(token)
                        signedIn()
                        return@LaunchedEffect
                    }
                    "expired" -> break
                }
            }
        }
    }
    // The countdown
    LaunchedEffect(Unit) {
        while (true) {
            now = SystemClock.elapsedRealtime()
            delay(1000)
        }
    }
    LaunchedEffect(Unit) { passwordFocus.requestFocus() }

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Surface(modifier = Modifier.width(600.dp)) {
            Column(modifier = Modifier.padding(32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Text("Sign in with a code", style = MaterialTheme.typography.headlineSmall,
                    modifier = Modifier.fillMaxWidth())
                Text("Host: $host", style = MaterialTheme.typography.bodySmall, modifier = Modifier.fillMaxWidth())
                Spacer(modifier = Modifier.height(24.dp))
                Text(
                    code ?: "·····",
                    fontSize = 72.sp,
                    fontWeight = FontWeight.Bold,
                    fontFamily = FontFamily.Monospace,
                    letterSpacing = 12.sp,
                    color = WeirdStreamerTheme.spec.headingColor.takeIf { code != null } ?: Color.Gray
                )
                Spacer(modifier = Modifier.height(8.dp))
                val left = ((deadline - now).coerceAtLeast(0) + 999) / 1000
                Text(
                    when {
                        problem != null -> problem!!
                        code == null -> "Getting a code…"
                        else -> "Code expires in %d:%02d".format(left / 60, left % 60)
                    },
                    style = MaterialTheme.typography.bodyMedium,
                    color = if (problem != null) Color.Red else MaterialTheme.colorScheme.onSurfaceVariant
                )
                Spacer(modifier = Modifier.height(24.dp))
                Text(
                    "Enter this code in Settings on a phone, or on the web, under My Account › Sign in on a TV.",
                    style = MaterialTheme.typography.bodyLarge,
                    textAlign = TextAlign.Center
                )
                Spacer(modifier = Modifier.height(32.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    TvSecondaryButton(onClick = onUsePassword, modifier = Modifier.focusRequester(passwordFocus)) {
                        Text("Use a password")
                    }
                }
            }
        }
    }
}

/**
 * Settings' "Sign in on a TV" on the TV: the code another TV shows, entered with the TV's keyboard,
 * signs that TV in as this account.
 */
@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun TvSignInTvSection(server: ServerRepository, fieldModifier: Modifier = Modifier, buttonModifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val keyboard = LocalSoftwareKeyboardController.current
    var code by remember { mutableStateOf("") }
    var message by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    var busy by remember { mutableStateOf(false) }
    fun claim() {
        val clean = normalizePairingCode(code)
        if (clean.length != 5) { message = "Enter the 5-character code the TV shows." to true; return }
        if (busy) return
        busy = true
        scope.launch {
            val result = server.claimPairing(clean)
            busy = false
            message = if (result.isSuccess) { code = ""; "The TV is signing in." to false }
                else (result.exceptionOrNull()?.message ?: "Couldn't sign the TV in") to true
        }
    }

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp)) {
        Text("Sign in on a TV", style = MaterialTheme.typography.titleMedium)
        Text("Enter the code another TV shows on its sign-in screen to sign it in as ${server.signedInAs().first ?: "this account"}.",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(modifier = Modifier.height(12.dp))
        // The button below the field: Right in a field moves its cursor, Down leaves it.
        Column {
            TextField(
                colors = tvTextFieldColors(),
                value = code,
                onValueChange = { code = it.uppercase().take(8) },
                label = { FieldText("Code") },
                singleLine = true,
                keyboardOptions = pairingCodeKeyboard(),
                keyboardActions = KeyboardActions(onDone = { keyboard?.hide(); claim() }),
                modifier = Modifier.width(240.dp).tvFieldLeavesOnUpDown().then(fieldModifier)
            )
            Spacer(modifier = Modifier.height(12.dp))
            Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { claim() }, modifier = buttonModifier) {
                Text("Sign in on the TV")
            }
        }
        message?.let { (text, isError) ->
            Spacer(modifier = Modifier.height(8.dp))
            Text(text, style = MaterialTheme.typography.bodyMedium,
                color = if (isError) Color.Red else WeirdStreamerTheme.spec.headingColor)
        }
    }
}

/**
 * Up and Down leave a one-line TV text field for the control above or below it (a field keeps
 * them otherwise, unless they come from a D-pad device), so the D-pad reaches the dialog's buttons.
 */
@Composable
fun Modifier.tvFieldLeavesOnUpDown(): Modifier {
    val focusManager = LocalFocusManager.current
    return onPreviewKeyEvent { event ->
        if (event.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
        when (event.key) {
            Key.DirectionDown -> focusManager.moveFocus(FocusDirection.Down)
            Key.DirectionUp -> focusManager.moveFocus(FocusDirection.Up)
            else -> false
        }
    }
}

/** The keyboard for a TV's sign-in code: capitals, no suggestions. */
fun pairingCodeKeyboard() = KeyboardOptions(
    capitalization = KeyboardCapitalization.Characters,
    autoCorrectEnabled = false,
    keyboardType = KeyboardType.Ascii,
    imeAction = ImeAction.Done
)
