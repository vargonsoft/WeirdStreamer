import SwiftUI
import AVKit
import AVFoundation

// The phone player, as Android's PhoneVideoPlayer: the video on black (pinch to zoom, drag to
// pan, aspect ratio and rotation), and controls that show on a tap and hide after 5 seconds —
// Back, the title, the Transcode menu and Rotate along the top; −10 s, play/pause and +30 s, and
// frame steps, in the middle; trickplay frames, Aspect and Speed, the seek bar and times at the
// bottom; Skip Intro / Skip Credits above them. Picture in picture when you go home.

/// A transcode mode, as Android's transcodeModes.
struct TranscodeMode: Hashable {
    let name: String
    var isOriginal = false
    var auto = false
    var safe = false
    var bitrate: String? = nil
    var resolution: String? = nil

    static let all: [TranscodeMode] = [
        .init(name: "Original", isOriginal: true),
        .init(name: "Compatibility (Auto)", auto: true),
        .init(name: "Compatibility (Safe)", safe: true),
        .init(name: "Quality -20%", bitrate: "80%"),
        .init(name: "Quality -35%", bitrate: "65%"),
        .init(name: "Quality -50%", bitrate: "50%"),
        .init(name: "720p", resolution: "720"),
        .init(name: "480p", resolution: "480"),
        .init(name: "360p", resolution: "360"),
    ]
}

private let speeds: [Float] = [0.1, 0.25, 0.5, 1, 2, 4, 6, 8, 10, 12, 16, 32, 64]
private let aspectNames = ["Original", "16:9", "21:9", "4:3", "5:4", "1:1"]
private let aspectValues: [CGFloat?] = [nil, 16 / 9, 21 / 9, 4 / 3, 5 / 4, 1]

/// What iOS plays as it is: containers and codecs AVPlayer reads from a progressive stream.
private let playableContainers: Set<String> = ["mp4", "m4v", "mov"]
private let playableVideo: Set<String> = ["h264", "hevc", "mpeg4"]
private let playableAudio: Set<String> = ["aac", "mp3", "ac3", "eac3", "alac", "flac"]

@MainActor
@Observable
final class PlaybackModel {
    let request: PlayRequest
    private(set) var player = AVPlayer()
    private(set) var modeIndex = 0
    private(set) var isPlaying = false
    private(set) var isBuffering = true
    private(set) var failure: String?
    private(set) var position: Double = 0          // seconds, in the whole video
    private(set) var duration: Double = 0
    private(set) var trickplay: [TrickplayFrame] = []
    private(set) var segments: MediaSegments?
    private(set) var copiesVideo = false
    private(set) var subtitles: ServerSubtitles?
    /// The file's audio tracks, as the server lists them, and the one chosen (the server's index;
    /// -1: the file's default). A track other than the default comes from the server (audio=N),
    /// as on the web: AVPlayer plays only the original's default track here.
    private(set) var audioTracks: [AudioTrackInfo] = []
    private(set) var audioTrack = -1
    /// A DVD image (.iso): always transcoded by the server, one title at a time, with the chosen
    /// angle and tracks; the Disc sheet changes them.
    let isDisc: Bool
    private(set) var disc: DiscInfo?
    private(set) var discTitle = 0          // 0: the main title
    private(set) var discAngle = 1
    private(set) var discAudio = 0
    private(set) var discSubtitle = -1
    @ObservationIgnored private var allTrickplay: TrickplayInfo?
    var speedIndex = 3
    var title: String

    /// Where the current stream starts in the video: transcodes are restarted at an offset.
    @ObservationIgnored private var offset: Double = 0
    @ObservationIgnored private var codecs: CodecsInfo?
    /// The download playing, when it's a file on this phone.
    @ObservationIgnored private(set) var download: Download?
    @ObservationIgnored private var originalByHand = false
    @ObservationIgnored private var videoFailed = false
    @ObservationIgnored private var timeToken: Any?
    @ObservationIgnored private var observations: [NSKeyValueObservation] = []
    @ObservationIgnored private var endToken: NSObjectProtocol?
    @ObservationIgnored private var lastReport = Date.distantPast
    @ObservationIgnored private var pendingSeek: Double?
    @ObservationIgnored private weak var model: AppModel?
    @ObservationIgnored private var relay: HLSRelay?
    @ObservationIgnored var onFinished: (() -> Void)?

    var mode: TranscodeMode { TranscodeMode.all[modeIndex] }
    var isTranscoding: Bool { !mode.isOriginal }

    init(request: PlayRequest) {
        self.request = request
        self.title = request.title
        self.isDisc = request.localFile == nil && (request.path as NSString).pathExtension.lowercased() == "iso"
    }

    // MARK: Starting

