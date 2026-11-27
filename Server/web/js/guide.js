// The Guide (docs/GUIDE.md): live channels the server plans. A screen like Settings, opened from
// the Guide row above the folders: the channel on top left with its program's details beside it,
// and the grid below, each channel's programs from the current half hour on. A channel plays only
// what's on now: there's no seeking and no pause (pausing mutes it). Uses app.js's helpers.

const guideScreen = document.getElementById('guide-screen');
const guideVideo = document.getElementById('guide-video');

const guide = {
    lib: '',          // a library's own Guide, or '' for the home screen's
    data: null,       // the grid's reply
    channel: null,    // the number tuned to
    now: null,        // what it airs (the server's /now reply)
    selected: null,   // { channel, listing } shown in the details
    skew: 0,          // the server's clock less this one's, ms
    timer: null,      // asks what's next when the item ends
    gridTimer: null,  // moves the time line, reloads the grid at each half hour
    request: 0,       // the latest tune (older replies are dropped)
    muted: false,
    codecs: new Map(), // path → codecs report
    digits: '',       // channel number being typed
    digitTimer: null,
    voiceTimers: [],  // the voice-over's and squeeze's times
    voiceOver: null,  // the voice-over playing: { channel, line, announcer }, for a report
    weather: null,    // the Weather channel's data, and when it was read
    weatherAt: 0,
    screenTimer: null, // turns the Weather channel's pages
};

const GUIDE_HOURS = 3.5;
const GUIDE_SLOT_MS = 30 * 60 * 1000;
const GUIDE_SLOT_PX = 200; // a half hour's width in the grid, which scrolls on to the end of the plan

// A channel's logo (one color, transparent around it) w pixels wide, in color (CSS hex; the
// theme's text color by default).
function guideLogoUrl(c, w, color = '') {
    const hex = (color || getComputedStyle(document.documentElement).getPropertyValue('--text-color') || '#ffffff')
        .trim().replace('#', '');
    const rrggbb = hex.length === 3 ? hex.split('').map(x => x + x).join('') : hex.slice(0, 6);
    return api.getThumbnailUrl(`${c.logo}&w=${w}&color=${rrggbb}`);
}

function serverNow() {
    return Date.now() + guide.skew;
}

function guideTime(ms) {
    return new Date(ms).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
}

// The Guide row above the folders: a Guide button when there's a Guide to show here (the home
// screen's, or a library's own).
async function createGuideRow(lib = '') {
    let available = false;
    try {
        available = await api.guideAvailable(lib);
    } catch (error) {
        return null;
    }
    if (!available) return null;
    const section = el('section', 'guide-row');
    const button = el('button', 'btn guide-button');
    button.type = 'button';
    button.innerHTML = iconSvg('liveTv') + '<span class="btn-label">Guide</span>';
    button.onclick = () => showGuide(lib);
    section.appendChild(button);
    return section;
}

// The home screen's Guide: a folder-sized tile after the TV Shows and Movies libraries, when there's a Guide.
async function createGuideTile() {
    let available = false;
    try {
        available = await api.guideAvailable('');
    } catch (error) {
        return null;
    }
    if (!available) return null;
    const div = el('div', 'media-item directory-item guide-tile');
    div.onclick = () => showGuide('');
    const thumbnail = el('div', 'media-thumbnail folder-thumbnail');
    thumbnail.innerHTML = placeholderIcon('liveTv');
    div.append(thumbnail, el('div', 'media-name', 'Guide'));
    return div;
}

async function showGuide(lib = '', pushHistory = true) {
    guide.lib = lib;
    if (pushHistory) pushScreenHistory('guide', { guideLib: lib }); // a reload opens it again on this library's
    mainScreen.classList.remove('active');
    guideScreen.classList.add('active');
    window.scrollTo(0, 0);
    await loadGuideGrid();
    if (!guideScreen.classList.contains('active')) return;
    const channels = (guide.data && guide.data.channels) || [];
    if (!channels.length) return;
    // The last channel watched here, else the first.
    const last = Number(localStorage.getItem(guideChannelKey()));
    const start = channels.find(c => c.number === last) || channels[0];
    if (guide.channel === null || !channels.some(c => c.number === guide.channel)) tune(start.number);
    else if (!guide.now) tune(guide.channel);
}

function hideGuide() {
    stopGuidePlayback();
    clearInterval(guide.gridTimer);
    guide.gridTimer = null;
    guideScreen.classList.remove('active');
    mainScreen.classList.add('active');
}

function guideChannelKey() {
    return 'guideChannel' + (guide.lib ? ':' + guide.lib : '');
}

function stopGuidePlayback() {
    endVoiceOver();
    guide.request++;
    clearTimeout(guide.timer);
    guide.timer = null;
    guide.now = null;
    guide.channel = null;
    guideVideo.pause();
    guideVideo.removeAttribute('src');
    guideVideo.load();
    setChannelScreen('');
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
}

// Loads the grid from the current half hour, keeping as many hours as were loaded.
async function loadGuideGrid() {
    try {
        const first = Math.floor(serverNow() / GUIDE_SLOT_MS) * GUIDE_SLOT_MS;
        const hours = Math.max(GUIDE_HOURS, guide.data ? (guide.data.to - first) / 3600000 : 0);
        const data = await api.guideListings(guide.lib, hours);
        guide.skew = data.now - Date.now();
        guide.data = data;
    } catch (error) {
        document.getElementById('guide-grid').textContent = 'The Guide couldn’t be loaded: ' + error.message;
        return;
    }
    renderGuideGrid();
    if (document.getElementById('guide-player').dataset.channelKind === 'guide') renderGuideListings();
    clearInterval(guide.gridTimer);
    guide.gridTimer = setInterval(() => {
        if (serverNow() >= guide.data.from + GUIDE_SLOT_MS) loadGuideGrid();
        else placeTimeLine();
    }, 15000);
}

// Loads the next 3 hours onto the grid (scrolled near its end), up to as far as the plan reaches.
async function extendGuideGrid() {
    const d = guide.data;
    if (!d || guide.extending || (d.until && d.to >= d.until)) return;
    guide.extending = true;
    try {
        const more = await api.guideListings(guide.lib, 3, d.to);
        if (guide.data !== d) return; // reloaded meanwhile
        const byNumber = new Map((more.channels || []).map(c => [c.number, c]));
        guide.data = { ...d, to: more.to, until: more.until, channels: d.channels.map(c => {
            const have = c.listings || [];
            const add = ((byNumber.get(c.number) || {}).listings || []).filter(a => !have.some(h => h.start === a.start));
            return { ...c, listings: have.concat(add) }; // a program across the boundary comes in both: kept once
        }) };
        renderGuideGrid();
    } catch (error) {
        // tried again on the next scroll
    } finally {
        guide.extending = false;
    }
}

// The day at the grid's left edge, as it scrolls.
function guideGridDay(grid) {
    const corner = grid.querySelector('.guide-corner');
    if (!corner || !guide.data) return;
    const t = guide.data.from + Math.floor(grid.scrollLeft / GUIDE_SLOT_PX) * GUIDE_SLOT_MS;
    corner.textContent = new Date(t).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' });
}

