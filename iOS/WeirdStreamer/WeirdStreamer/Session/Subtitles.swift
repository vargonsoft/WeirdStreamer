import Foundation
import SwiftUI
import CoreGraphics

// Subtitles for a video from a WeirdStreamer server, as the Android app's ServerSubtitles: the
// video's own tracks, then files beside it and downloads from OpenSubtitles. Text tracks come from
// the server as WebVTT two minutes at a time and are drawn over the video, so they show whatever
// the stream is (a transcode has none of its own). PGS tracks come the same way, as pictures to
// decode here; other picture tracks (DVD) can only be burned into a transcode (`burnIn`).

/// One subtitle line, from start to end (milliseconds, in the file's time).
struct SubtitleCue {
    let startMs: Int64
    let endMs: Int64
    let text: AttributedString
    let plain: String
}

@MainActor
@Observable
final class ServerSubtitles {
    private(set) var tracks: [SubtitleTrack] = []
    private(set) var selected = -1
    private(set) var loaded = false

    @ObservationIgnored private let client: ServerClient?
    /// A download's saved tracks (its id), read from the phone instead of the server.
    @ObservationIgnored private let downloadId: String?
    @ObservationIgnored private let sourcePath: String
    @ObservationIgnored private let encodedPath: String
    @ObservationIgnored private var cues: [SubtitleCue] = []
    @ObservationIgnored private var pictures: [PgsSet] = []
    @ObservationIgnored private var fromMs: Int64 = 0
    @ObservationIgnored private var untilMs: Int64 = 0
    @ObservationIgnored private var generation = 0
    @ObservationIgnored private var saving: Task<Void, Never>?
    // An ASS track: the first window's header (to its [Events] and Format lines) and every
    // Dialogue line loaded, once each; assVersion counts changes, for the renderer to take the
    // script again.
    @ObservationIgnored private var assHeader = ""
    @ObservationIgnored private var assLines: [String] = []
    @ObservationIgnored private var assKnown: Set<String> = []
    @ObservationIgnored private(set) var assVersion = 0
    @ObservationIgnored private var fonts: [FontAttachment] = []
    @ObservationIgnored private var fontData: [(String, Data)]?

    private static let windowMs: Int64 = 120_000
    private static let aheadMs: Int64 = 40_000

    init(client: ServerClient?, path: String, downloadId: String? = nil) {
        self.client = client
        self.downloadId = downloadId
        self.sourcePath = path
        self.encodedPath = ServerClient.encodePath(path)
    }

    /// Whether search and download online can work (a server is reachable for this video).
    var online: Bool { client != nil }

    var labels: [String] { serverSubtitleLabels(tracks) }
    var selectedTrack: SubtitleTrack? { tracks.indices.contains(selected) ? tracks[selected] : nil }
    var offsetMs: Int { selectedTrack?.offsetMs ?? 0 }

    /// Whether the selected track is ASS/SSA, which libass draws styled (AssSubtitleView). A
    /// download's only when it saved the whole script (one saved before as WebVTT shows plain lines).
    var styled: Bool {
        guard let track = selectedTrack, track.isStyled else { return false }
        guard let downloadId else { return true }
        return DownloadExtras.savedSubtitle(downloadId, track)?.styled == true
    }

    /// The ASS script loaded so far: the header and every line, for libass.
    func assScript() -> String { assHeader + assLines.joined(separator: "\n") + "\n" }

    func setFonts(_ list: [FontAttachment]) { fonts = list }

    /// The video's embedded fonts (name, data), which its ASS tracks name: fetched once.
    func videoFonts() async -> [(String, Data)] {
        if let fontData { return fontData }
        if let downloadId {
            let saved = DownloadExtras.savedFonts(downloadId)
            fontData = saved
            return saved
        }
        guard let client else { return [] }
        var got: [(String, Data)] = []
        for font in fonts {
            if let data = try? await client.data("/api/media/font", query: [("path", encodedPath), ("font", String(font.index))]) {
                got.append((font.name ?? "font\(font.index)", data))
            }
        }
        fontData = got
        return got
    }

    /// The picture track to burn into the stream (subtitle=N), or -1.
    var burnIn: Int {
        guard let t = selectedTrack, !t.isText, !t.isPGS else { return -1 }
        return t.index
    }

    func setTracks(_ list: [SubtitleTrack]) {
        tracks = list
        loaded = true
    }

