import SwiftUI

// Collections admins make in a movie library (Android's PhoneUserCollections.kt): a name, an optional
// description and the library's movies by release date, shown as a collage of the first four
// posters. Everyone who sees the library sees them; admins create, edit and delete them.

extension LibraryTitle {
    var ref: CollectionTitle { CollectionTitle(kind: kind, tmdbId: tmdbId) }
}

func movieCount(_ n: Int) -> String { "\(n) movie\(n == 1 ? "" : "s")" }

/// A collage of up to four posters: one fills it, two side by side, three one tall and two stacked, four 2×2.
struct Collage: View {
    let posters: [String]

    var body: some View {
        let p = Array(posters.prefix(4))
        GeometryReader { geo in
            let gap: CGFloat = 2
            let half = (geo.size.width - gap) / 2
            let halfH = (geo.size.height - gap) / 2
            switch p.count {
            case 0: Color.clear
            case 1: cell(p[0]).frame(width: geo.size.width, height: geo.size.height)
            case 2:
                HStack(spacing: gap) {
                    cell(p[0]).frame(width: half, height: geo.size.height)
                    cell(p[1]).frame(width: half, height: geo.size.height)
                }
            default:
                HStack(spacing: gap) {
                    if p.count == 3 {
                        cell(p[0]).frame(width: half, height: geo.size.height)
                    } else {
                        VStack(spacing: gap) {
                            cell(p[0]).frame(width: half, height: halfH)
                            cell(p[2]).frame(width: half, height: halfH)
                        }
                    }
                    VStack(spacing: gap) {
                        cell(p[1]).frame(width: half, height: halfH)
                        cell(p[p.count == 3 ? 2 : 3]).frame(width: half, height: halfH)
                    }
                }
            }
        }
    }

    private func cell(_ poster: String) -> some View {
        RemoteImage(relative: poster).clipped()
    }
}

/// Collections view: the collections admins made, as collages; admins start with New collection.
struct UserCollectionsScreen: View {
    @Environment(AppModel.self) private var model
    @State private var creating = false

    var body: some View {
        if let listing = model.libraryListing {
            let collections = listing.userCollections ?? []
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if model.canEditCollections {
                        Button("New collection") { creating = true }.buttonStyle(PhoneButtonStyle())
                    }
                    if collections.isEmpty {
                        Text(model.canEditCollections ? "No collections yet. Make one with New collection, or from a movie’s page."
                             : "No collections yet.").padding(.vertical, 24)
                    }
                    LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                        ForEach(collections) { c in
                            PosterTile(title: c.name, subtitle: movieCount(c.count ?? 0), poster: nil, placeholder: "film",
                                       collage: c.posters ?? []) {
                                model.showUserCollection(id: c.id)
                            }
                        }
                    }
                }
                .padding(12)
            }
            .refreshable { model.loadLibrary() }
            .sheet(isPresented: $creating) {
                CollectionEditor(collection: nil, checked: []) { name, description, titles in
                    model.createUserCollection(name: name, description: description, titles: titles)
                }
            }
        } else if let error = model.libraryError {
            PhoneMessage(text: "Error: \(error)")
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

/// A made collection's page: its name, count and description, then its movies by release date.
/// Admins have Edit (name, description and movies) and Delete; holding a movie offers to take it out.
struct UserCollectionScreen: View {
    @Environment(AppModel.self) private var model
    let collection: UserCollectionDetail
    @State private var editing = false
    @State private var deleting = false
    @State private var removing: LibraryTitle?

    var body: some View {
        let admin = model.canEditCollections
        ScrollView {
            VStack(alignment: .leading, spacing: 4) {
                Text(collection.name).font(.title2.weight(.semibold))
                Text(movieCount(collection.parts.count)).font(.subheadline).foregroundStyle(Palette.muted)
                if let d = collection.description, !d.isEmpty { Text(d).font(.subheadline) }
                if let err = model.collectionError { Text(err).font(.caption).foregroundStyle(Palette.error) }
                if admin {
                    HStack {
                        Button("Edit") { editing = true }.buttonStyle(PhoneButtonStyle())
                        Button("Delete") { deleting = true }.buttonStyle(PhoneSecondaryButtonStyle())
                    }
                    .padding(.top, 4)
                    if !collection.parts.isEmpty {
                        Text("Hold a movie to take it out.").font(.caption).foregroundStyle(Palette.muted)
                    }
                }
                if collection.parts.isEmpty { Text("No movies in it yet.").padding(.vertical, 24) }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12).padding(.top, 12)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 108), spacing: 12)], spacing: 14) {
                ForEach(collection.parts) { t in
                    PosterTile(title: t.title, subtitle: t.year.map(String.init), poster: t.poster, placeholder: "film",
                               progress: t.progress ?? 0, watched: t.allWatched) {
                        model.openLibraryTitle(kind: t.kind, tmdbId: t.tmdbId)
                    }
                    .contextMenu {
                        if admin {
                            Button("Take out of \(collection.name)", role: .destructive) { removing = t }
                        }
                    }
                }
            }
            .padding(12)
        }
        .sheet(isPresented: $editing) {
            let before = Set(collection.parts.map(\.ref))
            CollectionEditor(collection: collection, checked: before) { name, description, titles in
                let now = Set(titles)
                model.updateUserCollection(id: collection.id, name: name, description: description,
                                           add: Array(now.subtracting(before)), remove: Array(before.subtracting(now)))
            }
        }
        .alert("Delete \(collection.name)?", isPresented: $deleting) {
            Button("Cancel", role: .cancel) {}
            Button("Delete", role: .destructive) { model.deleteUserCollection(id: collection.id) }
        } message: {
            Text("The collection goes; its movies stay in the library.")
        }
        .alert(removing.map { "Take \($0.title) out?" } ?? "", isPresented: Binding(get: { removing != nil }, set: { if !$0 { removing = nil } })) {
            Button("Cancel", role: .cancel) { removing = nil }
            Button("Take out", role: .destructive) {
                if let t = removing { model.updateUserCollection(id: collection.id, remove: [t.ref]) }
                removing = nil
            }
        } message: {
            Text("It stays in the library.")
        }
    }
}

