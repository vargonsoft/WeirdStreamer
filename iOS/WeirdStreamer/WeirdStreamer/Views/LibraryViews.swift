import SwiftUI

/// Library view (Android's PhoneLibraryScreen): a movie or TV library as posters, with Continue
/// Watching and Recently Added above and the unmatched files at the end, and an A–Z strip.
struct LibraryScreen: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if let listing = model.libraryListing {
            let titles = listing.titles ?? []
            let unmatched = listing.unmatched ?? []
            if titles.isEmpty && unmatched.isEmpty {
                PhoneMessage(text: "Nothing matched yet. Titles appear here as they are looked up.")
            } else {
                let names = titles.map(\.title) + unmatched.map(\.name)
                let groups = titles.map { _ in 0 } + unmatched.map { _ in 1 }
                let letters = alphabetIndex(names, groups: groups, skipArticles: true)
                ScrollViewReader { proxy in
                    HStack(spacing: 0) {
                        ScrollView {
                            VStack(alignment: .leading, spacing: 14) {
                                if model.hasTopRows { ContinueRows() }
                                LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                                    ForEach(Array(titles.enumerated()), id: \.element.id) { i, t in
                                        let showPart = t.kind == "show" && (t.watched ?? 0) > 0 && !t.allWatched
                                        PosterTile(
                                            title: t.title,
                                            subtitle: [t.year.map(String.init),
                                                       showPart ? "\(t.watched ?? 0) of \(t.fileCount ?? 0) watched" : nil]
                                                .compactMap { $0 }.joined(separator: " · "),
                                            poster: t.poster,
                                            placeholder: t.kind == "show" ? "tv" : "film",
                                            progress: t.kind == "movie" ? (t.progress ?? 0)
                                                : (showPart ? Double(t.watched ?? 0) / Double(max(t.fileCount ?? 1, 1)) : 0),
                                            watched: t.allWatched
                                        ) {
                                            model.openLibraryTitle(kind: t.kind, tmdbId: t.tmdbId)
                                        }
                                        .id(i)
                                    }
                                }
                                if !unmatched.isEmpty {
                                    Text("Unmatched (\(unmatched.count))").font(.headline).padding(.top, 16)
                                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                                        ForEach(Array(unmatched.enumerated()), id: \.element.path) { i, f in
                                            let playable = f.count == nil
                                            PosterTile(title: f.name,
                                                       subtitle: playable ? formatBytes(f.size) : "\(f.count ?? 0) files",
                                                       poster: nil,
                                                       placeholder: listing.libraryType == "tv" ? "tv" : "theatermasks",
                                                       progress: f.playback?.fraction(fallbackDuration: f.duration) ?? 0,
                                                       watched: f.playback?.watched == true) {
                                                if playable { model.playLibraryFile(f) }
                                            }
                                            .contextMenu {
                                                if model.canFixMatches {
                                                    Button("Fix match") { model.openMatch(path: f.path, isDir: false, name: f.name) }
                                                }
                                            }
                                            .id(titles.count + i)
                                        }
                                    }
                                }
                            }
                            .padding(12)
                        }
                        .refreshable { model.loadLibrary() }
                        if letters.count > 1 {
                            IndexBar(letters: letters) { entry in
                                withAnimation { proxy.scrollTo(entry.position, anchor: .top) }
                            }
                            .padding(.vertical, 8)
                        }
                    }
                }
            }
        } else if let error = model.libraryError {
            PhoneMessage(text: "Error: \(error)")
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

/// A poster (2:3) with its progress, then the title and a line under it.
struct PosterTile: View {
    let title: String
    let subtitle: String?
    let poster: String?
    let placeholder: String
    var progress: Double = 0
    var watched = false
    var dimmed = false   // a collection's film the library doesn't have: opens nothing
    var collage: [String]? = nil   // a made collection's: up to four posters instead of one
    let action: () -> Void

    var body: some View {
        Button(action: { if !dimmed { action() } }) {
            VStack(alignment: .leading, spacing: 0) {
                ZStack {
                    Palette.raised
                    Image(systemName: placeholder).font(.system(size: 34)).foregroundStyle(Palette.muted)
                    if let collage { Collage(posters: collage) } else { RemoteImage(relative: poster) }
                    WatchProgressOverlay(fraction: progress, watched: watched)
                }
                .aspectRatio(2.0 / 3.0, contentMode: .fit)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                Text(title).font(.subheadline.weight(.medium)).lineLimit(2).multilineTextAlignment(.leading)
                    .padding(.top, 6)
                if let subtitle, !subtitle.isEmpty {
                    Text(subtitle).font(.caption2).foregroundStyle(Palette.muted)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .opacity(dimmed ? 0.4 : 1)
        }
        .buttonStyle(.plain)
        .disabled(dimmed)
    }
}

/// "2 of 6 films", or "6 films" without a count.
func collectionCount(_ count: Int, total: Int? = nil) -> String {
    let n = total ?? count
    let films = "\(n) film\(n == 1 ? "" : "s")"
    return total == nil ? films : "\(count) of \(films)"
}

/// Collection view (Android's PhoneCollectionsScreen): a movie library's TMDB collections (film
/// series) as posters, by name.
struct CollectionsScreen: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if let listing = model.libraryListing {
            let collections = listing.collections ?? []
            if collections.isEmpty {
                PhoneMessage(text: "No movies in collections yet. Collections appear here as movies are looked up.")
            } else {
                ScrollView {
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                        ForEach(collections) { c in
                            PosterTile(title: c.name, subtitle: collectionCount(c.count ?? 0, total: c.total ?? 0),
                                       poster: c.poster, placeholder: "film") {
                                model.openLibraryCollection(id: c.id)
                            }
                        }
                    }
                    .padding(12)
                }
                .refreshable { model.loadLibrary() }
            }
        } else if let error = model.libraryError {
            PhoneMessage(text: "Error: \(error)")
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

/// A collection's page (Android's PhoneCollectionScreen): its name, how many of its films the
/// library has and its overview, then every film by release date; missing ones dimmed.
struct CollectionScreen: View {
    @Environment(AppModel.self) private var model
    let collection: CollectionDetail

    var body: some View {
        let have = collection.parts.filter { $0.missing != true }.count
        ScrollView {
            VStack(alignment: .leading, spacing: 4) {
                Text(collection.name).font(.title2.weight(.semibold))
                Text("\(have) of \(collectionCount(collection.parts.count)) in this library")
                    .font(.subheadline).foregroundStyle(Palette.muted)
                if let overview = collection.overview, !overview.isEmpty {
                    Text(overview).font(.subheadline)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12).padding(.top, 12)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                ForEach(collection.parts) { t in
                    let missing = t.missing == true
                    PosterTile(title: t.title,
                               subtitle: missing ? [t.year.map(String.init), "Not in this library"].compactMap { $0 }.joined(separator: " · ")
                                   : t.year.map(String.init),
                               poster: t.poster, placeholder: "film",
                               progress: t.progress ?? 0, watched: t.allWatched, dimmed: missing) {
                        model.openLibraryTitle(kind: t.kind, tmdbId: t.tmdbId)
                    }
                }
            }
            .padding(12)
        }
    }
}

/// A title page (Android's PhoneTitleScreen): the backdrop with the poster and title, the
/// details and cast, then a movie's files or a show's seasons and episodes.
struct TitleScreen: View {
    @Environment(AppModel.self) private var model
    let title: TitleDetail

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 0) {
                TitleHeader(title: title)
                TitleDetails(title: title)
                let isMovie = title.kind == "movie" || !(title.files ?? []).isEmpty
                if isMovie, (title.files ?? []).count >= 2 {
                    GroupWatchedButton(files: title.files ?? [], label: "Mark All Watched")
                        .padding(.horizontal, 16).padding(.vertical, 4)
                }
                if let cast = title.cast, !cast.isEmpty { CastRow(cast: cast) }
                if isMovie {
                    let files = title.files ?? []
                    SectionHeader(text: files.count > 1 ? "Files" : "File")
                    let label = title.title + (title.year.map { " (\($0))" } ?? "")
                    ForEach(files, id: \.path) { LibraryFileRow(file: $0, titleLabel: label) }
                } else {
                    if let seasons = title.seasons, !seasons.isEmpty {
                        SeasonsSection(title: title, seasons: seasons)
                    } else {
                        PhoneMessage(text: "No episodes found.")
                    }
                    if let other = title.otherFiles, !other.isEmpty {
                        SectionHeader(text: "Other files")
                        ForEach(other, id: \.path) { LibraryFileRow(file: $0, titleLabel: title.title) }
                    }
                }
            }
            .padding(.bottom, 24)
        }
        .refreshable { model.reloadTitle() }
    }
}

