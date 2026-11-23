import Foundation

/// One backup in the server's backup folder (GET /api/admin/backups).
struct BackupInfo: Decodable {
    let name: String
    var size: Int64?
    var created: String?    // RFC 3339
}

/// The server's backups, newest first, its folder (a disk path: admins only) and when the next is.
struct BackupList: Decodable {
    var folder: String?
    var backups: [BackupInfo]?
    var next: String?       // RFC 3339; none when backups are off
}

/// Backups' choices and texts for an admin's Server tab, as Android's BackupSettings and the web's:
/// the config's database.backupSchedule (daily, weekly or off), backupDay, backupTime, backupKeep.
enum BackupSettings {
    static let schedules = [("daily", "Every day"), ("weekly", "Every week"), ("off", "Only when asked")]
    static let days = ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"]
        .map { ($0, $0.prefix(1).uppercased() + $0.dropFirst()) }

    /// A time of day as the server takes it ("4:00" too).
    static func validTime(_ t: String) -> Bool {
        let parts = t.split(separator: ":", omittingEmptySubsequences: false)
        guard parts.count == 2, let h = Int(parts[0]), let m = Int(parts[1]), parts[1].count == 2 else { return false }
        return (0...23).contains(h) && (0...59).contains(m)
    }

    /// The time as the server keeps it ("04:00"), or the default for one that isn't a time.
    static func time(_ t: String) -> String {
        guard validTime(t) else { return "04:00" }
        return t.count == 4 ? "0" + t : t
    }

    /// "In /srv/ws/var/backups. The next one: Tue 24 Nov, 04:00."
    static func whereText(_ list: BackupList) -> String {
        let next = list.next.flatMap { date($0, dateStyle: .short) }.map { " The next one: \($0)." } ?? " Only Back up now makes them."
        return "In \(list.folder ?? "?").\(next)" + ((list.backups ?? []).isEmpty ? " None yet." : "")
    }

    /// A backup's time on this phone's calendar and clock. Go writes nanoseconds, which
    /// ISO8601DateFormatter doesn't read: the fraction goes (minutes are what's shown).
    static func date(_ rfc3339: String, dateStyle: DateFormatter.Style = .medium) -> String? {
        let plain = rfc3339.replacingOccurrences(of: #"\.\d+"#, with: "", options: .regularExpression)
        guard let d = ISO8601DateFormatter().date(from: plain) else { return nil }
        let f = DateFormatter()
        f.dateStyle = dateStyle
        f.timeStyle = .short
        return f.string(from: d)
    }
}
