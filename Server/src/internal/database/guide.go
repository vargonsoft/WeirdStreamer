package database

import (
	"database/sql"
	"errors"
	"time"
)

// The Guide's channels and their schedules (internal/guide). A channel keeps its number and name
// while its group of titles exists; slots are what it airs, from a few weeks back (the planner goes
// on from them: each show's next episode) to a few days ahead. Paths are client paths.

const guideSchema = `
CREATE TABLE IF NOT EXISTS guide_channels (
	key TEXT PRIMARY KEY,
	library_id TEXT NOT NULL,
	number INTEGER NOT NULL UNIQUE,
	name TEXT NOT NULL,
	genre TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '',
	active INTEGER NOT NULL DEFAULT 1,
	fingerprint TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS guide_slots (
	channel TEXT NOT NULL,
	start INTEGER NOT NULL,   -- Unix milliseconds
	length INTEGER NOT NULL,  -- milliseconds
	kind TEXT NOT NULL,       -- program, filler, bumper
	title TEXT NOT NULL DEFAULT '',
	item INTEGER NOT NULL DEFAULT 0,
	path TEXT NOT NULL DEFAULT '',
	track TEXT NOT NULL DEFAULT '',
	info TEXT NOT NULL DEFAULT '', -- programs: what the guide shows of them (JSON)
	PRIMARY KEY (channel, start)
);
`

func (db *DB) initGuide() error {
	_, err := db.mediaConn.Exec(guideSchema)
	return err
}

// GuideChannel is a channel as stored.
type GuideChannel struct {
	Key         string
	LibraryID   string
	Number      int
	Name        string
	Genre       string
	Kind        string
	Color       string
	Active      bool
	Fingerprint string // what it was last planned from (the planner starts again from 6 hours ahead when it changes)
}

// GuideSlot is a slot as stored.
type GuideSlot struct {
	Channel string
	Start   time.Time
	Length  time.Duration
	Kind    string
	Title   string
	Item    int
	Path    string
	Track   string
	Info    string
}

// GuideChannels lists the stored channels, by number.
func (db *DB) GuideChannels() ([]GuideChannel, error) {
	rows, err := db.mediaConn.Query(`SELECT key, library_id, number, name, genre, kind, color, active, fingerprint
		FROM guide_channels ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuideChannel
	for rows.Next() {
		var c GuideChannel
		if err := rows.Scan(&c.Key, &c.LibraryID, &c.Number, &c.Name, &c.Genre, &c.Kind, &c.Color, &c.Active, &c.Fingerprint); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddGuideChannel stores a new channel.
func (db *DB) AddGuideChannel(c GuideChannel) error {
	_, err := db.mediaConn.Exec(`INSERT INTO guide_channels (key, library_id, number, name, genre, kind, color, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1)`, c.Key, c.LibraryID, c.Number, c.Name, c.Genre, c.Kind, c.Color)
	return err
}

// SetGuideChannelActive turns a channel on or off (off: its titles are gone or out of the Guide;
// it keeps its number for when they're back).
func (db *DB) SetGuideChannelActive(key string, active bool) error {
	_, err := db.mediaConn.Exec(`UPDATE guide_channels SET active = ? WHERE key = ?`, active, key)
	return err
}

// SetGuideChannelColor changes a channel's badge color.
func (db *DB) SetGuideChannelColor(key, color string) error {
	_, err := db.mediaConn.Exec(`UPDATE guide_channels SET color = ? WHERE key = ?`, color, key)
	return err
}

// SetGuideFingerprint records what a channel was planned from.
func (db *DB) SetGuideFingerprint(key, fingerprint string) error {
	_, err := db.mediaConn.Exec(`UPDATE guide_channels SET fingerprint = ? WHERE key = ?`, fingerprint, key)
	return err
}

// GuideSlots lists a channel's slots that end after from and start before to, in order. kind ""
// lists every kind.
func (db *DB) GuideSlots(channel string, from, to time.Time, kind string) ([]GuideSlot, error) {
	q := `SELECT channel, start, length, kind, title, item, path, track, info FROM guide_slots
		WHERE channel = ? AND start + length > ? AND start < ?`
	args := []any{channel, from.UnixMilli(), to.UnixMilli()}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	rows, err := db.mediaConn.Query(q+` ORDER BY start`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuideSlot
	for rows.Next() {
		var s GuideSlot
		var start, length int64
		if err := rows.Scan(&s.Channel, &start, &length, &s.Kind, &s.Title, &s.Item, &s.Path, &s.Track, &s.Info); err != nil {
			return nil, err
		}
		s.Start, s.Length = time.UnixMilli(start), time.Duration(length)*time.Millisecond
		out = append(out, s)
	}
	return out, rows.Err()
}

// GuideSlotAt is the channel's slot playing at t, nil for none.
func (db *DB) GuideSlotAt(channel string, t time.Time) (*GuideSlot, error) {
	slots, err := db.GuideSlots(channel, t, t.Add(time.Millisecond), "")
	if err != nil || len(slots) == 0 {
		return nil, err
	}
	return &slots[0], nil
}

// LastGuideSlot is the channel's last slot, nil for none.
func (db *DB) LastGuideSlot(channel string) (*GuideSlot, error) {
	var s GuideSlot
	var start, length int64
	err := db.mediaConn.QueryRow(`SELECT channel, start, length, kind, title, item, path, track, info FROM guide_slots
		WHERE channel = ? ORDER BY start DESC LIMIT 1`, channel).
		Scan(&s.Channel, &start, &length, &s.Kind, &s.Title, &s.Item, &s.Path, &s.Track, &s.Info)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	s.Start, s.Length = time.UnixMilli(start), time.Duration(length)*time.Millisecond
	return &s, err
}

// AddGuideSlots stores planned slots.
func (db *DB) AddGuideSlots(slots []GuideSlot) error {
	tx, err := db.mediaConn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO guide_slots (channel, start, length, kind, title, item, path, track, info)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range slots {
		if _, err := stmt.Exec(s.Channel, s.Start.UnixMilli(), s.Length.Milliseconds(), s.Kind, s.Title, s.Item, s.Path,
			s.Track, s.Info); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteGuideSlotsFrom removes a channel's slots starting at or after t (to plan them again).
func (db *DB) DeleteGuideSlotsFrom(channel string, t time.Time) error {
	_, err := db.mediaConn.Exec(`DELETE FROM guide_slots WHERE channel = ? AND start >= ?`, channel, t.UnixMilli())
	return err
}

// DeleteGuideSlotsBefore removes every channel's slots that ended before t.
func (db *DB) DeleteGuideSlotsBefore(t time.Time) error {
	_, err := db.mediaConn.Exec(`DELETE FROM guide_slots WHERE start + length < ?`, t.UnixMilli())
	return err
}
