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
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.outlined.Circle
import androidx.compose.material.icons.filled.Movie
import androidx.compose.material.icons.filled.Replay
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
import androidx.compose.ui.text.style.TextAlign
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
    TvResumeDialog(viewModel) // held Continue Watching cards
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
            val hasContinue = viewModel.continueItems.isNotEmpty()
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
                // Start on Continue Watching when there is one: it's the top row.
                if (!restored) runCatching { (requesters[CONTINUE_ROW] ?: firstRequester).requestFocus() }
            }
            // A card removed from Continue Watching took focus with it: onto the next, or the posters.
            LaunchedEffect(viewModel.continueRemovals) {
                if (viewModel.continueRemovals == 0) return@LaunchedEffect
                awaitFrame()
                val onRow = hasContinue && runCatching { requesters[CONTINUE_ROW]?.requestFocus() }.getOrNull() != null
                if (!onRow) runCatching { firstRequester.requestFocus() }
            }
            // Continue Watching usually arrives just after the titles: show it and move onto it,
            // unless the user has already moved on or came back to a title.
            LaunchedEffect(listing.libraryId, hasContinue) {
                if (!hasContinue || returnTo != null || viewModel.toolbarKeepsFocus) return@LaunchedEffect
                if (gridState.firstVisibleItemIndex > 1) return@LaunchedEffect
                gridState.scrollToItem(0)
                awaitFrame()
                runCatching { requesters[CONTINUE_ROW]?.requestFocus() }
            }

            LazyVerticalGrid(
                columns = GridCells.Fixed(POSTER_COLUMNS),
                state = gridState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(8.dp),
                horizontalArrangement = Arrangement.spacedBy(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                if (hasContinue) {
                    item(span = { GridItemSpan(maxLineSpan) }, key = CONTINUE_ROW) {
                        val continueRequester = remember { FocusRequester() }
                        DisposableEffect(Unit) {
                            requesters[CONTINUE_ROW] = continueRequester
                            onDispose { if (requesters[CONTINUE_ROW] === continueRequester) requesters.remove(CONTINUE_ROW) }
                        }
                        TvContinueRow(viewModel, continueRequester, up = toolbarFocusRequester,
                            onDown = { runCatching { firstRequester.requestFocus() }.isSuccess })
                    }
                }
                itemsIndexed(listing.titles, key = { _, t -> "${t.kind}:${t.tmdbId}" }) { index, t ->
                    val key = "${t.kind}:${t.tmdbId}"
                    // A movie's place in it; a show's share of episodes watched.
                    val showPart = t.kind == "show" && t.watched > 0 && !t.allWatched
                    PosterTile(
                        title = t.title,
                        subtitle = listOfNotNull(t.year?.toString(),
                            if (showPart) "${t.watched} of ${t.fileCount} watched" else null).joinToString(" · "),
                        posterUrl = viewModel.serverUrl(t.poster),
                        placeholder = if (t.kind == "show") Icons.Default.Tv else Icons.Default.Movie,
                        progress = if (t.kind == "movie") t.progress.toFloat()
                            else if (showPart) t.watched.toFloat() / t.fileCount else 0f,
                        watched = t.allWatched,
                        modifier = Modifier
                            .gridEdges(index, toolbarFocusRequester, firstRow = !hasContinue)
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
                            progress = f.playback?.fraction(f.duration) ?: 0f,
                            watched = f.playback?.watched == true,
                            modifier = Modifier
                                .gridEdges(index, toolbarFocusRequester, firstRow = listing.titles.isEmpty() && !hasContinue)
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
private const val CONTINUE_ROW = "continue-row"

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
    progress: Float = 0f,
    watched: Boolean = false,
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
            WatchProgressOverlay(progress, watched, barHeight = 5.dp, badgeSize = 28.dp,
                accent = TvAccent, onAccent = TvOnAccent)
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
    val scrollToTop: () -> Unit = { scope.launch { listState.animateScrollToItem(0) } }
    LaunchedEffect(title.kind, title.tmdbId) {
        awaitFrame()
        runCatching { playRequester.requestFocus() }
    }

    Box(modifier = Modifier.fillMaxSize()) {
        // Scroll only as far as needed to show the focused item, so the hero stays on screen
        // instead of the TV default of pinning focus a third of the way down.
        CompositionLocalProvider(LocalBringIntoViewSpec provides MinimalBringIntoView) {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp)
            ) {
                // Movie libraries list files, also for a title matched to a TV show (a TV movie).
                if (title.kind == "movie" || !title.files.isNullOrEmpty()) {
                    // The files sit in the hero's right column so the whole page fits on screen.
                    val files = title.files.orEmpty()
                    item(key = "hero") {
                        TitleHero(viewModel, title, onCastFocused = scrollToTop) {
                            FileRow(viewModel, files, files, playRequester, Modifier.padding(top = 16.dp), thumbnailWidth = 220.dp)
                        }
                    }
                } else {
                    item(key = "hero") { TitleHero(viewModel, title, onCastFocused = scrollToTop) }
                    // Bring the whole hero back whenever focus reaches the season picker or the cast,
                    // instead of scrolling only as far as the focused row.
                    showSections(viewModel, title, playRequester, onPickerFocused = scrollToTop)
                }
            }
        }
    }
}

/** A title's dimmed backdrop, drawn by the browser behind the whole screen, header included. */
@Composable
fun TitleBackdrop(viewModel: MediaViewModel, title: TitleDetail) {
    val backdrop = viewModel.serverUrl(title.backdrop) ?: return
    RemoteImage(backdrop, Modifier.fillMaxSize(), alpha = 0.22f)
    Box(
        Modifier
            .fillMaxSize()
            .background(Brush.horizontalGradient(listOf(Color.Black.copy(alpha = 0.7f), Color.Transparent)))
    )
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
private fun TitleHero(
    viewModel: MediaViewModel,
    title: TitleDetail,
    onCastFocused: () -> Unit = {},
    below: @Composable () -> Unit = {}
) {
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
            title.cast?.takeIf { it.isNotEmpty() }?.let { CastRow(viewModel, it, onCastFocused) }
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
private fun CastRow(viewModel: MediaViewModel, cast: List<CreditInfo>, onFocused: () -> Unit) {
    // Focusable so the whole cast can be scrolled through with the D-pad; nothing happens on click.
    var hadFocus by remember { mutableStateOf(false) }
    LazyRow(
        horizontalArrangement = Arrangement.spacedBy(14.dp),
        contentPadding = PaddingValues(horizontal = FocusBleed, vertical = 6.dp),
        modifier = Modifier
            .padding(top = 8.dp)
            .focusBleed()
            .onFocusChanged {
                if (it.hasFocus && !hadFocus) onFocused()
                hadFocus = it.hasFocus
            }
    ) {
        itemsIndexed(cast, key = { i, person -> "$i:${person.tmdbId}" }) { _, person ->
            Column(
                horizontalAlignment = Alignment.CenterHorizontally,
                modifier = Modifier
                    .width(120.dp)
                    .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.05f, borderWidth = 2.dp, onClick = {})
                    .padding(6.dp)
            ) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .size(72.dp)
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

/**
 * A title's files, one per row: the video thumbnail (which plays it) with the full file name, its
 * size and a Play button to the right.
 */
@Composable
private fun FileRow(
    viewModel: MediaViewModel,
    files: List<LibraryFile>,
    queue: List<LibraryFile>,
    firstRequester: FocusRequester?,
    modifier: Modifier = Modifier,
    thumbnailWidth: Dp = 200.dp
) {
    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(16.dp)) {
        files.forEachIndexed { index, file ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    contentAlignment = Alignment.Center,
                    modifier = Modifier
                        .width(thumbnailWidth)
                        .aspectRatio(16f / 9f)
                        .tvClickable(shape = MaterialTheme.shapes.medium, focusedScale = 1.05f, borderWidth = 3.dp,
                            onClick = { viewModel.playLibraryFile(file, queue) })
                        .clip(MaterialTheme.shapes.medium)
                        .background(MaterialTheme.colorScheme.surfaceVariant)
                ) {
                    Icon(Icons.Default.Movie, contentDescription = null, modifier = Modifier.size(40.dp))
                    viewModel.serverUrl(file.thumbnailUrl)?.let { RemoteImage(it, Modifier.fillMaxSize()) }
                    PlaybackOverlay(file.playback, file.duration, barHeight = 5.dp, badgeSize = 26.dp,
                        accent = TvAccent, onAccent = TvOnAccent)
                }
                Spacer(Modifier.width(18.dp))
                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    file.match?.let { m ->
                        Text(m.title + (m.year?.let { " ($it)" } ?: ""), style = MaterialTheme.typography.titleSmall)
                    }
                    Text(file.name, style = MaterialTheme.typography.bodyMedium) // the whole name, wrapped
                    formatBytes(file.size).takeIf { it.isNotEmpty() }?.let {
                        Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    timeLeftText(file.playback, file.duration)?.let {
                        Text(it, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurface)
                    }
                    // Play (an icon), or Resume with where it picks up; Play from start; Mark watched.
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        val resumeAt = file.playback?.resumeAt ?: 0.0
                        Button(colors = tvButtonColors(),
                            onClick = { viewModel.playLibraryFile(file, queue) },
                            modifier = if (index == 0 && firstRequester != null) Modifier.focusRequester(firstRequester) else Modifier
                        ) {
                            Icon(Icons.Default.PlayArrow, contentDescription = if (resumeAt > 0) null else "Play")
                            if (resumeAt > 0) {
                                Spacer(Modifier.width(6.dp))
                                Text("Resume ${clockText(resumeAt)}")
                            }
                        }
                        if (resumeAt > 0) {
                            Button(colors = tvButtonColors(), onClick = { viewModel.playLibraryFile(file, queue, fromStart = true) }) {
                                Icon(Icons.Default.Replay, contentDescription = "Play from start")
                            }
                        }
                        TvWatchedButton(viewModel, listOf(file))
                    }
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
        val pickerRequester = remember { FocusRequester() }
        var pickerFocused by remember { mutableStateOf(false) }
        LazyRow(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            contentPadding = PaddingValues(horizontal = FocusBleed),
            modifier = Modifier.focusBleed().onFocusChanged {
                if (it.hasFocus && !pickerFocused) onPickerFocused()
                pickerFocused = it.hasFocus
            }
        ) {
            itemsIndexed(seasons, key = { _, s -> s.season }) { index, s ->
                val isSelected = index == selected
                Button(
                    onClick = { selected = index },
                    modifier = if (isSelected) Modifier.focusRequester(pickerRequester) else Modifier,
                    // The season shown: a button among plain labels (themed buttons, as on the web),
                    // or in the primary color.
                    colors = when {
                        hasThemedButtons && isSelected -> tvButtonColors()
                        hasThemedButtons -> tvButtonColors(containerColor = Color.Transparent)
                        isSelected -> tvButtonColors(
                            containerColor = MaterialTheme.colorScheme.primary,
                            contentColor = MaterialTheme.colorScheme.onPrimary
                        )
                        else -> tvButtonColors()
                    }
                ) {
                    // Episodes watched (any of an episode's files), with a check once they all are.
                    val watched = s.episodes.count { e -> e.files.any { it.playback?.watched == true } }
                    Text(if (watched > 0) "${s.name} ($watched/${s.episodes.size})" else "${s.name} (${s.episodes.size})")
                    if (watched == s.episodes.size) {
                        Spacer(Modifier.width(6.dp))
                        Icon(Icons.Default.Check, contentDescription = "Watched", modifier = Modifier.size(18.dp))
                    }
                }
            }
            item(key = "season-watched") {
                TvWatchedButton(viewModel, season.episodes.flatMap { it.files }, label = "Season")
            }
            item(key = "show-watched") {
                TvWatchedButton(viewModel, seasons.flatMap { s -> s.episodes.flatMap { it.files } }, label = "Show")
            }
        }
        season.episodes.forEachIndexed { index, episode ->
            // Up from the first episode goes to the season picker (Compose's own search can pick the
            // cast instead, since the full-width row lines up with it too).
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f)) {
                    EpisodeRow(viewModel, episode, queue, if (index == 0) playRequester else null,
                        up = if (index == 0) pickerRequester else null)
                }
                // Selecting the row resumes; these start over and mark it watched.
                Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(start = 12.dp)) {
                    val first = episode.files.firstOrNull()
                    if (first != null && (first.playback?.resumeAt ?: 0.0) > 0) {
                        Button(colors = tvButtonColors(), onClick = { viewModel.playLibraryFile(first, queue, fromStart = true) }) {
                            Icon(Icons.Default.Replay, contentDescription = "Play from start")
                        }
                    }
                    TvWatchedButton(viewModel, episode.files, iconOnly = true)
                }
            }
        }
    }
}

