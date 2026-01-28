#!/bin/bash
# Quick test script to verify on-demand file browsing is working
# Run this after rebuilding the server on the remote machine

echo "=== Testing On-Demand File Browsing ==="
echo ""

# Configuration
SERVER_URL="${SERVER_URL:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-}"

if [ -z "$PASSWORD" ]; then
    echo "Usage: PASSWORD=yourpassword ./test_browse.sh"
    echo "Or set SERVER_URL, USERNAME if different from defaults"
    exit 1
fi

echo "1. Getting authentication token..."
TOKEN=$(curl -s -X POST "$SERVER_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
    echo "❌ Failed to get auth token. Check credentials."
    exit 1
fi

echo "✅ Got auth token"
echo ""

echo "2. Browsing root directories..."
RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" "$SERVER_URL/api/media/browse")
echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"
echo ""

# Extract first directory name if available
FIRST_DIR=$(echo "$RESPONSE" | grep -o '"path":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$FIRST_DIR" ]; then
    echo "⚠️  No directories found. Check your media paths in config.json"
    exit 0
fi

echo "3. Browsing first directory: $FIRST_DIR"
RESPONSE=$(curl -s -H "Authorization: Bearer $TOKEN" "$SERVER_URL/api/media/browse?path=$FIRST_DIR")
echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"
echo ""

# Check if response contains items array
ITEMS_COUNT=$(echo "$RESPONSE" | grep -o '"items":\[' | wc -l)
DIRS_COUNT=$(echo "$RESPONSE" | grep -o '"dirs":\[' | wc -l)

echo "=== Test Results ==="
if [ "$ITEMS_COUNT" -gt 0 ]; then
    echo "✅ Files found in directory! On-demand browsing is working."
    echo "   Files are being served directly from filesystem."
elif [ "$DIRS_COUNT" -gt 0 ]; then
    echo "⚠️  Found subdirectories but no files."
    echo "   This might be normal if the folder only contains subdirectories."
    echo "   Try browsing deeper into a folder with media files."
else
    echo "❌ No files or directories found."
    echo "   Possible issues:"
    echo "   - Folder is empty"
    echo "   - No supported media files (mp4, mkv, jpg, etc.)"
    echo "   - Permissions issue"
    echo "   - Old server binary (not rebuilt with latest changes)"
fi

echo ""
echo "=== Verification Checklist ==="
echo "✓ Check server binary date: ls -lah ./server"
echo "✓ Check if 'getMediaTypeFromExtension' exists: grep -n getMediaTypeFromExtension internal/handlers/media.go"
echo "✓ Restart server to load new code: killall server && ./server"
echo "✓ Check server logs for errors during browsing"
