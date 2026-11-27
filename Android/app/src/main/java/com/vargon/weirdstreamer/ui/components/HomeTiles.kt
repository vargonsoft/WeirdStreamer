package com.vargon.weirdstreamer.ui.components

import com.vargon.weirdstreamer.repository.FileItem

/**
 * The server's home screen's libraries: the TV Shows and Movies libraries first (by name), the Guide's
 * tile after them when there's a Guide, then the others (by name) from the next row. The Guide's tile and
 * the row's end are items of their own here, which the browsers draw (the web's renderMedia, the iPhone's
 * HomeTiles do the same).
 */
object HomeTiles {
    const val GUIDE = "\u0001guide"
    private const val END = "\u0001end:" // what's left of the typed row: empty cells (the TV) or one row break (the phone)

    fun isGuide(item: FileItem) = item.path == GUIDE
    fun isRowEnd(item: FileItem) = item.path.startsWith(END)
    /** The Guide's tile or the row's end: not a library (no metadata, no A–Z letter, no Fix). */
    fun isTile(item: FileItem) = item.path.startsWith("\u0001")

    /**
     * [items] arranged for the home screen. [columns]: the grid's columns, so the typed row ends with
     * empty cells (the TV's grid moves focus by index); 0 for one row-break item (the phone's grid, which
     * spans it across the row); null for none (lists).
     */
    fun arrange(items: List<FileItem>, guide: Boolean, columns: Int?): List<FileItem> {
        val up = items.filter { it.name == ".." || it.path == "UP" } // the phone's way back to the sources stays first
        val typed = items.filter { it.libraryType == "tv" || it.libraryType == "movie" }.sortedBy { it.name.lowercase() }
        val rest = items.filter { it !in typed && it !in up }
        val first = typed + if (guide) listOf(FileItem(name = "Guide", path = GUIDE, isDirectory = true)) else emptyList()
        if (first.isEmpty()) return up + rest
        val ends = when {
            columns == null || rest.isEmpty() -> emptyList()
            columns == 0 -> listOf(FileItem(name = "", path = END + "0", isDirectory = false))
            else -> List((columns - (up.size + first.size) % columns) % columns) { FileItem(name = "", path = "$END$it", isDirectory = false) }
        }
        return up + first + ends + rest
    }
}
