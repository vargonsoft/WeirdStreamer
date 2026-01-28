// API Configuration
console.log('api.js loaded');
const API_BASE = window.location.origin;
console.log('API_BASE:', API_BASE);

// API Client
class MediaAPI {
    constructor() {
        this.token = localStorage.getItem('auth_token');
        this.eventSource = null;
        this.thumbnailCallbacks = [];
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

        return response.json();
    }

    connectThumbnailUpdates() {
        if (this.eventSource) {
            return; // Already connected
        }

        const url = `${API_BASE}/api/media/thumbnail-updates`;
        this.eventSource = new EventSource(url);

        this.eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                // Notify all registered callbacks
                this.thumbnailCallbacks.forEach(callback => callback(data));
            } catch (error) {
                console.error('Error parsing SSE data:', error);
            }
        };

        this.eventSource.onerror = (error) => {
            console.error('SSE connection error:', error);
            this.eventSource.close();
            this.eventSource = null;
            // Reconnect after 5 seconds
            setTimeout(() => this.connectThumbnailUpdates(), 5000);
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

    async login(username, password) {
        console.log('login() called with username:', username);
        const data = await this.request('/api/auth/login', {
            method: 'POST',
            body: JSON.stringify({ username, password }),
        });

        console.log('Login response:', data);
        this.token = data.token;
        localStorage.setItem('auth_token', data.token);
        localStorage.setItem('user', JSON.stringify(data.user));
        return data;
    }

    logout() {
        this.token = null;
        localStorage.removeItem('auth_token');
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

    getStreamUrl(path, forceTranscode = false) {
        // Base64 encode the path to avoid special character issues in URLs
        const encodedPath = btoa(path);
        let url = `${API_BASE}/api/media/stream?path=${encodedPath}`;
        if (forceTranscode) {
            url += '&transcode=1';
        }
        console.log('Token available:', !!this.token);
        if (this.token) {
            const urlWithToken = `${url}&token=${encodeURIComponent(this.token)}`;
            console.log('Stream URL with token:', urlWithToken);
            return urlWithToken;
        }
        console.log('Stream URL without token:', url);
        return url;
    }

    getThumbnailUrl(thumbnailUrl) {
        if (!thumbnailUrl) return '';
        const url = `${API_BASE}${thumbnailUrl}`;
        if (this.token) {
            return `${url}${thumbnailUrl.includes('?') ? '&' : '?'}token=${encodeURIComponent(this.token)}`;
        }
        return url;
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

    async getUsers() {
        return this.request('/api/users');
    }

    async createUser(username, password, isAdmin) {
        return this.request('/api/users', {
            method: 'POST',
            body: JSON.stringify({ username, password, is_admin: isAdmin }),
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
