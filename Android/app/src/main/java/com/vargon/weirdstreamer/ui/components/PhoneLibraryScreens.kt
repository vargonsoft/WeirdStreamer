package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.ui.draw.alpha
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.filled.Theaters
import androidx.compose.material.icons.filled.Tv
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Surface
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material.icons.filled.Download
import androidx.compose.material3.FilterChip
import androidx.compose.material3.IconButton
import androidx.compose.material3.TextButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.window.Dialog
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.filled.Close
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.vargon.weirdstreamer.LibraryReturn
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CreditInfo
import com.vargon.weirdstreamer.api.models.LibraryEpisode
import com.vargon.weirdstreamer.api.models.LibraryFile
import com.vargon.weirdstreamer.api.models.TitleDetail
import java.util.Locale

/** Phone Library view: a movie or TV library as posters, with unmatched files at the end. */
@Composable
fun PhoneLibraryScreen(viewModel: MediaViewModel) {
    val listing = viewModel.libraryListing
    when {
        listing == null && viewModel.libraryError != null -> PhoneMessage("Error: ${viewModel.libraryError}")
        listing == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        listing.titles.isEmpty() && listing.unmatched.isEmpty() ->
            PhoneMessage("Nothing matched yet. Titles appear here as they are looked up.")
        else -> {
            // Back from a title page returns to where the grid was.
            val returnTo = remember(listing.libraryId) {
                viewModel.libraryReturn?.takeIf { it.libraryId == listing.libraryId }
            }
            val gridState = rememberLazyGridState(returnTo?.index ?: 0, returnTo?.offset ?: 0)
            val isTv = listing.libraryType == "tv"
            // The A–Z strip: the titles' letters (as the server sorts them, without "The"), then
            // the unmatched files'. Positions count titles, then unmatched files.
            val letters = remember(listing) {
                buildIndex(listing.titles.map { indexLabel(it.title, skipArticles = true) to 0 } +
                    listing.unmatched.map { indexLabel(it.name, skipArticles = true) to 1 },
                    { it.first }, { it.second })
            }
            val continueCount = if (viewModel.hasTopRows) 1 else 0
            val scope = rememberCoroutineScope()
            val topLetter by remember(letters, continueCount) {
                derivedStateOf {
                    val first = gridState.firstVisibleItemIndex - continueCount
                    letters.entryFor(if (first > listing.titles.size) first - 1 else first.coerceAtLeast(0))
                }
            }
            Row(Modifier.fillMaxSize()) {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(108.dp),
                state = gridState,
                modifier = Modifier.weight(1f).fillMaxHeight(),
                contentPadding = PaddingValues(12.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                if (viewModel.hasTopRows) {
                    item(span = { GridItemSpan(maxLineSpan) }, key = "continue") {
                        PhoneContinueRow(viewModel, sidePadding = 0.dp) // the grid has its own
                    }
                }
                itemsIndexed(listing.titles, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { _, t ->
                    // A movie's place in it; a show's share of episodes watched.
                    val showPart = t.kind == "show" && t.watched > 0 && !t.allWatched
                    PhonePosterTile(
                        title = t.title,
                        subtitle = listOfNotNull(t.year?.toString(),
                            if (showPart) "${t.watched} of ${t.fileCount} watched" else null).joinToString(" · "),
                        posterUrl = viewModel.serverUrl(t.poster),
                        placeholder = if (t.kind == "show") Icons.Default.Tv else Icons.Default.Movie,
                        progress = if (t.kind == "movie") t.progress.toFloat()
                            else if (showPart) t.watched.toFloat() / t.fileCount else 0f,
                        watched = t.allWatched
                    ) {
                        viewModel.libraryReturn = LibraryReturn(listing.libraryId, "${t.kind}:${t.tmdbId}",
                            gridState.firstVisibleItemIndex, gridState.firstVisibleItemScrollOffset)
                        viewModel.openLibraryTitle(t.kind, t.tmdbId)
                    }
                }
                if (listing.unmatched.isNotEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }, key = "unmatched-header") {
                        Text("Unmatched (${listing.unmatched.size})", style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.padding(top = 16.dp))
                    }
                    itemsIndexed(listing.unmatched, key = { _, f -> "file:${f.path}" }) { _, f ->
                        val playable = f.count == null
                        PhonePosterTile(
                            title = f.name,
                            subtitle = if (playable) formatBytes(f.size) else "${f.count} files",
                            posterUrl = null,
                            placeholder = if (isTv) Icons.Default.Tv else Icons.Default.Theaters,
                            progress = f.playback?.fraction(f.duration) ?: 0f,
                            watched = f.playback?.watched == true,
                            // Long press: Fix (admins)
                            onLongClick = if (viewModel.canFixMatches) ({ viewModel.openMatch(f.path, false, f.name) }) else null
                        ) { if (playable) viewModel.playLibraryFile(f, listOf(f)) }
                    }
                }
            }
            if (letters.size > 1) {
                PhoneIndexBar(letters, topLetter, onSelect = { i ->
                    val p = letters[i].position
                    val target = if (p < listing.titles.size) p + continueCount else p + continueCount + 1
                    scope.launch { gridState.scrollToItem(target) }
                }, Modifier.padding(vertical = 8.dp))
            }
            }
        }
    }
}

