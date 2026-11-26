package com.vargon.weirdstreamer.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp

/** The trickplay memory table (phone and TV): its summary, then a row for the server's memory and one for an NVIDIA
 *  card's, with what the settings need with 4K videos, what there is, and whether it fits. */
@Composable
internal fun TrickplayMemoryTable(table: TrickplayTable, style: TextStyle, modifier: Modifier = Modifier) {
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    Column(modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(table.summary, style = style, color = muted)
        if (table.rows.isEmpty()) return@Column
        @Composable
        fun line(what: String, needs: String, has: String, note: String, warn: Boolean, head: Boolean) {
            val weight = if (head) FontWeight.SemiBold else null
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(what, style = style, color = muted, fontWeight = weight, modifier = Modifier.weight(1.3f))
                Text(needs, style = style, color = muted, fontWeight = weight, textAlign = TextAlign.End, modifier = Modifier.weight(0.9f))
                Text(has, style = style, color = muted, fontWeight = weight, textAlign = TextAlign.End, modifier = Modifier.weight(1.4f))
                Text(note, style = style, fontWeight = weight, modifier = Modifier.weight(1.4f),
                    color = if (warn) MaterialTheme.colorScheme.error else muted)
            }
        }
        line("", "Needs with 4K", "Has", "", false, true)
        table.rows.forEach { line(it.what, it.needs, it.has, it.note, it.warn, false) }
    }
}
