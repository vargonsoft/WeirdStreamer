package com.vargon.weirdstreamer.api.models

import com.google.gson.annotations.SerializedName

data class User(
    @SerializedName("id") val id: Long,
    @SerializedName("username") val username: String,
    @SerializedName("created_at") val createdAt: String,
    @SerializedName("is_admin") val isAdmin: Boolean
)

data class MediaItem(
    @SerializedName("id") val id: Long,
    @SerializedName("path") val path: String,
    @SerializedName("name") val name: String,
    @SerializedName("type") val type: String, // video, image, document
    @SerializedName("size") val size: Long,
    @SerializedName("mime_type") val mimeType: String,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    @SerializedName("duration") val duration: Double? = null,
    @SerializedName("width") val width: Int? = null,
    @SerializedName("height") val height: Int? = null,
    @SerializedName("created_at") val createdAt: String,
    @SerializedName("updated_at") val updatedAt: String,
    @SerializedName("playback") val playback: Playback? = null,
    @SerializedName("live_video") val liveVideo: String? = null, // an Apple Live Photo's .MOV
    @SerializedName("format") val format: String? = null // a video's: "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"
)

/** Where the signed-in user is in a video (seconds); position is 0 once it's been watched. */
data class Playback(
    @SerializedName("position") val position: Double = 0.0,
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("watched") val watched: Boolean = false,
    @SerializedName("updated_at") val updatedAt: Long = 0
) {
    /** Where to resume, or 0 to start from the beginning. */
    val resumeAt: Double get() = if (position >= MIN_RESUME_SECONDS) position else 0.0

    /** How far in, 0..1, for a progress bar (0 when not in progress). */
    fun fraction(fallbackDuration: Double?): Float {
        val total = duration.takeIf { it > 0 } ?: fallbackDuration ?: 0.0
        return if (resumeAt > 0 && total > 0) (position / total).coerceIn(0.0, 1.0).toFloat() else 0f
    }

    companion object {
        const val MIN_RESUME_SECONDS = 10.0
    }
}

/** A video in progress, for Continue Watching. */
/** The home screen's Recently Added rows: a library's last videos to arrive. */
data class LibraryAdditions(
    @SerializedName("library_id") val libraryId: String,
    @SerializedName("library_name") val libraryName: String,
    @SerializedName("items") val items: List<ContinueItem> = emptyList()
)

data class ContinueItem(
    @SerializedName("id") val id: Long? = null,
    @SerializedName("path") val path: String,
    @SerializedName("name") val name: String,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    @SerializedName("duration") val duration: Double? = null,
    @SerializedName("library_id") val libraryId: String,
    @SerializedName("metadata") val metadata: ContinueMetadata? = null,
    @SerializedName("playback") val playback: Playback? = null,
    // The episode after one just finished, not started yet
    @SerializedName("up_next") val upNext: Boolean = false,
    // On Recently Added: when it arrived (Unix seconds)
    @SerializedName("added_at") val addedAt: Long? = null
) {
    /** "Example Show" and "S01E02 · Example Episode", or "Example Movie (2008)" and the file name. */
    val titleLines: Pair<String, String?>
        get() {
            val md = metadata
            return when {
                md?.kind == "episode" && md.season != null && md.episode != null -> (md.showTitle ?: md.title ?: name) to
                    String.format(java.util.Locale.US, "S%02dE%02d · %s", md.season, md.episode, md.title)
                md?.title != null -> (md.title + (md.year?.let { " ($it)" } ?: "")) to name
                else -> name to null
            }
        }
}

/** The episodes after a video (GET /api/media/upcoming), for Download ahead. */
data class UpcomingEpisodes(@SerializedName("episodes") val episodes: List<ContinueItem>? = null)

data class ContinueMetadata(
    @SerializedName("kind") val kind: String? = null,
    @SerializedName("title") val title: String? = null,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("show_title") val showTitle: String? = null,
    @SerializedName("season") val season: Int? = null,
    @SerializedName("episode") val episode: Int? = null,
    @SerializedName("still") val still: String? = null,
    @SerializedName("backdrop") val backdrop: String? = null,
    @SerializedName("poster") val poster: String? = null
) {
    /** The player's title: "Example Show · S01E02 · Example Episode", or "Example Movie (2008)". */
    val playerTitle: String?
        get() = when {
            kind == "episode" && season != null && episode != null ->
                listOfNotNull(showTitle, String.format(java.util.Locale.US, "S%02dE%02d", season, episode), title).joinToString(" · ")
            kind == "movie" && title != null -> title + (year?.let { " ($it)" } ?: "")
            else -> null
        }
}

data class WatchedRequest(
    @SerializedName("paths") val paths: List<String>, // base64 server paths
    @SerializedName("watched") val watched: Boolean,
    @SerializedName("library") val library: String? = null,
    @SerializedName("kind") val kind: String? = null,
    @SerializedName("tmdb_id") val tmdbId: Int? = null
)

data class SyncedPlayback(
    @SerializedName("path") val path: String, // base64 server path
    @SerializedName("position") val position: Double,
    @SerializedName("ended") val ended: Boolean,
    @SerializedName("updated_at") val updatedAt: Long // Unix milliseconds
)

data class PlaybackSync(@SerializedName("items") val items: List<SyncedPlayback>)

data class TrickplayFrame(
    @SerializedName("time") val time: Int,
    @SerializedName("url") val url: String
)

data class TrickplayInfo(
    @SerializedName("frames") val frames: List<TrickplayFrame>,
    // A disc image's frames: the title they were made from (its main feature)
    @SerializedName("title") val title: Int = 0
)

