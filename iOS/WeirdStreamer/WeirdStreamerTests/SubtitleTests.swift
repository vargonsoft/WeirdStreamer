import CoreGraphics
import Foundation
import Testing
@testable import WeirdStreamer

@MainActor
struct SubtitleTests {

    // MARK: Disc subtitles

    @Test func discForcedLines() throws {
        let title = try JSONDecoder().decode(DiscTitle.self, from: Data("""
        {"number":1,"duration":6000,"audio":[{"index":0,"language":"eng","codec":"ac3"},{"index":1,"language":"fre","codec":"ac3"}],
         "subtitles":[{"index":0,"language":"fre","codec":"dvd_subtitle"},{"index":1,"language":"eng","codec":"dvd_subtitle"}]}
        """.utf8))
        let subs = ServerSubtitles(client: nil, path: "01c6653f/Film/Film.iso")
        subs.useDisc(title, angle: 1, from: 3000, audio: 0)
        // None chosen: the forced lines of the track in the audio's language.
        #expect(subs.tracks.count == 2 && subs.selected == -1)
        #expect(subs.forcedTrack?.index == 1)
        subs.useDisc(title, angle: 1, from: 3000, audio: 1)
        #expect(subs.forcedTrack?.index == 0)
        // A chosen track shows whole: no forced lines then, and a new stream keeps the choice.
        subs.select(1)
        #expect(subs.forcedTrack == nil)
        subs.useDisc(title, angle: 1, from: 4500, audio: 1)
        #expect(subs.selected == 1)
    }

    // MARK: WebVTT

    @Test func webVttCues() throws {
        let cues = parseWebVtt("""
        WEBVTT\r
        \r
        1\r
        00:00:01.500 --> 00:00:03.000 position:10%\r
        <i>Hello</i> &amp; <b>bye</b>\r
        \r
        01:02.000 --> 01:04.250\r
        Line one\r
        Line two\r
        \r
        00:00:09.000 --> 00:00:10.000\r
        \r
        """)
        #expect(cues.count == 2) // the empty cue is left out
        #expect(cues[0].startMs == 1500 && cues[0].endMs == 3000)
        #expect(cues[0].plain == "Hello & bye")
        let italic = cues[0].text.runs.filter { $0.inlinePresentationIntent == .emphasized }
        #expect(italic.map { String(cues[0].text[$0.range].characters) } == ["Hello"])
        #expect(cues[1].startMs == 62_000 && cues[1].endMs == 64_250)
        #expect(cues[1].plain == "Line one\nLine two")
    }

    // MARK: Labels

    private func tracks(_ json: String) throws -> [SubtitleTrack] {
        try ServerClient.decode([SubtitleTrack].self, from: Data(json.utf8))
    }

