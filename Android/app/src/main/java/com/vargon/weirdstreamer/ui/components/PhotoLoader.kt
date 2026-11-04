package com.vargon.weirdstreamer.ui.components

import android.content.Context
import android.util.Log
import com.vargon.weirdstreamer.LivePhotoInfo
import com.vargon.weirdstreamer.LivePhotoType
import com.vargon.weirdstreamer.LivePhotoUtil
import com.vargon.weirdstreamer.MediaViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

/**
 * A photo the viewers (TV and phone) show: downloaded to [image], and a motion photo's clip in
 * [clip] (copied out of the photo, or an Apple Live Photo's .MOV). Both are temp files of its own;
 * [delete] removes them once the viewer moves on.
 */
class LoadedPhoto(val image: File, val clip: File?, val info: LivePhotoInfo?) {
    fun delete() {
        image.delete()
        clip?.delete()
    }
}

/**
 * Downloads the photo at [path] and finds its motion clip, off the main thread (motion photos are
 * read through to their end). Null if it can't be downloaded. A load cancelled partway (the
 * viewer moved on) leaves no files behind.
 */
suspend fun loadPhoto(viewModel: MediaViewModel, path: String, context: Context): LoadedPhoto? {
    val image = viewModel.downloadToTempFile(path, context, "." + viewModel.getFileExtension(path)) ?: return null
    var clip: File? = null
    try {
        clip = viewModel.liveVideoFile(path, context)
        val photo = withContext(Dispatchers.IO) {
            clip?.let { return@withContext LoadedPhoto(image, it, LivePhotoInfo(LivePhotoType.APPLE, 0, it.length())) }
            val info = LivePhotoUtil.findLivePhotoInfo(image) ?: return@withContext LoadedPhoto(image, null, null)
            clip = File.createTempFile("clip_", ".mp4", context.cacheDir)
            copyRange(image, info.videoOffset, info.videoLength, clip!!)
            if (clip!!.length() > 0) LoadedPhoto(image, clip, info)
            else LoadedPhoto(image, null, null).also { clip?.delete(); clip = null }
        }
        clip = null // the photo has it now
        return photo
    } catch (e: Exception) {
        // Cancelled (the viewer moved on) or failed: nothing is kept but the photo, if it can show.
        clip?.delete()
        if (e is kotlinx.coroutines.CancellationException) {
            image.delete()
            throw e
        }
        Log.e("PhotoLoader", "Couldn't read the motion clip of $path", e)
        return LoadedPhoto(image, null, null)
    }
}

private fun copyRange(from: File, offset: Long, length: Long, to: File) {
    from.inputStream().use { input ->
        var toSkip = offset
        while (toSkip > 0) {
            val skipped = input.skip(toSkip)
            if (skipped <= 0) break
            toSkip -= skipped
        }
        to.outputStream().use { output ->
            val buffer = ByteArray(64 * 1024)
            var left = length
            while (left > 0) {
                val n = input.read(buffer, 0, minOf(buffer.size.toLong(), left).toInt())
                if (n < 0) break
                output.write(buffer, 0, n)
                left -= n
            }
        }
    }
}
