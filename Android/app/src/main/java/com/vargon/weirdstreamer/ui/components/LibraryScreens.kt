package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.runtime.mutableStateOf
import kotlinx.coroutines.launch
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Tv
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusProperties
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.tv.material3.Button
import androidx.tv.material3.ButtonDefaults
import androidx.tv.material3.Icon
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import coil.compose.AsyncImage
import coil.request.ImageRequest
import com.vargon.weirdstreamer.LibraryReturn
import com.vargon.weirdstreamer.MediaViewModel
import com.vargon.weirdstreamer.api.models.CreditInfo
import com.vargon.weirdstreamer.api.models.LibraryEpisode
import com.vargon.weirdstreamer.api.models.LibraryFile
import com.vargon.weirdstreamer.api.models.TitleDetail
import kotlinx.coroutines.android.awaitFrame
import java.util.Locale

private const val POSTER_COLUMNS = 6

/**
 * Library view: a movie or TV library as posters (sorted by title), with an Unmatched section
 * for files that weren't matched to a title.
 */
@Composable
fun LibraryScreen(viewModel: MediaViewModel, toolbarFocusRequester: FocusRequester, contentEntry: ContentEntry) {
    val listing = viewModel.libraryListing
    when {
        listing == null && viewModel.libraryError != null -> CenteredMessage("Error: ${viewModel.libraryError}")
        listing == null -> CenteredMessage("Loading…")
        listing.titles.isEmpty() && listing.unmatched.isEmpty() ->
            CenteredMessage("Nothing matched yet. Titles appear here as they are looked up.")
        else -> {
            val firstRequester = remember { FocusRequester() }
            // Coming back from a title page, scroll to and re-focus the poster that was opened. This
            // lives in the view model because the grid leaves composition while the title is shown.
            val returnTo = remember(listing.libraryId) {
                viewModel.libraryReturn?.takeIf { it.libraryId == listing.libraryId }
            }
            val gridState = rememberLazyGridState(returnTo?.index ?: 0, returnTo?.offset ?: 0)
            val lastRequester = remember { FocusRequester() }
            val isTv = listing.libraryType == "tv"
            // Requesters of the composed tiles by key, for Down from the toolbar.
            val requesters = remember(listing.libraryId) { HashMap<Any, FocusRequester>() }
            DisposableEffect(contentEntry, listing.libraryId) {
                val focusTop = {
                    val key = gridState.topRowKey { it == UNMATCHED_HEADER }
                    key != null && runCatching { requesters[key]?.requestFocus() }.getOrNull() != null
                }
                contentEntry.focusTop = focusTop
                onDispose { if (contentEntry.focusTop === focusTop) contentEntry.focusTop = null }
            }

            LaunchedEffect(listing.libraryId) {
                if (viewModel.toolbarKeepsFocus) return@LaunchedEffect
                awaitFrame()
                val restored = returnTo != null && runCatching { lastRequester.requestFocus() }.isSuccess
                if (!restored) runCatching { firstRequester.requestFocus() }
            }

            LazyVerticalGrid(
                columns = GridCells.Fixed(POSTER_COLUMNS),
                state = gridState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(8.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                itemsIndexed(listing.titles, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { index, t ->
                    val key = "${t.kind}:${t.tmdbId}"
                    PosterTile(
                        title = t.title,
                        subtitle = t.year?.toString(),
                        posterUrl = viewModel.serverUrl(t.poster),
                        placeholder = if (t.kind == "show") Icons.Default.Tv else Icons.Default.Movie,
                        modifier = Modifier
                            .gridEdges(index, toolbarFocusRequester)
                            .registered(key, requesters)
                            .then(if (index == 0) Modifier.focusRequester(firstRequester) else Modifier)
                            .then(if (key == returnTo?.titleKey) Modifier.focusRequester(lastRequester) else Modifier),
                        onClick = {
                            viewModel.libraryReturn = LibraryReturn(listing.libraryId, key,
                                gridState.firstVisibleItemIndex, gridState.firstVisibleItemScrollOffset)
                            viewModel.openLibraryTitle(t.kind, t.tmdbId)
                        }
                    )
                }
                if (listing.unmatched.isNotEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }, key = UNMATCHED_HEADER) {
                        Text(
                            "Unmatched (${listing.unmatched.size})",
                            style = MaterialTheme.typography.titleLarge,
                            modifier = Modifier.padding(top = 24.dp, bottom = 4.dp)
                        )
                    }
                    itemsIndexed(listing.unmatched, key = { _, f -> "file:${f.path}" }) { index, f ->
                        val playable = f.count == null
                        PosterTile(
                            title = f.name,
                            subtitle = if (playable) formatBytes(f.size) else "${f.count} files",
                            posterUrl = null,
                            placeholder = if (isTv) Icons.Default.Tv else Icons.Default.Movie,
                            modifier = Modifier
                                .gridEdges(index, toolbarFocusRequester, firstRow = listing.titles.isEmpty())
                                .registered("file:${f.path}", requesters)
                                .then(if (listing.titles.isEmpty() && index == 0) Modifier.focusRequester(firstRequester) else Modifier),
                            onClick = { if (playable) viewModel.playLibraryFile(f, listOf(f)) }
                        )
                    }
                }
            }
        }
    }
}

