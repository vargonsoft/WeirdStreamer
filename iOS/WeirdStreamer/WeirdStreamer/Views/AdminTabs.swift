import SwiftUI

// Settings' tabs for administrators, as Android's PhoneServerSettings: Server (groups, libraries,
// the server's name and address, streaming, metadata keys), Users, Streams and Jobs.

/// A section heading in the accent color.
struct SectionTitle: View {
    let text: String
    var body: some View {
        Text(text).font(.headline).foregroundStyle(Palette.accent).padding(.top, 16).padding(.bottom, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// A line of feedback: muted, or in the error color.
struct MessageLine: View {
    let message: (String, Bool)?
    var body: some View {
        if let (text, isError) = message {
            Text(text).font(.footnote).foregroundStyle(isError ? Palette.error : Palette.muted)
        }
    }
}

/// A small card (Material's Card): the raised color with rounded corners.
struct CardBox<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        VStack(alignment: .leading, spacing: 4) { content }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Palette.raised.opacity(0.6), in: RoundedRectangle(cornerRadius: 12))
    }
}

/// The text of a server error, for the tabs.
func errorText(_ error: Error) -> String {
    if case ServerError.http(let status, let message) = error {
        if status == 403 { return "Only administrators can change these settings." }
        return message ?? "The server said no (\(status))."
    }
    return error.localizedDescription
}

private func pathEscape(_ s: String) -> String {
    s.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? s
}

// MARK: Server

private let libraryTypes = [("", "No type"), ("movie", "Movies"), ("tv", "TV Shows")]
private let libraryRowModes = [("on", "Home screen and library"), ("libraryOnly", "Library only"), ("off", "Off")]
// The Guide's prime time (guide.primeTime), the news hour after it
private let primeTimes = [("20", "8 to 11 pm"), ("19", "7 to 10 pm")]

/// A library being edited: the server's own entry (kept whole), and what the tab changes.
private struct PathEdit: Identifiable {
    let id = UUID()
    var source: [String: Any]
    var name: String
    var path: String
    var groups: [String]
    var type: String
    var continueWatching: String
    var newAdditions: String
    // The Guide: where its titles are on the channels (movie and TV libraries; showInGuide, as the rows' modes),
    // and whether its short videos fill gaps (guideFiller)
    var showInGuide: String
    var guideFiller: Bool
    var editing = false
}

struct ServerSettingsTab: View {
    @Environment(AppModel.self) private var model

    @State private var config: [String: Any]?
    @State private var loadError: String?
    @State private var groups: [String] = []
    @State private var paths: [PathEdit] = []
    @State private var serverName = ""
    @State private var port = ""
    @State private var host = ""
    @State private var http2 = false
    @State private var pauseJobs = false
    @State private var primeTime = "20" // the Guide's: the hour it starts
    @State private var weatherZip = "" // the Weather channel's US ZIP code ("" New York City's)
    // Hardware encoding: what the server found, and the choice ("" the CPU, else "id|device")
    @State private var hardware: HardwareReport? = HardwareReport(checking: true, gpus: nil, encoders: nil, problems: nil)
    @State private var hwEncoder = ""
    @State private var hwDevice = ""
    @State private var hwMax = "0"
    // Backups: the schedule (saved with the rest), and the backups in the folder config.yml names
    @State private var backupSchedule = "daily"
    @State private var backupDay = "sunday"
    @State private var backupTime = "04:00"
    @State private var backupKeep = "7"
    @State private var backups: BackupList?
    @State private var backingUp = false
    @State private var backupMessage: (String, Bool)?
    @State private var tmdbToken = ""
    @State private var openSubtitlesKey = ""
    @State private var language = ""
    @State private var newGroup = ""
    @State private var newPath = ""
    @State private var newType = ""
    @State private var renaming: String?
    @State private var renameTo = ""
    @State private var removing: UUID?
    @State private var groupMessage: (String, Bool)?
    @State private var message: (String, Bool)?
    @State private var saving = false

    var body: some View {
        Group {
            if config == nil, let loadError {
                PhoneMessage(text: loadError)
            } else if config == nil {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                form
            }
        }
        .task { await load() }
        .task { await loadHardware() }
        .task { await loadBackups() }
        .overlay { renameDialog }
        .overlay { removeDialog }
    }

    private var form: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                SectionTitle(text: "Media paths")
                ForEach($paths) { $p in libraryCard($p) }
                PhoneTextField(label: "New path on the server", text: $newPath, placeholder: "/path/to/media")
                OptionMenu(label: "Library type", options: libraryTypes, value: $newType)
                Button { addPath() } label: { Label("Add path", systemImage: "plus") }.buttonStyle(PhoneSecondaryButtonStyle())

                SectionTitle(text: "Groups")
                Text("Each user is in one group, and each library lists the groups that can see it. New users start in the default group (the star makes another one the default). Changes here apply right away.")
                    .font(.caption).foregroundStyle(Palette.muted)
                ForEach(groups, id: \.self) { g in
                    HStack {
                        Text(g)
                        if g == groups.first { Text("default").font(.caption2).foregroundStyle(Palette.accent) }
                        Spacer()
                        if g != groups.first {
                            IconButton(system: "star", label: "Make \(g) the default group") {
                                groupCall("PUT", "/api/groups/\(pathEscape(g))/default", nil, "\(g) is the default group: new users start in it.")
                            }
                        }
                        IconButton(system: "pencil", label: "Rename \(g)") { renaming = g; renameTo = g }
                        IconButton(system: "trash", label: "Remove \(g)", tint: Palette.error) {
                            groupCall("DELETE", "/api/groups/\(pathEscape(g))", nil, "Removed \(g).")
                        }
                    }
                }
                HStack(spacing: 8) {
                    PhoneTextField(label: "New group", text: $newGroup)
                    Button { addGroup() } label: { Label("Add", systemImage: "plus") }.buttonStyle(PhoneSecondaryButtonStyle())
                }
                MessageLine(message: groupMessage)

