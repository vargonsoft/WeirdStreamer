// Application State
console.log('app.js loaded');
// Views of a movie or tv library (the URL's view=), as against folders.
const LIBRARY_VIEWS = ['library', 'title', 'collections', 'collection', 'tmdbcollections', 'tmdbcollection'];

const state = {
    currentType: 'all',
    currentPath: '',
    mediaItems: [],
    directories: [],
    currentConfig: null,
    availableGroups: [],
    libraryType: '',  // "movie" or "tv" when browsing a typed library
    view: 'folders',  // "folders", "library" (titles) or "title" (one title's page)
    libraryId: '',
    libraryName: '',
    titleRef: null    // { kind, id } on a title page
};

// DOM Elements
console.log('Initializing DOM elements');
const loginScreen = document.getElementById('login-screen');
const mainScreen = document.getElementById('main-screen');
const settingsScreen = document.getElementById('settings-screen');
const loginForm = document.getElementById('login-form');
const loginError = document.getElementById('login-error');
const logoutBtn = document.getElementById('logout-btn');
const scanBtn = document.getElementById('scan-btn');
const regenerateThumbnailsBtn = document.getElementById('regenerate-thumbnails-btn');
const settingsBtn = document.getElementById('settings-btn');
const closeSettingsBtn = document.getElementById('close-settings-btn');
const tabs = document.querySelectorAll('.tab');
const mediaGrid = document.getElementById('media-grid');
const directoryList = document.getElementById('directory-list');
const breadcrumb = document.getElementById('breadcrumb');
const playerModal = document.getElementById('player-modal');
const imageModal = document.getElementById('image-modal');
const videoPlayer = document.getElementById('video-player');
const imageViewer = document.getElementById('image-viewer');
const livePhotoVideo = document.getElementById('live-photo-video');
const livePhotoButton = document.getElementById('live-photo-button');
const playerTitle = document.getElementById('player-title');
const imageTitle = document.getElementById('image-title');

// Natural sort helper function
function naturalCompare(s1, s2) {
    return s1.localeCompare(s2, undefined, { numeric: true, sensitivity: 'base' });
}

// Initialize
// The web UI has no version number: the server serves these files from disk, so a change to any of
// them is a new version. The page compares them with how they were when it loaded, every few
// minutes and on coming back to the tab, and offers a reload.
const WEB_FILES = ['./', 'js/api.js', 'js/app.js', 'js/pgs.js', 'css/style.css'];
const WEB_CHECK_MS = 5 * 60 * 1000;
let webFingerprint = null;

async function checkForNewWeb() {
    let fingerprint;
    try {
        const heads = await Promise.all(WEB_FILES.map(file => fetch(file, { method: 'HEAD', cache: 'no-cache' })));
        if (heads.some(head => !head.ok)) return; // the server restarting
        fingerprint = heads.map(head => `${head.headers.get('last-modified')}/${head.headers.get('content-length')}`).join(' ');
    } catch (error) {
        return;
    }
    if (webFingerprint === null) webFingerprint = fingerprint;
    else if (fingerprint !== webFingerprint) document.getElementById('update-banner').hidden = false;
}

function watchForNewWeb() {
    const banner = document.getElementById('update-banner');
    banner.innerHTML = `${iconSvg('refresh')}<span>A new version is available. Click to refresh.</span>`;
    banner.onclick = () => location.reload();
    checkForNewWeb();
    setInterval(checkForNewWeb, WEB_CHECK_MS);
    document.addEventListener('visibilitychange', () => {
        if (!document.hidden) checkForNewWeb();
    });
}

function init() {
    watchForNewWeb();
    // Apply saved theme
    applyTheme();
    setupSubtitleStyle();
    setupPlaybackSettings();
    
    if (api.isAuthenticated()) {
        showMainScreen();
    } else {
        showLoginScreen();
    }

    setupEventListeners();
}

function setupEventListeners() {
    // Login
    loginForm.addEventListener('submit', handleLogin);
    
    // Logout
    logoutBtn.addEventListener('click', handleLogout);
    
    // Scan
    scanBtn.addEventListener('click', handleScan);
    
    // Regenerate Thumbnails
    if (regenerateThumbnailsBtn) {
        regenerateThumbnailsBtn.addEventListener('click', handleRegenerateThumbnails);
    }

    // Settings
    if (settingsBtn) {
        settingsBtn.addEventListener('click', () => showSettings());
    }
    if (closeSettingsBtn) {
        closeSettingsBtn.addEventListener('click', () => closeScreen('settings'));
    }
    
    // Settings tabs
    document.querySelectorAll('.settings-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            const tabName = tab.dataset.tab;
            document.querySelectorAll('.settings-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.settings-tab-content').forEach(c => c.classList.remove('active'));
            tab.classList.add('active');
            document.getElementById(`${tabName}-tab`).classList.add('active');
            if (tabName === 'jobs') startJobsView();
            else stopJobsView();
            if (tabName === 'streams') startStreamsView();
            else stopStreamsView();
            
            // Load users when switching to users tab
            if (tabName === 'users') {
                loadUsers();
            }
        });
    });
    
    // User management
    document.getElementById('create-user-btn')?.addEventListener('click', handleCreateUser);
    document.getElementById('change-password-btn')?.addEventListener('click', handleChangePassword);
    document.getElementById('tv-pairing-btn')?.addEventListener('click', handleTvPairing);
    document.getElementById('tv-pairing-code')?.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') handleTvPairing();
    });
    
    // Theme management
    renderThemePicker();
    
    // Browser back/forward buttons
    // Each history entry remembers how far down its page was, for Back and Forward. The page
    // restores it itself, after re-rendering, since content arrives asynchronously.
    history.scrollRestoration = 'manual';
    window.addEventListener('scroll', () => {
        clearTimeout(scrollSaveTimer);
        scrollSaveTimer = setTimeout(rememberScroll, 150);
    }, { passive: true });

    window.addEventListener('popstate', (e) => {
        // Settings and Streams: Back closes them, onto the view they were opened from (unchanged
        // underneath, so it isn't loaded again); Forward opens them again.
        const screen = e.state && e.state.screen;
        const open = openScreen();
        if (open && open !== screen) {
            SCREENS[open].hide();
            if (!screen) {
                if (typeof e.state?.scrollY === 'number') requestAnimationFrame(() => window.scrollTo(0, e.state.scrollY));
                return;
            }
        }
        if (screen && SCREENS[screen]) {
            if (open !== screen && api.isAuthenticated()) SCREENS[screen].show();
            return;
        }
        if (e.state && api.isAuthenticated()) {
            pendingScroll = typeof e.state.scrollY === 'number' ? e.state.scrollY : null;
            if (e.state.view === 'library') {
                showLibrary(e.state.lib, false);
                return;
            }
            if (e.state.view === 'title') {
                showTitle(e.state.lib, e.state.kind, e.state.id, false);
                return;
            }
            if (e.state.view === 'collections') {
                showCollections(e.state.lib, false);
                return;
            }
            if (e.state.view === 'collection') {
                showCollection(e.state.lib, e.state.id, false);
                return;
            }
            if (e.state.view === 'tmdbcollections') {
                showTmdbCollections(e.state.lib, false);
                return;
            }
            if (e.state.view === 'tmdbcollection') {
                showTmdbCollection(e.state.lib, e.state.id, false);
                return;
            }
            state.currentPath = e.state.path || '';
            state.currentType = e.state.type || 'all';
            // Update active tab
            tabs.forEach(t => {
                t.classList.toggle('active', t.dataset.type === state.currentType);
            });
            loadMedia(false); // Don't push history
        }
    });
    
    // Cleanup SSE connection on page unload
    window.addEventListener('beforeunload', () => {
        api.disconnectThumbnailUpdates();
    });
    
    // Tabs
    tabs.forEach(tab => {
        tab.addEventListener('click', () => {
            tabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');
            state.currentType = tab.dataset.type;
            loadMedia();
        });
    });
    
    setupMatchDialog();
    setupFolderPicker();
    setupPlayerControls();
    document.querySelector('.app-download-github')?.insertAdjacentHTML('afterbegin', iconSvg('android'));
    decorateIconButtons();
    setupSecretToggles();

    // Modal close buttons
    document.querySelectorAll('.close').forEach(closeBtn => {
        closeBtn.addEventListener('click', () => {
            finishPlaybackReports();
            playerModal.classList.remove('active');
            imageModal.classList.remove('active');
            // Properly stop video loading
            videoPlayer.pause();
            videoPlayer.src = '';
            videoPlayer.load(); // This aborts the current HTTP request
            videoPlayer.onloadedmetadata = null;
            videoPlayer.onerror = null;
            videoPlayer.ontimeupdate = null;
            videoPlayer.onseeking = null;
            resetPlayer();
            videoPlayer.innerHTML = '';
            imageViewer.onload = null;
            imageViewer.onerror = null;
            imageViewer.src = '';
            stopLivePhoto();
        });
    });
    
    // Close modal on outside click
    window.addEventListener('click', (e) => {
        if (e.target === playerModal) {
            finishPlaybackReports();
            playerModal.classList.remove('active');
            videoPlayer.pause();
            videoPlayer.src = '';
            videoPlayer.load(); // This aborts the current HTTP request
            videoPlayer.onloadedmetadata = null;
            videoPlayer.onerror = null;
            videoPlayer.ontimeupdate = null;
            videoPlayer.onseeking = null;
            resetPlayer();
            videoPlayer.innerHTML = '';
        }
        if (e.target === imageModal) {
            imageModal.classList.remove('active');
            imageViewer.onload = null;
            imageViewer.onerror = null;
            imageViewer.src = '';
            stopLivePhoto();
        }
    });
}

function showLoginScreen() {
    loginScreen.classList.add('active');
    mainScreen.classList.remove('active');
}

function showMainScreen() {
    loginScreen.classList.remove('active');
    mainScreen.classList.add('active');

    const user = JSON.parse(localStorage.getItem('user') || '{}');

    // Show settings button for all users
    if (settingsBtn) {
        settingsBtn.style.display = '';
    }

    // Show scan button only for admins
    if (scanBtn) {
        scanBtn.style.display = user.is_admin ? '' : 'none';
    }
    
    // Update regenerate thumbnails button visibility
    updateRegenerateThumbnailsButton();
    updateHeaderServer();
    setupAppDownloads(); // signed in only: the server refuses the app to anyone else

    // Load config for friendly path names for all users
    if (!state.currentConfig) {
        api.getConfig().then(config => {
            state.currentConfig = config;
            updateAvailableGroups();
            // A new server (setup.sh makes none): its admin starts in Settings, at the libraries
            if (user.is_admin && !(config.media?.paths || []).length) openLibrarySettings();
        }).catch(err => {
            console.log('Could not load config:', err);
        });
    }
    
    // Connect to thumbnail updates via SSE
    api.connectThumbnailUpdates();
    api.onThumbnailUpdate((data) => {
        console.log('[SSE] Thumbnail update received:', data);
        // A folder's icon picture (Regenerate Thumbnails one level up)
        const folder = [...document.querySelectorAll('.directory-item')].find(el => el.dataset.path === data.path);
        if (folder) {
            const dir = state.directories.find(d => d.path === data.path);
            if (dir) dir.thumbnail_url = data.thumbnailURL;
            const thumbnail = folder.querySelector('.folder-thumbnail');
            thumbnail.querySelector('.folder-preview, .placeholder-icon')?.remove();
            thumbnail.prepend(folderPreview(data.thumbnailURL, dir ? dir.name : ''));
            return;
        }
        // Find and update the thumbnail for this item
        const item = state.mediaItems.find(i => i.path === data.path);
        if (item) {
            console.log('[SSE] Found matching item:', item.name);
            item.thumbnail_url = data.thumbnailURL;
            
            // Find the DOM element and update it directly to avoid full re-render
            const mediaItems = document.querySelectorAll('.media-item:not(.directory-item)');
            mediaItems.forEach(element => {
                const nameDiv = element.querySelector('.media-name');
                if (nameDiv && nameDiv.textContent === item.name) {
                    const thumbnailDiv = element.querySelector('.media-thumbnail');
                    if (thumbnailDiv) {
                        // Keep the Fix button, which lives in the thumbnail.
                        const fix = thumbnailDiv.querySelector('.fix-match-btn');
                        const replace = (content) => {
                            thumbnailDiv.innerHTML = '';
                            if (typeof content === 'string') thumbnailDiv.innerHTML = content;
                            else thumbnailDiv.appendChild(content);
                            if (fix) thumbnailDiv.appendChild(fix);
                        };
                        thumbnailDiv.classList.remove('no-thumb');
                        const img = document.createElement('img');
                        // Add cache buster to force reload
                        img.src = api.getThumbnailUrl(data.thumbnailURL) + (data.thumbnailURL.includes('?') ? '&' : '?') + 't=' + Date.now();
                        img.alt = item.name;
                        img.onerror = () => {
                            thumbnailDiv.classList.add('no-thumb');
                            replace(getMediaIcon(item.type));
                        };
                        replace(img);
                        console.log('[SSE] Updated thumbnail for:', item.name);
                    }
                }
            });
        } else {
            console.log('[SSE] No matching item found for path:', data.path);
        }
    });
    
    // Metadata for something on screen changed (a lookup or manual fix finished): refresh,
    // batching bursts of events into one reload.
    let metadataReload = null;
    api.onMetadataUpdate(() => {
        if (!state.libraryType) return;
        clearTimeout(metadataReload);
        metadataReload = setTimeout(() => {
            refreshCurrentView();
            if (matchState.target) refreshMatchInfo();
        }, 1500);
    });

    // Initialize from URL parameters if present
    const url = new URL(window.location);
    const urlPath = url.searchParams.get('path');
    const urlType = url.searchParams.get('type');
    
    const urlView = url.searchParams.get('view');
    if (history.state && typeof history.state.scrollY === 'number') {
        pendingScroll = history.state.scrollY; // a reload: back where it was
    }
    if (history.state && history.state.screen) {
        // A reload opens on the view, not Settings or Streams.
        const { screen, ...view } = history.state;
        history.replaceState(view, '');
    }
    if (!history.state) {
        // So Back to this first page has a state to restore it from.
        const initial = LIBRARY_VIEWS.includes(urlView)
            ? Object.fromEntries(['view', 'lib', 'kind', 'id'].filter(k => url.searchParams.has(k)).map(k => [k, url.searchParams.get(k)]))
            : { path: urlPath || '', type: urlType || 'all' };
        if (initial.id !== undefined) initial.id = Number(initial.id);
        history.replaceState(initial, '');
    }
    if (LIBRARY_VIEWS.includes(urlView)) {
        const lib = url.searchParams.get('lib');
        if (urlView === 'title') {
            showTitle(lib, url.searchParams.get('kind'), Number(url.searchParams.get('id')), false);
        } else if (urlView === 'collections') {
            showCollections(lib, false);
        } else if (urlView === 'collection') {
            showCollection(lib, Number(url.searchParams.get('id')), false);
        } else if (urlView === 'tmdbcollections') {
            showTmdbCollections(lib, false);
        } else if (urlView === 'tmdbcollection') {
            showTmdbCollection(lib, Number(url.searchParams.get('id')), false);
        } else {
            showLibrary(lib, false);
        }
        return;
    }
    if (urlPath !== null) {
        state.currentPath = urlPath;
    }
    if (urlType) {
        state.currentType = urlType;
        tabs.forEach(t => {
            t.classList.toggle('active', t.dataset.type === urlType);
        });
    }
    
    loadMedia(false); // Don't push initial state
}

async function handleLogin(e) {
    e.preventDefault();
    loginError.textContent = '';

    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;

    try {
        console.log('Attempting login for user:', username);
        const response = await api.login(username, password);
        showMainScreen();
    } catch (error) {
        console.error('Login failed:', error);
        if (error.retryAfter) {
            startLoginCooldown(error.retryAfter);
            return;
        }
        loginError.textContent = error.message || 'Login failed. Please try again.';
    }
}

// After too many wrong passwords the server refuses logins for a while. The button stays off and
// the error shows the countdown until then.
let loginCooldownTimer = null;

function startLoginCooldown(seconds) {
    const button = loginForm.querySelector('button[type="submit"]');
    const until = Date.now() + seconds * 1000;
    clearInterval(loginCooldownTimer);

    const tick = () => {
        const left = Math.ceil((until - Date.now()) / 1000);
        if (left > 0) {
            const minutes = Math.floor(left / 60);
            const shown = minutes ? `${minutes}m ${left % 60}s` : `${left}s`;
            loginError.textContent = `Too many failed attempts. Try again in ${shown}.`;
            button.disabled = true;
            return;
        }
        clearInterval(loginCooldownTimer);
        loginCooldownTimer = null;
        loginError.textContent = '';
        button.disabled = false;
    };
    tick();
    if (Date.now() < until) loginCooldownTimer = setInterval(tick, 1000);
}

// After the wordmark: the server's name (else its address) and who's signed in, as on Android.
async function updateHeaderServer() {
    const el = document.getElementById('header-server');
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    let name = '';
    try {
        const info = await (await fetch('/api/info', { cache: 'no-cache' })).json();
        name = info.name || '';
    } catch (e) { /* an older server: its address */ }
    el.textContent = [name || location.host, user.username].filter(Boolean).join(' · ');
}

function handleLogout() {
    api.disconnectThumbnailUpdates();
    api.logout();
    showLoginScreen();
}

async function handleScan() {
    try {
        scanBtn.disabled = true;
        setButtonLabel(scanBtn, 'Scanning...');
        await api.scanMedia();
        alert('Media scan started. This may take a while.');
        setTimeout(loadMedia, 2000);
    } catch (error) {
        alert('Failed to start scan: ' + error.message);
    } finally {
        scanBtn.disabled = false;
        setButtonLabel(scanBtn, 'Scan Media');
    }
}

async function handleRegenerateThumbnails() {
    if (!state.currentPath) {
        alert('Please navigate to a folder first');
        return;
    }

    if (!confirm(`Regenerate all thumbnails for this folder?\n\nPath: ${state.currentPath}\n\nThis will delete and regenerate all thumbnails. Continue?`)) {
        return;
    }

    try {
        regenerateThumbnailsBtn.disabled = true;
        setButtonLabel(regenerateThumbnailsBtn, 'Regenerating...');
        await api.regenerateThumbnails(state.currentPath);
    } catch (error) {
        alert('Failed to regenerate thumbnails: ' + error.message);
    } finally {
        regenerateThumbnailsBtn.disabled = false;
        setButtonLabel(regenerateThumbnailsBtn, 'Regenerate Thumbnails');
    }
}

function updateRegenerateThumbnailsButton() {
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    if (regenerateThumbnailsBtn) {
        // Show button only for admins when in a folder (not root)
        if (user.is_admin && state.currentPath) {
            regenerateThumbnailsBtn.style.display = '';
        } else {
            regenerateThumbnailsBtn.style.display = 'none';
        }
    }
}

async function loadMedia(pushHistory = true) {
    setView('folders');
    try {
        const data = await api.browseDirectory(state.currentPath);
        state.directories = data.dirs || [];
        state.mediaItems = data.items || [];
        state.libraryType = data.library_type || '';
        
        // Apply natural sort to directories and media items
        state.directories.sort((a, b) => naturalCompare(a.name, b.name));
        state.mediaItems.sort((a, b) => naturalCompare(a.name, b.name));

        // Filter by type if needed
        if (state.currentType !== 'all') {
            state.mediaItems = state.mediaItems.filter(item => item.type === state.currentType);
        }
        
        // Update browser history
        if (pushHistory) {
            rememberScroll();
            const url = new URL(window.location);
            ['view', 'lib', 'kind', 'id'].forEach(k => url.searchParams.delete(k));
            url.searchParams.set('path', state.currentPath);
            url.searchParams.set('type', state.currentType);
            window.history.pushState(
                { path: state.currentPath, type: state.currentType },
                '',
                url
            );
        }
        
        state.libraryId = state.currentPath.split('/').filter(p => p)[0] || '';
        updateBreadcrumb();
        updateRegenerateThumbnailsButton();
        renderMedia();
        if (pushHistory) window.scrollTo(0, 0);
        restorePendingScroll();
    } catch (error) {
        console.error('Failed to load media:', error);
    }
}

function updateBreadcrumb() {
    breadcrumb.innerHTML = '';
    if (state.libraryType && state.libraryId) {
        breadcrumb.appendChild(createViewToggle());
    }
    
    const home = document.createElement('a');
    home.href = '#';
    home.textContent = 'Home';
    home.onclick = (e) => {
        e.preventDefault();
        state.currentPath = '';
        loadMedia();
    };
    breadcrumb.appendChild(home);
    
    if (state.currentPath) {
        const parts = state.currentPath.split('/').filter(p => p);
        let currentPath = '';
        parts.forEach((part, index) => {
            breadcrumb.appendChild(document.createTextNode(' / '));
            const link = document.createElement('a');
            link.href = '#';
            
            // For the first part (root directory), use friendly name if available
            let displayName = part;
            if (index === 0 && state.currentConfig && state.currentConfig.media && state.currentConfig.media.paths) {
                const mediaPath = state.currentConfig.media.paths.find(mp => mp.id === part);
                if (mediaPath) {
                    displayName = mediaPath.name;
                }
            }
            
            link.textContent = displayName;
            currentPath = currentPath ? currentPath + '/' + part : part; // no leading slash, like browse paths
            const pathCopy = currentPath;
            link.onclick = (e) => {
                e.preventDefault();
                state.currentPath = pathCopy;
                loadMedia();
            };
            breadcrumb.appendChild(link);
        });
    }
}

function renderMedia() {
    directoryList.innerHTML = '';
    mediaGrid.innerHTML = '';
    
    // Render directories and media items in the same grid
    state.directories.forEach(dir => {
        const element = createDirectoryElement(dir);
        mediaGrid.appendChild(element);
    });
    
    state.mediaItems.forEach(item => {
        // Videos have filmstrip edges here, telling them from photos in the same folder.
        const element = createMediaElement(item, { filmstrip: true });
        mediaGrid.appendChild(element);
    });
    // Folders' letters, then files' (the names as shown).
    setAzIndex([...mediaGrid.children].map(el => ({
        el, label: azLabel(el.querySelector('.media-name')?.textContent || ''),
        run: el.classList.contains('directory-item') ? 0 : 1,
    })));

    // The server's front page leads with what's in progress in every library, and a library's own
    // page (shown as folders here; as posters in showLibrary) with what's in progress in it.
    const atLibrary = state.currentPath.split('/').filter(p => p).length === 1;
    if (!state.currentPath || atLibrary) {
        const path = state.currentPath;
        // The Guide row comes last, just above the folders.
        Promise.all([createContinueRow(atLibrary ? state.libraryId : ''),
            atLibrary ? createNewAdditionsRow(state.libraryId) : createNewAdditionsRows(),
            createGuideRow(atLibrary ? state.libraryId : '')]).then(rows => {
            const shown = rows.flat().filter(Boolean);
            if (shown.length && state.view === 'folders' && state.currentPath === path) {
                directoryList.innerHTML = '';
                directoryList.append(...shown);
            }
        });
    }
}

function createDirectoryElement(dir) {
    const div = document.createElement('div');
    div.className = 'media-item directory-item';
    div.dataset.path = dir.path; // for live updates of its icon's picture
    div.onclick = () => {
        const mode = !state.currentPath && dir.library_type ? getLibraryMode(dir.path) : 'folders';
        const moviesOnly = mode === 'collections' || mode === 'tmdb';
        if (mode === 'library' || (moviesOnly && dir.library_type !== 'movie')) {
            showLibrary(dir.path);
            return;
        }
        if (mode === 'collections') {
            showCollections(dir.path);
            return;
        }
        if (mode === 'tmdb') {
            showTmdbCollections(dir.path);
            return;
        }
        state.currentPath = dir.path;
        loadMedia();
    };
    
    const thumbnail = document.createElement('div');
    thumbnail.className = 'media-thumbnail folder-thumbnail';
    // Folder view: every folder is a folder icon with the server's picture in it (posters are the
    // Library view's).
    if (dir.thumbnail_url) {
        thumbnail.appendChild(folderPreview(dir.thumbnail_url, dir.name));
    } else {
        thumbnail.innerHTML = placeholderIcon('folder');
    }
    if (dir.fixable && canFixMatches()) {
        thumbnail.appendChild(createFixMatchButton({ dir: dir.path }, dir.name));
    }
    
    const name = document.createElement('div');
    name.className = 'media-name';
    
    // Use friendly name for root paths if configured
    let displayName = dir.name;
    if (!state.currentPath && state.currentConfig && state.currentConfig.media && state.currentConfig.media.paths) {
        // Match only by configured ID (server enforces ID-only API)
        const mediaPath = state.currentConfig.media.paths.find(mp => mp.id === dir.path);
        if (mediaPath) {
            displayName = mediaPath.name;
        }
    }
    
    name.textContent = displayName;
    
    div.appendChild(thumbnail);
    div.appendChild(name);
    const match = createMatchLabel(dir.metadata);
    if (match) div.appendChild(match);
    return div;
}

// [cornerButtons]: Fix and the watched/restart buttons over the thumbnail (a title page's files
// have them in a row underneath instead).
// A folder icon with a photo or video from inside it (the server's pick) in its front.
function folderPreview(url, name) {
    const box = document.createElement('div');
    box.className = 'folder-preview';
    box.innerHTML = iconSvg('folder').replace('<svg ', '<svg class="folder-preview-shape" ');
    const img = document.createElement('img');
    img.className = 'folder-preview-image';
    img.src = api.getThumbnailUrl(url);
    img.alt = name;
    img.onerror = () => box.outerHTML = placeholderIcon('folder');
    box.appendChild(img);
    return box;
}

function createMediaElement(item, { cornerButtons = true, filmstrip = false } = {}) {
    const div = document.createElement('div');
    div.className = 'media-item';
    div.onclick = () => handleMediaClick(item);
    
    const thumbnail = document.createElement('div');
    thumbnail.className = 'media-thumbnail';
    if (filmstrip && item.type === 'video') thumbnail.classList.add('filmstrip');
    
    if (item.thumbnail_url) {
        const img = document.createElement('img');
        img.src = api.getThumbnailUrl(item.thumbnail_url);
        img.alt = item.name;
        img.onerror = () => {
            // If thumbnail fails to load, show icon
            thumbnail.classList.add('no-thumb');
            thumbnail.innerHTML = getMediaIcon(item.type);
        };
        thumbnail.appendChild(img);
    } else {
        thumbnail.classList.add('no-thumb');
        thumbnail.innerHTML = getMediaIcon(item.type);
    }
    
    const info = document.createElement('div');
    info.className = 'media-info';
    
    const name = document.createElement('div');
    name.className = 'media-name';
    name.textContent = item.name;
    name.title = item.name;
    
    const size = document.createElement('div');
    size.className = 'media-size';
    size.textContent = formatSize(item.size);
    if (item.duration) {
        size.textContent += ` • ${formatDuration(item.duration)}`;
    }
    
    info.appendChild(name);
    // A video's format ("4K · HEVC · Dolby Vision · TrueHD Atmos 7.1"), between its name and its size and length
    if (item.format) info.appendChild(formatLineEl(item.format, 'media-format'));
    info.appendChild(size);
    const match = createMatchLabel(item.metadata);
    if (match) info.appendChild(match);
    if (item.type === 'video' && canFixMatches() && cornerButtons) {
        thumbnail.appendChild(createFixMatchButton({ path: item.path }, item.name));
    }
    if (item.type === 'video') {
        addProgressOverlay(thumbnail, item.playback, item.duration);
        if (cornerButtons) thumbnail.appendChild(createCardActions(item));
        const left = timeLeftText(item);
        if (left) size.textContent += ` • ${left}`;
    }
    div.appendChild(thumbnail);
    div.appendChild(info);
    
    return div;
}

function handleMediaClick(item) {
    if (item.type === 'video') {
        // A partly watched video asks first: Resume, or Play from start.
        if (resumePosition(item)) showResumeChoice(item);
        else playVideo(item);
    } else if (item.type === 'image') {
        viewImage(item);
    } else if (item.type === 'document') {
        window.open(api.getStreamUrl(item.path), '_blank');
    }
}

// ---------------------------------------------------------------------------------------------
// The video player: its own controls for every video (the browser's are hidden). A file plays as
// it is where the browser can play it, seeking natively. Otherwise the server transcodes it (all
// of it, or only its audio), and a transcoded stream can't be seeked past what has arrived: a
// seek there restarts it at the new place, as the TV and phone apps do. `offset` is where in the
// file the stream's time 0 is.

// Audio codecs (ffprobe's names) and the types to ask the browser about. Anything else (DTS,
// TrueHD, PCM…) is taken as unplayable. Not MP3: a browser that plays MP3 files (audio/mpeg) can
// stay silent with MP3 inside a video, MKV or MP4 (Firefox), so the server converts it to AAC.
const AUDIO_TYPES = {
    aac: 'audio/mp4; codecs="mp4a.40.2"',
    opus: 'audio/webm; codecs="opus"',
    vorbis: 'audio/webm; codecs="vorbis"',
    flac: 'audio/flac',
    alac: 'audio/mp4; codecs="alac"',
    ac3: 'audio/mp4; codecs="ac-3"',
    eac3: 'audio/mp4; codecs="ec-3"',
};

function browserPlaysAudio(track) {
    const type = AUDIO_TYPES[track.codec];
    return !!type && videoPlayer.canPlayType(type) !== '';
}

// The audio track a browser plays from a file on its own: the default one, else the first.
function defaultAudioTrack(codecs) {
    const tracks = codecs.audio || [];
    return tracks.find(t => t.default) || tracks[0] || null;
}

// The transcode modes, as on Android (transcodeModes in UiUtils.kt). Original plays the file as it
// is, Compatibility (Auto) converts only what the browser can't play (just the audio when the video
// is browser-friendly H.264), and the others transcode all of it. The player picks Compatibility
// (Auto) by itself when the original wouldn't play right (audio it can't decode, another audio
// track, picture subtitles, a playback error).
const TRANSCODE_MODES = [
    { id: 'original', name: 'Original' },
    { id: 'auto', name: 'Compatibility (Auto)' },
    { id: 'safe', name: 'Compatibility (Safe)' }, // keeps nothing: H.264 and stereo AAC
    { id: 'q80', name: 'Quality −20%', bitrate: '80%' },
    { id: 'q65', name: 'Quality −35%', bitrate: '65%' },
    { id: 'q50', name: 'Quality −50%', bitrate: '50%' },
    { id: '720', name: '720p', resolution: '720' },
    { id: '480', name: '480p', resolution: '480' },
    { id: '360', name: '360p', resolution: '360' },
];

// The file playing: { item, codecs, duration, offset, audio, subtitle, mode, encode, failed,
// start }. audio and subtitle are track indexes (-1: the default audio, no subtitles). mode is a
// TRANSCODE_MODES id; encode: Compatibility transcodes the video even if it could be copied (the
// copy didn't play); failed: the original didn't play; audioStep: how far down audioSteps it is.
let player = null;
let playRequest = 0; // the latest playVideo call: an older one still checking codecs gives up

// fromFolder: whether this viewing began from a folder; playing a folder's own item says so
// itself (a Continue Watching card for the same file is another object), and what plays next
// passes it on.
async function playVideo(item, forceTranscode = false, startAt = 0, fromFolder = folderVideos().includes(item)) {
    stopThemeMusic(800); // a title page's theme fades as its video starts
    closeSubtitleSearch();
    const request = ++playRequest;
    resetPlayer();
    if (isDiscImage(item)) {
        playDisc(item, startAt, fromFolder);
        return;
    }
    const p = { item, codecs: null, duration: item.duration || 0, offset: 0, audio: -1, subtitle: -1,
        mode: forceTranscode ? 'auto' : 'original', encode: false, failed: false, audioStep: 0, start: startAt, fromFolder };
    player = p;
    openPlayer(item.name);
    loadSegments(p);
    let unreadable = null;
    try {
        p.codecs = await api.getCodecs(item.path);
    } catch (error) {
        console.warn('Could not read the codecs:', error);
        if (error.unreadable) unreadable = error.message;
    }
    if (request !== playRequest || player !== p) return; // closed or replaced meanwhile
    if (unreadable) {
        showPlaybackFailed(unreadable, unreadable);
        return;
    }
    if (p.codecs) {
        if (p.codecs.duration) p.duration = p.codecs.duration;
        restoreTracks(p);
    }
    renderFileControls();
    loadFileStream(startAt);
    loadTrickplay(p);
}

// The player's title for a show or movie, with the file name small beneath it. A viewing begun in
// a folder keeps the file name.
function showPlayerTitle(now, md) {
    if (now.fromFolder || !md) return;
    let title = null;
    if (md.kind === 'episode' && md.season != null && md.episode != null) {
        title = [md.show_title, episodeCode(md.season, md.episode), md.title].filter(Boolean).join(' · ');
    } else if (md.kind === 'movie' && md.title) {
        title = md.title + (md.year ? ` (${md.year})` : '');
    }
    if (!title) return;
    playerTitle.textContent = title;
    const file = document.getElementById('player-filename');
    file.textContent = now.item.name;
    file.hidden = false;
}

// The videos of the folder on screen, in order.
function folderVideos() {
    return state.view === 'folders' ? state.mediaItems.filter(item => item.type === 'video') : [];
}

// Plays what follows a video that reached its end: the next in the folder, else the show's next
// episode (any season, wherever it's played from). With nothing left, a viewing begun in a folder
// plays the last one again, and any other closes the player. now is what's playing, a file
// (player) or a disc, whichever title of it reached the end.
async function playNextAfterEnd(now) {
    // By path: a refresh of the folder (progress, thumbnails) replaces its items.
    const videos = now.fromFolder ? folderVideos() : [];
    const i = videos.findIndex(item => item.path === now.item.path);
    let next = i >= 0 ? videos[i + 1] : null;
    if (!next) {
        const segments = now.segments || await api.getSegments(now.item.path).catch(() => null);
        if ((player || disc) !== now) return; // closed or replaced meanwhile
        next = segments && segments.next;
    }
    if (next) {
        playVideo(next, false, resumePosition(next), now.fromFolder);
    } else if (now.fromFolder) {
        seekTo(0);
        videoPlayer.play().catch(() => {});
    } else {
        playerModal.querySelector('.close').click();
    }
}

function openPlayer(title) {
    playerTitle.textContent = title;
    document.getElementById('player-filename').hidden = true;
    videoPlayer.controls = false;
    document.getElementById('player-controls').hidden = false;
    playerModal.classList.add('active');
    updatePlayerButtons();
}

function audioTrack(p) {
    if (!p.codecs) return null;
    return p.codecs.audio[p.audio] || defaultAudioTrack(p.codecs);
}

function subtitleTrack(p) {
    return p.codecs && p.subtitle >= 0 ? p.codecs.subtitles[p.subtitle] || null : null;
}

// Whether browsers play the video as it is: 8-bit 4:2:0 H.264 (as the server's BrowserVideo).
function browserVideo(codecs) {
    const v = codecs && codecs.video;
    return !!v && v.codec === 'h264' && (v.pix_fmt === 'yuv420p' || v.pix_fmt === 'yuvj420p');
}