struct TitleHeader: View {
    @Environment(AppModel.self) private var model
    let title: TitleDetail
    @State private var addingToCollection = false

    var body: some View {
        HStack(alignment: .bottom, spacing: 16) {
            ZStack {
                Palette.raised
                Image(systemName: title.kind == "show" ? "tv" : "film").font(.system(size: 34))
                RemoteImage(relative: title.poster)
            }
            .frame(width: 112, height: 168)
            .clipShape(RoundedRectangle(cornerRadius: 8))
            VStack(alignment: .leading, spacing: 4) {
                Text(title.title).font(.title2.weight(.semibold))
                let facts = [title.year.map(String.init), title.runtime.flatMap { $0 > 0 ? "\($0) min" : nil }].compactMap { $0 }
                if !facts.isEmpty {
                    Text(facts.joined(separator: " · ")).font(.subheadline).foregroundStyle(Palette.muted)
                }
                if let genres = title.genres, !genres.isEmpty {
                    Text(genres.joined(separator: ", ")).font(.caption).foregroundStyle(Palette.muted)
                }
                // Fix the whole title's match (through its first file, which every file of it follows)
                if model.canFixMatches, let f = titleFixFile(title) {
                    Button("Fix match") { model.openMatch(path: f.path, isDir: false, name: title.title, wholeTitle: true) }
                        .buttonStyle(PhoneSecondaryButtonStyle()).padding(.top, 8)
                }
                // Admins add a movie library's title to their collections
                if model.canEditCollections {
                    Button("Add to collection") { addingToCollection = true }
                        .buttonStyle(PhoneSecondaryButtonStyle()).padding(.top, 4)
                }
            }
            Spacer(minLength: 0)
        }
        .padding(16)
        .sheet(isPresented: $addingToCollection) {
            AddToCollectionSheet(title: CollectionTitle(kind: title.kind, tmdbId: title.tmdbId), name: title.title)
        }
        .background {
            if title.backdrop != nil {
                ZStack {
                    RemoteImage(relative: title.backdrop, opacity: 0.35)
                    LinearGradient(colors: [.clear, Palette.panel], startPoint: .top, endPoint: .bottom)
                }
                .clipped()
            }
        }
    }
}