/** The frames to show while [discTitle] of a disc plays (0: its main feature), or all of a video's. */
fun TrickplayInfo.forDiscTitle(discTitle: Int, mainTitle: Int?): TrickplayInfo? =
    takeIf { title == 0 || discTitle == title || (discTitle == 0 && (mainTitle == null || mainTitle == title)) }

data class Directory(
    // `path` JSON key now carries the root ID or the relative id-based path
    @SerializedName("path") val id: String,
    @SerializedName("name") val name: String,
    @SerializedName("items") val items: List<MediaItem>? = null,
    @SerializedName("dirs") val dirs: List<Directory>? = null,
    // "movie" or "tv" for libraries with a type (on root entries and on browse responses)
    @SerializedName("library_type") val libraryType: String? = null,
    // A picture to show inside the folder's icon (folders without a poster): its thumbnail.png, else
    // a photo's or video's from inside it
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    // The folder stands for one title (a movie's or a show's folder), so its match can be fixed
    @SerializedName("fixable") val fixable: Boolean = false
)

// Library view: a movie or TV library as titles instead of folders

data class LibraryListing(
    @SerializedName("library_id") val libraryId: String,
    @SerializedName("library_name") val libraryName: String,
    @SerializedName("library_type") val libraryType: String,
    @SerializedName("titles") val titles: List<LibraryTitle> = emptyList(),
    @SerializedName("unmatched") val unmatched: List<LibraryFile> = emptyList(),
    // Movie libraries: the TMDB collections (film series) its movies belong to, by name
    @SerializedName("collections") val collections: List<LibraryCollection>? = null,
    // Movie libraries: the collections admins made, by name
    @SerializedName("user_collections") val userCollections: List<UserCollectionSummary>? = null
)

/** A library title by kind ("movie", or "show" for a TV movie) and TMDB ID, as collections name them. */
data class CollectionTitle(
    @SerializedName("kind") val kind: String,
    @SerializedName("tmdb_id") val tmdbId: Int
)

/** A collection admins made: its titles here and the posters of its first four by release date (its collage). */
data class UserCollectionSummary(
    @SerializedName("id") val id: Long,
    @SerializedName("name") val name: String,
    @SerializedName("description") val description: String? = null,
    @SerializedName("count") val count: Int = 0,
    @SerializedName("posters") val posters: List<String> = emptyList(),
    @SerializedName("titles") val titles: List<CollectionTitle> = emptyList()
)

/** A made collection's page: its titles by release date, with progress. */
data class UserCollectionDetail(
    @SerializedName("id") val id: Long,
    @SerializedName("name") val name: String,
    @SerializedName("description") val description: String? = null,
    @SerializedName("posters") val posters: List<String> = emptyList(),
    @SerializedName("parts") val parts: List<LibraryTitle> = emptyList()
)

/** A collection (a film series) in a movie library: its movies here ([count]) of its films on TMDB ([total]). */
data class LibraryCollection(
    @SerializedName("id") val id: Int,
    @SerializedName("name") val name: String,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("count") val count: Int = 0,
    @SerializedName("total") val total: Int = 0
)

/**
 * A collection's page (GET /api/library/collection): every film TMDB lists in it, by release date,
 * the library's as its titles and the rest [LibraryTitle.missing].
 */
data class CollectionDetail(
    @SerializedName("id") val id: Int,
    @SerializedName("name") val name: String,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("backdrop") val backdrop: String? = null,
    @SerializedName("parts") val parts: List<LibraryTitle> = emptyList()
)

data class LibraryTitle(
    @SerializedName("kind") val kind: String, // "movie" or "show"
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("file_count") val fileCount: Int = 0,
    // The user's progress: files watched, and a movie's place in it (0..1)
    @SerializedName("watched") val watched: Int = 0,
    @SerializedName("progress") val progress: Double = 0.0,
    @SerializedName("collection_id") val collectionId: Int = 0,
    // On a collection's page: a film of it the library doesn't have (not playable)
    @SerializedName("missing") val missing: Boolean = false,
    // On a person's page: their part in it
    @SerializedName("role") val role: String? = null
) {
    val allWatched: Boolean get() = fileCount > 0 && watched >= if (kind == "movie") 1 else fileCount
}

data class LibraryFile(
    @SerializedName("id") val id: Long? = null,
    @SerializedName("path") val path: String,
    @SerializedName("name") val name: String,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("duration") val duration: Double? = null,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
    @SerializedName("format") val format: String? = null, // "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"
    @SerializedName("count") val count: Int? = null,
    // The movie a TV-library file that isn't an episode was matched to by hand
    @SerializedName("match") val match: FileMatch? = null,
    @SerializedName("playback") val playback: Playback? = null
)

/** What a file downloads as (GET /api/downloads/options). */
data class DownloadOptions(
    @SerializedName("size") val size: Long = 0,
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0,
    @SerializedName("disc") val disc: Boolean = false,
    @SerializedName("sizes") val sizes: List<Int> = listOf(90, 75, 50, 25),
    @SerializedName("scales") val scales: List<Int> = listOf(100, 75, 50)
)

data class CreateDownloadRequest(
    @SerializedName("path") val path: String, // base64 server path
    @SerializedName("size_pct") val sizePct: Int,
    @SerializedName("scale") val scale: Int,
    // Compatibility: kept as they are (what this device plays); the rest is encoded
    @SerializedName("keep_video") val keepVideo: Boolean = false,
    @SerializedName("keep_audio") val keepAudio: Boolean = false,
    // The audio track to keep (among the file's), -1 its default
    @SerializedName("audio") val audio: Int = -1
)