    func select(_ index: Int) {
        selected = tracks.indices.contains(index) ? index : -1
        cues = []
        pictures = []
        assHeader = ""
        assLines = []
        assKnown = []
        assVersion += 1
        fromMs = 0
        untilMs = 0
        generation += 1
    }

    /// Moves the selected text track's lines by `deltaMs` (negative: earlier) at once, and saves
    /// the correction for every player a moment after the last change.
    func shift(_ deltaMs: Int) {
        guard var track = selectedTrack, track.isText else { return }
        track.offsetMs = (track.offsetMs ?? 0) + deltaMs
        tracks[selected] = track
        if let downloadId { DownloadExtras.saveSubtitleTracks(downloadId, tracks) }
        guard let key = track.key, let client = self.client else { return }
        let offset = track.offsetMs ?? 0
        let path = encodedPath
        saving?.cancel()
        saving = Task {
            try? await Task.sleep(for: .milliseconds(600))
            guard !Task.isCancelled else { return }
            struct Body: Encodable { let path: String; let key: String; let offset_ms: Int }
            try? await client.send("PUT", "/api/media/subtitle-offset", body: Body(path: path, key: key, offset_ms: offset))
        }
    }

    /// Loads the cues around `playerMs`, and the next window before it's needed.
    func keepAhead(_ playerMs: Int64) async {
        guard let track = selectedTrack, track.isText || track.isPGS else { return }
        let positionMs = playerMs - Int64(offsetMs)
        let outside = positionMs < fromMs || positionMs > untilMs
        if !outside && positionMs < untilMs - Self.aheadMs { return }
        let start = outside ? max(0, positionMs - 10_000) : untilMs
        let gen = generation
        if let downloadId {
            // Saved whole: read once.
            guard outside || fromMs != 0 || untilMs != Int64.max else { return }
            guard let saved = DownloadExtras.savedSubtitle(downloadId, track),
                  let body = try? Data(contentsOf: saved.url) else { return }
            if saved.styled {
                addAssWindow(String(decoding: body, as: UTF8.self))
            } else if track.isPGS {
                let all = await Task.detached { decodePgs(body) }.value
                guard gen == generation else { return }
                pictures = all
            } else {
                cues = parseWebVtt(String(decoding: body, as: UTF8.self))
            }
            fromMs = 0
            untilMs = Int64.max
            return
        }
        guard let client else { return }
        if styled {
            await keepAssAhead(track, client: client, start: start, outside: outside, generation: gen)
            return
        }
        var query = [("path", encodedPath), ("track", String(track.index)),
                     ("start", String(format: "%.3f", Double(start) / 1000)),
                     ("duration", String(format: "%.3f", Double(Self.windowMs) / 1000))]
        if track.isPGS { query.append(("format", "sup")) }
        guard let body = try? await client.data("/api/media/subtitles", query: query), gen == generation else { return }
        if track.isPGS {
            let window = await Task.detached { decodePgs(body) }.value
            guard gen == generation else { return }
            if outside {
                pictures = window
                fromMs = start
            } else {
                let known = Set(pictures.map(\.startMs))
                pictures = (pictures.filter { $0.startMs > positionMs - 60_000 } + window.filter { !known.contains($0.startMs) })
                    .sorted { $0.startMs < $1.startMs }
            }
        } else {
            let window = parseWebVtt(String(decoding: body, as: UTF8.self))
            if outside {
                cues = window
                fromMs = start
            } else {
                // The windows overlap a little: skip lines already had (by time and text).
                let known = Set(cues.map { "\($0.startMs)|\($0.plain)" })
                cues += window.filter { !known.contains("\($0.startMs)|\($0.plain)") }
            }
        }
        untilMs = start + Self.windowMs
    }

    /// An ASS track's windows (format=ass: each a whole script at the file's times): the first
    /// one's header is kept, and every window's Dialogue lines are added once (they overlap a
    /// little where a window starts). A jump outside what's loaded adds the window there too: the
    /// lines loaded stay, as libass only shows those of the moment.
    private func keepAssAhead(_ track: SubtitleTrack, client: ServerClient, start: Int64, outside: Bool, generation gen: Int) async {
        let query = [("path", encodedPath), ("track", String(track.index)),
                     ("start", String(format: "%.3f", Double(start) / 1000)),
                     ("duration", String(format: "%.3f", Double(Self.windowMs) / 1000)), ("format", "ass")]
        guard let data = try? await client.data("/api/media/subtitles", query: query), gen == generation else { return }
        addAssWindow(String(decoding: data, as: UTF8.self))
        if outside { fromMs = start }
        untilMs = start + Self.windowMs
    }

