import SwiftUI
import CoreGraphics
import AssRender

/// libass (iOS/native/assrender.c, built by iOS/native/build-libass.sh) on its own: a script, the
/// video's fonts and a size, drawn a moment at a time into a picture. An actor, so the drawing is
/// off the main thread and one call at a time.
actor AssRenderer {
    private let handle: OpaquePointer
    private var pixels: [UInt8] = []
    private var width = 0
    private var height = 0
    private var version = -1

    enum Drawn: Sendable {
        case unchanged
        case image(CGImage?) // nil: nothing on screen
    }

    init?() {
        guard let handle = assr_create() else { return nil }
        self.handle = handle
    }

    deinit { assr_destroy(handle) }

    func addFont(_ name: String, _ data: Data) {
        data.withUnsafeBytes { assr_add_font(handle, name, $0.baseAddress, $0.count) }
    }

    /// Draws the script at `timeMs` on a width × height picture (pixels) of a video of `aspect`.
    /// `script` (with its `version`) replaces the one libass has when it's newer.
    func draw(script: String?, version: Int, width: Int, height: Int, aspect: CGFloat, timeMs: Int64) -> Drawn {
        guard width > 0, height > 0 else { return .unchanged }
        var force = false
        if let script, version != self.version {
            let bytes = Array(script.utf8)
            let ok = bytes.withUnsafeBytes { assr_set_script(handle, $0.baseAddress?.assumingMemoryBound(to: CChar.self), $0.count) }
            guard ok != 0 else { return .unchanged }
            self.version = version
            force = true
        }
        if width != self.width || height != self.height {
            self.width = width
            self.height = height
            pixels = [UInt8](repeating: 0, count: width * height * 4)
            assr_set_size(handle, Int32(width), Int32(height), Int32((1080 * aspect).rounded()), 1080)
            force = true
        }
        let result = pixels.withUnsafeMutableBufferPointer {
            assr_render(handle, timeMs, $0.baseAddress, Int32(width), Int32(height), Int32(width * 4), force ? 1 : 0)
        }
        guard result == 1 else { return .unchanged }
        guard pixels.contains(where: { $0 != 0 }) else { return .image(nil) }
        // RGBA, premultiplied, copied for the picture (the buffer is drawn into again).
        guard let provider = CGDataProvider(data: Data(pixels) as CFData),
              let image = CGImage(width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: width * 4,
                                  space: CGColorSpaceCreateDeviceRGB(),
                                  bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
                                  provider: provider, decode: nil, shouldInterpolate: true, intent: .defaultIntent)
        else { return .unchanged }
        return .image(image)
    }
}

/// An ASS/SSA track drawn by libass, styled, positioned and animated, over the picture (the part
/// of this view the video's shape fills). Put it in the video's own frame, under its zoom and
/// rotation, to stay on the picture. `position` is the file's time (ms). About 30 times a second
/// libass is asked for that moment, less the track's timing correction; the picture is replaced
/// only when the subtitles changed, also while paused.
struct AssSubtitleView: View {
    let subtitles: ServerSubtitles
    let position: () -> Int64
    let videoAspect: () -> CGFloat

    @Environment(\.displayScale) private var displayScale
    @State private var area: CGSize = .zero
    @State private var image: CGImage?
    @State private var shownIn: CGRect = .zero

    private struct Run: Equatable {
        let selected: Int
        let area: CGSize
    }

    var body: some View {
        GeometryReader { geo in
            ZStack {
                if let image {
                    Image(decorative: image, scale: displayScale)
                        .resizable()
                        .frame(width: shownIn.width, height: shownIn.height)
                        .position(x: shownIn.midX, y: shownIn.midY)
                }
            }
            .frame(width: geo.size.width, height: geo.size.height)
            .onAppear { area = geo.size }
            .onChange(of: geo.size) { _, now in area = now }
        }
        .allowsHitTesting(false)
        .task(id: Run(selected: subtitles.selected, area: area)) { await run() }
    }

    private func run() async {
        image = nil
        guard subtitles.styled, area.width > 0, area.height > 0, let renderer = AssRenderer() else { return }
        for (name, data) in await subtitles.videoFonts() {
            await renderer.addFont(name, data)
        }
        var version = -1
        while !Task.isCancelled {
            let aspect = videoAspect() > 0 ? videoAspect() : 16.0 / 9
            let w = min(area.width, area.height * aspect), h = w / aspect
            let rect = CGRect(x: (area.width - w) / 2, y: (area.height - h) / 2, width: w, height: h)
            let v = subtitles.assVersion
            let script = v != version ? subtitles.assScript() : nil
            version = v
            let drawn = await renderer.draw(script: script, version: v,
                                            width: Int((w * displayScale).rounded()), height: Int((h * displayScale).rounded()),
                                            aspect: aspect, timeMs: position() - Int64(subtitles.offsetMs))
            guard !Task.isCancelled else { break }
            if case .image(let picture) = drawn {
                image = picture
                shownIn = rect
            }
            try? await Task.sleep(for: .milliseconds(33))
        }
    }
}
