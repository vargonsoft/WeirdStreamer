package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CreditInfo
import com.vargon.weirdstreamer.api.models.PersonDetail
import com.vargon.weirdstreamer.api.models.PersonFilm
import java.time.LocalDate
import java.time.Period
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

// A person's window, opened from a title page's cast (the server's /api/library/person): a larger
// photo, their details and biography, their titles in this library (opened as title pages) and
// their whole TMDB filmography. The phone's is here; the TV's is in TvPersonDialog.kt.

/** Loads a person's page for the current library: (the page, an error). */
@Composable
internal fun rememberPerson(viewModel: MediaViewModel, id: Int): Pair<PersonDetail?, String?> {
    var person by remember(id) { mutableStateOf<PersonDetail?>(null) }
    var error by remember(id) { mutableStateOf<String?>(null) }
    LaunchedEffect(id) {
        val api = viewModel.connectedServer?.api
        val lib = viewModel.currentLibraryId
        if (api == null || lib == null) { error = "Not connected"; return@LaunchedEffect }
        val (got, err) = attempt { api.getPerson(lib, id) }
        person = got
        error = err
    }
    return person to error
}

/** "April 4, 1965 (age 61)" and "Died …" lines, and the rest of their facts, as (label, text). */
internal fun personFacts(p: PersonDetail): List<Pair<String, String>> {
    fun date(s: String?) = s?.takeIf { it.isNotEmpty() }?.let { runCatching { LocalDate.parse(it) }.getOrNull() }
    val fmt = DateTimeFormatter.ofLocalizedDate(FormatStyle.LONG)
    val born = date(p.birthday)
    val died = date(p.deathday)
    return listOfNotNull(
        p.knownFor?.takeIf { it.isNotEmpty() }?.let { "Known for" to it },
        born?.let { "Born" to it.format(fmt) + if (died == null) " (age ${Period.between(it, LocalDate.now()).years})" else "" },
        died?.let { "Died" to it.format(fmt) + (born?.let { b -> " (aged ${Period.between(b, it).years})" } ?: "") },
        p.placeOfBirth?.takeIf { it.isNotEmpty() }?.let { "Place of birth" to it },
    )
}

/** "2012 · Tony Stark · In this library" */
internal fun filmLine(f: PersonFilm) =
    listOfNotNull(f.year?.toString() ?: "Upcoming", f.role?.takeIf { it.isNotEmpty() }, if (f.inLibrary) "In this library" else null)
        .joinToString(" · ")

/** The phone's person window: full screen, scrolling. Opening a title closes it. */
@Composable
fun PhonePersonDialog(viewModel: MediaViewModel, cast: CreditInfo, onDismiss: () -> Unit) {
    val (person, error) = rememberPerson(viewModel, cast.tmdbId)
    fun open(kind: String, id: Int) { onDismiss(); viewModel.openLibraryTitle(kind, id) }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
            Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(cast.name, style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.weight(1f))
                IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, contentDescription = "Close") }
            }
            LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                item(key = "head") {
                    Row(horizontalArrangement = Arrangement.spacedBy(16.dp)) {
                        Box(Modifier.width(120.dp).height(180.dp).clip(RoundedCornerShape(8.dp))
                            .background(MaterialTheme.colorScheme.surfaceVariant)) {
                            (viewModel.serverUrl(person?.photo) ?: viewModel.serverUrl(cast.profile))?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                        }
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            person?.let { p ->
                                personFacts(p).forEach { (label, text) ->
                                    Text("$label: $text", style = MaterialTheme.typography.bodyMedium)
                                }
                            }
                            if (person == null) Text(error?.let { "Couldn't load their details: $it" } ?: "Loading…")
                        }
                    }
                }
                val p = person ?: return@LazyColumn
                p.biography?.takeIf { it.isNotEmpty() }?.let { bio ->
                    item(key = "bio") {
                        // Six lines, the rest on a tap
                        var whole by remember { mutableStateOf(false) }
                        Column(Modifier.clickable { whole = !whole }) {
                            Text(bio, style = MaterialTheme.typography.bodyMedium, maxLines = if (whole) Int.MAX_VALUE else 6,
                                overflow = TextOverflow.Ellipsis)
                            Text(if (whole) "Show less" else "Show more", style = MaterialTheme.typography.labelLarge,
                                color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
                        }
                    }
                }
                if (p.inLibrary.isNotEmpty()) {
                    item(key = "lib-head") {
                        Text("In ${viewModel.libraryListing?.libraryName ?: "this library"} (${p.inLibrary.size})",
                            style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
                    }
                    item(key = "lib") {
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                            items(p.inLibrary, key = { "${it.kind}:${it.tmdbId}" }) { t ->
                                Box(Modifier.width(108.dp)) {
                                    PhonePosterTile(title = t.title, subtitle = listOfNotNull(t.year?.toString(), t.role).joinToString(" · "),
                                        posterUrl = viewModel.serverUrl(t.poster), placeholder = Icons.Default.Movie,
                                        progress = t.progress.toFloat(), watched = t.allWatched) { open(t.kind, t.tmdbId) }
                                }
                            }
                        }
                    }
                }
                if (p.filmography.isNotEmpty()) {
                    item(key = "films-head") {
                        Text("Filmography (${p.filmography.size})", style = MaterialTheme.typography.titleMedium,
                            color = MaterialTheme.colorScheme.primary)
                    }
                    items(p.filmography, key = { "film:${it.kind}:${it.tmdbId}" }) { f ->
                        Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(6.dp))
                            .then(if (f.inLibrary) Modifier.clickable { open(f.kind, f.tmdbId) } else Modifier).padding(4.dp),
                            horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
                            Box(Modifier.width(40.dp).height(60.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceVariant)) {
                                f.poster?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                            }
                            Column(Modifier.weight(1f)) {
                                Text(f.title + if (f.kind == "show") " (TV)" else "", fontWeight = FontWeight.SemiBold, maxLines = 2,
                                    overflow = TextOverflow.Ellipsis,
                                    color = if (f.inLibrary) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface)
                                Text(filmLine(f), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                }
            }
        }
    }
}
