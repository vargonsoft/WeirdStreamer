# Build script for Weird Streamer

echo "Building Weird Streamer..."

# Run from Server/, wherever this script is called from
cd "$(dirname "$0")/.."

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
mkdir -p bin
go -C src mod download

# Build server
echo "Building server..."
CGO_ENABLED=1 go -C src build -o ../bin/server ./cmd/server
if [ $? -ne 0 ]; then
    echo "Error: Failed to build server"
    exit 1
fi

# Build CLI
echo "Building CLI tool..."
CGO_ENABLED=1 go -C src build -o ../bin/cli ./cmd/cli
if [ $? -ne 0 ]; then
    echo "Error: Failed to build CLI"
    exit 1
fi

echo ""
echo "Build successful!"
echo ""
echo "Next steps:"
echo "1. Create config.json: scripts/create-config.sh (or copy config.example.json and edit it)"
echo "2. Create an admin user: ./bin/cli user create admin --admin --group parents"
echo "3. Start the server: ./start-server.sh"
echo ""
