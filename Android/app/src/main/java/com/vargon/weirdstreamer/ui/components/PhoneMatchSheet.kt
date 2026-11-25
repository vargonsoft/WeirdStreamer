package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.vargon.weirdstreamer.api.models.MatchHit
import com.vargon.weirdstreamer.repository.ServerRepository

/**
 * The phone's Fix dialog (admins): the current match, its episode for an episode file, a TMDB
 * search whose results match it when tapped, and its poster. [onDismiss] gets whether anything
 * changed, so the screen behind can reload.
 */
@Composable
fun PhoneMatchSheet(server: ServerRepository, target: MatchTarget, onDismiss: (changed: Boolean) -> Unit) {
    val m = rememberMatchModel(server, target)
    var confirmNone by remember { mutableStateOf(false) }
    Dialog(onDismissRequest = { onDismiss(m.changed) }, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background)) {
            Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("Fix match", style = MaterialTheme.typography.titleLarge, color = MaterialTheme.colorScheme.primary)
                    Text(target.name, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
                IconButton(onClick = { onDismiss(m.changed) }) { Icon(Icons.Default.Close, contentDescription = "Close") }
            }
            LazyColumn(Modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)) {
                m.loadError?.let { err -> item { Text(err, color = MaterialTheme.colorScheme.error) } }
                val info = m.info
                if (info == null) {
                    if (m.loadError == null) item { Text("Loading…") }
                    return@LazyColumn
                }
                item { CurrentMatch(m, onNoMatch = { confirmNone = true }) }
                m.message?.let { (text, error) ->
                    item { Text(text, color = if (error) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary) }
                }
                if (m.episode != null) item { EpisodeSection(m) }
                item { SearchSection(m) }
                val hits = m.hits
                when {
                    m.searching -> item { Text("Searching…") }
                    hits != null && hits.isEmpty() -> item { Text("No results" + if (m.year.isNotBlank()) " (try without the year)." else ".") }
                    hits != null -> items(hits, key = { "${it.tmdbId}" }) { hit -> HitRow(m, hit) }
                }
                if (info.current != null) item { PosterSection(m) }
            }
        }
    }
    if (confirmNone) {
        AlertDialog(
            onDismissRequest = { confirmNone = false },
            title = { Text("Mark as no match?") },
            text = { Text("Automatic matching will leave it alone.") },
            confirmButton = { PhoneButton(onClick = { confirmNone = false; m.markNoMatch() }) { Text("Mark as no match") } },
            dismissButton = { PhoneSecondaryButton(onClick = { confirmNone = false }) { Text("Cancel") } },
        )
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun CurrentMatch(m: MatchModel, onNoMatch: () -> Unit) {
    val current = m.info?.current
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically) {
            current?.poster?.let { p -> m.server.absoluteUrl(p)?.let { RemoteImage(it, Modifier.width(60.dp).height(90.dp).clip(RoundedCornerShape(4.dp))) } }
            Column(Modifier.weight(1f)) {
                Text(m.statusText, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.SemiBold)
                Text(m.hint, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            PhoneSecondaryButton(onClick = onNoMatch) { Text("Mark as no match") }
            if (m.info?.manual == true) PhoneSecondaryButton(onClick = { m.reset() }) { Text(m.resetLabel) }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SearchSection(m: MatchModel) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Search TMDB", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
        if (m.kinds.size > 1) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                m.kinds.forEach { k ->
                    FilterChip(selected = m.searchKind == k, onClick = { m.switchKind(k) },
                        label = { Text(if (k == "tv") "TV shows" else "Movies") })
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(value = m.query, onValueChange = { m.query = it }, label = { Text("Title") }, singleLine = true,
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search), keyboardActions = KeyboardActions(onSearch = { m.search() }),
                modifier = Modifier.weight(1f))
            OutlinedTextField(value = m.year, onValueChange = { v -> m.year = v.filter { it.isDigit() }.take(4) }, label = { Text("Year") },
                singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { m.search() }), modifier = Modifier.width(90.dp))
        }
        PhoneButton(onClick = { m.search() }, enabled = m.query.isNotBlank()) { Text("Search") }
    }
}

