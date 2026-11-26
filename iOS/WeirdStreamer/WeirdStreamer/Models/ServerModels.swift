import Foundation

// The server's JSON, as the Android app's ApiModels.kt reads it. Decoded with snake_case keys
// turned into camelCase (ServerClient). Anything the server might leave out is optional.

struct ServerUser: Codable, Hashable {
    let id: Int64?
    let username: String
    let isAdmin: Bool?
}

struct LoginResponse: Decodable {
    let token: String
    let expiresAt: Int64
    let user: ServerUser
}

struct Playback: Decodable, Hashable {
    let position: Double?
    let duration: Double?
    let watched: Bool?
    let updatedAt: Int64?

    /// Resume only past the first 10 seconds, as the Android app does.
    var resumeAt: Double {
        guard let position, position >= 10 else { return 0 }
        return position
    }

    /// Fraction watched, 0 to 1, from the position and the known or fallback duration.
    func fraction(fallbackDuration: Double?) -> Double {
        let total = (duration ?? 0) > 0 ? duration! : (fallbackDuration ?? 0)
        guard total > 0, let position else { return 0 }
        return min(max(position / total, 0), 1)
    }
}

/// A file in a folder listing (`/api/media/browse`).
struct MediaItem: Decodable, Hashable {
    let id: Int64?
    let path: String
    let name: String
    let type: String?          // video, image, document
    let size: Int64?
    let mimeType: String?
    let thumbnailUrl: String?
    let duration: Double?
    let playback: Playback?
    let liveVideo: String?
    var format: String? = nil   // a video's: "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"

    var stableID: String { path }
    var isVideo: Bool { type == "video" }
}

/// A folder listing: its subfolders (`dirs`, with a library's `libraryType` at the top) and files.
final class Directory: Decodable {
    let id: String
    let name: String
    let items: [MediaItem]?
    let dirs: [Directory]?
    let libraryType: String?
    /// A picture for inside the folder's icon (folders without a poster): its thumbnail.png, else a
    /// photo's or video's from inside it.
    let thumbnailUrl: String?
    /// The folder stands for one title (a movie's or a show's folder), so an admin can fix its match.
    let fixable: Bool?

    enum CodingKeys: String, CodingKey { case id = "path", name, items, dirs, libraryType, thumbnailUrl, fixable }
}

struct ContinueMetadata: Decodable, Hashable {
    let kind: String?
    let title: String?
    let year: Int?
    let showTitle: String?
    let season: Int?
    let episode: Int?
    let still: String?
    let backdrop: String?
    let poster: String?

    /// The player's title: "Example Show · S01E02 · Example Episode", or "Example Movie (2008)".
    var playerTitle: String? {
        if kind == "episode", let season, let episode {
            return [showTitle, String(format: "S%02dE%02d", season, episode), title].compactMap { $0 }.joined(separator: " · ")
        }
        if kind == "movie", let title { return title + (year.map { " (\($0))" } ?? "") }
        return nil
    }
}

/// An item in Continue Watching, or a newly added file, with its library.
struct ContinueItem: Decodable, Hashable {
    let id: Int64?
    let path: String
    let name: String
    let size: Int64?
    let thumbnailUrl: String?
    let duration: Double?
    let libraryId: String
    let metadata: ContinueMetadata?
    let playback: Playback?
    let upNext: Bool?
    let addedAt: Int64?

    var stableID: String { path }

    /// "Example Show" and "S01E02 · Example Episode", or "Example Movie (2008)" and the file name.
    var titleLines: (String, String?) {
        if let md = metadata, md.kind == "episode", let season = md.season, let episode = md.episode {
            return (md.showTitle ?? md.title ?? name, String(format: "S%02dE%02d · %@", season, episode, md.title ?? ""))
        }
        if let title = metadata?.title { return (title + (metadata?.year.map { " (\($0))" } ?? ""), name) }
        return (name, nil)
    }
}

