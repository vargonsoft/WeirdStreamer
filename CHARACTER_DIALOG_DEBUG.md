# Character Dialog Debug Guide

## Issue
Right arrow in list view is not showing the character selection dialog.

## Debug Logging Added

I've added comprehensive logging to track the entire flow. When you press Right arrow in list view, you should see these log messages:

### Expected Log Sequence:

```
D/FocusableList: Right arrow pressed - uniqueFirstChars: X, isEmpty: false
D/FocusableList: Setting showCharacterSelector = true
D/FocusableList: LaunchedEffect: showCharacterSelector changed to: true
D/FocusableList: Checking showCharacterSelector: true
D/FocusableList: Showing CharacterSelectorDialog with X characters: [A, B, C, D, ...]
D/CharacterSelectorDialog: Dialog composing with X characters: [A, B, C, ...]
```

If the dialog appears and you select a character:
```
D/FocusableList: Character selected: A
```

If you dismiss the dialog:
```
D/FocusableList: Dialog dismissed
```

## What to Check

### 1. Check if Right Arrow is Being Detected
Look for this log message:
```
D/FocusableList: Right arrow pressed - uniqueFirstChars: X, isEmpty: false
```

**If you DON'T see this:**
- The key event isn't reaching the LazyColumn
- You might not be in list view (check if you're in grid view)
- Focus might be on a different component

**If you DO see this:**
- Continue to step 2

### 2. Check if Characters are Available
Look at the log message:
```
D/FocusableList: Right arrow pressed - uniqueFirstChars: 0, isEmpty: true
```

**If uniqueFirstChars is 0:**
- The folder might be empty
- All files might start with the same character
- Files might all be ".." (parent directory)

**If uniqueFirstChars > 0:**
- Continue to step 3

### 3. Check if Dialog State is Set
Look for:
```
D/FocusableList: Setting showCharacterSelector = true
```

**If you see this:**
- The state is being set correctly
- Continue to step 4

**If you DON'T see this:**
- There's a condition preventing it (likely no characters available)

### 4. Check if State Change Triggers Recomposition
Look for:
```
D/FocusableList: LaunchedEffect: showCharacterSelector changed to: true
```

**If you see this:**
- The state change is being observed by Compose
- Continue to step 5

**If you DON'T see this:**
- Compose is not recomposing when the state changes
- This is a serious recomposition bug

### 5. Check if Code Reaches Dialog Check
Look for:
```
D/FocusableList: Checking showCharacterSelector: true
```

**If you see "Checking showCharacterSelector: false":**
- The composable is recomposing but the state is false
- Something is resetting the state immediately

**If you see "Checking showCharacterSelector: true":**
- The state is correct when checking
- Continue to step 6

**If you DON'T see this at all:**
- The composable is not recomposing

### 6. Check if Dialog Composable is Called
Look for:
```
D/FocusableList: Showing CharacterSelectorDialog with X characters: [...]
```

**If you DON'T see this:**
- The `if (showCharacterSelector)` check is failing
- This would be a Compose recomposition issue
- The state might not be triggering recomposition

**If you DO see this:**
- The dialog should be visible
- If it's not, there's a rendering issue

## Testing Steps

1. **Build and install** the app with the new logging
2. **Navigate to a folder** with files starting with different letters (A, B, C, etc.)
3. **Switch to list view** (press the Grid/List toggle if needed)
4. **Focus on a file** in the list
5. **Press Right arrow** on your remote
6. **Check logcat** for the messages above

### Logcat Command:
```bash
adb logcat -s FocusableList:D
```

## Common Issues and Solutions

### Issue: No log messages at all
**Cause:** App not built with latest code
**Solution:** Clean build: `./gradlew clean :app:assembleDebug`

### Issue: "uniqueFirstChars: 0"
**Cause:** No valid first characters extracted
**Solution:** 
- Check that files don't all start with the same letter
- Check that folder has files other than ".."
- Verify `getFirstChar()` function logic

### Issue: Log shows state set but dialog doesn't show
**Cause:** Dialog composable not recomposing
**Solution:**
- Check that `showCharacterSelector` is a `remember { mutableStateOf(false) }` variable
- Verify the `if (showCharacterSelector)` is at the correct scope level
- Ensure the dialog is inside the same composition as the state

### Issue: "Right arrow pressed" not in logs but Left arrow works
**Cause:** Right arrow event being consumed elsewhere
**Solution:**
- Check if there's other key event handling higher in the composition tree
- Verify you're in list view, not grid view

## Code Locations

### Key Event Handler:
File: `ui/components/FocusableList.kt`
Line: ~160 (in LazyColumn modifier)

### Dialog Show Check:
File: `ui/components/FocusableList.kt`
Line: ~220 (after the Box containing LazyColumn/LazyVerticalGrid)

### Character Extraction:
File: `ui/components/FocusableList.kt`
Line: ~80 (uniqueFirstChars remember block)

## Next Steps Based on Logs

### If you see: "uniqueFirstChars: 0"
The issue is in character extraction. Check:
1. Are there files in the folder?
2. Do they start with different letters?
3. Is `getFirstChar()` returning null for all items?

### If you see: "Setting showCharacterSelector = true" but no dialog
The issue is in Compose recomposition. Check:
1. Is `showCharacterSelector` properly declared with `remember { mutableStateOf(false) }`?
2. Is the dialog check in the right scope?
3. Try adding `.background(Color.Red.copy(alpha = 0.5f))` to the dialog to see if it's rendering but invisible

### If you see: Dialog showing but not visible
The issue is rendering/styling. Check:
1. Z-index/elevation
2. Colors (might be transparent)
3. Size constraints

## Report Back

When you test, please provide:
1. The **complete log output** from logcat
2. What **view mode** you're in (list or grid)
3. What **folder** you're browsing (how many files, what names)
4. What **happens** when you press Right arrow

This will help diagnose the exact issue.
