import SwiftUI
import AVFoundation
import PDFKit

// Photos and PDFs, as Android's PhoneImageViewer and PhonePdfViewer: on black, with a bar (Back
// and the name) that a tap hides. Photos swipe through the folder's photos, pinch to zoom and pan,
// and a motion photo plays its clip in a loop (Samsung's and Google's inside the photo, an Apple
// Live Photo's .MOV beside it), muted, with three seconds of the still between; a tap pauses it.

struct ImageViewer: View {
    @Environment(AppModel.self) private var model
    let start: FileEntry

    @State private var index = 0
    @State private var immersive = false
    @State private var zoomed = false

    private var images: [FileEntry] {
        let list = model.filteredEntries.filter(\.isImage)
        return list.isEmpty ? [start] : list
    }

    var body: some View {
        let images = self.images
        ZStack(alignment: .top) {
            Color.black.ignoresSafeArea()
            TabView(selection: $index) {
                ForEach(Array(images.enumerated()), id: \.element.path) { i, entry in
                    ZoomablePhoto(entry: entry, active: i == index, zoomed: i == index ? $zoomed : .constant(false)) {
                        immersive.toggle()
                    }
                    .tag(i)
                }
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .ignoresSafeArea()
            .onChange(of: index) { zoomed = false }

            if !immersive {
                HStack(spacing: 4) {
                    PlayerIcon(system: "arrow.left", label: "Back") { model.viewingImage = nil }
                    Text(images.indices.contains(index) ? images[index].name : start.name)
                        .font(.headline).foregroundStyle(.white).lineLimit(1)
                    Spacer()
                }
                .padding(.horizontal, 4)
                .frame(height: 56)
                .background(.black.opacity(0.5))
            }
        }
        .statusBarHidden(immersive)
        .persistentSystemOverlays(immersive ? .hidden : .automatic)
        .onAppear { index = images.firstIndex(where: { $0.path == start.path }) ?? 0 }
        .gesture(DragGesture(minimumDistance: 30).onEnded { drag in
            if !zoomed && drag.startLocation.x < 28 && drag.translation.width > 80 { model.viewingImage = nil }
        })
    }
}

/// One photo: loaded with the account's token, pinch to zoom (pan while zoomed), and its motion
/// clip looping when it has one.
struct ZoomablePhoto: View {
    @Environment(AppModel.self) private var model
    let entry: FileEntry
    let active: Bool
    @Binding var zoomed: Bool
    let onTap: () -> Void

    @State private var image: UIImage?
    @State private var failed = false
    @State private var scale: CGFloat = 1
    @State private var scaleBase: CGFloat = 1
    @State private var offset: CGSize = .zero
    @State private var offsetBase: CGSize = .zero
    @State private var clip: AVPlayer?
    @State private var showClip = false
    @State private var paused = false
    @State private var showPausedIcon = false
    @State private var clipFile: URL?

    var body: some View {
        ZStack {
            if let image {
                Image(uiImage: image).resizable().scaledToFit()
            } else if failed {
                Text("Can't show this photo").foregroundStyle(.white.opacity(0.7))
            } else {
                ProgressView().tint(.white)
            }
            if let clip {
                ClipLayer(player: clip).opacity(showClip ? 1 : 0)
            }
            if showPausedIcon {
                Image(systemName: "pause.fill").font(.system(size: 56)).foregroundStyle(.white.opacity(0.5))
            }
        }
        .scaleEffect(scale)
        .offset(offset)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .contentShape(Rectangle())
        .onTapGesture {
            if clip != nil {
                paused.toggle()
                if paused { clip?.pause() }
            }
            onTap()
        }
        .gesture(MagnificationGesture()
            .onChanged { scale = max(1, min(scaleBase * $0, 6)) }
            .onEnded { _ in
                scaleBase = scale
                if scale <= 1.01 { scale = 1; scaleBase = 1; offset = .zero; offsetBase = .zero }
                zoomed = scale > 1
            })
        .simultaneousGesture(zoomed ? DragGesture()
            .onChanged { offset = CGSize(width: offsetBase.width + $0.translation.width, height: offsetBase.height + $0.translation.height) }
            .onEnded { _ in offsetBase = offset } : nil)
        .task(id: entry.path) { await load() }
        .task(id: "\(active)-\(paused)-\(clip != nil)") { await loop() }
        .task(id: paused) {
            guard clip != nil else { return }
            showPausedIcon = paused
            if paused {
                try? await Task.sleep(for: .seconds(2))
                showPausedIcon = false
            }
        }
        .onDisappear {
            clip?.pause()
            if let clipFile { try? FileManager.default.removeItem(at: clipFile) }
        }
    }