/// The collection editor (admins): a name, an optional description and the library's movies to
/// tick, with a filter. `collection` nil makes a new one; `checked` are ticked to start.
struct CollectionEditor: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let collection: UserCollectionDetail?
    let onSave: (String, String, [CollectionTitle]) -> Void
    @State private var name: String
    @State private var description: String
    @State private var filter = ""
    @State private var ticked: Set<CollectionTitle>
    @State private var missingName = false

    init(collection: UserCollectionDetail?, checked: Set<CollectionTitle>, onSave: @escaping (String, String, [CollectionTitle]) -> Void) {
        self.collection = collection
        self.onSave = onSave
        _name = State(initialValue: collection?.name ?? "")
        _description = State(initialValue: collection?.description ?? "")
        _ticked = State(initialValue: checked)
    }

    var body: some View {
        let titles = (model.libraryListing?.titles ?? []).filter {
            filter.trimmingCharacters(in: .whitespaces).isEmpty || $0.title.localizedCaseInsensitiveContains(filter.trimmingCharacters(in: .whitespaces))
        }
        NavigationStack {
            Form {
                Section {
                    TextField("Name", text: $name)
                    TextField("Description (optional)", text: $description, axis: .vertical).lineLimit(2...5)
                    if missingName { Text("A collection needs a name.").font(.caption).foregroundStyle(Palette.error) }
                }
                Section("Movies (\(ticked.count) chosen)") {
                    TextField("Filter", text: $filter)
                    ForEach(titles) { t in
                        Button {
                            if ticked.contains(t.ref) { ticked.remove(t.ref) } else { ticked.insert(t.ref) }
                        } label: {
                            HStack {
                                Image(systemName: ticked.contains(t.ref) ? "checkmark.square.fill" : "square")
                                    .foregroundStyle(Palette.accent)
                                Text(t.title + (t.year.map { " (\($0))" } ?? "")).foregroundStyle(Palette.text)
                            }
                        }
                    }
                }
            }
            .navigationTitle(collection == nil ? "New collection" : "Edit collection")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        let n = name.trimmingCharacters(in: .whitespacesAndNewlines)
                        guard !n.isEmpty else { missingName = true; return }
                        onSave(String(n.prefix(200)), description.trimmingCharacters(in: .whitespacesAndNewlines), Array(ticked))
                        dismiss()
                    }
                }
            }
        }
    }
}

/// A title page's Add to collection (admins, movie libraries): tick the collections it's in, or make a new one.
struct AddToCollectionSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    let title: CollectionTitle
    let name: String
    @State private var creating = false

    var body: some View {
        let collections = model.libraryListing?.userCollections ?? []
        NavigationStack {
            List {
                Section {
                    Text(name).font(.caption).foregroundStyle(Palette.muted)
                    Button("New collection…") { creating = true }
                }
                Section("Collections") {
                    if collections.isEmpty { Text("No collections yet.") }
                    ForEach(collections) { c in
                        let inIt = (c.titles ?? []).contains(title)
                        Button {
                            if inIt { model.updateUserCollection(id: c.id, remove: [title]) }
                            else { model.updateUserCollection(id: c.id, add: [title]) }
                        } label: {
                            HStack {
                                Image(systemName: inIt ? "checkmark.square.fill" : "square").foregroundStyle(Palette.accent)
                                Text("\(c.name) (\(c.count ?? 0))").foregroundStyle(Palette.text)
                            }
                        }
                    }
                }
                if let err = model.collectionError { Text(err).foregroundStyle(Palette.error) }
            }
            .navigationTitle("Add to collection")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .confirmationAction) { Button("Done") { dismiss() } } }
            .sheet(isPresented: $creating) {
                CollectionEditor(collection: nil, checked: [title]) { n, d, titles in
                    model.createUserCollection(name: n, description: d, titles: titles, open: false)
                }
            }
        }
    }
}
