package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Checkbox
import androidx.compose.material3.TextField
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusDirection
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.tv.material3.*

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun AuthDialog(host: String, onDismiss: () -> Unit, initialUsername: String? = null, title: String? = null, hint: String? = null, onLogin: (String, String, Boolean) -> Unit) {
    var username by remember { mutableStateOf(initialUsername.orEmpty()) }
    var password by remember { mutableStateOf("") }
    var savePassword by remember { mutableStateOf(true) }
    val focusManager = LocalFocusManager.current
    val keyboardController = LocalSoftwareKeyboardController.current
    val focusRequester = remember { FocusRequester() }
    val passwordRequester = remember { FocusRequester() }

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Surface(modifier = Modifier.width(400.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text(title ?: "Authentication Required", style = MaterialTheme.typography.headlineSmall)
                Text("Host: $host", style = MaterialTheme.typography.bodySmall)
                Spacer(modifier = Modifier.height(16.dp))
                
                TextField(
                    colors = tvTextFieldColors(),
                    value = username,
                    onValueChange = { username = it },
                    label = { FieldText("Username") },
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(focusRequester),
                    singleLine = true,
                    keyboardOptions = usernameKeyboard(ImeAction.Next),
                    keyboardActions = KeyboardActions(onNext = { focusManager.moveFocus(FocusDirection.Down) })
                )
                
                Spacer(modifier = Modifier.height(8.dp))
                
                TextField(
                    colors = tvTextFieldColors(),
                    value = password,
                    onValueChange = { password = it },
                    label = { FieldText("Password") },
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth().focusRequester(passwordRequester),
                    singleLine = true,
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Password,
                        imeAction = ImeAction.Done
                    ),
                    keyboardActions = KeyboardActions(onDone = {
                        keyboardController?.hide()
                        onLogin(username, password, savePassword)
                    })
                )
                
                Spacer(modifier = Modifier.height(8.dp))
                
                hint?.let { Text(it, style = MaterialTheme.typography.bodySmall) }
                
                Spacer(modifier = Modifier.height(24.dp))

                Button(shape = tvButtonShape(), colors = tvButtonColors(),
                    onClick = { onLogin(username, password, savePassword) },
                    modifier = Modifier.align(Alignment.End)
                ) {
                    Text("Login")
                }
            }
        }
    }

    LaunchedEffect(Unit) {
        // An account signing in again starts at its password.
        if (initialUsername.isNullOrEmpty()) focusRequester.requestFocus() else passwordRequester.requestFocus()
    }
}

@OptIn(ExperimentalTvMaterial3Api::class)
@Composable
fun ManualAddressDialog(initialAddress: String, onDismiss: () -> Unit, onConnect: (String) -> Unit) {
    var address by remember { mutableStateOf(initialAddress) }
    val keyboardController = LocalSoftwareKeyboardController.current
    val focusRequester = remember { FocusRequester() }

    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Surface(modifier = Modifier.width(500.dp)) {
            Column(modifier = Modifier.padding(24.dp)) {
                Text("Manual Connection", style = MaterialTheme.typography.headlineSmall)
                Spacer(modifier = Modifier.height(16.dp))
                TextField(
                    colors = tvTextFieldColors(),
                    value = address,
                    onValueChange = { address = it },
                    modifier = Modifier
                        .fillMaxWidth()
                        .focusRequester(focusRequester),
                    label = { FieldText("smb://host/share/") },
                    keyboardOptions = addressKeyboard(ImeAction.Done),
                    keyboardActions = KeyboardActions(onDone = { 
                        onConnect(address)
                        keyboardController?.hide()
                    }),
                    singleLine = true
                )
                Spacer(modifier = Modifier.height(24.dp))
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                    TvSecondaryButton(onClick = onDismiss) { Text("Cancel") }
                    Spacer(modifier = Modifier.width(8.dp))
                    Button(shape = tvButtonShape(), colors = tvButtonColors(), onClick = { onConnect(address) }) { Text("Connect") }
                }
            }
        }
    }

    LaunchedEffect(Unit) {
        focusRequester.requestFocus()
    }
}
