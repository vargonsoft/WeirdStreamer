# Media Streaming Server Project

This is a Go-based media streaming server with web client for serving videos, images, and PDFs.

## Architecture
- **Backend**: Go server with JWT authentication, media scanning, thumbnail generation, and streaming
- **Web Client**: HTML/JS/CSS frontend for browsing and playing media
- **Database**: SQLite for user accounts and media metadata
- **API**: RESTful with OpenAPI/Swagger documentation

## Key Features
- High-performance streaming with HTTP/2
- Bcrypt password hashing
- Thumbnail generation and caching
- Command-line management tools
- Docker support