/** A server-side encode for a compressed download. */
data class DownloadJob(
    @SerializedName("id") val id: String,
    @SerializedName("status") val status: String, // queued, encoding, ready, failed
    @SerializedName("progress") val progress: Double = 0.0,
    @SerializedName("error") val error: String? = null,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0
)

/** A user account on the server, as admins see it. */
data class ServerUser(
    @SerializedName("id") val id: Long,
    @SerializedName("username") val username: String,
    @SerializedName("is_admin") val isAdmin: Boolean,
    @SerializedName("group") val group: String? = null
)

data class CreateUserRequest(
    @SerializedName("username") val username: String,
    @SerializedName("password") val password: String,
    @SerializedName("is_admin") val isAdmin: Boolean,
    @SerializedName("group") val group: String
)

data class ChangePasswordRequest(
    @SerializedName("old_password") val oldPassword: String,
    @SerializedName("new_password") val newPassword: String
)

/** Settings → Jobs (admins): the server's background jobs, and the load they wait on. */
data class JobsResponse(
    @SerializedName("jobs") val jobs: List<BackgroundJob> = emptyList(),
    @SerializedName("load") val load: JobLoad? = null,
    @SerializedName("storage") val storage: List<StorageUse>? = null,
    @SerializedName("trickplay") val trickplay: TrickplaySettings? = null
)

/** The trickplay job's settings (kept in the server's config.yml) and their limits. [runners] and
 *  [framesPerRunner] are those in effect; [autoRunners] and [autoFrames] say they're automatic, which gives
 *  [autoRunnersValue] runners (one per disk the libraries are on) and [autoFramesFor] frames for 1 to
 *  [maxRunners] runners (`autoFramesFor[r - 1]`: what fills half the server's memory). */
data class TrickplaySettings(
    @SerializedName("runners") val runners: Int = 1,
    @SerializedName("frames_per_runner") val framesPerRunner: Int = 10,
    @SerializedName("auto_runners") val autoRunners: Boolean = false,
    @SerializedName("auto_frames") val autoFrames: Boolean = false,
    /** The GPU trickplay can decode on ("" none), and whether it does ([memory] and [autoFramesFor] are for the
     *  decoder in use). */
    @SerializedName("gpu") val gpu: String? = null,
    @SerializedName("use_gpu") val useGpu: Boolean = false,
    @SerializedName("auto_runners_value") val autoRunnersValue: Int = 0,
    @SerializedName("auto_frames_for") val autoFramesFor: List<Int>? = null,
    @SerializedName("max_runners") val maxRunners: Int = 8,
    @SerializedName("max_frames") val maxFrames: Int = 50,
    @SerializedName("max_product") val maxProduct: Int = 100,
    @SerializedName("memory") val memory: TrickplayMemory? = null
)

/** For the estimate beside the trickplay settings, for the decoder trickplay uses ([decoder]: cpu, vaapi or
 *  cuda): an ffmpeg process and each frame of its batch for a 4K video, the server's memory and swap, and for
 *  cuda the same in the card's own memory (0 when unknown). */
data class TrickplayMemory(
    @SerializedName("decoder") val decoder: String = "cpu",
    @SerializedName("card_process_bytes_4k") val cardProcessBytes4k: Long = 0,
    @SerializedName("card_frame_bytes_4k") val cardFrameBytes4k: Long = 0,
    @SerializedName("card_bytes") val cardBytes: Long = 0,
    @SerializedName("process_bytes_4k") val processBytes4k: Long = 0,
    @SerializedName("frame_bytes_4k") val frameBytes4k: Long = 0,
    @SerializedName("total_bytes") val totalBytes: Long = 0,
    @SerializedName("swap_bytes") val swapBytes: Long = 0
)

/** Trickplay's settings to save; those left null stay as they are (Gson leaves nulls out). */
data class TrickplaySettingsRequest(
    @SerializedName("runners") val runners: Int? = null,
    @SerializedName("frames_per_runner") val framesPerRunner: Int? = null,
    @SerializedName("use_gpu") val useGpu: Boolean? = null
)

/** One of the server's data folders and its size, for the Jobs tab's Storage card. */
data class StorageUse(
    @SerializedName("name") val name: String = "",
    @SerializedName("bytes") val bytes: Long = 0
)

/** One background job. Times are RFC 3339 strings, absent when unknown. */
data class BackgroundJob(
    @SerializedName("id") val id: String,
    @SerializedName("name") val name: String = "",
    @SerializedName("about") val about: String = "",
    @SerializedName("state") val state: String = "idle", // idle, running, waiting, paused
    @SerializedName("note") val note: String? = null, // why it's waiting
    @SerializedName("detail") val detail: String? = null, // how it's running ("2 of 3 runners")
    @SerializedName("runners") val runners: List<String>? = null, // what each runner is on ("": nothing)
    @SerializedName("done") val done: Int = 0,
    @SerializedName("total") val total: Int = 0,
    @SerializedName("started") val started: String? = null,
    @SerializedName("last_start") val lastStart: String? = null,
    @SerializedName("last_end") val lastEnd: String? = null,
    @SerializedName("last_result") val lastResult: String? = null,
    @SerializedName("next") val next: String? = null,
    @SerializedName("schedule") val schedule: String? = null,
    @SerializedName("paused") val paused: Boolean = false,
    @SerializedName("can_run") val canRun: Boolean = false
)

