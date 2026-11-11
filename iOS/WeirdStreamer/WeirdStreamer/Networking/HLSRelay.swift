import Foundation
import Network

// Transcodes from the server are fragmented MP4 of unknown length, sent as they're made. Android's
// ExoPlayer plays that as it comes; AVPlayer can't (a progressive stream needs a known length).
// So the app relays a transcode as HLS, which AVPlayer plays live: it reads the server's stream,
// cuts it at its fragments (ffmpeg's frag_keyframe: each starts on a keyframe), keeps them in a
// temporary folder, and serves an EVENT playlist that grows as they arrive, from a small HTTP
// server on 127.0.0.1. The folder is deleted when the player closes (and at the next launch).

/// One transcode being relayed: the server's stream in, an HLS playlist and segments out.
final class HLSRelay: NSObject, URLSessionDataDelegate, @unchecked Sendable {
    let id = UUID().uuidString
    private let source: URL
    private let token: String?
    private let folder: URL
    /// The video's length from where the stream starts, to time the last segment.
    private let expectedDuration: Double

    private let lock = NSLock()
    private var buffer = Data()
    private var initSegment: Data?
    private var pendingMoof: Data?
    private var timescale: UInt32 = 0
    private var videoTrack: UInt32 = 0
    private var fragments: [(file: URL, start: UInt64)] = []   // written, waiting for the next one's start
    private var segments: [(file: URL, duration: Double)] = []
    private var finished = false
    private var failure: Error?
    private var task: URLSessionDataTask?
    private var session: URLSession?

    init(source: URL, token: String?, expectedDuration: Double) {
        self.source = source
        self.token = token
        self.expectedDuration = expectedDuration
        self.folder = HLSRelay.root.appendingPathComponent("hls-\(id)", isDirectory: true)
        super.init()
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    }

