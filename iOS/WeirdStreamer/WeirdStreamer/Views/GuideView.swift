import SwiftUI
import AVFoundation

// The Guide's screen, as Android's PhoneGuide: the channel plays at the top (a tap makes it full
// screen, sideways, where a swipe up or down changes channel and a tap comes back), the program's
// details under it, and the grid below, its half hours scrolling sideways past the channels. A tap
// on a program shows its details, and on one on now tunes to its channel. Live TV doesn't pause:
// the speaker button mutes.

private let channelWidth: CGFloat = 112
private let halfHourWidth: CGFloat = 120
private let rowHeight: CGFloat = 64
private let headerHeight: CGFloat = 28

struct GuideScreen: View {
    @Environment(AppModel.self) private var model
    let library: String
    @State private var guide: GuideModel?

    var body: some View {
        ZStack {
            if let guide {
                if guide.fullscreen {
                    GuideFullScreen(guide: guide)
                } else {
                    ZStack {
                        ThemeBackground()
                        GuideLayout(guide: guide) { model.guideOpen = nil }
                    }
                }
            } else {
                Color.black
            }
        }
        .themed()
        .onAppear {
            guard guide == nil, let client = model.client else { return }
            let g = GuideModel(client: client, library: library.isEmpty ? nil : library, marksWatched: model.guideMarksWatched)
            guide = g
            g.start()
        }
        .onDisappear {
            guide?.stop()
            if guide?.fullscreen == true { setOrientation(.portrait) }
        }
    }
}

/// Upright: the bar, the picture, the details and the grid.
private struct GuideLayout: View {
    @Bindable var guide: GuideModel
    let close: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            Panel {
                HStack(spacing: 4) {
                    BarButton(system: "chevron.left", label: "Back", action: close)
                    Text("Guide").font(.title3.weight(.medium))
                    Spacer()
                }
                .frame(height: 56)
            }
            .padding(.horizontal, 8)
            .padding(.top, 4)
            GuidePicture(guide: guide)
                .aspectRatio(16 / 9, contentMode: .fit)
                .contentShape(Rectangle())
                .onTapGesture { goFullScreen() }
                .overlay(alignment: .bottomTrailing) {
                    HStack(spacing: 8) {
                        MuteButton(guide: guide)
                        RoundButton(system: "arrow.up.left.and.arrow.down.right", label: "Full screen") { goFullScreen() }
                    }
                    .padding(8)
                }
                .padding(.top, 8)
            GuideDetails(guide: guide)
                .frame(maxWidth: .infinity, maxHeight: 170, alignment: .topLeading)
                .padding(12)
            GuideGrid(guide: guide)
        }
    }

    private func goFullScreen() {
        guide.fullscreen = true
        setOrientation(.landscape)
    }
}

/// Full screen: the picture alone, sideways. A tap comes back; a swipe up or down changes channel.
private struct GuideFullScreen: View {
    @Bindable var guide: GuideModel

    var body: some View {
        GuidePicture(guide: guide)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(Color.black)
            .ignoresSafeArea()
            .contentShape(Rectangle())
            .onTapGesture { back() }
            .gesture(DragGesture(minimumDistance: 30).onEnded { drag in
                let dy = drag.translation.height
                guard abs(dy) > abs(drag.translation.width), abs(dy) > 60 else { return }
                guide.surf(dy < 0 ? 1 : -1)
            })
            .overlay(alignment: .bottomTrailing) { MuteButton(guide: guide).padding(16) }
            .statusBarHidden()
            .persistentSystemOverlays(.hidden)
    }

    private func back() {
        guide.fullscreen = false
        setOrientation(.portrait)
    }
}

/// Asks the window to turn: sideways for full screen, upright again after.
@MainActor
private func setOrientation(_ mask: UIInterfaceOrientationMask) {
    guard let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first else { return }
    scene.requestGeometryUpdate(.iOS(interfaceOrientations: mask)) { _ in }
}

private struct MuteButton: View {
    @Bindable var guide: GuideModel
    var body: some View {
        RoundButton(system: guide.muted ? "speaker.slash.fill" : "speaker.wave.2.fill",
                    label: guide.muted ? "Unmute" : "Mute") { guide.muted.toggle() }
    }
}

