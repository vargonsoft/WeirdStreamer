import SwiftUI

/// Material's folder icon (the one Android and the web draw), stretched to its frame: a tab at the
/// top left, and the body at x 2-22, y 6-20 of its 24 units.
struct FolderShape: Shape {
    func path(in r: CGRect) -> Path {
        let sx = r.width / 24, sy = r.height / 24
        func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: r.minX + x * sx, y: r.minY + y * sy) }
        var path = Path()
        path.move(to: p(10, 4))
        path.addLine(to: p(4, 4))
        path.addQuadCurve(to: p(2, 6), control: p(2, 4))
        path.addLine(to: p(2, 18))
        path.addQuadCurve(to: p(4, 20), control: p(2, 20))
        path.addLine(to: p(20, 20))
        path.addQuadCurve(to: p(22, 18), control: p(22, 20))
        path.addLine(to: p(22, 8))
        path.addQuadCurve(to: p(20, 6), control: p(22, 6))
        path.addLine(to: p(12, 6))
        path.closeSubpath()
        return path
    }
}

/// A folder's icon, square and as large as its frame allows, with `url` (the server's pick: the
/// folder's thumbnail.png, else a photo or video from inside it) in its front; nil draws the plain
/// folder at the same size. As Android's FolderPreview and the web's folder cards.
struct FolderPreview: View {
    let url: String?

    var body: some View {
        GeometryReader { g in
            ZStack(alignment: .topLeading) {
                FolderShape().fill(Palette.accent)
                if let url {
                    RemoteImage(relative: url)
                        .frame(width: g.size.width * 0.72, height: g.size.height * 0.44)
                        .clipShape(RoundedRectangle(cornerRadius: min(g.size.width, g.size.height) * 0.04))
                        .offset(x: g.size.width * 0.14, y: g.size.height * 0.34)
                }
            }
        }
        .aspectRatio(1, contentMode: .fit)
    }
}

/// Sprocket strips down the left and right edges of a video's thumbnail, telling a folder's
/// videos from its photos (Android's Modifier.filmstrip).
struct Filmstrip: View {
    var body: some View {
        Canvas { ctx, size in
            let strip = min(max(size.width * 0.06, 4), 14)
            let holeWidth = strip * 0.45, holeHeight = strip * 0.5, pitch = holeHeight * 2
            for x in [0, size.width - strip] {
                ctx.fill(Path(CGRect(x: x, y: 0, width: strip, height: size.height)),
                         with: .color(Color(red: 0.067, green: 0.067, blue: 0.067)))
                var y = pitch * 0.3
                while y + holeHeight <= size.height {
                    let hole = CGRect(x: x + (strip - holeWidth) / 2, y: y, width: holeWidth, height: holeHeight)
                    ctx.fill(Path(roundedRect: hole, cornerRadius: holeWidth * 0.2),
                             with: .color(Color(red: 0.94, green: 0.925, blue: 0.894, opacity: 0.85)))
                    y += pitch
                }
            }
        }
        .allowsHitTesting(false)
    }
}
