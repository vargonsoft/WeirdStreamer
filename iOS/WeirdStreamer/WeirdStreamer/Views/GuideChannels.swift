import SwiftUI

// The channels of their own (docs/GUIDE.md), drawn over the Guide's picture, as Android's
// GuideChannels.kt: the Guide channel's promo cards, preview captions and listings roll, and the
// Weather channel's pages. Sizes are hundredths of the picture's height, so they fill full screen
// the same way.

private let gold = Color(hex: 0xFFD23F)
private let lightBlue = Color(hex: 0x9FC3FF)

/// Bold text at a size in hundredths of the picture's height.
private struct Scaled: View {
    let text: String
    let size: CGFloat
    var color: Color = .white
    var weight: Font.Weight = .bold
    var lines = 1
    var mono = false
    var shadow = true

    init(_ text: String, _ size: CGFloat, color: Color = .white, weight: Font.Weight = .bold, lines: Int = 1,
         mono: Bool = false, shadow: Bool = true) {
        self.text = text
        self.size = size
        self.color = color
        self.weight = weight
        self.lines = lines
        self.mono = mono
        self.shadow = shadow
    }

    var body: some View {
        Text(text).font(.system(size: max(size, 6), weight: weight, design: mono ? .monospaced : .default))
            .foregroundStyle(color).lineLimit(lines)
            .shadow(color: shadow ? .black.opacity(0.6) : .clear, radius: 0.5, x: 1, y: 1)
    }
}

/// When a program airs: "Tonight at 8:00 PM", "Tomorrow at 9:00 AM", "Thursday at 7:00 PM".
func airsWhen(_ ms: Int64, now: Int64) -> String {
    let cal = Calendar.current
    let at = Date(timeIntervalSince1970: Double(ms) / 1000)
    let today = Date(timeIntervalSince1970: Double(now) / 1000)
    let days = cal.dateComponents([.day], from: cal.startOfDay(for: today), to: cal.startOfDay(for: at)).day ?? 0
    let time = guideTime(ms)
    switch days {
    case 0: return (cal.component(.hour, from: at) >= 17 ? "Tonight" : "Today") + " at " + time
    case 1: return "Tomorrow at " + time
    default: return at.formatted(.dateTime.weekday(.wide)) + " at " + time
    }
}

/// The Guide channel's card for a program ahead, with music.
struct GuidePromo: View {
    let guide: GuideModel
    let height: CGFloat

    var body: some View {
        let now = guide.now
        let p = now?.program.flatMap { $0.kind == "channel" ? nil : $0 }
        let on = now?.airsOn
        let h = height / 100
        ZStack {
            RadialGradient(colors: [guideColor((on ?? now?.channel)?.color), Color(hex: 0x0B0B1E)], center: .center,
                           startRadius: 0, endRadius: height)
            HStack(spacing: h * 6) {
                if let poster = p?.poster {
                    GuideImage(relative: poster).frame(maxHeight: h * 50).border(.white, width: 2)
                }
                VStack(alignment: .leading, spacing: h * 1.5) {
                    if let c = on ?? now?.channel {
                        GuideImage(relative: guide.logoPath(c, width: 512)).frame(height: h * 12, alignment: .leading)
                    }
                    if let p, let on, let airs = now?.airs {
                        Scaled("\(airsWhen(airs, now: guide.serverNow())) on \(on.number)".uppercased(), h * 4, color: gold)
                        Scaled(p.title, h * 6.5, weight: .black, lines: 2)
                        if let line = p.episodeLabel ?? p.year.map(String.init) { Scaled(line, h * 3.6) }
                    } else {
                        Scaled("STAY TUNED", h * 4, color: gold)
                        Scaled("The Guide Channel", h * 6.5, weight: .black)
                    }
                    if let credit = guide.musicCredit { Scaled(credit, h * 2.4, color: .white.opacity(0.85), weight: .regular) }
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, h * 8)
        }
    }
}

/// Over a preview: when and where it airs.
struct PreviewCaption: View {
    let now: GuideNow
    let height: CGFloat