    @Test func trackLabels() throws {
        let labels = serverSubtitleLabels(try tracks("""
        [{"index":0,"codec":"subrip","language":"eng","text":true},
         {"index":1,"codec":"subrip","language":"eng","text":true},
         {"index":2,"codec":"hdmv_pgs_subtitle","language":"eng","forced":true},
         {"index":3,"codec":"subrip","language":"eng","title":"English SDH","hearing_impaired":true,"text":true},
         {"index":4,"codec":"subrip","language":"spa","text":true,"external":"file"},
         {"index":5,"codec":"subrip","text":true,"external":"opensubtitles","hearing_impaired":true}]
        """))
        #expect(labels == [
            "English (1)", "English (2)",          // the same twice: numbered
            "English · Forced · Image",
            "English · English SDH",               // SDH already in the title
            "Spanish · File",
            "Subtitle 6 · SDH · Downloaded",
        ])
    }

    @Test func audioTrackLabels() throws {
        let audio = try ServerClient.decode([AudioTrackInfo].self, from: Data("""
        [{"index":0,"codec":"eac3","channels":6,"language":"eng","title":"Commentary","default":true},
         {"index":1,"codec":"aac","channels":2,"language":"und"},
         {"index":2,"codec":"truehd","channels":8,"language":"fre","title":"French"}]
        """.utf8))
        #expect(audioLabels(audio) == [
            "English · Commentary · E-AC-3 · 5.1 (default)",
            "Track 2 · AAC · stereo",
            "French · TrueHD · 7.1",
        ])
    }

    @Test func timingText() {
        #expect(subtitleOffsetText(0) == "In sync")
        #expect(subtitleOffsetText(-250) == "250 ms earlier")
        #expect(subtitleOffsetText(1500) == "1.5 s later")
        #expect(subtitleOffsetText(2000) == "2 s later")
        #expect(subtitleOffsetText(-1250) == "1.25 s earlier")
    }

    // MARK: CJK fonts

    @Test func cjkCharacters() {
        #expect(isCjkScalar(0x5B57))   // 字
        #expect(isCjkScalar(0xD55C))   // 한
        #expect(isCjkScalar(0x30A2))   // ア
        #expect(!isCjkScalar(0x41))    // A
        #expect(!isCjkScalar(0xE9))    // é
        let found = newCjkCharacters(in: "漢a字漢、", seen: [0x5B57])
        #expect(found.text == "、漢")  // sorted by code point, 字 already seen
        #expect(found.scalars == [0x3001, 0x6F22])
        #expect(newCjkCharacters(in: "plain text", seen: []).text.isEmpty)
    }

    // MARK: PGS

    /// One PGS segment: "PG", the presentation time (90 kHz), a decode time, type, size, payload.
    private func segment(_ type: UInt8, ms: UInt32 = 0, _ payload: [UInt8]) -> [UInt8] {
        let pts = ms * 90
        return [0x50, 0x47, UInt8(pts >> 24), UInt8(pts >> 16 & 0xFF), UInt8(pts >> 8 & 0xFF), UInt8(pts & 0xFF),
                0, 0, 0, 0, type, UInt8(payload.count >> 8), UInt8(payload.count & 0xFF)] + payload
    }

    /// A 4×2 white picture at (100, 50) on a 1920×1080 frame from 1 s, cleared at 2.5 s.
    private var supStream: Data {
        // Line 1 as single pixels of color 1, line 2 as a run of 4 (0, 0x80|4, color 1); 0, 0 ends a line.
        let rle: [UInt8] = [1, 1, 1, 1, 0, 0, 0, 0x84, 1, 0, 0]
        let palette = segment(0x14, ms: 1000, [0, 0, 1, 235, 128, 128, 255])
        let object = segment(0x15, ms: 1000, [0, 0, 0, 0xC0, 0, 0, UInt8(rle.count + 4), 0, 4, 0, 2] + rle)
        let shown = segment(0x16, ms: 1000, [0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, 0x80, 0, 0, 1,
                                             0, 0, 0, 0, 0, 100, 0, 50])
        let cleared = segment(0x16, ms: 2500, [0x07, 0x80, 0x04, 0x38, 0x10, 0, 2, 0, 0, 0, 0])
        let end1 = segment(0x80, ms: 1000, []), end2 = segment(0x80, ms: 2500, [])
        return Data(shown + palette + object + end1 + cleared + end2)
    }

    @Test func pgsDisplaySets() throws {
        let sets = decodePgs(supStream)
        #expect(sets.map(\.startMs) == [1000, 2500])
        #expect(sets[1].render() == nil) // no objects: the screen clears

        let picture = try #require(sets[0].render())
        #expect(picture.x == 100 && picture.y == 50)
        #expect(picture.frameWidth == 1920 && picture.frameHeight == 1080)
        #expect(picture.image.width == 4 && picture.image.height == 2)

        // Every pixel opaque white-ish (BT.709 limited range: Y 235 is full white), both coding paths.
        let bytes = try #require(picture.image.dataProvider?.data as Data?)
        for pixel in 0..<8 {
            let bgra = bytes[(pixel * 4)..<(pixel * 4 + 4)].map { Int($0) }
            #expect(bgra[3] == 255 && bgra[0] > 250 && bgra[1] > 250 && bgra[2] > 250, "pixel \(pixel): \(bgra)")
        }
    }

    @Test func truncatedPgsStopsCleanly() {
        let cut = supStream.prefix(30)
        #expect(decodePgs(cut).isEmpty)
        #expect(decodePgs(Data("not a sup file at all".utf8)).isEmpty)
    }
}
