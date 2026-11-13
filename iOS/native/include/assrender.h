// The ASS/SSA subtitle renderer for the iPhone app (AssRenderer.swift): libass behind a few calls,
// as Android's assrender.c. A renderer holds a script (replaced whole as the server's windows
// arrive), the video's fonts and the size it draws at, and draws a moment of the script into an
// RGBA buffer (premultiplied alpha). Not thread-safe: one caller at a time.
#ifndef ASSRENDER_H
#define ASSRENDER_H

#include <stddef.h>
#include <stdint.h>

typedef struct AssRender AssRender;

AssRender *assr_create(void);
void assr_destroy(AssRender *r);

// A font (the video's own, by its file name), kept by libass.
void assr_add_font(AssRender *r, const char *name, const void *data, size_t size);

// Replaces the script (the whole of it). Returns 0 if libass can't read it.
int assr_set_script(AssRender *r, const char *data, size_t size);

// frame: the buffer's size, which covers the picture; video: the video's own size (its aspect).
void assr_set_size(AssRender *r, int width, int height, int video_width, int video_height);

// Draws the script at time_ms into pixels (width × height, stride bytes a row). Returns 0 when
// nothing changed since the last call (pixels untouched, unless force), 1 when drawn, -1 on an error.
int assr_render(AssRender *r, long long time_ms, uint8_t *pixels, int width, int height, int stride, int force);

#endif
