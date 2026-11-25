package com.vargon.weirdstreamer.ui.components

import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import com.google.gson.JsonObject
import com.vargon.weirdstreamer.api.models.MatchHit
import com.vargon.weirdstreamer.api.models.MatchInfo
import com.vargon.weirdstreamer.api.models.PosterList
import com.vargon.weirdstreamer.api.models.TmdbEpisode
import com.vargon.weirdstreamer.api.models.TmdbSeason
import com.vargon.weirdstreamer.repository.ServerRepository
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch

// ---------------------------------------------------------------------------------------------
// Fixing a file's or folder's TMDB match (admins), as the web's Fix dialog: the current match, a
// TMDB search to pick another (or none), the poster, and an episode file's TMDB episode. The phone
// (PhoneMatchSheet.kt) and the TV (TvMatchDialog.kt) draw their own screens on this model.

/**
 * What Fix applies to: a file (its client path) or a folder that stands for one title. [wholeTitle]:
 * opened for a title page's whole title (through its first file), so its episode isn't offered.
 */
data class MatchTarget(val path: String, val isDir: Boolean, val name: String, val wholeTitle: Boolean = false) {
    /** The request's path (base64 client path) or dir (client path), the other null. */
    val pathParam: String? get() = if (isDir) null else android.util.Base64.encodeToString(path.toByteArray(), android.util.Base64.NO_WRAP)
    val dirParam: String? get() = if (isDir) path else null

    fun body(): JsonObject = JsonObject().apply {
        if (isDir) addProperty("dir", path) else addProperty("path", pathParam)
    }
}

/** A poster to pick: its TMDB path (what the server takes), a preview URL and a label. */
data class PosterPick(val path: String, val url: String, val label: String)

class MatchModel(val server: ServerRepository, val target: MatchTarget, private val scope: CoroutineScope) {
    var info by mutableStateOf<MatchInfo?>(null); private set
    var loadError by mutableStateOf<String?>(null); private set
    var message by mutableStateOf<Notice?>(null)
    /** Something was changed: the screen behind refreshes when the dialog closes. */
    var changed = false; private set

    var query by mutableStateOf("")
    var year by mutableStateOf("")
    /** What's searched, "movie" or "tv" (a movie library can search shows: TV movies). */
    var searchKind by mutableStateOf("movie"); private set
    var hits by mutableStateOf<List<MatchHit>?>(null); private set
    var searching by mutableStateOf(false); private set

    var posters by mutableStateOf<PosterList?>(null); private set
    private var postersFor = 0
    var postersError by mutableStateOf<String?>(null); private set

    var seasons by mutableStateOf<List<TmdbSeason>?>(null); private set
    private var seasonsFor = 0
    var season by mutableStateOf<Int?>(null); private set
    var episodes by mutableStateOf<List<TmdbEpisode>?>(null); private set
    var episodesError by mutableStateOf<String?>(null); private set
    /** The episode picked in the list, to apply to this file or its whole season. */
    var pick by mutableStateOf<Pair<Int, Int>?>(null)

    private var searchJob: Job? = null

    suspend fun load() {
        val (got, err) = attempt { server.api.getMatch(target.pathParam, target.dirParam) }
        loadError = err
        got ?: return
        query = got.query.orEmpty()
        year = got.year?.takeIf { it > 0 }?.toString().orEmpty()
        searchKind = got.matchKind ?: got.kind
        applyInfo(got)
        search()
    }

    /** The kinds the switch offers (two when the library can match either). */
    val kinds: List<String> get() = info?.kinds.orEmpty()

    /** A TV-library file that isn't an episode: matched on its own, by hand only. */
    val singleFile: Boolean get() = info?.let { it.kind == "tv" && it.kinds?.size == 1 && it.kinds[0] == "movie" } == true

    val matchedKind: String get() = info?.let { it.matchKind ?: it.kind } ?: "movie"

