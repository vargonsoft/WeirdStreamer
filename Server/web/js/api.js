// API Configuration
const API_BASE = window.location.origin;

// The web UI never sees its sign-in token: it's only in the HttpOnly session cookie, which every
// request sends, so a script slipped into the page can't carry it off. Requests that change
// something also send WEB_CLIENT_HEADER, which pages of other sites can't add (the server allows
// no other origins), so they can't use the cookie. The signed-in user's details (no secret) are
// kept in localStorage for the page.
const WEB_CLIENT_HEADER = { 'X-WeirdStreamer-Web': '1' };
localStorage.removeItem('auth_token');
localStorage.removeItem('auth_token_v2');

// API Client
class MediaAPI {
    constructor() {
        this.eventSource = null;
        this.thumbnailCallbacks = [];
        this.metadataCallbacks = [];
    }

    // Headers for requests made with fetch() outside request()
    authHeaders() {
        return { ...WEB_CLIENT_HEADER };
    }

    async request(endpoint, options = {}) {
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
            ...WEB_CLIENT_HEADER,
        };

        const response = await fetch(`${API_BASE}${endpoint}`, {
            ...options,
            headers,
            credentials: 'same-origin',
        });

        if (response.status === 401) {
            this.logout();
            throw new Error('Unauthorized');
        }

        if (!response.ok) {
            const error = await response.json();
            const failure = new Error(error.message || 'Request failed');
            // Set on 429: seconds until the server accepts another try
            failure.retryAfter = error.retry_after || 0;
            // The server can't read the video at all: no other way of playing it will help.
            failure.unreadable = response.headers.get('X-Stream-Error') === 'unreadable';
            throw failure;
        }
        if (response.status === 204) return null;

