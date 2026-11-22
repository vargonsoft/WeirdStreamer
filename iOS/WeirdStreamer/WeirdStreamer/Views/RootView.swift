import SwiftUI

/// The app's one screen, as Android's PhoneMediaBrowser: a top bar in its box, then the content
/// in a box (the source list, a folder, a library or a title page), with Settings and the player
/// over it. iOS has no Back button, so a swipe in from the left edge does what Android's Back does.
struct RootView: View {
    @Environment(AppModel.self) private var model
    @State private var showAddSource = false
    @State private var showSettings = false
    @State private var settingsTab = "App"
    @State private var recoveryShown = LocalStore.recoveryNote != nil

    var body: some View {
        ZStack {
            ThemeBackground()
            VStack(spacing: 0) {
                TopBar(showAddSource: $showAddSource, showSettings: $showSettings)
                content
            }
        }
        .themed()
        .gesture(backSwipe)
        .overlay {
            if showAddSource {
                AddSourceDialog { showAddSource = false }
            } else if let request = model.authRequest {
                AuthDialog(request: request)
            } else if model.resumeChoice != nil {
                ResumeDialog()
            } else if model.sourceChoice != nil {
                SourceDialog()
            } else if model.confirmRegenerate {
                PhoneDialog(title: "Regenerate thumbnails?", onDismiss: { model.confirmRegenerate = false }) {
                    EmptyView()
                } buttons: {
                    Button("Cancel") { model.confirmRegenerate = false }.buttonStyle(PhoneSecondaryButtonStyle())
                    Button("Regenerate") {
                        model.confirmRegenerate = false
                        model.regenerateThumbnails()
                    }
                    .buttonStyle(PhoneButtonStyle())
                }
            }
        }
        .fullScreenCover(isPresented: $showSettings) {
            SettingsView(startTab: settingsTab) { showSettings = false; settingsTab = "App" }
                .environment(model)
        }
        .onChange(of: showSettings) { _, open in model.settingsOpen = open }
        .fullScreenCover(item: Binding(get: { model.viewingImage }, set: { model.viewingImage = $0 })) { entry in
            ImageViewer(start: entry).environment(model)
        }
        .fullScreenCover(item: Binding(get: { model.viewingPdf }, set: { model.viewingPdf = $0 })) { entry in
            PdfViewer(entry: entry).environment(model)
        }
        .fullScreenCover(item: Binding(get: { model.settingsOpen ? nil : model.playing }, set: { model.playing = $0 }),
                         onDismiss: { model.playerClosed() }) { request in
            PlayerView(request: request).environment(model)
        }
        .alert("Saved data was reset", isPresented: $recoveryShown) {
            Button("OK") { LocalStore.recoveryNote = nil }
        } message: {
            Text(LocalStore.recoveryNote ?? "")
        }
        .alert("WeirdStreamer", isPresented: Binding(get: { model.message != nil }, set: { if !$0 { model.message = nil } })) {
            Button("OK") { model.message = nil }
        } message: {
            Text(model.message ?? "")
        }
    }

    @ViewBuilder
    private var content: some View {
        if model.isSourceList {
            SourceListView()
                .padding(.top, 4)
        } else {
            ContentBox {
                if model.isLoading && model.entries.isEmpty {
                    ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let error = model.errorMessage {
                    VStack(spacing: 16) {
                        Text("Error: \(error)").foregroundStyle(Palette.error).multilineTextAlignment(.center)
                        HStack(spacing: 12) {
                            Button("Retry") { model.loadPath(model.path, displayName: model.pathDisplayName) }
                                .buttonStyle(PhoneButtonStyle())
                            // Offline: what's downloaded still plays.
                            if !DownloadManager.shared.mine().filter(\.isDone).isEmpty {
                                Button {
                                    settingsTab = "Downloads"
                                    showSettings = true
                                } label: { Label("Downloads", systemImage: "arrow.down.circle") }
                                    .buttonStyle(PhoneSecondaryButtonStyle())
                            }
                        }
                    }
                    .padding()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if model.isLibraryView {
                    if let title = model.openTitle {
                        TitleScreen(title: title)
                    } else {
                        LibraryScreen()
                    }
                } else {
                    FileBrowserScreen()
                }
            }
        }
    }

    /// A swipe in from the left edge: Back (closes a title page, leaves Library view, goes up).
    private var backSwipe: some Gesture {
        DragGesture(minimumDistance: 30)
            .onEnded { drag in
                guard drag.startLocation.x < 28, drag.translation.width > 80,
                      abs(drag.translation.height) < 60, model.canGoBack else { return }
                model.navigateBack()
            }
    }
}

/// The content's box (Android's PhoneContentPanel): 8 points in from the screen's edges.
struct ContentBox<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        Panel { content.frame(maxWidth: .infinity, maxHeight: .infinity) }
            .padding(8)
    }
}