    /** "Movie matched by hand: Heat (1995)", or what's wrong. */
    val statusText: String get() {
        val i = info ?: return ""
        val kindLabel = if (matchedKind == "tv") "TV show" else "Movie"
        i.current?.let { c -> return "$kindLabel matched${if (i.manual) " by hand" else ""}: ${c.title}${c.year?.let { " ($it)" } ?: ""}" }
        return when (i.status) {
            "not_found" -> if (i.manual) "Marked as no match." else "No ${kindLabel.lowercase()} match found."
            "error" -> "The last lookup failed."
            "pending" -> if (singleFile) "Not matched. Files that aren't episodes are only matched by hand." else "Not matched yet (lookup pending)."
            "matched" -> "Matched; fetching details…"
            else -> i.status
        }
    }

    val hint: String get() = if (singleFile) "Choosing a result applies to this file only."
        else "Choosing a result applies to every file in this library with the same title."

    val resetLabel: String get() = if (singleFile) "Clear match" else "Reset to automatic"

    fun isCurrent(hit: MatchHit) = info?.current?.tmdbId == hit.tmdbId && matchedKind == searchKind

    fun switchKind(kind: String) {
        if (kind == searchKind) return
        searchKind = kind
        search()
    }

    fun search() {
        val q = query.trim()
        if (q.isEmpty() || info == null) return
        searchJob?.cancel()
        searchJob = scope.launch {
            searching = true
            val kind = searchKind
            val (got, err) = attempt { server.api.searchMatch(kind, q, year.trim().ifEmpty { null }) }
            if (kind != searchKind) return@launch
            searching = false
            if (err != null) message = "Search failed: $err" to true
            hits = got.orEmpty()
        }
    }

    fun choose(hit: MatchHit) = change("Matched to ${hit.title}. Fetching details…") {
        server.api.setMatch(target.body().apply { addProperty("tmdb_id", hit.tmdbId); addProperty("kind", searchKind) })
    }

    fun markNoMatch() = change("Marked as no match.") {
        server.api.setMatch(target.body().apply { addProperty("tmdb_id", 0); addProperty("kind", "") })
    }

    fun reset() {
        val single = singleFile
        change(if (single) "Match cleared." else "Reset. Matching automatically…") { server.api.resetMatch(target.pathParam, target.dirParam) }
    }

    /** The posters to pick from: the title's own, then other search results'. */
    val ownPosters: List<PosterPick> get() = posters?.posters.orEmpty().map {
        PosterPick(it.path, it.url, if (it.language.isEmpty()) "No text" else it.language.uppercase())
    }
    val otherPosters: List<PosterPick> get() {
        val current = info?.current ?: return emptyList()
        return hits.orEmpty().filter { !(it.tmdbId == current.tmdbId && searchKind == matchedKind) && !it.posterPath.isNullOrEmpty() }
            .map { PosterPick(it.posterPath!!, it.poster.orEmpty(), it.title + (it.year?.let { y -> " ($y)" } ?: "")) }
    }

    fun choosePoster(path: String) = change(if (path.isEmpty()) "Poster reset to the default." else "Poster changed.") {
        postersFor = 0 // asked again, for the new current one
        server.api.setMatchPoster(target.body().apply { addProperty("poster", path) })
    }

    /** The file's episode, for the episode section (none when fixing the whole title). */
    val episode get() = info?.episode?.takeIf { !target.wholeTitle }

    /** The episode section: the TMDB show the file's matched to, or null. */
    val show get() = info?.current?.takeIf { it.kind == "show" }

    fun chooseSeason(number: Int) {
        val showId = show?.tmdbId ?: return
        season = number
        pick = null
        episodes = null
        episodesError = null
        scope.launch {
            val (got, err) = attempt { server.api.matchEpisodes(showId, number) }
            if (season != number) return@launch
            episodesError = err?.let { "Could not load episodes: $it" }
            episodes = got.orEmpty()
        }
    }

    fun applyEpisode(wholeSeason: Boolean) {
        val (s, e) = pick ?: return
        change(if (wholeSeason) "Season mapped. Fetching episode details…" else "Episode set for this file.") {
            server.api.setEpisodeMapping(target.body().apply {
                addProperty("season", s); addProperty("episode", e); addProperty("whole_season", wholeSeason)
            })
        }
    }

    fun clearEpisode(wholeSeason: Boolean) = change("Episode mapping cleared.") {
        server.api.clearEpisodeMapping(target.pathParam ?: return@change null, if (wholeSeason) 1 else null)
    }