function renderGuideGrid() {
    const grid = document.getElementById('guide-grid');
    // drawn anew keeping where it was scrolled to (a half hour less to the right when the grid moved on)
    const shift = grid.dataset.from ? (guide.data.from - Number(grid.dataset.from)) / GUIDE_SLOT_MS * GUIDE_SLOT_PX : 0;
    const scrollLeft = Math.max(grid.scrollLeft - shift, 0), scrollTop = grid.scrollTop;
    grid.dataset.from = guide.data.from;
    grid.innerHTML = '';
    const { from, to, channels } = guide.data;
    grid.style.setProperty('--programs-width', `${(to - from) / GUIDE_SLOT_MS * GUIDE_SLOT_PX}px`);
    grid.onscroll = () => {
        guideGridDay(grid);
        if (grid.scrollLeft + grid.clientWidth > grid.scrollWidth - 3 * GUIDE_SLOT_PX) extendGuideGrid();
    };
    if (!channels.length) {
        grid.textContent = 'There are no channels yet: the Guide is planned a minute after the server starts.';
        return;
    }
    const span = to - from;
    const pct = ms => `${(100 * ms / span).toFixed(4)}%`;

    const head = el('div', 'guide-grid-row guide-grid-head');
    head.appendChild(el('div', 'guide-channel guide-corner', new Date(from).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })));
    const times = el('div', 'guide-programs');
    for (let t = from; t < to; t += GUIDE_SLOT_MS) {
        const midnight = new Date(t).getHours() === 0 && new Date(t).getMinutes() === 0;
        const label = el('div', 'guide-time', midnight
            ? new Date(t).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' }) : guideTime(t));
        label.style.left = pct(t - from);
        label.style.width = pct(Math.min(GUIDE_SLOT_MS, to - t));
        times.appendChild(label);
    }
    head.appendChild(times);
    grid.appendChild(head);

    channels.forEach(c => {
        const row = el('div', 'guide-grid-row');
        row.dataset.channel = c.number;
        const name = el('button', 'guide-channel');
        name.type = 'button';
        name.title = `Watch channel ${c.number}`;
        // its logo (with its number in it), and the number and name beside it
        const logo = el('img', 'guide-channel-logo');
        logo.src = guideLogoUrl(c, 256);
        logo.alt = '';
        const words = el('span', 'guide-channel-words');
        words.append(el('span', 'guide-channel-number', String(c.number)), el('span', 'guide-channel-name', c.name));
        name.append(logo, words);
        name.onclick = () => tune(c.number);
        row.appendChild(name);
        const programs = el('div', 'guide-programs');
        (c.listings || []).forEach(l => {
            const start = Math.max(l.start, from), end = Math.min(l.end, to);
            if (end <= start) return;
            const cell = el('button', 'guide-program');
            cell.type = 'button';
            cell.style.left = pct(start - from);
            cell.style.width = pct(end - start);
            if (l.start < from) cell.classList.add('continues');
            const info = l.info;
            cell.appendChild(el('span', 'guide-program-title', info ? info.title : 'Off the air'));
            // A second line: the episode, or a movie's year
            if (info && info.season) cell.appendChild(el('span', 'guide-program-episode', episodeLabel(info)));
            else if (info && info.kind === 'movie' && info.year) cell.appendChild(el('span', 'guide-program-episode', String(info.year)));
            cell.onclick = () => selectListing(c, l, true);
            cell.onfocus = () => selectListing(c, l, false);
            cell.dataset.start = l.start;
            cell.dataset.end = l.end;
            programs.appendChild(cell);
        });
        row.appendChild(programs);
        grid.appendChild(row);
    });
    const line = el('div', 'guide-now-line');
    line.id = 'guide-now-line';
    grid.appendChild(line);
    placeTimeLine();
    markOnNow();
    grid.scrollLeft = scrollLeft;
    grid.scrollTop = scrollTop;
    guideGridDay(grid);
    if (guide.selected) {
        // The same program selected again, now that the grid was drawn anew
        const c = channels.find(ch => ch.number === guide.selected.channel.number);
        const l = c && (c.listings || []).find(x => x.start === guide.selected.listing.start);
        if (c && l) showGuideDetails(c, l);
    }
}

function episodeLabel(info) {
    const code = `S${info.season}E${info.episode}`;
    return info.episode_title ? `${code} · ${info.episode_title}` : code;
}

// The time line over the grid's programs, at the server's now.
function placeTimeLine() {
    const line = document.getElementById('guide-now-line');
    const head = document.querySelector('#guide-grid .guide-grid-head .guide-programs');
    if (!line || !head || !guide.data) return;
    const { from, to } = guide.data;
    const f = Math.min(Math.max((serverNow() - from) / (to - from), 0), 1);
    line.style.left = `${head.offsetLeft + f * head.offsetWidth}px`;
    line.style.height = `${line.parentElement.scrollHeight}px`;
    markOnNow();
}

// Programs on now are the ones that can be watched.
function markOnNow() {
    const now = serverNow();
    document.querySelectorAll('#guide-grid .guide-program').forEach(cell => {
        cell.classList.toggle('on-now', Number(cell.dataset.start) <= now && now < Number(cell.dataset.end));
    });
    document.querySelectorAll('#guide-grid .guide-grid-row[data-channel]').forEach(row => {
        row.classList.toggle('tuned', Number(row.dataset.channel) === guide.channel);
    });
}

// A program chosen in the grid: its details show, and one on now (clicked) on another channel
// tunes to it. Full screen is a click on the picture.
function selectListing(c, l, clicked) {
    guide.selected = { channel: c, listing: l };
    showGuideDetails(c, l);
    const now = serverNow();
    if (clicked && l.start <= now && now < l.end && guide.channel !== c.number) tune(c.number);
}

function showGuideDetails(c, l) {
    const box = document.getElementById('guide-info');
    box.innerHTML = '';
    const info = l && l.info;
    if (!info) {
        box.append(el('h2', 'guide-info-title', 'Off the air'), el('p', 'guide-info-when', `Channel ${c.number} · ${c.name}`));
        return;
    }
    if (info.poster) {
        const img = el('img', 'guide-info-poster');
        img.src = api.getThumbnailUrl(info.poster);
        img.alt = '';
        box.appendChild(img);
    }
    const text = el('div', 'guide-info-text');
    text.appendChild(el('h2', 'guide-info-title', info.title + (info.year ? ` (${info.year})` : '')));
    if (info.season) text.appendChild(el('div', 'guide-info-episode', episodeLabel(info)));
    // the day too when it isn't today
    const day = new Date(l.start).toDateString() === new Date(serverNow()).toDateString() ? ''
        : new Date(l.start).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' }) + ' ';
    text.appendChild(el('div', 'guide-info-when', `${day}${guideTime(l.start)} – ${guideTime(l.end)} · Channel ${c.number} · ${c.name}`));
    const facts = [info.rating, (info.genres || []).join(', ')].filter(Boolean).join(' · ');
    if (facts) text.appendChild(el('div', 'guide-info-facts', facts));
    const overview = info.episode_overview || info.overview;
    if (overview) text.appendChild(el('p', 'guide-info-overview', overview));
    box.appendChild(text);
}

