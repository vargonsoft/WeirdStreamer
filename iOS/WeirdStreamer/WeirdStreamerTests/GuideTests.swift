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
}