private struct RoundButton: View {
    let system: String
    let label: String
    let action: () -> Void
    var body: some View {
        Button(action: action) {
            Image(systemName: system).font(.system(size: 17)).foregroundStyle(.white)
                .frame(width: 40, height: 40).background(.black.opacity(0.6), in: Circle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }
}

// MARK: The picture

/// What the channel airs, or the card between programs, with the channel's number and logo for a
/// few seconds after tuning, as a cable box shows them.
private struct GuidePicture: View {
    let guide: GuideModel

    var body: some View {
        // A channel of its own: the Guide channel's picture on top with every channel's listings below;
        // the Weather channel's pages over it all
        let kind = guide.now?.channel.kind ?? guide.data?.channels?.first { $0.number == guide.channel }?.kind
        // a voice-over's squeeze: the credits shrink into the corner beside what's next
        let squeezed = guide.squeezed && kind == nil
        GeometryReader { geo in
            ZStack(alignment: .top) {
                Color.black
                // under the squeezed picture: the background and what's next
                if squeezed { UpNextCard(guide: guide, size: geo.size).transition(.opacity.combined(with: .move(edge: .trailing))) }
                ZStack(alignment: .bottomLeading) {
                    GuideVideoLayer(player: guide.player)
                    if kind == "weather" {
                        EmptyView()
                    } else if kind == "guide" && !guide.playing {
                        GuidePromo(guide: guide, height: geo.size.height)
                    } else if !guide.playing {
                        GuideCard(guide: guide)
                    } else if kind == "guide", let now = guide.now, now.kind == "preview" {
                        PreviewCaption(now: now, height: geo.size.height)
                    }
                }
                .frame(width: squeezed ? geo.size.width * 0.52 : nil,
                       height: kind == "guide" ? geo.size.height * 0.58 : (squeezed ? geo.size.height * 0.52 : geo.size.height))
                .overlay { if squeezed { Rectangle().stroke(.white, lineWidth: 2) } }
                .padding(.leading, squeezed ? geo.size.width * 0.03 : 0)
                .padding(.top, squeezed ? geo.size.height * 0.04 : 0)
                .frame(maxWidth: .infinity, alignment: .leading)
                .animation(.easeInOut(duration: 0.6), value: squeezed)
                if kind == "guide" {
                    GuideListingsRoll(guide: guide, height: geo.size.height)
                        .frame(height: geo.size.height * 0.42)
                        .offset(y: geo.size.height * 0.58)
                }
                if kind == "weather" { WeatherScreen(guide: guide, height: geo.size.height) }
            }
        }
        .overlay(alignment: .topTrailing) {
            if let osd = guide.osd {
                GuideText(osd, size: 28, color: Color(hex: 0x7CFC00), mono: true).padding(12)
            }
        }
        .overlay(alignment: .topLeading) {
            if let c = guide.osdLogo {
                GeometryReader { geo in
                    GuideImage(relative: guide.logoPath(c, width: 512)).frame(width: geo.size.width * 0.22).padding(12)
                }
            }
        }
        .clipped()
    }
}

private struct GuideVideoLayer: UIViewRepresentable {
    let player: AVPlayer
    func makeUIView(context: Context) -> PlayerLayerView {
        let view = PlayerLayerView()
        view.playerLayer.player = player
        view.backgroundColor = .clear
        return view
    }
    func updateUIView(_ view: PlayerLayerView, context: Context) {
        if view.playerLayer.player !== player { view.playerLayer.player = player }
    }
}

/// The card between programs: "We'll be right back" or "Up next", with the channel's logo.
private struct GuideCard: View {
    let guide: GuideModel

    var body: some View {
        let now = guide.now
        GeometryReader { geo in
            let w = geo.size.width
            ZStack {
                RadialGradient(colors: [guideColor(now?.channel.color), Color(hex: 0x0B0B1E)],
                               center: .center, startRadius: 0, endRadius: w * 0.6)
                HStack(spacing: w * 0.04) {
                    if let poster = now?.next?.poster {
                        GuideImage(relative: poster).frame(width: w * 0.22).frame(maxHeight: geo.size.height * 0.8)
                    }
                    VStack(alignment: .leading, spacing: w * 0.01) {
                        if let c = now?.channel {
                            GuideImage(relative: guide.logoPath(c, width: 512)).frame(width: w * 0.28, alignment: .leading)
                        }
                        if now == nil {
                            GuideText("Please stand by", size: w * 0.05)
                        } else if let now, let next = now.next {
                            let long = (now.nextStart ?? 0) - guide.serverNow() > 90_000
                            GuideText(long ? "WE'LL BE RIGHT BACK" : "UP NEXT ON \(now.channel.number) \(now.channel.name.uppercased())",
                                      size: w * 0.032, color: Color(hex: 0xFFD23F))
                            GuideText(next.title, size: w * 0.055)
                            GuideText(([next.episodeLabel] + [now.nextStart.map { "at " + guideTime($0) }]).compactMap { $0 }
                                .joined(separator: " · "), size: w * 0.028)
                        } else {
                            GuideText("Programming will resume shortly", size: w * 0.04)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .padding(w * 0.05)
            }
        }
    }
}

/// A channel's color ("#rrggbb"), else the theme's plum.
func guideColor(_ hex: String?) -> Color {
    guard let hex, let value = UInt32(hex.trimmingCharacters(in: CharacterSet(charactersIn: "#")), radix: 16) else {
        return Color(hex: 0x6A3A62)
    }
    return Color(hex: value)
}

/// Text over the picture: white (or color) with a dark shadow, for any background.
struct GuideText: View {
    let text: String
    let size: CGFloat
    var color: Color = .white
    var mono = false

    init(_ text: String, size: CGFloat, color: Color = .white, mono: Bool = false) {
        self.text = text
        self.size = size
        self.color = color
        self.mono = mono
    }

    var body: some View {
        Text(text).font(.system(size: max(size, 8), weight: .bold, design: mono ? .monospaced : .default))
            .foregroundStyle(color).lineLimit(2)
            .shadow(color: .black.opacity(0.7), radius: 1, x: 1.5, y: 1.5)
    }
}

/// A picture from the server (a logo, a poster), fit to its frame.
struct GuideImage: View {
    @Environment(AppModel.self) private var model
    let relative: String?
    @State private var image: UIImage?

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image).resizable().scaledToFit()
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

// MARK: The details

private struct GuideDetails: View {
    let guide: GuideModel
    @State private var reported: String?

    var body: some View {
        if let picked = guide.selected {
            let c = picked.channel, l = picked.listing
            let info = l.info
            HStack(alignment: .top, spacing: 12) {
                if let poster = info?.poster {
                    GuideImage(relative: poster).frame(width: 72, height: 108).clipShape(RoundedRectangle(cornerRadius: 4))
                }
                ScrollView {
                    VStack(alignment: .leading, spacing: 2) {
                        Text((info?.title ?? "Off the air") + (info?.year.map { " (\($0))" } ?? ""))
                            .font(.headline).foregroundStyle(Palette.accent)
                        if let e = info?.episodeLabel { Text(e).font(.subheadline.weight(.bold)) }
                        Text(guideFacts(c, l)).font(.caption).foregroundStyle(Palette.muted)
                        if guide.voiceOverOn != nil && guide.channel == c.number {
                            Button(reported ?? "Report voice-over") {
                                Task { reported = await guide.reportVoiceOver() ? "Reported" : "Couldn't report it" }
                            }
                            .buttonStyle(PhoneSecondaryButtonStyle())
                            .padding(.vertical, 4)
                        }
                        if let text = info?.episodeOverview.flatMap({ $0.isEmpty ? nil : $0 }) ?? info?.overview {
                            Text(text).font(.caption)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        } else {
            Text(guide.error ?? (guide.data == nil ? "Loading the Guide…" : "")).font(.subheadline)
        }
    }
}

// MARK: The grid

/// The channels down the left, their programs to the right in half hours that scroll sideways
/// together. The half hours along the top follow that scroll, and the time line runs from them
/// down to the last channel, over the programs.
private struct GuideGrid: View {
    let guide: GuideModel
    @State private var scrollX: CGFloat = 0 // the programs' sideways scroll (0 or less)
    @State private var scrolledToTuned = false

    var body: some View {
        if let data = guide.data {
            let channels = data.channels ?? []
            if channels.isEmpty {
                Text("There are no channels yet: the Guide is planned a minute after the server starts.")
                    .font(.subheadline).padding(12).frame(maxHeight: .infinity, alignment: .top)
            } else {
                grid(data, channels)
            }
        } else {
            Spacer()
        }
    }

    private func x(_ ms: Int64, _ data: GuideListings) -> CGFloat {
        halfHourWidth * CGFloat(ms - data.from) / CGFloat(GuideModel.halfHour)
    }

    private func grid(_ data: GuideListings, _ channels: [GuideChannel]) -> some View {
        let width = x(data.to, data)
        let logoColor = Palette.textHex
        return GeometryReader { geo in
            VStack(spacing: 0) {
                // the day and the half hours, moved as the programs scroll
                HStack(spacing: 0) {
                    // the day at the grid's left edge, as it scrolls
                    Text(Date(timeIntervalSince1970: Double(data.from + Int64(-scrollX / halfHourWidth) * GuideModel.halfHour) / 1000)
                        .formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day()))
                        .font(.caption.weight(.medium)).padding(.leading, 8)
                        .frame(width: channelWidth, alignment: .leading)
                    ZStack(alignment: .leading) {
                        ForEach(Array(stride(from: data.from, to: data.to, by: Int(GuideModel.halfHour))), id: \.self) { t in
                            Text(guideTimeOrDay(t)).font(.caption.weight(.medium)).fixedSize()
                                .offset(x: x(t, data) + 6)
                        }
                    }
                    .frame(width: width, alignment: .leading)
                    .offset(x: scrollX)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .clipped()
                }
                .frame(height: headerHeight)
                .background(Palette.panel)
                ScrollViewReader { proxy in
                    ScrollView(.vertical) {
                        HStack(alignment: .top, spacing: 0) {
                            VStack(spacing: 0) {
                                ForEach(channels) { c in channelCell(c, logoColor).id(c.number) }
                            }
                            .frame(width: channelWidth)
                            ScrollView(.horizontal, showsIndicators: false) {
                                VStack(spacing: 0) {
                                    ForEach(channels) { c in programRow(c, data) }
                                }
                                .frame(width: width, alignment: .leading)
                                .onGeometryChange(for: CGFloat.self) { $0.frame(in: .named("guidePrograms")).minX } action: { x in
                                    scrollX = x
                                    // scrolled near the end of what's loaded: the next hours
                                    if -x + geo.size.width - channelWidth > width - 3 * halfHourWidth { guide.extendGrid() }
                                }
                            }
                            .coordinateSpace(name: "guidePrograms")
                        }
                    }
                    .onAppear {
                        guard !scrolledToTuned, let tuned = guide.channel else { return }
                        scrolledToTuned = true
                        proxy.scrollTo(tuned, anchor: .top)
                    }
                }
            }
            // the time line: one line from the half hours down to the last channel, over the programs
            .overlay(alignment: .topLeading) {
                let now = guide.clock > 0 ? guide.clock : guide.serverNow()
                let lineX = channelWidth + x(now, data) + scrollX
                if now >= data.from && now <= data.to && lineX >= channelWidth {
                    Palette.accent.frame(width: 2, height: min(geo.size.height, headerHeight + rowHeight * CGFloat(channels.count)))
                        .offset(x: lineX)
                        .allowsHitTesting(false)
                }
            }
        }
    }

    private func channelCell(_ c: GuideChannel, _ logoColor: String) -> some View {
        let tuned = guide.channel == c.number
        return Button { guide.tune(c.number) } label: {
            VStack(spacing: 2) {
                // the logo takes what the name leaves, so a large text size never pushes the name out
                GuideImage(relative: guide.logoPath(c, width: 256, color: logoColor))
                    .frame(maxWidth: 80, maxHeight: .infinity)
                Text("\(c.number) \(c.name)").font(.caption2).lineLimit(1).foregroundStyle(Palette.text)
            }
            .padding(.horizontal, 4).padding(.vertical, 2)
            .frame(width: channelWidth, height: rowHeight - 4)
            .background(tuned ? Palette.accent.opacity(0.18) : Palette.panel)
        }
        .buttonStyle(.plain)
        .frame(height: rowHeight)
        .accessibilityLabel("Channel \(c.number), \(c.name)")
    }

    private func programRow(_ c: GuideChannel, _ data: GuideListings) -> some View {
        let now = guide.serverNow()
        return ZStack(alignment: .topLeading) {
            ForEach(c.listings ?? [], id: \.start) { l in
                let start = max(l.start, data.from), end = min(l.end, data.to)
                if end > start {
                    let onNow = l.start <= now && now < l.end
                    let picked = guide.selected.map { $0.channel.number == c.number && $0.listing.start == l.start } ?? false
                    Button {
                        guide.selected = (c, l)
                        if onNow && guide.channel != c.number { guide.tune(c.number) }
                    } label: {
                        VStack(alignment: .leading, spacing: 1) {
                            Text(l.info?.title ?? "Off the air").font(.footnote.weight(.bold)).lineLimit(1)
                            if let second = l.info?.episodeLabel ?? (l.info?.kind == "movie" ? l.info?.year.map(String.init) : nil) {
                                Text(second).font(.caption2).lineLimit(1)
                            }
                        }
                        .foregroundStyle(Palette.text)
                        .padding(.horizontal, 6)
                        .frame(width: max(x(end, data) - x(start, data) - 4, 0), height: rowHeight - 4, alignment: .leading)
                        .background(picked ? Palette.accent.opacity(0.35) : onNow ? Palette.raised : Palette.panel,
                                    in: RoundedRectangle(cornerRadius: 4))
                    }
                    .buttonStyle(.plain)
                    .offset(x: x(start, data) + 2, y: 2)
                }
            }
        }
        .frame(width: x(data.to, data), height: rowHeight, alignment: .topLeading)
    }
}