// Tunes to a channel: plays what it airs now, from where it is.
function tune(number) {
    const changed = guide.channel !== number;
    guide.channel = number;
    localStorage.setItem(guideChannelKey(), String(number));
    markOnNow();
    if (changed) showChannelBadge();
    playGuideNow();
}

async function playGuideNow() {
    const request = ++guide.request;
    clearTimeout(guide.timer);
    let now;
    try {
        now = await api.guideNow(guide.channel, guide.lib);
    } catch (error) {
        if (request !== guide.request) return;
        showGuideCard({ title: 'Please stand by', text: error.message });
        guide.timer = setTimeout(playGuideNow, 30000);
        return;
    }
    if (request !== guide.request) return;
    guide.skew = now.server_time - Date.now();
    // A show that just played to its end on this channel, with the setting on: marked watched
    const ended = guide.now;
    if (ended && ended.kind === 'program' && !ended.channel.kind && ended.channel.number === now.channel.number &&
        now.start >= ended.end && guideMarksWatched()) {
        api.guideWatched(now.channel.number, guide.lib, ended.start).catch(() => {});
    }
    guide.now = now;
    // The details follow the channel when nothing else was picked on it.
    const c = guide.data && guide.data.channels.find(ch => ch.number === now.channel.number);
    if (c && now.program) {
        const listing = (c.listings || []).find(l => l.start <= now.start && now.start < l.end);
        if (listing && (!guide.selected || guide.selected.channel.number !== c.number || guide.selected.listing.end <= serverNow())) {
            guide.selected = { channel: c, listing };
            showGuideDetails(c, listing);
        }
    }
    const kind = now.channel.kind || '';
    setChannelScreen(kind);
    if (kind === 'weather') {
        stopGuideVideo();
        hideGuideCard();
        playMusic(now.music);
        showWeather();
    } else if (kind === 'guide' && now.kind === 'promo') {
        stopGuideVideo();
        playMusic(now.music);
        showPromo(now);
    } else if (now.kind === 'bumper' || !now.path) {
        stopMusic();
        stopGuideVideo();
        showBumper(now);
    } else {
        stopMusic();
        hideGuideCard();
        showPreviewCaption(kind === 'guide' && now.kind === 'preview' ? now : null);
        await playGuideItem(now, request);
    }
    if (request !== guide.request) return;
    scheduleVoiceOver(now, request);
    // What comes next, when this ends (by the server's clock).
    guide.timer = setTimeout(playGuideNow, Math.max(now.end - serverNow(), 0) + 50);
}

// Plays a program or clip from where the channel is in it: the file itself where the browser
// plays it (seeked there), else the server's stream from there (Compatibility (Auto)).
async function playGuideItem(now, request) {
    let codecs = guide.codecs.get(now.path);
    if (!codecs) {
        try {
            codecs = await api.getCodecs(now.path);
            guide.codecs.set(now.path, codecs);
        } catch (error) {
            codecs = null;
        }
        if (request !== guide.request) return;
    }
    // Seconds into the file now: where the server said, plus the time since
    const offset = () => Math.max((now.offset || 0) + (serverNow() - now.server_time) / 1000, 0);
    const track = codecs ? defaultAudioTrack(codecs) : null;
    const original = codecs && !originalProblem(codecs) && (!track || browserPlaysAudio(track));
    guideVideo.muted = guide.muted;
    guide.fileStart = 0; // where the player's time 0 is in the file (a stream's start)
    // The server's stream from where the channel is: its video copied where the browser plays it, else
    // transcoded; the audio kept where the browser plays it. full: all of it transcoded (H.264, stereo AAC).
    const playStream = (full) => {
        const copy = !full && browserVideo(codecs);
        let url = api.getStreamUrl(now.path, copy ? 'audio' : true);
        url += !full && track && KEPT_AUDIO.includes(track.codec) && browserPlaysAudio(track) ? '&audiocodec=copy' : '&audiocodec=aac&channels=2';
        const start = offset();
        guide.fileStart = 0;
        if (start > 1) {
            url += `&start=${start.toFixed(2)}`;
            guide.fileStart = start;
        }
        guideVideo.src = url;
        playGuideVideo();
    };
    // What the browser can't play (a file it can't open, such as a QuickTime .mov in Firefox, or a stream
    // it can't decode) goes on through the server: the stream, then a full transcode, as the player does.
    let step = original ? 'original' : browserVideo(codecs) ? 'copy' : 'full';
    guideVideo.onerror = () => {
        if (request !== guide.request || !guideVideo.getAttribute('src')) return;
        if (step === 'full') {
            guideVideo.onerror = null;
            return;
        }
        step = step === 'original' && browserVideo(codecs) ? 'copy' : 'full';
        playStream(step === 'full');
    };
    if (original) {
        guideVideo.src = api.getStreamUrl(now.path);
        guideVideo.addEventListener('loadedmetadata', () => {
            if (request === guide.request && step === 'original') guideVideo.currentTime = offset();
        }, { once: true });
        playGuideVideo();
    } else {
        playStream(step === 'full');
    }
}

// Plays the Guide's video; autoplay with sound refused, it plays muted (the mute button says how to unmute).
function playGuideVideo() {
    guideVideo.play().catch(error => {
        if (error.name === 'NotAllowedError' && !guideVideo.muted) {
            guide.muted = guideVideo.muted = true;
            updateGuideMute();
            guideVideo.play().catch(() => {});
        }
    });
}

function stopGuideVideo() {
    guideVideo.pause();
    guideVideo.removeAttribute('src');
    showPreviewCaption(null);
}

// ---- Voice-overs on the credits (docs/GUIDE.md) ----

// The program's voice-over, when the server made one: when the player reaches its place in the
// program's file (the player's own position, so it lines up with the picture even after a stall),
// the program's sound lowered under it; over known credits the picture is squeezed beside an "Up
// next" card. Anything from the last item ends first.
function scheduleVoiceOver(now, request) {
    endVoiceOver();
    const vo = now.voice_over;
    if (!vo || now.kind !== 'program') return;
    // where the player is in the file: its time plus where its stream started
    const filePos = () => guideVideo.src && guideVideo.readyState > 0
        ? (guide.fileStart || 0) + guideVideo.currentTime
        : (now.offset || 0) + (serverNow() - now.server_time) / 1000;
    const pending = [];
    const at = (secs, f) => pending.push({ secs, f });
    const poll = setInterval(() => {
        if (request !== guide.request) return clearInterval(poll);
        const pos = filePos();
        for (let i = 0; i < pending.length;) {
            if (pos >= pending[i].secs) pending.splice(i, 1)[0].f();
            else i++;
        }
        if (!pending.length) clearInterval(poll);
    }, 250);
    guide.voiceTimers.push(poll);
    if (vo.squeeze_at) at(vo.squeeze_at, () => squeezeForUpNext(now));
    at(vo.at, () => {
        const voice = document.getElementById('guide-voice');
        const into = filePos() - vo.at;
        if (into >= vo.seconds) return; // long past
        voice.src = api.getThumbnailUrl(vo.url);
        voice.muted = guide.muted;
        voice.addEventListener('loadedmetadata', () => { if (into > 0.5) voice.currentTime = into; }, { once: true });
        voice.play().catch(() => {});
        guideVideo.volume = 0.3; // the program's sound under the announcer
        guide.voiceOver = { channel: now.channel.number, line: vo.line, announcer: vo.announcer };
        showReportVoiceOver();
        at(vo.at + vo.seconds + 0.3, () => {
            guideVideo.volume = 1;
            guide.voiceOver = null;
            showReportVoiceOver();
        });
    });
}

