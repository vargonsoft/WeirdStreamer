# Home Button Addition to MediaBrowser Toolbar

## Feature
Added a "Home" button to the MediaBrowser toolbar that allows users to quickly return to the server selection screen from anywhere in the file browser.

## Implementation

### Changes Made
**File:** `ui/components/MediaBrowser.kt`

**1. Added Home icon import:**
```kotlin
import androidx.compose.material.icons.filled.Home
```

**2. Added Home button to toolbar:**
```kotlin
if (currentSource != null) {
    Button(onClick = { viewModel.returnToSourceSelection() }) {
        Icon(Icons.Default.Home, contentDescription = "Back to Servers")
        Spacer(modifier = Modifier.width(8.dp))
        Text("Home")
    }
    Spacer(modifier = Modifier.width(16.dp))
    // ... other buttons
}
```

## Behavior

### When Visible
The Home button appears in the toolbar **only when browsing within a source** (SMB or Server). It does not appear on the initial server selection screen.

### Button Order (Left to Right)
When browsing within a source:
1. **Home** - Returns to server selection
2. **Grid/List Toggle** - Switches between grid and list view
3. **Add Source** - Opens dialog to add new server
4. **Settings** - Opens settings screen

### What It Does
Clicking the Home button:
1. Calls `viewModel.returnToSourceSelection()`
2. Clears the current source (`currentSource = null`)
3. Clears the current path (`currentPath = ""`)
4. Clears any error messages (`errorMessage = null`)
5. Clears the file list (`fileList = emptyList()`)
6. Refreshes the sources list to show all SMB hosts and API servers
7. Returns user to the server selection screen

## Use Cases

### Quick Navigation
**Before:** To return to server selection from deep within a folder hierarchy:
- Press back button multiple times
- OR press backspace key multiple times

**After:** To return to server selection from anywhere:
- Click the Home button once
- Instantly return to server selection screen

### Switch Between Sources
**Scenario:** User is browsing an SMB share but wants to check a file on the server

**Before:**
1. Navigate up through all folders (multiple back presses)
2. Wait to reach server selection
3. Select different server

**After:**
1. Click Home button (instant)
2. Select different server

### Visual Clarity
The Home icon (house symbol) is immediately recognizable and clearly indicates its purpose: return to the main screen/server list.

## Technical Details

### Icon
- **Material Icon:** `Icons.Default.Home`
- **Description:** Standard house/home icon from Material Design
- **Accessibility:** "Back to Servers" content description for screen readers

### Button Placement
- Positioned as the **first button** in the toolbar when browsing
- Consistent with common UI patterns (home button typically on the left)
- Only visible when `currentSource != null`

### Method Called
```kotlin
fun returnToSourceSelection() {
    android.util.Log.d("MediaViewModel", "returnToSourceSelection - clearing source, path, error, and file list")
    currentSource = null
    currentPath = ""
    errorMessage = null
    fileList = emptyList()
    refreshSources()
}
```

This method already existed in `MediaViewModel` - it's used when navigating up from the root of a source. **Updated to also clear `errorMessage`** to prevent errors from persisting when returning to the server selection screen.

## Files Modified

1. ✅ `ui/components/MediaBrowser.kt` - Added Home icon import and Home button to toolbar

## Verification

### Visual Check:
1. Build and run the app
2. Select an SMB or Server source
3. **Expected:** Home button appears in toolbar (first button)
4. Click on folders to navigate deeper
5. **Expected:** Home button remains visible
6. Click Home button
7. **Expected:** Instantly return to server selection screen

### Behavior Check:
1. From server selection, note current servers
2. Navigate into a server, browse some folders
3. Click Home
4. **Expected:** 
   - Returns to server selection screen
   - Same servers listed as before
   - Can immediately select a different server

### Icon Check:
- **Expected:** Standard home/house icon (🏠)
- **Text:** "Home" label next to icon
- **Consistent:** Same style as other toolbar buttons

## Benefits

1. **Faster Navigation** - One click vs multiple back presses
2. **Better UX** - Familiar pattern (home button)
3. **Discoverability** - Visible and clearly labeled
4. **Accessibility** - Labeled for screen readers
5. **Consistency** - Matches common app patterns

## Alternative Approaches Considered

### Why not just "Back" button?
- Already have backspace key and BackHandler
- "Home" is semantically different from "Back"
- "Home" = go to root/main screen
- "Back" = go to previous screen

### Why not keyboard shortcut only?
- TV remote users benefit from visible button
- Discoverability - users know the option exists
- Touch-friendly for devices with both remote and touch

### Why not always visible?
- Would be confusing on server selection screen (already at "home")
- Cleaner UI when not needed
- Only shows when it has a purpose

## Conclusion

The Home button provides a quick, intuitive way to return to the server selection screen from anywhere in the file browser, improving navigation efficiency and user experience.
