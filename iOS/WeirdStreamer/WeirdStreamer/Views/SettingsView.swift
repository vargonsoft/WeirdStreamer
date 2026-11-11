import SwiftUI
import SwiftData
import UniformTypeIdentifiers

/// Settings, as Android's PhoneSettingsScreen: a bar with Back, then tabs. App has the view
/// options and the saved servers (with their users, and Remove); Themes picks the look; Account
/// (with a server open) changes the password. The backup (servers, accounts, progress, view
/// choices) is this app's own, for an iPhone without iCloud.
struct SettingsView: View {
    @Environment(AppModel.self) private var model
    let onBack: () -> Void

    @State private var tab: String

    init(startTab: String = "App", onBack: @escaping () -> Void) {
        self.onBack = onBack
        _tab = State(initialValue: startTab)
    }

    /// As Android: Downloads and Account with a server open; Server, Users, Streams and Jobs for administrators.
    private var tabs: [String] {
        guard model.currentAddress != nil else { return ["App", "Themes"] }
        return ["App", "Themes", "Downloads"] + (model.isAdmin ? ["Server", "Users", "Streams", "Jobs"] : []) + ["Account"]
    }

    var body: some View {
        ZStack {
            ThemeBackground()
            VStack(spacing: 0) {
                Panel {
                    HStack(spacing: 4) {
                        BarButton(system: "arrow.left", label: "Back", action: onBack)
                        Text("Settings").font(.title3.weight(.medium))
                        Spacer()
                    }
                    .frame(height: 64)
                }
                .padding(.horizontal, 8).padding(.top, 8)
                ContentBox {
                    VStack(spacing: 0) {
                        TabStrip(tabs: tabs, selected: $tab)
                        switch tab {
                        case "Themes": ThemesTab()
                        case "Downloads": DownloadsTab()
                        case "Server": ServerSettingsTab()
                        case "Users": UsersTab()
                        case "Streams": StreamsTab()
                        case "Jobs": JobsTab()
                        case "Account": AccountTab()
                        default: AppTab(onBack: onBack)
                        }
                    }
                }
            }
        }
        .themed()
        .fullScreenCover(item: Binding(get: { model.playing }, set: { model.playing = $0 })) { request in
            PlayerView(request: request).environment(model)
        }
        .gesture(DragGesture(minimumDistance: 30).onEnded { drag in
            if drag.startLocation.x < 28 && drag.translation.width > 80 { onBack() }
        })
    }
}

/// A row of tabs with a gold line under the chosen one (Material's scrollable tab row).
struct TabStrip: View {
    let tabs: [String]
    @Binding var selected: String

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 0) {
                    ForEach(tabs, id: \.self) { name in
                        Button { selected = name } label: {
                            VStack(spacing: 0) {
                                Text(name).font(.subheadline.weight(.medium))
                                    .foregroundStyle(name == selected ? Palette.accent : Palette.muted)
                                    .padding(.horizontal, 16).frame(height: 46)
                                Rectangle().fill(name == selected ? Palette.accent : .clear).frame(height: 3)
                            }
                        }
                        .buttonStyle(.plain)
                        .id(name)
                    }
                }
                .padding(.horizontal, 8)
            }
            .onChange(of: selected) { _, name in
                withAnimation { proxy.scrollTo(name) }
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .overlay(alignment: .bottom) { Rectangle().fill(Palette.raised).frame(height: 1) }
    }
}

/// A list row: a title, an optional line under it, and something at the end.
struct SettingsRow<Trailing: View>: View {
    let title: String
    var detail: String? = nil
    @ViewBuilder let trailing: Trailing

    var body: some View {
        HStack(spacing: 16) {
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.body)
                if let detail { Text(detail).font(.subheadline).foregroundStyle(Palette.muted) }
            }
            Spacer()
            trailing
        }
        .padding(.horizontal, 16).padding(.vertical, 12)
        .contentShape(Rectangle())
    }
}

struct AppTab: View {
    @Environment(AppModel.self) private var model
    let onBack: () -> Void