struct LibraryAdditions: Decodable {
    let libraryId: String
    let libraryName: String
    let items: [ContinueItem]?
}

struct LibraryTitle: Decodable, Identifiable, Hashable {
    let kind: String           // movie or show
    let tmdbId: Int
    let title: String
    let year: Int?
    let poster: String?
    let fileCount: Int?
    let watched: Int?
    let progress: Double?
    var collectionId: Int? = nil   // movies: the TMDB collection it belongs to
    var missing: Bool? = nil       // on a collection's page: a film the library doesn't have
    var role: String? = nil        // on a person's page: their part in it

    var id: String { "\(kind)-\(tmdbId)" }

    /// As the Android app: a movie is watched once, a show once every file is.
    var allWatched: Bool {
        let count = fileCount ?? 0
        return count > 0 && (watched ?? 0) >= (kind == "movie" ? 1 : count)
    }
}

struct FileMatch: Decodable, Hashable {
    let title: String
    let year: Int?

    var label: String { title + (year.map { " (\($0))" } ?? "") }
}

struct CreditInfo: Decodable, Hashable {
    let tmdbId: Int?
    let name: String
    let character: String?
    let profile: String?
}

struct LibraryFile: Decodable, Hashable {
    let id: Int64?
    let path: String
    let name: String
    let size: Int64?
    let duration: Double?
    let thumbnailUrl: String?
    var format: String? = nil   // "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"
    let count: Int?           // a folder of several files (unmatched): not playable as one
    let match: FileMatch?
    let playback: Playback?

    var stableID: String { path }
}

struct LibraryListing: Decodable {
    let libraryId: String
    let libraryName: String
    let libraryType: String
    let titles: [LibraryTitle]?
    let unmatched: [LibraryFile]?
    var collections: [LibraryCollection]? = nil   // movie libraries: their movies' TMDB collections, by name
    var userCollections: [UserCollectionSummary]? = nil   // movie libraries: the collections admins made, by name
}

/// A library title as collections name it: "movie" (or "show", a TV movie) and its TMDB ID.
struct CollectionTitle: Codable, Hashable {
    let kind: String
    let tmdbId: Int
}

/// A collection admins made: its titles here and the posters of its first four by release date (its collage).
struct UserCollectionSummary: Decodable, Identifiable, Hashable {
    let id: Int64
    let name: String
    let description: String?
    let count: Int?
    let posters: [String]?
    let titles: [CollectionTitle]?
}

/// A made collection's page: its titles by release date, with progress.
struct UserCollectionDetail: Decodable {
    let id: Int64
    let name: String
    let description: String?
    let posters: [String]?
    let parts: [LibraryTitle]
}

/// A collection (a film series) in a movie library: its movies here (`count`) of its films on TMDB (`total`).
struct LibraryCollection: Decodable, Identifiable, Hashable {
    let id: Int
    let name: String
    let poster: String?
    let count: Int?
    let total: Int?
}

/// A collection's page (GET /api/library/collection): every film TMDB lists in it, by release date,
/// the library's as its titles and the rest `missing`.
struct CollectionDetail: Decodable {
    let id: Int
    let name: String
    let overview: String?
    let poster: String?
    let backdrop: String?
    let parts: [LibraryTitle]
}

struct LibraryEpisode: Decodable, Identifiable, Hashable {
    let season: Int
    let episode: Int
    let title: String
    let overview: String?
    let airDate: String?
    let runtime: Int?
    let still: String?
    let files: [LibraryFile]?

    var id: String { "\(season)-\(episode)" }
}

struct LibrarySeason: Decodable, Identifiable, Hashable {
    let season: Int
    let name: String
    let poster: String?
    let episodes: [LibraryEpisode]?

    var id: Int { season }
}

