import SwiftUI

/// What to download: a file's client path, name, what it is, thumbnail and size.
struct DownloadRequest {
    let path: String
    let name: String
    let title: String?
    let thumb: String?
    let size: Int64
}

/// A file's download control (Android's DownloadButton): a download icon, a progress circle while
/// it downloads (tap to cancel), a delete button once it's done, or an error icon if it failed.
struct DownloadButton: View {
    @Environment(AppModel.self) private var model
    let request: DownloadRequest

    @State private var choosing = false
    @State private var confirming = false

    var body: some View {
        let manager = DownloadManager.shared
        let download = manager.find(request.path)
        Button {
            if download == nil { choosing = true } else { confirming = true }
        } label: {
            Group {
                if let download, download.isActive {
                    ZStack {
                        if download.state == "queued" || download.progress <= 0 {
                            ProgressView()
                        } else {
                            Circle().stroke(Palette.muted.opacity(0.3), lineWidth: 3)
                            Circle().trim(from: 0, to: download.progress).stroke(Palette.accent, style: StrokeStyle(lineWidth: 3, lineCap: .round))
                                .rotationEffect(.degrees(-90))
                        }
                        Image(systemName: "xmark").font(.system(size: 10, weight: .bold))
                    }
                    .frame(width: 26, height: 26)
                } else if let download, download.isDone {
                    Image(systemName: "trash")
                } else if download != nil {
                    Image(systemName: "exclamationmark.circle").foregroundStyle(Palette.error)
                } else {
                    Image(systemName: "arrow.down.circle")
                }
            }
            .font(.system(size: 20))
            .frame(width: 44, height: 44)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(download == nil ? "Download" : "Download options")
        .sheet(isPresented: $choosing) { DownloadQualitySheet(requests: [request]) }
        .alert(alertTitle(download), isPresented: $confirming) {
            if let download {
                if download.state == "failed" {
                    Button("Retry") { manager.retry(download) }
                    Button("Remove", role: .destructive) { manager.remove(download) }
                } else {
                    Button(download.isDone ? "Delete" : "Cancel download", role: .destructive) { manager.remove(download) }
                    Button(download.isDone ? "Keep" : "Keep downloading", role: .cancel) {}
                }
            }
        } message: {
            if let download {
                let name = download.title ?? download.name
                Text(download.state == "failed" ? "\(name)\n\n\(download.error ?? "Unknown error")"
                     : download.isDone ? "\(name) will be removed from this phone (\(formatBytes(download.bytesTotal)))."
                     : "\(name) will stop downloading.")
            }
        }
    }

    private func alertTitle(_ d: Download?) -> String {
        guard let d else { return "" }
        return d.state == "failed" ? "Download failed" : d.isDone ? "Delete download?" : "Cancel download?"
    }
}

/// How to download: the original; Compatibility, a full-quality copy keeping what this phone plays
/// and converting the rest on the server; or a compressed copy. Each file is checked against what
/// the phone plays first: one that won't play as it is makes Compatibility the choice, and says why.
struct DownloadQualitySheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let requests: [DownloadRequest]

    enum Mode { case original, compat, compressed }
    @State private var picked: Mode?
    @State private var sizePct = 50
    @State private var scale = 100
    @State private var checks: [String: OfflinePlayback]?

    private var pending: [DownloadRequest] { requests.filter { DownloadManager.shared.find($0.path) == nil } }

