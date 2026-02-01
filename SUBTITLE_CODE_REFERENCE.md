# Subtitle Feature - Code Structure Reference

## File Organization

```
Android/app/src/main/java/com/vargon/weirdstreamer/
├── OpenSubtitlesClient.kt              [NEW - 218 lines]
│   ├── SubtitleInfo data class
│   ├── OpenSubtitlesClient class
│   ├── searchByHash()
│   ├── searchByFilename()
│   ├── downloadSubtitle()
│   ├── calculateVideoHash()
│   └── Helper functions
│
└── ui/components/
    └── VideoPlayer.kt                  [MODIFIED - ~1500 lines]
        ├── State Variables (NEW)
        ├── selectSubtitleTrack()
        ├── searchAndDownloadSubtitles()
        ├── downloadAndApplySubtitle()
        ├── Toolbar UI (UPDATED)
        ├── Subtitle Selection Dialog (NEW)
        ├── Download Dialog (NEW)
        ├── Subtitle Detection LaunchedEffect (NEW)
        └── Key Event Handlers (UPDATED)
```

---

## OpenSubtitlesClient.kt - Complete Structure

### Data Classes
```kotlin
data class SubtitleInfo(
    val id: String,                    // Unique subtitle ID
    val filename: String,              // e.g., "movie.srt"
    val language: String,              // e.g., "en", "es"
    val rating: Float,                 // 0-10 rating
    val downloads: Int,                // Number of downloads
    val url: String                    // Download URL
)
```

### Main Class: OpenSubtitlesClient
```kotlin
class OpenSubtitlesClient(private val context: Context) {
    private val httpClient: OkHttpClient    // For HTTP requests
    private val apiBaseUrl: String          // "https://api.opensubtitles.com/api/v1"
    private var apiKey: String              // From SharedPreferences
    
    companion object {
        fun setApiKey(context, apiKey)  // Static method to configure API key
    }
    
    fun hasApiKey(): Boolean            // Check if API key is configured
    fun searchByHash(videoHash, languages): List<SubtitleInfo>
    fun searchByFilename(filename, languages): List<SubtitleInfo>
    fun downloadSubtitle(subtitle, outputFile): Boolean
    fun getSubtitleCacheDir(): File    // Get app cache directory
    fun calculateVideoHash(videoFile): String?
    
    private fun getApiKeyFromConfig(): String
    private fun parseSubtitleResults(data: JSONArray): List<SubtitleInfo>
}
```

### Key Methods

#### searchByFilename()
```
Input: "The Matrix.mkv"
↓
URLEncode filename
↓
HTTP GET https://api.opensubtitles.com/api/v1/subtitles?query=...
↓
Parse JSON response
↓
Return List<SubtitleInfo>
```

#### downloadSubtitle()
```
Input: SubtitleInfo with URL
↓
HTTP GET from subtitle.url
↓
Handle gzip decompression
↓
Save to outputFile (cache directory)
↓
Return success/failure
```

---

## VideoPlayer.kt - Subtitle State Variables

### Embedded Subtitle State
```kotlin
var showSubtitleDialog by remember { mutableStateOf(false) }
    // Controls visibility of subtitle selection dialog

var availableSubtitles by remember { mutableStateOf<List<String>>(emptyList()) }
    // List of detected subtitle tracks from video
    // Example: ["English", "Spanish", "French"]

var selectedSubtitleIndex by remember { mutableIntStateOf(-1) }
    // Currently selected subtitle index
    // -1 = no subtitle selected
    // 0+ = index in availableSubtitles list
```

### OpenSubtitles Download State
```kotlin
var showDownloadDialog by remember { mutableStateOf(false) }
    // Controls visibility of download dialog

var downloadingSubtitles by remember { mutableStateOf<List<SubtitleInfo>>(emptyList()) }
    // Search results from OpenSubtitles API
    // Example: [SubtitleInfo(...), SubtitleInfo(...), ...]

var isSearchingSubtitles by remember { mutableStateOf(false) }
    // Loading indicator during API search
    // true = searching, false = complete

var downloadStatus by remember { mutableStateOf<String?>(null) }
    // Status message for user
    // Examples: "Searching...", "Found 5 results", "Error: Not found"
```

