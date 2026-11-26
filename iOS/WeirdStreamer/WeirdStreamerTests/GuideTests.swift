import Foundation
import Testing
@testable import WeirdStreamer

/// The Guide's replies (handlers/guide.go, guide.ProgramInfo), as the server writes them.
@MainActor
struct GuideTests {

    private func decode<T: Decodable>(_ type: T.Type, _ text: String) throws -> T {
        try ServerClient.decode(type, from: Data(text.utf8))
    }

    @Test func available() throws {
        #expect(try decode(GuideAvailable.self, #"{"available":true}"#).available == true)
    }

    @Test func listings() throws {
        let g = try decode(GuideListings.self, """
        {"now":1764000000000,"from":1763999100000,"to":1764011700000,"channels":[
          {"number":14,"name":"Movies","genre":"Movies","color":"#6a3a62",
           "logo":"/api/guide/logo?channel=14&v=abc","listings":[
             {"start":1763999100000,"end":1764006300000,
              "info":{"kind":"movie","tmdb_id":812,"library":"01c6653f","title":"Aladdin","year":1992,
                      "rating":"G","genres":["Animation","Family"],"overview":"A street urchin…","poster":"/artwork/movie/812.jpg"}},
             {"start":1764006300000,"end":1764011700000}]},
          {"number":15,"name":"Variety","genre":"Variety","color":"#204060","logo":"/api/guide/logo?channel=15&v=def"}]}
        """)
        #expect(g.from == 1763999100000)
        #expect(g.channels?.map(\.number) == [14, 15])
        let movie = try #require(g.channels?[0].listings?[0].info)
        #expect(movie.tmdbId == 812)
        #expect(movie.year == 1992)
        #expect(movie.genres == ["Animation", "Family"])
        #expect(movie.episodeLabel == nil)
        #expect(g.channels?[0].listings?[1].info == nil) // off the air
        #expect(g.channels?[1].listings == nil)
    }

    @Test func now() throws {
        let n = try decode(GuideNow.self, """
        {"channel":{"number":15,"name":"Variety","genre":"Variety","color":"#204060","logo":"/api/guide/logo?channel=15"},
         "kind":"program","path":"01c6653f/Spartacus/S01E04.mkv","offset":312.5,"start":1763999100000,"end":1764000900000,
         "program":{"kind":"show","tmdb_id":46296,"library":"01c6653f","title":"Spartacus","season":1,"episode":4,
                    "episode_title":"The Thing in the Pit","episode_overview":"Spartacus fights in the Pits."},
         "next":{"kind":"show","tmdb_id":46296,"library":"01c6653f","title":"Spartacus","season":1,"episode":5},
         "next_start":1764000900000,"server_time":1764000000000}
        """)
        #expect(n.kind == "program")
        #expect(n.offset == 312.5)
        #expect(n.nextStart == 1764000900000)
        #expect(n.serverTime == 1764000000000)
        #expect(n.program?.episodeLabel == "S1E4 · The Thing in the Pit")
        #expect(n.next?.episodeLabel == "S1E5")
    }

    @Test func bumperHasNoPath() throws {
        let n = try decode(GuideNow.self, """
        {"channel":{"number":2,"name":"WKBT","logo":""},"kind":"bumper","offset":4,"start":1,"end":2,"server_time":1}
        """)
        #expect(n.path == nil)
        #expect(n.next == nil)
        #expect(n.nextStart == nil)
    }

    @Test func guideChannelBlock() throws {
        let n = try decode(GuideNow.self, """
        {"channel":{"number":1,"name":"Guide","logo":"/api/guide/logo?channel=1","kind":"guide"},
         "kind":"promo","offset":12,"start":1,"end":2,"server_time":1,
         "program":{"kind":"show","title":"Spartacus","season":1,"episode":3},
         "airs":1764000900000,"airs_on":{"number":15,"name":"Variety","logo":"/api/guide/logo?channel=15"},
         "music":{"url":"/api/guide/music?name=Local%20Forecast.mp3","title":"Local Forecast","credit":"\\"Local Forecast\\" by Kevin MacLeod",
                  "offset":42.5,"start":1,"end":2}}
        """)
        #expect(n.channel.kind == "guide")
        #expect(n.airs == 1764000900000)
        #expect(n.airsOn?.number == 15)
        #expect(n.music?.offset == 42.5)
        #expect(n.music?.credit?.hasPrefix("\"Local Forecast\"") == true)
    }

    @Test func weather() throws {
        let w = try decode(GuideWeather.self, """
        {"place":"New York, NY","zip":"10001","updated":1,"radar":true,
         "current":{"time":1,"station":"Central Park","conditions":"Cloudy","icon":"cloud","temp":43,"feels_like":35,
                    "dewpoint":35,"humidity":73,"wind_dir":"ENE","wind":18,"pressure":30.46,"visibility":10},
         "periods":[{"name":"Tonight","daytime":false,"temp":51,"forecast":"Heavy Rain","detail":"Rain.","icon":"rain","wind":"SE 13 mph","precip":100}],
         "alerts":[{"event":"High Wind Warning","headline":"High Wind Warning until 6 PM","severity":"Severe"}]}
        """)
        #expect(w.current?.feelsLike == 35)
        #expect(w.current?.windDir == "ENE")
        #expect(w.periods?.first?.precip == 100)
        #expect(w.periods?.first?.daytime == false)
        #expect(w.alerts?.first?.headline == "High Wind Warning until 6 PM")
        #expect(w.radar == true)
        let failed = try decode(GuideWeather.self, #"{"zip":"00000","periods":[],"alerts":[],"radar":false,"error":"00000 isn't a US ZIP code"}"#)
        #expect(failed.current == nil && failed.error != nil)
    }
}