@OptIn(androidx.compose.foundation.ExperimentalFoundationApi::class)
@Composable
internal fun PhonePosterTile(
    title: String,
    subtitle: String?,
    posterUrl: String?,
    placeholder: ImageVector,
    progress: Float = 0f,
    watched: Boolean = false,
    onLongClick: (() -> Unit)? = null,
    dimmed: Boolean = false, // a collection's film the library doesn't have: not clickable
    collage: List<String>? = null, // a made collection's: up to four posters instead of one
    onClick: () -> Unit
) {
    Column(modifier = Modifier.fillMaxWidth()
        .then(if (dimmed) Modifier.alpha(0.4f) else Modifier.combinedClickable(onClick = onClick, onLongClick = onLongClick))) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(2f / 3f)
                .clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(placeholder, contentDescription = null, modifier = Modifier.size(40.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant)
            if (collage != null) Collage(collage, Modifier.fillMaxSize()) else posterUrl?.let { RemoteImage(it, Modifier.fillMaxSize()) }
            WatchProgressOverlay(progress, watched)
        }
        Text(title, style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(top = 6.dp))
        if (!subtitle.isNullOrEmpty()) {
            Text(subtitle, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/**
 * Phone title page: backdrop with poster and title, then details, cast, and the files (movies)
 * or seasons and episodes (TV), each playable.
 */
@Composable
fun PhoneTitleScreen(viewModel: MediaViewModel, title: TitleDetail) {
    TitleThemeMusic(viewModel, title)
    LazyColumn(modifier = Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        item(key = "header") { PhoneTitleHeader(viewModel, title) }
        item(key = "details") { PhoneTitleDetails(title) }
        // A show's watched buttons are above its seasons, as on the TV.
        if (title.kind == "movie" || !title.files.isNullOrEmpty()) item(key = "watched") { PhoneTitleWatched(viewModel, title) }
        title.cast?.takeIf { it.isNotEmpty() }?.let { cast ->
            item(key = "cast") { PhoneCastRow(viewModel, cast) }
        }
        // Movie libraries list files, also for a title matched to a TV show (a TV movie).
        if (title.kind == "movie" || !title.files.isNullOrEmpty()) {
            val files = title.files.orEmpty()
            item(key = "files-header") { PhoneSectionHeader(if (files.size > 1) "Files" else "File") }
            val label = title.title + (title.year?.let { " ($it)" } ?: "")
            items(files, key = { "file:" + it.path }) { PhoneFileRow(viewModel, it, files, label) }
        } else {
            phoneShowSections(viewModel, title)
        }
    }
}

@Composable
private fun PhoneTitleHeader(viewModel: MediaViewModel, title: TitleDetail) {
    val background = MaterialTheme.colorScheme.background
    Box(modifier = Modifier.fillMaxWidth()) {
        viewModel.serverUrl(title.backdrop)?.let { backdrop ->
            RemoteImage(backdrop, Modifier.matchParentSize(), alpha = 0.35f)
            Box(Modifier.matchParentSize().background(Brush.verticalGradient(listOf(Color.Transparent, background))))
        }
        Row(modifier = Modifier.padding(16.dp), verticalAlignment = Alignment.Bottom) {
            Box(
                contentAlignment = Alignment.Center,
                modifier = Modifier
                    .width(112.dp)
                    .aspectRatio(2f / 3f)
                    .clip(RoundedCornerShape(8.dp))
                    .background(MaterialTheme.colorScheme.surfaceVariant)
            ) {
                Icon(if (title.kind == "show") Icons.Default.Tv else Icons.Default.Movie, contentDescription = null,
                    modifier = Modifier.size(40.dp))
                viewModel.serverUrl(title.poster)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
            }
            Spacer(Modifier.width(16.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(title.title, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.SemiBold)
                val facts = listOfNotNull(
                    title.year?.toString(),
                    title.runtime?.takeIf { it > 0 }?.let { "$it min" },
                )
                if (facts.isNotEmpty()) {
                    Text(facts.joinToString(" · "), style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
                }
                title.genres?.takeIf { it.isNotEmpty() }?.let {
                    Text(it.joinToString(", "), style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                // Fix the whole title's match (through its first file, which every file of it follows)
                titleFixFile(title)?.let { f ->
                    if (viewModel.canFixMatches) PhoneSecondaryButton(onClick = { viewModel.openMatch(f.path, false, title.title, wholeTitle = true) },
                        modifier = Modifier.padding(top = 8.dp)) { Text("Fix match") }
                }
                // Admins add a movie library's title to their collections
                if (viewModel.canEditCollections) {
                    var adding by remember { mutableStateOf(false) }
                    PhoneSecondaryButton(onClick = { adding = true }, modifier = Modifier.padding(top = 8.dp)) { Text("Add to collection") }
                    if (adding) PhoneAddToCollectionDialog(viewModel, com.vargon.weirdstreamer.api.models.CollectionTitle(title.kind, title.tmdbId),
                        title.title, onDismiss = { adding = false })
                }
            }
        }
    }
}

/** A file's Fix button (admins): its match, or its episode for an episode file. */
@Composable
private fun PhoneFixButton(viewModel: MediaViewModel, file: LibraryFile) {
    if (!viewModel.canFixMatches) return
    IconButton(onClick = { viewModel.openMatch(file.path, false, file.name) }) {
        Icon(Icons.Default.Edit, contentDescription = "Fix match")
    }
}

@Composable
private fun PhoneTitleDetails(title: TitleDetail) {
    Column(modifier = Modifier.padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        title.tagline?.takeIf { it.isNotEmpty() }?.let {
            Text(it, style = MaterialTheme.typography.bodyLarge, fontStyle = FontStyle.Italic)
        }
        title.overview?.takeIf { it.isNotEmpty() }?.let {
            Text(it, style = MaterialTheme.typography.bodyMedium)
        }
        val people = if (title.kind == "show") title.creators else title.directors
        people?.takeIf { it.isNotEmpty() }?.let {
            val label = when {
                title.kind == "show" -> "Created by"
                it.size > 1 -> "Directors"
                else -> "Director"
            }
            PhoneCreditLine(label, it)
        }
        if (title.kind == "movie") title.writers?.takeIf { it.isNotEmpty() }?.let { PhoneCreditLine("Writers", it) }
    }
}

@Composable
private fun PhoneCreditLine(label: String, people: List<CreditInfo>) {
    Text(
        androidx.compose.ui.text.buildAnnotatedString {
            pushStyle(androidx.compose.ui.text.SpanStyle(fontWeight = FontWeight.Bold))
            append("$label ")
            pop()
            append(people.joinToString(", ") { it.name })
        },
        style = MaterialTheme.typography.bodyMedium
    )
}

@Composable
private fun PhoneCastRow(viewModel: MediaViewModel, cast: List<CreditInfo>) {
    // A tap opens a person's window
    var opened by remember { mutableStateOf<CreditInfo?>(null) }
    opened?.let { PhonePersonDialog(viewModel, it, onDismiss = { opened = null }) }
    LazyRow(
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp)
    ) {
        itemsIndexed(cast, key = { i, person -> "$i:${person.tmdbId}" }) { _, person ->
            Column(horizontalAlignment = Alignment.CenterHorizontally,
                modifier = Modifier.width(84.dp).clip(RoundedCornerShape(8.dp)).clickable { opened = person }) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .size(64.dp)
                        .clip(CircleShape)
                        .background(MaterialTheme.colorScheme.surfaceVariant)
                ) {
                    Text(person.name.take(1), style = MaterialTheme.typography.titleLarge)
                    viewModel.serverUrl(person.profile)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                }
                // Names and roles wrap instead of being cut off.
                Text(person.name, style = MaterialTheme.typography.labelMedium, textAlign = TextAlign.Center,
                    modifier = Modifier.padding(top = 6.dp))
                person.character?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.labelSmall, textAlign = TextAlign.Center,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

@Composable
private fun PhoneSectionHeader(text: String) {
    Text(text, style = MaterialTheme.typography.titleMedium,
        modifier = Modifier.padding(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 8.dp))
}

/** A file: its thumbnail (tap to play) with the full name, size, and play and download buttons. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun PhoneFileRow(viewModel: MediaViewModel, file: LibraryFile, queue: List<LibraryFile>, titleLabel: String? = null) {
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp)
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .width(140.dp)
                .aspectRatio(16f / 9f)
                .clip(RoundedCornerShape(8.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
                .clickable { viewModel.playLibraryFile(file, queue) }
        ) {
            Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(32.dp))
            viewModel.serverUrl(file.thumbnailUrl)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
            PlaybackOverlay(file.playback, file.duration)
        }
        Spacer(Modifier.width(12.dp))
        Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            file.match?.let { m ->
                Text(m.title + (m.year?.let { " ($it)" } ?: ""), style = MaterialTheme.typography.titleSmall)
            }
            Text(file.name, style = MaterialTheme.typography.bodySmall) // the whole name, wrapped
            file.format?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            formatBytes(file.size).takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            timeLeftText(file.playback, file.duration)?.let {
                Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary)
            }
            // Play, or Resume with Play from start beside it; then watched and download.
            FlowRow(verticalArrangement = Arrangement.Center, itemVerticalAlignment = Alignment.CenterVertically) {
                val resumeAt = file.playback?.resumeAt ?: 0.0
                if (resumeAt > 0) {
                    FilledTonalButton(shape = AppButtonShape, onClick = { viewModel.playLibraryFile(file, queue) },
                        contentPadding = PaddingValues(horizontal = 12.dp)) {
                        Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("Resume ${clockText(resumeAt)}")
                    }
                    IconButton(onClick = { viewModel.playLibraryFile(file, queue, fromStart = true) }) {
                        Icon(Icons.Default.Replay, contentDescription = "Play from start")
                    }
                } else {
                    FilledTonalIconButton(shape = AppButtonShape, onClick = { viewModel.playLibraryFile(file, queue) }) {
                        Icon(Icons.Default.PlayArrow, contentDescription = "Play")
                    }
                }
                WatchedButton(viewModel, listOf(file))
                val label = file.match?.let { m -> m.title + (m.year?.let { " ($it)" } ?: "") } ?: titleLabel
                DownloadButton(viewModel, DownloadRequest.of(viewModel, file, label))
                PhoneFixButton(viewModel, file)
            }
        }
    }
}

/** TV: a season picker, the chosen season's episodes, and the show's other files. */
private fun androidx.compose.foundation.lazy.LazyListScope.phoneShowSections(viewModel: MediaViewModel, title: TitleDetail) {
    val seasons = title.seasons.orEmpty()
    if (seasons.isEmpty()) {
        item(key = "no-episodes") { PhoneMessage("No episodes found.") }
    } else {
        item(key = "seasons") { PhoneSeasons(viewModel, title) }
    }
    val other = title.otherFiles.orEmpty()
    if (other.isNotEmpty()) {
        item(key = "other-header") { PhoneSectionHeader("Other files") }
        items(other, key = { "other:" + it.path }) { PhoneFileRow(viewModel, it, other, title.title) }
    }
}

@Composable
private fun PhoneSeasons(viewModel: MediaViewModel, title: TitleDetail) {
    val seasons = title.seasons.orEmpty()
    // The season where the show picks up (the latest episode played, or the next), else the first.
    val pick = remember(title.tmdbId) { upNext(seasons, viewModel.lastPlayedFile) }
    var selected by rememberSaveable(title.tmdbId) { mutableIntStateOf(pick?.first ?: 0) }
    // That episode is scrolled into view once, on opening (the phone has no focus to show it by).
    var scrolled by remember(title.tmdbId) { mutableStateOf(false) }
    val season = seasons[selected.coerceIn(seasons.indices)]
    val queue = remember(season) { season.episodes.flatMap { it.files } }
    Column {
        // Watched buttons for the season shown and the whole show, above the seasons (as on the TV).
        // Each row's buttons share the tallest one's height (a named season's label has two lines).
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(top = 8.dp)
                .height(IntrinsicSize.Min)) {
            PhoneGroupWatchedButton(viewModel, season.episodes.flatMap { it.files }, season.name, season, Modifier.fillMaxHeight())
            PhoneGroupWatchedButton(viewModel, seasons.flatMap { s -> s.episodes.flatMap { it.files } }, "Whole show",
                modifier = Modifier.fillMaxHeight())
            // The players skip this show's intros and credits by themselves (for this user).
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxHeight()
                .clickable { viewModel.setAutoSkip(title.tmdbId, !title.autoSkip) }.padding(end = 8.dp)) {
                androidx.compose.material3.Checkbox(checked = title.autoSkip, onCheckedChange = { viewModel.setAutoSkip(title.tmdbId, it) })
                Text("Auto Skip")
            }
        }
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp)
                .height(IntrinsicSize.Min)
        ) {
            seasons.forEachIndexed { index, s ->
                // Episodes watched (any of an episode's files), as a ring after the name.
                val watched = s.episodes.count { e -> e.files.any { it.playback?.watched == true } }
                FilterChip(colors = phoneChipColors(), shape = AppButtonShape, selected = index == selected, border = phoneChipBorder(index == selected), onClick = { selected = index },
                    modifier = Modifier.fillMaxHeight(),
                    // Padded so a two-line label ("Season 1" over the name) clears the chip's edges.
                    label = { Box(Modifier.padding(vertical = 6.dp)) { PhoneSeasonLabel(s, s.name) } },
                    trailingIcon = {
                        SeasonProgressRing(if (s.episodes.isEmpty()) 0f else watched.toFloat() / s.episodes.size,
                            androidx.compose.material3.LocalContentColor.current, size = 16.dp)
                    })
            }
        }
        Text(episodeCountText(season.episodes.size), style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 8.dp))
        var downloading by remember { mutableStateOf<List<DownloadRequest>?>(null) }
        // The downloads scroll sideways when they don't fit.
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
            FilledTonalButton(shape = AppButtonShape, onClick = { downloading = episodeDownloads(viewModel, title, listOf(season)) }) {
                Icon(Icons.Default.Download, contentDescription = null)
                Spacer(Modifier.width(4.dp))
                Text("Download season")
            }
            if (seasons.size > 1) {
                FilledTonalButton(shape = AppButtonShape, onClick = { downloading = episodeDownloads(viewModel, title, seasons) }) {
                    Icon(Icons.Default.Download, contentDescription = null)
                    Spacer(Modifier.width(4.dp))
                    Text("Whole show")
                }
            }
        }
        downloading?.let { DownloadQualityDialog(viewModel, it) { downloading = null } }
        season.episodes.forEachIndexed { i, episode ->
            val target = !scrolled && pick != null && selected == pick.first && i == pick.second
            val requester = remember { BringIntoViewRequester() }
            PhoneEpisodeRow(viewModel, episode, queue, title.title,
                if (target) Modifier.bringIntoViewRequester(requester) else Modifier)
            if (target) LaunchedEffect(Unit) {
                withFrameNanos { } // laid out first
                requester.bringIntoView()
                scrolled = true
            }
        }
    }
}

@Composable
private fun PhoneEpisodeRow(viewModel: MediaViewModel, episode: LibraryEpisode, queue: List<LibraryFile>, showTitle: String,
                            modifier: Modifier = Modifier) {
    val first = episode.files.firstOrNull() ?: return
    var showDetails by remember { mutableStateOf(false) }
    if (showDetails) PhoneEpisodeDetails(episode) { showDetails = false }
    Row(
        verticalAlignment = Alignment.Top,
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 10.dp)
    ) {
        // The thumbnail with its buttons beneath, leaving the text the rest of the width.
        Column(Modifier.width(144.dp)) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(16f / 9f)
                .clip(RoundedCornerShape(6.dp))
                .background(MaterialTheme.colorScheme.surfaceVariant)
                .clickable { viewModel.playLibraryFile(first, queue) } // the thumbnail plays (resumes)
        ) {
            Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(28.dp))
            (viewModel.serverUrl(episode.still) ?: viewModel.serverUrl(first.thumbnailUrl))?.let {
                RemoteImage(it, Modifier.fillMaxSize())
            }
            PlaybackOverlay(first.playback, first.duration, badgeSize = 18.dp)
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
            if ((first.playback?.resumeAt ?: 0.0) > 0) {
                IconButton(onClick = { viewModel.playLibraryFile(first, queue, fromStart = true) }) {
                    Icon(Icons.Default.Replay, contentDescription = "Play from start")
                }
            }
            WatchedButton(viewModel, episode.files)
            episode.files.forEach { f -> DownloadButton(viewModel, DownloadRequest.of(viewModel, f, episodeLabel(showTitle, episode), episode.still)) }
            PhoneFixButton(viewModel, first)
        }
        }
        Spacer(Modifier.width(12.dp))
        // The text opens in full (the description is cut short here).
        Column(modifier = Modifier.weight(1f).clickable { showDetails = true }) {
            // The file (or files) on disk, then a blank line, then the episode it shows as.
            Text(episode.files.joinToString("\n") { it.name }, style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(10.dp))
            Text(
                String.format(Locale.US, "S%02dE%02d · %s", episode.season, episode.episode, episode.title),
                style = MaterialTheme.typography.titleSmall
            )
            // Its files' formats, between its title and its date and length
            episode.files.mapNotNull { it.format?.takeIf { f -> f.isNotEmpty() } }.distinct().forEach {
                Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            val meta = listOfNotNull(episode.airDate, episode.runtime?.takeIf { it > 0 }?.let { "$it min" },
                episode.files.takeIf { it.size > 1 }?.let { "${it.size} files" })
            if (meta.isNotEmpty()) {
                Text(meta.joinToString(" · "), style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            episode.overview?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, maxLines = 3, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp))
            }
            // Tapping the thumbnail resumes from here; the button under it starts over.
            val resumeAt = first.playback?.resumeAt ?: 0.0
            if (resumeAt > 0) {
                Text("Resumes at ${clockText(resumeAt)}", style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp))
            }
        }
    }
}

