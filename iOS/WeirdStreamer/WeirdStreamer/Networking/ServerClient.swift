import CryptoKit
import Foundation

enum ServerError: LocalizedError {
    case badAddress
    case http(status: Int, message: String?)
    case unauthorized
    case decoding(String)
    case transport(String)
    case certificateChanged(String)

    var errorDescription: String? {
        switch self {
        case .badAddress: return "That server address isn't valid."
        case .http(_, let message): return message ?? "The server returned an error."
        case .unauthorized: return "Your sign-in has ended. Sign in again."
        case .decoding(let detail): return "Couldn't read the server's reply (\(detail))."
        case .transport(let detail): return "Couldn't reach the server (\(detail))."
        case .certificateChanged(let host):
            return "\(host)'s certificate has changed since it was added, so the app won't connect: someone may be "
                + "posing as the server. If you replaced its certificate, remove the server and add it again."
        }
    }
}

/// Certificates, as on Android (ServerTrust): home servers often have self-signed certificates, so
/// a saved server's is trusted as it is, but only that one. The certificate it shows when it's added
/// (or, for a server saved before this, at its first connection) is kept by its SHA-256, and a
/// different one is refused (ServerError.certificateChanged). Removing the server forgets it. Every
/// other host is checked as usual. Keyed by host and port.
final class TrustPolicy: NSObject, URLSessionDelegate, @unchecked Sendable {
    static let shared = TrustPolicy()
    private let lock = NSLock()
    private var savedServers: Set<String> = [] // host:port of the servers clients were made for
    private var seen: [String: String] = [:]   // what probes of addresses not saved yet were shown
    private var changed: Set<String> = []      // servers whose certificate was refused
    private static let pinsKey = "serverCertificates"

    static func key(host: String, port: Int) -> String { "\(host.lowercased()):\(port)" }

    static func key(of url: URL) -> String? {
        guard url.scheme?.lowercased() == "https", let host = url.host else { return nil }
        return key(host: host, port: url.port ?? 443)
    }

    private var pins: [String: String] {
        UserDefaults.standard.dictionary(forKey: Self.pinsKey) as? [String: String] ?? [:]
    }

    private func setPin(_ fingerprint: String?, for key: String) {
        var all = pins
        all[key] = fingerprint
        UserDefaults.standard.set(all, forKey: Self.pinsKey)
    }

    /// A saved server: its certificate is kept at its first connection if it has none kept yet.
    func trust(_ url: URL) {
        guard let key = Self.key(of: url) else { return }
        lock.lock(); defer { lock.unlock() }
        savedServers.insert(key)
    }

    /// What an address check was shown, for keepOnAdd.
    func remember(_ key: String, _ fingerprint: String) {
        lock.lock(); defer { lock.unlock() }
        seen[key] = fingerprint
    }

    /// Keeps the certificate a newly added server showed when it was checked.
    func keepOnAdd(_ address: String) {
        guard let url = URL(string: address), let key = Self.key(of: url) else { return }
        lock.lock(); defer { lock.unlock() }
        savedServers.insert(key)
        if pins[key] == nil, let fingerprint = seen.removeValue(forKey: key) { setPin(fingerprint, for: key) }
    }

    /// Forgets a removed server's certificate, so adding it again keeps the one it has then.
    func forget(_ address: String) {
        guard let url = URL(string: address), let key = Self.key(of: url) else { return }
        lock.lock(); defer { lock.unlock() }
        setPin(nil, for: key)
        savedServers.remove(key)
        changed.remove(key)
    }

    /// Whether a request to url failed because its server's certificate changed.
    func certificateChanged(_ url: URL) -> Bool {
        guard let key = Self.key(of: url) else { return false }
        lock.lock(); defer { lock.unlock() }
        return changed.contains(key)
    }