struct TitleDetails: View {
    let title: TitleDetail

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let tagline = title.tagline, !tagline.isEmpty {
                Text(tagline).font(.body.italic())
            }
            if let overview = title.overview, !overview.isEmpty {
                Text(overview).font(.subheadline)
            }
            let people = title.kind == "show" ? title.creators : title.directors
            if let people, !people.isEmpty {
                CreditLine(label: title.kind == "show" ? "Created by" : (people.count > 1 ? "Directors" : "Director"), people: people)
            }
            if title.kind == "movie", let writers = title.writers, !writers.isEmpty {
                CreditLine(label: "Writers", people: writers)
            }
        }
        .padding(.horizontal, 16)
    }
}

struct CreditLine: View {
    let label: String
    let people: [CreditInfo]
    var body: some View {
        (Text(label + " ").bold() + Text(people.map(\.name).joined(separator: ", "))).font(.subheadline)
    }
}

struct CastRow: View {
    let cast: [CreditInfo]
    @State private var opened: CreditInfo?   // a tap opens their window

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(alignment: .top, spacing: 12) {
                ForEach(Array(cast.enumerated()), id: \.offset) { _, person in
                    Button { if person.tmdbId != nil { opened = person } } label: {
                    VStack(spacing: 2) {
                        ZStack {
                            Palette.raised
                            Text(String(person.name.prefix(1))).font(.title2)
                            RemoteImage(relative: person.profile)
                        }
                        .frame(width: 64, height: 64)
                        .clipShape(Circle())
                        Text(person.name).font(.caption.weight(.medium)).multilineTextAlignment(.center).padding(.top, 6)
                        if let character = person.character, !character.isEmpty {
                            Text(character).font(.caption2).foregroundStyle(Palette.muted).multilineTextAlignment(.center)
                        }
                    }
                    .frame(width: 84)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 16).padding(.vertical, 12)
        }
        .sheet(item: Binding(get: { opened.map(PersonRef.init) }, set: { if $0 == nil { opened = nil } })) { ref in
            PersonSheet(cast: ref.cast)
        }
    }
}