// The video's codec as browsers name it in a media type (RFC 6381), from the server's report:
// avc1.PPCCLL, hvc1…, av01…, vp09…. null for codecs with no such name (MPEG-4 Part 2, MPEG-2…).
function videoCodecString(v) {
    const depth = bitDepth(v);
    const hex = n => n.toString(16).padStart(2, '0');
    switch (v.codec) {
    case 'h264': {
        const profiles = {
            'Constrained Baseline': '4240', 'Baseline': '4200', 'Main': '4d00', 'Extended': '5800',
            'High': '6400', 'High 10': '6e00', 'High 10 Intra': '6e10', 'High 4:2:2': '7a00',
            'High 4:2:2 Intra': '7a10', 'High 4:4:4 Predictive': 'f400', 'High 4:4:4 Intra': 'f410',
        };
        return `avc1.${profiles[v.profile] || '6400'}${hex(v.level || 40)}`;
    }
    case 'hevc': {
        const profiles = { 'Main': '1.6', 'Main 10': '2.4', 'Main Still Picture': '3.E', 'Rext': '4.10' };
        return `hvc1.${profiles[v.profile] || '1.6'}.L${v.level || 120}.B0`;
    }
    case 'av1': {
        const profile = { 'Main': 0, 'High': 1, 'Professional': 2 }[v.profile] || 0;
        return `av01.${profile}.${String(v.level >= 0 ? v.level : 8).padStart(2, '0')}M.${String(depth).padStart(2, '0')}`;
    }
    case 'vp9': {
        const profile = Number((v.profile || '').replace(/\D/g, '')) || 0;
        const level = v.level > 0 ? v.level : 41;
        return `vp09.0${profile}.${level}.${String(depth).padStart(2, '0')}`;
    }
    case 'vp8':
        return 'vp8';
    }
    return null;
}

function bitDepth(v) {
    return /12[lb]e$/.test(v.pix_fmt || '') ? 12 : /10[lb]e$|p010/.test(v.pix_fmt || '') ? 10 : 8;
}

// Containers browsers don't open (ffprobe's format names); the video in them may still play
// once Compatibility (Auto) copies it into MP4.
const UNPLAYABLE_CONTAINERS = {
    mpegts: 'MPEG-TS', avi: 'AVI', asf: 'WMV', flv: 'FLV', mpeg: 'MPEG-PS', rm: 'RealMedia', nut: 'NUT',
};
// Video codecs no browser decodes.
const UNPLAYABLE_VIDEO = {
    mpeg4: 'MPEG-4', msmpeg4v3: 'DivX 3', msmpeg4v2: 'MS MPEG-4', mpeg2video: 'MPEG-2',
    mpeg1video: 'MPEG-1', vc1: 'VC-1', wmv3: 'WMV 9', wmv2: 'WMV 8', wmv1: 'WMV 7', h263: 'H.263',
    rv40: 'RealVideo', rv30: 'RealVideo', prores: 'ProRes', dvvideo: 'DV',
};

// Why the browser can't play the file as it is, before trying: "HEVC video", "MPEG-TS files", or
// '' (as far as can be told: other problems show up as a playback error).
function originalProblem(codecs) {
    const v = codecs && codecs.video;
    if (!v) return '';
    if (UNPLAYABLE_VIDEO[v.codec]) return `${UNPLAYABLE_VIDEO[v.codec]} video`;
    const codec = videoCodecString(v);
    if (codec) {
        const type = `video/${codec === 'vp8' ? 'webm' : 'mp4'}; codecs="${codec}"`;
        const supported = (window.MediaSource && MediaSource.isTypeSupported(type)) || videoPlayer.canPlayType(type) !== '';
        if (!supported) {
            const names = { h264: 'H.264', hevc: 'HEVC', av1: 'AV1', vp9: 'VP9', vp8: 'VP8' };
            const depth = bitDepth(v) > 8 ? ` ${bitDepth(v)}-bit` : '';
            const chroma = (v.profile || '').match(/4:[24]:[24]/);
            return `${names[v.codec]}${depth}${chroma ? ' ' + chroma[0] : ''} video`;
        }
    }
    const container = (codecs.container || '').split(',').find(name => UNPLAYABLE_CONTAINERS[name]);
    return container ? `${UNPLAYABLE_CONTAINERS[container]} files` : '';
}

// The audio has to come from the server: the browser can't decode the track, or it isn't the
// default one (the only one a browser plays from a file).
function audioNeedsConverting(p) {
    const track = audioTrack(p);
    return !!track && (track !== defaultAudioTrack(p.codecs) || !browserPlaysAudio(track));
}

// Picture subtitles the player draws itself, from the server's .sup data (see pgs.js): PGS, and
// DVD's, which the server sends as PGS.
function drawsPictureSubtitles(sub) {
    return sub.codec === 'hdmv_pgs_subtitle' || sub.codec === 'dvd_subtitle';
}

// Other picture subtitles (DVB, XSUB) can only be burned into a transcode.
function burnsInSubtitles(p) {
    const sub = subtitleTrack(p);
    return !!sub && !sub.text && !drawsPictureSubtitles(sub);
}

// Compatibility (Auto) copies the video and converts only the audio.
function copiesVideo(p) {
    return p.mode === 'auto' && !p.encode && !burnsInSubtitles(p) && browserVideo(p.codecs);
}

// The file itself, seeked by the browser; anything else is a stream from an offset.
function playsOriginalFile(p) {
    return p.mode === 'original';
}

// Original won't play the chosen tracks: Compatibility (Auto) converts them.
function chooseCompatibility(p) {
    if (p.mode !== 'original') return;
    if (audioNeedsConverting(p) || burnsInSubtitles(p) || originalProblem(p.codecs)) p.mode = 'auto';
}

// Audio a stream can carry as it is (not TrueHD, which is only experimental in MP4; not MP3, see
// AUDIO_TYPES: it's at most stereo, so converting it to AAC costs little).
const KEPT_AUDIO = ['aac', 'ac3', 'eac3', 'opus', 'flac', 'dts'];
const AUDIO_STEPS = {
    copy: { query: '&audiocodec=copy' },
    surround: { query: '&audiocodec=aac', label: 'AAC 5.1' },
    stereo: { query: '&audiocodec=aac&channels=2', label: 'stereo' },
};

// Whether the browser's output has surround channels to send 5.1 to.
let surroundOutputKnown = null;
function surroundOutput() {
    if (surroundOutputKnown === null) {
        try {
            const context = new AudioContext();
            surroundOutputKnown = context.destination.maxChannelCount >= 6;
            context.close();
        } catch (error) {
            surroundOutputKnown = false;
        }
    }
    return surroundOutputKnown;
}

// The audio for a stream in the order to try it, as on Android (audioLadder in StreamChoice.kt): the
// track as it is if the browser plays it, 5.1 if the output has it, then stereo. Safe keeps nothing;
// a smaller stream keeps only audio that stays small.
function audioSteps(p) {
    if (p.mode === 'safe') return ['stereo'];
    const mode = TRANSCODE_MODES.find(m => m.id === p.mode) || TRANSCODE_MODES[0];
    const track = audioTrack(p), steps = [];
    const big = (mode.bitrate || mode.resolution) && track && ['flac', 'dts'].includes(track.codec);
    if (track && !big && KEPT_AUDIO.includes(track.codec) && browserPlaysAudio(track)) steps.push('copy');
    if (track && track.channels > 2 && surroundOutput()) steps.push('surround');
    steps.push('stereo');
    return steps;
}

function audioStep(p) {
    const steps = audioSteps(p);
    return AUDIO_STEPS[steps[Math.min(p.audioStep, steps.length - 1)]];
}

function audioName(track) {
    return track.codec.toUpperCase() + (track.channels >= 8 ? ' 7.1' : track.channels >= 6 ? ' 5.1' : '');
}

// What the server does to the file, for the line under the controls.
function streamNote(p) {
    const track = audioTrack(p), video = p.codecs && p.codecs.video;
    const unplayable = track && !browserPlaysAudio(track) ? track.codec.toUpperCase() : '';
    if (p.mode === 'original') {
        if (unplayable) return `This browser can’t play ${unplayable} audio, so the original plays without sound.`;
        if (audioNeedsConverting(p)) return 'The original plays its default audio track.';
        return '';
    }
    const problem = originalProblem(p.codecs);
    const why = p.failed ? 'This browser couldn’t play the original. '
        : problem ? `This browser can’t play ${problem}. ` : '';
    const step = audioStep(p);
    const audio = step.label ? `converted to ${step.label}` : `original${track ? ` (${audioName(track)})` : ''}`;
    const copied = copiesVideo(p) ? (problem.endsWith(' files') ? 'copied into MP4' : 'copied') : `transcoded to H.264${video && video.codec !== 'h264' ? ` from ${video.codec.toUpperCase()}` : ''}`;
    const subs = burnsInSubtitles(p) ? ', subtitles burned in' : '';
    return `${why}Video: ${copied}${subs} · Audio: ${audio}.`;
}

function fileStreamUrl(p, start) {
    const mode = TRANSCODE_MODES.find(m => m.id === p.mode) || TRANSCODE_MODES[0];
    if (playsOriginalFile(p)) return api.getStreamUrl(p.item.path);
    // transcode=audio: the server copies the video if it can, and converts the audio.
    let url = api.getStreamUrl(p.item.path, copiesVideo(p) ? 'audio' : true);
    if (mode.bitrate) url += `&bitrate=${mode.bitrate}`;
    if (mode.resolution) url += `&resolution=${mode.resolution}`;
    if (p.audio >= 0) url += `&audio=${p.audio}`;
    if (burnsInSubtitles(p)) url += `&subtitle=${p.subtitle}`;
    url += audioStep(p).query;
    if (start > 0) url += `&start=${start.toFixed(2)}`;
    return url;
}

// Where a stream asked to start at start (seconds into the file) begins: a copied video starts at
// the keyframe the server's seek lands on, up to several seconds earlier.
async function streamStart(p, start) {
    if (!(start > 0) || !copiesVideo(p)) return start;
    try {
        const begins = (await api.getSeekPoint(p.item.path, start.toFixed(2))).start;
        return begins >= 0 ? begins : start; // below 0: no time at all (an older server, an AVI)
    } catch (error) {
        console.warn('Could not find where the stream starts:', error);
        return start;
    }
}

// A jump forward from `from` that the keyframe before start would undo (keyframes far apart, a
// short jump) goes on to the next keyframe instead, further than asked: otherwise it never moves.
async function jumpStart(p, from, start) {
    let begins = await streamStart(p, start);
    if (from == null || start <= from || begins > from || !copiesVideo(p)) return { start, begins };
    try {
        const next = await api.getSeekPoint(p.item.path, start.toFixed(2), true);
        if (!(next.start > from) || !(parseFloat(next.ask) > 0)) return { start, begins };
        return { start: parseFloat(next.ask), begins: next.start };
    } catch (error) {
        console.warn('No keyframe after the jump:', error);
        return { start, begins };
    }
}

// Starts the stream at start (seconds into the file); from: where a jump began, if it's one.
async function loadFileStream(start, from = null) {
    const p = player;
    if (p.duration) start = Math.min(start, Math.max(p.duration - 1, 0));
    start = Math.max(0, start);
    const original = playsOriginalFile(p);
    const load = p.load = (p.load || 0) + 1;
    let begins = 0;
    if (!original) ({ start, begins } = await jumpStart(p, from, start));
    if (player !== p || p.load !== load) return; // closed, or another seek meanwhile
    p.offset = begins;
    p.start = start;
    p.key = fileStreamUrl(p, 0); // the stream, wherever it starts: a change of it restarts it
    attachStream(fileStreamUrl(p, start), p.item, original ? start : 0);
    restartSubtitles(start);
    updatePlayerPosition();
}

// Plays url in the player. seekOnLoad: where to seek once it's loaded (an original file).
function attachStream(url, item, seekOnLoad = 0) {
    document.getElementById('player-failed').hidden = true;
    videoPlayer.pause();
    videoPlayer.innerHTML = '';
    videoPlayer.src = url;
    videoPlayer.load();
    videoPlayer.onloadedmetadata = () => {
        if (seekOnLoad > 0) videoPlayer.currentTime = seekOnLoad;
        videoPlayer.play().catch(err => console.error('Error playing video:', err));
    };
    videoPlayer.onerror = onPlayerError;
    videoPlayer.ontimeupdate = () => {
        updatePlayerPosition();
        keepSubtitlesAhead(playerPosition());
        updateSkipButton();
    };
    videoPlayer.onclick = togglePlay;
    videoPlayer.ondblclick = togglePlayerFullscreen;
    startPlaybackReports(item);
    // After the reports, which set onplay/onpause themselves.
    const onPlay = videoPlayer.onplay, onPause = videoPlayer.onpause;
    videoPlayer.onplay = () => { if (onPlay) onPlay(); updatePlayerButtons(); };
    videoPlayer.onpause = () => { if (onPause) onPause(); updatePlayerButtons(); };
    videoPlayer.onvolumechange = updatePlayerButtons;
    updatePlayerButtons();
}

// Shows that the video can't play, and why; detail (the server's reason) goes under the message too.
function showPlaybackFailed(reason, detail = '') {
    document.getElementById('player-status').textContent = reason;
    document.getElementById('player-failed-reason').textContent = detail;
    document.getElementById('player-failed').hidden = false;
}

// The browser can't play the stream (its video codec, its container): Original switches to
// Compatibility (Auto), as on Android, and a copied video gets transcoded. Unless the server can't
// read the file at all (a share refusing it, say): then it says so, as no other stream would help.
async function onPlayerError() {
    const err = videoPlayer.error;
    if (!err || !videoPlayer.getAttribute('src')) return; // closed
    console.error('Video error:', err);
    const p = player;
    const src = videoPlayer.getAttribute('src');
    if (p) {
        try {
            await api.getCodecs(p.item.path);
        } catch (error) {
            if (player !== p || videoPlayer.getAttribute('src') !== src) return; // moved on meanwhile
            if (error.unreadable) {
                showPlaybackFailed(error.message, error.message);
                return;
            }
        }
        if (player !== p || videoPlayer.getAttribute('src') !== src) return;
    }
    const steps = p ? audioSteps(p) : [];
    const canStepDown = p && (p.mode === 'original' || copiesVideo(p) || p.audioStep < steps.length - 1);
    if (!canStepDown) {
        showPlaybackFailed(`Playback failed: ${err.message || 'error ' + err.code}`);
        return;
    }
    const at = videoPlayer.currentTime > 0 ? playerPosition() : p.start;
    if (p.mode === 'original') p.mode = 'auto';
    else if (copiesVideo(p)) p.encode = true;
    else p.audioStep++;
    p.failed = true;
    renderFileControls();
    loadFileStream(at);
}

// Moves playback to t seconds into the file (the disc title): within what has arrived, the
// stream seeks; elsewhere a transcoded stream restarts there.
function seekTo(t) {
    const duration = playerDuration();
    if (duration) t = Math.min(t, Math.max(duration - 1, 0));
    t = Math.max(0, t);
    if (disc && disc.info) {
        if (inBuffer(t - disc.offset)) {
            videoPlayer.currentTime = t - disc.offset;
            seekSubtitles(t);
        } else {
            loadDiscStream(t);
        }
        return;
    }
    if (!player) return;
    if (playsOriginalFile(player) || inBuffer(t - player.offset)) {
        videoPlayer.currentTime = t - player.offset;
        seekSubtitles(t);
        updatePlayerPosition();
    } else {
        loadFileStream(t, playerPosition());
    }
}

// Whether a stream can seek to t (seconds into the stream) without restarting: it has arrived,
// and the browser can seek there. A streamed transcode's buffer isn't always seekable (Chromium
// can report nothing seekable for one), and seeking there would jump to its start instead.
function inBuffer(t) {
    const within = (ranges, end) => {
        for (let i = 0; i < ranges.length; i++) {
            if (t >= ranges.start(i) && t <= ranges.end(i) - end) return true;
        }
        return false;
    };
    return within(videoPlayer.buffered, 0.5) && within(videoPlayer.seekable, 0);
}

function playerDuration() {
    if (disc && disc.info) return discTitle().duration;
    if (!player) return 0;
    if (player.duration) return player.duration;
    return playsOriginalFile(player) && isFinite(videoPlayer.duration) ? videoPlayer.duration : 0;
}

function trackDescription(track) {
    const parts = [];
    if (track.language && track.language !== 'und') parts.push(track.language);
    if (track.title) parts.push(track.title);
    parts.push(track.codec);
    if (track.channels) parts.push(track.channels === 6 ? '5.1' : track.channels === 8 ? '7.1' : track.channels === 2 ? 'stereo' : track.channels === 1 ? 'mono' : `${track.channels} ch`);
    if (track.forced) parts.push('forced');
    if (track.hearing_impaired) parts.push('SDH');
    if (track.external === 'opensubtitles') parts.push('downloaded');
    return `${track.index + 1}: ${parts.join(' · ')}`;
}

function renderFileControls() {
    const p = player, codecs = p.codecs || { audio: [], subtitles: [] };
    document.getElementById('player-stream-label').hidden = false;
    document.querySelectorAll('.disc-only').forEach(el => { el.hidden = true; });
    fillSelect(document.getElementById('player-stream'), TRANSCODE_MODES.map(m => [m.id, m.name]), p.mode);
    const track = audioTrack(p);
    document.getElementById('player-audio-label').hidden = codecs.audio.length < 2 && !(track && !browserPlaysAudio(track));
    fillSelect(document.getElementById('player-audio'),
        codecs.audio.length ? codecs.audio.map(a => [a.index, trackDescription(a) + (a.default ? ' (default)' : '')]) : [[-1, 'None']],
        track ? track.index : -1);
    document.getElementById('player-subtitle-label').hidden = false;
    fillSelect(document.getElementById('player-subtitle'), [[-1, 'Off'], ...codecs.subtitles.map(s =>
        [s.index, trackDescription(s) + (s.text || drawsPictureSubtitles(s) ? '' : ' (burned in)')]), ['search', 'Search online…']], p.subtitle);
    updateSyncControls();
    document.getElementById('player-status').textContent = streamNote(p);
}

// Applies a change to the file's stream or tracks: restarts the stream if it changed.
function changeFileStream(change) {
    const p = player;
    if (!p) return;
    const at = playerPosition();
    change(p);
    saveTracks(p);
    renderFileControls();
    if (fileStreamUrl(p, 0) !== p.key) loadFileStream(at);
    else restartSubtitles(at);
}

// The tracks chosen for each video, kept in this browser so it plays the same way next time.
function restoreTracks(p) {
    let saved = null;
    try { saved = JSON.parse(localStorage.getItem('tracks:' + p.item.path) || 'null'); } catch (e) { /* none */ }
    const codecs = p.codecs;
    if (saved && saved.audio >= 0 && saved.audio < codecs.audio.length) p.audio = saved.audio;
    // Else the track in the account's audio language, if the file has one (the server picks it).
    else if (codecs.preferred_audio >= 0) p.audio = codecs.preferred_audio;
    if (saved && saved.subtitle < codecs.subtitles.length) p.subtitle = saved.subtitle;
    else if (codecs.preferred_subtitle >= 0) p.subtitle = codecs.preferred_subtitle;
    else {
        // Off unless the file has forced subtitles (translations of foreign-language lines).
        const forced = codecs.subtitles.find(s => s.forced && s.text);
        p.subtitle = forced ? forced.index : -1;
    }
    chooseCompatibility(p);
}

function saveTracks(p) {
    try {
        localStorage.setItem('tracks:' + p.item.path, JSON.stringify({ audio: p.audio, subtitle: p.subtitle }));
    } catch (e) { /* not kept */ }
}

// Subtitles online: OpenSubtitles results for the playing video, kept on the server once chosen.
function subtitleLanguages() {
    try {
        const saved = localStorage.getItem('subtitleLanguages');
        if (saved) return saved.split(',').map(l => l.trim()).filter(Boolean);
    } catch (e) { /* none saved */ }
    const langs = [...new Set((navigator.languages || [navigator.language || 'en']).map(l => l.slice(0, 2).toLowerCase()))];
    return langs.length ? langs : ['en'];
}

function openSubtitleSearch() {
    if (!player) return;
    document.getElementById('subtitle-languages').value = subtitleLanguages().join(', ');
    document.getElementById('subtitle-search').hidden = false;
    runSubtitleSearch();
}

function closeSubtitleSearch() {
    document.getElementById('subtitle-search').hidden = true;
}

async function runSubtitleSearch() {
    const p = player;
    const note = document.getElementById('subtitle-search-note');
    const list = document.getElementById('subtitle-results');
    const languages = document.getElementById('subtitle-languages').value.split(/[,\s]+/).map(l => l.trim()).filter(Boolean);
    try { localStorage.setItem('subtitleLanguages', languages.join(',')); } catch (e) { /* not kept */ }
    note.textContent = 'Searching…';
    list.innerHTML = '';
    let found;
    try {
        found = await api.searchSubtitles(p.item.path, languages);
    } catch (error) {
        if (player === p) note.textContent = error.message;
        return;
    }
    if (player !== p) return;
    const q = found.query;
    const what = [q.title, q.year, q.season || q.episode ? `S${String(q.season).padStart(2, '0')}E${String(q.episode).padStart(2, '0')}` : '']
        .filter(Boolean).join(' ');
    note.textContent = found.results.length
        ? `${found.results.length} found for ${what}${q.tmdb ? ' (by its TMDB match)' : ''}. Best matches first.`
        : `Nothing found for ${what} in ${(q.languages || []).join(', ')}.`;
    found.results.forEach(r => list.appendChild(subtitleResultButton(p, r)));
}

function subtitleResultButton(p, r) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'subtitle-hit';
    const name = document.createElement('span');
    name.className = 'subtitle-hit-name';
    name.textContent = r.release;
    const details = document.createElement('span');
    details.className = 'subtitle-hit-details';
    details.textContent = [r.language, `${r.downloads.toLocaleString()} downloads`,
        r.hash_match && 'made for this file', r.trusted && 'trusted', r.hearing_impaired && 'SDH',
        r.foreign_parts_only && 'foreign parts only', r.machine_translated && 'machine translated',
        r.downloaded && 'already downloaded'].filter(Boolean).join(' · ');
    b.append(name, details);
    b.onclick = () => chooseSubtitleResult(p, r, b);
    return b;
}

async function chooseSubtitleResult(p, r, button) {
    const note = document.getElementById('subtitle-search-note');
    document.querySelectorAll('.subtitle-hit').forEach(el => { el.disabled = true; });
    note.textContent = r.downloaded ? 'Loading…' : 'Downloading…';
    let got;
    try {
        got = await api.downloadSubtitle(p.item.path, r);
    } catch (error) {
        note.textContent = error.message;
        document.querySelectorAll('.subtitle-hit').forEach(el => { el.disabled = false; });
        return;
    }
    if (player !== p) return;
    closeSubtitleSearch();
    p.codecs.subtitles = got.subtitles;
    changeFileStream(q => { q.subtitle = got.index; chooseCompatibility(q); });
    if (got.quota) console.info(`OpenSubtitles: ${got.quota.remaining} downloads left today`);
}

// Subtitle style (Settings → Subtitles), kept per browser: Default is the browser's own look
// (white on translucent black); Custom sets colours, outline, font and size for the player's cues
// (video::cue) and the preview.
const SUBTITLE_STYLE_DEFAULTS = { mode: 'default', background: 'black', opacity: 0.5, foreground: 'white',
    textOpacity: 1, outline: false, font: 'default', size: 'normal' };
const SUBTITLE_SIZES = { tiny: 0.6, small: 0.8, normal: 1, large: 1.3, huge: 1.6 };
const SUBTITLE_FONTS = {
    default: 'sans-serif',
    serif: 'Georgia, "Times New Roman", serif',
    monospace: '"Courier New", "DejaVu Sans Mono", monospace',
    casual: '"Comic Neue", "Comic Sans MS", "Segoe Print", cursive',
    condensed: '"Roboto Condensed", "Arial Narrow", "DejaVu Sans Condensed", sans-serif',
};

function subtitleStyle() {
    try {
        return { ...SUBTITLE_STYLE_DEFAULTS, ...JSON.parse(localStorage.getItem('subtitleStyle') || '{}') };
    } catch (e) {
        return { ...SUBTITLE_STYLE_DEFAULTS };
    }
}

// A colour with alpha, from the hex or rgb() a theme variable holds.
function withAlpha(color, alpha) {
    const c = color.trim();
    let r, g, b;
    if (c.startsWith('#')) {
        const hex = c.length === 4 ? c.slice(1).split('').map(x => x + x).join('') : c.slice(1, 7);
        [r, g, b] = [0, 2, 4].map(i => parseInt(hex.slice(i, i + 2), 16));
    } else {
        [r, g, b] = (c.match(/[\d.]+/g) || [0, 0, 0]).map(Number);
    }
    return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

// The CSS a style gives the subtitle text (in em-free units, so the cue and the preview share it).
function subtitleCss(style) {
    if (style.mode !== 'custom') {
        return { color: '#fff', background: 'rgba(0, 0, 0, 0.8)', 'font-family': SUBTITLE_FONTS.default, 'text-shadow': 'none', scale: 1 };
    }
    const theme = getComputedStyle(document.documentElement);
    const named = { white: '#ffffff', black: '#000000' };
    const accent = theme.getPropertyValue('--progress-color') || '#f6c445';
    const box = theme.getPropertyValue('--surface-color') || '#321b2f';
    const text = style.foreground === 'themed' ? accent : named[style.foreground] || '#ffffff';
    const back = style.background === 'themed' ? box : named[style.background];
    // The text's opacity applies to its outline too (else the outline shows through the letters).
    const textAlpha = Number(style.textOpacity) || 1;
    const css = { color: withAlpha(text, textAlpha), background: 'transparent', 'text-shadow': 'none',
        'font-family': SUBTITLE_FONTS[style.font] || SUBTITLE_FONTS.default, scale: SUBTITLE_SIZES[style.size] || 1 };
    if (back && style.outline) {
        const o = withAlpha(back, textAlpha);
        css['text-shadow'] = [[-1, -1], [1, -1], [-1, 1], [1, 1], [0, -1.5], [0, 1.5], [-1.5, 0], [1.5, 0]]
            .map(([x, y]) => `${x}px ${y}px 0 ${o}`).join(', ');
    } else if (back) {
        css.background = withAlpha(back, Number(style.opacity) || 1);
    }
    return css;
}

// Applies the saved style to the player's cues and the preview.
function applySubtitleStyle() {
    const css = subtitleCss(subtitleStyle());
    let sheet = document.getElementById('subtitle-style-sheet');
    if (!sheet) {
        sheet = document.createElement('style');
        sheet.id = 'subtitle-style-sheet';
        document.head.appendChild(sheet);
    }
    const rules = [`color: ${css.color}`, `background: ${css.background}`, `text-shadow: ${css['text-shadow']}`,
        `font-family: ${css['font-family']}`];
    sheet.textContent = subtitleStyle().mode === 'custom'
        ? `#video-player::cue { ${rules.join('; ')}; font-size: ${Math.round(css.scale * 100)}%; }` : '';
    const preview = document.getElementById('subtitle-preview-text');
    if (preview) {
        rules.forEach(r => { const [k, v] = r.split(': '); preview.style.setProperty(k, v); });
        preview.style.fontSize = `${(22 * css.scale).toFixed(1)}px`;
        const cutout = document.getElementById('subtitle-preview-cutout');
        preview.hidden = cutoutSubtitles();
        cutout.hidden = !cutoutSubtitles();
        cutout.replaceChildren();
        if (cutoutSubtitles()) drawCutoutSubtitles(cutout, ['The quick brown fox', 'jumps over the lazy dog.'], 22);
    }
    // A player showing subtitles switches between the browser's cues and the cut-out ones.
    if (subs) {
        subs.track.mode = cutoutSubtitles() ? 'hidden' : 'showing';
        showCutoutCues();
    }
}

// Text: None makes see-through letters, cut out of the box or the outline. ::cue can't draw that,
// so those cues are drawn over the video instead, as SVG: the box or outline, masked by the letters.
function cutoutSubtitles() {
    const style = subtitleStyle();
    return style.mode === 'custom' && style.foreground === 'none';
}

let cutoutMasks = 0, cutoutMeasure = null;
function drawCutoutSubtitles(container, lines, basePx) {
    const style = subtitleStyle();
    const theme = getComputedStyle(document.documentElement);
    const named = { white: '#ffffff', black: '#000000' };
    const back = style.background === 'themed' ? (theme.getPropertyValue('--surface-color') || '#321b2f').trim() : named[style.background];
    if (!back || !lines.length) return;
    const ns = 'http://www.w3.org/2000/svg';
    const el = (name, attrs, parent) => {
        const e = document.createElementNS(ns, name);
        Object.entries(attrs).forEach(([k, v]) => e.setAttribute(k, v));
        if (parent) parent.appendChild(e);
        return e;
    };
    const px = basePx * (SUBTITLE_SIZES[style.size] || 1);
    const lineHeight = px * 1.3, padX = px * 0.4, stroke = px * 0.2;
    const font = { 'font-family': SUBTITLE_FONTS[style.font] || SUBTITLE_FONTS.default, 'font-size': px, 'text-anchor': 'middle' };
    const svg = el('svg', {}, container);
    // The lines' widths, from a canvas (text in a hidden page, like Settings at load, measures 0).
    const measure = (cutoutMeasure ||= document.createElement('canvas').getContext('2d'));
    measure.font = `${px}px ${font['font-family']}`;
    const widths = lines.map(line => measure.measureText(line).width);
    const width = Math.ceil(Math.max(...widths) + 2 * padX + stroke), height = Math.ceil(lines.length * lineHeight + stroke);
    svg.setAttribute('width', width);
    svg.setAttribute('height', height);
    svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
    const addLines = (parent, attrs) => lines.forEach((line, i) => {
        const t = el('text', { ...font, ...attrs, x: width / 2, y: stroke / 2 + i * lineHeight + lineHeight * 0.76 }, parent);
        t.textContent = line;
    });
    // Where the letters are, the mask hides what's drawn.
    const id = `cutout-mask-${++cutoutMasks}`;
    const mask = el('mask', { id, maskUnits: 'userSpaceOnUse', x: 0, y: 0, width, height }, el('defs', {}, svg));
    el('rect', { x: 0, y: 0, width, height, fill: 'white' }, mask);
    addLines(mask, { fill: 'black' });
    const drawn = el('g', { mask: `url(#${id})` }, svg);
    if (style.outline) {
        // The outer half of a stroke around the letters, as solid as the text opacity says.
        drawn.setAttribute('opacity', Number(style.textOpacity) || 1);
        addLines(drawn, { fill: 'none', stroke: back, 'stroke-width': stroke * 0.8, 'stroke-linejoin': 'round' });
    } else {
        drawn.setAttribute('fill-opacity', Number(style.opacity) || 1);
        lines.forEach((line, i) => el('rect', { x: (width - widths[i]) / 2 - padX, y: stroke / 2 + i * lineHeight,
            width: widths[i] + 2 * padX, height: lineHeight, rx: px * 0.15, fill: back }, drawn));
    }
}

// The cues showing now, drawn cut out over the video (when Text is None).
function showCutoutCues() {
    const box = document.getElementById('player-cutout-subs');
    box.replaceChildren();
    const on = !!(subs && cutoutSubtitles());
    box.hidden = !on;
    if (!on) return;
    const lines = [...(subs.track.activeCues || [])]
        .flatMap(cue => cue.text.replace(/<[^>]*>/g, '').split('\n'))
        .map(line => line.trim()).filter(Boolean);
    // Where the browser's own cues go (their last line 10% of the video's height above its bottom),
    // as big as theirs: 5% of its height.
    const v = videoPlayer, h = v.offsetHeight;
    box.style.left = `${v.offsetLeft}px`;
    box.style.width = `${v.offsetWidth}px`;
    box.style.bottom = `${box.offsetParent ? box.offsetParent.clientHeight - (v.offsetTop + h) + h * (100 - SUBTITLE_LINE) / 100 : 0}px`;
    drawCutoutSubtitles(box, lines, h * 0.05);
}

function setupSubtitleStyle() {
    const fields = { background: 'subtitle-background', opacity: 'subtitle-opacity', foreground: 'subtitle-foreground',
        textOpacity: 'subtitle-text-opacity', font: 'subtitle-font', size: 'subtitle-size' };
    const render = () => {
        const style = subtitleStyle();
        document.querySelectorAll('input[name="subtitle-mode"]').forEach(r => { r.checked = r.value === style.mode; });
        Object.entries(fields).forEach(([key, id]) => { document.getElementById(id).value = String(style[key]); });
        document.getElementById('subtitle-outline').checked = style.outline;
        // Translucency is for a box: not with no background, nor with an outline.
        document.getElementById('subtitle-opacity').disabled = style.background === 'none' || style.outline;
        document.getElementById('subtitle-outline').disabled = style.background === 'none';
        document.querySelector('.subtitle-style-options').classList.toggle('disabled', style.mode !== 'custom');
        applySubtitleStyle();
    };
    const save = (change) => {
        // Changing any option makes the style Custom.
        const style = { ...subtitleStyle(), mode: 'custom', ...change };
        try { localStorage.setItem('subtitleStyle', JSON.stringify(style)); } catch (e) { /* not kept */ }
        render();
    };
    document.querySelectorAll('input[name="subtitle-mode"]').forEach(r => {
        r.onchange = () => save({ mode: r.value });
    });
    Object.entries(fields).forEach(([key, id]) => {
        document.getElementById(id).onchange = (e) => save({ [key]: key === 'opacity' || key === 'textOpacity' ? Number(e.target.value) : e.target.value });
    });
    document.getElementById('subtitle-outline').onchange = (e) => save({ outline: e.target.checked });
    // Themed colours follow the theme.
    new MutationObserver(applySubtitleStyle).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });
    render();
}

// Trickplay frames, for the picture over the seek bar: [{ time, url }] by time.
// For a video playing (player) or a disc (disc): a disc's are for one title (framesTitle).
async function loadTrickplay(p) {
    if (!p.item.id) return;
    try {
        const info = await api.request(`/api/media/trickplay/${p.item.id}`);
        if (player === p || disc === p) {
            p.frames = info.frames || [];
            p.framesTitle = info.title || 0;
        }
    } catch (e) { /* none yet */ }
}

// ---------------------------------------------------------------------------------------------
// Text subtitles: the server sends a track as WebVTT a couple of minutes at a time (subtitles are
// spread through the whole file, and reading all of it at once would be slow on a big file), and
// the player adds the cues to a text track of the video, asking for the next minutes before it
// gets there. A transcoded stream's time counts from its offset, and the cues are shifted to match.

const SUBTITLE_WINDOW = 120; // seconds of cues per request
const SUBTITLE_LINE = 90; // where cues end, in percent of the picture's height from the top
let subs = null; // { track, index, offset, shift, from, until, loading, abort }

function subtitleTextTrack() {
    if (!videoPlayer.subtitleTrack) {
        videoPlayer.subtitleTrack = videoPlayer.addTextTrack('subtitles', 'Subtitles');
    }
    return videoPlayer.subtitleTrack;
}

function stopSubtitles() {
    if (subs && subs.abort) subs.abort.abort();
    if (subs && subs.renderer) subs.renderer.destroy();
    subs = null;
    document.getElementById('player-pgs-box').hidden = true;
    document.getElementById('player-ass-box').hidden = true;
    const track = subtitleTextTrack();
    while (track.cues && track.cues.length) track.removeCue(track.cues[0]);
    track.mode = 'disabled';
    showCutoutCues();
}