    static let root = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]

    /// Removes relay folders a closed player (or a killed app) left behind.
    static func sweep() {
        let items = (try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: nil)) ?? []
        for item in items where item.lastPathComponent.hasPrefix("hls-") { try? FileManager.default.removeItem(at: item) }
    }

    func start() {
        var request = URLRequest(url: source)
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        request.timeoutInterval = 120
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 120
        config.timeoutIntervalForResource = 24 * 3600
        let session = URLSession(configuration: config, delegate: self, delegateQueue: nil)
        self.session = session
        let task = session.dataTask(with: request)
        self.task = task
        task.resume()
    }

    func stop() {
        task?.cancel()
        session?.invalidateAndCancel()
        session = nil
        try? FileManager.default.removeItem(at: folder)
    }

    // MARK: What the player asks for

    var hasSegments: Bool { lock.withLock { !segments.isEmpty || failure != nil || finished } }
    var failed: Error? { lock.withLock { failure } }

    func playlist() -> String {
        lock.withLock {
            let longest = segments.map(\.duration).max() ?? 10
            var text = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:\(max(10, Int(longest.rounded(.up))))\n"
            text += "#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MEDIA-SEQUENCE:0\n"
            text += "#EXT-X-MAP:URI=\"init.mp4\"\n"
            for (i, segment) in segments.enumerated() {
                text += String(format: "#EXTINF:%.3f,\nseg%d.m4s\n", segment.duration, i)
            }
            if finished { text += "#EXT-X-ENDLIST\n" }
            return text
        }
    }

    func initData() -> Data? { lock.withLock { initSegment } }

    func segment(_ index: Int) -> Data? {
        let file: URL? = lock.withLock { index < segments.count ? segments[index].file : nil }
        return file.flatMap { try? Data(contentsOf: $0) }
    }

    // MARK: Reading the server's stream

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
                    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void) {
        if let http = response as? HTTPURLResponse, !(200..<300).contains(http.statusCode) {
            lock.withLock { failure = ServerError.http(status: http.statusCode, message: nil) }
            completionHandler(.cancel)
            return
        }
        completionHandler(.allow)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        lock.withLock {
            buffer.append(data)
            parseBoxes()
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        lock.withLock {
            if let error, (error as NSError).code != NSURLErrorCancelled { failure = failure ?? error }
            // The last fragment: its length is what's left of the video.
            if let last = fragments.last {
                let done = segments.reduce(0) { $0 + $1.duration }
                segments.append((last.file, max(0.5, expectedDuration - done)))
                fragments.removeAll()
            }
            finished = true
        }
    }

    /// Takes whole boxes off the front of the buffer: ftyp and moov make the init segment, each
    /// moof with its mdat a fragment.
    private func parseBoxes() {
        var offset = 0
        while buffer.count - offset >= 8 {
            let start = buffer.startIndex + offset
            var size = Int(buffer.readUInt32(at: start))
            let type = String(decoding: buffer[(start + 4)..<(start + 8)], as: UTF8.self)
            if size == 1 {
                guard buffer.count - offset >= 16 else { break }
                size = Int(buffer.readUInt64(at: start + 8))
            }
            guard size >= 8, buffer.count - offset >= size else { break }
            let box = Data(buffer[start..<(start + size)])
            offset += size
            switch type {
            case "ftyp":
                // iso5: the fragments' data is placed from their own start (default-base-is-moof).
                initSegment = HLSRelay.isoFtyp
            case "moov":
                initSegment = (initSegment ?? Data()) + box
                readTracks(moov: box)
            case "moof":
                pendingMoof = box
            case "mdat":
                if let moof = pendingMoof { addFragment(moof: moof, mdat: box) }
                pendingMoof = nil
            default:
                break // styp, sidx, free: not needed
            }
        }
        if offset > 0 { buffer.removeFirst(offset) }
    }

    /// The video track and its timescale, to time the fragments.
    private func readTracks(moov: Data) {
        for trak in Box.children(of: moov, from: 8) where trak.type == "trak" {
            var trackID: UInt32 = 0
            var scale: UInt32 = 0
            var isVideo = false
            for child in Box.children(of: moov, range: trak.content) {
                if child.type == "tkhd" {
                    let version = moov[child.content.lowerBound]
                    trackID = moov.readUInt32(at: child.content.lowerBound + (version == 1 ? 20 : 12))
                } else if child.type == "mdia" {
                    for item in Box.children(of: moov, range: child.content) {
                        if item.type == "mdhd" {
                            let version = moov[item.content.lowerBound]
                            scale = moov.readUInt32(at: item.content.lowerBound + (version == 1 ? 20 : 12))
                        } else if item.type == "hdlr" {
                            isVideo = String(decoding: moov[(item.content.lowerBound + 8)..<(item.content.lowerBound + 12)], as: UTF8.self) == "vide"
                        }
                    }
                }
            }
            if isVideo || videoTrack == 0 {
                videoTrack = trackID
                timescale = scale
                if isVideo { break }
            }
        }
    }

    static let isoFtyp: Data = {
        var body = Data("iso5".utf8)
        body.appendUInt32(512)
        body.append(Data("iso5iso6mp41".utf8))
        var box = Data()
        box.appendUInt32(UInt32(8 + body.count))
        box.append(Data("ftyp".utf8))
        return box + body
    }()

    /// ffmpeg points each fragment's data at its place in the whole file (tfhd's base data
    /// offset); AVPlayer's HLS wants it placed from the fragment's own start instead
    /// (default-base-is-moof). The offset is taken out, and the data offsets move up by as much.
    private static func rebase(_ moof: Data) -> Data {
        var children: [(type: String, data: Data)] = []
        var shrink = 0
        for child in Box.children(of: moof, from: 8) {
            let whole = Data(moof[(child.content.lowerBound - 8)..<child.content.upperBound])
            guard child.type == "traf" else { children.append((child.type, whole)); continue }
            var traf = Data()
            for item in Box.children(of: moof, range: child.content) {
                var box = Data(moof[(item.content.lowerBound - 8)..<item.content.upperBound])
                if item.type == "tfhd" {
                    let word = box.readUInt32(at: 8)
                    let flags = word & 0xFFFFFF
                    if flags & 0x1 != 0 {
                        let body = Data(box[12..<16]) + Data(box[24..<box.count])
                        var rebuilt = Data()
                        rebuilt.appendUInt32(UInt32(12 + body.count))
                        rebuilt.append(Data("tfhd".utf8))
                        rebuilt.appendUInt32((word & 0xFF000000) | ((flags & ~0x1) | 0x20000))
                        rebuilt.append(body)
                        box = rebuilt
                        shrink += 8
                    }
                }
                traf.append(box)
            }
            children.append(("traf", traf))
        }
        var body = Data()
        for child in children {
            guard child.type == "traf" else { body.append(child.data); continue }
            var traf = child.data
            for item in Box.children(of: traf, from: 0) where item.type == "trun" {
                let at = item.content.lowerBound
                if traf.readUInt32(at: at) & 0x1 != 0 {
                    let offset = Int32(bitPattern: traf.readUInt32(at: at + 8))
                    traf.writeUInt32(UInt32(bitPattern: offset - Int32(shrink)), at: at + 8)
                }
            }
            body.appendUInt32(UInt32(8 + traf.count))
            body.append(Data("traf".utf8))
            body.append(traf)
        }
        var result = Data()
        result.appendUInt32(UInt32(8 + body.count))
        result.append(Data("moof".utf8))
        return result + body
    }

    private func addFragment(moof: Data, mdat: Data) {
        var decodeTime: UInt64 = 0
        for traf in Box.children(of: moof, from: 8) where traf.type == "traf" {
            var track: UInt32 = 0
            var time: UInt64?
            for child in Box.children(of: moof, range: traf.content) {
                if child.type == "tfhd" {
                    track = moof.readUInt32(at: child.content.lowerBound + 4)
                } else if child.type == "tfdt" {
                    let at = child.content.lowerBound
                    time = moof[at] == 1 ? moof.readUInt64(at: at + 4) : UInt64(moof.readUInt32(at: at + 4))
                }
            }
            if track == videoTrack, let time { decodeTime = time }
        }
        let file = folder.appendingPathComponent("frag-\(fragments.count + segments.count).m4s")
        do { try (HLSRelay.rebase(moof) + mdat).write(to: file) } catch { failure = error; return }
        // The fragment before this one now has a length: up to where this one starts.
        if let previous = fragments.popLast() {
            let seconds = timescale > 0 ? Double(decodeTime &- previous.start) / Double(timescale) : 2
            segments.append((previous.file, max(0.04, seconds)))
        }
        fragments.append((file, decodeTime))
    }
}