/// A cast member to open in a sheet (Identifiable by their TMDB ID).
struct PersonRef: Identifiable {
    let cast: CreditInfo
    var id: Int { cast.tmdbId ?? 0 }
}

struct SectionHeader: View {
    let text: String
    var body: some View {
        Text(text).font(.headline).padding(.horizontal, 16).padding(.top, 12).padding(.bottom, 8)
    }
}

struct PhoneMessage: View {
    let text: String
    var body: some View {
        Text(text).font(.subheadline).foregroundStyle(Palette.muted).multilineTextAlignment(.center)
            .padding(32).frame(maxWidth: .infinity)
    }
}

/// A movie's file: the thumbnail (tap to play) with the match, full name, size, time left, and
/// Play (or Resume with Play from start beside it) and the watched check.
struct LibraryFileRow: View {
    @Environment(AppModel.self) private var model
    let file: LibraryFile
    let titleLabel: String

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Button { model.playLibraryFile(file, title: titleLabel) } label: {
                ZStack {
                    Palette.raised
                    Image(systemName: "film").font(.system(size: 28))
                    RemoteImage(relative: file.thumbnailUrl)
                    WatchProgressOverlay(fraction: file.playback?.fraction(fallbackDuration: file.duration) ?? 0,
                                         watched: file.playback?.watched == true)
                }
                .frame(width: 140, height: 78.75)
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
            .buttonStyle(.plain)
            VStack(alignment: .leading, spacing: 4) {
                if let match = file.match { Text(match.label).font(.subheadline.weight(.medium)) }
                Text(file.name).font(.caption)
                let size = formatBytes(file.size)
                if !size.isEmpty { Text(size).font(.caption2).foregroundStyle(Palette.muted) }
                if let left = timeLeftText(file.playback, file.duration) {
                    Text(left).font(.caption2).foregroundStyle(Palette.accent)
                }
                HStack(spacing: 4) {
                    let resumeAt = file.playback?.resumeAt ?? 0
                    if resumeAt > 0 {
                        Button { model.playLibraryFile(file, title: titleLabel) } label: {
                            Label("Resume \(clockText(resumeAt))", systemImage: "play.fill")
                        }
                        .buttonStyle(TonalButtonStyle())
                        IconButton(system: "arrow.counterclockwise", label: "Play from start") {
                            model.playLibraryFile(file, title: titleLabel, fromStart: true)
                        }
                    } else {
                        Button { model.playLibraryFile(file, title: titleLabel) } label: {
                            Image(systemName: "play.fill").frame(width: 24, height: 24)
                        }
                        .buttonStyle(TonalButtonStyle())
                        .accessibilityLabel("Play")
                    }
                    WatchedButton(files: [file])
                    DownloadButton(request: DownloadRequest(path: file.path, name: file.name,
                                                            title: file.match?.label ?? titleLabel,
                                                            thumb: file.thumbnailUrl, size: file.size ?? 0))
                    if model.canFixMatches {
                        IconButton(system: "pencil", label: "Fix match") {
                            model.openMatch(path: file.path, isDir: false, name: file.name)
                        }
                    }
                }
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 16).padding(.vertical, 6)
    }
}