struct TitleDetail: Decodable {
    let kind: String
    let tmdbId: Int
    let title: String
    let year: Int?
    let overview: String?
    let runtime: Int?
    let poster: String?
    let backdrop: String?
    let tagline: String?
    let genres: [String]?
    let directors: [CreditInfo]?
    let writers: [CreditInfo]?
    let creators: [CreditInfo]?
    let cast: [CreditInfo]?
    let files: [LibraryFile]?
    let seasons: [LibrarySeason]?
    let otherFiles: [LibraryFile]?
    let autoSkip: Bool?
    let theme: Bool?          // it has theme music (GET /api/library/theme)
}

/// A progress report for the player (`POST /api/streams/progress`).
struct StreamProgress: Encodable {
    let path: String           // base64 client path
    let position: Double
    let paused: Bool
    let ended: Bool
    var title: Int = 0         // discs: the title playing (0: the main title)
}

/// Offline progress pushed in one go (`POST /api/playback/sync`).
struct SyncedPlayback: Encodable {
    let path: String           // base64 client path
    let position: Double
    let ended: Bool
    let updatedAt: Int64       // Unix milliseconds

    enum CodingKeys: String, CodingKey { case path, position, ended, updatedAt = "updated_at" }
}

struct PlaybackSync: Encodable {
    let items: [SyncedPlayback]
}

/// Watched or unwatched for files (`POST /api/playback/watched`).
struct WatchedRequest: Encodable {
    let paths: [String]        // base64 client paths
    let watched: Bool
    let library: String?
    let kind: String?
    let tmdbId: Int?

    enum CodingKeys: String, CodingKey { case paths, watched, library, kind, tmdbId = "tmdb_id" }
}

/// What the player plays: a file's client path, its name, and where to start.
struct PlayRequest: Identifiable, Hashable {
    let id = UUID()
    let path: String
    let title: String
    let startAt: Double
    let libraryId: String
    /// A download playing from this phone: its file, played as it is.
    var localFile: URL? = nil
}

// MARK: Player

/// A file's codecs (`/api/media/codecs`): what the player checks before choosing a stream.
struct CodecsInfo: Decodable {
    let duration: Double?
    let video: VideoCodec?
    let audio: [AudioTrackInfo]?
    let subtitles: [SubtitleTrack]?
    let fonts: [FontAttachment]?    // the video's embedded fonts, which its ASS tracks name
    // The tracks to start with for the account's languages (the server's indexes; -1: the file's default)
    let preferredAudio: Int?
    let preferredSubtitle: Int?
}

/// GET/PUT /api/account: the account's languages for audio and subtitles ("" for the file's default), and the codes to choose from.
struct AccountSettings: Codable {
    var audioLanguage: String
    var subtitleLanguage: String
    var subtitlesOn: Bool           // subtitles start on, in subtitleLanguage
    var languages: [String]?

    // Sent with the server's names whatever the encoder does (the reply is read with
    // convertFromSnakeCase, so decoding keeps the synthesized keys).
    private enum RequestKeys: String, CodingKey { case audioLanguage = "audio_language", subtitleLanguage = "subtitle_language", subtitlesOn = "subtitles_on" }

    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: RequestKeys.self)
        try c.encode(audioLanguage, forKey: .audioLanguage)
        try c.encode(subtitleLanguage, forKey: .subtitleLanguage)
        try c.encode(subtitlesOn, forKey: .subtitlesOn)
    }
}

/// A font embedded in the video (an MKV attachment): GET /api/media/font?path=&font=index.
struct FontAttachment: Decodable, Hashable {
    let index: Int
    let name: String?
}

/// A subtitle track: the video's own, a file beside it, or a download from OpenSubtitles.
struct SubtitleTrack: Codable, Hashable {
    let index: Int
    var codec: String?
    var language: String?           // ISO 639-2: eng, spa…
    var title: String?
    var `default`: Bool?
    var forced: Bool?
    var text: Bool?                 // text tracks come as WebVTT; picture ones as .sup (PGS) or burned in
    var external: String?           // "file" (beside the video) or "opensubtitles"
    var hearingImpaired: Bool?
    var key: String?                // names the track for its timing correction
    var offsetMs: Int?

