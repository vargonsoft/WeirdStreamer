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

    @Test func waitLimits() throws {
        // GET /api/admin/jobs: the load with config.yml's wait limits (0: the default) and the defaults.
        let jobs = try decode(JobsResponse.self, #"""
        {"jobs":[],"load":{"available":true,"max_disk":0.5,"relaxed_disk":0.95,"max_share_ms":40,"relaxed_share_ms":500,
         "limits":{"disk_percent":0,"disk_idle_percent":95,"share_ms":40,"share_idle_ms":0},
         "default_limits":{"disk_percent":50,"disk_idle_percent":90,"share_ms":60,"share_idle_ms":500}}}
        """#)
        let l = try #require(jobs.load)
        #expect(l.limits == WaitLimits(diskPercent: 0, diskIdlePercent: 95, shareMs: 40, shareIdleMs: 0))
        #expect(l.defaultLimits?.shareIdleMs == 500 && l.relaxedDisk == 0.95)
    }

    @Test func trickplayMemory() throws {
        // GET /api/admin/jobs: trickplay's settings, frames per runner by size, with media.TrickplayMemory (4 GB and
        // 2 GB of swap).
        let jobs = try decode(JobsResponse.self, #"""
        {"jobs":[],"trickplay":{"runners":1,"max_runners":8,
         "tiers":[{"id":"4k","name":"4K","frames":20,"max_frames":50,"max_product":100,"scale":1},
                  {"id":"1080p","name":"1080p","frames":23,"auto":true,"max_frames":100,"max_product":200,"scale":0.37}],
         "memory":{"process_bytes_4k":62914560,"frame_bytes_4k":239075328,"total_bytes":4294967296,"swap_bytes":2147483648}}}
        """#)
        let tp = try #require(jobs.trickplay)
        let m = try #require(tp.memory)
        let tiers = try #require(tp.tiers)
        #expect(tiers.map(\.id) == ["4k", "1080p"] && tiers[1].auto == true && tiers[1].scale == 0.37)
        // Automatic settings: an empty field takes what the server worked out.
        let auto = try decode(TrickplaySettings.self, #"""
        {"runners":2,"auto_runners":true,"auto_runners_value":2,"gpu":"NVIDIA GeForce GTX 1060 6GB","use_gpu":false,
         "tiers":[{"id":"4k","name":"4K","frames":4,"auto":true,"auto_frames_for":[8,4,2,1,1,1,1,1]},
                  {"id":"sd","name":"SD","frames":40,"auto":true,"auto_frames_for":[80,40,26,20,16,13,11,10]}]}
        """#)
        #expect(auto.autoRunners == true && auto.tiers?.first?.auto == true)
        #expect(auto.gpu == "NVIDIA GeForce GTX 1060 6GB" && auto.useGpu == false)
        for (fields, want) in [(("", ["", ""]), [2, 4, 40]), (("1", []), [1, 8, 80]), (("3", ["6", ""]), [3, 6, 26])] {
            let (r, f) = auto.effective(fields.0, fields.1)
            #expect([r] + f == want)
        }
        #expect(m.frameBytes4K == 239_075_328 && m.totalBytes == 4_294_967_296 && m.swapBytes == 2_147_483_648)
        let table = trickplayMemoryTable(1, [20, 20], tiers, m)
        #expect(table.rows == [TrickplayRow(what: "4K", frames: "1 × 20", needs: "4.5 GB", has: "4.0 GB + 2.0 GB swap", note: "Uses swap", warn: true),
                               TrickplayRow(what: "1080p", frames: "1 × 20", needs: "1.7 GB", has: "4.0 GB + 2.0 GB swap", note: "Fits", warn: false)])
        #expect(trickplayMemoryTable(3, [25, 25], tiers, m).rows.first?.note == "Too much: frames are made one at a time")
        #expect(table.summary == "1 runner, decoded on the CPU; a runner's frames depend on its video's size.")

        // An NVIDIA card: the server's memory, and the card's own.
        let gpu = try decode(TrickplayMemory.self, #"""
        {"decoder":"cuda","process_bytes_4k":183500800,"frame_bytes_4k":130023424,"total_bytes":49392123904,"swap_bytes":0,
         "card_process_bytes_4k":62914560,"card_frame_bytes_4k":566231040,"card_bytes":6442450944}
        """#)
        #expect(gpu.decoder == "cuda" && gpu.cardFrameBytes4K == 566_231_040 && gpu.cardBytes == 6_442_450_944)
        let cardTable = trickplayMemoryTable(1, [20], [tiers[0]], gpu)
        #expect(cardTable.summary == "1 runner, decoded on the graphics card; a runner's frames depend on its video's size.")
        #expect(cardTable.rows == [TrickplayRow(what: "4K", frames: "1 × 20", needs: "2.6 GB", has: "46.0 GB", note: "Fits", warn: false),
                                   TrickplayRow(what: "4K, graphics card", frames: "1 × 20", needs: "10.6 GB", has: "6.0 GB", note: "Too much: batches are made smaller", warn: true)])
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

    // MARK: Collections

    /// GET /api/library (models.LibraryListing's collections) and /api/library/collection (CollectionDetail).
    @Test func collectionsReadTheServersNames() throws {
        let listing = try decode(LibraryListing.self, #"""
            {"library_id":"m","library_name":"Movies","library_type":"movie",
             "titles":[{"kind":"movie","tmdb_id":24428,"title":"The Avengers","year":2012,"file_count":1,"collection_id":86311}],
             "unmatched":[],"collections":[{"id":86311,"name":"The Avengers Collection","poster":"/artwork/collection/86311/poster.jpg","count":1,"total":6}]}
            """#)
        #expect(listing.titles?.first?.collectionId == 86311)
        #expect(listing.collections?.first?.count == 1 && listing.collections?.first?.total == 6)
        let page = try decode(CollectionDetail.self, #"""
            {"id":86311,"name":"The Avengers Collection","overview":"Assemble.","parts":[
             {"kind":"movie","tmdb_id":24428,"title":"The Avengers","year":2012,"file_count":1,"collection_id":86311},
             {"kind":"movie","tmdb_id":99861,"title":"Avengers: Age of Ultron","year":2015,"file_count":0,"collection_id":86311,"missing":true}]}
            """#)
        #expect(page.parts.map { $0.missing == true } == [false, true])
    }

    /// The listing's user_collections and GET /api/library/usercollection (models.UserCollectionSummary, UserCollectionDetail).
    @Test func userCollectionsReadTheServersNames() throws {
        let listing = try decode(LibraryListing.self, #"""
            {"library_id":"m","library_name":"Movies","library_type":"movie","titles":[],"unmatched":[],
             "user_collections":[{"id":3,"name":"Marvel Night","description":"Heroes.","count":2,
               "posters":["/artwork/movie/1/poster.jpg"],"titles":[{"kind":"movie","tmdb_id":10138}]}]}
            """#)
        let c = try #require(listing.userCollections?.first)
        #expect(c.id == 3 && c.count == 2 && c.titles?.first == CollectionTitle(kind: "movie", tmdbId: 10138))
        let page = try decode(UserCollectionDetail.self, #"""
            {"id":3,"name":"Marvel Night","posters":[],"parts":[{"kind":"movie","tmdb_id":10138,"title":"Iron Man 2","year":2010,"file_count":1}]}
            """#)
        #expect(page.parts.first?.ref == CollectionTitle(kind: "movie", tmdbId: 10138))
    }

    /// GET /api/library/person (models.PersonDetail with LibraryTitle and PersonCredit).
    @Test func personReadsTheServersNames() throws {
        let p = try decode(PersonDetail.self, #"""
            {"id":3223,"name":"Robert Downey Jr.","photo":"/artwork/person/3223large.jpg","biography":"Actor.",
             "birthday":"1965-04-04","place_of_birth":"New York City","known_for":"Acting",
             "in_library":[{"kind":"movie","tmdb_id":10138,"title":"Iron Man 2","year":2010,"file_count":1,"role":"Tony Stark"}],
             "filmography":[{"kind":"show","tmdb_id":1,"title":"Ally McBeal","year":1997,"poster":"https://image.tmdb.org/t/p/w185/a.jpg","role":"Larry Paul","in_library":false}]}
            """#)
        #expect(p.placeOfBirth == "New York City" && p.knownFor == "Acting")
        #expect(p.inLibrary.first?.role == "Tony Stark" && p.filmography.first?.inLibrary == false)
    }

    /// A video's format line, in folder listings (models.MediaItem) and on title pages (models.LibraryFile).
    @Test func formatLinesReadTheServersNames() throws {
        let item = try decode(MediaItem.self, #"{"path":"a/b.mkv","name":"b.mkv","type":"video","format":"4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"}"#)
        #expect(item.format == "4K · HEVC · Dolby Vision · TrueHD Atmos 7.1")
        let file = try decode(LibraryFile.self, #"{"path":"a/b.mkv","name":"b.mkv","format":"1080p · H.264 · AAC 2.0"}"#)
        #expect(file.format == "1080p · H.264 · AAC 2.0")
    }

    /// A title page says whether it has theme music (models.TitleDetail's theme).
    @Test func titleThemeReadsTheServersName() throws {
        let t = try decode(TitleDetail.self, #"{"kind":"show","tmdb_id":1,"title":"Spartacus","theme":true}"#)
        #expect(t.theme == true)
        #expect(themeVolume("low") < themeVolume("medium") && themeVolume("medium") < themeVolume("high"))
        #expect(themeVolume("unknown") == themeVolume("medium"))
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