@Composable
private fun HitRow(m: MatchModel, hit: MatchHit) {
    val current = m.isCurrent(hit)
    val shape = RoundedCornerShape(8.dp)
    Row(
        Modifier.fillMaxWidth().clip(shape)
            .then(if (current) Modifier.border(BorderStroke(2.dp, MaterialTheme.colorScheme.primary), shape) else Modifier)
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f))
            .clickable { m.choose(hit) }.padding(8.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Box(Modifier.width(46.dp).height(69.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceVariant)) {
            hit.poster?.let { RemoteImage(it, Modifier.fillMaxSize()) }
        }
        Column(Modifier.weight(1f)) {
            Text(hit.title + (hit.year?.let { " ($it)" } ?: "") + if (current) " — current" else "",
                style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.SemiBold)
            Text(hit.overview?.takeIf { it.isNotBlank() } ?: "No synopsis.", style = MaterialTheme.typography.bodySmall,
                maxLines = 3, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
private fun PosterSection(m: MatchModel) {
    val list = m.posters
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("Poster", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
            if (list != null) Text(if (list.manual) "Picked by hand." else "TMDB's default.", style = MaterialTheme.typography.bodySmall)
            Spacer(Modifier.weight(1f))
            if (list?.manual == true) TextButton(onClick = { m.choosePoster("") }) { Text("Use default") }
        }
        m.postersError?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
        if (list == null) {
            if (m.postersError == null) Text("Loading posters…", style = MaterialTheme.typography.bodySmall)
            return@Column
        }
        val own = m.ownPosters
        Text("${m.info?.current?.title}'s posters (${own.size})", style = MaterialTheme.typography.bodySmall)
        if (own.isEmpty()) Text("TMDB has no posters for this title.", style = MaterialTheme.typography.bodySmall)
        else PosterStrip(own, list.current) { m.choosePoster(it) }
        val others = m.otherPosters
        if (others.isNotEmpty()) {
            Text("From other search results", style = MaterialTheme.typography.bodySmall)
            PosterStrip(others, list.current) { m.choosePoster(it) }
        }
    }
}

@Composable
private fun PosterStrip(posters: List<PosterPick>, current: String, onPick: (String) -> Unit) {
    LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        items(posters, key = { it.path }) { p ->
            val shape = RoundedCornerShape(4.dp)
            Column(Modifier.width(80.dp).clickable { onPick(p.path) }, horizontalAlignment = Alignment.CenterHorizontally) {
                Box(Modifier.width(80.dp).height(120.dp).clip(shape)
                    .then(if (p.path == current) Modifier.border(BorderStroke(3.dp, MaterialTheme.colorScheme.primary), shape) else Modifier)) {
                    RemoteImage(p.url, Modifier.fillMaxSize())
                }
                Text(p.label, style = MaterialTheme.typography.labelSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun EpisodeSection(m: MatchModel) {
    val ep = m.episode ?: return
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Episode", style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
        Text(episodeSummary(ep), style = MaterialTheme.typography.bodyMedium)
        if (m.show == null) {
            Text("Match the show first to pick a different episode.", style = MaterialTheme.typography.bodySmall)
            return@Column
        }
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (ep.source == "manual") PhoneSecondaryButton(onClick = { m.clearEpisode(false) }) { Text("Clear this file’s fix") }
            if (ep.source == "season") PhoneSecondaryButton(onClick = { m.clearEpisode(true) }) { Text("Clear season ${ep.fileSeason} mapping") }
        }
        val seasons = m.seasons
        if (!seasons.isNullOrEmpty()) {
            var open by remember { mutableStateOf(false) }
            Box {
                PhoneSecondaryButton(onClick = { open = true }) {
                    Text(seasons.firstOrNull { it.number == m.season }?.let { seasonLabel(it) } ?: "Season")
                }
                DropdownMenu(border = phoneMenuBorder(), expanded = open, onDismissRequest = { open = false }) {
                    seasons.forEach { s -> DropdownMenuItem(text = { Text(seasonLabel(s)) }, onClick = { open = false; m.chooseSeason(s.number) }) }
                }
            }
        }
        m.episodesError?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
        val season = m.season
        val episodes = m.episodes
        when {
            season == null -> {}
            episodes == null -> Text("Loading episodes…", style = MaterialTheme.typography.bodySmall)
            episodes.isEmpty() -> Text("No episodes in this season.", style = MaterialTheme.typography.bodySmall)
            else -> Column(Modifier.fillMaxWidth()) {
                episodes.forEach { e ->
                    val isCurrent = season == ep.season && e.number == ep.episode
                    val picked = m.pick == (season to e.number)
                    Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(4.dp))
                        .background(when {
                            picked -> MaterialTheme.colorScheme.primary.copy(alpha = 0.25f)
                            isCurrent -> MaterialTheme.colorScheme.surfaceVariant
                            else -> androidx.compose.ui.graphics.Color.Transparent
                        })
                        .clickable { m.pick = season to e.number }.padding(horizontal = 8.dp, vertical = 10.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        Text(episodeCode(season, e.number), fontWeight = if (isCurrent) FontWeight.Bold else FontWeight.Normal)
                        Text(e.name.orEmpty(), Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(e.airDate.orEmpty(), style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
        }
        m.pick?.let { (s, e) ->
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                PhoneButton(onClick = { m.applyEpisode(false) }) { Text("Use ${episodeCode(s, e)} for this file") }
                PhoneSecondaryButton(onClick = { m.applyEpisode(true) }) { Text(m.wholeSeasonLabel()) }
            }
        }
    }
}
