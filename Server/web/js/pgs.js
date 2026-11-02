// PGS: Blu-ray's picture subtitles, as ffmpeg's raw .sup stream. A file is a list of segments ("PG",
// its time, a type, a size): palettes, objects (run-length pictures) and compositions (which objects
// go where), each display set closed by an end segment. A set with no objects clears the screen.
// Pictures are only decoded when shown: a film's worth of them would take hundreds of MB.
const PGS_PALETTE = 0x14, PGS_OBJECT = 0x15, PGS_COMPOSITION = 0x16, PGS_END = 0x80;

// A palette entry as the bytes of an RGBA pixel, in the BT.709 limited range Blu-rays use.
function pgsColor(y, cr, cb, alpha) {
    const clamp = (v) => Math.max(0, Math.min(255, Math.round(v)));
    const luma = 1.164 * (y - 16), red = cr - 128, blue = cb - 128;
    const r = clamp(luma + 1.793 * red), g = clamp(luma - 0.533 * red - 0.213 * blue), b = clamp(luma + 2.112 * blue);
    return ((alpha << 24) | (b << 16) | (g << 8) | r) >>> 0;
}

// Decodes a .sup stream into its display sets: { time (seconds), width, height, parts }.
function decodePgs(buffer) {
    const bytes = new Uint8Array(buffer), view = new DataView(buffer);
    const palettes = new Map(), objects = new Map(), sets = [];
    let composition = null;
    for (let i = 0; i + 13 <= bytes.length;) {
        if (view.getUint16(i) !== 0x5047) break; // "PG"
        const time = view.getUint32(i + 2) / 90000, type = bytes[i + 10], size = view.getUint16(i + 11);
        const p = i + 13;
        i = p + size;
        if (i > bytes.length) break;
        if (type === PGS_PALETTE) {
            const table = new Uint32Array(palettes.get(bytes[p]) || 256);
            for (let q = p + 2; q + 5 <= i; q += 5) table[bytes[q]] = pgsColor(bytes[q + 1], bytes[q + 2], bytes[q + 3], bytes[q + 4]);
            palettes.set(bytes[p], table);
        } else if (type === PGS_OBJECT) {
            const id = view.getUint16(p);
            if (bytes[p + 3] & 0x80) { // the first of its fragments
                objects.set(id, { width: view.getUint16(p + 7), height: view.getUint16(p + 9), chunks: [bytes.subarray(p + 11, i)] });
            } else if (objects.has(id)) {
                objects.get(id).chunks.push(bytes.subarray(p + 4, i));
            }
        } else if (type === PGS_COMPOSITION) {
            composition = { time, width: view.getUint16(p), height: view.getUint16(p + 2), palette: bytes[p + 9], placed: [] };
            for (let n = bytes[p + 10], q = p + 11; n > 0 && q + 8 <= i; n--) {
                const cropped = bytes[q + 3] & 0x80;
                composition.placed.push({ id: view.getUint16(q), x: view.getUint16(q + 4), y: view.getUint16(q + 6),
                    crop: cropped ? { x: view.getUint16(q + 8), y: view.getUint16(q + 10), width: view.getUint16(q + 12), height: view.getUint16(q + 14) } : null });
                q += cropped ? 16 : 8;
            }
        } else if (type === PGS_END && composition) {
            const palette = palettes.get(composition.palette);
            sets.push({ time: composition.time, width: composition.width, height: composition.height,
                parts: composition.placed.filter(c => palette && objects.has(c.id)).map(c => ({ ...c, object: objects.get(c.id), palette })) });
            composition = null;
        }
    }
    return sets;
}

// A part's pixels. Lines run-length coded: a nonzero byte is one pixel of that color; 0 then a
// flag byte is a run (bit 7: a color follows, else it's clear; bit 6: a longer count), or the line's end.
function pgsPixels(part) {
    const { width, height, chunks } = part.object;
    const pixels = new Uint32Array(width * height);
    let x = 0, y = 0;
    let chunk = 0, at = 0;
    const next = () => {
        while (chunk < chunks.length && at >= chunks[chunk].length) { chunk++; at = 0; }
        return chunk < chunks.length ? chunks[chunk][at++] : -1;
    };
    for (let b = next(); b >= 0 && y < height; b = next()) {
        if (b !== 0) {
            if (x < width) pixels[y * width + x] = part.palette[b];
            x++;
            continue;
        }
        const flags = next();
        if (flags <= 0) { // the line's end
            x = 0;
            y++;
            continue;
        }
        const length = flags & 0x40 ? ((flags & 0x3f) << 8) | next() : flags & 0x3f;
        const color = flags & 0x80 ? part.palette[next()] : 0;
        for (let n = 0; n < length && x < width; n++) pixels[y * width + x++] = color;
    }
    return new ImageData(new Uint8ClampedArray(pixels.buffer), width, height);
}

// Draws a display set on canvas (null clears it).
function drawPgs(canvas, set) {
    if (set && (canvas.width !== set.width || canvas.height !== set.height)) {
        canvas.width = set.width;
        canvas.height = set.height;
    }
    const context = canvas.getContext('2d');
    context.clearRect(0, 0, canvas.width, canvas.height);
    if (!set) return;
    for (const part of set.parts) {
        const image = pgsPixels(part), c = part.crop;
        if (c) context.putImageData(image, part.x - c.x, part.y - c.y, c.x, c.y, c.width, c.height);
        else context.putImageData(image, part.x, part.y);
    }
}