data class JobLoad(
    @SerializedName("available") val available: Boolean = false,
    @SerializedName("cpu") val cpu: Double = 0.0,
    @SerializedName("disk") val disk: Double = 0.0,
    @SerializedName("max_disk") val maxDisk: Double = 0.0,
    @SerializedName("relaxed_disk") val relaxedDisk: Double = 0.0, // the limit on a disk nobody plays from
    @SerializedName("shares") val shares: List<ShareLoad>? = null,
    @SerializedName("max_share_ms") val maxShareMs: Double = 0.0,
    @SerializedName("relaxed_share_ms") val relaxedShareMs: Double = 0.0, // ... and on a share nobody plays from
    @SerializedName("streaming") val streaming: Boolean = false,
    @SerializedName("pause_while_streaming") val pauseWhileStreaming: Boolean = false
)

/** A network share's read time (ms), from the Jobs load. */
data class ShareLoad(
    @SerializedName("library") val library: String = "",
    @SerializedName("ms") val ms: Double = 0.0
)

/** A stream playing on the server, from its admin Streams list. */
data class AdminStream(
    @SerializedName("id") val id: String,
    @SerializedName("user") val user: String = "",
    @SerializedName("client") val client: String = "",
    @SerializedName("client_ip") val clientIp: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("title") val title: String? = null,
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("state") val state: String = "",
    @SerializedName("mode") val mode: String = "",
    @SerializedName("bytes_sent") val bytesSent: Long = 0,
    @SerializedName("served_to") val servedTo: Double = 0.0,
    @SerializedName("position") val position: Double? = null,
    @SerializedName("width") val width: Int = 0,
    @SerializedName("height") val height: Int = 0,
    @SerializedName("transcode") val transcode: AdminTranscode? = null
)

data class AdminTranscode(
    @SerializedName("reason") val reason: String = "",
    @SerializedName("video") val video: String = "",
    @SerializedName("audio") val audio: String = "",
    @SerializedName("speed") val speed: Double = 0.0,
    @SerializedName("fps") val fps: Double = 0.0,
    @SerializedName("stalled") val stalled: Boolean = false,
    @SerializedName("source_video") val sourceVideo: String? = null
)

/** A DVD disc image's titles, for the player's disc controls. */
data class DiscInfo(
    @SerializedName("titles") val titles: List<DiscTitle> = emptyList(),
    @SerializedName("main_title") val mainTitle: Int = 1
) {
    fun title(number: Int): DiscTitle? =
        titles.firstOrNull { it.number == (if (number <= 0) mainTitle else number) } ?: titles.firstOrNull()
}

/** The title a progress report names: the main title is 0 even when picked by its number (the
 *  server keeps progress for the main title only, as the web and iPhone send it). */
fun DiscInfo?.progressTitle(discTitle: Int): Int = if (this != null && discTitle == mainTitle) 0 else discTitle

data class DiscTitle(
    @SerializedName("number") val number: Int,
    @SerializedName("duration") val duration: Double, // seconds
    @SerializedName("angles") val angles: Int = 1,
    @SerializedName("chapters") val chapters: List<Double> = emptyList(), // start times, seconds
    @SerializedName("audio") val audio: List<DiscTrack> = emptyList(),
    @SerializedName("subtitles") val subtitles: List<DiscTrack> = emptyList()
)

data class DiscTrack(
    @SerializedName("index") val index: Int,
    @SerializedName("language") val language: String? = null,
    @SerializedName("codec") val codec: String = "",
    @SerializedName("channels") val channels: Int = 0
) {
    val label: String
        get() = listOfNotNull(
            language?.takeIf { it.isNotBlank() } ?: "Unknown",
            codec.takeIf { it.isNotBlank() },
            when (channels) { 0 -> null; 2 -> "Stereo"; 6 -> "5.1"; else -> "$channels ch" }
        ).joinToString(" · ")
}

/** The player's position, reported for the server's admin Streams page. */
data class StreamProgress(
    @SerializedName("path") val path: String, // base64 server path
    @SerializedName("position") val position: Double, // seconds
    @SerializedName("paused") val paused: Boolean,
    @SerializedName("ended") val ended: Boolean = false, // played to the end
    @SerializedName("title") val title: Int = 0 // discs: the title playing (0: the main title)
)

data class FileMatch(
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null
)

data class CreditInfo(
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("name") val name: String,
    @SerializedName("character") val character: String? = null,
    @SerializedName("profile") val profile: String? = null
)

data class LibraryEpisode(
    @SerializedName("season") val season: Int,
    @SerializedName("episode") val episode: Int,
    @SerializedName("title") val title: String,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("air_date") val airDate: String? = null,
    @SerializedName("runtime") val runtime: Int? = null,
    @SerializedName("still") val still: String? = null,
    @SerializedName("files") val files: List<LibraryFile> = emptyList()
)

data class LibrarySeason(
    @SerializedName("season") val season: Int,
    @SerializedName("name") val name: String,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("episodes") val episodes: List<LibraryEpisode> = emptyList()
)

data class TitleDetail(
    @SerializedName("kind") val kind: String, // "movie" or "show"
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("runtime") val runtime: Int? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("backdrop") val backdrop: String? = null,
    @SerializedName("logo") val logo: String? = null,
    @SerializedName("tagline") val tagline: String? = null,
    @SerializedName("genres") val genres: List<String>? = null,
    @SerializedName("directors") val directors: List<CreditInfo>? = null,
    @SerializedName("writers") val writers: List<CreditInfo>? = null,
    @SerializedName("creators") val creators: List<CreditInfo>? = null,
    @SerializedName("cast") val cast: List<CreditInfo>? = null,
    @SerializedName("files") val files: List<LibraryFile>? = null,
    @SerializedName("seasons") val seasons: List<LibrarySeason>? = null,
    @SerializedName("other_files") val otherFiles: List<LibraryFile>? = null,
    // Shows: the user has the players skip intros and credits by themselves
    @SerializedName("auto_skip") val autoSkip: Boolean = false,
    // It has theme music for its page (GET /api/library/theme)
    @SerializedName("theme") val theme: Boolean = false
)

