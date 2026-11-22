import Foundation

/// The server's graphics hardware for transcodes (admins' Settings): GET /api/admin/hardware.
struct HardwareReport: Decodable {
    struct GPU: Decodable { let name: String; let vendor: String; let device: String? }
    struct Encoder: Decodable { let id: String; let device: String?; let name: String; let gpu: String; let vendor: String }
    struct Problem: Decodable { let gpu: String; let encoder: String; let reason: String }
    let checking: Bool
    let gpus: [GPU]?
    let encoders: [Encoder]?
    let problems: [Problem]?
}

/// Hardware encoding's choices and texts, as Android's HardwareSettings and the web's: the CPU or
/// an encoder that passed the server's test encode ("id|device"), kept in the config's
/// streaming.hardwareEncoder, hardwareDevice and hardwareMaxTranscodes.
enum HardwareSettings {
    static func key(_ e: HardwareReport.Encoder) -> String { e.id + "|" + (e.device ?? "") }

    static func key(encoder: String, device: String) -> String { encoder.isEmpty ? "" : encoder + "|" + device }

    /// The choice the config's values stand for (NVENC has no device).
    static func chosenKey(_ r: HardwareReport, encoder: String, device: String) -> String {
        if encoder.isEmpty { return "" }
        if let m = (r.encoders ?? []).first(where: { $0.id == encoder && ($0.id == "nvenc" || ($0.device ?? "") == device) }) {
            return key(m)
        }
        return key(encoder: encoder, device: device)
    }

    static func choices(_ r: HardwareReport, encoder: String, device: String) -> [(String, String)] {
        var list: [(String, String)] = [("", "The CPU (x264)")]
        list += (r.encoders ?? []).map { (key($0), "\($0.name) on \($0.gpu)") }
        let chosen = chosenKey(r, encoder: encoder, device: device)
        if !list.contains(where: { $0.0 == chosen }) {
            list.append((chosen, "\(encoder) (not working on this server: the CPU encodes)"))
        }
        return list
    }

    static func status(_ r: HardwareReport) -> String {
        if r.checking { return "Checking the server's graphics hardware…" }
        let gpus = (r.gpus ?? []).map(\.name).joined(separator: ", ")
        let problems = (r.problems ?? []).map { "\($0.encoder) on \($0.gpu): \($0.reason)" }.joined(separator: ". ")
        if (r.encoders ?? []).isEmpty {
            return (gpus.isEmpty ? "No graphics hardware was found, so " :
                    "Graphics: \(gpus). None of it can encode with this server's ffmpeg, so ") +
                "transcodes are encoded by the CPU (x264)." + (problems.isEmpty ? "" : " \(problems).")
        }
        return "Graphics: \(gpus)." + (problems.isEmpty ? "" : " Not usable: \(problems).")
    }

    static func note(_ r: HardwareReport, chosen: String) -> String {
        if chosen.isEmpty { return "Transcodes are encoded by the CPU." }
        let vendor = (r.encoders ?? []).first(where: { key($0) == chosen })?.vendor
        return "The GPU encodes the video; decoding, scaling and burned-in subtitles stay on the CPU. " +
            "A transcode the GPU fails to start is encoded by the CPU, as are those past the limit. " +
            (vendor == "nvidia" ? "NVIDIA's GeForce cards also limit how many encodes run at once. " : "") +
            "Downloads are still encoded by the CPU."
    }
}