/// The top bar, as Android's: on the source list the wordmark with + and ⋮; on a server, the
/// wordmark and "Server · account" above a box with Home, the folder's title, the view button and ⋮.
struct TopBar: View {
    @Environment(AppModel.self) private var model
    @Binding var showAddSource: Bool
    @Binding var showSettings: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if !model.isSourceList {
                HStack(spacing: 12) {
                    Wordmark(height: 16)
                    Text(model.headerLine).font(.subheadline.weight(.medium)).lineLimit(1)
                }
                .padding(.horizontal, 16)
                .padding(.top, 8)
            }
            Panel {
                HStack(spacing: 4) {
                    if model.isSourceList {
                        Wordmark(height: 22).padding(.leading, 16)
                        Spacer()
                        BarButton(system: "plus", label: "Add Source") { showAddSource = true }
                    } else {
                        BarButton(system: "house.fill", label: "Home") { model.returnToSourceSelection() }
                        Text(model.barTitle).font(.title3.weight(.medium)).lineLimit(1).truncationMode(.tail)
                            .padding(.leading, 4)
                        Spacer(minLength: 4)
                        BarButton(system: model.viewModeIcon, label: "Switch View") { model.toggleViewMode() }
                    }
                    OverflowMenu(showSettings: $showSettings)
                }
                .frame(height: 64)
            }
            .padding(.horizontal, 8)
            .padding(.top, model.isSourceList ? 8 : 4)
        }
    }
}

/// An icon button in the bar (Android's IconButton: a 48-point target).
struct BarButton: View {
    let system: String
    let label: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: system).font(.system(size: 20)).frame(width: 48, height: 48)
        }
        .foregroundStyle(Palette.text)
        .accessibilityLabel(label)
    }
}

/// The ⋮ menu: Show, Download this folder and Regenerate thumbnails (in folders), then Settings.
struct OverflowMenu: View {
    @Environment(AppModel.self) private var model
    @Binding var showSettings: Bool
    @State private var showDownloadFolder = false

    var body: some View {
        Menu {
            if !model.isSourceList && !model.isLibraryView && !model.isServerRoot {
                Section("Show") {
                    ForEach(ContentFilter.allCases, id: \.self) { filter in
                        Button {
                            model.chooseContentFilter(filter)
                        } label: {
                            if model.contentFilter == filter {
                                Label(filter.label, systemImage: "checkmark")
                            } else {
                                Label(filter.label, systemImage: filter.icon)
                            }
                        }
                    }
                }
            }
            // Not among the libraries: that would be every library.
            if !model.isSourceList && !model.isLibraryView && !model.isServerRoot {
                Button { showDownloadFolder = true } label: { Label("Download this folder", systemImage: "arrow.down.circle") }
            }
            if model.canRegenerateThumbnails {
                Button { model.confirmRegenerate = true } label: { Label("Regenerate thumbnails", systemImage: "photo.on.rectangle.angled") }
            }
            Button { showSettings = true } label: { Label("Settings", systemImage: "gearshape") }
        } label: {
            Image(systemName: "ellipsis").rotationEffect(.degrees(90)).font(.system(size: 20)).frame(width: 48, height: 48)
        }
        .foregroundStyle(Palette.text)
        .accessibilityLabel("More")
        .sheet(isPresented: $showDownloadFolder) { DownloadFolderSheet() }
    }
}
