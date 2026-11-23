import Foundation
import Testing
@testable import WeirdStreamer

/// The download sheet's language choices, the tracks they pick, and what this phone plays as it is.
@MainActor
struct DownloadTests {

    /// A film with English (default) and Spanish audio; English, Spanish, forced English and French
    /// picture subtitles; the account's audio language is English.
    private func film(preferredAudio: Int = 0, preferredSubtitle: Int = -1) throws -> CodecsInfo {
        try ServerClient.decode(CodecsInfo.self, from: Data("""
        {"duration":5400,"video":{"codec":"h264"},
         "audio":[{"index":0,"codec":"aac","language":"eng","default":true},{"index":1,"codec":"ac3","language":"spa"}],
         "subtitles":[{"index":0,"codec":"subrip","language":"eng","text":true},
                      {"index":1,"codec":"subrip","language":"spa","text":true},
                      {"index":2,"codec":"subrip","language":"eng","text":true,"forced":true},
                      {"index":3,"codec":"hdmv_pgs_subtitle","language":"fre"},
                      {"index":4,"codec":"dvb_subtitle","language":"ger"}],
         "preferred_audio":\(preferredAudio),"preferred_subtitle":\(preferredSubtitle)}
        """.utf8))
    }

    @Test func languageCodes() {
        #expect(languageCode("eng") == "en")
        #expect(languageCode("fre") == "fr")   // bibliographic form
        #expect(languageCode("ger") == "de")
        #expect(languageCode("pt-BR") == "pt")
        #expect(languageCode(" SPA ") == "es")
        #expect(languageCode("und") == nil)
        #expect(languageCode("") == nil)
        #expect(languageCode(nil) == nil)
    }

    @Test func audioTrackForALanguage() throws {
        let c = try film()
        #expect(DownloadLanguages.audioTrack(c, "es") == 1)
        #expect(DownloadLanguages.audioTrack(c, "en") == 0)
        #expect(DownloadLanguages.audioTrack(c, "de") == 0)  // not in the file: what playing picks
        #expect(DownloadLanguages.audioTrack(c, nil) == 0)
        #expect(DownloadLanguages.audioTrack(try film(preferredAudio: -1), "de") == -1)
    }

    @Test func subtitleTrackForALanguage() throws {
        let c = try film()
        #expect(DownloadLanguages.subtitleTrack(c, DownloadLanguages.off) == -1)
        #expect(DownloadLanguages.subtitleTrack(c, "en") == 0)  // the full track, not the forced one
        #expect(DownloadLanguages.subtitleTrack(c, "es") == 1)
        #expect(DownloadLanguages.subtitleTrack(c, "fr") == 3)  // PGS is drawn, so it counts
        #expect(DownloadLanguages.subtitleTrack(c, "de") == -1) // DVB isn't: playing's pick
        #expect(DownloadLanguages.subtitleTrack(try film(preferredSubtitle: 1), nil) == 1)
    }

    @Test func languagesOffered() throws {
        let files = [try film()]
        #expect(DownloadLanguages.audioLanguages(files) == ["en", "es"])
        #expect(DownloadLanguages.subtitleLanguages(files) == ["en", "es", "fr"])
    }

    @Test func languagesTheSheetStartsWith() throws {
        let files = [try film()]
        let audio = ["en", "es"], subtitles = ["en", "es", "fr"]
        #expect(DownloadLanguages.startAudio(files, audio, last: "es") == "es")
        #expect(DownloadLanguages.startAudio(files, audio, last: "de") == "en")  // not offered: playing's pick
        #expect(DownloadLanguages.startAudio(files, audio, last: nil) == "en")
        #expect(DownloadLanguages.startSubtitles(files, subtitles, last: nil) == DownloadLanguages.off)
        #expect(DownloadLanguages.startSubtitles(files, subtitles, last: "fr") == "fr")
        #expect(DownloadLanguages.startSubtitles([try film(preferredSubtitle: 1)], subtitles, last: nil) == "es")
    }

    @Test func whatPlaysOffline() throws {
        let c = try film()
        let mp4 = OfflinePlayback.of(c, path: "lib/Film.mp4")
        #expect(mp4.plays && mp4.problem == nil)

        let mkv = OfflinePlayback.of(c, path: "lib/Film.mkv")
        #expect(!mkv.plays && mkv.video && mkv.audio && mkv.problem == "its MKV file")

        let dts = try ServerClient.decode(CodecsInfo.self, from: Data("""
        {"video":{"codec":"hevc"},"audio":[{"index":0,"codec":"aac","default":true},{"index":1,"codec":"dts"}]}
        """.utf8))
        #expect(OfflinePlayback.of(dts, path: "a.mp4").plays)                  // the default track: AAC
        let chosen = OfflinePlayback.of(dts, path: "a.mp4", audioTrack: 1)       // downloaded with the DTS one
        #expect(!chosen.plays && !chosen.audio && chosen.problem == "its DTS audio")

        let vc1 = try ServerClient.decode(CodecsInfo.self, from: Data(#"{"video":{"codec":"vc1"}}"#.utf8))
        #expect(OfflinePlayback.of(vc1, path: "a.mp4").problem == "its VC1 video")
    }

    @Test func hardwareEncoderChoices() throws {
        let r = try ServerClient.decode(HardwareReport.self, from: Data("""
        {"checking":false,"gpus":[{"name":"GTX 1060","vendor":"nvidia"}],
         "encoders":[{"id":"nvenc","name":"NVENC","gpu":"GTX 1060","vendor":"nvidia","sessions":8}]}
        """.utf8))
        // NVENC has no device: a device left in the config doesn't make it another choice.
        #expect(HardwareSettings.chosenKey(r, encoder: "nvenc", device: "/dev/dri/renderD128") == "nvenc|")
        #expect(HardwareSettings.chosenKey(r, encoder: "", device: "x") == "")
        let choices = HardwareSettings.choices(r, encoder: "vaapi", device: "/dev/dri/renderD128")
        #expect(choices.map(\.0) == ["", "nvenc|", "vaapi|/dev/dri/renderD128"])
        #expect(choices.last?.1.contains("not working") == true)
        #expect(r.encoders?.first?.sessions == 8)
        #expect(HardwareSettings.note(r, chosen: "nvenc|").contains("allows 8 encodes at once"))
        #expect(HardwareSettings.note(r, chosen: "") == "Transcodes and downloads are encoded by the CPU.")

        let none = HardwareReport(checking: false, gpus: nil, encoders: nil, problems: nil)
        #expect(HardwareSettings.status(none) == "No graphics hardware was found, so transcodes are encoded by the CPU (x264).")
        #expect(HardwareSettings.status(HardwareReport(checking: true, gpus: nil, encoders: nil, problems: nil)).hasPrefix("Checking"))
    }
}
