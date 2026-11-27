import SwiftUI
import MediaAccessibility

/// Draws the selected subtitle track over the video at the player's position (milliseconds, in
/// the file's time), loading cues ahead as playback goes on. Text in the subtitle style (Settings);
/// PGS pictures on the disc's frame, centered on the video at its scale, as Android draws them.
struct SubtitleOverlay: View {
    @Environment(AppModel.self) private var model
    let subtitles: ServerSubtitles
    let position: () -> Int64
    let videoAspect: () -> CGFloat
    let bottomPadding: CGFloat

    @State private var text: AttributedString?
    @State private var picture: PgsPicture?
    @State private var shownSet: Int64?

    var body: some View {
        let style = SubtitleStyle.current(model)
        ZStack {
            if let picture { PictureSubtitle(picture: picture, videoAspect: videoAspect()) }
            if let text {
                VStack {
                    Spacer()
                    StyledSubtitleText(text: text, style: style)
                        .padding(.horizontal, 48)
                        .padding(.bottom, bottomPadding)
                }
            }
        }
        .allowsHitTesting(false)
        .task(id: subtitles.selected) {
            while !Task.isCancelled {
                await subtitles.keepAhead(position())
                try? await Task.sleep(for: .seconds(1))
            }
        }
        .task(id: subtitles.selected) {
            text = nil
            picture = nil
            shownSet = nil
            while !Task.isCancelled {
                let now = position()
                // Only a change of line redraws: a redraw every 100 ms closed the player's open menus.
                let cue = subtitles.cueAt(now)
                if cue != text { text = cue }
                let set = subtitles.pictureAt(now)
                if set?.startMs != shownSet {
                    shownSet = set?.startMs
                    picture = await Task.detached { set?.render() }.value
                }
                try? await Task.sleep(for: .milliseconds(100))
            }
        }
    }
}

/// The look of text subtitles on this phone (Settings › App › Subtitle style), as Android's: the
/// iPhone's own caption style (Settings › Accessibility › Subtitles & Captioning, read through
/// MediaAccessibility), or our own box or outline, colors, font and size.
struct SubtitleStyle: Equatable {
    var system = true
    var background = "Black"      // Black, White, the colors, Themed, None
    var opacity = 0.5             // the box's (or outline's)
    var foreground = "White"      // White, Black, the colors, Themed, None (see-through letters)
    var textOpacity = 1.0
    var outline = false           // the background color outlines the text instead of a box
    var font = "Default"          // Default, Serif, Monospace, Casual, Condensed
    var size = "Normal"           // Tiny, Small, Normal, Large, Huge

    /// The fixed colors, the same on Android and the web (Material's 500s, and maroon).
    static let colors: [String: Color] = [
        "Black": .black, "White": .white, "Yellow": Color(hex: 0xFFEB3B), "Green": Color(hex: 0x4CAF50),
        "Red": Color(hex: 0xF44336), "Blue": Color(hex: 0x2196F3), "Gray": Color(hex: 0x9E9E9E),
        "Purple": Color(hex: 0x9C27B0), "Orange": Color(hex: 0xFF9800), "Pink": Color(hex: 0xE91E63),
        "Maroon": Color(hex: 0x800000), "Teal": Color(hex: 0x009688), "Light green": Color(hex: 0x8BC34A),
    ]
    static let shades = ["Yellow", "Green", "Red", "Blue", "Gray", "Purple", "Orange", "Pink", "Maroon", "Teal",
                         "Light green"]
    static let backgrounds = ["Black", "White"] + shades + ["Themed", "None"]
    static let foregrounds = ["White", "Black"] + shades + ["Themed", "None"]
    static let fonts = ["Default", "Serif", "Monospace", "Casual", "Condensed"]
    static let sizes = ["Tiny", "Small", "Normal", "Large", "Huge"]
    static let opacities = [0.25, 0.5, 0.75, 1.0]

    static func opacityLabel(_ o: Double) -> String { o >= 1 ? "Solid" : "\(Int(o * 100))%" }

    var scale: CGFloat {
        if system { return CGFloat(MACaptionAppearanceGetRelativeCharacterSize(.user, nil)) }
        return ["Tiny": 0.6, "Small": 0.8, "Large": 1.3, "Huge": 1.6][size] ?? 1
    }

    /// The letters' color; nil when they're see-through.
    var textColor: Color? {
        if system {
            let color = Color(MACaptionAppearanceCopyForegroundColor(.user, nil).takeRetainedValue())
            return color.opacity(Double(MACaptionAppearanceGetForegroundOpacity(.user, nil)))
        }
        switch foreground {
        case "Themed": return Palette.accent.opacity(textOpacity)
        case "None": return nil
        default: return (Self.colors[foreground] ?? .white).opacity(textOpacity)
        }
    }

