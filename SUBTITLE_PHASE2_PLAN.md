# Subtitle Feature - Phase 2 Implementation Plan

## Current Status: Phase 1 ✅ COMPLETE

All subtitle UI, dialogs, and OpenSubtitles API integration is ready and tested. The foundation is solid and compiles without errors.

---

## Phase 2: ExoPlayer Integration & Settings

### Task 1: Load Downloaded Subtitles into ExoPlayer
**Priority**: HIGH  
**Time Estimate**: 2-3 hours  
**Files to Modify**: `VideoPlayer.kt`, `OpenSubtitlesClient.kt`

**What needs to be done**:
1. In `downloadAndApplySubtitle()`, after successful download:
   - Get the subtitle file path
   - Create `SubtitleConfiguration` with proper MIME type
   - Create new `MediaItem` with subtitle configuration
   - Set media item on ExoPlayer

**Code snippet**:
```kotlin
// After downloadSubtitle() returns true
val subtitleUri = Uri.fromFile(outputFile)
val subtitleConfig = MediaItem.SubtitleConfiguration.Builder(subtitleUri)
    .setMimeType(MimeTypes.APPLICATION_SUBRIP) // or appropriate MIME type
    .setLanguage(subtitle.language)
    .build()

val currentItem = exoPlayer.currentMediaItem ?: return
val newItem = currentItem.buildUpon()
    .setSubtitleConfigurations(listOf(subtitleConfig))
    .build()

exoPlayer.setMediaItem(newItem)
exoPlayer.prepare()
```

**Testing**:
- Download a subtitle
- Verify it displays on video
- Try different subtitle formats (.srt, .ass, .vtt)

---

### Task 2: Add API Key Configuration to Settings Screen
**Priority**: HIGH  
**Time Estimate**: 1-2 hours  
**Files to Modify**: `SettingsScreen.kt` (or equivalent)

**What needs to be done**:
1. Add new settings section "OpenSubtitles Configuration"
2. Create text input field for API key
3. Add validation (non-empty check)
4. Add "Save" button
5. Show confirmation/error messages
6. Store using `OpenSubtitlesClient.setApiKey()`

**UI Design**:
```
┌────────────────────────────────┐
│ OpenSubtitles Configuration    │
├────────────────────────────────┤
│ API Key:                       │
│ [_________________ (hidden)]    │
│                                │
│ [Get API Key] [Save] [Cancel]  │
├────────────────────────────────┤
│ Status: ✓ API Key Configured  │
└────────────────────────────────┘
```

**Testing**:
- Set API key successfully
- Try searching with API key
- Verify key persists across app restarts

---

### Task 3: Detect Subtitle Format and Set Correct MIME Type
**Priority**: MEDIUM  
**Time Estimate**: 1 hour  
**Files to Modify**: `OpenSubtitlesClient.kt`, `VideoPlayer.kt`

**What needs to be done**:
1. Extract file extension from subtitle filename
2. Map extension to MIME type
3. Pass MIME type to SubtitleConfiguration

**Supported Formats**:
```kotlin
val mimeTypeMap = mapOf(
    "srt" to MimeTypes.APPLICATION_SUBRIP,
    "ass" to MimeTypes.TEXT_SSA,
    "ssa" to MimeTypes.TEXT_SSA,
    "sub" to MimeTypes.APPLICATION_SUBRIP,
    "vtt" to MimeTypes.TEXT_VTT,
    "sbv" to "text/x-sbv"
)
```

**Testing**:
- Download .srt subtitles
- Download .ass/.ssa subtitles
- Download .vtt subtitles
- Verify each format displays correctly

---

### Task 4: Persist Downloaded Subtitles
**Priority**: MEDIUM  
**Time Estimate**: 1-2 hours  
**Files to Modify**: `OpenSubtitlesClient.kt`, database

**What needs to be done**:
1. Track downloaded subtitles in database (or preferences)
2. Store mapping: video_path → subtitle_file
3. On video load, check if subtitle exists locally
4. Show "Already downloaded ✓" indicator in search results
5. Auto-apply previously downloaded subtitles

**Database Schema**:
```sql
CREATE TABLE downloaded_subtitles (
    id INTEGER PRIMARY KEY,
    video_path TEXT UNIQUE,
    subtitle_filename TEXT,
    subtitle_file_path TEXT,
    subtitle_language TEXT,
    downloaded_at INTEGER,
    checksum TEXT
);
```

