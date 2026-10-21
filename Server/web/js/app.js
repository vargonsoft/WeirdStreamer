// Application State
console.log('app.js loaded');
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
const playerTitle = document.getElementById('player-title');
const imageTitle = document.getElementById('image-title');

// Natural sort helper function
function naturalCompare(s1, s2) {
    return s1.localeCompare(s2, undefined, { numeric: true, sensitivity: 'base' });
}

// Initialize
function init() {
    // Apply saved theme
    applyTheme();
    
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
        settingsBtn.addEventListener('click', showSettings);
    }
    document.getElementById('streams-btn').addEventListener('click', showStreams);
    document.getElementById('close-streams-btn').addEventListener('click', hideStreams);
    if (closeSettingsBtn) {
        closeSettingsBtn.addEventListener('click', () => {
            settingsScreen.classList.remove('active');
            mainScreen.classList.add('active');
        });
    }
    
    // Settings tabs
    document.querySelectorAll('.settings-tab').forEach(tab => {
        tab.addEventListener('click', () => {
            const tabName = tab.dataset.tab;
            document.querySelectorAll('.settings-tab').forEach(t => t.classList.remove('active'));
            document.querySelectorAll('.settings-tab-content').forEach(c => c.classList.remove('active'));
            tab.classList.add('active');
            document.getElementById(`${tabName}-tab`).classList.add('active');
            
            // Load users when switching to users tab
            if (tabName === 'users') {
                loadUsers();
            }
        });
    });
    
    // User management
    document.getElementById('create-user-btn')?.addEventListener('click', handleCreateUser);
    document.getElementById('change-password-btn')?.addEventListener('click', handleChangePassword);
    
    // Theme management
    document.getElementById('save-theme-btn')?.addEventListener('click', handleSaveTheme);
    
    // Browser back/forward buttons
    // Each history entry remembers how far down its page was, for Back and Forward. The page
    // restores it itself, after re-rendering, since content arrives asynchronously.
    history.scrollRestoration = 'manual';
    window.addEventListener('scroll', () => {
        clearTimeout(scrollSaveTimer);
        scrollSaveTimer = setTimeout(rememberScroll, 150);
    }, { passive: true });

    window.addEventListener('popstate', (e) => {
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
    setupDiscControls();
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
            hideDiscControls();
            videoPlayer.innerHTML = '';
            imageViewer.onload = null;
            imageViewer.onerror = null;
            imageViewer.src = '';
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
            hideDiscControls();
            videoPlayer.innerHTML = '';
        }
        if (e.target === imageModal) {
            imageModal.classList.remove('active');
            imageViewer.onload = null;
            imageViewer.onerror = null;
            imageViewer.src = '';
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
    document.getElementById('streams-btn').style.display = user.is_admin ? '' : 'none';
    
    // Update regenerate thumbnails button visibility
    updateRegenerateThumbnailsButton();

    // Load config for friendly path names for all users
    if (!state.currentConfig) {
        api.getConfig().then(config => {
            state.currentConfig = config;
            updateAvailableGroups();
        }).catch(err => {
            console.log('Could not load config:', err);
        });
    }
    
    // Connect to thumbnail updates via SSE
    api.connectThumbnailUpdates();
    api.onThumbnailUpdate((data) => {
        console.log('[SSE] Thumbnail update received:', data);
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
                        img.src = api.getThumbnailUrl(data.thumbnailURL) + '?t=' + Date.now();
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
    if (!history.state) {
        // So Back to this first page has a state to restore it from.
        const initial = urlView === 'library' || urlView === 'title'
            ? Object.fromEntries(['view', 'lib', 'kind', 'id'].filter(k => url.searchParams.has(k)).map(k => [k, url.searchParams.get(k)]))
            : { path: urlPath || '', type: urlType || 'all' };
        if (initial.id !== undefined) initial.id = Number(initial.id);
        history.replaceState(initial, '');
    }
    if (urlView === 'library' || urlView === 'title') {
        const lib = url.searchParams.get('lib');
        if (urlView === 'title') {
            showTitle(lib, url.searchParams.get('kind'), Number(url.searchParams.get('id')), false);
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
        console.log('Login successful:', response);
        showMainScreen();
    } catch (error) {
        console.error('Login failed:', error);
        loginError.textContent = error.message || 'Login failed. Please try again.';
    }
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
        const element = createMediaElement(item);
        mediaGrid.appendChild(element);
    });

    // The server's front page leads with what's in progress, in every library.
    if (!state.currentPath) {
        const path = state.currentPath;
        createContinueRow().then(row => {
            if (row && state.view === 'folders' && state.currentPath === path) {
                directoryList.innerHTML = '';
                directoryList.appendChild(row);
            }
        });
    }
}

function createDirectoryElement(dir) {
    const div = document.createElement('div');
    div.className = 'media-item directory-item';
    div.onclick = () => {
        if (!state.currentPath && dir.library_type && getLibraryMode(dir.path) === 'library') {
            showLibrary(dir.path);
            return;
        }
        state.currentPath = dir.path;
        loadMedia();
    };
    
    const thumbnail = document.createElement('div');
    thumbnail.className = 'media-thumbnail folder-thumbnail';
    if (dir.metadata && dir.metadata.poster) {
        const img = document.createElement('img');
        img.src = API_BASE + dir.metadata.poster;
        img.alt = dir.name;
        img.className = 'poster';
        img.onerror = () => {
            img.insertAdjacentHTML('afterend', placeholderIcon('folder'));
            img.remove();
        };
        thumbnail.appendChild(img);
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

function createMediaElement(item) {
    const div = document.createElement('div');
    div.className = 'media-item';
    div.onclick = () => handleMediaClick(item);
    
    const thumbnail = document.createElement('div');
    thumbnail.className = 'media-thumbnail';
    
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
    info.appendChild(size);
    const match = createMatchLabel(item.metadata);
    if (match) info.appendChild(match);
    if (item.type === 'video' && canFixMatches()) {
        thumbnail.appendChild(createFixMatchButton({ path: item.path }, item.name));
    }
    if (item.type === 'video') {
        addProgressOverlay(thumbnail, item.playback, item.duration);
        thumbnail.appendChild(createCardActions(item));
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

// Check if video needs transcoding (will be converted to MP4 by server)
function needsTranscoding(filename) {
    return false; // Disable transcoding for now to let browser handle it natively if possible
    const ext = filename.toLowerCase().split('.').pop();
    const incompatibleFormats = ['3gp', 'avi', 'divx', 'xvid', 'wmv', 'asf', 'flv', 'mkv'];
    return incompatibleFormats.includes(ext);
}

// startAt: where to start, in seconds (resuming).
function playVideo(item, forceTranscode = false, startAt = 0) {
    if (isDiscImage(item)) {
        playDisc(item, startAt);
        return;
    }
    hideDiscControls();
    console.log('Playing video:', item, 'Force transcode:', forceTranscode);
    console.log('API token exists:', !!api.token);
    let streamUrl = api.getStreamUrl(item.path, forceTranscode);
    // A transcoded stream starts at the resume point and its time counts from there; a direct
    // one seeks once it's loaded.
    streamOffset = forceTranscode && startAt > 0 ? startAt : 0;
    if (streamOffset > 0) streamUrl += `&start=${streamOffset.toFixed(2)}`;
    console.log('Final stream URL:', streamUrl);
    
    playerTitle.textContent = item.name;
    
    // Properly reset the video element
    videoPlayer.pause();
    videoPlayer.removeAttribute('src');
    videoPlayer.innerHTML = '';
    videoPlayer.load(); // Clear any existing source
    
    const source = document.createElement('source');
    source.src = streamUrl;
    
    // If file needs transcoding or forced, server will send MP4
    if (forceTranscode || needsTranscoding(item.name)) {
        console.log('File will be transcoded to MP4');
        source.type = 'video/mp4';
    } else {
        source.type = item.mime_type || 'video/mp4';
    }
    
    console.log('Video source type:', source.type);
    videoPlayer.appendChild(source);
    playerModal.classList.add('active');
    videoPlayer.load();
    
    // Show/hide transcode button based on whether we're already transcoding
    const transcodeBtn = document.getElementById('transcode-btn');
    if (forceTranscode || needsTranscoding(item.name)) {
        transcodeBtn.style.display = 'none';
    } else {
        transcodeBtn.style.display = '';
        transcodeBtn.onclick = () => {
            const at = playerPosition();
            playerModal.classList.remove('active');
            videoPlayer.pause();
            playVideo(item, true, at);
        };
    }
    
    startPlaybackReports(item);

    // Wait for video to be ready before playing
    videoPlayer.onloadedmetadata = () => {
        console.log('Video loaded, playing...');
        if (startAt > 0 && streamOffset === 0) videoPlayer.currentTime = startAt;
        videoPlayer.play().catch(err => {
            console.error('Error playing video:', err);
        });
    };
    
    videoPlayer.onerror = (e) => {
        console.error('Video error:', e, videoPlayer.error);
    };
}

// The player tells the server where it is, for the admin Streams page and the user's progress:
// every few seconds while playing, right away on play, pause and the end, and on closing.
let playbackReportTimer = null;
let playbackReport = null; // sends the current position: (ended) => void
let streamOffset = 0; // a transcoded stream started partway in: where its time 0 is

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
    videoPlayer.onended = () => send(true);
}

// Sends the last position as the player closes, then shows the new progress.
function finishPlaybackReports() {
    if (!playerModal.classList.contains('active') || !playbackReport) return;
    playbackReport(videoPlayer.ended);
    playbackReport = null;
    clearInterval(playbackReportTimer);
    setTimeout(refreshCurrentView, 300);
}

// Where playback is in the file: discs and resumed transcodes start their stream at an offset.
function playerPosition() {
    return (disc && disc.item ? disc.offset : streamOffset) + (videoPlayer.currentTime || 0);
}

// ---------------------------------------------------------------------------------------------
// DVD disc images: always transcoded by the server, which plays one title of the disc at a time.
// Choosing a title, chapter, angle or track restarts the stream at the right place.

let disc = null; // { item, info, title, angle, audio, subtitle, offset }

function isDiscImage(item) {
    return /\.iso$/i.test(item.path || item.name || '');
}

function hideDiscControls() {
    disc = null;
    document.getElementById('disc-controls').hidden = true;
    // Discs use their own controls (the browser's seek bar only covers the current stream).
    videoPlayer.controls = true;
    videoPlayer.onclick = null;
    videoPlayer.onplay = videoPlayer.onpause = videoPlayer.onvolumechange = null;
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
}

async function playDisc(item, startAt = 0) {
    disc = { item, info: null, title: 0, angle: 1, audio: 0, subtitle: -1, offset: 0 };
    videoPlayer.controls = false;
    playerTitle.textContent = item.name + ' — reading disc…';
    document.getElementById('transcode-btn').style.display = 'none';
    document.getElementById('disc-controls').hidden = true;
    playerModal.classList.add('active');
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
    playerTitle.textContent = item.name;
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
        `&title=${t.number}&angle=${disc.angle}&audio=${disc.audio}&subtitle=${disc.subtitle}`;
    if (disc.offset > 0) url += `&start=${disc.offset.toFixed(2)}`;
    videoPlayer.pause();
    videoPlayer.removeAttribute('src');
    videoPlayer.innerHTML = '';
    const source = document.createElement('source');
    source.src = url;
    source.type = 'video/mp4';
    videoPlayer.appendChild(source);
    videoPlayer.load();
    videoPlayer.onloadedmetadata = () => videoPlayer.play().catch(err => console.error('Error playing disc:', err));
    // The stream can't be seeked into beyond what has arrived: a seek there restarts it.
    videoPlayer.onseeking = () => {
        if (videoPlayer.readyState < 1) return; // the new stream is still opening
        const t = videoPlayer.currentTime;
        const b = videoPlayer.buffered;
        for (let i = 0; i < b.length; i++) {
            if (t >= b.start(i) && t <= b.end(i)) return;
        }
        loadDiscStream(disc.offset + t);
    };
    videoPlayer.onerror = () => console.error('Disc video error:', videoPlayer.error);
    videoPlayer.ontimeupdate = updateDiscPosition;
    videoPlayer.onclick = toggleDiscPlay;
    startPlaybackReports(disc.item);
    // After the reports, which set onplay/onpause themselves.
    const report = videoPlayer.onplay;
    videoPlayer.onplay = videoPlayer.onpause = () => { if (report) report(); updateDiscButtons(); };
    videoPlayer.onvolumechange = updateDiscButtons;
    updateDiscButtons();
    updateDiscPosition();
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
    const titles = [...info.titles].sort((a, b) => a.number - b.number);
    fillSelect(document.getElementById('disc-title'), titles.map(x =>
        [x.number, `${x.number}: ${clock(x.duration)}${x.number === info.main_title ? ' (main)' : ''}`]), t.number);
    const chapters = t.chapters.length ? t.chapters : [0];
    fillSelect(document.getElementById('disc-chapter'), chapters.map((start, i) =>
        [i, `${i + 1}: ${clock(start)}`]), 0);
    document.getElementById('disc-angle-label').hidden = t.angles <= 1;
    fillSelect(document.getElementById('disc-angle'),
        Array.from({ length: t.angles }, (_, i) => [i + 1, `${i + 1} of ${t.angles}`]), disc.angle);
    fillSelect(document.getElementById('disc-audio'),
        t.audio.length ? t.audio.map(a => [a.index, trackLabel(a)]) : [[0, 'None']], disc.audio);
    fillSelect(document.getElementById('disc-subtitle'),
        [[-1, 'Off'], ...t.subtitles.map(s => [s.index, trackLabel(s)])], disc.subtitle);
    document.getElementById('disc-seek').max = String(Math.max(1, Math.floor(t.duration)));
    document.getElementById('disc-controls').hidden = false;
}

function updateDiscPosition() {
    if (!disc || !disc.info) return;
    const t = discTitle(), pos = playerPosition();
    const seek = document.getElementById('disc-seek');
    if (!discSeekDragging) seek.value = String(Math.floor(pos));
    document.getElementById('disc-position').textContent =
        `${clock(discSeekDragging ? Number(seek.value) : pos)} / ${clock(t.duration)}`;
    // Keep the chapter list on the chapter playing.
    let current = 0;
    t.chapters.forEach((start, i) => { if (pos + 0.5 >= start) current = i; });
    const select = document.getElementById('disc-chapter');
    if (document.activeElement !== select && select.value !== String(current)) select.value = String(current);
}

let discSeekDragging = false;

// Material icons (Apache License 2.0), inlined so the page loads no icon font.
const MATERIAL_ICONS = {
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
    folder: 'M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z',
    movie: 'M18 4l2 4h-3l-2-4h-2l2 4h-3l-2-4H8l2 4H7L5 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V4h-4z',
    image: 'M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z',
    description: 'M14 2H6c-1.1 0-1.99.9-1.99 2L4 20c0 1.1.89 2 1.99 2H18c1.1 0 2-.9 2-2V8l-6-6zm2 16H8v-2h8v2zm0-4H8v-2h8v2zm-3-5V3.5L18.5 9H13z',
    tv: 'M21 3H3c-1.1 0-2 .9-2 2v12c0 1.1.9 2 2 2h5v2h8v-2h5c1.1 0 1.99-.9 1.99-2L23 5c0-1.1-.9-2-2-2zm0 14H3V5h18v12z',
    theaters: 'M18 3v2h-2V3H8v2H6V3H4v18h2v-2h2v2h8v-2h2v2h2V3h-2zM8 17H6v-2h2v2zm0-4H6v-2h2v2zm0-4H6V7h2v2zm10 8h-2v-2h2v2zm0-4h-2v-2h2v2zm0-4h-2V7h2v2z',
    check: 'M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z',
    close: 'M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z',
    search: 'M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z',
};

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
    else btn.textContent = text;
}

function iconSvg(name, { mirrored = false } = {}) {
    const flip = mirrored ? ' transform="matrix(-1 0 0 1 24 0)"' : '';
    return `<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true"><path fill="currentColor"${flip} d="${MATERIAL_ICONS[name]}"/></svg>`;
}

function setIcon(id, name, label) {
    const btn = document.getElementById(id);
    if (btn.dataset.icon === name) return;
    btn.dataset.icon = name;
    btn.innerHTML = iconSvg(name) + (label ? `<span>${label}</span>` : '');
}

function toggleDiscPlay() {
    if (videoPlayer.paused) videoPlayer.play().catch(() => {});
    else videoPlayer.pause();
}

function updateDiscButtons() {
    setIcon('disc-play', videoPlayer.paused ? 'play' : 'pause');
    setIcon('disc-mute', videoPlayer.muted || videoPlayer.volume === 0 ? 'volumeOff' : 'volumeUp');
    setIcon('disc-fullscreen', document.fullscreenElement ? 'fullscreenExit' : 'fullscreen');
    document.getElementById('disc-volume').value = String(videoPlayer.muted ? 0 : videoPlayer.volume);
}

// In fullscreen the controls and cursor hide after a moment without mouse or keyboard use.
const DISC_IDLE_MS = 2500;
let discIdleTimer = null;

function discActivity() {
    const box = playerModal.querySelector('.modal-content');
    box.classList.remove('controls-hidden');
    clearTimeout(discIdleTimer);
    if (!disc || document.fullscreenElement !== box) return;
    discIdleTimer = setTimeout(function hide() {
        if (discSeekDragging) { discIdleTimer = setTimeout(hide, DISC_IDLE_MS); return; }
        box.classList.add('controls-hidden');
    }, DISC_IDLE_MS);
}

function toggleDiscFullscreen() {
    // The video and the disc controls together, so the seek bar stays usable.
    const box = playerModal.querySelector('.modal-content');
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    else box.requestFullscreen().catch(() => {});
}

function setupDiscControls() {
    // Circular arrows: back (counterclockwise) for -10s, mirrored forward for +30s.
    document.getElementById('disc-back').innerHTML = iconSvg('replay') + '<span>−10s</span>';
    document.getElementById('disc-forward').innerHTML = iconSvg('replay', { mirrored: true }) + '<span>+30s</span>';
    document.addEventListener('fullscreenchange', () => {
        if (disc) updateDiscButtons();
        discActivity();
    });
    const box = playerModal.querySelector('.modal-content');
    ['mousemove', 'mousedown', 'wheel', 'touchstart'].forEach(type =>
        box.addEventListener(type, discActivity, { passive: true }));
    document.addEventListener('keydown', () => { if (document.fullscreenElement === box) discActivity(); }, true);
    updateDiscButtons();
    const seek = document.getElementById('disc-seek');
    seek.addEventListener('input', () => { discSeekDragging = true; updateDiscPosition(); });
    seek.addEventListener('change', () => {
        discSeekDragging = false;
        if (disc && disc.info) loadDiscStream(Number(seek.value));
    });
    document.getElementById('disc-back').addEventListener('click', () => {
        if (disc && disc.info) loadDiscStream(Math.max(0, playerPosition() - 10));
    });
    document.getElementById('disc-forward').addEventListener('click', () => {
        if (disc && disc.info) loadDiscStream(playerPosition() + 30);
    });
    document.getElementById('disc-play').addEventListener('click', toggleDiscPlay);
    document.getElementById('disc-mute').addEventListener('click', () => {
        videoPlayer.muted = !videoPlayer.muted;
        if (!videoPlayer.muted && videoPlayer.volume === 0) videoPlayer.volume = 1;
    });
    document.getElementById('disc-volume').addEventListener('input', (e) => {
        videoPlayer.volume = Number(e.target.value);
        videoPlayer.muted = videoPlayer.volume === 0;
    });
    document.getElementById('disc-fullscreen').addEventListener('click', toggleDiscFullscreen);
    document.addEventListener('keydown', (e) => {
        if (!disc || !disc.info || !playerModal.classList.contains('active')) return;
        // Sliders keep their arrow keys, lists and buttons their own keys (Space clicks a button).
        const target = e.target instanceof Element ? e.target : document.body;
        if (target.matches('input, select, textarea') && e.key !== ' ') return;
        if (target.matches('select, textarea, button')) return;
        const key = e.key.toLowerCase();
        if (key === ' ' || key === 'k') toggleDiscPlay();
        else if (key === 'arrowleft') loadDiscStream(Math.max(0, playerPosition() - 10));
        else if (key === 'arrowright') loadDiscStream(playerPosition() + 30);
        else if (key === 'm') videoPlayer.muted = !videoPlayer.muted;
        else if (key === 'f') toggleDiscFullscreen();
        else return;
        e.preventDefault();
    });
    const on = (id, handler) => document.getElementById(id).addEventListener('change', (e) => {
        if (disc && disc.info) handler(Number(e.target.value));
    });
    on('disc-title', (n) => {
        disc.title = n;
        disc.angle = 1;
        disc.audio = 0;
        disc.subtitle = -1;
        renderDiscControls();
        loadDiscStream(0);
    });
    on('disc-chapter', (i) => loadDiscStream(discTitle().chapters[i] || 0));
    on('disc-angle', (a) => { disc.angle = a; loadDiscStream(playerPosition()); });
    on('disc-audio', (a) => { disc.audio = a; loadDiscStream(playerPosition()); });
    on('disc-subtitle', (s) => { disc.subtitle = s; loadDiscStream(playerPosition()); });
}

function viewImage(item) {
    console.log('Viewing image:', item);
    console.log('API token exists:', !!api.token);
    const streamUrl = api.getStreamUrl(item.path);
    console.log('Final image URL:', streamUrl);
    imageTitle.textContent = item.name;
    imageViewer.src = '';
    imageViewer.alt = item.name;
    imageViewer.onload = () => {
        console.log('Image loaded successfully');
        imageModal.classList.add('active');
    };
    imageViewer.onerror = () => {
        console.error('Failed to load image:', item.path);
        alert('Failed to load image');
    };
    imageViewer.src = streamUrl;
}

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
    const sizes = ['B', 'KB', 'MB', 'GB'];
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
async function showSettings() {
    const user = JSON.parse(localStorage.getItem('user') || '{}');

    // Hide admin tabs for non-admin users
    const serverTabBtn = document.querySelector('.settings-tab[data-tab="server"]');
    const usersTabBtn = document.querySelector('.settings-tab[data-tab="users"]');

    if (serverTabBtn) serverTabBtn.style.display = user.is_admin ? 'block' : 'none';
    if (usersTabBtn) usersTabBtn.style.display = user.is_admin ? 'block' : 'none';

    // Set active tab to 'account' if not admin
    if (!user.is_admin) {
        document.querySelectorAll('.settings-tab').forEach(t => t.classList.remove('active'));
        document.querySelectorAll('.settings-tab-content').forEach(c => c.classList.remove('active'));
        const accountTabBtn = document.querySelector('.settings-tab[data-tab="account"]');
        if (accountTabBtn) accountTabBtn.classList.add('active');
        document.getElementById('account-tab').classList.add('active');
    }

    if (user.is_admin) {
        try {
            const config = await api.getConfig();
            state.currentConfig = config;
            updateAvailableGroups();

            // Populate form fields
            document.getElementById('server-port').value = config.server.port;
            document.getElementById('server-host').value = config.server.host;
            document.getElementById('enable-http2').checked = config.streaming.enableHTTP2;
            document.getElementById('chunk-size').value = config.streaming.chunkSize;
            const metadata = config.metadata || {};
            document.getElementById('tmdb-token').value = metadata.tmdbAccessToken || '';
            document.getElementById('metadata-language').value = metadata.language || '';

            // Render media paths
            renderMediaPaths(config.media.paths);

            // Setup event listeners for settings
            document.getElementById('add-path-btn').onclick = handleAddPath;
            document.getElementById('save-settings-btn').onclick = handleSaveSettings;
        } catch (error) {
            console.error('Failed to load admin settings:', error);
            alert('Failed to load system settings: ' + error.message);
        }
    }

    // Apply theme selection (accessible to all)
    applyTheme();

    // Setup theme radio button listeners for immediate preview
    document.querySelectorAll('input[name="theme"]').forEach(radio => {
        radio.addEventListener('change', (e) => {
            document.documentElement.setAttribute('data-theme', e.target.value);
        });
    });

    // Show settings screen
    mainScreen.classList.remove('active');
    settingsScreen.classList.add('active');
}

function updateAvailableGroups() {
    if (!state.currentConfig || !state.currentConfig.media || !state.currentConfig.media.paths) return;

    const groupsSet = new Set();
    state.currentConfig.media.paths.forEach(path => {
        if (path.groups) {
            path.groups.forEach(group => groupsSet.add(group));
        }
    });
    state.availableGroups = Array.from(groupsSet).sort();
}

// Library types the server accepts; keep in sync with config.LibraryType* on the server.
const LIBRARY_TYPES = [
    { value: '', label: 'No type' },
    { value: 'movie', label: 'Movies' },
    { value: 'tv', label: 'TV Shows' },
];

// Sets a media path's type, omitting the key for "no type" so the saved config stays clean.
function setMediaPathType(mediaPath, type) {
    if (type) {
        mediaPath.type = type;
    } else {
        delete mediaPath.type;
    }
}

function renderMediaPaths(paths) {
    const pathsList = document.getElementById('media-paths-list');
    pathsList.innerHTML = '';
    
    paths.forEach((mediaPath, index) => {
        const div = document.createElement('div');
        div.style.cssText = 'margin-bottom: 12px; padding: 10px; background: var(--card-bg); border-radius: 5px;';
        
        const nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.value = mediaPath.name;
        nameInput.className = 'settings-input';
        nameInput.style.cssText = 'margin-bottom: 5px; width: calc(100% - 20px);';
        nameInput.placeholder = 'Display Name';
        nameInput.onchange = (e) => {
            state.currentConfig.media.paths[index].name = e.target.value;
        };
        
        const groupsInput = document.createElement('input');
        groupsInput.type = 'text';
        groupsInput.value = (mediaPath.groups || []).join(', ');
        groupsInput.className = 'settings-input';
        groupsInput.style.cssText = 'margin-bottom: 5px; width: calc(100% - 20px); font-size: 12px;';
        groupsInput.placeholder = 'Allowed Groups (comma separated)';
        groupsInput.onchange = (e) => {
            state.currentConfig.media.paths[index].groups = e.target.value.split(',').map(s => s.trim()).filter(s => s);
            updateAvailableGroups();
        };

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
        
        pathDiv.appendChild(pathText);
        pathDiv.appendChild(removeBtn);
        
        div.appendChild(nameInput);
        div.appendChild(groupsInput);
        div.appendChild(typeSelect);
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
        renderMediaPaths(state.currentConfig.media.paths);
        input.value = '';
        typeSelect.value = '';
    } else {
        alert('Path already exists');
    }
}

function handleRemovePath(index) {
    if (confirm('Remove this path?')) {
        state.currentConfig.media.paths.splice(index, 1);
        renderMediaPaths(state.currentConfig.media.paths);
        updateAvailableGroups();
    }
}

async function handleSaveSettings() {
    const messageEl = document.getElementById('settings-message');
    
    try {
        // Update config object with form values
        state.currentConfig.server.port = parseInt(document.getElementById('server-port').value);
        state.currentConfig.server.host = document.getElementById('server-host').value;
        state.currentConfig.streaming.enableHTTP2 = document.getElementById('enable-http2').checked;
        state.currentConfig.streaming.chunkSize = parseInt(document.getElementById('chunk-size').value);
        state.currentConfig.metadata = {
            ...(state.currentConfig.metadata || {}),
            tmdbAccessToken: document.getElementById('tmdb-token').value.trim(),
            language: document.getElementById('metadata-language').value.trim(),
        };
        
        const result = await api.updateConfig(state.currentConfig);
        // If server returned updated config (with IDs), adopt it locally
        if (result && result.config) {
            state.currentConfig = result.config;
            updateAvailableGroups();
        }
        messageEl.textContent = (result && result.message) ? result.message : 'Configuration updated.';
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
        
        // Fetch fresh available groups list
        const groups = await api.getAvailableGroups();
        state.availableGroups = groups;

        usersList.innerHTML = users.map(user => {
            const isMe = user.username === currentUser.username;
            // Use the data from the server directly
            const isAdminChecked = user.is_admin ? 'checked' : '';

            return `
                <div class="user-item">
                    <div class="user-info">
                        <strong>${user.username}</strong>
                        ${user.is_admin ? '<span class="user-badge">ADMIN</span>' : ''}
                        ${isMe ? '<span class="user-badge user-badge-you">YOU</span>' : ''}
                        <div style="margin-top: 10px; display: flex; align-items: center; gap: 15px; flex-wrap: wrap;">
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: var(--muted-text);">Group: </span>
                                <select onchange="updateUserGroup('${user.username}', this.value)" class="settings-input" style="width: auto; padding: 2px 5px; font-size: 12px;">
                                    <option value="">None</option>
                                    ${state.availableGroups.map(g => `<option value="${g}" ${user.group === g ? 'selected' : ''}>${g}</option>`).join('')}
                                </select>
                            </div>
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: var(--muted-text);">Admin: </span>
                                <input type="checkbox" ${isAdminChecked} ${isMe ? 'disabled' : ''} onchange="updateUserAdmin('${user.username}', this.checked)">
                            </div>
                        </div>
                    </div>
                    ${!isMe ?
                        `<button type="button" class="btn btn-danger" onclick="deleteUser('${user.username}')">${iconSvg('delete')}<span class="btn-label">Delete</span></button>` :
                        '<span style="color: var(--muted-text); font-style: italic; font-size: 12px;">Cannot delete yourself</span>'}
                </div>
            `;
        }).join('');
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
    const group = document.getElementById('new-user-group').value.trim();
    const isAdmin = document.getElementById('new-user-admin').checked;
    
    if (!username || !password) {
        messageEl.textContent = 'Username and password are required';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    try {
        await api.createUser(username, password, isAdmin, group);
        messageEl.textContent = 'User created successfully';
        messageEl.style.color = '#27ae60';
        
        document.getElementById('new-username').value = '';
        document.getElementById('new-password').value = '';
        document.getElementById('new-user-group').value = '';
        document.getElementById('new-user-admin').checked = false;
        
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

// Theme Management
function applyTheme() {
    const savedTheme = localStorage.getItem('app-theme') || 'default';
    document.documentElement.setAttribute('data-theme', savedTheme);
    
    // Update radio buttons if on settings page
    const themeRadio = document.getElementById(`theme-${savedTheme}`);
    if (themeRadio) {
        themeRadio.checked = true;
    }
}

function handleSaveTheme() {
    const selectedTheme = document.querySelector('input[name="theme"]:checked')?.value;
    if (!selectedTheme) return;
    
    localStorage.setItem('app-theme', selectedTheme);
    document.documentElement.setAttribute('data-theme', selectedTheme);
    
    const messageEl = document.getElementById('theme-message');
    messageEl.textContent = 'Theme saved successfully!';
    messageEl.style.color = '#27ae60';
    
    setTimeout(() => {
        messageEl.textContent = '';
    }, 2000);
}

// Initialize app when DOM is ready
console.log('Calling init()');
init();

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
    state.view = view;
    const library = view !== 'folders';
    document.getElementById('library-view').hidden = !library;
    mediaGrid.hidden = library;
    directoryList.hidden = library;
    document.querySelector('.tabs').hidden = library;
}

function refreshCurrentView() {
    if (state.view === 'library') showLibrary(state.libraryId, false, true);
    else if (state.view === 'title') showTitle(state.libraryId, state.titleRef.kind, state.titleRef.id, false, true);
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
    [['folders', 'Folders'], ['library', 'Library']].forEach(([mode, label]) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.textContent = label;
        const active = mode === 'folders' ? state.view === 'folders' : state.view !== 'folders';
        btn.className = active ? 'active' : '';
        btn.setAttribute('aria-pressed', String(active));
        btn.onclick = () => {
            setLibraryMode(state.libraryId, mode);
            if (mode === 'library') {
                showLibrary(state.libraryId);
            } else {
                state.currentPath = state.libraryId;
                loadMedia();
            }
        };
        toggle.appendChild(btn);
    });
    return toggle;
}

function libraryBreadcrumb(titleName) {
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
        showLibrary(state.libraryId);
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

    const continueRow = await createContinueRow(libId);
    container.innerHTML = '';
    if (continueRow) container.appendChild(continueRow);
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
    return { path: f.path, name: f.name, type: 'video', size: f.size || 0, duration: f.duration || 0,
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
    // The whole title: a movie's files, or every episode of a show.
    const titleItems = kind === 'movie' || (t.files || []).length ? state.mediaItems
        : state.mediaItems.filter(i => !(t.other_files || []).some(f => f.path === i.path));
    if (titleItems.length) {
        const mark = createWatchedToggle(titleItems, 'btn secondary icon-btn watched-btn');
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
            const person = document.createElement('div');
            person.className = 'cast-member';
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
        info.appendChild(cast);
    }
    hero.appendChild(info);
    return hero;
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
    const grid = document.createElement('div');
    grid.className = 'media-grid';
    files.forEach(f => {
        const item = libraryFileItem(f);
        const wrap = document.createElement('div');
        wrap.className = 'title-file';
        wrap.appendChild(createMediaElement(item));
        wrap.appendChild(createPlayButtons(item));
        grid.appendChild(wrap);
    });
    section.appendChild(grid);
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

    const seasonActions = el('div', 'season-actions');
    const seasonItems = (se) => se.episodes.flatMap(ep => ep.files.map(libraryFileItem));
    const select = (index) => {
        state.seasonIndex = index;
        picker.querySelectorAll('button').forEach((b, i) => {
            b.classList.toggle('active', i === index);
            b.setAttribute('aria-selected', String(i === index));
        });
        list.innerHTML = '';
        seasons[index].episodes.forEach(ep => list.appendChild(createEpisodeRow(ep)));
        seasonActions.innerHTML = '';
        const toggle = createWatchedToggle(seasonItems(seasons[index]), 'btn secondary icon-btn watched-btn');
        toggle.title = toggle.title.replace('Mark', 'Mark season');
        seasonActions.appendChild(toggle);
    };
    seasons.forEach((se, i) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.setAttribute('role', 'tab');
        // An episode is watched when any of its files is.
        const watched = se.episodes.filter(ep => ep.files.some(f => f.playback && f.playback.watched)).length;
        btn.textContent = watched
            ? `${se.name} (${watched}/${se.episodes.length})` : `${se.name} (${se.episodes.length})`;
        if (watched === se.episodes.length) btn.classList.add('all-watched');
        btn.onclick = () => select(i);
        picker.appendChild(btn);
    });
    section.append(picker, seasonActions, list);
    // A refresh (after playing or marking) stays on the season that was showing.
    const keep = state.titleSeasonFor === t.tmdb_id ? state.seasonIndex : 0;
    state.titleSeasonFor = t.tmdb_id;
    select(Math.min(keep || 0, seasons.length - 1));
    return section;
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
    text.append(title, meta, overview);

    const actions = document.createElement('div');
    actions.className = 'episode-actions';
    ep.files.forEach(f => {
        const item = libraryFileItem(f);
        const ext = f.name.split('.').pop().toUpperCase();
        actions.appendChild(createPlayButtons(item, ep.files.length > 1 ? ext : ''));
        if (canFixMatches()) {
            const fix = createFixMatchButton({ path: f.path }, f.name);
            fix.classList.add('inline');
            actions.appendChild(fix);
        }
    });
    actions.appendChild(createWatchedToggle(ep.files.map(libraryFileItem), 'btn secondary icon-btn watched-btn'));
    row.append(still, text, actions);
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
function createWatchedToggle(items, className, { label = false } = {}) {
    const btn = el('button', className);
    btn.type = 'button';
    const watched = isWatched(items);
    const text = watched ? 'Mark unwatched' : 'Mark watched';
    btn.title = text;
    btn.setAttribute('aria-label', text);
    btn.classList.toggle('is-watched', watched);
    btn.innerHTML = iconSvg('check') + (label ? `<span>${text}</span>` : '');
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
    section.appendChild(row);
    return section;
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
    const { title, subtitle } = continueTitle(item);
    card.append(thumb, el('div', 'continue-title', title));
    if (subtitle) card.appendChild(el('div', 'continue-subtitle', subtitle));
    const left = timeLeftText(item);
    if (left) card.appendChild(el('div', 'continue-subtitle', left));
    const play = () => playVideo(item, false, resumePosition(item));
    card.onclick = play;
    card.onkeydown = (e) => { if (e.key === 'Enter') play(); };
    return card;
}

// ---------------------------------------------------------------------------------------------
// Streams page (admins): every stream playing now, refreshed every few seconds

let streamsTimer = null;

function showStreams() {
    mainScreen.classList.remove('active');
    document.getElementById('streams-screen').classList.add('active');
    refreshStreams();
    clearInterval(streamsTimer);
    streamsTimer = setInterval(refreshStreams, 2000);
}

function hideStreams() {
    clearInterval(streamsTimer);
    streamsTimer = null;
    document.getElementById('streams-screen').classList.remove('active');
    mainScreen.classList.add('active');
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
