package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// A person's TMDB details, fetched when someone opens them from a title page's cast and kept like
// titles' (refreshed after a while): a larger photo, the biography and facts, and their credits.

const personDetailsSchema = `
CREATE TABLE IF NOT EXISTS person_details (
	tmdb_id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	biography TEXT NOT NULL DEFAULT '',
	birthday TEXT NOT NULL DEFAULT '',
	deathday TEXT NOT NULL DEFAULT '',
	place_of_birth TEXT NOT NULL DEFAULT '',
	known_for TEXT NOT NULL DEFAULT '',
	photo TEXT NOT NULL DEFAULT '',
	credits TEXT NOT NULL DEFAULT '[]',
	updated_at DATETIME NOT NULL
);
`

// PersonDetails is a person's stored details; Photo is an artwork path ("" for none).
type PersonDetails struct {
	TMDBID       int
	Name         string
	Biography    string
	Birthday     string
	Deathday     string
	PlaceOfBirth string
	KnownFor     string
	Photo        string
	Credits      []PersonCredit
	UpdatedAt    time.Time
}

// PersonCredit is one of a person's credits: a movie or show ("movie" or "show"), when it came out,
// its TMDB poster path, and their part (a character, or a job).
type PersonCredit struct {
	Kind   string `json:"kind"`
	TMDBID int    `json:"tmdb_id"`
	Title  string `json:"title"`
	Date   string `json:"date,omitempty"`
	Poster string `json:"poster,omitempty"`
	Role   string `json:"role,omitempty"`
}

func (db *DB) initPersonDetails() error {
	_, err := db.mediaConn.Exec(personDetailsSchema)
	return err
}

// GetPersonDetails returns a person's stored details, or nil.
func (db *DB) GetPersonDetails(id int) (*PersonDetails, error) {
	p := &PersonDetails{}
	var credits string
	err := db.mediaConn.QueryRow(`SELECT tmdb_id, name, biography, birthday, deathday, place_of_birth, known_for, photo,
		credits, updated_at FROM person_details WHERE tmdb_id = ?`, id).
		Scan(&p.TMDBID, &p.Name, &p.Biography, &p.Birthday, &p.Deathday, &p.PlaceOfBirth, &p.KnownFor, &p.Photo, &credits, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(credits), &p.Credits); err != nil {
		p.Credits = nil
	}
	return p, nil
}

// SavePersonDetails stores a person's details.
func (db *DB) SavePersonDetails(p *PersonDetails) error {
	credits, err := json.Marshal(p.Credits)
	if err != nil {
		return err
	}
	_, err = db.mediaConn.Exec(`INSERT OR REPLACE INTO person_details (tmdb_id, name, biography, birthday, deathday,
		place_of_birth, known_for, photo, credits, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.TMDBID, p.Name, p.Biography, p.Birthday, p.Deathday, p.PlaceOfBirth, p.KnownFor, p.Photo, string(credits), p.UpdatedAt)
	return err
}

// PersonCredited reports whether a person is credited in any stored title (only they can be opened).
func (db *DB) PersonCredited(id int) (bool, error) {
	var n int
	err := db.mediaConn.QueryRow(`SELECT COUNT(*) FROM people WHERE tmdb_id = ?`, id).Scan(&n)
	return n > 0, err
}