/** An episode's text in full, to read: the files, title, date and length, and the whole description. Back or X closes it. */
@Composable
private fun PhoneEpisodeDetails(episode: LibraryEpisode, onDismiss: () -> Unit) {
    Dialog(onDismissRequest = onDismiss) {
        Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            Box {
                Column(Modifier.verticalScroll(rememberScrollState()).padding(20.dp).padding(end = 28.dp)) {
                    Text(episode.files.joinToString("\n") { it.name }, style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(10.dp))
                    Text(String.format(Locale.US, "S%02dE%02d · %s", episode.season, episode.episode, episode.title),
                        style = MaterialTheme.typography.titleMedium)
                    val meta = listOfNotNull(episode.airDate, episode.runtime?.takeIf { it > 0 }?.let { "$it min" })
                    if (meta.isNotEmpty()) {
                        Text(meta.joinToString(" · "), style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    episode.overview?.takeIf { it.isNotEmpty() }?.let {
                        Text(it, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 10.dp))
                    }
                }
                IconButton(onClick = onDismiss, modifier = Modifier.align(Alignment.TopEnd)) {
                    Icon(Icons.Default.Close, contentDescription = "Close")
                }
            }
        }
    }
}

private fun episodeLabel(showTitle: String, episode: LibraryEpisode) =
    String.format(Locale.US, "%s S%02dE%02d · %s", showTitle, episode.season, episode.episode, episode.title)