    var body: some View {
        let pending = self.pending
        let unplayable = checks?.values.filter { !$0.plays } ?? []
        let mode = picked ?? (unplayable.isEmpty ? .original : .compat)
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    if pending.isEmpty {
                        Text(requests.count > 1 ? "All of these files are downloaded or downloading." : "This file is downloaded or downloading.")
                    } else {
                        choice("Original", "The file as it is", selected: mode == .original) { picked = .original }
                        if checks == nil {
                            Text("Checking what this phone plays…").font(.caption).foregroundStyle(Palette.muted).padding(.leading, 32)
                        } else if !unplayable.isEmpty {
                            Text(pending.count == 1 ? "This phone can't play \(unplayable[0].problem ?? "it")."
                                 : "\(unplayable.count) of \(pending.count) files won't play on this phone as they are.")
                                .font(.caption).foregroundStyle(Palette.error).padding(.leading, 32)
                        }
                        choice("Compatibility", compatDetail(pending.count == 1 ? checks?.values.first : nil), selected: mode == .compat) { picked = .compat }
                        choice("Compressed", "A smaller copy, made on the server first", selected: mode == .compressed) { picked = .compressed }
                        if mode == .compressed {
                            Text("Size").font(.subheadline.weight(.medium)).padding(.top, 8)
                            chips([90, 75, 50, 25].map { ($0, "\($0)%") }, value: $sizePct)
                            Text("Resolution").font(.subheadline.weight(.medium))
                            chips([(100, "Original"), (75, "75%"), (50, "50%")], value: $scale)
                        }
                        let total = pending.reduce(0) { $0 + $1.size }
                        if mode != .compat, total > 0 {
                            Text(mode == .compressed ? "About \(formatBytes(total * Int64(sizePct) / 100))" : formatBytes(total))
                                .padding(.top, 8)
                        }
                        if DownloadManager.shared.wifiOnly {
                            Text("Downloads use Wi-Fi only. You can change this in Settings › Downloads.")
                                .font(.caption).foregroundStyle(Palette.muted)
                        }
                    }
                }
                .padding(20)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .navigationTitle(pending.isEmpty ? "Already downloaded" : pending.count == 1 ? "Download \(pending[0].name)" : "Download \(pending.count) files")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button(pending.isEmpty ? "OK" : "Cancel") { dismiss() } }
                if !pending.isEmpty {
                    ToolbarItem(placement: .confirmationAction) {
                        Button("Download") {
                            DownloadManager.shared.add(pending.map { (path: $0.path, name: $0.name, title: $0.title, thumb: $0.thumb, size: $0.size) },
                                                       sizePct: mode == .compressed ? sizePct : 100,
                                                       scale: mode == .compressed ? scale : 100,
                                                       compat: mode == .compat)
                            dismiss()
                        }
                    }
                }
            }
        }
        .presentationBackground(Palette.panel)
        .presentationDetents([.medium, .large])
        .themed()
        .task {
            guard let client = model.client else { return }
            var found: [String: OfflinePlayback] = [:]
            for r in pending {
                if let codecs: CodecsInfo = try? await client.get("/api/media/codecs", query: [("path", ServerClient.encodePath(r.path))]) {
                    found[r.path] = OfflinePlayback.of(codecs, path: r.path)
                }
            }
            checks = found
        }
    }

    private func choice(_ label: String, _ detail: String, selected: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 12) {
                Image(systemName: selected ? "largecircle.fill.circle" : "circle").foregroundStyle(selected ? Palette.accent : Palette.muted)
                VStack(alignment: .leading) {
                    Text(label)
                    Text(detail).font(.caption).foregroundStyle(Palette.muted)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func chips(_ options: [(Int, String)], value: Binding<Int>) -> some View {
        HStack(spacing: 8) {
            ForEach(options, id: \.0) { option in
                Button { value.wrappedValue = option.0 } label: {
                    Text(option.1).font(.subheadline)
                        .padding(.horizontal, 12).padding(.vertical, 6)
                        .foregroundStyle(value.wrappedValue == option.0 ? Palette.onAccent : Palette.text)
                        .background(value.wrappedValue == option.0 ? Palette.accent : .clear, in: RoundedRectangle(cornerRadius: 5))
                        .overlay(RoundedRectangle(cornerRadius: 5).stroke(value.wrappedValue == option.0 ? .clear : Palette.muted))
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func compatDetail(_ check: OfflinePlayback?) -> String {
        guard let check else { return "Full quality, converting only what this phone can't play" }
        if check.plays { return "Plays here as it is: the same as the original" }
        return [check.video ? "Video kept" : "Video converted to H.264", check.audio ? "audio kept" : "audio converted to stereo AAC"]
            .joined(separator: " · ") + ", on the server first"
    }
}

/// Settings › Downloads: Wi-Fi only, Download ahead, the phone's storage, and the open account's downloads.
struct DownloadsTab: View {
    @Environment(AppModel.self) private var model
    @State private var confirming: Download?

    var body: some View {
        let manager = DownloadManager.shared
        let items = manager.mine()
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                SettingsRow(title: "Download on Wi-Fi only", detail: manager.wifiOnly ? "Downloads wait for Wi-Fi" : "Downloads also use mobile data") {
                    Toggle("", isOn: Binding(get: { manager.wifiOnly }, set: { manager.wifiOnly = $0 })).labelsHidden()
                }
                SettingsRow(title: "Download ahead", detail: manager.downloadAhead
                            ? "Watching an episode downloads the next 3, removed once you've watched past them"
                            : "Off: only what you download yourself") {
                    Toggle("", isOn: Binding(get: { manager.downloadAhead }, set: { manager.downloadAhead = $0 })).labelsHidden()
                }
                storage(items).padding(.horizontal, 16).padding(.vertical, 4)
                Divider().overlay(Palette.raised).padding(.top, 8)
                if items.isEmpty {
                    Text("Nothing downloaded yet. Use the download button next to a file, or ⋮ › Download this folder.")
                        .font(.subheadline).foregroundStyle(Palette.muted).padding(16)
                }
                ForEach(items.sorted { ($0.title ?? $0.name).localizedStandardCompare($1.title ?? $1.name) == .orderedAscending }, id: \.id) { d in
                    row(d)
                }
            }
        }
        .alert(confirming.map { $0.state == "failed" ? "Download failed" : $0.isDone ? "Delete download?" : "Cancel download?" } ?? "",
               isPresented: Binding(get: { confirming != nil }, set: { if !$0 { confirming = nil } })) {
            if let d = confirming {
                if d.state == "failed" { Button("Retry") { manager.retry(d) } }
                Button(d.isDone ? "Delete" : d.state == "failed" ? "Remove" : "Cancel download", role: .destructive) { manager.remove(d) }
                Button("Keep", role: .cancel) {}
            }
        } message: {
            if let d = confirming { Text(d.error ?? (d.title ?? d.name)) }
        }
    }

    private func storage(_ items: [Download]) -> some View {
        let ours = items.reduce(Int64(0)) { sum, d in
            sum + ((d.localURL.flatMap { try? FileManager.default.attributesOfItem(atPath: $0.path)[.size] as? Int64 }) ?? 0)
        }
        let values = try? URL(fileURLWithPath: NSHomeDirectory()).resourceValues(forKeys: [.volumeTotalCapacityKey, .volumeAvailableCapacityForImportantUsageKey])
        let total = Int64(values?.volumeTotalCapacity ?? 0)
        let free = values?.volumeAvailableCapacityForImportantUsage ?? 0
        let others = max(0, total - free - ours)
        let done = items.filter(\.isDone).count
        return VStack(alignment: .leading, spacing: 6) {
            Text("Storage").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent)
            if total > 0 {
                GeometryReader { geo in
                    HStack(spacing: 0) {
                        Rectangle().fill(Palette.accent).frame(width: ours > 0 ? max(geo.size.width * CGFloat(ours) / CGFloat(total), geo.size.width * 0.01) : 0)
                        Rectangle().fill(Palette.muted).frame(width: geo.size.width * CGFloat(others) / CGFloat(total))
                        Spacer(minLength: 0)
                    }
                }
                .frame(height: 10)
                .background(Palette.raised)
                .clipShape(RoundedRectangle(cornerRadius: 5))
            }
            legend(Palette.accent, "Downloads", "\(formatBytes(ours).isEmpty ? "0 B" : formatBytes(ours)) · \(done) \(done == 1 ? "file" : "files")")
            legend(Palette.muted, "Everything else", formatBytes(others))
            legend(Palette.raised, "Free", "\(formatBytes(free)) of \(formatBytes(total))")
        }
    }

    private func legend(_ color: Color, _ label: String, _ value: String) -> some View {
        HStack {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 10, height: 10)
            Text(label).font(.caption)
            Spacer()
            Text(value).font(.caption).foregroundStyle(Palette.muted)
        }
    }

    private func row(_ d: Download) -> some View {
        HStack(spacing: 12) {
            Button { play(d) } label: {
                ZStack {
                    Palette.raised
                    Image(systemName: "film").font(.system(size: 24))
                    RemoteImage(relative: DownloadExtras.thumbnail(d.id)?.absoluteString ?? d.thumbnailUrl)
                    if d.isDone { Image(systemName: "play.fill").font(.system(size: 26)).foregroundStyle(.white) }
                }
                .frame(width: 112, height: 63)
                .clipShape(RoundedRectangle(cornerRadius: 6))
            }
            .buttonStyle(.plain)
            .disabled(!d.isDone)
            VStack(alignment: .leading, spacing: 2) {
                if let title = d.title { Text(title).font(.subheadline.weight(.medium)).lineLimit(2) }
                Text(d.name).font(.caption).lineLimit(2)
                let facts = [d.ahead ? "Downloaded ahead" : nil, d.qualityLabel,
                             d.width > 0 && d.height > 0 ? "\(d.width)×\(d.height)" : nil,
                             d.isDone ? formatBytes(d.bytesTotal) : nil].compactMap { $0 }.filter { !$0.isEmpty }
                Text(facts.joined(separator: " · ")).font(.caption2).foregroundStyle(Palette.muted)
                switch d.state {
                case "done": EmptyView()
                case "failed": Text(d.error ?? "Download failed").font(.caption2).foregroundStyle(Palette.error).lineLimit(3)
                default:
                    Text(d.state == "queued" ? (d.stage ?? "Waiting")
                         : d.state == "preparing" ? "\(d.stage ?? "Preparing") · \(Int(d.progress * 100))%"
                         : "Downloading · \(formatBytes(d.bytesDone)) of \(formatBytes(d.bytesTotal))")
                        .font(.caption2).foregroundStyle(Palette.accent)
                    if d.state == "queued" { ProgressView().progressViewStyle(.linear).tint(Palette.accent) }
                    else { ProgressView(value: d.progress).tint(Palette.accent) }
                }
            }
            Spacer(minLength: 0)
            IconButton(system: d.isActive ? "xmark" : d.state == "failed" ? "exclamationmark.circle" : "trash",
                       label: d.isActive ? "Cancel download" : "Delete download",
                       tint: d.state == "failed" ? Palette.error : Palette.text) { confirming = d }
        }
        .padding(.horizontal, 16).padding(.vertical, 8)
    }

    private func play(_ d: Download) {
        guard let url = d.localURL else { return }
        model.playDownload(d, file: url)
    }
}

/// ⋮ › Download this folder: every video in it and the folders below it, then the quality choice.
struct DownloadFolderSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var found: [DownloadRequest]?

    var body: some View {
        Group {
            if let found, !found.isEmpty {
                DownloadQualitySheet(requests: found)
            } else if found != nil {
                VStack(spacing: 16) {
                    Text("No videos here").font(.headline)
                    Text("This folder has no videos to download.").font(.subheadline)
                    Button("OK") { dismiss() }.buttonStyle(PhoneButtonStyle())
                }
                .padding(24)
            } else {
                VStack(spacing: 16) {
                    ProgressView()
                    Text("Finding videos…")
                    Button("Cancel") { dismiss() }.buttonStyle(PhoneSecondaryButtonStyle())
                }
                .padding(24)
            }
        }
        .presentationBackground(Palette.panel)
        .presentationDetents([.medium, .large])
        .themed()
        .task { found = await videos(under: model.path, depth: 0) }
    }

    private func videos(under path: String, depth: Int) async -> [DownloadRequest] {
        guard let client = model.client, depth < 6,
              let dir: Directory = try? await client.get("/api/media/browse", query: [("path", path)]) else { return [] }
        var result = (dir.items ?? []).filter { $0.isVideo }
            .map { DownloadRequest(path: $0.path, name: $0.name, title: nil, thumb: $0.thumbnailUrl, size: $0.size ?? 0) }
        for sub in dir.dirs ?? [] where result.count < 500 {
            result += await videos(under: sub.id, depth: depth + 1)
        }
        return result
    }
}