/** A part of a video, in seconds, and where it was found ("chapters", "fingerprint", "black", "manual"). */
data class SegmentSpan(
    @SerializedName("start") val start: Double,
    @SerializedName("end") val end: Double,
    @SerializedName("source") val source: String? = null
)

/** A video's intro and credits, for the players' Skip buttons, and the episode after it. */
data class MediaSegments(
    @SerializedName("intro") val intro: SegmentSpan? = null,
    @SerializedName("credits") val credits: SegmentSpan? = null,
    @SerializedName("auto_skip") val autoSkip: Boolean = false,
    @SerializedName("show_id") val showId: Int = 0,
    @SerializedName("next") val next: ContinueItem? = null,
    @SerializedName("metadata") val metadata: ContinueMetadata? = null
)

data class AutoSkipRequest(
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("on") val on: Boolean
)

data class LoginRequest(
    @SerializedName("username") val username: String,
    @SerializedName("password") val password: String
)

data class LoginResponse(
    @SerializedName("token") val token: String,
    @SerializedName("expires_at") val expiresAt: Long,
    @SerializedName("user") val user: User
)

/** A code a TV shows to be signed in from another device (POST /api/auth/pairing). */
data class PairingCode(
    @SerializedName("code") val code: String,
    @SerializedName("expires_at") val expiresAt: String? = null // RFC 3339
)

/** GET /api/auth/pairing/{code}: "waiting", "expired", or "approved" with the account's token and user. */
data class PairingStatus(
    @SerializedName("status") val status: String,
    @SerializedName("token") val token: String? = null,
    @SerializedName("user") val user: User? = null
)

/** POST /api/pairing/claim: signs the TV showing [code] in as the account making the request. */
data class PairingClaim(
    @SerializedName("code") val code: String
)

data class ErrorResponse(
    @SerializedName("error") val error: String,
    @SerializedName("message") val message: String? = null
)

/** A video's subtitle tracks from GET /api/media/codecs: its own, then files beside it and downloads. */
data class CodecsInfo(
    @SerializedName("duration") val duration: Double = 0.0,
    @SerializedName("video") val video: VideoCodec? = null,
    @SerializedName("audio") val audio: List<AudioTrackInfo>? = null,
    @SerializedName("subtitles") val subtitles: List<SubtitleTrack>? = null,
    // Fonts embedded in the video, which its ASS/SSA tracks name (GET /api/media/font)
    @SerializedName("fonts") val fonts: List<FontAttachment>? = null,
    // The tracks to start with for the account's languages (the server's indexes; -1: the file's default)
    @SerializedName("preferred_audio") val preferredAudio: Int = -1,
    @SerializedName("preferred_subtitle") val preferredSubtitle: Int = -1,
)

/** GET/PUT /api/account: the account's languages for audio and subtitles ("" for the file's default), and the codes to choose from. */
data class AccountSettings(
    @SerializedName("audio_language") val audioLanguage: String = "",
    @SerializedName("subtitle_language") val subtitleLanguage: String = "",
    @SerializedName("subtitles_on") val subtitlesOn: Boolean = false, // subtitles start on, in subtitleLanguage
    @SerializedName("languages") val languages: List<String>? = null,
)

/** POST /api/media/subtitle-font: the text whose CJK characters need a font. */
data class SubtitleFontRequest(@SerializedName("text") val text: String)

data class FontAttachment(
    @SerializedName("index") val index: Int = 0, // among the file's attachments: the font= param
    @SerializedName("name") val name: String = "",
)

data class AudioTrackInfo(
    @SerializedName("index") val index: Int = 0, // among the file's audio tracks: a stream's audio= param
    @SerializedName("codec") val codec: String = "",
    @SerializedName("language") val language: String? = null, // ISO 639-2: eng, spa…
    @SerializedName("title") val title: String? = null,
    @SerializedName("channels") val channels: Int = 0,
    @SerializedName("default") val isDefault: Boolean = false,
)

data class VideoCodec(
    @SerializedName("codec") val codec: String = "",
    @SerializedName("profile") val profile: String? = null,
    @SerializedName("pix_fmt") val pixFmt: String? = null,
)

/** Where a stream that copies the video really starts, in seconds (GET /api/media/seekpoint). */
data class SeekPoint(
    @SerializedName("start") val start: Double = 0.0,
    @SerializedName("ask") val ask: String? = null, // with next=1: the start to request for it
)

data class SubtitleTrack(
    @SerializedName("index") val index: Int,
    @SerializedName("codec") val codec: String = "",
    @SerializedName("language") val language: String? = null, // ISO 639-2: eng, spa…
    @SerializedName("title") val title: String? = null,
    @SerializedName("default") val isDefault: Boolean = false,
    @SerializedName("forced") val forced: Boolean = false,
    // Text tracks come as WebVTT from /api/media/subtitles; picture ones are burned into a transcode.
    @SerializedName("text") val text: Boolean = false,
    @SerializedName("external") val external: String? = null, // "file" (beside the video) or "opensubtitles"
    @SerializedName("hearing_impaired") val hearingImpaired: Boolean = false,
    // Names the track whatever its place in the list; its timing correction, shared by every
    // player (positive: lines shown later)
    @SerializedName("key") val key: String? = null,
    @SerializedName("offset_ms") val offsetMs: Int = 0,
)

data class SubtitleOffsetRequest(
    @SerializedName("path") val path: String,
    @SerializedName("key") val key: String,
    @SerializedName("offset_ms") val offsetMs: Int
)

