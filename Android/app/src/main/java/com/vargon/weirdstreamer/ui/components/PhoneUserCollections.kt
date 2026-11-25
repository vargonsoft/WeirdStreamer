package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CollectionTitle
import com.vargon.weirdstreamer.api.models.LibraryTitle
import com.vargon.weirdstreamer.api.models.UserCollectionDetail

// Collections admins make in a movie library (the server's user collections): a name, an optional
// description and movies of the library by release date, shown as a collage of the first four
// posters. Everyone who sees the library sees them; admins create, edit and delete them.

/** A collage of up to four posters: one fills it, two side by side, three one tall and two stacked, four 2×2. */
@Composable
fun Collage(posters: List<String>, modifier: Modifier) {
    val viewModel: MediaViewModel = androidx.lifecycle.viewmodel.compose.viewModel()
    val urls = posters.take(4).mapNotNull { viewModel.serverUrl(it) }
    val gap = 2.dp
    @Composable
    fun Cell(i: Int, m: Modifier) = Box(m) { urls.getOrNull(i)?.let { RemoteImage(it, Modifier.fillMaxSize()) } }
    when (urls.size) {
        0 -> Box(modifier)
        1 -> Cell(0, modifier)
        2 -> Row(modifier, horizontalArrangement = Arrangement.spacedBy(gap)) {
            Cell(0, Modifier.weight(1f).fillMaxHeight()); Cell(1, Modifier.weight(1f).fillMaxHeight())
        }
        else -> Row(modifier, horizontalArrangement = Arrangement.spacedBy(gap)) {
            if (urls.size == 3) {
                Cell(0, Modifier.weight(1f).fillMaxHeight())
            } else {
                Column(Modifier.weight(1f).fillMaxHeight(), verticalArrangement = Arrangement.spacedBy(gap)) {
                    Cell(0, Modifier.weight(1f).fillMaxWidth()); Cell(2, Modifier.weight(1f).fillMaxWidth())
                }
            }
            Column(Modifier.weight(1f).fillMaxHeight(), verticalArrangement = Arrangement.spacedBy(gap)) {
                Cell(1, Modifier.weight(1f).fillMaxWidth()); Cell(if (urls.size == 3) 2 else 3, Modifier.weight(1f).fillMaxWidth())
            }
        }
    }
}

internal fun movieCount(n: Int) = "$n movie${if (n == 1) "" else "s"}"

internal fun LibraryTitle.ref() = CollectionTitle(kind, tmdbId)

/** Phone Collections view: the collections admins made, as collages; admins start with New collection. */
@Composable
fun PhoneUserCollectionsScreen(viewModel: MediaViewModel) {
    val listing = viewModel.libraryListing
    var creating by remember { mutableStateOf(false) }
    val collections = listing?.userCollections.orEmpty()
    when {
        listing == null && viewModel.libraryError != null -> PhoneMessage("Error: ${viewModel.libraryError}")
        listing == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { androidx.compose.material3.CircularProgressIndicator() }
        else -> LazyVerticalGrid(
            columns = GridCells.Adaptive(108.dp),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            if (viewModel.canEditCollections) {
                item(span = { GridItemSpan(maxLineSpan) }, key = "new") {
                    Row { PhoneButton(onClick = { creating = true }) { Text("New collection") } }
                }
            }
            if (collections.isEmpty()) {
                item(span = { GridItemSpan(maxLineSpan) }, key = "empty") {
                    Text(if (viewModel.canEditCollections) "No collections yet. Make one with New collection, or from a movie’s page."
                        else "No collections yet.", modifier = Modifier.padding(vertical = 24.dp))
                }
            }
            items(collections, key = { it.id }) { c ->
                PhonePosterTile(title = c.name, subtitle = movieCount(c.count), posterUrl = null, placeholder = Icons.Default.Movie,
                    collage = c.posters) { viewModel.openUserCollection(c.id) }
            }
        }
    }
    if (creating) {
        PhoneCollectionEditor(viewModel, null, emptySet(), onDismiss = { creating = false }) { name, description, titles ->
            viewModel.createUserCollection(name, description, titles)
        }
    }
}