    /// The box's (or outline's) color; nil for none.
    var boxColor: Color? {
        if system {
            let alpha = Double(MACaptionAppearanceGetBackgroundOpacity(.user, nil))
            guard alpha > 0 else { return nil }
            return Color(MACaptionAppearanceCopyBackgroundColor(.user, nil).takeRetainedValue()).opacity(alpha)
        }
        switch background {
        case "Themed": return Palette.panel.opacity(opacity)
        case "None": return nil
        default: return (Self.colors[background] ?? .black).opacity(opacity)
        }
    }

    var outlined: Bool { !system && outline && boxColor != nil }

    /// The outline: the background color, solid (Translucency is the box's; the outline goes
    /// with the text's opacity, applied to it as a whole).
    var outlineColor: Color {
        switch background {
        case "Themed": return Color(hex: ThemeStore.shared.palette.panel)
        default: return Self.colors[background] ?? .black
        }
    }

    func font(_ points: CGFloat) -> Font {
        let size = points * scale
        if system { return .system(size: size, weight: .medium) }
        switch font {
        case "Serif": return .system(size: size, weight: .medium, design: .serif)
        case "Monospace": return .system(size: size, weight: .medium, design: .monospaced)
        case "Casual": return .system(size: size, weight: .medium, design: .rounded)
        case "Condensed": return .system(size: size, weight: .medium).width(.condensed)
        default: return .system(size: size, weight: .medium)
        }
    }

    /// One line for Settings: "iPhone system", or "Custom · Large · Serif".
    var summary: String { system ? "iPhone system (Accessibility › Subtitles & Captioning)" : "Custom · \(size) · \(font)" }

    @MainActor static func current(_ model: AppModel) -> SubtitleStyle {
        var s = SubtitleStyle()
        if let v = model.setting("subtitle_system") { s.system = v != "false" }
        if let v = model.setting("subtitle_background") { s.background = v }
        if let v = model.setting("subtitle_opacity").flatMap(Double.init) { s.opacity = v }
        if let v = model.setting("subtitle_foreground") { s.foreground = v }
        if let v = model.setting("subtitle_text_opacity").flatMap(Double.init) { s.textOpacity = v }
        if let v = model.setting("subtitle_outline") { s.outline = v == "true" }
        if let v = model.setting("subtitle_font") { s.font = v }
        if let v = model.setting("subtitle_size") { s.size = v }
        return s
    }

    @MainActor func save(_ model: AppModel) {
        model.setSetting("subtitle_system", system ? "true" : "false")
        model.setSetting("subtitle_background", background)
        model.setSetting("subtitle_opacity", String(opacity))
        model.setSetting("subtitle_foreground", foreground)
        model.setSetting("subtitle_text_opacity", String(textOpacity))
        model.setSetting("subtitle_outline", outline ? "true" : "false")
        model.setSetting("subtitle_font", font)
        model.setSetting("subtitle_size", size)
    }
}

/// A subtitle line in the style: 18 points at Normal, centered. In a box, or outlined in the
/// box's color; see-through letters are cut out of the box or outline; with neither box nor
/// outline, a shadow keeps it readable.
struct StyledSubtitleText: View {
    let text: AttributedString
    let style: SubtitleStyle
    var points: CGFloat = 18

    var body: some View {
        let box = style.boxColor
        let letters = Text(text).font(style.font(points)).multilineTextAlignment(.center)
        ZStack {
            if style.outlined {
                // The outline: the text drawn around itself in the background color.
                let w = max(1.5, points * style.scale / 10)
                ZStack {
                    ForEach(0..<8, id: \.self) { i in
                        let angle = Double(i) * .pi / 4
                        letters.foregroundStyle(style.outlineColor).offset(x: cos(angle) * w, y: sin(angle) * w)
                    }
                }
                .opacity(style.textOpacity) // once for the whole outline, so overlaps don't darken it
            }
            if let color = style.textColor {
                letters.foregroundStyle(color)
                    .shadow(color: box == nil ? .black : .clear, radius: 3)
            } else {
                letters.foregroundStyle(.black).blendMode(.destinationOut) // see-through
            }
        }
        .padding(.horizontal, 10).padding(.vertical, 4)
        .background(style.outlined ? .clear : (box ?? .clear), in: RoundedRectangle(cornerRadius: 6))
        .compositingGroup()
    }
}