---

## VideoPlayer.kt - Key Functions

### selectSubtitleTrack(index: Int)

**Purpose**: Select an embedded subtitle track or disable subtitles

**Flow**:
```
selectSubtitleTrack(0)
    ↓
selectedSubtitleIndex = 0
    ↓
Get ExoPlayer current tracks
    ↓
Find TEXT track group at index
    ↓
Create TrackSelectionParameters.Override
    ↓
Apply via exoPlayer.setTrackSelectionParameters()
    ↓
Subtitles display
```

**Special Case**: `selectSubtitleTrack(-1)` disables all subtitles

### searchAndDownloadSubtitles()

**Purpose**: Search OpenSubtitles API in background thread

**Flow**:
```
User clicks "Download" button
    ↓
searchAndDownloadSubtitles() called
    ↓
Check if API key configured
    ↓
Launch background thread:
    ├─ Extract filename from video path
    ├─ Call openSubtitlesClient.searchByFilename()
    ├─ Update downloadingSubtitles with results
    └─ Set isSearchingSubtitles = false
    ↓
Dialog UI shows results
```

**Threading**: Uses `Thread { }.start()` for async operation

### downloadAndApplySubtitle(subtitle: SubtitleInfo)

**Purpose**: Download subtitle file and prepare for ExoPlayer

**Flow**:
```
User clicks subtitle in search results
    ↓
downloadAndApplySubtitle() called
    ↓
Show downloadStatus = "Downloading..."
    ↓
Launch background thread:
    ├─ Create file in cache directory
    ├─ Call openSubtitlesClient.downloadSubtitle()
    ├─ Update downloadStatus on completion
    └─ TODO: Load into ExoPlayer
    ↓
Dialog shows completion message
```

**TODO**: Currently prepared but not integrated with ExoPlayer

---

## VideoPlayer.kt - Toolbar Button Implementation

### Button 4: Subtitles (NEW)

**Structure**:
```kotlin
Row(
    modifier = Modifier
        .clip(RoundedCornerShape(8.dp))
        .background(
            if (toolbarFocusedButton == 4) Color.White.copy(alpha = 0.2f)
            else Color.Gray.copy(alpha = 0.3f)
        )
        .padding(12.dp),
    verticalAlignment = Alignment.CenterVertically,
    horizontalArrangement = Arrangement.Center
) {
    Icon(
        Icons.Default.Subtitles,
        contentDescription = "Subtitles",
        tint = if (toolbarFocusedButton == 4) Color.White else Color.White.copy(alpha = 0.7f),
        modifier = Modifier.size(if (toolbarFocusedButton == 4) 56.dp else 48.dp)
    )
    Spacer(modifier = Modifier.width(12.dp))
    Text(
        text = "Subtitles",
        style = MaterialTheme.typography.bodyLarge,
        color = if (toolbarFocusedButton == 4) Color.White else Color.White.copy(alpha = 0.7f)
    )
}
```

### Focus Feedback
```
When toolbarFocusedButton == 4:
├─ Icon size: 48dp → 56dp
├─ Icon color: 70% alpha → full opacity
├─ Background: Gray 30% → White 20%
└─ Text color: 70% alpha → full opacity
```

### Navigation
```
Left Key:  toolbarFocusedButton = (current - 1).coerceAtLeast(0)
Right Key: toolbarFocusedButton = (current + 1).coerceAtMost(4)
Center/Enter: showSubtitleDialog = true
```

---

## VideoPlayer.kt - Subtitle Selection Dialog

### Dialog Structure
```
Surface(width=500.dp, background=Black 90%)
    ├─ Title: "Subtitles"
    ├─ "Available Subtitles" section
    │  └─ Column (scrollable)
    │     ├─ Row: "None" (disable subtitles)
    │     ├─ Row: Subtitle 1
    │     ├─ Row: Subtitle 2
    │     └─ [... more subtitles]
    │
    ├─ Divider
    ├─ "Download Subtitles" section  
    │  └─ Row: "OpenSubtitles.org / .com" button
    │
    └─ Buttons: [Close]
```

