package database

import (
	"database/sql"
	"os"
	"strings"
	"time"
)

// media_added keeps when each file arrived: its time on disk (when it was copied or downloaded
// there), kept here so Recently Added needn't read every file's. media_items can't hold it: its
// rows are rewritten on every save (so their created_at is only the last save).
const mediaAddedSchema = `
CREATE TABLE IF NOT EXISTS media_added (
	path TEXT PRIMARY KEY,
	added_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_media_added ON media_added(added_at);`

// AddedMedia is a video and when it arrived (Unix seconds).
type AddedMedia struct {
	Path    string
	AddedAt int64
}

func recordAdded(conn execer, path string, at time.Time) error {
	_, err := conn.Exec(`INSERT INTO media_added (path, added_at) VALUES (?, ?)
		ON CONFLICT(path) DO UPDATE SET added_at = excluded.added_at`, path, at.Unix())
	return err
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// diskTime is when a file arrived by its time on disk; never later than now (a clock that was
// ahead would keep it first for good). False when it can't be read (gone, or its disk is away).
func diskTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	if t := info.ModTime(); t.Before(time.Now()) {
		return t, true
	}
	return time.Now(), true
}

// recordArrival dates a file by its time on disk (SaveMediaItem).
func (db *DB) recordArrival(path string) error {
	at, ok := diskTime(path)
	if !ok {
		return nil // dated when next seen
	}
	return recordAdded(db.mediaConn, path, at)
}

// DateMediaAdded dates every known file by its time on disk, correcting the dates that differ (a
// file replaced in place, or dated before by when the server first saw it).
func (db *DB) DateMediaAdded() error {
	rows, err := db.mediaConn.Query(`SELECT m.path, a.added_at FROM media_items m
		LEFT JOIN media_added a ON a.path = m.path`)
	if err != nil {
		return err
	}
	type dated struct {
		path string
		at   sql.NullInt64
	}
	var files []dated
	for rows.Next() {
		var f dated
		if err := rows.Scan(&f.path, &f.at); err != nil {
			rows.Close()
			return err
		}
		files = append(files, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, f := range files {
		at, ok := diskTime(f.path)
		if !ok || (f.at.Valid && f.at.Int64 == at.Unix()) {
			continue
		}
		if err := recordAdded(tx, f.path, at); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// NewestMedia lists the videos under root that arrived last: the newest day first, and within a
// day by path, so a batch of episodes starts from its first.
func (db *DB) NewestMedia(root string, limit int) ([]AddedMedia, error) {
	root = strings.TrimRight(root, "/")
	// Every path under root/ sorts between root/ and root0 ('0' follows '/').
	rows, err := db.mediaConn.Query(`SELECT a.path, a.added_at FROM media_added a
		JOIN media_items m ON m.path = a.path
		WHERE m.type = 'video' AND a.path >= ? AND a.path < ?
		ORDER BY date(a.added_at, 'unixepoch', 'localtime') DESC, a.path LIMIT ?`, root+"/", root+"0", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AddedMedia
	for rows.Next() {
		var m AddedMedia
		if err := rows.Scan(&m.Path, &m.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