function endVoiceOver() {
    (guide.voiceTimers || []).forEach(clearInterval);
    guide.voiceTimers = [];
    const voice = document.getElementById('guide-voice');
    voice.pause();
    voice.removeAttribute('src');
    guideVideo.volume = 1;
    guide.voiceOver = null;
    document.getElementById('guide-player').classList.remove('guide-player--squeezed');
    document.getElementById('guide-upnext').hidden = true;
    showReportVoiceOver();
}

// The credits squeezed into the corner, and what's next beside them.
function squeezeForUpNext(now) {
    const card = document.getElementById('guide-upnext');
    card.innerHTML = '';
    if (now.next) {
        const logo = el('img', 'guide-upnext-logo');
        logo.src = guideLogoUrl(now.channel, 512, '#ffffff');
        logo.alt = '';
        card.append(logo, el('div', 'guide-upnext-kicker', `Up next · ${guideTime(now.next_start)}`),
            el('div', 'guide-upnext-title', now.next.title));
        if (now.next.season) card.appendChild(el('div', 'guide-upnext-line', episodeLabel(now.next)));
        if (now.next.poster) {
            const poster = el('img', 'guide-upnext-poster');
            poster.src = api.getThumbnailUrl(now.next.poster);
            poster.alt = '';
            card.appendChild(poster);
        }
    }
    card.style.setProperty('--channel-color', now.channel.color || 'var(--button-bg)');
    card.hidden = false;
    document.getElementById('guide-player').classList.add('guide-player--squeezed');
}

// While a voice-over plays, the channel's info has "Report voice-over", for one said wrong.
function showReportVoiceOver() {
    const box = document.getElementById('guide-info');
    let button = document.getElementById('guide-report-voice');
    if (!guide.voiceOver) {
        if (button) button.remove();
        return;
    }
    if (button) return;
    button = el('button', 'btn btn-small btn-secondary guide-report-voice');
    button.id = 'guide-report-voice';
    button.type = 'button';
    button.textContent = 'Report voice-over';
    const vo = guide.voiceOver;
    button.onclick = async () => {
        button.disabled = true;
        try {
            await api.request('/api/guide/voiceover/report', { method: 'POST', body: JSON.stringify(vo) });
            button.textContent = 'Reported';
        } catch (error) {
            button.disabled = false;
            alert('Couldn’t report it: ' + error.message);
        }
    };
    box.appendChild(button);
}

// ---- The channels of their own: the Guide channel and the Weather channel (docs/GUIDE.md) ----

// Shows the screen a channel of its own draws ('guide', 'weather'), or none ('').
function setChannelScreen(kind) {
    const player = document.getElementById('guide-player');
    if (player.dataset.channelKind === kind) return;
    player.dataset.channelKind = kind;
    player.classList.toggle('guide-player--listings', kind === 'guide');
    document.getElementById('guide-listings').hidden = kind !== 'guide';
    document.getElementById('guide-weather').hidden = kind !== 'weather';
    clearInterval(guide.screenTimer);
    guide.screenTimer = null;
    if (kind === 'guide') renderGuideListings();
    if (kind !== 'weather' && kind !== 'guide') stopMusic();
    if (!kind) showPreviewCaption(null);
}

// The music under a card or the Weather channel: its track from where the clock says, credited.
function playMusic(music) {
    const audio = document.getElementById('guide-music');
    if (!music) {
        stopMusic();
        return;
    }
    const at = () => Math.max(music.offset + (serverNow() - guide.now.server_time) / 1000, 0);
    const src = api.getThumbnailUrl(music.url);
    audio.muted = guide.muted;
    if (audio.dataset.src !== src) {
        audio.dataset.src = src;
        audio.src = src;
        audio.addEventListener('loadedmetadata', () => { audio.currentTime = at(); }, { once: true });
    } else if (Math.abs(audio.currentTime - at()) > 3) {
        audio.currentTime = at();
    }
    audio.dataset.credit = music.credit || music.title || '';
    audio.play().catch(() => {});
}

function stopMusic() {
    const audio = document.getElementById('guide-music');
    audio.pause();
    audio.removeAttribute('src');
    delete audio.dataset.src;
    delete audio.dataset.credit;
}

function musicCredit() {
    const credit = document.getElementById('guide-music').dataset.credit;
    return credit ? '♪ ' + credit : '';
}

// When and where a program airs: "Tonight at 8:00 PM", "Tomorrow at 9:00 AM", "Thursday at 7:00 PM".
function airsWhen(ms) {
    const at = new Date(ms), today = new Date(serverNow());
    const days = Math.round((new Date(at.getFullYear(), at.getMonth(), at.getDate()) -
        new Date(today.getFullYear(), today.getMonth(), today.getDate())) / 86400000);
    const time = guideTime(ms);
    if (days === 0) return (at.getHours() >= 17 ? 'Tonight' : 'Today') + ' at ' + time;
    if (days === 1) return 'Tomorrow at ' + time;
    return at.toLocaleDateString([], { weekday: 'long' }) + ' at ' + time;
}

// The Guide channel's card for a program ahead, with music.
function showPromo(now) {
    showPreviewCaption(null);
    const p = now.program, on = now.airs_on;
    if (!p || p.kind === 'channel' || !on) {
        showGuideCard({ logo: guideLogoUrl(now.channel, 512, '#ffffff'), kicker: 'Stay tuned', title: 'The Guide Channel',
            credit: musicCredit(), color: now.channel.color });
        return;
    }
    showGuideCard({
        logo: guideLogoUrl(on, 512, '#ffffff'),
        kicker: `${airsWhen(now.airs)} on ${on.number}`,
        title: p.title,
        text: p.season ? episodeLabel(p) : (p.year ? String(p.year) : ''),
        credit: musicCredit(),
        poster: p.poster,
        color: on.color,
    });
}

// Over a preview: when and where it airs.
function showPreviewCaption(now) {
    const caption = document.getElementById('guide-caption');
    if (!now || !now.program || !now.airs_on) {
        caption.hidden = true;
        return;
    }
    caption.innerHTML = '';
    caption.append(el('div', 'guide-caption-when', `${airsWhen(now.airs)} · ${now.airs_on.number} ${now.airs_on.name}`),
        el('div', 'guide-caption-title', now.program.title + (now.program.season ? ' · ' + episodeLabel(now.program) : '')));
    caption.hidden = false;
}

