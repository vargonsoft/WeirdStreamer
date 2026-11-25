import SwiftUI

/// A folder (or a server's libraries), as Android's PhoneFileBrowserScreen: Continue Watching and
/// Recently Added on top where they apply, then the folders and files as a list or a grid, with
/// an A–Z strip at the side.
struct FileBrowserScreen: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        let items = model.filteredEntries
        let letters = alphabetIndex(items.map(\.name), groups: items.map { $0.isDirectory ? 0 : 1 })
        ScrollViewReader { proxy in
            HStack(spacing: 0) {
                ScrollView {
                    if model.isGridView {
                        VStack(spacing: 8) {
                            if model.hasTopRows { ContinueRows() }
                            LazyVGrid(columns: [GridItem(.adaptive(minimum: 120), spacing: 8)], spacing: 8) {
                                ForEach(items) { item in
                                    FileTile(item: item).id(item.path)
                                }
                            }
                        }
                        .padding(8)
                    } else {
                        LazyVStack(spacing: 0) {
                            if model.hasTopRows {
                                ContinueRows().padding(.horizontal, 8).padding(.bottom, 8)
                            }
                            ForEach(items) { item in
                                FileRow(item: item).id(item.path)
                            }
                        }
                        .padding(.vertical, 8)
                    }
                }
                .refreshable { model.loadPath(model.path, displayName: model.pathDisplayName) }
                if letters.count > 1 {
                    IndexBar(letters: letters) { entry in
                        withAnimation { proxy.scrollTo(items[entry.position].path, anchor: .top) }
                    }
                    .padding(.vertical, 8)
                }
            }
        }
    }
}

/// A folder or file row (Android's PhoneFileRowItem): a 40-point thumbnail or icon with its
/// progress, the name, and "size, length · time left". Holding a video offers Play from start
/// and Mark watched.
struct FileRow: View {
    @Environment(AppModel.self) private var model
    let item: FileEntry

    var body: some View {
        Button { open(model, item) } label: {
            HStack(spacing: 16) {
                ZStack {
                    if item.isDirectory {
                        FolderPreview(url: item.thumbnailUrl)
                    } else {
                        Image(systemName: item.isImage ? "photo" : (item.isPdf ? "doc.richtext" : "doc.text"))
                            .font(.system(size: 20))
                            .foregroundStyle(Palette.text)
                    }
                    if !item.isDirectory {
                        RemoteImage(relative: item.thumbnailUrl).frame(width: 40, height: 40)
                            .clipShape(RoundedRectangle(cornerRadius: 4))
                    }
                    // Videos have filmstrip edges, telling them from photos in the same folder.
                    if item.isVideo { Filmstrip().clipShape(RoundedRectangle(cornerRadius: 4)) }
                    WatchProgressOverlay(fraction: item.playback?.fraction(fallbackDuration: item.duration) ?? 0,
                                         watched: item.playback?.watched == true, barHeight: 3, badgeSize: 14)
                }
                .frame(width: 40, height: 40)
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.name).font(.body).lineLimit(1)
                    if !item.isDirectory {
                        Text(infoLine(item)).font(.subheadline).foregroundStyle(Palette.muted)
                    }
                }
                Spacer(minLength: 0)
                if item.isVideo {
                    DownloadButton(request: DownloadRequest(path: item.path, name: item.name, title: nil,
                                                            thumb: item.thumbnailUrl, size: item.size))
                }
            }
            .padding(.horizontal, 16).padding(.vertical, 8)
            .frame(minHeight: item.isDirectory ? 56 : 72)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .contextMenu {
            if item.isVideo { VideoMenu(item: item) }
            FixMatchMenuItem(item: item)
        }
    }
}

/// A folder or file tile (Android's PhoneFileTileItem): a square card with the thumbnail or icon,
/// the name, and size and length.
struct FileTile: View {
    @Environment(AppModel.self) private var model
    let item: FileEntry