    /// Adds an ASS script's lines (a window, or a download's whole script); the first one gives the header.
    private func addAssWindow(_ text: String) {
        let body = text.replacingOccurrences(of: "\r", with: "")
        let events = body.range(of: "\n[Events]")
        if assHeader.isEmpty {
            let tail = events.map { String(body[$0.lowerBound...]) } ?? ""
            let format = tail.components(separatedBy: "\n").first { $0.hasPrefix("Format:") }
                ?? "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"
            let head = events.map { String(body[..<$0.lowerBound]) } ?? body
            assHeader = head.trimmingCharacters(in: .whitespacesAndNewlines) + "\n\n[Events]\n" + format + "\n"
        }
        if let events {
            for line in body[events.lowerBound...].components(separatedBy: "\n") where line.hasPrefix("Dialogue:") {
                if assKnown.insert(line).inserted { assLines.append(line) }
            }
        }
        assVersion += 1
    }

    /// The line (or lines) to show at `playerMs` (none for an ASS track libass draws).
    func cueAt(_ playerMs: Int64) -> AttributedString? {
        guard selectedTrack?.isText == true, !styled else { return nil }
        let positionMs = playerMs - Int64(offsetMs)
        let at = cues.filter { positionMs >= $0.startMs && positionMs < $0.endMs }
        guard !at.isEmpty else { return nil }
        var joined = AttributedString()
        for (i, cue) in at.enumerated() {
            if i > 0 { joined += AttributedString("\n") }
            joined += cue.text
        }
        return joined
    }

    /// The PGS display set showing at `playerMs`.
    func pictureAt(_ playerMs: Int64) -> PgsSet? {
        guard selectedTrack?.isPGS == true else { return nil }
        return pictures.last { $0.startMs <= playerMs }
    }

    /// OpenSubtitles results for the video (in this phone's languages); throws with the server's message.
    func search() async throws -> [SubtitleHit] {
        guard let client else { throw ServerError.transport("Not connected to the server") }
        let found: SubtitleSearch = try await client.get("/api/media/subtitles/search",
                                                         query: [("path", encodedPath), ("languages", deviceSubtitleLanguages().joined(separator: ","))])
        return found.results ?? []
    }

    /// Downloads (or reuses) a result and selects it.
    func download(_ hit: SubtitleHit) async throws -> Int {
        struct Body: Encodable { let path: String; let result: SubtitleHit }
        guard let client else { throw ServerError.transport("Not connected to the server") }
        let got: SubtitleDownload = try await client.callSnake("POST", "/api/media/subtitles/download",
                                                               body: Body(path: encodedPath, result: hit))
        tracks = got.subtitles ?? tracks
        select(got.index ?? -1)
        DownloadManager.shared.extrasChanged(sourcePath) // its downloads save the new track
        return got.index ?? -1
    }
}

/// Two-letter language codes to search OpenSubtitles in: this phone's languages.
func deviceSubtitleLanguages() -> [String] {
    var seen: [String] = []
    for id in Locale.preferredLanguages {
        if let code = Locale(identifier: id).language.languageCode?.identifier, code.count == 2, !seen.contains(code) {
            seen.append(code)
        }
    }
    return seen.isEmpty ? ["en"] : seen
}

// MARK: WebVTT

/// Reads WebVTT cues: their times, and their text with italics kept and other tags dropped.
func parseWebVtt(_ body: String) -> [SubtitleCue] {
    var cues: [SubtitleCue] = []
    let text = body.replacingOccurrences(of: "\r\n", with: "\n")
    for block in text.components(separatedBy: "\n\n") {
        let lines = block.components(separatedBy: "\n")
        guard let timing = lines.firstIndex(where: { $0.contains("-->") }) else { continue }
        let ends = lines[timing].components(separatedBy: "-->").map {
            $0.trimmingCharacters(in: .whitespaces).components(separatedBy: " ").first ?? ""
        }
        guard ends.count == 2, let start = vttTime(ends[0]), let end = vttTime(ends[1]) else { continue }
        let raw = lines[(timing + 1)...].joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !raw.isEmpty else { continue }
        let (styled, plain) = styledCue(raw)
        cues.append(SubtitleCue(startMs: start, endMs: end, text: styled, plain: plain))
    }
    return cues
}