    var isText: Bool { text == true }
    /// Picture subtitles drawn over the video from the server's .sup data: PGS, and DVD's, which the
    /// server sends as PGS.
    var drawnAsPGS: Bool { codec == "hdmv_pgs_subtitle" || codec == "dvd_subtitle" }
    /// ASS/SSA (tracks and .ass/.ssa files): drawn styled by libass.
    var isStyled: Bool { codec == "ass" || codec == "ssa" }
}

/// An OpenSubtitles result; sent back as it came to download it.
struct SubtitleHit: Codable, Hashable {
    let fileId: Int
    var language: String?
    var release: String?
    var downloads: Int?
    var rating: Double?
    var hearingImpaired: Bool?
    var foreignPartsOnly: Bool?
    var hashMatch: Bool?
    var trusted: Bool?
    var machineTranslated: Bool?
    var fps: Double?
    var uploaded: String?
    var languageCode: String?
    var downloaded: Bool?

}

struct SubtitleSearchQuery: Decodable {
    let title: String?
    let year: Int?
    let season: Int?
    let episode: Int?
}

struct SubtitleSearch: Decodable {
    let query: SubtitleSearchQuery?
    let results: [SubtitleHit]?
}

struct SubtitleQuota: Decodable {
    let remaining: Int?
    let reset: String?
}

struct SubtitleDownload: Decodable {
    let subtitles: [SubtitleTrack]?
    let index: Int?
    let quota: SubtitleQuota?
}

struct VideoCodec: Decodable {
    let codec: String?
    let profile: String?
    let pixFmt: String?
}

struct AudioTrackInfo: Decodable {
    let index: Int?          // among the file's audio tracks: a stream's audio= param
    let codec: String?
    let channels: Int?
    let language: String?    // ISO 639-2: eng, spa…
    let title: String?
    let `default`: Bool?
}

/// Names a file's audio tracks as the server lists them, as the web and Android do: "English ·
/// Commentary · AAC · stereo", with the file's default marked.
func audioLabels(_ tracks: [AudioTrackInfo]) -> [String] {
    tracks.enumerated().map { i, t in
        let language = languageName(t.language)
        var parts = [language ?? "Track \((t.index ?? i) + 1)"]
        if let title = t.title?.trimmingCharacters(in: .whitespaces), !title.isEmpty,
           title.lowercased() != language?.lowercased() {
            parts.append(String(title.prefix(40)))
        }
        if let codec = t.codec?.lowercased() {
            let names = ["eac3": "E-AC-3", "ac3": "AC-3", "truehd": "TrueHD", "opus": "Opus", "vorbis": "Vorbis"]
            parts.append(names[codec] ?? codec.uppercased())
        }
        switch t.channels ?? 0 {
        case 0: break
        case 1: parts.append("mono")
        case 2: parts.append("stereo")
        case 6: parts.append("5.1")
        case 8: parts.append("7.1")
        case let n: parts.append("\(n) ch")
        }
        return parts.joined(separator: " · ") + (t.default == true ? " (default)" : "")
    }
}

/// Where a stream that copies the video really starts (`/api/media/seekpoint`).
struct SeekPoint: Decodable {
    let start: Double?
    let ask: String?
}

struct TrickplayFrame: Decodable, Hashable {
    let time: Int
    let url: String
}

struct TrickplayInfo: Decodable {
    let frames: [TrickplayFrame]?
    /// A disc image's frames: the title they were made from (its main feature); 0 for files.
    var title: Int?
}

/// A DVD disc image's titles, for the player's disc controls (`/api/media/disc`).
struct DiscInfo: Decodable {
    let titles: [DiscTitle]?
    let mainTitle: Int?

    /// A title by number; 0 (or one not on the disc) is the main title.
    func title(_ number: Int) -> DiscTitle? {
        let list = titles ?? []
        let wanted = number <= 0 ? (mainTitle ?? 1) : number
        return list.first { $0.number == wanted } ?? list.first
    }
}