private const val UNMATCHED_HEADER = "unmatched-header"

/** Attaches a focus requester recorded in [requesters] under [key] while the item is composed. */
@Composable
private fun Modifier.registered(key: Any, requesters: HashMap<Any, FocusRequester>): Modifier {
    val requester = remember { FocusRequester() }
    DisposableEffect(key) {
        requesters[key] = requester
        onDispose { if (requesters[key] === requester) requesters.remove(key) }
    }
    return focusRequester(requester)
}

/** Left from the first column and up from the first row go to the toolbar, as in folders view. */
private fun Modifier.gridEdges(index: Int, toolbar: FocusRequester, firstRow: Boolean = true): Modifier {
    val isFirstColumn = index % POSTER_COLUMNS == 0
    val isFirstRow = firstRow && index < POSTER_COLUMNS
    if (!isFirstColumn && !isFirstRow) return this
    return focusProperties {
        if (isFirstColumn) left = toolbar
        if (isFirstRow) up = toolbar
    }
}

/** A movie-poster-shaped (2:3) tile, so posters aren't cropped to squares. */
@Composable
fun PosterTile(
    title: String,
    subtitle: String?,
    posterUrl: String?,
    placeholder: ImageVector,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    Column(modifier = modifier.fillMaxWidth()) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .fillMaxWidth()
                .aspectRatio(2f / 3f)
                .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.06f, borderWidth = 3.dp, onClick = onClick)
                .clip(MaterialTheme.shapes.medium)
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(placeholder, contentDescription = null, modifier = Modifier.size(48.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant)
            if (posterUrl != null) {
                RemoteImage(posterUrl, Modifier.fillMaxSize())
            }
        }
        Spacer(Modifier.height(8.dp))
        Text(title, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (!subtitle.isNullOrEmpty()) {
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/**
 * A title page: poster on the left with title, details, synopsis and credits beside it over the
 * dimmed backdrop; then the files (movies) or seasons and episodes (TV), each with a Play button.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun TitleScreen(viewModel: MediaViewModel, title: TitleDetail) {
    val playRequester = remember { FocusRequester() }
    val listState = rememberLazyListState()
    val scope = rememberCoroutineScope()
    LaunchedEffect(title.kind, title.tmdbId) {
        awaitFrame()
        runCatching { playRequester.requestFocus() }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        viewModel.serverUrl(title.backdrop)?.let { backdrop ->
            RemoteImage(backdrop, Modifier.fillMaxSize(), alpha = 0.22f)
            Box(
                Modifier
                    .fillMaxSize()
                    .background(Brush.horizontalGradient(listOf(Color.Black.copy(alpha = 0.7f), Color.Transparent)))
            )
        }

        // Scroll only as far as needed to show the focused item, so the hero stays on screen
        // instead of the TV default of pinning focus a third of the way down.
        CompositionLocalProvider(LocalBringIntoViewSpec provides MinimalBringIntoView) {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                if (title.kind == "movie") {
                    // The files sit in the hero's right column so the whole page fits on screen.
                    val files = title.files.orEmpty()
                    item(key = "hero") {
                        TitleHero(viewModel, title) {
                            FileRow(viewModel, files, files, playRequester, Modifier.padding(top = 16.dp), tileWidth = 160.dp)
                        }
                    }
                } else {
                    item(key = "hero") { TitleHero(viewModel, title) }
                    // The hero can't take focus, so bring it back whenever focus reaches the season
                    // picker; otherwise Up from there jumps to the toolbar with the hero off screen.
                    showSections(viewModel, title, playRequester,
                        onPickerFocused = { scope.launch { listState.animateScrollToItem(0) } })
                }
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
private val MinimalBringIntoView = object : BringIntoViewSpec {
    override fun calculateScrollDistance(offset: Float, size: Float, containerSize: Float): Float {
        val trailing = offset + size
        return when {
            offset >= 0 && trailing <= containerSize -> 0f // already fully visible
            offset < 0 && trailing > containerSize -> 0f   // bigger than the screen
            offset < 0 -> offset
            else -> trailing - containerSize
        }
    }
}

@Composable
private fun TitleHero(viewModel: MediaViewModel, title: TitleDetail, below: @Composable () -> Unit = {}) {
    Row(modifier = Modifier.fillMaxWidth()) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .width(160.dp)
                .aspectRatio(2f / 3f)
                .clip(MaterialTheme.shapes.medium)
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(if (title.kind == "show") Icons.Default.Tv else Icons.Default.Movie, contentDescription = null,
                modifier = Modifier.size(64.dp))
            viewModel.serverUrl(title.poster)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
        }
        Spacer(Modifier.width(28.dp))
        // The screen header already shows the title, so the hero starts with the details.
        Column(modifier = Modifier.weight(1f)) {
            val facts = listOfNotNull(
                title.year?.toString(),
                title.runtime?.takeIf { it > 0 }?.let { "$it min" },
                title.genres?.takeIf { it.isNotEmpty() }?.joinToString(", ")
            )
            if (facts.isNotEmpty()) {
                Text(facts.joinToString(" · "), style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            title.tagline?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.bodyLarge, fontStyle = FontStyle.Italic, maxLines = 1,
                    overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 6.dp))
            }
            title.overview?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, maxLines = 3, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 6.dp))
            }
            val people = if (title.kind == "show") title.creators else title.directors
            people?.takeIf { it.isNotEmpty() }?.let {
                val label = when {
                    title.kind == "show" -> "Created by"
                    it.size > 1 -> "Directors"
                    else -> "Director"
                }
                CreditLine(label, it)
            }
            title.cast?.takeIf { it.isNotEmpty() }?.let { CastRow(viewModel, it) }
            below()
        }
    }
}

@Composable
private fun CreditLine(label: String, people: List<CreditInfo>) {
    Row(modifier = Modifier.padding(top = 6.dp)) {
        Text("$label ", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold)
        Text(people.joinToString(", ") { it.name }, style = MaterialTheme.typography.bodyMedium,
            maxLines = 1, overflow = TextOverflow.Ellipsis)
    }
}

@Composable
private fun CastRow(viewModel: MediaViewModel, cast: List<CreditInfo>) {
    // Not focusable: the cast is information, and D-pad focus stays on things that play.
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.padding(top = 10.dp)) {
        cast.take(9).forEach { person ->
            Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.width(68.dp)) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .size(40.dp)
                        .clip(CircleShape)
                        .background(MaterialTheme.colorScheme.surfaceVariant)
                ) {
                    Text(person.name.take(1), style = MaterialTheme.typography.titleMedium)
                    viewModel.serverUrl(person.profile)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                }
                Text(person.name, style = MaterialTheme.typography.labelSmall, maxLines = 1,
                    overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 4.dp))
                person.character?.let {
                    Text(it, style = MaterialTheme.typography.labelSmall, maxLines = 1, overflow = TextOverflow.Ellipsis,
                        color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
    }
}

/** The square video thumbnails with a Play button beneath each; the thumbnail plays too. */
@Composable
private fun FileRow(
    viewModel: MediaViewModel,
    files: List<LibraryFile>,
    queue: List<LibraryFile>,
    firstRequester: FocusRequester?,
    modifier: Modifier = Modifier,
    tileWidth: Dp = 200.dp
) {
    LazyRow(modifier = modifier, horizontalArrangement = Arrangement.spacedBy(20.dp), contentPadding = PaddingValues(horizontal = 4.dp)) {
        itemsIndexed(files, key = { _, f -> f.path }) { index, file ->
            Column(modifier = Modifier.width(tileWidth), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .fillMaxWidth()
                        .aspectRatio(16f / 9f)
                        .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.05f, borderWidth = 3.dp,
                            onClick = { viewModel.playLibraryFile(file, queue) })
                        .clip(MaterialTheme.shapes.medium)
                        .background(MaterialTheme.colorScheme.surfaceVariant)
                ) {
                    Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(40.dp))
                    viewModel.serverUrl(file.thumbnailUrl)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                }
                Text(
                    listOf(file.name, formatBytes(file.size)).filter { it.isNotEmpty() }.joinToString(" · "),
                    // Two lines, so files of one title (part 1, part 2...) can be told apart.
                    style = MaterialTheme.typography.labelSmall, maxLines = 2, overflow = TextOverflow.Ellipsis,
                    minLines = 2
                )
                Button(
                    onClick = { viewModel.playLibraryFile(file, queue) },
                    modifier = if (index == 0 && firstRequester != null) Modifier.focusRequester(firstRequester) else Modifier
                ) {
                    Icon(Icons.Default.PlayArrow, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text("Play")
                }
            }
        }
    }
}

