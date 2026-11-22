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

    enum CodingKeys: String, CodingKey { case id = "path", name, items, dirs, libraryType, thumbnailUrl }
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

struct TrickplaySettings: Decodable {
    let runners: Int?
    let framesPerRunner: Int?
    let maxRunners: Int?
    let maxFrames: Int?
    let maxProduct: Int?
}

struct JobsResponse: Decodable {
    let jobs: [BackgroundJob]?
    let load: JobLoad?
    let storage: [StorageUse]?
    let trickplay: TrickplaySettings?
}
