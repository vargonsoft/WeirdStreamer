import SwiftUI

// The themes, as the Android app's ThemePalette and AppTheme (Color.kt, Theme.kt): Ember (the
// default: the logo's aubergine, gold and cream) and 20 others, each a palette that every screen
// draws with. The theme in use is observable, so picking one redraws the app at once.

struct ThemePalette {
    var isLight = false
    let background: UInt32
    let panel: UInt32          // the boxes the header and content sit in
    let raised: UInt32         // placeholders and chips inside a box
    let button: UInt32
    let buttonLabel: UInt32
    let text: UInt32
    let muted: UInt32
    let accent: UInt32         // headings, focus, progress
    let onAccent: UInt32
    let secondary: UInt32
    var panelAlpha: Double = 1 // below 1: boxes let the background image show through
    var monospace = false      // the terminal themes
    var backgroundImage: String? = nil
}

enum AppTheme: String, CaseIterable {
    case `default` = "DEFAULT", light = "LIGHT", dark = "DARK", auto = "AUTO", cinemaBlue = "CINEMA_BLUE",
         cinemaRed = "CINEMA_RED", teal = "TEAL", orange = "ORANGE", brown = "BROWN", purple = "PURPLE", gray = "GRAY",
         yellow = "YELLOW", green = "GREEN", harborLights = "HARBOR_LIGHTS", mossAndRose = "MOSS_AND_ROSE",
         desertBloom = "DESERT_BLOOM", arcticBerry = "ARCTIC_BERRY", copperSea = "COPPER_SEA", neonArcade = "NEON_ARCADE",
         terminalGreen = "TERMINAL_GREEN", terminalAmber = "TERMINAL_AMBER"

    var label: String {
        switch self {
        case .default: return "Default"
        case .auto: return "Auto Light/Dark"
        case .cinemaBlue: return "Cinema Blue"
        case .cinemaRed: return "Cinema Red"
        case .harborLights: return "Harbor Lights"
        case .mossAndRose: return "Moss and Rose"
        case .desertBloom: return "Desert Bloom"
        case .arcticBerry: return "Arctic Berry"
        case .copperSea: return "Copper Sea"
        case .neonArcade: return "Neon Arcade"
        case .terminalGreen: return "Terminal Green"
        case .terminalAmber: return "Terminal Amber"
        default: return rawValue.prefix(1) + rawValue.dropFirst().lowercased()
        }
    }

