package com.vargon.weirdstreamer.ui.components

import com.google.gson.JsonObject
import com.vargon.weirdstreamer.api.models.HardwareEncoder
import com.vargon.weirdstreamer.api.models.HardwareReport

/**
 * Hardware encoding (admins' Settings, phone and TV, as the web's Streaming settings): what
 * graphics hardware the server found (GET /api/admin/hardware) and the choice among the encoders
 * that passed its test encode, kept in the config's streaming.hardwareEncoder, hardwareDevice and
 * hardwareMaxTranscodes.
 */
object HardwareSettings {
    /** A choice's key: "" for the CPU, else "id|device". */
    fun key(e: HardwareEncoder) = e.id + "|" + (e.device ?: "")

    fun key(encoder: String, device: String) = if (encoder.isEmpty()) "" else "$encoder|$device"

    /** The choices: the CPU, each working encoder, and a chosen one that isn't working now. */
    fun choices(report: HardwareReport, encoder: String, device: String): List<Pair<String, String>> {
        val list = mutableListOf("" to "The CPU (x264)")
        report.encoders.orEmpty().forEach { list += key(it) to "${it.name} on ${it.gpu}" }
        val chosen = chosenKey(report, encoder, device)
        if (list.none { it.first == chosen }) list += chosen to "$encoder (not working on this server: the CPU encodes)"
        return list
    }

    /** The choice the config's values stand for (NVENC has no device). */
    fun chosenKey(report: HardwareReport, encoder: String, device: String): String {
        if (encoder.isEmpty()) return ""
        val match = report.encoders.orEmpty().firstOrNull { it.id == encoder && (it.id == "nvenc" || (it.device ?: "") == device) }
        return match?.let { key(it) } ?: key(encoder, device)
    }

    /** What the server has: "Graphics: NVIDIA GeForce GTX 1060 6GB." and what doesn't work. */
    fun status(report: HardwareReport): String {
        if (report.checking) return "Checking the server's graphics hardware…"
        val gpus = report.gpus.orEmpty().joinToString(", ") { it.name }
        val problems = report.problems.orEmpty().joinToString(". ") { "${it.encoder} on ${it.gpu}: ${it.reason}" }
        if (report.encoders.isNullOrEmpty()) {
            return (if (gpus.isNotEmpty()) "Graphics: $gpus. None of it can encode with this server's ffmpeg, so "
                else "No graphics hardware was found, so ") + "transcodes are encoded by the CPU (x264)." +
                (if (problems.isNotEmpty()) " $problems." else "")
        }
        return "Graphics: $gpus." + if (problems.isNotEmpty()) " Not usable: $problems." else ""
    }

    /** What the chosen key means for transcodes. */
    fun note(report: HardwareReport, chosen: String): String {
        if (chosen.isEmpty()) return "Transcodes and downloads are encoded by the CPU."
        val vendor = report.encoders.orEmpty().firstOrNull { key(it) == chosen }?.vendor
        return "The GPU encodes the video; decoding, scaling and burned-in subtitles stay on the CPU. " +
            "A transcode the GPU fails to start is encoded by the CPU, as are those past the limit. " +
            (if (vendor == "nvidia") "NVIDIA's GeForce cards also limit how many encodes run at once. " else "") +
            "Downloads are encoded the same way and count toward the limit."
    }

    /** The limit as shown: "No limit" for 0. */
    fun limitLabel(max: Int) = if (max <= 0) "No limit" else "$max at once"

    /** Writes a choice and limit into a config (the streaming section) to send back whole. */
    fun apply(config: JsonObject, chosen: String, max: Int) {
        val streaming = config.getAsJsonObject("streaming") ?: JsonObject().also { config.add("streaming", it) }
        streaming.addProperty("hardwareEncoder", chosen.substringBefore('|', ""))
        streaming.addProperty("hardwareDevice", if (chosen.isEmpty()) "" else chosen.substringAfter('|', ""))
        streaming.addProperty("hardwareMaxTranscodes", max.coerceAtLeast(0))
    }

    fun encoderOf(config: JsonObject) = config.getAsJsonObject("streaming")?.get("hardwareEncoder")?.asString ?: ""
    fun deviceOf(config: JsonObject) = config.getAsJsonObject("streaming")?.get("hardwareDevice")?.asString ?: ""
    fun maxOf(config: JsonObject) = config.getAsJsonObject("streaming")?.get("hardwareMaxTranscodes")?.asInt ?: 0
}
