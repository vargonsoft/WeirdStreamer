package database

import (
	"time"

	"github.com/weirdstreamer/server/internal/models"
)

// Where each user is in each video: their last position, and whether they've watched it. Rows
// are keyed by user and the video's client path (keys.go), in the users database.

const playbackSchema = `
CREATE TABLE IF NOT EXISTS playback (
	user_id INTEGER NOT NULL,
	path TEXT NOT NULL,
	position REAL NOT NULL DEFAULT 0,
	duration REAL NOT NULL DEFAULT 0,
	watched INTEGER NOT NULL DEFAULT 0,
	updated_at INTEGER NOT NULL,
	continue_state INTEGER NOT NULL DEFAULT 0, -- ContinueListed or ContinueRemoved (0: neither)
	PRIMARY KEY (user_id, path)
);
CREATE INDEX IF NOT EXISTS idx_playback_recent ON playback(user_id, updated_at);`

func (db *DB) initPlaybackSchema() error {
	if _, err := db.userConn.Exec(playbackSchema); err != nil {
		return err
	}
	_, _ = db.userConn.Exec("ALTER TABLE playback ADD COLUMN continue_state INTEGER NOT NULL DEFAULT 0")
	return nil
}

// ListedPosition is how far into a video it goes on Continue Watching (seconds): a video only
// opened doesn't. Once on it, a video stays there, whatever position is reported later, until it's
// finished or taken off the list.
const ListedPosition = 10

// A record's continue_state.
const (
	ContinueListed  = 1 // on Continue Watching (in progress), at any position
	ContinueRemoved = 2 // taken off it by the user (an up-next episode stays off too)
)

// PlaybackUpdate is a player's report: where it is in a file, and whether it reached the end.
type PlaybackUpdate struct {
	Position  float64
	Duration  float64
	Finished  bool
	UpdatedAt time.Time
}

// SavePlayback records a position, or that the video was watched (its position then starts over).
// A watched video stays watched when it's played again. With onlyIfNewer, a report older than
// the stored one (an offline report sent late) is ignored.
func (db *DB) SavePlayback(userID int64, path string, u PlaybackUpdate, onlyIfNewer bool) error {
	if err := checkKey(path); err != nil {
		return err
	}
	position, watched := u.Position, 0
	if u.Finished {
		position, watched = 0, 1
	}
	query := `
		INSERT INTO playback (user_id, path, position, duration, watched, updated_at, continue_state) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, path) DO UPDATE SET
			position = excluded.position,
			duration = CASE WHEN excluded.duration > 0 THEN excluded.duration ELSE playback.duration END,
			watched = MAX(playback.watched, excluded.watched),
			updated_at = excluded.updated_at,
			continue_state = CASE
				WHEN excluded.continue_state = 0 AND excluded.watched = 1 THEN 0
				WHEN excluded.continue_state = 1 OR playback.continue_state = 1 OR playback.position >= ? THEN 1
				ELSE playback.continue_state END`
	if onlyIfNewer {
		query += ` WHERE excluded.updated_at > playback.updated_at`
	}
	state := 0
	if !u.Finished && position >= ListedPosition {
		state = ContinueListed
	}
	_, err := db.userConn.Exec(query, userID, path, position, u.Duration, watched, u.UpdatedAt.UnixMilli(), state, ListedPosition)
	return err
}

// SetWatched marks videos watched (from the start next time) or unwatched (no record at all).
func (db *DB) SetWatched(userID int64, paths []string, watched bool) error {
	tx, err := db.userConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for _, path := range paths {
		if err := checkKey(path); err != nil {
			return err
		}
		if watched {
			_, err = tx.Exec(`
				INSERT INTO playback (user_id, path, position, duration, watched, updated_at) VALUES (?, ?, 0, 0, 1, ?)
				ON CONFLICT(user_id, path) DO UPDATE SET position = 0, watched = 1, updated_at = excluded.updated_at, continue_state = 0`,
				userID, path, now)
		} else {
			_, err = tx.Exec(`DELETE FROM playback WHERE user_id = ? AND path = ?`, userID, path)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UserPlayback returns all of a user's records, by client path.
func (db *DB) UserPlayback(userID int64) (map[string]*models.Playback, error) {
	rows, err := db.userConn.Query(`SELECT path, position, duration, watched, updated_at, continue_state FROM playback WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*models.Playback{}
	for rows.Next() {
		var path string
		p := &models.Playback{}
		if err := rows.Scan(&path, &p.Position, &p.Duration, &p.Watched, &p.UpdatedAt, &p.ContinueState); err != nil {
			return nil, err
		}
		out[path] = p
	}
	return out, rows.Err()
}

// RecentPlayback returns a user's records on Continue Watching (a position to resume from, or
// listed before), most recent first.
func (db *DB) RecentPlayback(userID int64, minPosition float64, limit int) ([]string, map[string]*models.Playback, error) {
	rows, err := db.userConn.Query(`
		SELECT path, position, duration, watched, updated_at, continue_state FROM playback
		WHERE user_id = ? AND (position >= ? OR continue_state = 1) ORDER BY updated_at DESC LIMIT ?`, userID, minPosition, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var paths []string
	out := map[string]*models.Playback{}
	for rows.Next() {
		var path string
		p := &models.Playback{}
		if err := rows.Scan(&path, &p.Position, &p.Duration, &p.Watched, &p.UpdatedAt, &p.ContinueState); err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
		out[path] = p
	}
	return paths, out, rows.Err()
}

// RecentFinished returns a user's records of videos finished (watched, with no position to resume
// from), most recent first, for the episodes that follow them on Continue Watching.
func (db *DB) RecentFinished(userID int64, minPosition float64, limit int) ([]string, map[string]*models.Playback, error) {
	rows, err := db.userConn.Query(`
		SELECT path, position, duration, watched, updated_at, continue_state FROM playback
		WHERE user_id = ? AND watched = 1 AND position < ? AND continue_state != 1 ORDER BY updated_at DESC LIMIT ?`, userID, minPosition, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var paths []string
	out := map[string]*models.Playback{}
	for rows.Next() {
		var path string
		p := &models.Playback{}
		if err := rows.Scan(&path, &p.Position, &p.Duration, &p.Watched, &p.UpdatedAt, &p.ContinueState); err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
		out[path] = p
	}
	return paths, out, rows.Err()
}

// ClearPosition forgets where a user stopped in a video (keeping whether it was watched), for
// Continue Watching's remove. A video with no record gets one (no position, not watched): an
// up-next episode removed that way stays off the list until the show is watched further.
func (db *DB) ClearPosition(userID int64, path string) error {
	if err := checkKey(path); err != nil {
		return err
	}
	_, err := db.userConn.Exec(`
		INSERT INTO playback (user_id, path, position, duration, watched, updated_at, continue_state) VALUES (?, ?, 0, 0, 0, ?, 2)
		ON CONFLICT(user_id, path) DO UPDATE SET position = 0, updated_at = excluded.updated_at, continue_state = 2`,
		userID, path, time.Now().UnixMilli())
	return err
}
