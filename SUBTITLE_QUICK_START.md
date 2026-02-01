# Subtitle Feature - Quick Start for Developers

## 5-Minute Overview

The subtitle feature allows Android TV users to:
1. **Select** embedded subtitles from videos
2. **Download** subtitles from OpenSubtitles.com/org
3. **Manage** subtitles through dedicated UI

**Architecture**: Two dialogs (embed select + download) controlled from toolbar button #4

---

## File Structure

```
New File (218 lines):
  OpenSubtitlesClient.kt
  ├─ API calls to OpenSubtitles
  ├─ File download & caching
  └─ API key management

Modified (1300+ lines added):
  VideoPlayer.kt
  ├─ Subtitle state variables (7 new)
  ├─ Toolbar button #4 (Subtitles)
  ├─ Subtitle selection dialog
  ├─ Download dialog
  ├─ Track detection LaunchedEffect
  └─ Key event handlers
```

---

## State Variables Quick Reference

```kotlin
// Embedded subtitles (detected from video)
var showSubtitleDialog: Boolean
var availableSubtitles: List<String>  // ["English", "Spanish", ...]
var selectedSubtitleIndex: Int        // -1 = none, 0+ = index

// Download subtitles (from OpenSubtitles)
var showDownloadDialog: Boolean
var downloadingSubtitles: List<SubtitleInfo>
var isSearchingSubtitles: Boolean
var downloadStatus: String?           // Status message for user
```

---

## Main Functions

### selectSubtitleTrack(index: Int)
```kotlin
// Select embedded subtitle
selectSubtitleTrack(0)  // Select first subtitle
selectSubtitleTrack(-1) // Disable all subtitles
```

### searchAndDownloadSubtitles()
```kotlin
// Called when user clicks "Download" button
// Searches OpenSubtitles API in background thread
// Updates: isSearchingSubtitles, downloadingSubtitles, downloadStatus
```

### downloadAndApplySubtitle(subtitle: SubtitleInfo)
```kotlin
// Called when user clicks a search result
// Downloads file to app cache
// TODO in Phase 2: Load to ExoPlayer
```

---

## User Flow: Selecting Embedded Subtitle

```
Down Key
  ↓
showZoomToolbar = true
  ↓
Right × 4
  ↓
toolbarFocusedButton = 4 (Subtitles button)
  ↓
Center/Enter
  ↓
showSubtitleDialog = true
  ↓
Dialog renders with availableSubtitles
  ↓
User presses Up/Down to select
  ↓
User presses Center to select
  ↓
selectSubtitleTrack(selectedIndex)
  ↓
Subtitles display on video
```

---

## User Flow: Downloading Subtitle

```
In Subtitle Dialog
  ↓
Click "OpenSubtitles.org / .com"
  ↓
showDownloadDialog = true
  ↓
searchAndDownloadSubtitles() (background thread)
  ↓
isSearchingSubtitles = true (spinner shows)
  ↓
OpenSubtitlesClient.searchByFilename(videoname)
  ↓
HTTP GET to OpenSubtitles API
  ↓
Parse JSON response
  ↓
downloadingSubtitles = results
  ↓
isSearchingSubtitles = false (spinner hides)
  ↓
Results render in dialog
  ↓
User clicks result
  ↓
downloadAndApplySubtitle(subtitle)
  ↓
downloadStatus = "Downloading..."
  ↓
OpenSubtitlesClient.downloadSubtitle()
  ↓
File saved to app cache
  ↓
downloadStatus = "Downloaded!"
  ↓
TODO: Load to ExoPlayer
```

---

## Configuration

### OpenSubtitles API Key

**Get Key**:
1. Visit https://www.opensubtitles.com/api
2. Sign up (free)
3. Create API key
4. Copy key

**Set in App**:
```kotlin
// In your initialization code
OpenSubtitlesClient.setApiKey(context, "your-api-key-here")

// Or in SharedPreferences directly
val prefs = context.getSharedPreferences("opensubtitles_config", Context.MODE_PRIVATE)
prefs.edit().putString("opensubtitles_api_key", "your-key").apply()
```

**Check if Set**:
```kotlin
val client = OpenSubtitlesClient(context)
if (client.hasApiKey()) {
    // Can search
} else {
    // Show error: "API key not configured"
}
```

---

## Building & Running

```bash
# Build
./gradlew :app:assembleDebug

# Install to device
./gradlew :app:installDebug

# Or run through Android Studio
```

---

## Testing Checklist

### Manual Tests
- [ ] Open video with embedded subtitles
- [ ] Down key → toolbar appears
- [ ] Right 4× → Subtitles button focused
- [ ] Center key → dialog opens
- [ ] Navigate subtitles with Up/Down
- [ ] Select subtitle → displays on video
- [ ] Select "None" → subtitles disappear
- [ ] Click "Download" → search dialog
- [ ] Search completes (with API key)
- [ ] Results show in dialog
- [ ] Click result → download starts
- [ ] Status message updates