/** TV: a season picker, the season's episodes, and other files. */
private fun androidx.compose.foundation.lazy.LazyListScope.showSections(
    viewModel: MediaViewModel,
    title: TitleDetail,
    playRequester: FocusRequester,
    onPickerFocused: () -> Unit
) {
    val seasons = title.seasons.orEmpty()
    if (seasons.isEmpty()) {
        item(key = "no-episodes") { Text("No episodes found.", color = Color.Gray) }
    } else {
        item(key = "seasons") {
            SeasonsAndEpisodes(viewModel, title, playRequester, onPickerFocused)
        }
    }
    val other = title.otherFiles.orEmpty()
    if (other.isNotEmpty()) {
        item(key = "other-header") { Text("Other files", style = MaterialTheme.typography.titleLarge) }
        item(key = "other") { FileRow(viewModel, other, other, null) }
    }
}

@Composable
private fun SeasonsAndEpisodes(
    viewModel: MediaViewModel,
    title: TitleDetail,
    playRequester: FocusRequester,
    onPickerFocused: () -> Unit
) {
    val seasons = title.seasons.orEmpty()
    var selected by rememberSaveable(title.tmdbId) { mutableIntStateOf(0) }
    val season = seasons[selected.coerceIn(seasons.indices)]
    val queue = remember(season) { season.episodes.flatMap { it.files } }

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        var pickerFocused by remember { mutableStateOf(false) }
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            modifier = Modifier.onFocusChanged {
                if (it.hasFocus && !pickerFocused) onPickerFocused()
                pickerFocused = it.hasFocus
            }
        ) {
            itemsIndexed(seasons, key = { _, s -> s.season }) { index, s ->
                val isSelected = index == selected
                Button(
                    onClick = { selected = index },
                    colors = if (isSelected) ButtonDefaults.colors(
                        containerColor = MaterialTheme.colorScheme.primary,
                        contentColor = MaterialTheme.colorScheme.onPrimary
                    ) else ButtonDefaults.colors()
                ) {
                    Text("${s.name} (${s.episodes.size})")
                }
            }
        }
        season.episodes.forEachIndexed { index, episode ->
            EpisodeRow(viewModel, episode, queue, if (index == 0) playRequester else null)
        }
    }
}