    private func load() async {
        guard let client = model.client else { return }
        guard let data = try? await client.image(at: client.streamURL(for: entry.path)) else { failed = true; return }
        let embedded = entry.liveVideo == nil ? embeddedVideo(in: data) : nil
        // ImageIO can refuse a JPEG with a clip after it: then just the picture part.
        image = UIImage(data: data) ?? embedded.flatMap { UIImage(data: data[..<pictureEnd(data, before: $0.lowerBound)]) }
        failed = image == nil
        if let live = entry.liveVideo {
            // An Apple Live Photo: its .MOV, streamed.
            clip = muted(AVPlayer(playerItem: AVPlayerItem(asset: OriginalLoader.asset(for: client.streamURL(for: live),
                                                                                        token: client.token))))
        } else if let range = embedded {
            // A Samsung or Google motion photo: the MP4 after the picture.
            let file = FileManager.default.temporaryDirectory.appendingPathComponent("weirdstreamer-motion-\(UUID().uuidString).mp4")
            if (try? data[range].write(to: file)) != nil {
                clipFile = file
                clip = muted(AVPlayer(url: file))
            }
        }
    }

    private func muted(_ player: AVPlayer) -> AVPlayer {
        player.isMuted = true
        return player
    }

    /// Plays the clip, shows the still for three seconds, and again, while the photo is on screen.
    private func loop() async {
        guard let clip, active, !paused else {
            clip?.pause()
            return
        }
        while !Task.isCancelled {
            await clip.seek(to: .zero)
            showClip = true
            clip.play()
            let length = (try? await clip.currentItem?.asset.load(.duration))?.seconds ?? 3
            try? await Task.sleep(for: .seconds(max(1, length.isFinite ? length : 3)))
            guard !Task.isCancelled else { return }
            showClip = false
            try? await Task.sleep(for: .seconds(3))
        }
    }
}

/// Where a motion photo's MP4 starts and ends in the file: the last "ftyp" box after the JPEG,
/// as Samsung (after "MotionPhoto_Data") and Google (MicroVideo) append it.
func embeddedVideo(in data: Data) -> Range<Int>? {
    let bytes = [UInt8](data)
    guard bytes.count > 1024 else { return nil }
    let ftyp: [UInt8] = Array("ftyp".utf8)
    var i = bytes.count - 8
    while i > 4 {
        if bytes[i] == ftyp[0] && bytes[i + 1] == ftyp[1] && bytes[i + 2] == ftyp[2] && bytes[i + 3] == ftyp[3] {
            let size = Int(bytes[i - 4]) << 24 | Int(bytes[i - 3]) << 16 | Int(bytes[i - 2]) << 8 | Int(bytes[i - 1])
            if size >= 8 && size < 256 { return (i - 4)..<bytes.count }
        }
        i -= 1
    }
    return nil
}

/// Where the picture ends: before Samsung's "MotionPhoto_Data" marker if it's there, else at the clip.
func pictureEnd(_ data: Data, before clip: Int) -> Int {
    let marker = Data("MotionPhoto_Data".utf8)
    if let found = data.range(of: marker, options: .backwards, in: 0..<clip) { return found.lowerBound }
    return clip
}

/// A muted clip's picture over the photo.
private struct ClipLayer: UIViewRepresentable {
    let player: AVPlayer
    func makeUIView(context: Context) -> PlayerLayerView {
        let view = PlayerLayerView()
        view.playerLayer.player = player
        view.backgroundColor = .clear
        return view
    }
    func updateUIView(_ view: PlayerLayerView, context: Context) { view.playerLayer.player = player }
}

/// A PDF: its pages to swipe through and zoom (PDFKit), Previous / page / Next at the bottom, and a
/// grid of its pages to jump to (the bar's grid button).
struct PdfViewer: View {
    @Environment(AppModel.self) private var model
    let entry: FileEntry

