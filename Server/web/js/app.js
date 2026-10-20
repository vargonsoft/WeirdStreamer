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
    window.addEventListener('popstate', (e) => {
        if (e.state && api.isAuthenticated()) {
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

    // Modal close buttons
    document.querySelectorAll('.close').forEach(closeBtn => {
        closeBtn.addEventListener('click', () => {
            playerModal.classList.remove('active');
            imageModal.classList.remove('active');
            // Properly stop video loading
            videoPlayer.pause();
            videoPlayer.src = '';
            videoPlayer.load(); // This aborts the current HTTP request
            videoPlayer.onloadedmetadata = null;
            videoPlayer.onerror = null;
            videoPlayer.innerHTML = '';
            imageViewer.onload = null;
            imageViewer.onerror = null;
            imageViewer.src = '';
        });
    });
    
    // Close modal on outside click
    window.addEventListener('click', (e) => {
        if (e.target === playerModal) {
            playerModal.classList.remove('active');
            videoPlayer.pause();
            videoPlayer.src = '';
            videoPlayer.load(); // This aborts the current HTTP request
            videoPlayer.onloadedmetadata = null;
            videoPlayer.onerror = null;
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
        settingsBtn.style.display = 'inline-block';
    }

    // Show scan button only for admins
    if (scanBtn) {
        scanBtn.style.display = user.is_admin ? 'inline-block' : 'none';
    }
    
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
                        thumbnailDiv.classList.remove('no-thumb');
                        thumbnailDiv.innerHTML = '';
                        const img = document.createElement('img');
                        // Add cache buster to force reload
                        img.src = api.getThumbnailUrl(data.thumbnailURL) + '?t=' + Date.now();
                        img.alt = item.name;
                        img.onerror = () => {
                            thumbnailDiv.classList.add('no-thumb');
                            thumbnailDiv.innerHTML = getMediaIcon(item.type);
                        };
                        thumbnailDiv.appendChild(img);
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
        scanBtn.textContent = 'Scanning...';
        await api.scanMedia();
        alert('Media scan started. This may take a while.');
        setTimeout(loadMedia, 2000);
    } catch (error) {
        alert('Failed to start scan: ' + error.message);
    } finally {
        scanBtn.disabled = false;
        scanBtn.textContent = 'Scan Media';
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
        regenerateThumbnailsBtn.textContent = '🔄 Regenerating...';
        await api.regenerateThumbnails(state.currentPath);
    } catch (error) {
        alert('Failed to regenerate thumbnails: ' + error.message);
    } finally {
        regenerateThumbnailsBtn.disabled = false;
        regenerateThumbnailsBtn.textContent = '🔄 Regenerate Thumbnails';
    }
}

function updateRegenerateThumbnailsButton() {
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    if (regenerateThumbnailsBtn) {
        // Show button only for admins when in a folder (not root)
        if (user.is_admin && state.currentPath) {
            regenerateThumbnailsBtn.style.display = 'inline-block';
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
        
        state.libraryId = state.currentPath.split('/')[0] || '';
        updateBreadcrumb();
        updateRegenerateThumbnailsButton();
        renderMedia();
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
            currentPath += '/' + part;
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
        img.onerror = () => img.replaceWith(document.createTextNode('📁'));
        thumbnail.appendChild(img);
    } else {
        thumbnail.textContent = '📁';
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
        thumbnail.textContent = getMediaIcon(item.type);
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
    div.appendChild(thumbnail);
    div.appendChild(info);
    
    return div;
}

function handleMediaClick(item) {
    if (item.type === 'video') {
        playVideo(item);
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

function playVideo(item, forceTranscode = false) {
    console.log('Playing video:', item, 'Force transcode:', forceTranscode);
    console.log('API token exists:', !!api.token);
    const streamUrl = api.getStreamUrl(item.path, forceTranscode);
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
        transcodeBtn.style.display = 'inline-block';
        transcodeBtn.onclick = () => {
            playerModal.classList.remove('active');
            videoPlayer.pause();
            playVideo(item, true);
        };
    }
    
    // Wait for video to be ready before playing
    videoPlayer.onloadedmetadata = () => {
        console.log('Video loaded, playing...');
        videoPlayer.play().catch(err => {
            console.error('Error playing video:', err);
        });
    };
    
    videoPlayer.onerror = (e) => {
        console.error('Video error:', e, videoPlayer.error);
    };
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

function getMediaIcon(type) {
    const icons = {
        video: '🎬',
        image: '🖼️',
        document: '📄',
    };
    return icons[type] || '📁';
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
        div.style.cssText = 'margin-bottom: 12px; padding: 10px; background: white; border-radius: 5px;';
        
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
        pathText.style.cssText = 'flex: 1; color: #666; font-size: 13px;';
        
        const removeBtn = document.createElement('button');
        removeBtn.textContent = '✕ Remove';
        removeBtn.className = 'btn';
        removeBtn.style.cssText = 'background: #e74c3c; padding: 5px 10px; font-size: 12px;';
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
                        ${isMe ? '<span class="user-badge" style="background: #27ae60;">YOU</span>' : ''}
                        <div style="margin-top: 10px; display: flex; align-items: center; gap: 15px; flex-wrap: wrap;">
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: #666;">Group: </span>
                                <select onchange="updateUserGroup('${user.username}', this.value)" class="settings-input" style="width: auto; padding: 2px 5px; font-size: 12px;">
                                    <option value="">None</option>
                                    ${state.availableGroups.map(g => `<option value="${g}" ${user.group === g ? 'selected' : ''}>${g}</option>`).join('')}
                                </select>
                            </div>
                            <div style="display: flex; align-items: center; gap: 5px;">
                                <span style="font-size: 12px; color: #666;">Admin: </span>
                                <input type="checkbox" ${isAdminChecked} ${isMe ? 'disabled' : ''} onchange="updateUserAdmin('${user.username}', this.checked)">
                            </div>
                        </div>
                    </div>
                    ${!isMe ?
                        `<button class="btn" onclick="deleteUser('${user.username}')">🗑️ Delete</button>` :
                        '<span style="color: #999; font-style: italic; font-size: 12px;">Cannot delete yourself</span>'}
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
    const newPassword = document.getElementById('new-user-password').value;
    const confirmPassword = document.getElementById('confirm-user-password').value;
    
    if (!newPassword || !confirmPassword) {
        messageEl.textContent = 'Please fill in both password fields';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    if (newPassword !== confirmPassword) {
        messageEl.textContent = 'Passwords do not match';
        messageEl.style.color = '#e74c3c';
        return;
    }
    
    try {
        await api.changePassword('', newPassword);
        messageEl.textContent = 'Password changed successfully';
        messageEl.style.color = '#27ae60';
        
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

const matchState = { target: null, name: '', info: null, episodePick: null, seasonsFor: null };

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
            matchState.info = await api.resetMatch(matchState.target);
            setMatchMessage('Reset. Matching automatically…');
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
    document.getElementById('match-episode').innerHTML = '';
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
    renderMatchCurrent();
    runMatchSearch();
    document.getElementById('match-query').focus();
}

async function refreshMatchInfo() {
    try {
        matchState.info = await api.getMatch(matchState.target);
        renderMatchCurrent();
    } catch (error) {
        console.log('Could not refresh match:', error);
    }
}

function renderMatchCurrent() {
    const info = matchState.info;
    const box = document.getElementById('match-current');
    box.innerHTML = '';
    document.getElementById('match-reset').style.display = info && info.manual ? '' : 'none';
    if (!info) return;

    const status = document.createElement('div');
    status.className = 'match-status';
    const kindLabel = info.kind === 'tv' ? 'TV show' : 'Movie';
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
            pending: 'Not matched yet (lookup pending).',
            matched: 'Matched; fetching details…',
        }[info.status] || info.status;
    }
    box.appendChild(status);
    const hint = document.createElement('div');
    hint.className = 'match-hint';
    hint.textContent = 'Choosing a result applies to every file in this library with the same title.';
    box.appendChild(hint);
    markCurrentHit();
    renderEpisodeSection();
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
    const currentId = matchState.info && matchState.info.current && matchState.info.current.tmdb_id;
    document.querySelectorAll('.match-hit').forEach(row => {
        const isCurrent = Number(row.dataset.tmdbId) === currentId;
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
    results.innerHTML = '<p class="match-empty">Searching…</p>';
    let hits;
    try {
        hits = await api.searchMatch(matchState.info.kind, query, year);
    } catch (error) {
        results.innerHTML = '';
        setMatchMessage('Search failed: ' + error.message, true);
        return;
    }
    results.innerHTML = '';
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
    row.onclick = () => applyMatch(hit.tmdb_id, `Matched to ${hit.title}. Fetching details…`);
    return row;
}

async function applyMatch(tmdbId, message) {
    try {
        matchState.info = await api.setMatch(matchState.target, tmdbId);
        setMatchMessage(message);
        renderMatchCurrent();
    } catch (error) {
        setMatchMessage('Could not save: ' + error.message, true);
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

function pushLibraryHistory(params) {
    const url = new URL(window.location);
    url.search = '';
    Object.entries(params).forEach(([k, v]) => url.searchParams.set(k, v));
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

    container.innerHTML = '';
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
        poster.textContent = t.kind === 'show' ? '📺' : '🎬';
    }
    const title = document.createElement('div');
    title.className = 'poster-title';
    title.textContent = t.title;
    const year = document.createElement('div');
    year.className = 'poster-year';
    year.textContent = t.year || '';
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
    poster.textContent = '🎞️';
    if (canFixMatches()) poster.appendChild(createFixMatchButton({ path: f.path }, f.name));
    const title = document.createElement('div');
    title.className = 'poster-title';
    title.textContent = f.name;
    title.title = f.name;
    const meta = document.createElement('div');
    meta.className = 'poster-year';
    meta.textContent = f.count ? `${f.count} files` : formatSize(f.size || 0);
    card.appendChild(poster);
    card.appendChild(title);
    card.appendChild(meta);
    card.onclick = () => (f.count ? null : handleMediaClick(item));
    return card;
}

function libraryFileItem(f) {
    return { path: f.path, name: f.name, type: 'video', size: f.size || 0, duration: f.duration || 0,
             thumbnail_url: f.thumbnail_url || '' };
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
    container.appendChild(createTitleHero(t));

    if (kind === 'movie') {
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
    if (keepScroll) window.scrollTo(0, scroll);
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
        poster.textContent = t.kind === 'show' ? '📺' : '🎬';
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
        const play = document.createElement('button');
        play.type = 'button';
        play.className = 'btn play-btn';
        play.textContent = '▶ Play';
        play.onclick = () => playVideo(item);
        wrap.appendChild(play);
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

    const select = (index) => {
        picker.querySelectorAll('button').forEach((b, i) => {
            b.classList.toggle('active', i === index);
            b.setAttribute('aria-selected', String(i === index));
        });
        list.innerHTML = '';
        seasons[index].episodes.forEach(ep => list.appendChild(createEpisodeRow(ep)));
    };
    seasons.forEach((se, i) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.setAttribute('role', 'tab');
        btn.textContent = `${se.name} (${se.episodes.length})`;
        btn.onclick = () => select(i);
        picker.appendChild(btn);
    });
    section.append(picker, list);
    select(0);
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
    still.onclick = () => playVideo(first);

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
        const play = document.createElement('button');
        play.type = 'button';
        play.className = 'btn play-btn';
        const ext = f.name.split('.').pop().toUpperCase();
        play.textContent = ep.files.length > 1 ? `▶ ${ext}` : '▶ Play';
        play.title = f.name;
        play.onclick = () => playVideo(item);
        actions.appendChild(play);
        if (canFixMatches()) {
            const fix = createFixMatchButton({ path: f.path }, f.name);
            fix.classList.add('inline');
            actions.appendChild(fix);
        }
    });
    row.append(still, text, actions);
    return row;
}
