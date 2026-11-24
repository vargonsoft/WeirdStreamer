package database

// MoveFile moves what's kept for a library file to its new name (a damaged video replaced by its
// fixed copy, .avi to .mkv): its watch progress, intros and credits, subtitle timing and episode
// matches, by client path. The scanner's records (media_items, media_added: disk paths) are
// dropped instead, for the next scan to add the new file, dated by its time on disk.
func (db *DB) MoveFile(oldDisk, oldClient, newClient string) error {
	if err := checkKey(oldClient); err != nil {
		return err
	}
	if err := checkKey(newClient); err != nil {
		return err
	}
	if _, err := db.userConn.Exec(`UPDATE OR REPLACE playback SET path = ? WHERE path = ?`, newClient, oldClient); err != nil {
		return err
	}
	for _, q := range []string{
		`UPDATE OR REPLACE segments SET path = ? WHERE path = ?`,
		`UPDATE OR REPLACE subtitle_offsets SET path = ? WHERE path = ?`,
		`UPDATE OR REPLACE episode_overrides SET file_path = ? WHERE file_path = ?`,
	} {
		if _, err := db.mediaConn.Exec(q, newClient, oldClient); err != nil {
			return err
		}
	}
	for _, q := range []string{`DELETE FROM media_items WHERE path = ?`, `DELETE FROM media_added WHERE path = ?`} {
		if _, err := db.mediaConn.Exec(q, oldDisk); err != nil {
			return err
		}
	}
	return nil
}
