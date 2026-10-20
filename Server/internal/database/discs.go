package database

import (
	"database/sql"
	"errors"
)

// Probed DVD disc image structure (titles, chapters, tracks), so images aren't probed again
// after a restart. Rows are keyed by path and dropped when the file's size or time changes.

const discSchema = `
CREATE TABLE IF NOT EXISTS dvd_discs (
	path TEXT PRIMARY KEY,
	size INTEGER NOT NULL,
	mod_time INTEGER NOT NULL,
	info TEXT NOT NULL
);`

func (db *DB) initDiscSchema() error {
	_, err := db.mediaConn.Exec(discSchema)
	return err
}

// GetDiscInfo returns the stored info for a disc image of the given size and time, or nil.
func (db *DB) GetDiscInfo(path string, size, modTime int64) ([]byte, error) {
	var info string
	err := db.mediaConn.QueryRow(`SELECT info FROM dvd_discs WHERE path = ? AND size = ? AND mod_time = ?`,
		path, size, modTime).Scan(&info)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return []byte(info), err
}

// SaveDiscInfo stores a disc image's info.
func (db *DB) SaveDiscInfo(path string, size, modTime int64, info []byte) error {
	_, err := db.mediaConn.Exec(`INSERT OR REPLACE INTO dvd_discs (path, size, mod_time, info) VALUES (?, ?, ?, ?)`,
		path, size, modTime, string(info))
	return err
}
