# Build script for Media Streaming Server

echo "Building Media Streaming Server..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go 1.21 or higher."
    echo "Visit: https://golang.org/doc/install"
    exit 1
fi

# Check if FFmpeg is installed
if ! command -v ffmpeg &> /dev/null; then
    echo "Warning: FFmpeg is not installed. Video thumbnail generation will not work."
    echo "Install FFmpeg: sudo apt install ffmpeg (Ubuntu/Debian)"
fi

# Download dependencies
echo "Downloading dependencies..."
go mod download

# Build server
echo "Building server..."
CGO_ENABLED=1 go build -o server ./cmd/server
if [ $? -ne 0 ]; then
    echo "Error: Failed to build server"
    exit 1
fi

# Build CLI
echo "Building CLI tool..."
CGO_ENABLED=1 go build -o cli ./cmd/cli
if [ $? -ne 0 ]; then
    echo "Error: Failed to build CLI"
    exit 1
fi

echo ""
echo "Build successful!"
echo ""
echo "Next steps:"
echo "1. Copy config: cp config.example.json config.json"
echo "2. Edit config.json and set your JWT secret and media paths"
echo "3. Create admin user: ./cli user create admin --admin"
echo "4. Add media paths: ./cli config add-path /path/to/media"
echo "5. Scan media: ./cli scan"
echo "6. Start server: ./server"
echo ""
