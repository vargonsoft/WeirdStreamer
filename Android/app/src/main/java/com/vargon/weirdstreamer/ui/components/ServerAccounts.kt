package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.accountLabel
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Dns
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.CredentialManager.ServerAccount
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.repository.MediaSource
import com.vargon.weirdstreamer.repository.SourceProvider
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

// The WeirdStreamer servers on the source list: one row per server, its card fixed on the left
// and the accounts signed in to it on this device beside it, as round buttons, then "Add user".
// Choosing an account opens the server as that account; holding one offers to sign it out.

/** The circle's letter for an account (a profile picture, later). */
private fun initialOf(name: String) = name.firstOrNull { it.isLetterOrDigit() }?.uppercaseChar()?.toString() ?: "?"

/** The server as its row shows it: host (and port), and the address in full. */
private fun serverLines(provider: SourceProvider, address: String): Pair<String, String> =
    if (provider.typeId == "SMB") address to "smb://$address" else MediaSource(provider.typeId, address).displayName() to address

/** Settings' line for a server: who's signed in to it on this device. */
fun serverUsersText(users: List<String>) = if (users.isEmpty()) "No users" else "Users: " + users.joinToString(", ")

/** What removing a server takes with it: its users' sign-ins on this device. */
fun removeServerWarning(address: String, users: List<String>) = when (users.size) {
    0 -> "Remove $address from this device?"
    1 -> "Remove $address from this device? ${users[0]} will be signed out and removed."
    else -> "Remove $address from this device? These users will be signed out and removed: ${users.joinToString(", ")}."
}

// ---- TV ----

@Composable
fun TvServerAccountsRow(
    viewModel: MediaViewModel,
    provider: SourceProvider,
    address: String,
    accounts: List<ServerAccount>,
    firstModifier: Modifier = Modifier,
    focusAccount: String? = null, // the account firstModifier goes on (else the first)
) {
    val focused = accounts.firstOrNull { it.username == focusAccount } ?: accounts.firstOrNull()
    var signOutChoice by remember { mutableStateOf<String?>(null) }
    val (name, full) = serverLines(provider, address)
    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(
            Modifier.width(280.dp).height(116.dp)
                .background(androidx.tv.material3.MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(12.dp))
                .padding(16.dp),
            verticalArrangement = Arrangement.Center
        ) {
            androidx.tv.material3.Icon(Icons.Default.Dns, contentDescription = null, modifier = Modifier.size(32.dp),
                tint = WeirdStreamerTheme.spec.serverIconTint.takeOrElse { androidx.tv.material3.LocalContentColor.current })
            androidx.tv.material3.Text(name, style = androidx.tv.material3.MaterialTheme.typography.titleMedium,
                maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 8.dp))
            if (full != name) androidx.tv.material3.Text(full, style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                color = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(24.dp),
            contentPadding = PaddingValues(horizontal = 28.dp, vertical = 8.dp),
            modifier = Modifier.weight(1f)
        ) {
            items(accounts, key = { it.username }) { account ->
                TvAccountButton(
                    label = accountLabel(account.username),
                    note = if (account.signedIn) null else "Signed out",
                    modifier = if (account == focused) firstModifier else Modifier,
                    onHold = { signOutChoice = account.username },
                    onClick = { viewModel.chooseServerAccount(provider, address, account.username) }
                ) { CircleText(initialOf(accountLabel(account.username)), it) }
            }
            item(key = "+add") {
                TvAccountButton(
                    label = "Add user",
                    modifier = if (accounts.isEmpty()) firstModifier else Modifier,
                    onClick = { viewModel.addServerAccount(provider, address) }
                ) { color -> androidx.tv.material3.Icon(Icons.Default.Add, contentDescription = null, tint = color, modifier = Modifier.size(36.dp)) }
            }
        }
    }
    signOutChoice?.let { user ->
        TvSignOutDialog(accountLabel(user), name, onDismiss = { signOutChoice = null }) {
            signOutChoice = null
            viewModel.signOutServerAccount(provider, address, user)
        }
    }
}

@Composable
private fun CircleText(text: String, color: Color) =
    androidx.tv.material3.Text(text, color = color, style = androidx.tv.material3.MaterialTheme.typography.headlineSmall)

/**
 * A round account button with its name below: plum, gold when focused (in Ember). OK opens it;
 * holding OK calls [onHold] (the key repeats while held: the first repeat counts, and the release
 * that follows is swallowed so it doesn't also open it).
 */