    var body: some View {
        Button { open(model, item) } label: {
            VStack(spacing: 4) {
                ZStack {
                    if item.isDirectory {
                        FolderPreview(url: item.thumbnailUrl).padding(4)
                    } else {
                        Image(systemName: "doc.text")
                            .font(.system(size: 28))
                            .foregroundStyle(Palette.text)
                    }
                    if !item.isDirectory {
                        RemoteImage(relative: item.thumbnailUrl).clipShape(RoundedRectangle(cornerRadius: 8))
                    }
                    if item.isVideo { Filmstrip().clipShape(RoundedRectangle(cornerRadius: 8)) }
                    WatchProgressOverlay(fraction: item.playback?.fraction(fallbackDuration: item.duration) ?? 0,
                                         watched: item.playback?.watched == true, barHeight: 3, badgeSize: 18)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                Text(item.name).font(.caption.weight(.medium)).lineLimit(1)
                if !item.isDirectory {
                    Text(sizeAndLength(item)).font(.caption2).foregroundStyle(Palette.muted).lineLimit(1)
                }
            }
            .padding(8)
            .aspectRatio(1, contentMode: .fit)
            .background(Palette.raised.opacity(0.3), in: RoundedRectangle(cornerRadius: 12))
        }
        .buttonStyle(.plain)
        .contextMenu {
            if item.isVideo { VideoMenu(item: item) }
            FixMatchMenuItem(item: item)
        }
    }
}

/// Holding a video: Play from start (when it has a place to resume), Mark watched or unwatched.
struct VideoMenu: View {
    @Environment(AppModel.self) private var model
    let item: FileEntry

    var body: some View {
        if (item.playback?.resumeAt ?? 0) > 0 {
            Button("Play from start") { model.playFromStart(item) }
        }
        let watched = item.playback?.watched == true
        Button(watched ? "Mark unwatched" : "Mark watched") { model.setWatched([item.path], !watched) }
    }
}

/// Holding a video, or a folder standing for one title: Fix match (admins).
struct FixMatchMenuItem: View {
    @Environment(AppModel.self) private var model
    let item: FileEntry

    var body: some View {
        if model.canFixMatches && (item.isVideo || (item.isDirectory && item.fixable)) {
            Button("Fix match") { model.openMatch(path: item.path, isDir: item.isDirectory, name: item.name) }
        }
    }
}

private func open(_ model: AppModel, _ item: FileEntry) {
    if item.isDirectory {
        model.loadPath(item.path, displayName: item.name)
    } else if item.isVideo {
        model.openVideo(item)
    } else if item.isImage {
        model.viewingImage = item
    } else if item.isPdf {
        model.viewingPdf = item
    }
}

private func sizeAndLength(_ item: FileEntry) -> String {
    let size = formatFileSize(item.size)
    if let d = item.duration, d > 0 { return "\(size), \(clockText(d))" }
    return size
}

private func infoLine(_ item: FileEntry) -> String {
    let info = sizeAndLength(item)
    if let left = timeLeftText(item.playback, item.duration) { return "\(info) · \(left)" }
    return info
}

// MARK: Continue Watching and Recently Added

/// Continue Watching, then the Recently Added rows (nothing when there are none).
struct ContinueRows: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            VideoRow(heading: "Continue Watching", items: model.continueItems)
            ForEach(model.newRows, id: \.heading) { row in
                VideoRow(heading: row.heading, items: row.items)
            }
        }
    }
}

struct VideoRow: View {
    let heading: String
    let items: [ContinueItem]

    var body: some View {
        if !items.isEmpty {
            Text(heading).font(.headline).padding(.top, 12).padding(.bottom, 8)
                .frame(maxWidth: .infinity, alignment: .leading)
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(alignment: .top, spacing: 12) {
                    ForEach(items, id: \.path) { ContinueCard(item: $0) }
                }
            }
        }
    }
}

/// A card: the still (or backdrop, or thumbnail) with its progress and an X to remove it from
/// Continue Watching, then the title, subtitle and "23 min left" / "Added 2 days ago".
struct ContinueCard: View {
    @Environment(AppModel.self) private var model
    let item: ContinueItem

