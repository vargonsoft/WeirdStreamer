package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Movie
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CollectionTitle
import com.vargon.weirdstreamer.api.models.LibraryTitle
import com.vargon.weirdstreamer.api.models.UserCollectionDetail
import kotlinx.coroutines.android.awaitFrame

// The TV's collections admins make (PhoneUserCollections.kt has the phone's and what they are):
// the grid of collages, a collection's page, and dialogs to make and change them.

/** TV Collections view: the collections admins made, as collages; admins start with a New collection tile. */
@Composable
fun TvUserCollectionsScreen(viewModel: MediaViewModel, toolbarFocusRequester: FocusRequester, contentEntry: ContentEntry) {
    val listing = viewModel.libraryListing
    var creating by remember { mutableStateOf(false) }
    if (listing == null) {
        if (viewModel.libraryError != null) TvNote("Error: ${viewModel.libraryError}") else TvNote("Loading…")
        return
    }
    val admin = viewModel.canEditCollections
    val collections = listing.userCollections.orEmpty()
    val returnTo = remember(listing.libraryId) { viewModel.userCollectionReturn }
    val tiles = (if (admin) 1 else 0) + collections.size
    val start = (collections.indexOfFirst { it.id == returnTo }.takeIf { it >= 0 }?.plus(if (admin) 1 else 0)) ?: 0
    val gridState = rememberLazyGridState(start)
    val first = remember { FocusRequester() }
    DisposableEffect(contentEntry) {
        val focusTop = { runCatching { first.requestFocus() }.isSuccess }
        contentEntry.focusTop = focusTop
        onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
    }
    LaunchedEffect(listing.libraryId, tiles) {
        if (viewModel.toolbarKeepsFocus || tiles == 0) return@LaunchedEffect
        awaitFrame()
        runCatching { first.requestFocus() }
    }
    if (tiles == 0) {
        TvNote("No collections yet.")
        return
    }
    LazyVerticalGrid(
        columns = GridCells.Fixed(POSTER_COLUMNS),
        state = gridState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(8.dp),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        fun Modifier.tile(i: Int) = this
            .then(if (i == start) Modifier.focusRequester(first) else Modifier)
            .then(if (i < POSTER_COLUMNS) Modifier.focusProperties { up = toolbarFocusRequester } else Modifier)
        if (admin) item(key = "new") {
            PosterTile(title = "New collection", subtitle = null, posterUrl = null, placeholder = Icons.Default.Add,
                modifier = Modifier.tile(0)) { creating = true }
        }
        itemsIndexed(collections, key = { _, c -> c.id }) { i, c ->
            PosterTile(title = c.name, subtitle = movieCount(c.count), posterUrl = null, placeholder = Icons.Default.Movie,
                collage = c.posters, modifier = Modifier.tile(i + if (admin) 1 else 0)) {
                viewModel.userCollectionReturn = c.id
                viewModel.openUserCollection(c.id)
            }
        }
    }
    if (creating) {
        TvTextEntryDialog("New collection's name", "", note = "Add its movies on its page, or from a movie's page.",
            onDismiss = { creating = false }) { name ->
            if (name.isNotBlank()) viewModel.createUserCollection(name.trim(), "", emptyList())
        }
    }
}

/**
 * TV made-collection page: its count and description, then its movies by release date. Admins have
 * an Edit collection tile first (name, description, movies, delete); holding OK on a movie takes it out.
 */
