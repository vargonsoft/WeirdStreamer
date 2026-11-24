import Foundation
import Testing
@testable import WeirdStreamer

/// Account PINs: the hash checks, the wait after 5 wrong ones, and clearing.
struct PinStoreTests {
    private func store(at clock: @escaping () -> Date) -> PinStore {
        let defaults = UserDefaults(suiteName: "PinStoreTests-\(UUID().uuidString)")!
        var store = PinStore(defaults: defaults)
        store.now = clock
        return store
    }

    @Test func rightAndWrong() {
        let pins = store { Date() }
        #expect(!pins.has("https://a", "ash"))
        #expect(pins.check("https://a", "ash", pin: "0000") == .right) // none set: nothing to ask
        pins.set("https://a", "ash", pin: "2468")
        #expect(pins.has("https://a", "ash"))
        #expect(!pins.has("https://a", "bea"))
        #expect(pins.check("https://a", "ash", pin: "1357") == .wrong(lockedUntil: nil))
        #expect(pins.check("https://a", "ash", pin: "2468") == .right)
        pins.clear("https://a", "ash")
        #expect(!pins.has("https://a", "ash"))
    }

    @Test func waitsAfterFiveWrong() {
        var now = Date(timeIntervalSince1970: 1_000_000)
        let pins = store { now }
        pins.set("https://a", "ash", pin: "2468")
        for _ in 1...4 { #expect(pins.check("https://a", "ash", pin: "1111") == .wrong(lockedUntil: nil)) }
        let first = now.addingTimeInterval(30)
        #expect(pins.check("https://a", "ash", pin: "1111") == .wrong(lockedUntil: first))
        // While waiting even the right PIN isn't tried
        #expect(pins.check("https://a", "ash", pin: "2468") == .locked(until: first))
        now = first.addingTimeInterval(1)
        #expect(pins.check("https://a", "ash", pin: "1111") == .wrong(lockedUntil: now.addingTimeInterval(60))) // doubles
        now = now.addingTimeInterval(61)
        #expect(pins.check("https://a", "ash", pin: "2468") == .right)
        // A right one starts the count over
        #expect(pins.check("https://a", "ash", pin: "1111") == .wrong(lockedUntil: nil))
        #expect(pins.lockedUntil("https://a", "ash") == nil)
    }

    @Test func waitText() {
        let now = Date(timeIntervalSince1970: 0)
        #expect(PinStore.wait(until: now.addingTimeInterval(30), now: now) == "0:30")
        #expect(PinStore.wait(until: now.addingTimeInterval(125), now: now) == "2:05")
    }
}
