# Subtitle Feature - Quick Reference

## Using Built-in Subtitles

### Opening the Subtitle Menu
1. While playing a video, press **Down** on your remote
2. A toolbar appears at the bottom of the screen
3. Press **Right** arrow key 4 times to highlight the **Subtitles** button (last button)
4. Press **Center** or **Enter** to open the subtitle dialog

### Selecting a Subtitle
1. In the subtitle dialog, you'll see a list of available subtitles from the video
2. Each option shows the language name
3. Press **Up/Down** arrows to navigate the list
4. Press **Center** to select a subtitle
5. The video will immediately show the selected subtitles

### Disabling Subtitles  
1. In the subtitle dialog, select the **"None"** option at the top
2. Subtitles will be removed from the video

---

## Downloading Subtitles from OpenSubtitles

### Prerequisites
⚠️ **Important**: You must first configure an OpenSubtitles API key:
1. Go to SettingsScreen in the app
2. Look for "OpenSubtitles API Configuration"  
3. Enter your API key (get one free at https://www.opensubtitles.com/api)

### Downloading Steps
1. Open the subtitle dialog (see above)
2. At the bottom, click **"OpenSubtitles.org / .com"** button
3. The app will search for subtitles matching your video filename
4. A loading spinner appears while searching
5. Once found, up to 10 results will display showing:
   - Filename
   - Language
   - Rating (0-10)
   - Download count
6. Click on a subtitle to download and apply it
7. A status message shows the download progress
8. Once complete, the subtitle appears on your video

---

## Toolbar Button Layout

```
┌─────────────────────────────────────────────────────┐
│  [Zoom] [Zoom+] [Zoom-] [Aspect Ratio] [Subtitles] │
└─────────────────────────────────────────────────────┘
     0       1        2          3             4
```

Use **Left/Right** arrows to navigate between buttons.

---

## Subtitle Dialog Structure

```
┌─────────────────────────────────┐
│         Subtitles               │
├─────────────────────────────────┤
│ Available Subtitles             │
│  ✓ None                         │
│    English                      │
│    Spanish                      │
│    French                       │
├─────────────────────────────────┤
│ Download Subtitles              │
│ [Download from OpenSubtitles..]│
├─────────────────────────────────┤
│                          [Close]│
└─────────────────────────────────┘
```

---

## Download Dialog Structure

```
┌─────────────────────────────────┐
│    Download Subtitles           │
├─────────────────────────────────┤
│ Searching...  [Loading Spinner] │
├─────────────────────────────────┤
│ Found 5 subtitle(s)             │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ │
│ movie.srt                       │
│ English | Rating: 8.5 | 1250 ↓ │
│ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ │
│ movie-alt.srt                   │
│ English | Rating: 7.2 | 456 ↓  │
│ [scroll for more]               │
├─────────────────────────────────┤
│                          [Close]│
└─────────────────────────────────┘
```

---

## Keyboard Controls

| Key | Action |
|-----|--------|
| **Down** | Show zoom toolbar |
| **Left/Right** | Navigate toolbar buttons (0-4) |
| **Center/Enter** | Activate selected button |
| **Up/Down** (in dialog) | Navigate subtitle list |
| **Left/Right** (in dialog) | Scroll left/right |
| **Center/Enter** (in dialog) | Select subtitle |
| **Back** | Close dialog |

---

## Status Messages

| Message | Meaning |
|---------|---------|
| "Searching..." | App is searching OpenSubtitles API |
| "Found X subtitle(s)" | Search completed successfully |
| "No subtitles found" | No matching subtitles on OpenSubtitles |
| "Downloading..." | File is downloading |
| "Downloaded successfully" | Subtitle ready to use |
| "Download failed" | Error downloading subtitle |
| "Error: [message]" | API error occurred |
| "API key not configured..." | Need to set up OpenSubtitles API key |

---

## Troubleshooting

### "API key not configured"
- You haven't set up your OpenSubtitles API key yet
- Go to Settings and add your API key
- Get a free key at https://www.opensubtitles.com/api

### "No subtitles found"
- OpenSubtitles doesn't have subtitles for your video
- Try searching with a different video filename
- Or upload your subtitle file manually

### Subtitles not showing
- Make sure you have **selected** a subtitle from the dialog
- Check if the video format is supported
- Try disabling and re-enabling subtitles

### App crashes when downloading
- Check your internet connection
- Ensure API key is valid
- Try restarting the app

---

## Tips & Tricks

✅ **Tip 1**: The app automatically detects embedded subtitles in the video file. Start there before downloading.

✅ **Tip 2**: Downloaded subtitles are cached for faster loading next time.

✅ **Tip 3**: Subtitle filenames are clickable - they link to subtitles with better ratings/downloads.

✅ **Tip 4**: Search works better with cleaner video filenames (e.g., "MovieName.mkv" instead of random names).

✅ **Tip 5**: Multiple languages - you can search multiple times with different subtitle languages.

---

## Getting an OpenSubtitles API Key

1. Visit https://www.opensubtitles.com/api
2. Sign up for a free account (or log in)
3. Create an API key
4. Copy the key
5. Go to WeirdStreamer Settings
6. Paste your API key in the configuration field
7. Done! Now you can download subtitles

---

**Note**: Subtitle support is designed for Android TV with D-pad navigation. 
For best experience, use a remote control with directional keys.
