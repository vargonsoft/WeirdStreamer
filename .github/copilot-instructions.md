# WeirdStreamer Project

A hybrid media streaming platform with a Go backend server and Android TV client for SMB network shares.

## Architecture Overview

**Multi-component System:**
- `Server/` - Go-based HTTP/2 media streaming server with JWT auth, thumbnail generation, and RESTful API
- `app/` - Android TV Compose app with SMB client, ExoPlayer video streaming, and Coil image loading
- `web/` - Vanilla JS web client for server browsing (in Server/web/)

**Integration Goal:** Android app currently supports direct SMB share access. **Target: Add Server API as a second media source** to enable:
- Browsing server-indexed media alongside SMB shares
- Streaming through server API (HTTP/2, range requests, transcoding)
- Leveraging server's thumbnail cache and metadata
- See [Server/ANDROID_INTEGRATION.md](Server/ANDROID_INTEGRATION.md) for Retrofit/API integration patterns

## Android App (`app/`)

### Architecture Pattern
- **Single Activity + Compose:** [MainActivity.kt](app/src/main/java/com/vargon/weirdstreamer/MainActivity.kt) switches between full-screen composables (SmbBrowser, VideoPlayer, ImageViewer, PdfViewer, SettingsScreen)
- **ViewModel-centric:** [SmbViewModel.kt](app/src/main/java/com/vargon/weirdstreamer/SmbViewModel.kt) (362 lines) manages all state - navigation, file browsing, credential storage, playback
- **Custom ExoPlayer DataSource:** [SmbDataSource.kt](app/src/main/java/com/vargon/weirdstreamer/SmbDataSource.kt) implements Media3 `BaseDataSource` for streaming SMB files directly to ExoPlayer

### Critical Dependencies
- **jcifs-ng** (`eu.agno3.jcifs:jcifs-ng:2.1.10`) - SMB protocol, requires BaseContext initialization for auth
- **Media3** (`androidx.media3:media3-exoplayer:1.9.0`) - Video playback, needs custom DataSource.Factory
- **Coil** with video/gif/svg extensions - Thumbnails, requires custom [SmbFetcher.kt](app/src/main/java/com/vargon/weirdstreamer/SmbFetcher.kt) and [VideoThumbnailDecoder.kt](app/src/main/java/com/vargon/weirdstreamer/VideoThumbnailDecoder.kt)

### Credential Management
**Encrypted local storage** via [CredentialManager.kt](app/src/main/java/com/vargon/weirdstreamer/CredentialManager.kt):
- Uses `androidx.security:security-crypto` EncryptedSharedPreferences
- Stores per-host credentials, view modes (grid/list), hidden file preferences
- Pattern: `credentialManager.getCredentials(host)` returns `Pair<String?, String?>` (username, password)

### Build & Run
```bash
# From project root
./gradlew :app:assembleDebug  # Output: app/build/outputs/apk/debug/app-debug.apk
./gradlew :app:installDebug   # Install to connected Android TV/device
```

**Gradle Catalog:** Dependencies in [gradle/libs.versions.toml](gradle/libs.versions.toml) using version catalog pattern (`libs.media3.exoplayer`)

## Go Server (`Server/`)

### Core Services
- **Auth Service** ([internal/auth/auth.go](Server/internal/auth/auth.go)) - JWT tokens with `golang-jwt/jwt/v5`, bcrypt passwords, 720h default expiry
- **Media Scanner** ([internal/media/scanner.go](Server/internal/media/scanner.go)) - Recursive directory scanning, FFmpeg metadata extraction, thumbnail generation with SHA256 hash-based storage (`/0/a/0ab123...jpg`)
- **Streaming Service** ([internal/media/streamer.go](Server/internal/media/streamer.go)) - HTTP range request handling, base64-encoded path parameters
- **Database** ([internal/database/database.go](Server/internal/database/database.go)) - Dual SQLite DBs (users.db, media.db), no ORM

### Configuration
**Required:** Copy [Server/config.example.json](Server/config.example.json) to `Server/config.json`
- **CRITICAL:** Change `auth.jwtSecret` from default before deployment
- Media paths are top-level array (`media.paths`), thumbnails stored in subdirs per hash prefix
- Config loaded via `CONFIG_PATH` env var or defaults to `./config.json`

### Development Workflow
```bash
cd Server
make install          # go mod download + tidy
make build           # Outputs: ./server and ./cli binaries (CGO_ENABLED=1 required)
make run             # Build + start server
make dev             # Auto-reload with air (install: go install github.com/cosmtrek/air@latest)

# CLI management
./cli user create admin --admin  # Create admin user interactively
./cli config add-path /path/to/media
./cli media scan
```

**Docker:** [Server/docker-compose.yml](Server/docker-compose.yml) mounts volumes for config, data, thumbnails, and media directories

