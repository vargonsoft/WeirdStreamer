# Improved Navigation: Toolbar Focus and Character Jump Dialog

## Overview
Enhanced the file browser navigation to provide more intuitive controls:
- **Left Arrow** - Moves focus to the toolbar
- **Right Arrow** - Opens a character selection dialog for quick jumping

This replaces the previous left/right arrow behavior which jumped between first letters of files.

## Implementation Date
January 26, 2026

## Changes Made

### 1. FocusableList Component Updates
**File:** `ui/components/FocusableList.kt`

#### New Features:
- **Added `onRequestToolbarFocus` callback parameter** - Allows the list to request focus move to toolbar
- **Added character selector dialog** - Full-screen overlay with grid of available first letters
- **Replaced left/right navigation logic** - Now triggers toolbar focus and dialog instead of jumping

#### Key Code Changes:

**New Function Parameter:**
```kotlin
fun <T> FocusableList(
    items: List<T>,
    initialSelection: String?,
    isGridView: Boolean,
    onRequestToolbarFocus: () -> Unit = {},  // ✨ NEW
    itemContent: @Composable (T, Modifier) -> Unit
)
```

**New State:**
```kotlin
var showCharacterSelector by remember { mutableStateOf(false) }
```

**Unique Characters Extraction:**
```kotlin
val uniqueFirstChars = remember(items) {
    items.mapNotNull { item ->
        getFirstChar(item)?.uppercaseChar()
    }.distinct().sorted()
}
```

**Updated Key Handling:**
```kotlin
.onPreviewKeyEvent { event ->
    if (event.type == KeyEventType.KeyDown && items.isNotEmpty()) {
        when (event.key) {
            Key.DirectionLeft -> {
                // Move focus to toolbar
                onRequestToolbarFocus()
                true
            }
            Key.DirectionRight -> {
                // Show character selector dialog
                if (uniqueFirstChars.isNotEmpty()) {
                    showCharacterSelector = true
                }
                true
            }
            else -> false
        }
    } else {
        false
    }
}
```

### 2. Character Selector Dialog
**New Composable:** `CharacterSelectorDialog`

#### Features:
- **Full-screen overlay** with semi-transparent background
- **Grid layout** - 6 columns, automatically calculates rows
- **Large buttons** - 80x80dp for easy TV remote navigation
- **Focus management** - Auto-focuses first character
- **Back button handling** - Dismiss with back/backspace key

#### UI Layout:
```
┌─────────────────────────────────────┐
│        Jump to Letter               │
│                                     │
│   [A] [B] [C] [D] [E] [F]          │
│   [G] [H] [I] [J] [K] [L]          │
│   [M] [N] [O] [P] [Q] [R]          │
│   [S] [T] [V] [W] [X] [Y]          │
│                                     │
│   Press Back to cancel              │
└─────────────────────────────────────┘
```

#### Code:
```kotlin
@Composable
fun CharacterSelectorDialog(
    characters: List<Char>,
    onCharacterSelected: (Char) -> Unit,
    onDismiss: () -> Unit
) {
    // Full-screen overlay with grid of character buttons
    // Each button is 80x80dp with large text
    // Supports TV remote D-pad navigation
}
```

### 3. MediaBrowser Toolbar Integration
**File:** `ui/components/MediaBrowser.kt`

#### Changes:
- **Added toolbar focus requester** - `FocusRequester` for first toolbar button
- **Attached to first visible button**:
  - Home button when browsing a source
  - Add Source button on server selection screen
- **Passed to FileBrowserScreen** - Enables focus handoff from file list

#### Code:
```kotlin
val toolbarFocusRequester = remember { FocusRequester() }

// Attach to first button
if (currentSource != null) {
    Button(
        onClick = { viewModel.returnToSourceSelection() },
        modifier = Modifier.focusRequester(toolbarFocusRequester)
    ) {
        Icon(Icons.Default.Home, contentDescription = "Back to Servers")
    }
}

// Pass to file browser
FileBrowserScreen(viewModel, toolbarFocusRequester)
```

## User Experience

### Previous Behavior (Old):
- **Left Arrow** - Jump backward to previous first letter
- **Right Arrow** - Jump forward to next first letter
- **Feedback** - Small popup showing current letter

**Problems:**
- Unpredictable jumps (no preview of available letters)
- No easy way to access toolbar
- Trial and error to find desired letter

### New Behavior:
- **Left Arrow** - Instant focus to toolbar (access Home, Settings, etc.)
- **Right Arrow** - Opens dialog showing ALL available letters
- **Select Letter** - Click or press Enter on desired letter to jump
- **Visual Feedback** - See all available options at once

**Benefits:**
- ✅ Direct toolbar access from file list
- ✅ See all available letters before choosing
- ✅ Predictable, intentional navigation
- ✅ Better TV remote UX