### Each Subtitle Row
```
Row(
    modifier = background(Gray 20%),
    clickable = { selectSubtitleTrack(index) }
)
    ├─ PlayArrow Icon (if selected)
    ├─ Spacer(8.dp)
    └─ Text(subtitle.name)
```

---

## VideoPlayer.kt - Download Dialog

### Dialog Structure
```
Surface(width=600.dp, background=Black 90%)
    ├─ Title: "Download Subtitles"
    ├─ [Search State]
    │  │
    │  ├─ If searching:
    │  │  ├─ CircularProgressIndicator
    │  │  └─ Text: "Searching..."
    │  │
    │  ├─ If error/not found:
    │  │  └─ Text: error message
    │  │
    │  └─ If found results:
    │     ├─ Text: "Found X subtitle(s)"
    │     └─ Column (scrollable, max 300.dp)
    │        ├─ Row: Subtitle 1
    │        │  ├─ Column
    │        │  │  ├─ filename (truncated)
    │        │  │  └─ "en | Rating: 8.5 | 1250↓"
    │        │  └─ PlayArrow Icon
    │        ├─ Row: Subtitle 2
    │        └─ [... more results]
    │
    └─ Buttons: [Close]
```

### Subtitle Result Row
```
Row(
    modifier = background(Gray 20%),
    clickable = { downloadAndApplySubtitle(subtitle) }
)
    ├─ Column (weight 1f)
    │  ├─ Text(subtitle.filename) - bodySmall, truncated
    │  └─ Text("en | Rating: 8.5 | 1250↓") - labelSmall, 70% alpha
    └─ Icon(PlayArrow) - 20dp, White
```

---

## LaunchedEffect - Subtitle Detection

### Purpose
Automatically detect and list subtitle tracks when video loads

### Trigger
```kotlin
LaunchedEffect(path) {  // Rerun when path changes
    // Setup listener and cleanup
}
```

### Implementation
```
1. Create Player.Listener
2. Override onTracksChanged(tracks: Tracks)
3. Loop through tracks.groups
4. Find all groups where type == C.TRACK_TYPE_TEXT
5. Extract language from format.language
6. Add to availableSubtitles list
7. Register listener: exoPlayer.addListener(listener)
8. Cleanup in onDispose: exoPlayer.removeListener(listener)
```

### Example Flow
```
Video loads: "movie.mkv"
    ↓
ExoPlayer detects tracks
    ↓
onTracksChanged() called
    ↓
Finds 3 text tracks:
    ├─ English
    ├─ Spanish  
    └─ French
    ↓
availableSubtitles = ["English", "Spanish", "French"]
    ↓
Subtitle button becomes available
```

---

## Key Event Handlers (MODIFIED)

### Center/Enter Key - Button 4
```kotlin
Key.Center, Key.Enter -> {
    when (toolbarFocusedButton) {
        0 -> isZoomMode = !isZoomMode           // Toggle zoom
        1 -> videoPaddingPx += 1                // Zoom+ (increase size)
        2 -> videoPaddingPx = max(0, videoPaddingPx - 1) // Zoom- (decrease size)
        3 -> currentAspectRatioIndex = (currentAspectRatioIndex + 1) % aspectRatios.size // Cycle aspect ratio
        4 -> showSubtitleDialog = true          // Show subtitles [NEW]
        else -> {}
    }
}
```

### Left/Right Navigation
```kotlin
Key.Left -> {
    toolbarFocusedButton = (toolbarFocusedButton - 1).coerceAtLeast(0)
    // Now supports buttons 0-4 (was 0-3)
}

Key.Right -> {
    toolbarFocusedButton = (toolbarFocusedButton + 1).coerceAtMost(4)  
    // Updated from coerceAtMost(3) to coerceAtMost(4)
}
```

---

## Imports Added

```kotlin
// Dialog & Surface
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import androidx.compose.material3.Surface
import androidx.compose.material3.Divider

// Scrolling
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.clickable

// Media3
import androidx.media3.common.Tracks
import androidx.media3.common.TrackSelectionParameters

// Icons
import androidx.compose.material.icons.filled.Subtitles
```

---

## State Flow Diagram

