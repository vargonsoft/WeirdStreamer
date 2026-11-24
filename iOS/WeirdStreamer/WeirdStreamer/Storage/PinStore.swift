import CommonCrypto
import Foundation

/// The PINs that guard a server's accounts on this phone (Android's AccountPins): opening an
/// account from the source list asks for its PIN when it has one. Set in Settings › Account, 4 or
/// more digits, kept as a salted PBKDF2 hash. After 5 wrong ones in a row, each try waits (30 s,
/// doubling to an hour at most) until one is right. Forgotten, the account's password opens it
/// instead, which clears the PIN.
struct PinStore {
    enum Check: Equatable {
        case right
        /// Wrong; the next try counts from `lockedUntil` (nil: now).
        case wrong(lockedUntil: Date?)
        /// Not tried: still waiting after earlier wrong ones.
        case locked(until: Date)
    }

    static let minLength = 4
    static let freeTries = 5
    static let shared = PinStore(defaults: .standard)

    let defaults: UserDefaults
    var now: () -> Date = Date.init

    private func key(_ address: String, _ username: String) -> String { "pin|\(address)|\(username)" }

    func has(_ address: String, _ username: String) -> Bool {
        defaults.string(forKey: key(address, username) + "|hash") != nil
    }

    func set(_ address: String, _ username: String, pin: String) {
        let k = key(address, username)
        var salt = [UInt8](repeating: 0, count: 16)
        _ = SecRandomCopyBytes(kSecRandomDefault, salt.count, &salt)
        defaults.set(Data(salt).base64EncodedString(), forKey: k + "|salt")
        defaults.set(Self.hash(pin, salt: salt).base64EncodedString(), forKey: k + "|hash")
        defaults.removeObject(forKey: k + "|fails")
        defaults.removeObject(forKey: k + "|locked")
    }

    func clear(_ address: String, _ username: String) {
        let k = key(address, username)
        for suffix in ["|salt", "|hash", "|fails", "|locked"] { defaults.removeObject(forKey: k + suffix) }
    }

    /// When the next try counts, or nil when it can be tried now.
    func lockedUntil(_ address: String, _ username: String) -> Date? {
        let at = defaults.double(forKey: key(address, username) + "|locked")
        guard at > 0 else { return nil }
        let until = Date(timeIntervalSince1970: at)
        return until > now() ? until : nil
    }

    func check(_ address: String, _ username: String, pin: String) -> Check {
        let k = key(address, username)
        guard let stored = defaults.string(forKey: k + "|hash").flatMap({ Data(base64Encoded: $0) }),
              let salt = defaults.string(forKey: k + "|salt").flatMap({ Data(base64Encoded: $0) }) else { return .right }
        if let until = lockedUntil(address, username) { return .locked(until: until) }
        if Self.hash(pin, salt: [UInt8](salt)) == stored {
            defaults.removeObject(forKey: k + "|fails")
            defaults.removeObject(forKey: k + "|locked")
            return .right
        }
        let fails = defaults.integer(forKey: k + "|fails") + 1
        defaults.set(fails, forKey: k + "|fails")
        guard fails >= Self.freeTries else { defaults.removeObject(forKey: k + "|locked"); return .wrong(lockedUntil: nil) }
        let wait = min(30.0 * pow(2, Double(min(fails - Self.freeTries, 7))), 3600)
        let until = now().addingTimeInterval(wait)
        defaults.set(until.timeIntervalSince1970, forKey: k + "|locked")
        return .wrong(lockedUntil: until)
    }

    /// "0:30", "2:00": how long until `until`.
    static func wait(until: Date, now: Date = Date()) -> String {
        let s = max(0, Int(ceil(until.timeIntervalSince(now))))
        return "\(s / 60):" + String(format: "%02d", s % 60)
    }

    private static func hash(_ pin: String, salt: [UInt8]) -> Data {
        var out = [UInt8](repeating: 0, count: 32)
        let password = Array(pin.utf8).map { Int8(bitPattern: $0) }
        CCKeyDerivationPBKDF(CCPBKDFAlgorithm(kCCPBKDF2), password, password.count, salt, salt.count,
                             CCPseudoRandomAlgorithm(kCCPRFHmacAlgSHA256), 20_000, &out, out.count)
        return Data(out)
    }
}
