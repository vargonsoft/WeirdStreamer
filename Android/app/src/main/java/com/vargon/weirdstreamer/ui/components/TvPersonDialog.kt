package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.Alignment
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.getValue
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.focusable
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
import kotlinx.coroutines.launch
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
    val list = androidx.compose.foundation.lazy.rememberLazyListState()
    val scope = androidx.compose.runtime.rememberCoroutineScope()
    LaunchedEffect(person != null) {
        if (person == null) return@LaunchedEffect
        runCatching { first.requestFocus() }
        kotlinx.coroutines.android.awaitFrame()
        list.scrollToItem(0) // the heading over the first posters shows too
    }
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
        LazyColumn(Modifier.heightIn(max = 520.dp), state = list) {
            if (p.inLibrary.isNotEmpty()) {
                item(key = "lib-head") {
                    TvSectionTitle("In ${viewModel.libraryListing?.libraryName ?: "this library"} (${p.inLibrary.size})")
                }
                item(key = "lib") {
                    Column {
                        // Room around the posters for the focused one's frame (as the cast row); with
                        // focus on them, the list is back at its top, their heading shown
                        LazyRow(horizontalArrangement = Arrangement.spacedBy(14.dp),
                            contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = FocusBleed, vertical = 10.dp),
                            modifier = Modifier.focusBleed().onFocusChanged {
                                if (it.hasFocus) scope.launch { list.animateScrollToItem(0) }
                            }) {
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
                    TvFilmRow(viewModel, f, focus = if (i == 0 && p.inLibrary.isEmpty()) first else null) {
                        if (f.inLibrary) open(f.kind, f.tmdbId)
                    }
                }
            }
        }
    }
}

/**
 * A filmography row: the title's poster (TMDB's), its title, and the year and their part, filled
 * with the focus color when focused (as TvValueRow). Those in the library are in the heading color;
 * OK opens them.
 */
@Composable
private fun TvFilmRow(viewModel: MediaViewModel, f: com.vargon.weirdstreamer.api.models.PersonFilm,
                      focus: FocusRequester?, onClick: () -> Unit) {
    val spec = com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme.spec
    var focused by remember { mutableStateOf(false) }
    val text = if (focused) spec.focusLabel.takeOrElse { Color.Black } else MaterialTheme.colorScheme.onSurface
    Row(
        Modifier.fillMaxWidth()
            .then(if (focus != null) Modifier.focusRequester(focus) else Modifier)
            .onFocusChanged { focused = it.isFocused }
            .onPreviewKeyEvent { e ->
                val ok = e.key == Key.DirectionCenter || e.key == Key.Enter || e.key == Key.NumPadEnter
                if (ok && e.type == KeyEventType.KeyDown) { onClick(); true } else false
            }
            .focusable()
            .background(if (focused) spec.focusColor.takeOrElse { Color.White } else Color.Transparent, RoundedCornerShape(8.dp))
            .padding(horizontal = 14.dp, vertical = 6.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.width(40.dp).height(60.dp).clip(RoundedCornerShape(4.dp))
            .background(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.08f))) {
            f.poster?.let { RemoteImage(it, Modifier.fillMaxSize()) }
        }
        Column(Modifier.weight(1f)) {
            Text(f.title + (if (f.kind == "show") " (TV)" else ""), style = MaterialTheme.typography.titleMedium, maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                color = if (!focused && f.inLibrary) spec.headingColor.takeOrElse { text } else text)
            Text(filmLine(f), style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis,
                color = if (focused) text else MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
