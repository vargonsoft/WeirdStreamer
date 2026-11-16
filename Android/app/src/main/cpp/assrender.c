// The ASS/SSA subtitle renderer: libass behind a few JNI calls for AssRenderer.kt. A renderer
// holds a script (replaced whole as the server's windows arrive), the video's fonts and the size
// it draws at, and draws a moment of the script into an Android bitmap (RGBA, premultiplied).

#include <jni.h>
#include <android/bitmap.h>
#include <android/log.h>
#include <stdarg.h>
#include <stdlib.h>
#include <string.h>
#include <ass/ass.h>

#define TAG "AssRender"

typedef struct {
    ASS_Library *library;
    ASS_Renderer *renderer;
    ASS_Track *track;
    char *default_font; // a font file used for any face the script names that isn't loaded
    int fonts_dirty;    // fonts were added since libass last looked
} Context;

static void log_libass(int level, const char *fmt, va_list args, void *data) {
    (void) data;
    if (level <= 2) __android_log_vprint(ANDROID_LOG_WARN, TAG, fmt, args); // errors and warnings
}

static void apply_fonts(Context *c) {
    if (!c->fonts_dirty) return;
    // No system font provider: the fonts libass has are the video's own, added from memory, and
    // the default font for everything else. A glyph none of them has (a Chinese, Japanese or Korean
    // character) is looked for in the default family, which is the CJK subset's (the server's, added
    // as fonts): libass's own fallback needs a provider. Family names it doesn't know are skipped.
    ass_set_fonts(c->renderer, c->default_font, "Noto Sans CJK JP", ASS_FONTPROVIDER_NONE, NULL, 0);
    c->fonts_dirty = 0;
}

JNIEXPORT jlong JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeCreate(JNIEnv *env, jclass cls, jstring defaultFont) {
    (void) cls;
    Context *c = calloc(1, sizeof(Context));
    if (!c) return 0;
    c->library = ass_library_init();
    if (!c->library) { free(c); return 0; }
    ass_set_message_cb(c->library, log_libass, NULL);
    ass_set_extract_fonts(c->library, 1); // fonts in a script's own [Fonts] section
    c->renderer = ass_renderer_init(c->library);
    if (!c->renderer) { ass_library_done(c->library); free(c); return 0; }
    if (defaultFont) {
        const char *path = (*env)->GetStringUTFChars(env, defaultFont, NULL);
        c->default_font = strdup(path);
        (*env)->ReleaseStringUTFChars(env, defaultFont, path);
    }
    c->fonts_dirty = 1;
    return (jlong) (intptr_t) c;
}

JNIEXPORT void JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeAddFont(JNIEnv *env, jclass cls, jlong handle,
                                                                       jstring name, jbyteArray data) {
    (void) cls;
    Context *c = (Context *) (intptr_t) handle;
    const char *n = (*env)->GetStringUTFChars(env, name, NULL);
    jsize size = (*env)->GetArrayLength(env, data);
    jbyte *bytes = (*env)->GetByteArrayElements(env, data, NULL);
    ass_add_font(c->library, n, (const char *) bytes, size); // libass keeps its own copy
    (*env)->ReleaseByteArrayElements(env, data, bytes, JNI_ABORT);
    (*env)->ReleaseStringUTFChars(env, name, n);
    c->fonts_dirty = 1;
}

// Replaces the script (the whole of it, header and every line loaded so far).
JNIEXPORT jboolean JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeSetScript(JNIEnv *env, jclass cls, jlong handle,
                                                                         jbyteArray script) {
    (void) cls;
    Context *c = (Context *) (intptr_t) handle;
    jsize size = (*env)->GetArrayLength(env, script);
    char *copy = malloc((size_t) size + 1);
    if (!copy) return JNI_FALSE;
    (*env)->GetByteArrayRegion(env, script, 0, size, (jbyte *) copy);
    copy[size] = 0;
    ASS_Track *track = ass_read_memory(c->library, copy, (size_t) size, NULL);
    free(copy);
    if (!track) return JNI_FALSE;
    if (c->track) ass_free_track(c->track);
    c->track = track;
    return JNI_TRUE;
}