    var palette: ThemePalette {
        switch self {
        case .default, .auto: return .init(background: 0x1C0F1B, panel: 0x321B2F, raised: 0x4A2A45, button: 0x6A3A62, buttonLabel: 0xFFF2D8, text: 0xFFF2D8, muted: 0xD8C6B4, accent: 0xF6C445, onAccent: 0x321B2F, secondary: 0xE0892C)
        case .dark: return .init(background: 0x141218, panel: 0x211F26, raised: 0x2B2930, button: 0x4A4458, buttonLabel: 0xE6E1E5, text: 0xE6E1E5, muted: 0xCAC4D0, accent: 0xD0BCFF, onAccent: 0x381E72, secondary: 0xCCC2DC)
        case .light: return .init(isLight: true, background: 0xF3EFF7, panel: 0xFFFFFF, raised: 0xECE6F0, button: 0x6750A4, buttonLabel: 0xFFFFFF, text: 0x1D1B20, muted: 0x49454F, accent: 0x6750A4, onAccent: 0xFFFFFF, secondary: 0x625B71)
        case .cinemaBlue: return .init(background: 0x02013D, panel: 0x0D1150, raised: 0x182066, button: 0x26307F, buttonLabel: 0xE6E9F7, text: 0xE6E9F7, muted: 0xA8AED6, accent: 0xF2B83A, onAccent: 0x02013D, secondary: 0x7C8FE0, panelAlpha: 0.75, backgroundImage: "FilmReel")
        case .cinemaRed: return .init(background: 0x2B0B10, panel: 0x43141B, raised: 0x561B24, button: 0x72252F, buttonLabel: 0xF6E7E4, text: 0xF6E7E4, muted: 0xD4AFAA, accent: 0xF2B83A, onAccent: 0x2B0B10, secondary: 0xE0896F)
        case .teal: return .init(background: 0x0B1E1E, panel: 0x123130, raised: 0x1B4443, button: 0x1F5F5B, buttonLabel: 0xE4F5F3, text: 0xE4F5F3, muted: 0xA6CDC9, accent: 0x5EEAD4, onAccent: 0x0B2423, secondary: 0xF2A65A)
        case .orange: return .init(background: 0x1D110A, panel: 0x331E10, raised: 0x4A2D18, button: 0x8A4A1B, buttonLabel: 0xFFF0E0, text: 0xFFF0E0, muted: 0xE2C2A2, accent: 0xFFA24A, onAccent: 0x2B1607, secondary: 0xF6C445)
        case .brown: return .init(background: 0x18120D, panel: 0x2D2219, raised: 0x423226, button: 0x6B4D36, buttonLabel: 0xF5EADC, text: 0xF5EADC, muted: 0xCDB9A2, accent: 0xE3B676, onAccent: 0x2D2219, secondary: 0xC77C4A)
        case .purple: return .init(background: 0x140E22, panel: 0x23193B, raised: 0x322556, button: 0x4F3C8A, buttonLabel: 0xEEE8FF, text: 0xEEE8FF, muted: 0xBCB1DD, accent: 0xC3A6FF, onAccent: 0x1E1436, secondary: 0xF29FC5)
        case .gray: return .init(background: 0x151617, panel: 0x232527, raised: 0x313437, button: 0x4A4E53, buttonLabel: 0xEFF1F3, text: 0xEFF1F3, muted: 0xB3B8BD, accent: 0xDDE2E7, onAccent: 0x232527, secondary: 0x8FB8E8)
        case .yellow: return .init(background: 0x181606, panel: 0x2B2809, raised: 0x3F3A10, button: 0x6A6015, buttonLabel: 0xFFF9DA, text: 0xFFF9DA, muted: 0xDAD19E, accent: 0xFFE04D, onAccent: 0x2B2809, secondary: 0xF29B38)
        case .green: return .init(background: 0x0D190F, panel: 0x162B19, raised: 0x213F26, button: 0x2E6A37, buttonLabel: 0xE8F6E9, text: 0xE8F6E9, muted: 0xB0D2B3, accent: 0x93E27D, onAccent: 0x12261A, secondary: 0xE8C55A)
        case .harborLights: return .init(background: 0x0C1626, panel: 0x14243D, raised: 0x322A39, button: 0x8C3B2E, buttonLabel: 0xF3EAD8, text: 0xF3EAD8, muted: 0xBBB8B1, accent: 0xF2B544, onAccent: 0x14243D, secondary: 0xB0786A)
        case .mossAndRose: return .init(background: 0x0F1F18, panel: 0x1D372B, raised: 0x393A36, button: 0x8E4458, buttonLabel: 0xF4EEDF, text: 0xF4EEDF, muted: 0xBEC0B2, accent: 0xF1D49B, onAccent: 0x1D372B, secondary: 0xB28087)
        case .desertBloom: return .init(background: 0x1E0F17, panel: 0x38202B, raised: 0x542C2B, button: 0xA84E2A, buttonLabel: 0xFBEDE3, text: 0xFBEDE3, muted: 0xCABAB5, accent: 0x4FD1C5, onAccent: 0x1E0F17, secondary: 0xC5866B)
        case .arcticBerry: return .init(background: 0x10172A, panel: 0x1F2A44, raised: 0x402B4A, button: 0xA32E5B, buttonLabel: 0xEEF2FA, text: 0xEEF2FA, muted: 0xBAC0CC, accent: 0x9BE7D8, onAccent: 0x10172A, secondary: 0xBD7393)
        case .copperSea: return .init(background: 0x082221, panel: 0x0F3B3A, raised: 0x324238, button: 0x9C5530, buttonLabel: 0xF1EBDD, text: 0xF1EBDD, muted: 0xB8BFB4, accent: 0xEAD7A1, onAccent: 0x0F3B3A, secondary: 0xBA8A6D)
        case .neonArcade: return .init(background: 0x111116, panel: 0x1D1D26, raised: 0x2C234D, button: 0x5B36C2, buttonLabel: 0xF1EEFF, text: 0xF1EEFF, muted: 0xBCBAC9, accent: 0xB8F04A, onAccent: 0x16161E, secondary: 0x9076D7)
        case .terminalGreen: return .init(background: 0x020802, panel: 0x061206, raised: 0x0B200C, button: 0x0F3A14, buttonLabel: 0x8CFF9E, text: 0x8CFF9E, muted: 0x4FC963, accent: 0x33FF66, onAccent: 0x021202, secondary: 0xC8FFD2, monospace: true)
        case .terminalAmber: return .init(background: 0x080500, panel: 0x140C00, raised: 0x221500, button: 0x3D2600, buttonLabel: 0xFFC870, text: 0xFFC870, muted: 0xC98A2E, accent: 0xFFB000, onAccent: 0x140C00, secondary: 0xFFE3A8, monospace: true)
        }
    }
}