    var body: some View {
        let h = height / 100
        if let p = now.program, let on = now.airsOn, let airs = now.airs {
            HStack(spacing: 0) {
                gold.frame(width: h * 0.8)
                VStack(alignment: .leading, spacing: 2) {
                    Scaled("\(airsWhen(airs, now: now.serverTime)) · \(on.number) \(on.name)".uppercased(), h * 2.8, color: gold)
                    Scaled(p.title + (p.episodeLabel.map { " · " + $0 } ?? ""), h * 3.6)
                }
                .padding(.horizontal, h * 1.5).padding(.vertical, h * 0.8)
            }
            .fixedSize()
            .background(Color(red: 0, green: 0, blue: 0.16).opacity(0.75))
            .padding(.leading, h * 2).padding(.bottom, h * 2)
        }
    }
}

/// The Guide channel's lower half: every channel, now and the next two half hours, rolling up.
struct GuideListingsRoll: View {
    let guide: GuideModel
    let height: CGFloat // the whole picture's
    @State private var start = Date()

    var body: some View {
        let h = height / 100
        let channels = (guide.data?.channels ?? []).filter { $0.kind != "guide" }
        let first = guide.serverNow() / GuideModel.halfHour * GuideModel.halfHour
        let slots = [first, first + GuideModel.halfHour, first + 2 * GuideModel.halfHour]
        let row = h * 9
        let listHeight = row * CGFloat(max(channels.count, 1))
        // enough copies to fill the box the whole way round, so the roll loops without a jump or a gap
        let copies = max(2, Int(ceil(height * 0.38 / listHeight)) + 1)
        let period = Double(max(channels.count, 1)) * 3
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                Scaled(Date(timeIntervalSince1970: Double(first) / 1000).formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day()),
                       h * 3.2, color: Color(hex: 0x1A1A1A), shadow: false)
                    .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, h * 1.5)
                ForEach(slots, id: \.self) { t in
                    Scaled(guideTime(t), h * 3.2, color: Color(hex: 0x1A1A1A), shadow: false)
                        .frame(maxWidth: .infinity, alignment: .leading).padding(.horizontal, h * 1.5)
                }
            }
            .padding(.vertical, h * 0.6)
            .background(Color(hex: 0xC8A000))
            TimelineView(.animation) { context in
                let progress = context.date.timeIntervalSince(start).truncatingRemainder(dividingBy: period) / period
                VStack(spacing: 0) {
                    ForEach(0..<copies, id: \.self) { copy in
                        ForEach(channels) { c in
                            HStack(spacing: 0) {
                                HStack(spacing: h * 1) {
                                    GuideImage(relative: guide.logoPath(c, width: 256)).frame(width: h * 12, height: h * 6)
                                    Scaled("\(c.number) \(c.name)", h * 2.6)
                                }
                                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
                                .padding(.horizontal, h * 1)
                                .background(.white.opacity(0.08))
                                ForEach(slots, id: \.self) { t in
                                    let l = c.listings?.first { $0.start <= t && t < $0.end }
                                    let continued = l.map { $0.start < t && $0.info?.kind != "channel" } ?? false
                                    Scaled(l?.info?.title ?? "—", h * 3, color: continued ? .white.opacity(0.6) : .white)
                                        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
                                        .padding(.horizontal, h * 1)
                                }
                            }
                            .frame(height: row)
                            .overlay(alignment: .bottom) { Color.white.opacity(0.15).frame(height: 1) }
                            .id("\(copy)-\(c.number)")
                        }
                    }
                }
                .offset(y: -listHeight * progress)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
                .clipped()
            }
        }
        .background(LinearGradient(colors: [Color(hex: 0x0A1A6A), Color(hex: 0x06104A)], startPoint: .top, endPoint: .bottom))
    }
}

/// The Weather channel: its pages, turned every 10 seconds by the server's clock (the same on every screen).
struct WeatherScreen: View {
    let guide: GuideModel
    let height: CGFloat

