import Foundation

// What a download needs to play offline as it does streamed, as the Android app's DownloadWorker
// saves it: the codecs report (length, tracks), its subtitle tracks (text as WebVTT, ASS/SSA whole
// as .ass with the video's fonts, PGS as .sup: this phone's languages, forced ones, files beside the
// video and untagged ones), its intro and
// credits, its trickplay frames and its thumbnail. They're kept in `Documents/DownloadExtras/<id>/`
// and saved again every 12 hours while the server is open, and when a subtitle is added to the video.

enum DownloadExtras {
    static let refreshInterval: TimeInterval = 12 * 60 * 60

    static let root: URL = {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        let url = docs.appendingPathComponent("DownloadExtras", isDirectory: true)
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }()

    static func folder(_ id: String) -> URL { root.appendingPathComponent(id, isDirectory: true) }
    static func subtitleFolder(_ id: String) -> URL { folder(id).appendingPathComponent("subtitles", isDirectory: true) }
    static func trickplayFolder(_ id: String) -> URL { folder(id).appendingPathComponent("trickplay", isDirectory: true) }

    /// The file a saved subtitle track is kept in: a file beside the video by its key (its index
    /// can change as files come and go), the video's own tracks by index.
    static func subtitleName(_ t: SubtitleTrack) -> String {
        subtitleBase(t) + (t.isStyled ? ".ass" : t.isText ? ".vtt" : ".sup")
    }

    private static func subtitleBase(_ t: SubtitleTrack) -> String {
        if t.external != nil, let key = t.key {
            return String(key.map { $0.isLetter || $0.isNumber ? $0 : "-" })
        }
        return String(t.index)
    }

    /// A saved track's file and whether it's the whole ASS script (libass draws it): an ASS track
    /// saved before as WebVTT shows its plain lines until the next refresh saves it again.
    static func savedSubtitle(_ id: String, _ t: SubtitleTrack) -> (url: URL, styled: Bool)? {
        let dir = subtitleFolder(id)
        let file = dir.appendingPathComponent(subtitleName(t))
        if FileManager.default.fileExists(atPath: file.path) { return (file, t.isStyled) }
        let plain = dir.appendingPathComponent(subtitleBase(t) + ".vtt")
        if t.isStyled, FileManager.default.fileExists(atPath: plain.path) { return (plain, false) }
        return nil
    }

    /// A saved font of the video's: "font-<index>-<its name>".
    static func fontName(_ font: FontAttachment) -> String {
        let name = (font.name ?? "font").map { $0.isLetter || $0.isNumber || $0 == "." || $0 == "-" || $0 == "_" ? $0 : "-" }
        return "font-\(font.index)-" + String(name)
    }