private func vttTime(_ s: String) -> Int64? {
    let parts = s.components(separatedBy: ":")
    guard let seconds = Double(parts.last ?? "") else { return nil }
    let minutes = parts.count >= 2 ? Int64(parts[parts.count - 2]) ?? 0 : 0
    let hours = parts.count >= 3 ? Int64(parts[0]) ?? 0 : 0
    return (hours * 3600 + minutes * 60) * 1000 + Int64(seconds * 1000)
}

private let cueTag = try! NSRegularExpression(pattern: "<(/?)([a-zA-Z]+)[^>]*>|\\{\\\\[^}]*\\}")

/// The cue's text with its italics; other tags (and ASS overrides) dropped.
private func styledCue(_ text: String) -> (AttributedString, String) {
    var result = AttributedString()
    var plain = ""
    var italic = 0
    var at = text.startIndex
    func put(_ s: Substring) {
        let clean = s.replacingOccurrences(of: "&amp;", with: "&").replacingOccurrences(of: "&lt;", with: "<")
            .replacingOccurrences(of: "&gt;", with: ">").replacingOccurrences(of: "&nbsp;", with: " ")
        var piece = AttributedString(clean)
        if italic > 0 { piece.inlinePresentationIntent = .emphasized }
        result += piece
        plain += clean
    }
    let ns = text as NSString
    for m in cueTag.matches(in: text, range: NSRange(location: 0, length: ns.length)) {
        guard let range = Range(m.range, in: text) else { continue }
        put(text[at..<range.lowerBound])
        at = range.upperBound
        if let nameRange = Range(m.range(at: 2), in: text), text[nameRange].lowercased() == "i" {
            let closing = Range(m.range(at: 1), in: text).map { !text[$0].isEmpty } ?? false
            italic = closing ? max(0, italic - 1) : italic + 1
        }
    }
    put(text[at...])
    return (result, plain)
}

// MARK: Labels

private let bibliographic = ["fre": "fra", "ger": "deu", "chi": "zho", "dut": "nld", "cze": "ces", "gre": "ell",
                             "per": "fas", "rum": "ron", "slo": "slk", "ice": "isl", "may": "msa", "baq": "eus",
                             "arm": "hye", "geo": "kat", "mac": "mkd", "alb": "sqi", "bur": "mya", "tib": "bod", "wel": "cym"]

/// A language code (eng, fre, en…) as a name in this phone's language: "English", "French".
func languageName(_ code: String?) -> String? {
    guard let c = code?.trimmingCharacters(in: .whitespaces).lowercased(), !c.isEmpty, c != "und" else { return nil }
    let name = Locale.current.localizedString(forLanguageCode: bibliographic[c] ?? c) ?? c
    return name.prefix(1).uppercased() + name.dropFirst()
}

/// Names the server's subtitle tracks as the Android app does: language, title, marks (Forced,
/// SDH), where it's from (a file beside the video, a download), and Image for picture tracks; a
/// number for ones that still read the same.
func serverSubtitleLabels(_ tracks: [SubtitleTrack]) -> [String] {
    let base = tracks.enumerated().map { i, t -> String in
        let language = languageName(t.language)
        let trimmed = t.title?.trimmingCharacters(in: .whitespaces) ?? ""
        let title: String? = trimmed.isEmpty || trimmed.caseInsensitiveCompare(language ?? "") == .orderedSame
            ? nil : String(trimmed.prefix(40))
        var parts = [language ?? "Subtitle \(i + 1)"]
        if let title { parts.append(title) }
        if t.forced == true { parts.append("Forced") }
        if t.hearingImpaired == true && !(title?.localizedCaseInsensitiveContains("SDH") ?? false) { parts.append("SDH") }
        if t.external == "opensubtitles" { parts.append("Downloaded") } else if t.external == "file" { parts.append("File") }
        if !t.isText { parts.append("Image") }
        return parts.joined(separator: " · ")
    }
    var counts: [String: Int] = [:]
    base.forEach { counts[$0, default: 0] += 1 }
    var seen: [String: Int] = [:]
    return base.map { label in
        guard (counts[label] ?? 0) > 1 else { return label }
        seen[label, default: 0] += 1
        return "\(label) (\(seen[label]!))"
    }
}

