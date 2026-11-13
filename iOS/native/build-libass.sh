#!/usr/bin/env bash
# Builds the iPhone app's ASS subtitle renderer (assrender.c, include/assrender.h) and what it
# links: libass and what it needs (FreeType, FriBiDi, HarfBuzz), the same versions as Android's
# (Android/native/build-libass.sh), as static libraries for iPhones (iphoneos) and the simulator
# on Apple silicon (iphonesimulator), into iOS/native/build/<platform>/lib (git-ignored), which the
# Xcode project links. libass finds fonts the script names that aren't the video's own through
# CoreText. Run it once before building the app, and again after changing assrender.c (only the
# renderer is compiled again); --clean starts over.
#
# The sources are fetched from their projects' releases and checked against the hashes below.
# Needs: Xcode, cmake, make, pkg-config, curl (macOS with Homebrew: brew install cmake pkg-config).
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
build="$here/build"
min_ios=17.0 # the app's deployment target
platforms=(iphoneos iphonesimulator)

names=(freetype fribidi harfbuzz libass)
url_freetype=https://download.savannah.gnu.org/releases/freetype/freetype-2.15.0.tar.xz
sha_freetype=93cd47eb719c325107f2d3cf087d06299e944c3b48a40b55537302304dea1dd5
url_fribidi=https://github.com/fribidi/fribidi/releases/download/v1.0.17/fribidi-1.0.17.tar.xz
sha_fribidi=6949dcde27d41cebad1fd741fcafc36d55a1020d2d872d4a6eb3914caabbada2
url_harfbuzz=https://github.com/harfbuzz/harfbuzz/releases/download/15.0.0/harfbuzz-15.0.0.tar.xz
sha_harfbuzz=968536e9abd782f07f7954044c0c4020199fcdd39b41d221ec42d2a3be4a44df
url_libass=https://github.com/libass/libass/releases/download/0.17.5/libass-0.17.5.tar.xz
sha_libass=2dca25c0e0c837ddf00b52011b3f82cac1e4ddd3ad018227806b0c2288864acc

if [ "${1:-}" = "--clean" ]; then rm -rf "$build"; shift; fi
[ "${1:-}" = "--help" ] && { sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 0; }

for tool in xcrun cmake make pkg-config curl shasum; do
    command -v "$tool" >/dev/null || { echo "Needs $tool" >&2; exit 1; }
done
jobs="$(sysctl -n hw.ncpu)"

# Sources: fetched once, checked, unpacked.
mkdir -p "$build/src"
for name in "${names[@]}"; do
    url_var="url_$name" sha_var="sha_$name"
    url="${!url_var}" sha="${!sha_var}"
    file="$build/src/$(basename "$url")"
    if [ ! -f "$file" ]; then
        echo "Fetching $url"
        curl -sSfL -o "$file.part" "$url"
        mv "$file.part" "$file"
    fi
    echo "$sha  $file" | shasum -a 256 -c --quiet - || { rm -f "$file"; echo "$file: wrong checksum" >&2; exit 1; }
    dir="$build/src/$name"
    if [ ! -d "$dir" ]; then
        mkdir -p "$dir.part"
        tar -xf "$file" -C "$dir.part" --strip-components=1
        mv "$dir.part" "$dir"
    fi
done