    var body: some View {
        let (title, subtitle) = item.titleLines
        Button { model.playContinue(item) } label: {
            VStack(alignment: .leading, spacing: 0) {
                ZStack {
                    Palette.raised
                    Image(systemName: "film").font(.system(size: 28))
                    RemoteImage(relative: item.metadata?.still ?? item.metadata?.backdrop ?? item.thumbnailUrl)
                    WatchProgressOverlay(fraction: item.playback?.fraction(fallbackDuration: item.duration) ?? 0,
                                         watched: item.playback?.watched == true)
                }
                .frame(width: 200, height: 112.5)
                .clipShape(RoundedRectangle(cornerRadius: 8))
                .overlay(alignment: .topTrailing) {
                    if item.addedAt == nil {
                        Button { model.removeFromContinue(item) } label: {
                            Image(systemName: "xmark").font(.system(size: 11, weight: .bold)).foregroundStyle(.white)
                                .frame(width: 26, height: 26).background(.black.opacity(0.6), in: Circle())
                                .frame(width: 36, height: 36)
                        }
                        .accessibilityLabel("Remove from Continue Watching")
                    }
                }
                Text(title).font(.subheadline.weight(.medium)).lineLimit(1).padding(.top, 6)
                if let subtitle {
                    Text(subtitle).font(.caption2).foregroundStyle(Palette.muted).lineLimit(1)
                }
                if let note = continueNote(item) {
                    Text(note).font(.caption2).foregroundStyle(Palette.muted)
                }
            }
            .frame(width: 200, alignment: .leading)
        }
        .buttonStyle(.plain)
    }
}

// MARK: The A–Z strip

struct IndexEntry: Hashable {
    let label: String
    let position: Int
}

private let articles = ["the ", "a ", "an "]

/// A name's letter for the strip ("#" for anything else); with `skipArticles`, "The Wire" is W.
func indexLabel(_ name: String, skipArticles: Bool = false) -> String? {
    var n = name.trimmingCharacters(in: .whitespaces)
    guard !n.isEmpty, n != ".." else { return nil }
    if skipArticles, let article = articles.first(where: { n.lowercased().hasPrefix($0) && n.count > $0.count }) {
        n = String(n.dropFirst(article.count)).trimmingCharacters(in: .whitespaces)
    }
    guard let c = n.first else { return nil }
    return c.isLetter ? String(c).uppercased() : "#"
}

/// The strip's entries: each letter's first item in each run, folders' letters then files'.
func alphabetIndex(_ names: [String], groups: [Int], skipArticles: Bool = false) -> [IndexEntry] {
    var result: [IndexEntry] = []
    var last: (String, Int)?
    for (i, name) in names.enumerated() {
        guard let label = indexLabel(name, skipArticles: skipArticles) else { continue }
        if last?.0 != label || last?.1 != groups[i] { result.append(IndexEntry(label: label, position: i)) }
        last = (label, groups[i])
    }
    return result
}

/// The letters down the right edge, like the contacts app's (Android's PhoneIndexBar): each in a
/// slot of at most 20 points from the top; tapping or dragging along it jumps there, with the
/// letter under the finger in gold.
struct IndexBar: View {
    let letters: [IndexEntry]
    let onSelect: (IndexEntry) -> Void

    @State private var touched: Int?

    var body: some View {
        GeometryReader { geo in
            let slot = min(20, (geo.size.height - 16) / CGFloat(max(letters.count, 1)))
            VStack(spacing: 0) {
                ForEach(Array(letters.enumerated()), id: \.offset) { i, entry in
                    Text(entry.label).font(.system(size: 11, weight: .semibold))
                        .foregroundStyle(i == touched ? Palette.onAccent : Palette.muted)
                        .frame(width: 20, height: slot)
                        .background(i == touched ? Palette.accent : .clear, in: Circle())
                }
            }
            .padding(.vertical, 8)
            .frame(width: 24, alignment: .top)
            .contentShape(Rectangle())
            .gesture(DragGesture(minimumDistance: 0)
                .onChanged { drag in
                    let i = min(max(Int((drag.location.y - 8) / max(slot, 1)), 0), letters.count - 1)
                    if i != touched {
                        touched = i
                        UISelectionFeedbackGenerator().selectionChanged()
                        onSelect(letters[i])
                    }
                }
                .onEnded { _ in touched = nil })
        }
        .frame(width: 24)
    }
}
