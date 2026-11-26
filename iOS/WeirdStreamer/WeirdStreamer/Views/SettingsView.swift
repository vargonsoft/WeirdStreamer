import SwiftUI
import SwiftData
import UniformTypeIdentifiers

/// Settings, as Android's PhoneSettingsScreen: a bar with Back, then tabs. App has the view
/// options and the saved servers (with their users, and Remove); Account (with a server open) signs
/// a TV in, and has the languages and the password; Themes picks the look. The backup (servers, accounts, progress, view
/// choices) is this app's own, for an iPhone without iCloud.
struct SettingsView: View {
    @Environment(AppModel.self) private var model
    let onBack: () -> Void

    /// "0.0.1.496", as Android's versionName: the version and the build number (the commit count).
    static var version: String {
        let info = Bundle.main.infoDictionary ?? [:]
        let short = info["CFBundleShortVersionString"] as? String ?? ""
        let build = info["CFBundleVersion"] as? String ?? ""
        return build.isEmpty ? short : "\(short).\(build)"
    }

    @State private var tab: String

    init(startTab: String = "App", onBack: @escaping () -> Void) {
        self.onBack = onBack
        _tab = State(initialValue: startTab)
    }

    /// As Android: this phone's and the account's tabs first (Account and Downloads with a server
    /// open), then the server's for administrators (Server, Users, Streams and Jobs).
    private var tabs: [String] {
        guard model.currentAddress != nil else { return ["App", "Themes"] }
        return ["App", "Account", "Downloads", "Themes"] + (model.isAdmin ? ["Server", "Users", "Streams", "Jobs"] : [])
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
                SettingsRow(title: "Rewind 5 seconds when resuming") {
                    Toggle("", isOn: Binding(get: { model.rewindOnResume },
                                             set: { model.setSetting("rewind_on_resume", $0 ? "true" : "false") }))
                        .labelsHidden()
                }
                SettingsRow(title: "Mark Guide shows watched",
                            detail: "A show the Guide plays to its end while you're on its channel is marked watched") {
                    Toggle("", isOn: Binding(get: { model.guideMarksWatched },
                                             set: { model.setSetting("guide_marks_watched", $0 ? "true" : "false") }))
                        .labelsHidden()
                }
                SettingsRow(title: "Play theme music on title pages",
                            detail: "A show's or movie's theme plays quietly while its page is open") {
                    Toggle("", isOn: Binding(get: { model.themeMusic },
                                             set: { model.setSetting("theme_music", $0 ? "true" : "false") }))
                        .labelsHidden()
                }
                if model.themeMusic {
                    SettingsRow(title: "Theme music volume") {
                        Picker("Theme music volume", selection: Binding(get: { model.themeVolume },
                                                                        set: { model.setSetting("theme_volume", $0) })) {
                            ForEach(themeVolumes.map { $0.0 }, id: \.self) { level in
                                Text(themeVolumes.first { $0.0 == level }?.1 ?? level).tag(level)
                            }
                        }
                        .pickerStyle(.segmented).fixedSize()
                    }
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
                Text("Version \(SettingsView.version)")
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

/// The open account: who it is, changing its password, and signing in a TV with the code it shows.
struct AccountTab: View {
    @Environment(AppModel.self) private var model
    @State private var current = ""
    @State private var password = ""
    @State private var confirm = ""
    @State private var message: (String, Bool)?
    @State private var working = false
    @State private var tvCode = ""
    @State private var tvMessage: (String, Bool)?
    @State private var pairing = false
    @State private var account: AccountSettings?
    @State private var languageMessage: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text("\(model.currentAccount ?? "Unknown")\(model.isAdmin ? " (administrator)" : "") on \(model.currentAddress ?? "")")
                    .font(.subheadline)

                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("Sign in on a TV").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent)
                Text("Enter the code the TV shows, and the TV signs in as \(model.currentAccount ?? "this account").")
                    .font(.footnote).foregroundStyle(Palette.muted)
                // PhoneTextField's look, with capitals: its own field turns autocapitalization off.
                VStack(alignment: .leading, spacing: 2) {
                    Text("TV code").font(.caption).foregroundStyle(Palette.accent)
                    TextField("Code on the TV", text: $tvCode)
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()
                        .font(.body)
                        .onChange(of: tvCode) { _, value in
                            let limited = String(value.uppercased().prefix(8))
                            if limited != value { tvCode = limited }
                        }
                }
                .padding(.horizontal, 16).padding(.vertical, 8)
                .background(Palette.raised, in: UnevenRoundedRectangle(topLeadingRadius: 4, topTrailingRadius: 4))
                .overlay(alignment: .bottom) { Rectangle().fill(Palette.accent).frame(height: 1) }
                if let tvMessage {
                    Text(tvMessage.0).font(.footnote).foregroundStyle(tvMessage.1 ? Palette.error : Palette.muted)
                }
                Button("Sign in on the TV") { Task { await signInTV() } }
                    .buttonStyle(PhoneButtonStyle())
                    .disabled(pairing || tvCode.trimmingCharacters(in: .whitespaces).isEmpty)
                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("Languages").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent)
                Text("Videos start with audio in this language when they have it, and with subtitles in this one if subtitles are shown by default. On every device.")
                    .font(.footnote).foregroundStyle(Palette.muted)
                if let account {
                    languageMenu("Audio", account.audioLanguage) { code in
                        var next = account; next.audioLanguage = code; Task { await setLanguages(next) }
                    }
                    languageMenu("Subtitles", account.subtitleLanguage) { code in
                        var next = account; next.subtitleLanguage = code; Task { await setLanguages(next) }
                    }
                    Toggle("Show subtitles by default", isOn: Binding(get: { account.subtitlesOn }, set: { on in
                        var next = account; next.subtitlesOn = on; Task { await setLanguages(next) }
                    }))
                    .tint(Palette.accent)
                }
                if let languageMessage {
                    Text(languageMessage).font(.footnote).foregroundStyle(Palette.error)
                }

                if let address = model.currentAddress, let username = model.currentAccount {
                    Divider().overlay(Palette.raised).padding(.vertical, 8)
                    PinSettings(address: address, username: username)
                }

                Divider().overlay(Palette.raised).padding(.vertical, 8)
                Text("Change password").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent)
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
        .task { await loadAccount() }
    }

    private func languageLabel(_ code: String) -> String { code.isEmpty ? "File default" : languageName(code) ?? code }

    private func languageMenu(_ label: String, _ value: String, set: @escaping (String) -> Void) -> some View {
        HStack {
            Text(label).frame(width: 90, alignment: .leading)
            Menu {
                Picker(label, selection: Binding(get: { value }, set: set)) {
                    Text("File default").tag("")
                    ForEach((account?.languages ?? []).sorted { languageLabel($0) < languageLabel($1) }, id: \.self) { code in
                        Text(languageLabel(code)).tag(code)
                    }
                }
            } label: {
                Text(languageLabel(value))
            }
            .buttonStyle(PhoneButtonStyle())
        }
    }

    private func loadAccount() async {
        guard let client = model.client else { return }
        do { account = try await client.get("/api/account") } catch { languageMessage = error.localizedDescription }
    }

    /// The account's languages for audio and subtitles, kept on the server for every device.
    private func setLanguages(_ next: AccountSettings) async {
        guard let client = model.client else { return }
        do {
            try await client.send("PUT", "/api/account", body: AccountSettings(audioLanguage: next.audioLanguage, subtitleLanguage: next.subtitleLanguage,
                                                                                subtitlesOn: next.subtitlesOn),
                                  snakeCase: true)
            account = next
            languageMessage = nil
        } catch {
            model.handle(error)
            languageMessage = error.localizedDescription
        }
    }

    private func signInTV() async {
        guard let client = model.client else { return }
        pairing = true
        defer { pairing = false }
        do {
            try await client.claimPairingCode(tvCode)
            tvMessage = ("The TV is signing in.", false)
            tvCode = ""
        } catch {
            model.handle(error)
            tvMessage = (error.localizedDescription, true)
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

/// Settings › Account: this account's PIN on this phone (Android's PhonePinSettings): set, change or remove it.
struct PinSettings: View {
    let address: String
    let username: String

    @State private var has = false
    @State private var editing: Bool?   // true: set or change, false: remove
    @State private var current = ""
    @State private var pin = ""
    @State private var confirm = ""
    @State private var message: (String, Bool)?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("PIN on this phone").font(.subheadline.weight(.medium)).foregroundStyle(Palette.accent)
            Text(has ? "Opening \(username) on this phone asks for its PIN."
                 : "Anyone using this phone can open \(username). A PIN of \(PinStore.minLength) or more digits keeps it to you.")
                .font(.footnote).foregroundStyle(Palette.muted)
            if let editing {
                if has { PhoneTextField(label: "Current PIN", text: digits($current), secure: true, keyboard: .numberPad) }
                if editing {
                    PhoneTextField(label: "New PIN (\(PinStore.minLength) or more digits)", text: digits($pin), secure: true, keyboard: .numberPad)
                    PhoneTextField(label: "New PIN again", text: digits($confirm), secure: true, keyboard: .numberPad)
                }
                if let message { Text(message.0).font(.footnote).foregroundStyle(message.1 ? Palette.error : Palette.muted) }
                HStack {
                    Button("Cancel") { close(nil) }.buttonStyle(PhoneSecondaryButtonStyle())
                    Button(editing ? "Save" : "Remove") { save(editing) }.buttonStyle(PhoneButtonStyle())
                }
            } else {
                if let message { Text(message.0).font(.footnote).foregroundStyle(message.1 ? Palette.error : Palette.muted) }
                HStack {
                    Button(has ? "Change PIN" : "Set a PIN") { editing = true; message = nil }.buttonStyle(PhoneButtonStyle())
                    if has { Button("No PIN") { editing = false; message = nil }.buttonStyle(PhoneSecondaryButtonStyle()) }
                }
            }
        }
        .onAppear { has = PinStore.shared.has(address, username) }
    }

    private func digits(_ binding: Binding<String>) -> Binding<String> {
        Binding(get: { binding.wrappedValue }, set: { binding.wrappedValue = $0.filter(\.isNumber) })
    }

    private func close(_ done: String?) {
        editing = nil; current = ""; pin = ""; confirm = ""
        message = done.map { ($0, false) }
        has = PinStore.shared.has(address, username)
    }

    private func save(_ setting: Bool) {
        let store = PinStore.shared
        if has {
            switch store.check(address, username, pin: current) {
            case .right: break
            case .wrong(let until):
                current = ""
                message = (until.map { "Too many wrong tries. Try again in \(PinStore.wait(until: $0))." } ?? "The current PIN isn't right.", true)
                return
            case .locked(let until):
                message = ("Too many wrong tries. Try again in \(PinStore.wait(until: until)).", true)
                return
            }
        }
        if !setting { store.clear(address, username); close("No PIN: anyone using this phone can open \(username)."); return }
        if pin.count < PinStore.minLength { message = ("Use \(PinStore.minLength) or more digits.", true); return }
        if pin != confirm { message = ("The PINs don't match.", true); return }
        store.set(address, username, pin: pin)
        close("PIN saved.")
    }
}
