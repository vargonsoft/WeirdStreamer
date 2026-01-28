#!/bin/bash
# Test FFmpeg thumbnail generation
# Run this on babylon5 to diagnose the 577-byte issue

VIDEO_FILE="/smb/basestar/Video/1/Movies/10 Things I Hate About You (1999) - 1080p/10.Things.I.Hate.About.You.1999.1080p.BluRay.x264.VPPV.mp4"
OUTPUT_FILE="/tmp/test_thumbnail.jpg"

echo "=== Testing FFmpeg Thumbnail Generation ==="
echo ""
echo "Video file: $VIDEO_FILE"
echo "Output file: $OUTPUT_FILE"
echo ""

# Test 1: Check if file exists and is readable
echo "=== Test 1: File Accessibility ==="
if [ -f "$VIDEO_FILE" ]; then
    echo "✓ File exists"
    ls -lh "$VIDEO_FILE"
    echo ""

    # Try to read first 1MB
    echo "Testing read access..."
    if dd if="$VIDEO_FILE" of=/dev/null bs=1M count=1 2>&1 | grep -q "1048576 bytes"; then
        echo "✓ File is readable"
    else
        echo "✗ File read failed"
    fi
else
    echo "✗ File not found!"
    exit 1
fi
echo ""

# Test 2: FFmpeg with seeking (current method)
echo "=== Test 2: FFmpeg with seeking (current server method) ==="
rm -f "$OUTPUT_FILE"
ffmpeg -ss 00:00:01.000 -i "$VIDEO_FILE" -frames:v 1 -vf "scale=320:-1" -q:v 2 -y "$OUTPUT_FILE" 2>&1 | tail -20
echo ""

if [ -f "$OUTPUT_FILE" ]; then
    SIZE=$(stat -c%s "$OUTPUT_FILE" 2>/dev/null || stat -f%z "$OUTPUT_FILE" 2>/dev/null)
    echo "Output file size: $SIZE bytes"
    if [ "$SIZE" -gt 1024 ]; then
        echo "✓ SUCCESS: Thumbnail created ($SIZE bytes)"
        file "$OUTPUT_FILE"
    else
        echo "✗ FAILED: File too small ($SIZE bytes)"
        echo "This is the same 577-byte issue!"
    fi
else
    echo "✗ FAILED: No output file created"
fi
echo ""

# Test 3: FFmpeg without seeking (fallback method)
echo "=== Test 3: FFmpeg without seeking (fallback method) ==="
rm -f "$OUTPUT_FILE"
ffmpeg -i "$VIDEO_FILE" -frames:v 1 -vf "scale=320:-1" -q:v 2 -y "$OUTPUT_FILE" 2>&1 | tail -20
echo ""

if [ -f "$OUTPUT_FILE" ]; then
    SIZE=$(stat -c%s "$OUTPUT_FILE" 2>/dev/null || stat -f%z "$OUTPUT_FILE" 2>/dev/null)
    echo "Output file size: $SIZE bytes"
    if [ "$SIZE" -gt 1024 ]; then
        echo "✓ SUCCESS: Thumbnail created ($SIZE bytes)"
        file "$OUTPUT_FILE"
    else
        echo "✗ FAILED: File too small ($SIZE bytes)"
    fi
else
    echo "✗ FAILED: No output file created"
fi
echo ""

# Test 4: FFmpeg info about the video
echo "=== Test 4: Video Information ==="
ffmpeg -i "$VIDEO_FILE" 2>&1 | grep -E "(Duration|Stream|Video|Audio|codec)" | head -10
echo ""

# Test 5: Simple frame extraction (no scaling)
echo "=== Test 5: Simple frame extraction (no scaling, no seeking) ==="
rm -f "$OUTPUT_FILE"
ffmpeg -i "$VIDEO_FILE" -vframes 1 -y "$OUTPUT_FILE" 2>&1 | tail -10
echo ""

if [ -f "$OUTPUT_FILE" ]; then
    SIZE=$(stat -c%s "$OUTPUT_FILE" 2>/dev/null || stat -f%z "$OUTPUT_FILE" 2>/dev/null)
    echo "Output file size: $SIZE bytes"
    if [ "$SIZE" -gt 1024 ]; then
        echo "✓ SUCCESS: Simple extraction works!"
        echo "Problem is likely with the scaling or seeking parameters"
    else
        echo "✗ FAILED: Even simple extraction creates small file"
        echo "Problem is with video decoding itself"
    fi
else
    echo "✗ FAILED: No output file created"
fi
echo ""

echo "=== Summary ==="
echo "If Test 2 fails but Test 3 or 5 succeeds:"
echo "  → Problem is with -ss (seeking) parameter"
echo "  → Solution: Always use fallback method (no seeking)"
echo ""
echo "If all tests create 577-byte files:"
echo "  → Problem is with video codec or FFmpeg build"
echo "  → Check FFmpeg error messages above"
echo ""
echo "If all tests succeed:"
echo "  → Problem is with how Go executes FFmpeg"
echo "  → Check environment, PATH, permissions"