/** GET /api/media/subtitles/search: OpenSubtitles results for a video, best first. */
data class SubtitleSearch(
    @SerializedName("query") val query: SubtitleSearchQuery? = null,
    @SerializedName("results") val results: List<SubtitleHit> = emptyList(),
)

data class SubtitleSearchQuery(
    @SerializedName("title") val title: String? = null,
    @SerializedName("year") val year: Int = 0,
    @SerializedName("season") val season: Int = 0,
    @SerializedName("episode") val episode: Int = 0,
    @SerializedName("tmdb") val tmdb: Boolean = false,
    @SerializedName("languages") val languages: List<String>? = null,
)

/** A search result; sent back as is to download it. */
data class SubtitleHit(
    @SerializedName("file_id") val fileId: Int,
    @SerializedName("language") val language: String = "",
    @SerializedName("release") val release: String = "",
    @SerializedName("downloads") val downloads: Int = 0,
    @SerializedName("rating") val rating: Double = 0.0,
    @SerializedName("hearing_impaired") val hearingImpaired: Boolean = false,
    @SerializedName("foreign_parts_only") val foreignPartsOnly: Boolean = false,
    @SerializedName("hash_match") val hashMatch: Boolean = false,
    @SerializedName("trusted") val trusted: Boolean = false,
    @SerializedName("machine_translated") val machineTranslated: Boolean = false,
    @SerializedName("fps") val fps: Double = 0.0,
    @SerializedName("uploaded") val uploaded: String? = null,
    @SerializedName("language_code") val languageCode: String? = null,
    @SerializedName("downloaded") val downloaded: Boolean = false, // kept already: no download used
)

data class SubtitleDownloadRequest(
    @SerializedName("path") val path: String, // base64, as in the stream URL
    @SerializedName("result") val result: SubtitleHit,
)

/** POST /api/media/subtitles/download: the video's tracks now, and the new one's index. */
data class SubtitleDownload(
    @SerializedName("subtitles") val subtitles: List<SubtitleTrack> = emptyList(),
    @SerializedName("index") val index: Int = -1,
    @SerializedName("quota") val quota: SubtitleQuota? = null,
)

data class SubtitleQuota(
    @SerializedName("remaining") val remaining: Int = 0,
    @SerializedName("reset") val reset: String? = null,
)

// The server's graphics hardware for transcodes (admins' Settings): GET /api/admin/hardware
data class HardwareGpu(
    @SerializedName("name") val name: String,
    @SerializedName("vendor") val vendor: String,
    @SerializedName("device") val device: String? = null
)

data class HardwareEncoder(
    @SerializedName("id") val id: String, // streaming.hardwareEncoder: nvenc, qsv, vaapi
    @SerializedName("device") val device: String? = null, // streaming.hardwareDevice
    @SerializedName("name") val name: String,
    @SerializedName("gpu") val gpu: String,
    @SerializedName("vendor") val vendor: String,
    @SerializedName("sessions") val sessions: Int = 0 // encodes the driver allows at once, 0 no limit
)

data class HardwareProblem(
    @SerializedName("gpu") val gpu: String,
    @SerializedName("encoder") val encoder: String,
    @SerializedName("reason") val reason: String
)

/** One backup in the server's backup folder (GET /api/admin/backups). */
data class BackupInfo(
    @SerializedName("name") val name: String = "",
    @SerializedName("size") val size: Long = 0,
    @SerializedName("created") val created: String = "", // RFC 3339
)

/** The server's backups, newest first, its folder (a disk path: admins only) and when the next is. */
data class BackupList(
    @SerializedName("folder") val folder: String = "",
    @SerializedName("backups") val backups: List<BackupInfo>? = null,
    @SerializedName("next") val next: String? = null, // RFC 3339; none when backups are off
)

/** A damaged video (its seek index broken) and its fixed copy, for Settings (GET /api/admin/damaged). */
data class DamagedVideo(
    @SerializedName("path") val path: String = "", // client path
    @SerializedName("name") val name: String = "",
    @SerializedName("library") val library: String = "",
    @SerializedName("folder") val folder: String = "",
    @SerializedName("reason") val reason: String = "",
    @SerializedName("state") val state: String = "", // "damaged", "fixing", "fixed" or "failed"
    @SerializedName("error") val error: String? = null,
    @SerializedName("size") val size: Long = 0,
    @SerializedName("fixed_size") val fixedSize: Long = 0,
    @SerializedName("claimed") val claimed: Double = 0.0, // the length the original says
    @SerializedName("duration") val duration: Double = 0.0, // the fixed copy's
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
) {
    val fixed get() = state == "fixed"
    /** Fix makes its copy: not fixed yet, or it failed. */
    val fixable get() = state == "damaged" || state == "failed"
    val fixing get() = state == "fixing"
}

/** A replaced original in the trash (GET /api/admin/trash), which Restore puts back. */
data class TrashItem(
    @SerializedName("id") val id: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("library") val library: String = "",
    @SerializedName("folder") val folder: String = "",
    @SerializedName("size") val size: Long = 0,
    @SerializedName("trashed_at") val trashedAt: String = "", // RFC 3339
    @SerializedName("deletes_at") val deletesAt: String = "",
    @SerializedName("reason") val reason: String = "",
    @SerializedName("replacement") val replacement: String = "", // the name of the file that replaced it
    @SerializedName("restorable") val restorable: Boolean = false,
    @SerializedName("thumbnail_url") val thumbnailUrl: String? = null,
)