**Testing**:
- Download a subtitle for a video
- Close app and reopen
- Video should auto-load subtitle
- Download dialog should show checkmark

---

## Phase 2 (Continued): Advanced Features

### Task 5: Subtitle Search Language Selection
**Priority**: MEDIUM  
**Time Estimate**: 1.5 hours  
**Files to Modify**: `VideoPlayer.kt`, `OpenSubtitlesClient.kt`

**What needs to be done**:
1. Add language selector in download dialog
2. Common languages: English, Spanish, French, German, Italian
3. Multi-language search option
4. Store user preference

**UI**:
```
Search Language: [English ▼]
  ✓ English
    Spanish
    French
    German
    Italian
    [Custom...]
```

---

### Task 6: Subtitle Styling Options
**Priority**: LOW  
**Time Estimate**: 2-3 hours  
**Files to Modify**: `VideoPlayer.kt`, settings

**What needs to be done**:
1. Add subtitle styling controls
2. Font size adjustment
3. Font color selection
4. Background color/opacity
5. Position adjustment
6. Store preferences

**Options**:
- Font size: Small, Medium, Large, Extra Large (or 12-48pt)
- Color: White, Yellow, Green, Cyan, Blue (or color picker)
- Background: None, Black, Semi-transparent
- Position: Bottom, Middle (or top for hearing impaired)

**Code Integration**:
```kotlin
// In ExoPlayer PlayerView setup
playerView.subtitleView?.setStyle(
    SubtitleView.CaptionStyleCompat(
        foregroundColor = subtitleColor,
        backgroundColor = backgroundColor,
        // ... other style properties
    )
)
```

---

### Task 7: Offline Subtitle Support  
**Priority**: LOW  
**Time Estimate**: 2 hours  
**Files to Modify**: UI components, file browser

**What needs to be done**:
1. Allow users to select .srt/.ass files from device
2. Apply local file as subtitle
3. File picker UI
4. Auto-detection of subtitle files in video directory

**Implementation**:
```kotlin
// File picker for manual subtitle selection
fun openSubtitleFilePicker() {
    // Show file browser
    // Filter for .srt, .ass, .vtt files
    // Apply selected file to video
}
```

---

### Task 8: Subtitle Synchronization
**Priority**: LOW  
**Time Estimate**: 3-4 hours  
**Files to Modify**: `VideoPlayer.kt`, new `SubtitleSyncDialog.kt`

**What needs to be done**:
1. Add "Sync Subtitles" button to toolbar
2. Create sync adjustment UI
3. Show offset controls: ±500ms, ±100ms, ±10ms
4. Real-time preview
5. Save sync preference

**UI**:
```
Subtitle Sync
├─ Offset: -500ms | -100ms | -10ms | +10ms | +100ms | +500ms
├─ Current: 0ms
├─ Test at: 00:45:30 (show subtitle preview)
└─ [Reset] [Save]
```

---

## Phase 2 Timeline

```
Week 1:
  ✓ Task 1: ExoPlayer Integration (2-3 hours) 
  ✓ Task 2: Settings Screen (1-2 hours)
  ✓ Task 3: MIME Type Detection (1 hour)
  └→ Subtotal: 4-6 hours

Week 2:
  ✓ Task 4: Persist Downloaded Subtitles (1-2 hours)
  ✓ Task 5: Language Selection (1.5 hours)
  ✓ Task 6: Subtitle Styling (2-3 hours)
  └→ Subtotal: 4.5-6.5 hours

Week 3:
  • Task 7: Offline Subtitle Support (2 hours) - Optional
  • Task 8: Subtitle Sync (3-4 hours) - Optional
  └→ Subtotal: 5-6 hours (optional)

Total: 13.5-18.5 hours for Phase 2 (full features)
Minimum: 8-10 hours for Tasks 1-5 (core features)
```

---

## Testing Checklist - Phase 2

### Basic Functionality
- [ ] Download subtitle and apply to video
- [ ] Verify subtitle displays correctly
- [ ] Disable subtitle and re-enable
- [ ] Switch between multiple subtitles

### Format Support
- [ ] Test .srt subtitles
- [ ] Test .ass/.ssa subtitles
- [ ] Test .vtt subtitles
- [ ] Test with Unicode characters (Chinese, Arabic, etc.)

