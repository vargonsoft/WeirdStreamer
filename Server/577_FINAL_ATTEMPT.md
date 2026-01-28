# 577-Byte Thumbnail - Final Fix Attempt

## Current Status

After multiple attempts, thumbnails are still 577 bytes. FFmpeg is running without the "sequence pattern" error, but the output file size is still wrong.

## Latest Command

```bash
ffmpeg -i <input> -ss 00:00:01.000 -vframes 1 -vf scale=320:-1 -y -f image2 -update 1 <output>
```

**Key changes in this version:**
- Put `-f image2` and `-update 1` IMMEDIATELY BEFORE the output filename
- This is the proper position for output format options in FFmpeg

## Why This Might Work

FFmpeg output format options (`-f`, `-update`) should come as close to the output filename as possible. The order matters:

**WRONG (previous attempts):**
- `-vframes 1 -update 1 -vf ... -y <output>` ← update is too far from output
- `-vframes 1 -f image2 -update 1 -vf ... <output>` ← filters between format and output

**CORRECT (current):**
- `-vframes 1 -vf ... -y -f image2 -update 1 <output>` ← format options right before output

## Test After Rebuild

```bash
# On babylon5
cd <repo>/Server
git pull
CGO_ENABLED=1 go build -o server ./cmd/server
killall server
./server > server.log 2>&1 &

# Trigger regeneration and watch logs
tail -f server.log | grep THUMBNAIL
```

## If This Still Doesn't Work

Try manually on babylon5:

```bash
# Test the exact command
ffmpeg -i "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4" \
  -ss 00:00:01.000 \
  -vframes 1 \
  -vf "scale=320:-1" \
  -y \
  -f image2 \
  -update 1 \
  /tmp/test_hash_name_33dd3e08923068cd72a33fde6d62bc992b4f9d61e9a45b00d1e6367e830998fe.jpg

# Check size
ls -lh /tmp/test_hash_name_33dd3e08923068cd72a33fde6d62bc992b4f9d61e9a45b00d1e6367e830998fe.jpg

# If still 577 bytes, try WITHOUT the hash-like name:
ffmpeg -i "/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4" \
  -ss 00:00:01.000 \
  -vframes 1 \
  -vf "scale=320:-1" \
  -y \
  /tmp/simple_name.jpg

# If simple name works but hash name doesn't, it's definitely the filename issue
```

## Alternative Approach

If the filename is truly the problem, we could:
1. Generate to a temp file with a simple name
2. Rename to the hash-based name after generation

This would bypass the image2 muxer's filename detection entirely.