    /** "Use S02E05 for all of season 1 (→ season 2, episode numbers +4)" */
    fun wholeSeasonLabel(): String {
        val ep = info?.episode ?: return ""
        val (s, e) = pick ?: return ""
        val shift = e - ep.fileEpisode
        val shiftText = if (shift == 0) "same episode numbers" else "episode numbers ${if (shift > 0) "+" else ""}$shift"
        return "Use for all of season ${ep.fileSeason} (→ season $s, $shiftText)"
    }

    /** Sends a change; its answer is the new match info. */
    private fun change(done: String, request: suspend () -> retrofit2.Response<MatchInfo>?) {
        scope.launch {
            val (got, err) = try {
                request()?.let { r -> r.body() to r.problem() } ?: (null to "Not a file")
            } catch (e: Exception) {
                null to (e.localizedMessage ?: "Couldn't reach the server")
            }
            if (err != null) {
                message = "Could not save: $err" to true
                return@launch
            }
            changed = true
            message = done to false
            got?.let { applyInfo(it) }
        }
    }

    /** Takes new match info: loads the posters and seasons it needs. */
    private fun applyInfo(got: MatchInfo) {
        info = got
        pick = null
        val current = got.current
        if (current == null) {
            posters = null; postersFor = 0
        } else if (postersFor != current.tmdbId) {
            postersFor = current.tmdbId
            posters = null
            postersError = null
            scope.launch {
                val (list, err) = attempt { server.api.matchPosters(target.pathParam, target.dirParam) }
                if (postersFor != current.tmdbId) return@launch
                postersError = err?.let { "Posters unavailable: $it" }
                posters = list
            }
        }
        val ep = got.episode
        val showId = show?.tmdbId
        if (ep == null || showId == null) {
            seasons = null; seasonsFor = 0; season = null; episodes = null
        } else if (seasonsFor != showId) {
            seasonsFor = showId
            seasons = null
            scope.launch {
                val (list, err) = attempt { server.api.matchSeasons(showId) }
                if (seasonsFor != showId) return@launch
                if (err != null) episodesError = "Could not load seasons: $err"
                seasons = list.orEmpty()
                val all = list.orEmpty()
                chooseSeason(all.firstOrNull { it.number == ep.season }?.number ?: all.firstOrNull()?.number ?: return@launch)
            }
        } else if (season != ep.season && seasons?.any { it.number == ep.season } == true) {
            chooseSeason(ep.season)
        }
    }
}

@Composable
fun rememberMatchModel(server: ServerRepository, target: MatchTarget): MatchModel {
    val scope = rememberCoroutineScope()
    val model = remember(server.address, target) { MatchModel(server, target, scope) }
    LaunchedEffect(model) { model.load() }
    return model
}

/** "S01E05" */
internal fun episodeCode(season: Int, episode: Int) = "S%02dE%02d".format(season, episode)

/** "This file is S01E05, shown as S01E05 “Pilot” (from the file name)." */
internal fun episodeSummary(ep: com.vargon.weirdstreamer.api.models.MatchEpisodeInfo): String {
    val source = when (ep.source) {
        "file" -> "from the file name"
        "season" -> "from a season mapping"
        "manual" -> "set for this file"
        else -> ep.source
    }
    val shownAs = episodeCode(ep.season, ep.episode) + (ep.title?.takeIf { it.isNotEmpty() }?.let { " “$it”" } ?: "")
    return "This file is ${episodeCode(ep.fileSeason, ep.fileEpisode)}, shown as $shownAs ($source)."
}

/** "Season 2 – The Return (10)", "Specials (3)" */
internal fun seasonLabel(s: TmdbSeason): String {
    val base = if (s.number == 0) "Specials" else "Season ${s.number}"
    val name = s.name?.takeIf { it.isNotEmpty() && !Regex("^(Season \\d+|Specials)$").matches(it) }
    return base + (name?.let { " – $it" } ?: "") + " (${s.episodeCount})"
}

/** The file a title page's Fix goes through: a movie's first file, else a show's first episode's. */
internal fun titleFixFile(title: com.vargon.weirdstreamer.api.models.TitleDetail): com.vargon.weirdstreamer.api.models.LibraryFile? =
    title.files?.firstOrNull()
        ?: title.seasons.orEmpty().flatMap { it.episodes }.firstNotNullOfOrNull { it.files.firstOrNull() }