    /// The video's fonts saved with its ASS tracks: (name, data) for libass.
    static func savedFonts(_ id: String) -> [(String, Data)] {
        let dir = subtitleFolder(id)
        let names = ((try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? []).filter { $0.hasPrefix("font-") }.sorted()
        return names.compactMap { name in
            guard let data = try? Data(contentsOf: dir.appendingPathComponent(name)) else { return nil }
            let parts = name.split(separator: "-", maxSplits: 2)
            return (parts.count == 3 ? String(parts[2]) : name, data)
        }
    }

    static func remove(_ id: String) {
        try? FileManager.default.removeItem(at: folder(id))
    }

    // MARK: Saving

    /// Saves everything for a finished download. Each part goes on without the others; a network
    /// error (the server is away) is thrown, so the rest of that server's downloads wait too.
    /// Returns whether every part was saved.
    static func save(id: String, sourcePath: String, thumbnail: String?, subtitleTrack: Int = -1, client: ServerClient,
                     stillWanted: @escaping () async -> Bool) async throws -> Bool {
        let dir = folder(id)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let encoded = ServerClient.encodePath(sourcePath)
        var complete = true
        func step(_ work: () async throws -> Void) async throws {
            do { try await work() } catch ServerError.transport(let m) { throw ServerError.transport(m) } catch { complete = false }
        }
        var codecs: CodecsInfo?
        try await step {
            let body = try await client.data("/api/media/codecs", query: [("path", encoded)])
            codecs = try ServerClient.decode(CodecsInfo.self, from: body)
            try write(body, to: dir.appendingPathComponent("codecs.json"))
        }
        if let codecs {
            try await step {
                try await saveSubtitles(id: id, encoded: encoded, tracks: codecs.subtitles ?? [], fonts: codecs.fonts ?? [],
                                  chosen: subtitleTrack, client: client)
            }
        }
        try await step {
            do {
                let body = try await client.data("/api/media/segments", query: [("path", encoded)])
                _ = try ServerClient.decode(MediaSegments.self, from: body)
                try write(body, to: dir.appendingPathComponent("segments.json"))
            } catch ServerError.http(404, _) {
                try? FileManager.default.removeItem(at: dir.appendingPathComponent("segments.json"))
            }
        }
        try await step { try await saveTrickplay(id: id, encoded: encoded, client: client, stillWanted: stillWanted) }
        if let url = client.url(for: thumbnail) {
            try await step { try write(try await client.image(at: url), to: dir.appendingPathComponent("thumb.jpg")) }
        }
        if !(await stillWanted()) { remove(id) }
        return complete
    }

    private static func saveSubtitles(id: String, encoded: String, tracks: [SubtitleTrack], fonts: [FontAttachment],
                                      chosen: Int, client: ServerClient) async throws {
        let languages = Set(deviceSubtitleLanguages().compactMap { Locale(identifier: $0).language.languageCode?.identifier(.alpha3) })
        let wanted = tracks.filter { t in
            (t.isText || t.isPGS) &&
                (t.forced == true || t.external != nil || (t.language ?? "").isEmpty || t.language == "und" ||
                    languages.contains(t.language ?? "") || languages.contains(bibliographic(t.language ?? "")) || t.index == chosen)
        }
        let dir = subtitleFolder(id)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        var saved: [SubtitleTrack] = []
        for t in wanted {
            let file = dir.appendingPathComponent(subtitleName(t))
            if (try? file.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0 > 0 { saved.append(t); continue }
            var query = [("path", encoded), ("track", String(t.index)), ("start", "0"), ("duration", "0")]
            if t.isStyled { query.append(("format", "ass")) } else if !t.isText { query.append(("format", "sup")) }
            do {
                try write(try await client.data("/api/media/subtitles", query: query), to: file)
                saved.append(t)
            } catch ServerError.transport(let m) {
                throw ServerError.transport(m)
            } catch {
                continue // this track only
            }
        }
        // The video's fonts, which its ASS tracks name, when one is saved.
        var fontFiles: [String] = []
        if saved.contains(where: \.isStyled) {
            for font in fonts {
                let file = dir.appendingPathComponent(fontName(font))
                if (try? file.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0 > 0 { fontFiles.append(fontName(font)); continue }
                do {
                    try write(try await client.data("/api/media/font", query: [("path", encoded), ("font", String(font.index))]), to: file)
                    fontFiles.append(fontName(font))
                } catch ServerError.transport(let m) {
                    throw ServerError.transport(m)
                } catch {
                    continue // this font only
                }
            }
        }
        // The CJK characters of the saved ASS scripts, as a subset of the server's CJK font, saved
        // beside them for offline play (the system's fonts may lack some, as Simplified Chinese does).
        let cjkFile = dir.appendingPathComponent("font-cjk.ttf")
        let styledFiles = saved.filter { $0.isStyled }.map { dir.appendingPathComponent(subtitleName($0)) }
        if !styledFiles.isEmpty {
            if (try? cjkFile.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0 > 0 {
                fontFiles.append("font-cjk.ttf")
            } else {
                var text = ""
                for file in styledFiles { text += (try? String(contentsOf: file, encoding: .utf8)) ?? "" }
                let chars = newCjkCharacters(in: text, seen: []).text
                if !chars.isEmpty, let font = try? await client.subtitleFont(chars) {
                    try write(font, to: cjkFile)
                    fontFiles.append("font-cjk.ttf")
                }
            }
        }
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        try write(try encoder.encode(saved), to: dir.appendingPathComponent("tracks.json"))
        // Tracks gone from the server (or saved before in another form): their files aren't needed.
        let keep = Set(saved.map(subtitleName) + fontFiles + ["tracks.json"])
        for name in (try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? [] where !keep.contains(name) {
            try? FileManager.default.removeItem(at: dir.appendingPathComponent(name))
        }
    }

    private static func saveTrickplay(id: String, encoded: String, client: ServerClient,
                                      stillWanted: () async -> Bool) async throws {
        let info: TrickplayInfo
        do {
            info = try await client.get("/api/media/trickplay", query: [("path", encoded)])
        } catch ServerError.http(let status, _) where status <= 500 {
            return // none made yet
        }
        let dir = trickplayFolder(id)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let frames = info.frames ?? []
        let names = Set(frames.map { "frame-\($0.time).jpg" })
        for name in (try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? [] where !names.contains(name) {
            try? FileManager.default.removeItem(at: dir.appendingPathComponent(name))
        }
        for (i, frame) in frames.enumerated() {
            if i % 50 == 0, !(await stillWanted()) { return }
            let file = dir.appendingPathComponent("frame-\(frame.time).jpg")
            if FileManager.default.fileExists(atPath: file.path) { continue }
            guard let url = client.url(for: frame.url) else { continue }
            do {
                try write(try await client.image(at: url), to: file)
            } catch ServerError.transport(let m) {
                throw ServerError.transport(m)
            } catch {
                continue // this frame only
            }
        }
    }

    /// Writes through a temporary file, so a cut-short write never leaves half a file.
    private static func write(_ data: Data, to url: URL) throws {
        try data.write(to: url, options: .atomic)
    }

    /// ISO 639-2/B codes for the few languages whose /T code differs (iOS gives /T: "deu", the
    /// server's files often say "ger").
    private static func bibliographic(_ code: String) -> String {
        let pairs = ["ger": "deu", "fre": "fra", "chi": "zho", "dut": "nld", "gre": "ell", "per": "fas",
                     "cze": "ces", "rum": "ron", "slo": "slk", "ice": "isl", "arm": "hye", "baq": "eus",
                     "bur": "mya", "geo": "kat", "mac": "mkd", "mao": "mri", "may": "msa", "alb": "sqi",
                     "tib": "bod", "wel": "cym"]
        return pairs[code] ?? code
    }

    // MARK: Reading, for the player offline

    static func codecs(_ id: String) -> CodecsInfo? {
        guard let data = try? Data(contentsOf: folder(id).appendingPathComponent("codecs.json")) else { return nil }
        return try? ServerClient.decode(CodecsInfo.self, from: data)
    }

    static func segments(_ id: String) -> MediaSegments? {
        guard let data = try? Data(contentsOf: folder(id).appendingPathComponent("segments.json")) else { return nil }
        return try? ServerClient.decode(MediaSegments.self, from: data)
    }

    static func subtitleTracks(_ id: String) -> [SubtitleTrack]? {
        guard let data = try? Data(contentsOf: subtitleFolder(id).appendingPathComponent("tracks.json")) else { return nil }
        return try? ServerClient.decode([SubtitleTrack].self, from: data)
    }

    /// Keeps a timing correction made offline with the saved track.
    static func saveSubtitleTracks(_ id: String, _ tracks: [SubtitleTrack]) {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        guard let data = try? encoder.encode(tracks) else { return }
        try? write(data, to: subtitleFolder(id).appendingPathComponent("tracks.json"))
    }

    /// The saved trickplay frames, as file:// URLs.
    static func trickplay(_ id: String) -> [TrickplayFrame] {
        let dir = trickplayFolder(id)
        let names = (try? FileManager.default.contentsOfDirectory(atPath: dir.path)) ?? []
        return names.compactMap { name -> TrickplayFrame? in
            guard name.hasPrefix("frame-"), name.hasSuffix(".jpg"),
                  let time = Int(name.dropFirst(6).dropLast(4)) else { return nil }
            return TrickplayFrame(time: time, url: dir.appendingPathComponent(name).absoluteString)
        }
        .sorted { $0.time < $1.time }
    }

    static func thumbnail(_ id: String) -> URL? {
        let url = folder(id).appendingPathComponent("thumb.jpg")
        return FileManager.default.fileExists(atPath: url.path) ? url : nil
    }
}
