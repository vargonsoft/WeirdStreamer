import SwiftUI
import SwiftData

/// Hands the background download session's events back to it when iOS wakes the app for them.
final class AppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, handleEventsForBackgroundURLSession identifier: String,
                     completionHandler: @escaping () -> Void) {
        Task { @MainActor in DownloadManager.shared.backgroundCompletion = completionHandler }
    }
}

@main
struct WeirdStreamerApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    private let container: ModelContainer
    @State private var model: AppModel

    init() {
        HLSRelay.sweep() // transcode relays a closed (or killed) player left behind
        let container = LocalStore.makeContainer()
        self.container = container
        _model = State(initialValue: AppModel(context: container.mainContext))
    }

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
        }
        .modelContainer(container)
    }
}
