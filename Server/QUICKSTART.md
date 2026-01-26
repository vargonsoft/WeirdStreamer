# Quick Start Guide

Get your media streaming server running in 5 minutes!

## Prerequisites Check

```bash
# Check if Go is installed
go version
# Should show: go version go1.21 or higher

# Check if FFmpeg is installed
ffmpeg -version
# Should show FFmpeg version

# If not installed:
# Ubuntu/Debian: sudo apt install golang-go ffmpeg
# macOS: brew install go ffmpeg
```

## Step 1: Build (30 seconds)

```bash
# Make build script executable and run it
chmod +x build.sh
./build.sh
```

## Step 2: Configure (1 minute)

```bash
# Copy example config
cp config.example.json config.json

# Edit config (change JWT secret!)
nano config.json
```

**Minimum changes needed:**
- Change `jwtSecret` to a random string
- Update `media.paths` to your media directories

## Step 3: Setup (2 minutes)

```bash
# Create admin user
./cli user create admin --admin
# Enter a password when prompted

# Add your media directory
./cli config add-path /path/to/your/videos

# Scan media (this may take a while for large libraries)
./cli scan
```

## Step 4: Run (10 seconds)

```bash
# Start the server
./server
```

Server is now running at http://localhost:8080

## Step 5: Test (1 minute)

### Web Browser
1. Open http://localhost:8080
2. Login with: admin / your-password
3. Browse and play media!

### API Test
```bash
# Login
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'

# Save the token from response, then:
TOKEN="your-token-here"

# List media
curl http://localhost:8080/api/media \
  -H "Authorization: Bearer $TOKEN"
```

## Common Issues

**"Go not found"**
```bash
sudo apt install golang-go  # Ubuntu/Debian
brew install go              # macOS
```

**"FFmpeg not found"**
```bash
sudo apt install ffmpeg      # Ubuntu/Debian
brew install ffmpeg          # macOS
```

**"Connection refused" from Android**
- Use your computer's IP address, not localhost
- Check firewall allows port 8080
- Try: `curl http://YOUR_IP:8080/health` from phone

**No thumbnails appearing**
- FFmpeg must be installed
- Run `./cli scan` again
- Check logs for errors

## Next Steps

- **Android Integration**: See ANDROID_INTEGRATION.md
- **Production Deployment**: See README.md Docker section
- **API Documentation**: See api/openapi.yaml
- **Add More Users**: `./cli user create username`
- **Add More Paths**: `./cli config add-path /path`

## Docker Quick Start

```bash
# Edit docker-compose.yml with your paths and secrets
nano docker-compose.yml

# Start
docker-compose up -d

# Create user
docker-compose exec media-server ./cli user create admin --admin

# Scan
docker-compose exec media-server ./cli scan
```

## File Locations

- **Binary**: `./server` and `./cli`
- **Config**: `./config.json`
- **Database**: `./data/media.db`
- **Thumbnails**: `./thumbnails/`
- **Logs**: Console output

## Support

Need help? Check:
1. README.md - Full documentation
2. ANDROID_INTEGRATION.md - Android client guide
3. GitHub Issues - Report problems

Enjoy your media streaming server! 🎬