    func start(model: AppModel) async {
        self.model = model
        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback)
        try? AVAudioSession.sharedInstance().setActive(true)
        if let file = request.localFile {
            // A download: the file on this phone, as it is (it was made to play here), with the
            // subtitles, skip times and trickplay frames saved beside it.
            originalByHand = true
            modeIndex = 0
            if let d = DownloadManager.shared.download(atFile: file) {
                download = d
                codecs = DownloadExtras.codecs(d.id)
                if let length = codecs?.duration, length > 0 { duration = length }
                if let saved = DownloadExtras.subtitleTracks(d.id) {
                    let subs = ServerSubtitles(client: model.client, path: request.path, downloadId: d.id)
                    subs.setTracks(saved)
                    let remembered = model.setting(subtitleMemoryKey(model)).flatMap { label in subs.labels.firstIndex(of: label) }
                    subs.select(remembered ?? subs.tracks.firstIndex { $0.forced == true && $0.isText } ?? -1)
                    subtitles = subs
                }
                trickplay = DownloadExtras.trickplay(d.id)
                segments = DownloadExtras.segments(d.id)
                if let t = segments?.metadata?.playerTitle { title = t }
            }
            let item = AVPlayerItem(url: file)
            player.replaceCurrentItem(with: item)
            pendingSeek = request.startAt > 0 ? request.startAt : nil
            observe(item)
            player.rate = speeds[speedIndex]
            return
        }
        guard let client = model.client else { failure = "Not signed in."; return }
        let encoded = ServerClient.encodePath(request.path)
        if isDisc {
            // A disc: its titles, then the main one transcoded (a disc can't play as it is).
            disc = try? await client.get("/api/media/disc", query: [("path", encoded)])
            if let t = disc?.title(0) { duration = t.duration }
            modeIndex = 1
            await open(at: request.startAt)
            if let info: TrickplayInfo = try? await client.get("/api/media/trickplay", query: [("path", encoded)]) {
                allTrickplay = info
                applyDiscTrickplay()
            }
            return
        }
        codecs = try? await client.get("/api/media/codecs", query: [("path", encoded)])
        if let d = codecs?.duration, d > 0 { duration = d }
        // Subtitles: the one chosen for this video before, else a forced text track, else off.
        let subs = ServerSubtitles(client: client, path: request.path)
        subs.setTracks(codecs?.subtitles ?? [])
        let remembered = model.setting(subtitleMemoryKey(model)).flatMap { label in subs.labels.firstIndex(of: label) }
        subs.select(remembered ?? subs.tracks.firstIndex { $0.forced == true && $0.isText } ?? -1)
        subtitles = subs
        // Audio: the track chosen for this video before, else the file's default.
        audioTracks = codecs?.audio ?? []
        if let label = model.setting(audioMemoryKey(model)), let i = audioLabels(audioTracks).firstIndex(of: label) {
            audioTrack = audioTracks[i].index ?? i
        }
        // As Android: the original unless this phone can't play it, then Compatibility (Auto).
        modeIndex = originalPlays() ? 0 : 1
        await open(at: request.startAt)
        Task {
            if let info: TrickplayInfo = try? await client.get("/api/media/trickplay", query: [("path", encoded)]) {
                trickplay = info.frames ?? []
            }
        }
        Task {
            if let found: MediaSegments = try? await client.get("/api/media/segments", query: [("path", encoded)]) {
                segments = found
                if let t = found.metadata?.playerTitle { title = t }
            }
        }
    }

    /// Whether the file plays as it is here: its container and its video and default audio codecs.
    private func originalPlays() -> Bool {
        let ext = (request.path as NSString).pathExtension.lowercased()
        guard playableContainers.contains(ext) else { return false }
        if (subtitles?.burnIn ?? -1) >= 0 { return false } // a DVD picture track is burned into a transcode
        guard let codecs else { return true }
        if let v = codecs.video?.codec?.lowercased(), !playableVideo.contains(v) { return false }
        if let a = defaultAudio()?.codec?.lowercased(), !playableAudio.contains(a) { return false }
        if choosesOtherAudio { return false } // another track comes from the server
        return true
    }

    private func defaultAudio() -> AudioTrackInfo? {
        codecs?.audio?.first { $0.default == true } ?? codecs?.audio?.first
    }

    /// The audio track the stream plays: the one chosen, else the default.
    private func playingAudio() -> AudioTrackInfo? {
        audioTracks.first { $0.index == audioTrack } ?? defaultAudio()
    }

    /// The track the Audio menu checks: the one chosen, else the default.
    var shownAudioTrack: Int { audioTrack >= 0 ? audioTrack : (defaultAudio()?.index ?? -1) }

    /// Whether a track other than the file's default is chosen.
    private var choosesOtherAudio: Bool {
        audioTrack >= 0 && audioTrack != (defaultAudio()?.index ?? -1)
    }

    private var keepsAudio: Bool {
        guard !mode.safe, mode.bitrate == nil, mode.resolution == nil,
              let codec = playingAudio()?.codec?.lowercased() else { return false }
        return playableAudio.contains(codec)
    }

    /// The stream's URL parameters for the mode, from `start` seconds.
    private func query(start: Double) -> String {
        let mode = self.mode
        if isDisc {
            var parts = ["transcode=1"]
            if let b = mode.bitrate { parts.append("bitrate=\(b)") }
            if let r = mode.resolution { parts.append("resolution=\(r)") }
            parts.append(mode.safe ? "audiocodec=aac&channels=2" : "audiocodec=aac")
            parts.append("title=\(discTitle)&angle=\(discAngle)&audio=\(discAudio)&subtitle=\(discSubtitle)")
            if start > 0 { parts.append("start=" + String(format: "%.2f", start)) }
            copiesVideo = false
            return parts.joined(separator: "&")
        }
        if mode.isOriginal { return "" }
        let video = codecs?.video
        let burn = subtitles?.burnIn ?? -1
        copiesVideo = mode.auto && !videoFailed && burn < 0 && video?.codec == "h264"
            && ["yuv420p", "yuv420p", "yuvj420p"].contains(video?.pixFmt ?? "")
        var parts: [String] = []
        if copiesVideo {
            parts.append("transcode=audio")
        } else {
            parts.append("transcode=1")
            if let b = mode.bitrate { parts.append("bitrate=\(b)") }
            if let r = mode.resolution { parts.append("resolution=\(r)") }
        }
        if mode.safe {
            parts.append("audiocodec=aac&channels=2")
        } else {
            parts.append(keepsAudio ? "audiocodec=copy" : "audiocodec=aac")
        }
        if audioTrack >= 0 { parts.append("audio=\(audioTrack)") }
        if burn >= 0 { parts.append("subtitle=\(burn)") }
        if start > 0 { parts.append("start=" + String(format: "%.2f", start)) }
        return parts.joined(separator: "&")
    }

    /// Opens the stream for the current mode at `start` seconds.
    private func open(at start: Double) async {
        guard let model, let client = model.client else { return }
        tearDownItem()
        failure = nil
        isBuffering = true
        var begins = start
        if !mode.isOriginal {
            _ = query(start: start) // decides copiesVideo
            if copiesVideo, start > 0,
               let point: SeekPoint = try? await client.get("/api/media/seekpoint",
                                                            query: [("path", ServerClient.encodePath(request.path)),
                                                                    ("start", String(format: "%.2f", start))]),
               let s = point.start, s >= 0 {
                begins = s
            }
        }
        offset = mode.isOriginal ? 0 : begins
        position = start
        let url = client.streamURL(for: request.path, query: query(start: start))
        let item: AVPlayerItem
        if mode.isOriginal {
            // The file as it is, straight from the server (it answers byte ranges).
            let headers = ["Authorization": "Bearer \(client.token ?? "")"]
            item = AVPlayerItem(asset: AVURLAsset(url: url, options: ["AVURLAssetHTTPHeaderFieldsKey": headers]))
        } else {
            // A transcode has no length: relayed as HLS from the app's own little server.
            do {
                _ = try await HLSServer.shared.start()
            } catch {
                failure = error.localizedDescription
                isBuffering = false
                return
            }
            let relay = HLSRelay(source: url, token: client.token, expectedDuration: max((duration > 0 ? duration : 0) - begins, 1))
            HLSServer.shared.add(relay)
            relay.start()
            self.relay = relay
            item = AVPlayerItem(url: HLSServer.shared.playlistURL(for: relay))
        }
        player.replaceCurrentItem(with: item)
        pendingSeek = mode.isOriginal && start > 0 ? start : nil
        observe(item)
        player.rate = speeds[speedIndex]
    }

    private func observe(_ item: AVPlayerItem) {
        observations = [
            item.observe(\.status, options: [.new]) { [weak self] item, _ in
                guard let self else { return }
                Task { @MainActor in self.statusChanged(item) }
            },
            player.observe(\.timeControlStatus, options: [.new]) { [weak self] player, _ in
                guard let self else { return }
                Task { @MainActor in
                    self.isPlaying = player.timeControlStatus != .paused
                    self.isBuffering = player.timeControlStatus == .waitingToPlayAtSpecifiedRate
                }
            },
        ]
        timeToken = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 0.5, preferredTimescale: 600),
                                                   queue: .main) { [weak self] time in
            MainActor.assumeIsolated { self?.tick(time) }
        }
        endToken = NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: item,
                                                          queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.ended() }
        }
    }

    private func statusChanged(_ item: AVPlayerItem) {
        switch item.status {
        case .readyToPlay:
            if duration <= 0, item.duration.seconds.isFinite { duration = item.duration.seconds }
            if let seek = pendingSeek {
                pendingSeek = nil
                player.seek(to: CMTime(seconds: seek, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
            }
        case .failed:
            Task { await recover(item.error) }
        default:
            break
        }
    }

    /// As Android: the original fails over to Compatibility (Auto) unless it was picked by hand;
    /// a copied video gets a transcode; then the player says it can't play the video.
    private func recover(_ error: Error?) async {
        let at = position
        if mode.isOriginal && !originalByHand {
            modeIndex = 1
            await open(at: at)
        } else if copiesVideo && !videoFailed {
            videoFailed = true
            await open(at: at)
        } else {
            // The relay knows why better than AVPlayer (which only saw a failed playlist).
            failure = relay?.failed?.localizedDescription ?? error?.localizedDescription ?? "This video can't be played."
            isBuffering = false
        }
    }

    private func tick(_ time: CMTime) {
        guard time.seconds.isFinite, pendingSeek == nil else { return }
        position = offset + time.seconds
        if Date().timeIntervalSince(lastReport) >= 5 { report(paused: !isPlaying, ended: false) }
    }

    private func ended() {
        report(paused: true, ended: true)
        onFinished?()
    }

    // MARK: Controls

    func togglePlay() {
        if player.timeControlStatus == .paused {
            player.rate = speeds[speedIndex]
        } else {
            player.pause()
            report(paused: true, ended: false)
        }
    }

    /// Seeks within the stream when that part has arrived (or for the original); a transcode
    /// past what has arrived restarts at the new place.
    func seek(to target: Double) {
        let t = max(0, duration > 0 ? min(target, duration) : target)
        position = t
        if mode.isOriginal {
            player.seek(to: CMTime(seconds: t, preferredTimescale: 600), toleranceBefore: .zero, toleranceAfter: .zero)
            return
        }
        let local = t - offset
        let buffered = player.currentItem?.loadedTimeRanges.map(\.timeRangeValue).contains {
            local >= $0.start.seconds && local <= ($0.start + $0.duration).seconds
        } ?? false
        if local >= 0 && buffered {
            player.seek(to: CMTime(seconds: local, preferredTimescale: 600))
        } else {
            Task { await open(at: t) }
        }
    }

    func step(frames: Int) {
        player.pause()
        player.currentItem?.step(byCount: frames)
    }

    func chooseMode(_ index: Int) {
        guard index != modeIndex, !(isDisc && TranscodeMode.all[index].isOriginal) else { return }
        if TranscodeMode.all[index].isOriginal { originalByHand = true }
        modeIndex = index
        videoFailed = false
        let at = position
        Task { await open(at: at) }
    }

    /// Turns a subtitle track on (or off: -1) and remembers it for this video. A DVD picture track
    /// is burned into the stream, so the stream restarts (as a transcode) where it was.
    func chooseSubtitle(_ index: Int) {
        guard let subtitles, let model else { return }
        let before = subtitles.burnIn
        subtitles.select(index)
        model.setSetting(subtitleMemoryKey(model), subtitles.labels.indices.contains(index) ? subtitles.labels[index] : "off")
        guard subtitles.burnIn != before else { return }
        if subtitles.burnIn >= 0 && mode.isOriginal { modeIndex = 1 }
        let at = position
        Task { await open(at: at) }
    }

    /// Plays audio track `index` (the server's) and remembers it for this video. The original
    /// plays only its default track, so another one switches it to Compatibility (Auto).
    func chooseAudio(_ index: Int) {
        guard let model, let i = audioTracks.firstIndex(where: { $0.index == index }) else { return }
        model.setSetting(audioMemoryKey(model), audioLabels(audioTracks)[i])
        guard index != shownAudioTrack || audioTrack < 0 else { return }
        let before = audioTrack
        audioTrack = index
        if mode.isOriginal {
            guard choosesOtherAudio else { return } // the default, which the original plays already
            modeIndex = 1
            originalByHand = false
        } else if before < 0 && !choosesOtherAudio {
            return // the default, which the stream plays already
        }
        let at = position
        Task { await open(at: at) }
    }

    private func audioMemoryKey(_ model: AppModel) -> String {
        "audio_choice_\(model.currentAddress ?? "")|\(request.path)"
    }

    /// The picture's shape, for drawing PGS subtitles on it.
    var videoAspect: CGFloat {
        guard let size = player.currentItem?.presentationSize, size.height > 0 else { return 0 }
        return size.width / size.height
    }

    private func subtitleMemoryKey(_ model: AppModel) -> String {
        "subtitle_choice_\(model.currentAddress ?? "")|\(request.path)"
    }

    func chooseSpeed(_ index: Int) {
        speedIndex = index
        if player.timeControlStatus != .paused { player.rate = speeds[index] }
    }

    /// The Transcode menu's line on what the stream does.
    var status: String? {
        guard let codecs else { return nil }
        let track = playingAudio()
        let ch = track?.channels ?? 0
        let channels = ch >= 8 ? " 7.1" : (ch >= 6 ? " 5.1" : "")
        let kept = "original" + (track?.codec.map { " (\($0.uppercased())\(channels))" } ?? "")
        if mode.isOriginal { return codecs.video == nil ? nil : "Video: original · Audio: \(kept)" }
        let audio = mode.safe ? "converted to stereo" : (keepsAudio ? kept : "converted to AAC")
        return "Video: " + (copiesVideo ? "copied" : "transcoded to H.264") + " · Audio: " + audio
    }

    /// Skip Intro or Skip Credits, while the position is in one.
    var skipPrompt: (label: String, to: Double, goesOn: Bool)? {
        if let intro = segments?.intro, position >= intro.start, position < intro.end - 1 {
            return ("Skip Intro", intro.end, false)
        }
        if let credits = segments?.credits, position >= credits.start, position < credits.end - 1 {
            return ("Skip Credits", credits.end, nextDownload != nil || (download == nil && segments?.next != nil))
        }
        return nil
    }

    /// Offline, the next episode is the next one downloaded: the server's next if it's on the
    /// phone, else the next downloaded in the same folder.
    private var nextDownload: Download? {
        guard let download else { return nil }
        let manager = DownloadManager.shared
        if let next = segments?.next, let d = manager.find(next.path), d.isDone, d.localURL != nil { return d }
        return manager.nextDownloaded(after: download)
    }

    /// Goes on to the next episode; false if there's none.
    @discardableResult
    func playNext(_ model: AppModel) -> Bool {
        if let d = nextDownload, let file = d.localURL {
            model.playDownload(d, file: file)
            return true
        }
        if download == nil, let next = segments?.next {
            model.playContinue(next)
            return true
        }
        return false
    }

    // MARK: Discs

    /// Another title: it starts at its beginning, with the first angle and tracks.
    func chooseDiscTitle(_ number: Int) {
        guard let t = disc?.title(number), t.number != disc?.title(discTitle)?.number else { return }
        report(paused: true, ended: false)
        discTitle = number
        discAngle = 1
        discAudio = 0
        discSubtitle = -1
        duration = t.duration
        applyDiscTrickplay()
        Task { await open(at: 0) }
    }

    func chooseDiscAngle(_ angle: Int) { discAngle = angle; restartDisc() }
    func chooseDiscAudio(_ index: Int) { discAudio = index; restartDisc() }
    func chooseDiscSubtitle(_ index: Int) { discSubtitle = index; restartDisc() }

    private func restartDisc() {
        let at = position
        Task { await open(at: at) }
    }

    /// A disc's frames are of its main feature: none while another title plays.
    private func applyDiscTrickplay() {
        guard let info = allTrickplay else { return }
        let made = info.title ?? 0
        let main = disc?.mainTitle
        let fits = made == 0 || discTitle == made || (discTitle == 0 && (main == nil || main == made))
        trickplay = fits ? (info.frames ?? []) : []
    }

    // MARK: Progress

    func report(paused: Bool, ended: Bool) {
        guard let model, position.isFinite else { return }
        lastReport = Date()
        let path = request.path, at = position, total = duration
        // The main title is 0 to the server even when picked by its number (as the web sends it).
        let title = discTitle == disc?.mainTitle ? 0 : discTitle
        Task { await model.recordProgress(path: path, position: at, duration: total, paused: paused, ended: ended, discTitle: title) }
    }

    func stop() {
        report(paused: true, ended: false)
        tearDownItem()
        player.pause()
        player.replaceCurrentItem(with: nil)
    }

    private func tearDownItem() {
        if let relay { HLSServer.shared.remove(relay) }
        relay = nil
        if let timeToken { player.removeTimeObserver(timeToken) }
        timeToken = nil
        if let endToken { NotificationCenter.default.removeObserver(endToken) }
        endToken = nil
        observations.forEach { $0.invalidate() }
        observations = []
    }
}

struct PlayerView: View {
    @Environment(AppModel.self) private var model
    let request: PlayRequest

    @State private var playback: PlaybackModel
    @State private var showControls = true
    @State private var lastInteraction = Date()
    @State private var menuOpen = false // a Transcode, Audio or Subtitles menu is showing
    @State private var aspectIndex = 0
    @State private var rotation: Double = 0
    @State private var zoom: CGFloat = 1
    @State private var pinchBase: CGFloat = 1
    @State private var pan: CGSize = .zero
    @State private var panBase: CGSize = .zero
    @State private var scrubbing: Double?
    @State private var showSpeed = false
    @State private var showDisc = false
    @State private var showTiming = false
    @State private var showSubtitleSearch = false

    init(request: PlayRequest) {
        self.request = request
        _playback = State(initialValue: PlaybackModel(request: request))
    }

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()
            GeometryReader { geo in
                let turned = rotation.truncatingRemainder(dividingBy: 180) != 0
                let size = turned ? CGSize(width: geo.size.height, height: geo.size.width) : geo.size
                VideoLayerView(player: playback.player, aspect: aspectValues[aspectIndex])
                    .frame(width: size.width, height: size.height)
                    .rotationEffect(.degrees(rotation))
                    .scaleEffect(zoom)
                    .offset(pan)
                    .position(x: geo.size.width / 2, y: geo.size.height / 2)
            }
            .ignoresSafeArea()
            .contentShape(Rectangle())
            .onTapGesture { showControls.toggle(); lastInteraction = Date() }
            .gesture(MagnificationGesture()
                .onChanged { zoom = max(1, min(pinchBase * $0, 5)) }
                .onEnded { _ in
                    pinchBase = zoom
                    if zoom == 1 { pan = .zero; panBase = .zero }
                })
            .simultaneousGesture(DragGesture(minimumDistance: 10)
                .onChanged { drag in
                    guard zoom > 1 else { return }
                    pan = CGSize(width: panBase.width + drag.translation.width, height: panBase.height + drag.translation.height)
                }
                .onEnded { _ in panBase = pan })

            if let subtitles = playback.subtitles {
                SubtitleOverlay(subtitles: subtitles, position: { Int64(playback.position * 1000) },
                                videoAspect: { playback.videoAspect }, bottomPadding: showControls ? 160 : 32)
            }

            if let failure = playback.failure {
                VStack(spacing: 8) {
                    Text("Can't play this video").font(.headline)
                    Text(failure).font(.footnote).multilineTextAlignment(.center).opacity(0.8)
                }
                .foregroundStyle(.white).padding(24)
                .offset(y: 140) // below play/pause and the frame steps
            } else if playback.isBuffering {
                ProgressView().tint(.white).scaleEffect(1.4)
            }

            if showControls { controls.transition(.opacity) }

            if showTiming, let subtitles = playback.subtitles {
                SubtitleTimingPanel(offsetMs: subtitles.offsetMs, onShift: { subtitles.shift($0); touch() },
                                    onClose: { showTiming = false })
                    .frame(maxHeight: .infinity, alignment: .top)
                    .padding(.top, 24).padding(.horizontal, 12)
            }

            if showSubtitleSearch, let subtitles = playback.subtitles {
                SubtitleSearchDialog(subtitles: subtitles, onDismiss: { showSubtitleSearch = false }) { index in
                    showSubtitleSearch = false
                    playback.chooseSubtitle(index)
                }
            }

            // Speed and the disc's choices: panels from the bottom in the player itself (as Android's
            // bottom sheets), not UIKit sheets, which left touches blocked after closing here.
            if showSpeed {
                PlayerPanel(onClose: { showSpeed = false }) { speedSheet }
            }
            if showDisc {
                PlayerPanel(onClose: { showDisc = false }) { discSheet }
            }

            if let skip = playback.skipPrompt {
                Button {
                    if skip.goesOn {
                        playback.report(paused: true, ended: true)
                        playback.playNext(model)
                    } else {
                        playback.seek(to: skip.to)
                    }
                } label: {
                    Text(skip.label).font(.subheadline.weight(.semibold))
                }
                .buttonStyle(PhoneButtonStyle())
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
                .padding(.trailing, 20)
                .padding(.bottom, showControls ? 150 : 40)
            }
        }
        .statusBarHidden(!showControls)
        .persistentSystemOverlays(showControls ? .automatic : .hidden)
        .animation(.easeInOut(duration: 0.2), value: showControls)
        .task {
            let model = self.model
            let playback = self.playback
            playback.onFinished = {
                if !playback.playNext(model) { model.playing = nil }
            }
            await playback.start(model: model)
        }
        .task(id: lastInteraction) {
            // A newer interaction cancels this wait: then the controls stay (the new task decides).
            do { try await Task.sleep(for: .seconds(max(1, lastInteraction.timeIntervalSinceNow + 5))) } catch { return }
            if playback.isPlaying && scrubbing == nil && !showDisc && !showSpeed && !menuOpen { showControls = false }
        }
        .onDisappear { playback.stop() }
    }

    private func touch() { lastInteraction = Date() }

    /// A menu is open: the controls (and the menu with them) stay up a while longer.
    private func holdControls() { lastInteraction = Date().addingTimeInterval(25) }

    /// A menu opened or closed: the controls stay up while it shows, and hide 5 s after it closes.
    private func menuShown(_ open: Bool) { menuOpen = open; touch() }

    private var controls: some View {
        ZStack {
            Color.black.opacity(0.4).ignoresSafeArea().allowsHitTesting(false)
            VStack(spacing: 0) {
                // Top: Back, the title (and the file under a show's or movie's name), Transcode, Rotate
                HStack(spacing: 4) {
                    PlayerIcon(system: "arrow.left", label: "Back") { model.playing = nil }
                    VStack(alignment: .leading, spacing: 2) {
                        Text(playback.title).font(.headline).lineLimit(2)
                        if playback.title != fileName {
                            Text(fileName).font(.caption).opacity(0.8).lineLimit(1)
                        }
                    }
                    .padding(.leading, 8)
                    Spacer(minLength: 4)
                    PickMenu(icon: "slider.horizontal.below.rectangle", tint: playback.isTranscoding ? .green : .white,
                             label: "Transcode", header: playback.status,
                             items: TranscodeMode.all.enumerated().map { i, mode in
                                 MenuItem(title: mode.name, checked: i == playback.modeIndex,
                                          disabled: playback.isDisc && mode.isOriginal) // discs can't play as they are
                             },
                             onPick: { i in playback.chooseMode(i); touch() }, onOpen: menuShown)
                        .equatable()
                    if playback.disc != nil {
                        // Disc: chapters, angles, tracks, titles
                        PlayerIcon(system: "opticaldisc", label: "Disc") { showDisc = true; holdControls() }
                    }
                    if playback.audioTracks.count > 1 { audioMenu }
                    if let subtitles = playback.subtitles { subtitleMenu(subtitles) }
                    PlayerIcon(system: "rotate.right", label: "Rotate") {
                        rotation = (rotation + 90).truncatingRemainder(dividingBy: 360)
                        touch()
                    }
                }
                .padding(16)
                Spacer()
                // Middle: −10 s, play/pause, +30 s; frame steps under them
                VStack(spacing: 16) {
                    HStack(spacing: 48) {
                        PlayerIcon(system: "gobackward.10", label: "-10s", size: 40) {
                            playback.seek(to: playback.position - 10); touch()
                        }
                        PlayerIcon(system: playback.isPlaying ? "pause.fill" : "play.fill", label: "Play/Pause", size: 56) {
                            playback.togglePlay(); touch()
                        }
                        PlayerIcon(system: "goforward.30", label: "+30s", size: 40) {
                            playback.seek(to: playback.position + 30); touch()
                        }
                    }
                    HStack(spacing: 64) {
                        PlayerIcon(system: "backward.end.fill", label: "Frame Back", size: 26) { playback.step(frames: -1); touch() }
                        PlayerIcon(system: "forward.end.fill", label: "Frame Forward", size: 26) { playback.step(frames: 1); touch() }
                    }
                }
                Spacer()
                // Bottom: trickplay, Aspect and Speed, the seek bar and times
                VStack(spacing: 0) {
                    if !playback.trickplay.isEmpty { trickplayRow }
                    HStack(spacing: 16) {
                        Spacer()
                        Button { aspectIndex = (aspectIndex + 1) % aspectNames.count; touch() } label: {
                            Label(aspectNames[aspectIndex], systemImage: "aspectratio").font(.caption.weight(.medium))
                        }
                        Button { showSpeed = true; touch() } label: {
                            Label(speedLabel(speeds[playback.speedIndex]), systemImage: "speedometer").font(.caption.weight(.medium))
                        }
                    }
                    .foregroundStyle(.white)
                    .padding(.bottom, 8)
                    Slider(value: Binding(get: { scrubbing ?? playback.position }, set: { scrubbing = $0; touch() }),
                           in: 0...max(playback.duration, 1)) { editing in
                        if !editing, let target = scrubbing {
                            playback.seek(to: target)
                            scrubbing = nil
                        }
                    }
                    .tint(.red)
                    HStack {
                        Text(longClock(scrubbing ?? playback.position))
                        Spacer()
                        Text(longClock(playback.duration))
                    }
                    .font(.caption2)
                }
                .padding(16)
            }
            .foregroundStyle(.white)
        }
    }

    /// Audio: the file's tracks (a check on the one playing).
    private var audioMenu: some View {
        let tracks = playback.audioTracks
        let labels = audioLabels(tracks)
        let indexes = tracks.enumerated().map { i, track in track.index ?? i }
        return PickMenu(icon: "waveform", tint: .white, label: "Audio", header: nil,
                        items: labels.enumerated().map { i, label in
                            MenuItem(title: label, checked: indexes[i] == playback.shownAudioTrack)
                        },
                        onPick: { i in playback.chooseAudio(indexes[i]); touch() }, onOpen: menuShown)
            .equatable()
    }

    /// Subtitles: Off and the tracks (a check on the one in use), then Timing and Search online.
    private func subtitleMenu(_ subtitles: ServerSubtitles) -> some View {
        let labels = subtitles.labels
        var items = (-1..<labels.count).map { i in
            MenuItem(title: i < 0 ? "Off" : labels[i], checked: i == subtitles.selected)
        }
        let timing = subtitles.selectedTrack?.isText == true ? items.count : -1
        if timing >= 0 {
            items.append(MenuItem(title: "Timing: \(subtitleOffsetText(subtitles.offsetMs))", systemImage: "clock", dividerBefore: true))
        }
        let search = items.count
        items.append(MenuItem(title: "Search online…", systemImage: "magnifyingglass", dividerBefore: timing < 0))
        return PickMenu(icon: "captions.bubble", tint: subtitles.selected >= 0 ? .green : .white, label: "Subtitles", header: nil,
                        items: items,
                        onPick: { i in
                            if i == search { showSubtitleSearch = true }
                            else if i == timing { showTiming = true }
                            else { playback.chooseSubtitle(i - 1) }
                            touch()
                        }, onOpen: menuShown)
            .equatable()
    }

    private var trickplayRow: some View {
        ScrollViewReader { proxy in
            ScrollView(.horizontal, showsIndicators: false) {
                LazyHStack(spacing: 4) {
                    ForEach(playback.trickplay, id: \.time) { frame in
                        Button { playback.seek(to: Double(frame.time)); touch() } label: {
                            ZStack(alignment: .bottomTrailing) {
                                Color.black
                                RemoteImage(relative: frame.url)
                                Text(longClock(Double(frame.time))).font(.caption2).foregroundStyle(.white)
                                    .padding(.horizontal, 2).background(.black.opacity(0.6))
                            }
                            .frame(width: 100, height: 72)
                            .clipShape(RoundedRectangle(cornerRadius: 4))
                        }
                        .buttonStyle(.plain)
                        .id(frame.time)
                    }
                }
                .padding(.horizontal, 8)
            }
            .frame(height: 72)
            .padding(.bottom, 8)
            .onChange(of: Int(playback.position) / 10) {
                guard Date().timeIntervalSince(lastInteraction) > 8,
                      let frame = playback.trickplay.first(where: { Double($0.time) >= playback.position }) else { return }
                withAnimation { proxy.scrollTo(frame.time, anchor: .leading) }
            }
        }
    }

    /// A DVD's chapters, angles, audio and subtitle tracks, and other titles; picking one
    /// restarts the stream there (Android's PhoneDiscSheet).
    @ViewBuilder
    private var discSheet: some View {
        if let disc = playback.disc, let title = disc.title(playback.discTitle) {
            let chapters = title.chapters ?? []
            let current = max(0, chapters.lastIndex { playback.position + 0.5 >= $0 } ?? 0)
            List {
                if chapters.count > 1 {
                    Section("Chapters") {
                        ForEach(Array(chapters.enumerated()), id: \.offset) { i, start in
                            discChoice("\(i + 1)   \(discClock(start))", selected: i == current) { playback.seek(to: start) }
                        }
                    }
                }
                if let angles = title.angles, angles > 1 {
                    Section("Angle") {
                        ForEach(1...angles, id: \.self) { a in
                            discChoice("Angle \(a) of \(angles)", selected: a == playback.discAngle) { playback.chooseDiscAngle(a) }
                        }
                    }
                }
                if let audio = title.audio, audio.count > 1 {
                    Section("Audio") {
                        ForEach(audio, id: \.index) { t in
                            discChoice("\(t.index + 1)   \(t.label)", selected: t.index == playback.discAudio) { playback.chooseDiscAudio(t.index) }
                        }
                    }
                }
                Section("Subtitles") {
                    discChoice("Off", selected: playback.discSubtitle < 0) { playback.chooseDiscSubtitle(-1) }
                    ForEach(title.subtitles ?? [], id: \.index) { t in
                        discChoice("\(t.index + 1)   \(t.label)", selected: t.index == playback.discSubtitle) { playback.chooseDiscSubtitle(t.index) }
                    }
                }
                if let titles = disc.titles, titles.count > 1 {
                    Section("Titles") {
                        ForEach(titles.sorted { $0.number < $1.number }, id: \.number) { t in
                            let main = t.number == disc.mainTitle ? "  (main)" : ""
                            discChoice("Title \(t.number)   \(discClock(t.duration))\(main)", selected: t.number == title.number) {
                                playback.chooseDiscTitle(t.number)
                            }
                        }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .background(Palette.panel)
            .foregroundStyle(Palette.text)

        }
    }

    private func discChoice(_ label: String, selected: Bool, action: @escaping () -> Void) -> some View {
        Button {
            showDisc = false
            action()
            touch()
        } label: {
            HStack(spacing: 12) {
                Image(systemName: selected ? "largecircle.fill.circle" : "circle")
                    .foregroundStyle(selected ? Palette.accent : Palette.muted)
                Text(label).monospacedDigit()
                Spacer()
            }
        }
        .listRowBackground(Palette.panel)
    }

    private func discClock(_ seconds: Double) -> String {
        let s = Int(seconds)
        return s >= 3600 ? String(format: "%d:%02d:%02d", s / 3600, s % 3600 / 60, s % 60) : String(format: "%d:%02d", s / 60, s % 60)
    }

    private var speedSheet: some View {
        List {
            ForEach(Array(speeds.enumerated()), id: \.offset) { i, speed in
                Button {
                    playback.chooseSpeed(i)
                    showSpeed = false
                } label: {
                    HStack {
                        Text(speedLabel(speed))
                        Spacer()
                        if i == playback.speedIndex { Image(systemName: "checkmark").foregroundStyle(Palette.accent) }
                    }
                }
                .listRowBackground(Palette.panel)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Palette.panel)
        .foregroundStyle(Palette.text)

    }

    private var fileName: String { (request.path as NSString).lastPathComponent }

    private func speedLabel(_ speed: Float) -> String {
        speed == speed.rounded() ? "\(Int(speed)).0x" : "\(speed)x"
    }

    /// "0:12:34", as Android's player shows times.
    private func longClock(_ seconds: Double) -> String {
        let s = max(0, Int(seconds.isFinite ? seconds : 0))
        return String(format: "%d:%02d:%02d", s / 3600, (s % 3600) / 60, s % 60)
    }
}

/// A white icon button on the video.
/// One line of a PickMenu.
struct MenuItem: Equatable {
    var title: String
    var checked = false
    var disabled = false
    var systemImage: String? = nil
    var dividerBefore = false
}

/// A player menu (Transcode, Audio, Subtitles). It tells the player when its list opens and
/// closes, so the controls (and the menu with them) don't hide while it's open. Equal by its
/// contents, so the player compares it with `.equatable()` and position updates leave it alone;
/// its actions only reach the model and the player's @State.
struct PickMenu: View, Equatable {
    let icon: String
    let tint: Color
    let label: String
    let header: String?
    let items: [MenuItem]
    let onPick: (Int) -> Void
    let onOpen: (Bool) -> Void // the menu's list opened (true) or closed (false)

    static func == (a: PickMenu, b: PickMenu) -> Bool {
        a.icon == b.icon && a.tint == b.tint && a.label == b.label && a.header == b.header && a.items == b.items
    }

    var body: some View {
        Menu {
            Group {
                if let header { Section(header) {} }
                ForEach(items.indices, id: \.self) { i in
                    let item = items[i]
                    if item.dividerBefore { Divider() }
                    Button { onPick(i) } label: {
                        if item.checked { Label(item.title, systemImage: "checkmark") }
                        else if let image = item.systemImage { Label(item.title, systemImage: image) }
                        else { Text(item.title) }
                    }
                    .disabled(item.disabled)
                }
            }
            // A tap gesture on a Menu never fires (the menu takes the touch), but its list
            // appears and disappears with the menu.
            .onAppear { onOpen(true) }
            .onDisappear { onOpen(false) }
        } label: {
            Image(systemName: icon).font(.system(size: 22))
                .foregroundStyle(tint)
                .frame(width: 48, height: 48).contentShape(Rectangle())
        }
        .accessibilityLabel(label)
    }
}

struct PlayerIcon: View {
    let system: String
    let label: String
    var size: CGFloat = 22
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: system).font(.system(size: size)).foregroundStyle(.white)
                .frame(width: max(48, size + 8), height: max(48, size + 8))
                .contentShape(Rectangle()) // the whole square, not only the symbol's strokes (pause's gap, the disc's hole)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }
}

/// The video's layer, fit to the view or stretched to a chosen aspect ratio, with picture in
/// picture starting by itself when you go home.
struct VideoLayerView: UIViewRepresentable {
    let player: AVPlayer
    let aspect: CGFloat?

    func makeUIView(context: Context) -> PlayerLayerView {
        let view = PlayerLayerView()
        view.playerLayer.player = player
        view.backgroundColor = .black
        if AVPictureInPictureController.isPictureInPictureSupported(),
           let pip = AVPictureInPictureController(playerLayer: view.playerLayer) {
            pip.canStartPictureInPictureAutomaticallyFromInline = true
            context.coordinator.pip = pip
        }
        return view
    }

    func updateUIView(_ view: PlayerLayerView, context: Context) {
        if view.playerLayer.player !== player { view.playerLayer.player = player }
        view.aspect = aspect
        view.setNeedsLayout()
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    final class Coordinator { var pip: AVPictureInPictureController? }
}

final class PlayerLayerView: UIView {
    let playerLayer = AVPlayerLayer()
    var aspect: CGFloat?

    override init(frame: CGRect) {
        super.init(frame: frame)
        layer.addSublayer(playerLayer)
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    override func layoutSubviews() {
        super.layoutSubviews()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        if let aspect {
            // A chosen aspect ratio: the picture is stretched to it, as big as fits.
            playerLayer.videoGravity = .resize
            let w = min(bounds.width, bounds.height * aspect)
            let h = w / aspect
            playerLayer.frame = CGRect(x: (bounds.width - w) / 2, y: (bounds.height - h) / 2, width: w, height: h)
        } else {
            playerLayer.videoGravity = .resizeAspect
            playerLayer.frame = bounds
        }
        CATransaction.commit()
    }
}

/// A panel over the bottom of the player (up to two thirds of it) on a dimmed backdrop; a tap on
/// the backdrop or a swipe down on the panel closes it.
struct PlayerPanel<Content: View>: View {
    let onClose: () -> Void
    @ViewBuilder let content: Content
    @State private var drag: CGFloat = 0

    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .bottom) {
                Color.black.opacity(0.4).ignoresSafeArea()
                    .contentShape(Rectangle())
                    .onTapGesture(perform: onClose)
                VStack(spacing: 0) {
                    Capsule().fill(Palette.muted.opacity(0.6)).frame(width: 40, height: 5).padding(.vertical, 8)
                        .frame(maxWidth: .infinity)
                        .contentShape(Rectangle())
                        .gesture(DragGesture()
                            .onChanged { drag = max(0, $0.translation.height) }
                            .onEnded { if $0.translation.height > 80 { onClose() }; drag = 0 })
                    content
                }
                .frame(maxHeight: geo.size.height * 0.66)
                .background(Palette.panel, in: UnevenRoundedRectangle(topLeadingRadius: 20, topTrailingRadius: 20))
                .background(Palette.background, in: UnevenRoundedRectangle(topLeadingRadius: 20, topTrailingRadius: 20))
                .offset(y: drag)
            }
            .ignoresSafeArea(edges: .bottom)
        }
        .transition(.move(edge: .bottom))
    }
}