/** The trash of replaced originals: deleted [days] after they went in (GET /api/admin/trash, and Empty's answer). */
data class TrashReport(
    @SerializedName("items") val items: List<TrashItem>? = null,
    @SerializedName("files") val files: Int = 0,
    @SerializedName("bytes") val bytes: Long = 0,
    @SerializedName("days") val days: Int = 30,
)

data class DamagedList(
    @SerializedName("videos") val videos: List<DamagedVideo>? = null,
)

/** Replace's and Restore's answer: the client path of the file now in the library. */
data class DamagedReplaced(
    @SerializedName("path") val path: String? = null,
)

data class HardwareReport(
    @SerializedName("checking") val checking: Boolean,
    @SerializedName("gpus") val gpus: List<HardwareGpu>? = null,
    @SerializedName("encoders") val encoders: List<HardwareEncoder>? = null,
    @SerializedName("problems") val problems: List<HardwareProblem>? = null
)

// ---- Fixing TMDB matches (admins; the web's Fix dialog) ----

/** What a file or folder is matched to (GET /api/media/match), and what the dialog may search. */
data class MatchInfo(
    @SerializedName("kind") val kind: String = "movie", // the library's: "movie" or "tv"
    @SerializedName("match_kind") val matchKind: String? = null, // what it's matched to (a movie library's can be a show)
    @SerializedName("kinds") val kinds: List<String>? = null, // what may be searched
    @SerializedName("query") val query: String? = null,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("status") val status: String = "", // "matched", "not_found", "error" or "pending"
    @SerializedName("manual") val manual: Boolean = false,
    @SerializedName("current") val current: MatchTitle? = null,
    @SerializedName("episode") val episode: MatchEpisodeInfo? = null, // TV episode files
)

/** The title matched (the server's item metadata). poster is a server artwork URL. */
data class MatchTitle(
    @SerializedName("kind") val kind: String = "", // "movie", "show", "season" or "episode"
    @SerializedName("tmdb_id") val tmdbId: Int = 0,
    @SerializedName("title") val title: String = "",
    @SerializedName("year") val year: Int? = null,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("poster") val poster: String? = null,
)

/** Which TMDB episode an episode file shows as, and why ("file", "season" or "manual"). */
data class MatchEpisodeInfo(
    @SerializedName("file_season") val fileSeason: Int = 0,
    @SerializedName("file_episode") val fileEpisode: Int = 0,
    @SerializedName("season") val season: Int = 0,
    @SerializedName("episode") val episode: Int = 0,
    @SerializedName("source") val source: String = "file",
    @SerializedName("title") val title: String? = null,
)

/** A TMDB search result. poster is a TMDB image URL, posterPath its TMDB path (to use as a poster). */
data class MatchHit(
    @SerializedName("tmdb_id") val tmdbId: Int = 0,
    @SerializedName("title") val title: String = "",
    @SerializedName("year") val year: Int? = null,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("poster_path") val posterPath: String? = null,
)

/** The matched title's TMDB posters (GET /api/media/match/posters). */
data class PosterList(
    @SerializedName("current") val current: String = "", // TMDB path of the one in use
    @SerializedName("manual") val manual: Boolean = false,
    @SerializedName("posters") val posters: List<PosterOption> = emptyList(),
)

data class PosterOption(
    @SerializedName("path") val path: String = "",
    @SerializedName("url") val url: String = "",
    @SerializedName("language") val language: String = "", // "" for posters without text
)

data class TmdbSeason(
    @SerializedName("number") val number: Int = 0,
    @SerializedName("name") val name: String? = null,
    @SerializedName("episode_count") val episodeCount: Int = 0,
)

data class TmdbEpisode(
    @SerializedName("number") val number: Int = 0,
    @SerializedName("name") val name: String? = null,
    @SerializedName("air_date") val airDate: String? = null,
)

/**
 * A person's page (GET /api/library/person), opened from a title page's cast: a larger photo, their
 * details, their titles in the library and their TMDB filmography, newest first.
 */
data class PersonDetail(
    @SerializedName("id") val id: Int,
    @SerializedName("name") val name: String,
    @SerializedName("photo") val photo: String? = null,
    @SerializedName("biography") val biography: String? = null,
    @SerializedName("birthday") val birthday: String? = null,
    @SerializedName("deathday") val deathday: String? = null,
    @SerializedName("place_of_birth") val placeOfBirth: String? = null,
    @SerializedName("known_for") val knownFor: String? = null,
    @SerializedName("in_library") val inLibrary: List<LibraryTitle> = emptyList(),
    @SerializedName("filmography") val filmography: List<PersonFilm> = emptyList()
)

/** A title in a person's filmography: [poster] a TMDB image URL; [inLibrary] those the library has. */
data class PersonFilm(
    @SerializedName("kind") val kind: String,
    @SerializedName("tmdb_id") val tmdbId: Int,
    @SerializedName("title") val title: String,
    @SerializedName("year") val year: Int? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("role") val role: String? = null,
    @SerializedName("in_library") val inLibrary: Boolean = false
)

// ---- The Guide: live channels the server plans (docs/GUIDE.md) ----

/** GET /api/guide/available */
data class GuideAvailable(@SerializedName("available") val available: Boolean = false)

/** GET /api/guide: the grid, from the current half hour (Unix ms) */
data class GuideListings(
    @SerializedName("now") val now: Long = 0,
    @SerializedName("from") val from: Long = 0,
    @SerializedName("to") val to: Long = 0,
    // as far as the plan reaches: the grid loads more (from = to) as it scrolls, up to this
    @SerializedName("until") val until: Long = 0,
    @SerializedName("channels") val channels: List<GuideChannel> = emptyList(),
)