/// How far the subtitles are moved, in words: "In sync", "250 ms earlier", "1.5 s later".
func subtitleOffsetText(_ ms: Int) -> String {
    if ms == 0 { return "In sync" }
    let a = abs(ms)
    var amount: String
    if a >= 1000 {
        amount = String(format: "%.2f", Double(a) / 1000)
        while amount.hasSuffix("0") { amount.removeLast() }
        if amount.hasSuffix(".") { amount.removeLast() }
        amount += " s"
    } else {
        amount = "\(a) ms"
    }
    return amount + (ms < 0 ? " earlier" : " later")
}

// MARK: PGS

/// PGS: Blu-ray's picture subtitles, as ffmpeg's raw .sup stream: palettes, run-length objects and
/// compositions (which objects go where), each display set closed by an end segment. A set with
/// no objects clears the screen. Pictures are decoded only when shown.
nonisolated final class PgsObject: @unchecked Sendable {
    let width: Int
    let height: Int
    var chunks: [Data]
    init(width: Int, height: Int, chunks: [Data]) { self.width = width; self.height = height; self.chunks = chunks }
}

nonisolated struct PgsPart: @unchecked Sendable {
    let x: Int, y: Int
    let crop: [Int]?
    let object: PgsObject
    let palette: [UInt32]
}

/// What's on screen from `startMs` until the next set: `parts` on a width×height frame.
nonisolated struct PgsSet: @unchecked Sendable {
    let startMs: Int64
    let width: Int
    let height: Int
    let parts: [PgsPart]

    /// The whole set as one picture (only the part of the frame in use), or nil when it clears.
    nonisolated func render() -> PgsPicture? {
        guard !parts.isEmpty else { return nil }
        let left = parts.map(\.x).min()!, top = parts.map(\.y).min()!
        let right = parts.map { $0.x + ($0.crop?[2] ?? $0.object.width) }.max()!
        let bottom = parts.map { $0.y + ($0.crop?[3] ?? $0.object.height) }.max()!
        let w = right - left, h = bottom - top
        guard w > 0, h > 0 else { return nil }
        var out = [UInt32](repeating: 0, count: w * h)
        for part in parts {
            let source = pixels(part)
            let crop = part.crop ?? [0, 0, part.object.width, part.object.height]
            for row in 0..<crop[3] {
                let y = crop[1] + row
                if y >= part.object.height { break }
                let copied = min(crop[2], part.object.width - crop[0])
                guard copied > 0 else { continue }
                let src = y * part.object.width + crop[0]
                let dst = (part.y - top + row) * w + part.x - left
                guard dst >= 0, dst + copied <= out.count, src + copied <= source.count else { continue }
                for k in 0..<copied { out[dst + k] = source[src + k] }
            }
        }
        // Premultiplied BGRA for CoreGraphics.
        let premultiplied = out.map { argb -> UInt32 in
            let a = (argb >> 24) & 0xFF
            let r = ((argb >> 16) & 0xFF) * a / 255, g = ((argb >> 8) & 0xFF) * a / 255, b = (argb & 0xFF) * a / 255
            return (a << 24) | (r << 16) | (g << 8) | b
        }
        let data = premultiplied.withUnsafeBufferPointer { Data(buffer: $0) }
        guard let provider = CGDataProvider(data: data as CFData),
              let image = CGImage(width: w, height: h, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: w * 4,
                                  space: CGColorSpaceCreateDeviceRGB(),
                                  bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue),
                                  provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent) else { return nil }
        return PgsPicture(image: image, x: left, y: top, frameWidth: width, frameHeight: height)
    }

    // Lines run-length coded: a nonzero byte is one pixel of that color; 0 then a flag byte is a
    // run (bit 7: a color follows, else clear; bit 6: a longer count), or the line's end.
    private func pixels(_ part: PgsPart) -> [UInt32] {
        let obj = part.object
        var out = [UInt32](repeating: 0, count: obj.width * obj.height)
        var chunk = 0, at = 0
        func next() -> Int {
            while chunk < obj.chunks.count && at >= obj.chunks[chunk].count { chunk += 1; at = 0 }
            guard chunk < obj.chunks.count else { return -1 }
            let data = obj.chunks[chunk]
            defer { at += 1 }
            return Int(data[data.startIndex + at])
        }
        var x = 0, y = 0
        var b = next()
        while b >= 0 && y < obj.height {
            if b != 0 {
                if x < obj.width { out[y * obj.width + x] = part.palette[b] }
                x += 1
            } else {
                let flags = next()
                if flags <= 0 {
                    x = 0
                    y += 1
                } else {
                    let length = flags & 0x40 != 0 ? ((flags & 0x3F) << 8) | max(next(), 0) : flags & 0x3F
                    let color: UInt32 = flags & 0x80 != 0 ? part.palette[max(next(), 0)] : 0
                    for _ in 0..<length where x < obj.width {
                        out[y * obj.width + x] = color
                        x += 1
                    }
                }
            }
            b = next()
        }
        return out
    }
}