@Composable
private fun EpisodeRow(
    viewModel: MediaViewModel,
    episode: LibraryEpisode,
    queue: List<LibraryFile>,
    requester: FocusRequester?
) {
    val first = episode.files.firstOrNull() ?: return
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .then(if (requester != null) Modifier.focusRequester(requester) else Modifier)
            .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.02f, borderWidth = 2.dp,
                onClick = { viewModel.playLibraryFile(first, queue) })
            .padding(8.dp)
    ) {
        Box(
            contentAlignment = Alignment.Center,
            modifier = Modifier
                .width(200.dp)
                .aspectRatio(16f / 9f)
                .clip(MaterialTheme.shapes.small)
                .background(MaterialTheme.colorScheme.surfaceVariant)
        ) {
            Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(36.dp))
            (viewModel.serverUrl(episode.still) ?: viewModel.serverUrl(first.thumbnailUrl))?.let {
                RemoteImage(it, Modifier.fillMaxSize())
            }
        }
        Spacer(Modifier.width(20.dp))
        Column(modifier = Modifier.weight(1f)) {
            Text(
                String.format(Locale.US, "S%02dE%02d · %s", episode.season, episode.episode, episode.title),
                style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis
            )
            val meta = listOfNotNull(episode.airDate, episode.runtime?.takeIf { it > 0 }?.let { "$it min" },
                episode.files.takeIf { it.size > 1 }?.let { "${it.size} files" })
            if (meta.isNotEmpty()) {
                Text(meta.joinToString(" · "), style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            episode.overview?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 4.dp))
            }
        }
    }
}

@Composable
private fun RemoteImage(url: String, modifier: Modifier, alpha: Float = 1f) {
    val context = LocalContext.current
    val request = remember(url) { ImageRequest.Builder(context).data(url).crossfade(false).build() }
    AsyncImage(model = request, contentDescription = null, contentScale = ContentScale.Crop, alpha = alpha, modifier = modifier)
}

@Composable
private fun CenteredMessage(text: String) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(text, style = MaterialTheme.typography.titleLarge)
    }
}

private fun formatBytes(size: Long): String {
    if (size <= 0) return ""
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val group = (Math.log10(size.toDouble()) / Math.log10(1024.0)).toInt().coerceIn(0, units.size - 1)
    return String.format(Locale.US, "%.1f %s", size / Math.pow(1024.0, group.toDouble()), units[group])
}