/**
 * Phone made-collection page: its name, count and description, then its movies by release date.
 * Admins have Edit (name, description and movies) and Delete, and a long press takes a movie out.
 */
@Composable
fun PhoneUserCollectionScreen(viewModel: MediaViewModel, collection: UserCollectionDetail) {
    var editing by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }
    var removing by remember { mutableStateOf<LibraryTitle?>(null) }
    val admin = viewModel.canEditCollections
    LazyVerticalGrid(
        columns = GridCells.Adaptive(108.dp),
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(12.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item(span = { GridItemSpan(maxLineSpan) }, key = "head") {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(collection.name, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold)
                Text(movieCount(collection.parts.size), style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                collection.description?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                viewModel.collectionError?.let { Text(it, color = MaterialTheme.colorScheme.error) }
                if (admin) Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
                    PhoneButton(onClick = { editing = true }) { Text("Edit") }
                    PhoneSecondaryButton(onClick = { deleting = true }) { Text("Delete") }
                }
                if (admin && collection.parts.isNotEmpty()) {
                    Text("Long press a movie to take it out.", style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        if (collection.parts.isEmpty()) {
            item(span = { GridItemSpan(maxLineSpan) }, key = "empty") {
                Text("No movies in it yet.", modifier = Modifier.padding(vertical = 24.dp))
            }
        }
        items(collection.parts, key = { "${it.kind}:${it.tmdbId}" }) { t ->
            PhonePosterTile(
                title = t.title, subtitle = t.year?.toString(), posterUrl = viewModel.serverUrl(t.poster),
                placeholder = Icons.Default.Movie, progress = t.progress.toFloat(), watched = t.allWatched,
                onLongClick = if (admin) ({ removing = t }) else null,
            ) { viewModel.openLibraryTitle(t.kind, t.tmdbId) }
        }
    }
    if (editing) {
        PhoneCollectionEditor(viewModel, collection, collection.parts.map { it.ref() }.toSet(), onDismiss = { editing = false }) { name, description, titles ->
            val before = collection.parts.map { it.ref() }.toSet()
            viewModel.updateUserCollection(collection.id, name, description, add = titles - before, remove = before - titles)
        }
    }
    if (deleting) {
        AlertDialog(
            onDismissRequest = { deleting = false },
            title = { Text("Delete ${collection.name}?") },
            text = { Text("The collection goes; its movies stay in the library.") },
            confirmButton = { PhoneButton(onClick = { deleting = false; viewModel.deleteUserCollection(collection.id) }, destructive = true) { Text("Delete") } },
            dismissButton = { PhoneSecondaryButton(onClick = { deleting = false }) { Text("Cancel") } },
        )
    }
    removing?.let { t ->
        AlertDialog(
            onDismissRequest = { removing = null },
            title = { Text("Take ${t.title} out?") },
            text = { Text("It stays in the library.") },
            confirmButton = { PhoneButton(onClick = { removing = null; viewModel.updateUserCollection(collection.id, remove = listOf(t.ref())) }) { Text("Take out") } },
            dismissButton = { PhoneSecondaryButton(onClick = { removing = null }) { Text("Cancel") } },
        )
    }
}

/**
 * The collection editor (admins): a name, an optional description and the library's movies to tick,
 * with a filter. [collection] null makes a new one; [checked] are ticked to start. [onSave] gets the
 * name, description and the movies ticked.
 */
@Composable
fun PhoneCollectionEditor(viewModel: MediaViewModel, collection: UserCollectionDetail?, checked: Set<CollectionTitle>,
                          onDismiss: () -> Unit, onSave: (String, String, Set<CollectionTitle>) -> Unit) {
    var name by remember { mutableStateOf(collection?.name.orEmpty()) }
    var description by remember { mutableStateOf(collection?.description.orEmpty()) }
    var filter by remember { mutableStateOf("") }
    val ticked = remember { mutableStateListOf<CollectionTitle>().apply { addAll(checked) } }
    var missingName by remember { mutableStateOf(false) }
    val titles = viewModel.libraryListing?.titles.orEmpty()
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(if (collection == null) "New collection" else "Edit collection", style = MaterialTheme.typography.titleLarge,
                    color = MaterialTheme.colorScheme.primary, modifier = Modifier.weight(1f))
                IconButton(onClick = onDismiss) { Icon(Icons.Default.Close, contentDescription = "Close") }
            }
            OutlinedTextField(name, { name = it.take(200); missingName = false }, label = { Text("Name") }, singleLine = true,
                isError = missingName, modifier = Modifier.fillMaxWidth())
            OutlinedTextField(description, { description = it.take(4000) }, label = { Text("Description (optional)") },
                minLines = 2, maxLines = 4, modifier = Modifier.fillMaxWidth())
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text("Movies (${ticked.size} chosen)", style = MaterialTheme.typography.titleSmall)
                OutlinedTextField(filter, { filter = it }, label = { Text("Filter") }, singleLine = true, modifier = Modifier.weight(1f))
            }
            val shown = titles.filter { filter.isBlank() || it.title.contains(filter.trim(), ignoreCase = true) }
            LazyColumn(Modifier.weight(1f)) {
                items(shown, key = { "${it.kind}:${it.tmdbId}" }) { t ->
                    val ref = t.ref()
                    Row(Modifier.fillMaxWidth().clickable { if (ref in ticked) ticked.remove(ref) else ticked.add(ref) },
                        verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(checked = ref in ticked, onCheckedChange = { on -> if (on) ticked.add(ref) else ticked.remove(ref) })
                        Text(t.title + (t.year?.let { " ($it)" } ?: ""), maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
            if (missingName) Text("A collection needs a name.", color = MaterialTheme.colorScheme.error)
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                PhoneSecondaryButton(onClick = onDismiss) { Text("Cancel") }
                Spacer(Modifier.width(8.dp))
                PhoneButton(onClick = {
                    if (name.isBlank()) missingName = true
                    else { onDismiss(); onSave(name.trim(), description.trim(), ticked.toSet()) }
                }) { Text("Save") }
            }
        }
    }
}

/** A title page's Add to collection (admins, movie libraries): tick the collections it's in, or make a new one. */
@Composable
fun PhoneAddToCollectionDialog(viewModel: MediaViewModel, title: CollectionTitle, name: String, onDismiss: () -> Unit) {
    var creating by remember { mutableStateOf(false) }
    val collections = viewModel.libraryListing?.userCollections.orEmpty()
    if (creating) {
        PhoneCollectionEditor(viewModel, null, setOf(title), onDismiss = { creating = false; onDismiss() }) { n, d, titles ->
            viewModel.createUserCollection(n, d, titles, open = false)
        }
        return
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add to collection") },
        text = {
            Column {
                Text(name, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (collections.isEmpty()) Text("No collections yet.", modifier = Modifier.padding(top = 8.dp))
                LazyColumn {
                    items(collections, key = { it.id }) { c ->
                        val inIt = title in c.titles
                        Row(Modifier.fillMaxWidth().clickable {
                            if (inIt) viewModel.updateUserCollection(c.id, remove = listOf(title)) else viewModel.updateUserCollection(c.id, add = listOf(title))
                        }, verticalAlignment = Alignment.CenterVertically) {
                            Checkbox(checked = inIt, onCheckedChange = { on ->
                                if (on) viewModel.updateUserCollection(c.id, add = listOf(title)) else viewModel.updateUserCollection(c.id, remove = listOf(title))
                            })
                            Text("${c.name} (${c.count})")
                        }
                    }
                }
                viewModel.collectionError?.let { Text(it, color = MaterialTheme.colorScheme.error) }
            }
        },
        confirmButton = { PhoneButton(onClick = onDismiss) { Text("Done") } },
        dismissButton = { PhoneSecondaryButton(onClick = { creating = true }) { Text("New collection…") } },
    )
}