// A DVD track's forced lines (foreign dialogue meant to show with subtitles off), when subtitles
// are off: from the DVD track in the language of the audio playing. tracks are the file's or the
// disc title's subtitles; null when none matches.
function forcedLinesTrack(tracks, audio) {
    const language = audio && audio.language;
    return (language && tracks.find(s => (s.codec === 'dvd_subtitle') && s.language === language)) || null;
}

// Starts the chosen subtitles from position (seconds into the file or disc title), or with
// subtitles off a DVD track's forced lines.
function restartSubtitles(position) {
    stopSubtitles();
    if (disc && disc.info) {
        // A disc title's subtitles are DVD pictures, drawn as a file's: the server reads them as
        // the stream reads the disc (from where it started, disc.offset).
        const t = discTitle();
        let sub = t.subtitles[disc.subtitle] || null, forced = false;
        if (!sub) { sub = forcedLinesTrack(t.subtitles, t.audio[disc.audio]); forced = !!sub; }
        if (sub) startPictureSubtitles(sub, position, forced);
        return;
    }
    const p = player;
    let sub = p && subtitleTrack(p), forced = false;
    if (p && !sub && p.codecs) { sub = forcedLinesTrack(p.codecs.subtitles, audioTrack(p)); forced = !!sub; }
    if (sub && drawsPictureSubtitles(sub)) {
        startPictureSubtitles(sub, position, forced);
        return;
    }
    if (forced) return;
    if (sub && drawsAssSubtitles(sub)) {
        const from = Math.max(0, position - 10);
        subs = { ass: true, index: sub.index, header: '', format: '', lines: [], seen: new Set(),
                 shift: (sub.offset_ms || 0) / 1000, renderer: null, from, until: from, loading: false,
                 abort: new AbortController(), cjkSeen: new Set(), cjkFonts: [] };
        loadAssWindow(subs);
        return;
    }
    if (!sub || !sub.text) return;
    const track = subtitleTextTrack();
    track.mode = cutoutSubtitles() ? 'hidden' : 'showing';
    track.oncuechange = showCutoutCues;
    // A little earlier, for the cue already on screen there.
    const from = Math.max(0, position - 10);
    // shift: the track's timing correction (seconds; positive shows lines later)
    subs = { track, index: sub.index, offset: p.offset, shift: (sub.offset_ms || 0) / 1000, from, until: from,
             loading: false, abort: new AbortController() };
    loadSubtitleWindow(subs);
}

function startPictureSubtitles(sub, position, forced) {
    const from = Math.max(0, position - 10);
    subs = { track: subtitleTextTrack(), index: sub.index, forced, sets: [], shown: undefined, from, until: from,
             loading: false, abort: new AbortController() };
    loadPictureWindow(subs);
    requestAnimationFrame(() => showPictureSubtitles(subs));
}

// A window of a PGS track, merged into what's loaded: the windows overlap a little where a seek
// lands early. Sets far behind are dropped.
async function loadPictureWindow(s) {
    const from = s.until;
    s.until = from + SUBTITLE_WINDOW;
    s.loading = true;
    const item = disc && disc.info ? disc.item : player.item;
    let url = `${API_BASE}/api/media/subtitles?path=${encodeURIComponent(api.encodePath(item.path))}` +
        `&track=${s.index}&start=${from.toFixed(2)}&duration=${SUBTITLE_WINDOW}&format=sup`;
    // A disc's: its title, angle, and where its stream started, whose times the server matches.
    if (disc && disc.info) url += `&title=${discTitle().number}&angle=${disc.angle}&from=${disc.offset.toFixed(2)}`;
    if (s.forced) url += '&forced=1';
    try {
        const headers = api.authHeaders();
        const res = await fetch(url, { signal: s.abort.signal, credentials: 'same-origin', headers });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const decoded = decodePgs(await res.arrayBuffer());
        const known = new Set(s.sets.map(set => set.time));
        const position = playerPosition();
        s.sets = [...s.sets.filter(set => set.time > position - 60), ...decoded.filter(set => !known.has(set.time))]
            .sort((a, b) => a.time - b.time);
    } catch (err) {
        if (err.name !== 'AbortError') console.warn('Subtitles failed:', err);
    } finally {
        s.loading = false;
    }
}

// Draws the display set for the moment playing, while these subtitles are on. Each frame, as a
// picture's start is exact, unlike ontimeupdate's few times a second.
function showPictureSubtitles(s) {
    if (subs !== s) return;
    const position = playerPosition();
    let set = null;
    for (let i = s.sets.length - 1; i >= 0; i--) {
        if (s.sets[i].time <= position) { set = s.sets[i]; break; }
    }
    const canvas = document.getElementById('player-pgs'), box = document.getElementById('player-pgs-box');
    if (set !== s.shown) {
        s.shown = set;
        drawPgs(canvas, set);
    }
    box.hidden = !(set && set.parts.length);
    if (!box.hidden) placePictureSubtitles(box, canvas, set);
    requestAnimationFrame(() => showPictureSubtitles(s));
}

// Over the picture, inside any bars the video element has around it. The pictures are drawn for
// the disc's frame (1920x1080), and a video cropped of its black bars no longer fills that: it's
// centered on the video at the video's scale, as it was on the disc, and a line that would fall
// off the picture is moved onto it.
function placePictureSubtitles(box, canvas, set) {
    const v = videoPlayer;
    box.style.left = `${v.offsetLeft}px`;
    box.style.top = `${v.offsetTop}px`;
    box.style.width = `${v.offsetWidth}px`;
    box.style.height = `${v.offsetHeight}px`;
    const ratio = v.videoWidth && v.videoHeight ? v.videoWidth / v.videoHeight : set.width / set.height;
    const shownWidth = Math.min(v.offsetWidth, v.offsetHeight * ratio), shownHeight = shownWidth / ratio;
    const scale = Math.max(shownWidth / set.width, shownHeight / set.height);
    const left = (v.offsetWidth - set.width * scale) / 2, top = (v.offsetHeight - set.height * scale) / 2;
    const highest = Math.min(...set.parts.map(p => p.y));
    const lowest = Math.max(...set.parts.map(p => p.y + (p.crop ? p.crop.height : p.object.height)));
    const shownTop = (v.offsetHeight - shownHeight) / 2, shownBottom = shownTop + shownHeight;
    const moved = Math.max(0, shownTop - (top + highest * scale)) - Math.max(0, top + lowest * scale - shownBottom);
    canvas.style.left = `${left}px`;
    canvas.style.top = `${top + moved}px`;
    canvas.style.width = `${set.width * scale}px`;
    canvas.style.height = `${set.height * scale}px`;
}

// ASS/SSA: the server sends each window as a whole script (styles and its Dialogue lines). The
// first window's header is kept, every window's lines are added once, and libass (JASSUB, in
// vendor/jassub) is given the script so far: it draws the lines styled, positioned and animated,
// in the video's own fonts.
async function loadAssWindow(s) {
    const from = s.until;
    s.until = from + SUBTITLE_WINDOW;
    s.loading = true;
    const url = `${API_BASE}/api/media/subtitles?path=${encodeURIComponent(api.encodePath(player.item.path))}` +
        `&track=${s.index}&start=${from.toFixed(2)}&duration=${SUBTITLE_WINDOW}&format=ass`;
    try {
        const headers = api.authHeaders();
        const res = await fetch(url, { signal: s.abort.signal, credentials: 'same-origin', headers });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const text = (await res.text()).replace(/\r/g, '');
        if (subs !== s) return;
        const events = text.indexOf('\n[Events]');
        if (!s.header) s.header = (events >= 0 ? text.slice(0, events) : text).trim() + '\n\n[Events]\n';
        const lines = (events >= 0 ? text.slice(events) : '').split('\n');
        if (!s.format) s.format = lines.find(line => line.startsWith('Format:')) ||
            'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text';
        // A window starts at a keyframe before from: lines there came with the window before too.
        for (const line of lines) {
            if (line.startsWith('Dialogue:') && !s.seen.has(line)) {
                s.seen.add(line);
                s.lines.push(line);
            }
        }
        // The lines show at once; a font for their CJK characters follows when it's made (addCjkFonts
        // adds it to the renderer, which then draws them again).
        addCjkFonts(s, lines.filter(line => line.startsWith('Dialogue:')).join('\n'));
        await showAssSubtitles(s);
    } catch (err) {
        if (err.name !== 'AbortError') console.warn('Subtitles failed:', err);
    } finally {
        s.loading = false;
    }
}

// ASS/SSA tracks, which libass draws styled (see loadAssWindow).
function drawsAssSubtitles(sub) {
    return sub.codec === 'ass' || sub.codec === 'ssa';
}

// JASSUB's script, loaded the first time an ASS track is shown.
let jassubLoading = null;
function loadJassub() {
    if (window.JASSUB) return Promise.resolve();
    if (!jassubLoading) {
        jassubLoading = new Promise((resolve, reject) => {
            const script = document.createElement('script');
            script.src = 'vendor/jassub/jassub.umd.js';
            script.onload = resolve;
            script.onerror = () => { jassubLoading = null; reject(new Error('Could not load JASSUB')); };
            document.head.appendChild(script);
        });
    }
    return jassubLoading;
}

// The video's embedded fonts (the codecs report's fonts), which its ASS tracks name: fetched once
// per video, through the API (JASSUB's own fetches wouldn't carry the sign-in).
let assFonts = { path: null, fonts: [] };
async function videoFonts(p) {
    if (assFonts.path === p.item.path) return assFonts.fonts;
    const headers = api.authHeaders();
    const path = encodeURIComponent(api.encodePath(p.item.path));
    const fonts = await Promise.all(((p.codecs && p.codecs.fonts) || []).map(async f => {
        try {
            const res = await fetch(`${API_BASE}/api/media/font?path=${path}&font=${f.index}`, { credentials: 'same-origin', headers });
            return res.ok ? new Uint8Array(await res.arrayBuffer()) : null;
        } catch (err) {
            console.warn('Font failed:', f.name, err);
            return null;
        }
    }));
    assFonts = { path: p.item.path, fonts: fonts.filter(Boolean) };
    return assFonts.fonts;
}

// Gives libass the script so far, starting it the first time: over the picture, at the file's
// time less the track's timing correction (JASSUB's time is the video's plus timeOffset).
async function showAssSubtitles(s) {
    const script = s.header + s.format + '\n' + s.lines.join('\n') + '\n';
    if (s.renderer) {
        s.renderer.setTrack(script);
        redrawAss(s);
        return;
    }
    await loadJassub();
    const fonts = [...await videoFonts(player), ...s.cjkFonts];
    if (subs !== s) return;
    // A new canvas each time: JASSUB may hand one over to its worker, and it can't be used again.
    const box = document.getElementById('player-ass-box');
    const canvas = document.createElement('canvas');
    canvas.id = 'player-ass';
    box.replaceChildren(canvas);
    box.hidden = false;
    placeAssSubtitles(s);
    const at = file => new URL(`vendor/jassub/${file}`, location.href).href;
    s.renderer = new JASSUB({
        video: videoPlayer, canvas, subContent: script, fonts,
        workerUrl: at('jassub-worker.js'), wasmUrl: at('jassub-worker.wasm'), modernWasmUrl: at('jassub-worker-modern.wasm'),
        // Until a CJK subset is loaded the default font is Liberation Sans, so Latin lines always draw.
        availableFonts: { 'liberation sans': at('default.woff2') }, fallbackFont: 'liberation sans',
        timeOffset: player.offset - s.shift,
    });
    if (s.cjkFonts.length) s.renderer.setDefaultFont(CJK_FAMILY);
    redrawAss(s);
}

// JASSUB draws as the video's frames go by: while paused, a new script or timing is drawn here.
// Chinese, Japanese and Korean characters (and their punctuation): a player's own fonts often lack
// them, so the server makes a subset of a CJK font for the ones a track uses (once each).
const CJK_FAMILY = 'noto sans cjk jp'; // the name the subsets carry
const CJK_CHARACTER = /[ᄀ-ᇿ　-〿぀-ヿㇰ-ㇿ㄰-㆏㐀-䶿一-鿿가-힯豈-﫿＀-￯\u{20000}-\u{2FA1F}]/gu;