struct DiscTitle: Decodable {
    let number: Int
    let duration: Double            // seconds
    var angles: Int?
    var chapters: [Double]?         // start times, seconds
    var audio: [DiscTrack]?
    var subtitles: [DiscTrack]?
}

struct DiscTrack: Decodable {
    let index: Int
    var language: String?
    var codec: String?
    var channels: Int?

    /// "eng · ac3 · 5.1", as Android labels it.
    var label: String {
        let lang = (language ?? "").isEmpty ? "Unknown" : language!
        let ch: String? = switch channels ?? 0 { case 0: nil; case 2: "Stereo"; case 6: "5.1"; default: "\(channels!) ch" }
        return [lang, (codec ?? "").isEmpty ? nil : codec, ch].compactMap { $0 }.joined(separator: " · ")
    }
}

struct SegmentSpan: Decodable {
    let start: Double
    let end: Double
}

/// A video's intro and credits, and the episode after it (`/api/media/segments`).
struct MediaSegments: Decodable {
    let intro: SegmentSpan?
    let credits: SegmentSpan?
    let autoSkip: Bool?
    let next: ContinueItem?
    let metadata: ContinueMetadata?
}

// MARK: Administration

struct AdminUser: Decodable, Hashable {
    let id: Int64?
    let username: String
    let isAdmin: Bool?
    let group: String?
}

struct AdminTranscode: Decodable {
    let reason: String?
    let video: String?
    let audio: String?
    let speed: Double?
    let fps: Double?
    let stalled: Bool?
    let sourceVideo: String?
}

/// A viewing on the Streams page.
struct AdminStream: Decodable, Identifiable {
    let id: String
    let user: String?
    let client: String?
    let clientIp: String?
    let name: String?
    let title: String?
    let duration: Double?
    let state: String?
    let mode: String?
    let servedTo: Double?
    let position: Double?
    let width: Int?
    let height: Int?
    let transcode: AdminTranscode?
}

struct BackgroundJob: Decodable, Identifiable {
    let id: String
    let name: String?
    let about: String?
    let state: String?
    let note: String?
    let detail: String?
    let runners: [String]?
    let done: Int?
    let total: Int?
    let started: String?
    let lastStart: String?
    let lastEnd: String?
    let lastResult: String?
    let next: String?
    let schedule: String?
    let paused: Bool?
    let canRun: Bool?
}

struct ShareLoad: Decodable { let library: String?; let ms: Double? }

struct JobLoad: Decodable {
    let available: Bool?
    let cpu: Double?
    let disk: Double?
    let maxDisk: Double?
    let relaxedDisk: Double?
    let shares: [ShareLoad]?
    let maxShareMs: Double?
    let relaxedShareMs: Double?
    let streaming: Bool?
    let pauseWhileStreaming: Bool?
}

struct StorageUse: Decodable { let name: String; let bytes: Int64 }

/// The trickplay job's settings and their limits. `runners` and `framesPerRunner` are those in effect;
/// `autoRunners` and `autoFrames` say they're automatic, which gives `autoRunnersValue` runners (one per disk
/// the libraries are on) and `autoFramesFor[r - 1]` frames for r runners (what fills half the server's memory).
struct TrickplaySettings: Decodable {
    let runners: Int?
    let framesPerRunner: Int?
    let autoRunners: Bool?
    let autoFrames: Bool?
    let autoRunnersValue: Int?
    let autoFramesFor: [Int]?
    /// The GPU trickplay can decode on (nil or "" for none), and whether it does (`memory` and `autoFramesFor`
    /// are for the decoder in use).
    let gpu: String?
    let useGpu: Bool?
    let maxRunners: Int?
    let maxFrames: Int?
    let maxProduct: Int?
    let memory: TrickplayMemory?
}