data class GuideChannel(
    @SerializedName("number") val number: Int = 0,
    @SerializedName("name") val name: String = "",
    @SerializedName("genre") val genre: String = "",
    @SerializedName("color") val color: String = "",
    // its logo: GET this with w= (width), color= (rrggbb) or format=svg
    @SerializedName("logo") val logo: String = "",
    @SerializedName("listings") val listings: List<GuideListing>? = null,
    // a channel of its own's: guide or weather (the app draws it); "" for a library's channels
    @SerializedName("kind") val kind: String = "",
)

/** A program in the grid, from its start to the next one's (Unix ms); no info: off the air. */
data class GuideListing(
    @SerializedName("start") val start: Long = 0,
    @SerializedName("end") val end: Long = 0,
    @SerializedName("info") val info: ProgramInfo? = null,
)

data class ProgramInfo(
    @SerializedName("kind") val kind: String = "",
    @SerializedName("tmdb_id") val tmdbId: Int = 0,
    @SerializedName("library") val library: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("year") val year: Int? = null,
    @SerializedName("rating") val rating: String? = null,
    @SerializedName("genres") val genres: List<String>? = null,
    @SerializedName("overview") val overview: String? = null,
    @SerializedName("poster") val poster: String? = null,
    @SerializedName("season") val season: Int? = null,
    @SerializedName("episode") val episode: Int? = null,
    @SerializedName("episode_title") val episodeTitle: String? = null,
    @SerializedName("episode_overview") val episodeOverview: String? = null,
) {
    /** "S2E5 · Name" for an episode, else null. */
    val episodeLabel: String?
        get() = season?.takeIf { it > 0 }?.let { s ->
            "S${s}E${episode ?: 0}" + (episodeTitle?.takeIf { it.isNotBlank() }?.let { " · $it" } ?: "")
        }
}

/** GET /api/guide/now: what a channel airs, and how far into it (seconds). */
data class GuideNow(
    @SerializedName("channel") val channel: GuideChannel = GuideChannel(),
    @SerializedName("kind") val kind: String = "",
    @SerializedName("path") val path: String? = null,
    @SerializedName("offset") val offset: Double = 0.0,
    @SerializedName("start") val start: Long = 0,
    @SerializedName("end") val end: Long = 0,
    @SerializedName("program") val program: ProgramInfo? = null,
    @SerializedName("next") val next: ProgramInfo? = null,
    @SerializedName("next_start") val nextStart: Long = 0,
    @SerializedName("server_time") val serverTime: Long = 0,
    // the Guide channel's preview and promo blocks: when and where the program airs
    @SerializedName("airs") val airs: Long = 0,
    @SerializedName("airs_on") val airsOn: GuideChannel? = null,
    // what plays under the Weather channel and the Guide channel's cards
    @SerializedName("music") val music: GuideMusic? = null,
    // a program's voice-over on its credits ("Stay tuned for…"), when it's been made
    @SerializedName("voice_over") val voiceOver: GuideVoiceOver? = null,
)

/** When a program's voice-over plays: from [at] seconds into its file, the program lowered under it; from [squeezeAt] (0: never) the picture is squeezed beside an "Up next" card. */
data class GuideVoiceOver(
    @SerializedName("url") val url: String = "",
    @SerializedName("at") val at: Double = 0.0,
    @SerializedName("seconds") val seconds: Double = 0.0,
    @SerializedName("squeeze_at") val squeezeAt: Double = 0.0,
    @SerializedName("line") val line: String = "",
    @SerializedName("announcer") val announcer: String = "",
)

/** A track of the Guide's music, from [offset] seconds; the next starts at [end]. */
data class GuideMusic(
    @SerializedName("url") val url: String = "",
    @SerializedName("title") val title: String = "",
    @SerializedName("credit") val credit: String = "",
    @SerializedName("offset") val offset: Double = 0.0,
    @SerializedName("start") val start: Long = 0,
    @SerializedName("end") val end: Long = 0,
)

/** GET /api/guide/weather: the Weather channel's (the National Weather Service's, US units). */
data class GuideWeather(
    @SerializedName("place") val place: String = "",
    @SerializedName("zip") val zip: String = "",
    @SerializedName("updated") val updated: Long = 0,
    @SerializedName("current") val current: WeatherNow? = null,
    @SerializedName("periods") val periods: List<WeatherPeriod>? = null,
    @SerializedName("alerts") val alerts: List<WeatherAlert>? = null,
    @SerializedName("radar") val radar: Boolean = false,
    @SerializedName("error") val error: String? = null,
)

data class WeatherNow(
    @SerializedName("conditions") val conditions: String = "",
    @SerializedName("icon") val icon: String = "cloud",
    @SerializedName("temp") val temp: Int? = null,
    @SerializedName("feels_like") val feelsLike: Int? = null,
    @SerializedName("dewpoint") val dewpoint: Int? = null,
    @SerializedName("humidity") val humidity: Int? = null,
    @SerializedName("wind_dir") val windDir: String? = null,
    @SerializedName("wind") val wind: Int? = null,
    @SerializedName("gust") val gust: Int? = null,
    @SerializedName("pressure") val pressure: Double? = null,
    @SerializedName("visibility") val visibility: Double? = null,
)

data class WeatherPeriod(
    @SerializedName("name") val name: String = "",
    @SerializedName("daytime") val daytime: Boolean = true,
    @SerializedName("temp") val temp: Int = 0,
    @SerializedName("forecast") val forecast: String = "",
    @SerializedName("icon") val icon: String = "cloud",
    @SerializedName("precip") val precip: Int? = null,
)

data class WeatherAlert(
    @SerializedName("event") val event: String = "",
    @SerializedName("headline") val headline: String = "",
)
