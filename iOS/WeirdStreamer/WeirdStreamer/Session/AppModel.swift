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
    var fixable = false            // a folder standing for one title, whose match an admin can fix

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

/// An account chosen on the source list that has a PIN on this phone: asked for before it opens.
struct PinRequest: Identifiable, Equatable {
    var id: String { "\(address)|\(username)" }
    let address: String
    let username: String
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
    var pinRequest: PinRequest?
    // The account whose PIN was forgotten: its password's sign-in, once it works, clears the PIN.
    private var pinResetFor: PinRequest?

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
    /// Returns why it wasn't saved (the add dialog shows it and stays open), or nil once it is.
    func addServer(_ typed: String) async -> String? {
        let text = typed.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let address = await ServerClient.checkAddress(text) else {
            if text.contains("://") || ServerClient.normalized("https://" + text) == nil {
                return "That server address isn't valid."
            }
            return "No server answered at \(text). Check the address and that the phone is online, "
                + "or type it in full (https://\(text):9000) to add it anyway."
        }
        if !servers.contains(where: { $0.address == address }) {
            context.insert(SavedServer(address: address))
            save()
            reloadServers()
        }
        await refreshServerName(address)
        // Its certificate is kept now (the one the address check saw), and a different one is refused.
        TrustPolicy.shared.keepOnAdd(address)
        return nil
    }

