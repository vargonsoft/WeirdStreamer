#!/usr/bin/env bash
# Builds the server's own ffmpeg and ffprobe into Server/ffmpeg/bin (git-ignored), with what
# scripts/check-ffmpeg.sh asks for. Homebrew's ffmpeg lacks the DVD reader (dvdvideo, from
# libdvdnav and libdvdread), the chromaprint muxer (intro detection by sound) and the subtitles
# filter (libass). live.sh and run-e2e.sh put Server/ffmpeg/bin first on the PATH when it's there.
#
# macOS with Homebrew: ./scripts/build-ffmpeg.sh [version]   (default below; about 10 minutes)
# The libraries come from Homebrew; the build happens in Server/ffmpeg-build (git-ignored).
set -euo pipefail
version=${1:-8.0.1}
cd "$(dirname "$0")/.."
server=$(pwd)

brew install pkgconf x264 x265 dav1d libvpx opus lame libdvdnav libdvdread chromaprint libass >/dev/null
export PKG_CONFIG_PATH="$(brew --prefix)/lib/pkgconfig:${PKG_CONFIG_PATH:-}"

mkdir -p ffmpeg-build
cd ffmpeg-build
tarball="ffmpeg-$version.tar.xz"
if [ ! -d "ffmpeg-$version" ]; then
    curl -fsSLO "https://ffmpeg.org/releases/$tarball"
    tar xf "$tarball"
fi
cd "ffmpeg-$version"
./configure --prefix="$server/ffmpeg" \
    --enable-gpl --enable-version3 \
    --enable-libx264 --enable-libx265 --enable-libdav1d --enable-libvpx \
    --enable-libopus --enable-libmp3lame \
    --enable-libdvdnav --enable-libdvdread --enable-chromaprint --enable-libass \
    --enable-videotoolbox --enable-audiotoolbox \
    --disable-doc --disable-ffplay \
    --extra-cflags="-I$(brew --prefix)/include" --extra-ldflags="-L$(brew --prefix)/lib"
make -j"$(sysctl -n hw.ncpu)"
make install
FFMPEG="$server/ffmpeg/bin/ffmpeg" "$server/scripts/check-ffmpeg.sh"
