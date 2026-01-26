# Media Streaming Server

A high-performance media streaming server built with Go, featuring JWT authentication, automatic thumbnail generation, and optimized HTTP/2 streaming for videos, images, and PDFs.

## Features

- 🚀 **High-Performance Streaming**: HTTP/2 support with efficient range request handling
- 🔐 **Secure Authentication**: JWT-based authentication with bcrypt password hashing
- 🖼️ **Automatic Thumbnails**: Generate thumbnails for videos and images
- 📱 **RESTful API**: Well-documented API with OpenAPI/Swagger support
- 🌐 **Web Client**: Built-in responsive web interface for browsing and playing media
- 🛠️ **CLI Management**: Command-line tools for user and media management
- 🐳 **Docker Support**: Easy deployment with Docker and Docker Compose
- 📊 **SQLite Database**: Lightweight database for users and media metadata

## Architecture

```
├── cmd/
│   ├── server/          # Main server application
│   └── cli/             # Command-line management tool
├── internal/
│   ├── auth/            # Authentication service
│   ├── config/          # Configuration management
│   ├── database/        # Database layer
│   ├── handlers/        # HTTP handlers
│   ├── media/           # Media scanning and streaming
│   └── models/          # Data models
├── web/                 # Web client (HTML/CSS/JS)
└── config.example.json  # Example configuration
```

## Prerequisites

- **Go 1.21+** (for building from source)
- **FFmpeg** (for video thumbnail generation and metadata extraction)
- **SQLite** (included with Go driver)

### Installing Prerequisites

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install golang-go ffmpeg
```

**Fedora/RHEL:**
```bash
sudo dnf install golang ffmpeg
```

**macOS:**
```bash
brew install go ffmpeg
```

## Quick Start

### 1. Clone and Build

```bash
git clone <repository-url>
cd Server

# Download dependencies
go mod tidy

# Build server and CLI
go build -o server ./cmd/server
go build -o cli ./cmd/cli
```

### 2. Configure

```bash
# Copy example config
cp config.example.json config.json

# Edit config.json and set:
# - JWT secret (change from default!)
# - Media paths
# - Server port
nano config.json
```

**Important**: Change the `jwtSecret` in config.json to a secure random string!

### 3. Create Admin User

```bash
./cli user create admin --admin
# Enter password when prompted
```

### 4. Add Media Directories

```bash
./cli config add-path /path/to/your/videos
./cli config add-path /path/to/your/photos
```

### 5. Scan Media

```bash
./cli scan
```

### 6. Start Server

```bash
./server
```

The server will start on http://localhost:8080 (or your configured port).

## Using the Web Client

1. Open http://localhost:8080 in your browser
2. Login with your admin credentials
3. Browse and play media files
4. Use the "Scan Media" button to refresh the library

## API Documentation

### Authentication

**Login**
```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "your-password"
}
```

**Response:**
```json
{
  "token": "eyJhbGc...",
  "expires_at": 1234567890,
  "user": {
    "id": 1,
    "username": "admin",
    "is_admin": true
  }
}
```

### Media Endpoints

All media endpoints require authentication via Bearer token:
```http
Authorization: Bearer <your-jwt-token>
```

**List Media**
```http
GET /api/media?type=video
```

**Browse Directory**
```http
GET /api/media/browse?path=/subfolder
```

**Stream Media**
```http
GET /api/media/stream?path=/full/path/to/file.mp4
```

**Trigger Scan**
```http
POST /api/media/scan
```

## CLI Commands

### User Management

```bash
# Create user
./cli user create <username> [--admin]

# List users
./cli user list

# Delete user
./cli user delete <username>
```

### Media Management

```bash
# Scan media directories
./cli scan

# Add media directory
./cli config add-path /path/to/media

# List media directories
./cli config list-paths
```

## Docker Deployment

### Build and Run with Docker Compose

```bash
# Edit docker-compose.yml to set your media paths and JWT secret
nano docker-compose.yml

# Start services
docker-compose up -d

# Create admin user
docker-compose exec media-server ./cli user create admin --admin