/// A box (atom) in an MP4: its type and where its content is.
private struct Box {
    let type: String
    let content: Range<Int>

    static func children(of data: Data, from start: Int) -> [Box] {
        children(of: data, range: (data.startIndex + start)..<data.endIndex)
    }

    static func children(of data: Data, range: Range<Int>) -> [Box] {
        var result: [Box] = []
        var at = range.lowerBound
        while at + 8 <= range.upperBound {
            let size = Int(data.readUInt32(at: at))
            guard size >= 8, at + size <= range.upperBound else { break }
            let type = String(decoding: data[(at + 4)..<(at + 8)], as: UTF8.self)
            result.append(Box(type: type, content: (at + 8)..<(at + size)))
            at += size
        }
        return result
    }
}

private extension Data {
    func readUInt32(at i: Int) -> UInt32 {
        UInt32(self[i]) << 24 | UInt32(self[i + 1]) << 16 | UInt32(self[i + 2]) << 8 | UInt32(self[i + 3])
    }
    func readUInt64(at i: Int) -> UInt64 {
        UInt64(readUInt32(at: i)) << 32 | UInt64(readUInt32(at: i + 4))
    }
    mutating func writeUInt32(_ value: UInt32, at i: Int) {
        for k in 0..<4 { self[i + k] = UInt8((value >> (24 - 8 * UInt32(k))) & 0xFF) }
    }
    mutating func appendUInt32(_ value: UInt32) {
        append(contentsOf: [UInt8(value >> 24), UInt8((value >> 16) & 0xFF), UInt8((value >> 8) & 0xFF), UInt8(value & 0xFF)])
    }
}

/// The local HTTP server AVPlayer reads relayed transcodes from: /<relay id>/index.m3u8,
/// init.mp4 and seg<N>.m4s, on 127.0.0.1 only.
final class HLSServer: @unchecked Sendable {
    static let shared = HLSServer()