for platform in "${platforms[@]}"; do
    prefix="$build/$platform"
    sdk="$(xcrun --sdk "$platform" --show-sdk-path)"
    case $platform in
        iphoneos) target=arm64-apple-ios$min_ios ;;
        iphonesimulator) target=arm64-apple-ios$min_ios-simulator ;;
    esac
    flags="-target $target -isysroot $sdk -Os -ffunction-sections -fdata-sections"
    dep_flags="$flags -w" # their own warnings are not ours
    cc="$(xcrun --sdk "$platform" -f clang)"
    cxx="$(xcrun --sdk "$platform" -f clang++)"

    if [ ! -f "$prefix/lib/libass.a" ]; then
        echo "== $platform"
        work="$build/work/$platform"
        rm -rf "$work" "$prefix"
        mkdir -p "$work" "$prefix"
        cmake_args=(-DCMAKE_SYSTEM_NAME=iOS -DCMAKE_OSX_SYSROOT="$sdk" -DCMAKE_OSX_ARCHITECTURES=arm64
            -DCMAKE_OSX_DEPLOYMENT_TARGET="$min_ios" -DCMAKE_BUILD_TYPE=MinSizeRel -DBUILD_SHARED_LIBS=OFF
            -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" -DCMAKE_FIND_ROOT_PATH="$prefix"
            -DCMAKE_C_COMPILER="$cc" -DCMAKE_CXX_COMPILER="$cxx"
            -DCMAKE_C_FLAGS="$dep_flags" -DCMAKE_CXX_FLAGS="$dep_flags -fno-exceptions -fno-rtti")
        export PKG_CONFIG_LIBDIR="$prefix/lib/pkgconfig" PKG_CONFIG_PATH=""
        export CC="$cc" CXX="$cxx" CFLAGS="$dep_flags" CXXFLAGS="$dep_flags" LDFLAGS="-target $target -isysroot $sdk"
        export AR="$(xcrun -f ar)" RANLIB="$(xcrun -f ranlib)" CC_FOR_BUILD="$(xcrun --sdk macosx -f clang)"

        # FreeType: fonts only from files and memory, no compressed or bitmap extras, nor Apple's HVF
        # (the SDKs have its header but no library for apps).
        cmake -S "$build/src/freetype" -B "$work/freetype" "${cmake_args[@]}" -DFT_DISABLE_ZLIB=ON \
            -DFT_DISABLE_BZIP2=ON -DFT_DISABLE_PNG=ON -DFT_DISABLE_HARFBUZZ=ON -DFT_DISABLE_BROTLI=ON \
            -DFT_DISABLE_HVF=ON >/dev/null
        cmake --build "$work/freetype" -j "$jobs" >/dev/null
        cmake --install "$work/freetype" >/dev/null

        # HarfBuzz: the shaper only, on FreeType.
        cmake -S "$build/src/harfbuzz" -B "$work/harfbuzz" "${cmake_args[@]}" -DHB_HAVE_FREETYPE=ON \
            -DHB_BUILD_SUBSET=OFF -DHB_BUILD_RASTER=OFF -DHB_BUILD_VECTOR=OFF -DHB_BUILD_GPU=OFF \
            -DHB_BUILD_UTILS=OFF -DHB_HAVE_GLIB=OFF -DHB_HAVE_ICU=OFF -DHB_HAVE_CORETEXT=OFF \
            -DHB_BUILD_TESTS=OFF >/dev/null
        cmake --build "$work/harfbuzz" -j "$jobs" >/dev/null
        cmake --install "$work/harfbuzz" >/dev/null

        # FriBiDi and libass: autotools. libass uses CoreText to find system fonts.
        (cd "$work" && mkdir -p fribidi && cd fribidi && "$build/src/fribidi/configure" --host=aarch64-apple-darwin \
            --prefix="$prefix" --disable-shared --enable-static >/dev/null && make -j "$jobs" -C lib >/dev/null &&
            make -C lib install >/dev/null && make install-pkgconfigDATA >/dev/null)
        (cd "$work" && mkdir -p libass && cd libass && "$build/src/libass/configure" --host=aarch64-apple-darwin \
            --prefix="$prefix" --disable-shared --enable-static --disable-asm --disable-fontconfig \
            --disable-libunibreak --enable-coretext >/dev/null &&
            make -j "$jobs" >/dev/null && make install >/dev/null)
        echo "   $(du -sh "$prefix/lib" | cut -f1) of static libraries"
    fi

    # The renderer.
    out="$prefix/lib/libassrender.a"
    src="$here/assrender.c"
    if [ ! "$out" -nt "$src" ] || [ ! "$out" -nt "$here/include/assrender.h" ] || [ ! "$out" -nt "$prefix/lib/libass.a" ]; then
        mkdir -p "$build/work/$platform"
        "$cc" $flags -O2 -Wall -I"$here/include" -I"$prefix/include" -c "$src" -o "$build/work/$platform/assrender.o"
        rm -f "$out"
        "$(xcrun -f ar)" rcs "$out" "$build/work/$platform/assrender.o"
        echo "$platform: libassrender.a built"
    fi
done
