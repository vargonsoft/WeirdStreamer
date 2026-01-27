# Home Button Error State Fix

## Problem
When a connection error occurred while browsing a WS Server (or SMB source), clicking the newly added Home button would:
1. Make the Home button disappear (because `currentSource` was cleared)
2. **But the error message remained on the screen**
3. The server selection screen was not shown

This left the user in a broken state where they couldn't see the server list and couldn't navigate.

## Root Cause

### Error Display Logic
In `MediaBrowser.kt`, the content area uses a `when` statement with this order:
```kotlin
when {
    viewModel.isLoading -> { /* show loading */ }
    viewModel.errorMessage != null -> { /* show error */ }  // ⬅️ Checked BEFORE currentSource
    viewModel.authRequiredHost != null -> { /* show auth */ }
    currentSource == null -> { /* show server selection */ }  // ⬅️ Checked AFTER error
    else -> { /* show file browser */ }
}
```

### The Bug
When `returnToSourceSelection()` was called by the Home button, it only cleared:
- ✅ `currentSource = null`
- ✅ `currentPath = ""`
- ✅ `fileList = emptyList()`
- ❌ **Did NOT clear** `errorMessage`

**Result:** 
- `currentSource` became `null` (Home button disappeared)
- `errorMessage` was still set (error screen still shown)
- Server selection screen never shown (because error check comes first)

## Solution

Updated `returnToSourceSelection()` in `MediaViewModel.kt` to also clear the error message:

**Before:**
```kotlin
fun returnToSourceSelection() {
    currentSource = null
    currentPath = ""
    fileList = emptyList()
    refreshSources()
}
```

**After:**
```kotlin
fun returnToSourceSelection() {
    android.util.Log.d("MediaViewModel", "returnToSourceSelection - clearing source, path, error, and file list")
    currentSource = null
    currentPath = ""
    errorMessage = null  // ⬅️ ADDED: Clear error message
    fileList = emptyList()
    refreshSources()
}
```

## How It Works Now

### Error State → Home Button Flow:
1. **Error occurs** (e.g., server connection failed)
   - `errorMessage = "Failed to connect..."`
   - `currentSource = MediaSource.Server(...)`
   - Error screen shown with "Retry" button

2. **User clicks Home button**
   - Calls `returnToSourceSelection()`
   - Clears: `currentSource`, `currentPath`, `errorMessage`, `fileList`
   - Calls `refreshSources()` to reload server list

3. **UI updates**
   - `when` statement checks:
     - ✅ `isLoading`? No
     - ✅ `errorMessage != null`? No (now cleared!)
     - ✅ `authRequiredHost != null`? No
     - ✅ `currentSource == null`? Yes
   - Shows server selection screen ✅

## Files Modified

1. ✅ `MediaViewModel.kt` - Added `errorMessage = null` to `returnToSourceSelection()`
2. ✅ `HOME_BUTTON_FEATURE.md` - Updated documentation to reflect error clearing

## Testing

### Before Fix (Broken):
1. Try to connect to a server that's down or has wrong credentials
2. See error message
3. Click Home button
4. **Bug:** Error message still shown, server list not visible, Home button gone
5. **Stuck:** No way to navigate (had to use Back navigation or restart app)

### After Fix (Working):
1. Try to connect to a server that's down or has wrong credentials
2. See error message
3. Click Home button
4. **Expected:** Error cleared, server selection screen shown
5. Can select a different server or retry

### Specific Test Cases:

#### Case 1: Server Connection Error
```
1. Add WS Server with wrong URL/credentials
2. Try to browse → Error: "Server error: 401 Unauthorized"
3. Click Home button
4. Expected: Return to server list, error gone
```

#### Case 2: SMB Connection Error
```
1. Try to connect to offline SMB server
2. Error: "Failed to connect: ..."
3. Click Home button
4. Expected: Return to server list, error gone
```

#### Case 3: Network Error
```
1. Disconnect network, try to browse server
2. Error: "Network error..."
3. Click Home button
4. Expected: Return to server list, error gone
```

## Debug Logging

Added logging to track when state is cleared:
```
D/MediaViewModel: returnToSourceSelection - clearing source, path, error, and file list
```

This helps verify:
- When the method is called
- That all state is being properly cleared

## Related Issues

This fix also improves the behavior when:
- Using BackHandler to navigate up from an error state
- Pressing backspace key from an error state
- Any other code path that calls `returnToSourceSelection()`

All these paths now properly clear the error message.

## Why This Matters

### User Experience Impact:
- **Before:** Users could get stuck in error state, requiring app restart
- **After:** Users can always recover by clicking Home
- **Confidence:** Users know they can always get back to a working state

### Navigation Consistency:
- Home button now properly resets **all** browsing state
- No partial state left behind
- Clean slate when returning to server selection

### Error Recovery:
- Provides a clear escape hatch from error states
- Doesn't require backing through navigation history
- One-click recovery to known-good state

## Conclusion

The fix ensures that clicking the Home button always returns to a clean server selection screen state, regardless of whether an error was present. This provides a reliable way for users to recover from any error condition and continue using the app.