extension TrickplaySettings {
    /// The runners and frames the fields give: an empty one is automatic, worked out as the server does.
    /// As Android's trickplayEffective.
    func effective(_ runners: String, _ frames: String) -> (Int, Int) {
        let auto = (autoRunnersValue ?? 0) > 0 ? autoRunnersValue! : (self.runners ?? 1)
        let r = Int(runners).flatMap { $0 > 0 ? $0 : nil } ?? auto
        let list = autoFramesFor ?? []
        let autoFrames = r >= 1 && r <= list.count ? list[r - 1] : (framesPerRunner ?? 10)
        return (r, Int(frames).flatMap { $0 > 0 ? $0 : nil } ?? autoFrames)
    }
}

/// For the estimate beside the trickplay settings, for the decoder trickplay uses (`decoder`: cpu, vaapi or
/// cuda): an ffmpeg process and each frame of its batch for a 4K video, the server's memory and swap, and for
/// cuda the same in the card's own memory (0 when unknown).
struct TrickplayMemory: Decodable {
    let decoder: String?
    let processBytes4K: Int64?
    let frameBytes4K: Int64?
    let totalBytes: Int64?
    let swapBytes: Int64?
    let cardProcessBytes4K: Int64?
    let cardFrameBytes4K: Int64?
    let cardBytes: Int64?
}

/// A row of the trickplay memory table: what it's about, what the settings need with 4K videos, what the server
/// has, and whether that fits (`warn` when it doesn't).
struct TrickplayRow: Equatable { let what, needs, has, note: String; let warn: Bool }

struct TrickplayTable { let summary: String; let rows: [TrickplayRow] }

/// What r runners of f frames take with 4K videos (each frame in a batch holds a decoder of its own), as the server
/// measured it for the decoder it uses (`m.decoder`: the CPU, an Intel or AMD GPU whose buffers are the server's
/// memory, or an NVIDIA card with memory of its own), against the server's memory and swap, and the card's: a
/// summary line and a row each. The server uses the settings as they are. As Android's trickplayMemoryTable.
func trickplayMemoryTable(_ r: Int, _ f: Int, _ m: TrickplayMemory?) -> TrickplayTable {
    func gb(_ bytes: Int64) -> String { String(format: "%.1f GB", Double(bytes) / 1_073_741_824) }
    let decodedOn: String = switch m?.decoder {
    case "vaapi": ", decoded on the GPU"
    case "cuda": ", decoded on the graphics card"
    default: ", decoded on the CPU"
    }
    let on = m == nil ? "" : decodedOn
    let summary = "\(r) × \(f) = \(r * f) frames at once\(on)."
    guard let m, let process = m.processBytes4K, let frame = m.frameBytes4K, frame > 0 else { return TrickplayTable(summary: summary, rows: []) }
    var rows: [TrickplayRow] = []
    let ram = Int64(r) * (process + Int64(f) * frame)
    let swap = m.swapBytes ?? 0
    let what = m.decoder == "vaapi" ? "Memory (with the GPU's buffers)" : "Memory"
    if let total = m.totalBytes, total > 0 {
        let has = gb(total) + (swap > 0 ? " + \(gb(swap)) swap" : "")
        if ram <= total {
            rows.append(TrickplayRow(what: what, needs: gb(ram), has: has, note: "Fits", warn: false))
        } else if ram <= total + swap {
            rows.append(TrickplayRow(what: what, needs: gb(ram), has: has, note: "Uses swap", warn: true))
        } else {
            rows.append(TrickplayRow(what: what, needs: gb(ram), has: has, note: "Too much: frames are made one at a time", warn: true))
        }
    } else {
        rows.append(TrickplayRow(what: what, needs: gb(ram), has: "?", note: "", warn: false))
    }
    if let cardFrame = m.cardFrameBytes4K, cardFrame > 0 {
        let card = Int64(r) * ((m.cardProcessBytes4K ?? 0) + Int64(f) * cardFrame)
        let cardTotal = m.cardBytes ?? 0
        if cardTotal <= 0 {
            rows.append(TrickplayRow(what: "Graphics card", needs: gb(card), has: "?", note: "", warn: false))
        } else if card <= cardTotal {
            rows.append(TrickplayRow(what: "Graphics card", needs: gb(card), has: gb(cardTotal), note: "Fits", warn: false))
        } else {
            rows.append(TrickplayRow(what: "Graphics card", needs: gb(card), has: gb(cardTotal), note: "Too much: batches are made smaller", warn: true))
        }
    }
    return TrickplayTable(summary: summary, rows: rows)
}