    var body: some View {
        let h = height / 100
        let w = guide.weather
        let clock = guide.clock > 0 ? guide.clock : guide.serverNow()
        var pages: [String] = []
        if w?.current != nil { pages.append("now") }
        if !(w?.periods ?? []).isEmpty { pages += ["today", "extended"] }
        if w?.radar == true { pages.append("radar") }
        let page = pages.isEmpty ? "none" : pages[Int((clock / 10_000) % Int64(pages.count))]
        let titles = ["now": "Current Conditions", "today": "Local Forecast", "extended": "Extended Forecast", "radar": "Local Radar"]
        let alerts = w?.alerts ?? []
        return VStack(spacing: 0) {
            // the gold bar: the page, the place, the time
            HStack {
                Scaled((titles[page] ?? "Local Forecast").uppercased(), h * 6, color: Color(hex: 0x10204A), weight: .black, shadow: false)
                Spacer()
                Scaled(w?.place ?? "", h * 4, color: Color(hex: 0x10204A), shadow: false).padding(.trailing, h * 3)
                Scaled(Date(timeIntervalSince1970: Double(clock) / 1000).formatted(date: .omitted, time: .standard), h * 4,
                       color: Color(hex: 0x10204A), mono: true, shadow: false)
            }
            .padding(.horizontal, h * 4).padding(.vertical, h * 2)
            .background(LinearGradient(colors: [Color(hex: 0xE6B800), Color(hex: 0xB38600)], startPoint: .top, endPoint: .bottom))
            Group {
                switch page {
                case "now": nowPage(w!.current!, h)
                case "today":
                    HStack(alignment: .top) {
                        ForEach(Array((w?.periods ?? []).prefix(3).enumerated()), id: \.offset) { _, p in
                            VStack(spacing: h * 1.2) {
                                Scaled(p.name.uppercased(), h * 4.6, color: gold, weight: .black)
                                GuideImage(relative: guide.weatherIconPath(p.icon)).frame(width: h * 14, height: h * 14)
                                Scaled("\(p.daytime ? "High" : "Low") \(p.temp)°", h * 6, color: p.daytime ? .white : lightBlue, weight: .black)
                                Scaled(p.forecast, h * 3.6, lines: 2).multilineTextAlignment(.center)
                                if let c = p.precip, c > 0 { Scaled("\(c)% chance", h * 3.2, color: lightBlue) }
                            }
                            .frame(maxWidth: .infinity)
                        }
                    }
                case "extended":
                    let periods = w?.periods ?? []
                    let days = periods.enumerated().compactMap { i, p -> (WeatherPeriod, WeatherPeriod?)? in
                        guard p.daytime else { return nil }
                        let night = i + 1 < periods.count && !periods[i + 1].daytime ? periods[i + 1] : nil
                        return (p, night)
                    }.dropFirst().prefix(5)
                    HStack(alignment: .top) {
                        ForEach(Array(days.enumerated()), id: \.offset) { _, d in
                            VStack(spacing: h * 1.2) {
                                Scaled(String(d.0.name.prefix(3)).uppercased(), h * 4.6, color: gold, weight: .black)
                                GuideImage(relative: guide.weatherIconPath(d.0.icon)).frame(width: h * 14, height: h * 14)
                                Scaled(d.0.forecast, h * 3.4, lines: 2).multilineTextAlignment(.center)
                                Scaled("\(d.0.temp)°" + (d.1.map { " / \($0.temp)°" } ?? ""), h * 5, weight: .black)
                            }
                            .frame(maxWidth: .infinity)
                        }
                    }
                case "radar":
                    GuideImage(relative: guide.weatherRadarPath).border(.white, width: 2)
                default:
                    Scaled(w?.error ?? "The forecast isn't in yet.", h * 5, lines: 3).multilineTextAlignment(.center)
                }
            }
            .padding(.horizontal, h * 4).padding(.vertical, h * 3)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            // warnings scroll along the bottom in red; the music's credit stays on screen
            if !alerts.isEmpty, let credit = guide.musicCredit {
                Scaled(credit, h * 2.6, color: Color(hex: 0xC9DCFF), weight: .regular)
                    .frame(maxWidth: .infinity, alignment: .trailing).padding(.horizontal, h * 4).padding(.vertical, h * 0.6)
            }
            ZStack(alignment: .leading) {
                (alerts.isEmpty ? Color.black.opacity(0.35) : Color(hex: 0xB00020))
                if !alerts.isEmpty {
                    Ticker(text: alerts.map { ($0.headline?.isEmpty == false ? $0.headline! : $0.event) }.joined(separator: "   ✦   "), size: h * 4)
                } else if let credit = guide.musicCredit {
                    Scaled(credit, h * 3, weight: .regular).padding(.horizontal, h * 4)
                }
            }
            .frame(height: h * 7)
            .clipped()
        }
        .background(LinearGradient(colors: [Color(hex: 0x1D3F9A), Color(hex: 0x0A1F5C), Color(hex: 0x081640)], startPoint: .top, endPoint: .bottom))
        .task(id: clock / 60_000) { await guide.loadWeather() } // read again once it's 5 minutes old
    }