    static func fingerprint(_ trust: SecTrust) -> String? {
        guard let chain = SecTrustCopyCertificateChain(trust) as? [SecCertificate], let leaf = chain.first else { return nil }
        return SHA256.hash(data: SecCertificateCopyData(leaf) as Data).map { String(format: "%02x", $0) }.joined()
    }

    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        let space = challenge.protectionSpace
        guard space.authenticationMethod == NSURLAuthenticationMethodServerTrust, let trust = space.serverTrust,
              let fingerprint = Self.fingerprint(trust) else {
            completionHandler(.performDefaultHandling, nil)
            return
        }
        let key = Self.key(host: space.host, port: space.port)
        lock.lock()
        var accept: Bool? // nil: checked as usual
        if let pinned = pins[key] {
            accept = pinned == fingerprint
            if pinned == fingerprint { changed.remove(key) } else { changed.insert(key) }
        } else if savedServers.contains(key) {
            setPin(fingerprint, for: key) // first connection of a server saved before certificates were kept
            accept = true
        }
        lock.unlock()
        switch accept {
        case true?: completionHandler(.useCredential, URLCredential(trust: trust))
        case false?: completionHandler(.cancelAuthenticationChallenge, nil)
        case nil: completionHandler(.performDefaultHandling, nil)
        }
    }
}

/// The address probe's session: short timeouts, any certificate (it only reads /api/info), which
/// it remembers for TrustPolicy.keepOnAdd.
private let probeSession: URLSession = {
    let config = URLSessionConfiguration.ephemeral
    config.timeoutIntervalForRequest = 8 // a phone network can be slow to connect
    config.timeoutIntervalForResource = 10
    let delegate = ProbeTrust()
    return URLSession(configuration: config, delegate: delegate, delegateQueue: nil)
}()

private final class ProbeTrust: NSObject, URLSessionDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, didReceive challenge: URLAuthenticationChallenge,
                    completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void) {
        let space = challenge.protectionSpace
        if let trust = space.serverTrust {
            if let fingerprint = TrustPolicy.fingerprint(trust) {
                TrustPolicy.shared.remember(TrustPolicy.key(host: space.host, port: space.port), fingerprint)
            }
            completionHandler(.useCredential, URLCredential(trust: trust))
        } else {
            completionHandler(.performDefaultHandling, nil)
        }
    }
}

/// One WeirdStreamer server's REST API, as the Android app uses it. Paths are the server's own
/// "client paths" (`<library ID>/<path below the library>`); requests send them base64-encoded.
final class ServerClient: Sendable {
    let baseURL: URL
    let token: String?
    let session: URLSession

    init(address: String, token: String?) throws {
        guard let url = URL(string: address), url.host != nil else { throw ServerError.badAddress }
        self.baseURL = url
        self.token = token
        TrustPolicy.shared.trust(url)
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 30
        self.session = URLSession(configuration: config, delegate: TrustPolicy.shared, delegateQueue: nil)
    }

    /// The ports tried for an address without one, in order (as Android).
    static let probePorts = [9000, 443, 80, 8080]

    /// A server address as typed, made into the one requests use, as Android's
    /// ServerSourceProvider.checkAddress: "myserver" becomes the first address a WeirdStreamer
    /// server answers on (ports 9000, 443, 80, 8080; https before http on each, all tried at once),
    /// "myserver:9100" tries https and http on that port, and an address with a scheme is used as typed.
    /// Nil when it isn't an address, or (without a scheme) nothing answered: a port isn't guessed.
    static func checkAddress(_ typed: String) async -> String? {
        var text = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        while text.hasSuffix("/") { text.removeLast() }
        guard !text.isEmpty else { return nil }
        if text.contains("://") { return normalized(text) }
        let candidates: [String]
        if hasPort(text) {
            candidates = ["https://\(text)", "http://\(text)"]
        } else {
            candidates = probePorts.flatMap { port -> [String] in
                switch port {
                case 443: return ["https://\(text)"]
                case 80: return ["http://\(text)"]
                default: return ["https://\(text):\(port)", "http://\(text):\(port)"]
                }
            }
        }
        let answers = await withTaskGroup(of: (Int, Probe).self) { group in
            for (i, base) in candidates.enumerated() { group.addTask { (i, await probe(base)) } }
            var found = [Probe](repeating: .none, count: candidates.count)
            for await (i, answer) in group { found[i] = answer }
            return found
        }
        // A WeirdStreamer server first; else anything that answered (a proxy hiding /api/info).
        if let i = answers.firstIndex(of: .weirdStreamer) ?? answers.firstIndex(of: .other) {
            return normalized(candidates[i])
        }
        return nil
    }

    private enum Probe { case weirdStreamer, other, none }

