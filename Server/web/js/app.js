// Application State
console.log('app.js loaded');
const state = {
    currentType: 'all',
    currentPath: '',
    mediaItems: [],
    directories: [],
    currentConfig: null,
    availableGroups: []
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
    
    // Check if user is admin and show settings button
    const user = JSON.parse(localStorage.getItem('user') || '{}');
    if (user.is_admin && settingsBtn) {
        settingsBtn.style.display = 'inline-block';
    }
    
    // Update regenerate thumbnails button visibility
    updateRegenerateThumbnailsButton();

    // Load config for friendly path names
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
    
    // Initialize from URL parameters if present
    const url = new URL(window.location);
    const urlPath = url.searchParams.get('path');
    const urlType = url.searchParams.get('type');
    
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
    try {
        const data = await api.browseDirectory(state.currentPath);
        state.directories = data.dirs || [];
        state.mediaItems = data.items || [];
        
        // Filter by type if needed
        if (state.currentType !== 'all') {
            state.mediaItems = state.mediaItems.filter(item => item.type === state.currentType);
        }
        
        // Update browser history
        if (pushHistory) {
            const url = new URL(window.location);
            url.searchParams.set('path', state.currentPath);
            url.searchParams.set('type', state.currentType);
            window.history.pushState(
                { path: state.currentPath, type: state.currentType },
                '',
                url
            );
        }
        
        updateBreadcrumb();
        updateRegenerateThumbnailsButton();
        renderMedia();
    } catch (error) {
        console.error('Failed to load media:', error);
    }
}

function updateBreadcrumb() {
    breadcrumb.innerHTML = '';
    
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
            if (index === 0 && state.currentConfig && state.currentConfig.media.paths) {
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
        state.currentPath = dir.path;
        loadMedia();
    };
    
    const thumbnail = document.createElement('div');
    thumbnail.className = 'media-thumbnail folder-thumbnail';
    thumbnail.textContent = '📁';
    
    const name = document.createElement('div');
    name.className = 'media-name';
    
    // Use friendly name for root paths if configured
    let displayName = dir.name;
    if (!state.currentPath && state.currentConfig && state.currentConfig.media.paths) {
        // Match only by configured ID (server enforces ID-only API)
        const mediaPath = state.currentConfig.media.paths.find(mp => mp.id === dir.path);
        if (mediaPath) {
            displayName = mediaPath.name;
        }
    }
    
    name.textContent = displayName;
    
    div.appendChild(thumbnail);
    div.appendChild(name);
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
    const incompatibleFormats = ['3gp', 'avi', 'wmv', 'asf', 'flv', 'mkv'];
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
    try {
        const config = await api.getConfig();
        state.currentConfig = config;
        updateAvailableGroups();

        // Populate form fields
        document.getElementById('server-port').value = config.server.port;
        document.getElementById('server-host').value = config.server.host;
        document.getElementById('enable-http2').checked = config.streaming.enableHTTP2;
        document.getElementById('chunk-size').value = config.streaming.chunkSize;
        
        // Apply theme selection
        applyTheme();
        
        // Setup theme radio button listeners for immediate preview
        document.querySelectorAll('input[name="theme"]').forEach(radio => {
            radio.addEventListener('change', (e) => {
                document.documentElement.setAttribute('data-theme', e.target.value);
            });
        });
        
        // Render media paths
        renderMediaPaths(config.media.paths);
        
        // Setup event listeners for settings
        document.getElementById('add-path-btn').onclick = handleAddPath;
        document.getElementById('save-settings-btn').onclick = handleSaveSettings;
        
        // Show settings screen
        mainScreen.classList.remove('active');
        settingsScreen.classList.add('active');
    } catch (error) {
        alert('Failed to load settings: ' + error.message);
    }
}

function updateAvailableGroups() {
    if (!state.currentConfig || !state.currentConfig.media.paths) return;

    const groupsSet = new Set();
    state.currentConfig.media.paths.forEach(path => {
        if (path.groups) {
            path.groups.forEach(group => groupsSet.add(group));
        }
    });
    state.availableGroups = Array.from(groupsSet).sort();
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
        state.currentConfig.media.paths.push({ name: name, path: newPath, groups: [] });
        renderMediaPaths(state.currentConfig.media.paths);
        input.value = '';
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
            console.log('Rendering user:', user.username, 'isAdmin:', user.is_admin, 'isMe:', isMe);
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
                                <input type="checkbox" ${user.is_admin ? 'checked' : ''} ${isMe ? 'disabled' : ''} onchange="updateUserAdmin('${user.username}', this.checked)">
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
