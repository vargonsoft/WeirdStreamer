import AVFoundation
import Foundation
import Observation

// The Guide (docs/GUIDE.md): live channels the server plans, as Android's GuideModel. A channel
// plays only what's on now, from where it is: no seeking, and a pause mutes instead. GuideView.swift
// draws the screen on it.

@MainActor
@Observable
final class GuideModel {
    static let halfHour: Int64 = 30 * 60 * 1000

    let client: ServerClient
    /// The library whose own Guide it is, nil the home screen's.
    let library: String?
    private(set) var data: GuideListings?
    private(set) var error: String?
    private(set) var channel: Int?
    private(set) var now: GuideNow?
    /// The program whose details show: the one chosen in the grid, else the channel's.
    var selected: (channel: GuideChannel, listing: GuideListing)?
    /// Whether something plays (a program or filler); nil between programs, where the card shows.
    private(set) var playing = false
    var muted = false { didSet { player.isMuted = muted; music.isMuted = muted } }
    /// The Weather channel's forecast, while it's tuned.
    private(set) var weather: GuideWeather?
    @ObservationIgnored private var weatherAt = Date.distantPast
    /// The music under the Weather channel and the Guide channel's cards.
    let music = AVPlayer()
    @ObservationIgnored private var musicURL: URL?
    var fullscreen = false
    /// Shown for a few seconds after tuning: the channel's number (or the digits being typed), and its logo.
    private(set) var osd: String?
    private(set) var osdLogo: GuideChannel?
    /// The server's time, each second, for the time line.
    private(set) var clock: Int64 = 0

    let player = AVPlayer()
    @ObservationIgnored private var skew: Int64 = 0 // the server's clock less this one's
    @ObservationIgnored private var loop: Task<Void, Never>?
    @ObservationIgnored private var follow: Task<Void, Never>?
    @ObservationIgnored private var osdTask: Task<Void, Never>?
    @ObservationIgnored private var relay: HLSRelay?
    @ObservationIgnored private var fellBack = false
    @ObservationIgnored private var itemObservation: NSKeyValueObservation?
    @ObservationIgnored private var pauseObservation: NSKeyValueObservation?
    @ObservationIgnored private var streamKey = ""
    @ObservationIgnored private let lastKey: String

    /// Settings' "Mark Guide shows watched": a show that plays to its end on the tuned channel is marked watched.
    @ObservationIgnored private let marksWatched: Bool

    init(client: ServerClient, library: String?, marksWatched: Bool = false) {
        self.client = client
        self.library = library
        self.marksWatched = marksWatched
        lastKey = "guideChannel:\(client.baseURL.absoluteString):\(library ?? "")"
        // Live TV doesn't pause: a pause from outside (a headset's button, Control Center) mutes instead.
        pauseObservation = player.observe(\.timeControlStatus, options: [.new]) { [weak self] player, _ in
            guard let self else { return }
            Task { @MainActor in
                guard self.playing, player.timeControlStatus == .paused,
                      player.currentItem?.status == .readyToPlay else { return }
                let item = player.currentItem
                if let item, item.duration.isNumeric, item.currentTime().seconds >= item.duration.seconds - 0.5 { return } // its end
                self.muted = true
                player.play()
            }
        }
    }

    func serverNow() -> Int64 { Int64(Date().timeIntervalSince1970 * 1000) + skew }

    private var libQuery: [(String, String)] { library.map { [("lib", $0)] } ?? [] }