    /// Removes a server from this phone, with its accounts' sign-ins and its queued progress.
    func removeServer(_ address: String) {
        for account in accounts(of: address) {
            endSession(address, TokenStore.load(address: address, username: account.username)?.token)
            TokenStore.delete(address: address, username: account.username)
            PinStore.shared.clear(address, account.username)
        }
        TrustPolicy.shared.forget(address)
        deleteAll(SavedAccount.self, matching: #Predicate { $0.address == address })
        deleteAll(LocalProgress.self, matching: #Predicate { $0.address == address })
        deleteAll(SavedServer.self, matching: #Predicate { $0.address == address })
        save()
        reloadServers()
        if currentAddress == address { returnToSourceSelection() }
    }

    /// Opens a server as one of its accounts, or asks for its password when its sign-in has run out.
    /// An account with a PIN on this phone asks for it first (`pinRequest`).
    func chooseAccount(_ address: String, _ username: String) {
        if PinStore.shared.has(address, username) {
            pinRequest = PinRequest(address: address, username: username)
            return
        }
        openAccount(address, username)
    }

    /// The PIN typed for `pinRequest`: right opens the account.
    func pinEntered(_ pin: String) -> PinStore.Check {
        guard let request = pinRequest else { return .right }
        let result = PinStore.shared.check(request.address, request.username, pin: pin)
        if result == .right {
            pinRequest = nil
            openAccount(request.address, request.username)
        }
        return result
    }

    /// The PIN is forgotten: the account's password signs it in instead, and clears the PIN.
    func usePasswordInstead() {
        guard let request = pinRequest else { return }
        pinRequest = nil
        pinResetFor = request
        authRequest = AuthRequest(address: request.address, username: request.username, title: "Sign in again")
    }

    private func openAccount(_ address: String, _ username: String) {
        if let saved = TokenStore.load(address: address, username: username) {
            open(address, username: username, token: saved.token)
        } else {
            authRequest = AuthRequest(address: address, username: username, title: "Sign in again")
        }
    }

    /// Tells a server a sign-in is over, so its token stops working there (best effort: offline, it just expires).
    private func endSession(_ address: String, _ token: String?) {
        guard let token, let client = try? ServerClient(address: address, token: token) else { return }
        Task.detached { try? await client.execute("POST", "/api/auth/logout") }
    }

    /// Signs another account in to a server, then opens it.
    func addAccount(_ address: String) {
        authRequest = AuthRequest(address: address, username: nil, title: "Add user")
    }

    /// Signs an account out of this phone; it stays on the row, marked Signed out.
    func signOut(_ address: String, _ username: String) {
        endSession(address, TokenStore.load(address: address, username: username)?.token)
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
            if let reset = pinResetFor, reset.address == request.address, reset.username == username {
                PinStore.shared.clear(request.address, username)
            }
            pinResetFor = nil
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
        thumbnailListener?.cancel()
        thumbnailListener = nil
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
        listenForThumbnails()
        loadPath("")
    }

    // MARK: Thumbnails

    @ObservationIgnored private var thumbnailListener: Task<Void, Never>?
    /// The ⋮ menu's Regenerate thumbnails was picked: its confirm dialog shows.
    var confirmRegenerate = false

    /// Whether ⋮ offers Regenerate thumbnails: in a folder of a server's library (not the
    /// libraries themselves, Library view or a title page), for an administrator, as the server
    /// allows it only for them.
    var canRegenerateThumbnails: Bool {
        !isSourceList && !isServerRoot && !isLibraryView && openTitle == nil && isAdmin
    }

    /// Makes the open folder's thumbnails again. The ones cached on the phone are dropped now;
    /// each new one shows as the server reports it made (`listenForThumbnails`).
    func regenerateThumbnails() {
        guard let client, canRegenerateThumbnails else { return }
        let folder = path
        Task {
            do {
                try await client.regenerateThumbnails(folder)
                for entry in entries where !entry.isDirectory {
                    if let url = client.url(for: entry.thumbnailUrl) { ImageCache.shared.removeObject(forKey: url as NSURL) }
                }
            } catch {
                handle(error)
                if !isUnauthorized(error) { message = "Failed to regenerate thumbnails: \(error.localizedDescription)" }
            }
        }
    }

    /// Listens to the server's thumbnail-ready events while it's open (as Android's
    /// ThumbnailUpdateListener): a file in the folder shown gets its new thumbnail, under an
    /// address of its own so the old picture isn't taken from the cache. Reconnects after a
    /// drop, waiting longer each time (1 s to 30 s).
    private func listenForThumbnails() {
        thumbnailListener?.cancel()
        guard let client else { return }
        thumbnailListener = Task { [weak self] in
            var wait: Double = 1
            while !Task.isCancelled {
                do {
                    for try await update in client.thumbnailUpdates() {
                        wait = 1
                        self?.thumbnailMade(update)
                    }
                } catch {}
                guard !Task.isCancelled else { return }
                try? await Task.sleep(for: .seconds(wait))
                wait = min(wait * 2, 30)
            }
        }
    }

    private func thumbnailMade(_ update: ServerClient.ThumbnailUpdate) {
        guard let i = entries.firstIndex(where: { $0.path == update.path }) else { return }
        let separator = update.thumbnailURL.contains("?") ? "&" : "?"
        entries[i].thumbnailUrl = update.thumbnailURL + separator + "t=\(Int(Date().timeIntervalSince1970 * 1000))"
    }

    // MARK: Folders

    var isServerRoot: Bool { currentAddress != nil && path.isEmpty }

    /// The library the open folder is in, and its type (movie, tv; nil for a plain folder).
    var currentLibraryId: String? { path.isEmpty ? nil : String(path.prefix(while: { $0 != "/" })) }
    var libraryType: String? { currentLibraryId.flatMap { libraryTypes[$0] } }

    /// The title in the bar: the title page's, the library's, else the folder's ("/" at the top).
    var barTitle: String {
        if let openTitle { return openTitle.title }
        if let openCollection { return openCollection.name }
        if isCollectionsView { return (libraryListing?.libraryName ?? "Library") + " · Collections" }
        if isLibraryView { return libraryListing?.libraryName ?? "Library" }
        return pathDisplayName ?? (path.isEmpty ? "/" : path)
    }

    func loadPath(_ newPath: String, displayName: String? = nil) {
        guard let client else { return }
        path = newPath
        pathDisplayName = displayName ?? (newPath.isEmpty ? nil : libraryNames[newPath] ?? newPath.components(separatedBy: "/").last)
        openTitle = nil
        openCollection = nil
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
                entries = (directory.dirs ?? []).map { FileEntry(name: $0.name, path: $0.id, isDirectory: true, thumbnailUrl: $0.thumbnailUrl, fixable: $0.fixable ?? false) }
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

    // MARK: Fixing TMDB matches (MatchSheet.swift)

    /// The file or folder whose Fix sheet is open, if any.
    var matchTarget: MatchTarget?

    /// Fix is offered: an admin's account, in a library with a type (movies or TV).
    var canFixMatches: Bool { libraryType != nil && isAdmin }

    /// Opens Fix for a file (its client path) or a folder standing for one title; `wholeTitle` for a
    /// title page's whole title (through its first file), without its episode.
    func openMatch(path: String, isDir: Bool, name: String, wholeTitle: Bool = false) {
        matchTarget = MatchTarget(path: path, isDir: isDir, name: name, wholeTitle: wholeTitle)
    }

    /// After a change in Fix: what's on screen is loaded again (a title page that's gone closes).
    func matchChanged() {
        guard let client else { return }
        Task {
            if let title = openTitle, let lib = currentLibraryId {
                let fresh: TitleDetail? = try? await client.get("/api/library/title",
                                                                query: [("lib", lib), ("kind", title.kind), ("id", String(title.tmdbId))])
                if openTitle?.tmdbId == title.tmdbId { openTitle = fresh } // nil: matched to another title
            }
            if isLibraryView {
                loadLibrary()
            } else if openTitle == nil {
                loadPath(path, displayName: pathDisplayName)
            }
        }
    }

    var showHiddenFiles: Bool {
        _ = settingsVersion
        return setting("show_hidden_files") == "true"
    }

    /// Resuming a paused video goes back 5 seconds first (Settings, off by default).
    var rewindOnResume: Bool {
        _ = settingsVersion
        return setting("rewind_on_resume") == "true"
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
        if openCollection != nil { openCollection = nil; return }
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

    /// Collection view: Library view shows a movie library's collections rather than its posters.
    var isCollectionsView: Bool {
        _ = settingsVersion
        guard isLibraryView, libraryType == "movie", let lib = currentLibraryId, let address = currentAddress else { return false }
        return setting("collections_mode_\(address)|\(lib)") == "true"
    }

    /// Cycles list → grid → library (movie and TV libraries) → collections (movie libraries) → list.
    func toggleViewMode() {
        guard let address = currentAddress else { return }
        let libraryKey = "library_mode_\(address)|\(currentLibraryId ?? "")"
        let collectionsKey = "collections_mode_\(address)|\(currentLibraryId ?? "")"
        let gridKey = "grid_view_\(address)|\(path)"
        if isLibraryView && libraryType == "movie" && !isCollectionsView {
            setSetting(collectionsKey, "true")
            openTitle = nil
        } else if isLibraryView {
            setSetting(collectionsKey, "false")
            openCollection = nil
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
        if isLibraryView && libraryType == "movie" && !isCollectionsView { return "square.stack.3d.up" }
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

    /// The collection page shown (Collection view), if any; a title opened from it shows over it.
    var openCollection: CollectionDetail?

    func openLibraryCollection(id: Int) {
        guard let client, let lib = currentLibraryId else { return }
        Task {
            do {
                openCollection = try await client.get("/api/library/collection", query: [("lib", lib), ("id", String(id))])
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
        openCollection = nil
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

    /// A Continue Watching entry, or (with `ask` false, going on by itself) the show's next episode.
    func playContinue(_ item: ContinueItem, ask: Bool = true) {
        play(item.path, name: item.metadata?.playerTitle ?? item.name, from: resumeSeconds(item.path, item.playback), ask: ask)
    }

    /// Plays a library video from the start by its client path (Settings' Damaged videos).
    func playServerFile(_ path: String, name: String) {
        startPlaying(path, name: name, from: 0, file: nil)
    }

    func playLibraryFile(_ file: LibraryFile, title: String? = nil, fromStart: Bool = false) {
        play(file.path, name: title ?? file.name, from: fromStart ? 0 : resumeSeconds(file.path, file.playback))
    }

    /// A server video that's also downloaded on this phone: play the download or stream it? (Android's sourceChoice)
    struct SourceChoice {
        let path: String
        let name: String
        let start: Double
        let file: URL
    }
    var sourceChoice: SourceChoice?

    /// Plays a server video, first asking whether to play its download instead when it has one
    /// (not when going on to the next episode by itself: `ask` false streams, as the last one did).
    private func play(_ path: String, name: String, from start: Double, ask: Bool = true) {
        if ask, let d = DownloadManager.shared.find(path), d.isDone, let file = d.localURL,
           FileManager.default.fileExists(atPath: file.path) {
            sourceChoice = SourceChoice(path: path, name: name, start: start, file: file)
            return
        }
        startPlaying(path, name: name, from: start, file: nil)
    }

    /// Answers the source choice: the download (true), the server (false), or (nil) cancel.
    func chooseSource(download: Bool?) {
        guard let c = sourceChoice else { return }
        sourceChoice = nil
        guard let download else { return }
        startPlaying(c.path, name: c.name, from: c.start, file: download ? c.file : nil)
    }

    private func startPlaying(_ path: String, name: String, from start: Double, file: URL?) {
        lastPlayedFile = path
        playing = PlayRequest(path: path, title: name, startAt: start, libraryId: String(path.prefix(while: { $0 != "/" })),
                              localFile: file)
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
