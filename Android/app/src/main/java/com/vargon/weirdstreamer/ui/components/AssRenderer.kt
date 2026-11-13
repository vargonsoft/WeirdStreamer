package com.vargon.weirdstreamer.ui.components

import android.graphics.Bitmap
import java.io.File

/**
 * libass (in libassrender.so, built by native/build-libass.sh): draws an ASS/SSA script, styled,
 * positioned and animated, into a bitmap. It holds the script (replaced whole as more of it loads),
 * the video's fonts and the size it draws at. Not thread-safe: use one from one thread.
 */
class AssRenderer private constructor(private var handle: Long) : AutoCloseable {
    /** Adds a font the script may name (one embedded in the video). */
    fun addFont(name: String, data: ByteArray) = nativeAddFont(check(), name, data)

    /** Replaces the script; false if libass can't read it. */
    fun setScript(script: String): Boolean = nativeSetScript(check(), script.toByteArray())

    /** [width]×[height]: the bitmap's size, covering the picture; the video's own size, for its aspect. */
    fun setSize(width: Int, height: Int, videoWidth: Int, videoHeight: Int) =
        nativeSetSize(check(), width, height, videoWidth, videoHeight)

    /**
     * Draws the script at [timeMs] (its own time) into [bitmap] (ARGB_8888, the size set). Returns
     * false when nothing changed since the last call (the bitmap isn't touched, unless [force]).
     */
    fun render(timeMs: Long, bitmap: Bitmap, force: Boolean = false): Boolean =
        nativeRender(check(), timeMs, bitmap, force) == 1

    override fun close() {
        if (handle != 0L) nativeDestroy(handle)
        handle = 0
    }

    private fun check(): Long = handle.also { check(it != 0L) { "AssRenderer closed" } }

    companion object {
        init { System.loadLibrary("assrender") }

        /** Fonts for any face a script names that isn't loaded: the system's own sans serif. */
        private val defaultFont: String? = listOf("/system/fonts/Roboto-Regular.ttf", "/system/fonts/DroidSans.ttf",
            "/system/fonts/NotoSans-Regular.ttf").firstOrNull { File(it).exists() }

        fun create(): AssRenderer? = nativeCreate(defaultFont).takeIf { it != 0L }?.let { AssRenderer(it) }

        @JvmStatic private external fun nativeCreate(defaultFont: String?): Long
        @JvmStatic private external fun nativeAddFont(handle: Long, name: String, data: ByteArray)
        @JvmStatic private external fun nativeSetScript(handle: Long, script: ByteArray): Boolean
        @JvmStatic private external fun nativeSetSize(handle: Long, width: Int, height: Int, videoWidth: Int, videoHeight: Int)
        @JvmStatic private external fun nativeRender(handle: Long, timeMs: Long, bitmap: Bitmap, force: Boolean): Int
        @JvmStatic private external fun nativeDestroy(handle: Long)
    }
}