/// The theme in use (kept in Settings), and the system's dark setting for Auto.
@MainActor
@Observable
final class ThemeStore {
    static let shared = ThemeStore()
    var theme: AppTheme = .default
    var systemDark = true

    var palette: ThemePalette {
        theme == .auto ? (systemDark ? AppTheme.dark.palette : AppTheme.light.palette) : theme.palette
    }
}

/// The theme's colors, read by every view (so a new theme redraws them).
@MainActor
enum Palette {
    private static var p: ThemePalette { ThemeStore.shared.palette }
    static var background: Color { Color(hex: p.background) }
    static var panel: Color { Color(hex: p.panel).opacity(p.panelAlpha) }
    static var raised: Color { Color(hex: p.raised) }
    static var button: Color { Color(hex: p.button) }
    static var buttonLabel: Color { Color(hex: p.buttonLabel) }
    static var text: Color { Color(hex: p.text) }
    static var muted: Color { Color(hex: p.muted) }
    static var accent: Color { Color(hex: p.accent) }
    static var onAccent: Color { Color(hex: p.onAccent) }
    static var error: Color { p.isLight ? Color(hex: 0xB3261E) : Color(hex: 0xFFB4AB) }
    /// Quieter buttons (Cancel, Back): 40% of the way from the box to a button, outlined in the button color.
    static var buttonSecondary: Color { Color(hex: mix(p.panel, p.button, 0.4)) }
    static var isLight: Bool { p.isLight }
    static var monospace: Bool { p.monospace }
    static var backgroundImage: String? { p.backgroundImage }

    private static func mix(_ a: UInt32, _ b: UInt32, _ t: Double) -> UInt32 {
        func channel(_ shift: UInt32) -> UInt32 {
            let x = Double((a >> shift) & 0xFF), y = Double((b >> shift) & 0xFF)
            return UInt32((x + (y - x) * t).rounded()) << shift
        }
        return channel(16) | channel(8) | channel(0)
    }
}

/// A theme's preview tile: its background, a box with the accent and a line of text, and a button.
struct ThemePreview: View {
    let theme: AppTheme

    var body: some View {
        if theme == .auto {
            HStack(spacing: 0) {
                PalettePreview(p: AppTheme.light.palette, half: true)
                PalettePreview(p: AppTheme.dark.palette, half: true)
            }
            .clipShape(RoundedRectangle(cornerRadius: 5))
        } else {
            PalettePreview(p: theme.palette, half: false).clipShape(RoundedRectangle(cornerRadius: 5))
        }
    }
}

private struct PalettePreview: View {
    let p: ThemePalette
    let half: Bool

    var body: some View {
        ZStack {
            Color(hex: p.background)
            ZStack(alignment: .topLeading) {
                Color(hex: p.panel)
                VStack(alignment: .leading, spacing: 5) {
                    GeometryReader { geo in
                        VStack(alignment: .leading, spacing: 5) {
                            RoundedRectangle(cornerRadius: 2).fill(Color(hex: p.accent)).frame(width: geo.size.width * 0.4, height: 4)
                            RoundedRectangle(cornerRadius: 2).fill(Color(hex: p.text).opacity(0.6)).frame(width: geo.size.width * 0.55, height: 4)
                        }
                    }
                    .padding(.leading, 7).padding(.top, 8)
                }
                VStack {
                    Spacer()
                    HStack {
                        Spacer()
                        if half {
                            RoundedRectangle(cornerRadius: 3).fill(Color(hex: p.button)).frame(width: 16, height: 10)
                        } else {
                            Text("Aa").font(.system(size: 10)).foregroundStyle(Color(hex: p.buttonLabel))
                                .padding(.horizontal, 6)
                                .background(Color(hex: p.button), in: RoundedRectangle(cornerRadius: 3))
                        }
                    }
                    .padding(5)
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 4))
            .padding(7)
        }
    }
}

