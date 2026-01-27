# Video Transcoding Feature

## Overview
Added a transcode toggle option to the VideoPlayer for WS Server video streams. This allows users to request on-the-fly transcoding from the server when playing videos that may have compatibility issues with the Android TV player.

## Changes Made

### VideoPlayer.kt

1. **Import Added**:
   - `Icons.Default.VideoSettings` - Icon for the transcode button

2. **State Management**:
   - Added `enableTranscode` state variable to track whether transcoding is enabled
   - Added `isFocusedOnTranscode` state variable to track focus on the transcode button
   - Added `videoUrl` computed value that appends `?transcode=1` or `&transcode=1` to the stream URL when transcoding is enabled
   - Only applicable for WS Server sources (not SMB)

3. **UI Components**:
   - **Transcode Button**: Added ABOVE the progress slider in the playback controls
     - Only visible for WS Server videos (not shown for SMB sources)
     - Shows as a Surface with VideoSettings icon and text label
     - Text displays "Transcoding ON" (green) or "Transcoding OFF" (white)
     - Highlights with semi-transparent white background when focused
     - Size: Icon 32dp, with padding for comfortable focus
     - Click action: Toggles transcode state and updates interaction time

4. **Video URL Construction**:
   - When transcoding is enabled for server sources, the URL is modified to include `transcode=1` parameter
   - Properly handles URLs that already have query parameters
   - ExoPlayer is recreated when the URL changes (via `remember(videoUrl)`)
   - **Player View Update**: AndroidView includes an `update` block that updates the player reference when exoPlayer changes
   - This ensures the new transcoded stream is properly loaded with the correct MIME type
   - The old player instance is automatically released via DisposableEffect when a new one is created

5. **Navigation Flow**:
   - **From playback controls**: Press UP to move to progress slider
   - **From progress slider**: Press UP to move to transcode button (if WS Server source)
   - **From transcode button**: Press DOWN to move to progress slider
   - **From progress slider**: Press DOWN to move to playback controls
   - **On transcode button**: Press CENTER/ENTER to toggle transcoding
   - **Left/Right on transcode button**: Does nothing (only used for seeking on slider)

## Server Integration

The WS Server already supports transcoding via the `transcode=1` query parameter on the `/api/media/stream` endpoint:
- When `transcode=1` is present, the server uses FFmpeg to transcode the video on-the-fly
- Transcodes to H.264 with AAC audio for maximum compatibility
- Uses progressive streaming so playback can start immediately

## User Experience

### Normal Playback (Default)
1. User opens a video from WS Server
2. Video plays with direct streaming (no transcoding)
3. When controls are shown, transcode button appears above progress slider showing "Transcoding OFF" in white

### With Transcoding Enabled
1. User presses any key to show controls
2. User presses UP twice to navigate from playback controls → slider → transcode button
3. Transcode button highlights with semi-transparent background
4. User presses Select/Enter to toggle transcoding
5. Text changes to "Transcoding ON" in green color
6. Video is reloaded with `transcode=1` parameter
7. ExoPlayer is recreated with new instance for transcoded stream
8. Server transcodes the video on-the-fly
9. Playback continues with transcoded stream (H.264/AAC)

### Navigation
- **Position**: Above the progress slider, at the top of the controls overlay
- **Access**: Press UP from progress slider to reach the transcode button
- **Focus indicator**: Button has semi-transparent white background when focused
- **Visual state**: Icon and text turn green when transcoding is active
- Part of the control overlay that shows/hides with other controls

## Technical Details

### URL Modification Logic
```kotlin
val videoUrl = remember(path, enableTranscode) {
    if (!isSmbSource && enableTranscode) {
        if (path.contains("?")) {
            "$path&transcode=1"
        } else {
            "$path?transcode=1"
        }
    } else {
        path
    }
}
```

### ExoPlayer Reinitialization
- When `videoUrl` changes (due to transcode toggle), ExoPlayer is recreated
- This ensures the new URL with transcode parameter is used
- Playback position is reset to the beginning
- User may need to seek back to their previous position manually

## Future Enhancements

Potential improvements for future versions:
1. **Remember Playback Position**: Save position before transcoding and restore after reload
2. **Persist Transcode Preference**: Remember user's transcode preference per file or globally
3. **Auto-detect Compatibility**: Automatically enable transcoding if player detects unsupported codec
4. **Transcode Options**: Allow selection of quality/bitrate for transcoding
5. **Seamless Switching**: Hot-swap between transcoded and direct stream without stopping playback

## Testing

To test the feature:
1. Build and install the app on Android TV
2. Connect to a WS Server source
3. Navigate to a video file
4. Open the video player
5. Press any key to show controls
6. Press UP to move focus to the progress slider
7. Press UP again to move focus to the transcode button (should see it highlight)
8. Press Select/Enter on the transcode button to toggle transcoding
9. Verify the text changes to "Transcoding ON" in green
10. Verify the video reloads and plays the transcoded stream
11. Observe that playback continues smoothly with the new MIME type
12. Press UP to focus transcode button again, then Select/Enter to toggle off
13. Verify video reloads without transcoding

## Notes

- SMB sources do not show the transcode button (not applicable)
- Transcoding adds server-side processing overhead
- Initial buffering may take slightly longer when transcoding is enabled
- Transcoding ensures compatibility but may reduce video quality slightly
- The server must have FFmpeg installed for transcoding to work
