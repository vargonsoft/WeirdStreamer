import Foundation
import SwiftData
import Observation

// Offline downloads, as the Android app's DownloadStore and DownloadWorker. Each is kept on the
// phone (SwiftData) with the account it was made in (only that account sees it). An original comes
// straight from the stream; Compatibility (full quality, keeping the video and audio this phone
// plays, converting the rest) and Compressed (a share of the size and resolution) are encoded by a
// server job first, then fetched. Files come through a background URLSession, so they carry on
// while the app is away; a download the app lost track of (killed, or expired past its signing)
// is picked up again at the next launch.

@Model
final class Download {
    @Attribute(.unique) var id: String
    var address: String
    var account: String
    var sourcePath: String
    var name: String
    var title: String?
    var thumbnailUrl: String?
    var sizePct: Int
    var scale: Int
    var compat: Bool
    var state: String           // queued, preparing, downloading, done, failed
    var stage: String?
    var progress: Double
    var bytesDone: Int64
    var bytesTotal: Int64
    var jobId: String?
    var fileName: String?       // in the Downloads folder
    var width: Int
    var height: Int
    var error: String?
    var createdAt: Date
    var duration: Double
    var ahead: Bool             // downloaded ahead of the episode being watched
    var extrasAt: Date? = nil   // when its subtitles, skip times and trickplay were last saved in full

    init(address: String, account: String, sourcePath: String, name: String, title: String?, thumbnailUrl: String?,
         sizePct: Int, scale: Int, compat: Bool, size: Int64, ahead: Bool = false) {
        self.id = UUID().uuidString
        self.address = address
        self.account = account
        self.sourcePath = sourcePath
        self.name = name
        self.title = title
        self.thumbnailUrl = thumbnailUrl
        self.sizePct = sizePct
        self.scale = scale
        self.compat = compat
        self.state = "queued"
        self.progress = 0
        self.bytesDone = 0
        self.bytesTotal = size
        self.width = 0
        self.height = 0
        self.createdAt = Date()
        self.duration = 0
        self.ahead = ahead
    }

    var isOriginal: Bool { sizePct == 100 && scale == 100 && !compat }
    var isActive: Bool { state == "queued" || state == "preparing" || state == "downloading" }
    var isDone: Bool { state == "done" }

    var qualityLabel: String {
        if isOriginal { return "Original" }
        if compat { return "Compatibility" }
        if sizePct == 100 && scale == 100 { return "Full quality" }
        return [sizePct < 100 ? "\(sizePct)% size" : "Full quality", scale < 100 ? "\(scale)% resolution" : nil]
            .compactMap { $0 }.joined(separator: " · ")
    }

    var localURL: URL? { fileName.map { DownloadManager.folder.appendingPathComponent($0) } }
}

/// What this phone plays of a file, for Compatibility: its video, its default audio track, and the file as a whole.
struct OfflinePlayback {
    let video: Bool
    let audio: Bool
    let plays: Bool
    let problem: String?

    static func of(_ codecs: CodecsInfo, path: String) -> OfflinePlayback {
        let v = codecs.video?.codec?.lowercased()
        let a = (codecs.audio?.first { $0.default == true } ?? codecs.audio?.first)?.codec?.lowercased()
        let video = v == nil || ["h264", "hevc", "mpeg4"].contains(v!)
        let audio = a == nil || ["aac", "mp3", "ac3", "eac3", "alac", "flac"].contains(a!)
        let container = ["mp4", "m4v", "mov"].contains((path as NSString).pathExtension.lowercased())
        let problem = !video ? "its \(v!.uppercased()) video" : !audio ? "its \(a!.uppercased()) audio" : !container ? "its \((path as NSString).pathExtension.uppercased()) file" : nil
        return OfflinePlayback(video: video, audio: audio, plays: video && audio && container, problem: problem)
    }
}

@MainActor
@Observable
final class DownloadManager: NSObject {
    static let shared = DownloadManager()
    nonisolated static let folder: URL = {
        let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        let url = docs.appendingPathComponent("Downloads", isDirectory: true)
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }()

    /// Bumped when downloads change, so views redraw.
    private(set) var version = 0

    @ObservationIgnored private weak var model: AppModel?
    @ObservationIgnored private var running = false
    @ObservationIgnored private var session: URLSession!
    @ObservationIgnored var backgroundCompletion: (() -> Void)?