    @State private var removing: String?
    @State private var exporting = false
    @State private var importing = false
    @State private var backup = BackupDocument(data: Data())
    @State private var notice: String?
    @State private var showSubtitleStyle = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                if model.currentAddress == nil {
                    Text("Open a WeirdStreamer server to manage it here.").font(.footnote).foregroundStyle(Palette.muted)
                        .padding(.horizontal, 16).padding(.vertical, 8)
                }
                SettingsRow(title: "Show hidden files") {
                    Toggle("", isOn: Binding(get: { model.showHiddenFiles },
                                             set: { model.setSetting("show_hidden_files", $0 ? "true" : "false") }))
                        .labelsHidden()
                }
                Button {
                    ImageCache.shared.removeAllObjects()
                    URLCache.shared.removeAllCachedResponses()
                    notice = "Thumbnail cache cleared."
                } label: {
                    SettingsRow(title: "Clear Thumbnail Cache") { Image(systemName: "trash.slash") }
                }
                .buttonStyle(.plain)
                Button { showSubtitleStyle = true } label: {
                    SettingsRow(title: "Subtitle style", detail: SubtitleStyle.current(model).summary) {
                        Image(systemName: "captions.bubble")
                    }
                }
                .buttonStyle(.plain)

                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("Backup").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent).padding(16)
                Button { exportBackup() } label: {
                    SettingsRow(title: "Export backup", detail: "Servers, accounts, progress and view choices, to a file in Files. Passwords and sign-ins stay on this phone.") {
                        Image(systemName: "square.and.arrow.up")
                    }
                }
                .buttonStyle(.plain)
                Button { importing = true } label: {
                    SettingsRow(title: "Import backup", detail: "Adds what the file has; sign in again to accounts it adds.") {
                        Image(systemName: "square.and.arrow.down")
                    }
                }
                .buttonStyle(.plain)
                if let notice {
                    Text(notice).font(.footnote).foregroundStyle(Palette.muted).padding(.horizontal, 16)
                }

                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("WeirdStreamer Server").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent).padding(16)
                if model.servers.isEmpty {
                    Text("No WeirdStreamer Servers saved.").font(.subheadline).foregroundStyle(Palette.muted)
                        .padding(.horizontal, 16).padding(.vertical, 8)
                }
                ForEach(model.servers, id: \.address) { server in
                    let users = model.accounts(of: server.address).map(\.username)
                    HStack(spacing: 16) {
                        Image(systemName: "dot.radiowaves.left.and.right").foregroundStyle(Palette.accent)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(server.address).font(.body)
                            Text(users.isEmpty ? "No users" : "Users: " + users.joined(separator: ", "))
                                .font(.subheadline).foregroundStyle(Palette.muted)
                        }
                        Spacer()
                        IconButton(system: "trash", label: "Remove") { removing = server.address }
                    }
                    .padding(.horizontal, 16).padding(.vertical, 8)
                }

                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("Version \(Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "")")
                    .font(.footnote).foregroundStyle(Palette.muted).padding(16)
            }
        }
        .overlay {
            if showSubtitleStyle {
                SubtitleStyleDialog { showSubtitleStyle = false }
            }
        }
        .overlay {
            if let address = removing {
                let users = model.accounts(of: address).map(\.username)
                PhoneDialog(title: "Remove server?", onDismiss: { removing = nil }) {
                    Text(removeServerWarning(address, users)).font(.subheadline)
                } buttons: {
                    Button("Cancel") { removing = nil }.buttonStyle(PhoneSecondaryButtonStyle())
                    Button("Remove") {
                        let isCurrent = address == model.currentAddress
                        model.removeServer(address)
                        removing = nil
                        if isCurrent { onBack() }
                    }
                    .buttonStyle(PhoneButtonStyle(destructive: true))
                }
            }
        }
        .fileExporter(isPresented: $exporting, document: backup, contentType: .json,
                      defaultFilename: "WeirdStreamer-backup") { result in
            if case .failure = result { notice = "The backup couldn't be saved." } else { notice = "Backup saved." }
        }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.json]) { result in
            if case .success(let url) = result { notice = Backup.restore(from: url, into: model) }
        }
    }

    private func exportBackup() {
        guard let data = Backup.make(from: model) else { notice = "The backup couldn't be made."; return }
        backup = BackupDocument(data: data)
        exporting = true
    }
}

/// What removing a server takes with it: its users' sign-ins on this phone.
func removeServerWarning(_ address: String, _ users: [String]) -> String {
    switch users.count {
    case 0: return "Remove \(address) from this device?"
    case 1: return "Remove \(address) from this device? \(users[0]) will be signed out and removed."
    default: return "Remove \(address) from this device? These users will be signed out and removed: \(users.joined(separator: ", "))."
    }
}

/// Themes: a grid of tiles drawn in each theme's colors; a tap applies it and keeps it (Android's PhoneThemePicker).
struct ThemesTab: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        let current = ThemeStore.shared.theme
        ScrollView {
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 104), spacing: 8)], spacing: 8) {
                ForEach(AppTheme.allCases, id: \.self) { theme in
                    Button {
                        ThemeStore.shared.theme = theme
                        model.setSetting("app_theme", theme.rawValue)
                    } label: {
                        VStack(spacing: 6) {
                            ThemePreview(theme: theme).frame(height: 58)
                            Text(theme.label).font(.subheadline.weight(theme == current ? .semibold : .regular))
                                .foregroundStyle(theme == current ? Palette.accent : Palette.text)
                                .lineLimit(1).minimumScaleFactor(0.8)
                        }
                        .padding(6)
                        .overlay(RoundedRectangle(cornerRadius: 8).stroke(theme == current ? Palette.accent : .clear, lineWidth: 2))
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(12)
        }
    }
}