// The first call always asks (the subset holds the basic Latin too, the renderer's default font).
async function addCjkFonts(s, text) {
    const fresh = [...new Set(text.match(CJK_CHARACTER) || [])].filter(ch => !s.cjkSeen.has(ch));
    if (!fresh.length && s.cjkFonts.length) return;
    fresh.forEach(ch => s.cjkSeen.add(ch));
    try {
        const headers = { 'Content-Type': 'application/json' };
        Object.assign(headers, api.authHeaders());
        const res = await fetch(`${API_BASE}/api/media/subtitle-font`, {
            method: 'POST', signal: s.abort.signal, credentials: 'same-origin', headers,
            body: JSON.stringify({ text: fresh.join('') }),
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const font = new Uint8Array(await res.arrayBuffer());
        if (subs !== s) return;
        s.cjkFonts.push(font);
        if (s.renderer) {
            s.renderer.addFont(font);
            // The subset holds the Latin too: it becomes the default font, for every glyph after.
            s.renderer.setDefaultFont(CJK_FAMILY);
            redrawAss(s);
        }
    } catch (err) {
        fresh.forEach(ch => s.cjkSeen.delete(ch)); // tried again with the next window
        if (err.name !== 'AbortError') console.warn('CJK font failed (those lines show as boxes):', err.message);
    }
}

function redrawAss(s) {
    if (!s.renderer) return;
    s.renderer.timeOffset = player.offset - s.shift;
    if (videoPlayer.paused) s.renderer.setCurrentTime(true, videoPlayer.currentTime + s.renderer.timeOffset);
}

// Keeps the ASS canvas on the picture (inside any bars the video element has around it), as the
// player's size changes: JASSUB sizes the canvas to the picture, and this box puts it there.
function placeAssSubtitles(s) {
    if (subs !== s) return;
    const v = videoPlayer, box = document.getElementById('player-ass-box');
    const ratio = v.videoWidth && v.videoHeight ? v.videoWidth / v.videoHeight : 16 / 9;
    const width = Math.min(v.offsetWidth, v.offsetHeight * ratio), height = width / ratio;
    const place = `${v.offsetLeft + (v.offsetWidth - width) / 2}px,${v.offsetTop + (v.offsetHeight - height) / 2}px,${width}px,${height}px`;
    if (box.dataset.place !== place) {
        box.dataset.place = place;
        const [left, top, w, h] = place.split(',');
        Object.assign(box.style, { left, top, width: w, height: h });
    }
    if (s.renderer) s.renderer.timeOffset = player.offset - s.shift;
    requestAnimationFrame(() => placeAssSubtitles(s));
}

function loadSubtitleWindow(s) {
    const from = s.until;
    s.until = from + SUBTITLE_WINDOW;
    s.loading = true;
    const url = `${API_BASE}/api/media/subtitles?path=${encodeURIComponent(api.encodePath(player.item.path))}` +
        `&track=${s.index}&start=${from.toFixed(2)}&duration=${SUBTITLE_WINDOW}`;
    streamSubtitles(s, url)
        .catch(err => { if (err.name !== 'AbortError') console.warn('Subtitles failed:', err); })
        .finally(() => { s.loading = false; });
}

// As playback goes on: the next minutes of cues before they're needed.
function keepSubtitlesAhead(position) {
    const s = subs;
    if (!s || s.loading || position < s.until - 40) return;
    const duration = playerDuration();
    if (duration && s.until >= duration) return;
    if (s.sets) loadPictureWindow(s);
    else if (s.ass) loadAssWindow(s);
    else loadSubtitleWindow(s);
}

// Subtitle timing: the selected text track's correction, saved for every player. "Already said"
// (the line was spoken before its subtitle showed) moves subtitles earlier; "Not said yet", later.
function updateSyncControls() {
    const p = player, sub = p && !disc && subtitleTrack(p);
    const box = document.getElementById('player-sync');
    box.hidden = !(sub && sub.text);
    if (box.hidden) return;
    const ms = sub.offset_ms || 0;
    const amount = Math.abs(ms) >= 1000 ? `${(Math.abs(ms) / 1000).toFixed(2).replace(/\.?0+$/, '')} s` : `${Math.abs(ms)} ms`;
    document.getElementById('sync-value').textContent = ms === 0 ? 'In sync' : `${amount} ${ms < 0 ? 'earlier' : 'later'}`;
}

let syncSave = null;
function shiftSubtitles(direction) {
    const p = player, sub = p && subtitleTrack(p);
    if (!sub || !sub.text) return;
    const step = Number(document.getElementById('sync-step').value) || 250;
    const delta = direction * step;
    sub.offset_ms = (sub.offset_ms || 0) + delta;
    // The cues already loaded move by the change; ones loaded later come with it.
    if (subs && subs.index === sub.index) {
        subs.shift += delta / 1000;
        if (subs.ass) redrawAss(subs);
        for (const cue of [...((subs.track && subs.track.cues) || [])]) {
            cue.endTime = Math.max(0, cue.endTime + delta / 1000);
            cue.startTime = Math.max(0, cue.startTime + delta / 1000);
        }
    }
    updateSyncControls();
    clearTimeout(syncSave);
    const path = p.item.path, key = sub.key, ms = sub.offset_ms;
    syncSave = setTimeout(() => api.setSubtitleOffset(path, key, ms).catch(e => console.warn('Saving the timing failed:', e)), 600);
}

// After a seek within the stream: restarts the subtitles if the seek left the cues loaded.
function seekSubtitles(position) {
    if (subs && (position < subs.from || position > subs.until)) restartSubtitles(position);
}

async function streamSubtitles(s, url) {
    const headers = api.authHeaders();
    const res = await fetch(url, { signal: s.abort.signal, credentials: 'same-origin', headers });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf = (buf + decoder.decode(value, { stream: true })).replace(/\r/g, '');
        let end;
        while ((end = buf.indexOf('\n\n')) >= 0) {
            addSubtitleCue(s, buf.slice(0, end));
            buf = buf.slice(end + 2);
        }
    }
    addSubtitleCue(s, buf);
}

// One WebVTT block: "[id]\nstart --> end [settings]\ntext…".
function addSubtitleCue(s, block) {
    if (subs !== s) return;
    const lines = block.split('\n');
    const at = lines.findIndex(line => line.includes('-->'));
    if (at < 0) return;
    const [a, b] = lines[at].split('-->');
    const text = lines.slice(at + 1).join('\n').trim();
    // Cue times are the file's; the stream's time 0 is s.offset in the file.
    const start = vttSeconds(a) - s.offset + s.shift, end = vttSeconds(b.trim().split(/\s+/)[0]) - s.offset + s.shift;
    if (!text || !(end > 0)) return;
    // A window starts at a keyframe before from: cues there came with the window before too.
    if ([...(s.track.cues || [])].some(c => c.text === text && Math.abs(c.startTime - start) < 0.05)) return;
    const cue = new VTTCue(Math.max(0, start), end, text);
    // Up off the bottom edge: the cue's last line ends 10% of the picture's height above it.
    cue.snapToLines = false;
    cue.line = SUBTITLE_LINE;
    cue.lineAlign = 'end';
    s.track.addCue(cue);
}

function vttSeconds(text) {
    return text.trim().split(':').reduce((total, part) => total * 60 + Number(part), 0);
}

// The player tells the server where it is, for the admin Streams page and the user's progress:
// every few seconds while playing, right away on play, pause and the end, and on closing.
let playbackReportTimer = null;
let playbackReport = null; // sends the current position: (ended) => void

function startPlaybackReports(item) {
    clearInterval(playbackReportTimer);
    let started = false; // until playback starts, the position is still the resume point's
    const send = (ended = false) => {
        if (!playerModal.classList.contains('active')) {
            clearInterval(playbackReportTimer);
            return;
        }
        if (!started && !ended) return;
        const extra = { ended };
        if (disc && disc.info) extra.title = disc.title === disc.info.main_title ? 0 : disc.title;
        api.reportPlayback(item.path, playerPosition(), videoPlayer.paused, extra).catch(() => {});
    };
    playbackReport = send;
    playbackReportTimer = setInterval(() => send(), 5000);
    videoPlayer.onplay = () => { started = true; send(); };
    videoPlayer.onpause = () => send();
    videoPlayer.onended = () => {
        send(true);
        if (player || disc) playNextAfterEnd(player || disc);
    };
}

// Sends the last position as the player closes, then shows the new progress.
function finishPlaybackReports() {
    if (!playerModal.classList.contains('active') || !playbackReport) return;
    playbackReport(videoPlayer.ended);
    playbackReport = null;
    clearInterval(playbackReportTimer);
    setTimeout(refreshCurrentView, 300);
}

// Where playback is in the file: discs and transcodes start their stream at an offset.
function playerPosition() {
    const offset = disc && disc.item ? disc.offset : player ? player.offset : 0;
    return offset + (videoPlayer.currentTime || 0);
}

// ---------------------------------------------------------------------------------------------
// DVD disc images: always transcoded by the server, which plays one title of the disc at a time.
// Choosing a title, chapter, angle or audio track restarts the stream at the right place; the
// subtitles are drawn over it (restartSubtitles), so choosing them doesn't.

let disc = null; // { item, info, title, angle, audio, subtitle, offset, fromFolder }

// A DVD: an image (.iso), or an unpacked disc's VIDEO_TS folder (listed by its parent folder's name)
function isDiscImage(item) {
    return /(\.iso|(^|\/)VIDEO_TS)$/i.test(item.path || item.name || '');
}

// ---------------------------------------------------------------------------------------------
// Skip Intro / Skip Credits: the server finds each video's intro and credits (chapters, a
// season's shared audio, dark frames) or an admin marks them. The button shows for the first 10
// seconds of either (and while fullscreen controls show during it); with the show's Auto Skip on,
// the player skips by itself. Skipping credits goes on to the next episode, else closes.

const SKIP_SHOWN_MS = 10000;

async function loadSegments(p) {
    try {
        const segments = await api.getSegments(p.item.path);
        if (player === p) {
            p.segments = segments;
            showPlayerTitle(p, segments.metadata);
        }
    } catch (e) { /* no Skip button */ }
    const mark = document.getElementById('player-mark-label');
    if (mark) mark.hidden = !canFixMatches();
}

function updateSkipButton() {
    const p = player, btn = document.getElementById('player-skip');
    const s = p && p.segments;
    const t = s ? playerPosition() : 0;
    const kind = !s ? null : ['intro', 'credits'].find(k => s[k] && t >= s[k].start && t < s[k].end - 0.5);
    if (!kind) {
        btn.hidden = true;
        if (p) p.skipFor = null;
        return;
    }
    const seg = s[kind];
    const key = `${kind}:${seg.start}`;
    if (p.skipFor !== key) { // just entered it
        p.skipFor = key;
        p.skipShownAt = Date.now();
        p.autoSkipped = p.autoSkipped || {};
        if (s.auto_skip && !p.autoSkipped[key]) {
            p.autoSkipped[key] = true;
            skipSegment(kind);
            return;
        }
    }
    const box = playerModal.querySelector('.modal-content');
    const controlsShown = document.fullscreenElement === box && !box.classList.contains('controls-hidden');
    btn.textContent = kind === 'intro' ? 'Skip Intro' : 'Skip Credits';
    btn.hidden = !(Date.now() - p.skipShownAt < SKIP_SHOWN_MS || controlsShown);
    btn.onclick = (e) => { e.stopPropagation(); skipSegment(kind); };
}

function skipSegment(kind) {
    const p = player;
    if (!p || !p.segments) return;
    document.getElementById('player-skip').hidden = true;
    if (kind === 'intro') {
        seekTo(p.segments.intro.end);
        return;
    }
    // Credits: this one's watched; on to the next episode, or done.
    api.setWatched([p.item.path], true).catch(() => {});
    const next = p.segments.next;
    if (next) playVideo(next, false, resumePosition(next));
    else playerModal.querySelector('.close').click();
}

// Admins: the Mark menu sets the intro and credits at the current position.
async function markSegment(action) {
    const p = player;
    if (!p) return;
    const t = playerPosition();
    const s = p.segments || {};
    try {
        let result;
        if (action === 'intro-start') {
            p.introStart = t; // saved with "Intro ends here"
            document.getElementById('player-status').textContent = `Intro starts at ${clock(t)}; now mark where it ends.`;
            return;
        } else if (action === 'intro-end') {
            const start = p.introStart ?? (s.intro ? s.intro.start : 0);
            result = await api.setSegment(p.item.path, 'intro', start, t);
        } else if (action === 'credits-start') {
            result = await api.setSegment(p.item.path, 'credits', t, Math.max(p.duration || 0, t + 1));
        } else if (action === 'clear-intro' || action === 'clear-credits') {
            result = await api.setSegment(p.item.path, action.slice(6), 0, 0, true);
        }
        if (result && player === p) p.segments = result;
        document.getElementById('player-status').textContent = 'Saved.';
    } catch (e) {
        document.getElementById('player-status').textContent = 'Could not save: ' + e.message;
    }
}

// Stops what's playing and its subtitles, and leaves fullscreen.
function resetPlayer() {
    videoPlayer.pause();
    disc = null;
    player = null;
    stopSubtitles();
    hidePreview();
    document.getElementById('player-controls').hidden = true;
    document.getElementById('player-status').textContent = '';
    document.getElementById('player-skip').hidden = true;
    document.getElementById('player-sync').hidden = true;
    videoPlayer.onclick = videoPlayer.ondblclick = null;
    videoPlayer.onplay = videoPlayer.onpause = videoPlayer.onvolumechange = null;
    videoPlayer.ontimeupdate = videoPlayer.onerror = videoPlayer.onloadedmetadata = null;
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
}

async function playDisc(item, startAt = 0, fromFolder = false) {
    stopThemeMusic(800);
    disc = { item, info: null, title: 0, angle: 1, audio: 0, subtitle: -1, offset: 0, fromFolder };
    openPlayer(item.name + ' — reading disc…');
    document.getElementById('player-controls').hidden = true;
    let info;
    try {
        info = await api.getDisc(item.path);
    } catch (error) {
        playerTitle.textContent = `${item.name} — ${error.message}`;
        return;
    }
    if (!disc || disc.item !== item) return; // closed or replaced meanwhile
    disc.info = info;
    disc.title = info.main_title;
    loadTrickplay(disc);
    playerTitle.textContent = item.name;
    if (!fromFolder) {
        const now = disc;
        api.getSegments(item.path).then(segments => {
            if (disc === now) showPlayerTitle(now, segments.metadata);
        }).catch(() => {});
    }
    renderDiscControls();
    loadDiscStream(startAt); // progress is kept for the main title, which plays first
}

function discTitle() {
    return disc.info.titles.find(t => t.number === disc.title) || disc.info.titles[0];
}

function loadDiscStream(start) {
    const t = discTitle();
    disc.offset = Math.max(0, Math.min(start, Math.max(t.duration - 1, 0)));
    let url = api.getStreamUrl(disc.item.path, true) +
        `&title=${t.number}&angle=${disc.angle}&audio=${disc.audio}`;
    if (disc.offset > 0) url += `&start=${disc.offset.toFixed(2)}`;
    attachStream(url, disc.item);
    restartSubtitles(disc.offset); // read as this stream reads the disc, from where it starts
    updatePlayerPosition();
}

function fillSelect(select, options, selected) {
    select.innerHTML = '';
    options.forEach(([value, label]) => {
        const opt = document.createElement('option');
        opt.value = value;
        opt.textContent = label;
        select.appendChild(opt);
    });
    select.value = String(selected);
}

function trackLabel(track) {
    const parts = [track.language || 'unknown', track.codec];
    if (track.channels) parts.push(track.channels === 6 ? '5.1' : track.channels === 2 ? 'stereo' : `${track.channels} ch`);
    return `${track.index + 1}: ${parts.join(' · ')}`;
}

function renderDiscControls() {
    const info = disc.info, t = discTitle();
    document.getElementById('player-stream-label').hidden = true;
    document.querySelectorAll('.disc-only').forEach(el => { el.hidden = false; });
    const titles = [...info.titles].sort((a, b) => a.number - b.number);
    fillSelect(document.getElementById('disc-title'), titles.map(x =>
        [x.number, `${x.number}: ${clock(x.duration)}${x.number === info.main_title ? ' (main)' : ''}`]), t.number);
    const chapters = t.chapters.length ? t.chapters : [0];
    fillSelect(document.getElementById('disc-chapter'), chapters.map((start, i) =>
        [i, `${i + 1}: ${clock(start)}`]), 0);
    document.getElementById('disc-angle-label').hidden = t.angles <= 1;
    fillSelect(document.getElementById('disc-angle'),
        Array.from({ length: t.angles }, (_, i) => [i + 1, `${i + 1} of ${t.angles}`]), disc.angle);
    document.getElementById('player-audio-label').hidden = false;
    fillSelect(document.getElementById('player-audio'),
        t.audio.length ? t.audio.map(a => [a.index, trackLabel(a)]) : [[0, 'None']], disc.audio);
    document.getElementById('player-subtitle-label').hidden = false;
    fillSelect(document.getElementById('player-subtitle'),
        [[-1, 'Off'], ...t.subtitles.map(s => [s.index, trackLabel(s)])], disc.subtitle);
    document.getElementById('player-controls').hidden = false;
}

function updatePlayerPosition() {
    if (!disc && !player) return;
    if (disc && !disc.info) return;
    const pos = playerPosition(), duration = playerDuration();
    const seek = document.getElementById('player-seek');
    seek.max = String(Math.max(1, Math.floor(duration)));
    if (!seekDragging) seek.value = String(Math.floor(pos));
    document.getElementById('player-position').textContent =
        `${clock(seekDragging ? Number(seek.value) : pos)} / ${clock(duration)}`;
    if (!disc) return;
    // Keep the chapter list on the chapter playing.
    let current = 0;
    discTitle().chapters.forEach((start, i) => { if (pos + 0.5 >= start) current = i; });
    const select = document.getElementById('disc-chapter');
    if (document.activeElement !== select && select.value !== String(current)) select.value = String(current);
}

let seekDragging = false;

// Material icons (Apache License 2.0), inlined so the page loads no icon font.
const MATERIAL_ICONS = {
    // Material Symbols' feature_search, in its 960-unit grid (see iconSvg)
    checkCircle: 'M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z',
    edit: 'M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a.996.996 0 0 0 0-1.41l-2.34-2.34a.996.996 0 0 0-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z',
    featureSearch: { viewBox: '0 -960 960 960', d: 'm772-604 128 128-56 56-128-128q-21 12-45 20t-51 8q-75 0-127.5-52.5T440-700q0-75 52.5-127.5T620-880q75 0 127.5 52.5T800-700q0 27-8 51t-20 45Zm-81-25q29-29 29-71t-29-71q-29-29-71-29t-71 29q-29 29-29 71t29 71q29 29 71 29t71-29ZM160-80q-33 0-56.5-23.5T80-160v-560q0-33 23.5-56.5T160-800h220q-11 25-15 51.5t-4 52.5q0 109 77 182t183 73q19 0 38-2.5t39-8.5l102 102v190q0 33-23.5 56.5T720-80H160Z' },
    subtitles: 'M20 4H4c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V6c0-1.1-.9-2-2-2zM4 12h4v2H4v-2zm10 6H4v-2h10v2zm6 0h-4v-2h4v2zm0-4H10v-2h10v2z',
    play: 'M8 5v14l11-7z',
    pause: 'M6 19h4V5H6v14zm8-14v14h4V5h-4z',
    volumeUp: 'M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z',
    volumeOff: 'M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51C20.63 14.91 21 13.5 21 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06c1.38-.31 2.63-.95 3.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z',
    fullscreen: 'M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z',
    fullscreenExit: 'M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z',
    replay: 'M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z',
    settings: 'M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.57-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.21-.08.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z',
    liveTv: 'M21 6h-7.59l3.29-3.29L16 2l-4 4-4-4-.71.71L10.59 6H3c-1.1 0-2 .89-2 2v12c0 1.1.9 2 2 2h18c1.1 0 2-.9 2-2V8c0-1.11-.9-2-2-2zm0 14H3V8h18v12zM9 10v8l7-4z',
    refresh: 'M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z',
    save: 'M17 3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V7l-4-4zm-5 16c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3zm3-10H5V5h10v4z',
    palette: 'M12 3c-4.97 0-9 4.03-9 9s4.03 9 9 9c.83 0 1.5-.67 1.5-1.5 0-.39-.15-.74-.39-1.01-.23-.26-.38-.61-.38-.99 0-.83.67-1.5 1.5-1.5H16c2.76 0 5-2.24 5-5 0-4.42-4.03-8-9-8zm-5.5 9c-.83 0-1.5-.67-1.5-1.5S5.67 9 6.5 9 8 9.67 8 10.5 7.33 12 6.5 12zm3-4C8.67 8 8 7.33 8 6.5S8.67 5 9.5 5s1.5.67 1.5 1.5S10.33 8 9.5 8zm5 0c-.83 0-1.5-.67-1.5-1.5S13.67 5 14.5 5s1.5.67 1.5 1.5S15.33 8 14.5 8zm3 4c-.83 0-1.5-.67-1.5-1.5S16.67 9 17.5 9s1.5.67 1.5 1.5-.67 1.5-1.5 1.5z',
    key: 'M12.65 10C11.83 7.67 9.61 6 7 6c-3.31 0-6 2.69-6 6s2.69 6 6 6c2.61 0 4.83-1.67 5.65-4H17v4h4v-4h2v-4H12.65zM7 14c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z',
    logout: 'M17 7l-1.41 1.41L18.17 11H8v2h10.17l-2.58 2.58L17 17l5-5zM4 5h8V3H4c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h8v-2H4V5z',
    delete: 'M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z',
    visibility: 'M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z',
    visibilityOff: 'M12 7c2.76 0 5 2.24 5 5 0 .65-.13 1.26-.36 1.83l2.92 2.92c1.51-1.26 2.7-2.89 3.43-4.75-1.73-4.39-6-7.5-11-7.5-1.4 0-2.74.25-3.98.7l2.16 2.16C10.74 7.13 11.35 7 12 7zM2 4.27l2.28 2.28.46.46C3.08 8.3 1.78 10.02 1 12c1.73 4.39 6 7.5 11 7.5 1.55 0 3.03-.3 4.38-.84l.42.42L19.73 22 21 20.73 3.27 3 2 4.27zM7.53 9.8l1.55 1.55c-.05.21-.08.43-.08.65 0 1.66 1.34 3 3 3 .22 0 .44-.03.65-.08l1.55 1.55c-.67.33-1.41.53-2.2.53-2.76 0-5-2.24-5-5 0-.79.2-1.53.53-2.2zm4.31-.78l3.15 3.15.02-.16c0-1.66-1.34-3-3-3l-.17.01z',
    personAdd: 'M15 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm-9-2V7H4v3H1v2h3v3h2v-3h3v-2H6zm9 4c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z',
    arrowUpward: 'M4 12l1.41 1.41L11 7.83V20h2V7.83l5.58 5.59L20 12l-8-8-8 8z',
    folder: 'M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z',
    movie: 'M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4h-4z',
    image: 'M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z',
    description: 'M14 2H6c-1.1 0-1.99.9-1.99 2L4 20c0 1.1.89 2 1.99 2H18c1.1 0 2-.9 2-2V8l-6-6zm2 16H8v-2h8v2zm0-4H8v-2h8v2zm-3-5V3.5L18.5 9H13z',
    tv: 'M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h5v2h8v-2h5c1.1 0 1.99-.9 1.99-2L23 5c0-1.1-.9-2-2-2zm0 14H3V5h18v12z',
    theaters: 'M18 3v2h-2V3H8v2H6V3H4v18h2v-2h2v2h8v-2h2v2h2V3h-2zM8 17H6v-2h2v2zm0-4H6v-2h2v2zm0-4H6V7h2v2zm10 8h-2v-2h2v2zm0-4h-2v-2h2v2zm0-4h-2V7h2v2z',
    check: 'M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z',
    chevronLeft: 'M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z',
    chevronRight: 'M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z',
    close: 'M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z',
    android: 'M17.6 9.48l1.84-3.18c.16-.31.04-.69-.26-.85-.29-.15-.65-.06-.83.22l-1.88 3.24c-2.86-1.21-6.08-1.21-8.94 0L5.65 5.67c-.19-.29-.58-.38-.87-.2-.28.18-.37.54-.22.83L6.4 9.48C3.3 11.25 1.28 14.44 1 18h22c-.28-3.56-2.3-6.75-5.4-8.52zM7 15.25c-.69 0-1.25-.56-1.25-1.25s.56-1.25 1.25-1.25 1.25.56 1.25 1.25-.56 1.25-1.25 1.25zm10 0c-.69 0-1.25-.56-1.25-1.25s.56-1.25 1.25-1.25 1.25.56 1.25 1.25-.56 1.25-1.25 1.25z',
    search: 'M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z',
};

// Links to the Android app's release APK, which Android/deploy-release.sh --web publishes to
// web/downloads with WeirdStreamer.json ({ version, build, size, built }): shown, with its
// version, build number, size and date, once it's there. Only for those signed in (Settings).
async function setupAppDownloads() {
    try {
        let app = null;
        const meta = await fetch('downloads/WeirdStreamer.json', { cache: 'no-cache' });
        if (meta.ok) {
            app = await meta.json();
        } else {
            // Published without the version file: the APK's own size and date
            const res = await fetch('downloads/WeirdStreamer.apk', { method: 'HEAD', cache: 'no-cache' });
            if (!res.ok) return;
            app = { size: Number(res.headers.get('Content-Length')), built: res.headers.get('Last-Modified') };
        }
        const date = new Date(app.built || '');
        const info = [
            app.version ? `Version ${app.version}` : '', // 0.0.1.254: the build number is its last part
            app.size ? formatSize(app.size) : '',
            isNaN(date) ? '' : date.toLocaleDateString(),
        ].filter(Boolean).join(' · ');
        document.querySelectorAll('.app-download, .app-download-group').forEach(el => { el.hidden = false; });
        document.querySelectorAll('.app-download').forEach(link => {
            link.querySelector('.app-download-info').textContent = info;
            if (!link.querySelector('svg')) link.insertAdjacentHTML('afterbegin', iconSvg('android'));
        });
    } catch (e) {
        // no app published
    }
}

// Buttons and headings with a data-icon get that Material icon in front of their label.
function decorateIconButtons() {
    document.querySelectorAll('button[data-icon], .icon-heading[data-icon]').forEach(btn => {
        if (btn.querySelector('svg') || !MATERIAL_ICONS[btn.dataset.icon]) return;
        btn.insertAdjacentHTML('afterbegin', iconSvg(btn.dataset.icon));
    });
}

// Eye buttons that show or hide a password-type field's value.
function setupSecretToggles() {
    document.querySelectorAll('.secret-toggle').forEach(btn => {
        const input = document.getElementById(btn.dataset.for);
        const render = () => {
            const shown = input.type === 'text';
            btn.innerHTML = iconSvg(shown ? 'visibilityOff' : 'visibility');
            btn.title = shown ? 'Hide token' : 'Show token';
            btn.setAttribute('aria-label', btn.title);
            btn.setAttribute('aria-pressed', String(shown));
        };
        btn.addEventListener('click', () => {
            input.type = input.type === 'password' ? 'text' : 'password';
            render();
        });
        render();
    });
}

function setButtonLabel(btn, text) {
    const label = btn.querySelector('.btn-label');
    if (label) label.textContent = text;
    else if (btn.querySelector('svg')) { btn.title = text; btn.setAttribute('aria-label', text); } // icon only: its tooltip
    else btn.textContent = text;
}

function iconSvg(name, { mirrored = false } = {}) {
    const icon = MATERIAL_ICONS[name];
    // Most are Material Icons paths on a 24-unit grid; a few are Material Symbols, with their own.
    const { viewBox, d } = typeof icon === 'string' ? { viewBox: '0 0 24 24', d: icon } : icon;
    const flip = mirrored ? ' transform="matrix(-1 0 0 1 24 0)"' : '';
    return `<svg viewBox="${viewBox}" width="20" height="20" aria-hidden="true"><path fill="currentColor"${flip} d="${d}"/></svg>`;
}

function setIcon(id, name, label) {
    const btn = document.getElementById(id);
    if (btn.dataset.icon === name) return;
    btn.dataset.icon = name;
    btn.innerHTML = iconSvg(name) + (label ? `<span>${label}</span>` : '');
}

function togglePlay() {
    if (videoPlayer.paused) resumePlayback();
    else videoPlayer.pause();
}

// Settings › My Account › Playback, "Rewind 5 seconds when resuming" (this browser's; off by
// default): playing a paused video goes back up to 5 seconds first, as on the apps.
const RESUME_REWIND_S = 5;

function rewindOnResume() {
    try { return localStorage.getItem('rewindOnResume') === 'true'; } catch (e) { return false; }
}

function setupPlaybackSettings() {
    const box = document.getElementById('rewind-on-resume');
    box.checked = rewindOnResume();
    box.onchange = () => {
        try { localStorage.setItem('rewindOnResume', String(box.checked)); } catch (e) { /* not kept */ }
    };
    const theme = document.getElementById('theme-music');
    theme.checked = themeMusicOn();
    theme.onchange = () => {
        try { localStorage.setItem('themeMusic', String(theme.checked)); } catch (e) { /* not kept */ }
        if (!theme.checked) stopThemeMusic();
        volume.disabled = !theme.checked;
    };
    const volume = document.getElementById('theme-volume');
    volume.value = themeVolumeLevel();
    volume.disabled = !theme.checked;
    volume.onchange = () => {
        try { localStorage.setItem('themeVolume', volume.value); } catch (e) { /* not kept */ }
        if (themeAudio) themeAudio.volume = themeVolume();
    };
}

// Theme music on title pages (Settings › My Account › Playback, this browser's; on by default, at
// Low, Medium or High): a
// show's or movie's theme (GET /api/library/theme) plays once, quietly, while its page is open, and
// fades out when the page closes or a video plays. A browser may keep it silent until the page has
// been clicked (a title opened from a link).
const THEME_VOLUMES = { low: 0.15, medium: 0.3, high: 0.5 };
let themeAudio = null;

function themeVolumeLevel() {
    let level = null;
    try { level = localStorage.getItem('themeVolume'); } catch (e) { /* default */ }
    return THEME_VOLUMES[level] ? level : 'medium';
}

function themeVolume() {
    return THEME_VOLUMES[themeVolumeLevel()];
}

function themeMusicOn() {
    try { return localStorage.getItem('themeMusic') !== 'false'; } catch (e) { return true; }
}

function playThemeMusic(libId, kind, id) {
    stopThemeMusic(0);
    // Not over a video that's playing
    if (!themeMusicOn() || document.getElementById('player-modal')?.classList.contains('active')) return;
    const audio = new Audio(`${API_BASE}/api/library/theme?lib=${encodeURIComponent(libId)}&kind=${kind}&id=${id}`);
    audio.volume = themeVolume();
    audio.dataset.title = `${libId}/${kind}/${id}`;
    themeAudio = audio;
    audio.addEventListener('ended', () => { if (themeAudio === audio) themeAudio = null; });
    audio.play().catch(() => { /* not allowed yet, or none: stays quiet */ });
}

// Fades the theme out over fadeMs (0 stops it at once).
function stopThemeMusic(fadeMs = 1500) {
    const audio = themeAudio;
    themeAudio = null;
    if (!audio) return;
    if (!fadeMs || audio.paused) {
        audio.pause();
        return;
    }
    const from = audio.volume, start = performance.now();
    const step = (now) => {
        const left = 1 - (now - start) / fadeMs;
        if (left <= 0) { audio.pause(); return; }
        audio.volume = from * left;
        requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
}

// Settings › My Account › Language: the account's languages for audio and for subtitles, and
// whether subtitles start on (kept on the server, for every device). The server picks the tracks
// they mean (the codecs report's preferred_audio and preferred_subtitle); "File default" is as the
// file says.
async function setupLanguageSetting() {
    const audio = document.getElementById('account-audio-language');
    const subtitle = document.getElementById('account-subtitle-language');
    const on = document.getElementById('account-subtitles-on');
    let account;
    try { account = await api.getAccount(); } catch (e) { return; }
    let names = null;
    try { names = new Intl.DisplayNames([navigator.language], { type: 'language' }); } catch (e) { /* codes */ }
    const name = code => (names && names.of(code)) || code;
    const options = account.languages.map(code => [code, name(code)]).sort((a, b) => a[1].localeCompare(b[1]));
    for (const select of [audio, subtitle]) {
        select.replaceChildren(select.options[0]);
        options.forEach(([code, label]) => select.add(new Option(label, code)));
    }
    audio.value = account.audio_language || '';
    subtitle.value = account.subtitle_language || '';
    on.checked = !!account.subtitles_on;
    audio.onchange = subtitle.onchange = on.onchange = async () => {
        const next = { audio_language: audio.value, subtitle_language: subtitle.value, subtitles_on: on.checked };
        try { await api.setLanguages(next); } catch (e) {
            alert('Could not save the setting: ' + e.message);
            audio.value = account.audio_language || '';
            subtitle.value = account.subtitle_language || '';
            on.checked = !!account.subtitles_on;
            return;
        }
        Object.assign(account, next);
    };
}

function resumePlayback() {
    if (rewindOnResume() && (player || disc) && !videoPlayer.ended) {
        const at = playerPosition();
        const back = Math.min(RESUME_REWIND_S, Math.floor(at));
        if (back > 0) {
            seekTo(at - back);
            showSeekIndicator(-back, at - back);
        }
    }
    videoPlayer.play().catch(() => {});
}

// A seek's "−0:05 / 0:41" over the picture for a moment: the jump, then where it lands.
let seekIndicatorTimer = null;
function showSeekIndicator(delta, target) {
    const el = document.getElementById('player-seek-indicator');
    el.textContent = `${delta < 0 ? '−' : '+'}${clock(Math.abs(delta))} / ${clock(target)}`;
    el.hidden = false;
    clearTimeout(seekIndicatorTimer);
    seekIndicatorTimer = setTimeout(() => { el.hidden = true; }, 1500);
}

function updatePlayerButtons() {
    setIcon('player-play', videoPlayer.paused ? 'play' : 'pause');
    setIcon('player-mute', videoPlayer.muted || videoPlayer.volume === 0 ? 'volumeOff' : 'volumeUp');
    setIcon('player-fullscreen', document.fullscreenElement ? 'fullscreenExit' : 'fullscreen');
    document.getElementById('player-volume').value = String(videoPlayer.muted ? 0 : videoPlayer.volume);
}

// In fullscreen the controls and cursor hide after a moment without mouse or keyboard use.
const PLAYER_IDLE_MS = 2500;
let playerIdleTimer = null;

function playerActivity() {
    const box = playerModal.querySelector('.modal-content');
    box.classList.remove('controls-hidden');
    clearTimeout(playerIdleTimer);
    if (!(disc || player) || document.fullscreenElement !== box) return;
    playerIdleTimer = setTimeout(function hide() {
        if (seekDragging) { playerIdleTimer = setTimeout(hide, PLAYER_IDLE_MS); return; }
        box.classList.add('controls-hidden');
    }, PLAYER_IDLE_MS);
}

function togglePlayerFullscreen() {
    // The video and the controls together, so the seek bar stays usable.
    const box = playerModal.querySelector('.modal-content');
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    else box.requestFullscreen().catch(() => {});
}

// The trickplay frame nearest t seconds, for the picture over the seek bar.
function trickplayFrame(t) {
    // A disc's frames show only while the title they were made from plays.
    const from = player || (disc && disc.framesTitle === disc.title ? disc : null);
    const frames = from && from.frames;
    if (!frames || !frames.length) return null;
    let best = frames[0];
    for (const f of frames) {
        if (f.time > t) break;
        best = f;
    }
    return best;
}

// Shows what's at t over the seek bar at x (pixels from its left): a trickplay frame, and the time.
function showPreview(t, x) {
    const preview = document.getElementById('player-preview');
    const img = preview.querySelector('img');
    const frame = trickplayFrame(t);
    img.hidden = !frame;
    if (frame) {
        const src = `${API_BASE}${frame.url}`;
        if (img.getAttribute('src') !== src) img.src = src;
    }
    preview.querySelector('span').textContent = clock(t);
    preview.hidden = false;
    const bar = document.getElementById('player-seek');
    const half = preview.offsetWidth / 2;
    preview.style.left = `${bar.offsetLeft + Math.min(Math.max(x, half), bar.offsetWidth - half) - half}px`;
}

function hidePreview() {
    document.getElementById('player-preview').hidden = true;
}

function setupPlayerControls() {
    document.getElementById('subtitle-search-close').onclick = closeSubtitleSearch;
    document.getElementById('subtitle-search-form').onsubmit = (e) => { e.preventDefault(); runSubtitleSearch(); };
    // Circular arrows: back (counterclockwise) for -10s, mirrored forward for +30s.
    document.getElementById('player-back').innerHTML = iconSvg('replay') + '<span>−10s</span>';
    document.getElementById('player-forward').innerHTML = iconSvg('replay', { mirrored: true }) + '<span>+30s</span>';
    const mark = document.getElementById('player-mark');
    mark.onchange = () => { const action = mark.value; mark.value = ''; markSegment(action); };
    document.addEventListener('fullscreenchange', () => {
        if (disc || player) updatePlayerButtons();
        playerActivity();
    });
    const box = playerModal.querySelector('.modal-content');
    ['mousemove', 'mousedown', 'wheel', 'touchstart'].forEach(type =>
        box.addEventListener(type, playerActivity, { passive: true }));
    document.addEventListener('keydown', () => { if (document.fullscreenElement === box) playerActivity(); }, true);
    updatePlayerButtons();
    const seek = document.getElementById('player-seek');
    const seekX = () => (Number(seek.value) / Math.max(1, Number(seek.max))) * seek.offsetWidth;
    seek.addEventListener('input', () => {
        seekDragging = true;
        updatePlayerPosition();
        showPreview(Number(seek.value), seekX());
    });
    seek.addEventListener('change', () => {
        seekDragging = false;
        hidePreview();
        seekTo(Number(seek.value));
    });
    seek.addEventListener('mousemove', (e) => {
        if (seekDragging) return;
        const x = e.clientX - seek.getBoundingClientRect().left;
        showPreview(Math.max(0, x / seek.offsetWidth) * playerDuration(), x);
    });
    seek.addEventListener('mouseleave', () => { if (!seekDragging) hidePreview(); });
    document.getElementById('player-back').addEventListener('click', () => seekTo(playerPosition() - 10));
    document.getElementById('player-forward').addEventListener('click', () => seekTo(playerPosition() + 30));
    document.getElementById('player-play').addEventListener('click', togglePlay);
    document.getElementById('player-mute').addEventListener('click', () => {
        videoPlayer.muted = !videoPlayer.muted;
        if (!videoPlayer.muted && videoPlayer.volume === 0) videoPlayer.volume = 1;
    });
    document.getElementById('player-volume').addEventListener('input', (e) => {
        videoPlayer.volume = Number(e.target.value);
        videoPlayer.muted = videoPlayer.volume === 0;
    });
    document.getElementById('player-fullscreen').addEventListener('click', togglePlayerFullscreen);
    document.addEventListener('keydown', (e) => {
        if (!(player || (disc && disc.info)) || !playerModal.classList.contains('active')) return;
        // Sliders keep their arrow keys, lists and buttons their own keys (Space clicks a button).
        const target = e.target instanceof Element ? e.target : document.body;
        if (target.matches('input, select, textarea') && e.key !== ' ') return;
        if (target.matches('select, textarea, button')) return;
        const key = e.key.toLowerCase();
        if (key === ' ' || key === 'k') togglePlay();
        else if (key === 'arrowleft') seekTo(playerPosition() - 10);
        else if (key === 'arrowright') seekTo(playerPosition() + 30);
        else if (key === 'm') videoPlayer.muted = !videoPlayer.muted;
        else if (key === 'f') togglePlayerFullscreen();
        else return;
        e.preventDefault();
    });
    const on = (id, handler) => document.getElementById(id).addEventListener('change', (e) => handler(e.target.value));
    on('player-stream', (mode) => changeFileStream(p => {
        p.mode = mode;
        p.encode = p.failed = false;
        p.audioStep = 0;
        // The original as it is: no burned-in subtitles (its audio is the browser's to play).
        if (mode === 'original' && burnsInSubtitles(p)) p.subtitle = -1;
    }));
    on('player-audio', (value) => {
        const a = Number(value);
        if (disc && disc.info) { disc.audio = a; loadDiscStream(playerPosition()); return; }
        changeFileStream(p => { p.audio = a; p.audioStep = 0; chooseCompatibility(p); });
    });
    document.getElementById('sync-earlier').onclick = () => shiftSubtitles(-1);
    document.getElementById('sync-later').onclick = () => shiftSubtitles(1);
    on('player-subtitle', (value) => {
        if (value === 'search') {
            document.getElementById('player-subtitle').value = String(player ? player.subtitle : -1);
            openSubtitleSearch();
            return;
        }
        const s = Number(value);
        if (disc && disc.info) { disc.subtitle = s; restartSubtitles(playerPosition()); return; }
        changeFileStream(p => {
            p.subtitle = s;
            chooseCompatibility(p);
        });
    });
    const onDisc = (id, handler) => on(id, (value) => { if (disc && disc.info) handler(Number(value)); });
    onDisc('disc-title', (n) => {
        disc.title = n;
        disc.angle = 1;
        disc.audio = 0;
        disc.subtitle = -1;
        renderDiscControls();
        loadDiscStream(0);
    });
    onDisc('disc-chapter', (i) => seekTo(discTitle().chapters[i] || 0));
    onDisc('disc-angle', (a) => { disc.angle = a; loadDiscStream(playerPosition()); });
}

function viewImage(item) {
    const streamUrl = api.getStreamUrl(item.path);
    imageTitle.textContent = item.name;
    stopLivePhoto();
    imageViewer.src = '';
    imageViewer.alt = item.name;
    imageViewer.onload = () => {
        imageModal.classList.add('active');
        if (item.live_video) startLivePhoto(item.live_video);
    };
    imageViewer.onerror = () => {
        console.error('Failed to load image:', item.path);
        alert('Failed to load image');
    };
    imageViewer.src = streamUrl;
}

// Apple Live Photos: the server pairs a photo with the .MOV of the same name (live_video). Its
// video plays once over the still, muted, when the photo opens, and LIVE plays it again with sound.
// It comes as Compatibility (Auto), since an iPhone's HEVC doesn't play in every browser.
function startLivePhoto(path) {
    livePhotoButton.hidden = false;
    livePhotoVideo.src = api.getStreamUrl(path, 'audio');
    playLivePhoto(true);
}

function playLivePhoto(muted) {
    if (!livePhotoVideo.src) return;
    livePhotoVideo.muted = muted;
    livePhotoVideo.currentTime = 0;
    livePhotoVideo.hidden = false;
    livePhotoVideo.play().catch(() => { livePhotoVideo.hidden = true; });
}

function stopLivePhoto() {
    livePhotoButton.hidden = true;
    livePhotoVideo.hidden = true;
    livePhotoVideo.pause();
    livePhotoVideo.removeAttribute('src');
    livePhotoVideo.load(); // ends the request
}

livePhotoVideo.addEventListener('ended', () => { livePhotoVideo.hidden = true; });
livePhotoVideo.addEventListener('error', () => { livePhotoVideo.hidden = true; });
livePhotoButton.addEventListener('click', () => playLivePhoto(false));

// A large Material icon (HTML) standing in for a missing thumbnail.
function getMediaIcon(type) {
    const icons = { video: 'movie', image: 'image', document: 'description' };
    return placeholderIcon(icons[type] || 'folder');
}

function placeholderIcon(name) {
    return iconSvg(name).replace('<svg ', '<svg class="placeholder-icon" ');
}

function formatSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
}

function formatDuration(seconds) {
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    
    if (h > 0) {
        return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
    }
    return `${m}:${s.toString().padStart(2, '0')}`;
}

// Settings Functions
// Settings' Server & Media tab, at Media Paths: where a server with no libraries gets its first one.
async function openLibrarySettings() {
    await showSettings();
    document.querySelectorAll('.settings-tab').forEach(t => t.classList.toggle('active', t.dataset.tab === 'server'));
    document.querySelectorAll('.settings-tab-content').forEach(c => c.classList.toggle('active', c.id === 'server-tab'));
    document.getElementById('no-libraries-hint').scrollIntoView({ block: 'center' });
    document.getElementById('new-path-input').focus({ preventScroll: true });
}

// Admins see when the server's HTTPS certificate is about to expire or has (the server checks
// daily), or when a newer one waits for a restart.
async function showCertificateWarning(isAdmin) {
    const box = document.getElementById('cert-warning');
    box.hidden = true;
    if (!isAdmin) return;
    try {
        const cert = await api.request('/api/admin/certificate');
        if (cert.message) {
            box.textContent = cert.message;
            box.classList.toggle('cert-expired', cert.state === 'expired');
            box.hidden = false;
        }
    } catch (e) { /* no warning */ }
}

async function showSettings(pushHistory = true) {
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    // The account signed in (watch status is kept per account)
    document.getElementById('account-signed-in').textContent =
        user.username ? `Signed in as ${user.username}${user.is_admin ? ' (admin)' : ''}` : '';
    // The server's version (from /api/info, which is public), at the foot of My Account
    setupLanguageSetting();
    fetch('/api/info', { cache: 'no-cache' }).then(r => r.json()).then(info => {
        document.getElementById('server-version').textContent = info.version ? `WeirdStreamer server ${info.version}` : '';
    }).catch(() => {});

    // Hide admin tabs for non-admin users
    const serverTabBtn = document.querySelector('.settings-tab[data-tab="server"]');
    const usersTabBtn = document.querySelector('.settings-tab[data-tab="users"]');
    const jobsTabBtn = document.querySelector('.settings-tab[data-tab="jobs"]');
    const streamsTabBtn = document.querySelector('.settings-tab[data-tab="streams"]');

    if (serverTabBtn) serverTabBtn.style.display = user.is_admin ? 'block' : 'none';
    if (usersTabBtn) usersTabBtn.style.display = user.is_admin ? 'block' : 'none';
    if (jobsTabBtn) jobsTabBtn.style.display = user.is_admin ? 'block' : 'none';
    if (streamsTabBtn) streamsTabBtn.style.display = user.is_admin ? 'block' : 'none';

    // Settings opens on My Account (openLibrarySettings then goes to Server & Media)
    {
        document.querySelectorAll('.settings-tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.settings-tab-content').forEach(c => c.classList.remove('active'));
        const accountTabBtn = document.querySelector('.settings-tab[data-tab="account"]');
        if (accountTabBtn) accountTabBtn.classList.add('active');
        document.getElementById('account-tab').classList.add('active');
    }

    showCertificateWarning(user.is_admin);

    if (user.is_admin) {
        try {
            const config = await api.getConfig();
            state.currentConfig = config;
            updateAvailableGroups();

            // Populate form fields
            document.getElementById('server-name').value = config.server.name || '';
            document.getElementById('server-port').value = config.server.port;
            document.getElementById('server-host').value = config.server.host;
            document.getElementById('enable-http2').checked = config.streaming.enableHTTP2;
            document.getElementById('pause-jobs-streaming').checked = !!config.streaming.pauseJobsWhileStreaming;
            showHardwareSettings(config.streaming);
            showBackupSettings(config.database || {});
            document.getElementById('guide-prime-time').value = String((config.guide && config.guide.primeTime) || 20);
            loadDamagedVideos();
            const metadata = config.metadata || {};
            document.getElementById('tmdb-token').value = metadata.tmdbAccessToken || '';
            document.getElementById('opensubtitles-key').value = metadata.openSubtitlesApiKey || '';
            document.getElementById('metadata-language').value = metadata.language || '';

            // Render the groups and media paths
            renderGroups();
            editingPaths.clear(); // opening Settings: every library as it is
            renderMediaPaths(config.media.paths);
            document.getElementById('add-group-btn').onclick = handleAddGroup;
            document.getElementById('new-group-input').onkeydown = (e) => { if (e.key === 'Enter') handleAddGroup(); };
            document.getElementById('new-user-group').onchange = (e) => handleNewUserGroupChange(e.target);

            // Setup event listeners for settings
            document.getElementById('add-path-btn').onclick = handleAddPath;
            document.getElementById('browse-path-btn').onclick = openFolderPicker;
            document.getElementById('save-settings-btn').onclick = handleSaveSettings;
        } catch (error) {
            console.error('Failed to load admin settings:', error);
            alert('Failed to load system settings: ' + error.message);
        }
    }

    applyTheme(); // marks the theme in use in the picker

    // Show settings screen
    if (pushHistory) pushScreenHistory('settings'); // first, keeping the view's scroll position
    mainScreen.classList.remove('active');
    settingsScreen.classList.add('active');
}

function hideSettings() {
    stopJobsView();
    stopStreamsView();
    settingsScreen.classList.remove('active');
    mainScreen.classList.add('active');
}

// Settings is a screen over the media view, a history entry of its own (the view's state plus
// `screen`), so the browser's Back leaves it for the view (see popstate).
const SCREENS = {
    settings: { element: () => settingsScreen, show: () => showSettings(false), hide: hideSettings },
    guide: { element: () => guideScreen, show: () => showGuide(guide.lib, false), hide: () => hideGuide() }, // js/guide.js
};

function openScreen() {
    return Object.keys(SCREENS).find(name => SCREENS[name].element().classList.contains('active')) || null;
}

function pushScreenHistory(name) {
    if (history.state && history.state.screen === name) return;
    rememberScroll();
    const { screen, scrollY, ...view } = history.state || {};
    history.pushState({ ...view, screen: name }, '');
}

// A screen's close button: back through its history entry (popstate hides it), or just hidden.
function closeScreen(name) {
    if (history.state && history.state.screen === name) history.back();
    else SCREENS[name].hide();
}

// Text made safe to put in HTML.
function escapeHtml(text) {
    return String(text).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// The defined groups (the config's groups list): the only ones users and libraries may name.
function updateAvailableGroups() {
    if (!state.currentConfig) return;
    state.availableGroups = [...(state.currentConfig.groups || [])];
}

// Settings → Groups: the defined groups, with Rename and Remove. Changes go to the server right
// away, and into the settings being edited (the media paths), which aren't reloaded.
function renderGroups() {
    const list = document.getElementById('groups-list');
    list.innerHTML = '';
    const groups = state.availableGroups;
    if (!groups.length) {
        list.innerHTML = '<p class="settings-hint">No groups yet. Add one to give users and libraries access.</p>';
    }
    groups.forEach((name, i) => {
        const row = document.createElement('div');
        row.className = 'group-row';
        const label = document.createElement('span');
        label.className = 'group-name';
        label.textContent = name;
        if (i === 0) { // new users start in it
            const tag = document.createElement('span');
            tag.className = 'group-default';
            tag.textContent = 'default';
            label.append(' ', tag);
        }
        row.append(label);
        if (i > 0) {
            const makeDefault = document.createElement('button');
            makeDefault.type = 'button';
            makeDefault.className = 'btn btn-small secondary';
            makeDefault.textContent = 'Make default';
            makeDefault.title = `New users start in ${name}`;
            makeDefault.onclick = () => handleMakeDefaultGroup(name);
            row.append(makeDefault);
        }
        const rename = document.createElement('button');
        rename.type = 'button';
        rename.className = 'btn btn-small secondary';
        rename.textContent = 'Rename';
        rename.onclick = () => handleRenameGroup(name);
        const remove = document.createElement('button');
        remove.type = 'button';
        remove.className = 'btn btn-danger btn-small';
        remove.innerHTML = iconSvg('delete') + '<span class="btn-label">Remove</span>';
        remove.onclick = () => handleRemoveGroup(name);
        row.append(rename, remove);
        list.appendChild(row);
    });
    renderNewUserGroups();
}

function groupsMessage(text, isError = false) {
    const el = document.getElementById('groups-message');
    el.textContent = text;
    el.style.color = isError ? '#e74c3c' : '#27ae60';
    if (!isError) setTimeout(() => { if (el.textContent === text) el.textContent = ''; }, 4000);
}

// Takes the server's new list of groups, here and in the settings being edited.
function setGroups(groups) {
    if (state.currentConfig) state.currentConfig.groups = groups;
    state.availableGroups = [...groups];
    renderGroups();
    if (state.currentConfig) renderMediaPaths(state.currentConfig.media.paths);
}

async function handleAddGroup() {
    const input = document.getElementById('new-group-input');
    const name = input.value.trim();
    if (!name) return;
    try {
        setGroups(await api.createGroup(name));
        input.value = '';
        groupsMessage(`Added ${name}`);
    } catch (error) {
        groupsMessage(error.message, true);
    }
}

async function handleRenameGroup(name) {
    const newName = (prompt(`Rename the group "${name}" to:`, name) || '').trim();
    if (!newName || newName === name) return;
    try {
        const groups = await api.renameGroup(name, newName);
        // Its libraries in the settings being edited follow, as on the server
        (state.currentConfig?.media?.paths || []).forEach(p => {
            p.groups = (p.groups || []).map(g => g === name ? newName : g);
        });
        setGroups(groups);
        groupsMessage(`Renamed ${name} to ${newName}, for its users and libraries`);
        if (document.getElementById('users-tab').classList.contains('active')) loadUsers();
    } catch (error) {
        groupsMessage(error.message, true);
    }
}

async function handleMakeDefaultGroup(name) {
    try {
        setGroups(await api.makeDefaultGroup(name));
        renderNewUserGroups(true); // Create New User starts on it now
        groupsMessage(`${name} is the default group: new users start in it`);
    } catch (error) {
        groupsMessage(error.message, true);
    }
}

async function handleRemoveGroup(name) {
    if (!confirm(`Remove the group "${name}"?`)) return;
    try {
        setGroups(await api.deleteGroup(name));
        groupsMessage(`Removed ${name}`);
    } catch (error) {
        groupsMessage(error.message, true); // still in use: says by whom
    }
}

// The new user form's group menu: the groups, and "New group…" to make one on the spot. It starts
// on the first group, the default (reset puts it back there after creating a user).
function renderNewUserGroups(reset = false) {
    const select = document.getElementById('new-user-group');
    if (!select) return;
    const current = reset ? '' : select.value;
    select.innerHTML = '<option value="" disabled>Group</option>' +
        state.availableGroups.map(g => `<option value="${escapeHtml(g)}">${escapeHtml(g)}</option>`).join('') +
        '<option value="__new__">New group…</option>';
    select.value = state.availableGroups.includes(current) ? current : (state.availableGroups[0] || '');
}

async function handleNewUserGroupChange(select) {
    if (select.value !== '__new__') return;
    const name = (prompt('Name of the new group:') || '').trim();
    if (!name) {
        renderNewUserGroups();
        return;
    }
    try {
        setGroups(await api.createGroup(name));
        select.value = name;
    } catch (error) {
        alert(error.message);
        renderNewUserGroups();
    }
}

// Library types the server accepts; keep in sync with config.LibraryType* on the server.
const LIBRARY_TYPES = [
    { value: '', label: 'No type' },
    { value: 'movie', label: 'Movies' },
    { value: 'tv', label: 'TV Shows' },
];

// Where a library's videos in progress show on Continue Watching (the config's continueWatchingMode)
const CONTINUE_MODES = [
    { value: 'on', label: 'Continue Watching: on the home screen and the library' },
    { value: 'libraryOnly', label: 'Continue Watching: in the library only' },
    { value: 'off', label: 'Continue Watching: off' },
];

// Where a library's new videos show on Recently Added (the config's showNewAdditions)
const NEW_ADDITIONS_MODES = [
    { value: 'on', label: 'Recently Added: on the home screen and the library' },
    { value: 'libraryOnly', label: 'Recently Added: in the library only' },
    { value: 'off', label: 'Recently Added: off' },
];

// Where a library's titles are on the Guide's channels (the config's showInGuide)
const GUIDE_MODES = [
    { value: 'on', label: 'Show in Guide: on the home screen and the library' },
    { value: 'libraryOnly', label: 'Show in Guide: in the library only' },
    { value: 'off', label: 'Show in Guide: off' },
];

// Sets a media path's type, omitting the key for "no type" so the saved config stays clean.
function setMediaPathType(mediaPath, type) {
    if (type) {
        mediaPath.type = type;
    } else {
        delete mediaPath.type;
    }
}

// The libraries being edited (by index): the others show what they are, with an Edit button, so
// a stray click can't change one. A library just added opens for editing.
const editingPaths = new Set();

// A library as it is, read-only: its name and folder, then its type, groups and rows.
function mediaPathSummary(mediaPath, index) {
    const div = el('div', 'media-path');
    const head = el('div', 'media-path-head');
    const names = el('div', 'media-path-names');
    names.append(el('div', 'media-path-name', mediaPath.name || '(no name)'), el('div', 'media-path-folder', mediaPath.path));
    const edit = el('button', 'btn btn-small');
    edit.type = 'button';
    edit.dataset.icon = 'edit';
    edit.innerHTML = iconSvg('edit') + '<span class="btn-label">Edit</span>';
    edit.onclick = () => { editingPaths.add(index); renderMediaPaths(state.currentConfig.media.paths); };
    head.append(names, edit);
    // A label and its value per row: the type, the groups (as chips) and where its rows show.
    const details = el('dl', 'media-path-details');
    const row = (term, value) => {
        const dd = el('dd');
        if (typeof value === 'string') dd.textContent = value; else dd.appendChild(value);
        details.append(el('dt', '', term), dd);
    };
    const where = { on: 'Home screen and library', libraryOnly: 'Library only', off: 'Off' };
    row('Type', (LIBRARY_TYPES.find(o => o.value === (mediaPath.type || '')) || LIBRARY_TYPES[0]).label);
    const chips = el('span', 'media-path-chips');
    (mediaPath.groups || []).forEach(g => chips.appendChild(el('span', 'media-path-chip', g)));
    row('Groups', (mediaPath.groups || []).length ? chips : 'None (no one sees it)');
    row('Continue Watching', where[mediaPath.continueWatchingMode || 'on'] || mediaPath.continueWatchingMode);
    row('Recently Added', where[mediaPath.showNewAdditions || 'off'] || mediaPath.showNewAdditions);
    if (mediaPath.type) row('Show in Guide', where[mediaPath.showInGuide || 'off'] || mediaPath.showInGuide);
    row('Guide filler', mediaPath.guideFiller ? 'Its short videos fill gaps between programs' : 'No');
    div.append(head, details);
    return div;
}

function renderMediaPaths(paths) {
    const pathsList = document.getElementById('media-paths-list');
    pathsList.innerHTML = '';
    document.getElementById('no-libraries-hint').hidden = paths.length > 0;
    
    paths.forEach((mediaPath, index) => {
        if (!editingPaths.has(index)) {
            pathsList.appendChild(mediaPathSummary(mediaPath, index));
            return;
        }
        const div = el('div', 'media-path editing');
        
        const nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.value = mediaPath.name;
        nameInput.className = 'settings-input';
        nameInput.style.cssText = 'margin-bottom: 5px; width: calc(100% - 20px);';
        nameInput.placeholder = 'Display Name';
        nameInput.onchange = (e) => {
            state.currentConfig.media.paths[index].name = e.target.value;
        };
        
        // The groups that can see the library: any of the defined ones
        const groupsInput = document.createElement('div');
        groupsInput.className = 'path-groups';
        groupsInput.title = 'Groups that can see this library';
        const label = document.createElement('span');
        label.className = 'path-groups-label';
        label.textContent = 'Groups:';
        groupsInput.appendChild(label);
        state.availableGroups.forEach(group => {
            const option = document.createElement('label');
            option.className = 'path-group';
            const box = document.createElement('input');
            box.type = 'checkbox';
            box.checked = (mediaPath.groups || []).includes(group);
            box.onchange = () => {
                const p = state.currentConfig.media.paths[index];
                const set = new Set(p.groups || []);
                if (box.checked) set.add(group); else set.delete(group);
                p.groups = state.availableGroups.filter(g => set.has(g)); // in the groups' order
            };
            option.append(box, document.createTextNode(' ' + group));
            groupsInput.appendChild(option);
        });
        if (!state.availableGroups.length) {
            groupsInput.appendChild(document.createTextNode(' none defined (add one under Groups)'));
        }

        const typeSelect = document.createElement('select');
        typeSelect.className = 'settings-input';
        typeSelect.style.cssText = 'margin-bottom: 5px; font-size: 12px;';
        typeSelect.title = 'Library type';
        LIBRARY_TYPES.forEach(({ value, label }) => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = label;
            typeSelect.appendChild(option);
        });
        typeSelect.value = mediaPath.type || '';
        typeSelect.onchange = (e) => {
            setMediaPathType(state.currentConfig.media.paths[index], e.target.value);
        };

        const continueSelect = document.createElement('select');
        continueSelect.className = 'settings-input';
        continueSelect.style.cssText = 'margin-bottom: 5px; font-size: 12px;';
        continueSelect.title = 'Where videos in progress show on Continue Watching';
        CONTINUE_MODES.forEach(({ value, label }) => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = label;
            continueSelect.appendChild(option);
        });
        continueSelect.value = mediaPath.continueWatchingMode || 'on';
        continueSelect.onchange = (e) => {
            state.currentConfig.media.paths[index].continueWatchingMode = e.target.value;
        };

        const newAdditions = document.createElement('select');
        newAdditions.className = 'settings-input';
        newAdditions.style.cssText = 'margin-bottom: 5px; font-size: 12px;';
        newAdditions.title = 'Where the last videos to arrive show on Recently Added';
        NEW_ADDITIONS_MODES.forEach(({ value, label }) => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = label;
            newAdditions.appendChild(option);
        });
        newAdditions.value = mediaPath.showNewAdditions || 'off';
        newAdditions.onchange = (e) => {
            state.currentConfig.media.paths[index].showNewAdditions = e.target.value;
        };

        // The Guide airs movie and TV libraries' titles; any library can lend it filler.
        const guideSelect = document.createElement('select');
        guideSelect.className = 'settings-input';
        guideSelect.style.cssText = 'margin-bottom: 5px; font-size: 12px;';
        guideSelect.title = 'Where its titles are on the Guide’s live channels (movie and TV libraries)';
        GUIDE_MODES.forEach(({ value, label }) => {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = label;
            guideSelect.appendChild(option);
        });
        guideSelect.value = mediaPath.showInGuide || 'off';
        guideSelect.onchange = (e) => {
            state.currentConfig.media.paths[index].showInGuide = e.target.value;
        };
        const fillerLabel = document.createElement('label');
        fillerLabel.style.cssText = 'display: block; margin-bottom: 5px; font-size: 12px;';
        fillerLabel.title = 'Its videos of 15 minutes or less can fill the gaps between the Guide’s programs; it stays a library as usual';
        const filler = document.createElement('input');
        filler.type = 'checkbox';
        filler.checked = !!mediaPath.guideFiller;
        filler.onchange = (e) => {
            if (e.target.checked) state.currentConfig.media.paths[index].guideFiller = true;
            else delete state.currentConfig.media.paths[index].guideFiller;
        };
        fillerLabel.append(filler, ' Use for Guide Filler');

        const pathDiv = document.createElement('div');
        pathDiv.style.cssText = 'display: flex; align-items: center; gap: 10px;';
        
        const pathText = document.createElement('span');
        pathText.textContent = mediaPath.path;
        pathText.style.cssText = 'flex: 1; min-width: 0; overflow-wrap: anywhere; color: var(--muted-text); font-size: 13px;';
        
        const removeBtn = document.createElement('button');
        removeBtn.type = 'button';
        removeBtn.className = 'btn btn-danger btn-small';
        removeBtn.innerHTML = iconSvg('delete') + '<span class="btn-label">Remove</span>';
        removeBtn.onclick = () => handleRemovePath(index);
        
        const doneBtn = document.createElement('button');
        doneBtn.type = 'button';
        doneBtn.className = 'btn btn-small';
        doneBtn.innerHTML = iconSvg('check') + '<span class="btn-label">Done</span>';
        doneBtn.title = 'Back to the summary (Save Settings saves the change)';
        doneBtn.onclick = () => { editingPaths.delete(index); renderMediaPaths(state.currentConfig.media.paths); };

        pathDiv.appendChild(pathText);
        pathDiv.appendChild(removeBtn);
        pathDiv.appendChild(doneBtn);
        
        div.appendChild(nameInput);
        div.appendChild(groupsInput);
        div.appendChild(typeSelect);
        div.appendChild(continueSelect);
        div.appendChild(newAdditions);
        if (mediaPath.type) div.appendChild(guideSelect);
        div.appendChild(fillerLabel);
        div.appendChild(pathDiv);
        pathsList.appendChild(div);
    });
}