## Navigation Flow Examples

### Scenario 1: Quick Toolbar Access
```
User browsing files → Presses Left → Focus moves to Home button → Can press Home/Settings/etc.
```

### Scenario 2: Jump to Letter
```
User on file starting with 'B' → Presses Right
→ Dialog appears showing [A, B, D, F, M, P, S, T, V]
→ User navigates to 'M' and presses Enter
→ Dialog closes, jumps to first file starting with 'M'
→ Brief popup shows "M" as confirmation
```

### Scenario 3: Cancel Dialog
```
User presses Right → Dialog appears
→ User presses Back/Backspace
→ Dialog closes, focus returns to current file
```

## Technical Details

### Focus Management
- **FocusRequester** used for both toolbar and dialog buttons
- **Auto-focus** on dialog open (first character button)
- **Focus restoration** after dialog closes (returns to selected file)

### Character Extraction
- Only includes non-".." entries
- Lowercase normalized for comparison
- Uppercase displayed for readability
- Sorted alphabetically

### Jump Logic
```kotlin
fun jumpToCharacter(char: Char) {
    val targetIndex = items.indexOfFirst { item ->
        getFirstChar(item)?.uppercaseChar() == char.uppercaseChar()
    }
    
    if (targetIndex != -1) {
        jumpLabel = char.toString().uppercase()
        showJumpChar = true  // Show brief confirmation popup
        scope.launch {
            // Scroll and focus
            if (isGridView) gridState.scrollToItem(targetIndex)
            else listState.scrollToItem(targetIndex)
            delay(50)
            awaitFrame()
            focusRequesters[targetIndex].requestFocus()
        }
    }
}
```

### Dialog Styling
- **Background**: Black 70% opacity overlay
- **Surface**: Black 95% opacity with rounded corners
- **Grid**: 6 columns, 12dp spacing
- **Buttons**: 80x80dp, rounded 8dp corners
- **Text**: Headline large for single characters

## Compatibility

### Backward Compatibility
- **Default parameter** for `onRequestToolbarFocus` - Existing FocusableList calls don't need updates
- **SourceSelectionScreen** - Works without toolbar focus (uses default empty callback)
- **All existing functionality preserved** - Up/Down navigation, Enter to select, etc.

### TV Remote Support
- **D-pad navigation** - All directions work in dialog
- **Enter** - Select character
- **Back** - Dismiss dialog
- **Channel/Skip buttons** - Still work for existing features

## Files Modified

1. ✅ `ui/components/FocusableList.kt`
   - Added `onRequestToolbarFocus` parameter
   - Replaced left/right key logic
   - Added `CharacterSelectorDialog` composable
   - Added character jump logic

2. ✅ `ui/components/MediaBrowser.kt`
   - Added `toolbarFocusRequester`
   - Attached to first toolbar button
   - Passed to `FileBrowserScreen`
   - Updated `FileBrowserScreen` signature

## Testing Checklist

### Basic Navigation:
- [ ] Press Left while browsing files → Focus moves to toolbar
- [ ] Press Right while browsing files → Character dialog appears
- [ ] Dialog shows only letters that exist in current folder
- [ ] Dialog grid is properly laid out (6 columns)

### Dialog Interaction:
- [ ] First character button is auto-focused on open
- [ ] D-pad navigation works in dialog grid
- [ ] Clicking character jumps to that letter
- [ ] Back button closes dialog without action
- [ ] Brief popup shows selected letter after jump

### Toolbar Focus:
- [ ] Left arrow focuses Home button when browsing
- [ ] Left arrow focuses Add Source button on server list
- [ ] Can navigate between toolbar buttons with D-pad
- [ ] Down arrow returns focus to file list

### Edge Cases:
- [ ] Empty folder - Right arrow does nothing
- [ ] Single letter - Dialog shows one button
- [ ] Many letters - Dialog handles multiple rows
- [ ] Grid view - Left/Right still work
- [ ] List view - Left/Right work correctly

## Known Limitations

1. **Grid view** - Left/Right still work (but less useful than in list view)
2. **No keyboard shortcuts** - Dialog requires D-pad/Enter navigation
3. **Single column** - Fixed at 6 columns (could be adaptive)

## Future Enhancements

### Potential Improvements:
1. **Adaptive columns** - Adjust based on screen size
2. **Letter preview** - Show count of files per letter
3. **Recent letters** - Remember last jumped-to letters
4. **Quick keys** - Allow typing letter directly (if keyboard available)
5. **Visual indicator** - Show which letter current file belongs to

## Conclusion

This update provides a more intuitive and predictable navigation experience:
- **Left arrow** gives quick access to toolbar actions
- **Right arrow** presents clear visual menu of jump options
- **Character dialog** eliminates guesswork in navigation

The changes maintain backward compatibility while significantly improving the TV remote browsing experience.
