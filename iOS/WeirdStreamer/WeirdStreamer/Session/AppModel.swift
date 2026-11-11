import Foundation
import SwiftData
import Observation

// The app's state and actions, as the Android app's MediaViewModel: the source list (servers and
// their accounts), the open server and folder, Library view and title pages, Continue Watching
// and Recently Added, and what plays. Views read it from the environment.

/// A folder or file in a listing, as Android's FileItem.
struct FileEntry: Identifiable, Hashable {
    let name: String
    let path: String           // the client path ("<library ID>/<path below it>")
    let isDirectory: Bool
    var type: String? = nil    // video, image, document
    var size: Int64 = 0
    var duration: Double? = nil
    var thumbnailUrl: String? = nil
    var playback: Playback? = nil
    var liveVideo: String? = nil   // an Apple Live Photo's .MOV (client path)

    var id: String { path }
    var isVideo: Bool { type == "video" }
    var isImage: Bool { type == "image" }
    var isPdf: Bool { name.lowercased().hasSuffix(".pdf") }
}

/// Show in a folder: everything, or only videos, images or PDFs (the ⋮ menu's Show).
enum ContentFilter: String, CaseIterable {
    case all, video, image, pdf
    var label: String {
        switch self {
        case .all: return "Everything"
        case .video: return "Videos"
        case .image: return "Images"
        case .pdf: return "PDFs"
        }
    }
    var icon: String {
        switch self {
        case .all: return "line.3.horizontal.decrease"
        case .video: return "play.rectangle.on.rectangle"
        case .image: return "photo"
        case .pdf: return "doc.richtext"
        }
    }
}

/// The sign-in dialog: for a new account on a server ("Add user"), or one whose sign-in ran out.
struct AuthRequest: Identifiable {
    let id = UUID()
    let address: String
    let username: String?
    let title: String
}

/// An account on a server's row: its name, and whether its sign-in is still good here.
struct AccountRow: Identifiable, Hashable {
    let username: String
    let signedIn: Bool
    var id: String { username }
}

@MainActor
@Observable
final class AppModel {
    // ---- Source list ----
    private(set) var servers: [SavedServer] = []
    var authRequest: AuthRequest?
    var authError: String?

    // ---- The open server ----
    private(set) var currentAddress: String?
    private(set) var currentAccount: String?
    private(set) var client: ServerClient?
    private(set) var path = ""
    private(set) var pathDisplayName: String?
    private(set) var entries: [FileEntry] = []
    private(set) var isLoading = false
    private(set) var errorMessage: String?
    private var libraryTypes: [String: String] = [:]
    private var libraryNames: [String: String] = [:]

    // ---- Library view ----
    private(set) var libraryListing: LibraryListing?
    private(set) var libraryError: String?
    private(set) var openTitle: TitleDetail?

    // ---- Rows above the list ----
    private(set) var continueItems: [ContinueItem] = []
    private(set) var newRows: [(heading: String, items: [ContinueItem])] = []

    // ---- Playback ----
    var playing: PlayRequest?
    /// Settings is open: the player opens over it (a download played from its tab) and returns to it.
    var settingsOpen = false
    /// The photo or PDF open over the folder.
    var viewingImage: FileEntry?
    var viewingPdf: FileEntry?
    var resumeChoice: FileEntry?
    /// The file played last, so a show's page opens at its episode.
    private(set) var lastPlayedFile: String?

    var message: String?
    /// Bumped when view choices change, so views re-read them.
    private(set) var settingsVersion = 0

    @ObservationIgnored let context: ModelContext
    @ObservationIgnored private var loadToken = 0

