# Dialog Z-Order Fix for Android TV

## Issue
The dialog was composing correctly but not appearing on screen. This is an Android TV-specific z-ordering issue.

## Root Cause
The `CharacterSelectorDialog` was placed OUTSIDE the Box container in the composition tree:

```kotlin
Box(modifier = Modifier.fillMaxSize()) {
    // LazyColumn/LazyVerticalGrid here
    // Jump character popup here
}  // ← Box ends here

// Character dialog here - WRONG! Outside the Box
if (showCharacterSelector) {
    CharacterSelectorDialog(...)
}
```

**Problem:** On Android TV, composables need to be in the proper z-order within the same container to overlay correctly. Placing the dialog outside the Box meant it wasn't rendering on top of the content.

## Fix Applied
Moved the dialog INSIDE the Box so it's part of the same z-order stack:

```kotlin
Box(modifier = Modifier.fillMaxSize()) {
    // LazyColumn/LazyVerticalGrid here
    
    // Jump character popup here
    
    // Character dialog here - CORRECT! Inside the Box
    if (showCharacterSelector) {
        CharacterSelectorDialog(...)
    }
}
```

**Why this works:**
- Composables in a Box are stacked in z-order based on their declaration order
- Later composables appear on top of earlier ones
- The dialog is declared last, so it renders on top of everything else
- The dialog has a full-screen Box with magenta background, ensuring it covers everything

## Changes Made
**File:** `ui/components/FocusableList.kt`

**Before:**
- Dialog was a sibling of the Box (outside)
- No proper z-ordering relationship with the content

**After:**
- Dialog is a child of the Box (inside)
- Properly stacked above the LazyColumn/LazyVerticalGrid
- Renders at the correct z-level for Android TV

## Why This Is TV-Specific
On regular Android, you'd typically use:
- `AlertDialog` composable
- `Dialog` composable  
- Popup/DropdownMenu

On Android TV:
- No standard Dialog composable that works reliably
- Must use Box-based overlays with proper z-ordering
- Focus management is critical (which we already have)

## Testing
Build and install the app:
```bash
./gradlew :app:assembleDebug
./gradlew :app:installDebug
```

Press Right arrow in list view:
- **Expected:** Bright magenta overlay with cyan dialog box appears
- The dialog should now properly cover the entire screen
- Letter buttons should be visible and focusable

## Files Modified
- ✅ `ui/components/FocusableList.kt` - Moved dialog inside Box for proper z-ordering

## Next Steps
Once confirmed working, we can:
1. Adjust the colors to something more appropriate
2. Remove debug logging
3. Fine-tune the visual appearance
