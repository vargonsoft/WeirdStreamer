#!/usr/bin/env bash
# Builds the app's native library, libassrender.so (app/src/main/cpp/assrender.c: the ASS subtitle
# renderer), for each of the app's processor types, into Android/native/build/jniLibs/<abi>/
# (git-ignored), which the app packages. It links in libass and what it needs (FreeType, FriBiDi,
# HarfBuzz), built here once as static libraries (Android/native/build/<abi>/). Gradle runs it
# before each build (only the renderer is compiled again); --clean starts over.
#
# The sources are fetched from their projects' releases and checked against the hashes below
# (FreeType's against its maintainer's GPG signature when it was pinned).
# Needs: the Android NDK (ANDROID_NDK_HOME, or the version below under ~/Android/Sdk/ndk), cmake,
# make, pkg-config, curl.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
build="$here/build"
ndk_version=29.0.14206865
api=29 # the app's minSdk
abis=(arm64-v8a armeabi-v7a x86_64 x86)

declare -A url sha
url[freetype]=https://download.savannah.gnu.org/releases/freetype/freetype-2.15.0.tar.xz
sha[freetype]=93cd47eb719c325107f2d3cf087d06299e944c3b48a40b55537302304dea1dd5
url[fribidi]=https://github.com/fribidi/fribidi/releases/download/v1.0.17/fribidi-1.0.17.tar.xz
sha[fribidi]=6949dcde27d41cebad1fd741fcafc36d55a1020d2d872d4a6eb3914caabbada2
url[harfbuzz]=https://github.com/harfbuzz/harfbuzz/releases/download/15.0.0/harfbuzz-15.0.0.tar.xz
sha[harfbuzz]=968536e9abd782f07f7954044c0c4020199fcdd39b41d221ec42d2a3be4a44df
url[libass]=https://github.com/libass/libass/releases/download/0.17.5/libass-0.17.5.tar.xz
sha[libass]=2dca25c0e0c837ddf00b52011b3f82cac1e4ddd3ad018227806b0c2288864acc

if [ "${1:-}" = "--clean" ]; then rm -rf "$build"; shift; fi
[ "${1:-}" = "--help" ] && { sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0; }

ndk="${ANDROID_NDK_HOME:-$HOME/Android/Sdk/ndk/$ndk_version}"
[ -d "$ndk" ] || { echo "No Android NDK at $ndk (set ANDROID_NDK_HOME)" >&2; exit 1; }
toolchain="$ndk/toolchains/llvm/prebuilt/linux-x86_64"
for tool in cmake make pkg-config curl; do
    command -v "$tool" >/dev/null || { echo "Needs $tool" >&2; exit 1; }
done
jobs="$(nproc)"

# Sources: fetched once, checked, unpacked.
mkdir -p "$build/src"
for name in freetype fribidi harfbuzz libass; do
    file="$build/src/$(basename "${url[$name]}")"
    if [ ! -f "$file" ]; then
        echo "Fetching ${url[$name]}"
        curl -sSfL -o "$file.part" "${url[$name]}"
        mv "$file.part" "$file"
    fi
    echo "${sha[$name]}  $file" | sha256sum -c --quiet - || { rm -f "$file"; echo "$file: wrong checksum" >&2; exit 1; }
    dir="$build/src/$name"
    if [ ! -d "$dir" ]; then
        mkdir -p "$dir.part"
        tar -xf "$file" -C "$dir.part" --strip-components=1
        mv "$dir.part" "$dir"
    fi
done