struct JobsResponse: Decodable {
    let jobs: [BackgroundJob]?
    let load: JobLoad?
    let storage: [StorageUse]?
    let trickplay: TrickplaySettings?
}

// MARK: Fixing TMDB matches (admins; the web's Fix dialog)

/// What a file or folder is matched to (GET /api/media/match), and what may be searched.
struct MatchInfo: Decodable {
    var kind: String?            // the library's: "movie" or "tv"
    var matchKind: String?       // what it's matched to (a movie library's can be a show)
    var kinds: [String]?         // what may be searched
    var query: String?
    var year: Int?
    var status: String?          // "matched", "not_found", "error" or "pending"
    var manual: Bool?
    var current: MatchTitle?
    var episode: MatchEpisodeInfo?   // TV episode files
}

/// The title matched. `poster` is a server artwork URL.
struct MatchTitle: Decodable {
    var kind: String?            // "movie", "show", "season" or "episode"
    var tmdbId: Int
    var title: String
    var year: Int?
    var poster: String?
}

/// Which TMDB episode an episode file shows as, and why ("file", "season" or "manual").
struct MatchEpisodeInfo: Decodable {
    var fileSeason: Int
    var fileEpisode: Int
    var season: Int
    var episode: Int
    var source: String
    var title: String?
}

/// A TMDB search result: `poster` a TMDB image URL, `posterPath` its TMDB path (to use as a poster).
struct MatchHit: Decodable, Identifiable {
    var tmdbId: Int
    var title: String
    var year: Int?
    var overview: String?
    var poster: String?
    var posterPath: String?
    var id: Int { tmdbId }
}

/// The matched title's TMDB posters (GET /api/media/match/posters).
struct PosterList: Decodable {
    var current: String?         // TMDB path of the one in use
    var manual: Bool?
    var posters: [PosterOption]?
}

struct PosterOption: Decodable {
    var path: String
    var url: String
    var language: String?        // "" for posters without text
}

struct TmdbSeason: Decodable, Identifiable {
    var number: Int
    var name: String?
    var episodeCount: Int?
    var id: Int { number }
}

struct TmdbEpisode: Decodable, Identifiable {
    var number: Int
    var name: String?
    var airDate: String?
    var id: Int { number }
}

/// A person's page (GET /api/library/person), opened from a title page's cast: a larger photo,
/// their details, their titles in the library and their TMDB filmography, newest first.
struct PersonDetail: Decodable {
    let id: Int
    let name: String
    let photo: String?
    let biography: String?
    let birthday: String?
    let deathday: String?
    let placeOfBirth: String?
    let knownFor: String?
    let inLibrary: [LibraryTitle]
    let filmography: [PersonFilm]
}

/// A title in a person's filmography: `poster` a TMDB image URL; `inLibrary` those the library has.
struct PersonFilm: Decodable, Identifiable {
    let kind: String
    let tmdbId: Int
    let title: String
    let year: Int?
    let poster: String?
    let role: String?
    let inLibrary: Bool?
    var id: String { "\(kind)-\(tmdbId)" }
}

// MARK: The Guide: live channels the server plans (docs/GUIDE.md)

/// GET /api/guide/available
struct GuideAvailable: Decodable { let available: Bool? }

