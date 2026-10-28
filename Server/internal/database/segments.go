package database

import (
	"database/sql"
	"errors"
)

// Each video's intro and credits, for the players' Skip buttons: found by the segment analyzer
// (from chapters, a season's shared audio or dark frames; source says which) or set by hand
// ("manual", which analysis leaves alone). analyzed is the analyzer version that last looked at
// the file (0: not yet), so a better analyzer looks again.

const segmentSchema = `
CREATE TABLE IF NOT EXISTS segments (
	path TEXT PRIMARY KEY,
	intro_start REAL,
	intro_end REAL,
	intro_source TEXT NOT NULL DEFAULT '',
	credits_start REAL,
	credits_end REAL,
	credits_source TEXT NOT NULL DEFAULT '',
	analyzed INTEGER NOT NULL DEFAULT 0
);`

// Shows whose intros and credits a user has the players skip by themselves.
const autoSkipSchema = `
CREATE TABLE IF NOT EXISTS auto_skip (
	user_id INTEGER NOT NULL,
	show_id INTEGER NOT NULL,
	PRIMARY KEY (user_id, show_id)
);`

// Subtitle tracks' timing corrections, shared by every player: by video and track key.
const subtitleOffsetSchema = `
CREATE TABLE IF NOT EXISTS subtitle_offsets (
	path TEXT NOT NULL,
	track TEXT NOT NULL,
	offset_ms INTEGER NOT NULL,
	PRIMARY KEY (path, track)
);`

func (db *DB) initSegmentSchema() error {
	if _, err := db.mediaConn.Exec(segmentSchema); err != nil {
		return err
	}
	if _, err := db.mediaConn.Exec(subtitleOffsetSchema); err != nil {
		return err
	}
	_, err := db.userConn.Exec(autoSkipSchema)
	return err
}

// Span is a part of a video, in seconds, and where it was found.
type Span struct {
	Start, End float64
	Source     string // "chapters", "fingerprint", "black" or "manual"
}

// VideoSegments is a video's intro and credits (nil when it has none), and the analyzer version
// that last looked.
type VideoSegments struct {
	Intro, Credits *Span
	Analyzed       int
}

// GetSegments returns a video's segments (empty when it has no row).
func (db *DB) GetSegments(path string) (VideoSegments, error) {
	var out VideoSegments
	var is, ie, cs, ce sql.NullFloat64
	var isrc, csrc string
	err := db.mediaConn.QueryRow(`SELECT intro_start, intro_end, intro_source, credits_start, credits_end, credits_source, analyzed
		FROM segments WHERE path = ?`, path).Scan(&is, &ie, &isrc, &cs, &ce, &csrc, &out.Analyzed)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if is.Valid && ie.Valid {
		out.Intro = &Span{Start: is.Float64, End: ie.Float64, Source: isrc}
	}
	if cs.Valid && ce.Valid {
		out.Credits = &Span{Start: cs.Float64, End: ce.Float64, Source: csrc}
	}
	return out, nil
}

func spanValues(s *Span) (any, any, string) {
	if s == nil {
		return nil, nil, ""
	}
	return s.Start, s.End, s.Source
}

// SaveAnalysis stores what the analyzer found for a video, keeping a part set by hand.
func (db *DB) SaveAnalysis(path string, intro, credits *Span, version int) error {
	is, ie, isrc := spanValues(intro)
	cs, ce, csrc := spanValues(credits)
	_, err := db.mediaConn.Exec(`
		INSERT INTO segments (path, intro_start, intro_end, intro_source, credits_start, credits_end, credits_source, analyzed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			intro_start = CASE WHEN segments.intro_source = 'manual' THEN segments.intro_start ELSE excluded.intro_start END,
			intro_end = CASE WHEN segments.intro_source = 'manual' THEN segments.intro_end ELSE excluded.intro_end END,
			intro_source = CASE WHEN segments.intro_source = 'manual' THEN 'manual' ELSE excluded.intro_source END,
			credits_start = CASE WHEN segments.credits_source = 'manual' THEN segments.credits_start ELSE excluded.credits_start END,
			credits_end = CASE WHEN segments.credits_source = 'manual' THEN segments.credits_end ELSE excluded.credits_end END,
			credits_source = CASE WHEN segments.credits_source = 'manual' THEN 'manual' ELSE excluded.credits_source END,
			analyzed = excluded.analyzed`,
		path, is, ie, isrc, cs, ce, csrc, version)
	return err
}

