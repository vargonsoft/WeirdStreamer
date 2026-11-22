import Foundation

/// A track's language tag (eng, fre, en, pt-BR) as a two-letter code ("en", "fr"), as the account's
/// language is kept; nil when it's unknown (und, empty).
func languageCode(_ tag: String?) -> String? {
    guard var c = tag?.trimmingCharacters(in: .whitespaces).lowercased(), !c.isEmpty, c != "und" else { return nil }
    if let cut = c.firstIndex(where: { $0 == "-" || $0 == "_" }) { c = String(c[..<cut]) }
    if c.count == 2 { return c }
    let terminology = ["fre": "fra", "ger": "deu", "chi": "zho", "dut": "nld", "cze": "ces", "gre": "ell", "per": "fas",
                       "rum": "ron", "slo": "slk", "ice": "isl", "may": "msa", "baq": "eus", "arm": "hye", "geo": "kat",
                       "mac": "mkd", "alb": "sqi", "bur": "mya", "tib": "bod", "wel": "cym"][c] ?? c
    return Locale.LanguageCode(terminology).identifier(.alpha2) ?? c
}

/// The audio and subtitle languages a download is made with, chosen in the download sheet
/// (two-letter codes, as the account's language; subtitles also `off`), and the tracks they are in a
/// file, as Android's DownloadLanguages. A language the file doesn't have, or none chosen, leaves it
/// as playing it would: the server's pick for the account's languages (the codecs report's preferred
/// tracks), else the file's default audio and no subtitles but forced ones.
enum DownloadLanguages {
    static let off = "off"

    /// Subtitles the player draws: text, and PGS pictures.
    static func drawn(_ t: SubtitleTrack) -> Bool { t.isText || t.isPGS }

    /// The audio track (the server's index) in `language` in `c`, else the one playing would pick (-1: the default).
    static func audioTrack(_ c: CodecsInfo, _ language: String?) -> Int {
        let preferred = c.preferredAudio ?? -1
        let matches = (c.audio ?? []).filter { language != nil && languageCode($0.language) == language }
        guard let first = matches.first else { return preferred }
        let pick = matches.first { $0.index == preferred } ?? matches.first { $0.default == true } ?? first
        return pick.index ?? -1
    }

    /// The subtitle track (the server's index) to show for `language` in `c` (`off`: -1), else the one playing would pick.
    static func subtitleTrack(_ c: CodecsInfo, _ language: String?) -> Int {
        if language == off { return -1 }
        let preferred = c.preferredSubtitle ?? -1
        let matches = (c.subtitles ?? []).filter { $0.forced != true && drawn($0) && language != nil && languageCode($0.language) == language }
        guard let first = matches.first else { return preferred }
        return (matches.first { $0.index == preferred } ?? matches.first { $0.default == true }
            ?? matches.first { $0.hearingImpaired != true } ?? first).index
    }

    private static func distinct(_ codes: [String]) -> [String] {
        var seen = Set<String>()
        return codes.filter { seen.insert($0).inserted }
    }

    /// The audio languages of `files`, in order of first appearance.
    static func audioLanguages(_ files: [CodecsInfo]) -> [String] {
        distinct(files.flatMap { ($0.audio ?? []).compactMap { languageCode($0.language) } })
    }

    /// The languages of `files`' full subtitles (not forced) the player draws.
    static func subtitleLanguages(_ files: [CodecsInfo]) -> [String] {
        distinct(files.flatMap { ($0.subtitles ?? []).filter { $0.forced != true && drawn($0) }.compactMap { languageCode($0.language) } })
    }

    /// The audio language the sheet starts with: the last chosen (`last`), if `options` has it, else what playing would pick.
    static func startAudio(_ files: [CodecsInfo], _ options: [String], last: String?) -> String? {
        if let last, options.contains(last) { return last }
        guard let c = files.first(where: { ($0.preferredAudio ?? -1) >= 0 }) ?? files.first else { return nil }
        let list = c.audio ?? []
        let track = list.first { $0.index == c.preferredAudio } ?? list.first { $0.default == true } ?? list.first
        return languageCode(track?.language).flatMap { options.contains($0) ? $0 : nil }
    }

    /// The subtitle language the sheet starts with: the last chosen (`off` or one `options` has), else what playing would pick.
    static func startSubtitles(_ files: [CodecsInfo], _ options: [String], last: String?) -> String {
        if let last, last == off || options.contains(last) { return last }
        guard let c = files.first(where: { ($0.preferredSubtitle ?? -1) >= 0 }) else { return off }
        let code = languageCode((c.subtitles ?? []).first { $0.index == c.preferredSubtitle }?.language)
        return code.flatMap { options.contains($0) ? $0 : nil } ?? off
    }
}