struct IconButton: View {
    let system: String
    let label: String
    var tint: Color = Palette.text
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: system).font(.system(size: 20)).foregroundStyle(tint).frame(width: 44, height: 44)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }
}

/// A check that marks files watched, or unwatched when they all are.
struct WatchedButton: View {
    @Environment(AppModel.self) private var model
    let files: [LibraryFile]

    var body: some View {
        let watched = !files.isEmpty && files.allSatisfy { $0.playback?.watched == true }
        IconButton(system: watched ? "checkmark.circle.fill" : "checkmark.circle",
                   label: watched ? "Mark unwatched" : "Mark watched",
                   tint: watched ? Palette.accent : Palette.muted) {
            model.setWatched(files.map(\.path), !watched)
        }
    }
}

/// Marks a season's (or the whole show's) files watched, or unwatched when they all are.
struct GroupWatchedButton: View {
    @Environment(AppModel.self) private var model
    let files: [LibraryFile]
    let label: String
    var season: LibrarySeason? = nil

    var body: some View {
        if !files.isEmpty {
            let watched = files.allSatisfy { $0.playback?.watched == true }
            Button { model.setWatched(files.map(\.path), !watched) } label: {
                HStack(spacing: 6) {
                    Image(systemName: "checkmark.circle.fill").opacity(watched ? 1 : 0.35)
                    if let season { SeasonLabel(season: season, label: label) } else { Text(label).lineLimit(1) }
                }
                .frame(maxHeight: .infinity)
            }
            .buttonStyle(TonalButtonStyle())
            .accessibilityLabel(watched ? "Mark unwatched" : "Mark watched")
        }
    }
}

/// A season's label, with "Season N" small above it when it has a name of its own.
struct SeasonLabel: View {
    let season: LibrarySeason
    let label: String

    var body: some View {
        let number = "Season \(season.season)"
        if season.season > 0 && season.name.caseInsensitiveCompare(number) != .orderedSame {
            VStack(alignment: .leading, spacing: 0) {
                Text(number).font(.caption2)
                Text(label).lineLimit(1)
            }
        } else {
            Text(label).lineLimit(1)
        }
    }
}

/// A season's progress as a ring: dashed while none of it is watched, then an arc for the share watched.
struct SeasonProgressRing: View {
    let fraction: Double
    var size: CGFloat = 16

    var body: some View {
        ZStack {
            if fraction <= 0 {
                Circle().stroke(style: StrokeStyle(lineWidth: size * 0.14, dash: [size * 0.26, size * 0.11]))
                    .opacity(0.45)
            } else {
                Circle().stroke(lineWidth: size * 0.14).opacity(0.3)
                Circle().trim(from: 0, to: min(fraction, 1))
                    .stroke(style: StrokeStyle(lineWidth: size * 0.14, lineCap: .round))
                    .rotationEffect(.degrees(-90))
            }
        }
        .frame(width: size, height: size)
    }
}

/// A show's seasons: the watched buttons and Auto Skip, the season chips with their rings, the
/// episode count, and the chosen season's episodes. Opens at the season where the show picks up.
struct SeasonsSection: View {
    @Environment(AppModel.self) private var model
    let title: TitleDetail
    let seasons: [LibrarySeason]

    @State private var selected: Int?