    var wifiOnly: Bool {
        get { _ = version; return model?.setting("downloads_wifi_only") != "false" }
        set { model?.setSetting("downloads_wifi_only", newValue ? "true" : "false"); makeSession(); bump() }
    }
    var downloadAhead: Bool {
        get { _ = version; return model?.setting("downloads_ahead") == "true" }
        set { model?.setSetting("downloads_ahead", newValue ? "true" : "false"); bump() }
    }

    func attach(_ model: AppModel) {
        self.model = model
        makeSession()
        // Downloads the app lost track of (killed mid-way, or expired): queued again; the
        // background session's own tasks report back by themselves.
        session.getAllTasks { tasks in
            let live = Set(tasks.compactMap(\.taskDescription))
            Task { @MainActor in
                for d in self.all() where d.isActive && !live.contains(d.id) && d.state == "downloading" {
                    d.state = "queued"
                }
                self.save()
                self.run()
            }
        }
    }

    private func makeSession() {
        let config = URLSessionConfiguration.background(withIdentifier: "com.vargon.WeirdStreamer.downloads")
        config.allowsCellularAccess = model?.setting("downloads_wifi_only") == "false"
        config.sessionSendsLaunchEvents = true
        config.isDiscretionary = false
        session?.finishTasksAndInvalidate()
        session = URLSession(configuration: config, delegate: self, delegateQueue: .main)
    }

    // MARK: The list

    func all() -> [Download] {
        guard let context = model?.context else { return [] }
        return (try? context.fetch(FetchDescriptor<Download>(sortBy: [SortDescriptor(\.createdAt)]))) ?? []
    }

    /// The open account's downloads on the open server.
    func mine() -> [Download] {
        _ = version
        guard let address = model?.currentAddress, let account = model?.currentAccount else { return [] }
        return all().filter { $0.address == address && $0.account == account }
    }

    func find(_ sourcePath: String) -> Download? {
        mine().first { $0.sourcePath == sourcePath }
    }

    func add(_ requests: [(path: String, name: String, title: String?, thumb: String?, size: Int64)],
             sizePct: Int, scale: Int, compat: Bool, ahead: Bool = false) {
        guard let model, let address = model.currentAddress, let account = model.currentAccount else { return }
        for r in requests where find(r.path) == nil {
            model.context.insert(Download(address: address, account: account, sourcePath: r.path, name: r.name, title: r.title,
                                          thumbnailUrl: r.thumb, sizePct: sizePct, scale: scale, compat: compat, size: r.size, ahead: ahead))
        }
        save()
        run()
    }

    func remove(_ d: Download) {
        let id = d.id
        session.getAllTasks { tasks in tasks.filter { $0.taskDescription == id }.forEach { $0.cancel() } }
        if let job = d.jobId, let client = model?.client {
            Task { try? await client.execute("DELETE", "/api/downloads/\(job)") }
        }
        if let url = d.localURL { try? FileManager.default.removeItem(at: url) }
        DownloadExtras.remove(id)
        model?.context.delete(d)
        save()
    }

    func retry(_ d: Download) {
        d.state = "queued"
        d.error = nil
        d.progress = 0
        save()
        run()
    }

    // MARK: Working through the queue, one at a time

    func run() {
        guard !running, let model, let client = model.client else { return }
        guard let next = all().first(where: { $0.state == "queued" || $0.state == "preparing" }),
              next.address == model.currentAddress else {
            refreshExtras()
            return
        }
        // Downloads from another server or account wait until it's open again.
        running = true
        Task {
            defer { running = false }
            do {
                try await prepareAndStart(next, client: client)
            } catch {
                next.state = "failed"
                next.error = errorText(error)
                save()
                running = false
                run()
            }
        }
    }