/// The open account: who it is, and changing its password.
struct AccountTab: View {
    @Environment(AppModel.self) private var model
    @State private var current = ""
    @State private var password = ""
    @State private var confirm = ""
    @State private var message: (String, Bool)?
    @State private var working = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text("\(model.currentAccount ?? "Unknown")\(model.isAdmin ? " (administrator)" : "") on \(model.currentAddress ?? "")")
                    .font(.subheadline)
                PhoneTextField(label: "Current password", text: $current, secure: true)
                PhoneTextField(label: "New password", text: $password, secure: true)
                PhoneTextField(label: "Confirm password", text: $confirm, secure: true)
                if let message {
                    Text(message.0).font(.footnote).foregroundStyle(message.1 ? Palette.error : Palette.muted)
                }
                Button("Change password") { Task { await change() } }
                    .buttonStyle(PhoneButtonStyle())
                    .disabled(working || current.isEmpty || password.isEmpty)
            }
            .padding(16)
        }
    }

    private func change() async {
        guard password == confirm else { message = ("The new passwords don't match.", true); return }
        guard let client = model.client else { return }
        struct Body: Encodable { let old_password: String; let new_password: String }
        working = true
        defer { working = false }
        do {
            try await client.send("POST", "/api/users/change-password", body: Body(old_password: current, new_password: password))
            message = ("Password changed.", false)
            current = ""; password = ""; confirm = ""
        } catch {
            model.handle(error)
            message = (error.localizedDescription, true)
        }
    }
}

// MARK: Backup

/// The backup file: servers, accounts (names only), view choices and progress. No tokens or passwords.
struct BackupFile: Codable {
    struct Server: Codable { let address: String; let name: String?; let accounts: [String] }
    struct Progress: Codable {
        let address: String; let username: String; let path: String
        let position: Double; let duration: Double; let ended: Bool; let updatedAt: Date
    }
    let version: Int
    let exportedAt: Date
    let servers: [Server]
    let settings: [String: String]
    let progress: [Progress]
}

struct BackupDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.json] }
    var data: Data
    init(data: Data) { self.data = data }
    init(configuration: ReadConfiguration) throws {
        guard let file = configuration.file.regularFileContents else { throw CocoaError(.fileReadCorruptFile) }
        data = file
    }
    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper { FileWrapper(regularFileWithContents: data) }
}

@MainActor
enum Backup {
    static func make(from model: AppModel) -> Data? {
        let context = model.context
        let servers = (try? context.fetch(FetchDescriptor<SavedServer>())) ?? []
        let accounts = (try? context.fetch(FetchDescriptor<SavedAccount>())) ?? []
        let settings = (try? context.fetch(FetchDescriptor<AppSetting>())) ?? []
        let progress = (try? context.fetch(FetchDescriptor<LocalProgress>())) ?? []
        let file = BackupFile(
            version: 1, exportedAt: Date(),
            servers: servers.map { s in .init(address: s.address, name: s.name,
                                              accounts: accounts.filter { $0.address == s.address }.map(\.username)) },
            settings: Dictionary(settings.map { ($0.key, $0.value) }, uniquingKeysWith: { a, _ in a }),
            progress: progress.map { .init(address: $0.address, username: $0.username, path: $0.path, position: $0.position,
                                           duration: $0.duration, ended: $0.ended, updatedAt: $0.updatedAt) })
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        return try? encoder.encode(file)
    }

    /// Adds what's missing, keeps the newer of each progress position (queued for the server).
    static func restore(from url: URL, into model: AppModel) -> String {
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        guard let data = try? Data(contentsOf: url),
              let file = try? decoder.decode(BackupFile.self, from: data), file.version == 1 else {
            return "That isn't a WeirdStreamer backup."
        }
        let context = model.context
        for server in file.servers {
            let address = server.address
            if ((try? context.fetch(FetchDescriptor<SavedServer>(predicate: #Predicate { $0.address == address }))) ?? []).isEmpty {
                let new = SavedServer(address: address)
                new.name = server.name
                context.insert(new)
            }
            for username in server.accounts {
                let key = SavedAccount.key(address: address, username: username)
                if ((try? context.fetch(FetchDescriptor<SavedAccount>(predicate: #Predicate { $0.key == key }))) ?? []).isEmpty {
                    context.insert(SavedAccount(address: address, username: username))
                }
            }
        }
        for (key, value) in file.settings where model.setting(key) == nil { model.setSetting(key, value) }
        var restored = 0
        for p in file.progress {
            let key = LocalProgress.key(address: p.address, username: p.username, path: p.path)
            let existing = (try? context.fetch(FetchDescriptor<LocalProgress>(predicate: #Predicate { $0.key == key })))?.first
            guard existing == nil || p.updatedAt > existing!.updatedAt else { continue }
            let entry = existing ?? { let n = LocalProgress(address: p.address, username: p.username, path: p.path); context.insert(n); return n }()
            entry.position = p.position
            entry.duration = p.duration
            entry.ended = p.ended
            entry.updatedAt = p.updatedAt
            entry.pendingPush = true
            restored += 1
        }
        try? context.save()
        model.reloadServers()
        return "Restored \(file.servers.count) servers and \(restored) progress positions."
    }
}