// frame: the bitmap's size, which covers the picture; storage: the video's own size, for its
// aspect (libass scales the script's PlayRes to the frame).
JNIEXPORT void JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeSetSize(JNIEnv *env, jclass cls, jlong handle,
                                                                       jint width, jint height,
                                                                       jint videoWidth, jint videoHeight) {
    (void) env; (void) cls;
    Context *c = (Context *) (intptr_t) handle;
    ass_set_frame_size(c->renderer, width, height);
    if (videoWidth > 0 && videoHeight > 0) ass_set_storage_size(c->renderer, videoWidth, videoHeight);
}

// Blends one of libass's images (an alpha map in one colour) over the bitmap.
static void blend(const ASS_Image *img, uint8_t *pixels, uint32_t stride, uint32_t width, uint32_t height) {
    uint32_t color = img->color;
    uint32_t opacity = 255 - (color & 0xFF);
    if (opacity == 0) return;
    uint32_t r = color >> 24, g = (color >> 16) & 0xFF, b = (color >> 8) & 0xFF;
    for (int y = 0; y < img->h; y++) {
        int py = img->dst_y + y;
        if (py < 0 || (uint32_t) py >= height) continue;
        const uint8_t *src = img->bitmap + (size_t) y * img->stride;
        uint8_t *dst = pixels + (size_t) py * stride;
        for (int x = 0; x < img->w; x++) {
            int px = img->dst_x + x;
            if (px < 0 || (uint32_t) px >= width) continue;
            uint32_t k = src[x] * opacity / 255; // this pixel's coverage, times the colour's opacity
            if (k == 0) continue;
            uint8_t *p = dst + (size_t) px * 4;
            uint32_t keep = 255 - k;
            p[0] = (uint8_t) ((r * k + p[0] * keep) / 255);
            p[1] = (uint8_t) ((g * k + p[1] * keep) / 255);
            p[2] = (uint8_t) ((b * k + p[2] * keep) / 255);
            p[3] = (uint8_t) (k + p[3] * keep / 255);
        }
    }
}

// Draws the script at timeMs into bitmap (the frame size). Returns 0 when nothing changed since
// the last call (the bitmap is left as it is, unless force), 1 when it was drawn, -1 on an error.
JNIEXPORT jint JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeRender(JNIEnv *env, jclass cls, jlong handle,
                                                                      jlong timeMs, jobject bitmap, jboolean force) {
    (void) cls;
    Context *c = (Context *) (intptr_t) handle;
    if (!c->track) return -1;
    apply_fonts(c);
    int change = 0;
    ASS_Image *images = ass_render_frame(c->renderer, c->track, (long long) timeMs, &change);
    if (change == 0 && !force) return 0;
    AndroidBitmapInfo info;
    void *pixels;
    if (AndroidBitmap_getInfo(env, bitmap, &info) < 0 || info.format != ANDROID_BITMAP_FORMAT_RGBA_8888) return -1;
    if (AndroidBitmap_lockPixels(env, bitmap, &pixels) < 0) return -1;
    for (uint32_t y = 0; y < info.height; y++) memset((uint8_t *) pixels + (size_t) y * info.stride, 0, (size_t) info.width * 4);
    for (const ASS_Image *img = images; img; img = img->next) blend(img, pixels, info.stride, info.width, info.height);
    AndroidBitmap_unlockPixels(env, bitmap);
    return 1;
}

JNIEXPORT void JNICALL
Java_com_vargon_weirdstreamer_ui_components_AssRenderer_nativeDestroy(JNIEnv *env, jclass cls, jlong handle) {
    (void) env; (void) cls;
    Context *c = (Context *) (intptr_t) handle;
    if (!c) return;
    if (c->track) ass_free_track(c->track);
    ass_renderer_done(c->renderer);
    ass_library_done(c->library);
    free(c->default_font);
    free(c);
}