    var body: some View {
        let pick = upNext(seasons, justPlayed: model.lastPlayedFile)
        let index = min(max(selected ?? pick?.season ?? 0, 0), seasons.count - 1)
        let season = seasons[index]
        let episodes = season.episodes ?? []
        VStack(alignment: .leading, spacing: 0) {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    GroupWatchedButton(files: episodes.flatMap { $0.files ?? [] }, label: season.name, season: season)
                    GroupWatchedButton(files: seasons.flatMap { $0.episodes ?? [] }.flatMap { $0.files ?? [] }, label: "Whole show")
                    Button { model.setAutoSkip(tmdbId: title.tmdbId, on: !(title.autoSkip ?? false)) } label: {
                        HStack(spacing: 6) {
                            Image(systemName: title.autoSkip == true ? "checkmark.square.fill" : "square")
                                .foregroundStyle(title.autoSkip == true ? Palette.accent : Palette.muted)
                                .font(.system(size: 20))
                            Text("Auto Skip")
                        }
                    }
                    .buttonStyle(.plain)
                    .padding(.trailing, 8)
                }
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 16).padding(.top, 8)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(Array(seasons.enumerated()), id: \.offset) { i, s in
                        let eps = s.episodes ?? []
                        let watched = eps.filter { e in (e.files ?? []).contains { $0.playback?.watched == true } }.count
                        Button { selected = i } label: {
                            HStack(spacing: 8) {
                                SeasonLabel(season: s, label: s.name)
                                SeasonProgressRing(fraction: eps.isEmpty ? 0 : Double(watched) / Double(eps.count))
                            }
                            .padding(.horizontal, 12).padding(.vertical, 6)
                            .frame(maxHeight: .infinity)
                            .foregroundStyle(i == index ? Palette.onAccent : Palette.text)
                            .background(i == index ? Palette.accent : Color.clear, in: RoundedRectangle(cornerRadius: 5))
                            .overlay(RoundedRectangle(cornerRadius: 5).stroke(i == index ? Color.clear : Palette.muted, lineWidth: 1))
                        }
                        .buttonStyle(.plain)
                    }
                }
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, 16).padding(.vertical, 8)
            }
            Text(episodeCountText(episodes.count)).font(.subheadline).foregroundStyle(Palette.muted)
                .padding(.horizontal, 16).padding(.bottom, 8)
            ScrollViewReader { proxy in
                VStack(spacing: 0) {
                    ForEach(Array(episodes.enumerated()), id: \.offset) { i, episode in
                        EpisodeRow(episode: episode, showTitle: title.title).id("ep-\(i)")
                    }
                }
                .onAppear {
                    if selected == nil, let pick, pick.season == index {
                        proxy.scrollTo("ep-\(pick.episode)", anchor: .center)
                    }
                }
            }
        }
    }
}

/// An episode (Android's PhoneEpisodeRow): its thumbnail (tap to play or resume) with Play from
/// start and the watched check beneath, then its file names, "S01E02 · Title", date and length,
/// the start of its description and where it resumes. The text opens in full.
struct EpisodeRow: View {
    @Environment(AppModel.self) private var model
    let episode: LibraryEpisode
    let showTitle: String

    @State private var showDetails = false

