# Builds the server and CLI, then runs them on a small image with ffmpeg.
# The Go version matches src/go.mod.
FROM golang:1.24.4-alpine AS builder

RUN apk add --no-cache gcc musl-dev ffmpeg-dev

WORKDIR /build

# Module files first, so the dependency download is cached until they change
COPY src/go.mod src/go.sum ./src/
RUN go -C src mod download

COPY src ./src
RUN CGO_ENABLED=1 go -C src build -o /out/server ./cmd/server \
 && CGO_ENABLED=1 go -C src build -o /out/cli ./cmd/cli

FROM alpine:latest

RUN apk add --no-cache ffmpeg ca-certificates

# The server reads ./config.json and ./web from here. Its databases, thumbnails and other
# data go under ./var, which docker-compose.yml mounts.
WORKDIR /app

COPY --from=builder /out/server /out/cli /app/
COPY web ./web

EXPOSE 8080
CMD ["/app/server"]