/** Every episode file of [seasons], to download. */
private fun episodeDownloads(viewModel: MediaViewModel, title: TitleDetail, seasons: List<com.vargon.weirdstreamer.api.models.LibrarySeason>) =
    seasons.flatMap { s -> s.episodes.flatMap { e -> e.files.map { DownloadRequest.of(viewModel, it, episodeLabel(title.title, e), e.still) } } }

@Composable
internal fun PhoneMessage(text: String) {
    Box(Modifier.fillMaxWidth().padding(32.dp), contentAlignment = Alignment.Center) {
        Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center)
    }
}

/** A check that marks [files] watched, or unwatched when they all are. */
@Composable
private fun WatchedButton(viewModel: MediaViewModel, files: List<LibraryFile>) {
    val watched = files.isNotEmpty() && files.all { it.playback?.watched == true }
    IconButton(onClick = { viewModel.setWatched(files.map { it.path }, !watched) }) {
        Icon(
            if (watched) Icons.Default.CheckCircle else Icons.Outlined.CheckCircle,
            contentDescription = if (watched) "Mark unwatched" else "Mark watched",
            tint = if (watched) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

/**
 * Marks [files] (a season's, or the whole show's) watched, or unwatched when they all are: a button
 * named [label], with a check that's solid once they all are watched and dimmed until then.
 */
@Composable
private fun PhoneGroupWatchedButton(viewModel: MediaViewModel, files: List<LibraryFile>, label: String,
                                    season: com.vargon.weirdstreamer.api.models.LibrarySeason? = null, modifier: Modifier = Modifier) {
    if (files.isEmpty()) return
    val watched = files.all { it.playback?.watched == true }
    FilledTonalButton(shape = AppButtonShape, onClick = { viewModel.setWatched(files.map { it.path }, !watched) }, modifier = modifier) {
        Icon(Icons.Default.CheckCircle, contentDescription = if (watched) "Mark unwatched" else "Mark watched",
            modifier = Modifier.size(18.dp).alpha(if (watched) 1f else 0.35f))
        Spacer(Modifier.width(6.dp))
        if (season != null) PhoneSeasonLabel(season, label) else Text(label, maxLines = 1)
    }
}

/** A season's label, with "Season N" small above it when the season has a name of its own ("Blood and Sand"), as on the TV. */
@Composable
private fun PhoneSeasonLabel(season: com.vargon.weirdstreamer.api.models.LibrarySeason, label: String) {
    val number = "Season ${season.season}"
    if (season.season > 0 && !season.name.equals(number, ignoreCase = true)) {
        Column {
            Text(number, style = MaterialTheme.typography.labelSmall)
            Text(label, maxLines = 1)
        }
    } else Text(label, maxLines = 1)
}

/** Marks all of a movie's files watched or unwatched: only for a movie with several (versions, editions). */
@Composable
private fun PhoneTitleWatched(viewModel: MediaViewModel, title: TitleDetail) {
    val files = title.files.orEmpty()
    if (files.size < 2) return
    Box(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) { PhoneGroupWatchedButton(viewModel, files, "Mark All Watched") }
}

/** Phone Collection view: a movie library's TMDB collections (film series) as posters, by name. */
@Composable
fun PhoneCollectionsScreen(viewModel: MediaViewModel) {
    val listing = viewModel.libraryListing
    val collections = listing?.collections.orEmpty()
    when {
        listing == null && viewModel.libraryError != null -> PhoneMessage("Error: ${viewModel.libraryError}")
        listing == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        collections.isEmpty() -> PhoneMessage("No movies in collections yet. Collections appear here as movies are looked up.")
        else -> LazyVerticalGrid(
            columns = GridCells.Adaptive(108.dp),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            items(collections, key = { it.id }) { c ->
                PhonePosterTile(
                    title = c.name,
                    subtitle = collectionCount(c.count, c.total),
                    posterUrl = viewModel.serverUrl(c.poster),
                    placeholder = Icons.Default.Movie,
                ) { viewModel.openLibraryCollection(c.id) }
            }
        }
    }
}

/**
 * Phone collection page: its name, how many of its films the library has and its overview, then
 * every film by release date; those the library doesn't have are dimmed and open nothing.
 */
@Composable
fun PhoneCollectionScreen(viewModel: MediaViewModel, collection: com.vargon.weirdstreamer.api.models.CollectionDetail) {
    val have = collection.parts.count { !it.missing }
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
                Text("$have of ${collectionCount(collection.parts.size)} in this library", style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
                collection.overview?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
            }
        }
        items(collection.parts, key = { it.tmdbId }) { t ->
            PhonePosterTile(
                title = t.title,
                subtitle = if (t.missing) listOfNotNull(t.year?.toString(), "Not in this library").joinToString(" · ") else t.year?.toString(),
                posterUrl = viewModel.serverUrl(t.poster),
                placeholder = Icons.Default.Movie,
                progress = t.progress.toFloat(),
                watched = t.allWatched,
                dimmed = t.missing,
            ) { viewModel.openLibraryTitle(t.kind, t.tmdbId) }
        }
    }
}

/** "2 of 6 films", or "6 films" without a count. */
internal fun collectionCount(count: Int, total: Int? = null): String {
    val n = total ?: count
    val films = "$n film${if (n == 1) "" else "s"}"
    return if (total == null) films else "$count of $films"
}