/// Settings › App › Subtitle style: a preview above the options, each a menu of its values;
/// changing any of the Custom options makes the style Custom.
struct SubtitleStyleDialog: View {
    @Environment(AppModel.self) private var model
    let onDismiss: () -> Void
    @State private var style = SubtitleStyle()

    var body: some View {
        PhoneDialog(title: "Subtitle style", onDismiss: onDismiss) {
            ScrollView {
                VStack(alignment: .leading, spacing: 4) {
                    ZStack {
                        LinearGradient(colors: [.blue, .orange, .green], startPoint: .topLeading, endPoint: .bottomTrailing)
                        StyledSubtitleText(text: AttributedString("The quick brown fox\njumps over the lazy dog"), style: style, points: 16)
                    }
                    .frame(height: 110)
                    .clipShape(RoundedRectangle(cornerRadius: 8))
                    Text("Text subtitles only: picture ones (Blu-ray, DVD) keep their own look." +
                         (style.system ? " The iPhone's style is set in Settings › Accessibility › Subtitles & Captioning." : ""))
                        .font(.caption).foregroundStyle(Palette.muted)
                        .padding(.vertical, 6)
                    row("Style", style.system ? "iPhone system" : "Custom", applies: true,
                        [("iPhone system", { $0.system = true }), ("Custom", { $0.system = false })])
                    row("Background", style.background, applies: true,
                        SubtitleStyle.backgrounds.map { v in (v, { $0.background = v }) })
                    row("Translucency", SubtitleStyle.opacityLabel(style.opacity), applies: style.background != "None" && !style.outline,
                        SubtitleStyle.opacities.map { v in (SubtitleStyle.opacityLabel(v), { $0.opacity = v }) })
                    row("Text", style.foreground, applies: true,
                        SubtitleStyle.foregrounds.map { v in (v, { $0.foreground = v }) })
                    row("Text opacity", SubtitleStyle.opacityLabel(style.textOpacity), applies: true,
                        SubtitleStyle.opacities.map { v in (SubtitleStyle.opacityLabel(v), { $0.textOpacity = v }) })
                    row("Outline", style.outline ? "On" : "Off", applies: style.background != "None",
                        [("Off", { $0.outline = false }), ("On", { $0.outline = true })])
                    row("Font", style.font, applies: true, SubtitleStyle.fonts.map { v in (v, { $0.font = v }) })
                    row("Size", style.size, applies: true, SubtitleStyle.sizes.map { v in (v, { $0.size = v }) })
                }
            }
            .frame(maxHeight: 460)
        } buttons: {
            Button("Done", action: onDismiss).buttonStyle(PhoneButtonStyle())
        }
        .onAppear { style = SubtitleStyle.current(model) }
    }

    /// A row: its label, its value as a menu of the choices. Dimmed where it doesn't apply.
    private func row(_ label: String, _ value: String, applies: Bool,
                     _ choices: [(String, (inout SubtitleStyle) -> Void)]) -> some View {
        let isStyleRow = label == "Style"
        return Menu {
            ForEach(choices.indices, id: \.self) { i in
                Button(choices[i].0) {
                    var s = style
                    if !isStyleRow { s.system = false }
                    choices[i].1(&s)
                    style = s
                    s.save(model)
                }
            }
        } label: {
            HStack {
                Text(label).foregroundStyle(Palette.text)
                Spacer()
                Text(value).foregroundStyle(Palette.muted)
            }
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .opacity(!applies || (style.system && !isStyleRow) ? 0.5 : 1)
    }
}

/// A PGS picture. It's drawn for the disc's frame (1920×1080): the frame is centered on the video
/// at the video's scale, as it was on the disc, and a line that would fall below the picture is
/// moved up.
struct PictureSubtitle: View {
    let picture: PgsPicture
    let videoAspect: CGFloat

    var body: some View {
        GeometryReader { geo in
            let frameW = CGFloat(picture.frameWidth), frameH = CGFloat(picture.frameHeight)
            let aspect = videoAspect > 0 ? videoAspect : frameW / frameH
            let shownW = min(geo.size.width, geo.size.height * aspect)
            let shownH = shownW / aspect
            let scale = max(shownW / frameW, shownH / frameH)
            let left = geo.size.width / 2 - frameW * scale / 2
            let top = geo.size.height / 2 - frameH * scale / 2
            let w = CGFloat(picture.image.width) * scale, h = CGFloat(picture.image.height) * scale
            let bottom = top + (CGFloat(picture.y) * scale) + h
            let above = max(0, bottom - (geo.size.height + shownH) / 2)
            Image(decorative: picture.image, scale: 1)
                .resizable()
                .frame(width: w, height: h)
                .position(x: left + CGFloat(picture.x) * scale + w / 2, y: top + CGFloat(picture.y) * scale - above + h / 2)
        }
        .ignoresSafeArea()
    }
}

/// Subtitle timing: "Already said" when the line was spoken before its subtitle showed (moves
/// them earlier), "Not said yet" when the subtitle came first (later), by the step chosen.
struct SubtitleTimingPanel: View {
    let offsetMs: Int
    let onShift: (Int) -> Void
    let onClose: () -> Void