@Composable
private fun TvAccountButton(
    label: String,
    modifier: Modifier = Modifier,
    note: String? = null,
    onHold: (() -> Unit)? = null,
    onClick: () -> Unit,
    content: @Composable (Color) -> Unit,
) {
    val spec = WeirdStreamerTheme.spec
    val scheme = androidx.tv.material3.MaterialTheme.colorScheme
    var focused by remember { mutableStateOf(false) }
    var held by remember { mutableStateOf(false) }
    val focus = spec.focusColor.takeOrElse { scheme.inverseSurface }
    val back = if (focused) focus else spec.buttonColor.takeOrElse { scheme.secondaryContainer }
    val fore = if (focused) spec.focusLabel.takeOrElse { scheme.inverseOnSurface } else spec.buttonLabel.takeOrElse { scheme.onSecondaryContainer }
    Column(Modifier.width(110.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = modifier
                .size(76.dp)
                .onFocusChanged { focused = it.isFocused }
                .onPreviewKeyEvent { e ->
                    val select = e.key == Key.DirectionCenter || e.key == Key.Enter || e.key == Key.NumPadEnter
                    when {
                        !select || onHold == null -> false
                        e.type == KeyEventType.KeyDown && e.nativeKeyEvent.repeatCount > 0 -> {
                            if (!held) { held = true; onHold() }
                            true
                        }
                        e.type == KeyEventType.KeyUp && held -> { held = false; true }
                        else -> false
                    }
                }
                .tvClickable(shape = CircleShape, focusedScale = 1.1f, borderWidth = 3.dp, borderColor = focus, onClick = onClick)
                .clip(CircleShape)
                .background(back)
        ) { content(fore) }
        androidx.tv.material3.Text(label, style = androidx.tv.material3.MaterialTheme.typography.titleSmall,
            maxLines = 1, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Center, modifier = Modifier.padding(top = 6.dp))
        note?.let {
            androidx.tv.material3.Text(it, style = androidx.tv.material3.MaterialTheme.typography.bodySmall,
                color = scheme.onSurfaceVariant, maxLines = 1)
        }
    }
}

/** Held on an account: sign it out of this device, or not. */
@Composable
private fun TvSignOutDialog(user: String, server: String, onDismiss: () -> Unit, onSignOut: () -> Unit) {
    val cancelRequester = remember { FocusRequester() }
    // Opened by holding OK, the button is still down: OK counts only after a fresh press.
    var armed by remember { mutableStateOf(false) }
    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        androidx.tv.material3.Surface(shape = androidx.tv.material3.MaterialTheme.shapes.large, modifier = Modifier.onPreviewKeyEvent { e ->
            val select = e.key == Key.DirectionCenter || e.key == Key.Enter || e.key == Key.NumPadEnter
            if (!select || armed) return@onPreviewKeyEvent false
            if (e.type == KeyEventType.KeyDown && e.nativeKeyEvent.repeatCount == 0) {
                armed = true
                false
            } else true
        }) {
            Column(Modifier.padding(28.dp).width(420.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                androidx.tv.material3.Text(user, style = androidx.tv.material3.MaterialTheme.typography.titleLarge)
                androidx.tv.material3.Text("Signed in to $server", style = androidx.tv.material3.MaterialTheme.typography.bodyMedium,
                    color = androidx.tv.material3.MaterialTheme.colorScheme.onSurfaceVariant)
                androidx.tv.material3.Button(colors = tvButtonColors(), onClick = onSignOut, modifier = Modifier.fillMaxWidth()) {
                    androidx.tv.material3.Text("Sign out")
                }
                androidx.tv.material3.Button(colors = tvButtonColors(), onClick = onDismiss,
                    modifier = Modifier.fillMaxWidth().focusRequester(cancelRequester)) {
                    androidx.tv.material3.Text("Cancel")
                }
            }
        }
    }
    LaunchedEffect(Unit) {
        withFrameNanos { }
        runCatching { cancelRequester.requestFocus() }
    }
}