// The Guide channel's lower half: every channel, now and the next two half hours, scrolling.
function renderGuideListings() {
    const box = document.getElementById('guide-listings');
    box.innerHTML = '';
    const channels = ((guide.data && guide.data.channels) || []).filter(c => c.kind !== 'guide');
    if (!channels.length) return;
    const first = Math.floor(serverNow() / GUIDE_SLOT_MS) * GUIDE_SLOT_MS;
    const slots = [first, first + GUIDE_SLOT_MS, first + 2 * GUIDE_SLOT_MS];
    const head = el('div', 'guide-listings-head');
    head.append(el('div', 'guide-listings-day', new Date(first).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })),
        ...slots.map(t => el('div', 'guide-listings-time', guideTime(t))));
    const roll = el('div', 'guide-listings-roll');
    const rows = () => channels.map(c => {
        const row = el('div', 'guide-listings-row');
        const id = el('div', 'guide-listings-channel');
        const logo = el('img', 'guide-listings-logo');
        logo.src = guideLogoUrl(c, 256, '#ffffff');
        logo.alt = '';
        id.append(logo, el('span', 'guide-listings-number', `${c.number} ${c.name}`));
        row.appendChild(id);
        slots.forEach(t => {
            const l = (c.listings || []).find(x => x.start <= t && t < x.end);
            const title = l && l.info ? l.info.title : '';
            const started = l && l.start < t && l.info && l.info.kind !== 'channel'; // still on from before
            row.appendChild(el('div', 'guide-listings-cell' + (started ? ' guide-listings-cont' : ''), title || '—'));
        });
        return row;
    });
    const list = el('div', 'guide-listings-list');
    // enough copies to fill the box the whole way round (a row is 9% of the picture, the box 38%), so
    // the roll loops without a jump or a gap; it moves by one copy
    const copies = Math.max(2, Math.ceil(38 / (9 * channels.length)) + 1);
    for (let i = 0; i < copies; i++) list.append(...rows());
    list.style.setProperty('--roll', `${-100 / copies}%`);
    list.style.animationDuration = `${channels.length * 3}s`;
    roll.appendChild(list);
    box.append(head, roll);
}

// ---- The Weather channel ----

async function loadWeather() {
    if (guide.weather && Date.now() - guide.weatherAt < 5 * 60 * 1000) return guide.weather;
    try {
        guide.weather = await api.guideWeather();
        guide.weatherAt = Date.now();
    } catch (error) {
        if (!guide.weather) guide.weather = { error: error.message, periods: [], alerts: [] };
    }
    return guide.weather;
}

function weatherIconUrl(name, w = 256) {
    return api.getThumbnailUrl(`/api/guide/weather/icon?name=${encodeURIComponent(name || 'cloud')}&w=${w}&color=ffffff`);
}

// The pages, turned every 10 seconds by the server's clock (every screen shows the same one).
async function showWeather() {
    await loadWeather();
    renderWeather();
    clearInterval(guide.screenTimer);
    guide.screenTimer = setInterval(() => {
        if (Date.now() - guide.weatherAt > 5 * 60 * 1000) loadWeather().then(renderWeather);
        else renderWeather();
    }, 1000);
}

function weatherPages(w) {
    const pages = [];
    if (w.current) pages.push('now');
    if (w.periods && w.periods.length) pages.push('today', 'extended');
    if (w.radar) pages.push('radar');
    return pages;
}

function renderWeather() {
    const box = document.getElementById('guide-weather');
    const w = guide.weather || { periods: [], alerts: [] };
    const pages = weatherPages(w);
    const page = pages.length ? pages[Math.floor(serverNow() / 10000) % pages.length] : 'none';
    const clock = new Date(serverNow()).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' });
    // only the clock changes within a page
    const key = `${page}|${w.updated || 0}|${musicCredit()}`;
    if (box.dataset.key === key) {
        const c = box.querySelector('.weather-clock');
        if (c) c.textContent = clock;
        return;
    }
    box.dataset.key = key;
    box.innerHTML = '';
    const head = el('div', 'weather-head');
    const titles = { now: 'Current Conditions', today: 'Local Forecast', extended: 'Extended Forecast', radar: 'Local Radar', none: 'Local Forecast' };
    head.append(el('div', 'weather-title', titles[page]), el('div', 'weather-place', w.place || ''), el('div', 'weather-clock', clock));
    const body = el('div', 'weather-body weather-' + page);
    const icon = (name, cls) => {
        const img = el('img', cls || 'weather-icon');
        img.src = weatherIconUrl(name);
        img.alt = '';
        return img;
    };
    if (page === 'now') {
        const c = w.current;
        const left = el('div', 'weather-now-main');
        left.append(icon(c.icon, 'weather-icon-big'), el('div', 'weather-now-temp', c.temp != null ? `${c.temp}°` : '—'),
            el('div', 'weather-now-sky', c.conditions || ''));
        const facts = el('div', 'weather-facts');
        const fact = (label, value) => { if (value != null && value !== '') facts.append(el('div', 'weather-fact-label', label), el('div', 'weather-fact', value)); };
        fact('Humidity', c.humidity != null ? `${c.humidity}%` : null);
        fact('Dewpoint', c.dewpoint != null ? `${c.dewpoint}°` : null);
        fact('Wind', c.wind != null ? (c.wind === 0 ? 'Calm' : `${c.wind_dir || ''} ${c.wind} mph`.trim()) : null);
        fact('Gusts', c.gust != null ? `${c.gust} mph` : null);
        fact('Pressure', c.pressure != null ? `${c.pressure.toFixed(2)} in` : null);
        fact('Visibility', c.visibility != null ? `${c.visibility} mi` : null);
        if (c.feels_like != null && c.feels_like !== c.temp) fact('Feels like', `${c.feels_like}°`);
        body.append(left, facts);
    } else if (page === 'today') {
        w.periods.slice(0, 3).forEach(p => {
            const col = el('div', 'weather-period');
            col.append(el('div', 'weather-period-name', p.name), icon(p.icon), el('div', 'weather-period-temp' + (p.daytime ? '' : ' low'),
                `${p.daytime ? 'High' : 'Low'} ${p.temp}°`), el('div', 'weather-period-sky', p.forecast));
            if (p.precip) col.appendChild(el('div', 'weather-period-precip', `${p.precip}% chance`));
            body.appendChild(col);
        });
    } else if (page === 'extended') {
        // each day with the night after it: "Fri 57° / 44°"
        const days = [];
        w.periods.forEach((p, i) => {
            if (!p.daytime) return;
            const night = w.periods[i + 1] && !w.periods[i + 1].daytime ? w.periods[i + 1] : null;
            days.push({ p, night });
        });
        days.slice(1, 6).forEach(({ p, night }) => {
            const col = el('div', 'weather-day');
            col.append(el('div', 'weather-period-name', p.name.split(' ')[0].slice(0, 3)), icon(p.icon),
                el('div', 'weather-period-sky', p.forecast),
                el('div', 'weather-day-temps', `${p.temp}°` + (night ? ` / ${night.temp}°` : '')));
            body.appendChild(col);
        });
    } else if (page === 'radar') {
        const img = el('img', 'weather-radar');
        img.src = api.getThumbnailUrl('/api/guide/weather/radar?v=' + Math.floor(serverNow() / 300000));
        img.alt = 'Radar';
        body.appendChild(img);
    } else {
        body.appendChild(el('div', 'weather-error', w.error || 'The forecast isn’t in yet.'));
    }
    // warnings scroll along the bottom; else the music's credit
    const foot = el('div', 'weather-foot');
    if (w.alerts && w.alerts.length) {
        foot.classList.add('weather-alerts');
        const text = w.alerts.map(a => a.headline || a.event).join('   ✦   ');
        const ticker = el('div', 'weather-ticker', text);
        ticker.style.animationDuration = `${Math.max(15, text.length / 6)}s`;
        foot.appendChild(ticker);
    } else {
        foot.textContent = musicCredit();
    }
    box.append(head, body);
    if (foot.classList.contains('weather-alerts') && musicCredit()) box.appendChild(el('div', 'weather-credit', musicCredit()));
    box.appendChild(foot);
}

