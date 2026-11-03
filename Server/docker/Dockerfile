# Builds the server and CLI, and ffmpeg from source with the libraries the server uses, then runs
# them on a small image. scripts/check-ffmpeg.sh runs at the end of the ffmpeg stage and in the
# final image, so the build fails if a feature is missing.

ARG FFMPEG_VERSION=8.1.3
ARG FFMPEG_SHA256=7138d28c96d9d3e3af4ee3d8cad72741f8ffb40da90c1112235dea3ecd3178a3
ARG ALPINE_VERSION=3.24.3

# Go 1.24.4 matches src/go.mod
FROM golang:1.24.4-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Module files first, so the dependency download is cached until they change
COPY src/go.mod src/go.sum ./src/
RUN go -C src mod download

COPY src ./src
RUN CGO_ENABLED=1 go -C src build -o /out/server ./cmd/server \
 && CGO_ENABLED=1 go -C src build -o /out/cli ./cmd/cli

# ffmpeg, built from the pinned release. Each library enabled here has a check in check-ffmpeg.sh.
FROM alpine:${ALPINE_VERSION} AS ffmpeg

ARG FFMPEG_VERSION
ARG FFMPEG_SHA256

RUN apk add --no-cache build-base nasm pkgconf curl xz \
        x264-dev libass-dev chromaprint-dev libdvdread-dev libdvdnav-dev zlib-dev

WORKDIR /src
RUN curl -fsSL -o ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
 && echo "${FFMPEG_SHA256}  ffmpeg.tar.xz" | sha256sum -c - \
 && tar -xJf ffmpeg.tar.xz \
 && rm ffmpeg.tar.xz

WORKDIR /src/ffmpeg-${FFMPEG_VERSION}
RUN ./configure --prefix=/opt/ffmpeg \
        --enable-gpl --enable-version3 \
        --enable-libx264 --enable-libass --enable-chromaprint \
        --enable-libdvdread --enable-libdvdnav --enable-zlib \
        --disable-debug --disable-doc --disable-ffplay \
 && make -j"$(nproc)" \
 && make install

COPY scripts/check-ffmpeg.sh /opt/ffmpeg/check-ffmpeg.sh
RUN FFMPEG=/opt/ffmpeg/bin/ffmpeg LD_LIBRARY_PATH=/opt/ffmpeg/lib sh /opt/ffmpeg/check-ffmpeg.sh

FROM alpine:${ALPINE_VERSION}

# The libraries ffmpeg was built against, at run time
# Fonts for burned-in subtitles (libass draws them with fontconfig)
RUN apk add --no-cache ca-certificates x264-libs libass chromaprint libdvdread libdvdnav zlib fontconfig font-dejavu

WORKDIR /app

COPY --from=ffmpeg /opt/ffmpeg /opt/ffmpeg
ENV PATH=/opt/ffmpeg/bin:$PATH \
    LD_LIBRARY_PATH=/opt/ffmpeg/lib

# The server reads ./config.yml and ./web from here. Its databases, thumbnails and other
# data go under ./var, which docker-compose.yml mounts.
COPY --from=builder /out/server /out/cli /app/
COPY web ./web

RUN FFMPEG=ffmpeg sh /opt/ffmpeg/check-ffmpeg.sh

EXPOSE 8080
CMD ["/app/server"]
