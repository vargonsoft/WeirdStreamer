package database

import (
	"database/sql"
	"os"
	"strings"
	"time"
)

// media_added keeps when each file arrived. It's written once for a file and never replaced, unlike
// media_items, whose rows are rewritten on every save (so their created_at is only the last save).
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

func recordAdded(conn *sql.DB, path string, at time.Time) error {
	_, err := conn.Exec(`INSERT OR IGNORE INTO media_added (path, added_at) VALUES (?, ?)`, path, at.Unix())
	return err
}

// FillMediaAdded dates the files that have no arrival time yet from their time on disk: what was in
// the libraries before arrival times were kept, and what a CLI scan found. Files the server finds
// from here on are dated when it first saves them (SaveMediaItem).
func (db *DB) FillMediaAdded() error {
	fill := func() error {
		rows, err := db.mediaConn.Query(`SELECT path FROM media_items WHERE path NOT IN (SELECT path FROM media_added)`)
		if err != nil {
			return err
		}
		var paths []string
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				rows.Close()
				return err
			}
			paths = append(paths, path)
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
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				continue // gone, or its disk is away: dated when next seen
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO media_added (path, added_at) VALUES (?, ?)`,
				path, info.ModTime().Unix()); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
	if err := fill(); err != nil {
		return err
	}
	// Files saved while that ran have only their time on disk to go on too.
	db.datesNewMedia.Store(true)
	return fill()
}

// NewestMedia lists the videos under root that arrived last, newest first.
func (db *DB) NewestMedia(root string, limit int) ([]AddedMedia, error) {
	root = strings.TrimRight(root, "/")
	// Every path under root/ sorts between root/ and root0 ('0' follows '/').
	rows, err := db.mediaConn.Query(`SELECT a.path, a.added_at FROM media_added a
		JOIN media_items m ON m.path = a.path
		WHERE m.type = 'video' AND a.path >= ? AND a.path < ?
		ORDER BY a.added_at DESC, a.path DESC LIMIT ?`, root+"/", root+"0", limit)
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