                SectionTitle(text: "Server")
                PhoneTextField(label: "Name", text: $serverName, hint: "Shown in the apps' server lists")
                HStack(spacing: 12) {
                    PhoneTextField(label: "Port", text: $port, keyboard: .numberPad).frame(maxWidth: 110)
                    PhoneTextField(label: "Host", text: $host, keyboard: .URL)
                }

                SectionTitle(text: "Streaming")
                Toggle("Enable HTTP/2", isOn: $http2)
                Toggle(isOn: $pauseJobs) {
                    VStack(alignment: .leading) {
                        Text("Pause background jobs while anyone is streaming")
                        Text("Finding intros and credits, trickplay frames, cleanup").font(.caption).foregroundStyle(Palette.muted)
                    }
                }
                SectionTitle(text: "Guide")
                OptionMenu(label: "Prime time (then the news hour)", options: primeTimes, value: $primeTime)
                PhoneTextField(label: "Weather ZIP code", text: $weatherZip, placeholder: "10001", keyboard: .numberPad,
                               hint: "The Weather channel's forecast is the National Weather Service's for it (US only)")
                hardwareSection
                backupSection
                DamagedVideosSection()

                SectionTitle(text: "Metadata")
                Text("TMDB is used for libraries with a Movies or TV Shows type.").font(.caption).foregroundStyle(Palette.muted)
                PhoneTextField(label: "TMDB API read access token", text: $tmdbToken, secure: true)
                PhoneTextField(label: "OpenSubtitles API key", text: $openSubtitlesKey, secure: true, hint: "For searching subtitles online")
                PhoneTextField(label: "Language", text: $language, placeholder: "en-US")

