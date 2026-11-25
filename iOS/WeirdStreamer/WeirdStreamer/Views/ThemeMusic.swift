import AVFoundation
import SwiftUI

// Theme music on title pages (Settings: "Play theme music on title pages", this device's, on by
// default, at Low, Medium or High), as Android's ThemeMusic.kt: a show's or movie's theme (GET
// /api/library/theme: its folder's theme file, or cut from an intro or the credits) plays once,
// quietly, while its page is open, and fades out when the page closes or a video plays. It's
// fetched through the app's client (which trusts the saved server's certificate), then played
// from memory. It mixes with other audio and keeps to the silent switch.

/// The theme's volume for the setting's level (of the device's volume).
func themeVolume(_ level: String) -> Float {
    switch level {
    case "low": return 0.15
    case "high": return 0.5
    default: return 0.3
    }
}

/// The setting's levels, as (level, label).
let themeVolumes = [("low", "Low"), ("medium", "Medium"), ("high", "High")]

@MainActor
final class ThemeMusicPlayer {
    static let shared = ThemeMusicPlayer()
    private var player: AVAudioPlayer?
    private var playingKey: String?

    /// Plays this title's theme, unless it's the one already playing.
    func play(client: ServerClient, lib: String, kind: String, id: Int, volume: Float) async {
        let key = "\(lib)/\(kind)/\(id)"
        guard playingKey != key else { return }
        stop(fade: 0)
        playingKey = key
        guard let data = try? await client.data("/api/library/theme", query: [("lib", lib), ("kind", kind), ("id", String(id))]),
              playingKey == key, let audio = try? AVAudioPlayer(data: data) else { return }
        // Only when no video plays (its session is .playback): mix with what's there
        if AVAudioSession.sharedInstance().category != .playback {
            try? AVAudioSession.sharedInstance().setCategory(.ambient)
        }
        audio.volume = volume
        audio.play()
        player = audio
    }

    /// A new volume for the theme playing.
    func setVolume(_ volume: Float) { player?.volume = volume }

    /// Fades the theme out over `fade` seconds (0 stops it at once).
    func stop(fade: TimeInterval = 1.5) {
        playingKey = nil
        guard let audio = player else { return }
        player = nil
        guard fade > 0, audio.isPlaying else { audio.stop(); return }
        audio.setVolume(0, fadeDuration: fade)
        DispatchQueue.main.asyncAfter(deadline: .now() + fade) { audio.stop() }
    }
}

extension View {
    /// Plays the title's theme while this page shows (TitleScreen).
    func titleThemeMusic(_ title: TitleDetail, model: AppModel) -> some View {
        let wanted = (title.theme ?? false) && model.themeMusic && model.playing == nil
        return self
            .task(id: "\(title.kind)/\(title.tmdbId)/\(wanted)") {
                guard wanted else { ThemeMusicPlayer.shared.stop(fade: model.playing == nil ? 1.5 : 0.8); return }
                guard let client = model.client, let lib = model.currentLibraryId else { return }
                await ThemeMusicPlayer.shared.play(client: client, lib: lib, kind: title.kind, id: title.tmdbId,
                                                   volume: themeVolume(model.themeVolume))
            }
            .onChange(of: model.themeVolume) { _, level in ThemeMusicPlayer.shared.setVolume(themeVolume(level)) }
            .onDisappear { ThemeMusicPlayer.shared.stop() }
    }
}
