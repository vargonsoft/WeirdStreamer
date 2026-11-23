#!/usr/bin/env bash
# Builds the server's own ffmpeg and ffprobe into Server/ffmpeg/bin (git-ignored), with what
# scripts/check-ffmpeg.sh asks for. Homebrew's ffmpeg lacks the DVD reader (dvdvideo, from
# libdvdnav and libdvdread), the chromaprint muxer (intro detection by sound) and the subtitles
# filter (libass). live.sh and run-e2e.sh put Server/ffmpeg/bin first on the PATH when it's there.
#
# macOS with Homebrew: ./scripts/build-ffmpeg.sh [version]   (default below; about 10 minutes)
# The libraries come from Homebrew; the build happens in ffmpeg-build beside the install folder.
# FFMPEG_PREFIX installs it elsewhere: a release package's setup.sh builds it into the package's
# ffmpeg folder, whose bin the server puts first on its PATH (media.UseBundledTools).
set -euo pipefail
version=${1:-8.0.1}
here=$(cd "$(dirname "$0")" && pwd)
prefix=${FFMPEG_PREFIX:-$here/../ffmpeg}
mkdir -p "$prefix"
prefix=$(cd "$prefix" && pwd)
cd "$prefix/.."

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
./configure --prefix="$prefix" \
    --enable-gpl --enable-version3 \
    --enable-libx264 --enable-libx265 --enable-libdav1d --enable-libvpx \
    --enable-libopus --enable-libmp3lame \
    --enable-libdvdnav --enable-libdvdread --enable-chromaprint --enable-libass \
    --enable-videotoolbox --enable-audiotoolbox \
    --disable-doc --disable-ffplay \
    --extra-cflags="-I$(brew --prefix)/include" --extra-ldflags="-L$(brew --prefix)/lib"
make -j"$(sysctl -n hw.ncpu)"
make install
FFMPEG="$prefix/bin/ffmpeg" "$here/check-ffmpeg.sh"
