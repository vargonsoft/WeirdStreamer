# WeirdStreamer Theming Guide

## Overview

Both the Android app and Web UI now support three cinema-inspired themes:
- **Default** - Original purple gradient theme
- **Cinema Blue** - Rich blue and gold theme evocative of classic movie theaters
- **Cinema Red** - Burgundy/red and gold theme inspired by velvet curtains

Themes are independently configurable in each platform.

## Android App Themes

### Theme Selection
1. Navigate to Settings (gear icon from main browser)
2. At the top of settings, you'll see the Theme selector
3. Click to expand and choose from three options:
   - Default
   - Cinema Blue  
   - Cinema Red
4. Theme changes apply immediately

### Color Schemes

#### Cinema Blue Theme
- **Primary**: Rich Gold (#FBB024)
- **Secondary**: Bright Blue (#3B82F6)
- **Background**: Deep Navy (#0F172A → #1E3A8A gradient)
- **Surface**: Royal Blue (#1E3A8A)
- **Text**: Light Silver (#E5E7EB)

#### Cinema Red Theme
- **Primary**: Rich Gold (#FBB024)
- **Secondary**: Bright Red (#DC2626)
- **Background**: Dark Burgundy (#7F1D1D → #991B1B gradient)
- **Surface**: Deep Red (#991B1B)
- **Text**: Light Silver (#E5E7EB)

### Implementation Details

**Files Modified:**
- `app/src/main/java/com/vargon/weirdstreamer/ui/theme/Color.kt` - Color definitions
- `app/src/main/java/com/vargon/weirdstreamer/ui/theme/Theme.kt` - Theme composition with AppTheme enum
- `app/src/main/java/com/vargon/weirdstreamer/MainActivity.kt` - Theme state management
- `app/src/main/java/com/vargon/weirdstreamer/ui/components/SettingsScreen.kt` - Theme selector UI
- `app/src/main/java/com/vargon/weirdstreamer/CredentialManager.kt` - Theme preference storage

**Storage:**
Theme preference stored in EncryptedSharedPreferences as `app_theme` key with values:
- `"DEFAULT"`
- `"CINEMA_BLUE"`
- `"CINEMA_RED"`

## Web UI Themes

### Theme Selection
1. Log into the web UI
2. Click Settings button
3. Go to "My Account" tab
4. Select your preferred theme from the radio buttons:
   - Default (Purple)
   - Cinema Blue (Gold & Blue)
   - Cinema Red (Gold & Red)
5. Click "Save Theme" button
6. Theme applies immediately

### Implementation Details

**Files Modified:**
- `Server/web/css/style.css` - CSS variables and theme definitions
- `Server/web/index.html` - Theme selector UI in account settings
- `Server/web/js/app.js` - Theme management functions

**CSS Variables:**
Themes use CSS custom properties on `:root[data-theme="..."]`:
- `--primary-color` - Main accent color
- `--primary-dark` - Darker variant for hover states
- `--secondary-color` - Secondary accent
- `--background-gradient` - Page background
- `--text-color` - Main text color
- `--surface-color` - Card/surface backgrounds
- `--surface-hover` - Hover state for interactive surfaces

**Storage:**
Theme stored in `localStorage` as `app-theme` key.

**JavaScript Functions:**
- `applyTheme()` - Applies saved theme on load
- `handleSaveTheme()` - Saves selected theme to localStorage and applies it

## Design Philosophy

### Cinema Blue Theme
Inspired by classic movie theater architecture with rich blue velvet seating and golden Art Deco accents. The deep navy backgrounds evoke a darkened theater, while gold highlights bring warmth and elegance.

### Cinema Red Theme
Evokes the classic red velvet curtains of traditional theaters and opera houses. The burgundy and crimson palette creates a luxurious, dramatic atmosphere perfect for media viewing.

### Color Psychology
- **Gold/Yellow** - Associated with glamour, prestige, and the golden age of cinema
- **Blue** - Calming, professional, classic theater aesthetic
- **Red** - Dramatic, passionate, traditional theatrical elegance

## Customization

### Adding New Themes (Android)

1. **Define colors** in `Color.kt`:
```kotlin
val MyThemePrimary = Color(0xFF...)
val MyThemeSecondary = Color(0xFF...)
// etc.
```

2. **Add enum value** in `Theme.kt`:
```kotlin
enum class AppTheme {
    DEFAULT, CINEMA_BLUE, CINEMA_RED, MY_NEW_THEME
}
```

3. **Add color scheme** in `WeirdStreamerTheme`:
```kotlin
AppTheme.MY_NEW_THEME -> {
    darkColorScheme(
        primary = MyThemePrimary,
        // ... other colors
    )
}
```

4. **Update SettingsScreen.kt** to add UI option

### Adding New Themes (Web)

1. **Add CSS variables** in `style.css`:
```css
:root[data-theme="my-theme"] {
    --primary-color: #...;
    --background-gradient: linear-gradient(...);
    /* etc. */
}
```

2. **Add radio option** in `index.html` account tab:
```html
<label>
    <input type="radio" name="theme" value="my-theme" id="theme-my-theme"> My New Theme
</label>
```

## Technical Notes

- Android themes persist across app restarts
- Web themes persist per-browser (localStorage)
- Theme selection is independent between platforms
- All Material 3 color roles are properly mapped for Android
- Web CSS variables cascade throughout all components
- Both implementations use lazy loading - themes apply before UI renders

## Testing

### Android
1. Install app on Android TV or device
2. Navigate to Settings
3. Toggle between themes
4. Verify colors update across all screens (browser, video player, image viewer, PDF viewer)
5. Restart app and verify theme persists

### Web
1. Open web UI in browser
2. Log in
3. Go to Settings → My Account
4. Try each theme
5. Verify colors update across media grid, player modal, settings
6. Refresh page and verify theme persists
