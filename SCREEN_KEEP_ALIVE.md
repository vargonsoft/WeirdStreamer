# Screen Keep-Alive Feature

## Overview
Added functionality to keep the screen on (prevent screen timeout) when viewing videos, images, or PDFs. The screen will automatically return to normal timeout behavior when the user exits back to the file browser.

## Implementation

### MainActivity.kt Changes

**Added Import:**
```kotlin
import android.view.WindowManager
```

**Added DisposableEffect:**
```kotlin
// Keep screen on when viewing media (video, image, or PDF)
val isViewingMedia = selectedVideo != null || selectedImage != null || selectedPdf != null
DisposableEffect(isViewingMedia) {
    if (isViewingMedia) {
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    } else {
        window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }
    onDispose {
        window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }
}
```

## How It Works

### Flag Management
- **FLAG_KEEP_SCREEN_ON**: Android window flag that prevents the screen from turning off due to inactivity
- Added when: User opens a video, image, or PDF viewer
- Removed when: User returns to the file browser or settings screen

### Lifecycle Handling
1. **User opens media**: `isViewingMedia` becomes `true`, flag is set
2. **User views content**: Screen stays on indefinitely, no timeout
3. **User exits viewer**: `isViewingMedia` becomes `false`, flag is cleared
4. **App disposed**: `onDispose` ensures flag is always cleaned up

### State Tracking
```kotlin
val isViewingMedia = selectedVideo != null || selectedImage != null || selectedPdf != null
```

This reactive expression automatically updates when the ViewModel's selected path variables change, triggering the DisposableEffect to add or remove the keep-screen-on flag.

## Benefits

### User Experience
- ✅ **Videos**: Screen won't turn off during playback, even during long movies
- ✅ **Images**: Users can view photos/screenshots without screen timeout interruption
- ✅ **PDFs**: Can read documents at their own pace without screen going dark
- ✅ **Automatic**: No manual toggle needed - works automatically
- ✅ **Battery Friendly**: Only active when actually viewing media, not while browsing

### Technical Benefits
- ✅ **Declarative**: Uses Compose's DisposableEffect for clean state management
- ✅ **Lifecycle-aware**: Automatically cleans up when composable is disposed
- ✅ **Reactive**: Updates immediately when navigation state changes
- ✅ **No Memory Leaks**: Flag is guaranteed to be cleared via onDispose

## Testing

To verify the feature works:

1. **Build and install** the app on Android TV
2. **Navigate to a video** and open it
3. **Wait** (don't interact) - screen should NOT turn off even after normal timeout period
4. **Press Back** to exit video - screen timeout should resume normal behavior
5. **Repeat for images** - same behavior
6. **Repeat for PDFs** - same behavior
7. **Browse files** - screen should timeout normally when not viewing media

## Alternative Approaches Considered

### Per-Viewer Implementation
Could have added FLAG_KEEP_SCREEN_ON in each viewer (VideoPlayer, ImageViewer, PdfViewer):
- ❌ **Rejected**: Would require duplicate code in 3 places
- ❌ **Harder to maintain**: Changes needed in multiple files
- ❌ **Less centralized**: Logic spread across the codebase

### Always-On
Could have kept screen on always:
- ❌ **Rejected**: Wastes battery when browsing files
- ❌ **Poor UX**: Screen stays on even when user is idle in browser

### Manual Toggle
Could have added a settings option:
- ❌ **Rejected**: Extra step for users
- ❌ **Less intuitive**: Users need to discover and enable feature
- ✅ **Current approach**: Works automatically, no configuration needed

## Code Location

**File**: `/Users/vargon/AndroidStudioProjects/WeirdStreamer/app/src/main/java/com/vargon/weirdstreamer/MainActivity.kt`

**Lines**: 
- Import: Line 5 (`import android.view.WindowManager`)
- Implementation: Lines 32-43 (DisposableEffect block)

## Notes

- The flag only affects the current activity window
- Does not affect system-wide screen timeout settings
- Compatible with all Android versions
- No special permissions required
- Flag is automatically cleared when activity is destroyed
- Works with both SMB and WS Server sources

## Future Enhancements

Potential improvements for future versions:
1. **Configurable timeout**: Allow users to set custom timeout for media viewing
2. **Dimming**: Instead of full screen-off, dim the screen after X minutes
3. **Per-type settings**: Different behavior for videos vs images vs PDFs
4. **Activity detection**: Keep screen on only during actual playback, not when paused