### Edge Cases
- [ ] Video with NO subtitles → download works
- [ ] No API key → error message shows
- [ ] Network error → handled gracefully
- [ ] 100+ results → scrollable list
- [ ] Close dialog → state resets

---

## Common Tasks

### Task: Add Language Selection
Location: `searchAndDownloadSubtitles()` function

Change from:
```kotlin
val results = openSubtitlesClient.searchByFilename(filename)
```

To:
```kotlin
val results = openSubtitlesClient.searchByFilename(filename, "en,es,fr")
```

### Task: Change Search Language
Location: `searchAndDownloadSubtitles()` function

```kotlin
val results = openSubtitlesClient.searchByFilename(
    filename, 
    "es"  // Search Spanish instead of English
)
```

### Task: Show Only Top 5 Results
Location: Download dialog UI

Change from:
```kotlin
downloadingSubtitles.take(10)
```

To:
```kotlin
downloadingSubtitles.take(5)
```

### Task: Change Dialog Width
Locations: 
- Subtitle dialog: line ~1160, `width(500.dp)`
- Download dialog: line ~1380, `width(600.dp)`

```kotlin
// Make wider
width(700.dp)

// Or responsive
width(minOf(LocalConfiguration.current.screenWidthDp.dp - 48.dp, 800.dp))
```

---

## Debugging Tips

### Enable Logging
```kotlin
// Add to OpenSubtitlesClient search
android.util.Log.d("OpenSubtitles", "Searching for: $filename")

// Add to VideoPlayer
android.util.Log.d("VideoPlayer", "Subtitle selected: ${availableSubtitles[index]}")
```

### Check State
```kotlin
// In Compose Lambda
Row {
    Text("State: showDialog=$showSubtitleDialog, selected=$selectedSubtitleIndex")
}
```

### Test API Key
```kotlin
val client = OpenSubtitlesClient(context)
if (!client.hasApiKey()) {
    Log.e("OpenSubtitles", "API key not set!")
}
```

### Monitor Background Threads
```kotlin
Thread {
    try {
        Log.d("Thread", "Search started")
        val results = client.searchByFilename(filename)
        Log.d("Thread", "Found ${results.size} results")
    } catch (e: Exception) {
        Log.e("Thread", "Error", e)
    }
}.start()
```

---

## Phase 2 Preparation

### Upcoming Integration: SubtitleConfiguration

In Phase 2, replace TODO in `downloadAndApplySubtitle()`:

```kotlin
// Phase 2: Load subtitle to ExoPlayer
val subtitleUri = Uri.fromFile(outputFile)
val subtitleConfig = MediaItem.SubtitleConfiguration.Builder(subtitleUri)
    .setMimeType("application/x-subrip")  // For .srt files
    .setLanguage("en")
    .build()

val currentItem = exoPlayer.currentMediaItem ?: return@Thread
val newItem = currentItem.buildUpon()
    .setSubtitleConfigurations(listOf(subtitleConfig))
    .build()

exoPlayer.setMediaItem(newItem)
exoPlayer.prepare()
```

---

## Important Notes

### Threading
- ⚠️ **Network calls run in background threads**
- ✅ **State updates automatically sync to UI** (mutableStateOf)
- ⚠️ **Don't call exoPlayer methods from background thread**

### Memory
- 💾 **Subtitles cached in app cache** (auto-deleted on app uninstall)
- 💾 **Large result sets scrollable** (only renders visible items)
- 💾 **Dialogs disposed when closed** (cleans up state)

### Compatibility
- ✅ **ExoPlayer 1.0+** (uses Tracks API)
- ✅ **Android 8.0+** (for Compose)
- ✅ **All Android TV devices**

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| "API key not set" | Call `OpenSubtitlesClient.setApiKey(context, key)` |
| No results found | Check internet, try different video name |
| Subtitles not showing | Make sure subtitle was selected (not just downloaded) |
| Crash on download | Check `downloadStatus` message, check logs |
| Dialog not appearing | Check `showSubtitleDialog` state |
| Can't navigate buttons | Check `toolbarFocusedButton` range (0-4) |

---

## Quick Links

- **Full Implementation Docs**: `SUBTITLE_IMPLEMENTATION.md`
- **User Guide**: `SUBTITLE_USER_GUIDE.md`
- **Phase 2 Plan**: `SUBTITLE_PHASE2_PLAN.md`
- **Code Reference**: `SUBTITLE_CODE_REFERENCE.md`
- **OpenSubtitles API**: https://www.opensubtitles.com/api
- **ExoPlayer Docs**: https://developer.android.com/media/media3

---

## Need Help?

1. **Compilation errors?** → Check imports in VideoPlayer.kt
2. **Runtime crashes?** → Check logcat for exceptions
3. **State issues?** → Use remember { } for all mutableState variables
4. **UI not showing?** → Check if dialog/button visibility is true
5. **Downloads failing?** → Verify API key and network connectivity

---

**Status**: Ready for development ✅