/// A display set's picture at (x, y) on a frameWidth×frameHeight frame.
nonisolated struct PgsPicture: @unchecked Sendable {
    let image: CGImage
    let x: Int, y: Int
    let frameWidth: Int, frameHeight: Int
}

/// An ARGB color from a palette entry, in the BT.709 limited range Blu-rays use.
nonisolated private func pgsColor(_ y: Int, _ cr: Int, _ cb: Int, _ alpha: Int) -> UInt32 {
    let luma = 1.164 * Double(y - 16)
    let red = Double(cr - 128), blue = Double(cb - 128)
    func clamp(_ v: Double) -> UInt32 { UInt32(min(max(Int(v), 0), 255)) }
    return UInt32(alpha) << 24 | clamp(luma + 1.793 * red) << 16 | clamp(luma - 0.533 * red - 0.213 * blue) << 8 | clamp(luma + 2.112 * blue)
}

/// Decodes a .sup stream into its display sets.
nonisolated func decodePgs(_ data: Data) -> [PgsSet] {
    let bytes = [UInt8](data)
    func u8(_ i: Int) -> Int { Int(bytes[i]) }
    func u16(_ i: Int) -> Int { Int(bytes[i]) << 8 | Int(bytes[i + 1]) }
    struct Placed { let id: Int, x: Int, y: Int; let crop: [Int]? }
    struct Composition { let ms: Int64, width: Int, height: Int, palette: Int; let placed: [Placed] }
    var palettes: [Int: [UInt32]] = [:]
    var objects: [Int: PgsObject] = [:]
    var sets: [PgsSet] = []
    var composition: Composition?
    var i = 0
    while i + 13 <= bytes.count {
        guard u16(i) == 0x5047 else { break } // "PG"
        let pts = Int64(u16(i + 2)) << 16 | Int64(u16(i + 4))
        let type = u8(i + 10)
        let p = i + 13
        i = p + u16(i + 11)
        guard i <= bytes.count else { break }
        switch type {
        case 0x14: // palette
            var table = palettes[u8(p)] ?? [UInt32](repeating: 0, count: 256)
            var q = p + 2
            while q + 5 <= i {
                table[u8(q)] = pgsColor(u8(q + 1), u8(q + 2), u8(q + 3), u8(q + 4))
                q += 5
            }
            palettes[u8(p)] = table
        case 0x15: // object
            let id = u16(p)
            if u8(p + 3) & 0x80 != 0, p + 11 <= i {
                objects[id] = PgsObject(width: u16(p + 7), height: u16(p + 9), chunks: [Data(bytes[(p + 11)..<i])])
            } else if p + 4 <= i {
                objects[id]?.chunks.append(Data(bytes[(p + 4)..<i]))
            }
        case 0x16: // composition
            var placed: [Placed] = []
            var q = p + 11
            for _ in 0..<u8(p + 10) {
                guard q + 8 <= i else { break }
                let cropped = u8(q + 3) & 0x80 != 0
                placed.append(Placed(id: u16(q), x: u16(q + 4), y: u16(q + 6),
                                     crop: cropped && q + 16 <= i ? [u16(q + 8), u16(q + 10), u16(q + 12), u16(q + 14)] : nil))
                q += cropped ? 16 : 8
            }
            composition = Composition(ms: pts / 90, width: u16(p), height: u16(p + 2), palette: u8(p + 9), placed: placed)
        case 0x80: // end of a display set
            if let c = composition {
                let parts: [PgsPart] = palettes[c.palette].map { palette in
                    c.placed.compactMap { placed in
                        objects[placed.id].map { PgsPart(x: placed.x, y: placed.y, crop: placed.crop, object: $0, palette: palette) }
                    }
                } ?? []
                sets.append(PgsSet(startMs: c.ms, width: c.width, height: c.height, parts: parts))
                composition = nil
            }
        default:
            break
        }
    }
    return sets
}
