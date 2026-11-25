import Foundation
import Testing
@testable import WeirdStreamer

/// The JSON the app sends and reads, against the names the Go server uses (its `json:"…"` tags).
/// A request body with the wrong key names is accepted by the server and silently ignored, so
/// these are checked here rather than found in use.
@MainActor
struct APIContractTests {

    private func json<B: Encodable>(_ body: B, snakeCase: Bool = false) throws -> String {
        String(decoding: try ServerClient.encodeBody(body, snakeCase: snakeCase), as: UTF8.self)
    }

    private func decode<T: Decodable>(_ type: T.Type, _ text: String) throws -> T {
        try ServerClient.decode(type, from: Data(text.utf8))
    }

    // MARK: Request bodies

    @Test func accountSettingsUseTheServersNames() throws {
        let account = AccountSettings(audioLanguage: "en", subtitleLanguage: "", subtitlesOn: true, languages: ["en", "fr"])
        let expected = #"{"audio_language":"en","subtitle_language":"","subtitles_on":true}"#
        // PUT /api/account (database.Languages): the same however the call encodes it.
        #expect(try json(account) == expected)
        #expect(try json(account, snakeCase: true) == expected)
    }

    @Test func streamProgress() throws {
        let body = StreamProgress(path: "cGF0aA==", position: 12.5, paused: true, ended: false, title: 2)
        #expect(try json(body) == #"{"ended":false,"path":"cGF0aA==","paused":true,"position":12.5,"title":2}"#)
    }

    @Test func playbackSync() throws {
        let body = PlaybackSync(items: [SyncedPlayback(path: "cA==", position: 3, ended: true, updatedAt: 1_700_000_000_000)])
        #expect(try json(body) == #"{"items":[{"ended":true,"path":"cA==","position":3,"updated_at":1700000000000}]}"#)
    }

    @Test func watchedRequest() throws {
        let body = WatchedRequest(paths: ["cA=="], watched: true, library: nil, kind: "tv", tmdbId: 1399)
        #expect(try json(body) == #"{"kind":"tv","paths":["cA=="],"tmdb_id":1399,"watched":true}"#)
    }

    // MARK: Replies

    @Test func codecsReport() throws {
        let c = try decode(CodecsInfo.self, """
        {"container":"matroska,webm","duration":600.5,
         "video":{"codec":"h264","profile":"High","pix_fmt":"yuv420p"},
         "audio":[{"index":0,"codec":"eac3","channels":6,"language":"eng","default":true},
                  {"index":1,"codec":"aac","channels":2,"language":"spa","title":"Latino"}],
         "subtitles":[{"index":0,"codec":"subrip","language":"eng","text":true,"hearing_impaired":true,"offset_ms":-250,"key":"k1"},
                      {"index":1,"codec":"dvd_subtitle","language":"fre","forced":true}],
         "fonts":[],"preferred_audio":1,"preferred_subtitle":-1}
        """)
        #expect(c.duration == 600.5)
        #expect(c.video?.codec == "h264")
        #expect(c.audio?.map(\.index) == [0, 1])
        #expect(c.audio?[1].title == "Latino")
        #expect(c.preferredAudio == 1)
        #expect(c.preferredSubtitle == -1)
        let subs = try #require(c.subtitles)
        #expect(subs[0].hearingImpaired == true && subs[0].offsetMs == -250 && subs[0].isText)
        #expect(subs[1].drawnAsPGS && subs[1].forced == true && !subs[1].isText)
    }

    @Test func folderListing() throws {
        let d = try decode(Directory.self, """
        {"path":"01c6653f","name":"Clips","library_type":"movie",
         "dirs":[{"path":"01c6653f/Live Photos","name":"Live Photos","thumbnail_url":"/thumbnails/d/b/db.jpg?v=1"}],
         "items":[]}
        """)
        #expect(d.libraryType == "movie")
        #expect(d.dirs?.first?.id == "01c6653f/Live Photos")
        #expect(d.dirs?.first?.thumbnailUrl == "/thumbnails/d/b/db.jpg?v=1")
    }