    private static func probe(_ base: String) async -> Probe {
        guard let url = URL(string: base + "/api/info"),
              let (data, response) = try? await probeSession.data(from: url) else { return .none }
        let ok = (response as? HTTPURLResponse).map { (200..<300).contains($0.statusCode) } ?? false
        let named = ok && ((try? JSONSerialization.jsonObject(with: data)) as? [String: Any])?["name"] != nil
        return named ? .weirdStreamer : .other
    }

    private static func hasPort(_ hostPort: String) -> Bool {
        let afterBracket = hostPort.components(separatedBy: "]").last ?? hostPort
        guard let colon = afterBracket.lastIndex(of: ":") else { return false }
        if !hostPort.hasPrefix("[") && hostPort.filter({ $0 == ":" }).count > 1 { return false } // bare IPv6
        let port = afterBracket[afterBracket.index(after: colon)...]
        return !port.isEmpty && port.allSatisfy(\.isNumber)
    }

    /// Lower-case scheme and host, no default port, no trailing slash ("Https://Host:443/" is "https://host").
    static func normalized(_ address: String) -> String? {
        guard var parts = URLComponents(string: address), let host = parts.host else { return nil }
        parts.scheme = parts.scheme?.lowercased()
        parts.host = host.lowercased()
        if (parts.scheme == "https" && parts.port == 443) || (parts.scheme == "http" && parts.port == 80) { parts.port = nil }
        var text = parts.string ?? address
        while text.hasSuffix("/") { text.removeLast() }
        return text
    }

    /// The name a server gives itself (`/api/info`, no sign-in needed), or nil.
    static func serverName(at address: String) async -> String? {
        guard let url = URL(string: address + "/api/info"),
              let (data, _) = try? await probeSession.data(from: url),
              let name = ((try? JSONSerialization.jsonObject(with: data)) as? [String: Any])?["name"] as? String else { return nil }
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        return trimmed.isEmpty ? nil : trimmed
    }

    // MARK: Paths and URLs

    /// The base64 form of a client path, as the server's `path` parameter takes it.
    static func encodePath(_ path: String) -> String {
        Data(path.utf8).base64EncodedString()
    }

    /// Escapes everything but letters and digits, so base64's `+`, `/` and `=` survive the query.
    static func escape(_ value: String) -> String {
        value.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? value
    }

    /// A stream the player opens. `transcode` asks the server to re-encode (for files AVPlayer
    /// can't play, such as MKV or AVI); otherwise the original is served as it is.
    func streamURL(for path: String, query: String = "") -> URL {
        var text = "\(baseURL.absoluteString)/api/media/stream?path=\(Self.escape(Self.encodePath(path)))&auto-transcode=0"
        if !query.isEmpty { text += "&" + query }
        return URL(string: text)!
    }

    /// A server-relative image path ("/artwork/…") as a full URL.
    func url(for relative: String?) -> URL? {
        guard let relative, !relative.isEmpty else { return nil }
        if relative.hasPrefix("http") { return URL(string: relative) }
        return URL(string: baseURL.absoluteString + relative)
    }

    // MARK: Requests