                MessageLine(message: message)
                HStack(spacing: 12) {
                    Button { Task { await save() } } label: {
                        Label(saving ? "Saving…" : "Save settings", systemImage: "square.and.arrow.down").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneButtonStyle())
                    .disabled(saving)
                    Button { Task { await scan() } } label: {
                        Label("Scan media", systemImage: "arrow.clockwise").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneSecondaryButtonStyle())
                }
                .padding(.top, 8)
            }
            .padding(16)
        }
    }

    private func libraryCard(_ p: Binding<PathEdit>) -> some View {
        CardBox {
            if !p.wrappedValue.editing {
                HStack {
                    Text(p.wrappedValue.name.isEmpty ? "(no name)" : p.wrappedValue.name).font(.headline)
                    Spacer()
                    IconButton(system: "pencil", label: "Edit") { p.wrappedValue.editing = true }
                }
                Text(p.wrappedValue.path).font(.caption).foregroundStyle(Palette.muted)
                Divider().overlay(Palette.muted.opacity(0.35)).padding(.vertical, 6)
                summaryRow("Type", libraryTypes.first { $0.0 == p.wrappedValue.type }?.1 ?? "No type")
                summaryRow("Groups", p.wrappedValue.groups.isEmpty ? "None (no one sees it)" : p.wrappedValue.groups.joined(separator: ", "))
                summaryRow("Continue Watching", libraryRowModes.first { $0.0 == p.wrappedValue.continueWatching }?.1 ?? p.wrappedValue.continueWatching)
                summaryRow("Recently Added", libraryRowModes.first { $0.0 == p.wrappedValue.newAdditions }?.1 ?? p.wrappedValue.newAdditions)
                if !p.wrappedValue.type.isEmpty {
                    summaryRow("Show in Guide", libraryRowModes.first { $0.0 == p.wrappedValue.showInGuide }?.1 ?? p.wrappedValue.showInGuide)
                }
                summaryRow("Guide filler", p.wrappedValue.guideFiller ? "Its short videos fill gaps" : "No")
            } else {
                VStack(alignment: .leading, spacing: 8) {
                    PhoneTextField(label: "Name", text: p.name)
                    Text("Groups:").font(.subheadline).foregroundStyle(Palette.muted)
                    ForEach(groups, id: \.self) { g in
                        Toggle(g, isOn: Binding(
                            get: { p.wrappedValue.groups.contains(g) },
                            set: { on in
                                if on { p.wrappedValue.groups.append(g) } else { p.wrappedValue.groups.removeAll { $0 == g } }
                            }))
                    }
                    OptionMenu(label: "Library type", options: libraryTypes, value: p.type)
                    OptionMenu(label: "Continue Watching", options: libraryRowModes, value: p.continueWatching)
                    OptionMenu(label: "Recently Added", options: libraryRowModes, value: p.newAdditions)
                    if !p.wrappedValue.type.isEmpty {
                        OptionMenu(label: "Show in Guide", options: libraryRowModes, value: p.showInGuide)
                    }
                    Toggle(isOn: p.guideFiller) {
                        VStack(alignment: .leading) {
                            Text("Use for Guide Filler")
                            Text("Its videos of 15 minutes or less fill gaps between the Guide's programs")
                                .font(.caption).foregroundStyle(Palette.muted)
                        }
                    }
                    Text(p.wrappedValue.path).font(.caption).foregroundStyle(Palette.muted)
                    HStack {
                        Spacer()
                        IconButton(system: "trash", label: "Remove", tint: Palette.error) { removing = p.wrappedValue.id }
                        Button { p.wrappedValue.editing = false } label: { Label("Done", systemImage: "checkmark") }
                            .buttonStyle(PhoneSecondaryButtonStyle())
                    }
                }
            }
        }
    }

    private func summaryRow(_ label: String, _ value: String) -> some View {
        HStack(alignment: .top) {
            Text(label).font(.subheadline).foregroundStyle(Palette.muted).frame(width: 140, alignment: .leading)
            Text(value).font(.subheadline)
        }
    }

    @ViewBuilder private var renameDialog: some View {
        if let old = renaming {
            PhoneDialog(title: "Rename \(old)", onDismiss: { renaming = nil }) {
                PhoneTextField(label: "New name", text: $renameTo)
            } buttons: {
                Button("Cancel") { renaming = nil }.buttonStyle(PhoneSecondaryButtonStyle())
                Button("Rename") {
                    let name = renameTo.trimmingCharacters(in: .whitespaces)
                    renaming = nil
                    guard !name.isEmpty, name != old else { return }
                    groupCall("PUT", "/api/groups/\(pathEscape(old))", ["name": name], "Renamed \(old) to \(name).")
                }
                .buttonStyle(PhoneButtonStyle())
            }
        }
    }

    @ViewBuilder private var removeDialog: some View {
        if let id = removing, let p = paths.first(where: { $0.id == id }) {
            PhoneDialog(title: "Remove \(p.name.isEmpty ? "this path" : p.name)?", onDismiss: { removing = nil }) {
                Text("It's removed from the server's libraries when you save. The files aren't touched.").font(.subheadline)
            } buttons: {
                Button("Cancel") { removing = nil }.buttonStyle(PhoneSecondaryButtonStyle())
                Button("Remove") {
                    paths.removeAll { $0.id == id }
                    removing = nil
                    message = ("Removed. Save to apply.", false)
                }
                .buttonStyle(PhoneButtonStyle(destructive: true))
            }
        }
    }

    /// Hardware encoding: the server's GPUs, and the encoder and limit when one works.
    @ViewBuilder private var hardwareSection: some View {
        Text("Hardware encoding").font(.subheadline.weight(.semibold)).padding(.top, 8)
        if let report = hardware {
            Text(HardwareSettings.status(report)).font(.caption).foregroundStyle(Palette.muted)
            if !report.checking, !(report.encoders ?? []).isEmpty {
                let chosen = HardwareSettings.chosenKey(report, encoder: hwEncoder, device: hwDevice)
                OptionMenu(label: "Encode transcodes on",
                           options: HardwareSettings.choices(report, encoder: hwEncoder, device: hwDevice),
                           value: Binding(get: { chosen }, set: { value in
                               let parts = value.split(separator: "|", maxSplits: 1, omittingEmptySubsequences: false).map(String.init)
                               hwEncoder = parts.first ?? ""
                               hwDevice = value.isEmpty ? "" : (parts.count > 1 ? parts[1] : "")
                           }))
                if !chosen.isEmpty {
                    PhoneTextField(label: "Transcodes on the GPU at once", text: $hwMax, keyboard: .numberPad,
                                   hint: "0: no limit; the rest use the CPU")
                }
                Text(HardwareSettings.note(report, chosen: chosen)).font(.caption).foregroundStyle(Palette.muted)
            }
        } else {
            Text("Couldn't read the server's graphics hardware.").font(.caption).foregroundStyle(Palette.muted)
        }
    }

    /// Backups: the schedule, Back up now, and the backups made.
    @ViewBuilder private var backupSection: some View {
        SectionTitle(text: "Backups")
        Text("Both databases (accounts, watch progress, matches, marked intros and credits) and config.yml. Restore one with bin/cli backup restore, with the server stopped.")
            .font(.caption).foregroundStyle(Palette.muted)
        OptionMenu(label: "Back up", options: BackupSettings.schedules, value: $backupSchedule)
        if backupSchedule == "weekly" {
            OptionMenu(label: "On", options: BackupSettings.days, value: $backupDay)
        }
        if backupSchedule != "off" {
            PhoneTextField(label: "At (24-hour, the server's time)", text: $backupTime, placeholder: "04:00", keyboard: .numbersAndPunctuation,
                           hint: BackupSettings.validTime(backupTime) ? nil : "A time of day, as 04:00")
        }
        PhoneTextField(label: "Backups kept", text: $backupKeep, keyboard: .numberPad, hint: "The newest; the oldest go")
        Text(backups.map(BackupSettings.whereText) ?? "Couldn't read the backups.").font(.caption).foregroundStyle(Palette.muted)
        if let backupMessage {
            Text(backupMessage.0).font(.caption).foregroundStyle(backupMessage.1 ? Palette.error : Palette.accent)
        }
        Button { Task { await backUpNow() } } label: { Text(backingUp ? "Backing up…" : "Back up now") }
            .buttonStyle(PhoneSecondaryButtonStyle()).disabled(backingUp)
        ForEach(backups?.backups ?? [], id: \.name) { b in
            Text("\(BackupSettings.date(b.created ?? "") ?? b.name) · \(formatBytes(b.size))").font(.caption)
        }
    }

    // MARK: Actions

    private func loadBackups() async {
        guard let client = model.client else { return }
        backups = try? await client.get("/api/admin/backups") as BackupList
    }

    private func backUpNow() async {
        guard let client = model.client else { return }
        backingUp = true
        backupMessage = nil
        do {
            let made = try await client.json("POST", "/api/admin/backups") as? [String: Any]
            let size = (made?["size"] as? NSNumber)?.int64Value
            backupMessage = ("Made \(made?["name"] as? String ?? "a backup") (\(formatBytes(size))).", false)
        } catch {
            model.handle(error)
            backupMessage = (errorText(error), true)
        }
        backingUp = false
        await loadBackups()
    }

    /// The server tries its GPUs' encoders at startup: asked again until that's done.
    private func loadHardware() async {
        guard let client = model.client else { return }
        while true {
            let report = try? await client.get("/api/admin/hardware") as HardwareReport
            hardware = report
            guard let report, report.checking else { return }
            try? await Task.sleep(for: .seconds(2))
        }
    }

    private func load() async {
        guard let client = model.client else { return }
        do {
            guard let c = try await client.json("GET", "/api/config") as? [String: Any] else { return }
            adopt(c)
        } catch {
            model.handle(error)
            loadError = errorText(error)
        }
    }

    private func adopt(_ c: [String: Any]) {
        config = c
        groups = c["groups"] as? [String] ?? []
        let media = c["media"] as? [String: Any]
        paths = (media?["paths"] as? [[String: Any]] ?? []).map { o in
            PathEdit(source: o, name: o["name"] as? String ?? "", path: o["path"] as? String ?? "",
                     groups: o["groups"] as? [String] ?? [], type: o["type"] as? String ?? "",
                     continueWatching: o["continueWatchingMode"] as? String ?? "on",
                     newAdditions: o["showNewAdditions"] as? String ?? "off",
                     showInGuide: o["showInGuide"] as? String ?? "off",
                     guideFiller: o["guideFiller"] as? Bool ?? false)
        }
        let server = c["server"] as? [String: Any]
        serverName = server?["name"] as? String ?? ""
        port = (server?["port"]).map { "\($0)" } ?? ""
        host = server?["host"] as? String ?? ""
        let streaming = c["streaming"] as? [String: Any]
        http2 = streaming?["enableHTTP2"] as? Bool ?? false
        pauseJobs = streaming?["pauseJobsWhileStreaming"] as? Bool ?? false
        primeTime = ((c["guide"] as? [String: Any])?["primeTime"] as? Int).flatMap { $0 > 0 ? "\($0)" : nil } ?? "20"
        weatherZip = (c["guide"] as? [String: Any])?["weatherZip"] as? String ?? ""
        hwEncoder = streaming?["hardwareEncoder"] as? String ?? ""
        hwDevice = streaming?["hardwareDevice"] as? String ?? ""
        hwMax = "\(streaming?["hardwareMaxTranscodes"] as? Int ?? 0)"
        let database = c["database"] as? [String: Any]
        backupSchedule = (database?["backupSchedule"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? "daily"
        backupDay = (database?["backupDay"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? "sunday"
        backupTime = (database?["backupTime"] as? String).flatMap { $0.isEmpty ? nil : $0 } ?? "04:00"
        backupKeep = "\((database?["backupKeep"] as? Int).flatMap { $0 > 0 ? $0 : nil } ?? 7)"
        let metadata = c["metadata"] as? [String: Any]
        tmdbToken = metadata?["tmdbAccessToken"] as? String ?? ""
        openSubtitlesKey = metadata?["openSubtitlesApiKey"] as? String ?? ""
        language = metadata?["language"] as? String ?? ""
    }

    /// The config to send: the server's own, with what the tab changed.
    private func buildConfig() -> [String: Any]? {
        guard var c = config else { return nil }
        var media = c["media"] as? [String: Any] ?? [:]
        media["paths"] = paths.map { p -> [String: Any] in
            var o = p.source
            o["name"] = p.name.trimmingCharacters(in: .whitespaces)
            o["path"] = p.path.trimmingCharacters(in: .whitespaces)
            o["groups"] = p.groups
            if p.type.isEmpty { o.removeValue(forKey: "type") } else { o["type"] = p.type }
            // Only where set before or changed from the default, so config.yml changes only where it was changed.
            if o["continueWatchingMode"] != nil || p.continueWatching != "on" { o["continueWatchingMode"] = p.continueWatching }
            if o["showNewAdditions"] != nil || p.newAdditions != "off" { o["showNewAdditions"] = p.newAdditions }
            if o["showInGuide"] != nil || p.showInGuide != "off" { o["showInGuide"] = p.showInGuide }
            if p.guideFiller { o["guideFiller"] = true } else { o.removeValue(forKey: "guideFiller") }
            return o
        }
        c["media"] = media
        c["groups"] = groups
        var server = c["server"] as? [String: Any] ?? [:]
        server["name"] = serverName.trimmingCharacters(in: .whitespaces)
        if let n = Int(port.trimmingCharacters(in: .whitespaces)) { server["port"] = n }
        server["host"] = host.trimmingCharacters(in: .whitespaces)
        c["server"] = server
        var streaming = c["streaming"] as? [String: Any] ?? [:]
        streaming["enableHTTP2"] = http2
        streaming["pauseJobsWhileStreaming"] = pauseJobs
        if !(hardware?.encoders ?? []).isEmpty {
            streaming["hardwareEncoder"] = hwEncoder
            streaming["hardwareDevice"] = hwEncoder.isEmpty ? "" : hwDevice
            streaming["hardwareMaxTranscodes"] = max(0, Int(hwMax.filter(\.isNumber)) ?? 0)
        }
        c["streaming"] = streaming
        var guide = c["guide"] as? [String: Any] ?? [:]
        guide["primeTime"] = Int(primeTime) ?? 20
        guide["weatherZip"] = String(weatherZip.filter(\.isNumber).prefix(5))
        c["guide"] = guide
        var database = c["database"] as? [String: Any] ?? [:]
        database["backupSchedule"] = backupSchedule
        database["backupDay"] = backupDay
        database["backupTime"] = BackupSettings.time(backupTime)
        database["backupKeep"] = min(365, max(1, Int(backupKeep.filter(\.isNumber)) ?? 7))
        c["database"] = database
        var metadata = c["metadata"] as? [String: Any] ?? [:]
        metadata["tmdbAccessToken"] = tmdbToken.trimmingCharacters(in: .whitespaces)
        metadata["openSubtitlesApiKey"] = openSubtitlesKey.trimmingCharacters(in: .whitespaces)
        metadata["language"] = language.trimmingCharacters(in: .whitespaces)
        c["metadata"] = metadata
        return c
    }

    private func save() async {
        guard let client = model.client, let body = buildConfig() else { return }
        saving = true
        defer { saving = false }
        do {
            let reply = try await client.json("PUT", "/api/config", body: body) as? [String: Any]
            if let c = reply?["config"] as? [String: Any] { adopt(c) }
            message = (reply?["message"] as? String ?? "Settings saved.", false)
            await loadBackups() // the next backup, by the schedule saved
        } catch {
            model.handle(error)
            message = (errorText(error), true)
        }
    }

    private func scan() async {
        guard let client = model.client else { return }
        do {
            _ = try await client.json("POST", "/api/media/scan")
            message = ("Scan started.", false)
        } catch {
            message = (errorText(error), true)
        }
    }

    private func addGroup() {
        let name = newGroup.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty else { return }
        groupCall("POST", "/api/groups", ["name": name], "Added \(name).") { newGroup = "" }
    }

    /// A change to the groups: applies right away, and into the settings a Save sends back.
    private func groupCall(_ method: String, _ path: String, _ body: [String: Any]?, _ done: String, after: (() -> Void)? = nil) {
        guard let client = model.client else { return }
        Task {
            do {
                if let list = try await client.json(method, path, body: body) as? [String] {
                    groups = list
                    config?["groups"] = list
                    groupMessage = (done, false)
                    after?()
                }
            } catch {
                groupMessage = (errorText(error), true)
            }
        }
    }

    private func addPath() {
        let path = newPath.trimmingCharacters(in: .whitespaces)
        if path.isEmpty { message = ("Enter a path", true); return }
        if paths.contains(where: { $0.path == path }) { message = ("That path is already listed", true); return }
        let name = (path.hasSuffix("/") ? String(path.dropLast()) : path).components(separatedBy: "/").last ?? path
        paths.append(PathEdit(source: [:], name: name.isEmpty ? path : name, path: path, groups: [], type: newType,
                              continueWatching: "on", newAdditions: "off", showInGuide: "off", guideFiller: false))
        newPath = ""
        newType = ""
        message = ("Added. Save to apply.", false)
    }
}

/// A labeled choice among options (Android's dropdown).
struct OptionMenu: View {
    let label: String
    let options: [(String, String)]
    @Binding var value: String

    var body: some View {
        HStack {
            Text(label).font(.subheadline).foregroundStyle(Palette.muted)
            Spacer()
            Menu {
                ForEach(options, id: \.0) { option in
                    Button { value = option.0 } label: {
                        if option.0 == value { Label(option.1, systemImage: "checkmark") } else { Text(option.1) }
                    }
                }
            } label: {
                HStack(spacing: 4) {
                    Text(options.first { $0.0 == value }?.1 ?? value)
                    Image(systemName: "chevron.up.chevron.down").font(.caption)
                }
            }
        }
    }
}

// MARK: Users

struct UsersTab: View {
    @Environment(AppModel.self) private var model

    @State private var users: [AdminUser]?
    @State private var groups: [String] = []
    @State private var message: (String, Bool)?
    @State private var deleting: AdminUser?
    @State private var newName = ""
    @State private var newPassword = ""
    @State private var newGroup = ""
    @State private var newAdmin = false

    var body: some View {
        let me = model.client?.claims.username
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                SectionTitle(text: "Create a user")
                PhoneTextField(label: "Username", text: $newName)
                PhoneTextField(label: "Password", text: $newPassword, secure: true)
                OptionMenu(label: "Group", options: groups.map { ($0, $0) }, value: $newGroup)
                Toggle("Administrator", isOn: $newAdmin)
                Button { Task { await create() } } label: { Label("Create user", systemImage: "person.badge.plus") }
                    .buttonStyle(PhoneButtonStyle())
                MessageLine(message: message)
                SectionTitle(text: "Users")
                if let users {
                    ForEach(users, id: \.username) { user in
                        let isMe = user.username == me
                        CardBox {
                            HStack {
                                Text(user.username).font(.headline)
                                if user.isAdmin == true { Chip(text: "Admin") }
                                if isMe { Chip(text: "You") }
                                Spacer()
                                if !isMe {
                                    IconButton(system: "trash", label: "Delete \(user.username)", tint: Palette.error) { deleting = user }
                                }
                            }
                            HStack {
                                OptionMenu(label: "Group", options: groups.map { ($0, $0) }, value: Binding(
                                    get: { user.group ?? "" }, set: { update(user, ["group": $0]) }))
                                Toggle("Admin", isOn: Binding(get: { user.isAdmin == true }, set: { update(user, ["is_admin": $0]) }))
                                    .disabled(isMe)
                                    .frame(maxWidth: 120)
                            }
                        }
                    }
                } else {
                    ProgressView()
                }
            }
            .padding(16)
        }
        .task { await reload() }
        .overlay {
            if let user = deleting {
                PhoneDialog(title: "Delete \(user.username)?", onDismiss: { deleting = nil }) {
                    Text("They won't be able to sign in any more.").font(.subheadline)
                } buttons: {
                    Button("Cancel") { deleting = nil }.buttonStyle(PhoneSecondaryButtonStyle())
                    Button("Delete") {
                        deleting = nil
                        Task {
                            do {
                                _ = try await model.client?.json("DELETE", "/api/users", query: [("username", user.username)])
                                message = ("Deleted \(user.username).", false)
                            } catch { message = (errorText(error), true) }
                            await reload()
                        }
                    }
                    .buttonStyle(PhoneButtonStyle(destructive: true))
                }
            }
        }
    }

    private func reload() async {
        guard let client = model.client else { return }
        do {
            users = try await client.get("/api/users")
        } catch {
            model.handle(error)
            message = (errorText(error), true)
        }
        groups = (try? await client.json("GET", "/api/groups") as? [String]) ?? groups
        if !groups.contains(newGroup) { newGroup = groups.first ?? "" }
    }

    private func create() async {
        guard let client = model.client else { return }
        let name = newName.trimmingCharacters(in: .whitespaces)
        guard !name.isEmpty, !newPassword.isEmpty else { message = ("Enter a username and password", true); return }
        guard groups.contains(newGroup) else { message = ("Choose a group for the user", true); return }
        do {
            _ = try await client.json("POST", "/api/users", body: ["username": name, "password": newPassword,
                                                                   "is_admin": newAdmin, "group": newGroup])
            message = ("Created \(name).", false)
            newName = ""; newPassword = ""; newAdmin = false; newGroup = groups.first ?? ""
        } catch {
            message = (errorText(error), true)
        }
        await reload()
    }

    private func update(_ user: AdminUser, _ changes: [String: Any]) {
        guard let client = model.client else { return }
        Task {
            do {
                _ = try await client.json("PUT", "/api/users", query: [("username", user.username)], body: changes)
                message = ("Updated \(user.username).", false)
            } catch { message = (errorText(error), true) }
            await reload()
        }
    }
}

struct Chip: View {
    let text: String
    var body: some View {
        Text(text).font(.caption.weight(.medium)).padding(.horizontal, 8).padding(.vertical, 3)
            .overlay(RoundedRectangle(cornerRadius: 5).stroke(Palette.muted, lineWidth: 1))
    }
}

// MARK: Streams

/// Who's watching what, every 3 seconds.
struct StreamsTab: View {
    @Environment(AppModel.self) private var model
    @State private var streams: [AdminStream]?
    @State private var error: String?

    var body: some View {
        Group {
            if let streams, !streams.isEmpty {
                ScrollView {
                    VStack(spacing: 12) { ForEach(streams) { StreamCard(s: $0) } }.padding(16)
                }
            } else if streams != nil {
                PhoneMessage(text: "Nothing is playing.")
            } else if let error {
                PhoneMessage(text: error)
            } else {
                ProgressView().padding(32).frame(maxWidth: .infinity)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .task {
            while !Task.isCancelled {
                do {
                    streams = try await model.client?.get("/api/admin/streams")
                    error = nil
                } catch { self.error = errorText(error) }
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }
}

private struct StreamCard: View {
    let s: AdminStream

    var body: some View {
        CardBox {
            Text(s.title ?? s.name ?? "").font(.headline)
            let state = s.state ?? ""
            Text("\(state.prefix(1).uppercased() + state.dropFirst()) · \(s.mode == "transcode" ? "Transcoding" : "Direct play")")
                .font(.subheadline.weight(.medium))
                .foregroundStyle(state == "playing" ? Color(hex: 0x2E7D32) : (state == "paused" ? Color(hex: 0xB7791F) : Palette.muted))
            Text("\(s.user ?? "") · \(s.client ?? "") · \(s.clientIp ?? "")").font(.caption).foregroundStyle(Palette.muted)
            let position = s.position ?? s.servedTo ?? 0
            if let d = s.duration, d > 0 {
                ProgressView(value: min(max(position / d, 0), 1)).tint(Palette.accent)
            }
            Text("\(clockText(position)) / \(clockText(s.duration ?? 0))" + (s.position == nil ? " (estimated)" : "")).font(.caption)
            Divider()
            Text(s.name ?? "").font(.caption)
            if let w = s.width, w > 0 { Text("Source \(w)×\(s.height ?? 0)").font(.caption) }
            if let t = s.transcode {
                Text(t.reason ?? "").font(.caption)
                Text("Video: \(t.video ?? "")").font(.caption)
                Text("Audio: \(t.audio ?? "")").font(.caption)
                if let source = t.sourceVideo { Text("Source: \(source)").font(.caption) }
                Text(t.stalled == true ? "Transcoder waiting for the player"
                     : String(format: "Transcoder %.2f× realtime · %.0f fps", t.speed ?? 0, t.fps ?? 0)).font(.caption)
            }
        }
    }
}

// MARK: Jobs

/// The server's background jobs: what each is doing, its last and next run, Run now and Pause;
/// trickplay's settings; and what the server's data folders hold.
struct JobsTab: View {
    @Environment(AppModel.self) private var model
    @State private var data: JobsResponse?
    @State private var error: String?
    @State private var now = Date()

    var body: some View {
        Group {
            if let data {
                ScrollView {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Jobs run at low priority, so playback gets the CPU and disks first. They wait while a disk, network share or mergerfs pool is busy (or, with the setting under Streaming, while anyone is streaming). Pause holds a job until you resume it, or the server restarts.")
                            .font(.caption).foregroundStyle(Palette.muted)
                        if let load = data.load { Text(loadText(load)).font(.caption).foregroundStyle(Palette.muted) }
                        if let error { Text(error).font(.caption).foregroundStyle(Palette.error) }
                        if let tp = data.trickplay { TrickplayBox(settings: tp) { self.data = $0 } }
                        ForEach(data.jobs ?? []) { job in JobCard(job: job, now: now) { control(job.id, $0) } }
                        StorageCard(storage: data.storage ?? [])
                    }
                    .padding(16)
                }
            } else if let error {
                PhoneMessage(text: error)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task {
            while !Task.isCancelled {
                do {
                    data = try await model.client?.get("/api/admin/jobs")
                    error = nil
                } catch { self.error = errorText(error) }
                try? await Task.sleep(for: .seconds(5))
            }
        }
        .task {
            while !Task.isCancelled {
                now = Date()
                try? await Task.sleep(for: .seconds(1))
            }
        }
    }

    private func control(_ id: String, _ action: String) {
        Task {
            do {
                struct Empty: Encodable {}
                data = try await model.client?.call("POST", "/api/admin/jobs/\(id)/\(action)", body: Empty())
            } catch { self.error = errorText(error) }
        }
    }

    private func loadText(_ l: JobLoad) -> String {
        func pct(_ f: Double?) -> String { "\(Int(((f ?? 0) * 100).rounded()))%" }
        var parts: [String] = []
        if l.available == true {
            parts += ["CPU \(pct(l.cpu))",
                      "Disk \(pct(l.disk)) (jobs wait above \(pct(l.maxDisk)) on a disk someone is playing from, \(pct(l.relaxedDisk)) otherwise)"]
            if let shares = l.shares, !shares.isEmpty {
                // Libraries on network shares or FUSE pools (mergerfs), whose disks can't be told apart
                parts.append("Read times \(shares.map { "\($0.library ?? "") \(Int(($0.ms ?? 0).rounded())) ms" }.joined(separator: ", ")) (above \(Int((l.maxShareMs ?? 0).rounded())) ms where someone is playing, \(Int((l.relaxedShareMs ?? 0).rounded())) ms otherwise)")
            }
        } else {
            parts.append("No load readings: jobs wait while anyone is streaming")
        }
        if l.streaming == true { parts.append("Someone is streaming") }
        if l.pauseWhileStreaming == true { parts.append("Jobs pause while anyone streams") }
        return parts.joined(separator: " · ")
    }
}

private func isoDate(_ s: String?) -> Date? {
    guard let s else { return nil }
    let f = ISO8601DateFormatter()
    f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    return f.date(from: s) ?? ISO8601DateFormatter().date(from: s)
}

/// A span of time: "2h 05m 09s", "4m 09s", "12s" (days as "3d 4h").
private func jobDuration(_ seconds: TimeInterval) -> String {
    let s = max(0, Int(seconds))
    let d = s / 86400, h = s % 86400 / 3600, m = s % 3600 / 60, sec = s % 60
    if d > 0 { return "\(d)d \(h)h" }
    if h > 0 { return String(format: "%dh %02dm %02ds", h, m, sec) }
    if m > 0 { return String(format: "%dm %02ds", m, sec) }
    return "\(sec)s"
}

private func jobWhen(_ at: Date, _ now: Date) -> String {
    let ago = now.timeIntervalSince(at)
    if ago < 60 { return "just now" }
    if ago < 86400 { return jobDuration(ago).replacingOccurrences(of: #" \d+s$"#, with: "", options: .regularExpression) + " ago" }
    return at.formatted(date: .numeric, time: .shortened)
}

private struct JobCard: View {
    let job: BackgroundJob
    let now: Date
    let control: (String) -> Void

    var body: some View {
        let started = isoDate(job.started)
        CardBox {
            HStack {
                Text(job.name ?? job.id).font(.headline)
                Spacer()
                let state = job.state ?? "idle"
                Text(state.prefix(1).uppercased() + state.dropFirst()).font(.subheadline.weight(.medium))
                    .foregroundStyle(state == "running" ? Color(hex: 0x2E7D32) : state == "waiting" ? Color(hex: 0xB7791F)
                                     : state == "paused" ? Color(hex: 0xC62828) : Palette.muted)
            }
            Text(job.about ?? "").font(.caption).foregroundStyle(Palette.muted)
            if let started {
                if let total = job.total, total > 0 {
                    ProgressView(value: Double(job.done ?? 0) / Double(total)).tint(Palette.accent)
                }
                let count = (job.total ?? 0) > 0 ? "\((job.done ?? 0).formatted()) of \((job.total ?? 0).formatted()) · " : ""
                Text("\(count)running for \(jobDuration(now.timeIntervalSince(started)))").font(.caption)
                if let detail = job.detail, !detail.isEmpty { Text(detail).font(.caption) }
                ForEach(Array((job.runners ?? []).enumerated()), id: \.offset) { i, file in
                    if !file.isEmpty { Text("Runner \(i + 1): \(file)").font(.caption) }
                }
            }
            if job.state == "waiting", let note = job.note, !note.isEmpty {
                Text("Waiting: \(note)").font(.caption.weight(.semibold))
            }
            if let end = isoDate(job.lastEnd) {
                let took = jobDuration(end.timeIntervalSince(isoDate(job.lastStart) ?? end))
                Text("Last run: \(jobWhen(end, now)), took \(took)" + ((job.lastResult?.isEmpty == false) ? " · \(job.lastResult!)" : ""))
                    .font(.caption)
            } else {
                Text("Last run: not since the server started").font(.caption)
            }
            let next = isoDate(job.next)
            let nextText: String = started != nil ? "Next run: after this one"
                : next.map { $0 > now ? "Next run: in \(jobDuration($0.timeIntervalSince(now)))" : "Next run: any moment" }
                ?? "Runs: \(job.schedule ?? "when asked")"
            Text(nextText + (job.paused == true ? " (paused)" : "")).font(.caption)
            HStack(spacing: 8) {
                Button("Run now") { control("run") }.buttonStyle(PhoneButtonStyle())
                    .disabled(job.canRun != true || started != nil)
                Button(job.paused == true ? "Resume" : "Pause") { control(job.paused == true ? "resume" : "pause") }
                    .buttonStyle(PhoneButtonStyle())
            }
            .padding(.top, 4)
        }
    }
}

/// The trickplay memory table: its summary, then a row for the server's memory and one for an NVIDIA card's, with
/// what the settings need with 4K videos, what there is, and whether it fits.
private struct TrickplayMemoryTableView: View {
    let table: TrickplayTable

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(table.summary).font(.caption).foregroundStyle(Palette.muted)
            if !table.rows.isEmpty {
                Grid(alignment: .leading, horizontalSpacing: 10, verticalSpacing: 2) {
                    GridRow {
                        Text("")
                        Text("Needs with 4K").gridColumnAlignment(.trailing)
                        Text("Has").gridColumnAlignment(.trailing)
                        Text("")
                    }
                    .fontWeight(.semibold)
                    ForEach(table.rows, id: \.what) { row in
                        GridRow {
                            Text(row.what)
                            Text(row.needs).monospacedDigit()
                            Text(row.has).monospacedDigit()
                            Text(row.note).foregroundStyle(row.warn ? Color.red : Palette.muted)
                        }
                    }
                }
                .font(.caption).foregroundStyle(Palette.muted)
            }
        }
    }
}

/// Trickplay's runners and frames per runner, saved to the server's settings (an empty field is automatic,
/// sent as 0).
private struct TrickplayBox: View {
    @Environment(AppModel.self) private var model
    let settings: TrickplaySettings
    let onSaved: (JobsResponse) -> Void

    @State private var runners = ""
    @State private var frames = ""
    @State private var edited = false
    @State private var message: (String, Bool)?

    var body: some View {
        let r = Int(runners) ?? 0, f = Int(frames) ?? 0 // 0: automatic
        let (er, ef) = settings.effective(runners, frames)
        CardBox {
            Text("Trickplay").font(.headline)
            HStack(spacing: 8) {
                PhoneTextField(label: "Runners", text: Binding(get: { runners }, set: { runners = $0.filter(\.isNumber); edited = true }),
                               placeholder: "Auto: \(settings.effective("", "").0)", keyboard: .numberPad)
                PhoneTextField(label: "Frames per runner", text: Binding(get: { frames }, set: { frames = $0.filter(\.isNumber); edited = true }),
                               placeholder: "Auto: \(settings.effective(runners, "").1)", keyboard: .numberPad)
            }
            // Saved at once, apart from the fields.
            if let gpu = settings.gpu, !gpu.isEmpty {
                Toggle(isOn: Binding(get: { settings.useGpu ?? false }, set: { on in save(useGpu: on) })) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Decode on the GPU")
                        Text("\(gpu); off, the CPU decodes").font(.caption).foregroundStyle(Palette.muted)
                    }
                }
            }
            TrickplayMemoryTableView(table: trickplayMemoryTable(er, ef, settings.memory))
            Text("Runners 1 to \(settings.maxRunners ?? 8), frames per runner 1 to \(settings.maxFrames ?? 50), at most \(settings.maxProduct ?? 100) frames at once. Empty is automatic: a runner per disk the libraries are on, and frames filling half the server's memory with 4K videos.")
                .font(.caption).foregroundStyle(Palette.muted)
            HStack(spacing: 12) {
                Button("Save") {
                    Task {
                        do {
                            struct Body: Encodable { let runners: Int; let frames_per_runner: Int }
                            if let reply: JobsResponse = try await model.client?.call("POST", "/api/admin/jobs/trickplay/settings",
                                                                                       body: Body(runners: r, frames_per_runner: f)) {
                                onSaved(reply)
                            }
                            edited = false
                            message = ("Saved. Applies from the next batch.", false)
                        } catch { message = (errorText(error), true) }
                    }
                }
                .buttonStyle(PhoneButtonStyle())
                MessageLine(message: message)
            }
        }
        .onAppear { sync() }
        .onChange(of: settings.runners) { sync() }
        .onChange(of: settings.framesPerRunner) { sync() }
        .onChange(of: settings.autoRunners) { sync() }
        .onChange(of: settings.autoFrames) { sync() }
    }

    private func save(useGpu: Bool) {
        Task {
            do {
                struct Body: Encodable { let use_gpu: Bool }
                if let reply: JobsResponse = try await model.client?.call("POST", "/api/admin/jobs/trickplay/settings", body: Body(use_gpu: useGpu)) {
                    onSaved(reply)
                }
            } catch { message = (errorText(error), true) }
        }
    }

    private func sync() {
        guard !edited else { return }
        runners = settings.autoRunners == true ? "" : String(settings.runners ?? 1)
        frames = settings.autoFrames == true ? "" : String(settings.framesPerRunner ?? 10)
    }
}

private struct StorageCard: View {
    let storage: [StorageUse]

    var body: some View {
        CardBox {
            Text("Storage").font(.headline)
            if storage.isEmpty {
                Text("Counting…").font(.caption)
            } else {
                ForEach(storage, id: \.name) { line($0.name, $0.bytes, bold: false) }
                Divider()
                line("Total", storage.reduce(0) { $0 + $1.bytes }, bold: true)
            }
        }
    }

    private func line(_ name: String, _ bytes: Int64, bold: Bool) -> some View {
        HStack {
            Text(name).font(.subheadline.weight(bold ? .bold : .regular))
            Spacer()
            VStack(alignment: .trailing) {
                Text(formatBytes(bytes).isEmpty ? "0 B" : formatBytes(bytes)).font(.subheadline.weight(.semibold))
                Text("\(bytes.formatted()) bytes").font(.caption).foregroundStyle(Palette.muted)
            }
        }
    }
}