    /// Loads the grid and tunes to the last channel watched here (else the first); keeps the grid current.
    func start() {
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        try? AVAudioSession.sharedInstance().setActive(true)
        loop = Task { [weak self] in
            guard let self else { return }
            await self.loadGrid()
            if let channels = self.data?.channels, !channels.isEmpty, self.channel == nil {
                let last = UserDefaults.standard.object(forKey: self.lastKey) as? Int
                self.tune((channels.first { $0.number == last } ?? channels[0]).number)
            }
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                self.clock = self.serverNow()
                if let from = self.data?.from, self.serverNow() >= from + Self.halfHour { await self.loadGrid() }
            }
        }
    }

    /// Loads the grid from the current half hour, keeping as many hours as were loaded (at least 3½).
    func loadGrid() async {
        let first = serverNow() / Self.halfHour * Self.halfHour
        let hours = max(3.5, Double((data?.to ?? 0) - first) / 3_600_000)
        do {
            let grid: GuideListings = try await client.get("/api/guide", query: libQuery + [("hours", String(hours))])
            skew = grid.now - Int64(Date().timeIntervalSince1970 * 1000)
            data = grid
            error = nil
        } catch {
            if data == nil { self.error = errorText(error) }
        }
    }

    @ObservationIgnored private var extending = false

    /// Loads the next 3 hours onto the grid (scrolled near its end), up to as far as the plan reaches.
    func extendGrid() {
        guard let d = data, !extending, d.until.map({ d.to < $0 }) ?? true else { return }
        extending = true
        Task {
            defer { extending = false }
            guard let more: GuideListings = try? await client.get("/api/guide",
                query: libQuery + [("hours", "3"), ("from", "\(d.to)")]), data?.to == d.to else { return }
            var grid = d
            grid.to = more.to
            grid.until = more.until
            let byNumber = Dictionary((more.channels ?? []).map { ($0.number, $0) }, uniquingKeysWith: { a, _ in a })
            grid.channels = d.channels?.map { c in
                var c = c
                let have = c.listings ?? []
                // a program across the boundary comes in both: kept once
                let add = (byNumber[c.number]?.listings ?? []).filter { a in !have.contains { $0.start == a.start } }
                c.listings = have + add
                return c
            }
            data = grid
        }
    }

    /// A channel's logo, width pixels wide in color ("rrggbb"), transparent around it.
    func logoPath(_ c: GuideChannel, width: Int, color: String = "ffffff") -> String? {
        guard let logo = c.logo, !logo.isEmpty else { return nil }
        return "\(logo)&w=\(width)&color=\(color)"
    }

    /// Tunes to a channel: plays what it airs now, from where it is.
    func tune(_ number: Int) {
        let changed = channel != number
        channel = number
        UserDefaults.standard.set(number, forKey: lastKey)
        if changed { showOsd("\(number)", data?.channels?.first { $0.number == number }) }
        follow?.cancel()
        follow = Task { [weak self] in await self?.follow(number) }
    }

    /// The next or previous channel in the grid's order.
    func surf(_ step: Int) {
        guard let channels = data?.channels, !channels.isEmpty else { return }
        let i = channels.firstIndex { $0.number == channel } ?? 0
        tune(channels[((i + step) % channels.count + channels.count) % channels.count].number)
    }

    private func showOsd(_ text: String, _ c: GuideChannel?) {
        osd = text
        osdLogo = c
        osdTask?.cancel()
        osdTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(4))
            guard !Task.isCancelled else { return }
            self?.osd = nil
            self?.osdLogo = nil
        }
    }

    // Follows the channel: what it airs, and when that ends, what's next.
    private func follow(_ number: Int) async {
        while !Task.isCancelled && channel == number {
            let n: GuideNow
            do {
                n = try await client.get("/api/guide/now", query: [("channel", "\(number)")] + libQuery)
            } catch {
                guard !Task.isCancelled else { return }
                now = nil
                stopItem()
                if case ServerError.http(503, _) = error { self.error = "This channel isn't planned yet" } else { self.error = errorText(error) }
                try? await Task.sleep(for: .seconds(30))
                continue
            }
            guard !Task.isCancelled && channel == number else { return }
            skew = n.serverTime - Int64(Date().timeIntervalSince1970 * 1000)
            // A show that just played to its end on this channel, with the setting on: marked watched
            if marksWatched, let ended = now, ended.kind == "program", ended.channel.number == number,
               ended.channel.kind == nil, n.start >= ended.end {
                markWatched(channel: number, start: ended.start)
            }
            now = n
            fellBack = false
            // the details follow the channel unless something else on it was picked
            if let c = data?.channels?.first(where: { $0.number == number }),
               let l = c.listings?.first(where: { $0.start <= n.start && n.start < $0.end }),
               selected?.channel.number != number || (selected?.listing.end ?? 0) <= serverNow() {
                selected = (c, l)
            }
            if n.channel.kind == "weather" { Task { await loadWeather() } }
            // The Weather channel and the Guide channel's cards are drawn, with music; the rest plays
            if n.channel.kind == "weather" || (n.channel.kind == "guide" && n.kind == "promo") {
                stopItem()
                playMusic(n)
            } else if let path = n.path, n.kind != "bumper" {
                stopMusic()
                await play(n, path: path, transcode: false)
            } else {
                stopMusic()
                stopItem()
            }
            let wait = max(n.end - serverNow(), 0) + 50
            try? await Task.sleep(for: .milliseconds(wait))
        }
    }

    /// Plays an item: the file itself, seeked, where this phone plays it; else the server's stream
    /// from where the channel is (the video copied when it can be, the audio kept when it plays here).
    private func play(_ n: GuideNow, path: String, transcode: Bool) async {
        let key = "\(n.channel.number)|\(n.start)|\(transcode)"
        if key == streamKey && playing { return }
        let codecs: CodecsInfo? = transcode ? nil
            : try? await client.get("/api/media/codecs", query: [("path", ServerClient.encodePath(path))])
        guard !Task.isCancelled && channel == n.channel.number else { return }
        stopItem()
        streamKey = key
        // where the server said it is in the file, plus the time since
        let offset = max((n.offset ?? 0) + Double(serverNow() - n.serverTime) / 1000, 0)
        let item: AVPlayerItem
        if let codecs, OfflinePlayback.of(codecs, path: path).plays {
            // The file as it is, in byte ranges through the app (for the server's kept certificate).
            item = AVPlayerItem(asset: OriginalLoader.asset(for: client.streamURL(for: path), token: client.token))
            observe(item, seekTo: offset)
        } else {
            let video = codecs?.video
            let copy = !transcode && video?.codec == "h264" && ["yuv420p", "yuvj420p"].contains(video?.pixFmt ?? "")
            let track = codecs?.audio?.first { $0.default == true } ?? codecs?.audio?.first
            let keepAudio = !transcode && ["aac", "mp3", "ac3", "eac3", "alac", "flac"].contains(track?.codec?.lowercased() ?? "")
            var parts = [copy ? "transcode=audio" : "transcode=1"]
            parts.append(transcode ? "audiocodec=aac&channels=2" : keepAudio ? "audiocodec=copy" : "audiocodec=aac")
            if offset > 1 { parts.append("start=" + String(format: "%.2f", offset)) }
            // A transcode has no length: relayed as HLS from the app's own little server.
            do {
                _ = try await HLSServer.shared.start()
            } catch {
                self.error = error.localizedDescription
                return
            }
            let remaining = max(Double(n.end - serverNow()) / 1000, 1)
            let relay = HLSRelay(source: client.streamURL(for: path, query: parts.joined(separator: "&")),
                                 token: client.token, expectedDuration: remaining)
            HLSServer.shared.add(relay)
            relay.start()
            self.relay = relay
            item = AVPlayerItem(url: HLSServer.shared.playlistURL(for: relay))
            observe(item, seekTo: nil)
        }
        player.replaceCurrentItem(with: item)
        player.isMuted = muted
        playing = true
        player.play()
    }

    private func observe(_ item: AVPlayerItem, seekTo offset: Double?) {
        var pending = offset
        itemObservation = item.observe(\.status, options: [.new]) { [weak self] item, _ in
            guard let self else { return }
            Task { @MainActor in
                guard self.player.currentItem === item else { return }
                switch item.status {
                case .readyToPlay:
                    if let seek = pending, seek > 0.5 {
                        pending = nil
                        // where the channel is now, not where it was when the item was opened
                        let start = self.now.map { max(($0.offset ?? 0) + Double(self.serverNow() - $0.serverTime) / 1000, 0) } ?? seek
                        self.player.seek(to: CMTime(seconds: start, preferredTimescale: 600),
                                         toleranceBefore: .zero, toleranceAfter: .zero)
                    }
                case .failed:
                    self.playerFailed()
                default: break
                }
            }
        }
    }

    /// The player failed on what it was given: a full transcode from where the channel is, once.
    private func playerFailed() {
        guard let n = now, let path = n.path, !fellBack else { return }
        fellBack = true
        Task { await play(n, path: path, transcode: true) }
    }

    private func stopItem() {
        playing = false
        streamKey = ""
        itemObservation?.invalidate()
        itemObservation = nil
        player.pause()
        player.replaceCurrentItem(with: nil)
        if let relay { HLSServer.shared.remove(relay) }
        relay = nil
    }

    /// Marks the program that started at start on a channel watched: it played to its end.
    private func markWatched(channel: Int, start: Int64) {
        let query = [("channel", "\(channel)"), ("start", "\(start)")] + libQuery
        Task { try? await client.execute("POST", "/api/guide/watched", query: query) }
    }

    /// Plays the item's music from where the clock says (going on if it's already that track).
    private func playMusic(_ n: GuideNow) {
        guard let m = n.music, let url = client.url(for: m.url) else { return stopMusic() }
        let at = max(m.offset + Double(serverNow() - n.serverTime) / 1000, 0)
        music.isMuted = muted
        if musicURL == url, music.currentItem != nil {
            if abs(music.currentTime().seconds - at) > 3 { music.seek(to: CMTime(seconds: at, preferredTimescale: 600)) }
        } else {
            musicURL = url
            let item = AVPlayerItem(asset: OriginalLoader.asset(for: url, token: client.token))
            music.replaceCurrentItem(with: item)
            music.seek(to: CMTime(seconds: at, preferredTimescale: 600))
        }
        music.play()
    }

    private func stopMusic() {
        musicURL = nil
        music.pause()
        music.replaceCurrentItem(with: nil)
    }

    /// The music's credit line while it plays ("♪ …"), else nil.
    var musicCredit: String? {
        guard let m = now?.music else { return nil }
        let text = (m.credit?.isEmpty == false ? m.credit : m.title) ?? ""
        return text.isEmpty ? nil : "♪ " + text
    }

    /// Reads the forecast (the server keeps it 10 minutes; asked again after 5).
    func loadWeather() async {
        if weather != nil && Date().timeIntervalSince(weatherAt) < 300 { return }
        do {
            weather = try await client.get("/api/guide/weather")
            weatherAt = Date()
        } catch {
            if weather == nil { weather = GuideWeather(error: errorText(error)) }
        }
    }

    func weatherIconPath(_ name: String?) -> String {
        "/api/guide/weather/icon?name=\(name ?? "cloud")&w=256"
    }

    var weatherRadarPath: String { "/api/guide/weather/radar?v=\(serverNow() / 300_000)" }

    func stop() {
        stopMusic()
        loop?.cancel()
        follow?.cancel()
        osdTask?.cancel()
        stopItem()
        now = nil
        channel = nil
    }
}