/// GET /api/guide: the grid, from the current half hour (Unix ms).
struct GuideListings: Decodable, Equatable {
    var now: Int64
    var from: Int64
    var to: Int64
    /// As far as the plan reaches: the grid loads more (from = to) as it scrolls, up to this.
    var until: Int64? = nil
    var channels: [GuideChannel]?
}

struct GuideChannel: Decodable, Hashable, Identifiable {
    let number: Int
    let name: String
    let genre: String?
    let color: String?
    /// Its logo: GET this with w= (width), color= (rrggbb) or format=svg.
    let logo: String?
    var listings: [GuideListing]?
    /// A channel of its own's: guide or weather (the app draws it); nil for a library's channels.
    var kind: String? = nil
    var id: Int { number }
}

/// A program in the grid, from its start to the next one's (Unix ms); no info: off the air.
struct GuideListing: Decodable, Hashable {
    let start: Int64
    let end: Int64
    let info: ProgramInfo?
}

struct ProgramInfo: Decodable, Hashable {
    let kind: String?          // movie or show
    let tmdbId: Int?
    let library: String?
    let title: String
    let year: Int?
    let rating: String?
    let genres: [String]?
    let overview: String?
    let poster: String?
    let season: Int?
    let episode: Int?
    let episodeTitle: String?
    let episodeOverview: String?

    /// "S2E5 · Name" for an episode, else nil.
    var episodeLabel: String? {
        guard let season, season > 0 else { return nil }
        let code = "S\(season)E\(episode ?? 0)"
        if let t = episodeTitle, !t.isEmpty { return "\(code) · \(t)" }
        return code
    }
}

/// GET /api/guide/now: what a channel airs (program, filler or bumper), and how far into it (seconds).
struct GuideNow: Decodable, Equatable {
    let channel: GuideChannel
    let kind: String
    let path: String?
    let offset: Double?
    let start: Int64
    let end: Int64
    let program: ProgramInfo?
    let next: ProgramInfo?
    let nextStart: Int64?
    let serverTime: Int64
    /// The Guide channel's preview and promo blocks: when and where the program airs.
    var airs: Int64? = nil
    var airsOn: GuideChannel? = nil
    /// What plays under the Weather channel and the Guide channel's cards.
    var music: GuideMusic? = nil
    /// A program's voice-over on its credits ("Stay tuned for…"), when it's been made.
    var voiceOver: GuideVoiceOver? = nil
}

/// When a program's voice-over plays: from at seconds into its file, the program lowered under it;
/// from squeezeAt (nil: never) the picture is squeezed beside an "Up next" card.
struct GuideVoiceOver: Decodable, Equatable {
    let url: String
    let at: Double
    let seconds: Double
    let squeezeAt: Double?
    let line: String
    let announcer: String
}

/// A track of the Guide's music, from offset seconds; the next starts at end.
struct GuideMusic: Decodable, Equatable {
    let url: String
    let title: String?
    let credit: String?
    let offset: Double
    let start: Int64?
    let end: Int64?
}

/// GET /api/guide/weather: the Weather channel's (the National Weather Service's, US units).
struct GuideWeather: Decodable, Equatable {
    var place: String? = nil
    var zip: String? = nil
    var updated: Int64? = nil
    var current: WeatherNow? = nil
    var periods: [WeatherPeriod]? = nil
    var alerts: [WeatherAlert]? = nil
    var radar: Bool? = nil
    var error: String? = nil
}

struct WeatherNow: Decodable, Equatable {
    let conditions: String?
    let icon: String?
    let temp: Int?
    let feelsLike: Int?
    let dewpoint: Int?
    let humidity: Int?
    let windDir: String?
    let wind: Int?
    let gust: Int?
    let pressure: Double?
    let visibility: Double?
}

struct WeatherPeriod: Decodable, Equatable {
    let name: String
    let daytime: Bool
    let temp: Int
    let forecast: String
    let icon: String?
    let precip: Int?
}

struct WeatherAlert: Decodable, Equatable {
    let event: String
    let headline: String?
}