    @State private var step = 250
    private let steps = [50, 250, 500, 1000]

    var body: some View {
        VStack(spacing: 8) {
            HStack {
                Text("Subtitle timing: \(subtitleOffsetText(offsetMs))").foregroundStyle(.white)
                PlayerIcon(system: "xmark", label: "Close", action: onClose)
            }
            HStack(spacing: 8) {
                Button("Already said") { onShift(-step) }.buttonStyle(PhoneButtonStyle())
                Button("Not said yet") { onShift(step) }.buttonStyle(PhoneButtonStyle())
            }
            HStack(spacing: 6) {
                ForEach(steps, id: \.self) { ms in
                    Button { step = ms } label: {
                        Text(ms >= 1000 ? "\(ms / 1000) s" : (ms == 500 ? "0.5 s" : "\(ms) ms"))
                            .font(.caption.weight(.medium))
                            .padding(.horizontal, 10).padding(.vertical, 6)
                            .foregroundStyle(ms == step ? Palette.onAccent : .white)
                            .background(ms == step ? Palette.accent : .clear, in: RoundedRectangle(cornerRadius: 5))
                            .overlay(RoundedRectangle(cornerRadius: 5).stroke(ms == step ? .clear : Palette.muted, lineWidth: 1))
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(12)
        .background(.black.opacity(0.85), in: RoundedRectangle(cornerRadius: 5))
        .overlay(RoundedRectangle(cornerRadius: 5).stroke(Palette.muted, lineWidth: 1))
    }
}

/// "Subtitles online": OpenSubtitles results for the video, in this phone's languages; picking
/// one downloads it (through the server) and turns it on.
struct SubtitleSearchDialog: View {
    let subtitles: ServerSubtitles
    let onDismiss: () -> Void
    let onChosen: (Int) -> Void

    @State private var results: [SubtitleHit] = []
    @State private var note: String? = "Searching…"
    @State private var busy = true

    var body: some View {
        PhoneDialog(title: "Subtitles online", onDismiss: onDismiss) {
            VStack(alignment: .leading, spacing: 8) {
                if let note { Text(note).font(.footnote).foregroundStyle(Palette.muted) }
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 0) {
                        ForEach(results, id: \.fileId) { hit in
                            Button { choose(hit) } label: {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(hit.release ?? "").font(.subheadline).lineLimit(2)
                                    Text(details(hit)).font(.caption).foregroundStyle(Palette.muted)
                                }
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(.vertical, 10)
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .disabled(busy)
                        }
                    }
                }
                .frame(maxHeight: 420)
            }
        } buttons: {
            Button("Close", action: onDismiss).buttonStyle(PhoneSecondaryButtonStyle())
        }
        .task {
            do {
                results = try await subtitles.search()
                note = results.isEmpty ? "Nothing found in \(deviceSubtitleLanguages().joined(separator: ", "))." : "\(results.count) found."
            } catch {
                note = error.localizedDescription
            }
            busy = false
        }
    }

    private func choose(_ hit: SubtitleHit) {
        busy = true
        note = hit.downloaded == true ? "Loading…" : "Downloading…"
        Task {
            do {
                onChosen(try await subtitles.download(hit))
            } catch {
                note = error.localizedDescription
                busy = false
            }
        }
    }

    private func details(_ hit: SubtitleHit) -> String {
        var parts: [String] = [languageName(hit.languageCode) ?? hit.language ?? ""]
        parts.append("\((hit.downloads ?? 0).formatted()) downloads")
        if hit.hashMatch == true { parts.append("made for this file") }
        if hit.trusted == true { parts.append("trusted") }
        if hit.hearingImpaired == true { parts.append("SDH") }
        if hit.foreignPartsOnly == true { parts.append("foreign parts only") }
        if hit.machineTranslated == true { parts.append("machine translated") }
        if hit.downloaded == true { parts.append("already downloaded") }
        return parts.filter { !$0.isEmpty }.joined(separator: " · ")
    }
}
