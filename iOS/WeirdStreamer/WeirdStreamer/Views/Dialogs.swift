import SwiftUI

/// A dialog as Android's AlertDialog draws it in the phone theme: a box over a dimmed screen,
/// its title, its content, and its buttons at the bottom right (the quieter one first).
struct PhoneDialog<Content: View, Buttons: View>: View {
    let title: String
    let onDismiss: () -> Void
    @ViewBuilder let content: Content
    @ViewBuilder let buttons: Buttons

    var body: some View {
        ZStack {
            Color.black.opacity(0.5).ignoresSafeArea().onTapGesture(perform: onDismiss)
            VStack(alignment: .leading, spacing: 16) {
                Text(title).font(.title2)
                content
                HStack(spacing: 8) {
                    Spacer()
                    buttons
                }
            }
            .padding(24)
            .frame(maxWidth: 560)
            // Opaque even in themes whose boxes are see-through (Cinema Blue): the page color under the box's.
            .background(Palette.panel, in: RoundedRectangle(cornerRadius: 28))
            .background(Palette.background, in: RoundedRectangle(cornerRadius: 28))
            .padding(.horizontal, 24)
        }
        .foregroundStyle(Palette.text)
    }
}

/// A text field as Android's filled TextField: a raised box with its label above the text.
struct PhoneTextField: View {
    let label: String
    @Binding var text: String
    var placeholder: String = ""
    var secure = false
    var keyboard: UIKeyboardType = .default
    var hint: String? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            VStack(alignment: .leading, spacing: 2) {
                Text(label).font(.caption).foregroundStyle(Palette.accent)
                Group {
                    if secure {
                        SecureField(placeholder, text: $text)
                    } else {
                        TextField(placeholder, text: $text)
                            .keyboardType(keyboard)
                    }
                }
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .font(.body)
            }
            .padding(.horizontal, 16).padding(.vertical, 8)
            .background(Palette.raised, in: UnevenRoundedRectangle(topLeadingRadius: 4, topTrailingRadius: 4))
            .overlay(alignment: .bottom) { Rectangle().fill(Palette.accent).frame(height: 1) }
            if let hint {
                Text(hint).font(.caption).foregroundStyle(Palette.muted).padding(.horizontal, 16)
            }
        }
    }
}

/// The + button's dialog: pick the kind of source, then type its address (Android's
/// PhoneAddSourceDialog). The server is saved without signing in; its accounts are added on its row.
struct AddSourceDialog: View {
    @Environment(AppModel.self) private var model
    let onDismiss: () -> Void

    @State private var picked = false
    @State private var address = ""
    @State private var checking = false
    @State private var error: String?

    var body: some View {
        PhoneDialog(title: picked ? "Add WeirdStreamer Server" : "Select Source Type", onDismiss: onDismiss) {
            if picked {
                PhoneTextField(label: "Server URL", text: $address, placeholder: "myserver or https://myserver:9000",
                               keyboard: .URL,
                               hint: "Without http:// or https://, the server is asked which it uses; without a port too, ports 9000, 443, 80 and 8080 are tried.")
                if let error {
                    Text(error).font(.footnote).foregroundStyle(Palette.error)
                }
            } else {
                Button { picked = true } label: {
                    Text("WeirdStreamer Server").font(.body)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.vertical, 12).padding(.horizontal, 4)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        } buttons: {
            Button(picked ? "Back" : "Cancel") {
                if picked { picked = false } else { onDismiss() }
            }
            .buttonStyle(PhoneSecondaryButtonStyle())
            if picked {
                Button {
                    guard !address.trimmingCharacters(in: .whitespaces).isEmpty else { return }
                    checking = true
                    error = nil
                    Task {
                        error = await model.addServer(address)
                        checking = false
                        if error == nil { onDismiss() }
                    }
                } label: {
                    if checking { ProgressView().tint(Palette.buttonLabel) } else { Text("Add") }
                }
                .buttonStyle(PhoneButtonStyle())
                .disabled(checking)
            }
        }
    }
}

/// Sign in: Add user, or Sign in again for an account whose sign-in ran out (Android's PhoneAuthDialog).
struct AuthDialog: View {
    @Environment(AppModel.self) private var model
    let request: AuthRequest

    @State private var username = ""
    @State private var password = ""
    @State private var working = false

    var body: some View {
        PhoneDialog(title: request.title, onDismiss: cancel) {
            VStack(alignment: .leading, spacing: 8) {
                Text("Host: \(model.serverTitle(request.address))").font(.footnote)
                    .padding(.bottom, 8)
                PhoneTextField(label: "Username", text: $username)
                PhoneTextField(label: "Password", text: $password, secure: true)
                if let error = model.authError {
                    Text(error).font(.footnote).foregroundStyle(Palette.error)
                }
            }
        } buttons: {
            Button("Cancel", action: cancel).buttonStyle(PhoneSecondaryButtonStyle())
            Button {
                working = true
                Task {
                    await model.login(request, username: username, password: password)
                    working = false
                }
            } label: {
                if working { ProgressView().tint(Palette.buttonLabel) } else { Text("Login") }
            }
            .buttonStyle(PhoneButtonStyle())
            .disabled(working || username.isEmpty)
        }
        .onAppear { username = request.username ?? "" }
    }

    private func cancel() {
        model.authError = nil
        model.authRequest = nil
    }
}

/// A video downloaded on this phone, picked from the server: play the download or stream it
/// (Android's PhoneSourceDialog).
struct SourceDialog: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if let choice = model.sourceChoice {
            PhoneDialog(title: choice.name, onDismiss: { model.chooseSource(download: nil) }) {
                VStack(spacing: 4) {
                    Text("This video is downloaded on this phone.").font(.subheadline).foregroundStyle(Palette.muted)
                        .frame(maxWidth: .infinity, alignment: .leading).padding(.bottom, 8)
                    Button { model.chooseSource(download: true) } label: {
                        Label("Play the download", systemImage: "checkmark.circle").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneButtonStyle())
                    Button { model.chooseSource(download: false) } label: {
                        Label("Stream from the server", systemImage: "cloud").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneSecondaryButtonStyle())
                }
            } buttons: {
                Button("Cancel") { model.chooseSource(download: nil) }.buttonStyle(PhoneSecondaryButtonStyle())
            }
        }
    }
}

/// A partly watched video picked in a folder: Resume, or Play from start (Android's PhoneResumeDialog).
struct ResumeDialog: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        if let entry = model.resumeChoice {
            let resumeAt = model.resumeSeconds(entry.path, entry.playback)
            PhoneDialog(title: entry.name, onDismiss: { model.chooseResume(fromStart: nil) }) {
                VStack(spacing: 4) {
                    if let left = timeLeftText(entry.playback, entry.duration) {
                        Text(left).font(.subheadline).foregroundStyle(Palette.muted)
                            .frame(maxWidth: .infinity, alignment: .leading).padding(.bottom, 8)
                    }
                    Button { model.chooseResume(fromStart: false) } label: {
                        Label("Resume from \(clockText(resumeAt))", systemImage: "play.fill").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneButtonStyle())
                    Button { model.chooseResume(fromStart: true) } label: {
                        Label("Play from start", systemImage: "arrow.counterclockwise").frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PhoneSecondaryButtonStyle())
                }
            } buttons: {
                Button("Cancel") { model.chooseResume(fromStart: nil) }.buttonStyle(PhoneSecondaryButtonStyle())
            }
        }
    }
}