### API Patterns
- **Auth:** All `/api/*` routes (except `/api/auth/login`) require `Authorization: Bearer <JWT>` header
- **Streaming:** `GET /api/media/stream?path=<base64-encoded-path>` - Paths MUST be base64-encoded
- **Middleware:** [internal/auth/context.go](Server/internal/auth/context.go) extracts user claims to request context
- **Handlers:** [internal/handlers/](Server/internal/handlers/) use `respondJSON()` and `respondError()` helpers

### OpenAPI Spec
[Server/api/openapi.yaml](Server/api/openapi.yaml) - Use with `openapi-generator-cli` to generate clients (see [Server/ANDROID_INTEGRATION.md](Server/ANDROID_INTEGRATION.md))

## Project-Specific Conventions

### Android
- **Current: SMB-only** - Direct SMB access via jcifs-ng, custom ExoPlayer DataSource
- **Target: Dual-source architecture** - Add Server API client (Retrofit) alongside SMB, share UI/playback components
- **API Integration Pattern** - Use Repository pattern with sealed classes for source types (SMB vs Server), see [ANDROID_INTEGRATION.md](Server/ANDROID_INTEGRATION.md) for Retrofit setup
- **TV-optimized UI** - Uses `androidx.tv:tv-material` with D-pad navigation focus management
- **Stateful navigation** - No Compose Navigation library, ViewModel properties (`selectedVideoPath`, `selectedImagePath`) control screen switching
- **Authentication** - Server API requires JWT (via `/api/auth/login`), store token in EncryptedSharedPreferences like SMB credentials

### Go Server
- **Module path:** `github.com/weirdstreamer/server` (check imports when adding packages)
- **CGO required** - SQLite dependency (`go-sqlite3`) needs `CGO_ENABLED=1`
- **Thumbnail structure:** Two-level hex prefix dirs (`/0/a/`, `/f/f/`) for hash distribution, prevents too many files in single directory
- **Error handling:** Return JSON error objects (`models.ErrorResponse`) with appropriate HTTP status codes

## Testing & Verification
**Server:**
```bash
cd Server
go test ./...                    # Unit tests
./verify_pw.go <hash> <password> # Verify bcrypt hash
./test_auth.go                   # Test JWT auth flow
./test_login.go                  # Test login endpoint
```
Adding Server API to Android App

**Implementation Roadmap:**
1. **Add dependencies** - Retrofit, OkHttp, Gson (see [ANDROID_INTEGRATION.md](Server/ANDROID_INTEGRATION.md#add-dependencies-to-buildgradle))
2. **API models** - Create data classes matching server's [models.go](Server/internal/models/models.go) (`MediaItem`, `Directory`, `LoginResponse`)
3. **Retrofit service** - Interface for `/api/auth/login`, `/api/media`, `/api/media/browse`, `/api/media/stream`
4. **Repository abstraction** - Sealed class `MediaSource` (SMB/Server), unify file browsing interface
5. **ExoPlayer integration** - Server streams work with standard `MediaItem.fromUri()`, add auth headers via OkHttp interceptor
6. **Credential storage** - Extend [CredentialManager.kt](app/src/main/java/com/vargon/weirdstreamer/CredentialManager.kt) for JWT tokens (per server URL)
7. **UI source selector** - Add "Add Server" option alongside current SMB host discovery

**Key Technical Details:**
- Server streaming paths must be **base64-encoded**: `GET /api/media/stream?path=<base64>` 
- ExoPlayer can stream server URLs directly (supports HTTP range requests automatically)
- Server thumbnails accessible at `/thumbnails/<hash_prefix>/<hash>.jpg` with auth header
- Coil can load server thumbnails via custom Fetcher with OkHttp client (reuse auth interceptor)

## Common Pitfalls
1. **Server JWT secret:** Default value `change-this-to-a-random-secret-key` will compromise security
2. **Android SMB auth:** Must call `withCredentials()` on `CIFSContext` per request, not globally cached
3. **Server path encoding:** Streaming endpoint requires base64, not raw paths - decode before file access
4. **CGO build errors:** Ensure `CGO_ENABLED=1` set for go build/run commands with server
5. **Thumbnail 404s:** Scanner creates nested dirs (`/0/a/0ab...`), ensure write permissions on thumbnail root
6. **Server auth in Android:** All API calls except `/api/auth/login` need `Authorization: Bearer <token>` header - configure OkHttp interceptor
7. **Cleartext traffic:** For local dev/testing, enable in `AndroidManifest.xml`: `android:usesCleartextTraffic="true"`
3. **Server path encoding:** Streaming endpoint requires base64, not raw paths - decode before file access
4. **CGO build errors:** Ensure `CGO_ENABLED=1` set for go build/run commands with server
5. **Thumbnail 404s:** Scanner creates nested dirs (`/0/a/0ab...`), ensure write permissions on thumbnail root