# Scan media
docker-compose exec media-server ./cli scan
```

### Build Docker Image

```bash
docker build -t media-server .
```

## Android Client Integration

The server provides a RESTful API perfect for Android apps. Here's a quick integration guide:

### 1. Add Dependencies (build.gradle)

```gradle
dependencies {
    implementation 'com.squareup.retrofit2:retrofit:2.9.0'
    implementation 'com.squareup.retrofit2:converter-gson:2.9.0'
    implementation 'com.squareup.okhttp3:okhttp:4.11.0'
}
```

### 2. Define API Interface

```kotlin
interface MediaApi {
    @POST("/api/auth/login")
    suspend fun login(@Body credentials: LoginRequest): LoginResponse
    
    @GET("/api/media")
    suspend fun getMedia(@Query("type") type: String?): List<MediaItem>
    
    @GET("/api/media/stream")
    @Streaming
    fun streamMedia(@Query("path") path: String): Call<ResponseBody>
}
```

### 3. Data Models

```kotlin
data class LoginRequest(val username: String, val password: String)
data class LoginResponse(val token: String, val expires_at: Long, val user: User)
data class MediaItem(
    val id: Long,
    val path: String,
    val name: String,
    val type: String,
    val size: Long,
    val mime_type: String,
    val thumbnail_url: String?,
    val duration: Double?,
    val width: Int?,
    val height: Int?
)
```

### 4. Use ExoPlayer for Video Streaming

```kotlin
val player = ExoPlayer.Builder(context).build()
val mediaItem = MediaItem.fromUri("http://your-server:8080/api/media/stream?path=/video.mp4")
player.setMediaItem(mediaItem)
player.prepare()
player.play()
```

## Configuration Reference

**config.json:**

```json
{
  "server": {
    "port": 8080,
    "host": "0.0.0.0"
  },
  "database": {
    "path": "./data/media.db"
  },
  "media": {
    "paths": ["/path/to/media"],
    "thumbnailPath": "./thumbnails",
    "supportedVideoFormats": [".mp4", ".mkv", ".avi", ".mov"],
    "supportedImageFormats": [".jpg", ".jpeg", ".png", ".gif"],
    "supportedDocFormats": [".pdf"]
  },
  "auth": {
    "jwtSecret": "CHANGE-THIS-TO-RANDOM-SECRET",
    "tokenExpiryHours": 720
  },
  "streaming": {
    "chunkSize": 1048576,
    "enableHTTP2": true
  }
}
```

## Performance Optimization

### For HTTP/2 (Recommended)

Generate self-signed certificates for development:

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

Set `enableHTTP2: true` in config.json and start the server.

### For Maximum Speed

- Use SSD storage for media files
- Enable HTTP/2 with TLS
- Adjust `chunkSize` based on your network (default: 1MB)
- Use appropriate video codecs (H.264/H.265) for web compatibility

## Troubleshooting

**Q: Thumbnails not generating?**
- Ensure FFmpeg is installed: `ffmpeg -version`
- Check file permissions on thumbnail directory
- Check server logs for errors

**Q: Can't stream videos?**
- Verify video codec is web-compatible (H.264)
- Check that file path is within configured media paths
- Ensure proper authentication token is sent

**Q: High CPU usage during scan?**
- Thumbnail generation is CPU-intensive
- Scans run in background
- Subsequent scans only process new files

## Development

### Project Structure

- `cmd/server/main.go` - Server entry point
- `cmd/cli/main.go` - CLI tool entry point
- `internal/auth/` - JWT authentication
- `internal/media/scanner.go` - Media file scanning
- `internal/media/streamer.go` - HTTP range request streaming
- `internal/handlers/` - HTTP request handlers
- `web/` - Frontend web client

### Running Tests

```bash
go test ./...
```

### Building for Production

```bash
# Build with optimizations
CGO_ENABLED=1 go build -ldflags="-s -w" -o server ./cmd/server
CGO_ENABLED=1 go build -ldflags="-s -w" -o cli ./cmd/cli
```

## Security Considerations

1. **Change JWT Secret**: Always use a strong, random secret in production
2. **Use HTTPS**: Enable HTTP/2 with TLS for encrypted communication
3. **Firewall**: Restrict access to trusted networks
4. **Regular Updates**: Keep Go and FFmpeg updated
5. **Password Policy**: Enforce strong passwords for users

## License

GNU Affero General Public License v3.0 or later - See LICENSE file for details

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## Support

For issues, questions, or suggestions, please open a GitHub issue.
