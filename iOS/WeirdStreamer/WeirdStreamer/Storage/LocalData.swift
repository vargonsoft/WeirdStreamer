import Foundation
import SwiftData

// Everything the app keeps is on this phone: the servers on the source list, the accounts signed
// in to each (their tokens are in the Keychain, TokenStore), view choices, and playback progress
// waiting to reach its server. As the Android app's CredentialManager and LocalProgress.

/// A WeirdStreamer server on the source list, by its address as requests use it
/// ("http://192.168.1.5:9000").
@Model
final class SavedServer {
    @Attribute(.unique) var address: String
    /// The name the server gave itself (`/api/info`), kept so it shows while offline.
    var name: String?
    var addedAt: Date
    /// The account last opened on this server.
    var accountInUse: String?

    init(address: String) {
        self.address = address
        self.addedAt = Date()
    }
}

/// An account signed in to a server on this phone (one round button on the server's row).
@Model
final class SavedAccount {
    @Attribute(.unique) var key: String      // "<address>|<username>"
    var address: String
    var username: String
    var addedAt: Date
    var isAdmin: Bool = false

    init(address: String, username: String) {
        self.key = SavedAccount.key(address: address, username: username)
        self.address = address
        self.username = username
        self.addedAt = Date()
    }

    static func key(address: String, username: String) -> String { "\(address)|\(username)" }
}

/// A view choice, as Android's preferences: library or folder view per library, grid or list
/// per folder, the content filter, and the like. Key and value are strings.
@Model
final class AppSetting {
    @Attribute(.unique) var key: String
    var value: String

    init(key: String, value: String) {
        self.key = key
        self.value = value
    }
}

/// The last position the phone saw for a file. Written on every report, so progress isn't lost
/// when the server can't be reached; `pendingPush` marks what still has to go to the server
/// (`/api/playback/sync`, whenever it answers again).
@Model
final class LocalProgress {
    @Attribute(.unique) var key: String      // "<address>|<username>|<client path>"
    var address: String
    var username: String
    var path: String
    var position: Double
    var duration: Double
    var ended: Bool
    var updatedAt: Date
    var pendingPush: Bool

    init(address: String, username: String, path: String) {
        self.key = LocalProgress.key(address: address, username: username, path: path)
        self.address = address
        self.username = username
        self.path = path
        self.position = 0
        self.duration = 0
        self.ended = false
        self.updatedAt = Date()
        self.pendingPush = false
    }

    static func key(address: String, username: String, path: String) -> String { "\(address)|\(username)|\(path)" }
}

/// Opens the on-device database, a SQLite file in Application Support. If it can't be opened (a
/// damaged file, a write cut short when the app was killed, or a store from an older version that
/// won't migrate after the app sat expired past its 7-day signing), its files are moved into a
/// Recovery folder and a fresh store starts, so the app always launches. Sign-ins survive in the
/// Keychain; servers and accounts are rebuilt from it (`AppModel.restoreFromKeychain`).
enum LocalStore {
    static let schema = Schema([SavedServer.self, SavedAccount.self, AppSetting.self, LocalProgress.self, Download.self])

    /// Set when the store had to be reset at launch; the app shows it once.
    nonisolated(unsafe) static var recoveryNote: String?

    static func makeContainer() -> ModelContainer {
        let folder = storeFolder()
        let url = folder.appendingPathComponent("WeirdStreamer.store")
        if let container = try? open(at: url) { return container }

        let recovery = folder.appendingPathComponent("Recovery-\(Int(Date().timeIntervalSince1970))", isDirectory: true)
        try? FileManager.default.createDirectory(at: recovery, withIntermediateDirectories: true)
        for suffix in ["", "-shm", "-wal"] {
            let file = URL(fileURLWithPath: url.path + suffix)
            guard FileManager.default.fileExists(atPath: file.path) else { continue }
            try? FileManager.default.moveItem(at: file, to: recovery.appendingPathComponent(file.lastPathComponent))
        }
        pruneRecoveries(in: folder, keeping: 3)
        recoveryNote = "The saved data couldn't be opened, so a new store was started. Your sign-ins were kept."
        if let container = try? open(at: url) { return container }

        // Last resort: the app still runs this session, but nothing is kept on disk.
        recoveryNote = "The saved data couldn't be opened at all, so nothing will be kept after this session."
        let memory = ModelConfiguration("memory", schema: schema, isStoredInMemoryOnly: true)
        return try! ModelContainer(for: schema, configurations: [memory])
    }

    private static func open(at url: URL) throws -> ModelContainer {
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        return try ModelContainer(for: schema, configurations: [ModelConfiguration(schema: schema, url: url)])
    }

    /// Keeps only the newest few Recovery folders.
    private static func pruneRecoveries(in folder: URL, keeping count: Int) {
        let items = (try? FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil)) ?? []
        let recoveries = items.filter { $0.lastPathComponent.hasPrefix("Recovery-") }
            .sorted { $0.lastPathComponent > $1.lastPathComponent }
        for old in recoveries.dropFirst(count) { try? FileManager.default.removeItem(at: old) }
    }

    static func storeFolder() -> URL {
        let support = (try? FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask,
                                                    appropriateFor: nil, create: true))
            ?? FileManager.default.temporaryDirectory
        return support.appendingPathComponent("WeirdStreamer", isDirectory: true)
    }
}