// A bumper: the card between programs ("Up next…", or "We'll be right back" for a long gap).
function showBumper(now) {
    if (!now.next) {
        showGuideCard({ title: now.channel.name, text: 'Programming will resume shortly' });
        return;
    }
    const long = now.next_start - serverNow() > 90000;
    showGuideCard({
        logo: guideLogoUrl(now.channel, 512, '#ffffff'),
        kicker: long ? 'We’ll be right back' : `Up next on ${now.channel.number} ${now.channel.name}`,
        title: now.next.title,
        text: [now.next.season ? episodeLabel(now.next) : '', `at ${guideTime(now.next_start)}`].filter(Boolean).join(' · '),
        poster: now.next.poster,
        color: now.channel.color,
    });
}

function showGuideCard({ logo = '', kicker = '', title = '', text = '', poster = '', color = '', credit = '' }) {
    const card = document.getElementById('guide-card');
    card.innerHTML = '';
    card.style.setProperty('--channel-color', color || 'var(--button-bg)');
    if (poster) {
        const img = el('img', 'guide-card-poster');
        img.src = api.getThumbnailUrl(poster);
        img.alt = '';
        card.appendChild(img);
    }
    const words = el('div', 'guide-card-text');
    if (logo) {
        const img = el('img', 'guide-card-logo');
        img.src = logo;
        img.alt = '';
        words.appendChild(img);
    }
    if (kicker) words.appendChild(el('div', 'guide-card-kicker', kicker));
    words.appendChild(el('div', 'guide-card-title', title));
    if (text) words.appendChild(el('div', 'guide-card-line', text));
    if (credit) words.appendChild(el('div', 'guide-card-credit', credit));
    card.appendChild(words);
    card.hidden = false;
}

function hideGuideCard() {
    document.getElementById('guide-card').hidden = true;
}

// The channel's number over the picture for a few seconds after tuning, as a cable box shows it,
// and its logo.
function showChannelBadge() {
    const badge = document.getElementById('guide-osd');
    const c = guide.data && guide.data.channels.find(ch => ch.number === guide.channel);
    badge.textContent = c ? `${c.number}  ${c.name}` : String(guide.channel);
    badge.hidden = false;
    const logo = document.getElementById('guide-osd-logo');
    if (c && c.logo) {
        logo.src = guideLogoUrl(c, 512, '#ffffff');
        logo.hidden = false;
    }
    clearTimeout(showChannelBadge.timer);
    showChannelBadge.timer = setTimeout(() => {
        badge.hidden = true;
        document.getElementById('guide-osd-logo').hidden = true;
    }, 4000);
}

function toggleGuideMute() {
    guide.muted = !guide.muted;
    guideVideo.muted = guide.muted;
    document.getElementById('guide-voice').muted = guide.muted;
    document.getElementById('guide-music').muted = guide.muted;
    updateGuideMute();
}

function updateGuideMute() {
    const button = document.getElementById('guide-mute');
    button.innerHTML = iconSvg(guide.muted ? 'volumeOff' : 'volumeUp');
    button.title = guide.muted ? 'Unmute' : 'Mute';
}

function guideFullscreen() {
    const box = document.getElementById('guide-player');
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    else box.requestFullscreen().catch(() => {});
}

// The next or previous channel in the grid's order.
function surf(step) {
    const channels = (guide.data && guide.data.channels) || [];
    if (!channels.length) return;
    const i = channels.findIndex(c => c.number === guide.channel);
    const next = channels[(i + step + channels.length) % channels.length];
    tune(next.number);
}

// Typing a channel's number tunes to it.
function typeChannelDigit(d) {
    guide.digits = (guide.digits + d).slice(-3);
    const badge = document.getElementById('guide-osd');
    badge.textContent = guide.digits;
    badge.hidden = false;
    clearTimeout(guide.digitTimer);
    guide.digitTimer = setTimeout(() => {
        const n = Number(guide.digits);
        guide.digits = '';
        if ((guide.data.channels || []).some(c => c.number === n)) tune(n);
        else badge.hidden = true;
    }, 1500);
}

function setupGuide() {
    document.getElementById('close-guide-btn').onclick = () => closeScreen('guide');
    const mute = document.getElementById('guide-mute');
    mute.onclick = toggleGuideMute;
    updateGuideMute();
    document.getElementById('guide-fullscreen').innerHTML = iconSvg('fullscreen');
    document.getElementById('guide-fullscreen').onclick = guideFullscreen;
    // A click on the picture (or a bumper's card) goes full screen and back.
    guideVideo.addEventListener('click', guideFullscreen);
    document.getElementById('guide-card').addEventListener('click', guideFullscreen);
    // Live TV doesn't pause: a pause (media keys, the browser's own controls) mutes instead.
    guideVideo.addEventListener('pause', () => {
        if (!guideScreen.classList.contains('active') || !guide.now || guideVideo.ended || !guideVideo.src) return;
        if (!guide.muted) toggleGuideMute();
        guideVideo.play().catch(() => {});
    });
    document.addEventListener('keydown', e => {
        if (!guideScreen.classList.contains('active') || e.target.closest('input, textarea, select')) return;
        if (e.key === ' ' || e.key === 'm') {
            e.preventDefault();
            toggleGuideMute();
        } else if (e.key === 'f') {
            guideFullscreen();
        } else if (/^[0-9]$/.test(e.key)) {
            typeChannelDigit(e.key);
        } else if (document.fullscreenElement && (e.key === 'ArrowUp' || e.key === 'PageUp')) {
            e.preventDefault();
            surf(-1);
        } else if (document.fullscreenElement && (e.key === 'ArrowDown' || e.key === 'PageDown')) {
            e.preventDefault();
            surf(1);
        }
    });
    document.addEventListener('fullscreenchange', () => {
        document.getElementById('guide-fullscreen').innerHTML = iconSvg(document.fullscreenElement ? 'fullscreenExit' : 'fullscreen');
    });
    window.addEventListener('resize', placeTimeLine);
}

setupGuide();

// ---- The voice-over tester (Settings › Server › Guide, admins; docs/GUIDE.md) ----