@Composable
fun TvUserCollectionScreen(viewModel: MediaViewModel, collection: UserCollectionDetail,
                           toolbarFocusRequester: FocusRequester, contentEntry: ContentEntry) {
    val admin = viewModel.canEditCollections
    var managing by remember { mutableStateOf(false) }
    var removing by remember { mutableStateOf<LibraryTitle?>(null) }
    val offset = if (admin) 1 else 0
    val start = remember(collection.id, collection.parts.size) {
        collection.parts.indexOfFirst { it.tmdbId == viewModel.collectionTitleReturn }.takeIf { it >= 0 }?.plus(offset) ?: 0
    }
    val first = remember { FocusRequester() }
    val gridState = rememberLazyGridState(if (start >= POSTER_COLUMNS) start + 1 else 0)
    DisposableEffect(contentEntry) {
        val focusTop = { runCatching { first.requestFocus() }.isSuccess }
        contentEntry.focusTop = focusTop
        onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
    }
    // On opening, and when a movie goes (the focused one with it), focus comes back to the grid
    LaunchedEffect(collection.id, collection.parts.size) {
        awaitFrame()
        runCatching { first.requestFocus() }
    }
    LazyVerticalGrid(
        columns = GridCells.Fixed(POSTER_COLUMNS),
        state = gridState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(8.dp),
        horizontalArrangement = Arrangement.spacedBy(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp)
    ) {
        fun Modifier.tile(i: Int) = this
            .then(if (i == start) Modifier.focusRequester(first) else Modifier)
            .then(if (i < POSTER_COLUMNS) Modifier.focusProperties { up = toolbarFocusRequester } else Modifier)
        item(span = { GridItemSpan(maxLineSpan) }, key = "head") {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(movieCount(collection.parts.size) + if (admin && collection.parts.isNotEmpty()) " · hold OK on one to take it out" else "",
                    style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                collection.description?.takeIf { it.isNotEmpty() }?.let {
                    Text(it, style = MaterialTheme.typography.bodyMedium, maxLines = 3, overflow = TextOverflow.Ellipsis)
                }
                viewModel.collectionError?.let { Text(it, color = MaterialTheme.colorScheme.error) }
            }
        }
        if (admin) item(key = "edit") {
            PosterTile(title = "Edit collection", subtitle = "Name, movies, delete", posterUrl = null,
                placeholder = Icons.Default.Edit, modifier = Modifier.tile(0)) { managing = true }
        }
        itemsIndexed(collection.parts, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { i, t ->
            PosterTile(
                title = t.title, subtitle = t.year?.toString(), posterUrl = viewModel.serverUrl(t.poster),
                placeholder = Icons.Default.Movie, progress = t.progress.toFloat(), watched = t.allWatched,
                onHold = if (admin) ({ removing = t }) else null,
                modifier = Modifier.tile(i + offset),
            ) {
                viewModel.collectionTitleReturn = t.tmdbId
                viewModel.openLibraryTitle(t.kind, t.tmdbId)
            }
        }
    }
    if (managing) TvManageCollectionDialog(viewModel, collection, onDismiss = { managing = false })
    removing?.let { t ->
        TvConfirmDialog("Take ${t.title} out?", "It stays in the library.", "Take out", onDismiss = { removing = null }) {
            viewModel.updateUserCollection(collection.id, remove = listOf(t.ref()))
        }
    }
}

/** A collection's settings (admins): its name, description and movies, each saved as changed, and Delete. */
@Composable
private fun TvManageCollectionDialog(viewModel: MediaViewModel, collection: UserCollectionDetail, onDismiss: () -> Unit) {
    var entry by remember { mutableStateOf<String?>(null) } // "name" or "description"
    var picking by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    TvDialogFrame("Edit ${collection.name}", { onDismiss() }, width = 760) {
        Column(Modifier.okAfterFreshPress()) {
            TvValueRow("Name", collection.name, focus = first) { entry = "name" }
            TvValueRow("Description", collection.description?.takeIf { it.isNotEmpty() }?.let { if (it.length > 40) it.take(40) + "…" else it } ?: "None") {
                entry = "description"
            }
            TvValueRow("Movies", movieCount(collection.parts.size)) { picking = true }
            TvValueRow("Delete collection", destructive = true) { deleting = true }
        }
    }
    when (entry) {
        "name" -> TvTextEntryDialog("Name", collection.name, onDismiss = { entry = null }) { name ->
            if (name.isNotBlank()) viewModel.updateUserCollection(collection.id, name = name.trim())
        }
        "description" -> TvTextEntryDialog("Description (optional)", collection.description.orEmpty(), onDismiss = { entry = null }) { d ->
            viewModel.updateUserCollection(collection.id, description = d.trim())
        }
    }
    if (picking) {
        TvMoviePickerDialog(viewModel, collection.parts.map { it.ref() }.toSet(), onDismiss = { picking = false }) { add, remove ->
            viewModel.updateUserCollection(collection.id, add = add, remove = remove)
        }
    }
    if (deleting) {
        TvConfirmDialog("Delete ${collection.name}?", "The collection goes; its movies stay in the library.", "Delete",
            onDismiss = { deleting = false }) {
            onDismiss()
            viewModel.deleteUserCollection(collection.id)
        }
    }
}

/** The library's movies to tick (OK toggles one, ✓ marks those in); Done saves what changed. */
@Composable
private fun TvMoviePickerDialog(viewModel: MediaViewModel, checked: Set<CollectionTitle>, onDismiss: () -> Unit,
                                onDone: (add: List<CollectionTitle>, remove: List<CollectionTitle>) -> Unit) {
    val titles = viewModel.libraryListing?.titles.orEmpty()
    val ticked = remember { mutableStateListOf<CollectionTitle>().apply { addAll(checked) } }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    fun done() {
        onDismiss()
        onDone(ticked.filter { it !in checked }, checked.filter { it !in ticked })
    }
    TvDialogFrame("Movies (${ticked.size} chosen)", { done() }, width = 760) {
        TvNote("OK puts a movie in or takes it out; Back saves.")
        TvValueRow("Done", focus = first) { done() }
        LazyColumn(Modifier.heightIn(max = 420.dp)) {
            itemsIndexed(titles, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { _, t ->
                val ref = t.ref()
                TvValueRow(t.title + (t.year?.let { " ($it)" } ?: ""), if (ref in ticked) "✓" else "") {
                    if (ref in ticked) ticked.remove(ref) else ticked.add(ref)
                }
            }
        }
    }
}

/** A title page's Add to collection (admins, movie libraries): OK puts it in a collection or takes it out, or makes a new one. */
@Composable
fun TvAddToCollectionDialog(viewModel: MediaViewModel, title: CollectionTitle, name: String, onDismiss: () -> Unit) {
    var creating by remember { mutableStateOf(false) }
    val collections = viewModel.libraryListing?.userCollections.orEmpty()
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    TvDialogFrame("Add to collection", onDismiss, width = 760) {
        TvNote(name)
        TvValueRow("New collection…", focus = first) { creating = true }
        LazyColumn(Modifier.heightIn(max = 380.dp)) {
            itemsIndexed(collections, key = { _, c -> c.id }) { _, c ->
                val inIt = title in c.titles
                TvValueRow("${c.name} (${c.count})", if (inIt) "✓" else "") {
                    if (inIt) viewModel.updateUserCollection(c.id, remove = listOf(title))
                    else viewModel.updateUserCollection(c.id, add = listOf(title))
                }
            }
        }
        viewModel.collectionError?.let { TvNote(it) }
    }
    if (creating) {
        TvTextEntryDialog("New collection's name", "", onDismiss = { creating = false }) { n ->
            if (n.isNotBlank()) viewModel.createUserCollection(n.trim(), "", listOf(title), open = false)
        }
    }
}