    private func request(_ method: String, _ path: String, query: [(String, String)] = [],
                         body: Data? = nil) throws -> URLRequest {
        var text = baseURL.absoluteString + path
        if !query.isEmpty {
            text += "?" + query.map { "\($0.0)=\(Self.escape($0.1))" }.joined(separator: "&")
        }
        guard let url = URL(string: text) else { throw ServerError.badAddress }
        var req = URLRequest(url: url)
        req.httpMethod = method
        if let token { req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        if let body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return req
    }

    private func perform(_ req: URLRequest) async throws -> Data {
        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            if TrustPolicy.shared.certificateChanged(baseURL) { throw ServerError.certificateChanged(baseURL.host ?? "The server") }
            throw ServerError.transport(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else { throw ServerError.transport("no reply") }
        switch http.statusCode {
        case 200..<300:
            return data
        case 401:
            throw ServerError.unauthorized
        default:
            let message = (try? JSONDecoder().decode(ErrorReply.self, from: data))?.message
            throw ServerError.http(status: http.statusCode, message: message)
        }
    }

    func get<T: Decodable>(_ path: String, query: [(String, String)] = []) async throws -> T {
        try decode(T.self, from: try await perform(try request("GET", path, query: query)))
    }

    /// A request with no body and no reply to read (such as DELETE).
    func execute(_ method: String, _ path: String, query: [(String, String)] = []) async throws {
        _ = try await perform(try request(method, path, query: query))
    }

    func send<B: Encodable>(_ method: String, _ path: String, body: B) async throws {
        let data = try JSONEncoder().encode(body)
        _ = try await perform(try request(method, path, body: data))
    }

    func call<B: Encodable, T: Decodable>(_ method: String, _ path: String, body: B) async throws -> T {
        let data = try JSONEncoder().encode(body)
        return try decode(T.self, from: try await perform(try request(method, path, body: data)))
    }

    /// A request whose body is sent with snake_case keys (for bodies that go back to the server
    /// as it sent them, such as a subtitle search result).
    func callSnake<B: Encodable, T: Decodable>(_ method: String, _ path: String, body: B) async throws -> T {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        return try decode(T.self, from: try await perform(try request(method, path, body: try encoder.encode(body))))
    }

    /// A request whose body and reply are plain JSON (the server's config, kept whole so settings
    /// this app doesn't edit go back untouched).
    func json(_ method: String, _ path: String, query: [(String, String)] = [], body: Any? = nil) async throws -> Any {
        let data = try body.map { try JSONSerialization.data(withJSONObject: $0) }
        let reply = try await perform(try request(method, path, query: query, body: data))
        return reply.isEmpty ? [:] : try JSONSerialization.jsonObject(with: reply)
    }

    /// The account the token is for, and whether it's an administrator (its claims, as Android reads them).
    var claims: (username: String?, isAdmin: Bool) {
        guard let token, token.split(separator: ".").count >= 2 else { return (nil, false) }
        var part = String(token.split(separator: ".")[1]).replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        while part.count % 4 != 0 { part += "=" }
        guard let data = Data(base64Encoded: part),
              let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else { return (nil, false) }
        return (object["username"] as? String, object["is_admin"] as? Bool ?? false)
    }

    /// Raw bytes (a .sup subtitle window, a WebVTT body).
    func data(_ path: String, query: [(String, String)]) async throws -> Data {
        try await perform(try request("GET", path, query: query))
    }

    /// A subset of the server's CJK subtitle font holding these characters (POST
    /// /api/media/subtitle-font): the font's bytes. Throws when the server has no CJK font.
    func subtitleFont(_ text: String) async throws -> Data {
        let body = try JSONSerialization.data(withJSONObject: ["text": text])
        return try await perform(try request("POST", "/api/media/subtitle-font", body: body))
    }

    /// Signs in a TV as this account with the code it shows (POST /api/pairing/claim). The code
    /// goes upper-case without spaces or hyphens. An unknown, expired or used code (404) and too
    /// many tries (429) throw `ServerError.http` with the server's message.
    func claimPairingCode(_ code: String) async throws {
        let normalized = code.uppercased().filter { $0 != " " && $0 != "-" }
        do {
            try await send("POST", "/api/pairing/claim", body: ["code": normalized])
        } catch ServerError.http(404, let message) {
            throw ServerError.http(status: 404, message: message ?? "That code isn't valid. Check the code on the TV.")
        } catch ServerError.http(429, let message) {
            throw ServerError.http(status: 429, message: message ?? "Too many tries. Wait a minute and try again.")
        }
    }

    /// Sign-in: returns the token and the user. Used with no token yet.
    func login(username: String, password: String) async throws -> LoginResponse {
        struct Body: Encodable { let username: String; let password: String }
        return try await call("POST", "/api/auth/login", body: Body(username: username, password: password))
    }

    /// Fetches an image with the token (the server requires it), for AsyncImage-style views.
    func image(at url: URL) async throws -> Data {
        var req = URLRequest(url: url)
        if let token { req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        return try await perform(req)
    }

    private func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        try Self.decode(type, from: data)
    }

    /// Reads a reply as the server sends it (snake_case keys), such as one saved with a download.
    static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        let decoder = JSONDecoder()
        decoder.keyDecodingStrategy = .convertFromSnakeCase
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw ServerError.decoding(String(describing: error).prefix(120).description)
        }
    }
}

private struct ErrorReply: Decodable { let error: String?; let message: String? }
