package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

// Every sign-in is a session: its token names it, and a token whose session is gone is refused.
// Signing out ends the session; a new password ends all of the user's others; deleting the user
// ends them all.
const sessionSchema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	user_id INTEGER NOT NULL,
	created_at DATETIME NOT NULL,
	expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
`

func (db *DB) initSessionSchema() error {
	_, err := db.userConn.Exec(sessionSchema)
	return err
}

// CreateSession starts a session for a user, lasting until expires, and returns its ID. Expired
// sessions are cleared on the way.
func (db *DB) CreateSession(userID int64, expires time.Time) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	now := time.Now().UTC()
	if _, err := db.userConn.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now); err != nil {
		return "", err
	}
	_, err := db.userConn.Exec(`INSERT INTO sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		id, userID, now, expires.UTC())
	return id, err
}

// SessionUser is the user a session belongs to; ok is false for an ended or expired session.
func (db *DB) SessionUser(id string) (userID int64, ok bool, err error) {
	if id == "" {
		return 0, false, nil
	}
	err = db.userConn.QueryRow(`SELECT user_id FROM sessions WHERE id = ? AND expires_at > ?`, id, time.Now().UTC()).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return userID, err == nil, err
}

// EndSession ends one session (signing out).
func (db *DB) EndSession(id string) error {
	_, err := db.userConn.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// EndUserSessions ends every session of a user except keep ("" ends them all).
func (db *DB) EndUserSessions(userID int64, keep string) error {
	_, err := db.userConn.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keep)
	return err
}