    init(context: ModelContext) {
        self.context = context
        ThemeStore.shared.theme = AppTheme(rawValue: (try? context.fetch(FetchDescriptor<AppSetting>(
            predicate: #Predicate { $0.key == "app_theme" })))?.first?.value ?? "") ?? .default
        restoreFromKeychain()
        reloadServers()
        DownloadManager.shared.attach(self)
    }

    // MARK: Source list

    var isSourceList: Bool { currentAddress == nil }

    func reloadServers() {
        servers = (try? context.fetch(FetchDescriptor<SavedServer>(sortBy: [SortDescriptor(\.addedAt)]))) ?? []
    }

    /// The accounts signed in to a server on this phone, for its row.
    func accounts(of address: String) -> [AccountRow] {
        let saved = (try? context.fetch(FetchDescriptor<SavedAccount>(
            predicate: #Predicate { $0.address == address }, sortBy: [SortDescriptor(\.addedAt)]))) ?? []
        return saved.map { AccountRow(username: $0.username, signedIn: TokenStore.load(address: address, username: $0.username) != nil) }
    }

    /// The server's name (as it last said), else its host and port.
    func serverTitle(_ address: String) -> String {
        if let name = servers.first(where: { $0.address == address })?.name, !name.isEmpty { return name }
        guard let url = URL(string: address), let host = url.host else { return address }
        return url.port.map { "\(host):\($0)" } ?? host
    }

    /// Asks a server for its name (no sign-in needed) and keeps it.
    func refreshServerName(_ address: String) async {
        guard let name = await ServerClient.serverName(at: address),
              let server = servers.first(where: { $0.address == address }), server.name != name else { return }
        server.name = name
        save()
    }

    /// Saves a server without signing in to it; its accounts are added on its row. Without a
    /// scheme, the server is asked which it uses; without a port too, 9000, 443, 80 and 8080 are tried.
    func addServer(_ typed: String) async {
        guard let address = await ServerClient.checkAddress(typed) else {
            message = "That server address isn't valid."
            return
        }
        if !servers.contains(where: { $0.address == address }) {
            context.insert(SavedServer(address: address))
            save()
            reloadServers()
        }
        await refreshServerName(address)
    }

    /// Removes a server from this phone, with its accounts' sign-ins and its queued progress.
    func removeServer(_ address: String) {
        for account in accounts(of: address) { TokenStore.delete(address: address, username: account.username) }
        deleteAll(SavedAccount.self, matching: #Predicate { $0.address == address })
        deleteAll(LocalProgress.self, matching: #Predicate { $0.address == address })
        deleteAll(SavedServer.self, matching: #Predicate { $0.address == address })
        save()
        reloadServers()
        if currentAddress == address { returnToSourceSelection() }
    }

    /// Opens a server as one of its accounts, or asks for its password when its sign-in has run out.
    func chooseAccount(_ address: String, _ username: String) {
        if let saved = TokenStore.load(address: address, username: username) {
            open(address, username: username, token: saved.token)
        } else {
            authRequest = AuthRequest(address: address, username: username, title: "Sign in again")
        }
    }

    /// Signs another account in to a server, then opens it.
    func addAccount(_ address: String) {
        authRequest = AuthRequest(address: address, username: nil, title: "Add user")
    }

    /// Signs an account out of this phone; it stays on the row, marked Signed out.
    func signOut(_ address: String, _ username: String) {
        TokenStore.delete(address: address, username: username)
        settingsVersion += 1
    }

    /// The sign-in dialog's Login: signs in, saves the account, and opens the server as it.
    func login(_ request: AuthRequest, username: String, password: String) async {
        authError = nil
        do {
            let anonymous = try ServerClient(address: request.address, token: nil)
            let reply = try await anonymous.login(username: username, password: password)
            TokenStore.save(.init(address: request.address, username: username, token: reply.token, expiresAt: reply.expiresAt))
            let key = SavedAccount.key(address: request.address, username: username)
            let account = (try? context.fetch(FetchDescriptor<SavedAccount>(predicate: #Predicate { $0.key == key })))?.first
                ?? { let new = SavedAccount(address: request.address, username: username); context.insert(new); return new }()
            account.isAdmin = reply.user.isAdmin ?? false
            save()
            authRequest = nil
            open(request.address, username: username, token: reply.token)
        } catch {
            authError = error.localizedDescription
        }
    }

    func returnToSourceSelection() {
        currentAddress = nil
        currentAccount = nil
        client = nil
        path = ""
        pathDisplayName = nil
        entries = []
        errorMessage = nil
        libraryListing = nil
        openTitle = nil
        continueItems = []
        newRows = []
        reloadServers()
    }

    /// The header's line for the open server: its name and the account in use.
    var headerLine: String {
        guard let currentAddress else { return "" }
        return serverTitle(currentAddress) + (currentAccount.map { " · \($0)" } ?? "")
    }

    private func open(_ address: String, username: String, token: String) {
        currentAddress = address
        currentAccount = username
        client = try? ServerClient(address: address, token: token)
        if let server = servers.first(where: { $0.address == address }) { server.accountInUse = username; save() }
        continueItems = []
        newRows = []
        libraryTypes = [:]
        Task {
            await refreshServerName(address)
            await flushPendingProgress()
        }
        DownloadManager.shared.run() // downloads waiting for this server and account
        loadPath("")
    }

    // MARK: Folders

    var isServerRoot: Bool { currentAddress != nil && path.isEmpty }

    /// The library the open folder is in, and its type (movie, tv; nil for a plain folder).
    var currentLibraryId: String? { path.isEmpty ? nil : String(path.prefix(while: { $0 != "/" })) }
    var libraryType: String? { currentLibraryId.flatMap { libraryTypes[$0] } }

    /// The title in the bar: the title page's, the library's, else the folder's ("/" at the top).
    var barTitle: String {
        if let openTitle { return openTitle.title }
        if isLibraryView { return libraryListing?.libraryName ?? "Library" }
        return pathDisplayName ?? (path.isEmpty ? "/" : path)
    }

    func loadPath(_ newPath: String, displayName: String? = nil) {
        guard let client else { return }
        path = newPath
        pathDisplayName = displayName ?? (newPath.isEmpty ? nil : libraryNames[newPath] ?? newPath.components(separatedBy: "/").last)
        openTitle = nil
        errorMessage = nil
        isLoading = true
        loadToken += 1
        let token = loadToken
        Task {
            do {
                let directory: Directory = try await client.get("/api/media/browse", query: [("path", newPath)])
                guard token == loadToken else { return }
                if newPath.isEmpty {
                    for dir in directory.dirs ?? [] {
                        libraryNames[dir.id] = dir.name
                        if let type = dir.libraryType { libraryTypes[dir.id] = type }
                    }
                } else if let type = directory.libraryType, let lib = currentLibraryId {
                    libraryTypes[lib] = type
                }
                entries = (directory.dirs ?? []).map { FileEntry(name: $0.name, path: $0.id, isDirectory: true) }
                    .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
                    + (directory.items ?? []).map {
                        FileEntry(name: $0.name, path: $0.path, isDirectory: false, type: $0.type, size: $0.size ?? 0,
                                  duration: $0.duration, thumbnailUrl: $0.thumbnailUrl, playback: $0.playback,
                                  liveVideo: $0.liveVideo)
                    }.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
                isLoading = false
                if isLibraryView { loadLibrary() }
                loadContinueWatching()
            } catch {
                guard token == loadToken else { return }
                isLoading = false
                handle(error)
                if !(error is ServerError && isUnauthorized(error)) { errorMessage = error.localizedDescription }
            }
        }
    }

    /// Whether the open account is an administrator (its sign-in's claims: a change counts from the next sign-in).
    var isAdmin: Bool { client?.claims.isAdmin ?? false }

    var showHiddenFiles: Bool {
        _ = settingsVersion
        return setting("show_hidden_files") == "true"
    }

    /// The folder's entries, through the Show filter (folders always show) and without hidden
    /// files (names starting with a dot) unless Settings shows them.
    var filteredEntries: [FileEntry] {
        let entries = showHiddenFiles ? self.entries : self.entries.filter { !$0.name.hasPrefix(".") }
        guard !isServerRoot else { return entries }
        switch contentFilter {
        case .all: return entries
        case .video: return entries.filter { $0.isDirectory || $0.isVideo }
        case .image: return entries.filter { $0.isDirectory || $0.type == "image" }
        case .pdf: return entries.filter { $0.isDirectory || $0.name.lowercased().hasSuffix(".pdf") }
        }
    }

    /// Back: closes a title page, then leaves Library view for the libraries, else goes up.
    func navigateBack() {
        if openTitle != nil { openTitle = nil; return }
        if isLibraryView { leaveLibrary(); return }
        guard currentAddress != nil else { return }
        if path.isEmpty { returnToSourceSelection(); return }
        let parent = path.contains("/") ? String(path[..<path.lastIndex(of: "/")!]) : ""
        loadPath(parent)
    }

    var canGoBack: Bool { currentAddress != nil }

    // MARK: View modes (list, grid, library)

    var isLibraryView: Bool {
        _ = settingsVersion
        guard libraryType != nil, let lib = currentLibraryId, let address = currentAddress else { return false }
        return setting("library_mode_\(address)|\(lib)") != "false"
    }

    var isGridView: Bool {
        _ = settingsVersion
        guard let address = currentAddress else { return false }
        return setting("grid_view_\(address)|\(path)") == "true"
    }

    var contentFilter: ContentFilter {
        _ = settingsVersion
        return ContentFilter(rawValue: setting("content_filter") ?? "") ?? .all
    }

    func chooseContentFilter(_ filter: ContentFilter) {
        setSetting("content_filter", filter.rawValue)
    }

    /// Cycles list → grid → library (movie and TV libraries) → list.
    func toggleViewMode() {
        guard let address = currentAddress else { return }
        let libraryKey = "library_mode_\(address)|\(currentLibraryId ?? "")"
        let gridKey = "grid_view_\(address)|\(path)"
        if isLibraryView {
            setSetting(libraryKey, "false")
            setSetting(gridKey, "false")
        } else if !isGridView {
            setSetting(gridKey, "true")
        } else if libraryType != nil {
            setSetting(libraryKey, "true")
            loadLibrary()
        } else {
            setSetting(gridKey, "false")
        }
        loadContinueWatching()
    }

    /// The bar's view button shows the mode it switches to.
    var viewModeIcon: String {
        if isLibraryView { return "list.bullet" }
        if !isGridView { return "square.grid.2x2" }
        if libraryType != nil { return "film.stack" }
        return "list.bullet"
    }

    // MARK: Library view

    func loadLibrary() {
        guard let client, let lib = currentLibraryId else { return }
        libraryError = nil
        if libraryListing?.libraryId != lib { libraryListing = nil }
        Task {
            do {
                libraryListing = try await client.get("/api/library", query: [("lib", lib)])
            } catch {
                handle(error)
                libraryError = error.localizedDescription
            }
        }
    }

    func openLibraryTitle(kind: String, tmdbId: Int) {
        guard let client, let lib = currentLibraryId else { return }
        Task {
            do {
                openTitle = try await client.get("/api/library/title",
                                                 query: [("lib", lib), ("kind", kind), ("id", String(tmdbId))])
            } catch {
                handle(error)
                libraryError = error.localizedDescription
            }
        }
    }

    func reloadTitle() {
        guard let title = openTitle else { return }
        openLibraryTitle(kind: title.kind, tmdbId: title.tmdbId)
    }

    /// Leaves Library view for the server's list of libraries.
    func leaveLibrary() {
        openTitle = nil
        loadPath("")
    }

    func setAutoSkip(tmdbId: Int, on: Bool) {
        guard let client else { return }
        struct Body: Encodable { let tmdb_id: Int; let on: Bool }
        Task {
            do {
                try await client.send("PUT", "/api/library/autoskip", body: Body(tmdb_id: tmdbId, on: on))
                reloadTitle()
            } catch { handle(error) }
        }
    }

    // MARK: Continue Watching and Recently Added

    var hasTopRows: Bool { !continueItems.isEmpty || !newRows.isEmpty }

    /// A library's own page (Library view, or its top folder) shows its own videos; the server's
    /// front page shows every library's (Recently Added a row per library); deeper folders none.
    func loadContinueWatching() {
        guard let client else { return }
        let atLibraryRoot = !path.isEmpty && !path.contains("/")
        let libraryId = isLibraryView ? currentLibraryId : (atLibraryRoot ? path : nil)
        if !isLibraryView && !atLibraryRoot && !path.isEmpty {
            continueItems = []
            newRows = []
            return
        }
        let ownPage = isLibraryView || atLibraryRoot
        Task {
            await flushPendingProgress()
            // Offline, the rows keep what they had.
            if let items: [ContinueItem] = try? await client.get("/api/playback/continue",
                                                                 query: libraryId.map { [("lib", $0)] } ?? []) {
                continueItems = items
            }
            if ownPage, let libraryId {
                let added: [ContinueItem] = (try? await client.get("/api/library/new", query: [("lib", libraryId)])) ?? []
                newRows = added.isEmpty ? [] : [("Recently Added", added)]
            } else {
                let byLibrary: [LibraryAdditions] = (try? await client.get("/api/library/new", query: [("by", "library")])) ?? []
                newRows = byLibrary.compactMap { lib in
                    guard let items = lib.items, !items.isEmpty else { return nil }
                    return ("Recently Added in \(lib.libraryName)", items)
                }
            }
        }
    }

    func removeFromContinue(_ item: ContinueItem) {
        guard let client else { return }
        continueItems.removeAll { $0.path == item.path }
        Task { try? await client.execute("DELETE", "/api/playback/continue", query: [("path", ServerClient.encodePath(item.path))]) }
    }

    // MARK: Playing

    /// Folder view: plays a video, first asking whether to resume when it's partly watched.
    func openVideo(_ entry: FileEntry) {
        if resumeSeconds(entry.path, entry.playback) > 0 { resumeChoice = entry } else { play(entry.path, name: entry.name, from: 0) }
    }

    /// Answers the resume choice: resume, start over, or (nil) cancel.
    func chooseResume(fromStart: Bool?) {
        guard let entry = resumeChoice else { return }
        resumeChoice = nil
        guard let fromStart else { return }
        play(entry.path, name: entry.name, from: fromStart ? 0 : resumeSeconds(entry.path, entry.playback))
    }

    func playFromStart(_ entry: FileEntry) {
        play(entry.path, name: entry.name, from: 0)
    }

    func playContinue(_ item: ContinueItem) {
        play(item.path, name: item.metadata?.playerTitle ?? item.name, from: resumeSeconds(item.path, item.playback))
    }

    func playLibraryFile(_ file: LibraryFile, title: String? = nil, fromStart: Bool = false) {
        play(file.path, name: title ?? file.name, from: fromStart ? 0 : resumeSeconds(file.path, file.playback))
    }

    private func play(_ path: String, name: String, from start: Double) {
        lastPlayedFile = path
        playing = PlayRequest(path: path, title: name, startAt: start, libraryId: String(path.prefix(while: { $0 != "/" })))
    }

    /// Where to resume, in seconds: the newest of the server's position and the phone's.
    func resumeSeconds(_ path: String, _ server: Playback?) -> Double {
        if let local = localProgress(path), local.pendingPush,
           Int64(local.updatedAt.timeIntervalSince1970 * 1000) > (server?.updatedAt ?? 0) {
            return local.ended || local.position < 10 ? 0 : local.position
        }
        return server?.resumeAt ?? 0
    }

    /// Plays a download from this phone, resuming where it was left.
    func playDownload(_ d: Download, file: URL) {
        lastPlayedFile = d.sourcePath
        playing = PlayRequest(path: d.sourcePath, title: d.title ?? d.name, startAt: resumeSeconds(d.sourcePath, nil),
                              libraryId: String(d.sourcePath.prefix(while: { $0 != "/" })), localFile: file)
    }

    /// Download ahead runs once per episode a viewing reaches two minutes into (or the end of).
    @ObservationIgnored private var aheadDone: Set<String> = []

    /// After the player closes: the lists show the new progress.
    func playerClosed() {
        if openTitle != nil { reloadTitle() }
        else if isLibraryView { loadLibrary() }
        else if currentAddress != nil { loadPath(path, displayName: pathDisplayName) }
    }

    /// Marks videos (client paths) watched or unwatched, then shows it.
    func setWatched(_ paths: [String], _ watched: Bool) {
        guard let client else { return }
        Task {
            do {
                try await client.send("POST", "/api/playback/watched",
                                      body: WatchedRequest(paths: paths.map(ServerClient.encodePath), watched: watched,
                                                           library: nil, kind: nil, tmdbId: nil))
                playerClosed()
                loadContinueWatching()
            } catch { handle(error) }
        }
    }

    // MARK: Progress

    /// Records where a file was, on the phone first, then tells the server. If that fails the
    /// position stays queued until the server can be reached.
    func recordProgress(path: String, position: Double, duration: Double, paused: Bool, ended: Bool, discTitle: Int = 0) async {
        guard let client, let address = currentAddress, let username = currentAccount else { return }
        // A disc's extras (titles other than the main one) have no progress of their own: the
        // server ignores them, and kept here they'd become the film's resume point (synced later
        // without their title).
        if discTitle != 0 {
            _ = try? await client.send("POST", "/api/streams/progress",
                                       body: StreamProgress(path: ServerClient.encodePath(path), position: position,
                                                            paused: paused, ended: ended, title: discTitle))
            return
        }
        let entry = localProgress(path) ?? {
            let new = LocalProgress(address: address, username: username, path: path)
            context.insert(new)
            return new
        }()
        entry.position = position
        if duration > 0 { entry.duration = duration }
        entry.ended = ended
        entry.updatedAt = Date()
        entry.pendingPush = true
        save()
        if (position >= 120 || ended) && !aheadDone.contains(path) {
            aheadDone.insert(path)
            Task { await DownloadManager.shared.downloadAhead(after: path) }
        }
        do {
            try await client.send("POST", "/api/streams/progress",
                                  body: StreamProgress(path: ServerClient.encodePath(path), position: position,
                                                       paused: paused, ended: ended, title: discTitle))
            entry.pendingPush = false
            save()
        } catch { handle(error) }
    }

    /// Sends progress the server hasn't seen yet (played while it couldn't be reached).
    func flushPendingProgress() async {
        guard let client, let address = currentAddress, let username = currentAccount else { return }
        let pending = (try? context.fetch(FetchDescriptor<LocalProgress>(
            predicate: #Predicate { $0.address == address && $0.username == username && $0.pendingPush }))) ?? []
        guard !pending.isEmpty else { return }
        let items = pending.map {
            SyncedPlayback(path: ServerClient.encodePath($0.path), position: $0.position, ended: $0.ended,
                           updatedAt: Int64($0.updatedAt.timeIntervalSince1970 * 1000))
        }
        do {
            try await client.send("POST", "/api/playback/sync", body: PlaybackSync(items: items))
            pending.forEach { $0.pendingPush = false }
            save()
        } catch { handle(error) }
    }

    private func localProgress(_ path: String) -> LocalProgress? {
        guard let address = currentAddress, let username = currentAccount else { return nil }
        let key = LocalProgress.key(address: address, username: username, path: path)
        return (try? context.fetch(FetchDescriptor<LocalProgress>(predicate: #Predicate { $0.key == key })))?.first
    }

    // MARK: Errors

    /// A request answered "not signed in": the account's sign-in has run out, so ask again.
    func handle(_ error: Error) {
        guard isUnauthorized(error), let address = currentAddress else { return }
        let username = currentAccount
        if let username { TokenStore.delete(address: address, username: username) }
        returnToSourceSelection()
        authRequest = AuthRequest(address: address, username: username, title: "Sign in again")
    }

    private func isUnauthorized(_ error: Error) -> Bool {
        if case ServerError.unauthorized = error { return true }
        return false
    }

    // MARK: Settings store

    func setting(_ key: String) -> String? {
        (try? context.fetch(FetchDescriptor<AppSetting>(predicate: #Predicate { $0.key == key })))?.first?.value
    }

    func setSetting(_ key: String, _ value: String) {
        if let found = (try? context.fetch(FetchDescriptor<AppSetting>(predicate: #Predicate { $0.key == key })))?.first {
            found.value = value
        } else {
            context.insert(AppSetting(key: key, value: value))
        }
        save()
        settingsVersion += 1
    }

    private func save() {
        do { try context.save() } catch { context.rollback() }
    }

    private func deleteAll<T: PersistentModel>(_ type: T.Type, matching predicate: Predicate<T>) {
        let found = (try? context.fetch(FetchDescriptor<T>(predicate: predicate))) ?? []
        found.forEach(context.delete)
    }

    /// A store that had to be reset (or a first launch after one) gets its servers and accounts
    /// back from the sign-ins in the Keychain.
    private func restoreFromKeychain() {
        let known = Set(((try? context.fetch(FetchDescriptor<SavedAccount>())) ?? []).map(\.key))
        var added = false
        for saved in TokenStore.all() where !known.contains(SavedAccount.key(address: saved.address, username: saved.username)) {
            let address = saved.address
            if ((try? context.fetch(FetchDescriptor<SavedServer>(predicate: #Predicate { $0.address == address }))) ?? []).isEmpty {
                context.insert(SavedServer(address: address))
            }
            context.insert(SavedAccount(address: address, username: saved.username))
            added = true
        }
        if added { save() }
    }
}