function handleAddPath() {
    const input = document.getElementById('new-path-input');
    const newPath = input.value.trim();
    
    if (!newPath) {
        alert('Please enter a path');
        return;
    }
    
    // Check if path already exists
    const exists = state.currentConfig.media.paths.some(mp => mp.path === newPath);
    if (!exists) {
        const pathParts = newPath.split('/');
        const name = pathParts[pathParts.length - 1] || pathParts[pathParts.length - 2] || newPath;
        const typeSelect = document.getElementById('new-path-type');
        const mediaPath = { name: name, path: newPath, groups: [] };
        setMediaPathType(mediaPath, typeSelect.value);
        state.currentConfig.media.paths.push(mediaPath);
        editingPaths.add(state.currentConfig.media.paths.length - 1);
        renderMediaPaths(state.currentConfig.media.paths);
        input.value = '';
        typeSelect.value = '';
    } else {
        alert('Path already exists');
    }
}

// The folder picker: walks the server's folders (hidden ones only by typing their path) and puts
// the one chosen in the new path's field. It opens where that field points, else beside the
// last library.
const folderPicker = { path: null };

function openFolderPicker() {
    const typed = document.getElementById('new-path-input').value.trim();
    const paths = state.currentConfig?.media?.paths || [];
    const last = paths.length ? paths[paths.length - 1].path : '';
    document.getElementById('folder-modal').classList.add('active');
    // The server's paths may be Windows' (C:\Videos): it says what's a full path and what's above one.
    if (typed) showFolder(typed, true);
    else if (last) showFolder(last, true, true);
    else showFolder('', true);
}

function closeFolderPicker() {
    document.getElementById('folder-modal').classList.remove('active');
}

// Lists [path] (its parent with [parent]; '' is the top: / or Windows' drives); one that can't be
// listed keeps the folder shown (or, opening, the top).
async function showFolder(path, opening = false, parent = false) {
    const info = document.getElementById('folder-info');
    const list = document.getElementById('folder-list');
    try {
        const f = await api.listFolders(path, parent);
        folderPicker.path = f.path;
        document.getElementById('folder-path').value = f.path;
        document.getElementById('folder-up').disabled = f.top;
        document.getElementById('folder-up').onclick = () => showFolder(f.parent);
        document.getElementById('folder-choose').disabled = !f.path;
        list.innerHTML = '';
        f.folders.forEach(entry => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'folder-entry';
            btn.innerHTML = iconSvg('folder') + `<span>${escapeHtml(entry.name)}</span>`;
            btn.addEventListener('click', () => showFolder(entry.path));
            list.appendChild(btn);
        });
        if (!f.folders.length) list.innerHTML = '<div class="match-empty">No folders in here.</div>';
        info.classList.remove('error');
        info.textContent = f.library ? `Already the library “${f.library}”`
            : f.videos ? `${f.videos} video${f.videos === 1 ? '' : 's'} in this folder` : '';
        list.scrollTop = 0;
    } catch (e) {
        if (opening && path !== '') return showFolder('');
        if (folderPicker.path) document.getElementById('folder-path').value = folderPicker.path;
        info.classList.add('error');
        info.textContent = e.message || "Couldn't open that folder";
    }
}

function setupFolderPicker() {
    const modal = document.getElementById('folder-modal');
    document.getElementById('folder-close').addEventListener('click', closeFolderPicker);
    document.getElementById('folder-cancel').addEventListener('click', closeFolderPicker);
    modal.addEventListener('click', (e) => { if (e.target === modal) closeFolderPicker(); });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && modal.classList.contains('active')) closeFolderPicker();
    });
    document.getElementById('folder-go').addEventListener('submit', (e) => {
        e.preventDefault();
        const typed = document.getElementById('folder-path').value.trim();
        if (typed) showFolder(typed);
    });
    document.getElementById('folder-choose').addEventListener('click', () => {
        if (!folderPicker.path) return;
        const input = document.getElementById('new-path-input');
        input.value = folderPicker.path;
        closeFolderPicker();
        input.focus();
    });
}

function handleRemovePath(index) {
    if (confirm('Remove this path?')) {
        state.currentConfig.media.paths.splice(index, 1);
        // The ones after it move up a place.
        const editing = [...editingPaths].filter(i => i !== index).map(i => i > index ? i - 1 : i);
        editingPaths.clear();
        editing.forEach(i => editingPaths.add(i));
        renderMediaPaths(state.currentConfig.media.paths);
        updateAvailableGroups();
    }
}

// Settings › Backups: the schedule (saved with the other settings), Back up now, and the backups
// in the folder config.yml names (GET/POST /api/admin/backups).
function showBackupSettings(database) {
    const schedule = document.getElementById('backup-schedule');
    schedule.value = database.backupSchedule || 'daily';
    document.getElementById('backup-day').value = database.backupDay || 'sunday';
    document.getElementById('backup-time').value = database.backupTime || '04:00';
    document.getElementById('backup-keep').value = database.backupKeep || 7;
    const updateWhen = () => {
        document.getElementById('backup-when').hidden = schedule.value === 'off';
        document.getElementById('backup-day-label').hidden = schedule.value !== 'weekly';
    };
    schedule.onchange = updateWhen;
    updateWhen();
    document.getElementById('backup-now-btn').onclick = backUpNow;
    document.getElementById('backup-message').textContent = '';
    loadBackupList();
}

async function loadBackupList() {
    const folder = document.getElementById('backup-folder'), list = document.getElementById('backup-list');
    let info;
    try {
        info = await api.request('/api/admin/backups');
    } catch (e) {
        folder.textContent = "Couldn't read the backups: " + e.message;
        return;
    }
    const next = info.next ? ` The next one: ${new Date(info.next).toLocaleString([], { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}.` : '';
    folder.textContent = `In ${info.folder} (config.yml's database.backupPath).${next}`;
    list.innerHTML = '';
    if (!info.backups.length) {
        const li = document.createElement('li');
        li.textContent = 'No backups yet.';
        list.appendChild(li);
    }
    for (const b of info.backups) {
        const li = document.createElement('li');
        li.textContent = `${new Date(b.created).toLocaleString()} · ${formatSize(b.size)}`;
        li.title = b.name;
        list.appendChild(li);
    }
}

async function backUpNow() {
    const button = document.getElementById('backup-now-btn'), message = document.getElementById('backup-message');
    button.disabled = true;
    message.textContent = 'Backing up…';
    try {
        const made = await api.request('/api/admin/backups', { method: 'POST' });
        message.textContent = `Made ${made.name} (${formatSize(made.size)}).`;
    } catch (e) {
        message.textContent = e.message;
    } finally {
        button.disabled = false;
        loadBackupList();
    }
}

// Settings › Server › Damaged videos (GET /api/admin/damaged): videos with a broken seek index and
// their fixed copies, each playable (the original as it is, or the fixed copy: POST
// /api/admin/damaged/play) and replaceable by its fixed copy; and the trash of replaced originals
// (loadTrash).
function clockText(seconds) {
    const s = Math.round(seconds || 0);
    return `${Math.floor(s / 3600)}:${String(Math.floor(s / 60) % 60).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

async function loadDamagedVideos() {
    const list = document.getElementById('damaged-list');
    let info;
    try {
        info = await api.request('/api/admin/damaged');
    } catch (e) {
        list.innerHTML = '';
        document.getElementById('damaged-message').textContent = "Couldn't read the damaged videos: " + e.message;
        return;
    }
    list.innerHTML = '';
    if (!info.videos.length) {
        const li = document.createElement('li');
        li.textContent = 'None found.';
        list.appendChild(li);
    }
    for (const v of info.videos) {
        const li = document.createElement('li');
        const title = document.createElement('div');
        title.className = 'damaged-name';
        title.textContent = v.name;
        const where = document.createElement('div');
        where.className = 'settings-hint';
        where.textContent = [v.library, v.folder].filter(Boolean).join(' › ') + ' · ' + v.reason;
        const sizes = document.createElement('div');
        sizes.className = 'settings-hint';
        sizes.textContent = {
            fixed: `Original: ${formatSize(v.size)}, says ${clockText(v.claimed)} long · Fixed copy: ${formatSize(v.fixed_size)}, ${clockText(v.duration)}`,
            fixing: `${formatSize(v.size)} · Making the fixed copy…`,
            failed: `${formatSize(v.size)} · Couldn't be fixed: ${v.error || 'unknown'}`,
        }[v.state] || `${formatSize(v.size)} · Not fixed: Fix makes a copy of about its size`;
        const buttons = document.createElement('div');
        buttons.className = 'damaged-buttons';
        const button = (label, icon, secondary, onClick) => {
            const b = document.createElement('button');
            b.type = 'button';
            b.className = secondary ? 'btn btn-secondary' : 'btn';
            b.dataset.icon = icon;
            b.innerHTML = `<span class="btn-label">${label}</span>`;
            b.onclick = onClick;
            buttons.appendChild(b);
        };
        button('Play original', 'play', true, () => playDamaged(v, true));
        if (v.state === 'damaged' || v.state === 'failed') {
            button(v.state === 'failed' ? 'Try fixing again' : 'Fix', 'save', false, () => fixDamaged(v));
        }
        if (v.state === 'fixed') {
            button('Play fixed copy', 'play', false, () => playDamaged(v, false));
            button('Replace with fixed copy', 'save', false, () => replaceDamaged(v));
        }
        const body = document.createElement('div');
        body.className = 'damaged-body';
        body.append(title, where, sizes, buttons);
        if (v.thumbnail_url) {
            const img = document.createElement('img');
            img.className = 'damaged-thumb';
            img.src = v.thumbnail_url;
            img.alt = '';
            li.append(img);
        }
        li.append(body);
        list.appendChild(li);
    }
    decorateIconButtons();
    // While a copy is being made, the list is asked again until it's done
    clearTimeout(loadDamagedVideos.timer);
    if (info.videos.some(v => v.state === 'fixing') && document.getElementById('damaged-list').isConnected) {
        loadDamagedVideos.timer = setTimeout(loadDamagedVideos, 3000);
    }
    loadTrash();
}

// The trash of replaced originals (GET /api/admin/trash), each with Restore (POST
// /api/admin/trash/restore), which undoes its replace.
async function loadTrash(t) {
    const trashStatus = document.getElementById('trash-status'), emptyBtn = document.getElementById('empty-trash-btn');
    const list = document.getElementById('trash-list');
    if (!t) {
        try {
            t = await api.request('/api/admin/trash');
        } catch (e) {
            trashStatus.textContent = "Couldn't read the trash: " + e.message;
            return;
        }
    }
    trashStatus.textContent = t.files
        ? `The trash holds ${t.files} replaced original${t.files === 1 ? '' : 's'} (${formatSize(t.bytes)}), each deleted ${t.days} days after it went in.`
        : `The trash is empty. Replaced originals are kept there ${t.days} days.`;
    emptyBtn.hidden = !t.files;
    emptyBtn.onclick = emptyTrash;
    list.innerHTML = '';
    const day = iso => new Date(iso).toLocaleDateString();
    for (const item of t.items) {
        const li = document.createElement('li');
        const title = document.createElement('div');
        title.className = 'damaged-name';
        title.textContent = item.name;
        const where = document.createElement('div');
        where.className = 'settings-hint';
        where.textContent = [item.library, item.folder].filter(Boolean).join(' › ') || 'Not in a library';
        const sizes = document.createElement('div');
        sizes.className = 'settings-hint';
        sizes.textContent = `${formatSize(item.size)} · Replaced ${day(item.trashed_at)}`
            + (item.replacement ? ` by ${item.replacement}` : '') + ` · Deleted ${day(item.deletes_at)}`;
        const body = document.createElement('div');
        body.className = 'damaged-body';
        body.append(title, where, sizes);
        if (item.restorable) {
            const buttons = document.createElement('div');
            buttons.className = 'damaged-buttons';
            const b = document.createElement('button');
            b.type = 'button';
            b.className = 'btn btn-secondary';
            b.dataset.icon = 'replay';
            b.innerHTML = '<span class="btn-label">Restore</span>';
            b.onclick = () => restoreTrash(item);
            buttons.appendChild(b);
            body.append(buttons);
        }
        if (item.thumbnail_url) {
            const img = document.createElement('img');
            img.className = 'damaged-thumb';
            img.src = item.thumbnail_url;
            img.alt = '';
            li.append(img);
        }
        li.append(body);
        list.appendChild(li);
    }
    decorateIconButtons();
}

async function restoreTrash(item) {
    const replaced = item.replacement ? `"${item.replacement}" goes back to the fixed copies, ` : '';
    if (!confirm(`Restore "${item.name}"?\n\nIt goes back to its folder, ${replaced}and its watch progress goes back to it. It's listed with the damaged videos again.`)) return;
    const message = document.getElementById('damaged-message');
    try {
        await api.request('/api/admin/trash/restore', { method: 'POST', body: JSON.stringify({ id: item.id }) });
        message.textContent = `Restored: "${item.name}" is back in its folder.`;
    } catch (e) {
        message.textContent = e.message;
    }
    loadDamagedVideos();
}

async function playDamaged(v, original) {
    const message = document.getElementById('damaged-message');
    message.textContent = '';
    try {
        await api.request('/api/admin/damaged/play', { method: 'POST', body: JSON.stringify({ path: api.encodePath(v.path), original }) });
    } catch (e) {
        message.textContent = e.message;
        return;
    }
    playVideo({ path: v.path, name: original ? `${v.name} (original)` : `${v.name} (fixed copy)`, type: 'video',
        duration: original ? 0 : v.duration }, false, 0, false);
}

async function fixDamaged(v) {
    const message = document.getElementById('damaged-message');
    try {
        await api.request('/api/admin/damaged/fix', { method: 'POST', body: JSON.stringify({ path: api.encodePath(v.path) }) });
        message.textContent = `Making the fixed copy of "${v.name}"…`;
    } catch (e) {
        message.textContent = e.message;
    }
    loadDamagedVideos();
}

async function replaceDamaged(v) {
    const newName = v.name.replace(/\.[^.]*$/, '') + '.mkv';
    if (!confirm(`Replace "${v.name}" with its fixed copy?\n\nThe fixed copy goes into its folder as "${newName}", with its watch progress, and the original goes to the trash (deleted after 30 days).`)) return;
    const message = document.getElementById('damaged-message');
    try {
        await api.request('/api/admin/damaged/replace', { method: 'POST', body: JSON.stringify({ path: api.encodePath(v.path) }) });
        message.textContent = `Replaced: "${newName}" is in its place, and the original is in the trash.`;
    } catch (e) {
        message.textContent = e.message;
    }
    loadDamagedVideos();
}

async function emptyTrash() {
    if (!confirm('Delete every replaced original in the trash now? They can\'t be brought back.')) return;
    const message = document.getElementById('damaged-message');
    try {
        await api.request('/api/admin/trash/empty', { method: 'POST' });
        message.textContent = 'The trash is empty.';
    } catch (e) {
        message.textContent = e.message;
    }
    loadTrash();
}

// Settings › Streaming › Hardware encoding: the server's GPUs and the encoders that passed its
// test encode (GET /api/admin/hardware), and the choice among them. Checked again until the
// server's startup test is done.
async function showHardwareSettings(streaming) {
    const status = document.getElementById('hardware-status');
    const options = document.getElementById('hardware-options');
    let report;
    try {
        report = await api.request('/api/admin/hardware');
    } catch (e) {
        status.textContent = "Couldn't read the server's graphics hardware.";
        options.hidden = true;
        return;
    }
    if (report.checking) {
        status.textContent = "Checking the server's graphics hardware…";
        options.hidden = true;
        setTimeout(() => {
            if (document.getElementById('settings-screen').classList.contains('active')) showHardwareSettings(streaming);
        }, 2000);
        return;
    }
    const gpus = report.gpus.map(g => g.name).join(', ');
    const problems = (report.problems || []).map(p => `${p.encoder} on ${p.gpu}: ${p.reason}`);
    if (!report.encoders.length) {
        status.textContent = (gpus ? `Graphics: ${gpus}. None of it can encode with this server's ffmpeg, so ` :
            'No graphics hardware was found, so ') + 'transcodes are encoded by the CPU (x264).' +
            (problems.length ? ' ' + problems.join('. ') + '.' : '');
        options.hidden = true;
        return;
    }
    status.textContent = `Graphics: ${gpus}.` + (problems.length ? ` Not usable: ${problems.join('. ')}.` : '');
    const select = document.getElementById('hardware-encoder');
    select.innerHTML = '';
    select.appendChild(new Option('The CPU (x264)', ''));
    report.encoders.forEach(e => select.appendChild(new Option(`${e.name} on ${e.gpu}`, `${e.id}|${e.device || ''}`)));
    const chosen = `${streaming.hardwareEncoder || ''}|${streaming.hardwareDevice || ''}`;
    const match = report.encoders.find(e => e.id === streaming.hardwareEncoder && (e.id === 'nvenc' || (e.device || '') === (streaming.hardwareDevice || '')));
    if (streaming.hardwareEncoder && !match) {
        select.appendChild(new Option(`${streaming.hardwareEncoder} (not working on this server: the CPU encodes)`, chosen));
    }
    select.value = match ? `${match.id}|${match.device || ''}` : (streaming.hardwareEncoder ? chosen : '');
    document.getElementById('hardware-max').value = streaming.hardwareMaxTranscodes || 0;
    const note = document.getElementById('hardware-note');
    const updateNote = () => {
        const sessions = (report.encoders.find(e => `${e.id}|${e.device || ''}` === select.value) || {}).sessions || 0;
        note.textContent = !select.value ? 'Transcodes and downloads are encoded by the CPU, and trickplay frames decoded by it.' :
            'The GPU encodes the video; decoding, scaling and burned-in subtitles stay on the CPU. ' +
            'A transcode the GPU fails to start is encoded by the CPU, as are those past the limit. ' +
            (sessions > 0 ? `Its driver allows ${sessions} encodes at once, so the limit is never more than that. ` : '') +
            'Downloads are encoded the same way and count toward the limit. ' +
            'The GPU also decodes the videos for trickplay frames (the seek bar\'s pictures), outside the limit; ' +
            'a video it can\'t decode is done by the CPU.';
    };
    select.onchange = updateNote;
    updateNote();
    options.hidden = false;
}

async function handleSaveSettings() {
    const messageEl = document.getElementById('settings-message');
    
    try {
        // Update config object with form values
        state.currentConfig.server.name = document.getElementById('server-name').value.trim();
        state.currentConfig.server.port = parseInt(document.getElementById('server-port').value);
        state.currentConfig.server.host = document.getElementById('server-host').value;
        state.currentConfig.streaming.enableHTTP2 = document.getElementById('enable-http2').checked;
        state.currentConfig.streaming.pauseJobsWhileStreaming = document.getElementById('pause-jobs-streaming').checked;
        if (!document.getElementById('hardware-options').hidden) {
            const [encoder, device] = document.getElementById('hardware-encoder').value.split('|');
            state.currentConfig.streaming.hardwareEncoder = encoder || '';
            state.currentConfig.streaming.hardwareDevice = device || '';
            state.currentConfig.streaming.hardwareMaxTranscodes = Math.max(0, parseInt(document.getElementById('hardware-max').value) || 0);
        }
        state.currentConfig.database = {
            ...(state.currentConfig.database || {}),
            backupSchedule: document.getElementById('backup-schedule').value,
            backupDay: document.getElementById('backup-day').value,
            backupTime: document.getElementById('backup-time').value || '04:00',
            backupKeep: Math.min(365, Math.max(1, parseInt(document.getElementById('backup-keep').value) || 7)),
        };
        state.currentConfig.guide = {
            ...(state.currentConfig.guide || {}),
            primeTime: parseInt(document.getElementById('guide-prime-time').value) || 20,
        };
        state.currentConfig.metadata = {
            ...(state.currentConfig.metadata || {}),
            tmdbAccessToken: document.getElementById('tmdb-token').value.trim(),
            openSubtitlesApiKey: document.getElementById('opensubtitles-key').value.trim(),
            language: document.getElementById('metadata-language').value.trim(),
        };
        
        const result = await api.updateConfig(state.currentConfig);
        // If server returned updated config (with IDs), adopt it locally
        if (result && result.config) {
            state.currentConfig = result.config;
            updateAvailableGroups();
        }
        messageEl.textContent = (result && result.message) ? result.message : 'Configuration updated.';
        updateHeaderServer(); // a new name
        loadBackupList(); // the next backup, by the schedule saved
        messageEl.style.color = '#27ae60';
        
        setTimeout(() => {
            messageEl.textContent = '';
        }, 5000);
    } catch (error) {
        messageEl.textContent = 'Failed to save: ' + error.message;
        messageEl.style.color = '#e74c3c';
    }
}

// User Management Functions
async function loadUsers() {
    try {
        const users = await api.getUsers();
        const usersList = document.getElementById('users-list');
        const currentUser = JSON.parse(localStorage.getItem('user'));
        
        // The defined groups (fresh)
        state.availableGroups = await api.getAvailableGroups();
        renderNewUserGroups();

        // Names go in as text and the handlers as listeners: a username can hold ' and &, which
        // an onclick="…('name')" attribute would run as script.
        usersList.innerHTML = users.map(user => {
            const isMe = user.username === currentUser.username;
            // Use the data from the server directly
            const isAdminChecked = user.is_admin ? 'checked' : '';

            return `
                <div class="user-item">
                    <div class="user-info">
                        <strong>${escapeHtml(user.username)}</strong>
                        ${user.is_admin ? '<span class="user-badge">ADMIN</span>' : ''}
                        ${isMe ? '<span class="user-badge user-badge-you">YOU</span>' : ''}
                        <div style="margin-top: 10px; display: flex; align-items: center; gap: 15px; flex-wrap: wrap;">
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: var(--muted-text);">Group: </span>
                                <select data-user-action="group" class="settings-input" style="width: auto; padding: 2px 5px; font-size: 12px;">
                                    ${state.availableGroups.includes(user.group) ? '' : `<option value="" selected disabled>${escapeHtml(user.group || 'None')} (not a group)</option>`}
                                    ${state.availableGroups.map(g => `<option value="${escapeHtml(g)}" ${user.group === g ? 'selected' : ''}>${escapeHtml(g)}</option>`).join('')}
                                </select>
                            </div>
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: var(--muted-text);">Admin: </span>
                                <input type="checkbox" data-user-action="admin" ${isAdminChecked} ${isMe ? 'disabled' : ''}>
                            </div>
                        </div>
                    </div>
                    ${!isMe ?
                        `<button type="button" class="btn btn-danger" data-user-action="delete">${iconSvg('delete')}<span class="btn-label">Delete</span></button>` :
                        '<span style="color: var(--muted-text); font-style: italic; font-size: 12px;">Cannot delete yourself</span>'}
                </div>
            `;
        }).join('');
        usersList.querySelectorAll('.user-item').forEach((row, i) => {
            const name = users[i].username;
            row.querySelector('[data-user-action="group"]').addEventListener('change', e => updateUserGroup(name, e.target.value));
            row.querySelector('[data-user-action="admin"]').addEventListener('change', e => updateUserAdmin(name, e.target.checked));
            row.querySelector('[data-user-action="delete"]')?.addEventListener('click', () => deleteUser(name));
        });
    } catch (error) {
        console.error('Failed to load users:', error);
    }
}

async function updateUserGroup(username, group) {
    try {
        await api.updateUser(username, { group: group });
        const messageEl = document.getElementById('users-message');
        messageEl.textContent = `Updated group for ${username}`;
        messageEl.style.color = '#27ae60';
        setTimeout(() => messageEl.textContent = '', 3000);
    } catch (error) {
        alert('Failed to update group: ' + error.message);
        loadUsers();
    }
}

async function updateUserAdmin(username, isAdmin) {
    try {
        await api.updateUser(username, { is_admin: isAdmin });
        const messageEl = document.getElementById('users-message');
        messageEl.textContent = `Updated admin status for ${username}`;
        messageEl.style.color = '#27ae60';
        setTimeout(() => messageEl.textContent = '', 3000);
        loadUsers();
    } catch (error) {
        alert('Failed to update admin status: ' + error.message);
        loadUsers();
    }
}

