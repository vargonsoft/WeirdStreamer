import Foundation
import Security

/// Sign-in tokens, one per account on a server, in the Keychain. Without a paid developer account
/// there is no shared access group, so the items belong to this app alone (all it needs). The
/// Keychain also outlives a damaged database: servers and accounts are rebuilt from it.
enum TokenStore {
    struct Saved: Codable {
        let address: String
        let username: String
        let token: String
        let expiresAt: Int64   // Unix seconds
    }

    private static let service = "com.vargon.WeirdStreamer.tokens"

    static func save(_ saved: Saved) {
        guard let data = try? JSONEncoder().encode(saved) else { return }
        let query = baseQuery(account: account(saved.address, saved.username))
        SecItemDelete(query as CFDictionary)
        var add = query
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(add as CFDictionary, nil)
    }

    /// The account's token, or nil when it has none or it has run out (the server's own lifetime).
    static func load(address: String, username: String) -> Saved? {
        var query = baseQuery(account: account(address, username))
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data,
              let saved = try? JSONDecoder().decode(Saved.self, from: data) else { return nil }
        guard saved.expiresAt > Int64(Date().timeIntervalSince1970) else { return nil }
        return saved
    }

    static func delete(address: String, username: String) {
        SecItemDelete(baseQuery(account: account(address, username)) as CFDictionary)
    }

    /// Every saved sign-in, expired ones too (to rebuild the source list).
    static func all() -> [Saved] {
        var query: [String: Any] = [kSecClass as String: kSecClassGenericPassword,
                                    kSecAttrService as String: service,
                                    kSecReturnData as String: true,
                                    kSecMatchLimit as String: kSecMatchLimitAll]
        query[kSecReturnAttributes as String] = false
        var result: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let items = result as? [Data] else { return [] }
        return items.compactMap { try? JSONDecoder().decode(Saved.self, from: $0) }
    }

    private static func account(_ address: String, _ username: String) -> String { "\(address)|\(username)" }

    private static func baseQuery(account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: account]
    }
}