    var body: some View {
        if let first = episode.files?.first {
            let files = episode.files ?? []
            let label = String(format: "%@ · S%02dE%02d · %@", showTitle, episode.season, episode.episode, episode.title)
            HStack(alignment: .top, spacing: 12) {
                VStack(spacing: 0) {
                    Button { model.playLibraryFile(first, title: label) } label: {
                        ZStack {
                            Palette.raised
                            Image(systemName: "play.fill").font(.system(size: 24))
                            RemoteImage(relative: episode.still ?? first.thumbnailUrl)
                            WatchProgressOverlay(fraction: first.playback?.fraction(fallbackDuration: first.duration) ?? 0,
                                                 watched: first.playback?.watched == true, badgeSize: 18)
                        }
                        .frame(width: 144, height: 81)
                        .clipShape(RoundedRectangle(cornerRadius: 6))
                    }
                    .buttonStyle(.plain)
                    HStack {
                        Spacer()
                        if (first.playback?.resumeAt ?? 0) > 0 {
                            IconButton(system: "arrow.counterclockwise", label: "Play from start") {
                                model.playLibraryFile(first, title: label, fromStart: true)
                            }
                            Spacer()
                        }
                        WatchedButton(files: files)
                        DownloadButton(request: DownloadRequest(path: first.path, name: first.name, title: label,
                                                                thumb: episode.still ?? first.thumbnailUrl, size: first.size ?? 0))
                        if model.canFixMatches {
                            Spacer()
                            IconButton(system: "pencil", label: "Fix match") {
                                model.openMatch(path: first.path, isDir: false, name: first.name)
                            }
                        }
                        Spacer()
                    }
                }
                .frame(width: 144)
                Button { showDetails = true } label: {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(files.map(\.name).joined(separator: "\n")).font(.caption2).foregroundStyle(Palette.muted)
                        Text(String(format: "S%02dE%02d · %@", episode.season, episode.episode, episode.title))
                            .font(.subheadline.weight(.medium)).padding(.top, 10)
                        let meta = [episode.airDate, episode.runtime.flatMap { $0 > 0 ? "\($0) min" : nil },
                                    files.count > 1 ? "\(files.count) files" : nil].compactMap { $0 }
                        if !meta.isEmpty {
                            Text(meta.joined(separator: " · ")).font(.caption2).foregroundStyle(Palette.muted)
                        }
                        if let overview = episode.overview, !overview.isEmpty {
                            Text(overview).font(.caption).lineLimit(3).padding(.top, 4)
                        }
                        let resumeAt = first.playback?.resumeAt ?? 0
                        if resumeAt > 0 {
                            Text("Resumes at \(clockText(resumeAt))").font(.caption.weight(.medium))
                                .foregroundStyle(Palette.accent).padding(.top, 4)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .multilineTextAlignment(.leading)
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 16).padding(.vertical, 10)
            .sheet(isPresented: $showDetails) { EpisodeDetails(episode: episode) }
        }
    }
}

/// An episode's text in full: the files, title, date and length, and the whole description.
struct EpisodeDetails: View {
    @Environment(\.dismiss) private var dismiss
    let episode: LibraryEpisode

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 4) {
                Text((episode.files ?? []).map(\.name).joined(separator: "\n")).font(.caption2).foregroundStyle(Palette.muted)
                Text(String(format: "S%02dE%02d · %@", episode.season, episode.episode, episode.title))
                    .font(.headline).padding(.top, 10)
                let meta = [episode.airDate, episode.runtime.flatMap { $0 > 0 ? "\($0) min" : nil }].compactMap { $0 }
                if !meta.isEmpty { Text(meta.joined(separator: " · ")).font(.caption).foregroundStyle(Palette.muted) }
                if let overview = episode.overview, !overview.isEmpty {
                    Text(overview).font(.subheadline).padding(.top, 10)
                }
            }
            .padding(20).padding(.trailing, 28)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .overlay(alignment: .topTrailing) {
            IconButton(system: "xmark", label: "Close") { dismiss() }
        }
        .foregroundStyle(Palette.text)
        .presentationBackground(Palette.panel)
        .presentationDetents([.medium, .large])
    }
}

/// Where a show picks up, as (season index, episode index): the episode played most recently if
/// it's unfinished, else the one after it (on into the next season, skipping specials unless it
/// is one). Nil when none has been played.
func upNext(_ seasons: [LibrarySeason], justPlayed: String?) -> (season: Int, episode: Int)? {
    struct At { let season: Int; let episode: Int; let ep: LibraryEpisode; let number: Int }
    var all: [At] = []
    for (si, s) in seasons.enumerated() {
        for (ei, e) in (s.episodes ?? []).enumerated() { all.append(At(season: si, episode: ei, ep: e, number: s.season)) }
    }
    func files(_ a: At) -> [LibraryFile] { a.ep.files ?? [] }
    let played = { (a: At) in files(a).contains { ($0.playback?.watched ?? false) || ($0.playback?.resumeAt ?? 0) > 0 } }
    let latest = all.firstIndex { a in justPlayed != nil && files(a).contains { $0.path == justPlayed } }
        ?? all.indices.filter { played(all[$0]) }.max { i, j in
            (files(all[i]).compactMap { $0.playback?.updatedAt }.max() ?? 0) < (files(all[j]).compactMap { $0.playback?.updatedAt }.max() ?? 0)
        }
    guard let latest else { return nil }
    let at = all[latest]
    let finished = files(at).contains { $0.playback?.watched == true }
        && !files(at).contains { ($0.playback?.resumeAt ?? 0) > 0 && $0.playback?.watched != true }
    let next = finished ? (all.dropFirst(latest + 1).first { $0.number != 0 || at.number == 0 } ?? at) : at
    return (next.season, next.episode)
}