async function handleCreateUser() {
    const messageEl = document.getElementById('users-message');
    const username = document.getElementById('new-username').value.trim();
    const password = document.getElementById('new-password').value;
    const group = document.getElementById('new-user-group').value;
    const isAdmin = document.getElementById('new-user-admin').checked;
    
    if (!username || !password) {
        messageEl.textContent = 'Username and password are required';
        messageEl.style.color = '#e74c3c';
        return;
    }
    if (!state.availableGroups.includes(group)) {
        messageEl.textContent = 'Choose a group for the user';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    try {
        await api.createUser(username, password, isAdmin, group);
        messageEl.textContent = 'User created successfully';
        messageEl.style.color = '#27ae60';
        
        document.getElementById('new-username').value = '';
        document.getElementById('new-password').value = '';
        document.getElementById('new-user-admin').checked = false;
        renderNewUserGroups(true);
        
        loadUsers();
        
        setTimeout(() => {
            messageEl.textContent = '';
        }, 3000);
    } catch (error) {
        messageEl.textContent = 'Failed to create user: ' + error.message;
        messageEl.style.color = '#e74c3c';
    }
}

async function deleteUser(username) {
    if (!confirm(`Are you sure you want to delete user "${username}"?`)) {
        return;
    }
    
    const messageEl = document.getElementById('users-message');
    
    try {
        await api.deleteUser(username);
        messageEl.textContent = 'User deleted successfully';
        messageEl.style.color = '#27ae60';
        
        loadUsers();
        
        setTimeout(() => {
            messageEl.textContent = '';
        }, 3000);
    } catch (error) {
        messageEl.textContent = 'Failed to delete user: ' + error.message;
        messageEl.style.color = '#e74c3c';
    }
}

async function handleChangePassword() {
    const messageEl = document.getElementById('account-message');
    const currentPassword = document.getElementById('current-user-password').value;
    const newPassword = document.getElementById('new-user-password').value;
    const confirmPassword = document.getElementById('confirm-user-password').value;
    
    if (!currentPassword || !newPassword || !confirmPassword) {
        messageEl.textContent = 'Please fill in all three password fields';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    if (newPassword !== confirmPassword) {
        messageEl.textContent = 'Passwords do not match';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    try {
        await api.changePassword(currentPassword, newPassword);
        messageEl.textContent = 'Password changed successfully';
        messageEl.style.color = '#27ae60';
        
        document.getElementById('current-user-password').value = '';
        document.getElementById('new-user-password').value = '';
        document.getElementById('confirm-user-password').value = '';
        
        setTimeout(() => {
            messageEl.textContent = '';
        }, 3000);
    } catch (error) {
        messageEl.textContent = 'Failed to change password: ' + error.message;
        messageEl.style.color = '#e74c3c';
    }
}

// Sign in on a TV: the code the TV shows signs it in as this account (POST /api/pairing/claim).
async function handleTvPairing() {
    const input = document.getElementById('tv-pairing-code');
    const messageEl = document.getElementById('tv-pairing-message');
    const code = input.value.trim();
    if (!code) {
        messageEl.textContent = 'Enter the code the TV shows';
        messageEl.style.color = '#e74c3c';
        return;
    }
    try {
        await api.request('/api/pairing/claim', {
            method: 'POST',
            body: JSON.stringify({ code }),
        });
        messageEl.textContent = 'The TV is signing in.';
        messageEl.style.color = '#27ae60';
        input.value = '';
    } catch (error) {
        messageEl.textContent = error.message;
        messageEl.style.color = '#e74c3c';
    }
}

// Theme Management: the theme is this browser's (localStorage), picked in Settings, My Account.
const THEMES = [
    ['default', 'Default (Ember)'], ['dark', 'Dark'], ['cinema-blue', 'Cinema Blue'], ['cinema-red', 'Cinema Red'],
    ['teal', 'Teal'], ['orange', 'Orange'], ['brown', 'Brown'], ['purple', 'Purple'], ['gray', 'Gray'],
    ['yellow', 'Yellow'], ['green', 'Green'], ['harbor-lights', 'Harbor Lights'], ['moss-and-rose', 'Moss and Rose'],
    ['desert-bloom', 'Desert Bloom'], ['arctic-berry', 'Arctic Berry'], ['copper-sea', 'Copper Sea'],
    ['neon-arcade', 'Neon Arcade'], ['terminal-green', 'Terminal Green'], ['terminal-amber', 'Terminal Amber'],
];

function applyTheme() {
    const savedTheme = localStorage.getItem('app-theme') || 'default';
    document.documentElement.setAttribute('data-theme', savedTheme);
    document.querySelectorAll('.theme-tile').forEach(tile => {
        tile.setAttribute('aria-checked', String(tile.dataset.theme === savedTheme));
    });
}

// A theme's own colors, read from its :root[data-theme=…] blocks, for drawing its tile
// whatever theme is in use.
function themeColors(name) {
    const vars = {};
    for (const sheet of document.styleSheets) {
        let rules;
        try { rules = sheet.cssRules; } catch (e) { continue; }
        for (const rule of rules) {
            if (rule.selectorText !== `:root[data-theme="${name}"]`) continue;
            for (const prop of rule.style) {
                if (prop.startsWith('--')) vars[prop] = rule.style.getPropertyValue(prop).trim();
            }
        }
    }
    return {
        background: vars['--background-gradient'] || '#f4f4f8',
        box: vars['--surface-color'] || '#ffffff',
        button: vars['--button-bg'] || vars['--primary-color'] || '#667eea',
        buttonText: vars['--button-text'] || '#ffffff',
        accent: vars['--heading-color'] || vars['--primary-color'] || '#667eea',
        text: vars['--text-color'] || '#333333',
    };
}

// The theme picker: a tile per theme, drawn in its colors (background, box, button, accent).
// Picking one applies and keeps it at once.
function renderThemePicker() {
    const picker = document.getElementById('theme-picker');
    if (!picker) return;
    picker.innerHTML = '';
    THEMES.forEach(([name, label]) => {
        const c = themeColors(name);
        const tile = document.createElement('button');
        tile.type = 'button';
        tile.className = 'theme-tile';
        tile.dataset.theme = name;
        tile.setAttribute('role', 'radio');
        tile.innerHTML = `
            <span class="theme-preview" style="background: ${c.background}">
                <span class="theme-preview-box" style="background: ${c.box}">
                    <span class="theme-preview-heading" style="background: ${c.accent}"></span>
                    <span class="theme-preview-line" style="background: ${c.text}"></span>
                    <span class="theme-preview-button" style="background: ${c.button}; color: ${c.buttonText}">Aa</span>
                </span>
            </span>
            <span class="theme-name">${escapeHtml(label)}</span>`;
        tile.addEventListener('click', () => {
            localStorage.setItem('app-theme', name);
            applyTheme();
        });
        picker.appendChild(tile);
    });
    applyTheme();
}


// ---------------------------------------------------------------------------------------------
// Manual metadata matching

const matchState = { target: null, name: '', info: null, episodePick: null, seasonsFor: null, hits: [], posters: null, searchKind: null };

function canFixMatches() {
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    return !!user.is_admin && !!state.libraryType;
}

// A one-line summary of what an item is matched to, shown under its name.
function describeMatch(md) {
    const year = md.year ? ` (${md.year})` : '';
    switch (md.kind) {
        case 'episode': {
            const code = `S${String(md.season).padStart(2, '0')}E${String(md.episode).padStart(2, '0')}`;
            return `${md.show_title} · ${code} · ${md.title}`;
        }
        case 'season':
            return `${md.show_title} · ${md.title}`;
        default:
            return md.title + year;
    }
}

function createMatchLabel(md) {
    if (!md) return null;
    const label = document.createElement('div');
    label.className = 'media-match';
    label.textContent = describeMatch(md);
    if (md.overview) label.title = md.overview;
    return label;
}

function createFixMatchButton(target, name) {
    const btn = document.createElement('button');
    btn.className = 'fix-match-btn';
    btn.type = 'button';
    btn.title = 'Fix match';
    btn.setAttribute('aria-label', `Fix match for ${name}`);
    btn.innerHTML = '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" fill="none" ' +
        'stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">' +
        '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg><span>Fix</span>';
    btn.onclick = (e) => {
        e.stopPropagation();
        openMatchDialog(target, name);
    };
    return btn;
}

function setupMatchDialog() {
    const modal = document.getElementById('match-modal');
    const close = () => {
        modal.classList.remove('active');
        matchState.target = null;
    };
    document.getElementById('match-close').addEventListener('click', close);
    modal.addEventListener('click', (e) => { if (e.target === modal) close(); });
    document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && modal.classList.contains('active')) close();
    });
    // Movie libraries can also search TV shows (TV movies); automatic matching never does.
    document.querySelectorAll('#match-kind button').forEach(btn => btn.addEventListener('click', () => {
        if (matchState.searchKind === btn.dataset.kind) return;
        matchState.searchKind = btn.dataset.kind;
        renderKindSwitch();
        runMatchSearch();
    }));
    document.getElementById('match-search').addEventListener('submit', (e) => {
        e.preventDefault();
        runMatchSearch();
    });
    document.getElementById('match-none').addEventListener('click', async () => {
        if (!confirm('Mark this as having no match? Automatic matching will leave it alone.')) return;
        await applyMatch(0, 'Marked as no match.');
    });
    document.getElementById('match-reset').addEventListener('click', async () => {
        try {
            const singleFile = isSingleFileMatch(matchState.info);
            matchState.info = await api.resetMatch(matchState.target);
            setMatchMessage(singleFile ? 'Match cleared.' : 'Reset. Matching automatically…');
            renderMatchCurrent();
        } catch (error) {
            setMatchMessage('Reset failed: ' + error.message, true);
        }
    });
}

async function openMatchDialog(target, name) {
    matchState.target = target;
    matchState.name = name;
    matchState.info = null;
    matchState.episodePick = null;
    matchState.seasonsFor = null;
    matchState.hits = [];
    matchState.posters = null;
    matchState.searchKind = null;
    document.getElementById('match-kind').hidden = true;
    document.getElementById('match-episode').innerHTML = '';
    document.getElementById('match-posters').innerHTML = '';
    document.getElementById('match-target').textContent = name;
    document.getElementById('match-current').innerHTML = '';
    document.getElementById('match-results').innerHTML = '';
    setMatchMessage('');
    document.getElementById('match-modal').classList.add('active');

    try {
        matchState.info = await api.getMatch(target);
    } catch (error) {
        document.getElementById('match-current').textContent = error.message;
        return;
    }
    document.getElementById('match-query').value = matchState.info.query || '';
    document.getElementById('match-year').value = matchState.info.year || '';
    matchState.searchKind = matchState.info.match_kind || matchState.info.kind;
    renderKindSwitch();
    renderMatchCurrent();
    runMatchSearch();
    document.getElementById('match-query').focus();
}

// The Movies | TV shows switch, shown when the library can match either.
function renderKindSwitch() {
    const box = document.getElementById('match-kind');
    const kinds = (matchState.info && matchState.info.kinds) || [];
    box.hidden = kinds.length < 2;
    box.querySelectorAll('button').forEach(btn => {
        btn.hidden = !kinds.includes(btn.dataset.kind);
        btn.classList.toggle('active', btn.dataset.kind === matchState.searchKind);
        btn.setAttribute('aria-pressed', btn.dataset.kind === matchState.searchKind);
    });
}

async function refreshMatchInfo() {
    try {
        matchState.info = await api.getMatch(matchState.target);
        renderMatchCurrent();
    } catch (error) {
        console.log('Could not refresh match:', error);
    }
}

function isSingleFileMatch(info) {
    return !!info && info.kind === 'tv' && (info.kinds || []).length === 1 && info.kinds[0] === 'movie';
}

function renderMatchCurrent() {
    const info = matchState.info;
    const box = document.getElementById('match-current');
    box.innerHTML = '';
    document.getElementById('match-reset').style.display = info && info.manual ? '' : 'none';
    if (!info) return;
    // A TV-library file that isn't an episode: matched on its own, by hand only.
    const singleFile = isSingleFileMatch(info);
    document.getElementById('match-reset').textContent = singleFile ? 'Clear match' : 'Reset to automatic';

    const status = document.createElement('div');
    status.className = 'match-status';
    const kindLabel = (info.match_kind || info.kind) === 'tv' ? 'TV show' : 'Movie';
    if (info.current) {
        if (info.current.poster) {
            const img = document.createElement('img');
            img.src = API_BASE + info.current.poster;
            img.alt = '';
            box.appendChild(img);
        }
        status.innerHTML = '<span class="match-caption"></span><strong></strong>';
        status.querySelector('.match-caption').textContent = `${kindLabel} matched${info.manual ? ' by hand' : ''}: `;
        status.querySelector('strong').textContent = info.current.title + (info.current.year ? ` (${info.current.year})` : '');
    } else {
        status.textContent = {
            not_found: info.manual ? 'Marked as no match.' : `No ${kindLabel.toLowerCase()} match found.`,
            error: 'The last lookup failed.',
            pending: singleFile ? "Not matched. Files that aren't episodes are only matched by hand." : 'Not matched yet (lookup pending).',
            matched: 'Matched; fetching details…',
        }[info.status] || info.status;
    }
    box.appendChild(status);
    const hint = document.createElement('div');
    hint.className = 'match-hint';
    hint.textContent = singleFile ? 'Choosing a result applies to this file only.'
        : 'Choosing a result applies to every file in this library with the same title.';
    box.appendChild(hint);
    markCurrentHit();
    renderEpisodeSection();
    renderPosterSection();
}

const EPISODE_SOURCES = {
    file: 'from the file name',
    season: 'from a season mapping',
    manual: 'set for this file',
};

function episodeCode(season, episode) {
    return `S${String(season).padStart(2, '0')}E${String(episode).padStart(2, '0')}`;
}

// The "Episode" section: which TMDB episode a TV file shows as, with a picker to change it for
// this file or its whole season.
async function renderEpisodeSection() {
    const box = document.getElementById('match-episode');
    const info = matchState.info;
    const ep = info && info.episode;
    if (!ep) {
        box.innerHTML = '';
        return;
    }
    const show = info.current && info.current.kind === 'show' ? info.current : null;
    box.innerHTML = '';

    const heading = document.createElement('h3');
    heading.textContent = 'Episode';
    box.appendChild(heading);

    const summary = document.createElement('p');
    summary.className = 'match-episode-summary';
    const shownAs = episodeCode(ep.season, ep.episode) + (ep.title ? ` “${ep.title}”` : '');
    summary.textContent = `This file is ${episodeCode(ep.file_season, ep.file_episode)}, shown as ${shownAs} (${EPISODE_SOURCES[ep.source] || ep.source}).`;
    box.appendChild(summary);

    if (!show) {
        const note = document.createElement('p');
        note.className = 'match-hint';
        note.textContent = 'Match the show first to pick a different episode.';
        box.appendChild(note);
        return;
    }

    const controls = document.createElement('div');
    controls.className = 'match-episode-controls';
    const seasonSelect = document.createElement('select');
    seasonSelect.className = 'settings-input';
    seasonSelect.setAttribute('aria-label', 'TMDB season');
    controls.appendChild(seasonSelect);
    if (ep.source === 'manual') {
        controls.appendChild(episodeButton('Clear this file’s fix', () => clearEpisodeMapping(false)));
    }
    if (ep.source === 'season') {
        controls.appendChild(episodeButton(`Clear season ${ep.file_season} mapping`, () => clearEpisodeMapping(true)));
    }
    box.appendChild(controls);

    const list = document.createElement('div');
    list.className = 'match-episode-list';
    box.appendChild(list);
    const actions = document.createElement('div');
    actions.className = 'match-episode-actions';
    box.appendChild(actions);

    let seasons;
    try {
        if (!matchState.seasonsFor || matchState.seasonsFor.show !== show.tmdb_id) {
            matchState.seasonsFor = { show: show.tmdb_id, seasons: await api.matchSeasons(show.tmdb_id) };
        }
        seasons = matchState.seasonsFor.seasons;
    } catch (error) {
        list.textContent = 'Could not load seasons: ' + error.message;
        return;
    }
    seasons.forEach(se => {
        const option = document.createElement('option');
        option.value = se.number;
        option.textContent = `${se.number === 0 ? 'Specials' : 'Season ' + se.number}${se.name && !/^(Season \d+|Specials)$/.test(se.name) ? ' – ' + se.name : ''} (${se.episode_count})`;
        seasonSelect.appendChild(option);
    });
    seasonSelect.value = String(ep.season);
    if (seasonSelect.value !== String(ep.season) && seasons.length) seasonSelect.value = String(seasons[0].number);
    seasonSelect.onchange = () => loadEpisodeList(show.tmdb_id, Number(seasonSelect.value), list, actions);
    loadEpisodeList(show.tmdb_id, Number(seasonSelect.value), list, actions);
}

function episodeButton(label, onClick) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'btn btn-secondary';
    btn.textContent = label;
    btn.onclick = onClick;
    return btn;
}

async function loadEpisodeList(showId, season, list, actions) {
    const ep = matchState.info.episode;
    list.innerHTML = '<p class="match-empty">Loading episodes…</p>';
    actions.innerHTML = '';
    let episodes;
    try {
        episodes = await api.matchEpisodes(showId, season);
    } catch (error) {
        list.textContent = 'Could not load episodes: ' + error.message;
        return;
    }
    list.innerHTML = '';
    if (!episodes.length) {
        list.innerHTML = '<p class="match-empty">No episodes in this season.</p>';
        return;
    }
    episodes.forEach(e => {
        const row = document.createElement('button');
        row.type = 'button';
        row.className = 'match-episode-row';
        if (season === ep.season && e.number === ep.episode) row.classList.add('current');
        row.innerHTML = '<span class="code"></span><span class="name"></span><span class="date"></span>';
        row.querySelector('.code').textContent = episodeCode(season, e.number);
        row.querySelector('.name').textContent = e.name || '';
        row.querySelector('.date').textContent = e.air_date || '';
        row.onclick = () => {
            list.querySelectorAll('.match-episode-row').forEach(r => r.classList.remove('picked'));
            row.classList.add('picked');
            matchState.episodePick = { season, episode: e.number };
            renderEpisodeActions(actions);
        };
        list.appendChild(row);
    });
    const current = list.querySelector('.current');
    if (current) current.scrollIntoView({ block: 'nearest' });
}

// Buttons to apply the picked episode to this file or to every file of its season.
function renderEpisodeActions(actions) {
    const ep = matchState.info.episode;
    const pick = matchState.episodePick;
    actions.innerHTML = '';
    const shift = pick.episode - ep.file_episode;
    const shiftText = shift === 0 ? 'same episode numbers' : `episode numbers ${shift > 0 ? '+' : ''}${shift}`;
    actions.appendChild(episodeButton(`Use ${episodeCode(pick.season, pick.episode)} for this file`,
        () => applyEpisodeMapping(false)));
    const whole = episodeButton(`Use for all of season ${ep.file_season} (→ season ${pick.season}, ${shiftText})`,
        () => applyEpisodeMapping(true));
    whole.className = 'btn';
    actions.appendChild(whole);
}

async function applyEpisodeMapping(wholeSeason) {
    const pick = matchState.episodePick;
    try {
        matchState.info = await api.setEpisodeMapping(matchState.target.path, pick.season, pick.episode, wholeSeason);
        setMatchMessage(wholeSeason ? 'Season mapped. Fetching episode details…' : 'Episode set for this file.');
        matchState.episodePick = null;
        renderMatchCurrent();
    } catch (error) {
        setMatchMessage('Could not save: ' + error.message, true);
    }
}

async function clearEpisodeMapping(wholeSeason) {
    try {
        matchState.info = await api.clearEpisodeMapping(matchState.target.path, wholeSeason);
        setMatchMessage('Episode mapping cleared.');
        renderMatchCurrent();
    } catch (error) {
        setMatchMessage('Could not clear: ' + error.message, true);
    }
}

// Highlights the result that is the current match (its details may arrive after the search).
function markCurrentHit() {
    const info = matchState.info;
    const currentId = info && info.current && info.current.tmdb_id;
    const currentKind = info && (info.match_kind || info.kind);
    document.querySelectorAll('.match-hit').forEach(row => {
        const isCurrent = Number(row.dataset.tmdbId) === currentId && row.dataset.kind === currentKind;
        row.classList.toggle('current', isCurrent);
        row.title = isCurrent ? 'Current match' : 'Use this match';
        row.querySelector('.current-tag').hidden = !isCurrent;
    });
}

async function runMatchSearch() {
    const query = document.getElementById('match-query').value.trim();
    const year = document.getElementById('match-year').value.trim();
    const results = document.getElementById('match-results');
    if (!query || !matchState.info) return;
    const kind = matchState.searchKind || matchState.info.kind;
    results.innerHTML = '<p class="match-empty">Searching…</p>';
    let hits;
    try {
        hits = await api.searchMatch(kind, query, year);
        if (kind !== (matchState.searchKind || matchState.info.kind)) return; // switched meanwhile
        hits.forEach(h => { h.kind = kind; });
    } catch (error) {
        results.innerHTML = '';
        setMatchMessage('Search failed: ' + error.message, true);
        return;
    }
    results.innerHTML = '';
    matchState.hits = hits;
    renderPosterSection();
    if (hits.length === 0) {
        results.innerHTML = `<p class="match-empty">No results${year ? ' (try without the year)' : ''}.</p>`;
        return;
    }
    hits.forEach(hit => results.appendChild(createMatchHit(hit)));
    markCurrentHit();
}

function createMatchHit(hit) {
    const row = document.createElement('button');
    row.type = 'button';
    row.className = 'match-hit';
    row.dataset.tmdbId = hit.tmdb_id;
    row.dataset.kind = hit.kind;

    const poster = document.createElement('div');
    poster.className = 'match-hit-poster';
    if (hit.poster) {
        const img = document.createElement('img');
        img.src = hit.poster;
        img.alt = '';
        img.loading = 'lazy';
        poster.appendChild(img);
    }
    const text = document.createElement('div');
    text.className = 'match-hit-text';
    const title = document.createElement('div');
    title.className = 'match-hit-title';
    title.textContent = hit.title + (hit.year ? ` (${hit.year})` : '');
    const tag = document.createElement('span');
    tag.className = 'current-tag';
    tag.textContent = ' — current';
    tag.hidden = true;
    title.appendChild(tag);
    const overview = document.createElement('div');
    overview.className = 'match-hit-overview';
    overview.textContent = hit.overview || 'No synopsis.';
    text.appendChild(title);
    text.appendChild(overview);
    row.appendChild(poster);
    row.appendChild(text);
    row.onclick = () => applyMatch(hit.tmdb_id, `Matched to ${hit.title}. Fetching details…`, hit.kind);
    return row;
}

async function applyMatch(tmdbId, message, kind) {
    try {
        matchState.info = await api.setMatch(matchState.target, tmdbId, kind);
        setMatchMessage(message);
        renderMatchCurrent();
    } catch (error) {
        setMatchMessage('Could not save: ' + error.message, true);
    }
}

// The "Poster" section: any poster can be picked for the matched title, separately from the
// match itself: the title's own TMDB posters (every language), or another search result's.
async function renderPosterSection() {
    const box = document.getElementById('match-posters');
    const current = matchState.info && matchState.info.current;
    if (!current) {
        box.innerHTML = '';
        return;
    }
    const target = matchState.target;
    if (!matchState.posters || matchState.posters.tmdbId !== current.tmdb_id) {
        matchState.posters = { tmdbId: current.tmdb_id, list: null };
        box.innerHTML = '<h3>Poster</h3><p class="match-hint">Loading posters…</p>';
        try {
            const list = await api.getMatchPosters(target);
            if (matchState.target !== target || !matchState.posters || matchState.posters.tmdbId !== current.tmdb_id) return;
            matchState.posters.list = list;
        } catch (error) {
            if (matchState.target !== target) return;
            matchState.posters = null;
            box.innerHTML = '<h3>Poster</h3>';
            const note = document.createElement('p');
            note.className = 'match-hint';
            note.textContent = 'Posters unavailable: ' + error.message;
            box.appendChild(note);
            return;
        }
    }
    const list = matchState.posters.list;
    if (!list) return; // still loading

    box.innerHTML = '';
    const header = document.createElement('div');
    header.className = 'match-posters-header';
    const heading = document.createElement('h3');
    heading.textContent = 'Poster';
    header.appendChild(heading);
    const note = document.createElement('span');
    note.className = 'match-hint';
    note.textContent = list.manual ? 'Picked by hand.' : "TMDB's default.";
    header.appendChild(note);
    if (list.manual) {
        const reset = document.createElement('button');
        reset.type = 'button';
        reset.className = 'btn btn-secondary btn-small';
        reset.textContent = 'Use default';
        reset.onclick = () => applyPoster('', 'Poster reset to the default.');
        header.appendChild(reset);
    }
    box.appendChild(header);

    const own = list.posters.map(p => ({ path: p.path, url: p.url, label: p.language ? p.language.toUpperCase() : 'No text' }));
    box.appendChild(createPosterStrip(`${current.title}'s posters (${own.length})`, own, list.current,
        'TMDB has no posters for this title.'));

    const others = matchState.hits
        .filter(h => !(h.tmdb_id === current.tmdb_id && h.kind === (matchState.info.match_kind || matchState.info.kind)) && h.poster_path)
        .map(h => ({ path: h.poster_path, url: h.poster, label: h.title + (h.year ? ` (${h.year})` : '') }));
    if (others.length) box.appendChild(createPosterStrip('From other search results', others, list.current));
}

function createPosterStrip(title, posters, currentPath, emptyText) {
    const section = document.createElement('div');
    const caption = document.createElement('div');
    caption.className = 'match-posters-caption';
    caption.textContent = title;
    section.appendChild(caption);
    if (!posters.length) {
        const empty = document.createElement('p');
        empty.className = 'match-hint';
        empty.textContent = emptyText || '';
        section.appendChild(empty);
        return section;
    }
    const strip = document.createElement('div');
    strip.className = 'match-poster-strip';
    posters.forEach(p => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'match-poster' + (p.path === currentPath ? ' current' : '');
        btn.title = p.path === currentPath ? 'Current poster' : `Use this poster (${p.label})`;
        const img = document.createElement('img');
        img.src = p.url;
        img.alt = '';
        img.loading = 'lazy';
        const label = document.createElement('span');
        label.textContent = p.label;
        btn.appendChild(img);
        btn.appendChild(label);
        btn.onclick = () => applyPoster(p.path, 'Poster changed.');
        strip.appendChild(btn);
    });
    section.appendChild(strip);
    return section;
}

async function applyPoster(path, message) {
    try {
        matchState.info = await api.setMatchPoster(matchState.target, path);
        matchState.posters = null; // reload to show the new current poster
        setMatchMessage(message);
        renderMatchCurrent();
    } catch (error) {
        setMatchMessage('Could not change the poster: ' + error.message, true);
    }
}

function setMatchMessage(text, isError = false) {
    const el = document.getElementById('match-message');
    el.textContent = text;
    el.classList.toggle('error', isError);
}

// ---------------------------------------------------------------------------------------------
// Library view: movie and tv libraries as posters instead of folders

function getLibraryMode(libId) {
    try {
        return localStorage.getItem(`library_mode:${libId}`) || 'library';
    } catch (e) {
        return 'library';
    }
}

function setLibraryMode(libId, mode) {
    try {
        localStorage.setItem(`library_mode:${libId}`, mode);
    } catch (e) { /* not remembered */ }
}

// Shows the folders grid or the library container, and the type tabs only for folders.
function setView(view) {
    if (view !== 'title') stopThemeMusic();
    state.view = view;
    const library = view !== 'folders';
    document.getElementById('library-view').hidden = !library;
    mediaGrid.hidden = library;
    directoryList.hidden = library;
    document.querySelector('.tabs').hidden = library;
    setAzIndex([]); // each view sets its own once it has rendered
}

// The A–Z bar: a letter per run of items (folders then files; titles then unmatched files), in
// the order shown. Clicking or dragging along it scrolls to that letter's first item; otherwise
// it marks the letter at the top of the window.
let azEntries = [];
const azBar = document.getElementById('az-bar');

function azLabel(name, skipArticles = false) {
    let n = name.trim();
    if (!n || n === '..') return null;
    if (skipArticles) {
        const article = ['the ', 'a ', 'an '].find(a => n.toLowerCase().startsWith(a) && n.length > a.length);
        if (article) n = n.slice(article.length).trimStart();
    }
    const c = n[0];
    return c.toLowerCase() !== c.toUpperCase() ? c.toUpperCase() : '#';
}

function setAzIndex(items) {
    const seen = new Set();
    azEntries = items.filter(i => i.label && !seen.has(i.run + i.label) && seen.add(i.run + i.label));
    azBar.innerHTML = '';
    azBar.hidden = azEntries.length < 2;
    document.body.classList.toggle('has-az', !azBar.hidden);
    if (azBar.hidden) return;
    azEntries.forEach((e, i) => {
        if (i > 0 && e.run !== azEntries[i - 1].run) azBar.appendChild(Object.assign(document.createElement('span'), { className: 'az-divider' }));
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'az-letter';
        b.textContent = e.label;
        b.dataset.index = i;
        e.button = b;
        azBar.appendChild(b);
    });
    azBar.appendChild(Object.assign(document.createElement('span'), { className: 'az-bubble', hidden: true }));
    markAzLetter();
}

// The page's own top: below the sticky parts, if any.
function azScrollTo(entry) {
    const top = entry.el.getBoundingClientRect().top + window.scrollY - 16;
    window.scrollTo({ top: Math.max(0, top) });
}

function markAzLetter(current) {
    if (current === undefined) {
        // The last letter whose first item starts at or above the top of the window.
        current = 0;
        azEntries.forEach((e, i) => { if (e.el.getBoundingClientRect().top <= 80) current = i; });
    }
    azEntries.forEach((e, i) => e.button.classList.toggle('current', i === current));
}

let azPointer = null;
function azTouch(event) {
    const button = document.elementFromPoint(azBar.getBoundingClientRect().left + azBar.clientWidth / 2, event.clientY)
        ?.closest('.az-letter');
    if (!button) return;
    const i = Number(button.dataset.index);
    const bubble = azBar.querySelector('.az-bubble');
    bubble.textContent = azEntries[i].label;
    bubble.hidden = false;
    bubble.style.top = (button.offsetTop + button.offsetHeight / 2) + 'px';
    if (azPointer === i) return;
    azPointer = i;
    azScrollTo(azEntries[i]);
    markAzLetter(i);
}
azBar.addEventListener('pointerdown', e => {
    if (e.button !== 0) return;
    e.preventDefault();
    azBar.setPointerCapture(e.pointerId);
    azBar.classList.add('dragging');
    azPointer = null;
    azTouch(e);
});
azBar.addEventListener('pointermove', e => { if (azBar.classList.contains('dragging')) azTouch(e); });
const azRelease = () => {
    azBar.classList.remove('dragging');
    const bubble = azBar.querySelector('.az-bubble');
    if (bubble) bubble.hidden = true;
};
azBar.addEventListener('pointerup', azRelease);
azBar.addEventListener('pointercancel', azRelease);
// Keyboard: Enter or Space on a letter jumps there.
azBar.addEventListener('click', e => {
    const button = e.target.closest('.az-letter');
    if (button && e.detail === 0) azScrollTo(azEntries[Number(button.dataset.index)]);
});
let azScrollFrame = 0;
window.addEventListener('scroll', () => {
    if (azBar.hidden || azBar.classList.contains('dragging') || azScrollFrame) return;
    azScrollFrame = requestAnimationFrame(() => { azScrollFrame = 0; markAzLetter(); });
}, { passive: true });

function refreshCurrentView() {
    if (state.view === 'library') showLibrary(state.libraryId, false, true);
    else if (state.view === 'title') showTitle(state.libraryId, state.titleRef.kind, state.titleRef.id, false, true);
    else if (state.view === 'collections') showCollections(state.libraryId, false, true);
    else if (state.view === 'collection') showCollection(state.libraryId, state.collectionId, false, true);
    else if (state.view === 'tmdbcollections') showTmdbCollections(state.libraryId, false, true);
    else if (state.view === 'tmdbcollection') showTmdbCollection(state.libraryId, state.collectionId, false, true);
    else loadMedia(false);
}

// Scroll restoration for Back/Forward (see the popstate handler).
let scrollSaveTimer = null;
let pendingScroll = null;

function rememberScroll() {
    clearTimeout(scrollSaveTimer);
    if (history.state) history.replaceState({ ...history.state, scrollY: window.scrollY }, '');
}

function restorePendingScroll() {
    if (pendingScroll === null) return;
    const y = pendingScroll;
    pendingScroll = null;
    requestAnimationFrame(() => window.scrollTo(0, y));
}

function pushLibraryHistory(params) {
    const url = new URL(window.location);
    url.search = '';
    Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
    rememberScroll();
    window.history.pushState(params, '', url);
}

// Folders | Library switch, shown inside movie and tv libraries.
function createViewToggle() {
    const toggle = document.createElement('div');
    toggle.className = 'view-toggle';
    toggle.setAttribute('role', 'group');
    toggle.setAttribute('aria-label', 'View');
    // Movie libraries also show their films by collection (series).
    const modes = [['folders', 'Folders'], ['library', 'Library']];
    if (state.libraryType === 'movie') modes.push(['collections', 'Collections'], ['tmdb', 'TMDB Collections']);
    const viewMode = { folders: 'folders', library: 'library', title: 'library', collections: 'collections', collection: 'collections',
                       tmdbcollections: 'tmdb', tmdbcollection: 'tmdb' };
    modes.forEach(([mode, label]) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        const active = viewMode[state.view] === mode;
        btn.innerHTML = (active ? iconSvg('check') : '') + `<span>${label}</span>`; // a check on the current view
        btn.className = active ? 'active' : '';
        btn.setAttribute('aria-pressed', String(active));
        btn.onclick = () => {
            if (active) return; // already showing
            setLibraryMode(state.libraryId, mode);
            if (mode === 'library') {
                showLibrary(state.libraryId);
            } else if (mode === 'collections') {
                showCollections(state.libraryId);
            } else if (mode === 'tmdb') {
                showTmdbCollections(state.libraryId);
            } else {
                state.currentPath = state.libraryId;
                loadMedia();
            }
        };
        toggle.appendChild(btn);
    });
    return toggle;
}

// The bar: the view switch, Home / the library / (a collection, or Collections) / the title.
function libraryBreadcrumb(titleName, back = null) {
    breadcrumb.innerHTML = '';
    breadcrumb.appendChild(createViewToggle());
    const home = document.createElement('a');
    home.href = '#';
    home.textContent = 'Home';
    home.onclick = (e) => {
        e.preventDefault();
        state.currentPath = '';
        loadMedia();
    };
    breadcrumb.appendChild(home);
    breadcrumb.appendChild(document.createTextNode(' / '));
    const lib = document.createElement('a');
    lib.href = '#';
    lib.textContent = state.libraryName || 'Library';
    lib.onclick = (e) => {
        e.preventDefault();
        if (back) back(); else showLibrary(state.libraryId);
    };
    breadcrumb.appendChild(lib);
    if (titleName) {
        breadcrumb.appendChild(document.createTextNode(' / '));
        const span = document.createElement('span');
        span.textContent = titleName;
        breadcrumb.appendChild(span);
    }
}

