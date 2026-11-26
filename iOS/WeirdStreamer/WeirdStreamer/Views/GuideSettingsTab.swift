import SwiftUI

// The Guide tab of Settings (admins): typed channels (a type, its libraries, up to how many channels),
// each library's Show in Guide and Use for Guide Filler, and the schedule. Saved with its button
// (PUT /api/admin/guide), apart from the Server tab. Android's is PhoneGuideSettings.kt.

private let primeTimes = [("20", "8 to 11 pm"), ("19", "7 to 10 pm")]

/// A typed channel definition being edited.
private struct TypedEdit: Identifiable {
    let id = UUID()
    var serverID: String // "" for a new one
    var type: String
    var libraries: [String]
    var max: Int
    var number: Int
    var channels: [GuideAdminChannel]
}

/// A library's Guide switches being edited.
private struct GuideLibraryEdit: Identifiable {
    var id: String { library.id }
    let library: GuideAdminLibrary
    var showInGuide: String
    var guideFiller: Bool
    var airs: Bool { library.type == "movie" || library.type == "tv" } // only those take Show in Guide
}

func guideLibraryKind(_ type: String) -> String {
    switch type { case "movie": "Movies"; case "tv": "TV Shows"; default: "Folders" }
}

struct GuideSettingsTab: View {
    @Environment(AppModel.self) private var model
    @State private var data: GuideAdmin?
    @State private var loadError: String?
    @State private var typed: [TypedEdit] = []
    @State private var libraries: [GuideLibraryEdit] = []
    @State private var primeTime = "20"
    @State private var weatherZip = ""
    @State private var message: (String, Bool)?
    @State private var saving = false
    @State private var replanAsk = false
    @State private var replanning = false

    var body: some View {
        Group {
            if data == nil, let loadError {
                PhoneMessage(text: loadError)
            } else if let data {
                form(data)
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task { await load() }
        .alert("Plan again from now?", isPresented: $replanAsk) {
            Button("Plan again", role: .destructive) { Task { await replan() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("What each channel airs now stays. Everything after it, the next 6 hours too, is planned anew, so the Guide everyone sees changes.")
        }
    }

    /// Plans every channel again from its next program (POST /api/admin/guide/replan).
    private func replan() async {
        guard let client = model.client else { return }
        replanning = true
        defer { replanning = false }
        do {
            let reply = try await client.json("POST", "/api/admin/guide/replan") as? [String: Any]
            message = (reply?["message"] as? String ?? "Planned again.", false)
            await load()
        } catch {
            model.handle(error)
            message = (errorText(error), true)
        }
    }

    private func form(_ data: GuideAdmin) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                SectionTitle(text: "Typed channels")
                Text("Channels made only from the libraries you pick, of one type, numbered from 100. Only the groups that can open every one of their libraries see them, and those libraries are in no other channel. Late Night airs everything in its libraries, all day; the other types take their TMDB titles of that genre.")
                    .font(.caption).foregroundStyle(Palette.muted)
                ForEach($typed) { $t in typedCard($t, data) }
                Button { addTyped(data) } label: { Label("Add typed channels", systemImage: "plus") }
                    .buttonStyle(PhoneSecondaryButtonStyle())
                    .disabled(typed.count >= data.maxTyped)

                SectionTitle(text: "Libraries")
                Text("Show in Guide puts a movie or TV library's titles on the channels, by network and genre. Use for Guide Filler lends its videos of 15 minutes or less to the gaps between programs.")
                    .font(.caption).foregroundStyle(Palette.muted)
                ForEach($libraries) { $l in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(l.library.name).font(.headline)
                        if inTyped(l.library.id) {
                            Text("On its typed channels").font(.caption).foregroundStyle(Palette.muted)
                        } else if l.airs {
                            OptionMenu(label: "Show in Guide", options: libraryRowModes, value: $l.showInGuide)
                        } else {
                            Text("\(guideLibraryKind(l.library.type)): only typed channels (Late Night) take it")
                                .font(.caption).foregroundStyle(Palette.muted)
                        }
                        Toggle("Use for Guide Filler", isOn: $l.guideFiller)
                        Divider().overlay(Palette.muted.opacity(0.35))
                    }
                    .padding(.top, 6)
                }

                SectionTitle(text: "Schedule")
                OptionMenu(label: "Prime time (then the news hour)", options: primeTimes, value: $primeTime)
                PhoneTextField(label: "Weather ZIP code", text: $weatherZip, placeholder: "10001", keyboard: .numberPad,
                               hint: "The Weather channel's forecast is the National Weather Service's for it (US only)")
                Text(Self.extras(tracks: data.musicTracks, voiceOvers: data.voiceOvers)).font(.caption).foregroundStyle(Palette.muted)

                MessageLine(message: message)
                Button { Task { await save() } } label: {
                    Label(saving ? "Saving…" : "Save Guide settings", systemImage: "square.and.arrow.down").frame(maxWidth: .infinity)
                }
                .buttonStyle(PhoneButtonStyle())
                .disabled(saving)
                .padding(.top, 8)

                SectionTitle(text: "Plan again")
                Text("The Guide plans by itself every hour and never changes the next 6 hours. This plans every channel again from its next program, the next 6 hours too: what's on now stays. Only when you press it.")
                    .font(.caption).foregroundStyle(Palette.muted)
                Button { replanAsk = true } label: { Text(replanning ? "Planning…" : "Plan again from now") }
                    .buttonStyle(PhoneSecondaryButtonStyle())
                    .disabled(replanning)
            }
            .padding(16)
        }
    }

