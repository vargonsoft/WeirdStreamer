package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// CopyTo writes a consistent copy of each database into dir, as users.db and media.db, while the
// server goes on (SQLite's VACUUM INTO: a snapshot at one moment, compacted, without the WAL).
// The files are the server account's only: they hold the password hashes.
func (db *DB) CopyTo(dir string) error {
	for name, conn := range map[string]*sql.DB{"users.db": db.userConn, "media.db": db.mediaConn} {
		path := filepath.Join(dir, name)
		os.Remove(path) // VACUUM INTO won't write over a file
		if _, err := conn.Exec("VACUUM INTO ?", path); err != nil {
			return fmt.Errorf("copying %s: %w", name, err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
	}
	return nil
}
