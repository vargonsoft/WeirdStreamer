import SwiftUI

/// What Fix applies to: a file (its client path) or a folder that stands for one title.
/// `wholeTitle`: opened for a title page's whole title (through its first file), so its episode
/// isn't offered. As Android's MatchTarget.
struct MatchTarget: Identifiable, Hashable {
    var path: String
    var isDir: Bool
    var name: String
    var wholeTitle = false
    var id: String { "\(isDir ? "dir" : "file"):\(path)" }

    /// The request's query: `path` (base64 client path) for a file, `dir` (client path) for a folder.
    var query: [(String, String)] { isDir ? [("dir", path)] : [("path", ServerClient.encodePath(path))] }

    /// A request body's target keys, plus `extra`.
    func body(_ extra: [String: Any] = [:]) -> [String: Any] {
        var body = extra
        if isDir { body["dir"] = path } else { body["path"] = ServerClient.encodePath(path) }
        return body
    }
}

/// A poster to pick: its TMDB path (what the server takes), a preview URL and a label.
struct PosterPick: Identifiable {
    var path: String
    var url: String
    var label: String
    var id: String { path }
}

/// Fixing a file's or folder's TMDB match: the current match, a TMDB search to pick another (or
/// none), the poster, and an episode file's TMDB episode. As Android's MatchModel.
@Observable
final class MatchModel {
    let client: ServerClient
    let target: MatchTarget
    var info: MatchInfo?
    var loadError: String?
    var message: (String, Bool)?
    /// Something was changed: the screen behind reloads when the sheet closes.
    private(set) var changed = false

    var query = ""
    var year = ""
    private(set) var searchKind = "movie"
    private(set) var hits: [MatchHit]?
    private(set) var searching = false

    private(set) var posters: PosterList?
    private var postersFor = 0
    private(set) var postersError: String?

    private(set) var seasons: [TmdbSeason]?
    private var seasonsFor = 0
    private(set) var season: Int?
    private(set) var episodes: [TmdbEpisode]?
    private(set) var episodesError: String?
    /// The episode picked in the list (season, episode), to apply to this file or its whole season.
    var pick: (Int, Int)?

    private var searchTask: Task<Void, Never>?

    init(client: ServerClient, target: MatchTarget) {
        self.client = client
        self.target = target
    }

    func load() async {
        do {
            let got: MatchInfo = try await client.get("/api/media/match", query: target.query)
            loadError = nil
            query = got.query ?? ""
            year = (got.year ?? 0) > 0 ? String(got.year!) : ""
            searchKind = got.matchKind ?? got.kind ?? "movie"
            apply(got)
            search()
        } catch {
            loadError = errorText(error)
        }
    }

    var kinds: [String] { info?.kinds ?? [] }

    /// A TV-library file that isn't an episode: matched on its own, by hand only.
    var singleFile: Bool { info?.kind == "tv" && info?.kinds == ["movie"] }

    var matchedKind: String { info?.matchKind ?? info?.kind ?? "movie" }

    /// "Movie matched by hand: Heat (1995)", or what's wrong.
    var statusText: String {
        guard let info else { return "" }
        let kindLabel = matchedKind == "tv" ? "TV show" : "Movie"
        if let c = info.current {
            return "\(kindLabel) matched\(info.manual == true ? " by hand" : ""): \(c.title)\(c.year.map { " (\($0))" } ?? "")"
        }
        switch info.status {
        case "not_found": return info.manual == true ? "Marked as no match." : "No \(kindLabel.lowercased()) match found."
        case "error": return "The last lookup failed."
        case "pending": return singleFile ? "Not matched. Files that aren't episodes are only matched by hand." : "Not matched yet (lookup pending)."
        case "matched": return "Matched; fetching details…"
        default: return info.status ?? ""
        }
    }

    var hint: String {
        singleFile ? "Choosing a result applies to this file only."
            : "Choosing a result applies to every file in this library with the same title."
    }

    var resetLabel: String { singleFile ? "Clear match" : "Reset to automatic" }

    func isCurrent(_ hit: MatchHit) -> Bool { info?.current?.tmdbId == hit.tmdbId && matchedKind == searchKind }