    private var listener: NWListener?
    private var relays: [String: HLSRelay] = [:]
    private let lock = NSLock()
    private let queue = DispatchQueue(label: "hls-server")
    private(set) var port: UInt16 = 0

    /// Starts the server (once) and returns its port.
    func start() async throws -> UInt16 {
        if port != 0 { return port }
        let params = NWParameters.tcp
        params.requiredLocalEndpoint = NWEndpoint.hostPort(host: "127.0.0.1", port: .any)
        let listener = try NWListener(using: params)
        self.listener = listener
        listener.newConnectionHandler = { [weak self] connection in self?.serve(connection) }
        return try await withCheckedThrowingContinuation { continuation in
            var resumed = false
            listener.stateUpdateHandler = { [weak self] state in
                guard !resumed else { return }
                switch state {
                case .ready:
                    resumed = true
                    self?.port = listener.port?.rawValue ?? 0
                    continuation.resume(returning: self?.port ?? 0)
                case .failed(let error):
                    resumed = true
                    continuation.resume(throwing: error)
                default:
                    break
                }
            }
            listener.start(queue: queue)
        }
    }

    func add(_ relay: HLSRelay) { lock.withLock { relays[relay.id] = relay } }

    func remove(_ relay: HLSRelay) {
        _ = lock.withLock { relays.removeValue(forKey: relay.id) }
        relay.stop()
    }

    func playlistURL(for relay: HLSRelay) -> URL {
        URL(string: "http://127.0.0.1:\(port)/\(relay.id)/index.m3u8")!
    }

    private func serve(_ connection: NWConnection) {
        connection.start(queue: queue)
        connection.receive(minimumIncompleteLength: 1, maximumLength: 16384) { [weak self] data, _, _, _ in
            guard let self, let data, let request = String(data: data, encoding: .utf8),
                  let line = request.split(separator: "\r\n").first else { connection.cancel(); return }
            let parts = line.split(separator: " ")
            guard parts.count >= 2 else { connection.cancel(); return }
            let path = String(parts[1].split(separator: "?").first ?? "")
            let pieces = path.split(separator: "/").map(String.init)
            guard pieces.count == 2, let relay = self.lock.withLock({ self.relays[pieces[0]] }) else {
                self.respond(connection, status: "404 Not Found", type: "text/plain", body: Data())
                return
            }
            Task.detached { await self.answer(connection, relay: relay, name: pieces[1]) }
        }
    }

    private func answer(_ connection: NWConnection, relay: HLSRelay, name: String) async {
        switch name {
        case "index.m3u8":
            // The first answer waits for a segment (up to a minute): an empty playlist is an error to AVPlayer.
            var waited = 0
            while !relay.hasSegments && waited < 600 {
                try? await Task.sleep(for: .milliseconds(100))
                waited += 1
            }
            if relay.failed != nil && relay.playlist().contains("#EXTINF") == false {
                respond(connection, status: "502 Bad Gateway", type: "text/plain", body: Data())
                return
            }
            respond(connection, status: "200 OK", type: "application/vnd.apple.mpegurl", body: Data(relay.playlist().utf8))
        case "init.mp4":
            if let data = relay.initData() {
                respond(connection, status: "200 OK", type: "video/mp4", body: data)
            } else {
                respond(connection, status: "404 Not Found", type: "text/plain", body: Data())
            }
        default:
            if name.hasPrefix("seg"), let index = Int(name.dropFirst(3).dropLast(4)), let data = relay.segment(index) {
                respond(connection, status: "200 OK", type: "video/iso.segment", body: data)
            } else {
                respond(connection, status: "404 Not Found", type: "text/plain", body: Data())
            }
        }
    }

    private func respond(_ connection: NWConnection, status: String, type: String, body: Data) {
        let head = "HTTP/1.1 \(status)\r\nContent-Type: \(type)\r\nContent-Length: \(body.count)\r\nCache-Control: no-cache\r\nConnection: close\r\n\r\n"
        connection.send(content: Data(head.utf8) + body, completion: .contentProcessed { _ in connection.cancel() })
    }
}