/// A day other than today as "Fri, Nov 27 " (before a time), else "".
func guideDay(_ ms: Int64) -> String {
    let date = Date(timeIntervalSince1970: Double(ms) / 1000)
    if Calendar.current.isDateInToday(date) { return "" }
    return date.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day()) + " "
}

/// A half hour's label in the grid: its time, or the day at midnight ("Fri, Nov 27").
func guideTimeOrDay(_ ms: Int64) -> String {
    let date = Date(timeIntervalSince1970: Double(ms) / 1000)
    let parts = Calendar.current.dateComponents([.hour, .minute], from: date)
    if parts.hour == 0 && parts.minute == 0 { return date.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day()) }
    return guideTime(ms)
}

/// A time of day as the phone shows them ("8:30 PM").
func guideTime(_ ms: Int64) -> String {
    Date(timeIntervalSince1970: Double(ms) / 1000).formatted(date: .omitted, time: .shortened)
}

/// A program's time slot, channel, rating and genres, for the details ("8:00 PM – 8:30 PM · Channel 4 · KBTV · TV-PG · Comedy").
func guideFacts(_ c: GuideChannel, _ l: GuideListing) -> String {
    var parts = ["\(guideDay(l.start))\(guideTime(l.start)) – \(guideTime(l.end))", "Channel \(c.number) · \(c.name)"]
    if let r = l.info?.rating, !r.isEmpty { parts.append(r) }
    if let g = l.info?.genres, !g.isEmpty { parts.append(g.joined(separator: ", ")) }
    return parts.joined(separator: " · ")
}