        return response.json();
    }

    connectThumbnailUpdates() {
        // Close existing connection if any
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
        }

        if (!this.isAuthenticated()) return;

        // EventSource can't set headers; it authenticates with the session cookie.
        const url = `${API_BASE}/api/media/thumbnail-updates`;
        this.eventSource = new EventSource(url);

        this.eventSource.onmessage = (event) => {
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
        const data = await this.request('/api/auth/login', {
            method: 'POST',
            // web: the token goes only into the cookie, not the reply
            body: JSON.stringify({ username, password, web: true }),
        });

        localStorage.setItem('user', JSON.stringify(data.user));
        return data;
    }

    logout() {
        // Ends the session on the server (so the token stops working) and clears the HttpOnly
        // session cookie, which scripts can't remove themselves
        fetch(`${API_BASE}/api/auth/logout`, { method: 'POST', headers: WEB_CLIENT_HEADER, credentials: 'same-origin' }).catch(() => {});
        localStorage.removeItem('user');
    }

    isAuthenticated() {
        return !!localStorage.getItem('user');
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

    // Subtitles for a video on OpenSubtitles: { query, results }. languages: two-letter codes.
    async searchSubtitles(path, languages) {
        return this.request(`/api/media/subtitles/search?path=${encodeURIComponent(this.encodePath(path))}` +
            `&languages=${encodeURIComponent(languages.join(','))}`);
    }

    // Downloads a search result for a video (or reuses it): { subtitles, index, quota }.
    async downloadSubtitle(path, result) {
        return this.request('/api/media/subtitles/download', {
            method: 'POST',
            body: JSON.stringify({ path: this.encodePath(path), result }),
        });
    }

    // Where a stream that copies the video starts when asked to start at start (a string, exactly
    // as in the stream URL): { start: seconds }. next: the first keyframe at or after it instead,
    // and ask: the start to request for it.
    async getSeekPoint(path, start, next = false) {
        return this.request(`/api/media/seekpoint?path=${encodeURIComponent(this.encodePath(path))}&start=${start}${next ? '&next=1' : ''}`);
    }

    async getDisc(path) {
        return this.request(`/api/media/disc?path=${encodeURIComponent(this.encodePath(path))}`);
    }

    // Active streams (admin), and the player's position for them
    async getStreams() {
        return this.request('/api/admin/streams');
    }

    // Settings → Jobs (admins): { jobs, load }; action is run, pause or resume.
    async getJobs() {
        return this.request('/api/admin/jobs');
    }

    async controlJob(id, action) {
        return this.request(`/api/admin/jobs/${encodeURIComponent(id)}/${action}`, { method: 'POST' });
    }

    // Trickplay's settings (saved to config.yml): any of { runners, frames_per_runner } (0 is automatic)
    // and { use_gpu }; returns the jobs status.
    async setTrickplaySettings(settings) {
        return this.request('/api/admin/jobs/trickplay/settings', {
            method: 'POST',
            body: JSON.stringify(settings),
        });
    }

    // extra: { ended, title } — the player reached the end; a disc's title (0: the main title)
    async reportPlayback(path, position, paused, extra = {}) {
        return this.request('/api/streams/progress', {
            method: 'POST',
            body: JSON.stringify({ path: this.encodePath(path), position, paused, ...extra }),
        });
    }

    // Marks files watched or unwatched for the signed-in user.
    // Saves a subtitle track's timing correction (ms; positive shows its lines later), for everyone.
    async setSubtitleOffset(path, key, offsetMs) {
        return this.request('/api/media/subtitle-offset', {
            method: 'PUT',
            body: JSON.stringify({ path: this.encodePath(path), key, offset_ms: offsetMs }),
        });
    }

    // A video's intro and credits ({ intro, credits, auto_skip, show_id, next }).
    async getSegments(path) {
        return this.request(`/api/media/segments?path=${encodeURIComponent(this.encodePath(path))}`);
    }

    // Admins: sets (or clears) a video's intro or credits by hand; returns its segments.
    async setSegment(path, kind, start, end, clear = false) {
        return this.request('/api/media/segments', {
            method: 'PUT',
            body: JSON.stringify({ path: this.encodePath(path), kind, start, end, clear }),
        });
    }

    // Skips a show's intros and credits by themselves (or not), for this user.
    async setAutoSkip(tmdbId, on) {
        return this.request('/api/library/autoskip', { method: 'PUT', body: JSON.stringify({ tmdb_id: tmdbId, on }) });
    }

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

    // The Guide (live channels): whether there is one, the grid, what a channel airs now, and
    // marking the program on now watched. lib: a library's own Guide.
    guideQuery(lib, extra = {}) {
        const params = new URLSearchParams(extra);
        if (lib) params.set('lib', lib);
        const q = params.toString();
        return q ? `?${q}` : '';
    }

    async guideAvailable(lib = '') {
        return (await this.request('/api/guide/available' + this.guideQuery(lib))).available;
    }

    async guideListings(lib = '', hours = 3.5) {
        return this.request('/api/guide' + this.guideQuery(lib, { hours }));
    }

    async guideNow(channel, lib = '') {
        return this.request('/api/guide/now' + this.guideQuery(lib, { channel }));
    }

    async guideWatched(channel, lib = '') {
        return this.request('/api/guide/watched' + this.guideQuery(lib, { channel }), { method: 'POST' });
    }

    async newAdditions(lib = '') {
        return this.request('/api/library/new' + (lib ? `?lib=${encodeURIComponent(lib)}` : ''));
    }

    // The home screen's: [{library_id, library_name, items}], a list per library
    async newAdditionsByLibrary() {
        return this.request('/api/library/new?by=library');
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

    // Collections admins make in a movie library (titles are { kind, tmdb_id })
    async getUserCollection(libId, id) {
        return this.request(`/api/library/usercollection?lib=${encodeURIComponent(libId)}&id=${id}`);
    }

    async createUserCollection(libId, name, description, titles) {
        return this.request('/api/library/usercollections', {
            method: 'POST', body: JSON.stringify({ lib: libId, name, description, titles }),
        });
    }

    // changes: { name?, description?, add?: [titles], remove?: [titles] }
    async updateUserCollection(libId, id, changes) {
        return this.request('/api/library/usercollections', {
            method: 'PUT', body: JSON.stringify({ lib: libId, id, ...changes }),
        });
    }

    async deleteUserCollection(libId, id) {
        return this.request(`/api/library/usercollections?lib=${encodeURIComponent(libId)}&id=${id}`, { method: 'DELETE' });
    }

    // A movie collection's page: every film TMDB lists in it, the library's and the missing ones
    async getLibraryCollection(libId, id) {
        return this.request(`/api/library/collection?lib=${encodeURIComponent(libId)}&id=${id}`);
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
        return this.request(`/api/media/regenerate-thumbnails?path=${encodeURIComponent(this.encodePath(path))}`, {
            method: 'POST',
        });
    }

    async getConfig() {
        return this.request('/api/config');
    }

    // A folder of the server's disk, for choosing a library's (admins): {path, parent, top, folders: [{name, path}], videos, library}; parent=true lists the one above path
    async listFolders(path, parent = false) {
        return this.request(`/api/config/folders?path=${encodeURIComponent(path || '')}${parent ? '&parent=1' : ''}`);
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

    // The defined groups (each returns the new list)
    async createGroup(name) {
        return this.request('/api/groups', { method: 'POST', body: JSON.stringify({ name }) });
    }

    async renameGroup(name, newName) {
        return this.request(`/api/groups/${encodeURIComponent(name)}`, { method: 'PUT', body: JSON.stringify({ name: newName }) });
    }

    // Makes a group the default for new users (the first); returns the groups in their new order.
    async makeDefaultGroup(name) {
        return this.request(`/api/groups/${encodeURIComponent(name)}/default`, { method: 'PUT' });
    }

    async deleteGroup(name) {
        return this.request(`/api/groups/${encodeURIComponent(name)}`, { method: 'DELETE' });
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

    // The account's own settings: {language, languages}.
    async getAccount() {
        return this.request('/api/account');
    }

    // {audio_language, subtitle_language, subtitles_on}
    async setLanguages(languages) {
        return this.request('/api/account', { method: 'PUT', body: JSON.stringify(languages) });
    }

    async changePassword(oldPassword, newPassword) {
        return this.request('/api/users/change-password', {
            method: 'POST',
            body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
        });
    }
}

const api = new MediaAPI();