    @State private var document: PDFDocument?
    @State private var page = 0
    @State private var thumbnails = false
    @State private var immersive = false
    @State private var failed = false

    var body: some View {
        VStack(spacing: 0) {
            if !immersive {
                HStack(spacing: 4) {
                    BarButton(system: "arrow.left", label: "Back") {
                        if thumbnails { thumbnails = false } else { model.viewingPdf = nil }
                    }
                    Text(entry.name).font(.headline).lineLimit(1)
                    Spacer()
                    BarButton(system: "square.grid.2x2", label: "Thumbnails") { thumbnails.toggle() }
                }
                .frame(height: 56)
                .background(Palette.panel)
            }
            ZStack {
                if let document {
                    if thumbnails {
                        PdfThumbnails(document: document) { i in
                            page = i
                            thumbnails = false
                        }
                    } else {
                        PdfPages(document: document, page: $page)
                            .onTapGesture { immersive.toggle() }
                    }
                } else if failed {
                    Text("Can't open this PDF").foregroundStyle(Palette.muted)
                } else {
                    ProgressView()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            if let document, !thumbnails, document.pageCount > 0 {
                HStack {
                    Button("Previous") { page = max(0, page - 1) }.disabled(page == 0)
                    Spacer()
                    Text("\(page + 1) / \(document.pageCount)")
                    Spacer()
                    Button("Next") { page = min(document.pageCount - 1, page + 1) }.disabled(page >= document.pageCount - 1)
                }
                .padding(.horizontal, 16)
                .frame(height: 56)
                .background(Palette.panel)
            }
        }
        .background(ThemeBackground())
        .themed()
        .statusBarHidden(immersive)
        .task {
            guard let client = model.client,
                  let data = try? await client.image(at: client.streamURL(for: entry.path)) else { failed = true; return }
            document = PDFDocument(data: data)
            failed = document == nil
        }
    }
}

/// PDFKit's view, one page at a time, swiped sideways; pinch zooms.
private struct PdfPages: UIViewRepresentable {
    let document: PDFDocument
    @Binding var page: Int

    func makeUIView(context: Context) -> PDFView {
        let view = PDFView()
        view.document = document
        view.displayMode = .singlePage
        view.displayDirection = .horizontal
        view.usePageViewController(true)
        view.autoScales = true
        view.backgroundColor = .black
        NotificationCenter.default.addObserver(context.coordinator, selector: #selector(Coordinator.changed(_:)),
                                               name: .PDFViewPageChanged, object: view)
        return view
    }

    func updateUIView(_ view: PDFView, context: Context) {
        if let target = document.page(at: page), view.currentPage != target { view.go(to: target) }
    }

    func makeCoordinator() -> Coordinator { Coordinator(page: $page) }

    final class Coordinator: NSObject {
        var page: Binding<Int>
        init(page: Binding<Int>) { self.page = page }
        @objc func changed(_ note: Notification) {
            guard let view = note.object as? PDFView, let current = view.currentPage,
                  let doc = view.document else { return }
            let i = doc.index(for: current)
            if page.wrappedValue != i { page.wrappedValue = i }
        }
    }
}

/// Every page as a small picture, to jump to.
private struct PdfThumbnails: View {
    let document: PDFDocument
    let onPick: (Int) -> Void

    var body: some View {
        ScrollView {
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 100), spacing: 8)], spacing: 8) {
                ForEach(0..<document.pageCount, id: \.self) { i in
                    Button { onPick(i) } label: {
                        ZStack {
                            Palette.raised
                            if let page = document.page(at: i) {
                                Image(uiImage: page.thumbnail(of: CGSize(width: 200, height: 280), for: .mediaBox))
                                    .resizable().scaledToFit()
                            }
                            Text("\(i + 1)").font(.caption.weight(.semibold)).padding(4)
                                .background(.black.opacity(0.5), in: RoundedRectangle(cornerRadius: 4))
                                .foregroundStyle(.white)
                                .frame(maxHeight: .infinity, alignment: .bottom)
                                .padding(4)
                        }
                        .aspectRatio(0.7, contentMode: .fit)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(8)
        }
    }
}
