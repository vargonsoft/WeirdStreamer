package com.vargon.weirdstreamer.downloads

import android.content.Context
import com.vargon.weirdstreamer.api.models.ContinueItem
import com.vargon.weirdstreamer.repository.ServerRepository
import java.util.Locale

/**
 * Download ahead (phone, Settings › Downloads): watching an episode downloads the next few in the
 * background, so the show is there on a flight or when the connection drops. They go through the
 * download queue like any other (Wi-Fi only unless that's turned off), marked [DownloadItem.ahead],
 * and are removed once the user watches past them; downloads made by hand are never touched.
 */
object DownloadAhead {
    const val MIN_EPISODES = 3
    const val MAX_EPISODES = 5

    /** How far into an episode it counts as being watched, and the next ones are fetched. */
    const val AFTER_MS = 2 * 60 * 1000L

    // Room left on the phone after the episodes downloaded ahead: 500 MB, or 5% of it if that's more.
    private fun keepFree(dir: java.io.File) = maxOf(500L * 1024 * 1024, dir.totalSpace / 20)

    /**
     * How many episodes to have ahead: as many as download in about an hour at the speed downloads
     * have arrived here ([DownloadStore.speed]), from [MIN_EPISODES] to [MAX_EPISODES]; the fewest
     * until that's known.
     */
    fun episodes(episodeBytes: Long): Int {
        val speed = DownloadStore.speed
        if (speed <= 0 || episodeBytes <= 0) return MIN_EPISODES
        return (speed * 3600 / episodeBytes).toInt().coerceIn(MIN_EPISODES, MAX_EPISODES)
    }

    /**
     * For the episode at [sourcePath] on [repo]'s server, being watched: queues the next episodes
     * that aren't downloaded, and removes those downloaded ahead that come before it. [absolute]
     * makes the server's image URLs whole. Returns how many it queued; throws when the server
     * can't be reached, to be tried again.
     */
    suspend fun run(context: Context, repo: ServerRepository, sourcePath: String, absolute: (String?) -> String?): Int {
        val server = repo.address
        val account = DownloadStore.accountOf(server)
        val current = (repo.segments(sourcePath) ?: throw java.io.IOException("The server didn't answer")).metadata
        if (current?.kind != "episode") return 0
        removeWatched(server, account, sourcePath, current.showTitle, current.season, current.episode)

        val upcoming = repo.upcoming(sourcePath, MAX_EPISODES).filter { it.size > 0 } // an empty file won't download
        if (upcoming.isEmpty()) return 0
        // The quality and languages of the show's episodes downloaded by hand, else Compatibility (full
        // quality, playable here) in the languages playing would pick.
        val like = DownloadStore.mine(DownloadStore.items.value).lastOrNull { d ->
            !d.ahead && d.serverAddress == server && d.episode != null && d.episode?.show == showOf(current.showTitle)
        }
        var room = DownloadStore.directory.let { it.usableSpace - keepFree(it) }
        val wanted = upcoming.take(episodes(upcoming.first().size)).mapNotNull { e ->
            if (DownloadStore.find(server, e.path) != null) return@mapNotNull null
            val sizePct = like?.sizePct ?: 100
            room -= e.size * sizePct / 100
            if (room < 0) return@mapNotNull null
            DownloadItem(
                serverAddress = server,
                account = account,
                sourcePath = e.path,
                name = e.name,
                title = titleOf(e),
                thumbnailUrl = absolute(e.thumbnailUrl) ?: absolute(e.metadata?.still),
                sizePct = sizePct,
                scale = like?.scale ?: 100,
                compat = like?.compat ?: true,
                audioLanguage = like?.audioLanguage,
                subtitleLanguage = like?.subtitleLanguage,
                bytesTotal = e.size,
                ahead = true
            )
        }
        val added = DownloadStore.add(wanted)
        if (added > 0) DownloadWorker.schedule(context)
        return added
    }

    /** Removes the show's episodes downloaded ahead that come before the one being watched. */
    private fun removeWatched(server: String, account: String?, playing: String, show: String?, season: Int?, episode: Int?) {
        if (show == null || season == null || episode == null) return
        val key = showOf(show)
        DownloadStore.items.value
            .filter { it.ahead && it.serverAddress == server && it.account == account && it.sourcePath != playing }
            .filter { d -> d.episode?.let { e -> e.show == key && (e.season < season || (e.season == season && e.episode < episode)) } == true }
            .forEach { DownloadStore.remove(it.id) }
    }

    /** "Example Show S01E02 · Example Episode", the title a library download gets. */
    private fun titleOf(e: ContinueItem): String? {
        val md = e.metadata ?: return null
        if (md.showTitle == null || md.season == null || md.episode == null) return null
        return String.format(Locale.US, "%s S%02dE%02d · %s", md.showTitle, md.season, md.episode, md.title ?: e.name)
    }

    /** A show's key, as [DownloadItem.episode] has it. */
    private fun showOf(title: String?): String? =
        title?.let { DownloadItem(serverAddress = "", account = null, sourcePath = "", name = "", title = "$it S01E01").episode?.show }
}
