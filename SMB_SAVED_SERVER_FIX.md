# SMB Saved Server Connection Fix

## Problem
Saved SMB servers were failing to connect with an error:
```
jcifs.smb.SmbException: Failed to connect: babylon5/192.168.94.20
```

This error shows that the path being constructed contains an unexpected slash and IP address, suggesting the saved server value was malformed or being parsed incorrectly.

## Root Causes

### 1. Saved Server Values May Contain Path Components
The saved server hostnames might inadvertently contain path components like `babylon5/192.168.94.20` instead of just `babylon5`. When `selectSmbSource` was called with this value, it would create:
```
smb://babylon5/192.168.94.20/
```

This is invalid because the path after the hostname should be a share name, not another hostname/IP.

### 2. Servers Not Saved After Successful Browse
When browsing an SMB share successfully (without authentication), the server was not being added to the saved servers list. This could lead to inconsistencies where discovered servers weren't persisted.

## Solutions

### 1. Defensive Hostname Extraction in `selectSmbSource`
**File:** `MediaViewModel.kt`

Added defensive code to extract only the hostname, removing any path components:

**Before:**
```kotlin
fun selectSmbSource(host: String) {
    val source = MediaSource.Smb(host, "", "")
    loadPath(source, "smb://$host/")
}
```

**After:**
```kotlin
fun selectSmbSource(host: String) {
    // Extract just the hostname, removing any path components or smb:// prefix
    val cleanHost = host.removePrefix("smb://").substringBefore("/").trim()
    android.util.Log.d("MediaViewModel", "selectSmbSource - input: '$host', cleaned: '$cleanHost'")
    val source = MediaSource.Smb(cleanHost, "", "")
    loadPath(source, "smb://$cleanHost/")
}
```

This ensures that even if the saved value is `babylon5/192.168.94.20`, we extract just `babylon5`.

### 2. Save Server After Successful Browse
**File:** `MediaViewModel.kt`

Added code to save the SMB host to the saved servers list after a successful browse:

```kotlin
if (result.isSuccess) {
    val items = result.getOrNull() ?: emptyList()
    
    // Save SMB host to saved servers list after successful browse
    if (source is MediaSource.Smb) {
        val host = browsePath.removePrefix("smb://").substringBefore("/")
        android.util.Log.d("MediaViewModel", "Saving SMB host after successful browse: $host from path: $browsePath")
        credentialManager.addSavedServer(host)
    }
    
    // ... rest of the code
}
```

This ensures that:
- Servers are persisted after successful browsing
- The hostname is properly extracted from the SMB path
- The saved value is always just the hostname (e.g., `babylon5`)

## How It Works

### Hostname Extraction:
For various inputs:
1. **`babylon5`** → `cleanHost = "babylon5"` ✅
2. **`babylon5/192.168.94.20`** → `cleanHost = "babylon5"` ✅ (removes path)
3. **`smb://babylon5/`** → `cleanHost = "babylon5"` ✅ (removes prefix)
4. **`smb://babylon5/share`** → `cleanHost = "babylon5"` ✅ (removes share)

### Path Construction:
After cleaning the hostname, the path is always:
```
smb://hostname/
```

For example:
- Input: `babylon5/192.168.94.20`
- Cleaned: `babylon5`
- Path: `smb://babylon5/`
- Browse: Lists shares on `babylon5`

### Saving After Browse:
When browsing `smb://babylon5/` succeeds:
1. Extract host: `browsePath.removePrefix("smb://").substringBefore("/")` → `babylon5`
2. Save to credentials: `credentialManager.addSavedServer("babylon5")`
3. Next time app loads: `getSavedServers()` returns `["babylon5"]`
4. Click on saved server: `selectSmbSource("babylon5")` → works correctly

## Debug Logging

Added logging to track hostname extraction and saving:

```kotlin
D/MediaViewModel: selectSmbSource - input: 'babylon5/192.168.94.20', cleaned: 'babylon5'
D/MediaViewModel: Saving SMB host after successful browse: babylon5 from path: smb://babylon5/
```

This helps verify:
1. What value was stored as a saved server
2. How it's being cleaned before use
3. What's being saved after successful browse

## Files Modified

1. ✅ `MediaViewModel.kt` - Added defensive hostname extraction in `selectSmbSource()`
2. ✅ `MediaViewModel.kt` - Save server after successful SMB browse

## Testing

### Fix Existing Malformed Saved Servers:
1. If you have saved servers that won't connect, try clicking on them
2. **Expected:** The hostname is cleaned (e.g., `babylon5/192.168.94.20` → `babylon5`)
3. **Check logs:** Should see `selectSmbSource - input: '...', cleaned: '...'`
4. Connection should succeed

### Save New Servers:
1. Discover or manually add an SMB server
2. Browse successfully
3. **Expected:** Server is added to saved list with just the hostname
4. **Check logs:** Should see `Saving SMB host after successful browse: ...`
5. Close and reopen app
6. **Expected:** Saved server appears and connects successfully

### Verify Clean Storage:
1. After the fix, all future saves will be clean hostnames only
2. Existing malformed saves will be cleaned when used
3. After first successful connection, the save will be overwritten with the clean value

## Verification Checklist

- [x] Added defensive hostname extraction in `selectSmbSource()`
- [x] Added server saving after successful browse
- [x] Added debug logging
- [x] Code compiles without errors
- [ ] Existing saved servers connect successfully (test required)
- [ ] New saved servers store only hostname (test required)
- [ ] Malformed saved values are cleaned on use (test required)

## Prevention

This fix prevents future issues by:
1. **Always cleaning input** - Any malformed input is sanitized
2. **Saving after browse** - Ensures saved values are consistent
3. **Proper extraction** - Uses `substringBefore("/")` to get only hostname
4. **Logging** - Makes it easy to diagnose if issues recur

## Related Code Patterns

The hostname extraction pattern is used consistently:
- **`selectSmbSource()`** - Cleans input before creating source
- **`SmbRepository.browse()`** - Extracts host from path
- **After successful browse** - Extracts host from browsePath

All use the same pattern:
```kotlin
val host = path.removePrefix("smb://").substringBefore("/").trim()
```

This ensures consistency across the codebase.