    func switchKind(_ kind: String) {
        guard kind != searchKind else { return }
        searchKind = kind
        search()
    }

    func search() {
        let q = query.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty, info != nil else { return }
        searchTask?.cancel()
        let kind = searchKind
        var params = [("kind", kind), ("query", q)]
        let y = year.trimmingCharacters(in: .whitespaces)
        if !y.isEmpty { params.append(("year", y)) }
        searching = true
        searchTask = Task {
            do {
                let got: [MatchHit] = try await client.get("/api/media/match/search", query: params)
                guard !Task.isCancelled, kind == searchKind else { return }
                hits = got
            } catch {
                guard !Task.isCancelled else { return }
                message = ("Search failed: \(errorText(error))", true)
                hits = []
            }
            searching = false
        }
    }

    func choose(_ hit: MatchHit) async {
        await change("Matched to \(hit.title). Fetching details…") {
            try await self.send("PUT", "/api/media/match", self.target.body(["tmdb_id": hit.tmdbId, "kind": self.searchKind]))
        }
    }

    func markNoMatch() async {
        await change("Marked as no match.") {
            try await self.send("PUT", "/api/media/match", self.target.body(["tmdb_id": 0, "kind": ""]))
        }
    }

    func reset() async {
        let single = singleFile
        await change(single ? "Match cleared." : "Reset. Matching automatically…") {
            try await self.client.fetch("DELETE", "/api/media/match", query: self.target.query)
        }
    }

    /// The posters to pick from: the title's own, then other search results'.
    var ownPosters: [PosterPick] {
        (posters?.posters ?? []).map {
            PosterPick(path: $0.path, url: $0.url, label: ($0.language ?? "").isEmpty ? "No text" : $0.language!.uppercased())
        }
    }

    var otherPosters: [PosterPick] {
        guard let current = info?.current else { return [] }
        return (hits ?? []).compactMap { h in
            guard let path = h.posterPath, !path.isEmpty, !(h.tmdbId == current.tmdbId && searchKind == matchedKind) else { return nil }
            return PosterPick(path: path, url: h.poster ?? "", label: h.title + (h.year.map { " (\($0))" } ?? ""))
        }
    }

    func choosePoster(_ path: String) async {
        postersFor = 0 // asked again, for the new current one
        await change(path.isEmpty ? "Poster reset to the default." : "Poster changed.") {
            try await self.send("PUT", "/api/media/match/poster", self.target.body(["poster": path]))
        }
    }

    /// The file's episode, for the episode section (none when fixing the whole title).
    var episode: MatchEpisodeInfo? { target.wholeTitle ? nil : info?.episode }

    /// The TMDB show the file's matched to, for the episode section.
    var show: MatchTitle? { info?.current?.kind == "show" ? info?.current : nil }

    func chooseSeason(_ number: Int) {
        guard let showId = show?.tmdbId else { return }
        season = number
        pick = nil
        episodes = nil
        episodesError = nil
        Task {
            do {
                let got: [TmdbEpisode] = try await client.get("/api/media/match/episodes",
                                                              query: [("show", String(showId)), ("season", String(number))])
                guard season == number else { return }
                episodes = got
            } catch {
                guard season == number else { return }
                episodesError = "Could not load episodes: \(errorText(error))"
                episodes = []
            }
        }
    }

    func applyEpisode(wholeSeason: Bool) async {
        guard let (s, e) = pick else { return }
        await change(wholeSeason ? "Season mapped. Fetching episode details…" : "Episode set for this file.") {
            try await self.send("PUT", "/api/media/match/episode",
                                self.target.body(["season": s, "episode": e, "whole_season": wholeSeason]))
        }
    }

    func clearEpisode(wholeSeason: Bool) async {
        var query = [("path", ServerClient.encodePath(target.path))]
        if wholeSeason { query.append(("whole_season", "1")) }
        await change("Episode mapping cleared.") {
            try await self.client.fetch("DELETE", "/api/media/match/episode", query: query)
        }
    }

