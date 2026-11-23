package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.api.models.BackupList
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * Backups' choices and texts for an admin's Server settings, as the web's and the iPhone's: the
 * config's database.backupSchedule (daily, weekly or off), backupDay, backupTime and backupKeep;
 * the backups themselves are GET/POST /api/admin/backups.
 */
object BackupSettings {
    val schedules = listOf("daily" to "Every day", "weekly" to "Every week", "off" to "Only when asked")
    val days = listOf("sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday")
        .map { it to it.replaceFirstChar(Char::uppercase) }

    /** A time of day as the server takes it ("4:00" too). */
    fun validTime(t: String): Boolean {
        val (h, m) = t.split(':').takeIf { it.size == 2 } ?: return false
        return h.toIntOrNull() in 0..23 && m.length == 2 && m.toIntOrNull() in 0..59
    }

    /** The time as the server keeps it ("04:00"), or the default for one that isn't a time. */
    fun time(t: String): String = if (validTime(t)) t.padStart(5, '0') else "04:00"

    /** "In /srv/ws/var/backups. The next one: Tue 24 Nov, 04:00." */
    fun where(list: BackupList): String {
        val next = list.next?.let { date(it, FormatStyle.SHORT) }?.let { " The next one: $it." } ?: " Only Back up now makes them."
        val count = list.backups.orEmpty().size
        return "In ${list.folder}.$next" + if (count == 0) " None yet." else ""
    }

    /** A backup's time on this phone's calendar and clock. */
    fun date(rfc3339: String, style: FormatStyle = FormatStyle.MEDIUM): String = runCatching {
        OffsetDateTime.parse(rfc3339).atZoneSameInstant(ZoneId.systemDefault())
            .format(DateTimeFormatter.ofLocalizedDateTime(style, FormatStyle.SHORT))
    }.getOrDefault(rfc3339)
}
