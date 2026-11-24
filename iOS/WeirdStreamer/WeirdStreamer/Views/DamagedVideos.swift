import SwiftUI

/// A damaged video (its seek index broken) and its fixed copy (GET /api/admin/damaged), as
/// Android's DamagedVideo. Keys are decoded from snake case.
struct DamagedVideo: Decodable, Identifiable {
    var path: String
    var name: String
    var library: String?
    var folder: String?
    var reason: String?
    var state: String?          // "fixed", or "failed"
    var error: String?
    var size: Int64?
    var fixedSize: Int64?
    var claimed: Double?        // the length the original says
    var duration: Double?       // the fixed copy's
    var id: String { path }
    var fixed: Bool { state == "fixed" }
}

/// The trash of replaced originals: deleted `days` after they went in.
struct TrashReport: Decodable {
    var files: Int?
    var bytes: Int64?
    var days: Int?
}

struct DamagedList: Decodable {
    var videos: [DamagedVideo]?
    var trash: TrashReport?
}

/// Texts for the damaged videos list, as Android's (AdminSettings.kt) and the web's.
enum DamagedVideos {
    static let about = "Videos players can't jump in (a broken seek index), found by scans. The server plays a fixed copy in their place: the same video and audio, copied untouched into an indexed file. Play each to compare, then replace the original with the fixed copy (as .mkv, keeping its watch progress); the original goes to the trash."

    /// "1:14:32"
    static func clock(_ seconds: Double?) -> String {
        let s = Int((seconds ?? 0).rounded())
        return "\(s / 3600):" + String(format: "%02d:%02d", s / 60 % 60, s % 60)
    }

    /// Its name once its fixed copy replaces it: its own, as .mkv.
    static func replacedName(_ name: String) -> String {
        let base = (name as NSString).deletingPathExtension
        return (base.isEmpty ? name : base) + ".mkv"
    }

    static func whereText(_ v: DamagedVideo) -> String {
        [v.library ?? "", v.folder ?? ""].filter { !$0.isEmpty }.joined(separator: " › ") + " · " + (v.reason ?? "")
    }

    static func sizes(_ v: DamagedVideo) -> String {
        v.fixed
            ? "Original: \(formatBytes(v.size)), says \(clock(v.claimed)) long · Fixed copy: \(formatBytes(v.fixedSize)), \(clock(v.duration))"
            : "Original: \(formatBytes(v.size)) · Couldn't be fixed: \(v.error ?? "unknown")"
    }

    static func trashText(_ t: TrashReport?) -> String {
        let files = t?.files ?? 0, days = t?.days ?? 30
        return files > 0
            ? "The trash holds \(files) replaced original\(files == 1 ? "" : "s") (\(formatBytes(t?.bytes))), each deleted \(days) days after it went in."
            : "The trash is empty. Replaced originals are kept there \(days) days."
    }
}

/// Settings › Server › Damaged videos (admins): each damaged video with Play original, Play fixed
/// copy and Replace with fixed copy (asked first), and the trash with Empty the trash now.
struct DamagedVideosSection: View {
    @Environment(AppModel.self) private var model
    @State private var list: DamagedList?
    @State private var loadError: String?
    @State private var message: (String, Bool)?
    @State private var replacing: DamagedVideo?
    @State private var emptying = false

    var body: some View {
        SectionTitle(text: "Damaged videos")
        Text(DamagedVideos.about).font(.caption).foregroundStyle(Palette.muted)
        if let loadError { Text("Couldn't read the damaged videos: \(loadError)").font(.caption).foregroundStyle(Palette.error) }
        if list?.videos?.isEmpty == true { Text("None found.").font(.subheadline) }
        ForEach(list?.videos ?? []) { v in
            VStack(alignment: .leading, spacing: 4) {
                Text(v.name).font(.headline)
                Text(DamagedVideos.whereText(v)).font(.caption).foregroundStyle(Palette.muted)
                Text(DamagedVideos.sizes(v)).font(.caption).foregroundStyle(Palette.muted)
                HStack {
                    Button("Play original") { Task { await play(v, original: true) } }.buttonStyle(PhoneSecondaryButtonStyle())
                    if v.fixed {
                        Button("Play fixed copy") { Task { await play(v, original: false) } }.buttonStyle(PhoneButtonStyle())
                    }
                }
                if v.fixed {
                    Button("Replace with fixed copy") { replacing = v }.buttonStyle(PhoneButtonStyle())
                }
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Palette.raised, in: RoundedRectangle(cornerRadius: 8))
        }
        MessageLine(message: message)
        Text(DamagedVideos.trashText(list?.trash)).font(.caption).foregroundStyle(Palette.muted)
        if (list?.trash?.files ?? 0) > 0 {
            Button("Empty the trash now") { emptying = true }.buttonStyle(PhoneSecondaryButtonStyle())
        }
        Color.clear.frame(height: 0)
            .task { await load() }
            .alert(replacing.map { "Replace \($0.name)?" } ?? "", isPresented: Binding(get: { replacing != nil }, set: { if !$0 { replacing = nil } })) {
                Button("Cancel", role: .cancel) { replacing = nil }
                Button("Replace") { if let v = replacing { Task { await replace(v) } }; replacing = nil }
            } message: {
                Text(replacing.map { "The fixed copy goes into its folder as \(DamagedVideos.replacedName($0.name)), with its watch progress, and the original goes to the trash (deleted after 30 days)." } ?? "")
            }
            .alert("Empty the trash?", isPresented: $emptying) {
                Button("Cancel", role: .cancel) {}
                Button("Empty the trash", role: .destructive) { Task { await emptyTrash() } }
            } message: {
                Text("Every replaced original in it is deleted now. They can't be brought back.")
            }
    }

    private func load() async {
        guard let client = model.client else { return }
        do {
            list = try await client.get("/api/admin/damaged") as DamagedList
            loadError = nil
        } catch {
            loadError = errorText(error)
        }
    }

    /// The server gives this account's players the original as it is (or the fixed copy), then it plays.
    private func play(_ v: DamagedVideo, original: Bool) async {
        guard let client = model.client else { return }
        do {
            _ = try await client.json("POST", "/api/admin/damaged/play", body: ["path": ServerClient.encodePath(v.path), "original": original])
            model.playServerFile(v.path, name: original ? "\(v.name) (original)" : "\(v.name) (fixed copy)")
        } catch {
            message = (errorText(error), true)
        }
    }

    private func replace(_ v: DamagedVideo) async {
        guard let client = model.client else { return }
        do {
            _ = try await client.json("POST", "/api/admin/damaged/replace", body: ["path": ServerClient.encodePath(v.path)])
            message = ("Replaced: \(DamagedVideos.replacedName(v.name)) is in its place, and the original is in the trash.", false)
        } catch {
            message = (errorText(error), true)
        }
        await load()
    }

    private func emptyTrash() async {
        guard let client = model.client else { return }
        do {
            _ = try await client.json("POST", "/api/admin/trash/empty")
            message = ("The trash is empty.", false)
        } catch {
            message = (errorText(error), true)
        }
        await load()
    }
}