    private func nowPage(_ c: WeatherNow, _ h: CGFloat) -> some View {
        var facts: [(String, String)] = []
        if let v = c.humidity { facts.append(("Humidity", "\(v)%")) }
        if let v = c.dewpoint { facts.append(("Dewpoint", "\(v)°")) }
        if let v = c.wind { facts.append(("Wind", v == 0 ? "Calm" : "\(c.windDir ?? "") \(v) mph".trimmingCharacters(in: .whitespaces))) }
        if let v = c.gust { facts.append(("Gusts", "\(v) mph")) }
        if let v = c.pressure { facts.append(("Pressure", String(format: "%.2f in", v))) }
        if let v = c.visibility { facts.append(("Visibility", "\(v) mi")) }
        if let v = c.feelsLike, v != c.temp { facts.append(("Feels like", "\(v)°")) }
        return HStack {
            VStack(spacing: h * 1) {
                HStack(spacing: h * 3) {
                    GuideImage(relative: guide.weatherIconPath(c.icon)).frame(width: h * 30, height: h * 30)
                    Scaled(c.temp.map { "\($0)°" } ?? "—", h * 22, weight: .black)
                }
                Scaled(c.conditions ?? "", h * 5)
            }
            .frame(maxWidth: .infinity)
            VStack(alignment: .leading, spacing: h * 1.2) {
                ForEach(facts, id: \.0) { f in
                    HStack(spacing: 0) {
                        Scaled(f.0, h * 4.6, color: lightBlue).frame(width: h * 30, alignment: .leading)
                        Scaled(f.1, h * 4.6)
                    }
                }
            }
            .frame(maxWidth: .infinity)
        }
    }
}

/// Text scrolling right to left, over and over.
private struct Ticker: View {
    let text: String
    let size: CGFloat
    @State private var start = Date()
    @State private var textWidth: CGFloat = 0

    var body: some View {
        GeometryReader { geo in
            TimelineView(.animation) { context in
                let travel = geo.size.width + textWidth
                let speed = max(geo.size.width / 12, 20) // points a second
                let x = travel > 0 ? CGFloat(context.date.timeIntervalSince(start)).truncatingRemainder(dividingBy: travel / speed) * speed : 0
                Scaled(text, size, shadow: false)
                    .fixedSize()
                    .onGeometryChange(for: CGFloat.self) { $0.size.width } action: { textWidth = $0 }
                    .offset(x: geo.size.width - x)
                    .frame(maxHeight: .infinity)
            }
        }
    }
}

/// Beside a voice-over's squeezed credits: what's next on the channel.
struct UpNextCard: View {
    let guide: GuideModel
    let size: CGSize

    var body: some View {
        let h = size.height / 100
        if let now = guide.now, let next = now.next {
            HStack {
                Spacer(minLength: 0)
                VStack(alignment: .leading, spacing: h * 1.2) {
                    GuideImage(relative: guide.logoPath(now.channel, width: 512)).frame(height: h * 14, alignment: .leading)
                    Scaled("UP NEXT · " + guideTime(now.nextStart ?? 0), h * 4, color: gold, weight: .black)
                    Scaled(next.title, h * 6.5, weight: .black, lines: 2)
                    if let e = next.episodeLabel { Scaled(e, h * 3.6) }
                    if let poster = next.poster {
                        GuideImage(relative: poster).border(.white, width: 2).frame(maxHeight: .infinity, alignment: .topLeading)
                    }
                }
                .frame(width: size.width * 0.38, alignment: .leading)
                .padding(.vertical, h * 6).padding(.trailing, size.width * 0.03)
            }
            .frame(width: size.width, height: size.height)
            .background(RadialGradient(colors: [guideColor(now.channel.color), Color(hex: 0x0B0B1E)], center: .center,
                                       startRadius: 0, endRadius: size.height).zIndex(-1))
            .allowsHitTesting(false)
        }
    }
}
