import AVFoundation
import UniformTypeIdentifiers

/// Originals (and Live Photo clips) from an HTTPS server, read through the app rather than by
/// AVPlayer itself. AVPlayer checks a server's certificate on its own and refuses a self-signed one;
/// the app's requests go through TrustPolicy, which accepts a saved server's kept certificate. So
/// the asset gets a made-up scheme, and AVPlayer asks this loader for each piece of the file it
/// wants: the loader fetches that byte range from the server over the app's session (with the
/// sign-in token) and hands it back. Nothing listens on a port. Plain-HTTP servers have no
/// certificate to check: AVPlayer reads those itself.
final class OriginalLoader: NSObject, AVAssetResourceLoaderDelegate, @unchecked Sendable {
    static let scheme = "wsoriginal"

    private let token: String?
    private var tasks: [ObjectIdentifier: URLSessionDataTask] = [:] // on OriginalFetcher's queue

    private init(token: String?) {
        self.token = token
    }

    /// The asset for a server URL (`client.streamURL`). An HTTPS one is read through a loader,
    /// which the asset keeps for as long as it lives.
    static func asset(for url: URL, token: String?) -> AVURLAsset {
        guard url.scheme?.lowercased() == "https",
              var parts = URLComponents(url: url, resolvingAgainstBaseURL: false) else {
            let headers = ["Authorization": "Bearer \(token ?? "")"]
            return AVURLAsset(url: url, options: ["AVURLAssetHTTPHeaderFieldsKey": headers])
        }
        parts.scheme = scheme
        let asset = AVURLAsset(url: parts.url ?? url)
        let loader = OriginalLoader(token: token)
        asset.resourceLoader.setDelegate(loader, queue: OriginalFetcher.shared.queue)
        // The resource loader holds its delegate weakly: the asset keeps it instead.
        objc_setAssociatedObject(asset, &loaderKey, loader, .OBJC_ASSOCIATION_RETAIN)
        return asset
    }

    func resourceLoader(_ resourceLoader: AVAssetResourceLoader,
                        shouldWaitForLoadingOfRequestedResource loadingRequest: AVAssetResourceLoadingRequest) -> Bool {
        guard let asked = loadingRequest.request.url,
              var parts = URLComponents(url: asked, resolvingAgainstBaseURL: false),
              parts.scheme == Self.scheme else { return false }
        parts.scheme = "https"
        guard let url = parts.url else { return false }

        // The bytes AVPlayer wants; a request for the file's type and length alone asks for two.
        var start: Int64 = 0
        var length: Int64? = 2
        if let data = loadingRequest.dataRequest {
            start = data.requestedOffset
            length = data.requestsAllDataToEndOfResource ? nil : Int64(data.requestedLength)
        }
        let window = ByteWindow(start: start, length: length)
        var request = URLRequest(url: url)
        if let token { request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        request.setValue(window.rangeHeader, forHTTPHeaderField: "Range")

        let fetcher = OriginalFetcher.shared
        let task = fetcher.session.dataTask(with: request)
        let id = ObjectIdentifier(loadingRequest)
        tasks[id] = task
        fetcher.loads[task.taskIdentifier] = OriginalLoad(request: loadingRequest, url: url, window: window) { [weak self] in
            self?.tasks[id] = nil
        }
        task.resume()
        return true
    }

    func resourceLoader(_ resourceLoader: AVAssetResourceLoader, didCancel loadingRequest: AVAssetResourceLoadingRequest) {
        tasks.removeValue(forKey: ObjectIdentifier(loadingRequest))?.cancel()
    }
}

nonisolated(unsafe) private var loaderKey: UInt8 = 0

/// The bytes of a file one request asks for, and which of those a response carries: the Range
/// header to send, and as data arrives, the part to hand on. A server that ignores the range
/// (200, the whole file) has the bytes before `start` dropped.
nonisolated struct ByteWindow {
    let start: Int64
    let length: Int64? // nil: to the end of the file
    private var skip: Int64 = 0
    private(set) var delivered: Int64 = 0

    init(start: Int64, length: Int64?) {
        self.start = start
        self.length = length
    }

    var rangeHeader: String { length.map { "bytes=\(start)-\(start + $0 - 1)" } ?? "bytes=\(start)-" }

    /// The response is the whole file from its first byte (a 200), not the range asked for.
    mutating func wholeFile() { skip = start }

    /// The part of `data` that's wanted, and whether that completes the window.
    mutating func take(_ data: Data) -> (data: Data, done: Bool) {
        var data = data
        if skip > 0 {
            let dropped = min(skip, Int64(data.count))
            data = data.dropFirst(Int(dropped))
            skip -= dropped
        }
        if let length {
            let wanted = max(length - delivered, 0)
            if Int64(data.count) > wanted { data = data.prefix(Int(wanted)) }
        }
        delivered += Int64(data.count)
        return (data, length.map { delivered >= $0 } ?? false)
    }

    /// The file's length from a Content-Range header ("bytes 0-1/123456"); nil when it's not
    /// given ("bytes 0-1/*") or there's none.
    static func totalLength(contentRange: String?) -> Int64? {
        contentRange?.split(separator: "/").last.flatMap { Int64($0) }
    }
}

/// One piece of a file AVPlayer asked for, as it arrives.
private final class OriginalLoad {
    let request: AVAssetResourceLoadingRequest
    let url: URL
    let done: () -> Void
    private var window: ByteWindow