/** Settings → Remove on a server: asks first, naming its users. */
@Composable
fun TvRemoveServerDialog(address: String, users: List<String>, onDismiss: () -> Unit, onRemove: () -> Unit) {
    val cancelRequester = remember { FocusRequester() }
    androidx.compose.ui.window.Dialog(onDismissRequest = onDismiss) {
        androidx.tv.material3.Surface(shape = androidx.tv.material3.MaterialTheme.shapes.large) {
            Column(Modifier.padding(28.dp).width(480.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
                androidx.tv.material3.Text("Remove server?", style = androidx.tv.material3.MaterialTheme.typography.titleLarge)
                androidx.tv.material3.Text(removeServerWarning(address, users), style = androidx.tv.material3.MaterialTheme.typography.bodyMedium)
                androidx.tv.material3.Button(colors = tvButtonColors(), onClick = onRemove, modifier = Modifier.fillMaxWidth()) {
                    androidx.tv.material3.Text("Remove")
                }
                androidx.tv.material3.Button(colors = tvButtonColors(), onClick = onDismiss,
                    modifier = Modifier.fillMaxWidth().focusRequester(cancelRequester)) {
                    androidx.tv.material3.Text("Cancel")
                }
            }
        }
    }
    LaunchedEffect(Unit) {
        withFrameNanos { }
        runCatching { cancelRequester.requestFocus() }
    }
}

// ---- Phone ----

@Composable
fun PhoneServerAccountsRow(
    viewModel: MediaViewModel,
    provider: SourceProvider,
    address: String,
    accounts: List<ServerAccount>,
) {
    val (name, _) = serverLines(provider, address)
    Row(Modifier.fillMaxWidth().padding(vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(
            Modifier.padding(start = 12.dp).width(112.dp).height(108.dp)
                .background(androidx.compose.material3.MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(12.dp))
                .padding(10.dp),
            verticalArrangement = Arrangement.Center
        ) {
            androidx.compose.material3.Icon(Icons.Default.Dns, contentDescription = null, modifier = Modifier.size(24.dp),
                tint = WeirdStreamerTheme.spec.serverIconTint.takeOrElse { androidx.compose.material3.LocalContentColor.current })
            androidx.compose.material3.Text(name, style = androidx.compose.material3.MaterialTheme.typography.bodyMedium,
                maxLines = 3, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 6.dp))
        }
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            contentPadding = PaddingValues(horizontal = 12.dp),
            modifier = Modifier.weight(1f)
        ) {
            items(accounts, key = { it.username }) { account ->
                var menu by remember { mutableStateOf(false) }
                Box {
                    PhoneAccountButton(
                        label = accountLabel(account.username),
                        note = if (account.signedIn) null else "Signed out",
                        onLongClick = { menu = true },
                        onClick = { viewModel.chooseServerAccount(provider, address, account.username) }
                    ) { color -> androidx.compose.material3.Text(initialOf(accountLabel(account.username)), color = color,
                        style = androidx.compose.material3.MaterialTheme.typography.headlineSmall) }
                    androidx.compose.material3.DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        androidx.compose.material3.DropdownMenuItem(
                            text = { androidx.compose.material3.Text("Sign out ${accountLabel(account.username)}") },
                            onClick = { menu = false; viewModel.signOutServerAccount(provider, address, account.username) }
                        )
                    }
                }
            }
            item(key = "+add") {
                PhoneAccountButton(label = "Add user", onClick = { viewModel.addServerAccount(provider, address) }) { color ->
                    androidx.compose.material3.Icon(Icons.Default.Add, contentDescription = null, tint = color, modifier = Modifier.size(30.dp))
                }
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun PhoneAccountButton(
    label: String,
    note: String? = null,
    size: Dp = 64.dp,
    onLongClick: (() -> Unit)? = null,
    onClick: () -> Unit,
    content: @Composable (Color) -> Unit,
) {
    val spec = WeirdStreamerTheme.spec
    val scheme = androidx.compose.material3.MaterialTheme.colorScheme
    Column(
        Modifier.width(76.dp).clip(RoundedCornerShape(12.dp)).combinedClickable(onLongClick = onLongClick, onClick = onClick),
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier.size(size).clip(CircleShape)
                .background(spec.buttonColor.takeOrElse { scheme.secondaryContainer })
        ) { content(spec.buttonLabel.takeOrElse { scheme.onSecondaryContainer }) }
        androidx.compose.material3.Text(label, style = androidx.compose.material3.MaterialTheme.typography.bodySmall,
            maxLines = 1, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Center, modifier = Modifier.padding(top = 6.dp))
        note?.let {
            androidx.compose.material3.Text(it, style = androidx.compose.material3.MaterialTheme.typography.labelSmall,
                color = scheme.onSurfaceVariant, maxLines = 1)
        }
    }
}
