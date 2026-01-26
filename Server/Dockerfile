FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ffmpeg-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build server and CLI
RUN CGO_ENABLED=1 go build -o server ./cmd/server
RUN CGO_ENABLED=1 go build -o cli ./cmd/cli

FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ffmpeg ca-certificates

WORKDIR /app

# Copy binaries
COPY --from=builder /build/server /app/
COPY --from=builder /build/cli /app/
COPY --from=builder /build/web /app/web

# Create directories
RUN mkdir -p /app/data /app/thumbnails

# Expose port
EXPOSE 8080

# Run server
CMD ["/app/server"]