// Hear a line in an announcer's voice, with or without the fixes; see how Piper spelled it; try a
// fix and save it to guide-voices.yml; and go through the voice-overs reported while watching.
async function loadVoiceTester() {
    const box = document.getElementById('voice-tester');
    if (!box) return;
    let data;
    try {
        data = await api.request('/api/admin/guide/voices');
    } catch (error) {
        box.textContent = '';
        return;
    }
    box.innerHTML = '';
    box.appendChild(el('h4', '', 'Voice-overs'));
    if (!data.enabled) {
        box.appendChild(el('p', 'hardware-status', 'Voice-overs need Piper: set guide.piperCommand (and guide.piperVoices) in config.yml.'));
    }
    box.appendChild(el('p', 'hardware-status', `Announcers, their fixes and lines are in ${data.file}; edits there apply without a restart.`));
    const sample = (data.later_lines[0] || data.lines[0] || '').replace('{next}', 'Spartacus').replace('{channel}', '15')
        .replace('{later_time}', '9').replace('{later}', 'Annie');
    const voice = el('select', 'settings-input');
    data.announcers.forEach(a => voice.appendChild(new Option(a.name, a.name)));
    const line = el('textarea', 'settings-input voice-line');
    line.rows = 2;
    line.value = sample;
    const templates = el('select', 'settings-input');
    templates.appendChild(new Option('A line they say…', ''));
    [...data.lines, ...data.later_lines].forEach(t => templates.appendChild(new Option(t, t)));
    templates.onchange = () => {
        if (!templates.value) return;
        line.value = templates.value.replace('{next}', 'Spartacus').replace('{channel}', '15')
            .replace('{later_time}', '9').replace('{later}', 'Annie');
    };
    const fixes = el('label', 'settings-check');
    const withFixes = el('input');
    withFixes.type = 'checkbox';
    withFixes.checked = true;
    fixes.append(withFixes, ' With the announcer\u2019s fixes');
    const out = el('div', 'voice-out');
    const audio = el('audio');
    audio.controls = true;
    const phonemes = el('div', 'voice-phonemes');
    out.append(audio, phonemes);
    const say = async fix => {
        phonemes.textContent = 'Saying it…';
        try {
            const r = await api.request('/api/admin/guide/voices/try', { method: 'POST',
                body: JSON.stringify({ announcer: voice.value, line: line.value, fixes: withFixes.checked, fix }) });
            audio.src = api.getThumbnailUrl(r.url);
            audio.play().catch(() => {});
            phonemes.textContent = 'Piper: ' + (r.phonemes || '');
        } catch (error) {
            phonemes.textContent = error.message;
        }
    };
    const sayButton = el('button', 'btn btn-small', 'Say it');
    sayButton.type = 'button';
    sayButton.onclick = () => say(null);

    // a fix: the words to match, what to say ([[ phonemes ]] allowed), an optional pace
    const match = el('input', 'settings-input');
    match.placeholder = 'Words to match (next on Channel)';
    const instead = el('input', 'settings-input');
    instead.placeholder = 'Say instead (next on [[ tʃˈæːnəl ]])';
    const pace = el('input', 'settings-input');
    pace.placeholder = 'Pace (1.3: slower)';
    pace.style.width = '120px';
    const fix = () => ({ match: match.value.trim(), say: instead.value.trim(), pace: parseFloat(pace.value) || 0 });
    const tryFix = el('button', 'btn btn-small btn-secondary', 'Try the fix');
    tryFix.type = 'button';
    tryFix.onclick = () => say(fix());
    const saveFix = el('button', 'btn btn-small', 'Save the fix');
    saveFix.type = 'button';
    saveFix.onclick = async () => {
        const f = fix();
        if (!f.match || !f.say) return;
        try {
            await api.request('/api/admin/guide/voices/fix', { method: 'POST', body: JSON.stringify({ announcer: voice.value, ...f }) });
            loadVoiceTester();
        } catch (error) {
            alert('Couldn’t save it: ' + error.message);
        }
    };
    const fixList = el('div', 'voice-fixes');
    const showFixes = () => {
        const a = data.announcers.find(x => x.name === voice.value);
        fixList.innerHTML = '';
        (a && a.fixes || []).forEach(f => fixList.appendChild(el('div', 'voice-fix',
            `“${f.match}” → ${f.say}${f.pace ? ` (pace ${f.pace})` : ''}`)));
        if (!fixList.childElementCount) fixList.appendChild(el('div', 'voice-fix', 'No fixes yet.'));
    };
    voice.onchange = showFixes;
    showFixes();

    const row = (...items) => { const r = el('div', 'voice-row'); r.append(...items); return r; };
    box.append(row(voice, templates), line, row(fixes, sayButton), out,
        el('h5', '', 'Fixes'), fixList, row(match, instead, pace), row(tryFix, saveFix));

    // reported while watching
    box.appendChild(el('h5', '', 'Reported'));
    if (!data.reports.length) box.appendChild(el('p', 'hardware-status', 'None. While a voice-over plays, the channel’s info has Report voice-over.'));
    data.reports.slice().reverse().forEach(r => {
        const item = el('div', 'voice-report');
        item.appendChild(el('div', '', `${new Date(r.time).toLocaleString()} · Channel ${r.channel} · ${r.announcer}${r.user ? ' · ' + r.user : ''}`));
        item.appendChild(el('div', 'voice-report-line', r.line));
        const tryIt = el('button', 'btn btn-small btn-secondary', 'Try it');
        tryIt.type = 'button';
        tryIt.onclick = () => {
            voice.value = r.announcer;
            showFixes();
            line.value = r.line;
            say(null);
        };
        const done = el('button', 'btn btn-small btn-secondary', 'Done');
        done.type = 'button';
        done.onclick = async () => {
            await api.request('/api/admin/guide/voices/report?time=' + r.time, { method: 'DELETE' }).catch(() => {});
            loadVoiceTester();
        };
        item.appendChild(row(tryIt, done));
        box.appendChild(item);
    });
}

// ---- The Guide tab (Settings, admins; docs/GUIDE.md, "The Guide tab") ----

// Typed channels (a type, its libraries, up to how many), each library's Show in Guide and Use for
// Guide Filler, and the schedule's settings: edited here, saved together (PUT /api/admin/guide).
const guideTab = { data: null, typed: [], libraries: {} };

const GUIDE_WHERE = [
    { value: 'on', label: 'Home screen and library' },
    { value: 'libraryOnly', label: 'Library only' },
    { value: 'off', label: 'Off' },
];

async function loadGuideSettings() {
    const message = document.getElementById('guide-save-message');
    try {
        showGuideSettings(await api.getGuideSettings());
    } catch (error) {
        if (message) message.textContent = 'Couldn’t load the Guide’s settings: ' + error.message;
        return;
    }
    if (message) message.textContent = '';
    loadVoiceTester();
}

function showGuideSettings(data) {
    guideTab.data = data;
    guideTab.typed = data.typed_channels.map(t => ({ ...t, libraries: [...t.libraries] }));
    guideTab.libraries = {};
    data.libraries.forEach(l => { guideTab.libraries[l.id] = { show_in_guide: l.show_in_guide || 'off', guide_filler: l.guide_filler }; });
    document.getElementById('guide-prime-time').value = String(data.prime_time || 20);
    document.getElementById('guide-weather-zip').value = data.weather_zip || '';
    const extras = [];
    extras.push(data.music_tracks ? `Music: ${data.music_tracks} track${data.music_tracks === 1 ? '' : 's'}.` : 'Music: none (guide.musicPath, or the setup’s Guide step).');
    extras.push(data.voice_overs ? 'Voice-overs: Piper is set up.' : 'Voice-overs: off (Piper isn’t set up: guide.piperCommand, or the setup’s Guide step).');
    document.getElementById('guide-extras').textContent = extras.join(' ');
    renderTypedChannels();
    renderGuideLibraries();
}

