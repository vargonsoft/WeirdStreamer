# 577-Byte Thumbnail Diagnosis

## Current Findings

### From the Log:
```
2026/01/27 23:06:43 [THUMBNAIL] Generating video thumbnail: source=/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4, output=run/thumbnails/3/3/33dd3e08923068cd72a33fde6d62bc992b4f9d61e9a45b00d1e6367e830998fe.jpg
2026/01/27 23:06:43 [THUMBNAIL] FFmpeg created thumbnail but size too small: 577 bytes
```

### Key Observations:

1. **FFmpeg runs without errors** (exit code 0)
2. **Creates 577-byte file** (JPEG header only, no image data)
3. **Source path**: `/smb/basestar/Video/...` - This is an SMB network share!

## Root Cause Analysis

### Why FFmpeg Creates 577-Byte Files:

FFmpeg successfully:
- ✅ Starts execution
- ✅ Opens output file
- ✅ Writes JPEG header (577 bytes)

But FFmpeg fails to:
- ❌ Read the input video file
- ❌ Extract a video frame
- ❌ Write image data to the JPEG

### Most Likely Causes:

#### 1. **SMB Mount Issue** (MOST LIKELY)
- Path `/smb/basestar/...` suggests an SMB network share
- FFmpeg may not have permission to read from the SMB share
- Or the share is mounted with options that prevent FFmpeg access
- Or the share disconnected/remounted between file check and FFmpeg execution

#### 2. **File Permissions**
- File exists but FFmpeg user can't read it
- Check: `ls -la /smb/basestar/Video/1/Movies/10\ Things\ I\ Hate\ About\ You\ (1999)\ -\ 1080p/`

#### 3. **Codec/Format Issue**
- FFmpeg can open the file but can't decode the video codec
- Would typically show warnings in output

## New Logging Added

The updated code now logs:

### 1. File Existence Check:
```
[THUMBNAIL] Source file exists: size=XXXXX bytes, path=/smb/basestar/...
```
or
```
[THUMBNAIL] ERROR: Source video file not found: /smb/basestar/...
```

### 2. FFmpeg Output (Even on Success):
```
[THUMBNAIL] FFmpeg output (last 500 chars): [FFmpeg warnings/errors]
```

This will show FFmpeg warnings even when exit code is 0!

## Expected Next Log Output

After rebuilding with the new changes, you should see:

```
[THUMBNAIL] Source file exists: size=XXXXX bytes, path=/smb/basestar/...
[THUMBNAIL] Generating video thumbnail: source=/smb/basestar/..., output=run/thumbnails/...
[THUMBNAIL] FFmpeg output (last 500 chars): [THIS WILL TELL US WHY!]
[THUMBNAIL] FFmpeg created thumbnail but size too small: 577 bytes
```

The FFmpeg output will reveal:
- "Permission denied" → File permission issue
- "No such file or directory" → File doesn't exist (mount issue)
- "Invalid data found" → File corrupted or inaccessible
- "Unknown decoder" → Codec not supported
- Network errors → SMB mount problem

## Potential Solutions

### If SMB Mount Issue:

**Option A: Fix SMB Mount Permissions**
```bash
# Check current mount
mount | grep smb

# Remount with proper permissions
sudo mount -t cifs //basestar/Video /smb/basestar/Video -o username=XXX,uid=1000,gid=1000
```

**Option B: Run Server as Different User**
```bash
# Check what user owns the files
ls -la /smb/basestar/Video/

# Run server as that user
sudo -u USERNAME ./server
```

**Option C: Copy/Symlink Files Locally**
- Not practical for large video libraries

### If Permission Issue:

```bash
# Check file permissions
ls -la "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/"

# Fix permissions if needed
chmod 644 "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/*.mp4"
```

### If Codec Issue:

```bash
# Check FFmpeg can decode the file
ffmpeg -i "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4" -frames:v 1 /tmp/test.jpg

# If it fails, rebuild FFmpeg with more codecs
```

## Test Commands

### 1. Check if server can read the file:
```bash
# As the user running the server
cat "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4" > /dev/null
echo $?  # Should be 0 if readable
```

### 2. Test FFmpeg directly:
```bash
ffmpeg -ss 00:00:01.000 -i "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4" -frames:v 1 -vf "scale=320:-1" -q:v 2 -y /tmp/test_thumbnail.jpg

# Check size
ls -lh /tmp/test_thumbnail.jpg
```

### 3. Check SMB mount status:
```bash
mount | grep smb
df -h | grep smb
```

## Deploy and Test

```bash
# On babylon5
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &

# Trigger thumbnail generation
# Browse to the same folder in web UI

# Check logs
tail -f server.log | grep -E '\[THUMBNAIL\]|\[SCANNER\]'
```

The new logs will immediately show us the exact reason FFmpeg is creating 577-byte files!