    init(request: AVAssetResourceLoadingRequest, url: URL, window: ByteWindow, done: @escaping () -> Void) {
        self.request = request
        self.url = url
        self.window = window
        self.done = done
    }

    func received(_ response: URLResponse) -> URLSession.ResponseDisposition {
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            let status = (response as? HTTPURLResponse)?.statusCode ?? 0
            finish(ServerError.http(status: status, message: nil))
            return .cancel
        }
        if http.statusCode == 200 { window.wholeFile() }
        if let info = request.contentInformationRequest {
            let mime = http.mimeType ?? ""
            info.contentType = UTType(mimeType: mime)?.identifier ?? AVFileType.mp4.rawValue
            info.isByteRangeAccessSupported = http.statusCode == 206
            // The file's length: Content-Range's total; a 200 gives it as its own length.
            if let total = ByteWindow.totalLength(contentRange: http.value(forHTTPHeaderField: "Content-Range")) {
                info.contentLength = total
            } else if http.statusCode == 200, http.expectedContentLength > 0 {
                info.contentLength = http.expectedContentLength
            }
        }
        if request.dataRequest == nil {
            finish(nil)
            return .cancel
        }
        return .allow
    }

    /// The bytes that came, handed on; true once all that was asked for has been.
    func received(_ data: Data) -> Bool {
        let (wanted, done) = window.take(data)
        if !wanted.isEmpty { request.dataRequest?.respond(with: wanted) }
        if done { finish(nil) }
        return done
    }

    func completed(_ error: Error?) {
        if let error = error as? URLError, error.code == .cancelled { return finish(nil, cancelled: true) }
        if error != nil, TrustPolicy.shared.certificateChanged(url) {
            return finish(ServerError.certificateChanged(url.host ?? "The server"))
        }
        finish(error)
    }

    private var finished = false
    private func finish(_ error: Error?, cancelled: Bool = false) {
        guard !finished else { return }
        finished = true
        done()
        guard !cancelled, !request.isCancelled, !request.isFinished else { return }
        if let error { request.finishLoading(with: error) } else { request.finishLoading() }
    }
}

/// The one session originals are fetched with: TrustPolicy for certificates, and every callback
/// (the loaders' too) on one queue, so the loads need no locks.
private final class OriginalFetcher: NSObject, URLSessionDataDelegate, @unchecked Sendable {
    static let shared = OriginalFetcher()
    let queue = DispatchQueue(label: "weirdstreamer.originals")
    var loads: [Int: OriginalLoad] = [:] // by task identifier, on queue
    private(set) var session: URLSession!

    override init() {
        super.init()
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 60
        let callbacks = OperationQueue()
        callbacks.underlyingQueue = queue
        callbacks.maxConcurrentOperationCount = 1
        session = URLSession(configuration: config, delegate: self, delegateQueue: callbacks)
    }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        TrustPolicy.shared.urlSession(session, didReceive: challenge, completionHandler: completionHandler)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
                    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void) {
        completionHandler(loads[dataTask.taskIdentifier]?.received(response) ?? .cancel)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        if loads[dataTask.taskIdentifier]?.received(data) == true { dataTask.cancel() }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        loads.removeValue(forKey: task.taskIdentifier)?.completed(error)
    }
}
