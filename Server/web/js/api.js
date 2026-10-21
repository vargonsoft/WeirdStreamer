// API Configuration
console.log('api.js loaded');
const API_BASE = window.location.origin;
console.log('API_BASE:', API_BASE);

// Bumped when the session format changed: logins now also set an HttpOnly session cookie, which
// <img>, <video> and EventSource requests rely on, so older stored sessions must log in again.
const TOKEN_KEY = 'auth_token_v2';
localStorage.removeItem('auth_token');

// API Client
class MediaAPI {
    constructor() {
        this.token = localStorage.getItem(TOKEN_KEY);
        this.eventSource = null;
        this.thumbnailCallbacks = [];
        this.metadataCallbacks = [];
        console.log('MediaAPI initialized, token:', this.token ? 'exists' : 'null');
    }

    async request(endpoint, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };

        if (this.token) {
            headers['Authorization'] = `Bearer ${this.token}`;
        }

        const response = await fetch(`${API_BASE}${endpoint}`, {
            ...options,
            headers,
        });

        if (response.status === 401) {
            this.logout();
            throw new Error('Unauthorized');
        }

        if (!response.ok) {
            const error = await response.json();
            throw new Error(error.message || 'Request failed');
        }
        if (response.status === 204) return null;

        return response.json();
    }

    connectThumbnailUpdates() {
        // Close existing connection if any
        if (this.eventSource) {
            console.log('[SSE] Closing existing connection before reconnecting');
            this.eventSource.close();
            this.eventSource = null;
        }

        if (!this.token) {
            console.error('[SSE] No auth token available');
            return;
        }

        // EventSource can't set headers; it authenticates with the session cookie.
        const url = `${API_BASE}/api/media/thumbnail-updates`;
        console.log('[SSE] Connecting to thumbnail updates...', url);
        this.eventSource = new EventSource(url);

        this.eventSource.onopen = (event) => {
            console.log('[SSE] Connection established', event);
        };

        this.eventSource.onmessage = (event) => {
            console.log('[SSE] Message received:', event.data);
            try {
                const data = JSON.parse(event.data);
                // Notify all registered callbacks
                this.thumbnailCallbacks.forEach(callback => callback(data));
            } catch (error) {
                console.error('[SSE] Error parsing data:', error);
            }
        };

        // Named "metadata" events: TMDB info for something on screen changed.
        this.eventSource.addEventListener('metadata', (event) => {
            try {
                const data = JSON.parse(event.data);
                this.metadataCallbacks.forEach(callback => callback(data));
            } catch (error) {
                console.error('[SSE] Error parsing metadata event:', error);
            }
        });

        this.eventSource.onerror = (error) => {
            console.error('[SSE] Connection error', error);
            console.log('[SSE] ReadyState:', this.eventSource?.readyState);
            if (this.eventSource) {
                this.eventSource.close();
                this.eventSource = null;
            }
        };
    }

    disconnectThumbnailUpdates() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
        }
    }

    onThumbnailUpdate(callback) {
        this.thumbnailCallbacks.push(callback);
    }

    clearThumbnailCallbacks() {
        this.thumbnailCallbacks = [];
    }

    onMetadataUpdate(callback) {
        this.metadataCallbacks.push(callback);
    }

    // Base64 of a path's UTF-8 bytes, as the server expects (btoa alone fails on non-Latin-1
    // names like "Shōgun").
    encodePath(path) {
        const bytes = new TextEncoder().encode(path);
        let binary = '';
        bytes.forEach(b => { binary += String.fromCharCode(b); });
        return btoa(binary);
    }

    async login(username, password) {
        console.log('login() called with username:', username);
        const data = await this.request('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password }),
        });

        console.log('Login response:', data);
        this.token = data.token;
        localStorage.setItem(TOKEN_KEY, data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        return data;
    }

    logout() {
        // Clears the HttpOnly session cookie, which scripts can't remove themselves
        fetch(`${API_BASE}/api/auth/logout`, { method: 'POST' }).catch(() => {});
        this.token = null;
        localStorage.removeItem(TOKEN_KEY);
        localStorage.removeItem('user');
    }

    isAuthenticated() {
        return !!this.token;
    }

    async listMedia(type = '') {
        const query = type ? `?type=${type}` : '';
        return this.request(`/api/media${query}`);
    }

    async browseDirectory(path = '') {
        const query = path ? `?path=${encodeURIComponent(path)}` : '';
        return this.request(`/api/media/browse${query}`);
    }

    async getMediaInfo(path) {
        return this.request(`/api/media/${encodeURIComponent(path)}`);
    }

    // forceTranscode: true for a full transcode, 'audio' to copy the video and transcode only
    // the audio (for audio the browser can't decode).
    getStreamUrl(path, forceTranscode = false) {
        // Base64 encode the path to avoid special character issues in URLs
        const encodedPath = encodeURIComponent(this.encodePath(path));
        let url = `${API_BASE}/api/media/stream?path=${encodedPath}`;

        // Disable auto-transcoding for WebUI - let browser attempt native playback first
        // User can explicitly transcode via the button if needed
        url += '&auto-transcode=0';

        if (forceTranscode) {
            url += forceTranscode === 'audio' ? '&transcode=audio' : '&transcode=1';
        }
        // Authenticated by the session cookie
        return url;
    }

    getThumbnailUrl(thumbnailUrl) {
        if (!thumbnailUrl) return '';
        // Authenticated by the session cookie
        return `${API_BASE}${thumbnailUrl}`;
    }

    // Manual metadata matching. target is { path } for a file or { dir } for a folder.
    matchTargetQuery(target) {
        return target.dir !== undefined
            ? `dir=${encodeURIComponent(target.dir)}`
            : `path=${encodeURIComponent(this.encodePath(target.path))}`;
    }

    async getMatch(target) {
        return this.request(`/api/media/match?${this.matchTargetQuery(target)}`);
    }

    async searchMatch(kind, query, year) {
        const params = new URLSearchParams({ kind, query });
        if (year) params.set('year', year);
        return this.request(`/api/media/match/search?${params}`);
    }

    // kind is what tmdbId is ('movie' or 'tv'); a movie library can match a TV show.
    async setMatch(target, tmdbId, kind) {
        const body = target.dir !== undefined
            ? { dir: target.dir, tmdb_id: tmdbId, kind: kind || '' }
            : { path: this.encodePath(target.path), tmdb_id: tmdbId, kind: kind || '' };
        return this.request('/api/media/match', { method: 'PUT', body: JSON.stringify(body) });
    }

    // A DVD disc image's titles, chapters, angles and tracks
    // A file's container and its video and audio codecs: { container, video, audio: [...] }
    async getCodecs(path) {
        return this.request(`/api/media/codecs?path=${encodeURIComponent(this.encodePath(path))}`);
    }

    async getDisc(path) {
        return this.request(`/api/media/disc?path=${encodeURIComponent(this.encodePath(path))}`);
    }

    // Active streams (admin), and the player's position for them
    async getStreams() {
        return this.request('/api/admin/streams');
    }

    // extra: { ended, title } — the player reached the end; a disc's title (0: the main title)
    async reportPlayback(path, position, paused, extra = {}) {
        return this.request('/api/streams/progress', {
            method: 'POST',
            body: JSON.stringify({ path: this.encodePath(path), position, paused, ...extra }),
        });
    }

    // Marks files watched or unwatched for the signed-in user.
    async setWatched(paths, watched) {
        return this.request('/api/playback/watched', {
            method: 'POST',
            body: JSON.stringify({ paths: paths.map(p => this.encodePath(p)), watched }),
        });
    }

    // Marks every file of a movie or show watched or unwatched.
    async setTitleWatched(library, kind, tmdbId, watched) {
        return this.request('/api/playback/watched', {
            method: 'POST',
            body: JSON.stringify({ paths: [], library, kind, tmdb_id: tmdbId, watched }),
        });
    }

    // Videos in progress, most recent first (optionally in one library).
    async continueWatching(lib = '') {
        return this.request('/api/playback/continue' + (lib ? `?lib=${encodeURIComponent(lib)}` : ''));
    }

    async removeFromContinue(path) {
        return this.request(`/api/playback/continue?path=${encodeURIComponent(this.encodePath(path))}`, { method: 'DELETE' });
    }

    // Every TMDB poster for the title a target is matched to: { current, manual, posters: [...] }
    async getMatchPosters(target) {
        return this.request(`/api/media/match/posters?${this.matchTargetQuery(target)}`);
    }

    // Use a TMDB image path (any title's) as the matched title's poster; '' for its default.
    async setMatchPoster(target, poster) {
        const body = target.dir !== undefined
            ? { dir: target.dir, poster }
            : { path: this.encodePath(target.path), poster };
        return this.request('/api/media/match/poster', { method: 'PUT', body: JSON.stringify(body) });
    }

    async resetMatch(target) {
        return this.request(`/api/media/match?${this.matchTargetQuery(target)}`, { method: 'DELETE' });
    }

    // Library view (movie and tv libraries as lists of titles)
    async getLibrary(libId) {
        return this.request(`/api/library?lib=${encodeURIComponent(libId)}`);
    }

    async getLibraryTitle(libId, kind, id) {
        return this.request(`/api/library/title?lib=${encodeURIComponent(libId)}&kind=${kind}&id=${id}`);
    }

    async matchSeasons(showId) {
        return this.request(`/api/media/match/seasons?show=${showId}`);
    }

    async matchEpisodes(showId, season) {
        return this.request(`/api/media/match/episodes?show=${showId}&season=${season}`);
    }

    async setEpisodeMapping(path, season, episode, wholeSeason) {
        return this.request('/api/media/match/episode', {
            method: 'PUT',
            body: JSON.stringify({ path: this.encodePath(path), season, episode, whole_season: wholeSeason }),
        });
    }

    async clearEpisodeMapping(path, wholeSeason) {
        const whole = wholeSeason ? '&whole_season=1' : '';
        return this.request(`/api/media/match/episode?path=${encodeURIComponent(this.encodePath(path))}${whole}`, { method: 'DELETE' });
    }

    async scanMedia() {
        return this.request('/api/media/scan', {
            method: 'POST',
        });
    }

    async regenerateThumbnails(path) {
        const query = path ? `?path=${encodeURIComponent(path)}` : '';
        return this.request(`/api/media/regenerate-thumbnails${query}`, {
            method: 'POST',
        });
    }

    async getConfig() {
        return this.request('/api/config');
    }

    async updateConfig(config) {
        return this.request('/api/config', {
            method: 'PUT',
            body: JSON.stringify(config),
        });
    }

    async getAvailableGroups() {
        return this.request('/api/groups');
    }

    async getUsers() {
        return this.request('/api/users');
    }

    async createUser(username, password, isAdmin, group = '') {
        return this.request('/api/users', {
            method: 'POST',
            body: JSON.stringify({
                username,
                password,
                is_admin: isAdmin,
                group: group
            }),
        });
    }

    async updateUser(username, data) {
        return this.request(`/api/users?username=${encodeURIComponent(username)}`, {
            method: 'PUT',
            body: JSON.stringify(data),
        });
    }

    async deleteUser(username) {
        return this.request(`/api/users?username=${encodeURIComponent(username)}`, {
            method: 'DELETE',
        });
    }

    async changePassword(oldPassword, newPassword) {
        return this.request('/api/users/change-password', {
            method: 'POST',
            body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
        });
    }
}

const api = new MediaAPI();
