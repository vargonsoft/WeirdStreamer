package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.takeOrElse
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.tv.material3.MaterialTheme
import androidx.tv.material3.Text
import com.vargon.weirdstreamer.repository.ServerRepository
import com.vargon.weirdstreamer.ui.theme.WeirdStreamerTheme

/**
 * The TV's Fix dialog (admins): rows for the current match, the search (starting from the title
 * the server took from the file's name, so the keyboard is rarely needed), its results (OK matches
 * one), the poster and an episode file's episode, each opening its own dialog. Messages are toasts.
 * [onDismiss] gets whether anything changed.
 */
@Composable
fun TvMatchDialog(server: ServerRepository, target: MatchTarget, onDismiss: (changed: Boolean) -> Unit) {
    val m = rememberMatchModel(server, target)
    var editing by remember { mutableStateOf<String?>(null) } // "query" or "year"
    var confirmNone by remember { mutableStateOf(false) }
    var posters by remember { mutableStateOf(false) }
    var episode by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    val info = m.info
    LaunchedEffect(info != null) { if (info != null) runCatching { first.requestFocus() } }
    TvNoticeToast(m.message)

    TvDialogFrame("Fix match", onDismiss = { onDismiss(m.changed) }, width = 760) {
      // Opened by holding OK: its release mustn't press the first row
      Column(Modifier.okAfterFreshPress(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        TvNote(target.name)
        m.loadError?.let { TvNote(it) }
        if (info == null) {
            if (m.loadError == null) TvNote("Loading…")
            return@Column
        }
        Row(horizontalArrangement = Arrangement.spacedBy(16.dp), verticalAlignment = Alignment.CenterVertically) {
            info.current?.poster?.let { p ->
                server.absoluteUrl(p)?.let { RemoteImage(it, Modifier.width(60.dp).height(90.dp).clip(RoundedCornerShape(4.dp))) }
            }
            Column(Modifier.weight(1f)) {
                Text(m.statusText, style = MaterialTheme.typography.titleMedium)
                Text(m.hint, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        LazyColumn(Modifier.heightIn(max = 300.dp)) {
            item {
                TvValueRow("Search", m.query.ifBlank { "Type a title" }, focus = first) { editing = "query" }
            }
            item { TvValueRow("Year", m.year.ifBlank { "Any" }) { editing = "year" } }
            if (m.kinds.size > 1) item {
                TvValueRow("Search for", if (m.searchKind == "tv") "TV shows" else "Movies",
                    onStep = { m.switchKind(m.kinds[(m.kinds.indexOf(m.searchKind) + it).mod(m.kinds.size)]) }) {
                    m.switchKind(m.kinds[(m.kinds.indexOf(m.searchKind) + 1).mod(m.kinds.size)])
                }
            }
            val hits = m.hits
            when {
                m.searching -> item { TvNote("Searching…", Modifier.padding(horizontal = 14.dp, vertical = 8.dp)) }
                hits != null && hits.isEmpty() -> item {
                    TvNote("No results" + if (m.year.isNotBlank()) " (try without the year)." else ".", Modifier.padding(horizontal = 14.dp, vertical = 8.dp))
                }
                hits != null -> itemsIndexed(hits, key = { i, h -> "hit:$i:${h.tmdbId}" }) { _, hit ->
                    TvValueRow(hit.title + (hit.year?.let { " ($it)" } ?: ""), if (m.isCurrent(hit)) "Current" else "") { m.choose(hit) }
                }
            }
            m.episode?.let { ep -> item {
                TvValueRow("Episode", episodeCode(ep.season, ep.episode) + (ep.title?.takeIf { it.isNotEmpty() }?.let { " “$it”" } ?: "")) { episode = true }
            } }
            if (info.current != null) item {
                TvValueRow("Poster", if (m.posters?.manual == true) "Picked by hand" else "TMDB's default") { posters = true }
            }
            item { TvValueRow("Mark as no match") { confirmNone = true } }
            if (info.manual) item { TvValueRow(m.resetLabel) { m.reset() } }
        }
      }
    }

    when (editing) {
        "query" -> TvTextEntryDialog("Search TMDB for", m.query, onDismiss = { editing = null }) { m.query = it; m.search() }
        "year" -> TvTextEntryDialog("Year (empty for any)", m.year, TvEntryKind.Number, onDismiss = { editing = null }) {
            m.year = it.take(4); m.search()
        }
    }
    if (confirmNone) {
        TvConfirmDialog("Mark as no match?", "Automatic matching will leave it alone.", "Mark as no match",
            onDismiss = { confirmNone = false }) { m.markNoMatch() }
    }
    if (posters) TvPosterDialog(m, onDismiss = { posters = false })
    if (episode) TvEpisodeDialog(m, onDismiss = { episode = false })
}

/** The matched title's posters, then other search results': OK uses one. */
@Composable
private fun TvPosterDialog(m: MatchModel, onDismiss: () -> Unit) {
    val list = m.posters
    val first = remember { FocusRequester() }
    LaunchedEffect(list != null) { if (list != null) runCatching { first.requestFocus() } }
    TvDialogFrame("Poster", onDismiss, width = 860) {
        m.postersError?.let { TvNote(it) }
        if (list == null) {
            if (m.postersError == null) TvNote("Loading posters…")
            return@TvDialogFrame
        }
        TvNote(if (list.manual) "Picked by hand." else "TMDB's default.")
        val own = m.ownPosters
        val others = m.otherPosters
        if (list.manual) TvValueRow("Use default", focus = if (own.isEmpty() && others.isEmpty()) first else null) { m.choosePoster("") }
        Text("${m.info?.current?.title}'s posters (${own.size})", style = MaterialTheme.typography.titleSmall)
        if (own.isEmpty()) TvNote("TMDB has no posters for this title.")
        else TvPosterRow(own, list.current, first) { m.choosePoster(it) }
        if (others.isNotEmpty()) {
            Text("From other search results", style = MaterialTheme.typography.titleSmall)
            TvPosterRow(others, list.current, if (own.isEmpty()) first else null) { m.choosePoster(it) }
        }
    }
}

@Composable
private fun TvPosterRow(posters: List<PosterPick>, current: String, focus: FocusRequester?, onPick: (String) -> Unit) {
    val start = posters.indexOfFirst { it.path == current }.coerceAtLeast(0)
    val state = rememberLazyListState(initialFirstVisibleItemIndex = (start - 2).coerceAtLeast(0))
    LazyRow(state = state, horizontalArrangement = Arrangement.spacedBy(12.dp), modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        itemsIndexed(posters, key = { i, p -> "$i:${p.path}" }) { i, p ->
            val shape = RoundedCornerShape(6.dp)
            Column(Modifier.width(84.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Box(
                    Modifier.width(84.dp).height(126.dp)
                        .then(if (i == start && focus != null) Modifier.focusRequester(focus) else Modifier)
                        .tvClickable(shape, focusedScale = 1.06f, borderWidth = 3.dp) { onPick(p.path) }
                        .clip(shape)
                        .background(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.08f))
                ) {
                    RemoteImage(p.url, Modifier.fillMaxSize())
                }
                val label = (if (p.path == current) "✓ " else "") + p.label
                Text(label, style = MaterialTheme.typography.labelSmall, maxLines = 2, overflow = TextOverflow.Ellipsis,
                    color = if (p.path == current) WeirdStreamerTheme.spec.headingColor.takeOrElse { MaterialTheme.colorScheme.onSurface }
                        else MaterialTheme.colorScheme.onSurface)
            }
        }
    }
}

/** Which TMDB episode the file shows as: a season (Left and Right) and its episodes; OK on one asks this file or its season. */
@Composable
private fun TvEpisodeDialog(m: MatchModel, onDismiss: () -> Unit) {
    val ep = m.episode ?: run { onDismiss(); return }
    var applying by remember { mutableStateOf(false) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    TvDialogFrame("Episode", onDismiss, width = 760) {
        TvNote(episodeSummary(ep))
        if (m.show == null) {
            TvNote("Match the show first to pick a different episode.")
            TvValueRow("Close", focus = first) { onDismiss() }
            return@TvDialogFrame
        }
        val seasons = m.seasons.orEmpty()
        val season = m.season
        val current = seasons.firstOrNull { it.number == season }
        TvValueRow("Season", current?.let { seasonLabel(it) } ?: "…", focus = first,
            onStep = { by ->
                val i = seasons.indexOfFirst { it.number == season }
                seasons.getOrNull(i + by)?.let { m.chooseSeason(it.number) }
            }) {
            val i = seasons.indexOfFirst { it.number == season }
            seasons.getOrNull((i + 1).mod(seasons.size.coerceAtLeast(1)))?.let { m.chooseSeason(it.number) }
        }
        m.episodesError?.let { TvNote(it) }
        val episodes = m.episodes
        if (ep.source == "manual") TvValueRow("Clear this file’s fix") { m.clearEpisode(false); onDismiss() }
        if (ep.source == "season") TvValueRow("Clear season ${ep.fileSeason} mapping") { m.clearEpisode(true); onDismiss() }
        LazyColumn(Modifier.heightIn(max = 280.dp)) {
            when {
                season == null -> {}
                episodes == null -> item { TvNote("Loading episodes…", Modifier.padding(horizontal = 14.dp, vertical = 8.dp)) }
                episodes.isEmpty() -> item { TvNote("No episodes in this season.", Modifier.padding(horizontal = 14.dp, vertical = 8.dp)) }
                else -> itemsIndexed(episodes, key = { _, e -> "ep:$season:${e.number}" }) { _, e ->
                    val isCurrent = season == ep.season && e.number == ep.episode
                    TvValueRow("${episodeCode(season, e.number)}  ${e.name.orEmpty()}", if (isCurrent) "Current" else e.airDate.orEmpty()) {
                        m.pick = season to e.number
                        applying = true
                    }
                }
            }
        }
    }
    val pick = m.pick
    if (applying && pick != null) {
        val focus = remember { FocusRequester() }
        LaunchedEffect(Unit) { runCatching { focus.requestFocus() } }
        TvDialogFrame("Use ${episodeCode(pick.first, pick.second)}?", onDismiss = { applying = false }, width = 760) {
            TvValueRow("For this file", focus = focus) { applying = false; m.applyEpisode(false); onDismiss() }
            TvValueRow(m.wholeSeasonLabel()) { applying = false; m.applyEpisode(true); onDismiss() }
        }
    }
}