    /// "Use for all of season 1 (→ season 2, episode numbers +4)"
    var wholeSeasonLabel: String {
        guard let ep = episode, let (s, e) = pick else { return "" }
        let shift = e - ep.fileEpisode
        let shiftText = shift == 0 ? "same episode numbers" : "episode numbers \(shift > 0 ? "+" : "")\(shift)"
        return "Use for all of season \(ep.fileSeason) (→ season \(s), \(shiftText))"
    }

    private func send(_ method: String, _ path: String, _ body: [String: Any]) async throws -> MatchInfo {
        try await client.sendJSON(method, path, body: body)
    }

    /// Sends a change; its answer is the new match info.
    private func change(_ done: String, _ request: @escaping () async throws -> MatchInfo) async {
        do {
            let got = try await request()
            changed = true
            message = (done, false)
            apply(got)
        } catch {
            message = ("Could not save: \(errorText(error))", true)
        }
    }

    /// Takes new match info: loads the posters and seasons it needs.
    private func apply(_ got: MatchInfo) {
        info = got
        pick = nil
        if let current = got.current {
            if postersFor != current.tmdbId {
                postersFor = current.tmdbId
                posters = nil
                postersError = nil
                let id = current.tmdbId
                Task {
                    do {
                        let list: PosterList = try await client.get("/api/media/match/posters", query: target.query)
                        guard postersFor == id else { return }
                        posters = list
                    } catch {
                        guard postersFor == id else { return }
                        postersError = "Posters unavailable: \(errorText(error))"
                    }
                }
            }
        } else {
            posters = nil
            postersFor = 0
        }
        guard let ep = episode, let showId = show?.tmdbId else {
            seasons = nil; seasonsFor = 0; season = nil; episodes = nil
            return
        }
        if seasonsFor != showId {
            seasonsFor = showId
            seasons = nil
            Task {
                do {
                    let list: [TmdbSeason] = try await client.get("/api/media/match/seasons", query: [("show", String(showId))])
                    guard seasonsFor == showId else { return }
                    seasons = list
                    if let first = list.first(where: { $0.number == ep.season }) ?? list.first { chooseSeason(first.number) }
                } catch {
                    guard seasonsFor == showId else { return }
                    episodesError = "Could not load seasons: \(errorText(error))"
                    seasons = []
                }
            }
        } else if season != ep.season, seasons?.contains(where: { $0.number == ep.season }) == true {
            chooseSeason(ep.season)
        }
    }
}

/// "S01E05"
func episodeCode(_ season: Int, _ episode: Int) -> String { String(format: "S%02dE%02d", season, episode) }

/// "This file is S01E05, shown as S01E05 “Pilot” (from the file name)."
func episodeSummary(_ ep: MatchEpisodeInfo) -> String {
    let source = ["file": "from the file name", "season": "from a season mapping", "manual": "set for this file"][ep.source] ?? ep.source
    let shownAs = episodeCode(ep.season, ep.episode) + ((ep.title ?? "").isEmpty ? "" : " “\(ep.title!)”")
    return "This file is \(episodeCode(ep.fileSeason, ep.fileEpisode)), shown as \(shownAs) (\(source))."
}