extension Color {
    init(hex: UInt32) {
        self.init(red: Double((hex >> 16) & 0xFF) / 255, green: Double((hex >> 8) & 0xFF) / 255,
                  blue: Double(hex & 0xFF) / 255)
    }
}

/// Android's Panel: a box in the panel color with 12-point corners.
struct Panel<Content: View>: View {
    var padding: CGFloat = 0
    @ViewBuilder let content: Content

    var body: some View {
        content
            .padding(padding)
            .background(Palette.panel, in: RoundedRectangle(cornerRadius: 12))
            .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

/// Buttons: square-ish corners (5 points), as on the web and Android.
struct PhoneButtonStyle: ButtonStyle {
    var destructive = false
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, 16).padding(.vertical, 10)
            .foregroundStyle(destructive ? Color(hex: 0x690005) : Palette.buttonLabel)
            .background(destructive ? Palette.error : Palette.button, in: RoundedRectangle(cornerRadius: 5))
            .opacity(configuration.isPressed ? 0.75 : 1)
    }
}

/// A quieter button (Cancel, Back, Close): the secondary fill, outlined in the button color.
struct PhoneSecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, 16).padding(.vertical, 10)
            .foregroundStyle(Palette.buttonLabel)
            .background(Palette.buttonSecondary, in: RoundedRectangle(cornerRadius: 5))
            .overlay(RoundedRectangle(cornerRadius: 5).stroke(Palette.button, lineWidth: 1))
            .opacity(configuration.isPressed ? 0.75 : 1)
    }
}

/// A tonal button (Resume, season watched): the secondary pair, as Material's FilledTonalButton.
struct TonalButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.medium))
            .padding(.horizontal, 12).padding(.vertical, 8)
            .foregroundStyle(Palette.buttonLabel)
            .background(Palette.buttonSecondary, in: RoundedRectangle(cornerRadius: 5))
            .opacity(configuration.isPressed ? 0.75 : 1)
    }
}

/// The wordmark, as Android's Wordmark.
struct Wordmark: View {
    var height: CGFloat
    var body: some View {
        Image("Wordmark").resizable().scaledToFit().frame(height: height)
    }
}

/// A progress line along the bottom of a thumbnail or poster, and a check in its corner once
/// watched (Android's WatchProgressOverlay).
struct WatchProgressOverlay: View {
    let fraction: Double
    let watched: Bool
    var barHeight: CGFloat = 4
    var badgeSize: CGFloat = 22

    var body: some View {
        ZStack(alignment: .bottomTrailing) {
            if fraction > 0 {
                GeometryReader { geo in
                    VStack {
                        Spacer()
                        ZStack(alignment: .leading) {
                            Rectangle().fill(.black.opacity(0.55))
                            Rectangle().fill(Palette.accent).frame(width: geo.size.width * min(max(fraction, 0), 1))
                        }
                        .frame(height: barHeight)
                    }
                }
            }
            if watched {
                Image(systemName: "checkmark")
                    .font(.system(size: badgeSize * 0.5, weight: .bold))
                    .foregroundStyle(Palette.onAccent)
                    .frame(width: badgeSize, height: badgeSize)
                    .background(Palette.accent, in: Circle())
                    .padding(.trailing, 6)
                    .padding(.bottom, barHeight + 6)
            }
        }
        .allowsHitTesting(false)
    }
}

/// A server image (poster, still, thumbnail), loaded with the account's token. Shows nothing
/// until it arrives, so the placeholder under it stays visible.
struct RemoteImage: View {
    @Environment(AppModel.self) private var model
    let relative: String?
    var opacity: Double = 1

    @State private var image: UIImage?

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image).resizable().scaledToFill().opacity(opacity)
            } else {
                Color.clear
            }
        }
        .task(id: relative) {
            image = nil
            guard let client = model.client, let url = client.url(for: relative) else { return }
            if let cached = ImageCache.shared.object(forKey: url as NSURL) { image = cached; return }
            if let data = try? await client.image(at: url), let decoded = UIImage(data: data) {
                ImageCache.shared.setObject(decoded, forKey: url as NSURL)
                image = decoded
            }
        }
    }
}

