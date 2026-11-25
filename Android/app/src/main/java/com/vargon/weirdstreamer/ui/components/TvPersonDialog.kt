package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Movie
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CreditInfo

/**
 * The TV's person window (PersonDialogs.kt has the phone's): the photo and details with the
 * biography beside them, their titles in this library as posters (OK opens one), then their
 * filmography as rows (those in the library open too). Back closes it.
 */
@Composable
fun TvPersonDialog(viewModel: MediaViewModel, cast: CreditInfo, onDismiss: () -> Unit) {
    val (person, error) = rememberPerson(viewModel, cast.tmdbId)
    val first = remember { FocusRequester() }
    fun open(kind: String, id: Int) { onDismiss(); viewModel.openLibraryTitle(kind, id) }
    LaunchedEffect(person != null) { if (person != null) runCatching { first.requestFocus() } }
    TvDialogFrame(cast.name, onDismiss, width = 1040) {
        Row(horizontalArrangement = Arrangement.spacedBy(20.dp)) {
            Box(Modifier.width(100.dp).height(150.dp).clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.08f))) {
                (viewModel.serverUrl(person?.photo) ?: viewModel.serverUrl(cast.profile))?.let { RemoteImage(it, Modifier.fillMaxSize()) }
            }
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                person?.let { p ->
                    personFacts(p).forEach { (label, text) -> Text("$label: $text", style = MaterialTheme.typography.bodyMedium) }
                    p.biography?.takeIf { it.isNotEmpty() }?.let {
                        Text(it, style = MaterialTheme.typography.bodySmall, maxLines = 4, overflow = TextOverflow.Ellipsis,
                            modifier = Modifier.padding(top = 6.dp))
                    }
                }
                if (person == null) Text(error?.let { "Couldn't load their details: $it" } ?: "Loading…")
            }
        }
        val p = person ?: return@TvDialogFrame
        LazyColumn(Modifier.heightIn(max = 520.dp)) {
            if (p.inLibrary.isNotEmpty()) {
                item(key = "lib") {
                    Column {
                        TvSectionTitle("In ${viewModel.libraryListing?.libraryName ?: "this library"} (${p.inLibrary.size})")
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                            itemsIndexed(p.inLibrary, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { i, t ->
                                Box(Modifier.width(120.dp)) {
                                    PosterTile(title = t.title, subtitle = listOfNotNull(t.year?.toString(), t.role).joinToString(" · "),
                                        posterUrl = viewModel.serverUrl(t.poster), placeholder = Icons.Default.Movie,
                                        progress = t.progress.toFloat(), watched = t.allWatched,
                                        modifier = if (i == 0) Modifier.focusRequester(first) else Modifier) { open(t.kind, t.tmdbId) }
                                }
                            }
                        }
                    }
                }
            }
            if (p.filmography.isNotEmpty()) {
                item(key = "films-head") { TvSectionTitle("Filmography (${p.filmography.size})") }
                itemsIndexed(p.filmography, key = { _, f -> "film:${f.kind}:${f.tmdbId}" }) { i, f ->
                    TvValueRow(f.title + (if (f.kind == "show") " (TV)" else ""), filmLine(f),
                        focus = if (i == 0 && p.inLibrary.isEmpty()) first else null) {
                        if (f.inLibrary) open(f.kind, f.tmdbId)
                    }
                }
            }
        }
    }
}
