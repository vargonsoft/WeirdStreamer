import SwiftUI

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
                text = subtitles.cueAt(now)
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

/// The subtitle style: the box behind the text, the text's color, size and font (Settings › App ›
/// Subtitle style), as Android's custom style.
struct SubtitleStyle {
    var background = "Black"      // Black, White, Themed, None
    var opacity = 0.5
    var foreground = "White"      // White, Black, Themed
    var size = "Normal"           // Tiny, Small, Normal, Large, Huge
    var font = "Default"          // Default, Serif, Monospace, Rounded

    static let backgrounds = ["Black", "White", "Themed", "None"]
    static let foregrounds = ["White", "Black", "Themed"]
    static let sizes = ["Tiny", "Small", "Normal", "Large", "Huge"]
    static let fonts = ["Default", "Serif", "Monospace", "Rounded"]
    static let opacities = [0.25, 0.5, 0.75, 1.0]

    var scale: CGFloat { ["Tiny": 0.6, "Small": 0.8, "Large": 1.3, "Huge": 1.6][size] ?? 1 }

    var textColor: Color {
        switch foreground {
        case "Black": return .black
        case "Themed": return Palette.accent
        default: return .white
        }
    }

    var boxColor: Color? {
        switch background {
        case "White": return .white.opacity(opacity)
        case "Themed": return Palette.panel.opacity(opacity)
        case "None": return nil
        default: return .black.opacity(opacity)
        }
    }

    var design: Font.Design {
        switch font {
        case "Serif": return .serif
        case "Monospace": return .monospaced
        case "Rounded": return .rounded
        default: return .default
        }
    }

    @MainActor static func current(_ model: AppModel) -> SubtitleStyle {
        var s = SubtitleStyle()
        if let v = model.setting("subtitle_background") { s.background = v }
        if let v = model.setting("subtitle_opacity").flatMap(Double.init) { s.opacity = v }
        if let v = model.setting("subtitle_foreground") { s.foreground = v }
        if let v = model.setting("subtitle_size") { s.size = v }
        if let v = model.setting("subtitle_font") { s.font = v }
        return s
    }

    @MainActor func save(_ model: AppModel) {
        model.setSetting("subtitle_background", background)
        model.setSetting("subtitle_opacity", String(opacity))
        model.setSetting("subtitle_foreground", foreground)
        model.setSetting("subtitle_size", size)
        model.setSetting("subtitle_font", font)
    }

    var summary: String { "\(size) · \(foreground) on \(background == "None" ? "no box" : background.lowercased())" }
}

/// A subtitle line in the style: 18 points at Normal, centered, with a shadow when there's no box.
struct StyledSubtitleText: View {
    let text: AttributedString
    let style: SubtitleStyle

    var body: some View {
        Text(text)
            .font(.system(size: 18 * style.scale, weight: .medium, design: style.design))
            .foregroundStyle(style.textColor)
            .multilineTextAlignment(.center)
            .shadow(color: style.boxColor == nil ? .black : .clear, radius: 3)
            .padding(.horizontal, 10).padding(.vertical, 4)
            .background(style.boxColor ?? .clear, in: RoundedRectangle(cornerRadius: 6))
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