@Composable
private fun EpisodeRow(
    viewModel: MediaViewModel,
    episode: LibraryEpisode,
    queue: List<LibraryFile>,
    requester: FocusRequester?,
    up: FocusRequester? = null
) {
    val first = episode.files.firstOrNull() ?: return
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .then(if (requester != null) Modifier.focusRequester(requester) else Modifier)
            .then(if (up != null) Modifier.focusProperties { this.up = up } else Modifier)
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
            PlaybackOverlay(first.playback, first.duration, barHeight = 5.dp, badgeSize = 24.dp,
                accent = TvAccent, onAccent = TvOnAccent)
        }
        Spacer(Modifier.width(20.dp))
        Column(modifier = Modifier.weight(1f)) {
            // The file (or files) on disk, then a blank line, then the episode it shows as.
            Text(episode.files.joinToString("\n") { it.name }, style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(14.dp))
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
            first.playback?.resumeAt?.takeIf { it > 0 }?.let {
                Text("Resumes at ${clockText(it)}" + (timeLeftText(first.playback, first.duration)?.let { l -> " · $l" } ?: ""),
                    style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.padding(top = 4.dp))
            }
        }
    }
}

/**
 * Marks [files] watched, or unwatched when they all are: an icon, with a [label] ("Season",
 * "Show") where buttons for different groups sit side by side.
 */
@Composable
private fun TvWatchedButton(viewModel: MediaViewModel, files: List<LibraryFile>, label: String? = null, iconOnly: Boolean = false) {
    if (files.isEmpty()) return
    val watched = files.all { it.playback?.watched == true }
    // Watched: a filled check; not yet: an empty circle (button colors alone don't tell them apart).
    Button(colors = tvButtonColors(), onClick = { viewModel.setWatched(files.map { it.path }, !watched) }) {
        Icon(if (watched) Icons.Default.CheckCircle else Icons.Outlined.Circle,
            contentDescription = if (watched) "Mark unwatched" else "Mark watched")
        if (!iconOnly && label != null) {
            Spacer(Modifier.width(6.dp))
            Text(label)
        }
    }
}

@Composable
internal fun RemoteImage(url: String, modifier: Modifier, alpha: Float = 1f) {
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

internal fun formatBytes(size: Long): String {
    if (size <= 0) return ""
    val units = arrayOf("B", "KB", "MB", "GB", "TB")
    val group = (Math.log10(size.toDouble()) / Math.log10(1024.0)).toInt().coerceIn(0, units.size - 1)
    return String.format(Locale.US, "%.1f %s", size / Math.pow(1024.0, group.toDouble()), units[group])
}