for abi in "${abis[@]}"; do
    prefix="$build/$abi"
    [ -f "$prefix/lib/libass.a" ] && continue
    case $abi in
        arm64-v8a) triple=aarch64-linux-android ;;
        armeabi-v7a) triple=armv7a-linux-androideabi ;;
        x86_64) triple=x86_64-linux-android ;;
        x86) triple=i686-linux-android ;;
    esac
    host=${triple/armv7a-linux-androideabi/arm-linux-androideabi}
    echo "== $abi"
    work="$build/work/$abi"
    rm -rf "$work" "$prefix"
    mkdir -p "$work" "$prefix"
    flags="-Os -fPIC -ffunction-sections -fdata-sections"
    cmake_args=(-DCMAKE_TOOLCHAIN_FILE="$ndk/build/cmake/android.toolchain.cmake" -DANDROID_ABI="$abi"
        -DANDROID_PLATFORM="android-$api" -DCMAKE_BUILD_TYPE=MinSizeRel -DBUILD_SHARED_LIBS=OFF
        -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" -DCMAKE_FIND_ROOT_PATH="$prefix"
        -DCMAKE_C_FLAGS="$flags" -DCMAKE_CXX_FLAGS="$flags -fno-exceptions -fno-rtti")
    export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" PKG_CONFIG_PATH=""
    export CC="$toolchain/bin/$triple$api-clang" CXX="$toolchain/bin/$triple$api-clang++"
    export AR="$toolchain/bin/llvm-ar" RANLIB="$toolchain/bin/llvm-ranlib" STRIP="$toolchain/bin/llvm-strip"
    export CFLAGS="$flags" CXXFLAGS="$flags" CC_FOR_BUILD=cc

    # FreeType: fonts only from files and memory, no compressed or bitmap extras.
    cmake -S "$build/src/freetype" -B "$work/freetype" "${cmake_args[@]}" -DFT_DISABLE_ZLIB=ON \
        -DFT_DISABLE_BZIP2=ON -DFT_DISABLE_PNG=ON -DFT_DISABLE_HARFBUZZ=ON -DFT_DISABLE_BROTLI=ON >/dev/null
    cmake --build "$work/freetype" -j "$jobs" >/dev/null
    cmake --install "$work/freetype" >/dev/null

    # HarfBuzz: the shaper only, on FreeType.
    cmake -S "$build/src/harfbuzz" -B "$work/harfbuzz" "${cmake_args[@]}" -DHB_HAVE_FREETYPE=ON \
        -DHB_BUILD_SUBSET=OFF -DHB_BUILD_RASTER=OFF -DHB_BUILD_VECTOR=OFF -DHB_BUILD_GPU=OFF \
        -DHB_BUILD_UTILS=OFF -DHB_HAVE_GLIB=OFF -DHB_HAVE_ICU=OFF -DHB_BUILD_TESTS=OFF >/dev/null
    cmake --build "$work/harfbuzz" -j "$jobs" >/dev/null
    cmake --install "$work/harfbuzz" >/dev/null

    # FriBiDi and libass: autotools.
    (cd "$work" && mkdir -p fribidi && cd fribidi && "$build/src/fribidi/configure" --host="$host" \
        --prefix="$prefix" --disable-shared --enable-static >/dev/null && make -j "$jobs" -C lib >/dev/null &&
        make -C lib install >/dev/null && make install-pkgconfigDATA >/dev/null)
    (cd "$work" && mkdir -p libass && cd libass && "$build/src/libass/configure" --host="$host" \
        --prefix="$prefix" --disable-shared --enable-static --disable-asm --disable-fontconfig \
        --disable-libunibreak --disable-require-system-font-provider >/dev/null &&
        make -j "$jobs" >/dev/null && make install >/dev/null)
    echo "   $(du -sh "$prefix/lib" | cut -f1) of static libraries"
done

# The renderer, linked with them: only its JNI functions are exported, unused code is dropped,
# and the C++ runtime HarfBuzz needs is linked in (static).
src="$here/../app/src/main/cpp/assrender.c"
for abi in "${abis[@]}"; do
    prefix="$build/$abi"
    out="$build/jniLibs/$abi/libassrender.so"
    [ "$out" -nt "$src" ] && [ "$out" -nt "$0" ] && [ "$out" -nt "$prefix/lib/libass.a" ] && continue
    case $abi in
        arm64-v8a) triple=aarch64-linux-android ;;
        armeabi-v7a) triple=armv7a-linux-androideabi ;;
        x86_64) triple=x86_64-linux-android ;;
        x86) triple=i686-linux-android ;;
    esac
    mkdir -p "$(dirname "$out")"
    "$toolchain/bin/$triple$api-clang++" -x c -Os -Wall -fPIC -ffunction-sections -fdata-sections \
        -I"$prefix/include" -shared -o "$out" "$src" -x none -L"$prefix/lib" \
        -lass -lharfbuzz -lfribidi -lfreetype -ljnigraphics -llog -lm -static-libstdc++ \
        -Wl,--gc-sections -Wl,--exclude-libs,ALL -Wl,--as-needed -Wl,-z,max-page-size=16384
    "$toolchain/bin/llvm-strip" --strip-unneeded "$out"
    echo "$abi: libassrender.so $(du -k "$out" | cut -f1) KB"
done
