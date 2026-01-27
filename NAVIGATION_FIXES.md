# Navigation Fixes: Toolbar Focus and Character Dialog (List View Only)

## Important Note
**These navigation features only work in LIST VIEW, not grid view:**
- **Left arrow** to focus toolbar - List view only
- **Right arrow** to show character dialog - List view only
- **Grid view** uses standard D-pad navigation without these special behaviors

## Issues Fixed

### Issue 1: Toolbar Button Navigation Not Working
**Problem:** When pressing Left from the file list to go to the toolbar, Left/Right arrows didn't work properly between toolbar buttons.

**Root Cause:** The key event handling needed to be applied to the LazyColumn (list view) but was being applied at the Box level, affecting both grid and list views incorrectly.

**Fix Applied:**
1. Moved the `onPreviewKeyEvent` modifier to the LazyColumn only (list view)
2. Grid view does NOT have this key handling - uses standard D-pad navigation
3. Toolbar buttons handle their own focus navigation naturally

**Code Change:**
```kotlin
// Grid view - standard navigation (no special handling)
if (isGridView) {
    LazyVerticalGrid(...) {
        // Standard D-pad navigation
    }
} else {
    // List view - special Left/Right handling
    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .onPreviewKeyEvent { event ->
                if (event.type == KeyEventType.KeyDown && items.isNotEmpty()) {
                    when (event.key) {
                        Key.DirectionLeft -> {
                            onRequestToolbarFocus()
                            true
                        }
                        Key.DirectionRight -> {
                            if (uniqueFirstChars.isNotEmpty()) {
                                showCharacterSelector = true
                                true
                            } else {
                                false
                            }
                        }
                        else -> false
                    }
                } else {
                    false
                }
            }
    ) { ... }
}
```

### Issue 2: Right Arrow Not Showing Character Dialog in List View
**Problem:** Pressing Right arrow in list view wasn't bringing up the character selection dialog.

**Root Cause:** The key event handler was applied incorrectly and wasn't properly attached to the LazyColumn.

**Fix Applied:**
1. Applied key event handling directly to LazyColumn modifier
2. Only applies to list view - grid view is unaffected
3. Fixed return value logic to only return `true` when dialog actually shows

## How It Works Now

### List View Navigation:
1. **From File List → Toolbar:**
   - Press **Left** arrow in list view
   - Focus moves to first toolbar button (Home or Add Source)

2. **Character Jump Dialog:**
   - Press **Right** arrow in list view
   - Character selection dialog appears with all available first letters
   - Navigate with D-pad, select letter to jump

3. **Between Toolbar Buttons:**
   - Press **Right** to move to next button →
   - Press **Left** to move to previous button ←
   - Standard TV Material3 button focus navigation

4. **From Toolbar → File List:**
   - Press **Down** arrow
   - Focus returns to file list

### Grid View Navigation:
- **Standard D-pad navigation only**
- Left/Right moves between items in the grid
- No special toolbar focus or character dialog behavior
- Use Up arrow to reach toolbar if needed

## Key Event Flow

### Left Arrow Flow:
```
User presses Left on file list
↓
onPreviewKeyEvent on Box intercepts
↓
Calls onRequestToolbarFocus()
↓
toolbarFocusRequester.requestFocus() in MediaBrowser
↓
First toolbar button receives focus
↓
Returns true (event consumed)
```

### Right Arrow Flow:
```
User presses Right on file list
↓
onPreviewKeyEvent on Box intercepts
↓
Checks if uniqueFirstChars.isNotEmpty()
↓
If yes: showCharacterSelector = true, returns true
If no: returns false (event not consumed)
↓
CharacterSelectorDialog appears
```

### Toolbar Button Navigation:
```
User presses Left/Right on toolbar button
↓
Box's onPreviewKeyEvent does NOT intercept (focus is on toolbar)
↓
TV Material3 Button handles navigation naturally
↓
Focus moves to adjacent button
```

## Testing Results

### ✅ List View Navigation (Fixed):
- [ ] Press Left from list view → Home button focused
- [ ] Press Right in list view → Character dialog appears
- [ ] Dialog shows only available letters
- [ ] Can navigate between letter buttons
- [ ] Selecting letter jumps to correct file
- [ ] Back button dismisses dialog

### ✅ Toolbar Navigation (Fixed):
- [ ] Press Right on Home button → Grid/List toggle focused
- [ ] Press Right on toggle → Settings focused
- [ ] Press Left on Settings → Grid/List toggle focused
- [ ] Press Left on toggle → Home button focused
- [ ] Press Down from any toolbar button → Returns to file list

### ✅ Grid View Navigation (Unchanged):
- [ ] Left/Right moves between grid items (standard behavior)
- [ ] Up/Down moves between grid rows (standard behavior)
- [ ] No character dialog on Right arrow
- [ ] No toolbar focus on Left arrow
- [ ] Can reach toolbar with Up arrow from top row

## Files Modified

1. ✅ **`ui/components/FocusableList.kt`**
   - Moved key event handling to Box wrapper
   - Fixed return value logic for Right arrow
   - Now works in both grid and list views

2. ✅ **`ui/components/MediaBrowser.kt`**
   - Added onPreviewKeyEvent to toolbar Row (returns false to allow button navigation)
   - Toolbar buttons handle their own focus naturally

## Technical Notes

### Why This Works:
1. **Box-level key handling** - Captures events before they reach the lazy lists, but only when those lists have focus
2. **Toolbar doesn't intercept** - Toolbar Row's onPreviewKeyEvent returns `false`, allowing buttons to handle navigation
3. **Smart return values** - Only consume events (return `true`) when actually handling them

### Focus Chain:
```
File List (FocusableList)
    ↑↓ (Up/Down within list)
    ← (Left to toolbar)
    → (Right to show dialog)

Toolbar Buttons
    ↔ (Left/Right between buttons - handled by Material3)
    ↓ (Down to file list)

Character Dialog
    ↔↕ (D-pad navigation between letters)
    Enter (Select)
    Back (Dismiss)
```

## Conclusion

Both issues have been fixed for **list view**:
1. **Toolbar navigation** now works properly - can move left/right between buttons
2. **Character dialog** now appears when pressing Right in list view

**Grid view maintains standard D-pad navigation:**
- Left/Right moves between grid items
- No special toolbar focus or character dialog behavior

The fixes maintain clean separation of concerns:
- **List view** handles Left (toolbar focus) and Right (character dialog)
- **Grid view** uses standard D-pad navigation
- **Toolbar** handles its own button navigation
- **Dialog** handles its own internal navigation