    private func prepareAndStart(_ d: Download, client: ServerClient) async throws {
        let encoded = ServerClient.encodePath(d.sourcePath)
        struct Options: Decodable { let size: Int64?; let duration: Double?; let width: Int?; let height: Int? }
        let options: Options? = try? await client.get("/api/downloads/options", query: [("path", encoded)])
        if let o = options { d.duration = o.duration ?? 0; if o.size ?? 0 > 0, d.isOriginal { d.bytesTotal = o.size! } }
        var keep: OfflinePlayback?
        if d.compat, let codecs: CodecsInfo = try? await client.get("/api/media/codecs", query: [("path", encoded)]) {
            keep = OfflinePlayback.of(codecs, path: d.sourcePath)
            if keep?.plays == true { d.compat = false } // nothing to convert: the original
        }
        let url: URL
        if d.isOriginal {
            d.width = options?.width ?? 0
            d.height = options?.height ?? 0
            url = client.streamURL(for: d.sourcePath)
        } else {
            // The server encodes the copy first ("Preparing on the server").
            struct Create: Encodable { let path: String; let size_pct: Int; let scale: Int; let keep_video: Bool; let keep_audio: Bool }
            struct Job: Decodable { let id: String; let status: String?; let progress: Double?; let error: String?; let width: Int?; let height: Int? }
            while true {
                guard all().contains(where: { $0.id == d.id }) else { return } // removed meanwhile
                if d.jobId == nil {
                    let job: Job = try await client.call("POST", "/api/downloads", body: Create(
                        path: encoded, size_pct: d.sizePct, scale: d.scale, keep_video: keep?.video == true, keep_audio: keep?.audio == true))
                    d.jobId = job.id
                    save()
                }
                let job: Job
                do {
                    job = try await client.get("/api/downloads/\(d.jobId!)")
                } catch ServerError.http(404, _) {
                    d.jobId = nil // the server restarted and lost the job: start over
                    continue
                }
                if job.status == "ready" { d.width = job.width ?? 0; d.height = job.height ?? 0; break }
                if job.status == "failed" { throw ServerError.http(status: 500, message: job.error ?? "The server couldn't prepare the file") }
                d.state = "preparing"
                d.progress = job.progress ?? 0
                d.stage = job.status == "queued" ? "Waiting for the server" : "Preparing on the server"
                save()
                try await Task.sleep(for: .seconds(2))
            }
            url = URL(string: client.baseURL.absoluteString + "/api/downloads/\(d.jobId!)/file")!
        }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(client.token ?? "")", forHTTPHeaderField: "Authorization")
        let task = session.downloadTask(with: request)
        task.taskDescription = d.id
        d.state = "downloading"
        d.stage = nil
        d.progress = 0
        save()
        task.resume()
    }

    /// Download ahead: the episodes after one being watched, in the quality of the show's episodes
    /// downloaded by hand (else Compatibility), three of them, those not downloaded yet.
    func downloadAhead(after path: String) async {
        guard downloadAhead else { return }
        removeAheadBefore(path)
        guard let client = model?.client else { return }
        struct Upcoming: Decodable { let episodes: [ContinueItem]? }
        guard let found: Upcoming = try? await client.get("/api/media/upcoming", query: [("path", ServerClient.encodePath(path)), ("count", "3")]),
              let episodes = found.episodes, !episodes.isEmpty else { return }
        let showFolder = (path as NSString).deletingLastPathComponent
        let byHand = mine().last { !$0.ahead && ($0.sourcePath as NSString).deletingLastPathComponent == showFolder }
        add(episodes.map { (path: $0.path, name: $0.name, title: $0.metadata?.playerTitle, thumb: $0.thumbnailUrl, size: $0.size ?? 0) },
            sizePct: byHand?.sizePct ?? 100, scale: byHand?.scale ?? 100, compat: byHand?.compat ?? true, ahead: true)
    }

    // MARK: Extras (subtitles, skip times, trickplay) for playing offline

    @ObservationIgnored private var savingExtras = false

    /// Saves the extras of the open account's finished downloads that have none yet, or whose
    /// last save is over 12 hours old, one at a time. A server that can't be reached ends the round.
    func refreshExtras() {
        guard !savingExtras, let model, let client = model.client else { return }
        let due = mine().filter { $0.isDone && ($0.extrasAt.map { Date().timeIntervalSince($0) > DownloadExtras.refreshInterval } ?? true) }
        guard !due.isEmpty else { return }
        savingExtras = true
        Task {
            defer { savingExtras = false }
            for d in due {
                let id = d.id
                do {
                    let complete = try await DownloadExtras.save(id: id, sourcePath: d.sourcePath, thumbnail: d.thumbnailUrl, client: client) {
                        await MainActor.run { self.all().contains { $0.id == id } }
                    }
                    guard all().contains(where: { $0.id == id }) else { continue }
                    if complete { d.extrasAt = Date() }
                    save()
                } catch {
                    return // the server is away: next time
                }
            }
        }
    }