enum ImageCache {
    static let shared: NSCache<NSURL, UIImage> = {
        let cache = NSCache<NSURL, UIImage>()
        cache.countLimit = 400
        return cache
    }()
}

// MARK: Text, as the Android app writes it

/// "1.2 GB" ("0 B" for nothing), for folder rows.
func formatFileSize(_ size: Int64) -> String {
    guard size > 0 else { return "0 B" }
    let units = ["B", "KB", "MB", "GB", "TB"]
    let group = min(Int(log10(Double(size)) / log10(1024)), units.count - 1)
    return String(format: "%.1f %@", Double(size) / pow(1024, Double(group)), units[group])
}

/// The same, but empty for nothing (library files).
func formatBytes(_ size: Int64?) -> String {
    guard let size, size > 0 else { return "" }
    return formatFileSize(size)
}

/// "1:23:45" or "12:34".
func clockText(_ seconds: Double) -> String {
    let s = max(0, Int(seconds))
    let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
    return h > 0 ? String(format: "%d:%02d:%02d", h, m, sec) : String(format: "%d:%02d", m, sec)
}

/// "23 min left", for a video in progress.
func timeLeftText(_ playback: Playback?, _ duration: Double?) -> String? {
    guard let p = playback else { return nil }
    let total = (p.duration ?? 0) > 0 ? p.duration! : (duration ?? 0)
    guard p.resumeAt > 0, total > 0 else { return nil }
    return "\(max(Int((total - (p.position ?? 0)) / 60), 1)) min left"
}

/// When a video arrived, in days on this phone's calendar: "Added 2 days ago".
func addedText(_ seconds: Int64) -> String {
    let calendar = Calendar.current
    let day = calendar.startOfDay(for: Date(timeIntervalSince1970: TimeInterval(seconds)))
    let days = calendar.dateComponents([.day], from: day, to: calendar.startOfDay(for: Date())).day ?? 0
    func count(_ n: Int, _ unit: String) -> String { "Added \(n) \(unit)\(n == 1 ? "" : "s") ago" }
    switch days {
    case ...0: return "Added today"
    case 1: return "Added yesterday"
    case ..<30: return count(days, "day")
    case ..<365: return count(days / 30, "month")
    default: return count(days / 365, "year")
    }
}

/// A card's last line: when it arrived (Recently Added), "Up next", else the time left.
func continueNote(_ item: ContinueItem) -> String? {
    if let added = item.addedAt { return addedText(added) }
    if item.upNext == true { return "Up next" }
    return timeLeftText(item.playback, item.duration)
}

/// "10 Episodes", or "1 Episode".
func episodeCountText(_ count: Int) -> String { count == 1 ? "1 Episode" : "\(count) Episodes" }

/// Applies the theme in use: its light or dark look (Auto follows the system), monospace text for
/// the terminal themes.
struct Themed: ViewModifier {
    @Environment(\.colorScheme) private var scheme

    func body(content: Content) -> some View {
        let store = ThemeStore.shared
        content
            .fontDesign(Palette.monospace ? .monospaced : .default)
            .foregroundStyle(Palette.text)
            .tint(Palette.accent)
            .preferredColorScheme(store.theme == .auto ? nil : (Palette.isLight ? .light : .dark))
            .onAppear { store.systemDark = scheme == .dark }
            .onChange(of: scheme) { _, now in store.systemDark = now == .dark }
    }
}

extension View {
    func themed() -> some View { modifier(Themed()) }
}

/// The screen behind the boxes: the background color, and the theme's picture (Cinema Blue's film
/// reel), faded, off to the right as on Android.
struct ThemeBackground: View {
    var body: some View {
        ZStack {
            Palette.background
            if let name = Palette.backgroundImage {
                // Sized to the screen (not the picture), so it never widens the layout.
                GeometryReader { geo in
                    Image(name).resizable().scaledToFill()
                        .frame(width: geo.size.width, height: geo.size.height)
                        .offset(x: 125)
                        .clipped()
                        .opacity(0.10)
                }
                .allowsHitTesting(false)
            }
        }
        .ignoresSafeArea()
    }
}
