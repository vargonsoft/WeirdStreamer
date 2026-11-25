import SwiftUI

/// A person's window (Android's PhonePersonDialog), opened from a title page's cast: a larger photo,
/// their details and biography (six lines, the rest on a tap), their titles in this library (a tap
/// opens one) and their whole TMDB filmography (those here open too).
struct PersonSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let cast: CreditInfo
    @State private var person: PersonDetail?
    @State private var error: String?
    @State private var wholeBio = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    HStack(alignment: .top, spacing: 16) {
                        ZStack {
                            Palette.raised
                            RemoteImage(relative: person?.photo ?? cast.profile)
                        }
                        .frame(width: 120, height: 180)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                        VStack(alignment: .leading, spacing: 4) {
                            if let person {
                                ForEach(Array(facts(person).enumerated()), id: \.offset) { _, fact in
                                    (Text(fact.0 + ": ").bold() + Text(fact.1)).font(.subheadline)
                                }
                            } else {
                                Text(error.map { "Couldn't load their details: \($0)" } ?? "Loading…").font(.subheadline)
                            }
                        }
                    }
                    if let person {
                        if let bio = person.biography, !bio.isEmpty {
                            Button { wholeBio.toggle() } label: {
                                VStack(alignment: .leading, spacing: 4) {
                                    Text(bio).font(.subheadline).lineLimit(wholeBio ? nil : 6).multilineTextAlignment(.leading)
                                    Text(wholeBio ? "Show less" : "Show more").font(.caption.weight(.semibold)).foregroundStyle(Palette.accent)
                                }
                            }
                            .buttonStyle(.plain)
                        }
                        if !person.inLibrary.isEmpty {
                            Text("In \(model.libraryListing?.libraryName ?? "this library") (\(person.inLibrary.count))")
                                .font(.headline).foregroundStyle(Palette.accent)
                            ScrollView(.horizontal, showsIndicators: false) {
                                LazyHStack(alignment: .top, spacing: 12) {
                                    ForEach(person.inLibrary) { t in
                                        PosterTile(title: t.title, subtitle: [t.year.map(String.init), t.role].compactMap { $0 }.joined(separator: " · "),
                                                   poster: t.poster, placeholder: "film", progress: t.progress ?? 0, watched: t.allWatched) {
                                            open(t.kind, t.tmdbId)
                                        }
                                        .frame(width: 108)
                                    }
                                }
                            }
                        }
                        if !person.filmography.isEmpty {
                            Text("Filmography (\(person.filmography.count))").font(.headline).foregroundStyle(Palette.accent)
                            LazyVStack(alignment: .leading, spacing: 6) {
                                ForEach(person.filmography) { f in
                                    let here = f.inLibrary == true
                                    Button { if here { open(f.kind, f.tmdbId) } } label: {
                                        HStack(spacing: 12) {
                                            ZStack {
                                                Palette.raised
                                                RemoteImage(relative: f.poster)
                                            }
                                            .frame(width: 40, height: 60)
                                            .clipShape(RoundedRectangle(cornerRadius: 4))
                                            VStack(alignment: .leading, spacing: 2) {
                                                Text(f.title + (f.kind == "show" ? " (TV)" : "")).font(.subheadline.weight(.semibold))
                                                    .foregroundStyle(here ? Palette.accent : Palette.text)
                                                Text(filmLine(f)).font(.caption).foregroundStyle(Palette.muted)
                                            }
                                            Spacer(minLength: 0)
                                        }
                                        .contentShape(Rectangle())
                                    }
                                    .buttonStyle(.plain)
                                    .disabled(!here)
                                }
                            }
                        }
                    }
                }
                .padding(16)
            }
            .background(Palette.background)
            .navigationTitle(cast.name)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } } }
        }
        .task { await load() }
    }

    private func load() async {
        guard let client = model.client, let lib = model.currentLibraryId, let id = cast.tmdbId else { return }
        do {
            person = try await client.get("/api/library/person", query: [("lib", lib), ("id", String(id))])
        } catch {
            self.error = errorText(error)
        }
    }

    private func open(_ kind: String, _ id: Int) {
        dismiss()
        model.openLibraryTitle(kind: kind, tmdbId: id)
    }

    /// "Born": "April 4, 1965 (age 61)", and the rest of their facts.
    private func facts(_ p: PersonDetail) -> [(String, String)] {
        let parse = DateFormatter()
        parse.dateFormat = "yyyy-MM-dd"
        parse.locale = Locale(identifier: "en_US_POSIX")
        let show = DateFormatter()
        show.dateStyle = .long
        func years(_ a: Date, _ b: Date) -> Int { Calendar.current.dateComponents([.year], from: a, to: b).year ?? 0 }
        let born = p.birthday.flatMap { parse.date(from: $0) }
        let died = p.deathday.flatMap { parse.date(from: $0) }
        var out: [(String, String)] = []
        if let k = p.knownFor, !k.isEmpty { out.append(("Known for", k)) }
        if let born { out.append(("Born", show.string(from: born) + (died == nil ? " (age \(years(born, Date())))" : ""))) }
        if let died { out.append(("Died", show.string(from: died) + (born.map { " (aged \(years($0, died)))" } ?? ""))) }
        if let place = p.placeOfBirth, !place.isEmpty { out.append(("Place of birth", place)) }
        return out
    }

    /// "2012 · Tony Stark · In this library"
    private func filmLine(_ f: PersonFilm) -> String {
        [f.year.map(String.init) ?? "Upcoming", f.role.flatMap { $0.isEmpty ? nil : $0 }, f.inLibrary == true ? "In this library" : nil]
            .compactMap { $0 }.joined(separator: " · ")
    }
}