    /// A subtitle was added to a video: its downloads save their tracks again now.
    func extrasChanged(_ sourcePath: String) {
        let matching = all().filter { $0.sourcePath == sourcePath && $0.isDone }
        guard !matching.isEmpty else { return }
        matching.forEach { $0.extrasAt = nil }
        save()
        refreshExtras()
    }

    /// The finished download playing from a file on this phone.
    func download(atFile url: URL) -> Download? {
        all().first { $0.localURL?.standardizedFileURL == url.standardizedFileURL }
    }

    /// The downloaded episode after `d` in the same folder (by name, as episodes sort), for going
    /// on offline.
    func nextDownloaded(after d: Download) -> Download? {
        let folder = (d.sourcePath as NSString).deletingLastPathComponent
        return all()
            .filter { $0.address == d.address && $0.account == d.account && $0.isDone && $0.localURL != nil }
            .filter { ($0.sourcePath as NSString).deletingLastPathComponent == folder && $0.sourcePath.localizedStandardCompare(d.sourcePath) == .orderedDescending }
            .min { $0.sourcePath.localizedStandardCompare($1.sourcePath) == .orderedAscending }
    }

    /// Episodes downloaded ahead that come before the one now playing (same show folder, by name)
    /// have been watched past: they go. Ones downloaded by hand never do.
    func removeAheadBefore(_ path: String) {
        let folder = (path as NSString).deletingLastPathComponent
        for d in mine() where d.ahead && d.sourcePath != path
            && (d.sourcePath as NSString).deletingLastPathComponent == folder
            && d.sourcePath.localizedStandardCompare(path) == .orderedAscending {
            remove(d)
        }
    }

    private func save() {
        try? model?.context.save()
        bump()
    }

    private func bump() { version += 1 }

    private func download(for task: URLSessionTask) -> Download? {
        all().first { $0.id == task.taskDescription }
    }
}

extension DownloadManager: URLSessionDownloadDelegate {
    nonisolated func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didWriteData bytesWritten: Int64,
                                totalBytesWritten: Int64, totalBytesExpectedToWrite: Int64) {
        MainActor.assumeIsolated {
            guard let d = download(for: downloadTask) else { return }
            d.bytesDone = totalBytesWritten
            if totalBytesExpectedToWrite > 0 { d.bytesTotal = totalBytesExpectedToWrite }
            d.progress = d.bytesTotal > 0 ? Double(totalBytesWritten) / Double(d.bytesTotal) : 0
            bump()
        }
    }

    nonisolated func urlSession(_ session: URLSession, downloadTask: URLSessionDownloadTask, didFinishDownloadingTo location: URL) {
        // The file has to be moved before this returns.
        let id = downloadTask.taskDescription ?? UUID().uuidString
        let status = (downloadTask.response as? HTTPURLResponse)?.statusCode ?? 0
        let ext = downloadTask.originalRequest?.url?.absoluteString.contains("/api/downloads/") == true ? "mp4" : nil
        let staged = DownloadManager.folder.appendingPathComponent("\(id).partial")
        try? FileManager.default.removeItem(at: staged)
        let moved = (try? FileManager.default.moveItem(at: location, to: staged)) != nil
        MainActor.assumeIsolated {
            guard let d = download(for: downloadTask) else { try? FileManager.default.removeItem(at: staged); return }
            guard moved, (200..<300).contains(status) else {
                d.state = "failed"
                d.error = status == 401 ? "Sign in again to download." : "Download refused (\(status))"
                try? FileManager.default.removeItem(at: staged)
                save()
                return
            }
            let name = "\(d.id).\(ext ?? ((d.name as NSString).pathExtension.isEmpty ? "mp4" : (d.name as NSString).pathExtension.lowercased()))"
            let final = DownloadManager.folder.appendingPathComponent(name)
            try? FileManager.default.removeItem(at: final)
            try? FileManager.default.moveItem(at: staged, to: final)
            d.fileName = name
            d.state = "done"
            d.progress = 1
            d.bytesTotal = (try? FileManager.default.attributesOfItem(atPath: final.path)[.size] as? Int64) ?? d.bytesTotal
            save()
        }
    }

    nonisolated func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        MainActor.assumeIsolated {
            if let error, let d = download(for: task), d.state != "done" {
                if (error as NSError).code != NSURLErrorCancelled {
                    d.state = "failed"
                    d.error = error.localizedDescription
                }
                save()
            }
            run()
        }
    }

    nonisolated func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        MainActor.assumeIsolated {
            backgroundCompletion?()
            backgroundCompletion = nil
        }
    }
}