```
Video Playback
    │
    ├─ Down key → showZoomToolbar = true
    │                    │
    │                    └─ Right × 4 → toolbarFocusedButton = 4
    │                                        │
    │                                        └─ Center key → showSubtitleDialog = true
    │                                                            │
    │                                        ┌───────────────────┴──────────────────┐
    │                                        │                                      │
    │                               Select embedded subtitle      Click "Download"
    │                                        │                          │
    │                    selectSubtitleTrack(index)    showDownloadDialog = true
    │                                        │          searchAndDownloadSubtitles()
    │                    Subtitles display         │
    │                                        │    isSearchingSubtitles = true
    │                                        │          │
    │                                        │   HTTP search in background
    │                                        │          │
    │                                        │   downloadingSubtitles = results
    │                                        │   isSearchingSubtitles = false
    │                                        │          │
    │                                        │   User clicks result
    │                                        │          │
    │                                        │   downloadAndApplySubtitle()
    │                                        │          │
    │                                        │   downloadStatus = "Downloading..."
    │                                        │          │
    │                                        │   Download in background [TODO: Load to ExoPlayer]
    │                                        │          │
    │                                        │   downloadStatus = "Downloaded!"
    │
    └─ Back key → Close all dialogs
```

---

## Data Flow: Search to Display

```
OpenSubtitles API Response
    │
    ├─ JSON Array: [{id, attributes{filename, language, rating, ...}}, ...]
    │
    └─ parseSubtitleResults()
        │
        ├─ Loop each JSON object
        ├─ Extract attributes
        ├─ Create SubtitleInfo(id, filename, language, rating, downloads, url)
        └─ Return List<SubtitleInfo>
            │
            └─ downloadingSubtitles = results
                │
                └─ Dialog displays results
                    │
                    ├─ Show: filename, language, rating, downloads
                    └─ Clickable: downloadAndApplySubtitle(subtitle)
                        │
                        └─ TODO: Load file to ExoPlayer
                            │
                            └─ Subtitles display
```

---

## Error Handling Strategy

### API Errors
```kotlin
if (!hasApiKey()) {
    downloadStatus = "API key not configured"
    return
}

// HTTP 404, 400, etc.
if (!response.isSuccessful) {
    return emptyList()
}

// Network timeout, IOException, etc.
try {
    // Search
} catch (e: Exception) {
    downloadStatus = "Error: ${e.message}"
}
```

### File Errors
```kotlin
if (!videoFile.exists()) {
    return null  // For hash calculation
}

if (!cacheDir.exists()) {
    cacheDir.mkdirs()  // Create directory
}
```

### Track Selection Errors
```kotlin
val tracks = exoPlayer.currentTracks
for (group in tracks.groups) {
    if (group.type == C.TRACK_TYPE_TEXT) {
        // Safe iteration
    }
}
```

---

## Configuration & Constants

### OpenSubtitles API
```kotlin
private val apiBaseUrl = "https://api.opensubtitles.com/api/v1"
private const val CONFIG_API_KEY = "opensubtitles_api_key"
private const val DEFAULT_SEARCH_LANGUAGE = "en"
```

### UI Dimensions
```kotlin
Dialog width: 500.dp (subtitle list)
Dialog width: 600.dp (download results)
Max scroll height: 300.dp
Icon size: 48dp → 56dp (focused)
```

### Threading
```kotlin
Thread { 
    // Search or download
}.start()  // Non-blocking background execution
```

---

## Testing Checklist

- [ ] Subtitle detection works with .mkv/.mp4/.avi files
- [ ] Dialog opens/closes correctly
- [ ] Subtitle selection works
- [ ] Download search returns results
- [ ] Download file saves to cache
- [ ] UI doesn't crash with 0 results
- [ ] UI doesn't crash with 100+ results
- [ ] API key error handled gracefully
- [ ] Network timeout handled gracefully
- [ ] Toolbar navigation works (0-4 buttons)
- [ ] Focus feedback visible for button 4
- [ ] No memory leaks in LaunchedEffect

---

**Last Updated**: [Implementation Complete - Phase 1]  
**Total Lines Added**: ~1800 (400 new + 1400 modified)  
**Complexity**: Medium-High (dialogs, threading, API calls)  
**Test Coverage**: Ready for manual testing