// keepScroll re-renders in place (after a metadata update) instead of jumping to the top.
async function showLibrary(libId, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('library');
    state.libraryId = libId;
    state.titleRef = null;
    if (push) pushLibraryHistory({ view: 'library', lib: libId });
    if (!keepScroll) container.innerHTML = '<p class="library-empty">Loading…</p>';

    let data;
    try {
        data = await api.getLibrary(libId);
    } catch (error) {
        container.textContent = 'Could not load library: ' + error.message;
        return;
    }
    state.libraryType = data.library_type;
    state.libraryName = data.library_name;
    state.mediaItems = data.unmatched.map(libraryFileItem);
    libraryBreadcrumb(null);

    const [continueRow, newRow] = await Promise.all([createContinueRow(libId), createNewAdditionsRow(libId)]);
    container.innerHTML = '';
    if (continueRow) container.appendChild(continueRow);
    if (newRow) container.appendChild(newRow);
    const grid = document.createElement('div');
    grid.className = 'poster-grid';
    data.titles.forEach(t => grid.appendChild(createPosterCard(t)));
    if (!data.titles.length) {
        container.innerHTML = '<p class="library-empty">Nothing matched yet. Titles appear here as they are looked up.</p>';
    }
    container.appendChild(grid);

    if (data.unmatched.length) {
        const heading = document.createElement('h2');
        heading.className = 'library-section';
        heading.textContent = `Unmatched (${data.unmatched.length})`;
        container.appendChild(heading);
        const unmatched = document.createElement('div');
        unmatched.className = 'poster-grid';
        data.unmatched.forEach(f => unmatched.appendChild(createUnmatchedCard(f)));
        container.appendChild(unmatched);
    }
    // Titles' letters (as the server sorts them, without "The"), then the unmatched files'.
    const cards = container.querySelectorAll('.poster-card');
    setAzIndex([...cards].map(el => ({
        el, label: azLabel(el.querySelector('.poster-title')?.textContent || '', true),
        run: el.classList.contains('unmatched') ? 1 : 0,
    })));
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

// Collections view (movie libraries): the collections admins made, each a collage of its first
// four posters; admins create, edit and delete them, and add the library's movies to them.

function isAdminUser() {
    try {
        return !!JSON.parse(localStorage.getItem('user') || '{}').is_admin;
    } catch (e) {
        return false;
    }
}

// A collage of up to four posters: one fills it, two side by side, three one tall and two stacked, four 2×2.
function createCollage(posters) {
    const box = document.createElement('div');
    box.className = `poster collage collage-${Math.min(posters.length, 4)}`;
    if (!posters.length) {
        box.classList.add('no-poster');
        box.innerHTML = placeholderIcon('movie');
        return box;
    }
    posters.slice(0, 4).forEach(src => {
        const img = document.createElement('img');
        img.src = API_BASE + src;
        img.alt = '';
        img.loading = 'lazy';
        box.appendChild(img);
    });
    return box;
}

async function showCollections(libId, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('collections');
    state.libraryId = libId;
    state.titleRef = null;
    if (push) pushLibraryHistory({ view: 'collections', lib: libId });
    if (!keepScroll) container.innerHTML = '<p class="library-empty">Loading…</p>';
    let data;
    try {
        data = await api.getLibrary(libId);
    } catch (error) {
        container.textContent = 'Could not load library: ' + error.message;
        return;
    }
    state.libraryType = data.library_type;
    state.libraryName = data.library_name;
    state.libraryTitles = data.titles;
    state.userCollections = data.user_collections || [];
    state.mediaItems = [];
    libraryBreadcrumb(null, () => showCollections(libId));
    container.innerHTML = '';
    if (isAdminUser()) {
        const bar = document.createElement('div');
        bar.className = 'collection-actions';
        const add = document.createElement('button');
        add.type = 'button';
        add.className = 'btn';
        add.textContent = 'New collection';
        add.onclick = () => openCollectionEditor(null, []);
        bar.appendChild(add);
        container.appendChild(bar);
    }
    if (!state.userCollections.length) {
        const empty = document.createElement('p');
        empty.className = 'library-empty';
        empty.textContent = isAdminUser() ? 'No collections yet. Make one with New collection, or from a movie’s page.'
            : 'No collections yet.';
        container.appendChild(empty);
        return;
    }
    const grid = document.createElement('div');
    grid.className = 'poster-grid';
    state.userCollections.forEach(c => {
        const card = document.createElement('button');
        card.type = 'button';
        card.className = 'poster-card';
        card.title = c.name;
        card.appendChild(createCollage(c.posters || []));
        const title = document.createElement('div');
        title.className = 'poster-title';
        title.textContent = c.name;
        const count = document.createElement('div');
        count.className = 'poster-year';
        count.textContent = `${c.count} movie${c.count === 1 ? '' : 's'}`;
        card.appendChild(title);
        card.appendChild(count);
        card.onclick = () => showCollection(libId, c.id);
        grid.appendChild(card);
    });
    container.appendChild(grid);
    const cards = container.querySelectorAll('.poster-card');
    setAzIndex([...cards].map(el => ({ el, label: azLabel(el.querySelector('.poster-title')?.textContent || '', true), run: 0 })));
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

// A made collection's page: its name, description and movies by release date; admins can edit
// it (name, description and movies), take a movie out, or delete it.
async function showCollection(libId, id, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('collection');
    state.libraryId = libId;
    state.collectionId = id;
    state.titleRef = null;
    if (push) pushLibraryHistory({ view: 'collection', lib: libId, id });
    if (!keepScroll) {
        container.innerHTML = '<p class="library-empty">Loading…</p>';
        window.scrollTo(0, 0);
    }
    let c;
    try {
        if (!state.libraryName || state.libraryType !== 'movie' || !state.libraryTitles) {
            const lib = await api.getLibrary(libId);
            state.libraryName = lib.library_name;
            state.libraryType = lib.library_type;
            state.libraryTitles = lib.titles;
        }
        c = await api.getUserCollection(libId, id);
    } catch (error) {
        container.textContent = 'Could not load collection: ' + error.message;
        return;
    }
    state.mediaItems = [];
    libraryBreadcrumb(c.name, () => showCollections(libId));
    container.innerHTML = '';
    const head = document.createElement('div');
    head.className = 'collection-head';
    const name = document.createElement('h2');
    name.className = 'library-section';
    name.textContent = c.name;
    head.appendChild(name);
    const count = document.createElement('p');
    count.className = 'collection-count';
    count.textContent = `${c.parts.length} movie${c.parts.length === 1 ? '' : 's'}`;
    head.appendChild(count);
    if (c.description) {
        const description = document.createElement('p');
        description.className = 'collection-overview';
        description.textContent = c.description;
        head.appendChild(description);
    }
    if (isAdminUser()) {
        const actions = document.createElement('div');
        actions.className = 'collection-actions';
        const edit = document.createElement('button');
        edit.type = 'button';
        edit.className = 'btn';
        edit.textContent = 'Edit';
        edit.onclick = () => openCollectionEditor(c, c.parts.map(t => ({ kind: t.kind, tmdb_id: t.tmdb_id })));
        const del = document.createElement('button');
        del.type = 'button';
        del.className = 'btn btn-danger';
        del.textContent = 'Delete collection';
        del.onclick = async () => {
            if (!confirm(`Delete the collection “${c.name}”? Its movies stay in the library.`)) return;
            try {
                await api.deleteUserCollection(libId, c.id);
                showCollections(libId);
            } catch (error) {
                alert('Could not delete it: ' + error.message);
            }
        };
        actions.appendChild(edit);
        actions.appendChild(del);
        head.appendChild(actions);
    }
    container.appendChild(head);
    if (!c.parts.length) {
        const empty = document.createElement('p');
        empty.className = 'library-empty';
        empty.textContent = 'No movies in it yet.';
        container.appendChild(empty);
    }
    const grid = document.createElement('div');
    grid.className = 'poster-grid';
    c.parts.forEach(t => {
        const card = createPosterCard(t);
        if (isAdminUser()) {
            // Take it out: a small × over the poster
            const remove = document.createElement('span');
            remove.className = 'collection-remove';
            remove.setAttribute('role', 'button');
            remove.tabIndex = 0;
            remove.title = 'Remove from this collection';
            remove.setAttribute('aria-label', `Remove ${t.title} from this collection`);
            remove.innerHTML = iconSvg('close');
            const take = async (e) => {
                e.stopPropagation();
                try {
                    await api.updateUserCollection(libId, c.id, { remove: [{ kind: t.kind, tmdb_id: t.tmdb_id }] });
                    showCollection(libId, c.id, false, true);
                } catch (error) {
                    alert('Could not remove it: ' + error.message);
                }
            };
            remove.onclick = take;
            remove.onkeydown = (e) => { if (e.key === 'Enter' || e.key === ' ') take(e); };
            card.querySelector('.poster').appendChild(remove);
        }
        grid.appendChild(card);
    });
    container.appendChild(grid);
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

// The collection editor (admins): a name, an optional description and the library's movies to
// tick, with a filter. collection null makes a new one; checked are the titles ticked to start.
// done(collection) runs after saving (by default, the collection's page opens).
function openCollectionEditor(collection, checked, done) {
    const titles = (state.libraryTitles || []).slice();
    const ticked = new Set(checked.map(t => `${t.kind}:${t.tmdb_id}`));
    const start = new Set(ticked);
    const modal = document.createElement('div');
    modal.className = 'modal active';
    modal.innerHTML = `<div class="modal-content collection-editor" role="dialog" aria-modal="true">
        <span class="match-close" aria-label="Close" role="button" tabindex="0">&times;</span>
        <h2></h2>
        <label>Name<input class="settings-input" name="name" maxlength="200" required></label>
        <label>Description (optional)<textarea class="settings-input" name="description" rows="3" maxlength="4000"></textarea></label>
        <div class="collection-picker-head"><strong>Movies</strong><span class="collection-picked"></span>
            <input class="settings-input" name="filter" type="search" placeholder="Filter"></div>
        <div class="collection-picker"></div>
        <p class="collection-message"></p>
        <div class="collection-buttons"><button type="button" class="btn btn-secondary" data-act="cancel">Cancel</button>
            <button type="button" class="btn" data-act="save">Save</button></div>
    </div>`;
    const $ = sel => modal.querySelector(sel);
    $('h2').textContent = collection ? 'Edit collection' : 'New collection';
    $('[name=name]').value = collection ? collection.name : '';
    $('[name=description]').value = collection ? (collection.description || '') : '';
    const picker = $('.collection-picker');
    const picked = () => { $('.collection-picked').textContent = `${ticked.size} chosen`; };
    titles.forEach(t => {
        const key = `${t.kind}:${t.tmdb_id}`;
        const row = document.createElement('label');
        row.className = 'collection-pick';
        row.dataset.name = (t.title || '').toLowerCase();
        const box = document.createElement('input');
        box.type = 'checkbox';
        box.checked = ticked.has(key);
        box.onchange = () => { if (box.checked) ticked.add(key); else ticked.delete(key); picked(); };
        const text = document.createElement('span');
        text.textContent = t.title + (t.year ? ` (${t.year})` : '');
        row.appendChild(box);
        row.appendChild(text);
        picker.appendChild(row);
    });
    picked();
    $('[name=filter]').oninput = (e) => {
        const q = e.target.value.trim().toLowerCase();
        picker.querySelectorAll('.collection-pick').forEach(r => { r.hidden = q && !r.dataset.name.includes(q); });
    };
    const close = () => modal.remove();
    $('.match-close').onclick = close;
    $('[data-act=cancel]').onclick = close;
    modal.addEventListener('click', e => { if (e.target === modal) close(); });
    modal.addEventListener('keydown', e => { if (e.key === 'Escape') close(); });
    const toTitle = key => { const [kind, id] = key.split(':'); return { kind, tmdb_id: Number(id) }; };
    $('[data-act=save]').onclick = async () => {
        const name = $('[name=name]').value.trim();
        const description = $('[name=description]').value.trim();
        if (!name) {
            $('.collection-message').textContent = 'A collection needs a name.';
            $('[name=name]').focus();
            return;
        }
        try {
            let saved;
            if (collection) {
                saved = await api.updateUserCollection(state.libraryId, collection.id, {
                    name, description,
                    add: [...ticked].filter(k => !start.has(k)).map(toTitle),
                    remove: [...start].filter(k => !ticked.has(k)).map(toTitle),
                });
            } else {
                saved = await api.createUserCollection(state.libraryId, name, description, [...ticked].map(toTitle));
            }
            close();
            if (done) done(saved); else showCollection(state.libraryId, saved.id, !collection);
        } catch (error) {
            $('.collection-message').textContent = 'Could not save: ' + error.message;
        }
    };
    document.body.appendChild(modal);
    $('[name=name]').focus();
}

// A title page's "Add to collection" (admins, movie libraries): tick the collections it's in, or make a new one.
async function openAddToCollection(t) {
    let data;
    try {
        data = await api.getLibrary(state.libraryId);
    } catch (error) {
        alert('Could not load the collections: ' + error.message);
        return;
    }
    state.libraryTitles = data.titles;
    const me = { kind: t.kind, tmdb_id: t.tmdb_id };
    const modal = document.createElement('div');
    modal.className = 'modal active';
    modal.innerHTML = `<div class="modal-content collection-editor" role="dialog" aria-modal="true">
        <span class="match-close" aria-label="Close" role="button" tabindex="0">&times;</span>
        <h2>Add to collection</h2><p class="match-target"></p>
        <div class="collection-picker"></div><p class="collection-message"></p>
        <div class="collection-buttons"><button type="button" class="btn btn-secondary" data-act="new">New collection…</button>
            <button type="button" class="btn" data-act="done">Done</button></div></div>`;
    const $ = sel => modal.querySelector(sel);
    $('.match-target').textContent = t.title;
    const picker = $('.collection-picker');
    const cols = data.user_collections || [];
    if (!cols.length) picker.innerHTML = '<p class="library-empty">No collections yet.</p>';
    cols.forEach(c => {
        const row = document.createElement('label');
        row.className = 'collection-pick';
        const box = document.createElement('input');
        box.type = 'checkbox';
        box.checked = (c.titles || []).some(x => x.kind === me.kind && x.tmdb_id === me.tmdb_id);
        box.onchange = async () => {
            try {
                await api.updateUserCollection(state.libraryId, c.id, box.checked ? { add: [me] } : { remove: [me] });
                $('.collection-message').textContent = box.checked ? `Added to ${c.name}.` : `Removed from ${c.name}.`;
            } catch (error) {
                box.checked = !box.checked;
                $('.collection-message').textContent = 'Could not save: ' + error.message;
            }
        };
        const text = document.createElement('span');
        text.textContent = `${c.name} (${c.count})`;
        row.appendChild(box);
        row.appendChild(text);
        picker.appendChild(row);
    });
    const close = () => modal.remove();
    $('.match-close').onclick = close;
    $('[data-act=done]').onclick = close;
    modal.addEventListener('click', e => { if (e.target === modal) close(); });
    modal.addEventListener('keydown', e => { if (e.key === 'Escape') close(); });
    $('[data-act=new]').onclick = () => {
        close();
        openCollectionEditor(null, [me], () => {}); // stays on the movie's page
    };
    document.body.appendChild(modal);
}

// TMDB Collections view (movie libraries): the TMDB collections (film series) the library's movies
// belong to, each opening its page.
async function showTmdbCollections(libId, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('tmdbcollections');
    state.libraryId = libId;
    state.titleRef = null;
    if (push) pushLibraryHistory({ view: 'tmdbcollections', lib: libId });
    if (!keepScroll) container.innerHTML = '<p class="library-empty">Loading…</p>';

    let data;
    try {
        data = await api.getLibrary(libId);
    } catch (error) {
        container.textContent = 'Could not load library: ' + error.message;
        return;
    }
    state.libraryType = data.library_type;
    state.libraryName = data.library_name;
    state.mediaItems = [];
    libraryBreadcrumb(null, () => showTmdbCollections(libId));
    const collections = data.collections || [];
    container.innerHTML = '';
    if (!collections.length) {
        container.innerHTML = '<p class="library-empty">No movies in collections yet. Collections appear here as movies are looked up.</p>';
        return;
    }
    const grid = document.createElement('div');
    grid.className = 'poster-grid';
    collections.forEach(c => grid.appendChild(createTmdbCollectionCard(c)));
    container.appendChild(grid);
    const cards = container.querySelectorAll('.poster-card');
    setAzIndex([...cards].map(el => ({ el, label: azLabel(el.querySelector('.poster-title')?.textContent || '', true), run: 0 })));
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

function createTmdbCollectionCard(c) {
    const card = document.createElement('button');
    card.type = 'button';
    card.className = 'poster-card';
    card.title = c.name;
    const poster = document.createElement('div');
    poster.className = 'poster';
    if (c.poster) {
        const img = document.createElement('img');
        img.src = API_BASE + c.poster;
        img.alt = '';
        img.loading = 'lazy';
        poster.appendChild(img);
    } else {
        poster.classList.add('no-poster');
        poster.innerHTML = placeholderIcon('movie');
    }
    const title = document.createElement('div');
    title.className = 'poster-title';
    title.textContent = c.name;
    const count = document.createElement('div');
    count.className = 'poster-year';
    count.textContent = `${c.count} of ${c.total} film${c.total === 1 ? '' : 's'}`;
    card.appendChild(poster);
    card.appendChild(title);
    card.appendChild(count);
    card.onclick = () => showTmdbCollection(state.libraryId, c.id);
    return card;
}

// A collection's page: every film TMDB lists in it, by release date; those the library doesn't
// have are dimmed and open nothing.
async function showTmdbCollection(libId, id, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('tmdbcollection');
    state.libraryId = libId;
    state.collectionId = id;
    state.titleRef = null;
    if (push) pushLibraryHistory({ view: 'tmdbcollection', lib: libId, id });
    if (!keepScroll) {
        container.innerHTML = '<p class="library-empty">Loading…</p>';
        window.scrollTo(0, 0);
    }
    let c;
    try {
        if (!state.libraryName || state.libraryType !== 'movie') {
            const lib = await api.getLibrary(libId);
            state.libraryName = lib.library_name;
            state.libraryType = lib.library_type;
        }
        c = await api.getLibraryCollection(libId, id);
    } catch (error) {
        container.textContent = 'Could not load collection: ' + error.message;
        return;
    }
    state.mediaItems = [];
    libraryBreadcrumb(c.name, () => showTmdbCollections(libId));
    container.innerHTML = '';
    const head = document.createElement('div');
    head.className = 'collection-head';
    const name = document.createElement('h2');
    name.className = 'library-section';
    name.textContent = c.name;
    head.appendChild(name);
    const have = c.parts.filter(p => !p.missing).length;
    const count = document.createElement('p');
    count.className = 'collection-count';
    count.textContent = `${have} of ${c.parts.length} film${c.parts.length === 1 ? '' : 's'} in this library`;
    head.appendChild(count);
    if (c.overview) {
        const overview = document.createElement('p');
        overview.className = 'collection-overview';
        overview.textContent = c.overview;
        head.appendChild(overview);
    }
    container.appendChild(head);
    const grid = document.createElement('div');
    grid.className = 'poster-grid';
    c.parts.forEach(t => {
        const card = createPosterCard(t);
        if (t.missing) {
            card.classList.add('missing');
            card.disabled = true;
            card.title += ' — not in this library';
            card.querySelector('.poster-year').textContent = [t.year, 'Not in this library'].filter(Boolean).join(' · ');
        }
        grid.appendChild(card);
    });
    container.appendChild(grid);
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

function createPosterCard(t) {
    const card = document.createElement('button');
    card.type = 'button';
    card.className = 'poster-card';
    card.title = t.title + (t.year ? ` (${t.year})` : '');
    const poster = document.createElement('div');
    poster.className = 'poster';
    if (t.poster) {
        const img = document.createElement('img');
        img.src = API_BASE + t.poster;
        img.alt = '';
        img.loading = 'lazy';
        poster.appendChild(img);
    } else {
        poster.classList.add('no-poster');
        poster.innerHTML = placeholderIcon(t.kind === 'show' ? 'tv' : 'movie');
    }
    const title = document.createElement('div');
    title.className = 'poster-title';
    title.textContent = t.title;
    const year = document.createElement('div');
    year.className = 'poster-year';
    year.textContent = t.year || '';
    // Progress: a movie's place in it; a show's share of episodes watched.
    const allWatched = t.file_count > 0 && (t.watched || 0) >= (t.kind === 'movie' ? 1 : t.file_count);
    addProgressOverlay(poster, null, 0, { watched: allWatched });
    if (t.kind === 'movie') {
        addProgressBar(poster, t.progress);
    } else if (t.watched && !allWatched) {
        addProgressBar(poster, t.watched / t.file_count);
        year.textContent = [t.year, `${t.watched} of ${t.file_count} watched`].filter(Boolean).join(' · ');
    }
    card.appendChild(poster);
    card.appendChild(title);
    card.appendChild(year);
    card.onclick = () => showTitle(state.libraryId, t.kind, t.tmdb_id);
    return card;
}

// Unmatched files (or, for TV, unmatched shows): a poster-shaped placeholder that plays the file.
function createUnmatchedCard(f) {
    const item = libraryFileItem(f);
    const card = document.createElement('div');
    card.className = 'poster-card unmatched';
    const poster = document.createElement('div');
    poster.className = 'poster no-poster';
    poster.innerHTML = placeholderIcon('theaters');
    if (canFixMatches()) poster.appendChild(createFixMatchButton({ path: f.path }, f.name));
    const title = document.createElement('div');
    title.className = 'poster-title';
    title.textContent = f.name;
    title.title = f.name;
    const meta = document.createElement('div');
    meta.className = 'poster-year';
    meta.textContent = f.count ? `${f.count} files` : formatSize(f.size || 0);
    if (!f.count) addProgressOverlay(poster, f.playback, f.duration);
    card.appendChild(poster);
    card.appendChild(title);
    card.appendChild(meta);
    card.onclick = () => (f.count ? null : handleMediaClick(item));
    return card;
}

function libraryFileItem(f) {
    // id: for its trickplay frames (the pictures over the seek bar)
    return { id: f.id, path: f.path, name: f.name, type: 'video', size: f.size || 0, duration: f.duration || 0,
             thumbnail_url: f.thumbnail_url || '', metadata: f.match || null, playback: f.playback || null };
}

async function showTitle(libId, kind, id, push = true, keepScroll = false) {
    const container = document.getElementById('library-view');
    const scroll = window.scrollY;
    setView('title');
    state.libraryId = libId;
    state.titleRef = { kind, id };
    if (push) pushLibraryHistory({ view: 'title', lib: libId, kind, id });
    if (!keepScroll) {
        container.innerHTML = '<p class="library-empty">Loading…</p>';
        window.scrollTo(0, 0);
    }

    let t;
    try {
        if (!state.libraryName) {
            const lib = await api.getLibrary(libId);
            state.libraryName = lib.library_name;
            state.libraryType = lib.library_type;
        }
        t = await api.getLibraryTitle(libId, kind, id);
    } catch (error) {
        container.textContent = 'Could not load title: ' + error.message;
        return;
    }
    if (!state.libraryType) state.libraryType = kind === 'show' ? 'tv' : 'movie';
    // Its theme music, unless it's already playing (a refresh of the same page)
    const themeId = `${libId}/${kind}/${id}`;
    if (!t.theme) stopThemeMusic();
    else if (!themeAudio || themeAudio.dataset.title !== themeId) playThemeMusic(libId, kind, id);
    libraryBreadcrumb(t.title);
    container.innerHTML = '';
    const hero = createTitleHero(t);
    container.appendChild(hero);

    // Movie libraries list files, also for a title matched to a TV show (a TV movie).
    if (kind === 'movie' || (t.files || []).length) {
        state.mediaItems = (t.files || []).map(libraryFileItem);
        container.appendChild(createFilesSection(t.files.length > 1 ? 'Files' : '', t.files || []));
    } else {
        state.mediaItems = [];
        (t.seasons || []).forEach(se => se.episodes.forEach(ep => ep.files.forEach(f => state.mediaItems.push(libraryFileItem(f)))));
        (t.other_files || []).forEach(f => state.mediaItems.push(libraryFileItem(f)));
        container.appendChild(createSeasonsSection(t));
        if ((t.other_files || []).length) {
            container.appendChild(createFilesSection('Other files', t.other_files));
        }
    }
    // Admins add a movie library's title to their collections
    if (isAdminUser() && state.libraryType === 'movie') {
        const add = document.createElement('button');
        add.type = 'button';
        add.className = 'btn btn-secondary collection-add-btn';
        add.textContent = 'Add to collection';
        add.onclick = () => openAddToCollection({ kind, tmdb_id: id, title: t.title });
        hero.querySelector('.title-info').appendChild(add);
    }
    // A movie with several files (versions, editions) marks them all at once; each file has its own too.
    const titleItems = kind === 'movie' || (t.files || []).length ? state.mediaItems : [];
    if (titleItems.length > 1) {
        const mark = createWatchedToggle(titleItems, 'btn watched-btn', { labelHtml: 'Mark All Watched' });
        hero.querySelector('.title-info').appendChild(mark);
    }
    if (keepScroll) window.scrollTo(0, scroll);
    restorePendingScroll();
}

// Poster on the left; title, details, synopsis and credits on the right; backdrop behind.
function createTitleHero(t) {
    const hero = document.createElement('section');
    hero.className = 'title-hero';
    if (t.backdrop) {
        hero.style.setProperty('--backdrop', `url("${API_BASE + t.backdrop}")`);
        hero.classList.add('has-backdrop');
    }
    const poster = document.createElement('div');
    poster.className = 'title-poster';
    if (t.poster) {
        const img = document.createElement('img');
        img.src = API_BASE + t.poster;
        img.alt = `${t.title} poster`;
        poster.appendChild(img);
    } else {
        poster.classList.add('no-poster');
        poster.innerHTML = placeholderIcon(t.kind === 'show' ? 'tv' : 'movie');
    }
    hero.appendChild(poster);

    const info = document.createElement('div');
    info.className = 'title-info';
    const h1 = document.createElement('h1');
    h1.textContent = t.title;
    info.appendChild(h1);

    const facts = [t.year, t.runtime ? `${t.runtime} min` : '', (t.genres || []).join(', ')].filter(Boolean);
    if (facts.length) {
        const meta = document.createElement('p');
        meta.className = 'title-meta';
        meta.textContent = facts.join(' · ');
        info.appendChild(meta);
    }
    if (t.tagline) {
        const tag = document.createElement('p');
        tag.className = 'title-tagline';
        tag.textContent = t.tagline;
        info.appendChild(tag);
    }
    if (t.overview) {
        const overview = document.createElement('p');
        overview.className = 'title-overview';
        overview.textContent = t.overview;
        info.appendChild(overview);
    }
    const people = (label, list) => {
        if (!list || !list.length) return;
        const p = document.createElement('p');
        p.className = 'title-credit';
        p.innerHTML = '<strong></strong> ';
        p.querySelector('strong').textContent = label;
        p.appendChild(document.createTextNode(list.map(c => c.name).join(', ')));
        info.appendChild(p);
    };
    people(t.kind === 'show' ? 'Created by' : (t.directors || []).length > 1 ? 'Directors' : 'Director',
           t.kind === 'show' ? t.creators : t.directors);
    if (t.kind === 'movie') people('Writers', t.writers);

    if ((t.cast || []).length) {
        const cast = document.createElement('div');
        cast.className = 'title-cast';
        t.cast.slice(0, 10).forEach(c => {
            // Opens their window: a larger photo, their details and filmography
            const person = document.createElement('button');
            person.type = 'button';
            person.className = 'cast-member';
            person.title = `About ${c.name}`;
            person.onclick = () => openPersonDialog(state.libraryId, c);
            const face = document.createElement('div');
            face.className = 'cast-face';
            if (c.profile) {
                const img = document.createElement('img');
                img.src = API_BASE + c.profile;
                img.alt = '';
                img.loading = 'lazy';
                face.appendChild(img);
            } else {
                face.textContent = c.name.charAt(0);
            }
            const name = document.createElement('div');
            name.className = 'cast-name';
            name.textContent = c.name;
            const role = document.createElement('div');
            role.className = 'cast-role';
            role.textContent = c.character || '';
            person.append(face, name, role);
            cast.appendChild(person);
        });
        info.appendChild(withScrollButtons(cast));
    }
    hero.appendChild(info);
    return hero;
}

// A person's window, from a title page's cast: a larger photo, their details and biography, their
// titles in this library (opened as title pages) and their whole TMDB filmography (those here open too).
async function openPersonDialog(libId, cast) {
    const modal = document.createElement('div');
    modal.className = 'modal active';
    modal.innerHTML = `<div class="modal-content person-dialog" role="dialog" aria-modal="true">
        <span class="match-close" aria-label="Close" role="button" tabindex="0">&times;</span>
        <div class="person-head"><div class="person-photo"></div><div class="person-facts"><h2></h2></div></div>
        <div class="person-body"><p class="library-empty">Loading…</p></div></div>`;
    const $ = sel => modal.querySelector(sel);
    $('h2').textContent = cast.name;
    if (cast.profile) $('.person-photo').innerHTML = `<img alt="" src="${API_BASE + cast.profile}">`;
    const close = () => modal.remove();
    $('.match-close').onclick = close;
    modal.addEventListener('click', e => { if (e.target === modal) close(); });
    modal.addEventListener('keydown', e => { if (e.key === 'Escape') close(); });
    document.body.appendChild(modal);
    $('.match-close').focus();

    let p;
    try {
        p = await api.request(`/api/library/person?lib=${encodeURIComponent(libId)}&id=${cast.tmdb_id}`);
    } catch (error) {
        $('.person-body').textContent = 'Could not load their details: ' + error.message;
        return;
    }
    if (p.photo) $('.person-photo').innerHTML = `<img alt="" src="${API_BASE + p.photo}">`;
    const facts = $('.person-facts');
    const line = (label, text) => {
        if (!text) return;
        const el = document.createElement('p');
        el.className = 'person-fact';
        el.innerHTML = '<strong></strong> ';
        el.querySelector('strong').textContent = label;
        el.appendChild(document.createTextNode(text));
        facts.appendChild(el);
    };
    const date = d => d ? new Date(d + 'T00:00:00').toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' }) : '';
    const age = (from, to) => {
        if (!from) return '';
        const a = new Date(from + 'T00:00:00'), b = to ? new Date(to + 'T00:00:00') : new Date();
        let years = b.getFullYear() - a.getFullYear();
        if (b < new Date(b.getFullYear(), a.getMonth(), a.getDate())) years--;
        return ` (${to ? 'aged' : 'age'} ${years})`;
    };
    line('Known for', p.known_for);
    line('Born', p.birthday ? date(p.birthday) + (p.deathday ? '' : age(p.birthday)) : '');
    line('Died', p.deathday ? date(p.deathday) + age(p.birthday, p.deathday) : '');
    line('Place of birth', p.place_of_birth);
    const body = $('.person-body');
    body.innerHTML = '';
    if (p.biography) {
        // Beside the photo, under their facts (below it on a narrow window)
        const bio = document.createElement('p');
        bio.className = 'person-bio';
        bio.textContent = p.biography;
        facts.appendChild(bio);
    }
    const openTitle = (kind, id) => { close(); showTitle(libId, kind, id); };
    if (p.in_library.length) {
        const h = document.createElement('h3');
        h.textContent = `In ${state.libraryName || 'this library'} (${p.in_library.length})`;
        body.appendChild(h);
        const grid = document.createElement('div');
        grid.className = 'poster-grid person-grid';
        p.in_library.forEach(t => {
            const card = createPosterCard(t);
            card.onclick = () => openTitle(t.kind, t.tmdb_id);
            if (t.role) card.querySelector('.poster-year').textContent = [t.year, t.role].filter(Boolean).join(' · ');
            grid.appendChild(card);
        });
        body.appendChild(grid);
    }
    if (p.filmography.length) {
        const h = document.createElement('h3');
        h.textContent = `Filmography (${p.filmography.length})`;
        body.appendChild(h);
        const list = document.createElement('div');
        list.className = 'person-films';
        p.filmography.forEach(f => {
            const row = document.createElement(f.in_library ? 'button' : 'div');
            row.className = 'person-film' + (f.in_library ? ' here' : '');
            if (f.in_library) {
                row.type = 'button';
                row.onclick = () => openTitle(f.kind, f.tmdb_id);
            }
            const thumb = document.createElement('div');
            thumb.className = 'person-film-poster';
            if (f.poster) thumb.innerHTML = `<img alt="" loading="lazy" src="${f.poster}">`;
            const text = document.createElement('div');
            const title = document.createElement('div');
            title.className = 'person-film-title';
            title.textContent = f.title + (f.kind === 'show' ? ' (TV)' : '');
            const meta = document.createElement('div');
            meta.className = 'person-film-meta';
            meta.textContent = [f.year || 'Upcoming', f.role, f.in_library ? 'In this library' : ''].filter(Boolean).join(' · ');
            text.append(title, meta);
            row.append(thumb, text);
            list.appendChild(row);
        });
        body.appendChild(list);
    }
}

// The square video thumbnails with a Play button under each (clicking the thumbnail plays too).
function createFilesSection(heading, files) {
    const section = document.createElement('section');
    section.className = 'title-files';
    if (heading) {
        const h2 = document.createElement('h2');
        h2.className = 'library-section';
        h2.textContent = heading;
        section.appendChild(h2);
    }
    // Each file as a TV show's episode row: the wide thumbnail (which plays it), then its name, size and
    // length, and Play, Fix and watched beside it.
    const list = document.createElement('div');
    list.className = 'episode-list title-file-list';
    files.forEach(f => {
        const item = libraryFileItem(f);
        const row = document.createElement('div');
        row.className = 'episode-row';
        const still = document.createElement('button');
        still.type = 'button';
        still.className = 'episode-still';
        still.title = 'Play';
        if (item.thumbnail_url) {
            const img = document.createElement('img');
            img.src = api.getThumbnailUrl(item.thumbnail_url);
            img.alt = '';
            img.loading = 'lazy';
            still.appendChild(img);
        }
        addProgressOverlay(still, item.playback, item.duration);
        still.onclick = () => playVideo(item, false, resumePosition(item));

        const text = document.createElement('div');
        text.className = 'episode-text';
        if (f.match) {
            const match = document.createElement('div');
            match.className = 'episode-title';
            match.textContent = describeMatch(f.match);
            text.appendChild(match);
        }
        const name = document.createElement('div');
        name.className = f.match ? 'episode-meta' : 'episode-title';
        name.textContent = f.name;
        const meta = document.createElement('div');
        meta.className = 'episode-meta';
        meta.textContent = [formatSize(item.size || 0), item.duration ? formatDuration(item.duration) : '', timeLeftText(item)]
            .filter(Boolean).join(' · ');
        text.append(name);
        if (f.format) text.appendChild(formatLineEl(f.format, 'episode-format'));
        text.append(meta);
        const actions = el('div', 'episode-actions');
        // As on every platform: Play (Resume, Play from start), watched, then Fix
        actions.appendChild(createPlayButtons(item));
        actions.appendChild(createWatchedToggle([item], 'btn secondary icon-btn watched-btn'));
        if (canFixMatches()) actions.appendChild(createFixIconButton(f));
        text.appendChild(actions);
        row.append(still, text);
        list.appendChild(row);
    });
    section.appendChild(list);
    return section;
}

function createSeasonsSection(t) {
    const section = document.createElement('section');
    section.className = 'title-seasons';
    const seasons = t.seasons || [];
    if (!seasons.length) {
        section.innerHTML = '<p class="library-empty">No episodes found.</p>';
        return section;
    }
    const picker = document.createElement('div');
    picker.className = 'season-picker';
    picker.setAttribute('role', 'tablist');
    const list = document.createElement('div');
    list.className = 'episode-list';

    // Watched buttons for the season shown and the whole show, above the seasons (as on Android).
    const seasonActions = el('div', 'season-actions');
    const count = el('div', 'episode-count'); // under the seasons: the one shown's
    const seasonItems = (se) => se.episodes.flatMap(ep => ep.files.map(libraryFileItem));
    const select = (index) => {
        state.seasonIndex = index;
        picker.querySelectorAll('button').forEach((b, i) => {
            b.classList.toggle('active', i === index);
            b.setAttribute('aria-selected', String(i === index));
        });
        list.innerHTML = '';
        seasons[index].episodes.forEach(ep => list.appendChild(createEpisodeRow(ep)));
        const n = seasons[index].episodes.length;
        count.textContent = n === 1 ? '1 Episode' : `${n} Episodes`;
        seasonActions.innerHTML = '';
        seasonActions.append(
            createWatchedToggle(seasonItems(seasons[index]), 'btn watched-btn',
                { labelHtml: seasonLabelHtml(seasons[index], seasons[index].name) }),
            createWatchedToggle(seasons.flatMap(seasonItems), 'btn watched-btn', { labelHtml: 'Whole show' }),
            autoSkipToggle(t));
    };
    seasons.forEach((se, i) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.setAttribute('role', 'tab');
        // An episode is watched when any of its files is; the ring after the name shows their share.
        const watched = se.episodes.filter(ep => ep.files.some(f => f.playback && f.playback.watched)).length;
        btn.innerHTML = seasonLabelHtml(se, se.name) + progressRingSvg(se.episodes.length ? watched / se.episodes.length : 0);
        btn.title = `${watched} of ${se.episodes.length} watched`;
        btn.onclick = () => select(i);
        picker.appendChild(btn);
    });
    section.append(seasonActions, picker, count, list);
    // A refresh (after playing or marking) stays on the season that was showing; opening the show
    // starts where it picks up, with that episode scrolled into view.
    const fresh = state.titleSeasonFor !== t.tmdb_id;
    const pick = fresh ? upNext(seasons) : null;
    const keep = fresh ? (pick ? pick.season : 0) : state.seasonIndex;
    state.titleSeasonFor = t.tmdb_id;
    select(Math.min(keep || 0, seasons.length - 1));
    if (pick) {
        // Once the page is laid out (the section isn't on it yet).
        requestAnimationFrame(() => requestAnimationFrame(() =>
            list.children[pick.episode]?.scrollIntoView({ block: 'center', behavior: 'smooth' })));
    }
    return section;
}

// A season's progress: a dimmed dashed ring while none is watched, then a fainter track with an
// arc for the share watched.
function progressRingSvg(fraction) {
    const r = 8, c = 2 * Math.PI * r;
    const ring = fraction > 0
        ? `<circle cx="10" cy="10" r="${r}" opacity="0.3"/>` +
          `<circle cx="10" cy="10" r="${r}" stroke-linecap="round" stroke-dasharray="${(Math.min(fraction, 1) * c).toFixed(2)} ${c.toFixed(2)}" transform="rotate(-90 10 10)"/>`
        // Six long dashes, evenly around (the circumference divides into them exactly)
        : `<circle cx="10" cy="10" r="${r}" opacity="0.45" stroke-dasharray="${(c / 6 * 0.7).toFixed(2)} ${(c / 6 * 0.3).toFixed(2)}" transform="rotate(-90 10 10)"/>`;
    return `<svg class="season-ring" viewBox="0 0 20 20" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2.4">${ring}</svg>`;
}

// Where a show picks up, as on Android: the episode played most recently if it's unfinished, else
// the one after it (on into the next season, skipping specials unless it's one). Null when none
// has been played.
function upNext(seasons) {
    const all = seasons.flatMap((s, si) => s.episodes.map((ep, ei) => ({ si, ei, ep, number: s.season })));
    const played = (f) => f.playback && (f.playback.watched || f.playback.position >= MIN_RESUME);
    let latest = -1, latestAt = -1;
    all.forEach((a, i) => a.ep.files.filter(played).forEach(f => {
        if ((f.playback.updated_at || 0) > latestAt) { latest = i; latestAt = f.playback.updated_at || 0; }
    }));
    if (latest < 0) return null;
    const at = all[latest];
    const finished = at.ep.files.some(f => f.playback && f.playback.watched) &&
        !at.ep.files.some(f => f.playback && !f.playback.watched && f.playback.position >= MIN_RESUME);
    const next = !finished ? at : (all.slice(latest + 1).find(a => a.number !== 0 || at.number === 0) || at);
    return { season: next.si, episode: next.ei };
}

// Auto Skip: the players skip this show's intros and credits by themselves (for this user).
function autoSkipToggle(t) {
    const label = el('label', 'auto-skip');
    label.title = "Skip this show's intros and credits without asking";
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.checked = !!t.auto_skip;
    box.onchange = async () => {
        try {
            await api.setAutoSkip(t.tmdb_id, box.checked);
            t.auto_skip = box.checked;
        } catch (e) {
            box.checked = !box.checked;
            alert('Could not save: ' + e.message);
        }
    };
    label.append(box, document.createTextNode(' Auto Skip'));
    return label;
}

// A season's label, with "Season N" small above it when the season has a name of its own ("Blood and Sand").
function seasonLabelHtml(se, label) {
    const number = `Season ${se.season}`;
    const named = se.season > 0 && se.name.toLowerCase() !== number.toLowerCase();
    return `<span class="season-label">${named ? `<small>${escapeHtml(number)}</small>` : ''}<span>${escapeHtml(label)}</span></span>`;
}

// Fix for a title page's file: an icon button like the others, "Fix" on hover.
function createFixIconButton(f) {
    const fix = el('button', 'btn secondary icon-btn');
    fix.type = 'button';
    fix.title = 'Fix';
    fix.setAttribute('aria-label', `Fix match for ${f.name}`);
    fix.innerHTML = iconSvg('edit');
    fix.onclick = (e) => { e.stopPropagation(); openMatchDialog({ path: f.path }, f.name); };
    return fix;
}

// A video's format line ("4K · HEVC · Dolby Vision · TrueHD Atmos 7.1").
function formatLineEl(format, className) {
    const line = document.createElement('div');
    line.className = className;
    line.textContent = format;
    line.title = format;
    return line;
}

function createEpisodeRow(ep) {
    const row = document.createElement('div');
    row.className = 'episode-row';
    const first = libraryFileItem(ep.files[0]);
    const still = document.createElement('button');
    still.type = 'button';
    still.className = 'episode-still';
    still.title = 'Play';
    const stillUrl = ep.still ? API_BASE + ep.still : (first.thumbnail_url ? api.getThumbnailUrl(first.thumbnail_url) : '');
    if (stillUrl) {
        const img = document.createElement('img');
        img.src = stillUrl;
        img.alt = '';
        img.loading = 'lazy';
        still.appendChild(img);
    }
    addProgressOverlay(still, first.playback, first.duration);
    still.onclick = () => playVideo(first, false, resumePosition(first));

    const text = document.createElement('div');
    text.className = 'episode-text';
    const title = document.createElement('div');
    title.className = 'episode-title';
    title.textContent = `${episodeCode(ep.season, ep.episode)} · ${ep.title}`;
    const meta = document.createElement('div');
    meta.className = 'episode-meta';
    meta.textContent = [ep.air_date, ep.runtime ? `${ep.runtime} min` : ''].filter(Boolean).join(' · ');
    const overview = document.createElement('div');
    overview.className = 'episode-overview';
    overview.textContent = ep.overview || '';
    text.append(title);
    // Its files' formats (one line each where they differ), between its title and its date and length
    [...new Set(ep.files.map(f => f.format).filter(Boolean))].forEach(f => text.appendChild(formatLineEl(f, 'episode-format')));
    text.append(meta, overview);

    const actions = document.createElement('div');
    actions.className = 'episode-actions';
    // As on every platform: Play (each file's), watched, then Fix
    ep.files.forEach(f => {
        const item = libraryFileItem(f);
        const ext = f.name.split('.').pop().toUpperCase();
        actions.appendChild(createPlayButtons(item, ep.files.length > 1 ? ext : ''));
    });
    actions.appendChild(createWatchedToggle(ep.files.map(libraryFileItem), 'btn secondary icon-btn watched-btn'));
    if (canFixMatches()) ep.files.forEach(f => actions.appendChild(createFixIconButton(f)));
    // The buttons go under the description, left-aligned.
    text.appendChild(actions);
    row.append(still, text);
    return row;
}

// ---------------------------------------------------------------------------------------------
// Playback progress: each video's progress for the signed-in user, from the server's playback
// records (item.playback: { position, duration, watched }).

const MIN_RESUME = 10; // seconds; earlier than this starts over

function resumePosition(item) {
    const p = item.playback;
    return p && p.position >= MIN_RESUME ? p.position : 0;
}

function progressFraction(playback, duration) {
    if (!playback || playback.position < MIN_RESUME) return 0;
    const total = playback.duration || duration || 0;
    return total > 0 ? Math.min(playback.position / total, 1) : 0;
}

// "23 min left", for a video in progress.
function timeLeftText(item) {
    const p = item.playback;
    const total = (p && p.duration) || item.duration || 0;
    if (!resumePosition(item) || !total) return '';
    const minutes = Math.max(1, Math.round((total - p.position) / 60));
    return `${minutes} min left`;
}

// A progress line along the bottom of a thumbnail, and a check in the corner once watched.
function addProgressOverlay(container, playback, duration, { watched } = {}) {
    container.querySelectorAll('.watch-progress, .watched-badge').forEach(n => n.remove());
    addProgressBar(container, progressFraction(playback, duration));
    if (watched ?? (playback && playback.watched)) {
        const badge = el('div', 'watched-badge');
        badge.innerHTML = iconSvg('check');
        badge.title = 'Watched';
        container.appendChild(badge);
    }
}

function addProgressBar(container, fraction) {
    if (!(fraction > 0)) return;
    const bar = el('div', 'watch-progress');
    const fill = el('span');
    fill.style.width = `${(Math.min(fraction, 1) * 100).toFixed(1)}%`;
    bar.appendChild(fill);
    container.appendChild(bar);
}

// Corner buttons on a video's thumbnail: Play from start (when in progress), and watched.
function createCardActions(item) {
    const actions = el('div', 'card-actions');
    if (resumePosition(item)) {
        const restart = el('button', 'card-action');
        restart.type = 'button';
        restart.title = 'Play from start';
        restart.innerHTML = iconSvg('replay');
        restart.onclick = (e) => { e.stopPropagation(); playVideo(item, false, 0); };
        actions.appendChild(restart);
    }
    actions.appendChild(createWatchedToggle([item], 'card-action'));
    return actions;
}

function isWatched(items) {
    return items.length > 0 && items.every(i => i.playback && i.playback.watched);
}

// A button that marks files watched, or unwatched if they all are.
function createWatchedToggle(items, className, { label = false, labelHtml = '' } = {}) {
    const btn = el('button', className);
    btn.type = 'button';
    const watched = isWatched(items);
    const text = watched ? 'Mark unwatched' : 'Mark watched';
    btn.title = text;
    btn.setAttribute('aria-label', text);
    btn.classList.toggle('is-watched', watched);
    // Watched buttons have a check in a circle, solid once watched and dimmed until then (as on Android).
    const icon = className.includes('watched-btn') ? 'checkCircle' : 'check';
    btn.innerHTML = iconSvg(icon) + (labelHtml || (label ? `<span>${text}</span>` : ''));
    btn.onclick = async (e) => {
        e.stopPropagation();
        try {
            await api.setWatched(items.map(i => i.path), !watched);
            refreshCurrentView();
        } catch (error) {
            alert('Could not save: ' + error.message);
        }
    };
    return btn;
}

// Play (or Resume, with a Play from start beside it) buttons for a file.
// Icons alone, except Resume, which says where it picks up. [label] tells several files apart.
function createPlayButtons(item, label = '') {
    const group = el('div', 'play-buttons');
    const at = resumePosition(item);
    const play = el('button', 'btn play-btn');
    play.type = 'button';
    const text = at ? `Resume ${clock(at)}` : label;
    play.innerHTML = iconSvg('play') + (text ? `<span>${text}</span>` : '');
    if (!text) play.classList.add('icon-btn');
    play.title = at ? `Resume ${item.name}` : `Play ${item.name}`;
    play.setAttribute('aria-label', play.title);
    play.onclick = () => playVideo(item, false, at);
    group.appendChild(play);
    if (at) {
        const restart = el('button', 'btn play-btn secondary icon-btn');
        restart.type = 'button';
        restart.innerHTML = iconSvg('replay');
        restart.title = 'Play from start';
        restart.setAttribute('aria-label', 'Play from start');
        restart.onclick = () => playVideo(item, false, 0);
        group.appendChild(restart);
    }
    return group;
}

// Resume or Play from start, for a partly watched video opened from a folder.
function showResumeChoice(item) {
    document.querySelector('.resume-choice')?.remove();
    const overlay = el('div', 'resume-choice');
    const box = el('div', 'resume-choice-box');
    box.setAttribute('role', 'dialog');
    box.setAttribute('aria-label', item.name);
    box.appendChild(el('div', 'resume-choice-title', item.name));
    const left = timeLeftText(item);
    if (left) box.appendChild(el('div', 'resume-choice-meta', left));
    const close = () => { overlay.remove(); document.removeEventListener('keydown', onKey); };
    const button = (icon, text, cls, action) => {
        const b = el('button', cls);
        b.type = 'button';
        b.innerHTML = iconSvg(icon) + `<span>${text}</span>`;
        b.onclick = () => { close(); action(); };
        box.appendChild(b);
        return b;
    };
    const resume = button('play', `Resume from ${clock(resumePosition(item))}`, 'btn',
        () => playVideo(item, false, resumePosition(item)));
    button('replay', 'Play from start', 'btn secondary', () => playVideo(item, false, 0));
    const onKey = (e) => { if (e.key === 'Escape') close(); };
    document.addEventListener('keydown', onKey);
    overlay.onclick = (e) => { if (e.target === overlay) close(); };
    overlay.appendChild(box);
    document.body.appendChild(overlay);
    resume.focus();
}

// Continue Watching: a row of videos in progress, newest first, optionally in one library.
async function createContinueRow(lib = '') {
    let items;
    try {
        items = await api.continueWatching(lib);
    } catch (error) {
        return null;
    }
    if (!items || !items.length) return null;
    const section = el('section', 'continue-row');
    section.appendChild(el('h2', 'library-section', 'Continue Watching'));
    const row = el('div', 'continue-items');
    items.forEach(item => row.appendChild(createContinueCard(item)));
    section.appendChild(withScrollButtons(row));
    return section;
}

// Recently Added: the videos that arrived last in a library (the showNewAdditions option).
async function createNewAdditionsRow(lib) {
    let items;
    try {
        items = await api.newAdditions(lib);
    } catch (error) {
        return null;
    }
    return newAdditionsSection('Recently Added', items);
}

// The home screen's: a Recently Added row for each library that shows there.
async function createNewAdditionsRows() {
    let libraries;
    try {
        libraries = await api.newAdditionsByLibrary();
    } catch (error) {
        return [];
    }
    return (libraries || []).map(lib => newAdditionsSection(`Recently Added in ${lib.library_name}`, lib.items));
}

function newAdditionsSection(heading, items) {
    if (!items || !items.length) return null;
    const section = el('section', 'continue-row');
    section.appendChild(el('h2', 'library-section', heading));
    const row = el('div', 'continue-items');
    items.forEach(item => row.appendChild(createContinueCard(item)));
    section.appendChild(withScrollButtons(row));
    return section;
}

// A sideways row with ‹ and › buttons over its ends, for a mouse (a touch screen swipes): each
// shows while the pointer is over the row and it can scroll that way, and moves it about a
// row's width.
function withScrollButtons(row) {
    const wrap = el('div', 'scroll-row');
    wrap.appendChild(row);
    // A few pixels' slack: snapping to the first card leaves the row its padding's width in.
    const update = () => {
        wrap.classList.toggle('can-prev', row.scrollLeft > 4);
        wrap.classList.toggle('can-next', row.scrollLeft + row.clientWidth < row.scrollWidth - 4);
    };
    [['prev', 'chevronLeft', 'Scroll left', -1], ['next', 'chevronRight', 'Scroll right', 1]].forEach(([side, icon, label, dir]) => {
        const btn = el('button', `scroll-row-btn ${side}`);
        btn.type = 'button';
        btn.tabIndex = -1; // the cards themselves take keyboard focus, and bring themselves into view
        btn.setAttribute('aria-label', label);
        btn.innerHTML = iconSvg(icon);
        btn.addEventListener('click', (e) => {
            e.stopPropagation();
            row.scrollBy({ left: dir * Math.max(row.clientWidth * 0.9, 200), behavior: 'smooth' });
        });
        wrap.appendChild(btn);
    });
    row.addEventListener('scroll', update, { passive: true });
    new ResizeObserver(update).observe(row); // its width, and images arriving
    requestAnimationFrame(update);
    return wrap;
}

// When a video arrived, in days on this device's calendar: "Added 2 days ago".
function addedText(seconds) {
    const midnight = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
    const days = Math.round((midnight(new Date()) - midnight(new Date(seconds * 1000))) / 86400000);
    if (days <= 0) return 'Added today';
    if (days === 1) return 'Added yesterday';
    if (days < 30) return `Added ${days} days ago`;
    const count = (n, unit) => `Added ${n} ${unit}${n === 1 ? '' : 's'} ago`;
    return days < 365 ? count(Math.floor(days / 30), 'month') : count(Math.floor(days / 365), 'year');
}

function continueTitle(item) {
    const md = item.metadata;
    if (md && md.kind === 'episode') {
        return { title: md.show_title, subtitle: `${episodeCode(md.season, md.episode)} · ${md.title}` };
    }
    if (md && md.title) return { title: md.title + (md.year ? ` (${md.year})` : ''), subtitle: item.name };
    return { title: item.name, subtitle: '' };
}

function createContinueCard(item) {
    const card = el('div', 'continue-card');
    card.tabIndex = 0;
    const thumb = el('div', 'continue-thumb');
    const md = item.metadata || {};
    const src = md.still ? API_BASE + md.still : md.backdrop ? API_BASE + md.backdrop
        : item.thumbnail_url ? api.getThumbnailUrl(item.thumbnail_url) : '';
    if (src) {
        const img = document.createElement('img');
        img.src = src;
        img.alt = '';
        img.loading = 'lazy';
        thumb.appendChild(img);
    } else {
        thumb.innerHTML = placeholderIcon('movie');
    }
    addProgressOverlay(thumb, item.playback, item.duration);
    if (!item.added_at) { // Recently Added cards aren't anyone's to remove
        const remove = el('button', 'card-action continue-remove');
        remove.type = 'button';
        remove.title = 'Remove from Continue Watching';
        remove.innerHTML = iconSvg('close');
        remove.onclick = async (e) => {
            e.stopPropagation();
            await api.removeFromContinue(item.path).catch(() => {});
            card.remove();
        };
        thumb.appendChild(remove);
    }
    const { title, subtitle } = continueTitle(item);
    card.append(thumb, el('div', 'continue-title', title));
    if (subtitle) card.appendChild(el('div', 'continue-subtitle', subtitle));
    // The episode after one finished says so; one in progress, how long is left.
    const left = item.added_at ? addedText(item.added_at) : item.up_next ? 'Up next' : timeLeftText(item);
    if (left) card.appendChild(el('div', 'continue-subtitle', left));
    const play = () => playVideo(item, false, resumePosition(item));
    card.onclick = play;
    card.onkeydown = (e) => { if (e.key === 'Enter') play(); };
    return card;
}

// ---------------------------------------------------------------------------------------------
// Settings → Streams (admins): every stream playing now, refreshed every 2 seconds while the tab is open

let streamsTimer = null;

function startStreamsView() {
    if (streamsTimer) return;
    refreshStreams();
    streamsTimer = setInterval(refreshStreams, 2000);
}

function stopStreamsView() {
    clearInterval(streamsTimer);
    streamsTimer = null;
}

// ---------------------------------------------------------------------------------------------
// Settings → Jobs (admins): each background job's state, progress, last run and next run (a
// countdown), with Run now and Pause/Resume. The status is fetched every few seconds while the
// tab is open, and the times tick in between.

let jobsView = null; // { data, poll, tick }
const JOB_STATES = { idle: 'Idle', running: 'Running', waiting: 'Waiting', paused: 'Paused' };

function startJobsView() {
    if (jobsView) return;
    jobsView = { data: null };
    const refresh = async () => {
        try {
            const data = await api.getJobs();
            if (jobsView) {
                jobsView.data = data;
                renderJobs();
            }
        } catch (e) {
            if (jobsView) document.getElementById('jobs-list').textContent = `Couldn't load the jobs: ${e.message}`;
        }
    };
    refresh();
    jobsView.poll = setInterval(refresh, 5000);
    jobsView.tick = setInterval(updateJobTimes, 1000);
}

function stopJobsView() {
    if (!jobsView) return;
    clearInterval(jobsView.poll);
    clearInterval(jobsView.tick);
    jobsView = null;
}

// A span of time: "2h 05m 09s", "4m 09s", "12s" (days as "3d 4h").
function jobDuration(ms) {
    const s = Math.max(0, Math.round(ms / 1000));
    const d = Math.floor(s / 86400), h = Math.floor(s % 86400 / 3600), m = Math.floor(s % 3600 / 60), sec = s % 60;
    const two = (n) => String(n).padStart(2, '0');
    if (d) return `${d}d ${h}h`;
    if (h) return `${h}h ${two(m)}m ${two(sec)}s`;
    if (m) return `${m}m ${two(sec)}s`;
    return `${sec}s`;
}

function jobWhen(iso) {
    const t = new Date(iso);
    const ago = Date.now() - t;
    if (ago < 60000) return 'just now';
    if (ago < 86400000) return `${jobDuration(ago).replace(/ \d+s$/, '')} ago`;
    return t.toLocaleString();
}

// The Jobs tab's Storage box: each data folder's size, readable and in bytes, and their total.
function renderStorage(storage) {
    const box = document.getElementById('storage-list');
    if (!storage.length) {
        box.replaceChildren(Object.assign(document.createElement('div'), { className: 'job-line', textContent: 'Counting…' }));
        return;
    }
    const row = (name, bytes, cls) => {
        const r = document.createElement('div');
        r.className = `storage-row ${cls || ''}`;
        for (const [text, c] of [[name, 'storage-name'], [formatSize(bytes), 'storage-size'], [`${bytes.toLocaleString()} bytes`, 'storage-bytes']]) {
            const span = document.createElement('span');
            span.className = c;
            span.textContent = text;
            r.append(span);
        }
        return r;
    };
    const total = storage.reduce((sum, u) => sum + u.bytes, 0);
    box.replaceChildren(...storage.map(u => row(u.name, u.bytes)), row('Total', total, 'storage-total'));
}

// Trickplay's settings box. The page refreshes every few seconds, so the fields only follow the
// server while they aren't being edited. The GPU checkbox (shown when there's a GPU to decode on)
// saves at once.
function renderTrickplaySettings(tp) {
    const box = document.getElementById('trickplay-settings');
    if (!tp) return;
    box.hidden = false;
    const runners = document.getElementById('tp-runners'), frames = document.getElementById('tp-frames');
    const gpu = document.getElementById('tp-gpu');
    runners.max = tp.max_runners;
    frames.max = tp.max_frames;
    if (!box.dataset.wired) {
        box.dataset.wired = '1';
        const edited = () => { box.dataset.edited = '1'; trickplayHint(); };
        runners.oninput = edited;
        frames.oninput = edited;
        document.getElementById('tp-save').onclick = saveTrickplaySettings;
        gpu.onchange = () => saveTrickplaySettings({ use_gpu: gpu.checked });
    }
    if (box.dataset.edited !== '1') {
        // An empty field is automatic.
        runners.value = tp.auto_runners ? '' : tp.runners;
        frames.value = tp.auto_frames ? '' : tp.frames_per_runner;
    }
    document.getElementById('tp-gpu-row').hidden = !tp.gpu;
    document.getElementById('tp-gpu-label').textContent = `Decode on the GPU (${tp.gpu || ''}); unchecked, the CPU decodes`;
    gpu.checked = !!tp.use_gpu;
    trickplayHint();
}

// The runners and frames the fields give: an empty one is automatic, worked out as the server does
// (one runner per disk the libraries are on; the frames that fill half the memory for those runners).
function trickplayEffective(tp) {
    const r = Number(document.getElementById('tp-runners').value) || tp.auto_runners_value || tp.runners;
    const f = Number(document.getElementById('tp-frames').value) || (tp.auto_frames_for || [])[r - 1] || tp.frames_per_runner;
    return [r, f];
}

function trickplayHint() {
    const tp = jobsView && jobsView.data && jobsView.data.trickplay;
    if (!tp) return;
    const [r, f] = trickplayEffective(tp);
    const runners = document.getElementById('tp-runners'), frames = document.getElementById('tp-frames');
    runners.placeholder = `Auto: ${tp.auto_runners_value || tp.runners}`;
    frames.placeholder = `Auto: ${(tp.auto_frames_for || [])[r - 1] || tp.frames_per_runner}`;
    const t = trickplayMemoryTable(r, f, tp.memory);
    document.getElementById('tp-summary').textContent = t.summary;
    const table = document.getElementById('tp-memory');
    table.hidden = !t.rows.length;
    const cell = (tag, text, cls) => Object.assign(document.createElement(tag), { textContent: text, className: cls || '' });
    const head = document.createElement('tr');
    head.append(cell('th', ''), cell('th', 'Needs with 4K', 'num'), cell('th', 'Has', 'num'), cell('th', ''));
    table.replaceChildren(head, ...t.rows.map(row => {
        const tr = document.createElement('tr');
        tr.append(cell('td', row.what), cell('td', row.needs, 'num'), cell('td', row.has, 'num'), cell('td', row.note, row.warn ? 'tp-warn' : ''));
        return tr;
    }));
    document.getElementById('tp-hint').textContent =
        `Runners 1 to ${tp.max_runners}, frames per runner 1 to ${tp.max_frames}, at most ${tp.max_product} frames at once. ` +
        'Empty is automatic: a runner per disk the libraries are on, and frames filling half the server\'s memory with 4K videos.';
}

// What r runners of f frames take with 4K videos (each frame in a batch holds a decoder of its own), as
// the server measured it for the decoder it uses (m.decoder: the CPU, an Intel or AMD GPU whose buffers
// are the server's memory, or an NVIDIA card with memory of its own), against the server's memory and
// swap, and the card's: a summary line and a row each. The server uses the settings as they are. As
// the apps' trickplayMemoryTable.
function trickplayMemoryTable(r, f, m) {
    const gb = bytes => `${(bytes / 2 ** 30).toFixed(1)} GB`;
    const on = !m ? '' : { vaapi: ', decoded on the GPU', cuda: ', decoded on the graphics card' }[m.decoder] || ', decoded on the CPU';
    const summary = `${r} × ${f} = ${r * f} frames at once${on}.`;
    if (!m || !m.frame_bytes_4k) return { summary, rows: [] };
    const rows = [];
    const ram = r * (m.process_bytes_4k + f * m.frame_bytes_4k);
    const swap = m.swap_bytes || 0;
    const row = { what: m.decoder === 'vaapi' ? 'Memory (with the GPU\'s buffers)' : 'Memory', needs: gb(ram), has: '?', note: '', warn: false };
    if (m.total_bytes) {
        row.has = gb(m.total_bytes) + (swap ? ` + ${gb(swap)} swap` : '');
        if (ram <= m.total_bytes) row.note = 'Fits';
        else if (ram <= m.total_bytes + swap) Object.assign(row, { note: 'Uses swap', warn: true });
        else Object.assign(row, { note: 'Too much: frames are made one at a time', warn: true });
    }
    rows.push(row);
    if (m.card_frame_bytes_4k) {
        const card = r * (m.card_process_bytes_4k + f * m.card_frame_bytes_4k);
        const c = { what: 'Graphics card', needs: gb(card), has: m.card_bytes ? gb(m.card_bytes) : '?', note: '', warn: false };
        if (m.card_bytes) Object.assign(c, card <= m.card_bytes ? { note: 'Fits' } : { note: 'Too much: batches are made smaller', warn: true });
        rows.push(c);
    }
    return { summary, rows };
}

// Saves the fields (Save), or only what's given (the GPU checkbox: { use_gpu }).
async function saveTrickplaySettings(only) {
    const box = document.getElementById('trickplay-settings');
    const message = document.getElementById('tp-message');
    const fields = !only || only instanceof Event;
    const settings = fields ? {
        runners: Number(document.getElementById('tp-runners').value) || 0, // 0: automatic
        frames_per_runner: Number(document.getElementById('tp-frames').value) || 0,
    } : only;
    try {
        const data = await api.setTrickplaySettings(settings);
        if (fields) delete box.dataset.edited;
        message.textContent = 'Saved. Applies from the next batch.';
        message.style.color = '';
        if (jobsView) {
            jobsView.data = data;
            renderJobs();
        }
    } catch (e) {
        message.textContent = e.message;
        message.style.color = 'var(--danger-color, #c62828)';
    }
}

function renderJobs() {
    const { jobs = [], load } = jobsView.data;
    const pct = (f) => `${Math.round(f * 100)}%`;
    const loadText = [];
    if (load) {
        if (load.available) {
            // Jobs wait at the lower limit on a disk or share someone is playing from.
            loadText.push(`CPU ${pct(load.cpu)}`,
                `Disk ${pct(load.disk)} (jobs wait above ${pct(load.max_disk)} on a disk someone is playing from, ${pct(load.relaxed_disk)} otherwise)`);
            if (load.shares?.length) {
                // Libraries on network shares or FUSE pools (mergerfs), whose disks can't be told apart
                loadText.push(`Read times ${load.shares.map(sh => `${sh.library} ${Math.round(sh.ms)} ms`).join(', ')} ` +
                    `(above ${load.max_share_ms} ms where someone is playing, ${load.relaxed_share_ms} ms otherwise)`);
            }
        } else {
            loadText.push('No load readings on this machine: jobs wait while anyone is streaming');
        }
        if (load.streaming) loadText.push('Someone is streaming');
        if (load.pause_while_streaming) loadText.push('Jobs pause while anyone streams');
    }
    document.getElementById('jobs-load').textContent = loadText.join(' · ');
    renderStorage(jobsView.data.storage || []);
    renderTrickplaySettings(jobsView.data.trickplay);

    const list = document.getElementById('jobs-list');
    list.replaceChildren(...jobs.map(job => {
        const el = (tag, cls, text) => {
            const e = document.createElement(tag);
            if (cls) e.className = cls;
            if (text != null) e.textContent = text;
            return e;
        };
        const card = el('div', 'job-card');
        card.dataset.id = job.id;
        const head = el('div', 'stream-head');
        head.append(el('span', 'stream-title', job.name),
            el('span', `stream-badge job-state state-${job.state}`, JOB_STATES[job.state] || job.state));
        card.append(head, el('div', 'job-about', job.about));
        if (job.started) {
            const progress = el('div', 'job-progress');
            if (job.total > 0) {
                const bar = el('div', 'stream-bar');
                const fill = el('div', 'job-bar-fill');
                fill.style.width = `${Math.min(100, job.done / job.total * 100)}%`;
                bar.append(fill);
                progress.append(bar);
            }
            progress.append(el('div', 'job-line', '')); // filled by updateJobTimes
            if (job.detail) progress.append(el('div', 'job-line job-detail', job.detail));
            // What each runner is on, for a job that runs several at once.
            (job.runners || []).forEach((file, i) => {
                if (file) progress.append(el('div', 'job-line job-runner', `Runner ${i + 1}: ${file}`));
            });
            card.append(progress);
        }
        if (job.state === 'waiting' && job.note) card.append(el('div', 'job-note', `Waiting: ${job.note}`));
        card.append(el('div', 'job-line job-last'), el('div', 'job-line job-next'));
        const actions = el('div', 'job-actions');
        const run = el('button', 'btn', 'Run now');
        run.type = 'button';
        run.disabled = !job.can_run || !!job.started;
        run.title = job.started ? 'Already running' : '';
        run.onclick = () => controlJob(job.id, 'run');
        const pause = el('button', 'btn', job.paused ? 'Resume' : 'Pause');
        pause.type = 'button';
        pause.onclick = () => controlJob(job.id, job.paused ? 'resume' : 'pause');
        actions.append(run, pause);
        card.append(actions);
        return card;
    }));
    updateJobTimes();
}

// The times on the job cards, by the clock: elapsed, last run, and the countdown to the next.
function updateJobTimes() {
    if (!jobsView || !jobsView.data) return;
    const now = Date.now();
    for (const job of jobsView.data.jobs || []) {
        const card = document.querySelector(`.job-card[data-id="${CSS.escape(job.id)}"]`);
        if (!card) continue;
        const progress = card.querySelector('.job-progress .job-line');
        if (progress) {
            const count = job.total > 0 ? `${job.done.toLocaleString()} of ${job.total.toLocaleString()} · ` : '';
            progress.textContent = `${count}running for ${jobDuration(now - new Date(job.started))}`;
        }
        let last = 'Last run: not since the server started';
        if (job.last_end) {
            const took = jobDuration(new Date(job.last_end) - new Date(job.last_start));
            last = `Last run: ${jobWhen(job.last_end)}, took ${took}${job.last_result ? ` · ${job.last_result}` : ''}`;
        }
        card.querySelector('.job-last').textContent = last;
        let next;
        if (job.started) next = 'Next run: after this one';
        else if (job.next) {
            const left = new Date(job.next) - now;
            next = left > 0 ? `Next run: in ${jobDuration(left)}` : 'Next run: any moment';
        } else next = `Runs: ${job.schedule || 'when asked'}`;
        if (job.paused) next += ' (paused)';
        card.querySelector('.job-next').textContent = next;
    }
}

async function controlJob(id, action) {
    try {
        const data = await api.controlJob(id, action);
        if (jobsView) {
            jobsView.data = data;
            renderJobs();
        }
    } catch (e) {
        alert(`Couldn't ${action === 'run' ? 'start' : action} the job: ${e.message}`);
    }
}

async function refreshStreams() {
    const list = document.getElementById('streams-list');
    const updated = document.getElementById('streams-updated');
    let streams;
    try {
        streams = await api.getStreams();
    } catch (error) {
        updated.textContent = 'Could not load streams: ' + error.message;
        return;
    }
    updated.textContent = `Updated ${new Date().toLocaleTimeString()} · refreshes every 2 seconds`;
    list.innerHTML = '';
    if (!streams.length) {
        list.appendChild(el('p', 'streams-empty', 'Nothing is playing.'));
        return;
    }
    streams.forEach(s => list.appendChild(createStreamCard(s)));
}

function el(tag, className, text) {
    const e = document.createElement(tag);
    if (className) e.className = className;
    if (text !== undefined && text !== null) e.textContent = text;
    return e;
}

const STREAM_STATES = {
    playing: 'Playing',
    paused: 'Paused',
    streaming: 'Streaming',
    idle: 'Idle',
};

function clock(seconds) {
    if (!(seconds >= 0)) return '–';
    const s = Math.floor(seconds);
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
    const mm = String(m).padStart(h ? 2 : 1, '0'), ss = String(sec).padStart(2, '0');
    return h ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

function ago(iso) {
    const secs = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
    if (secs < 60) return `${secs}s ago`;
    if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
    return `${Math.floor(secs / 3600)}h ${Math.floor((secs % 3600) / 60)}m ago`;
}

function createStreamCard(s) {
    const card = el('div', 'stream-card');

    const head = el('div', 'stream-head');
    head.appendChild(el('div', 'stream-title', s.title || s.name));
    head.appendChild(el('span', `stream-badge state-${s.state}`, STREAM_STATES[s.state] || s.state));
    head.appendChild(el('span', `stream-badge mode-${s.mode}`, s.mode === 'transcode' ? 'Transcoding' : 'Direct play'));
    card.appendChild(head);

    card.appendChild(el('div', 'stream-who',
        `${s.user} · ${s.client} · ${s.client_ip} · started ${ago(s.started_at)}`));

    // Progress: the player's own position when it reports one; otherwise how far has been sent.
    const reported = s.position !== undefined && s.position !== null;
    const position = reported ? s.position : s.served_to;
    const duration = s.duration || 0;
    const progress = el('div', 'stream-progress');
    const bar = el('div', 'stream-bar');
    if (duration > 0) {
        const sent = el('div', 'stream-bar-sent');
        sent.style.width = `${Math.min(100, (s.served_to / duration) * 100)}%`;
        bar.appendChild(sent);
        const pos = el('div', 'stream-bar-pos');
        pos.style.width = `${Math.min(100, (position / duration) * 100)}%`;
        bar.appendChild(pos);
    }
    progress.appendChild(bar);
    let label = `${clock(position)} / ${clock(duration)}`;
    if (duration > 0) label += ` (${Math.round((position / duration) * 100)}%)`;
    label += reported ? ` · player position, reported ${ago(s.position_at)}` : ' · estimated from what has been sent';
    if (reported && s.served_to > position) label += ` · sent up to ${clock(s.served_to)}`;
    progress.appendChild(el('div', 'stream-progress-label', label));
    card.appendChild(progress);

    const facts = [
        ['File', s.name],
        ['Size', formatSize(s.size || 0)],
        ['Source resolution', s.width && s.height ? `${s.width}×${s.height}` : '–'],
        ['Sent', `${formatSize(s.bytes_sent || 0)} in ${s.requests} request${s.requests === 1 ? '' : 's'}` +
            (s.active_requests > 0 ? ` (${s.active_requests} open)` : '')],
        ['Last activity', ago(s.last_activity)],
    ];
    const t = s.transcode;
    if (t) {
        facts.push(
            ['Why transcoding', t.reason || '–'],
            ['Output video', t.video],
            ['Output audio', t.audio],
            ['Container', t.container],
            ['Source video', t.source_video || '–'],
            ['Source audio', t.source_audio || '–'],
            ['Started at', t.start > 0 ? `${clock(t.start)} into the file` : 'the beginning'],
            ['Transcoded to', `${clock(t.start + t.out_time)} (frame ${t.frame || 0})`],
            ['Transcoder', t.stalled ? 'Waiting for the player (paused, or far enough ahead)'
                : `${t.speed ? t.speed.toFixed(2) + '× realtime' : '–'} · ${t.fps ? Math.round(t.fps) + ' fps' : '–'} · ` +
                  `${t.out_bitrate ? Math.round(t.out_bitrate) + ' kb/s' : '–'}`],
        );
    }
    const grid = el('dl', 'stream-facts');
    facts.forEach(([k, v]) => {
        grid.appendChild(el('dt', null, k));
        grid.appendChild(el('dd', null, v));
    });
    card.appendChild(grid);

    if (t && t.args) {
        const details = el('details', 'stream-args');
        details.appendChild(el('summary', null, 'ffmpeg command'));
        details.appendChild(el('code', null, 'ffmpeg ' + t.args.join(' ')));
        card.appendChild(details);
    }
    const ua = el('div', 'stream-ua', s.user_agent);
    card.appendChild(ua);
    return card;
}

// Start the app last, once every top-level variable above is declared (init uses some of them,
// such as pendingScroll on a reload).
console.log('Calling init()');
init();
