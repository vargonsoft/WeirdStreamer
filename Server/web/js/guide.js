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
};

const GUIDE_HOURS = 3.5;
const GUIDE_SLOT_MS = 30 * 60 * 1000;

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

async function showGuide(lib = '', pushHistory = true) {
    guide.lib = lib;
    if (pushHistory) pushScreenHistory('guide');
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
    guide.request++;
    clearTimeout(guide.timer);
    guide.timer = null;
    guide.now = null;
    guide.channel = null;
    guideVideo.pause();
    guideVideo.removeAttribute('src');
    guideVideo.load();
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
}

async function loadGuideGrid() {
    try {
        const data = await api.guideListings(guide.lib, GUIDE_HOURS);
        guide.skew = data.now - Date.now();
        guide.data = data;
    } catch (error) {
        document.getElementById('guide-grid').textContent = 'The Guide couldn’t be loaded: ' + error.message;
        return;
    }
    renderGuideGrid();
    clearInterval(guide.gridTimer);
    guide.gridTimer = setInterval(() => {
        if (serverNow() >= guide.data.from + GUIDE_SLOT_MS) loadGuideGrid();
        else placeTimeLine();
    }, 15000);
}

function renderGuideGrid() {
    const grid = document.getElementById('guide-grid');
    grid.innerHTML = '';
    const { from, to, channels } = guide.data;
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
        const label = el('div', 'guide-time', guideTime(t));
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
        const badge = el('span', 'guide-badge', String(c.number));
        badge.style.background = c.color || '';
        name.append(badge, el('span', 'guide-channel-name', c.name));
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
    text.appendChild(el('div', 'guide-info-when', `${guideTime(l.start)} – ${guideTime(l.end)} · Channel ${c.number} · ${c.name}`));
    const facts = [info.rating, (info.genres || []).join(', ')].filter(Boolean).join(' · ');
    if (facts) text.appendChild(el('div', 'guide-info-facts', facts));
    const overview = info.episode_overview || info.overview;
    if (overview) text.appendChild(el('p', 'guide-info-overview', overview));
    const now = serverNow();
    if (guide.channel === c.number && l.start <= now && now < l.end) {
        const watched = el('button', 'btn btn-small btn-secondary');
        watched.type = 'button';
        watched.innerHTML = iconSvg('check') + '<span class="btn-label">Mark watched</span>';
        watched.onclick = async () => {
            watched.disabled = true;
            try {
                await api.guideWatched(c.number, guide.lib);
                watched.querySelector('.btn-label').textContent = 'Watched';
            } catch (error) {
                watched.disabled = false;
                alert('Couldn’t mark it watched: ' + error.message);
            }
        };
        text.appendChild(watched);
    }
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
    if (now.kind === 'bumper' || !now.path) {
        guideVideo.pause();
        guideVideo.removeAttribute('src');
        showBumper(now);
    } else {
        hideGuideCard();
        await playGuideItem(now, request);
    }
    if (request !== guide.request) return;
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
    const offset = () => Math.max((serverNow() - now.start) / 1000, 0);
    const track = codecs ? defaultAudioTrack(codecs) : null;
    const original = codecs && !originalProblem(codecs) && (!track || browserPlaysAudio(track));
    guideVideo.muted = guide.muted;
    if (original) {
        guideVideo.src = api.getStreamUrl(now.path);
        guideVideo.addEventListener('loadedmetadata', () => {
            if (request === guide.request) guideVideo.currentTime = offset();
        }, { once: true });
    } else {
        const copy = browserVideo(codecs);
        let url = api.getStreamUrl(now.path, copy ? 'audio' : true);
        url += track && KEPT_AUDIO.includes(track.codec) && browserPlaysAudio(track) ? '&audiocodec=copy' : '&audiocodec=aac&channels=2';
        const start = offset();
        if (start > 1) url += `&start=${start.toFixed(2)}`;
        guideVideo.src = url;
    }
    guideVideo.play().catch(error => {
        // Autoplay with sound refused: play muted, and say how to unmute.
        if (error.name === 'NotAllowedError' && !guideVideo.muted) {
            guide.muted = guideVideo.muted = true;
            updateGuideMute();
            guideVideo.play().catch(() => {});
        }
    });
}

// A bumper: the card between programs ("Up next…", or "We'll be right back" for a long gap).
function showBumper(now) {
    if (!now.next) {
        showGuideCard({ title: now.channel.name, text: 'Programming will resume shortly' });
        return;
    }
    const long = now.next_start - serverNow() > 90000;
    showGuideCard({
        kicker: long ? 'We’ll be right back' : `Up next on ${now.channel.number} ${now.channel.name}`,
        title: now.next.title,
        text: [now.next.season ? episodeLabel(now.next) : '', `at ${guideTime(now.next_start)}`].filter(Boolean).join(' · '),
        poster: now.next.poster,
        color: now.channel.color,
    });
}

function showGuideCard({ kicker = '', title = '', text = '', poster = '', color = '' }) {
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
    if (kicker) words.appendChild(el('div', 'guide-card-kicker', kicker));
    words.appendChild(el('div', 'guide-card-title', title));
    if (text) words.appendChild(el('div', 'guide-card-line', text));
    card.appendChild(words);
    card.hidden = false;
}

function hideGuideCard() {
    document.getElementById('guide-card').hidden = true;
}

// The channel's number and name over the picture for a few seconds after tuning.
function showChannelBadge() {
    const badge = document.getElementById('guide-osd');
    const c = guide.data && guide.data.channels.find(ch => ch.number === guide.channel);
    badge.textContent = c ? `${c.number}  ${c.name}` : String(guide.channel);
    badge.hidden = false;
    clearTimeout(showChannelBadge.timer);
    showChannelBadge.timer = setTimeout(() => { badge.hidden = true; }, 4000);
}

function toggleGuideMute() {
    guide.muted = !guide.muted;
    guideVideo.muted = guide.muted;
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
