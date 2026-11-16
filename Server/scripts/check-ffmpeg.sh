#!/bin/sh
# Checks that ffmpeg has what Weird Streamer calls: the encoders, muxers, demuxers and filters in
# src/internal/media. Written for plain sh so the Docker build can run it too.
# Exits 1 when a required feature is missing. Optional ones only warn.
ff=${FFMPEG:-ffmpeg}
if ! command -v "$ff" >/dev/null 2>&1; then
    echo "ffmpeg isn't installed." >&2
    exit 1
fi

# has LISTING NAME: whether ffmpeg's -LISTING output names NAME (the second column)
has() {
    "$ff" -hide_banner "-$1" 2>/dev/null | awk -v n="$2" '$2 == n { found = 1 } END { exit !found }'
}

missing=0
check() { # check KIND NAME REQUIRED-OR-OPTIONAL WHY
    if has "$1" "$2"; then
        return
    fi
    if [ "$3" = required ]; then
        echo "missing: $2 ($4)" >&2
        missing=1
    else
        echo "warning: $2 is missing ($4); $5" >&2
    fi
}

check encoders libx264 required "transcoding video to H.264"
check encoders aac required "transcoding audio"
check muxers mp4 required "fragmented MP4 output"
check muxers webvtt required "text subtitles as WebVTT"
check muxers framecrc required "finding the keyframe a copied video starts at"
check filters scale required "thumbnails and trickplay frames"
check filters overlay required "burning picture subtitles into a video"
check filters subtitles required "burning subtitles in (needs libass)"
check filters blackdetect required "intro and credits detection"
check demuxers dvdvideo optional "disc images (.iso)" "disc images won't play"
check muxers chromaprint optional "intro detection by audio" "intros aren't detected by sound"

if [ "$missing" = 0 ]; then
    echo "ffmpeg has everything Weird Streamer needs: $("$ff" -version | head -1)"
fi

# Not ffmpeg, and optional (it doesn't change the exit status): the CJK subtitle font and the
# subsetter that makes its subsets (see the README's CJK subtitle font section).
subset="${SUBSET:-pyftsubset}"
if command -v "$subset" >/dev/null; then
    echo "pyftsubset is there ($subset): styled subtitles get Chinese, Japanese and Korean fonts"
else
    echo "Optional: pyftsubset (fontTools) isn't installed; styled subtitles in Chinese, Japanese or Korean show as boxes"
fi
if ! ls /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc \
    /usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.otf \
    /usr/share/fonts/noto-cjk/NotoSansCJK-Regular.otf >/dev/null 2>&1; then
    echo "Optional: no Noto Sans CJK font found (set media.cjkFontPath in config.yml if yours is elsewhere)"
fi
exit "$missing"
