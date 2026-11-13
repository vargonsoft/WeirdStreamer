// The ASS/SSA subtitle renderer (include/assrender.h): Android's assrender.c without JNI. Fonts the
// script names that aren't the video's own come from the system through CoreText.

#include "assrender.h"

#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ass/ass.h>

struct AssRender {
    ASS_Library *library;
    ASS_Renderer *renderer;
    ASS_Track *track;
    int fonts_dirty; // fonts were added since libass last looked
};

static void log_libass(int level, const char *fmt, va_list args, void *data) {
    (void) data;
    if (level > 2) return; // errors and warnings only
    fputs("[libass] ", stderr);
    vfprintf(stderr, fmt, args);
    fputc('\n', stderr);
}

static void apply_fonts(AssRender *r) {
    if (!r->fonts_dirty) return;
    // The video's fonts (added from memory) first, then the system's through CoreText.
    ass_set_fonts(r->renderer, NULL, "Helvetica", ASS_FONTPROVIDER_AUTODETECT, NULL, 0);
    r->fonts_dirty = 0;
}

AssRender *assr_create(void) {
    AssRender *r = calloc(1, sizeof(AssRender));
    if (!r) return NULL;
    r->library = ass_library_init();
    if (!r->library) { free(r); return NULL; }
    ass_set_message_cb(r->library, log_libass, NULL);
    ass_set_extract_fonts(r->library, 1); // fonts in a script's own [Fonts] section
    r->renderer = ass_renderer_init(r->library);
    if (!r->renderer) { ass_library_done(r->library); free(r); return NULL; }
    r->fonts_dirty = 1;
    return r;
}

void assr_destroy(AssRender *r) {
    if (!r) return;
    if (r->track) ass_free_track(r->track);
    ass_renderer_done(r->renderer);
    ass_library_done(r->library);
    free(r);
}

void assr_add_font(AssRender *r, const char *name, const void *data, size_t size) {
    ass_add_font(r->library, name, (const char *) data, (int) size); // libass keeps its own copy
    r->fonts_dirty = 1;
}

int assr_set_script(AssRender *r, const char *data, size_t size) {
    char *copy = malloc(size + 1); // ass_read_memory may change the buffer
    if (!copy) return 0;
    memcpy(copy, data, size);
    copy[size] = 0;
    ASS_Track *track = ass_read_memory(r->library, copy, size, NULL);
    free(copy);
    if (!track) return 0;
    if (r->track) ass_free_track(r->track);
    r->track = track;
    return 1;
}

void assr_set_size(AssRender *r, int width, int height, int video_width, int video_height) {
    ass_set_frame_size(r->renderer, width, height);
    if (video_width > 0 && video_height > 0) ass_set_storage_size(r->renderer, video_width, video_height);
}

// Blends one of libass's images (an alpha map in one colour) over the buffer, premultiplied.
static void blend(const ASS_Image *img, uint8_t *pixels, int stride, int width, int height) {
    uint32_t color = img->color;
    uint32_t opacity = 255 - (color & 0xFF);
    if (opacity == 0) return;
    uint32_t r = color >> 24, g = (color >> 16) & 0xFF, b = (color >> 8) & 0xFF;
    for (int y = 0; y < img->h; y++) {
        int py = img->dst_y + y;
        if (py < 0 || py >= height) continue;
        const uint8_t *src = img->bitmap + (size_t) y * img->stride;
        uint8_t *dst = pixels + (size_t) py * stride;
        for (int x = 0; x < img->w; x++) {
            int px = img->dst_x + x;
            if (px < 0 || px >= width) continue;
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

int assr_render(AssRender *r, long long time_ms, uint8_t *pixels, int width, int height, int stride, int force) {
    if (!r->track) return -1;
    apply_fonts(r);
    int change = 0;
    ASS_Image *images = ass_render_frame(r->renderer, r->track, time_ms, &change);
    if (change == 0 && !force) return 0;
    for (int y = 0; y < height; y++) memset(pixels + (size_t) y * stride, 0, (size_t) width * 4);
    for (const ASS_Image *img = images; img; img = img->next) blend(img, pixels, stride, width, height);
    return 1;
}
