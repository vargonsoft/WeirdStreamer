import SwiftUI

/// The source list, as Android's: each server in its own box, its card on top (the logo faded
/// behind its icon, name, address and kind) and its accounts beneath as round buttons, then
/// "Add user". Tapping an account opens the server as it; holding one offers to sign it out.
struct SourceListView: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        ScrollView {
            LazyVStack(spacing: 8) {
                if model.servers.isEmpty {
                    Text("No servers yet. Use + to add one.")
                        .font(.subheadline).foregroundStyle(Palette.muted)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(16)
                }
                ForEach(model.servers, id: \.address) { server in
                    Panel { ServerAccountsRow(address: server.address) }
                }
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
        }
    }
}

struct ServerAccountsRow: View {
    @Environment(AppModel.self) private var model
    let address: String

    var body: some View {
        let _ = model.settingsVersion // sign-outs redraw the row
        let accounts = model.accounts(of: address)
        let name = model.serverTitle(address)
        VStack(alignment: .leading, spacing: 12) {
            ZStack {
                Palette.raised
                Image("LogoMark").resizable().scaledToFill().opacity(0.12)
                HStack(spacing: 16) {
                    Image(systemName: "dot.radiowaves.left.and.right")
                        .font(.system(size: 26)).foregroundStyle(Palette.accent).frame(width: 32)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(name).font(.headline).lineLimit(1)
                        if name != address {
                            Text(address).font(.caption).foregroundStyle(Palette.muted).lineLimit(1)
                        }
                        Text("Weird Streamer").font(.caption.weight(.medium)).foregroundStyle(Palette.accent)
                    }
                    Spacer()
                }
                .padding(.horizontal, 16)
            }
            .frame(height: 96)
            .clipShape(RoundedRectangle(cornerRadius: 12))
            .padding(.horizontal, 12)

            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: 12) {
                    ForEach(accounts) { account in
                        AccountButton(label: account.username, note: account.signedIn ? nil : "Signed out") {
                            Text(String(account.username.first(where: { $0.isLetter || $0.isNumber }) ?? "?").uppercased())
                                .font(.title2)
                        } action: {
                            model.chooseAccount(address, account.username)
                        }
                        .contextMenu {
                            Button("Sign out \(account.username)") { model.signOut(address, account.username) }
                        }
                    }
                    AccountButton(label: "Add user") {
                        Image(systemName: "plus").font(.system(size: 26))
                    } action: {
                        model.addAccount(address)
                    }
                }
                .padding(.horizontal, 12)
            }
        }
        .padding(.vertical, 12)
        .task(id: address) { await model.refreshServerName(address) }
    }
}

/// A round account button with its name below (plum, with a cream letter).
struct AccountButton<Face: View>: View {
    let label: String
    var note: String? = nil
    @ViewBuilder let face: Face
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(spacing: 6) {
                face
                    .foregroundStyle(Palette.buttonLabel)
                    .frame(width: 64, height: 64)
                    .background(Palette.button, in: Circle())
                Text(label).font(.caption).lineLimit(1).foregroundStyle(Palette.text)
                if let note {
                    Text(note).font(.caption2).foregroundStyle(Palette.muted).lineLimit(1)
                }
            }
            .frame(width: 76)
        }
        .buttonStyle(.plain)
    }
}