    @Test func accountReply() throws {
        let a = try decode(AccountSettings.self, #"{"audio_language":"fr","subtitle_language":"en","subtitles_on":false,"languages":["en","fr"]}"#)
        #expect(a.audioLanguage == "fr" && a.subtitleLanguage == "en" && !a.subtitlesOn)
        #expect(a.languages == ["en", "fr"])
    }

    @Test func backupList() throws {
        // As the server writes it: Go's times have nanoseconds.
        let list = try decode(BackupList.self, #"{"folder":"/srv/ws/var/backups","next":"2026-11-24T04:00:00-05:00","backups":[{"name":"weirdstreamer-backup-20261123-184226.tar.gz","size":212664,"created":"2026-11-23T18:42:26.123456789-05:00"}]}"#)
        #expect(list.backups?.first?.size == 212664)
        #expect(BackupSettings.date(list.backups?.first?.created ?? "") != nil)
        #expect(BackupSettings.whereText(list).hasPrefix("In /srv/ws/var/backups. The next one: "))
        #expect(BackupSettings.time("4:00") == "04:00" && BackupSettings.time("25:00") == "04:00" && !BackupSettings.validTime("4:0"))
    }

    @Test func damagedList() throws {
        // As the server writes it (snake case; error only for one that couldn't be fixed)
        let list = try decode(DamagedList.self, #"{"videos":[{"path":"01c6653f/Movies/Film.avi","name":"Film.avi","library":"Movies","folder":"","reason":"Its seek index is broken: players can't jump in it","state":"fixed","size":316013903,"fixed_size":312475648,"claimed":7672.419,"duration":4472.12}]}"#)
        let v = try #require(list.videos?.first)
        #expect(v.fixed && v.fixedSize == 312475648 && v.claimed == 7672.419)
        #expect(DamagedVideos.clock(v.duration) == "1:14:32" && DamagedVideos.clock(v.claimed) == "2:07:52")
        #expect(DamagedVideos.replacedName("Film.avi") == "Film.mkv" && DamagedVideos.replacedName("Melissa P.flv") == "Melissa P.mkv")
        #expect(DamagedVideos.whereText(v).hasPrefix("Movies · Its seek index"))
        let trash = try decode(TrashReport.self, #"{"items":[{"id":"1795550429-1","name":"Film.avi","library":"Movies","folder":"Old","size":316013903,"trashed_at":"2026-11-24T15:00:29Z","deletes_at":"2026-12-24T15:00:29Z","reason":"Jumping into it finds no video","replacement":"Film.mkv","restorable":true,"thumbnail_url":"/api/admin/trash/thumbnail?id=1795550429-1"},{"id":"1795550000-1","name":"Old.avi","folder":"","size":10,"trashed_at":"2026-11-24T14:53:20Z","deletes_at":"2026-12-24T14:53:20Z","restorable":false}],"files":2,"bytes":316013913,"days":30}"#)
        #expect(DamagedVideos.trashText(trash).hasPrefix("The trash holds 2 replaced originals ("))
        let t = try #require(trash.items?.first)
        #expect(t.restorable == true && t.thumbnailUrl == "/api/admin/trash/thumbnail?id=1795550429-1")
        #expect(DamagedVideos.trashWhere(t) == "Movies › Old" && DamagedVideos.trashWhere(trash.items![1]) == "Not in a library")
        #expect(DamagedVideos.trashDates(t).contains(" by Film.mkv · Deleted ") && !DamagedVideos.trashDates(t).contains("?"))
        #expect(DamagedVideos.restoreText(t).contains("Film.mkv goes back to the fixed copies"))
        #expect(DamagedVideos.trashText(TrashReport(files: 0, bytes: 0, days: 30)) == "The trash is empty. Replaced originals are kept there 30 days.")
        let waiting = try decode(DamagedVideo.self, #"{"path":"01c6653f/Movies/Clip.avi","name":"Clip.avi","state":"damaged","size":1048576}"#)
        #expect(waiting.fixable && !waiting.fixed && DamagedVideos.sizes(waiting).hasSuffix("Fix makes a copy of about its size"))
    }

    @Test func hardwareReportWithNothingFound() throws {
        // The server sends null for empty lists (gpus and encoders aren't omitempty).
        let r = try decode(HardwareReport.self, #"{"checking":false,"gpus":null,"encoders":null}"#)
        #expect(!r.checking && r.gpus == nil && r.encoders == nil && r.problems == nil)
    }

    // MARK: Thumbnail updates (server-sent events)

    @Test func thumbnailEvents() {
        var parser = ServerClient.ThumbnailEventParser()
        let lines = [
            "event: connected", #"data: {"status":"ok"}"#,                      // a named event: not a thumbnail
            ": keepalive",
            #"data: {"path":"01c6653f/Clips/Clip 01.mkv","thumbnailURL":"/thumbnails/c/a/ca.jpg?v=9"}"#,
            "data: not json",
            "event: message", #"data:{"path":"01c6653f/Clips/Live Photos","thumbnailURL":"/thumbnails/d.jpg"}"#,
        ]
        let updates = lines.compactMap { parser.take($0) }
        #expect(updates == [
            .init(path: "01c6653f/Clips/Clip 01.mkv", thumbnailURL: "/thumbnails/c/a/ca.jpg?v=9"),
            .init(path: "01c6653f/Clips/Live Photos", thumbnailURL: "/thumbnails/d.jpg"),
        ])
    }

    // MARK: Fixing matches

    /// GET /api/media/match (metadata.MatchInfo with models.ItemMetadata and EpisodeInfo).
    @Test func matchInfoReadsTheServersNames() throws {
        let info = try decode(MatchInfo.self, #"""
            {"kind":"tv","match_kind":"tv","kinds":["tv"],"query":"Spartacus","year":2010,"status":"matched","manual":true,
             "current":{"kind":"show","tmdb_id":46296,"title":"Spartacus","year":2010,"poster":"/artwork/p.jpg"},
             "episode":{"file_season":1,"file_episode":3,"season":1,"episode":5,"source":"manual","title":"Shadow Games"}}
            """#)
        #expect(info.matchKind == "tv" && info.manual == true && info.current?.tmdbId == 46296)
        #expect(info.episode?.fileEpisode == 3 && info.episode?.episode == 5 && info.episode?.source == "manual")
        let hits = try decode([MatchHit].self, #"[{"tmdb_id":1,"title":"8MM","year":1999,"poster":"https://img/p.jpg","poster_path":"/p.jpg"}]"#)
        #expect(hits.first?.posterPath == "/p.jpg")
        let posters = try decode(PosterList.self, #"{"current":"/p.jpg","manual":false,"posters":[{"path":"/p.jpg","url":"https://img/p.jpg","language":""}]}"#)
        #expect(posters.posters?.first?.language == "")
        let seasons = try decode([TmdbSeason].self, #"[{"number":1,"name":"Blood and Sand","episode_count":13}]"#)
        #expect(seasons.first?.episodeCount == 13)
        let episodes = try decode([TmdbEpisode].self, #"[{"number":5,"name":"Shadow Games","air_date":"2010-02-19"}]"#)
        #expect(episodes.first?.airDate == "2010-02-19")
    }

    /// PUT /api/media/match, /poster and /episode (handlers' matchRequest and episodeMappingRequest):
    /// a file by its base64 path, a folder by its client path.
    @Test func matchRequestsNameTheirTarget() throws {
        let file = MatchTarget(path: "01c6653f/A/a.mkv", isDir: false, name: "a.mkv")
        let folder = MatchTarget(path: "01c6653f/A", isDir: true, name: "A")
        #expect(file.query.map { $0.0 } == ["path"] && file.query[0].1 == ServerClient.encodePath("01c6653f/A/a.mkv"))
        #expect(folder.query.map { $0.0 } == ["dir"] && folder.query[0].1 == "01c6653f/A")
        let body = file.body(["tmdb_id": 603, "kind": "movie"])
        let text = String(decoding: try JSONSerialization.data(withJSONObject: body, options: .sortedKeys), as: UTF8.self)
        #expect(text == #"{"kind":"movie","path":"MDFjNjY1M2YvQS9hLm1rdg==","tmdb_id":603}"#)
        #expect(folder.body(["poster": ""])["dir"] as? String == "01c6653f/A")
        let mapping = file.body(["season": 2, "episode": 5, "whole_season": true])
        #expect(Set(mapping.keys) == ["path", "season", "episode", "whole_season"])
    }

    // MARK: Addresses and paths

    @Test func addressesAreNormalized() {
        #expect(ServerClient.normalized("Https://Host.Example:443/") == "https://host.example")
        #expect(ServerClient.normalized("http://media.local:80//") == "http://media.local")
        #expect(ServerClient.normalized("https://media.local:9000/") == "https://media.local:9000")
        #expect(ServerClient.normalized("not an address") == nil)
    }

    @Test func certificatePinsAreKeyedByHostAndPort() throws {
        #expect(TrustPolicy.key(of: try #require(URL(string: "https://Host:9000/api"))) == "host:9000")
        #expect(TrustPolicy.key(of: try #require(URL(string: "https://host/api"))) == "host:443")
        #expect(TrustPolicy.key(of: try #require(URL(string: "http://host:9000"))) == nil)
    }

    @Test func pathsAreBase64() {
        #expect(ServerClient.encodePath("01c6653f/EAC3 Test/EAC3 Test.mkv") == "MDFjNjY1M2YvRUFDMyBUZXN0L0VBQzMgVGVzdC5ta3Y=")
        // The query keeps base64's + / = intact.
        #expect(ServerClient.escape("a+b/c=") == "a%2Bb%2Fc%3D")
    }
}