/// "Season 2 – The Return (10)", "Specials (3)"
func seasonLabel(_ s: TmdbSeason) -> String {
    let base = s.number == 0 ? "Specials" : "Season \(s.number)"
    let name = s.name.flatMap { n in n.isEmpty || n.range(of: #"^(Season \d+|Specials)$"#, options: .regularExpression) != nil ? nil : n }
    return base + (name.map { " – \($0)" } ?? "") + " (\(s.episodeCount ?? 0))"
}

/// The file a title page's Fix goes through: a movie's first file, else a show's first episode's.
func titleFixFile(_ title: TitleDetail) -> LibraryFile? {
    title.files?.first ?? (title.seasons ?? []).flatMap { $0.episodes ?? [] }.lazy.compactMap { $0.files?.first }.first
}

/// The Fix sheet (admins): the current match, its episode for an episode file, a TMDB search whose
/// results match it when tapped, and its poster. As Android's PhoneMatchSheet.
struct MatchSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var m: MatchModel
    @State private var confirmNone = false
    let onClose: (Bool) -> Void

    init(client: ServerClient, target: MatchTarget, onClose: @escaping (Bool) -> Void) {
        _m = State(initialValue: MatchModel(client: client, target: target))
        self.onClose = onClose
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text(m.target.name).font(.caption).foregroundStyle(Palette.muted)
                    if let err = m.loadError { Text(err).foregroundStyle(Palette.error) }
                    if m.info == nil {
                        if m.loadError == nil { ProgressView() }
                    } else {
                        current
                        MessageLine(message: m.message)
                        if m.episode != nil { episodeSection }
                        searchSection
                        results
                        if m.info?.current != nil { posterSection }
                    }
                }
                .padding(16)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .background(Palette.background)
            .navigationTitle("Fix match")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } }
            }
        }
        .task { await m.load() }
        .onDisappear { onClose(m.changed) }
        .alert("Mark as no match?", isPresented: $confirmNone) {
            Button("Cancel", role: .cancel) {}
            Button("Mark as no match") { Task { await m.markNoMatch() } }
        } message: {
            Text("Automatic matching will leave it alone.")
        }
    }

    private var current: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 12) {
                if let poster = m.info?.current?.poster {
                    RemoteImage(relative: poster).frame(width: 60, height: 90).clipShape(RoundedRectangle(cornerRadius: 4))
                }
                VStack(alignment: .leading, spacing: 4) {
                    Text(m.statusText).font(.headline)
                    Text(m.hint).font(.caption).foregroundStyle(Palette.muted)
                }
            }
            HStack {
                Button("Mark as no match") { confirmNone = true }.buttonStyle(PhoneSecondaryButtonStyle())
                if m.info?.manual == true {
                    Button(m.resetLabel) { Task { await m.reset() } }.buttonStyle(PhoneSecondaryButtonStyle())
                }
            }
        }
    }

    private var searchSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            SectionTitle(text: "Search TMDB")
            if m.kinds.count > 1 {
                Picker("Search for", selection: Binding(get: { m.searchKind }, set: { m.switchKind($0) })) {
                    ForEach(m.kinds, id: \.self) { Text($0 == "tv" ? "TV shows" : "Movies").tag($0) }
                }
                .pickerStyle(.segmented)
            }
            HStack {
                TextField("Title", text: $m.query).textFieldStyle(.roundedBorder).submitLabel(.search)
                    .onSubmit { m.search() }
                TextField("Year", text: Binding(get: { m.year }, set: { m.year = String($0.filter(\.isNumber).prefix(4)) }))
                    .textFieldStyle(.roundedBorder).keyboardType(.numberPad).frame(width: 80)
            }
            Button("Search") { m.search() }.buttonStyle(PhoneButtonStyle())
                .disabled(m.query.trimmingCharacters(in: .whitespaces).isEmpty)
        }
    }

    @ViewBuilder private var results: some View {
        if m.searching {
            Text("Searching…").font(.subheadline)
        } else if let hits = m.hits {
            if hits.isEmpty {
                Text("No results" + (m.year.isEmpty ? "." : " (try without the year).")).font(.subheadline)
            }
            ForEach(hits) { hit in
                let isCurrent = m.isCurrent(hit)
                Button { Task { await m.choose(hit) } } label: {
                    HStack(alignment: .top, spacing: 12) {
                        RemoteImage(relative: hit.poster).frame(width: 46, height: 69)
                            .background(Palette.raised).clipShape(RoundedRectangle(cornerRadius: 4))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(hit.title + (hit.year.map { " (\($0))" } ?? "") + (isCurrent ? " — current" : ""))
                                .font(.subheadline.weight(.semibold))
                            Text((hit.overview ?? "").isEmpty ? "No synopsis." : hit.overview!)
                                .font(.caption).foregroundStyle(Palette.muted).lineLimit(3)
                        }
                        Spacer(minLength: 0)
                    }
                    .padding(8)
                    .background(Palette.raised.opacity(0.4), in: RoundedRectangle(cornerRadius: 8))
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(isCurrent ? Palette.accent : .clear, lineWidth: 2))
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var posterSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                SectionTitle(text: "Poster")
                if let list = m.posters {
                    Text(list.manual == true ? "Picked by hand." : "TMDB's default.").font(.caption).foregroundStyle(Palette.muted)
                    if list.manual == true { Button("Use default") { Task { await m.choosePoster("") } } }
                }
            }
            if let err = m.postersError { Text(err).font(.caption).foregroundStyle(Palette.error) }
            if let list = m.posters {
                let own = m.ownPosters
                Text("\(m.info?.current?.title ?? "")'s posters (\(own.count))").font(.caption)
                if own.isEmpty {
                    Text("TMDB has no posters for this title.").font(.caption).foregroundStyle(Palette.muted)
                } else {
                    posterStrip(own, current: list.current ?? "")
                }
                let others = m.otherPosters
                if !others.isEmpty {
                    Text("From other search results").font(.caption)
                    posterStrip(others, current: list.current ?? "")
                }
            } else if m.postersError == nil {
                Text("Loading posters…").font(.caption)
            }
        }
    }

    private func posterStrip(_ posters: [PosterPick], current: String) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(posters) { p in
                    Button { Task { await m.choosePoster(p.path) } } label: {
                        VStack(spacing: 2) {
                            RemoteImage(relative: p.url).frame(width: 80, height: 120)
                                .clipShape(RoundedRectangle(cornerRadius: 4))
                                .overlay(RoundedRectangle(cornerRadius: 4).stroke(p.path == current ? Palette.accent : .clear, lineWidth: 3))
                            Text(p.label).font(.caption2).lineLimit(2).frame(width: 80)
                        }
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    @ViewBuilder private var episodeSection: some View {
        if let ep = m.episode {
            VStack(alignment: .leading, spacing: 8) {
                SectionTitle(text: "Episode")
                Text(episodeSummary(ep)).font(.subheadline)
                if m.show == nil {
                    Text("Match the show first to pick a different episode.").font(.caption).foregroundStyle(Palette.muted)
                } else {
                    if ep.source == "manual" {
                        Button("Clear this file’s fix") { Task { await m.clearEpisode(wholeSeason: false) } }.buttonStyle(PhoneSecondaryButtonStyle())
                    }
                    if ep.source == "season" {
                        Button("Clear season \(ep.fileSeason) mapping") { Task { await m.clearEpisode(wholeSeason: true) } }
                            .buttonStyle(PhoneSecondaryButtonStyle())
                    }
                    if let seasons = m.seasons, !seasons.isEmpty {
                        Picker("Season", selection: Binding(get: { m.season ?? seasons[0].number }, set: { m.chooseSeason($0) })) {
                            ForEach(seasons) { Text(seasonLabel($0)).tag($0.number) }
                        }
                        .pickerStyle(.menu)
                    }
                    if let err = m.episodesError { Text(err).font(.caption).foregroundStyle(Palette.error) }
                    episodeList(ep)
                    if let (s, e) = m.pick {
                        Button("Use \(episodeCode(s, e)) for this file") { Task { await m.applyEpisode(wholeSeason: false) } }
                            .buttonStyle(PhoneButtonStyle())
                        Button(m.wholeSeasonLabel) { Task { await m.applyEpisode(wholeSeason: true) } }
                            .buttonStyle(PhoneSecondaryButtonStyle())
                    }
                }
            }
        }
    }

    @ViewBuilder private func episodeList(_ ep: MatchEpisodeInfo) -> some View {
        if let season = m.season {
            if let episodes = m.episodes {
                if episodes.isEmpty { Text("No episodes in this season.").font(.caption) }
                ForEach(episodes) { e in
                    let isCurrent = season == ep.season && e.number == ep.episode
                    let picked = m.pick.map { $0 == (season, e.number) } ?? false
                    Button { m.pick = (season, e.number) } label: {
                        HStack(spacing: 12) {
                            Text(episodeCode(season, e.number)).fontWeight(isCurrent ? .bold : .regular)
                            Text(e.name ?? "").lineLimit(1)
                            Spacer(minLength: 0)
                            Text(e.airDate ?? "").font(.caption).foregroundStyle(Palette.muted)
                        }
                        .padding(.horizontal, 8).padding(.vertical, 10)
                        .background(picked ? Palette.accent.opacity(0.25) : (isCurrent ? Palette.raised : .clear),
                                    in: RoundedRectangle(cornerRadius: 4))
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            } else {
                Text("Loading episodes…").font(.caption)
            }
        }
    }
}