function libraryTypeName(type) {
    return type === 'movie' ? 'Movies' : type === 'tv' ? 'TV Shows' : 'Folders';
}

function renderTypedChannels() {
    const data = guideTab.data;
    const box = document.getElementById('typed-channels');
    box.innerHTML = '';
    if (!guideTab.typed.length) box.appendChild(el('p', 'settings-hint', 'None yet.'));
    guideTab.typed.forEach((t, index) => {
        const card = el('div', 'typed-channel');
        const head = el('div', 'typed-channel-head');
        const type = el('select', 'settings-input');
        data.types.forEach(ty => type.appendChild(new Option(ty.name, ty.id)));
        type.value = t.type;
        type.onchange = () => { t.type = type.value; };
        const max = el('select', 'settings-input');
        for (let n = 1; n <= data.max_channels; n++) max.appendChild(new Option(`Up to ${n} channel${n === 1 ? '' : 's'}`, String(n)));
        max.value = String(t.max || data.max_channels);
        max.onchange = () => { t.max = parseInt(max.value); };
        const remove = el('button', 'btn btn-small btn-secondary', 'Remove');
        remove.type = 'button';
        remove.onclick = () => { guideTab.typed.splice(index, 1); renderTypedChannels(); renderGuideLibraries(); };
        head.append(type, max, remove);
        card.appendChild(head);

        const libs = el('div', 'typed-channel-libraries');
        data.libraries.forEach(l => {
            const label = el('label', 'settings-check');
            const box = el('input');
            box.type = 'checkbox';
            box.checked = t.libraries.includes(l.id);
            // a library is one typed channel's at most
            const other = guideTab.typed.some((o, i) => i !== index && o.libraries.includes(l.id));
            box.disabled = other && !box.checked;
            box.onchange = () => {
                if (box.checked) t.libraries.push(l.id); else t.libraries = t.libraries.filter(id => id !== l.id);
                renderTypedChannels();
                renderGuideLibraries();
            };
            const groups = (l.groups || []).length ? l.groups.join(', ') : 'no group';
            label.append(box, ` ${l.name}`, el('span', 'settings-hint', ` (${libraryTypeName(l.type)}; ${groups}${other ? '; another typed channel has it' : ''})`));
            libs.appendChild(label);
        });
        card.appendChild(libs);

        // Who sees it: the groups that can open every one of its libraries (as chosen now)
        let groups = null;
        t.libraries.forEach(id => {
            const l = data.libraries.find(x => x.id === id);
            const g = l ? (l.groups || []) : [];
            groups = groups === null ? [...g] : groups.filter(x => g.includes(x));
        });
        const seen = !t.libraries.length ? 'Choose its libraries.'
            : groups.length ? `Seen by: ${groups.join(', ')}.` : 'No group can open all of these libraries: no one will see it.';
        card.appendChild(el('p', groups && groups.length ? 'settings-hint' : 'hardware-status', seen));
        const numbers = t.number ? `Numbers ${t.number}–${t.number + 9}. ` : 'Numbered when saved. ';
        const chans = (t.channels || []).map(c => `${c.number} ${c.name}`).join(', ');
        card.appendChild(el('p', 'settings-hint', numbers + (chans ? `Now: ${chans}.` : 'Its channels are made at the next planning, as its content allows.')));
        box.appendChild(card);
    });
    const add = document.getElementById('typed-add');
    add.disabled = guideTab.typed.length >= data.max_typed;
    add.onclick = () => {
        guideTab.typed.push({ type: data.types[0].id, libraries: [], max: data.max_channels, channels: [] });
        renderTypedChannels();
    };
}

function renderGuideLibraries() {
    const data = guideTab.data;
    const box = document.getElementById('guide-libraries');
    box.innerHTML = '';
    const table = el('table', 'guide-libraries-table');
    const head = el('tr');
    ['Library', 'Show in Guide', 'Use for Guide Filler'].forEach(h => head.appendChild(el('th', '', h)));
    table.appendChild(head);
    data.libraries.forEach(l => {
        const cur = guideTab.libraries[l.id];
        const row = el('tr');
        const name = el('td');
        name.append(el('div', '', l.name), el('div', 'settings-hint', libraryTypeName(l.type)));
        const where = el('td');
        const typed = guideTab.typed.some(t => t.libraries.includes(l.id));
        if (typed) {
            where.appendChild(el('span', 'settings-hint', 'Its typed channels'));
        } else if (l.type === 'movie' || l.type === 'tv') {
            const select = el('select', 'settings-input');
            GUIDE_WHERE.forEach(o => select.appendChild(new Option(o.label, o.value)));
            select.value = cur.show_in_guide || 'off';
            select.onchange = () => { cur.show_in_guide = select.value; };
            where.appendChild(select);
        } else {
            where.appendChild(el('span', 'settings-hint', 'Only typed channels (Late Night) take folder libraries'));
        }
        const filler = el('td');
        const check = el('input');
        check.type = 'checkbox';
        check.checked = !!cur.guide_filler;
        check.onchange = () => { cur.guide_filler = check.checked; };
        filler.appendChild(check);
        row.append(name, where, filler);
        table.appendChild(row);
    });
    box.appendChild(table);
}

async function saveGuideSettings() {
    const message = document.getElementById('guide-save-message');
    const zip = document.getElementById('guide-weather-zip').value.trim();
    if (zip && !/^\d{5}$/.test(zip)) {
        message.textContent = 'The weather ZIP code is five digits.';
        return;
    }
    const body = {
        prime_time: parseInt(document.getElementById('guide-prime-time').value) || 20,
        weather_zip: zip,
        libraries: Object.entries(guideTab.libraries).map(([id, l]) => ({ id, show_in_guide: l.show_in_guide === 'off' ? '' : l.show_in_guide, guide_filler: !!l.guide_filler })),
        typed_channels: guideTab.typed.map(t => ({ id: t.id || '', type: t.type, libraries: t.libraries, max: t.max })),
    };
    message.textContent = 'Saving…';
    try {
        showGuideSettings(await api.saveGuideSettings(body));
        message.textContent = 'Saved. The Guide plans again now.';
    } catch (error) {
        message.textContent = 'Couldn’t save: ' + error.message;
    }
}

document.getElementById('guide-save')?.addEventListener('click', saveGuideSettings);

// Plan again from now: every channel from its next program, the next 6 hours too (asked first).
async function replanGuide() {
    const message = document.getElementById('guide-replan-message');
    if (!confirm('Plan every channel again from its next program?\n\nWhat each channel airs now stays. Everything after it, the next 6 hours too, is planned anew, so the Guide everyone sees changes.')) return;
    message.textContent = 'Planning…';
    try {
        const r = await api.replanGuide();
        message.textContent = r.message || 'Planned again.';
        loadGuideSettings();
    } catch (error) {
        message.textContent = 'Couldn’t plan again: ' + error.message;
    }
}

document.getElementById('guide-replan')?.addEventListener('click', replanGuide);
