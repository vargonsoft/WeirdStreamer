import Foundation
import Testing
@testable import WeirdStreamer

@MainActor
struct PlaybackLogicTests {

    // MARK: Originals over HTTPS (OriginalLoader's byte ranges)

    @Test func rangeHeaders() {
        #expect(ByteWindow(start: 100, length: 50).rangeHeader == "bytes=100-149")
        #expect(ByteWindow(start: 0, length: 2).rangeHeader == "bytes=0-1")
        #expect(ByteWindow(start: 4096, length: nil).rangeHeader == "bytes=4096-")
    }

    @Test func aRangeArrivingInPieces() {
        var w = ByteWindow(start: 100, length: 50)
        let first = w.take(Data(repeating: 1, count: 30))
        #expect(first.data.count == 30 && !first.done)
        let second = w.take(Data(repeating: 2, count: 30)) // more than asked for: cut to the 20 left
        #expect(second.data.count == 20 && second.done)
        #expect(w.delivered == 50)
    }

    @Test func aServerThatIgnoresTheRange() {
        // A 200 sends the file from byte 0: bytes 10-14 are wanted, across two pieces.
        var w = ByteWindow(start: 10, length: 5)
        w.wholeFile()
        let file = Data((0..<40).map { UInt8($0) })
        let a = w.take(file[0..<8])
        #expect(a.data.isEmpty && !a.done)
        let b = w.take(file[8..<16])
        #expect([UInt8](b.data) == [10, 11, 12, 13, 14] && b.done)
    }

    @Test func toTheEndOfTheFile() {
        var w = ByteWindow(start: 0, length: nil)
        #expect(!w.take(Data(count: 1000)).done)
        #expect(w.delivered == 1000)
    }

    @Test func contentRangeTotals() {
        #expect(ByteWindow.totalLength(contentRange: "bytes 0-1/123456") == 123_456)
        #expect(ByteWindow.totalLength(contentRange: "bytes 0-1/*") == nil)
        #expect(ByteWindow.totalLength(contentRange: nil) == nil)
    }

    // MARK: Up next on a show's page

    /// Seasons from the server's JSON; each episode one file, with `playback` as given (nil: never played).
    private func show(_ seasons: [(number: Int, episodes: [String?])]) throws -> [LibrarySeason] {
        let json = seasons.map { s in
            let eps = s.episodes.enumerated().map { e, playback in
                #"{"season":\#(s.number),"episode":\#(e + 1),"title":"E","files":[{"path":"s\#(s.number)e\#(e + 1)","name":"f","playback":\#(playback ?? "null")}]}"#
            }
            return #"{"season":\#(s.number),"name":"S","episodes":[\#(eps.joined(separator: ","))]}"#
        }
        return try ServerClient.decode([LibrarySeason].self, from: Data("[\(json.joined(separator: ","))]".utf8))
    }

    private let watched = #"{"position":1300,"duration":1320,"watched":true,"updated_at":100}"#
    private let halfway = #"{"position":600,"duration":1320,"watched":false,"updated_at":200}"#

    @Test func upNextAfterAFinishedEpisode() throws {
        let s = try show([(1, [watched, nil, nil])])
        #expect(upNext(s, justPlayed: nil).map { [$0.season, $0.episode] } == [0, 1])
    }

    @Test func upNextResumesAnUnfinishedOne() throws {
        let s = try show([(1, [watched, halfway, nil])])
        #expect(upNext(s, justPlayed: nil).map { [$0.season, $0.episode] } == [0, 1])
    }

    @Test func upNextGoesOnToTheNextSeasonPastSpecials() throws {
        let s = try show([(0, [nil]), (1, [nil, watched]), (2, [nil])])
        #expect(upNext(s, justPlayed: nil).map { [$0.season, $0.episode] } == [2, 0])
    }

    @Test func nothingPlayedHasNoUpNext() throws {
        #expect(upNext(try show([(1, [nil, nil])]), justPlayed: nil) == nil)
    }

    // MARK: The A–Z strip

    @Test func indexLetters() {
        #expect(indexLabel("the Wire", skipArticles: true) == "W")
        #expect(indexLabel("The Wire") == "T")
        #expect(indexLabel("The", skipArticles: true) == "T")   // the whole name: kept
        #expect(indexLabel("9 Songs") == "#")
        #expect(indexLabel("..") == nil)
        #expect(indexLabel("  ") == nil)
        let entries = alphabetIndex(["Apple", "Avocado", "banana", "Alpha", "Zed"], groups: [0, 0, 0, 1, 1])
        #expect(entries == [IndexEntry(label: "A", position: 0), IndexEntry(label: "B", position: 2),
                            IndexEntry(label: "A", position: 3), IndexEntry(label: "Z", position: 4)])
    }

    // MARK: Motion photos

    @Test func motionPhotoClip() throws {
        let jpeg = Data([0xFF, 0xD8]) + Data(repeating: 0x11, count: 2000) + Data([0xFF, 0xD9])
        let marker = Data("MotionPhoto_Data".utf8)
        let mp4 = Data([0, 0, 0, 24]) + Data("ftypisom".utf8) + Data(repeating: 0, count: 12) + Data(repeating: 7, count: 500)
        let file = jpeg + marker + mp4
        let clip = try #require(embeddedVideo(in: file))
        #expect(clip == (jpeg.count + marker.count)..<file.count)
        #expect(pictureEnd(file, before: clip.lowerBound) == jpeg.count)
        #expect(pictureEnd(jpeg + mp4, before: jpeg.count) == jpeg.count) // Google's: no marker
        #expect(embeddedVideo(in: jpeg) == nil)
    }

    // MARK: Text

    @Test func clocksAndSizes() {
        #expect(clockText(75) == "1:15")
        #expect(clockText(3725) == "1:02:05")
        #expect(clockText(-3) == "0:00")
        #expect(formatFileSize(0) == "0 B")
        #expect(formatFileSize(1536) == "1.5 KB")
        #expect(formatFileSize(12_058_624) == "11.5 MB")
        #expect(formatBytes(nil) == "")
        #expect(seekGestureText(delta: -5, target: 83) == "−0:05 / 1:23")
        #expect(seekGestureText(delta: 90, target: 3700) == "+1:30 / 1:01:40")
    }

    @Test func timeLeftAndResume() throws {
        let p = try ServerClient.decode(Playback.self, from: Data(#"{"position":600,"duration":1320}"#.utf8))
        #expect(timeLeftText(p, nil) == "12 min left")
        #expect(p.fraction(fallbackDuration: nil) == 600.0 / 1320)
        let start = try ServerClient.decode(Playback.self, from: Data(#"{"position":8}"#.utf8))
        #expect(start.resumeAt == 0)            // the first 10 seconds start over
        #expect(timeLeftText(start, 1000) == nil)
    }
}