    private func typedCard(_ t: Binding<TypedEdit>, _ data: GuideAdmin) -> some View {
        CardBox {
            HStack {
                OptionMenu(label: "Type", options: data.types.map { ($0.id, $0.name) }, value: t.type)
                IconButton(system: "trash", label: "Remove", tint: Palette.error) { typed.removeAll { $0.id == t.wrappedValue.id } }
            }
            OptionMenu(label: "Channels", options: (1...max(1, data.maxChannels)).map { ("\($0)", "Up to \($0)") },
                       value: Binding(get: { "\(t.wrappedValue.max)" }, set: { t.wrappedValue.max = Int($0) ?? data.maxChannels }))
            Text("Libraries").font(.subheadline).foregroundStyle(Palette.muted)
            ForEach(data.libraries, id: \.id) { l in
                let other = typed.contains { $0.id != t.wrappedValue.id && $0.libraries.contains(l.id) }
                Toggle(isOn: Binding(
                    get: { t.wrappedValue.libraries.contains(l.id) },
                    set: { on in
                        if on { if !other { t.wrappedValue.libraries.append(l.id) } } else { t.wrappedValue.libraries.removeAll { $0 == l.id } }
                    })) {
                    VStack(alignment: .leading) {
                        Text(l.name)
                        Text("\(guideLibraryKind(l.type)) · \(l.groups.isEmpty ? "no group" : l.groups.joined(separator: ", "))" + (other ? " · another typed channel has it" : ""))
                            .font(.caption).foregroundStyle(Palette.muted)
                    }
                }
                .disabled(other && !t.wrappedValue.libraries.contains(l.id))
            }
            let groups = groupsFor(t.wrappedValue, data)
            Text(groups == nil ? "Choose its libraries." : groups!.isEmpty ? "No group can open all of these libraries: no one will see it." : "Seen by: \(groups!.joined(separator: ", ")).")
                .font(.caption).foregroundStyle(groups?.isEmpty == false ? Palette.muted : Palette.error)
            let chans = t.wrappedValue.channels.map { "\($0.number) \($0.name)" }.joined(separator: ", ")
            Text((t.wrappedValue.number > 0 ? "Numbers \(t.wrappedValue.number)–\(t.wrappedValue.number + 9). " : "Numbered when saved. ")
                 + (chans.isEmpty ? "Its channels are made at the next planning, as its content allows." : "Now: \(chans)."))
                .font(.caption).foregroundStyle(Palette.muted)
        }
    }

    /// The groups that see a typed channel: those that can open every one of its libraries (nil: none chosen).
    private func groupsFor(_ t: TypedEdit, _ data: GuideAdmin) -> [String]? {
        var out: [String]?
        for id in t.libraries {
            let g = data.libraries.first { $0.id == id }?.groups ?? []
            out = out.map { $0.filter { g.contains($0) } } ?? g
        }
        return out
    }

    private func inTyped(_ id: String) -> Bool { typed.contains { $0.libraries.contains(id) } }

    private func addTyped(_ data: GuideAdmin) {
        typed.append(TypedEdit(serverID: "", type: data.types.first?.id ?? "latenight", libraries: [], max: data.maxChannels,
                               number: 0, channels: []))
    }

    static func extras(tracks: Int, voiceOvers: Bool) -> String {
        let music = tracks > 0 ? "Music: \(tracks) track\(tracks == 1 ? "" : "s")." : "Music: none (the setup's Guide step adds it)."
        let voices = voiceOvers ? "Voice-overs: on." : "Voice-overs: off (Piper isn't set up; the setup's Guide step adds it)."
        return "\(music) \(voices)"
    }

    private func adopt(_ g: GuideAdmin) {
        data = g
        primeTime = g.primeTime > 0 ? "\(g.primeTime)" : "20"
        weatherZip = g.weatherZip
        typed = g.typedChannels.map { TypedEdit(serverID: $0.id, type: $0.type, libraries: $0.libraries, max: $0.max,
                                                number: $0.number, channels: $0.channels) }
        libraries = g.libraries.map { GuideLibraryEdit(library: $0, showInGuide: $0.showInGuide.isEmpty ? "off" : $0.showInGuide,
                                                       guideFiller: $0.guideFiller) }
    }

    private func load() async {
        guard let client = model.client else { return }
        do {
            adopt(try await client.get("/api/admin/guide"))
        } catch {
            model.handle(error)
            loadError = errorText(error)
        }
    }

    private func save() async {
        guard let client = model.client else { return }
        let zip = weatherZip.trimmingCharacters(in: .whitespaces)
        if !zip.isEmpty && (zip.count != 5 || !zip.allSatisfy(\.isNumber)) {
            message = ("The weather ZIP code is five digits.", true)
            return
        }
        let body = GuideAdminSave(primeTime: Int(primeTime) ?? 20, weatherZip: zip,
            libraries: libraries.map { .init(id: $0.library.id, showInGuide: $0.showInGuide == "off" ? "" : $0.showInGuide, guideFiller: $0.guideFiller) },
            typedChannels: typed.map { .init(id: $0.serverID, type: $0.type, libraries: $0.libraries, max: $0.max) })
        saving = true
        defer { saving = false }
        do {
            let saved: GuideAdmin = try await client.callSnake("PUT", "/api/admin/guide", body: body)
            adopt(saved)
            message = ("Saved. The Guide plans again now.", false)
        } catch {
            model.handle(error)
            message = (errorText(error), true)
        }
    }
}