// SetManualSegment sets a video's intro or credits by hand ("intro" or "credits"); a nil span
// clears it and has the analyzer look at the file again.
func (db *DB) SetManualSegment(path, kind string, span *Span) error {
	if kind != "intro" && kind != "credits" {
		return errors.New("unknown segment kind")
	}
	if span == nil {
		_, err := db.mediaConn.Exec(`UPDATE segments SET `+kind+`_start = NULL, `+kind+`_end = NULL, `+kind+`_source = '', analyzed = 0 WHERE path = ?`, path)
		return err
	}
	_, err := db.mediaConn.Exec(`
		INSERT INTO segments (path, `+kind+`_start, `+kind+`_end, `+kind+`_source) VALUES (?, ?, ?, 'manual')
		ON CONFLICT(path) DO UPDATE SET `+kind+`_start = excluded.`+kind+`_start, `+kind+`_end = excluded.`+kind+`_end, `+kind+`_source = 'manual'`,
		path, span.Start, span.End)
	return err
}

// AnalyzedVersions returns, for the given videos, the analyzer version that last looked at each.
func (db *DB) AnalyzedVersions() (map[string]int, error) {
	rows, err := db.mediaConn.Query(`SELECT path, analyzed FROM segments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var path string
		var v int
		if err := rows.Scan(&path, &v); err != nil {
			return nil, err
		}
		out[path] = v
	}
	return out, rows.Err()
}

// VideoPaths lists every video's path.
func (db *DB) VideoPaths() ([]string, error) {
	rows, err := db.mediaConn.Query(`SELECT path FROM media_items WHERE type = 'video' ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LargestVideo returns the biggest video under a folder ("" if none).
func (db *DB) LargestVideo(dir string) (string, error) {
	var p string
	// Paths from dir+"/" up to dir+"0" (the byte after '/') are the ones under it.
	err := db.mediaConn.QueryRow(`SELECT path FROM media_items WHERE type = 'video' AND path >= ? AND path < ?
		ORDER BY size DESC LIMIT 1`, dir+"/", dir+"0").Scan(&p)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return p, err
}

// AutoSkip says whether a user has a show's intros and credits skipped by themselves.
func (db *DB) AutoSkip(userID int64, showID int) bool {
	var one int
	err := db.userConn.QueryRow(`SELECT 1 FROM auto_skip WHERE user_id = ? AND show_id = ?`, userID, showID).Scan(&one)
	return err == nil
}

// SetAutoSkip turns a show's auto-skip on or off for a user.
func (db *DB) SetAutoSkip(userID int64, showID int, on bool) error {
	var err error
	if on {
		_, err = db.userConn.Exec(`INSERT OR IGNORE INTO auto_skip (user_id, show_id) VALUES (?, ?)`, userID, showID)
	} else {
		_, err = db.userConn.Exec(`DELETE FROM auto_skip WHERE user_id = ? AND show_id = ?`, userID, showID)
	}
	return err
}

// MediaRecord is a media file's row, as cleanup needs it.
type MediaRecord struct {
	Path         string
	ThumbnailURL string
}

// MediaRecords lists every media file's path and thumbnail.
func (db *DB) MediaRecords() ([]MediaRecord, error) {
	rows, err := db.mediaConn.Query(`SELECT path, COALESCE(thumbnail_url, '') FROM media_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRecord
	for rows.Next() {
		var r MediaRecord
		if err := rows.Scan(&r.Path, &r.ThumbnailURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ForgetMedia removes files' records: their rows, intros and credits, and disc info.
func (db *DB) ForgetMedia(paths []string) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range paths {
		for _, q := range []string{`DELETE FROM media_items WHERE path = ?`, `DELETE FROM segments WHERE path = ?`,
			`DELETE FROM dvd_discs WHERE path = ?`, `DELETE FROM subtitle_offsets WHERE path = ?`} {
			if _, err := tx.Exec(q, p); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// SubtitleOffsets returns a video's subtitle timing corrections (ms), by track key.
func (db *DB) SubtitleOffsets(path string) (map[string]int, error) {
	rows, err := db.mediaConn.Query(`SELECT track, offset_ms FROM subtitle_offsets WHERE path = ?`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var ms int
		if err := rows.Scan(&k, &ms); err != nil {
			return nil, err
		}
		out[k] = ms
	}
	return out, rows.Err()
}

// SetSubtitleOffset sets a subtitle track's timing correction (0 forgets it).
func (db *DB) SetSubtitleOffset(path, track string, ms int) error {
	if ms == 0 {
		_, err := db.mediaConn.Exec(`DELETE FROM subtitle_offsets WHERE path = ? AND track = ?`, path, track)
		return err
	}
	_, err := db.mediaConn.Exec(`INSERT INTO subtitle_offsets (path, track, offset_ms) VALUES (?, ?, ?)
		ON CONFLICT(path, track) DO UPDATE SET offset_ms = excluded.offset_ms`, path, track, ms)
	return err
}
