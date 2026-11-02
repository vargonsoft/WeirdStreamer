package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap

/**
 * PGS: Blu-ray's picture subtitles, as ffmpeg's raw .sup stream. A file is a list of segments ("PG",
 * its time, a type, a size): palettes, objects (run-length pictures) and compositions (which
 * objects go where), each display set closed by an end segment. A set with no objects clears the
 * screen. Pictures are only decoded when shown: a film's worth would take hundreds of MB.
 */
class PgsObject(val width: Int, val height: Int, val chunks: MutableList<ByteArray>)
class PgsPart(val x: Int, val y: Int, val crop: IntArray?, val obj: PgsObject, val palette: IntArray)

/** What's on screen from [startMs], until the next set: [parts] on a [width]x[height] frame. */
class PgsSet(val startMs: Long, val width: Int, val height: Int, val parts: List<PgsPart>) {
    /** The picture of the whole set, or null when it clears the screen. */
    fun render(): PgsPicture? {
        if (parts.isEmpty()) return null
        val images = parts.map { it to pixels(it) }
        // Only the part of the frame in use: a line of text, not 1920x1080.
        val left = parts.minOf { it.x }
        val top = parts.minOf { it.y }
        val right = parts.maxOf { it.x + (it.crop?.get(2) ?: it.obj.width) }
        val bottom = parts.maxOf { it.y + (it.crop?.get(3) ?: it.obj.height) }
        val w = right - left
        val h = bottom - top
        if (w <= 0 || h <= 0) return null
        val out = IntArray(w * h)
        for ((part, source) in images) {
            val crop = part.crop ?: intArrayOf(0, 0, part.obj.width, part.obj.height)
            for (row in 0 until crop[3]) {
                val y = crop[1] + row
                if (y >= part.obj.height) break
                val copied = minOf(crop[2], part.obj.width - crop[0])
                System.arraycopy(source, y * part.obj.width + crop[0], out, (part.y - top + row) * w + part.x - left, copied)
            }
        }
        return PgsPicture(Bitmap.createBitmap(out, w, h, Bitmap.Config.ARGB_8888), left, top, width, height)
    }

    // Lines run-length coded: a nonzero byte is one pixel of that color; 0 then a flag byte is a run
    // (bit 7: a color follows, else it's clear; bit 6: a longer count), or the line's end.
    private fun pixels(part: PgsPart): IntArray {
        val obj = part.obj
        val out = IntArray(obj.width * obj.height)
        var chunk = 0
        var at = 0
        fun next(): Int {
            while (chunk < obj.chunks.size && at >= obj.chunks[chunk].size) { chunk++; at = 0 }
            return if (chunk < obj.chunks.size) obj.chunks[chunk][at++].toInt() and 0xff else -1
        }
        var x = 0
        var y = 0
        var b = next()
        while (b >= 0 && y < obj.height) {
            if (b != 0) {
                if (x < obj.width) out[y * obj.width + x] = part.palette[b]
                x++
            } else {
                val flags = next()
                if (flags <= 0) { // the line's end
                    x = 0
                    y++
                } else {
                    val length = if (flags and 0x40 != 0) ((flags and 0x3f) shl 8) or next() else flags and 0x3f
                    val color = if (flags and 0x80 != 0) part.palette[next()] else 0
                    repeat(length) { if (x < obj.width) out[y * obj.width + x++] = color }
                }
            }
            b = next()
        }
        return out
    }
}

/** A display set's pixels: [bitmap] at ([x], [y]) on a [frameWidth]x[frameHeight] frame. */
class PgsPicture(val bitmap: Bitmap, val x: Int, val y: Int, val frameWidth: Int, val frameHeight: Int)

private const val PALETTE = 0x14
private const val OBJECT = 0x15
private const val COMPOSITION = 0x16
private const val END = 0x80

/** An ARGB color from a palette entry, in the BT.709 limited range Blu-rays use. */
private fun pgsColor(y: Int, cr: Int, cb: Int, alpha: Int): Int {
    val luma = 1.164f * (y - 16)
    val red = cr - 128
    val blue = cb - 128
    fun clamp(v: Float) = v.toInt().coerceIn(0, 255)
    return (alpha shl 24) or (clamp(luma + 1.793f * red) shl 16) or
        (clamp(luma - 0.533f * red - 0.213f * blue) shl 8) or clamp(luma + 2.112f * blue)
}

/** Decodes a .sup stream into its display sets. */
fun decodePgs(data: ByteArray): List<PgsSet> {
    fun u8(at: Int) = data[at].toInt() and 0xff
    fun u16(at: Int) = (u8(at) shl 8) or u8(at + 1)
    val palettes = HashMap<Int, IntArray>()
    val objects = HashMap<Int, PgsObject>()
    val sets = ArrayList<PgsSet>()
    class Placed(val id: Int, val x: Int, val y: Int, val crop: IntArray?)
    class Composition(val ms: Long, val width: Int, val height: Int, val palette: Int, val placed: List<Placed>)
    var composition: Composition? = null
    var i = 0
    while (i + 13 <= data.size) {
        if (u16(i) != 0x5047) break // "PG"
        val pts = (u16(i + 2).toLong() shl 16) or u16(i + 4).toLong()
        val type = u8(i + 10)
        val p = i + 13
        i = p + u16(i + 11)
        if (i > data.size) break
        when (type) {
            PALETTE -> {
                val table = palettes[u8(p)]?.copyOf() ?: IntArray(256)
                var q = p + 2
                while (q + 5 <= i) {
                    table[u8(q)] = pgsColor(u8(q + 1), u8(q + 2), u8(q + 3), u8(q + 4))
                    q += 5
                }
                palettes[u8(p)] = table
            }
            OBJECT -> {
                val id = u16(p)
                if (u8(p + 3) and 0x80 != 0) { // the first of its fragments
                    objects[id] = PgsObject(u16(p + 7), u16(p + 9), mutableListOf(data.copyOfRange(p + 11, i)))
                } else objects[id]?.chunks?.add(data.copyOfRange(p + 4, i))
            }
            COMPOSITION -> {
                val placed = ArrayList<Placed>()
                var q = p + 11
                repeat(u8(p + 10)) {
                    if (q + 8 > i) return@repeat
                    val cropped = u8(q + 3) and 0x80 != 0
                    placed += Placed(u16(q), u16(q + 4), u16(q + 6),
                        if (cropped) intArrayOf(u16(q + 8), u16(q + 10), u16(q + 12), u16(q + 14)) else null)
                    q += if (cropped) 16 else 8
                }
                composition = Composition(pts / 90, u16(p), u16(p + 2), u8(p + 9), placed)
            }
            END -> composition?.let { c ->
                val palette = palettes[c.palette]
                val parts = if (palette == null) emptyList() else c.placed.mapNotNull { placed ->
                    objects[placed.id]?.let { PgsPart(placed.x, placed.y, placed.crop, it, palette) }
                }
                sets += PgsSet(c.ms, c.width, c.height, parts)
                composition = null
            }
        }
    }
    return sets
}