### Persistence
- [ ] Downloaded subtitles persist across app sessions
- [ ] Auto-load previously downloaded subtitles
- [ ] Download indicator works correctly

### Settings
- [ ] API key configuration works
- [ ] API key persists across app restarts
- [ ] Invalid API key shows error

### Edge Cases
- [ ] Video with no embedded subtitles → download works
- [ ] Video with embedded subtitles → both options available
- [ ] Very large subtitle files → handle gracefully
- [ ] Network errors → show helpful messages
- [ ] Invalid subtitle format → handle error

### UI/UX
- [ ] Dialogs are properly sized for TV
- [ ] Focus navigation works with D-pad
- [ ] Buttons are easily clickable
- [ ] Status messages are clear

---

## Dependencies to Add (if needed)

Current dependencies should be sufficient. If issues arise:

```gradle
dependencies {
    // Already included in project
    // androidx.media3:media3-exoplayer
    // com.squareup.okhttp3:okhttp
    // org.json:json
    
    // May be needed
    // androidx.security:security-crypto (for API key encryption)
    // com.google.code.gson:gson (for JSON if switching from org.json)
}
```

---

## Migration Notes for Task 1

The `downloadAndApplySubtitle()` function currently logs:
```kotlin
downloadStatus = "Downloaded successfully"
// TODO: Load subtitle file into ExoPlayer
```

Replace with:
```kotlin
try {
    val subtitleUri = Uri.fromFile(outputFile)
    val mimeType = getMimeTypeForSubtitle(subtitle.filename)
    
    val subtitleConfig = MediaItem.SubtitleConfiguration.Builder(subtitleUri)
        .setMimeType(mimeType)
        .setLanguage(subtitle.language)
        .build()
    
    val currentItem = exoPlayer.currentMediaItem ?: return@Thread
    val newItem = currentItem.buildUpon()
        .setSubtitleConfigurations(listOf(subtitleConfig))
        .build()
    
    exoPlayer.setMediaItem(newItem)
    exoPlayer.prepare()
    exoPlayer.playWhenReady = true
    
    downloadStatus = "Applied: ${subtitle.filename}"
} catch (e: Exception) {
    downloadStatus = "Error loading subtitle: ${e.message}"
}
```

---

## Implementation Order Recommendation

**Must-have (Core Functionality)**:
1. Task 1: ExoPlayer Integration
2. Task 2: Settings Screen
3. Task 3: MIME Type Detection
4. Task 4: Persist Downloaded Subtitles

**Nice-to-have (Enhanced UX)**:
5. Task 5: Language Selection
6. Task 6: Subtitle Styling
7. Task 7: Offline Support (if demand)
8. Task 8: Subtitle Sync (if demand)

---

## Known Limitations & Workarounds

1. **No Automatic Sync**: Currently subtitles load as-is. If timing is off, users must manually select different subtitle.
   - *Workaround*: Implement Task 8 for manual sync adjustment

2. **Single Subtitle Only**: Can only load one subtitle track at a time.
   - *Workaround*: Allow user to merge two .srt files for bilingual subtitles (future enhancement)

3. **OpenSubtitles API Rate Limiting**: Free tier has limits.
   - *Workaround*: Cache searches locally, add cool-down between requests

4. **Limited Format Support**: Only common formats supported initially.
   - *Workaround*: Convert other formats to .srt before applying

---

## Success Criteria - Phase 2

✅ All Tasks 1-4 completed  
✅ User can download and view subtitles  
✅ API key configuration in settings  
✅ Downloaded subtitles persist  
✅ All format types display correctly  
✅ Zero crashes related to subtitles  
✅ Tested on multiple Android TV devices  
✅ User documentation updated  

---

## Next Action Steps

1. **Start with Task 1** - Get ExoPlayer integration working first
2. **Add comprehensive logging** - Help diagnose issues during testing
3. **Create test videos** - With and without embedded subtitles
4. **Beta test with users** - Gather feedback before Phase 2 completion
5. **Document API key setup** - Add to app's first-run setup or tutorials

---

**Phase 1 Completion Date**: [TODAY]  
**Phase 2 Start Target**: [Next week]  
**Phase 2 Completion Target**: [2-3 weeks]  
**Full Feature Release**: [4 weeks from start]
