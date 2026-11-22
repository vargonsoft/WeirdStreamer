package com.vargon.weirdstreamer.downloads

import com.vargon.weirdstreamer.api.models.CodecsInfo
import com.vargon.weirdstreamer.api.models.SubtitleTrack
import com.vargon.weirdstreamer.ui.components.languageCode

/**
 * The audio and subtitle languages a download is made with, chosen in the download dialog
 * (two-letter codes, as the account's language; subtitles also [OFF]), and the tracks they are in a
 * file. A language the file doesn't have, or none chosen, leaves it as playing it would: the
 * server's pick for the account's languages (the codecs report's preferred tracks), else the file's
 * default audio and no subtitles but forced ones.
 */
object DownloadLanguages {
    /** No subtitles (forced ones still show). */
    const val OFF = "off"

    /** Subtitles the players draw: text, and PGS pictures (DVD ones would need a transcode). */
    fun drawn(t: SubtitleTrack) = t.text || com.vargon.weirdstreamer.ui.components.drawnAsPgs(t)

    /** The audio track (the server's index) in [language] in [c], else the one playing would pick (-1: the default). */
    fun audioTrack(c: CodecsInfo, language: String?): Int {
        val matches = c.audio.orEmpty().filter { language != null && languageCode(it.language) == language }
        if (matches.isEmpty()) return c.preferredAudio
        return (matches.firstOrNull { it.index == c.preferredAudio } ?: matches.firstOrNull { it.isDefault } ?: matches.first()).index
    }

    /** The subtitle track (the server's index) to show for [language] in [c] ([OFF]: -1), else the one playing would pick. */
    fun subtitleTrack(c: CodecsInfo, language: String?): Int {
        if (language == OFF) return -1
        val matches = c.subtitles.orEmpty().filter { !it.forced && drawn(it) && language != null && languageCode(it.language) == language }
        if (matches.isEmpty()) return c.preferredSubtitle
        return (matches.firstOrNull { it.index == c.preferredSubtitle } ?: matches.firstOrNull { it.isDefault }
            ?: matches.firstOrNull { !it.hearingImpaired } ?: matches.first()).index
    }

    /** The audio languages of [files], in order of first appearance. */
    fun audioLanguages(files: List<CodecsInfo>): List<String> =
        files.flatMap { c -> c.audio.orEmpty().mapNotNull { languageCode(it.language) } }.distinct()

    /** The languages of [files]' full subtitles (not forced) the players draw. */
    fun subtitleLanguages(files: List<CodecsInfo>): List<String> =
        files.flatMap { c -> c.subtitles.orEmpty().filter { !it.forced && drawn(it) }.mapNotNull { languageCode(it.language) } }.distinct()

    /** The audio language the dialog starts with: the last chosen, if [options] has it, else what playing would pick. */
    fun startAudio(files: List<CodecsInfo>, options: List<String>): String? {
        DownloadStore.lastAudioLanguage?.takeIf { it in options }?.let { return it }
        val c = files.firstOrNull { it.preferredAudio >= 0 } ?: files.firstOrNull() ?: return null
        val track = c.audio.orEmpty().let { list ->
            list.firstOrNull { it.index == c.preferredAudio } ?: list.firstOrNull { it.isDefault } ?: list.firstOrNull()
        }
        return languageCode(track?.language)?.takeIf { it in options }
    }

    /** The subtitle language the dialog starts with: the last chosen ([OFF] or one [options] has), else what playing would pick. */
    fun startSubtitles(files: List<CodecsInfo>, options: List<String>): String {
        DownloadStore.lastSubtitleLanguage?.takeIf { it == OFF || it in options }?.let { return it }
        val c = files.firstOrNull { it.preferredSubtitle >= 0 } ?: return OFF
        return languageCode(c.subtitles.orEmpty().firstOrNull { it.index == c.preferredSubtitle }?.language)?.takeIf { it in options } ?: OFF
    }
}
